package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/common"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
)

// ProjectWorkspaceRuntimeView builds the internal RuntimeView from a snapshot.
func ProjectWorkspaceRuntimeView(snapshot readmodelcore.WorkspaceSnapshot, activityLimit int, beforeCursor string) readmodelcore.WorkspaceRuntimeView {
	return readmodelprojection.ProjectWorkspaceRuntimeView(snapshot, activityLimit, beforeCursor)
}

// ProjectWorkspaceRuntimeViewWithActivity builds RuntimeView using store-owned activity.
func ProjectWorkspaceRuntimeViewWithActivity(snapshot readmodelcore.WorkspaceSnapshot, storedActivity readmodelcore.WorkspaceActivityFeed, hasStoredActivity bool, activityLimit int, beforeCursor string) readmodelcore.WorkspaceRuntimeView {
	return readmodelprojection.ProjectWorkspaceRuntimeViewWithActivity(snapshot, storedActivity, hasStoredActivity, activityLimit, beforeCursor)
}

// PortWorkspaceRuntimeViewInfo maps the internal RuntimeView to the stable port contract.
func PortWorkspaceRuntimeViewInfo(view readmodelcore.WorkspaceRuntimeView) port.WorkspaceRuntimeViewInfo {
	return port.WorkspaceRuntimeViewInfo{
		WorkspaceID:       common.WorkspaceID(view.WorkspaceID),
		ActiveRunID:       common.RunID(view.ActiveRunID),
		DisplayRunID:      common.RunID(view.DisplayRunID),
		Projection:        PortProjectionInfo(view.Projection),
		Revision:          view.Revision,
		PromptMode:        view.PromptMode,
		RunState:          common.RunState(view.RunState),
		TaskProgress:      portTaskProgressInfo(view.TaskProgress),
		RunHeartbeat:      portRunHeartbeatInfo(view.RunHeartbeat),
		ConvergenceStatus: portConvergenceStatusInfo(view.ConvergenceStatus),
		Acceptance:        portAcceptanceProjectionInfo(view.Acceptance),
		RunCommand:        portRunCommandStatusInfo(view.RunCommand),
		BlockingDecision:  portDecisionPtr(view.BlockingDecision),
		BlockingApproval:  portApprovalPtr(view.BlockingApproval),
		Outcome:           portWorkspaceOutcomeInfo(view.Outcome),
		Activity:          portWorkspaceActivityFeedInfo(view.Activity),
	}
}

// ProjectAndPortWorkspaceRuntimeView builds RuntimeView from a snapshot and maps it to port.
func ProjectAndPortWorkspaceRuntimeView(snapshot readmodelcore.WorkspaceSnapshot, activityLimit int, beforeCursor string) port.WorkspaceRuntimeViewInfo {
	view := readmodelprojection.ProjectWorkspaceRuntimeView(snapshot, activityLimit, beforeCursor)
	return PortWorkspaceRuntimeViewInfo(view)
}

// ProjectAndPortWorkspaceRuntimeViewWithActivity uses store-owned activity authority.
func ProjectAndPortWorkspaceRuntimeViewWithActivity(snapshot readmodelcore.WorkspaceSnapshot, storedActivity readmodelcore.WorkspaceActivityFeed, hasStoredActivity bool, activityLimit int, beforeCursor string) port.WorkspaceRuntimeViewInfo {
	view := readmodelprojection.ProjectWorkspaceRuntimeViewWithActivity(snapshot, storedActivity, hasStoredActivity, activityLimit, beforeCursor)
	return PortWorkspaceRuntimeViewInfo(view)
}

func portWorkspaceActivityFeedInfo(item readmodelcore.WorkspaceActivityFeed) port.WorkspaceActivityFeedInfo {
	return port.WorkspaceActivityFeedInfo{
		WorkspaceID:      common.WorkspaceID(item.WorkspaceID),
		DisplayRunID:     common.RunID(item.DisplayRunID),
		ActivityRevision: item.ActivityRevision,
		Items:            portWorkspaceActivityItemInfos(item.Items),
		Truncated:        item.Truncated,
		NextCursor:       item.NextCursor,
	}
}

func portWorkspaceActivityItemInfos(items []readmodelcore.WorkspaceActivityItem) []port.WorkspaceActivityItemInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]port.WorkspaceActivityItemInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, port.WorkspaceActivityItemInfo{
			ID:                   item.ID,
			WorkspaceID:          common.WorkspaceID(item.WorkspaceID),
			RunID:                common.RunID(item.RunID),
			StepID:               common.StepID(item.StepID),
			Kind:                 port.WorkspaceActivityKind(item.Kind),
			State:                item.State,
			Title:                item.Title,
			Summary:              item.Summary,
			OccurredAt:           item.OccurredAt,
			SourceEventRevision:  item.SourceEventRevision,
			RuntimeStateRevision: item.RuntimeStateRevision,
			EvidenceSequence:     item.EvidenceSequence,
		})
	}
	return ret
}

func portDecisionPtr(item *appmodelcore.Decision) *port.DecisionInfo {
	if item == nil {
		return nil
	}
	ret := PortDecisionInfo(*item)
	return &ret
}

func portApprovalPtr(item *appmodelcore.Approval) *port.ApprovalInfo {
	if item == nil {
		return nil
	}
	ret := PortApprovalInfo(*item)
	return &ret
}
