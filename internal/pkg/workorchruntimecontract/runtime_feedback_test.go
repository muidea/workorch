package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func validRuntimeFeedback() RuntimeFeedback {
	return RuntimeFeedback{
		Domain:              "candidate_payload",
		StableCategory:      "schema_invalid",
		WorkspaceID:         common.WorkspaceID("ws-1"),
		RunID:               common.RunID("run-1"),
		AuthoritySnapshotID: "authority-1",
		DispatchID:          "dispatch-1",
		OriginalDiagnostic: &RuntimeOriginalDiagnostic{
			SchemaVersion:   RuntimeOriginalDiagnosticSchemaCurrent,
			OwnerModuleID:   "blocks/llm",
			OriginalClass:   "json_syntax",
			OriginalCode:    "invalid_character",
			OriginalMessage: "invalid character '}' after array element",
			Signature:       "candidate-json-1",
			OperationID:     "op-1",
			TargetRefs:      []string{"z.go", "a.go", "z.go"},
			FieldViolations: []RuntimeFieldViolation{{
				Path:      "operations[0].parameters.summary",
				Actual:    "unexpected",
				Allowed:   []string{"command", "args"},
				Required:  []string{"command", "targetRef"},
				Forbidden: []string{"summary", "payload"},
			}},
			DetailsRef:    "workorch://diagnostics/ws-1/run-1/1",
			DetailsHash:   "sha256:abc",
			DetailsBytes:  12,
			ContentType:   "application/json",
			Truncated:     true,
			OmittedReason: "bounded",
		},
	}
}

func TestRuntimeFeedbackJSONRoundTripPreservesOriginalDiagnostic(t *testing.T) {
	feedback := validRuntimeFeedback()
	encoded, err := json.Marshal(feedback)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded RuntimeFeedback
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, feedback) {
		t.Fatalf("JSON round-trip = %#v, want %#v", decoded, feedback)
	}
	if got, want := StableRuntimeFeedbackHash(decoded), StableRuntimeFeedbackHash(feedback); got != want {
		t.Fatalf("StableRuntimeFeedbackHash() = %q, want %q", got, want)
	}
}

func TestRuntimeFeedbackNormalizationDoesNotOverwriteOriginalOrOrder(t *testing.T) {
	feedback := validRuntimeFeedback()
	feedback.OriginalDiagnostic.OriginalMessage = " original message "
	feedback.OriginalDiagnostic.TargetRefs = []string{" z.go ", "a.go", " z.go "}
	feedback.OriginalDiagnostic.FieldViolations[0].Allowed = []string{" args ", "command", " args "}
	originalHash := StableRuntimeFeedbackHash(feedback)

	normalized := feedback.Normalized()
	if got, want := normalized.OriginalDiagnostic.TargetRefs, []string{"z.go", "a.go", "z.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized targets = %#v, want %#v", got, want)
	}
	if got, want := normalized.OriginalDiagnostic.FieldViolations[0].Allowed, []string{"args", "command", "args"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("normalized allowed = %#v, want %#v", got, want)
	}
	if got, want := feedback.OriginalDiagnostic.OriginalMessage, " original message "; got != want {
		t.Fatalf("original message = %q, want %q", got, want)
	}
	if got := StableRuntimeFeedbackHash(feedback); got != originalHash {
		t.Fatalf("normalization mutated original hash: got %q, want %q", got, originalHash)
	}
}

func TestRuntimeFeedbackCloneDoesNotShareSlices(t *testing.T) {
	feedback := validRuntimeFeedback()
	clone := feedback.CloneOriginal()
	clone.OriginalDiagnostic.TargetRefs[0] = "changed.go"
	clone.OriginalDiagnostic.FieldViolations[0].Allowed[0] = "changed"
	if feedback.OriginalDiagnostic.TargetRefs[0] == "changed.go" {
		t.Fatal("clone target refs share backing storage")
	}
	if feedback.OriginalDiagnostic.FieldViolations[0].Allowed[0] == "changed" {
		t.Fatal("clone field violation slices share backing storage")
	}
}

func TestOriginalDiagnosticClonePreservesRawNilEmptyAndNestedShape(t *testing.T) {
	for _, original := range []*RuntimeOriginalDiagnostic{
		{OriginalMessage: " original "},
		{TargetRefs: []string{}, FieldViolations: []RuntimeFieldViolation{}, SemanticViolations: []RuntimeSemanticContractViolation{}},
		{TargetRefs: []string{" b ", "a", " b "}, ValidationDiagnostic: &ValidationFailureDiagnostic{TargetRefs: []string{" b ", "a", " b "}}},
	} {
		cloned := original.CloneOriginal()
		if !reflect.DeepEqual(cloned, original) {
			t.Fatalf("diagnostic clone normalized raw facts: got=%#v want=%#v", cloned, original)
		}
		assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(original), reflect.ValueOf(cloned), "original diagnostic")
	}
}

func TestRuntimeFeedbackValidationEnforcesDiagnosticAndSafetyBoundary(t *testing.T) {
	feedback := validRuntimeFeedback()
	if err := feedback.Validate(); err != nil {
		t.Fatalf("valid feedback Validate() error = %v", err)
	}

	withoutDiagnostic := feedback.CloneOriginal()
	withoutDiagnostic.OriginalDiagnostic = nil
	if err := withoutDiagnostic.Validate(); err == nil {
		t.Fatal("missing diagnostic for rejection Validate() error = nil")
	}
	withoutDiagnostic.StableCategory = RuntimeFeedbackCategoryAccepted
	if err := withoutDiagnostic.Validate(); err != nil {
		t.Fatalf("accepted feedback without diagnostic Validate() error = %v", err)
	}

	for name, mutate := range map[string]func(*RuntimeOriginalDiagnostic){
		"credential": func(d *RuntimeOriginalDiagnostic) { d.OriginalMessage = "authorization: Bearer secret" },
		"endpoint":   func(d *RuntimeOriginalDiagnostic) { d.OriginalMessage = "provider https://example.invalid/v1 failed" },
		"raw_prompt": func(d *RuntimeOriginalDiagnostic) { d.OriginalMessage = "raw prompt: user private text" },
		"raw_body":   func(d *RuntimeOriginalDiagnostic) { d.FieldViolations[0].Actual = "-----BEGIN PRIVATE KEY-----" },
	} {
		t.Run(name, func(t *testing.T) {
			unsafe := feedback.CloneOriginal()
			mutate(unsafe.OriginalDiagnostic)
			if err := unsafe.Validate(); err == nil {
				t.Fatalf("unsafe %s Validate() error = nil", name)
			}
		})
	}
}

func TestStableRuntimeFeedbackHashIncludesOriginalOrder(t *testing.T) {
	left := validRuntimeFeedback()
	right := left.CloneOriginal()
	right.OriginalDiagnostic.TargetRefs[0], right.OriginalDiagnostic.TargetRefs[1] = right.OriginalDiagnostic.TargetRefs[1], right.OriginalDiagnostic.TargetRefs[0]
	if StableRuntimeFeedbackHash(left) == StableRuntimeFeedbackHash(right) {
		t.Fatal("feedback hash ignored ordered target refs")
	}
	if StableRuntimeFeedbackHash(left) != StableRuntimeFeedbackHash(left.CloneOriginal()) {
		t.Fatal("feedback hash is not clone stable")
	}
}

func TestStableRuntimeFeedbackHashIncludesExactValidationRepairRouteFields(t *testing.T) {
	left := validRuntimeFeedback()
	left.OriginalDiagnostic.ValidationDiagnostic = &ValidationFailureDiagnostic{
		SchemaVersion: ValidationFailureDiagnosticSchemaVersion,
		Class:         ValidationDiagnosticBuildFailed, OwnerCode: "shell_exit_nonzero",
		RecommendedEvidenceTargetType: ValidationEvidenceTargetFailureSurface,
	}
	right := left.CloneOriginal()
	right.OriginalDiagnostic.ValidationDiagnostic.OwnerCode = "different_owner_code"
	if StableRuntimeFeedbackHash(left) == StableRuntimeFeedbackHash(right) {
		t.Fatal("feedback hash ignored validation owner code used by exact repair routing")
	}
	right = left.CloneOriginal()
	right.OriginalDiagnostic.ValidationDiagnostic.RecommendedEvidenceTargetType = ValidationEvidenceTargetBehaviorEvidence
	if StableRuntimeFeedbackHash(left) == StableRuntimeFeedbackHash(right) {
		t.Fatal("feedback hash ignored validation evidence target type used by exact repair routing")
	}
}

func TestRuntimeFeedbackConversionsPreserveSafeSourceDetail(t *testing.T) {
	scope := RuntimeFeedbackScope{WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1", DispatchID: "dispatch-1"}
	candidate, err := RuntimeFeedbackFromCandidateFailure(scope, "candidate_payload", "schema_invalid", "blocks/llm", RuntimeCandidateFailure{
		Class: "schema", Code: "unknown_field", Signature: "candidate-1", Reason: "unknown field dependsOn", TargetRefs: []string{"z.go", "a.go"},
		Diagnostic: &RuntimeCandidateFailureDiagnostic{OperationID: "op-1", Path: "operations[0].dependsOn", Actual: "dependsOn", Allowed: []string{"args", "command"}, Required: []string{"command"}, Forbidden: []string{"dependsOn"}},
	})
	if err != nil {
		t.Fatalf("RuntimeFeedbackFromCandidateFailure() error = %v", err)
	}
	if got, want := candidate.OriginalDiagnostic.FieldViolations[0].Forbidden, []string{"dependsOn"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate forbidden = %#v, want %#v", got, want)
	}
	if got, want := candidate.OriginalDiagnostic.TargetRefs, []string{"z.go", "a.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate targets = %#v, want %#v", got, want)
	}

	owner, err := RuntimeFeedbackFromFailureFact(scope, "capability", "resource_absent", "blocks/file", FailureFact{Class: "execution_failure", Code: "target_missing", Signature: "owner-1", Message: "target does not exist", TargetRefs: []string{"new.go"}})
	if err != nil {
		t.Fatalf("RuntimeFeedbackFromFailureFact() error = %v", err)
	}
	if got, want := owner.OriginalDiagnostic.OriginalCode, "target_missing"; got != want {
		t.Fatalf("owner original code = %q, want %q", got, want)
	}

	validation, err := RuntimeFeedbackFromValidationDiagnostic(scope, "validation", "test_failed", "blocks/shell", "validation-1", ValidationFailureDiagnostic{Class: "validation_test_failed", Reason: "test failed", Operation: "shell/command", CommandKind: "go test", TargetRefs: []string{"main.go"}})
	if err != nil {
		t.Fatalf("RuntimeFeedbackFromValidationDiagnostic() error = %v", err)
	}
	if got, want := validation.OriginalDiagnostic.FieldViolations[0].Actual, "go test"; got != want {
		t.Fatalf("validation actual = %q, want %q", got, want)
	}
}
