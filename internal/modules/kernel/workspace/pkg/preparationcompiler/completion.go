package preparationcompiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"reflect"
	"time"
)

// Recompile from complete original archives. Neither the input assessment nor
// a bootstrap hash by itself proves readiness. Repository reads/writes use the
// same independent gate, including historical records after later mutations.
func CompileCompletion(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (workspace.WorkspacePreparationCompletion, error) {
	if workspace.ValidatePreparationEvidencePersistence(w) != nil {
		return workspace.WorkspacePreparationCompletion{}, common.ErrConflict
	}
	return compileCompletion(w, a)
}

func compileCompletion(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (workspace.WorkspacePreparationCompletion, error) {
	source, err := compileOriginalPreparedSource(w, a)
	if err != nil {
		return workspace.WorkspacePreparationCompletion{}, err
	}
	compiled, err := source.bootstrap(w, a)
	if err != nil {
		return workspace.WorkspacePreparationCompletion{}, err
	}
	return compileCompletionWithBootstrap(w, a, compiled)
}

func compileCompletionWithBootstrap(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt, compiled OriginalPreparedBootstrap) (workspace.WorkspacePreparationCompletion, error) {
	if a.Assessment == nil || a.Assessment.Outcome != "checks_passed" || a.NoRunEvaluationVersion != workspace.PreparationNoRunFinalVersion {
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
	if compiled.Compilation == nil || compiled.FailureCode != "" {
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
	_, err := validatePreparedPersistence(w)
	return err
}

// All products below are local variables derived from the same complete,
// immutable input. No cross-invocation cache or credential is constructed.
func validatePreparedPersistence(w *workspace.Workspace) (map[string]OriginalPreparedBootstrap, error) {
	products := make(map[string]OriginalPreparedBootstrap)
	if w == nil {
		return nil, common.ErrInvalidArgument
	}
	sources := make(map[string]originalPreparedSource)
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			if a.DefinitionCompilation == nil {
				continue
			}
			source, err := compileOriginalPreparedDefinitionSource(w, a)
			if err != nil || !reflect.DeepEqual(source.definition, *a.DefinitionCompilation) {
				return nil, common.ErrConflict
			}
			sources[a.RequestID] = source
		}
	}
	if w.StartupPreparation == nil {
		if w.FirstExecutionPreparationEvidence != nil {
			return nil, common.ErrConflict
		}
		if w.PreparationFence != nil {
			if w.PreparationFence.Pending != nil && w.PreparationFence.Pending.PreparationCompletionID != "" {
				return nil, common.ErrConflict
			}
			for _, s := range w.PreparationFence.Settled {
				if s.Reservation.PreparationCompletionID != "" {
					return nil, common.ErrConflict
				}
			}
		}
		return products, nil
	}
	// All completions below share exactly this invocation's immutable archive.
	// Validate it once, then continue privately; no cross-call cache or success
	// credential is retained. Definition validation above remains first.
	if err := workspace.ValidatePreparationEvidencePersistence(w); err != nil {
		return nil, err
	}
	for _, c := range w.StartupPreparation.Completions {
		found := false
		for _, a := range w.StartupPreparation.Attempts {
			if a.RequestID == c.RequestID {
				source, foundSource := sources[a.RequestID]
				if !foundSource {
					return nil, common.ErrConflict
				}
				original, err := source.bootstrap(w, a)
				if err != nil {
					return nil, err
				}
				expected, err := compileCompletionWithBootstrap(w, a, original)
				if err != nil || !reflect.DeepEqual(expected, c) {
					return nil, common.ErrConflict
				}
				products[a.RequestID] = original
				found = true
			}
		}
		if !found {
			return nil, common.ErrConflict
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
				return nil, common.ErrConflict
			}
			e := w.FirstExecutionPreparationEvidence
			if consumed || e == nil || e.Version != workspace.FirstExecutionPreparationEvidenceVersion || e.WorkspaceID != w.ID || e.RequestID != r.RequestID {
				return nil, common.ErrConflict
			}
			consumed = true
			matched := false
			for _, c := range w.StartupPreparation.Completions {
				if c.CompletionID != r.PreparationCompletionID {
					continue
				}
				if matched || c.CommittedWorkspaceRevision != r.PreparationRevision || r.StartupInputHash != c.Binding.StartupInputHash || r.PreparationConsumedAt.Before(c.CompletedAt) || !r.PreparationConsumedAt.Before(c.ExpiresAt) {
					return nil, common.ErrConflict
				}
				for _, a := range w.StartupPreparation.Attempts {
					if a.RequestID != c.RequestID {
						continue
					}
					original, exists := products[a.RequestID]
					if !exists || original.Compilation == nil {
						return nil, common.ErrConflict
					}
					hash, err := runmodel.ExecutionIntentPayloadHash(w.ID, r.RequestID, original.Compilation.Bootstrap)
					if err != nil || hash != r.RunPayloadHash {
						return nil, common.ErrConflict
					}
					if err := validateFirstExecutionFiles(w, a, c.CompletedAt, r.PreparationConsumedAt, e.Files); err != nil {
						return nil, err
					}
				}
				matched = true
			}
			if !matched {
				return nil, common.ErrConflict
			}
		}
	}
	if !consumed && w.FirstExecutionPreparationEvidence != nil {
		return nil, common.ErrConflict
	}
	return products, nil
}
