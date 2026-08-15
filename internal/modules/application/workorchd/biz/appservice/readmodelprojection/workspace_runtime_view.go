package readmodelprojection

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// projectWorkspaceRuntimeView builds a RuntimeView without activity authority.
// Production reads supply store-owned activity through
// projectWorkspaceRuntimeViewWithActivity.
func projectWorkspaceRuntimeView(snapshot readmodelcore.WorkspaceSnapshotState, activityLimit int, beforeCursor string) projectioncontract.WorkspaceRuntimeView {
	return projectWorkspaceRuntimeViewWithActivity(snapshot, projectioncontract.WorkspaceActivityFeed{}, false, activityLimit, beforeCursor)
}

// projectWorkspaceRuntimeViewWithActivity builds RuntimeView using a store-owned
// activity feed when available. ActivityRevision is never regenerated on read.
func projectWorkspaceRuntimeViewWithActivity(snapshot readmodelcore.WorkspaceSnapshotState, storedActivity projectioncontract.WorkspaceActivityFeed, hasStoredActivity bool, activityLimit int, beforeCursor string) projectioncontract.WorkspaceRuntimeView {
	displayRunID := selectDisplayRunID(snapshot)
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	var activity projectioncontract.WorkspaceActivityFeed
	if hasStoredActivity {
		activity = pageWorkspaceActivityFeed(storedActivity, activityLimit, beforeCursor)
	}
	if activity.DisplayRunID == "" {
		activity.DisplayRunID = displayRunID
	}

	progress := snapshot.TaskProgress
	if runtimeSummary := runtimeProgressSummary(progress.Runtime); runtimeSummary != "" {
		// Runtime progress must produce an observable summary when revision/focus/action change.
		progress.Summary = runtimeSummary
	}

	revision := workspaceRefreshRevisionFromSnapshot(snapshot, activity.ActivityRevision)
	revision.StreamGeneration = strings.TrimSpace(activity.StreamGeneration)
	acceptance := workspaceRuntimeAcceptance(snapshot.Acceptance, snapshot.Workspace.ID, displayRunID, &revision)
	blockingDecision := selectBlockingDecision(snapshot, activeRunID)
	blockingApproval := selectBlockingApproval(snapshot, activeRunID)
	runState := ""
	if run := selectDisplayRun(snapshot); run != nil {
		runState = strings.TrimSpace(run.State)
	}
	if runState == "" {
		runState = strings.TrimSpace(progress.State)
	}

	return projectioncontract.WorkspaceRuntimeView{
		WorkspaceID:       strings.TrimSpace(snapshot.Workspace.ID),
		ActiveRunID:       activeRunID,
		DisplayRunID:      displayRunID,
		Projection:        projectioncontract.ProjectionStatusFromWorkspaceViewModel(snapshot.ViewModel),
		Revision:          revision,
		PromptMode:        firstNonEmptyTimelineString(snapshot.ViewModel.PromptMode, workspacePromptMode(snapshot)),
		RunState:          runState,
		TaskProgress:      progress,
		RunHeartbeat:      snapshot.RunHeartbeat,
		ConvergenceStatus: snapshot.ConvergenceStatus,
		Acceptance:        acceptance,
		RunCommand:        snapshot.RunCommand,
		BlockingDecision:  blockingDecision,
		BlockingApproval:  blockingApproval,
		Outcome:           snapshot.Outcome,
		Activity:          activity,
	}
}

// workspaceRuntimeAcceptance keeps semantic planning tied to the same
// workspace/run/revision selected by this RuntimeView. A mismatched projection
// is omitted instead of being presented as current owner state.
func workspaceRuntimeAcceptance(
	acceptance projectioncontract.AcceptanceProjection,
	workspaceID string,
	displayRunID string,
	revision *projectioncontract.WorkspaceRefreshRevision,
) projectioncontract.AcceptanceProjection {
	semantic := acceptance.SemanticPlanning
	if semantic == nil {
		return acceptance
	}
	workspaceID = strings.TrimSpace(workspaceID)
	displayRunID = strings.TrimSpace(displayRunID)
	if strings.TrimSpace(semantic.WorkspaceID) != workspaceID ||
		strings.TrimSpace(semantic.RunID) != displayRunID ||
		workspaceID == "" || displayRunID == "" {
		acceptance.SemanticPlanning = nil
		return acceptance
	}
	if revision == nil {
		acceptance.SemanticPlanning = nil
		return acceptance
	}
	if revision.RuntimeStateRevision == 0 {
		revision.RuntimeStateRevision = semantic.StateRevision
		return acceptance
	}
	if semantic.StateRevision != revision.RuntimeStateRevision {
		acceptance.SemanticPlanning = nil
	}
	return acceptance
}

// runtimeProgressSummary renders a compact progress line from Runtime.
func runtimeProgressSummary(runtime *projectioncontract.RuntimeProgress) string {
	if runtime == nil {
		return ""
	}
	parts := make([]string, 0, 6)
	parts = append(parts, "revision "+strconv.FormatUint(runtime.StateRevision, 10))
	parts = append(parts, "evidence "+strconv.FormatUint(runtime.EvidenceSequence, 10))
	if focus := strings.TrimSpace(runtime.CurrentFocusID); focus != "" {
		parts = append(parts, "focus "+focus)
	}
	if state := strings.TrimSpace(runtime.ConvergenceState); state != "" {
		parts = append(parts, "convergence "+state)
	}
	parts = append(parts, fmt.Sprintf("dispatch %d", runtime.CompletedDispatchCount))
	parts = append(parts, fmt.Sprintf("attempts %d", runtime.CandidateAttemptCount))
	if state := strings.TrimSpace(runtime.PrerequisiteRefreshState); state != "" {
		parts = append(parts, fmt.Sprintf("prerequisite refresh %s %d/%d", state, runtime.PrerequisiteRefreshUsed, runtime.PrerequisiteRefreshLimit))
	}
	if runtime.AcceptedEvidenceCount > 0 || runtime.RejectedEvidenceCount > 0 {
		parts = append(parts, fmt.Sprintf("accepted %d rejected %d", runtime.AcceptedEvidenceCount, runtime.RejectedEvidenceCount))
	}
	if len(runtime.MissingEvidenceKinds) > 0 {
		parts = append(parts, "missing "+strings.Join(runtime.MissingEvidenceKinds, ","))
	}
	if msg := strings.TrimSpace(runtime.FailureMessage); msg != "" {
		parts = append(parts, msg)
	} else if class := strings.TrimSpace(runtime.FailureClass); class != "" {
		parts = append(parts, class)
	}
	return strings.Join(parts, " | ")
}

// workspaceRefreshRevisionFromSnapshot extracts multi-domain revisions.
func workspaceRefreshRevisionFromSnapshot(snapshot readmodelcore.WorkspaceSnapshotState, activityRevision int64) projectioncontract.WorkspaceRefreshRevision {
	revision := projectioncontract.WorkspaceRefreshRevision{
		WorkspaceRevision:       snapshot.ViewModel.WorkspaceRevision,
		ProjectedSourceRevision: snapshot.ViewModel.ProjectedSourceRevision,
		ActivityRevision:        activityRevision,
	}
	if revision.WorkspaceRevision == 0 {
		revision.WorkspaceRevision = snapshot.ViewModel.Revision
	}
	if runtime := snapshot.TaskProgress.Runtime; runtime != nil {
		revision.RuntimeStateRevision = runtime.StateRevision
		revision.EvidenceSequence = runtime.EvidenceSequence
	}
	// ActivityRevision remains its own domain. Do not fall back to source/runtime revisions.
	return revision
}

func selectBlockingDecision(snapshot readmodelcore.WorkspaceSnapshotState, activeRunID string) *projectioncontract.Decision {
	ticketID := strings.TrimSpace(snapshot.Workspace.BlockingTicketID)
	if ticketID == "" && snapshot.Run != nil {
		ticketID = strings.TrimSpace(snapshot.Run.BlockingTicketID)
	}
	if ticketID == "" {
		return nil
	}
	for idx := range snapshot.Decisions {
		item := snapshot.Decisions[idx]
		if strings.TrimSpace(item.ID) != ticketID {
			continue
		}
		if activeRunID != "" && strings.TrimSpace(item.RunID) != "" && strings.TrimSpace(item.RunID) != activeRunID {
			continue
		}
		if !isOpenGovernanceState(item.State) {
			return nil
		}
		ret := item
		return &ret
	}
	return nil
}

func selectBlockingApproval(snapshot readmodelcore.WorkspaceSnapshotState, activeRunID string) *projectioncontract.Approval {
	approvalID := strings.TrimSpace(snapshot.Workspace.BlockingApprovalID)
	if approvalID == "" && snapshot.Run != nil {
		approvalID = strings.TrimSpace(snapshot.Run.BlockingApprovalID)
	}
	if approvalID == "" {
		return nil
	}
	for idx := range snapshot.Approvals {
		item := snapshot.Approvals[idx]
		if strings.TrimSpace(item.ID) != approvalID {
			continue
		}
		if activeRunID != "" && strings.TrimSpace(item.RunID) != "" && strings.TrimSpace(item.RunID) != activeRunID {
			continue
		}
		if !isOpenGovernanceState(item.State) {
			return nil
		}
		ret := item
		return &ret
	}
	return nil
}
