package readmodelprojection

import (
	"strings"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func applyRuntimeProgress(progress *projectioncontract.TaskProgress, run *runmodel.Run) {
	if progress == nil || run == nil || run.RuntimeState == nil {
		return
	}
	summary := mapRuntimeProgress(*run.RuntimeState)
	progress.Runtime = &summary
	applyRuntimeProgressToMilestones(progress, summary, string(common.CanonicalRunState(run.State)))
	applyRuntimeProgressToRepairLoop(progress, summary, *run.RuntimeState)
	applyRuntimeProgressToCurrentStep(progress, summary, *run.RuntimeState)
	if runtimeSummary := runtimeProgressSummary(&summary); runtimeSummary != "" {
		progress.Summary = runtimeSummary
	} else if progress.Summary == "" && strings.TrimSpace(summary.FailureMessage) != "" {
		progress.Summary = strings.TrimSpace(summary.FailureMessage)
	}
}

func mapRuntimeProgress(state runtimecontract.RuntimeStateRecord) projectioncontract.RuntimeProgress {
	facts := state.Facts.Normalized()
	runEvidence := &projectioncontract.RuntimeEvidenceCounts{
		Accepted: len(facts.Evidence.Accepted), Rejected: len(facts.Evidence.Rejected),
	}
	accepted := 0
	acceptedValidation := 0
	rejected := 0
	for _, evidence := range facts.Evidence.Accepted {
		if evidence.Kind == runtimecontract.EvidenceKindValidation {
			runEvidence.AcceptedValidation++
		}
		if runtimecontract.EvidenceBelongsToCurrentFocusForDiagnosticLineage(facts, evidence) {
			accepted++
			if evidence.Kind == runtimecontract.EvidenceKindValidation && len(evidence.CoverageWorkItemIDs) > 0 {
				acceptedValidation++
			}
		}
	}
	for _, evidence := range facts.Evidence.Rejected {
		if evidence.Kind == runtimecontract.EvidenceKindValidation {
			runEvidence.RejectedValidation++
		}
		if runtimecontract.EvidenceBelongsToCurrentFocusForDiagnosticLineage(facts, evidence) {
			rejected++
		}
	}
	activeDispatchID := ""
	if state.ActiveDispatch != nil {
		activeDispatchID = strings.TrimSpace(state.ActiveDispatch.ID)
	}
	acceptedAt, effectAt, effectCount := runtimeOwnerEvidenceProgress(state)
	ret := projectioncontract.RuntimeProgress{
		ImplementationWork:              runtimeImplementationProgress(state),
		RunEvidence:                     runEvidence,
		LatestAcceptedEvidenceAt:        acceptedAt,
		LatestDurableEffectAt:           effectAt,
		DurableEffectCount:              effectCount,
		StateRevision:                   state.StateRevision,
		EvidenceSequence:                state.EvidenceSequence,
		CompletedDispatchCount:          len(state.CompletedDispatches),
		CandidateAttemptCount:           len(state.CandidateAttempts),
		AcceptedEvidenceCount:           accepted,
		AcceptedValidationEvidenceCount: acceptedValidation,
		RejectedEvidenceCount:           rejected,
		MissingEvidenceKinds:            runtimecontract.MissingRequiredEvidenceKinds(facts),
		CurrentFocusID:                  strings.TrimSpace(facts.Focus.ID),
		ConvergenceState:                string(state.Decision.State),
		ActiveDispatchID:                activeDispatchID,
	}
	if err := runtimecontract.ValidateRuntimeStateTerminalDeliveryLedger(state.Decision, state.Termination, state.DeliveryAcknowledgements, state.Facts.Recovery); err == nil {
		for _, acknowledgement := range state.DeliveryAcknowledgements {
			ret.TerminalAcknowledgedOwners = append(ret.TerminalAcknowledgedOwners, string(acknowledgement.Owner))
		}
	}
	for _, item := range facts.Coverage {
		if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			ret.SatisfiedCWICount++
		}
	}
	ret.ValidationFailureFingerprint, ret.ValidationFailureOccurrences = runtimeValidationFailureComparison(facts)
	if state.Decision.Terminal != nil {
		ret.TerminalCode = strings.TrimSpace(state.Decision.Terminal.Code)
	}
	if facts.Failure != nil {
		ret.FailureClass = string(facts.Failure.Class)
		ret.FailureCode = strings.TrimSpace(facts.Failure.Code)
		ret.FailureSignature = strings.TrimSpace(facts.Failure.Signature)
		ret.FailureMessage = strings.TrimSpace(facts.Failure.Message)
		ret.OriginalDiagnostic = facts.Failure.OriginalDiagnostic.CloneOriginal()
	}
	if state.Decision.Terminal != nil && state.Decision.Terminal.OriginalDiagnostic != nil {
		ret.OriginalDiagnostic = state.Decision.Terminal.OriginalDiagnostic.CloneOriginal()
	}
	if state.ExecutionBudget.Policy.PolicyRevision != "" {
		activeWaitID := ""
		if wait, found := activeRuntimeRecoverableWait(state.RecoverableWaits); found {
			activeWaitID = wait.WaitID
			if view, viewErr := runtimecontract.RuntimeRecoverableWaitProjection(wait, state.StateRevision); viewErr == nil {
				ret.RecoverableWait = &view
			}
		}
		terminalCode := ""
		if state.Decision.Terminal != nil {
			terminalCode = strings.TrimSpace(state.Decision.Terminal.Code)
		}
		if view, err := runtimecontract.BuildRuntimeExecutionBudgetView(facts.WorkspaceID, facts.RunID, state.StateRevision, state.ExecutionBudget, activeWaitID, terminalCode, runtimecontract.RuntimeExecutionBudgetSourceUsageFromFacts(facts)); err == nil {
			ret.ExecutionBudget = &view
		}
	}
	ret.PrerequisiteRefreshUsed = facts.Recovery.Used(runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh)
	ret.PrerequisiteRefreshLimit = facts.RecoveryPolicy.Limit(runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh)
	ret.PrerequisiteInvalidations = mapPrerequisiteRefreshInvalidations(state.PrerequisiteInvalidations)
	ret.PrerequisiteRefreshState = runtimePrerequisiteRefreshState(facts, ret.PrerequisiteInvalidations)
	return ret
}

func runtimeImplementationProgress(state runtimecontract.RuntimeStateRecord) *projectioncontract.RuntimeImplementationProgress {
	if len(state.ImplementationWork.Rounds) == 0 && len(state.ImplementationWork.PurposeChoices) == 0 {
		return nil
	}
	ret := &projectioncontract.RuntimeImplementationProgress{ReservedRounds: len(state.ImplementationWork.Rounds), SemanticGenerations: state.SemanticPlanning.HistoryCount()}
	if archive := state.SemanticPlanning.HistoryArchive; archive != nil {
		ret.ArchivedSegments = len(archive.Segments)
	}
	dispatches := append([]runtimecontract.RuntimeDispatchRecord(nil), state.CompletedDispatches...)
	if state.ActiveDispatch != nil {
		dispatches = append(dispatches, *state.ActiveDispatch)
	}
	for _, round := range state.ImplementationWork.Rounds {
		if round.CompletedReceiptSetHash != "" {
			ret.CompletedRounds++
		}
		// A late receipt cannot erase the original unknown round disposition.
		if round.OutcomeUnknownExecutionKey != "" {
			ret.UnknownExecutions++
		}
		for _, dispatch := range dispatches {
			if dispatch.ID != round.DispatchID {
				continue
			}
			for _, execution := range dispatch.OperationExecutions {
				if execution.State != runtimecontract.OperationExecutionOwnerCommitted && execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
					continue
				}
				receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(state.OperationReceipts, execution.ExecutionKey, execution.Fence)
				if found && receipt.OperationID == execution.OperationID && execution.OwnerReceiptHash == runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
					ret.CommittedReceipts++
				}
			}
		}
	}
	return ret
}

func activeRuntimeRecoverableWait(values []runtimecontract.RuntimeRecoverableWaitRecord) (runtimecontract.RuntimeRecoverableWaitRecord, bool) {
	for _, raw := range values {
		wait := raw.Clone()
		if wait.Active() {
			return wait, true
		}
	}
	return runtimecontract.RuntimeRecoverableWaitRecord{}, false
}

func runtimePrerequisiteRefreshState(facts runtimecontract.RuntimeFacts, invalidations []projectioncontract.RuntimePrerequisiteRefreshInvalidation) string {
	if facts.Failure != nil && facts.Failure.Class == runtimecontract.FailureClassPrerequisiteRefreshExhausted {
		return "prerequisite_refresh_exhausted"
	}
	if len(invalidations) == 0 {
		return ""
	}
	pending := make(map[string]struct{}, len(facts.Prerequisites.Pending))
	for _, prerequisite := range facts.Prerequisites.Pending {
		if id := strings.TrimSpace(prerequisite.ID); id != "" {
			pending[id] = struct{}{}
		}
	}
	for _, invalidation := range invalidations {
		for _, prerequisiteID := range invalidation.PrerequisiteIDs {
			if _, found := pending[prerequisiteID]; found {
				return "prerequisite_refreshing"
			}
		}
	}
	return ""
}

func mapPrerequisiteRefreshInvalidations(values []runtimecontract.RuntimePrerequisiteInvalidationRecord) []projectioncontract.RuntimePrerequisiteRefreshInvalidation {
	values = runtimecontract.NormalizeRuntimePrerequisiteInvalidations(values)
	ret := make([]projectioncontract.RuntimePrerequisiteRefreshInvalidation, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		item := projectioncontract.RuntimePrerequisiteRefreshInvalidation{
			Signature:       value.Signature,
			PrerequisiteIDs: append([]string(nil), value.PrerequisiteIDs...),
			ObservationIDs:  append([]string(nil), value.ObservationIDs...),
			DispatchID:      value.DispatchID,
			OperationID:     value.OperationID,
			ReceiptHash:     value.ReceiptHash,
		}
		if value.Stale != nil {
			stale := value.Stale.Normalized()
			if value.DispatchID == "" {
				item.Source = "seal_time_probe_stale"
			} else {
				item.Source = "execution_receipt_stale"
			}
			item.OwnerModuleID = stale.OwnerModuleID
			item.ResourceDomain = string(stale.Domain)
			item.SubjectRef = stale.SubjectRef
			item.ExpectedRevision = stale.ExpectedRevision
			item.ObservedRevision = stale.ObservedRevision
			item.Reason = stale.Reason
		} else {
			item.Source = "owner_receipt_resource_change"
			for _, rawChange := range value.ResourceChanges {
				change := rawChange.Normalized()
				item.ResourceChanges = append(item.ResourceChanges, projectioncontract.RuntimePrerequisiteResourceChange{
					OwnerModuleID: change.OwnerModuleID, ResourceDomain: string(change.Domain), SubjectRef: change.SubjectRef,
					BeforeRevision: change.BeforeRevision, AfterRevision: change.AfterRevision,
				})
			}
			if len(item.ResourceChanges) == 1 {
				change := item.ResourceChanges[0]
				item.OwnerModuleID = change.OwnerModuleID
				item.ResourceDomain = change.ResourceDomain
				item.SubjectRef = change.SubjectRef
				item.ExpectedRevision = change.BeforeRevision
				item.ObservedRevision = change.AfterRevision
			}
		}
		ret = append(ret, item)
	}
	return ret
}

func applyRuntimeProgressToMilestones(progress *projectioncontract.TaskProgress, summary projectioncontract.RuntimeProgress, runState string) {
	if progress == nil {
		return
	}
	terminalFailed := runState == string(common.RunStateFailed) || progress.State == string(common.RunStateFailed)
	terminalCompleted := runState == string(common.RunStateCompleted) || progress.State == string(common.RunStateCompleted)
	missingValidation := runtimeContainsString(summary.MissingEvidenceKinds, runtimecontract.EvidenceKindValidation)
	for idx := range progress.Milestones {
		item := &progress.Milestones[idx]
		switch item.ID {
		case "execution_steps":
			switch {
			case terminalFailed:
				item.State = "failed"
			case terminalCompleted:
				item.State = "completed"
			case summary.CompletedDispatchCount > 0 || summary.AcceptedEvidenceCount > 0 || summary.CandidateAttemptCount > 0:
				if item.State == "pending" || item.State == "" {
					item.State = "running"
				}
			}
			if summary.AcceptedEvidenceCount > 0 {
				item.EvidenceRefs = compactUniqueStrings(append(item.EvidenceRefs, "runtime/accepted-evidence"))
			}
		case "validation":
			switch {
			case terminalFailed && summary.AcceptedValidationEvidenceCount == 0:
				item.State = "failed"
			case summary.AcceptedValidationEvidenceCount > 0:
				item.State = "completed"
				item.EvidenceRefs = compactUniqueStrings(append(item.EvidenceRefs, "runtime/validation-evidence"))
			case missingValidation && summary.RejectedEvidenceCount > 0:
				item.State = "failed"
			case missingValidation && (summary.AcceptedEvidenceCount > 0 || summary.CandidateAttemptCount > 0):
				if item.State == "pending" || item.State == "" {
					item.State = "running"
				}
			}
		}
	}
	progress.CurrentMilestone = currentProgressMilestone(progress.Milestones)
}

func applyRuntimeProgressToRepairLoop(progress *projectioncontract.TaskProgress, summary projectioncontract.RuntimeProgress, state runtimecontract.RuntimeStateRecord) {
	if progress == nil {
		return
	}
	loop := progress.RepairLoop
	facts := state.Facts.Normalized()
	repairAttempts := facts.Repair.RepairAttempts
	loop.AttemptCount = repairAttempts
	loop.LastAttempt = repairAttempts
	loop.LastFailureClass = ""
	loop.LastFailureSignature = ""
	loop.RepeatedFailureReason = ""
	loop.Escalated = false
	switch {
	case progress.State == string(common.RunStateFailed):
		if repairAttempts > 0 || strings.TrimSpace(summary.FailureClass) != "" {
			loop.State = "terminated"
		} else if loop.State == "" {
			loop.State = "inactive"
		}
	case progress.State == string(common.RunStateCompleted):
		// Completed runs must not keep reporting an active repair loop.
		if repairAttempts > 0 {
			loop.State = "resolved"
		} else {
			loop.State = "inactive"
		}
	case repairAttempts > 0 && (facts.Repair.ActiveGap != nil || facts.Failure != nil):
		loop.State = "active"
	default:
		loop.State = "inactive"
	}
	if strings.TrimSpace(summary.FailureClass) != "" {
		loop.LastFailureClass = summary.FailureClass
	}
	if strings.TrimSpace(summary.FailureSignature) != "" {
		loop.LastFailureSignature = summary.FailureSignature
	}
	for _, repeated := range facts.Repair.SignatureCounts {
		if repeated.Signature == summary.FailureSignature && repeated.Signature != "" && repeated.Count > 1 {
			loop.RepeatedFailureReason = summary.FailureMessage
			break
		}
	}
	if summary.ValidationFailureOccurrences > 1 && facts.Failure != nil {
		loop.RepeatedFailureReason = "same validation check failure recurred across repair transactions"
	}
	progress.RepairLoop = loop
}

// A causation-only repair is a durable effect, not acceptance progress. Both
// clocks require an exact committed receipt; neither activity nor an unbound
// read can supply the acceptance clock.
func latestRuntimeAcceptedEvidenceAt(state runtimecontract.RuntimeStateRecord) string {
	latest, _, _ := runtimeOwnerEvidenceProgress(state)
	return latest
}

func runtimeOwnerEvidenceProgress(state runtimecontract.RuntimeStateRecord) (string, string, int) {
	latest, effectAt := "", ""
	effects := make(map[string]struct{})
	dispatches := append([]runtimecontract.RuntimeDispatchRecord(nil), state.CompletedDispatches...)
	if state.ActiveDispatch != nil {
		dispatches = append(dispatches, *state.ActiveDispatch)
	}
	for _, fact := range state.Facts.Evidence.Accepted {
		for _, dispatch := range dispatches {
			if dispatch.ID != fact.DispatchID || dispatch.WorkspaceID != state.Facts.WorkspaceID || dispatch.RunID != state.Facts.RunID {
				continue
			}
			for _, execution := range dispatch.OperationExecutions {
				if execution.OperationID != fact.OperationID || execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
					continue
				}
				receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(state.OperationReceipts, execution.ExecutionKey, execution.Fence)
				if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.OperationID != fact.OperationID ||
					execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
					continue
				}
				if len(fact.CoverageWorkItemIDs) > 0 {
					latest = latestStringTime(latest, receipt.CompletedAt)
				}
				if receipt.Changed && runtimecontract.RuntimeEffectIsDurable(receipt.ObservedEffectScope) {
					effectAt = latestStringTime(effectAt, receipt.CompletedAt)
					effects[receipt.ExecutionKey] = struct{}{}
				}
			}
		}
	}
	return latest, effectAt, len(effects)
}

func runtimeValidationFailureComparison(facts runtimecontract.RuntimeFacts) (string, int) {
	for i := len(facts.Evidence.Rejected) - 1; i >= 0; i-- {
		fact := facts.Evidence.Rejected[i]
		if !runtimecontract.EvidenceBelongsToCurrentFocusForDiagnosticLineage(facts, fact) {
			continue
		}
		if fingerprint := runtimecontract.RuntimeValidationFailureFingerprint(fact, facts.Focus.CoverageEvidenceBindings...); fingerprint != "" {
			return fingerprint, runtimecontract.RuntimeValidationFailureOccurrences(facts.Evidence, fact, facts.Focus.CoverageEvidenceBindings...)
		}
	}
	return "", 0
}

func applyRuntimeProgressToCurrentStep(progress *projectioncontract.TaskProgress, summary projectioncontract.RuntimeProgress, state runtimecontract.RuntimeStateRecord) {
	if progress == nil {
		return
	}
	// Prefer real active/completed dispatch identity over authority snapshot IDs.
	if progress.CurrentStepID == "" {
		if summary.ActiveDispatchID != "" {
			progress.CurrentStepID = summary.ActiveDispatchID
		} else if len(state.CompletedDispatches) > 0 {
			last := state.CompletedDispatches[len(state.CompletedDispatches)-1]
			if id := strings.TrimSpace(last.ID); id != "" {
				progress.CurrentStepID = id
			}
		}
		if progress.CurrentStepID != "" {
			progress.DiagnosticLineage = progress.CurrentStepID
		}
	}
	if progress.CurrentStepKind == "" && summary.ConvergenceState != "" {
		progress.CurrentStepKind = "runtime/" + summary.ConvergenceState
	}
	if progress.CurrentStepState == "" {
		switch progress.State {
		case string(common.RunStateFailed):
			progress.CurrentStepState = "rejected"
		case string(common.RunStateCompleted):
			progress.CurrentStepState = "completed"
		default:
			if summary.ActiveDispatchID != "" {
				progress.CurrentStepState = "executing"
			} else if summary.CandidateAttemptCount > 0 {
				progress.CurrentStepState = "running"
			}
		}
	}
}

func runtimeContainsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
