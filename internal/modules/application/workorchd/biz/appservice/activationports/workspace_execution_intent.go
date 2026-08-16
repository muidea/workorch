package activationports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CompileWorkspaceExecutionRestartBootstrap is the application's narrow
// restart compiler. It only reads owner snapshots and produces an immutable
// Runtime bootstrap; it never creates, starts, continues, supersedes, or
// releases a Run/Workspace slot.
func CompileWorkspaceExecutionRestartBootstrap(ctx context.Context, request ActivateWorkspaceExecutionViewRequest) (*runtimecontract.RuntimeBootstrap, error) {
	if request.Hub == nil || strings.TrimSpace(request.WorkspaceID) == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.WorkspaceID(strings.TrimSpace(request.WorkspaceID))
	workspace, err := workspaceports.ReadWorkspaceRecord(ctx, request.Hub, workspaceID)
	if err != nil {
		return nil, err
	}
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, workspace.ID)
	if err != nil {
		return nil, err
	}
	toolHints := WorkspaceToolContextHintsFor(ctx, request.Hub, workspace)
	if err := workspacecore.EnsureManagedConfigs(workspace.InternalDirectory, toolHints.Template, toolHints.Workspace); err != nil {
		return nil, err
	}
	goal, dod, err := selectWorkspaceExecutionDefinitions(ctx, request.Hub, workspace.ID)
	if err != nil {
		return nil, err
	}
	activationBuild, err := workspaceRuntimeActivation(ctx, request, workspace, goal, dod)
	if err != nil {
		return nil, err
	}
	bootstrap, err := activationcore.RuntimeBootstrap(
		activationBuild.EffectiveGoal,
		activationBuild.EffectiveDoD,
		activationBuild.DefinitionRevision,
		activationBuild.DefinitionChangeImpact,
		activationBuild.RuntimeActivation,
	)
	if err != nil {
		return nil, err
	}
	return bootstrap, nil
}

// workspaceRuntimeActivation is the compiler-local projection from immutable
// owner snapshots to an activation context.  It deliberately has no Run or
// Workspace lifecycle side effect; lifecycle admission belongs to the
// Workspace and Run owners after the compiler returns its sealed bootstrap.
func workspaceRuntimeActivation(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspace *workspacemodel.Workspace, goal *workspacemodel.Goal, dod *workspacemodel.DoD) (workspaceRuntimeActivationBuild, error) {
	return buildWorkspaceRuntimeActivation(ctx, workspaceRuntimeActivationRequest{
		Hub: request.Hub,

		Workspace:                      workspace,
		Goal:                           goal,
		DoD:                            dod,
		SystemDefaultSoulID:            common.SoulID(strings.TrimSpace(string(request.SystemDefaultSoulID))),
		DefaultDialogLanguage:          strings.TrimSpace(request.DefaultDialogLanguage),
		DefaultEmbeddingProviderLoader: request.DefaultEmbeddingProviderLoader,
		SkillHubCatalog:                request.SkillHubCatalog,
	})
}
