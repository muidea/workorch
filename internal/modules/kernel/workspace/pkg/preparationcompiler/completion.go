package preparationcompiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/pkg/common"
	"reflect"
	"time"
)

// Recompile from complete original archives. Neither the input assessment nor
// a bootstrap hash by itself proves readiness. Repository reads/writes use the
// same independent gate, including historical records after later mutations.
func CompileCompletion(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (workspace.WorkspacePreparationCompletion, error) {
	if workspace.ValidatePreparationEvidencePersistence(w) != nil || a.Assessment == nil || a.Assessment.Outcome != "checks_passed" || a.NoRunEvaluationVersion != workspace.PreparationNoRunFinalVersion {
		return workspace.WorkspacePreparationCompletion{}, common.ErrConflict
	}
	stale, err := workspace.PreparationOwnerRevalidationStaleForAttempt(w, a, a.CompletedAt)
	if err != nil || stale {
		return workspace.WorkspacePreparationCompletion{}, common.ErrConflict
	}
	epoch := ""
	for _, d := range w.PreparationDirectoryEvidence {
		if d.ContentHash == a.DirectoryEvidenceHash && d.Revalidation != nil {
			epoch = d.Revalidation.OwnerEpoch
		}
	}
	if epoch == "" {
		return workspace.WorkspacePreparationCompletion{}, common.ErrConflict
	}
	stale, err = workspace.PreparationFileRevalidationStaleForAttempt(w, a, a.CompletedAt, epoch)
	if err != nil || stale {
		return workspace.WorkspacePreparationCompletion{}, common.ErrConflict
	}
	compiled, err := CompileOriginalPreparedBootstrap(w, a)
	if err != nil || compiled.Compilation == nil || compiled.FailureCode != "" {
		return workspace.WorkspacePreparationCompletion{}, common.ErrConflict
	}
	c := workspace.WorkspacePreparationCompletion{Version: workspace.PreparationCompletionVersion, RequestID: a.RequestID, PayloadHash: a.PayloadHash, CreationEpoch: a.CreationEpoch, PreparationEpoch: a.PreparationEpoch, Binding: a.Binding, BootstrapHash: compiled.Compilation.BootstrapHash, CompletedAt: a.CompletedAt, ExpiresAt: a.CompletedAt.Add(30 * time.Second), CommittedWorkspaceRevision: a.CommittedWorkspaceRevision}
	raw, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	digest := sha256.Sum256(raw)
	c.CompletionID = "preparation-completion-sha256:" + hex.EncodeToString(digest[:])
	return c, c.ValidateFor(a)
}
func ValidatePreparedPersistence(w *workspace.Workspace) error {
	if err := startuppreparation.ValidatePreparedDefinitionPersistence(w); err != nil {
		return err
	}
	if w == nil {
		return nil
	}
	if w.StartupPreparation == nil {
		if w.FirstExecutionPreparationEvidence != nil {
			return common.ErrConflict
		}
		if w.PreparationFence != nil {
			if w.PreparationFence.Pending != nil && w.PreparationFence.Pending.PreparationCompletionID != "" {
				return common.ErrConflict
			}
			for _, s := range w.PreparationFence.Settled {
				if s.Reservation.PreparationCompletionID != "" {
					return common.ErrConflict
				}
			}
		}
		return nil
	}
	for _, c := range w.StartupPreparation.Completions {
		found := false
		for _, a := range w.StartupPreparation.Attempts {
			if a.RequestID == c.RequestID {
				expected, err := CompileCompletion(w, a)
				if err != nil || !reflect.DeepEqual(expected, c) {
					return common.ErrConflict
				}
				found = true
			}
		}
		if !found {
			return common.ErrConflict
		}
	}
	consumed := false
	if w.PreparationFence != nil {
		var reservations []workspace.WorkspaceFreshIntentReservation
		for _, s := range w.PreparationFence.Settled {
			reservations = append(reservations, s.Reservation)
		}
		if w.PreparationFence.Pending != nil {
			reservations = append(reservations, *w.PreparationFence.Pending)
		}
		for i, r := range reservations {
			if r.PreparationCompletionID == "" {
				continue
			}
			if i != 0 || r.Validate() != nil {
				return common.ErrConflict
			}
			e := w.FirstExecutionPreparationEvidence
			if consumed || e == nil || e.Version != workspace.FirstExecutionPreparationEvidenceVersion || e.WorkspaceID != w.ID || e.RequestID != r.RequestID {
				return common.ErrConflict
			}
			consumed = true
			matched := false
			for _, c := range w.StartupPreparation.Completions {
				if c.CompletionID != r.PreparationCompletionID {
					continue
				}
				if matched || c.CommittedWorkspaceRevision != r.PreparationRevision || r.StartupInputHash != c.Binding.StartupInputHash || r.PreparationConsumedAt.Before(c.CompletedAt) || !r.PreparationConsumedAt.Before(c.ExpiresAt) {
					return common.ErrConflict
				}
				for _, a := range w.StartupPreparation.Attempts {
					if a.RequestID != c.RequestID {
						continue
					}
					original, err := CompileOriginalPreparedBootstrap(w, a)
					if err != nil || original.Compilation == nil {
						return common.ErrConflict
					}
					hash, err := runmodel.ExecutionIntentPayloadHash(w.ID, r.RequestID, original.Compilation.Bootstrap)
					if err != nil || hash != r.RunPayloadHash {
						return common.ErrConflict
					}
					if err := validateFirstExecutionFiles(w, a, c.CompletedAt, r.PreparationConsumedAt, e.Files); err != nil {
						return err
					}
				}
				matched = true
			}
			if !matched {
				return common.ErrConflict
			}
		}
	}
	if !consumed && w.FirstExecutionPreparationEvidence != nil {
		return common.ErrConflict
	}
	return nil
}
