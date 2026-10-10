package skillselectcore

import (
	"reflect"
	"testing"

	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestSelectionInputProjectionCopiesModels(t *testing.T) {
	goal := &workspacemodel.Goal{Text: "use repo summary"}
	dod := &workspacemodel.DoD{Items: []string{"tool completes"}}
	soul := &soulmodel.Soul{AllowedSkillRefs: []string{" skill-a ", "", "skill-b"}}
	skills := []workorchmanagementcontract.Skill{{
		ID:            "skill-a",
		Name:          "Repo Summary",
		TriggerRules:  []string{"repo summary"},
		SourceSkillID: "source-a",
		Scope:         "workspace",
		Source:        "skill-hub",
	}}

	if got := GoalText(goal); got != "use repo summary" {
		t.Fatalf("GoalText = %q", got)
	}
	dodItems := DoDItems(dod)
	dod.Items[0] = "mutated"
	if !reflect.DeepEqual(dodItems, []string{"tool completes"}) {
		t.Fatalf("DoDItems = %#v", dodItems)
	}
	if got := AllowedSkillRefs(soul); !reflect.DeepEqual(got, []string{"skill-a", "skill-b"}) {
		t.Fatalf("AllowedSkillRefs = %#v", got)
	}
	candidates := CandidatesFromSkills(skills)
	skills[0].TriggerRules[0] = "mutated"
	if got := candidates[0].TriggerRules; !reflect.DeepEqual(got, []string{"repo summary"}) {
		t.Fatalf("candidate trigger rules = %#v", got)
	}
}

func TestSkillsBySelectedCandidatesPreservesSelectionOrder(t *testing.T) {
	source := []workorchmanagementcontract.Skill{
		{ID: "skill-a", Name: "A"},
		{ID: "skill-b", Name: "B"},
		{ID: "skill-c", Name: "C"},
	}
	selected := []Candidate{{ID: "skill-c"}, {ID: "missing"}, {ID: "skill-a"}}

	got := SkillsBySelectedCandidates(source, selected)
	if gotIDs := []string{got[0].ID, got[1].ID}; !reflect.DeepEqual(gotIDs, []string{"skill-c", "skill-a"}) {
		t.Fatalf("selected skill ids = %#v", gotIDs)
	}
}

func TestSelectionInputProjectionNilInputs(t *testing.T) {
	if got := GoalText(nil); got != "" {
		t.Fatalf("nil GoalText = %q", got)
	}
	if got := DoDItems(nil); got != nil {
		t.Fatalf("nil DoDItems = %#v", got)
	}
	if got := AllowedSkillRefs(nil); got != nil {
		t.Fatalf("nil AllowedSkillRefs = %#v", got)
	}
	if got := SkillsBySelectedCandidates(nil, nil); got != nil {
		t.Fatalf("empty selected skills = %#v", got)
	}
}
