package events

import (
	"testing"

	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
)

func TestConcreteSkillResultsCloneDetachedOwnerState(t *testing.T) {
	item := &skillmodel.Skill{TriggerRules: []string{"owner"}}
	created := (CreateSkillResult{Skill: item}).Clone()
	got := (GetSkillResult{Skill: item}).Clone()
	listed := (ListSkillsResult{Skills: []*skillmodel.Skill{item}, NextCursor: "skill-1"}).Clone()
	item.TriggerRules[0] = "mutated"
	if created.Skill.TriggerRules[0] != "owner" || got.Skill.TriggerRules[0] != "owner" || listed.Skills[0].TriggerRules[0] != "owner" || listed.NextCursor != "skill-1" {
		t.Fatal("concrete Skill result clone shares owner state")
	}
}

func TestSkillContractInventoryIncludesBoundedPreparationObservation(t *testing.T) {
	contracts := Contracts()
	if len(contracts) != 4 || len(MessageContracts()) != 4 {
		t.Fatalf("Skill contracts = %d/%d, want four concrete topics", len(contracts), len(MessageContracts()))
	}
	want := map[string]bool{TopicSkillCommandCreate: true, TopicSkillCommandGet: true, TopicSkillCommandList: true, TopicObserveRegistryCatalog: true}
	for _, contract := range contracts {
		if !want[contract.Topic] {
			t.Fatalf("unexpected Skill topic %q", contract.Topic)
		}
		delete(want, contract.Topic)
	}
	if len(want) != 0 {
		t.Fatalf("missing Skill topics: %+v", want)
	}
}
