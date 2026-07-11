package activationports

import (
	"context"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func createReplacementWorkspaceExecutionRun(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, oldRunID common.RunID) (common.RunID, error) {
	if workspace == nil || goal == nil || dod == nil || oldRunID == "" {
		return "", common.ErrInvalidArgument
	}
	executionContext, err := workspaceExecutionContext(ctx, request, workspace, goal, dod)
	if err != nil {
		return "", err
	}
	if err := SupersedeRun(ctx, request.Hub, workspace.ID, oldRunID); err != nil {
		return "", err
	}
	return startWorkspaceExecutionRun(ctx, request, workspace.ID, goal.ID, dod.ID, executionContext)
}

func createWorkspaceExecutionRun(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD) (common.RunID, error) {
	if workspace == nil || goal == nil || dod == nil {
		return "", common.ErrInvalidArgument
	}
	executionContext, err := workspaceExecutionContext(ctx, request, workspace, goal, dod)
	if err != nil {
		return "", err
	}
	return startWorkspaceExecutionRun(ctx, request, workspace.ID, goal.ID, dod.ID, executionContext)
}

func workspaceExecutionContext(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD) (map[string]any, error) {
	return BuildWorkspaceExecutionContext(ctx, WorkspaceExecutionContextRequest{
		Hub: request.Hub,

		Workspace:                      workspace,
		Goal:                           goal,
		DoD:                            dod,
		Command:                        request.Command,
		SystemDefaultSoulID:            common.SoulID(strings.TrimSpace(string(request.SystemDefaultSoulID))),
		DefaultDialogLanguage:          strings.TrimSpace(request.DefaultDialogLanguage),
		DefaultEmbeddingProviderLoader: request.DefaultEmbeddingProviderLoader,
		SkillHubCatalog:                request.SkillHubCatalog,
	})
}

func startWorkspaceExecutionRun(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID, executionContext map[string]any) (common.RunID, error) {
	return CreateAndStartWorkspaceExecutionRun(ctx, request.Hub, CreateAndStartWorkspaceExecutionRunRequest{
		WorkspaceID:           workspaceID,
		GoalID:                goalID,
		DoDID:                 dodID,
		ExecutionContext:      executionContext,
		Priority:              request.Command.Priority,
		LLMProviderID:         request.Command.LLMProviderID,
		DefaultDialogLanguage: strings.TrimSpace(request.DefaultDialogLanguage),
	})
}
