package events

import (
	"testing"

	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
)

func TestSkillCommandResultCloneDetachesOwnerState(t *testing.T) {
	item := &skillmodel.Skill{TriggerRules: []string{"owner"}}
	cloned := (SkillCommandResult{Skill: item}).Clone()
	item.TriggerRules[0] = "mutated"
	if cloned.Skill.TriggerRules[0] != "owner" {
		t.Fatal("cloned skill shares trigger rules")
	}
}
