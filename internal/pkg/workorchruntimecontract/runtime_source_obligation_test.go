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

func TestRuntimeProjectDesignHashesExactChecksAndRejectsIncompleteOrChangedBasis(t *testing.T) {
	inventory := sourceObligationInventoryFixture()
	original := inventory.Clone()
	inventory.Verifications[0].ProjectDesigns = []RuntimeSourceProjectDesign{{CandidateItemRef: inventory.Obligations[0].DoDItemRefs[0], Decision: " Fixed project rule\r\n", VerificationBasis: " Independent assertion\t "}}
	inventory = inventory.WithHash()
	if inventory.Validate() != nil || inventory.Hash == original.Hash {
		t.Fatal("design not bound to exact inventory")
	}
	item := RuntimeCoverageItem{ID: "cwi", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "共同判据"}
	checks, err := CompileRuntimeSourceObligationChecks(inventory, item)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 2 || !reflect.DeepEqual(checks[0].ProjectDesigns, inventory.Verifications[0].ProjectDesigns) {
		t.Fatal("per-obligation input review lost design")
	}
	for _, bad := range []string{"", " ", "bad\x00", string([]byte{0xff})} {
		changed := inventory.Clone()
		changed.Verifications[0].ProjectDesigns[0].VerificationBasis = bad
		changed = changed.WithHash()
		if changed.Validate() == nil {
			t.Fatal("incomplete/invalid design accepted")
		}
	}
	changed := CloneRuntimeSourceObligationChecks(checks)[0]
	changed.ProjectDesigns[0].VerificationBasis += "changed"
	if changed.CheckRef == StableRuntimeSourceObligationCheckRef(inventory.DefinitionRevision, item.ID, changed) || ValidateRuntimeSourceObligationChecks(inventory.DefinitionRevision, item.ID, []RuntimeSourceObligationCheck{changed}) == nil {
		t.Fatal("changed design retained original check identity")
	}
}

func TestRuntimeSourceProjectDesignsRetainEverySourceAndDetachClones(t *testing.T) {
	inventory := sourceObligationInventoryFixture()
	source := inventory.Sources[1]
	for _, v := range inventory.Sources {
		if v.SourceScope == "dodItem" {
			source = v
			break
		}
	}
	source.CandidateItemRef = "second-source"
	source.ItemRef = "second-formal"
	source.SourceID = "dod-002"
	inventory.Sources = append(inventory.Sources, source)
	inventory.Requirements[0].SourceItemRefs = append(inventory.Requirements[0].SourceItemRefs, source.CandidateItemRef)
	for n := range inventory.Obligations {
		inventory.Obligations[n].DoDItemRefs = append(inventory.Obligations[n].DoDItemRefs, source.CandidateItemRef)
	}
	first := inventory.Obligations[0].DoDItemRefs[0]
	inventory.Verifications[0].ProjectDesigns = []RuntimeSourceProjectDesign{{CandidateItemRef: first, Decision: " first rule\r\n", VerificationBasis: "first assertion"}, {CandidateItemRef: source.CandidateItemRef, Decision: "second rule", VerificationBasis: " second assertion\t "}}
	inventory = inventory.WithHash()
	if err := inventory.Validate(); err != nil {
		t.Fatal(err)
	}
	checks, err := CompileRuntimeSourceObligationChecks(inventory, RuntimeCoverageItem{ID: "cwi", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "共同判据"})
	if err != nil || len(checks[0].ProjectDesigns) != 2 || !reflect.DeepEqual(checks[0].ProjectDesigns, inventory.Verifications[0].ProjectDesigns) {
		t.Fatal("source designs lost", err)
	}
	detached := CloneRuntimeSourceObligationChecks(checks)
	detached[0].ProjectDesigns[0].Decision = "changed"
	if checks[0].ProjectDesigns[0].Decision == "changed" || inventory.Verifications[0].ProjectDesigns[0].Decision == "changed" {
		t.Fatal("design clone aliases sealed input")
	}
	for _, ref := range []string{"foreign", first} {
		bad := inventory.Clone()
		if ref == first {
			bad.Verifications[0].ProjectDesigns = append(bad.Verifications[0].ProjectDesigns, bad.Verifications[0].ProjectDesigns[0])
		} else {
			bad.Verifications[0].ProjectDesigns[0].CandidateItemRef = ref
		}
		bad = bad.WithHash()
		if bad.Validate() == nil {
			t.Fatal("foreign/duplicate design source admitted")
		}
	}
}
