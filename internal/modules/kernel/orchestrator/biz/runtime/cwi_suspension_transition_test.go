package runtime

import (
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// These are isolated write-set gates. Persisted source validity and actual CAS
// recovery are separately exercised by the real-owner mainline matrix.
func TestCWISuspensionTransitionRejectsPauseWriteSetSmuggling(t *testing.T) {
	previous := runtimeSuspensionProjectionFixture(t)
	next := previous.CloneRaw()
	if changed, err := suspendRuntimeSuccessorForStalePredecessor(&next); err != nil || !changed {
		t.Fatalf("pause fixture: %v", err)
	}
	next.StateRevision++
	if err := validateRuntimeCWISuspensionTransition(previous, next); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"history_removed", "revision", "focus", "repair", "directive", "failure", "archive", "effect", "receipt", "budget", "pre_resumed", "dispatch", "submitted", "terminal", "material", "termination"} {
		t.Run(scenario, func(t *testing.T) {
			before, after := previous.CloneRaw(), next.CloneRaw()
			s := &after.Facts.CWISuspensions[0]
			switch scenario {
			case "history_removed":
				before = next.CloneRaw()
				after.Facts.CWISuspensions = nil
			case "revision":
				s.SuspendedAtStateRevision++
			case "focus":
				s.FrozenFocus.TargetRefs = append(s.FrozenFocus.TargetRefs, "drift")
			case "repair":
				s.Repair.RepairAttempts = 0
			case "directive":
				s.RepairDirective = &runtimecontract.RuntimeRepairDirective{FocusID: "invented"}
			case "failure":
				s.Failure = &runtimecontract.FailureFact{Code: "invented"}
			case "archive":
				s.FrozenPrerequisites = append(s.FrozenPrerequisites, runtimecontract.RuntimePrerequisite{ID: "invented"})
			case "effect":
				after.CompletedDispatches[0].ID += "drift"
			case "receipt":
				after.OperationReceipts = append(after.OperationReceipts, runtimecontract.RuntimeOperationReceipt{})
			case "budget":
				after.ExecutionBudget.Policy.PolicyHash += "drift"
			case "pre_resumed":
				s.ResumeBinding = &runtimecontract.RuntimeCWIBindingRef{BindingID: "invented"}
			case "dispatch":
				before.ActiveDispatch = &SealedDispatch{ID: "in-flight"}
			case "submitted":
				before.CandidateRequestSessions = append(before.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{State: runtimecontract.RuntimeCandidateRequestSessionPrepared, ProviderSubmitted: true})
			case "terminal":
				before.Decision.State = runtimecontract.ConvergenceStateFailed
			case "material":
				before.TerminalMaterialSeal = &runtimecontract.RuntimeTerminalMaterialSeal{}
			case "termination":
				before.Termination = &runtimecontract.RuntimeTerminationRecord{}
			}
			originalBefore, originalAfter := before.CloneRaw(), after.CloneRaw()
			if err := validateRuntimeCWISuspensionTransition(before, after); err == nil {
				t.Fatal("illegal pause write set accepted")
			}
			if !reflect.DeepEqual(before, originalBefore) || !reflect.DeepEqual(after, originalAfter) {
				t.Fatal("write-set rejection mutated caller inputs")
			}
		})
	}
}

func TestCWISuspensionTransitionRejectsResumeRewriteAndUnsafePreimage(t *testing.T) {
	previous := runtimeSuspensionProjectionFixture(t)
	if changed, err := suspendRuntimeSuccessorForStalePredecessor(&previous); err != nil || !changed {
		t.Fatalf("pause fixture: %v", err)
	}
	previous.StateRevision++
	next := previous.CloneRaw()
	s := &next.Facts.CWISuspensions[0]
	s.ResumeBinding = &runtimecontract.RuntimeCWIBindingRef{BindingID: "new-ordinary-proof"}
	s.ResumeAssessmentID, s.ResumeEvidenceRevision, s.ResumeObservationCount = "actual-assessment", "revision-3", 1
	next.Facts.Focus, next.Facts.Repair = s.FrozenFocus, s.Repair
	next.Facts.RepairDirective, next.Facts.Failure = s.RepairDirective, s.Failure
	if err := validateRuntimeCWISuspensionTransition(previous, next); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"original_source", "resume_twice", "binding_missing", "focus_body", "repair", "directive", "failure", "dispatch", "submitted", "terminal", "material", "termination"} {
		t.Run(scenario, func(t *testing.T) {
			before, after := previous.CloneRaw(), next.CloneRaw()
			switch scenario {
			case "original_source":
				after.Facts.CWISuspensions[0].EffectEvidenceID += "drift"
			case "resume_twice":
				before = next.CloneRaw()
				after.Facts.CWISuspensions[0].ResumeAssessmentID += "drift"
			case "binding_missing":
				after.Facts.CWISuspensions[0].ResumeBinding = nil
			case "focus_body":
				after.Facts.Focus.TargetRefs = append(after.Facts.Focus.TargetRefs, "drift")
			case "repair":
				after.Facts.Repair.RepairAttempts = 0
			case "directive":
				after.Facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{FocusID: "invented"}
			case "failure":
				after.Facts.Failure = &runtimecontract.FailureFact{Code: "invented"}
			case "dispatch":
				before.ActiveDispatch = &SealedDispatch{ID: "in-flight"}
			case "submitted":
				before.CandidateRequestSessions = append(before.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{State: runtimecontract.RuntimeCandidateRequestSessionPrepared, ProviderSubmitted: true})
			case "terminal":
				before.Decision.State = runtimecontract.ConvergenceStateFailed
			case "material":
				before.TerminalMaterialSeal = &runtimecontract.RuntimeTerminalMaterialSeal{}
			case "termination":
				before.Termination = &runtimecontract.RuntimeTerminationRecord{}
			}
			originalBefore, originalAfter := before.CloneRaw(), after.CloneRaw()
			if err := validateRuntimeCWISuspensionTransition(before, after); err == nil {
				t.Fatal("illegal resume write set accepted")
			}
			if !reflect.DeepEqual(before, originalBefore) || !reflect.DeepEqual(after, originalAfter) {
				t.Fatal("write-set rejection mutated caller inputs")
			}
		})
	}
}
