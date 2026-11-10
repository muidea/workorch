package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExecutionPlanBindsMethodDeadlineChecksAndFrozenSubject(t *testing.T) {
	_, _, _, b := semanticInputReviewFixture(t)
	s := b.InputReview
	s.Recipe = &RuntimeValidationRecipe{ID: b.ValidationRecipeID, Command: "owner-tool", TimeoutMillis: 300000}
	s.ExecutionPlan = &RuntimeValidationExecutionPlan{TimeoutMillis: 300000, Stages: []RuntimeValidationPlannedStage{{Identity: RuntimeValidationExecutionStageIdentity{ID: "ordinary", Scope: "whole", RequireTests: true}, Command: "owner-tool", Args: []string{"all"}, Instrumentation: []string{"independent_observation"}, RequiredCheckRefs: b.RequiredCheckRefs}}}
	for i := range b.CheckAttestations {
		b.CheckAttestations[i].ExecutionStagesHash = StableRuntimeValidationExecutionStagesHash([]RuntimeValidationExecutionStageIdentity{s.ExecutionPlan.Stages[0].Identity})
	}
	s.Hash = StableRuntimeValidationInputReviewSubjectHash(*s, b)
	if err := s.ValidateAgainstBinding(b); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var frozen RuntimeValidationProofRequirementBinding
	if err = json.Unmarshal(raw, &frozen); err != nil || !reflect.DeepEqual(frozen, b) {
		t.Fatal("frozen method changed", err)
	}
	for _, arm := range []string{"timeout", "unbound-deadline", "missing-check", "unknown-check", "duplicate-stage", "weaken-stage", "command", "instrumentation"} {
		t.Run(arm, func(t *testing.T) {
			v := b.Clone()
			p := v.InputReview.ExecutionPlan
			switch arm {
			case "timeout":
				p.TimeoutMillis++
			case "unbound-deadline":
				v.InputReview.Recipe = nil
			case "missing-check":
				p.Stages[0].RequiredCheckRefs = nil
			case "unknown-check":
				p.Stages[0].RequiredCheckRefs = []string{"unknown"}
			case "duplicate-stage":
				p.Stages = append(p.Stages, p.Stages[0])
			case "weaken-stage":
				p.Stages[0].Identity.RequireTests = false
			case "command":
				p.Stages[0].Args[0] = "different"
			case "instrumentation":
				p.Stages[0].Instrumentation[0] = "different"
			}
			if v.InputReview.ValidateAgainstBinding(v) == nil {
				t.Fatal("changed method retained original subject identity")
			}
			if arm != "command" && arm != "instrumentation" {
				v.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*v.InputReview, v)
				if v.InputReview.ValidateAgainstBinding(v) == nil {
					t.Fatal("self hash bypassed method/obligation invariant")
				}
			}
			if err := frozen.InputReview.ValidateAgainstBinding(frozen); err != nil || !reflect.DeepEqual(b, frozen) {
				t.Fatal("clone/new method mutated frozen input", err)
			}
		})
	}
}
