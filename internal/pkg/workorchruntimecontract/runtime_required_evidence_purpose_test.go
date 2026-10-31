package workorchruntimecontract

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func requiredPurposeInventory(t *testing.T) (RuntimeSourceObligationInventory, []RuntimeProfileContract, []RuntimeCoverageItem) {
	t.Helper()
	i, p, items := profilePurposeFixture()
	p[0].DeliverableExpectations[0].RequiredEvidencePurposeContractRef = RuntimeProfileDeliverableEvidencePurpose
	i.EvidencePurposeMappings[0].HistoricalProfileHash = StableRuntimeProfileContractsHash(p)
	i = i.WithHash()
	return i, p, items
}
func TestRequiredProfilePurposeIndependentCompletenessAndLegacySnapshot(t *testing.T) {
	i, p, items := requiredPurposeInventory(t)
	before, _ := json.Marshal(p)
	if err := ValidateRuntimeSourceObligationApplicability(&i, p); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeEvidencePurposePresealCoverage(i, p, items); err != nil {
		t.Fatal(err)
	}
	required, err := RequiredRuntimeEvidencePurposes(p)
	if err != nil || len(required) != 1 || required[0].AcceptanceItemRef != "" {
		t.Fatal("catalog chose a consumer", err)
	}
	required[0].HistoricalText = "changed"
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("catalog aliases source")
	}
	missing := i.Clone()
	missing.EvidencePurposeMappings = nil
	missing = missing.WithHash()
	for _, err := range []error{ValidateRuntimeEvidencePurposeMappings(missing, p), ValidateRuntimeSourceObligationApplicability(&missing, p), ValidateRuntimeSourceObligationApplicability(nil, p)} {
		if err == nil || !strings.Contains(err.Error(), "required_evidence_purpose_missing") {
			t.Fatal("missing required source escaped independent gate", err)
		}
	}
	legacy := CloneRuntimeProfileContracts(p)
	legacy[0].DeliverableExpectations[0].RequiredEvidencePurposeContractRef = ""
	if err := ValidateRuntimeEvidencePurposeMappings(missing, legacy); err != nil {
		t.Fatal("legacy empty mapping changed", err)
	}
	raw, _ := json.Marshal(legacy[0].DeliverableExpectations[0])
	if strings.Contains(string(raw), "RequiredEvidencePurposeContractRef") {
		t.Fatal("empty extension changed old JSON")
	}
	if StableRuntimeProfileContractsHash(legacy) == StableRuntimeProfileContractsHash(p) {
		t.Fatal("new requirement absent from hash")
	}
	for _, change := range []func(*RuntimeSourceObligationInventory){
		func(i *RuntimeSourceObligationInventory) {
			i.EvidencePurposeMappings = append(i.EvidencePurposeMappings, i.EvidencePurposeMappings[0])
		},
		func(i *RuntimeSourceObligationInventory) { i.EvidencePurposeMappings[0].HistoricalText = "summary" },
		func(i *RuntimeSourceObligationInventory) {
			i.EvidencePurposeMappings[0].HistoricalProfileHash = StableRuntimeProfileContractsHash(legacy)
		},
		func(i *RuntimeSourceObligationInventory) { i.EvidencePurposeMappings[0].AcceptanceItemRef = "goal" },
	} {
		bad := i.Clone()
		change(&bad)
		bad = bad.WithHash()
		if ValidateRuntimeSourceObligationApplicability(&bad, p) == nil {
			t.Fatal("coordinated rehash bypassed required source")
		}
	}
}

func TestRequiredProfilePurposePresealRejectsHiddenSettlementCycle(t *testing.T) {
	i, p, items := requiredPurposeInventory(t)
	intermediate := items[1].Normalized()
	intermediate.ID = "dependent"
	intermediate.SourceID = "independent"
	intermediate.TargetText = "original independent obligation"
	intermediate.PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{items[0].ID}
	items = append(items, intermediate)
	if err := ValidateRuntimeEvidencePurposePresealCoverage(i, p, items); err != nil {
		t.Fatal(err)
	}
	items[1].PrerequisiteCoverageWorkItemIDs = append(items[1].PrerequisiteCoverageWorkItemIDs, intermediate.ID)
	if err := ValidateRuntimeCoverageItemDependencies(items); err != nil {
		t.Fatal("negative must have an ordinary acyclic graph", err)
	}
	if err := ValidateRuntimeEvidencePurposePresealCoverage(i, p, items); err == nil {
		t.Fatal("preseal accepted hidden terminal credit cycle")
	}
}
