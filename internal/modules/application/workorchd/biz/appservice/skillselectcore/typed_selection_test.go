package skillselectcore

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/skillselection"
	"reflect"
	"slices"
	"testing"
)

func TestTypedSelectionMatchesExistingPolicyAndDetachesReasons(t *testing.T) {
	candidates := []skillselection.Candidate{
		{ID: "registry", Name: "go", Scope: "global", Source: "workorch-registry", TriggerRules: []string{"go"}, SourceSkillID: "registry"},
		{ID: "skillhub:global:s", Name: "go", Scope: "global", Source: "skill-hub", TriggerRules: []string{"go"}, SourceSkillID: "s"},
		{ID: "skillhub:workspace:ws:s", Name: "go", Scope: "workspace", Source: "skill-hub", TriggerRules: []string{"go"}, SourceSkillID: "s"},
		{ID: "not-matched", Name: "unrelated", Scope: "global", Source: "workorch-registry"},
	}
	for _, allowed := range [][]string{nil, {"s"}, {"skillhub:global:s"}, {"missing"}} {
		legacy := AutoSelect(candidates, "write go", []string{"verify go"}, allowed, "workspace", DefaultMaxAutoSelectedSkills)
		typed, err := skillselection.AutoSelectTyped(candidates, "write go", []string{"verify go"}, allowed)
		if err != nil || typed.Policy != legacy.Evidence.Policy || len(typed.Matches) != len(legacy.Candidates) {
			t.Fatal("typed and legacy selection disagree")
		}
		for i, match := range typed.Matches {
			score, reasons := ScoreCandidate(match.Candidate, SearchText("write go", []string{"verify go"}))
			if !reflect.DeepEqual(match.Candidate, legacy.Candidates[i]) || score != match.Score || !reflect.DeepEqual(reasons, match.Reasons) {
				t.Fatal("policy or source identity changed")
			}
		}
	}
	value, _ := skillselection.AutoSelectTyped(candidates, "go", nil, nil)
	if value.Matches[0].Candidate.Scope != "workspace" {
		t.Fatal("workspace tie priority lost")
	}
	value.Matches[0].Candidate.TriggerRules[0] = "caller"
	if candidates[2].TriggerRules[0] != "go" {
		t.Fatal("selection aliases source")
	}
	for range 10 {
		slices.Reverse(candidates)
		current, err := skillselection.AutoSelectTyped(candidates, "go", nil, nil)
		if err != nil || current.Matches[0].Candidate.Scope != "workspace" || current.Matches[1].Candidate.ID != "registry" {
			t.Fatal("selection depends on inventory order")
		}
	}
}

func TestTypedSelectionBoundsAndInvalidCatalogNeverBecomeNoMatch(t *testing.T) {
	candidates := make([]skillselection.Candidate, 7)
	for i := range candidates {
		candidates[i] = skillselection.Candidate{ID: fmt.Sprintf("s%d", i), Name: "go", Scope: "global", Source: "workorch-registry"}
	}
	value, err := skillselection.AutoSelectTyped(candidates, "go", nil, nil)
	if err != nil || len(value.Matches) != DefaultMaxAutoSelectedSkills {
		t.Fatal("selection count policy changed")
	}
	for _, mutate := range []func(*skillselection.Candidate){
		func(c *skillselection.Candidate) { c.ID = "" }, func(c *skillselection.Candidate) { c.ID = " s" }, func(c *skillselection.Candidate) { c.ID = "s1" },
		func(c *skillselection.Candidate) { c.Scope = "project" }, func(c *skillselection.Candidate) { c.Source = "unknown" },
		func(c *skillselection.Candidate) { c.Name = "\xff" }, func(c *skillselection.Candidate) { c.TriggerRules = []string{" "} },
	} {
		items := slices.Clone(candidates)
		mutate(&items[0])
		if value, err := skillselection.AutoSelectTyped(items, "unrelated", nil, nil); err == nil || len(value.Matches) != 0 {
			t.Fatal("bad full catalog silently became no-match")
		}
	}
	if _, err := skillselection.AutoSelectTyped(make([]skillselection.Candidate, 3073), "go", nil, nil); err == nil {
		t.Fatal("catalog bound not enforced")
	}
	for _, allowed := range [][]string{{""}, {"\xff"}} {
		if _, err := skillselection.AutoSelectTyped(candidates, "go", nil, allowed); err == nil {
			t.Fatal("invalid allowlist silently widened")
		}
	}
	if _, err := skillselection.AutoSelectTyped(candidates, "go", []string{"\xff"}, nil); err == nil {
		t.Fatal("invalid original DoD accepted")
	}
	if value, err := skillselection.AutoSelectTyped(nil, "go", nil, nil); err != nil || len(value.Matches) != 0 {
		t.Fatal("actual empty catalog rejected")
	}
}
