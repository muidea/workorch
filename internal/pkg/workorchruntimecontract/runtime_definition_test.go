package workorchruntimecontract

import "testing"

func TestRuntimeDefinitionContextNormalizesCapabilityContract(t *testing.T) {
	definition := RuntimeDefinitionContext{
		Revision: " definition-1 ", GoalID: "goal-1", GoalDefinition: " goal ",
		DoDID: "dod-1", DoDDefinition: " dod ", CapabilityContract: []string{"shell/command", "file/write", "shell/command"},
	}.Normalized()
	if err := definition.Validate(); err != nil {
		t.Fatal(err)
	}
	if definition.Revision != "definition-1" || definition.GoalDefinition != "goal" || len(definition.CapabilityContract) != 2 || definition.CapabilityContract[0] != "file/write" {
		t.Fatalf("normalized definition = %+v", definition)
	}
}

func TestRuntimeStateRejectsDefinitionRevisionMismatch(t *testing.T) {
	facts := testRuntimeFacts()
	decision, err := SealRuntimeDecision(facts, RuntimeDecision{
		State: ConvergenceStateNeedStateChange, Action: RuntimeActionProduceStateChange, Focus: facts.Focus,
		RequiredOutput: RequiredOutputContract{Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{"file/write"}, AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	record := RuntimeStateRecord{
		StateRevision: 1,
		Definition: RuntimeDefinitionContext{
			Revision: "other-definition", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/write"},
		},
		Facts: facts, Decision: decision,
	}
	if err := record.Validate(); err == nil {
		t.Fatal("definition revision mismatch was accepted")
	}
}

func TestRuntimeCandidatePrefixChangesWithExecutionContract(t *testing.T) {
	definition := RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/edit"},
	}
	optional := StableRuntimeCandidatePrefixHashWithExecutionContract(definition, RuntimeExecutionContract{StateChangePolicy: RuntimeStateChangeAllowedButNotRequired})
	required := StableRuntimeCandidatePrefixHashWithExecutionContract(definition, RuntimeExecutionContract{
		StateChangePolicy:       RuntimeStateChangeMustProduce,
		CapabilityUsagePolicies: []RuntimeCapabilityUsagePolicy{{ID: "usage-1", PreferredTools: []string{"lsp_diagnostics"}}},
	})
	if optional == required {
		t.Fatalf("candidate prefix did not change with execution contract: %s", optional)
	}
}

func TestRuntimeCandidatePrefixChangesWithOperationContractRevision(t *testing.T) {
	definition := RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/edit"},
	}
	base := RuntimeExecutionContract{OperationContractRevision: "operation-contract-a"}
	changed := base
	changed.OperationContractRevision = "operation-contract-b"
	if StableRuntimeCandidatePrefixHashWithExecutionContract(definition, base) == StableRuntimeCandidatePrefixHashWithExecutionContract(definition, changed) {
		t.Fatal("candidate prefix did not change with operation contract revision")
	}
}

func TestRuntimeCandidatePrefixChangesWithProfileContract(t *testing.T) {
	definition := RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"git/write"},
	}
	base := RuntimeExecutionContract{ProfileContracts: []RuntimeProfileContract{{
		ID:                      "software-dev",
		AcceptanceDimensions:    []RuntimeAcceptanceDimension{{ID: "runtime_entrypoint", Required: true}},
		DeliverableExpectations: []RuntimeDeliverableExpectation{{ID: "version_control_snapshot", Required: true, EvidenceRole: "supplemental_evidence"}},
		EvidenceRequirements:    []RuntimeProfileEvidenceRequirement{{ID: "version_control_snapshot", Scope: "deliverable", AcceptedStepKinds: []string{"git/write"}, AcceptedOperationVariants: []RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "commit"}}}},
	}}}
	changed := base
	changed.ProfileContracts = append([]RuntimeProfileContract(nil), base.ProfileContracts...)
	changed.ProfileContracts[0].EvidenceRequirements = append([]RuntimeProfileEvidenceRequirement(nil), base.ProfileContracts[0].EvidenceRequirements...)
	changed.ProfileContracts[0].EvidenceRequirements[0].AcceptedOperationVariants = []RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "push"}}
	baseHash := StableRuntimeCandidatePrefixHashWithExecutionContract(definition, base)
	changedHash := StableRuntimeCandidatePrefixHashWithExecutionContract(definition, changed)
	if baseHash == changedHash {
		t.Fatalf("candidate prefix did not change with profile contract: %s", baseHash)
	}
}

func TestRuntimeCandidatePrefixChangesWithTemplateStagePolicy(t *testing.T) {
	definition := RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/edit"},
	}
	base := RuntimeExecutionContract{TemplateStagePolicy: &RuntimeTemplateStagePolicy{SchemaVersion: RuntimeTemplateStagePolicySchemaV1, Stages: []RuntimeTemplateStage{{
		ID: "execution", Entry: RuntimeTemplateStageEntry{NextAction: "state_change"},
		RequiredOutputs: []RuntimeTemplateStageRequiredOutput{{Kind: "state_change", EvidenceKinds: []string{"state_change"}}},
	}}}}
	changed := base.Normalized()
	changed.TemplateStagePolicy.Stages[0].Entry.NextAction = "validate"
	if StableRuntimeCandidatePrefixHashWithExecutionContract(definition, base) == StableRuntimeCandidatePrefixHashWithExecutionContract(definition, changed) {
		t.Fatal("candidate prefix did not change with template stage policy")
	}
}

func TestRuntimeCandidatePrefixChangesWithSealedValidationAndSlicePolicies(t *testing.T) {
	definition := RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/read"},
	}
	base := RuntimeExecutionContract{
		MaxRepairAttemptsPerFocus: 7,
		ValidationPolicies: []RuntimeValidationPolicy{{
			ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{EvidenceKindValidation}, MaxValidationOnlyRetriesPerFocus: 2,
		}},
		AcceptanceSlicePolicy: &RuntimeAcceptanceSlicePolicy{SchemaVersion: RuntimeAcceptanceSlicePolicySchemaV1, MaxWorkItems: 4},
	}
	changedValidation := base.Normalized()
	changedValidation.ValidationPolicies[0].MaxValidationOnlyRetriesPerFocus = 3
	if StableRuntimeCandidatePrefixHashWithExecutionContract(definition, base) == StableRuntimeCandidatePrefixHashWithExecutionContract(definition, changedValidation) {
		t.Fatal("candidate prefix did not change with validation policy")
	}
	changedSlice := base.Normalized()
	changedSlice.AcceptanceSlicePolicy.MaxWorkItems = 8
	if StableRuntimeCandidatePrefixHashWithExecutionContract(definition, base) == StableRuntimeCandidatePrefixHashWithExecutionContract(definition, changedSlice) {
		t.Fatal("candidate prefix did not change with acceptance slice policy")
	}
}

func TestRuntimeCandidatePrefixChangesWithDefinitionChangeImpact(t *testing.T) {
	definition := RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-2", GoalDefinition: "goal", DoDID: "dod-2", DoDDefinition: "dod", CapabilityContract: []string{"file/edit"},
		ChangeImpact: &RuntimeDefinitionChangeImpact{
			Version: RuntimeDefinitionChangeImpactVersionV1, Changed: true, Policy: "cumulative", RequiredPreExecutionAssessment: true,
			GoalChange: RuntimeDefinitionChangeSet{EffectiveDefinitionID: "goal-2", AllDefinitionIDs: []string{"goal-1", "goal-2"}},
			DoDChange:  RuntimeDefinitionChangeSet{EffectiveDefinitionID: "dod-2", AllDefinitionIDs: []string{"dod-1", "dod-2"}},
		},
	}
	changed := definition.Clone()
	changed.ChangeImpact.StrategyRequirements = []string{"identify conflicts"}
	if StableRuntimeCandidatePrefixHash(definition) == StableRuntimeCandidatePrefixHash(changed) {
		t.Fatal("candidate prefix did not change with definition change impact")
	}
}
