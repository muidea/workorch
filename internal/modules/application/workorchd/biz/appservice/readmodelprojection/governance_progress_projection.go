package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: progress_governance_resolution.go

func acceptedTerminalRun(snapshot readmodelcore.RunSnapshotState) bool {
	return snapshot.Run.State == string(common.RunStateCompleted) && hasAcceptedRunResult(snapshot)
}

func isResolvedDecisionState(state string) bool {
	return state == string(common.DecisionTicketStateResolved) || state == string(common.DecisionTicketStateSuperseded)
}

func isResolvedApprovalState(state string) bool {
	return state == string(common.ApprovalStateApproved) || state == string(common.ApprovalStateRejected) || state == string(common.ApprovalStateSuperseded)
}

func hasOpenDecisionBlocker(decisions []projectioncontract.Decision, workspaceID, runID, ticketID string) bool {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	ticketID = strings.TrimSpace(ticketID)
	if workspaceID == "" || runID == "" || ticketID == "" {
		return false
	}
	for _, item := range decisions {
		if strings.TrimSpace(item.ID) == ticketID &&
			strings.TrimSpace(item.WorkspaceID) == workspaceID &&
			strings.TrimSpace(item.RunID) == runID &&
			strings.TrimSpace(item.State) == string(common.DecisionTicketStateOpen) {
			return true
		}
	}
	return false
}

func hasOpenApprovalBlocker(approvals []projectioncontract.Approval, workspaceID, runID, approvalID string) bool {
	workspaceID = strings.TrimSpace(workspaceID)
	runID = strings.TrimSpace(runID)
	approvalID = strings.TrimSpace(approvalID)
	if workspaceID == "" || runID == "" || approvalID == "" {
		return false
	}
	for _, item := range approvals {
		if strings.TrimSpace(item.ID) == approvalID &&
			strings.TrimSpace(item.WorkspaceID) == workspaceID &&
			strings.TrimSpace(item.RunID) == runID &&
			strings.TrimSpace(item.State) == string(common.ApprovalStatePending) {
			return true
		}
	}
	return false
}

// source: progress_workspace_approval_blockers.go

func applyWorkspaceProgressApprovalBlockers(progress *projectioncontract.TaskProgress, snapshot readmodelcore.WorkspaceSnapshotState) {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	if workspaceID == "" || activeRunID == "" {
		return
	}
	for _, item := range snapshot.Approvals {
		if strings.TrimSpace(string(item.WorkspaceID)) != workspaceID || strings.TrimSpace(item.RunID) != activeRunID {
			continue
		}
		if strings.TrimSpace(item.State) != string(common.ApprovalStatePending) {
			continue
		}
		progress.Blockers = append(progress.Blockers, projectioncontract.ProgressBlocker{
			ID:          item.ID,
			Kind:        "approval",
			State:       item.State,
			Reason:      item.Reason,
			EvidenceRef: item.StepID,
		})
		if progress.Recovery.BlockingApprovalID == "" && progress.Recovery.BlockingTicketID == "" {
			progress.Recovery.Mode = "wait_approval"
			progress.Recovery.NextAction = "resolve_approval"
			progress.Recovery.BlockingApprovalID = item.ID
			progress.Recovery.StepID = item.StepID
			progress.Recovery.Reason = item.Reason
		}
	}
}

// source: progress_workspace_blockers.go

func applyWorkspaceProgressBlockers(progress *projectioncontract.TaskProgress, snapshot readmodelcore.WorkspaceSnapshotState) {
	workspaceID := strings.TrimSpace(snapshot.Workspace.ID)
	activeRunID := strings.TrimSpace(snapshot.Workspace.ActiveRunID)
	if workspaceID == "" || activeRunID == "" {
		return
	}
	for _, item := range snapshot.Decisions {
		if strings.TrimSpace(item.WorkspaceID) != workspaceID || strings.TrimSpace(item.RunID) != activeRunID {
			continue
		}
		if strings.TrimSpace(item.State) != string(common.DecisionTicketStateOpen) {
			continue
		}
		progress.Blockers = append(progress.Blockers, projectioncontract.ProgressBlocker{
			ID:          item.ID,
			Kind:        "decision",
			State:       item.State,
			Reason:      firstNonEmptyTimelineString(item.Question, item.Reason),
			EvidenceRef: item.StepID,
		})
		if progress.Recovery.BlockingTicketID == "" {
			progress.Recovery.Mode = "wait_decision"
			progress.Recovery.NextAction = "resolve_decision_ticket"
			progress.Recovery.BlockingTicketID = item.ID
			progress.Recovery.StepID = item.StepID
			progress.Recovery.Reason = firstNonEmptyTimelineString(item.Question, item.Reason)
		}
	}
	applyWorkspaceProgressApprovalBlockers(progress, snapshot)
}

// source: task_lifecycle_metadata.go

type taskLifecycleMetadata struct {
	sourceOutputState string
	retryOf           string
	retryAttempt      int
	failureClass      string
	failureSignature  string
	targetScope       string
	blockingTicketID  string
	evidenceRefs      []string
}

func taskLifecycleMetadataFromStep(step projectioncontract.StepView) taskLifecycleMetadata {
	output := taskMapValue(step.Output)
	input := taskMapValue(step.Input)
	ret := taskLifecycleMetadata{
		sourceOutputState: sourceOutputStateFromStep(step),
		retryOf:           firstNonEmptyString(readModelStringValue(input["retryOf"]), readModelStringValue(input["sourceStepId"])),
		retryAttempt:      readModelIntValue(input["retryAttempt"]),
		failureClass:      readModelStringValue(output["failureClass"]),
		failureSignature:  readModelStringValue(output["failureSignature"]),
		targetScope:       readModelStringValue(output["targetScope"]),
		blockingTicketID:  readModelStringValue(output["blockingTicketId"]),
		evidenceRefs:      compactUniqueStrings([]string{step.ID}),
	}
	if ret.retryOf == "" {
		ret.retryOf = firstNonEmptyString(readModelStringValue(input["repairOfStepId"]), readModelStringValue(input["repairOfStepID"]))
	}
	if ret.retryAttempt == 0 {
		ret.retryAttempt = readModelIntValue(firstNonNilValue(input["attempt"], input["repairAttempt"], output["attempt"]))
	}
	if refs := readModelStringSliceValue(output["evidenceRefs"]); len(refs) > 0 {
		ret.evidenceRefs = append(ret.evidenceRefs, refs...)
	}
	ret.evidenceRefs = compactUniqueStrings(ret.evidenceRefs)
	return ret
}

func sourceOutputStateFromStep(step projectioncontract.StepView) string {
	switch strings.TrimSpace(step.State) {
	case string(common.StepStateSucceeded):
		return "accepted"
	case string(common.StepStateFailed):
		return "rejected"
	case string(common.StepStateExecuting):
		return "running"
	default:
		return "proposed"
	}
}

func taskLifecycleMetadataForNode(node projectioncontract.TaskDecompositionNode, meta taskLifecycleMetadata) taskLifecycleMetadata {
	if strings.TrimSpace(node.StepID) == "" {
		switch meta.sourceOutputState {
		case "rejected", "retrying":
			return meta
		default:
			return taskLifecycleMetadata{}
		}
	}
	return meta
}

func applyTaskLifecycleToNode(node *projectioncontract.TaskDecompositionNode, meta taskLifecycleMetadata) {
	if node == nil {
		return
	}
	if meta.retryOf != "" {
		node.RetryOf = meta.retryOf
	}
	if meta.retryAttempt > 0 {
		node.RetryAttempt = meta.retryAttempt
	}
	if meta.failureClass != "" {
		node.FailureClass = meta.failureClass
	}
	if meta.failureSignature != "" {
		node.FailureSignature = meta.failureSignature
	}
	if meta.targetScope != "" {
		node.TargetScope = meta.targetScope
	}
	if meta.blockingTicketID != "" {
		node.BlockingTicketID = meta.blockingTicketID
	}
	if len(meta.evidenceRefs) > 0 {
		node.EvidenceRefs = compactUniqueStrings(append(node.EvidenceRefs, meta.evidenceRefs...))
	}
	if meta.sourceOutputState == "retrying" && node.State != "completed" {
		node.State = "retrying"
	}
	if meta.sourceOutputState == "rejected" && node.State == "pending" {
		node.State = "rejected"
	}
}

func applyTaskLifecycleToPlan(plan *projectioncontract.TaskPlan, meta taskLifecycleMetadata) {
	if plan == nil {
		return
	}
	plan.SourceOutputState = meta.sourceOutputState
	if meta.retryOf != "" {
		plan.RetryOf = meta.retryOf
	}
	if meta.retryAttempt > 0 {
		plan.RetryAttempt = meta.retryAttempt
	}
	if meta.failureClass != "" {
		plan.FailureClass = meta.failureClass
	}
	if meta.failureSignature != "" {
		plan.FailureSignature = meta.failureSignature
	}
	if meta.targetScope != "" {
		plan.TargetScope = meta.targetScope
	}
	if meta.blockingTicketID != "" {
		plan.BlockingTicketID = meta.blockingTicketID
	}
	if len(meta.evidenceRefs) > 0 {
		plan.EvidenceRefs = compactUniqueStrings(append(plan.EvidenceRefs, meta.evidenceRefs...))
	}
	switch meta.sourceOutputState {
	case "retrying":
		if plan.State != "accepted" {
			plan.State = "retrying"
		}
	case "rejected":
		if plan.State == "proposed" || plan.State == "revised" {
			plan.State = "rejected"
		}
	}
}

func taskCoverageTargetsFromInput(input map[string]any) []string {
	if len(input) == 0 {
		return nil
	}
	var ret []string
	for _, key := range []string{"coverageTargets", "coverageTargetRefs", "coverageWorkItemIds", "targetRefs", "affectedTargets", "relatedFailureTargets"} {
		ret = append(ret, readModelStringSliceValue(input[key])...)
		if value := strings.TrimSpace(readModelStringValue(input[key])); value != "" {
			ret = append(ret, value)
		}
	}
	return compactUniqueStrings(ret)
}

func taskBlockingTicketByStep(snapshot readmodelcore.RunSnapshotState) map[string]string {
	ret := map[string]string{}
	for _, item := range snapshot.Decisions {
		if isResolvedDecisionState(item.State) {
			continue
		}
		stepID := strings.TrimSpace(item.StepID)
		if stepID == "" {
			continue
		}
		ret[stepID] = item.ID
	}
	return ret
}

func firstNonEmptyMap(items ...map[string]any) map[string]any {
	for _, item := range items {
		if len(item) > 0 {
			return item
		}
	}
	return nil
}

func taskMapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}

func firstNonNilValue(items ...any) any {
	for _, item := range items {
		if item != nil {
			return item
		}
	}
	return nil
}

// source: task_retry_plan_projection.go

func taskRetryPlanFromStep(snapshot readmodelcore.RunSnapshotState, step projectioncontract.StepView, occurrences map[string]string, knownAnchors map[string]bool) (projectioncontract.TaskPlan, bool) {
	meta := taskLifecycleMetadataFromStep(step)
	if meta.retryOf == "" {
		return projectioncontract.TaskPlan{}, false
	}
	anchor := strings.TrimSpace(meta.retryOf)
	if anchor == "" {
		anchor = strings.TrimSpace(readModelStringValue(step.Input["graphRootStepId"]))
	}
	if anchor == "" || !knownAnchors[anchor] {
		return projectioncontract.TaskPlan{}, false
	}
	if ticketID := taskBlockingTicketByStep(snapshot)[step.ID]; ticketID != "" {
		meta.blockingTicketID = ticketID
	}
	state := "retrying"
	if meta.blockingTicketID != "" || snapshot.Run.BlockingTicketID != "" && step.State == string(common.StepStateFailed) {
		state = "blocked_on_decision"
	} else if step.State == string(common.StepStateFailed) {
		state = "retry_exhausted"
	} else if step.State == string(common.StepStateSucceeded) {
		state = "retry_completed"
	}
	plan := projectioncontract.TaskPlan{
		WorkspaceID:       snapshot.Run.WorkspaceID,
		RunID:             snapshot.Run.ID,
		AnchorStepID:      anchor,
		RevisionStepID:    step.ID,
		State:             state,
		Stage:             firstNonEmptyString(readModelStringValue(step.Output["stage"]), readModelStringValue(step.Input["stage"])),
		SourceOutputState: meta.sourceOutputState,
		Summary:           timelineStepSummary(step.Output),
		RetryOf:           anchor,
		RetryAttempt:      meta.retryAttempt,
		FailureClass:      meta.failureClass,
		FailureSignature:  meta.failureSignature,
		TargetScope:       meta.targetScope,
		BlockingTicketID:  meta.blockingTicketID,
		EvidenceRefs:      compactUniqueStrings(append([]string{step.ID}, meta.evidenceRefs...)),
		OccurredAt:        strings.TrimSpace(occurrences[step.ID]),
		SourceProjection:  "retry",
	}
	if plan.BlockingTicketID == "" && state == "blocked_on_decision" {
		plan.BlockingTicketID = snapshot.Run.BlockingTicketID
	}
	return plan, true
}

// source: workspace_outcome_definitions.go

func applyWorkspaceDefinitionsChangedOutcome(ret *projectioncontract.WorkspaceDisplayOutcome, snapshot readmodelcore.WorkspaceSnapshotState) bool {
	if ret == nil {
		return false
	}
	if common.WorkspaceState(snapshot.Workspace.State) != common.WorkspaceStateActive ||
		strings.TrimSpace(snapshot.Workspace.ActiveRunID) != "" {
		return false
	}
	goalID, dodID := latestWorkspaceDefinitionIDs(snapshot)
	if goalID == "" && dodID == "" {
		return false
	}
	if !workspaceDefinitionsChangedInPlace(snapshot) && workspaceHasTerminalResultForDefinitions(snapshot, goalID, dodID) {
		return false
	}
	ret.State = string(common.WorkspaceStateActive)
	ret.Terminal = false
	ret.Accepted = false
	ret.Reason = "workspace definitions changed; execution must be rerun"
	ret.Summary = ret.Reason
	return true
}

func workspaceDefinitionsChangedInPlace(snapshot readmodelcore.WorkspaceSnapshotState) bool {
	for idx := range snapshot.Goals {
		if strings.TrimSpace(snapshot.Goals[idx].PreviousDefinitionHash) != "" {
			return true
		}
	}
	for idx := range snapshot.DoDs {
		if strings.TrimSpace(snapshot.DoDs[idx].PreviousDefinitionHash) != "" {
			return true
		}
	}
	return false
}

func latestWorkspaceDefinitionIDs(snapshot readmodelcore.WorkspaceSnapshotState) (string, string) {
	var goalID string
	for idx := range snapshot.Goals {
		if id := strings.TrimSpace(snapshot.Goals[idx].ID); id != "" {
			goalID = id
		}
	}
	var dodID string
	for idx := range snapshot.DoDs {
		if id := strings.TrimSpace(snapshot.DoDs[idx].ID); id != "" {
			dodID = id
		}
	}
	return goalID, dodID
}

func workspaceHasTerminalResultForDefinitions(snapshot readmodelcore.WorkspaceSnapshotState, goalID string, dodID string) bool {
	for idx := range snapshot.Results {
		item := snapshot.Results[idx]
		if goalID != "" && strings.TrimSpace(item.GoalID) != goalID {
			continue
		}
		if dodID != "" && strings.TrimSpace(item.DoDID) != dodID {
			continue
		}
		switch common.ResultState(item.State) {
		case common.ResultStateAccepted, common.ResultStateRejected:
			return true
		}
	}
	return false
}
