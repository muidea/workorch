package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func normalizeWorkspaceGovernanceProjection(snapshot readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState {
	// Keep every Workspace/Run/TaskProgress owner field byte-for-byte intact.
	// Governance correlation is a separate display-only value and mismatches are
	// surfaced explicitly instead of being repaired by this projection.
	snapshot.GovernanceDisplay = governanceDisplay(snapshot)
	snapshot.ConsistencyDiagnostic = governanceConsistencyDiagnostic(snapshot, snapshot.GovernanceDisplay)
	return snapshot
}

func workspacePromptMode(snapshot readmodelcore.WorkspaceSnapshotState) string {
	if mode := strings.TrimSpace(snapshot.GovernanceDisplay.PromptMode); mode != "" {
		return mode
	}
	return governanceDisplay(snapshot).PromptMode
}

func governanceDisplay(snapshot readmodelcore.WorkspaceSnapshotState) projectioncontract.GovernanceDisplay {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	run := canonicalActiveRun(snapshot, activeRunID)
	preferredDecisionID := ""
	preferredApprovalID := ""
	if run != nil {
		preferredDecisionID = strings.TrimSpace(run.BlockingTicketID)
		preferredApprovalID = strings.TrimSpace(run.BlockingApprovalID)
	}
	ret := projectioncontract.GovernanceDisplay{RunID: activeRunID, PromptMode: "workspace"}
	if activeRunID == "" {
		return ret
	}
	ret.DecisionID = openDecisionBlockerID(snapshot.Decisions, workspaceID, activeRunID, preferredDecisionID)
	ret.ApprovalID = openApprovalBlockerID(snapshot.Approvals, workspaceID, activeRunID, preferredApprovalID)
	switch {
	case ret.DecisionID != "":
		ret.PromptMode = "decision"
	case ret.ApprovalID != "":
		ret.PromptMode = "approval"
	case run != nil && common.RunState(run.State) == common.RunStateActive:
		ret.PromptMode = "running"
	}
	return ret
}

func governanceConsistencyDiagnostic(snapshot readmodelcore.WorkspaceSnapshotState, display projectioncontract.GovernanceDisplay) *projectioncontract.ConsistencyDiagnostic {
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	run := canonicalActiveRun(snapshot, activeRunID)
	diagnostic := &projectioncontract.ConsistencyDiagnostic{
		CanonicalWorkspaceState: strings.TrimSpace(snapshot.Workspace.State),
		CanonicalWorkspaceRunID: activeRunID,
		ObservedDecisionID:      strings.TrimSpace(display.DecisionID),
		ObservedApprovalID:      strings.TrimSpace(display.ApprovalID),
	}
	if snapshot.TaskProgress.Runtime != nil {
		diagnostic.RuntimeConvergenceState = strings.TrimSpace(snapshot.TaskProgress.Runtime.ConvergenceState)
	}
	if run != nil {
		diagnostic.CanonicalRunState = strings.TrimSpace(run.State)
		diagnostic.CanonicalRunDecisionID = strings.TrimSpace(run.BlockingTicketID)
		diagnostic.CanonicalRunApprovalID = strings.TrimSpace(run.BlockingApprovalID)
	}
	switch {
	case activeRunID != "" && run == nil:
		diagnostic.Code = "canonical_active_run_missing"
		diagnostic.Summary = "Workspace owner active run is absent from the bounded projection"
	case display.DecisionID != "" && (run == nil || run.State != string(common.RunStateBlockedOnDecision) || strings.TrimSpace(run.BlockingTicketID) != display.DecisionID):
		diagnostic.Code = "lifecycle_handoff_pending"
		diagnostic.Summary = "Decision is open while canonical Run owner state has not acknowledged the matching decision fence"
	case display.ApprovalID != "" && (run == nil || run.State != string(common.RunStateBlockedOnApproval) || strings.TrimSpace(run.BlockingApprovalID) != display.ApprovalID):
		diagnostic.Code = "lifecycle_handoff_pending"
		diagnostic.Summary = "Approval is pending while canonical Run owner state has not acknowledged the matching approval fence"
	case run != nil && strings.TrimSpace(run.BlockingTicketID) != "" && display.DecisionID == "":
		diagnostic.Code = "owner_governance_reference_unresolved"
		diagnostic.Summary = "Canonical Run decision reference has no matching open Decision projection"
	case run != nil && strings.TrimSpace(run.BlockingApprovalID) != "" && display.ApprovalID == "":
		diagnostic.Code = "owner_governance_reference_unresolved"
		diagnostic.Summary = "Canonical Run approval reference has no matching pending Approval projection"
	case diagnostic.RuntimeConvergenceState == "need_governance" && display.DecisionID == "" && display.ApprovalID == "":
		diagnostic.Code = "runtime_governance_unbound"
		diagnostic.Summary = "Runtime reports governance wait without a matching open governance resource projection"
	default:
		return nil
	}
	return diagnostic
}

func canonicalActiveRun(snapshot readmodelcore.WorkspaceSnapshotState, activeRunID string) *appmodelcore.Run {
	activeRunID = strings.TrimSpace(activeRunID)
	if activeRunID == "" {
		return nil
	}
	if snapshot.Run != nil && strings.TrimSpace(snapshot.Run.ID) == activeRunID {
		return snapshot.Run
	}
	for idx := range snapshot.Runs {
		if strings.TrimSpace(snapshot.Runs[idx].ID) == activeRunID {
			return &snapshot.Runs[idx]
		}
	}
	return nil
}

func applyRunSnapshotGovernanceProjection(snapshot *readmodelcore.RunSnapshotState) {
	if snapshot == nil {
		return
	}
	run := snapshot.Run
	workspace := readmodelcore.WorkspaceSnapshotState{
		Workspace:    snapshot.Workspace,
		Run:          &run,
		Runs:         []appmodelcore.Run{run},
		TaskProgress: snapshot.TaskProgress,
		Decisions:    snapshot.Decisions,
		Approvals:    snapshot.Approvals,
	}
	workspace = normalizeWorkspaceGovernanceProjection(workspace)
	snapshot.GovernanceDisplay = workspace.GovernanceDisplay
	snapshot.ConsistencyDiagnostic = workspace.ConsistencyDiagnostic
}

func runBlockingTicketID(run *appmodelcore.Run) string {
	if run == nil {
		return ""
	}
	return run.BlockingTicketID
}

func runBlockingApprovalID(run *appmodelcore.Run) string {
	if run == nil {
		return ""
	}
	return run.BlockingApprovalID
}

func openDecisionBlockerID(items []projectioncontract.Decision, workspaceID, runID, preferredID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	preferredID = strings.TrimSpace(preferredID)
	if hasOpenDecisionBlocker(items, workspaceID, runID, preferredID) {
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

func openApprovalBlockerID(items []projectioncontract.Approval, workspaceID, runID, preferredID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	preferredID = strings.TrimSpace(preferredID)
	if hasOpenApprovalBlocker(items, workspaceID, runID, preferredID) {
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
