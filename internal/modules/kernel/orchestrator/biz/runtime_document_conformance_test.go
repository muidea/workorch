package orchestrator

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeCompletedCandidateDocumentShapeUsesClosedClassification(t *testing.T) {
	tests := []struct {
		name     string
		document string
		shape    string
		rejected bool
	}{
		{name: "object", document: `{}`, rejected: false},
		{name: "array", document: `[]`, shape: runtimecontract.RuntimeCandidateDocumentShapeNonObjectJSONValue, rejected: true},
		{name: "null", document: `null`, shape: runtimecontract.RuntimeCandidateDocumentShapeNonObjectJSONValue, rejected: true},
		{name: "string", document: `"text"`, shape: runtimecontract.RuntimeCandidateDocumentShapeNonObjectJSONValue, rejected: true},
		{name: "number", document: `1`, shape: runtimecontract.RuntimeCandidateDocumentShapeNonObjectJSONValue, rejected: true},
		{name: "multiple", document: `{} {}`, shape: runtimecontract.RuntimeCandidateDocumentShapeMultipleJSONValues, rejected: true},
		{name: "trailing prose", document: `{} prose`, shape: runtimecontract.RuntimeCandidateDocumentShapeTrailingContent, rejected: true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			shape, rejected := runtimeCompletedCandidateDocumentShape([]byte(testCase.document))
			if rejected != testCase.rejected || shape != testCase.shape {
				t.Fatalf("shape=(%q,%t), want (%q,%t)", shape, rejected, testCase.shape, testCase.rejected)
			}
			if rejected {
				failure := runtimeCandidateDocumentConformanceFailure(shape)
				if failure.Diagnostic == nil || failure.Diagnostic.Actual != shape || failure.Reason == testCase.document {
					t.Fatalf("unbounded document failure: %+v", failure)
				}
			}
		})
	}
}

func TestRuntimeSemanticDocumentShapeDiagnosticIsBounded(t *testing.T) {
	diagnostic := runtimeSemanticDocumentShapeDiagnostic(runtimecontract.RuntimeCandidateDocumentShapeTrailingContent)
	if diagnostic.OriginalCode != "semantic_document_nonconformant" || len(diagnostic.FieldViolations) != 1 ||
		diagnostic.FieldViolations[0].Actual != runtimecontract.RuntimeCandidateDocumentShapeTrailingContent ||
		diagnostic.OriginalMessage == "{} provider prose" {
		t.Fatalf("semantic document diagnostic = %+v", diagnostic)
	}
}
