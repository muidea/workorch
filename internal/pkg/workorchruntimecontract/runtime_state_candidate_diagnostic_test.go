package workorchruntimecontract

import "testing"

func TestRuntimeStateRecordClonePreservesCandidateFailureDiagnostic(t *testing.T) {
	diagnostic := &RuntimeCandidateFailureDiagnostic{
		OperationID: "operation", Path: "$.operations[id=operation].targetRefs", Actual: "directory",
		Allowed: []string{"replacement"}, ManifestHash: "manifest", RejectedShapeHash: "shape",
		Objectives: []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionReplaceTarget},
	}
	state := RuntimeStateRecord{CandidateAttempts: []RuntimeCandidateAttemptRecord{{
		Failure: &RuntimeCandidateFailure{Class: RuntimeCandidateFailureClassInfeasible, Code: "operation_target_not_regular", Signature: "signature", Reason: "owner detail", Diagnostic: diagnostic},
	}}}
	clone := state.Clone()
	got := clone.CandidateAttempts[0].Failure.Diagnostic
	if got == nil || got == diagnostic || got.ManifestHash != "manifest" || got.RejectedShapeHash != "shape" || len(got.Allowed) != 1 || got.Allowed[0] != "replacement" {
		t.Fatalf("candidate diagnostic clone=%+v", got)
	}
	diagnostic.Allowed[0] = "mutated"
	if got.Allowed[0] != "replacement" {
		t.Fatalf("clone aliases candidate diagnostic: %+v", got)
	}
}
