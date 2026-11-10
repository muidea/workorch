package workspace

import (
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Read the owner-committed outcomes and their original source associations.
// New completions are adjudicated in the private completion reducer; a query,
// unrelated event, or recovery does not compile the historical mapping again.
func readWorkspaceAuthoringCommitted(w *workspacemodel.Workspace) error {
	if w == nil {
		return common.ErrInvalidArgument
	}
	if err := w.ReadCommittedAuthoring(); err != nil {
		return err
	}
	return w.ValidatePreparationDiagnosticHistory()
}
