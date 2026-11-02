package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
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

func TestRequiredProfilePurposeSeparatesOriginalAuthenticationFromCurrentMethods(t *testing.T) {
	inventory, original, items := requiredPurposeInventory(t)
	before, _ := json.Marshal(inventory)
	for _, change := range []string{"proof method", "deliverable description", "another Profile"} {
		t.Run(change, func(t *testing.T) {
			current := CloneRuntimeProfileContracts(original)
			switch change {
			case "proof method":
				current[0].EvidenceRequirements[1].Description = "current proof method"
			case "deliverable description":
				current[0].DeliverableExpectations[0].Description = "new descriptive text is not the original obligation"
			case "another Profile":
				current = append(current, RuntimeProfileContract{ID: "independent", EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{ID: "unrelated", Scope: "dodItem", RequiredEvidenceKinds: []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}}}})
			}
			for range 2 {
				if err := ValidateRuntimeHistoricalProfileEvidencePurposeSources(&inventory, original); err != nil {
					t.Fatal(err)
				}
				if err := ValidateRuntimeSourceObligationApplicability(&inventory, current); err != nil {
					t.Fatalf("current method blocked unchanged required source: %v", err)
				}
				if err := ValidateRuntimeEvidencePurposePresealCoverage(inventory, current, items); err != nil {
					t.Fatal(err)
				}
			}
			for _, mutate := range []func(*RuntimeSourceObligationInventory){
				func(i *RuntimeSourceObligationInventory) {
					i.EvidencePurposeMappings[0].HistoricalText = "invented history"
				},
				func(i *RuntimeSourceObligationInventory) {
					i.EvidencePurposeMappings[0].HistoricalProfileHash = "runtime-profile-contracts:candidate-prefix-01234567890123456789"
				},
			} {
				forged := inventory.Clone()
				mutate(&forged)
				forged = forged.WithHash()
				if ValidateRuntimeHistoricalProfileEvidencePurposeSources(&forged, original) == nil {
					t.Fatal("current method authenticated coordinated rehash of original source")
				}
			}
		})
	}
	after, _ := json.Marshal(inventory)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("current applicability rewrote original inventory")
	}
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
		if ValidateRuntimeHistoricalProfileEvidencePurposeSources(&bad, p) == nil && ValidateRuntimeSourceObligationApplicability(&bad, p) == nil {
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

func TestRequiredProfilePurposesKeepEachOriginalProfileAndAllConsumerChecks(t *testing.T) {
	inventory, original, items := requiredPurposeInventory(t)
	second := RuntimeProfileContract{ID: "second", DeliverableExpectations: []RuntimeDeliverableExpectation{{
		ID: "second-delivery", Required: true, Description: "第二份原交付义务\r\n全文",
		RequiredEvidencePurposeContractRef: RuntimeProfileDeliverableEvidencePurpose,
	}}, EvidenceRequirements: []RuntimeProfileEvidenceRequirement{{ID: "second-delivery", Scope: "deliverable", RequiredEvidenceKinds: []string{EvidenceKindStateChange}, AcceptedStepKinds: []string{StepKindFileWrite}}}}
	original = append(original, second)
	inventory.EvidencePurposeMappings = append(inventory.EvidencePurposeMappings, RuntimeEvidencePurposeMapping{
		ContractRef: RuntimeProfileDeliverableEvidencePurpose, HistoricalItemRef: RuntimeProfileDeliverablePurposeRef(second.ID, "second-delivery"),
		HistoricalProfileID: second.ID, HistoricalDeliverableID: "second-delivery", HistoricalProfileHash: StableRuntimeProfileContractsHash([]RuntimeProfileContract{second}),
		HistoricalText: second.DeliverableExpectations[0].Description, AcceptanceItemRef: "acceptance",
	})
	inventory = inventory.WithHash()
	items = append(items, RuntimeCoverageItem{ID: "second-history", SliceID: "slice", SourceScope: "deliverable", SourceID: "second-delivery", TargetText: second.DeliverableExpectations[0].Description})
	current := CloneRuntimeProfileContracts(original)
	for n := range current {
		current[n].DeliverableExpectations[0].Description = "current guidance for " + current[n].ID
	}
	if err := ValidateRuntimeHistoricalProfileEvidencePurposeSources(&inventory, original); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeSourceObligationApplicability(&inventory, current); err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileRuntimeEvidencePurposeMappingCoverage(inventory, items)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeEvidencePurposePresealCoverage(inventory, current, compiled); err != nil {
		t.Fatal(err)
	}
	checks, err := CompileRuntimeSourceObligationChecks(inventory, compiled[1])
	if err != nil || len(checks) != 3 || len(compiled[1].PrerequisiteCoverageWorkItemIDs) != 2 {
		t.Fatalf("consumer lost independent original/ordinary all-of checks: checks=%d error=%v", len(checks), err)
	}
	forged := inventory.Clone()
	forged.EvidencePurposeMappings[1].HistoricalProfileHash = inventory.EvidencePurposeMappings[0].HistoricalProfileHash
	forged = forged.WithHash()
	if ValidateRuntimeHistoricalProfileEvidencePurposeSources(&forged, original) == nil {
		t.Fatal("another Profile hash substituted for independent historical source")
	}
}
