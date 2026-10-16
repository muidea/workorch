package activationcore

import (
	"encoding/json"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// A supported declaration cannot authorize a partial bootstrap or fall back
// to same-CWI proof without exact original coverage and profile applicability.
func TestBootstrapEvidencePurposeMappingRejectsPartialOriginalSource(t *testing.T) {
	b, i := sourceObligationBootstrapFixture(t)
	i.Sources = append(i.Sources, runtimecontract.RuntimeDefinitionSource{CandidateItemRef: "source", ItemRef: "source-formal", SourceScope: "dodItem", SourceID: "dod-002", Text: "后继原普通验收"})
	i.Obligations = append(i.Obligations, runtimecontract.RuntimeSourceObligation{ID: "purpose-source-obligation", RequirementID: i.Obligations[0].RequirementID, DoDItemRefs: []string{"source"}, Text: "后继原普通验收"})
	i.Verifications = append(i.Verifications, runtimecontract.RuntimeSourceVerification{ID: "purpose-source-verification", ObligationID: "purpose-source-obligation", Declaration: "后继普通证明", ProfileID: i.Verifications[0].ProfileID, EvidenceRequirementID: "dod-002", Scope: "dodItem"})
	i.EvidencePurposeMappings = []runtimecontract.RuntimeEvidencePurposeMapping{{ContractRef: runtimecontract.RuntimeCrossCWIEvidencePurposeV1, HistoricalItemRef: "dod", AcceptanceItemRef: "source"}}
	i = i.WithHash()
	if err := i.Validate(); err != nil {
		t.Fatal(err)
	}
	for label, inventory := range map[string]*runtimecontract.RuntimeSourceObligationInventory{"original": &i, "clone": func() *runtimecontract.RuntimeSourceObligationInventory { value := i.Clone(); return &value }()} {
		if _, err := RuntimeBootstrapWithSourceObligations(nil, nil, i.DefinitionRevision, i.StartupInputHash, nil, b.Activation, inventory); err == nil {
			t.Fatalf("%s entered runtime: %v", label, err)
		}
	}
	b.Definition.SourceObligations = &i
	b.DefinitionSeed.SourceObligations = &i
	b.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(b.DefinitionSeed)
	if err := b.Validate(); err == nil {
		t.Fatalf("direct typed bootstrap bypassed incomplete source: %v", err)
	}
	if _, err := b.InitialFacts("workspace", "run"); err == nil {
		t.Fatal("partial mapping granted fresh authority")
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var reload runtimecontract.RuntimeBootstrap
	if err := json.Unmarshal(raw, &reload); err != nil {
		t.Fatal(err)
	}
	if err := reload.Validate(); err == nil {
		t.Fatalf("JSON reload enabled partial mapped authority: %v", err)
	}
	// The same complete original inventory can be admitted once both exact
	// sources and their distinct ordinary profile declarations are present.
	goal := &workspacemodel.Goal{ID: "goal", WorkspaceID: "workspace", Text: "目标"}
	dod := &workspacemodel.DoD{ID: "dod", WorkspaceID: "workspace", Items: []string{"共同判据", "后继原普通验收"}}
	activation := withTestDeterministicProof(goal, dod, runtimecontract.RuntimeActivationContext{
		ProviderSelection: b.Activation.ProviderSelection,
		ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired},
	})
	for p := range activation.ExecutionContract.ProfileContracts {
		for r := range activation.ExecutionContract.ProfileContracts[p].EvidenceRequirements {
			requirement := &activation.ExecutionContract.ProfileContracts[p].EvidenceRequirements[r]
			if requirement.Scope == "dodItem" && requirement.ID == "dod-001" {
				requirement.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
				requirement.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite}
				requirement.ValidationRecipeID, requirement.RequiredCheckRefs = "", nil
				requirement.RequiredOwnerFacts, requirement.OwnerFactConvergences = nil, nil
			}
		}
	}
	complete, err := RuntimeBootstrapWithSourceObligations(goal, dod, i.DefinitionRevision, i.StartupInputHash, nil, activation, &i)
	if err != nil {
		t.Fatalf("complete original mapped bootstrap rejected: %v", err)
	}
	raw, err = json.Marshal(complete)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &reload); err != nil || reload.Validate() != nil {
		t.Fatalf("complete original mapped bootstrap reload: %v", err)
	}
	facts, err := reload.InitialFacts("workspace", "run")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range facts.Coverage {
		if row.State != runtimecontract.RuntimeCoverageItemStateOpen || row.HistoricalBehavior != nil || len(row.HistoricalAcceptanceProofs) != 0 {
			t.Fatal("mapped declaration granted behavior or terminal credit")
		}
	}
}
