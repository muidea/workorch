package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func inputSemanticsReviewFixture(t *testing.T) (RuntimeValidationProofRequirementBinding, RuntimeValidationInputReview) {
	t.Helper()
	binding, report := diagnosticSourceReviewFixture(t)
	binding.InputReview.EvaluationScope = RuntimeValidationInputSemantics
	binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	report.SubjectHash = binding.InputReview.Hash
	return binding, report
}

func TestInputSemanticsCannotWaitForFutureOwnerResults(t *testing.T) {
	for _, source := range []bool{false, true} {
		for _, kind := range []string{"", RuntimeValidationExecutionObservationPending, RuntimeValidationAssessmentPending, RuntimeValidationTerminalDeliveryPending, "invented"} {
			t.Run(strings.Join([]string{map[bool]string{false: "check", true: "source"}[source], kind}, "/"), func(t *testing.T) {
				binding, report := inputSemanticsReviewFixture(t)
				if source {
					report.DiagnosticSources[0].Conclusion = RuntimeValidationInputInconclusive
					report.DiagnosticSources[0].UncertaintyKind = kind
				} else {
					report.Checks[0].Conclusion = RuntimeValidationInputInconclusive
					report.Checks[0].Obligations[0].Finding = RuntimeValidationInputUnknown
					report.Checks[0].UncertaintyKind = kind
				}
				before, _ := json.Marshal(report)
				for i := 0; i < 2; i++ {
					err := report.ValidateAgainstBinding(binding)
					var violation *runtimeValidationInputViolation
					if !errors.As(err, &violation) || !strings.HasSuffix(violation.Path, ".uncertaintyKind") {
						t.Fatalf("future owner result became accepted semantic uncertainty: %v", err)
					}
				}
				after, _ := json.Marshal(report)
				if string(before) != string(after) {
					t.Fatal("validator rewrote the rejected report")
				}
			})
		}
	}
}

func TestInputSemanticsPreservesRealUncertaintyConflictAndFrozenSources(t *testing.T) {
	binding, report := inputSemanticsReviewFixture(t)
	if err := report.ValidateAgainstBinding(binding); err != nil {
		t.Fatal(err)
	}
	for _, source := range []bool{false, true} {
		unknown := report.Clone()
		if source {
			unknown.DiagnosticSources[0].Conclusion = RuntimeValidationInputInconclusive
			unknown.DiagnosticSources[0].UncertaintyKind = RuntimeValidationSemanticInformationMissing
		} else {
			unknown.Checks[0].Conclusion = RuntimeValidationInputInconclusive
			unknown.Checks[0].Obligations[0].Finding = RuntimeValidationInputUnknown
			unknown.Checks[0].UncertaintyKind = RuntimeValidationSemanticInformationMissing
		}
		if err := unknown.ValidateAgainstBinding(binding); err != nil || RuntimeValidationInputReviewConclusion(unknown) != RuntimeValidationInputInconclusive {
			t.Fatalf("real semantic uncertainty was hidden: %v", err)
		}
		var loaded RuntimeValidationInputReview
		raw, _ := json.Marshal(unknown)
		if json.Unmarshal(raw, &loaded) != nil || !reflect.DeepEqual(loaded, unknown) || StableRuntimeValidationInputReviewHash(loaded) != StableRuntimeValidationInputReviewHash(unknown) {
			t.Fatal("reload changed report identity")
		}
		if err := loaded.ValidateAgainstBinding(binding); err != nil {
			t.Fatal(err)
		}
		conflict := report.Clone()
		conflict.DiagnosticSources[0].Conclusion = RuntimeValidationInputConflict
		if err := conflict.ValidateAgainstBinding(binding); err != nil || RuntimeValidationInputReviewConclusion(conflict) != RuntimeValidationInputConflict {
			t.Fatalf("known conflict was waived: %v", err)
		}
	}
	conclusive := report.Clone()
	conclusive.Checks[0].UncertaintyKind = RuntimeValidationSemanticInformationMissing
	if conclusive.ValidateAgainstBinding(binding) == nil {
		t.Fatal("conclusive report with hidden uncertainty accepted")
	}
	frozen, original := diagnosticSourceReviewFixture(t)
	before, _ := json.Marshal(frozen)
	if err := original.ValidateAgainstBinding(frozen); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(frozen)
	if string(before) != string(after) || frozen.InputReview.EvaluationScope != "" {
		t.Fatal("consumption injected new scope into frozen source")
	}
	if binding.InputReview.Hash == frozen.InputReview.Hash {
		t.Fatal("declared scope missing from subject identity")
	}
	tampered := binding.Clone()
	tampered.InputReview.EvaluationScope = "execution"
	if tampered.InputReview.ValidateAgainstBinding(tampered) == nil {
		t.Fatal("undeclared stage accepted")
	}
	tampered = binding.Clone()
	tampered.InputReview.EvaluationScope = ""
	if tampered.InputReview.ValidateAgainstBinding(tampered) == nil {
		t.Fatal("scope erasure retained subject identity")
	}
}

func TestInputReviewUncertaintyWireAndSchemaAgree(t *testing.T) {
	binding, report := inputSemanticsReviewFixture(t)
	for _, value := range []string{"null", `""`, "3", "{}"} {
		raw, _ := json.Marshal(report)
		var object map[string]json.RawMessage
		_ = json.Unmarshal(raw, &object)
		var checks []map[string]json.RawMessage
		_ = json.Unmarshal(object["checks"], &checks)
		checks[0]["uncertaintyKind"] = json.RawMessage(value)
		object["checks"], _ = json.Marshal(checks)
		raw, _ = json.Marshal(object)
		if validateRuntimeValidationInputReviewPayload("$.review", raw) == nil {
			t.Fatalf("invalid uncertainty wire accepted: %s", value)
		}
	}
	schema := map[string]any{"properties": map[string]any{}}
	addRuntimeInputReviewUncertaintySchema(schema, *binding.InputReview)
	b, _ := json.Marshal(schema)
	for _, required := range []string{"uncertaintyKind", RuntimeValidationSemanticInformationMissing, RuntimeValidationInputInconclusive, `"else"`} {
		if !strings.Contains(string(b), required) {
			t.Fatalf("schema missing stage rule: %s", required)
		}
	}
}
