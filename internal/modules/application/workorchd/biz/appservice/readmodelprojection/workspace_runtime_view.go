package readmodelprojection

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
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
		WorkspaceID:           strings.TrimSpace(snapshot.Workspace.ID),
		ActiveRunID:           activeRunID,
		DisplayRunID:          displayRunID,
		Projection:            projectioncontract.ProjectionStatusFromWorkspaceViewModel(snapshot.ViewModel),
		Revision:              revision,
		PromptMode:            firstNonEmptyTimelineString(snapshot.ViewModel.PromptMode, workspacePromptMode(snapshot)),
		RunState:              runState,
		GovernanceDisplay:     snapshot.GovernanceDisplay,
		ConsistencyDiagnostic: projectioncontract.CloneConsistencyDiagnostic(snapshot.ConsistencyDiagnostic),
		TaskProgress:          progress,
		RunHeartbeat:          snapshot.RunHeartbeat,
		ConvergenceStatus:     snapshot.ConvergenceStatus,
		Acceptance:            acceptance,
		RunCommand:            snapshot.RunCommand,
		BlockingDecision:      blockingDecision,
		BlockingApproval:      blockingApproval,
		DisplayOutcome:        snapshot.DisplayOutcome,
		Activity:              activity,
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
	parts = append(parts, fmt.Sprintf("whole Run: durable effects %d | satisfied CWI %d", runtime.DurableEffectCount, runtime.SatisfiedCWICount))
	if counts := runtime.RunEvidence; counts != nil {
		parts = append(parts, fmt.Sprintf("whole Run: evidence accepted %d rejected %d | validation accepted %d rejected %d", counts.Accepted, counts.Rejected, counts.AcceptedValidation, counts.RejectedValidation))
	}
	if runtime.ValidationFailureOccurrences > 1 {
		parts = append(parts, fmt.Sprintf("same validation failure %d", runtime.ValidationFailureOccurrences))
	}
	if state := strings.TrimSpace(runtime.PrerequisiteRefreshState); state != "" {
		parts = append(parts, fmt.Sprintf("prerequisite refresh %s %d/%d", state, runtime.PrerequisiteRefreshUsed, runtime.PrerequisiteRefreshLimit))
	}
	parts = append(parts, fmt.Sprintf("current focus: evidence accepted %d rejected %d | CWI-credited validation %d", runtime.AcceptedEvidenceCount, runtime.RejectedEvidenceCount, runtime.AcceptedValidationEvidenceCount))
	if len(runtime.MissingEvidenceKinds) > 0 {
		parts = append(parts, "missing "+strings.Join(runtime.MissingEvidenceKinds, ","))
	}
	if msg := strings.TrimSpace(runtime.FailureMessage); msg != "" {
		parts = append(parts, msg)
	} else if class := strings.TrimSpace(runtime.FailureClass); class != "" {
		parts = append(parts, class)
	}
	if diagnostic := runtime.OriginalDiagnostic; diagnostic != nil {
		parts = append(parts, "owner cause "+diagnostic.OwnerModuleID+" / "+diagnostic.OriginalCode+": "+diagnostic.OriginalMessage)
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
	activeRunID = strings.TrimSpace(activeRunID)
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	run := canonicalActiveRun(snapshot, activeRunID)
	if workspaceID == "" || activeRunID == "" || run == nil || strings.TrimSpace(run.WorkspaceID) != workspaceID {
		return nil
	}
	return selectRunBlockingDecision(*run, snapshot.Decisions)
}

func selectRunBlockingDecision(run appmodelcore.Run, decisions []projectioncontract.Decision) *projectioncontract.Decision {
	workspaceID := strings.TrimSpace(run.WorkspaceID)
	runID := strings.TrimSpace(run.ID)
	if workspaceID == "" || runID == "" || common.RunState(strings.TrimSpace(run.State)) != common.RunStateBlockedOnDecision {
		return nil
	}
	ticketID := strings.TrimSpace(run.BlockingTicketID)
	if ticketID == "" {
		return nil
	}
	for idx := range decisions {
		item := decisions[idx]
		if strings.TrimSpace(item.ID) == ticketID &&
			strings.TrimSpace(item.WorkspaceID) == workspaceID &&
			strings.TrimSpace(item.RunID) == runID &&
			common.DecisionTicketState(strings.TrimSpace(item.State)) == common.DecisionTicketStateOpen {
			ret := item
			return &ret
		}
	}
	return nil
}

func selectBlockingApproval(snapshot readmodelcore.WorkspaceSnapshotState, activeRunID string) *projectioncontract.Approval {
	activeRunID = strings.TrimSpace(activeRunID)
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	run := canonicalActiveRun(snapshot, activeRunID)
	if workspaceID == "" || activeRunID == "" || run == nil || strings.TrimSpace(run.WorkspaceID) != workspaceID {
		return nil
	}
	return selectRunBlockingApproval(*run, snapshot.Approvals)
}

func selectRunBlockingApproval(run appmodelcore.Run, approvals []projectioncontract.Approval) *projectioncontract.Approval {
	workspaceID := strings.TrimSpace(run.WorkspaceID)
	runID := strings.TrimSpace(run.ID)
	if workspaceID == "" || runID == "" || common.RunState(strings.TrimSpace(run.State)) != common.RunStateBlockedOnApproval {
		return nil
	}
	approvalID := strings.TrimSpace(run.BlockingApprovalID)
	if approvalID == "" {
		return nil
	}
	for idx := range approvals {
		item := approvals[idx]
		if strings.TrimSpace(item.ID) == approvalID &&
			strings.TrimSpace(item.WorkspaceID) == workspaceID &&
			strings.TrimSpace(item.RunID) == runID &&
			common.ApprovalState(strings.TrimSpace(item.State)) == common.ApprovalStatePending {
			ret := item
			return &ret
		}
	}
	return nil
}
