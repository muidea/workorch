package readmodelprojection

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapConvergenceStatus(snapshot readmodelcore.RunSnapshotState) *projectioncontract.ConvergenceStatus {
	ret := projectioncontract.ConvergenceStatus{
		WorkspaceID: snapshot.Run.WorkspaceID,
		RunID:       snapshot.Run.ID,
		RunState:    snapshot.Run.State,
	}
	applyGapBaselineConvergenceStatus(&ret, snapshot.GapBaseline)
	applyAcceptanceConvergenceStatus(&ret, snapshot.Acceptance)
	applyTaskProgressConvergenceStatus(&ret, snapshot.TaskProgress)
	applyActivationConvergenceStatus(&ret)
	applyDecisionEligibilityConvergenceStatus(&ret, snapshot.Steps)
	if !finalizeConvergenceStatus(&ret, snapshot.Workspace.ID) {
		return nil
	}
	return &ret
}

func applyGapBaselineConvergenceStatus(ret *projectioncontract.ConvergenceStatus, baseline *projectioncontract.GapBaseline) {
	if ret == nil || baseline == nil {
		return
	}
	ret.CurrentFocusID = focusIDFromMap(baseline.CurrentFocus)
	ret.CurrentGap = firstNonEmptyString(ret.CurrentGap, currentGapFromTodoItems(baseline.TodoItems))
	ret.MissingEvidenceKinds = compactUniqueStrings(requiredEvidenceKindsFromTodoItems(baseline.TodoItems))
	ret.AcceptedEvidenceCount = len(compactUniqueStrings(baseline.AcceptedEvidenceRefs))
	ret.RejectedEvidenceCount = len(compactUniqueStrings(baseline.RejectedEvidenceRefs))
	if refs := compactUniqueStrings(baseline.AcceptedEvidenceRefs); len(refs) > 0 {
		ret.LatestAcceptedEvidenceRef = refs[len(refs)-1]
	}
	if open := len(baseline.OpenCoverageWorkItems); open > 0 {
		ret.RemainingCoverageItemCount = open
	}
	ret.GapSummary = firstNonEmptyString(ret.GapSummary, ret.CurrentGap)
}

func applyAcceptanceConvergenceStatus(ret *projectioncontract.ConvergenceStatus, acceptance projectioncontract.AcceptanceProjection) {
	if ret == nil {
		return
	}
	if focus := acceptance.CurrentFocus; focus != nil {
		ret.CurrentFocusID = firstNonEmptyString(ret.CurrentFocusID, focus.ID)
		ret.CurrentSliceID = firstNonEmptyString(ret.CurrentSliceID, focus.SliceID)
		if ret.AcceptedEvidenceCount == 0 {
			ret.AcceptedEvidenceCount = len(compactUniqueStrings(focus.AcceptedEvidenceRefs))
		}
		if ret.RejectedEvidenceCount == 0 {
			ret.RejectedEvidenceCount = len(compactUniqueStrings(focus.RejectedEvidenceRefs))
		}
		if refs := compactUniqueStrings(focus.AcceptedEvidenceRefs); len(refs) > 0 {
			ret.LatestAcceptedEvidenceRef = firstNonEmptyString(ret.LatestAcceptedEvidenceRef, refs[len(refs)-1])
		}
		if ret.CurrentGap == "" && strings.TrimSpace(focus.FailureClass) != "" {
			ret.CurrentGap = strings.TrimSpace(focus.FailureClass)
		}
		if ret.GapSummary == "" && strings.TrimSpace(focus.FailureSignature) != "" {
			ret.GapSummary = strings.TrimSpace(focus.FailureSignature)
		}
	}
	if acceptance.ActiveSliceID != "" {
		ret.CurrentSliceID = firstNonEmptyString(ret.CurrentSliceID, acceptance.ActiveSliceID)
	}
	if budget := acceptance.Budget; budget != nil {
		ret.RepairAttempts = budget.RepairAttempts
		ret.MaxRepairAttempts = budget.MaxRepairAttemptsPerFocus
		ret.RepeatedFailureSignatureCount = budget.RepeatedFailureSignatures
		if ret.CurrentSliceID == "" {
			ret.CurrentSliceID = strings.TrimSpace(budget.SliceID)
		}
		if ret.CurrentFocusID == "" {
			ret.CurrentFocusID = strings.TrimSpace(budget.FocusID)
		}
		if ret.GapSummary == "" {
			ret.GapSummary = firstNonEmptyString(budget.LastDecisionReason, budget.LastNoDeltaReason)
		}
	}
}

func applyTaskProgressConvergenceStatus(ret *projectioncontract.ConvergenceStatus, progress projectioncontract.TaskProgress) {
	if ret == nil {
		return
	}
	if runtime := progress.Runtime; runtime != nil {
		ret.CurrentFocusID = firstNonEmptyString(ret.CurrentFocusID, runtime.CurrentFocusID)
		ret.ConvergenceState = strings.TrimSpace(runtime.ConvergenceState)
		if runtime.AcceptedEvidenceCount > ret.AcceptedEvidenceCount {
			ret.AcceptedEvidenceCount = runtime.AcceptedEvidenceCount
		}
		if runtime.RejectedEvidenceCount > ret.RejectedEvidenceCount {
			ret.RejectedEvidenceCount = runtime.RejectedEvidenceCount
		}
		ret.MissingEvidenceKinds = compactUniqueStrings(append(ret.MissingEvidenceKinds, runtime.MissingEvidenceKinds...))
		if ret.CurrentGap == "" {
			ret.CurrentGap = firstNonEmptyString(runtime.FailureClass, runtime.FailureCode)
		}
		if ret.GapSummary == "" {
			ret.GapSummary = firstNonEmptyString(runtime.FailureMessage, runtime.FailureSignature)
		}
	}
	outcome := progress.AssessmentOutcome
	if outcome.SatisfiedCoverageCount > 0 || outcome.RemainingCoverageCount > 0 {
		ret.ClosedCoverageItemCount = outcome.SatisfiedCoverageCount
		ret.RemainingCoverageItemCount = outcome.RemainingCoverageCount
	}
	if disposition := strings.TrimSpace(outcome.State); disposition != "" {
		ret.AssessmentDisposition = disposition
	}
	if ret.GapSummary == "" && strings.TrimSpace(outcome.LatestRejectedReason) != "" {
		ret.GapSummary = strings.TrimSpace(outcome.LatestRejectedReason)
	}
}

func applyActivationConvergenceStatus(ret *projectioncontract.ConvergenceStatus) {
	if ret != nil && isTerminalRunStateForActivation(ret.RunState) && ret.RemainingCoverageItemCount > 0 {
		ret.NextActivationRequired = true
	}
}

func applyDecisionEligibilityConvergenceStatus(ret *projectioncontract.ConvergenceStatus, steps []projectioncontract.StepView) {
	if ret == nil {
		return
	}
	if denied := latestDecisionEligibilityDenied(steps); denied != nil {
		ret.DecisionEligibilityDenied = true
		ret.DecisionNotEligibleReason = denied.reason
		ret.DecisionNotEligibleStepID = denied.stepID
	}
	ret.DecisionCause = convergenceDecisionCause(*ret)
}

func finalizeConvergenceStatus(ret *projectioncontract.ConvergenceStatus, workspaceID string) bool {
	if ret == nil {
		return false
	}
	if ret.WorkspaceID == "" && workspaceID != "" {
		ret.WorkspaceID = workspaceID
	}
	if ret.RunID == "" {
		return false
	}
	if ret.GapSummary == "" && ret.CurrentGap != "" {
		ret.GapSummary = ret.CurrentGap
	}
	if ret.GapSummary == "" && ret.RemainingCoverageItemCount > 0 {
		ret.GapSummary = fmt.Sprintf("%d coverage items remaining", ret.RemainingCoverageItemCount)
	}
	return true
}

func isTerminalRunStateForActivation(state string) bool {
	switch strings.TrimSpace(state) {
	case "completed", "failed", "cancelled", "superseded", "rejected":
		return true
	default:
		return false
	}
}

func convergenceDecisionCause(status projectioncontract.ConvergenceStatus) string {
	runState := strings.TrimSpace(status.RunState)
	waitingDecision := runState == "waiting_decision" || runState == "blocked_on_decision"
	if !waitingDecision {
		return ""
	}
	if status.DecisionEligibilityDenied {
		return "decision_not_eligible"
	}
	return "governance_required"
}

func mapWorkspaceConvergenceStatus(snapshot readmodelcore.WorkspaceSnapshotState) *projectioncontract.ConvergenceStatus {
	run := appmodelcore.Run{}
	if snapshot.Run != nil {
		run = *snapshot.Run
	} else if len(snapshot.Runs) > 0 {
		run = snapshot.Runs[0]
		for _, item := range snapshot.Runs {
			if item.ID == snapshot.Workspace.ActiveRunID {
				run = item
				break
			}
		}
	}
	if run.ID == "" && snapshot.RunHeartbeat.RunID != "" {
		run.ID = snapshot.RunHeartbeat.RunID
		run.WorkspaceID = snapshot.RunHeartbeat.WorkspaceID
		run.State = snapshot.RunHeartbeat.State
	}
	if run.WorkspaceID == "" {
		run.WorkspaceID = snapshot.Workspace.ID
	}
	runSnapshot := readmodelcore.RunSnapshotState{
		Workspace:    snapshot.Workspace,
		Run:          run,
		GapBaseline:  snapshot.GapBaseline,
		Acceptance:   snapshot.Acceptance,
		TaskProgress: snapshot.TaskProgress,
		Steps:        nil,
	}
	ret := mapConvergenceStatus(runSnapshot)
	if ret != nil {
		ret.LatestEffectiveProgressAt = snapshot.RunHeartbeat.LatestEffectiveProgressAt
		ret.LastProgressAt = firstNonEmptyString(snapshot.RunHeartbeat.LatestEffectiveProgressAt, ret.LastProgressAt)
		ret.Stalled = snapshot.RunHeartbeat.Stalled
		ret.IneffectiveRetry = snapshot.RunHeartbeat.IneffectiveRetry
	}
	return ret
}

type decisionEligibilityDeniedProjection struct {
	stepID string
	reason string
}

func latestDecisionEligibilityDenied(steps []projectioncontract.StepView) *decisionEligibilityDeniedProjection {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		if !readModelBoolValue(step.Input["decisionEligibilityDenied"]) {
			continue
		}
		reason := firstNonEmptyString(
			readModelStringValue(step.Input["decisionEligibilityDeniedReason"]),
			readModelStringValue(step.Input["decisionNotEligibleReason"]),
		)
		return &decisionEligibilityDeniedProjection{
			stepID: strings.TrimSpace(step.ID),
			reason: reason,
		}
	}
	return nil
}

func focusIDFromMap(item map[string]any) string {
	for _, key := range []string{"id", "focusId"} {
		if value, ok := item[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func currentGapFromTodoItems(items []projectioncontract.GapTodoItem) string {
	for _, item := range items {
		if strings.TrimSpace(item.Reason) != "" {
			return strings.TrimSpace(item.Reason)
		}
		if len(item.RequiredEvidenceKinds) > 0 {
			return "missing accepted evidence"
		}
	}
	return ""
}

func requiredEvidenceKindsFromTodoItems(items []projectioncontract.GapTodoItem) []string {
	var ret []string
	for _, item := range items {
		ret = append(ret, item.RequiredEvidenceKinds...)
	}
	return ret
}
