package activationports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
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
	if err := materializeWorkspaceDefaultLLMTarget(ctx, request, workspace, activationBuild); err != nil {
		return "", err
	}
	if err := SupersedeRun(ctx, request.Hub, workspace.ID, oldRunID); err != nil {
		return "", err
	}
	return startWorkspaceExecutionRun(ctx, request, workspace, goal, dod, activationBuild)
}

// createFreshActivationReplacementWorkspaceExecutionRun handles the one
// explicit recovery path for a Run whose opaque Runtime V2 aggregate cannot be
// read by this binary. The old Run remains a persisted non-resumable SM-1
// administrative outcome. Unlike ordinary restart, this path must not invoke
// SupersedeRun because that would require Orchestrator to decode the old
// aggregate it has already declared incompatible.
func createFreshActivationReplacementWorkspaceExecutionRun(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD, oldRun *runmodel.Run) (common.RunID, error) {
	if workspace == nil || goal == nil || dod == nil || oldRun == nil || oldRun.ID == "" || oldRun.WorkspaceID != workspace.ID {
		return "", common.ErrInvalidArgument
	}
	activationBuild, err := workspaceRuntimeActivation(ctx, request, workspace, goal, dod)
	if err != nil {
		return "", err
	}
	if err := materializeWorkspaceDefaultLLMTarget(ctx, request, workspace, activationBuild); err != nil {
		return "", err
	}
	if err := ReleaseFreshActivationRunSlot(ctx, request.Hub, oldRun); err != nil {
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
	if err := materializeWorkspaceDefaultLLMTarget(ctx, request, workspace, activationBuild); err != nil {
		return "", err
	}
	return startWorkspaceExecutionRun(ctx, request, workspace, goal, dod, activationBuild)
}

// materializeWorkspaceDefaultLLMTarget turns the first successfully resolved
// activation target into the Workspace owner's next-run preference. The Run
// still seals its own target below; active Run Resume never reaches this path.
func materializeWorkspaceDefaultLLMTarget(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspace *workspacemodel.Workspace, activationBuild WorkspaceRuntimeActivationBuild) error {
	if request.Hub == nil || workspace == nil {
		return common.ErrInvalidArgument
	}
	if workspace.DefaultLLMTarget != nil {
		return nil
	}
	target := activationBuild.RuntimeActivation.ProviderTarget.Clone()
	if err := target.Validate(); err != nil {
		return err
	}
	if _, err := workspaceports.UpdateWorkspaceLLMTarget(ctx, request.Hub, workspace.ID, &target); err != nil {
		return err
	}
	workspace.DefaultLLMTarget = &target
	return nil
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
	lineage := runmodel.RunLineage{LineageState: "active"}
	if rejected := activationBuild.RuntimeActivation.Continuation.RejectedResult; rejected != nil {
		predecessorID := rejected.PredecessorRunID
		if leaf, err := resolveLineageLeafPredecessor(ctx, request.Hub, workspace.ID, predecessorID); err == nil && leaf != "" {
			predecessorID = leaf
		}
		lineage.PredecessorRunID = predecessorID
		lineage.PredecessorResultID = rejected.ResultID
	} else if replan := activationBuild.RuntimeActivation.Continuation.Replan; replan != nil {
		predecessorID := replan.PredecessorRunID
		if leaf, err := resolveLineageLeafPredecessor(ctx, request.Hub, workspace.ID, predecessorID); err == nil && leaf != "" {
			predecessorID = leaf
		}
		lineage.PredecessorRunID = predecessorID
	}
	return CreateAndStartWorkspaceExecutionRun(ctx, request.Hub, CreateAndStartWorkspaceExecutionRunRequest{
		WorkspaceID:        workspace.ID,
		GoalID:             goal.ID,
		DoDID:              dod.ID,
		DefinitionRevision: bootstrap.Definition.Revision,
		Priority:           request.Command.Priority,
		RuntimeBootstrap:   bootstrap,
		Lineage:            lineage,
	})
}
