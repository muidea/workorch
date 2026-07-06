package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func NormalizeWorkspaceGovernanceProjection(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceSnapshot {
	snapshot.Workspace.State = string(common.CanonicalWorkspaceOwnerState(common.WorkspaceState(snapshot.Workspace.State)))
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	if activeRunID == "" && snapshot.Run != nil {
		activeRunID = strings.TrimSpace(snapshot.Run.ID)
	}
	if activeRunID == "" {
		snapshot.Workspace.BlockingTicketID = ""
		snapshot.Workspace.BlockingApprovalID = ""
		snapshot.TaskProgress.BlockingTicketID = ""
		snapshot.TaskProgress.BlockingApprovalID = ""
		snapshot.TaskProgress.Recovery.BlockingTicketID = ""
		snapshot.TaskProgress.Recovery.BlockingApprovalID = ""
		return snapshot
	}

	openDecisionID := openDecisionBlockerID(snapshot.Decisions, snapshot.Workspace.ID, activeRunID, snapshot.Workspace.BlockingTicketID)
	if openDecisionID == "" && snapshot.Run != nil {
		openDecisionID = openDecisionBlockerID(snapshot.Decisions, snapshot.Workspace.ID, activeRunID, snapshot.Run.BlockingTicketID)
	}
	openApprovalID := openApprovalBlockerID(snapshot.Approvals, snapshot.Workspace.ID, activeRunID, snapshot.Workspace.BlockingApprovalID)
	if openApprovalID == "" && snapshot.Run != nil {
		openApprovalID = openApprovalBlockerID(snapshot.Approvals, snapshot.Workspace.ID, activeRunID, snapshot.Run.BlockingApprovalID)
	}

	snapshot.Workspace.BlockingTicketID = openDecisionID
	snapshot.Workspace.BlockingApprovalID = openApprovalID
	snapshot.TaskProgress.BlockingTicketID = openDecisionID
	snapshot.TaskProgress.BlockingApprovalID = openApprovalID
	snapshot.TaskProgress.Recovery.BlockingTicketID = openDecisionID
	snapshot.TaskProgress.Recovery.BlockingApprovalID = openApprovalID
	switch {
	case openDecisionID != "":
		snapshot.TaskProgress.Recovery.Mode = "wait_decision"
		snapshot.TaskProgress.Recovery.NextAction = "resolve_decision_ticket"
	case openApprovalID != "":
		snapshot.TaskProgress.Recovery.Mode = "wait_approval"
		snapshot.TaskProgress.Recovery.NextAction = "resolve_approval"
	case snapshot.TaskProgress.Recovery.Mode == "wait_decision" || snapshot.TaskProgress.Recovery.Mode == "wait_approval":
		snapshot.TaskProgress.Recovery.Mode = "resume"
		snapshot.TaskProgress.Recovery.NextAction = "inspect_active_run"
	}
	switch common.RunState(snapshot.TaskProgress.State) {
	case common.RunStateBlockedOnDecision:
		if openDecisionID == "" {
			snapshot.TaskProgress.State = string(common.RunStateActive)
		}
	case common.RunStateBlockedOnApproval:
		if openApprovalID == "" {
			snapshot.TaskProgress.State = string(common.RunStateActive)
		}
	}

	switch common.WorkspaceState(snapshot.Workspace.State) {
	case common.WorkspaceStateBlockedOnApproval:
		if openApprovalID == "" {
			snapshot.Workspace.State = string(common.WorkspaceStateActive)
		}
	}
	if openDecisionID != "" {
		snapshot.Workspace.State = string(common.WorkspaceStateActive)
	} else if openApprovalID != "" {
		snapshot.Workspace.State = string(common.WorkspaceStateBlockedOnApproval)
	}
	return snapshot
}

func WorkspacePromptMode(snapshot readmodelcore.WorkspaceSnapshot) string {
	workspace := snapshot.Workspace
	activeRunID := strings.TrimSpace(workspace.ActiveRunID)
	if activeRunID == "" && snapshot.Run != nil {
		activeRunID = strings.TrimSpace(snapshot.Run.ID)
	}
	if ticketID := openDecisionBlockerID(snapshot.Decisions, workspace.ID, activeRunID, firstNonEmptyProjectionString(workspace.BlockingTicketID, runBlockingTicketID(snapshot.Run))); ticketID != "" {
		return "decision"
	}
	if approvalID := openApprovalBlockerID(snapshot.Approvals, workspace.ID, activeRunID, firstNonEmptyProjectionString(workspace.BlockingApprovalID, runBlockingApprovalID(snapshot.Run))); approvalID != "" {
		return "approval"
	}
	switch common.WorkspaceState(workspace.State) {
	case common.WorkspaceStateBlockedOnApproval:
		if HasOpenApprovalBlocker(snapshot.Approvals, workspace.ID, activeRunID, workspace.BlockingApprovalID) {
			return "approval"
		}
	case common.WorkspaceStateActive:
		if snapshot.Run != nil && common.RunState(snapshot.Run.State) == common.RunStateActive {
			return "running"
		}
	}
	return "workspace"
}

func runBlockingTicketID(run *readmodelcore.Run) string {
	if run == nil {
		return ""
	}
	return run.BlockingTicketID
}

func runBlockingApprovalID(run *readmodelcore.Run) string {
	if run == nil {
		return ""
	}
	return run.BlockingApprovalID
}

func openDecisionBlockerID(items []readmodelcore.Decision, workspaceID, runID, preferredID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	preferredID = strings.TrimSpace(preferredID)
	if HasOpenDecisionBlocker(items, workspaceID, runID, preferredID) {
		return preferredID
	}
	for _, item := range items {
		if strings.TrimSpace(item.WorkspaceID) == workspaceID &&
			strings.TrimSpace(item.RunID) == runID &&
			strings.TrimSpace(item.State) == string(common.DecisionTicketStateOpen) &&
			strings.TrimSpace(item.ID) != "" {
			return strings.TrimSpace(item.ID)
		}
	}
	return ""
}

func openApprovalBlockerID(items []readmodelcore.Approval, workspaceID, runID, preferredID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	preferredID = strings.TrimSpace(preferredID)
	if HasOpenApprovalBlocker(items, workspaceID, runID, preferredID) {
		return preferredID
	}
	for _, item := range items {
		if strings.TrimSpace(item.WorkspaceID) == workspaceID &&
			strings.TrimSpace(item.RunID) == runID &&
			strings.TrimSpace(item.State) == string(common.ApprovalStatePending) &&
			strings.TrimSpace(item.ID) != "" {
			return strings.TrimSpace(item.ID)
		}
	}
	return ""
}
