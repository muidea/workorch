package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func sourceObligationInventoryFixture() RuntimeSourceObligationInventory {
	return (RuntimeSourceObligationInventory{
		SchemaVersion: RuntimeSourceObligationInventorySchemaCurrent,
		CandidateHash: "candidate-hash", SpecificationHash: "specification-hash", DefinitionRevision: "definition-r1", StartupInputHash: "startup-hash",
		Sources: []RuntimeDefinitionSource{
			{CandidateItemRef: "goal", ItemRef: "goal-formal", SourceScope: "goal", SourceID: "goal", Text: "目标"},
			{CandidateItemRef: "dod", ItemRef: "dod-formal", SourceScope: "dodItem", SourceID: "dod-001", Text: "共同判据"},
		},
		Resources:    []RuntimeSourceResource{{ID: "input", Ref: "input-ref", SnapshotHash: "input-hash", Required: true, Available: true}},
		Requirements: []RuntimeSourceRequirement{{ID: "requirement", DimensionID: "dimension", Required: true, SourceKind: "USER_EXPLICIT", SourceItemRefs: []string{"goal", "dod"}, ResourceIDs: []string{"input"}, Text: "源要求"}},
		Obligations: []RuntimeSourceObligation{
			{ID: "A", RequirementID: "requirement", DoDItemRefs: []string{"dod"}, Text: " A 原判据\n "},
			{ID: "B", RequirementID: "requirement", DoDItemRefs: []string{"dod"}, Text: "B 原判据"},
		},
		Verifications: []RuntimeSourceVerification{
			{ID: "verify-A", ObligationID: "A", Declaration: "验证 A", ProfileID: "profile", EvidenceRequirementID: "dod-001", Scope: "dodItem"},
			{ID: "verify-B", ObligationID: "B", Declaration: "验证 B", ProfileID: "profile", EvidenceRequirementID: "dod-001", Scope: "dodItem"},
		},
	}).WithHash()
}

func TestRuntimeSourceObligationInventoryExactHashAndClone(t *testing.T) {
	original := sourceObligationInventoryFixture()
	if err := original.Validate(); err != nil {
		t.Fatal(err)
	}
	before := original.Clone()
	clone := original.Clone()
	clone.Requirements[0].SourceItemRefs[0] = "changed"
	clone.Requirements[0].ResourceIDs[0] = "changed"
	clone.Obligations[0].DoDItemRefs[0] = "changed"
	clone.Obligations[0].Text = "changed"
	if !reflect.DeepEqual(original, before) {
		t.Fatal("clone aliases original source graph")
	}
	permuted := original.Clone()
	permuted.Obligations[0], permuted.Obligations[1] = permuted.Obligations[1], permuted.Obligations[0]
	if permuted.Validate() == nil {
		t.Fatal("noncanonical persisted ordering accepted")
	}
	if permuted.WithHash().Hash != original.Hash {
		t.Fatal("set ordering changes canonical identity")
	}
	changed := original.Clone()
	changed.Obligations[0].Text = "A 原判据"
	if changed.WithHash().Hash == original.Hash {
		t.Fatal("criterion bytes omitted from hash")
	}
}

func TestRuntimeSourceObligationInventoryRejectsMissingDuplicateAndWrongSource(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*RuntimeSourceObligationInventory)
	}{
		{"missing B verification", func(i *RuntimeSourceObligationInventory) { i.Verifications = i.Verifications[:1] }},
		{"duplicate obligation", func(i *RuntimeSourceObligationInventory) { i.Obligations = append(i.Obligations, i.Obligations[0]) }},
		{"duplicate verification", func(i *RuntimeSourceObligationInventory) {
			i.Verifications = append(i.Verifications, i.Verifications[0])
		}},
		{"unknown requirement", func(i *RuntimeSourceObligationInventory) { i.Obligations[0].RequirementID = "unknown" }},
		{"goal as DoD", func(i *RuntimeSourceObligationInventory) { i.Obligations[0].DoDItemRefs = []string{"goal"} }},
		{"duplicate DoD ref", func(i *RuntimeSourceObligationInventory) { i.Obligations[0].DoDItemRefs = []string{"dod", "dod"} }},
		{"unmapped DoD", func(i *RuntimeSourceObligationInventory) {
			i.Sources = append(i.Sources, RuntimeDefinitionSource{CandidateItemRef: "dod-extra", ItemRef: "dod-extra-formal", SourceScope: "dodItem", SourceID: "dod-002", Text: "extra"})
		}},
		{"unmapped required requirement", func(i *RuntimeSourceObligationInventory) {
			r := i.Requirements[0]
			r.ID = "extra"
			i.Requirements = append(i.Requirements, r)
		}},
		{"missing required resource", func(i *RuntimeSourceObligationInventory) { i.Resources[0].Available = false }},
		{"duplicate formal source", func(i *RuntimeSourceObligationInventory) { i.Sources[0].ItemRef = i.Sources[1].ItemRef }},
		{"missing source seal", func(i *RuntimeSourceObligationInventory) { i.SpecificationHash = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inventory := sourceObligationInventoryFixture()
			tc.mutate(&inventory)
			inventory = inventory.WithHash()
			if err := inventory.Validate(); err == nil {
				t.Fatalf("invalid graph accepted: %+v", inventory)
			}
		})
	}
}
