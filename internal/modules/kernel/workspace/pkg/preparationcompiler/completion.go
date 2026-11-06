package preparationcompiler

import (
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"reflect"
	"time"
)

// The private Workspace settlement compiles a NEW completion from complete
// owner observations. Historical persistence validation below consumes this
// result; it must not rerun completion adjudication with deployment defaults.
func CompileCompletion(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (workspace.WorkspacePreparationCompletion, error) {
	if workspace.ValidatePreparationEvidencePersistence(w) != nil {
		return workspace.WorkspacePreparationCompletion{}, rejectCompletion("evidence_invalid", "workspace", "evidence", common.ErrConflict)
	}
	return compileCompletion(w, a)
}

func compileCompletion(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (workspace.WorkspacePreparationCompletion, error) {
	source, err := compileOriginalPreparedSource(w, a)
	if err != nil {
		return workspace.WorkspacePreparationCompletion{}, rejectCompletion("original_source_rejected", "workspace", "source", err)
	}
	compiled, err := source.bootstrap(w, a)
	if err != nil {
		return workspace.WorkspacePreparationCompletion{}, rejectCompletion("bootstrap_rejected", "workspace", "bootstrap", err)
	}
	return compileCompletionWithBootstrap(w, a, compiled)
}

func compileCompletionWithBootstrap(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt, compiled OriginalPreparedBootstrap) (workspace.WorkspacePreparationCompletion, error) {
	if a.Assessment == nil || a.Assessment.Outcome != "checks_passed" || a.NoRunEvaluationVersion != workspace.PreparationNoRunFinalVersion {
		return workspace.WorkspacePreparationCompletion{}, rejectCompletion("checks_incomplete", "workspace", "checks", common.ErrConflict)
	}
	stale, err := workspace.PreparationOwnerRevalidationStaleForAttempt(w, a, a.CompletedAt)
	if err != nil || stale {
		owner := "workspace"
		code := "owner_observation_incomplete"
		for _, e := range w.PreparationSealEvidence {
			if e.Ref != a.SealEvidenceRef {
				continue
			}
			r := e.FinalOwners
			if r != nil {
				code = "owner_observation_changed"
				owner = r.MismatchOwner(w, a)
				if owner == "" {
					owner = "workspace"
				}
				if r.MatchesOriginal(w, a) && (a.CompletedAt.Before(r.CompletedAt) || !a.CompletedAt.Before(r.StartedAt.Add(workspace.PreparationOwnerRevalidationMaxAge))) {
					code = "owner_observation_expired"
				}
			}
			switch {
			case r == nil:
				code = "owner_observation_incomplete"
			case r.Souls == nil:
				owner, code = "soul", "owner_observation_incomplete"
			case r.Skills == nil:
				owner, code = "skill", "owner_observation_incomplete"
			case r.Memory == nil || r.Memory.SelectionSource == nil:
				owner, code = "memory", "owner_observation_incomplete"
			case r.MCP == nil:
				owner, code = "mcpregistry", "owner_observation_incomplete"
			}
		}
		return workspace.WorkspacePreparationCompletion{}, rejectCompletion(code, owner, "final_owner_revalidation", err)
	}
	epoch := ""
	for _, d := range w.PreparationDirectoryEvidence {
		if d.ContentHash == a.DirectoryEvidenceHash && d.Revalidation != nil {
			epoch = d.Revalidation.OwnerEpoch
		}
	}
	if epoch == "" {
		return workspace.WorkspacePreparationCompletion{}, rejectCompletion("file_epoch_unavailable", "file", "final_file_revalidation", common.ErrConflict)
	}
	stale, err = workspace.PreparationFileRevalidationStaleForAttempt(w, a, a.CompletedAt, epoch)
	if err != nil || stale {
		return workspace.WorkspacePreparationCompletion{}, rejectCompletion("file_observation_changed_or_expired", "file", "final_file_revalidation", err)
	}
	if compiled.Compilation == nil || compiled.FailureCode != "" {
		return workspace.WorkspacePreparationCompletion{}, rejectCompletion("bootstrap_rejected", "workspace", "bootstrap", common.ErrConflict)
	}
	c := workspace.WorkspacePreparationCompletion{Version: workspace.PreparationCompletionVersion, RequestID: a.RequestID, PayloadHash: a.PayloadHash, CreationEpoch: a.CreationEpoch, PreparationEpoch: a.PreparationEpoch, Binding: a.Binding, BootstrapHash: compiled.Compilation.BootstrapHash, CompletedAt: a.CompletedAt, ExpiresAt: a.CompletedAt.Add(30 * time.Second), CommittedWorkspaceRevision: a.CommittedWorkspaceRevision}
	c.CompletionID = c.Hash()
	return c, c.ValidateFor(a)
}
func ValidatePreparedPersistence(w *workspace.Workspace) error {
	if w == nil {
		return common.ErrInvalidArgument
	}
	if err := workspace.ValidatePreparationEvidencePersistence(w); err != nil {
		return err
	}
	if err := w.PreparationFence.Validate(w.ID); err != nil {
		return err
	}
	// Definition source authentication is independent of execution defaults.
	// It does not compile a Bootstrap or reconsider a completion's readiness.
	if w.StartupPreparation != nil {
		if err := w.StartupPreparation.Validate(w.ID); err != nil {
			return err
		}
		for _, a := range w.StartupPreparation.Attempts {
			if a.DefinitionCompilation == nil {
				continue
			}
			source, err := compileOriginalPreparedDefinitionSource(w, a)
			if err != nil || !reflect.DeepEqual(source.definition, *a.DefinitionCompilation) {
				return common.ErrConflict
			}
		}
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
	// Authenticate original evidence and result associations, not a new
	// completion judgment. The bootstrap compiler uses current defaults and is
	// deliberately absent from this path, including consumed first executions.
	for _, c := range w.StartupPreparation.Completions {
		found := false
		for _, a := range w.StartupPreparation.Attempts {
			if a.RequestID == c.RequestID {
				if c.ValidateFor(a) != nil || c.CompletionID != c.Hash() {
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
					// RunPayloadHash was bound by the reservation reducer before
					// CAS. The Run owner authenticates its actual Bootstrap and
					// receipt; the Workspace fence checks exact receipt identity.
					// Never recreate that payload with today's Bootstrap defaults.
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
