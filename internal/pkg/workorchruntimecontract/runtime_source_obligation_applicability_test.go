package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func sourceObligationProfileFixture() []RuntimeProfileContract {
	return []RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{
		ID: "dod-001", Scope: "dodItem", RequiredEvidenceKinds: []string{EvidenceKindValidation},
		AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "source-recipe",
	}}}}
}

func TestRuntimeSourceObligationApplicabilityRejectsSelfConsistentWrongDeclaration(t *testing.T) {
	inventory := sourceObligationInventoryFixture()
	profiles := sourceObligationProfileFixture()
	before, _ := json.Marshal(inventory)
	if err := ValidateRuntimeSourceObligationApplicability(&inventory, profiles); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*RuntimeSourceObligationInventory, *[]RuntimeProfileContract)
	}{
		{"wrong Profile", func(i *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			i.Verifications[1].ProfileID = "other"
		}},
		{"wrong evidence requirement", func(i *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			i.Verifications[1].EvidenceRequirementID = "dod-002"
		}},
		{"wrong scope", func(i *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			i.Verifications[1].Scope = "goalCoverage"
		}},
		{"missing Profile", func(i *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) { *p = nil }},
		{"missing declaration", func(i *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].EvidenceRequirements = nil
		}},
		{"duplicate declaration", func(i *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].EvidenceRequirements = append((*p)[0].EvidenceRequirements, (*p)[0].EvidenceRequirements[0])
		}},
		{"no proof operation", func(i *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].EvidenceRequirements[0].AcceptedStepKinds = []string{StepKindFileRead}
		}},
		{"lost required evidence", func(i *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			(*p)[0].EvidenceRequirements[0].RequiredEvidenceKinds = nil
		}},
		{"second DoD outside declaration", func(i *RuntimeSourceObligationInventory, p *[]RuntimeProfileContract) {
			i.Sources = append(i.Sources, RuntimeDefinitionSource{CandidateItemRef: "dod-extra", ItemRef: "dod-formal-extra", SourceScope: "dodItem", SourceID: "dod-002", Text: "第二判据"})
			i.Requirements[0].SourceItemRefs = append(i.Requirements[0].SourceItemRefs, "dod-extra")
			i.Obligations[1].DoDItemRefs = append(i.Obligations[1].DoDItemRefs, "dod-extra")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := inventory.Clone()
			policy := sourceObligationProfileFixture()
			test.change(&changed, &policy)
			changed = changed.WithHash()
			if err := changed.Validate(); err != nil {
				t.Fatalf("反例必须保持 inventory 自洽：%v", err)
			}
			if err := ValidateRuntimeSourceObligationApplicability(&changed, policy); err == nil {
				t.Fatal("自洽 hash 掩盖不适用的源声明")
			}
			body, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded RuntimeSourceObligationInventory
			if err := json.Unmarshal(body, &reloaded); err != nil {
				t.Fatal(err)
			}
			if err := ValidateRuntimeSourceObligationApplicability(&reloaded, policy); err == nil {
				t.Fatal("fresh-load 丢失声明适用性反证")
			}
		})
	}
	after, _ := json.Marshal(inventory)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("适用性校验修改原库存")
	}
	if err := ValidateRuntimeSourceObligationApplicability(&inventory, profiles); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeSourceObligationApplicabilitySupportsConjunctiveWildcard(t *testing.T) {
	inventory := sourceObligationInventoryFixture()
	profiles := sourceObligationProfileFixture()
	profiles[0].EvidenceRequirements[0].ID = "*"
	profiles[0].EvidenceRequirements[0].ValidationRecipeID = ""
	for index := range inventory.Verifications {
		inventory.Verifications[index].EvidenceRequirementID = "*"
	}
	inventory = inventory.WithHash()
	if err := ValidateRuntimeSourceObligationApplicability(&inventory, profiles); err != nil {
		t.Fatal(err)
	}
	profiles[0].EvidenceRequirements[0].ValidationRecipeID = "wildcard-must-not-select-recipe"
	if err := ValidateRuntimeSourceObligationApplicability(&inventory, profiles); err == nil {
		t.Fatal("wildcard 假造 exact recipe")
	}
}
