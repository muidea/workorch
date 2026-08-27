package workorchruntimecontract

import (
	"strconv"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeCandidateCorrectionObservationIsClosedAndBounded(t *testing.T) {
	witness := RuntimeCandidateCorrectionWitness{
		Dimension: RuntimeCandidateCorrectionWitnessSchemaField, Path: "$.operations",
		RejectedValue: "field_absent", AlternativeValue: "field_present",
	}
	base := RuntimeCandidateCorrectionObservation{
		Class: RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
		TargetRefs: []string{"main.go"}, Diagnostic: &RuntimeCandidateFailureDiagnostic{
			Path: "$.operations", Actual: "missing", Required: []string{"operations"}, ManifestHash: "manifest-hash",
			Objectives: []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionSupplyRequiredField},
		},
		SealedCorrectionHash: StableRuntimeCandidateSealedCorrectionHash("manifest-hash", []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionSupplyRequiredField}, &witness),
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeCandidateCorrectionObservation){
		"secret": func(value *RuntimeCandidateCorrectionObservation) { value.Diagnostic.Actual = "Bearer secret" },
		"url": func(value *RuntimeCandidateCorrectionObservation) {
			value.Diagnostic.Path = "https://owner.invalid/raw"
		},
		"oversize": func(value *RuntimeCandidateCorrectionObservation) {
			value.Diagnostic.Actual = strings.Repeat("x", maxRuntimeCorrectionObservationTextBytes+1)
		},
		"unknown objective": func(value *RuntimeCandidateCorrectionObservation) {
			value.Diagnostic.Objectives = []RuntimeCandidateCorrectionObjective{"invent_retry"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := base.Clone()
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatal("tampered candidate correction observation was accepted")
			}
		})
	}
}

func TestRuntimeSemanticCorrectionObservationEnforcesNestedBoundsAndSafeText(t *testing.T) {
	t.Parallel()
	base := RuntimeSemanticCorrectionObservation{
		SchemaVersion: RuntimeSemanticCorrectionObservationSchemaCurrent,
		OwnerModuleID: RuntimeModuleLLM, Class: "contract", Code: "semantic_contract_invalid", Signature: "semantic-contract-invalid",
		FieldViolations: []RuntimeFieldViolation{{Path: "$.proposals[0].targetRefs", Actual: "other.go", Allowed: []string{"main.go"}}},
		SemanticViolations: []RuntimeSemanticContractViolation{{
			Kind: RuntimeSemanticContractViolationProposalConstraint, Path: "$.proposals[0].targetRefs",
			CoverageWorkItemID: "cwi-001", ProposalIndex: 0, Rule: RuntimeSemanticConstraintRelational,
			Actual: "other.go", Required: []string{"main.go"}, Allowed: []string{"main.go"},
		}},
		TargetObservation: &RuntimeFailureTargetObservation{
			OwnerModuleID: RuntimeModuleLLM, SubjectRef: "workspace_deliverable", TargetRole: "deliverable",
			ObservedRevision: "revision-1", TargetRefs: []string{"main.go"}, DiagnosticClass: "managed_tool",
			EvidenceRefs: []string{"workorch://evidence/1"}, ContentHash: strings.Repeat("a", 64),
		},
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	tooManyStrings := make([]string, maxRuntimeCorrectionObservationItems+1)
	for idx := range tooManyStrings {
		tooManyStrings[idx] = "value-" + strconv.Itoa(idx)
	}
	tooManyCoverage := make([]common.CoverageWorkItemID, maxRuntimeCorrectionObservationItems+1)
	for idx := range tooManyCoverage {
		tooManyCoverage[idx] = common.CoverageWorkItemID("cwi-bounded-" + strconv.Itoa(idx))
	}
	for name, mutate := range map[string]func(*RuntimeSemanticCorrectionObservation){
		"field allowed count":   func(value *RuntimeSemanticCorrectionObservation) { value.FieldViolations[0].Allowed = tooManyStrings },
		"field required count":  func(value *RuntimeSemanticCorrectionObservation) { value.FieldViolations[0].Required = tooManyStrings },
		"field forbidden count": func(value *RuntimeSemanticCorrectionObservation) { value.FieldViolations[0].Forbidden = tooManyStrings },
		"semantic required count": func(value *RuntimeSemanticCorrectionObservation) {
			value.SemanticViolations[0].Required = tooManyStrings
		},
		"semantic allowed count": func(value *RuntimeSemanticCorrectionObservation) {
			value.SemanticViolations[0].Allowed = tooManyStrings
		},
		"semantic coverage count": func(value *RuntimeSemanticCorrectionObservation) {
			value.SemanticViolations = []RuntimeSemanticContractViolation{{
				Kind: RuntimeSemanticContractViolationProofBatchDocument, Path: "$.proposals",
				RequiredCoverageWorkItemIDs: tooManyCoverage,
			}}
		},
		"semantic required-before url": func(value *RuntimeSemanticCorrectionObservation) {
			value.SemanticViolations = []RuntimeSemanticContractViolation{{
				Kind: RuntimeSemanticContractViolationDependencyOrder, Path: "$.coverageOrder",
				RequiredBeforeID: "https://owner.invalid/raw", RequiredAfterID: "cwi-002",
				ActualBeforePosition: 1, ActualAfterPosition: 0,
			}}
		},
		"semantic coverage secret": func(value *RuntimeSemanticCorrectionObservation) {
			value.SemanticViolations[0].CoverageWorkItemID = "Bearer secret"
		},
		"semantic required coverage oversize": func(value *RuntimeSemanticCorrectionObservation) {
			value.SemanticViolations = []RuntimeSemanticContractViolation{{
				Kind: RuntimeSemanticContractViolationProofBatchDocument, Path: "$.proposals",
				RequiredCoverageWorkItemIDs: []common.CoverageWorkItemID{common.CoverageWorkItemID(strings.Repeat("x", maxRuntimeCorrectionObservationTextBytes+1))},
			}}
		},
		"target refs count": func(value *RuntimeSemanticCorrectionObservation) { value.TargetObservation.TargetRefs = tooManyStrings },
		"target evidence count": func(value *RuntimeSemanticCorrectionObservation) {
			value.TargetObservation.EvidenceRefs = tooManyStrings
		},
		"target subject url": func(value *RuntimeSemanticCorrectionObservation) {
			value.TargetObservation.SubjectRef = "https://owner.invalid/raw"
		},
		"target evidence secret": func(value *RuntimeSemanticCorrectionObservation) {
			value.TargetObservation.EvidenceRefs = []string{"Bearer secret"}
		},
		"target revision oversize": func(value *RuntimeSemanticCorrectionObservation) {
			value.TargetObservation.ObservedRevision = strings.Repeat("x", maxRuntimeCorrectionObservationTextBytes+1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := base.Clone()
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatal("unsafe or unbounded nested correction observation was accepted")
			}
		})
	}
}

func TestRuntimeSemanticCorrectionObservationHashBindsOnlyTypedObservation(t *testing.T) {
	observation := RuntimeSemanticCorrectionObservation{
		SchemaVersion: RuntimeSemanticCorrectionObservationSchemaCurrent,
		OwnerModuleID: RuntimeModuleLLM, Class: "schema", Code: "semantic_document_nonconformant", Signature: "semantic-document-nonconformant",
		FieldViolations: []RuntimeFieldViolation{{Path: "$", Actual: "invalid", Required: []string{"conformant"}}},
		SemanticViolations: []RuntimeSemanticContractViolation{{
			Kind: RuntimeSemanticContractViolationCoverageOrderDocument, Path: "$.coverageOrder",
			RequiredCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		}},
	}
	if err := observation.Validate(); err != nil {
		t.Fatal(err)
	}
	baseHash := StableRuntimeSemanticCorrectionObservationHash(observation)
	if baseHash == "" {
		t.Fatal("semantic correction observation hash is empty")
	}
	changed := observation.Clone()
	changed.Code = "semantic_document_changed"
	if StableRuntimeSemanticCorrectionObservationHash(changed) == baseHash {
		t.Fatal("semantic observation hash ignored typed code")
	}
	for name, mutate := range map[string]func(*RuntimeSemanticCorrectionObservation){
		"secret": func(value *RuntimeSemanticCorrectionObservation) { value.FieldViolations[0].Actual = "Bearer secret" },
		"url":    func(value *RuntimeSemanticCorrectionObservation) { value.OperationID = "https://owner.invalid/raw" },
		"oversize": func(value *RuntimeSemanticCorrectionObservation) {
			value.ObservedRevision = strings.Repeat("x", maxRuntimeCorrectionObservationTextBytes+1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := observation.Clone()
			mutate(&changed)
			if err := changed.Validate(); err == nil || StableRuntimeSemanticCorrectionObservationHash(changed) != "" {
				t.Fatalf("tampered semantic correction observation remained hashable: %v", err)
			}
		})
	}
}
