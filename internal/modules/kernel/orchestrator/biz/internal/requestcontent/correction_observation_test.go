package requestcontent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCorrectionObservationsExcludeOwnerPrivateAuditFieldsBeforeEventHub(t *testing.T) {
	witness := &runtimecontract.RuntimeCandidateCorrectionWitness{
		Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField, Path: "$.operations",
		RejectedValue: "field_absent", AlternativeValue: "field_present",
	}
	directive := &runtimecontract.RuntimeCandidateCorrectionDirective{
		Disposition:  runtimecontract.RuntimeCandidateCorrectionCorrectableOnce,
		ManifestHash: "manifest-hash", Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField},
		Witness: witness,
	}
	failure := runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		Reason: "private parser reason", TargetRefs: []string{"main.go"},
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations", Required: []string{"operations"}, ManifestHash: "manifest-hash",
			Objectives: []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField},
		},
		ValidationDiagnostic: &runtimecontract.ValidationFailureDiagnostic{Class: "validation", Reason: "private validation reason"},
	}
	candidate, err := ProjectRuntimeCandidateCorrectionObservation(&failure, directive)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private parser reason", "private validation reason", "Reason", "Validation"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("candidate observation leaked %q: %s", forbidden, encoded)
		}
	}
	firstHash := runtimecontract.StableRuntimeCandidateRequestCorrectionHash(candidate, "", nil, nil)
	failure.Reason = "different audit-only reason"
	drifted, err := ProjectRuntimeCandidateCorrectionObservation(&failure, directive)
	if err != nil {
		t.Fatal(err)
	}
	if runtimecontract.StableRuntimeCandidateRequestCorrectionHash(drifted, "", nil, nil) != firstHash {
		t.Fatal("candidate correction identity drifted with owner-private reason")
	}

	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, OriginalClass: "schema", OriginalCode: "semantic_plan_invalid",
		OriginalMessage: "private provider document", Signature: "semantic-plan/invalid-document",
		DetailsRef: "artifact://diagnostic/1", DetailsHash: strings.Repeat("a", 64), DetailsBytes: 10,
		SemanticViolations: []runtimecontract.RuntimeSemanticContractViolation{{
			Kind: runtimecontract.RuntimeSemanticContractViolationCoverageOrderDocument, Path: "$.coverageOrder",
			RequiredCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		}},
	}
	semantic, err := ProjectRuntimeSemanticCorrectionObservation(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(semantic)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private provider document", "details", "originalMessage", "validation", "governance"} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(forbidden)) {
			t.Fatalf("semantic observation leaked %q: %s", forbidden, encoded)
		}
	}
	semanticHash := runtimecontract.StableRuntimeSemanticCorrectionObservationHash(*semantic)
	diagnostic.OriginalMessage = "different audit-only provider text"
	driftedSemantic, err := ProjectRuntimeSemanticCorrectionObservation(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if runtimecontract.StableRuntimeSemanticCorrectionObservationHash(*driftedSemantic) != semanticHash {
		t.Fatal("semantic correction identity drifted with OriginalMessage")
	}
}
