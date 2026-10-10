package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestRuntimeSourceObligationChecksRetainSharedCoverageIndependentIdentity(t *testing.T) {
	inventory := sourceObligationInventoryFixture()
	item := RuntimeCoverageItem{ID: "cwi-shared", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "共同判据"}
	checks, err := CompileRuntimeSourceObligationChecks(inventory, item)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 2 || checks[0].ObligationID != "A" || checks[1].ObligationID != "B" || checks[0].CheckRef == checks[1].CheckRef || checks[0].Criterion != inventory.Obligations[0].Text {
		t.Fatalf("shared coverage lost separate source checks: %+v", checks)
	}
	if err := ValidateRuntimeSourceObligationChecks(inventory.DefinitionRevision, item.ID, checks); err != nil {
		t.Fatal(err)
	}
	clone := CloneRuntimeSourceObligationChecks(checks)
	clone[0].Criterion = "changed"
	if reflect.DeepEqual(clone, checks) || checks[0].Criterion != inventory.Obligations[0].Text {
		t.Fatal("check clone aliases source checks")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*RuntimeSourceObligationCheck)
	}{
		{"wrong obligation", func(c *RuntimeSourceObligationCheck) { c.ObligationID = "B" }},
		{"wrong requirement", func(c *RuntimeSourceObligationCheck) { c.RequirementID = "other" }},
		{"wrong verification", func(c *RuntimeSourceObligationCheck) { c.VerificationID = "other" }},
		{"criterion bytes", func(c *RuntimeSourceObligationCheck) { c.Criterion = "A 原判据" }},
		{"inventory seal", func(c *RuntimeSourceObligationCheck) { c.InventoryHash = "wrong" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			altered := CloneRuntimeSourceObligationChecks(checks)
			tc.mutate(&altered[0])
			if ValidateRuntimeSourceObligationChecks(inventory.DefinitionRevision, item.ID, altered) == nil {
				t.Fatal("check drift accepted")
			}
		})
	}
	if ValidateRuntimeSourceObligationChecks("other-definition", item.ID, checks) == nil || ValidateRuntimeSourceObligationChecks(inventory.DefinitionRevision, "other-cwi", checks) == nil {
		t.Fatal("cross-definition or cross-CWI check accepted")
	}
	duplicate := append(CloneRuntimeSourceObligationChecks(checks), checks[0])
	if ValidateRuntimeSourceObligationChecks(inventory.DefinitionRevision, item.ID, duplicate) == nil {
		t.Fatal("duplicate verification accepted")
	}
}

func TestRuntimeSourceObligationChecksRejectCoverageSourceDrift(t *testing.T) {
	inventory := sourceObligationInventoryFixture()
	item := RuntimeCoverageItem{ID: "cwi-shared", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "共同判据"}
	for _, tc := range []struct {
		name   string
		mutate func(*RuntimeCoverageItem)
	}{
		{"unknown source", func(i *RuntimeCoverageItem) { i.SourceID = "dod-002" }},
		{"changed criterion", func(i *RuntimeCoverageItem) { i.TargetText = "different" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			altered := item
			tc.mutate(&altered)
			if _, err := CompileRuntimeSourceObligationChecks(inventory, altered); err == nil {
				t.Fatal("source drift accepted")
			}
		})
	}
	checks, err := CompileRuntimeSourceObligationChecks(inventory, RuntimeCoverageItem{ID: "profile-cwi", SourceScope: "acceptanceDimension", SourceID: "profile-extra", TargetText: "附加质量约束"})
	if err != nil || len(checks) != 0 {
		t.Fatalf("invented authored source for Profile-only item: %+v %v", checks, err)
	}
}
