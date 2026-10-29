package preparationcompiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	if w == nil || w.StartupPreparation == nil {
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
	return nil
}
