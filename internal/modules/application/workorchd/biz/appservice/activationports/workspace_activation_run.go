package activationports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func createReplacementWorkspaceExecutionRun(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, oldRunID common.RunID) (common.RunID, error) {
	if workspace == nil || goal == nil || dod == nil || oldRunID == "" {
		return "", common.ErrInvalidArgument
	}
	activationBuild, err := workspaceRuntimeActivation(ctx, request, workspace, goal, dod)
	if err != nil {
		return "", err
	}
	if err := SupersedeRun(ctx, request.Hub, workspace.ID, oldRunID); err != nil {
		return "", err
	}
	return startWorkspaceExecutionRun(ctx, request, workspace, goal, dod, activationBuild)
}

func createWorkspaceExecutionRun(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD) (common.RunID, error) {
	if workspace == nil || goal == nil || dod == nil {
		return "", common.ErrInvalidArgument
	}
	activationBuild, err := workspaceRuntimeActivation(ctx, request, workspace, goal, dod)
	if err != nil {
		return "", err
	}
	return startWorkspaceExecutionRun(ctx, request, workspace, goal, dod, activationBuild)
}

func workspaceRuntimeActivation(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD) (WorkspaceRuntimeActivationBuild, error) {
	return BuildWorkspaceRuntimeActivation(ctx, WorkspaceRuntimeActivationRequest{
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

func startWorkspaceExecutionRun(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, activationBuild WorkspaceRuntimeActivationBuild) (common.RunID, error) {
	if workspace == nil || goal == nil || dod == nil || activationBuild.EffectiveGoal == nil || activationBuild.EffectiveDoD == nil {
		return "", common.ErrInvalidArgument
	}
	bootstrap, err := activationcore.RuntimeV2Bootstrap(
		activationBuild.EffectiveGoal,
		activationBuild.EffectiveDoD,
		activationBuild.DefinitionRevision,
		activationBuild.DefinitionChangeImpact,
		activationBuild.RuntimeActivation,
	)
	if err != nil {
		return "", err
	}
	return CreateAndStartWorkspaceExecutionRun(ctx, request.Hub, CreateAndStartWorkspaceExecutionRunRequest{
		WorkspaceID:        workspace.ID,
		GoalID:             goal.ID,
		DoDID:              dod.ID,
		DefinitionRevision: bootstrap.Definition.Revision,
		Priority:           request.Command.Priority,
		RuntimeBootstrap:   bootstrap,
	})
}
