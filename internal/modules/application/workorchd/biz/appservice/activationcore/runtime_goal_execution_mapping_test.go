package activationcore

import (
	"encoding/json"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestBootstrapGoalExecutionMappingRetainsFullGoalWithoutWorkCWI(t *testing.T) {
	base, inventory := sourceObligationBootstrapFixture(t)
	goal := &workspace.Goal{ID: "goal", WorkspaceID: "workspace", Text: " 完整 Goal\r\n必须真实两轮与修复。 "}
	dod := &workspace.DoD{ID: "dod", WorkspaceID: "workspace", Items: []string{"共同判据"}}
	for n := range inventory.Sources {
		if inventory.Sources[n].SourceScope == "goal" {
			inventory.Sources[n].Text = goal.Text
		}
	}
	rule := runtimecontract.RuntimeLineageAssertionRule{Ref: "goal-original-work", Kind: runtimecontract.RuntimeLineageCompletedWorkRounds, ConsumptionPoint: runtimecontract.RuntimeProofPhaseExecution, MinCompletedRounds: 2, ExecutionSourceContractRef: runtimecontract.RuntimeGoalSourceExecutionProof}
	inventory.ExecutionProofMappings = []runtimecontract.RuntimeExecutionProofMapping{{ContractRef: rule.ExecutionSourceContractRef, ConsumerItemRef: "goal", RuleRef: rule.Ref, SourceItemRef: "dod", ConsumerGoalText: goal.Text}}
	inventory = inventory.WithHash()
	activation := base.Activation.Clone()
	activation.ExecutionContract.ProfileContracts = append(activation.ExecutionContract.ProfileContracts, runtimecontract.RuntimeProfileContract{ID: "goal-guard", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "goal", Scope: "goal", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, LineageAssertions: []runtimecontract.RuntimeLineageAssertionRule{rule}}}})
	b, err := RuntimeBootstrapWithSourceObligations(goal, dod, inventory.DefinitionRevision, inventory.StartupInputHash, nil, activation, &inventory)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.CoverageItems) != 1 || len(b.CoverageItems[0].GoalExecutionAssertions) != 1 || len(b.CoverageItems[0].SourceObligationChecks) != 3 {
		t.Fatal("lost Goal consumer or fabricated independent Goal CWI")
	}
	facts, err := b.InitialFacts("workspace", "run")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(facts)
	var reload runtimecontract.RuntimeFacts
	if json.Unmarshal(payload, &reload) != nil || reload.Validate() != nil {
		t.Fatal("fresh-load Goal identity lost")
	}
	for _, strip := range []string{"guard", "ordinary Goal"} {
		changed := b.Clone()
		if strip == "guard" {
			changed.CoverageItems[0].GoalExecutionAssertions = nil
		} else {
			changed.CoverageItems[0].SourceObligationChecks = changed.CoverageItems[0].SourceObligationChecks[1:]
		}
		changed.DefinitionSeed.CoverageItems = changed.CoverageItems
		changed.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(changed.DefinitionSeed)
		if changed.Validate() == nil {
			t.Fatal("coordinated hash stripped", strip)
		}
	}
	if _, err := RuntimeBootstrapWithSourceObligations(goal, dod, inventory.DefinitionRevision, inventory.StartupInputHash, nil, activation, nil); err == nil {
		t.Fatal("Goal opt-in lost confirmation without blocking")
	}
}
