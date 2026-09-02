package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeStateRecordClonePreservesCandidateFailureDiagnostic(t *testing.T) {
	diagnostic := &RuntimeCandidateFailureDiagnostic{
		OperationID: "operation", Path: "$.operations[id=operation].targetRefs", Actual: "directory",
		Allowed: []string{"replacement"}, ManifestHash: "manifest", RejectedShapeHash: "shape",
		WireRule: &RuntimeCandidateWireRuleIdentity{
			WireContractHash: "wire-contract", OperationIndex: 0, OperationKind: RuntimeBlockKindPatchApply,
		},
		Objectives: []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionReplaceTarget},
	}
	state := RuntimeStateRecord{CandidateAttempts: []RuntimeCandidateAttemptRecord{{
		Failure: &RuntimeCandidateFailure{Class: RuntimeCandidateFailureClassContract, Code: "candidate_target_outside_grounding", Signature: "signature", Reason: "candidate detail", Diagnostic: diagnostic},
	}}}
	clone := state.Clone()
	got := clone.CandidateAttempts[0].Failure.Diagnostic
	if got == nil || got == diagnostic || got.WireRule == nil || got.WireRule == diagnostic.WireRule || got.ManifestHash != "manifest" || got.RejectedShapeHash != "shape" || len(got.Allowed) != 1 || got.Allowed[0] != "replacement" {
		t.Fatalf("candidate diagnostic clone=%+v", got)
	}
	diagnostic.Allowed[0] = "mutated"
	diagnostic.WireRule.WireContractHash = "mutated"
	if got.Allowed[0] != "replacement" {
		t.Fatalf("clone aliases candidate diagnostic: %+v", got)
	}
	if got.WireRule.WireContractHash != "wire-contract" {
		t.Fatalf("clone aliases candidate diagnostic wire rule: %+v", got.WireRule)
	}
}

func TestRuntimeStateRecordCloneDetachesDefinitionFocuses(t *testing.T) {
	state := RuntimeStateRecord{Facts: RuntimeFacts{DefinitionFocuses: []RuntimeDefinitionFocus{{
		ID: "definition-focus-1", SliceID: "slice-1",
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange},
	}}}}
	clone := state.Clone()
	clone.Facts.DefinitionFocuses[0].CoverageWorkItemIDs[0] = "cwi-mutated"
	if state.Facts.DefinitionFocuses[0].CoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatal("RuntimeStateRecord clone aliases immutable definition focus")
	}
}
