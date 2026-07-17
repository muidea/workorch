package readmodelprojection

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func MapConvergenceStatus(snapshot readmodelcore.RunSnapshot) *readmodelcore.ConvergenceStatus {
	ret := readmodelcore.ConvergenceStatus{
		WorkspaceID: snapshot.Run.WorkspaceID,
		RunID:       snapshot.Run.ID,
		RunState:    snapshot.Run.State,
	}
	if snapshot.GapBaseline != nil {
		ret.CurrentFocusID = focusIDFromMap(snapshot.GapBaseline.CurrentFocus)
		ret.NextActionKind = strings.TrimSpace(snapshot.GapBaseline.NextAction.Kind)
		ret.CurrentGap = firstNonEmptyString(ret.CurrentGap, currentGapFromTodoItems(snapshot.GapBaseline.TodoItems))
		ret.MissingEvidenceKinds = compactUniqueStrings(requiredEvidenceKindsFromTodoItems(snapshot.GapBaseline.TodoItems))
		ret.AcceptedEvidenceCount = len(compactUniqueStrings(snapshot.GapBaseline.AcceptedEvidenceRefs))
		ret.RejectedEvidenceCount = len(compactUniqueStrings(snapshot.GapBaseline.RejectedEvidenceRefs))
		if refs := compactUniqueStrings(snapshot.GapBaseline.AcceptedEvidenceRefs); len(refs) > 0 {
			ret.LatestAcceptedEvidenceRef = refs[len(refs)-1]
		}
		if open := len(snapshot.GapBaseline.OpenCoverageWorkItems); open > 0 {
			ret.RemainingCoverageItemCount = open
		}
		if reason := strings.TrimSpace(snapshot.GapBaseline.NextAction.Reason); reason != "" {
			ret.GapSummary = reason
		}
	}
	if snapshot.Acceptance.CurrentFocus != nil {
		focus := snapshot.Acceptance.CurrentFocus
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
	if snapshot.Acceptance.ActiveSliceID != "" {
		ret.CurrentSliceID = firstNonEmptyString(ret.CurrentSliceID, snapshot.Acceptance.ActiveSliceID)
	}
	if budget := snapshot.Acceptance.Budget; budget != nil {
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
	// TaskProgress / RuntimeV2 / AssessmentOutcome are authoritative product-boundary summaries.
	progress := snapshot.TaskProgress
	if progress.RuntimeV2 != nil {
		rv2 := progress.RuntimeV2
		ret.CurrentFocusID = firstNonEmptyString(ret.CurrentFocusID, rv2.CurrentFocusID)
		ret.NextActionKind = firstNonEmptyString(ret.NextActionKind, rv2.CurrentAction)
		if rv2.AcceptedEvidenceCount > ret.AcceptedEvidenceCount {
			ret.AcceptedEvidenceCount = rv2.AcceptedEvidenceCount
		}
		if rv2.RejectedEvidenceCount > ret.RejectedEvidenceCount {
			ret.RejectedEvidenceCount = rv2.RejectedEvidenceCount
		}
		ret.MissingEvidenceKinds = compactUniqueStrings(append(ret.MissingEvidenceKinds, rv2.MissingEvidenceKinds...))
		if ret.CurrentGap == "" {
			ret.CurrentGap = firstNonEmptyString(rv2.FailureClass, rv2.FailureCode)
		}
		if ret.GapSummary == "" {
			ret.GapSummary = firstNonEmptyString(rv2.FailureMessage, rv2.FailureSignature)
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
	// nextActivationRequired: terminal-like gap needing a new activation, or explicit next_activation action.
	next := strings.ToLower(strings.TrimSpace(ret.NextActionKind))
	if next == "next_activation" || next == "require_activation" || next == "activation_required" {
		ret.NextActivationRequired = true
	}
	if !ret.NextActivationRequired && isTerminalRunStateForActivation(ret.RunState) && ret.RemainingCoverageItemCount > 0 {
		ret.NextActivationRequired = true
	}
	if denied := latestDecisionEligibilityDenied(snapshot.Steps); denied != nil {
		ret.DecisionEligibilityDenied = true
		ret.DecisionNotEligibleReason = denied.reason
		ret.DecisionNotEligibleStepID = denied.stepID
	}
	ret.DecisionCause = convergenceDecisionCause(ret)
	if ret.WorkspaceID == "" && snapshot.Workspace.ID != "" {
		ret.WorkspaceID = snapshot.Workspace.ID
	}
	if ret.RunID == "" {
		return nil
	}
	// Default gap summary from structured fields when still empty.
	if ret.GapSummary == "" && ret.CurrentGap != "" {
		ret.GapSummary = ret.CurrentGap
	}
	if ret.GapSummary == "" && ret.RemainingCoverageItemCount > 0 {
		ret.GapSummary = fmt.Sprintf("%d coverage items remaining", ret.RemainingCoverageItemCount)
	}
	return &ret
}

func isTerminalRunStateForActivation(state string) bool {
	switch strings.TrimSpace(state) {
	case "completed", "failed", "cancelled", "superseded", "rejected":
		return true
	default:
		return false
	}
}

func convergenceDecisionCause(status readmodelcore.ConvergenceStatus) string {
	runState := strings.TrimSpace(status.RunState)
	waitingDecision := runState == "waiting_decision" || runState == "blocked_on_decision"
	if !waitingDecision && status.NextActionKind != "wait_decision" {
		return ""
	}
	if status.DecisionEligibilityDenied {
		return "decision_not_eligible"
	}
	return "governance_required"
}

func MapWorkspaceConvergenceStatus(snapshot readmodelcore.WorkspaceSnapshot) *readmodelcore.ConvergenceStatus {
	run := readmodelcore.Run{}
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
	runSnapshot := readmodelcore.RunSnapshot{
		Workspace:    snapshot.Workspace,
		Run:          run,
		GapBaseline:  snapshot.GapBaseline,
		Acceptance:   snapshot.Acceptance,
		TaskProgress: snapshot.TaskProgress,
		Steps:        nil,
	}
	ret := MapConvergenceStatus(runSnapshot)
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

func latestDecisionEligibilityDenied(steps []readmodelcore.StepView) *decisionEligibilityDeniedProjection {
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

func currentGapFromTodoItems(items []readmodelcore.GapTodoItem) string {
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

func requiredEvidenceKindsFromTodoItems(items []readmodelcore.GapTodoItem) []string {
	var ret []string
	for _, item := range items {
		ret = append(ret, item.RequiredEvidenceKinds...)
	}
	return ret
}
