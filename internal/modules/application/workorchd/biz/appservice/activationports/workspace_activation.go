package activationports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ActivateWorkspaceExecutionViewRequest struct {
	Hub                            magicevent.Hub
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
	if workspace.ActiveRunID == "" {
		activeRunID, err := uniqueNonTerminalRunID(ctx, request.Hub, workspace.ID)
		if err != nil {
			return readmodelcore.WorkspaceExecutionActivation{}, err
		}
		workspace.ActiveRunID = activeRunID
	}
	originalActiveRunID := workspace.ActiveRunID

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
			} else if cmd.Restart {
				runID, err := createReplacementWorkspaceExecutionRun(ctx, request, workspace, goal, dod, run.ID)
				if err != nil {
					return readmodelcore.WorkspaceExecutionActivation{}, err
				}
				startedRun, err := ownerreadports.ReadRunByOwnerCommand(ctx, request.Hub, runID)
				if err != nil {
					return readmodelcore.WorkspaceExecutionActivation{}, err
				}
				return WorkspaceExecutionActivationSnapshot(workspace, startedRun, goal, dod), nil
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
			if run.State == common.RunStatePending {
				if err := StartRun(ctx, request.Hub, workspace.ID, run.ID); err != nil {
					return readmodelcore.WorkspaceExecutionActivation{}, err
				}
				startedRun, err := ownerreadports.ReadRunByOwnerCommand(ctx, request.Hub, run.ID)
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
	if err := supersedePriorActivationRunIfNeeded(ctx, request, workspace.ID, originalActiveRunID, runID); err != nil {
		return readmodelcore.WorkspaceExecutionActivation{}, err
	}
	startedRun, err := ownerreadports.ReadRunByOwnerCommand(ctx, request.Hub, runID)
	if err != nil {
		return readmodelcore.WorkspaceExecutionActivation{}, err
	}
	return WorkspaceExecutionActivationSnapshot(workspace, startedRun, goal, dod), nil
}

func uniqueNonTerminalRunID(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) (common.RunID, error) {
	runs, err := ownerreadports.ListRunsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
	if err != nil {
		return "", err
	}
	var found common.RunID
	for _, run := range runs {
		if run == nil || activationcore.IsTerminalRunState(run.State) || run.State == common.RunStatePending {
			continue
		}
		if found != "" && found != run.ID {
			return "", common.ErrConflict
		}
		found = run.ID
	}
	return found, nil
}

func supersedePriorActivationRunIfNeeded(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspaceID common.WorkspaceID, priorRunID common.RunID, replacementRunID common.RunID) error {
	if priorRunID != "" && priorRunID != replacementRunID {
		prior, err := ownerreadports.ReadRunByOwnerCommand(ctx, request.Hub, priorRunID)
		if err != nil {
			return err
		}
		if !activationcore.IsTerminalRunState(prior.State) {
			return SupersedeRun(ctx, request.Hub, workspaceID, priorRunID)
		}
		return nil
	}
	return nil
}
