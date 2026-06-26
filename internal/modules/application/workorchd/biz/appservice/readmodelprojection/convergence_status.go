package readmodelprojection

import (
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
	}
	if snapshot.Acceptance.CurrentFocus != nil {
		focus := snapshot.Acceptance.CurrentFocus
		ret.CurrentFocusID = firstNonEmptyString(ret.CurrentFocusID, focus.ID)
		ret.MissingEvidenceKinds = compactUniqueStrings(append(ret.MissingEvidenceKinds, focus.RequiredEvidenceKinds...))
		if ret.AcceptedEvidenceCount == 0 {
			ret.AcceptedEvidenceCount = len(compactUniqueStrings(focus.AcceptedEvidenceRefs))
		}
		if ret.RejectedEvidenceCount == 0 {
			ret.RejectedEvidenceCount = len(compactUniqueStrings(focus.RejectedEvidenceRefs))
		}
	}
	if snapshot.RuntimeExecution != nil && snapshot.RuntimeExecution.ContractViolation != nil {
		violation := snapshot.RuntimeExecution.ContractViolation
		ret.LatestContractViolationRule = strings.TrimSpace(violation.ViolatedRule)
		ret.LatestRecovery = strings.TrimSpace(violation.Recoverability)
	}
	if denied := latestDecisionEligibilityDenied(snapshot.Steps); denied != nil {
		ret.DecisionEligibilityDenied = true
		ret.DecisionNotEligibleReason = denied.reason
		ret.DecisionNotEligibleStepID = denied.stepID
	}
	if ret.WorkspaceID == "" && snapshot.Workspace.ID != "" {
		ret.WorkspaceID = snapshot.Workspace.ID
	}
	if ret.RunID == "" {
		return nil
	}
	return &ret
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
		Workspace:        snapshot.Workspace,
		Run:              run,
		RuntimeExecution: snapshot.RuntimeExecution,
		GapBaseline:      snapshot.GapBaseline,
		Acceptance:       snapshot.Acceptance,
	}
	ret := MapConvergenceStatus(runSnapshot)
	if ret != nil {
		ret.LatestEffectiveProgressAt = snapshot.RunHeartbeat.LatestEffectiveProgressAt
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
