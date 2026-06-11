package activationports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type ActivateWorkspaceExecutionViewRequest struct {
	Hub                            events.Hub
	WorkspaceID                    string
	Command                        appmodelcore.ActivateWorkspaceExecutionCommand
	SystemDefaultSoulID            common.SoulID
	DefaultDialogLanguage          string
	DefaultEmbeddingProviderLoader DefaultEmbeddingProviderLoader
	SkillHubCatalog                contextregistrycore.SkillHubSkillCatalog
}

func ActivateWorkspaceExecutionView(ctx context.Context, request ActivateWorkspaceExecutionViewRequest) (readmodelcore.WorkspaceExecutionActivation, error) {
	if request.Hub == nil {
		return readmodelcore.WorkspaceExecutionActivation{}, common.ErrInvalidArgument
	}
	workspaceID := common.WorkspaceID(strings.TrimSpace(request.WorkspaceID))
	workspace, err := workspaceports.ReadWorkspaceRecord(ctx, request.Hub, workspaceID)
	if err != nil {
		return readmodelcore.WorkspaceExecutionActivation{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspace.ID)
	toolHints := WorkspaceToolContextHintsFor(ctx, request.Hub, workspace)
	if err := workspacecore.EnsureManagedConfigs(workspace.InternalDirectory, toolHints.Template, toolHints.Workspace); err != nil {
		return readmodelcore.WorkspaceExecutionActivation{}, err
	}

	cmd := request.Command
	if workspace.ActiveRunID != "" {
		run, err := ownerreadports.ReadRunByOwnerCommand(ctx, request.Hub, workspace.ActiveRunID)
		if err != nil {
			return readmodelcore.WorkspaceExecutionActivation{}, err
		}
		if !activationcore.IsTerminalRunState(run.State) {
			goal, dod, replaceErr := SelectWorkspaceExecutionDefinitions(ctx, request.Hub, workspaceID, common.GoalID(cmd.GoalID), common.DoDID(cmd.DoDID))
			if replaceErr != nil {
				if strings.TrimSpace(cmd.GoalID) != "" || strings.TrimSpace(cmd.DoDID) != "" {
					return readmodelcore.WorkspaceExecutionActivation{}, replaceErr
				}
			} else if ActiveRunDefinitionStale(ctx, request.Hub, workspace.ID, run, goal, dod) {
				runID, err := createReplacementWorkspaceExecutionRun(ctx, request, workspace, goal, dod, run.ID)
				if err != nil {
					return readmodelcore.WorkspaceExecutionActivation{}, err
				}
				startedRun, err := ownerreadports.ReadRunByOwnerCommand(ctx, request.Hub, runID)
				if err != nil {
					return readmodelcore.WorkspaceExecutionActivation{}, err
				}
				return WorkspaceExecutionActivationSnapshot(workspace, startedRun, goal, dod), nil
			}
			return WorkspaceExecutionActivationSnapshot(workspace, run, goal, dod), nil
		}
		workspace.ActiveRunID = ""
	}

	goal, dod, err := SelectWorkspaceExecutionDefinitions(ctx, request.Hub, workspaceID, common.GoalID(cmd.GoalID), common.DoDID(cmd.DoDID))
	if err != nil {
		return readmodelcore.WorkspaceExecutionActivation{}, err
	}
	runID, err := createWorkspaceExecutionRun(ctx, request, workspace, goal, dod)
	if err != nil {
		return readmodelcore.WorkspaceExecutionActivation{}, err
	}
	startedRun, err := ownerreadports.ReadRunByOwnerCommand(ctx, request.Hub, runID)
	if err != nil {
		return readmodelcore.WorkspaceExecutionActivation{}, err
	}
	return WorkspaceExecutionActivationSnapshot(workspace, startedRun, goal, dod), nil
}
