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
	if progress.Summary == "" && strings.TrimSpace(summary.FailureMessage) != "" {
		progress.Summary = strings.TrimSpace(summary.FailureMessage)
	}
}

func mapRuntimeV2Progress(state runtimecontract.RuntimeStateRecord) readmodelcore.RuntimeV2Progress {
	facts := state.Facts.Normalized()
	accepted := 0
	rejected := 0
	for _, evidence := range facts.Evidence.Accepted {
		if runtimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) {
			accepted++
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
		StateRevision:          state.StateRevision,
		EvidenceSequence:       state.EvidenceSequence,
		CompletedDispatchCount: len(state.CompletedDispatches),
		CandidateAttemptCount:  len(state.CandidateAttempts),
		AcceptedEvidenceCount:  accepted,
		RejectedEvidenceCount:  rejected,
		MissingEvidenceKinds:   runtimecontract.MissingRequiredEvidenceKinds(facts),
		CurrentFocusID:         strings.TrimSpace(facts.Focus.ID),
		CurrentAction:          string(state.Decision.Action),
		ActiveDispatchID:       activeDispatchID,
	}
	if facts.Failure != nil {
		ret.FailureClass = string(facts.Failure.Class)
		ret.FailureCode = strings.TrimSpace(facts.Failure.Code)
		ret.FailureSignature = strings.TrimSpace(facts.Failure.Signature)
		ret.FailureMessage = strings.TrimSpace(facts.Failure.Message)
	}
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

func runtimeV2RepairAttemptStats(attempts []runtimecontract.RuntimeCandidateAttemptRecord) (count int, lastAttempt int) {
	for _, attempt := range attempts {
		// Count only failed attempts and explicit retries (attempt > 0).
		// Healthy first-pass attempt=0/accepted must not mark the run as repairing.
		if attempt.State == runtimecontract.RuntimeCandidateAttemptStateFailed || attempt.Attempt > 0 {
			count++
			if attempt.Attempt+1 > lastAttempt {
				lastAttempt = attempt.Attempt + 1
			}
		}
	}
	return count, lastAttempt
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
			case terminalFailed && missingValidation:
				item.State = "failed"
			case !missingValidation && summary.AcceptedEvidenceCount > 0:
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
	repairAttempts, lastRepairAttempt := runtimeV2RepairAttemptStats(state.CandidateAttempts)
	if repairAttempts > loop.AttemptCount {
		loop.AttemptCount = repairAttempts
	}
	if lastRepairAttempt > loop.LastAttempt {
		loop.LastAttempt = lastRepairAttempt
	}
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
	case repairAttempts > 0:
		loop.State = "active"
	default:
		if loop.State == "" {
			loop.State = "inactive"
		}
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
	for _, code := range summary.RepairHintCodes {
		loop.RecommendedNextSteps = compactUniqueStrings(append(loop.RecommendedNextSteps, code))
	}
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
