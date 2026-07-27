package runports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type CreateRunViewRequest struct {
	Hub                            magicevent.Hub
	WorkspaceID                    string
	RunID                          string
	GoalID                         string
	DoDID                          string
	SystemDefaultSoulID            common.SoulID
	DefaultDialogLanguage          string
	DefaultEmbeddingProviderLoader activationports.DefaultEmbeddingProviderLoader
	SkillHubCatalog                contextregistrycore.SkillHubSkillCatalog
}

func CreateRunView(ctx context.Context, request CreateRunViewRequest) (appmodelcore.Run, error) {
	if request.Hub == nil {
		return appmodelcore.Run{}, common.ErrInvalidArgument
	}
	workspaceID := strings.TrimSpace(request.WorkspaceID)
	goalID := strings.TrimSpace(request.GoalID)
	dodID := strings.TrimSpace(request.DoDID)
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID))

	if err := ValidateRunDefinitions(ctx, request.Hub, workspaceID, goalID, dodID); err != nil {
		return appmodelcore.Run{}, err
	}
	var runtimeBootstrap *runtimecontract.RuntimeBootstrap
	var definitionRevision string
	if goalID != "" || dodID != "" {
		build, err := activationports.BuildWorkspaceRuntimeActivationForIDs(ctx, activationports.WorkspaceRuntimeActivationByIDRequest{
			Hub:                            request.Hub,
			WorkspaceID:                    common.WorkspaceID(workspaceID),
			GoalID:                         common.GoalID(goalID),
			DoDID:                          common.DoDID(dodID),
			SystemDefaultSoulID:            common.SoulID(strings.TrimSpace(string(request.SystemDefaultSoulID))),
			DefaultDialogLanguage:          strings.TrimSpace(request.DefaultDialogLanguage),
			DefaultEmbeddingProviderLoader: request.DefaultEmbeddingProviderLoader,
			SkillHubCatalog:                request.SkillHubCatalog,
		})
		if err != nil {
			return appmodelcore.Run{}, err
		}
		if build.EffectiveGoal == nil || build.EffectiveDoD == nil {
			return appmodelcore.Run{}, common.ErrInvalidArgument
		}
		goalID = string(build.EffectiveGoal.ID)
		dodID = string(build.EffectiveDoD.ID)
		definitionRevision = build.DefinitionRevision
		runtimeBootstrap, err = activationcore.RuntimeV2Bootstrap(build.EffectiveGoal, build.EffectiveDoD, build.DefinitionRevision, build.DefinitionChangeImpact, build.RuntimeActivation)
		if err != nil {
			return appmodelcore.Run{}, err
		}
	}
	id := common.RunID(strings.TrimSpace(request.RunID))
	wsID := common.WorkspaceID(workspaceID)
	if err := CreateRun(ctx, request.Hub, &runmodel.Run{
		ID:                 id,
		WorkspaceID:        wsID,
		GoalID:             common.GoalID(goalID),
		DoDID:              common.DoDID(dodID),
		DefinitionRevision: definitionRevision,
		RuntimeBootstrap:   runtimeBootstrap,
		State:              common.RunStatePending,
	}); err != nil {
		return appmodelcore.Run{}, err
	}

	item, err := ReadRun(ctx, request.Hub, id)
	if err != nil {
		return appmodelcore.Run{}, err
	}
	return RunView(item), nil
}
