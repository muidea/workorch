package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestDiagnosticRelationPreservesFailureAndCompleteSourceUnion(t *testing.T) {
	b, _ := diagnosticSourceReviewFixture(t)
	o := diagnosticExecutionFixture()
	o.RecipeID = b.ValidationRecipeID
	c := o.ExecutionConjunction
	c.Stages[1].DiagnosticSourceRef = b.InputReview.DiagnosticSources[0].SourceRef
	c.Stages[0].FailedSourceRefs = []string{c.Stages[1].DiagnosticSourceRef}
	for i := range c.Stages {
		c.Stages[i].Stdout.Storage = &RuntimeValidationOutputStorage{Reference: "stdout"}
		c.Stages[i].Stderr.Storage = &RuntimeValidationOutputStorage{Reference: "stderr"}
	}
	for i := range b.CheckAttestations {
		b.CheckAttestations[i].ObservationProtocolRef = o.ObservationProtocolRef
		b.CheckAttestations[i].ExecutionStagesHash = c.RequiredStagesHash
	}
	b.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*b.InputReview, b)
	failure := &RuntimeValidationFailureObservation{CandidateTargetRefs: []string{"implementation"}}
	for _, relation := range []RuntimeValidationDiagnosticReproduction{ValidationReproductionExact, ValidationReproductionChanged, ValidationReproductionAbsent} {
		t.Run(string(relation), func(t *testing.T) {
			v := o.Clone()
			s := &v.ExecutionConjunction.Stages[1]
			s.DiagnosticReproduction = relation
			if relation == ValidationReproductionAbsent {
				s.ExitCode, s.KnownFailure, s.PassedTests = 0, false, s.Tests
			}
			if err := ValidateRuntimeValidationDiagnosticFailureObservation(&v, b, []string{"implementation"}, failure, nil); err != nil {
				t.Fatal(err)
			}
			if v.ExecutionConjunction.Complete || !reflect.DeepEqual(v.ExecutionConjunction.Stages[0], c.Stages[0]) {
				t.Fatal("independent outcome replaced original")
			}
			hash := StableRuntimeOperationReceiptHash(RuntimeOperationReceipt{Evidence: RuntimeOperationEvidence{Validation: &v}})
			for _, arm := range []string{"outcome", "relation", "origin", "missing-failed-source", "partial-union", "no-behavior", "raw", "source", "credit"} {
				t.Run(arm, func(t *testing.T) {
					bad := v.Clone()
					stage := &bad.ExecutionConjunction.Stages[1]
					switch arm {
					case "outcome":
						stage.ExitCode = 3
						stage.KnownFailure = false
					case "relation":
						stage.DiagnosticReproduction = "guessed"
					case "origin":
						stage.DiagnosticOriginStageID = "another"
					case "missing-failed-source":
						bad.ExecutionConjunction.Stages[0].FailedSourceRefs = nil
					case "partial-union":
						bad.ExecutionConjunction.Stages[0].FailedSourceRefs = append(bad.ExecutionConjunction.Stages[0].FailedSourceRefs, b.InputReview.DiagnosticSources[1].SourceRef)
					case "no-behavior":
						stage.BehaviorTargetRefs = nil
					case "raw":
						stage.Stdout.Storage = nil
					case "source":
						stage.SourceAfter = StableRuntimeValidationExecutionStagesHash(nil)
					case "credit":
						bad.ProofClass = RuntimeValidationProofBehavioral
					}
					if bad.ValidateFailureDetails() == nil && ValidateRuntimeValidationDiagnosticFailureObservation(&bad, b, []string{"implementation"}, failure, nil) == nil {
						t.Fatal("incomplete repair facts accepted")
					}
					if StableRuntimeOperationReceiptHash(RuntimeOperationReceipt{Evidence: RuntimeOperationEvidence{Validation: &bad}}) == hash {
						t.Fatal("mutation not bound to receipt")
					}
				})
			}
		})
	}
}
