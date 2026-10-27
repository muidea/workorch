package workspace

import (
	"reflect"
	"testing"
)

func TestPreparationAllOfAssessmentUsesExactRequirements(t *testing.T) {
	requirements := []PreparationCheckRequirement{{"a", "one", true}, {"b", "two", true}, {"c", "three", false}}
	checks := []WorkspacePreparationCheckResult{{ID: "a", Owner: "one", Required: true, State: "passed", Reason: "observed"}, {ID: "b", Owner: "two", Required: true, State: "passed", Reason: "observed"}, {ID: "c", Owner: "three", State: "failed", Reason: "optional_failure"}}
	s, err := AssessPreparationChecks("manifest", requirements, checks)
	if err != nil || s.Outcome != "checks_passed" || s.RequiredCount != 2 || s.PassedRequiredCount != 2 || len(s.Outstanding) != 0 || s.ValidateFor("manifest", checks) != nil {
		t.Fatal("pure all-of positive", s, err)
	}
	// This is an aggregation algorithm test, not owner evidence or an M3.
	for _, state := range []WorkspacePreparationCheckState{"blocked", "failed", "pending"} {
		copy := append([]WorkspacePreparationCheckResult(nil), checks...)
		copy[1].State = state
		got, err := AssessPreparationChecks("manifest", requirements, copy)
		if err != nil || got.Outcome != "blocked" || got.PassedRequiredCount != 1 || !reflect.DeepEqual(got.Outstanding, []PreparationOutstandingCheck{{"b", "two", string(state), "observed"}}) {
			t.Fatal("non-PASS required was lost", got, err)
		}
		bad := got.Clone()
		bad.Outstanding[0].Reason = "changed"
		if bad.ValidateFor("manifest", copy) == nil || got.Outstanding[0].Reason != "observed" {
			t.Fatal("forged aggregate accepted or clone aliases")
		}
	}
	for _, mutate := range []func([]WorkspacePreparationCheckResult) []WorkspacePreparationCheckResult{
		func(c []WorkspacePreparationCheckResult) []WorkspacePreparationCheckResult { return c[:2] },
		func(c []WorkspacePreparationCheckResult) []WorkspacePreparationCheckResult {
			c[0].Required = false
			return c
		},
		func(c []WorkspacePreparationCheckResult) []WorkspacePreparationCheckResult {
			c[0].Owner = "caller"
			return c
		},
		func(c []WorkspacePreparationCheckResult) []WorkspacePreparationCheckResult {
			c[1].ID = c[0].ID
			return c
		},
		func(c []WorkspacePreparationCheckResult) []WorkspacePreparationCheckResult {
			c[1].State = "ready"
			return c
		},
	} {
		if _, err := AssessPreparationChecks("manifest", requirements, mutate(append([]WorkspacePreparationCheckResult(nil), checks...))); err == nil {
			t.Fatal("incomplete/substituted checklist accepted")
		}
	}
}
