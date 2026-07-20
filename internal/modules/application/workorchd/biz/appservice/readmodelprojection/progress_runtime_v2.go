package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func applyRuntimeV2Progress(progress *readmodelcore.TaskProgress, run *runmodel.Run) {
	if progress == nil || run == nil || run.RuntimeState == nil {
		return
	}
	summary := mapRuntimeV2Progress(*run.RuntimeState)
	progress.RuntimeV2 = &summary
	applyRuntimeV2ProgressToMilestones(progress, summary, string(common.CanonicalRunState(run.State)))
	applyRuntimeV2ProgressToRepairLoop(progress, summary, *run.RuntimeState)
	applyRuntimeV2ProgressToCurrentStep(progress, summary, *run.RuntimeState)
	if runtimeSummary := RuntimeV2ProgressSummary(&summary); runtimeSummary != "" {
		progress.Summary = runtimeSummary
	} else if progress.Summary == "" && strings.TrimSpace(summary.FailureMessage) != "" {
		progress.Summary = strings.TrimSpace(summary.FailureMessage)
	}
}

func mapRuntimeV2Progress(state runtimecontract.RuntimeStateRecord) readmodelcore.RuntimeV2Progress {
	facts := state.Facts.Normalized()
	accepted := 0
	acceptedValidation := 0
	rejected := 0
	for _, evidence := range facts.Evidence.Accepted {
		if runtimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) {
			accepted++
			if evidence.Kind == runtimecontract.EvidenceKindValidation {
				acceptedValidation++
			}
		}
	}
	for _, evidence := range facts.Evidence.Rejected {
		if runtimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) {
			rejected++
		}
	}
	activeDispatchID := ""
	if state.ActiveDispatch != nil {
		activeDispatchID = strings.TrimSpace(state.ActiveDispatch.ID)
	}
	ret := readmodelcore.RuntimeV2Progress{
		StateRevision:                   state.StateRevision,
		EvidenceSequence:                state.EvidenceSequence,
		CompletedDispatchCount:          len(state.CompletedDispatches),
		CandidateAttemptCount:           len(state.CandidateAttempts),
		AcceptedEvidenceCount:           accepted,
		AcceptedValidationEvidenceCount: acceptedValidation,
		RejectedEvidenceCount:           rejected,
		MissingEvidenceKinds:            runtimecontract.MissingRequiredEvidenceKinds(facts),
		CurrentFocusID:                  strings.TrimSpace(facts.Focus.ID),
		CurrentAction:                   string(state.Decision.Action),
		ActiveDispatchID:                activeDispatchID,
	}
	if facts.Failure != nil {
		ret.FailureClass = string(facts.Failure.Class)
		ret.FailureCode = strings.TrimSpace(facts.Failure.Code)
		ret.FailureSignature = strings.TrimSpace(facts.Failure.Signature)
		ret.FailureMessage = strings.TrimSpace(facts.Failure.Message)
	}
	ret.PrerequisiteRefreshUsed = facts.Recovery.Used(runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh)
	ret.PrerequisiteRefreshLimit = facts.RecoveryPolicy.Limit(runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh)
	ret.PrerequisiteResourceChangeRefreshUsed = facts.Recovery.Used(runtimecontract.RuntimeRecoveryBudgetPrerequisiteResourceChangeRefresh)
	ret.PrerequisiteResourceChangeRefreshLimit = facts.RecoveryPolicy.Limit(runtimecontract.RuntimeRecoveryBudgetPrerequisiteResourceChangeRefresh)
	ret.PrerequisiteInvalidations = mapPrerequisiteRefreshInvalidations(state.PrerequisiteInvalidations)
	ret.PrerequisiteRefreshState = runtimePrerequisiteRefreshState(facts, ret.PrerequisiteInvalidations)
	hints := runtimecontract.DeriveRuntimeRepairHints(facts)
	if len(hints) > 0 {
		ret.RepairHintCodes = make([]string, 0, len(hints))
		for _, hint := range hints {
			if code := strings.TrimSpace(hint.Code); code != "" {
				ret.RepairHintCodes = append(ret.RepairHintCodes, code)
			}
		}
	}
	return ret
}

func runtimePrerequisiteRefreshState(facts runtimecontract.RuntimeFacts, invalidations []readmodelcore.RuntimePrerequisiteRefreshInvalidation) string {
	if facts.Failure != nil && facts.Failure.Class == runtimecontract.FailureClassPrerequisiteRefreshExhausted {
		if facts.Failure.Code == "prerequisite_receipt_refresh_exhausted" {
			return "prerequisite_receipt_refresh_exhausted"
		}
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

func mapPrerequisiteRefreshInvalidations(values []runtimecontract.RuntimePrerequisiteInvalidationRecord) []readmodelcore.RuntimePrerequisiteRefreshInvalidation {
	values = runtimecontract.NormalizeRuntimePrerequisiteInvalidations(values)
	ret := make([]readmodelcore.RuntimePrerequisiteRefreshInvalidation, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		item := readmodelcore.RuntimePrerequisiteRefreshInvalidation{
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
				item.ResourceChanges = append(item.ResourceChanges, readmodelcore.RuntimePrerequisiteResourceChange{
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

func applyRuntimeV2ProgressToMilestones(progress *readmodelcore.TaskProgress, summary readmodelcore.RuntimeV2Progress, runState string) {
	if progress == nil {
		return
	}
	terminalFailed := runState == string(common.RunStateFailed) || progress.State == string(common.RunStateFailed)
	terminalCompleted := runState == string(common.RunStateCompleted) || progress.State == string(common.RunStateCompleted)
	missingValidation := runtimeV2ContainsString(summary.MissingEvidenceKinds, runtimecontract.EvidenceKindValidation)
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
				item.EvidenceRefs = compactUniqueStrings(append(item.EvidenceRefs, "runtime-v2/accepted-evidence"))
			}
		case "validation":
			switch {
			case terminalFailed && summary.AcceptedValidationEvidenceCount == 0:
				item.State = "failed"
			case summary.AcceptedValidationEvidenceCount > 0:
				item.State = "completed"
				item.EvidenceRefs = compactUniqueStrings(append(item.EvidenceRefs, "runtime-v2/validation-evidence"))
			case missingValidation && summary.RejectedEvidenceCount > 0:
				item.State = "failed"
			case missingValidation && (summary.AcceptedEvidenceCount > 0 || summary.CandidateAttemptCount > 0):
				if item.State == "pending" || item.State == "" {
					item.State = "running"
				}
			}
		case "formal_result":
			if terminalFailed {
				item.State = "failed"
			} else if terminalCompleted {
				item.State = "completed"
			}
		}
	}
	progress.CurrentMilestone = CurrentProgressMilestone(progress.Milestones)
}

func applyRuntimeV2ProgressToRepairLoop(progress *readmodelcore.TaskProgress, summary readmodelcore.RuntimeV2Progress, state runtimecontract.RuntimeStateRecord) {
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
	loop.RecommendedNextSteps = nil
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
	if strings.TrimSpace(summary.FailureMessage) != "" {
		loop.RepeatedFailureReason = summary.FailureMessage
	}
	loop.RecommendedNextSteps = compactUniqueStrings(summary.RepairHintCodes)
	progress.RepairLoop = loop
}

func applyRuntimeV2ProgressToCurrentStep(progress *readmodelcore.TaskProgress, summary readmodelcore.RuntimeV2Progress, state runtimecontract.RuntimeStateRecord) {
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
	if progress.CurrentStepKind == "" && summary.CurrentAction != "" {
		progress.CurrentStepKind = "runtime_v2/" + summary.CurrentAction
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
	if progress.CurrentActionLabel == "" && summary.CurrentAction != "" {
		progress.CurrentActionLabel = summary.CurrentAction
	}
}

func runtimeV2ContainsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
