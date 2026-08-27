package workorchruntimecontract

import "testing"

// The correction disposition is an SM-3 policy owned by Orchestrator. The
// shared contract package only owns the value object's structural invariant.
func TestRuntimeCandidateCorrectionDirectiveValidate(t *testing.T) {
	t.Parallel()
	if err := (RuntimeCandidateCorrectionDirective{
		Disposition:  RuntimeCandidateCorrectionCorrectableOnce,
		Objectives:   []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionSelectOperationKind},
		ManifestHash: "manifest-1",
		Witness: &RuntimeCandidateCorrectionWitness{
			Dimension: RuntimeCandidateCorrectionWitnessSchemaField,
			Path:      "$.operations", RejectedValue: "field_absent", AlternativeValue: "field_present",
		},
	}).Validate(); err != nil {
		t.Fatalf("correctable directive rejected: %v", err)
	}
	operationWitness := RuntimeCandidateCorrectionWitness{
		Dimension: RuntimeCandidateCorrectionWitnessTarget, OperationKind: RuntimeBlockKindPatchApply,
		Path: "$.operations[0].targetRefs", RejectedValue: "main.go", AlternativeValue: "support.go",
	}
	if err := operationWitness.Validate(); err == nil {
		t.Fatal("operation-level witness without distinct semantic shape hashes accepted")
	}
	operationWitness.RejectedShapeHash = "shape-main"
	operationWitness.AlternativeShapeHash = "shape-support"
	if err := operationWitness.Validate(); err != nil {
		t.Fatalf("operation-level witness with distinct semantic shapes rejected: %v", err)
	}
	if err := (RuntimeCandidateCorrectionDirective{
		Disposition:  RuntimeCandidateCorrectionRejectedFinal,
		ManifestHash: "manifest-1",
		TerminalCode: "runtime_contract_defect",
	}).Validate(); err != nil {
		t.Fatalf("rejected-final directive rejected: %v", err)
	}
	if err := (RuntimeCandidateCorrectionDirective{Disposition: RuntimeCandidateCorrectionCorrectableOnce}).Validate(); err == nil {
		t.Fatal("empty correctable directive accepted")
	}
	if err := (RuntimeCandidateCorrectionDirective{Disposition: RuntimeCandidateCorrectionRejectedFinal, TerminalCode: "candidate_authority_exhausted"}).Validate(); err == nil {
		t.Fatal("rejected-final directive without sealed manifest identity accepted")
	}
}
