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
	if authority := MapValue(snapshot.Run.ExecutionContext["convergenceAuthority"]); len(authority) > 0 {
		if focusID := focusIDFromMap(MapValue(authority["currentFocus"])); focusID != "" {
			ret.CurrentFocusID = focusID
		}
		if nextActionKind := strings.TrimSpace(stringValue(MapValue(authority["nextAction"]), "kind")); nextActionKind != "" {
			ret.NextActionKind = nextActionKind
		}
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
		ret.LatestRejectedDiagnosticRef = strings.TrimSpace(violation.RejectedOutputRef)
		ret.RetryAttempt = violation.Attempt
		ret.RetryMaxAttempts = violation.MaxAttempts
		ret.RetrySignatureRepeatCount = violation.SignatureRepeatCount
		ret.RetryBudgetScope = cloneReadModelMap(violation.BudgetScope)
		ret.NarrowedRetryAttempted = strings.TrimSpace(violation.CorrectionMode) != ""
		ret.ManagedValidationAvailable = convergenceContractHasManagedValidation(violation.ExpectedContract) ||
			convergenceContractHasManagedValidation(violation.CorrectionContract)
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
	return &ret
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
	if status.LatestContractViolationRule != "" {
		if status.RetryMaxAttempts > 0 && status.RetryAttempt >= status.RetryMaxAttempts {
			return "retry_exhaustion"
		}
		if status.LatestRecovery == "decision_required" {
			return "contract_violation_governance"
		}
		return "contract_violation"
	}
	return "governance_required"
}

func convergenceContractHasManagedValidation(contract map[string]any) bool {
	if len(contract) == 0 {
		return false
	}
	for key, value := range contract {
		if strings.EqualFold(strings.TrimSpace(key), "acceptedKinds") {
			for _, item := range readModelStringSliceValue(value) {
				if strings.TrimSpace(item) == "validation/managed_session" {
					return true
				}
			}
		}
		if nested := readModelMapValue(value); len(nested) > 0 && convergenceContractHasManagedValidation(nested) {
			return true
		}
		for _, nestedItem := range readModelMapSliceValue(value) {
			if convergenceContractHasManagedValidation(nestedItem) {
				return true
			}
		}
	}
	return false
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
