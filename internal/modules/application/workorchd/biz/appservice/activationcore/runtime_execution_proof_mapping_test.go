package activationcore

import (
	"encoding/json"
	"reflect"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestBootstrapExecutionProofMappingSealsExactPredecessorAndReload(t *testing.T) {
	_, inventory := sourceObligationBootstrapFixture(t)
	goal := &workspacemodel.Goal{ID: "goal", WorkspaceID: "workspace", Text: "目标"}
	dod := &workspacemodel.DoD{ID: "dod", WorkspaceID: "workspace", Items: []string{"共同判据", "原工作与修复判据"}}
	inventory.Sources = append(inventory.Sources, runtimecontract.RuntimeDefinitionSource{CandidateItemRef: "source", ItemRef: "source-formal", SourceScope: "dodItem", SourceID: "dod-002", Text: dod.Items[1]})
	inventory.Requirements[0].SourceItemRefs = append(inventory.Requirements[0].SourceItemRefs, "source")
	inventory.Obligations = append(inventory.Obligations, runtimecontract.RuntimeSourceObligation{ID: "source-obligation", RequirementID: "requirement", DoDItemRefs: []string{"source"}, Text: dod.Items[1]})
	inventory.Verifications = append(inventory.Verifications, runtimecontract.RuntimeSourceVerification{ID: "verify-source", ObligationID: "source-obligation", Declaration: "独立原工作证明", ProfileID: "test-deterministic-proof-dodItem", EvidenceRequirementID: "dod-002", Scope: "dodItem"})
	inventory.ExecutionProofMappings = []runtimecontract.RuntimeExecutionProofMapping{{ContractRef: runtimecontract.RuntimeDoDSourceExecutionProof, ConsumerItemRef: "dod", RuleRef: "original-work", SourceItemRef: "source"}}
	inventory = inventory.WithHash()
	activation := withTestDeterministicProof(goal, dod, runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "mock"}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired},
	})
	for n := range activation.ExecutionContract.ProfileContracts {
		for m := range activation.ExecutionContract.ProfileContracts[n].EvidenceRequirements {
			requirement := &activation.ExecutionContract.ProfileContracts[n].EvidenceRequirements[m]
			if requirement.Scope == "dodItem" && requirement.ID == "dod-001" {
				requirement.LineageAssertions = []runtimecontract.RuntimeLineageAssertionRule{{Ref: "original-work", Kind: runtimecontract.RuntimeLineageCompletedWorkRounds, ConsumptionPoint: runtimecontract.RuntimeProofPhaseExecution, MinCompletedRounds: 2, ExecutionSourceContractRef: runtimecontract.RuntimeDoDSourceExecutionProof}}
			}
		}
	}
	bootstrap, err := RuntimeBootstrapWithSourceObligations(goal, dod, inventory.DefinitionRevision, inventory.StartupInputHash, nil, activation, &inventory)
	if err != nil {
		t.Fatal(err)
	}
	var consumer, source int
	for n, item := range bootstrap.CoverageItems {
		if item.SourceID == "dod-001" {
			consumer = n
		}
		if item.SourceID == "dod-002" {
			source = n
		}
	}
	if consumer == source || !reflect.DeepEqual(bootstrap.CoverageItems[consumer].PrerequisiteCoverageWorkItemIDs, []common.CoverageWorkItemID{bootstrap.CoverageItems[source].ID}) {
		t.Fatal("bootstrap did not bind exact original source predecessor")
	}
	facts, err := bootstrap.InitialFacts("workspace", "run")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(facts)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded runtimecontract.RuntimeFacts
	if err := json.Unmarshal(payload, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err != nil {
		t.Fatalf("same-version reload: %v", err)
	}
	if !reflect.DeepEqual(facts.DefinitionSeed.SourceObligations.ExecutionProofMappings, inventory.ExecutionProofMappings) {
		t.Fatal("fresh facts lost confirmed mapping")
	}
	changed := bootstrap.Clone()
	changed.CoverageItems[consumer].PrerequisiteCoverageWorkItemIDs = nil
	changed.DefinitionSeed.CoverageItems = changed.CoverageItems
	changed.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(changed.DefinitionSeed)
	if changed.Validate() == nil {
		t.Fatal("coordinated rehash stripped declared source predecessor")
	}
	facts.DefinitionSeed = changed.DefinitionSeed
	if facts.Validate() == nil {
		t.Fatal("fresh facts accepted stripped source predecessor")
	}
	if _, err := RuntimeBootstrapWithSourceObligations(goal, dod, inventory.DefinitionRevision, inventory.StartupInputHash, nil, activation, nil); err == nil {
		t.Fatal("opted-in Profile silently fell back without confirmed mapping")
	}
	if err := bootstrap.Validate(); err != nil {
		t.Fatalf("tamper changed original bootstrap: %v", err)
	}
}
