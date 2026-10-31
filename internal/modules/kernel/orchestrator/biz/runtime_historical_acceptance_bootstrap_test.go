package orchestrator

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// A private admitted-source fixture. It retains both original texts and compiles
// all original checks for the exact consumer; it never supplies runtime proof.
func runtimeTestWithHistoricalPurposeMapping(t *testing.T, bootstrap runtimecontract.RuntimeBootstrap, historicalID, acceptanceID common.CoverageWorkItemID) runtimecontract.RuntimeBootstrap {
	t.Helper()
	inventory := runtimecontract.RuntimeSourceObligationInventory{
		SchemaVersion: runtimecontract.RuntimeSourceObligationInventorySchemaCurrent,
		CandidateHash: "historical-purpose-original-candidate", SpecificationHash: "historical-purpose-confirmed-specification",
		DefinitionRevision: bootstrap.Definition.Revision, StartupInputHash: bootstrap.Definition.StartupInputHash,
		Sources:                 []runtimecontract.RuntimeDefinitionSource{{CandidateItemRef: "original-goal", ItemRef: "formal-goal", SourceScope: "goal", SourceID: "goal", Text: bootstrap.Definition.GoalDefinition}},
		EvidencePurposeMappings: []runtimecontract.RuntimeEvidencePurposeMapping{{ContractRef: runtimecontract.RuntimeCrossCWIEvidencePurpose, HistoricalItemRef: string(historicalID), AcceptanceItemRef: string(acceptanceID)}},
	}
	for index := range bootstrap.CoverageItems {
		item := &bootstrap.CoverageItems[index]
		if item.ID != historicalID && item.ID != acceptanceID {
			continue
		}
		item.SourceScope = "dodItem"
		ref := string(item.ID)
		inventory.Sources = append(inventory.Sources, runtimecontract.RuntimeDefinitionSource{CandidateItemRef: ref, ItemRef: "formal/" + ref, SourceScope: item.SourceScope, SourceID: item.SourceID, Text: item.TargetText})
		inventory.Requirements = append(inventory.Requirements, runtimecontract.RuntimeSourceRequirement{ID: "requirement/" + ref, DimensionID: "dimension/" + ref, Required: true, SourceKind: "USER_EXPLICIT", SourceItemRefs: []string{ref}, Text: item.TargetText})
		inventory.Obligations = append(inventory.Obligations, runtimecontract.RuntimeSourceObligation{ID: "obligation/" + ref, RequirementID: "requirement/" + ref, DoDItemRefs: []string{ref}, Text: item.TargetText})
		for p := range bootstrap.Activation.ExecutionContract.ProfileContracts {
			profile := &bootstrap.Activation.ExecutionContract.ProfileContracts[p]
			for r := range profile.EvidenceRequirements {
				requirement := &profile.EvidenceRequirements[r]
				if requirement.ID == item.SourceID {
					requirement.Scope = item.SourceScope
					inventory.Verifications = append(inventory.Verifications, runtimecontract.RuntimeSourceVerification{ID: "verification/" + ref, ObligationID: "obligation/" + ref, Declaration: item.TargetText, ProfileID: profile.ID, EvidenceRequirementID: requirement.ID, Scope: item.SourceScope})
				}
			}
		}
	}
	inventory = inventory.WithHash()
	if err := inventory.Validate(); err != nil {
		t.Fatal(err)
	}
	bootstrap.Definition.SourceObligations = &inventory
	items, err := runtimecontract.CompileRuntimeEvidencePurposeMappingCoverage(inventory, bootstrap.CoverageItems)
	if err != nil {
		t.Fatal(err)
	}
	for index := range items {
		items[index].SourceObligationChecks, err = runtimecontract.CompileRuntimeSourceObligationChecks(inventory, items[index])
		if err != nil {
			t.Fatal(err)
		}
	}
	bootstrap.CoverageItems = items
	bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
	if err := bootstrap.Validate(); err != nil {
		t.Fatal(err)
	}
	if bootstrap.DefinitionSeed.SourceObligations == nil || bootstrap.DefinitionSeed.Hash != runtimecontract.StableRuntimeDefinitionSeedHash(bootstrap.DefinitionSeed) {
		t.Fatal("private compiler lost admitted purpose mapping")
	}
	return bootstrap
}
