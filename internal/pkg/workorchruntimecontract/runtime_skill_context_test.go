package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func TestSealRuntimeSkillContextsBoundsUTF8InstructionsAndRecordsBudget(t *testing.T) {
	values := make([]RuntimeSkillContext, 0, RuntimeSkillContextMaxSkills)
	for idx := 0; idx < RuntimeSkillContextMaxSkills; idx++ {
		id := "skill-" + string(rune('a'+idx))
		instruction := strings.Repeat("界", RuntimeSkillInstructionMaxBytesPerSkill)
		values = append(values, RuntimeSkillContext{
			ID: id, Instruction: instruction,
			Provenance: workorchskillcontract.Provenance{
				Source: workorchskillcontract.SourceSkillHub, Scope: workorchskillcontract.ScopeGlobal,
				ContentHash: workorchskillcontract.StableContentHash(workorchskillcontract.Content{ID: id, Instruction: instruction}),
			},
		})
	}
	sealed, budget, err := SealRuntimeSkillContexts(values)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if budget == nil || budget.IncludedInstructionBytes > RuntimeSkillInstructionMaxTotalBytes || budget.TruncatedSkills != len(values) {
		t.Fatalf("budget = %+v", budget)
	}
	for _, item := range sealed {
		if !item.InstructionTruncated || !strings.HasPrefix(values[0].Instruction, item.Instruction) || len(item.Instruction) > RuntimeSkillInstructionMaxBytesPerSkill {
			t.Fatalf("sealed item = %+v", item)
		}
	}
	if err := ValidateRuntimeSkillContexts(sealed, budget); err != nil {
		t.Fatalf("validate sealed: %v", err)
	}
	drifted := *budget
	drifted.IncludedInstructionBytes++
	if err := ValidateRuntimeSkillContexts(sealed, &drifted); err == nil {
		t.Fatal("drifted Skill budget unexpectedly valid")
	}
}

func TestSealRuntimeSkillContextsRejectsMissingProvenanceAndExcessCount(t *testing.T) {
	if _, _, err := SealRuntimeSkillContexts([]RuntimeSkillContext{{ID: "skill-1", Instruction: "instruction"}}); err == nil {
		t.Fatal("missing provenance unexpectedly accepted")
	}
	values := make([]RuntimeSkillContext, 0, RuntimeSkillContextMaxSkills+1)
	for idx := 0; idx <= RuntimeSkillContextMaxSkills; idx++ {
		id := "skill-count-" + string(rune('a'+idx))
		instruction := "instruction"
		values = append(values, RuntimeSkillContext{ID: id, Instruction: instruction, Provenance: workorchskillcontract.Provenance{
			Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal,
			ContentHash: workorchskillcontract.StableContentHash(workorchskillcontract.Content{ID: id, Instruction: instruction}),
		}})
	}
	if _, _, err := SealRuntimeSkillContexts(values); err == nil {
		t.Fatal("excess Skill count unexpectedly accepted")
	}
}

func TestSealRuntimeSkillContextsRejectsOversizedMetadata(t *testing.T) {
	id := "skill-large-metadata"
	instruction := "instruction"
	description := strings.Repeat("x", RuntimeSkillContextMetadataMaxTotalBytes+1)
	content := workorchskillcontract.Content{ID: id, Instruction: instruction, Description: description}
	_, _, err := SealRuntimeSkillContexts([]RuntimeSkillContext{{
		ID: id, Instruction: instruction, Description: description,
		Provenance: workorchskillcontract.Provenance{
			Source: workorchskillcontract.SourceSkillHub, Scope: workorchskillcontract.ScopeGlobal,
			ContentHash: workorchskillcontract.StableContentHash(content),
		},
	}})
	if err == nil {
		t.Fatal("oversized Skill metadata unexpectedly accepted")
	}
}

func TestSealRuntimeSkillContextsRejectsProvenanceContentDrift(t *testing.T) {
	id := "skill-content-drift"
	instruction := "sealed instruction"
	_, _, err := SealRuntimeSkillContexts([]RuntimeSkillContext{{
		ID: id, Instruction: instruction,
		Provenance: workorchskillcontract.Provenance{
			Source: workorchskillcontract.SourceSkillHub, Scope: workorchskillcontract.ScopeGlobal,
			ContentHash: workorchskillcontract.StableContentHash(workorchskillcontract.Content{ID: id, Instruction: "different instruction"}),
		},
	}})
	if err == nil {
		t.Fatal("Skill provenance content drift unexpectedly accepted")
	}
}
