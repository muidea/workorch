package runports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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
	var executionContext map[string]any
	if goalID != "" || dodID != "" {
		contextData, err := activationports.BuildWorkspaceExecutionContextForIDs(ctx, activationports.WorkspaceExecutionContextByIDRequest{
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
		executionContext = contextData
		if effectiveGoalID, ok := executionContext["goalId"].(string); ok && strings.TrimSpace(effectiveGoalID) != "" {
			goalID = strings.TrimSpace(effectiveGoalID)
		}
		if effectiveDoDID, ok := executionContext["doDId"].(string); ok && strings.TrimSpace(effectiveDoDID) != "" {
			dodID = strings.TrimSpace(effectiveDoDID)
		}
	}

	id := common.RunID(strings.TrimSpace(request.RunID))
	wsID := common.WorkspaceID(workspaceID)
	if err := CreateRun(ctx, request.Hub, &runmodel.Run{
		ID:               id,
		WorkspaceID:      wsID,
		GoalID:           common.GoalID(goalID),
		DoDID:            common.DoDID(dodID),
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: executionContext,
		State:            common.RunStatePending,
	}); err != nil {
		return appmodelcore.Run{}, err
	}

	item, err := ReadRun(ctx, request.Hub, id)
	if err != nil {
		return appmodelcore.Run{}, err
	}
	return RunView(item), nil
}
