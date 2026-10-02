package workspace

import (
	"context"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) queryAuthoringDiagnostic(ctx context.Context, command workspaceevents.QueryAuthoringDiagnosticCommand) (workspaceevents.QueryAuthoringDiagnosticResult, error) {
	if command.Validate() != nil || m == nil || m.workspaces == nil || m.manager == nil || ctx == nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, common.ErrInvalidArgument
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	m.templateMutationMu.Lock()
	defer m.templateMutationMu.Unlock()
	bound, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, err
	}
	w, err := m.workspaces.Get(bound, command.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, err
	}
	if err := workspaceDeletionRejectsMutation(w); err != nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, err
	}
	if w.DefinitionAuthoring == nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, common.ErrNotFound
	}
	if err := w.DefinitionAuthoring.Validate(); err != nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, common.ErrConflict
	}
	if w.ID != command.WorkspaceID || w.ValidateDefinitionAuthoringHistory() != nil {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, common.ErrConflict
	}
	if w.DefinitionAuthoring.Input.WorkspaceID != command.WorkspaceID {
		return workspaceevents.QueryAuthoringDiagnosticResult{}, common.ErrConflict
	}
	for _, request := range w.DefinitionAuthoring.SpecificationRequests {
		if request.PermitID != command.PermitID {
			continue
		}
		if request.FailureCode != "invalid_proposal" && request.FailureCode != "invalid_review" {
			return workspaceevents.QueryAuthoringDiagnosticResult{}, common.ErrNotFound
		}
		if request.Diagnostic == nil {
			return workspaceevents.QueryAuthoringDiagnosticResult{}, common.ErrNotFound
		}
		result := workspaceevents.QueryAuthoringDiagnosticResult{WorkspaceID: command.WorkspaceID, PermitID: request.PermitID, Phase: request.Phase, FailureCode: request.FailureCode, Diagnostic: request.Diagnostic.Clone()}
		if err := result.ValidateFor(command); err != nil {
			return workspaceevents.QueryAuthoringDiagnosticResult{}, common.ErrConflict
		}
		return result, nil
	}
	return workspaceevents.QueryAuthoringDiagnosticResult{}, common.ErrNotFound
}
