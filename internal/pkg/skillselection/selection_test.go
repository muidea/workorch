package skillselection

import (
	"fmt"
	"slices"
	"testing"
)

func TestExactSelectionPolicyBoundOrderAndDetachedReasons(t *testing.T) {
	candidates := make([]Candidate, 7)
	for i := range candidates {
		candidates[i] = Candidate{ID: fmt.Sprintf("s%d", i), Name: "go", Source: "workorch-registry", Scope: "global", TriggerRules: []string{"go"}}
	}
	candidates[6].Source, candidates[6].Scope, candidates[6].SourceSkillID = "skill-hub", "workspace", "local"
	for range 2 {
		v, err := AutoSelectTyped(candidates, "go", []string{"verify go"}, nil)
		if err != nil || len(v.Matches) != 5 || v.Matches[0].Candidate.ID != "s6" || v.Matches[1].Candidate.ID != "s0" || v.Matches[0].Score != 14 {
			t.Fatal("matching policy drift")
		}
		v.Matches[0].Candidate.TriggerRules[0] = "changed"
		if candidates[6].TriggerRules[0] != "go" {
			t.Fatal("alias")
		}
		slices.Reverse(candidates)
		// Restore the test's position-independent assertion below.
		v, err = AutoSelectTyped(candidates, "go", nil, []string{"local"})
		if err != nil || len(v.Matches) != 1 || v.Matches[0].Candidate.ID != "s6" {
			t.Fatal("source allowlist lost")
		}
		slices.Reverse(candidates)
	}
	bad := slices.Clone(candidates)
	bad[1].ID = bad[0].ID
	if v, err := AutoSelectTyped(bad, "unrelated", nil, []string{"no match"}); err == nil || len(v.Matches) != 0 {
		t.Fatal("filtered duplicate catalog credited")
	}
}
