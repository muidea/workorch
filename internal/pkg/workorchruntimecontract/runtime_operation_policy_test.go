package workorchruntimecontract

import "testing"

func TestEffectiveRuntimeOperationPoliciesCurrentOwnerCatalogReplacesActivation(t *testing.T) {
	stateChanging := false
	activationPolicy := RuntimeOperationPolicyFact{
		ID: "activation-policy", OwnerModuleID: RuntimeModuleShell, OperationKind: StepKindShellCommand,
		ParameterMatches: []RuntimeOperationParameterMatch{{Name: "stateChanging", Boolean: &stateChanging}},
		EffectScope:      RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}
	catalog := RuntimeOperationAdmissibility{
		PrerequisiteID: "catalog-shell", OwnerModuleID: RuntimeModuleShell,
		OperationKinds: []string{StepKindShellCommand}, ObservedRevision: "shell-v1",
	}
	if got := EffectiveRuntimeOperationPolicies([]RuntimeOperationPolicyFact{activationPolicy}, []RuntimeOperationAdmissibility{catalog}); len(got) != 0 {
		t.Fatalf("stale activation policy survived an authoritative empty owner catalog: %+v", got)
	}

	catalogPolicy := activationPolicy
	catalogPolicy.ID = "catalog-policy"
	catalogPolicy.EffectScope = RuntimeEffectEphemeralSession
	catalog.OperationPolicies = []RuntimeOperationPolicyFact{catalogPolicy}
	got := EffectiveRuntimeOperationPolicies([]RuntimeOperationPolicyFact{activationPolicy}, []RuntimeOperationAdmissibility{catalog})
	if len(got) != 1 || got[0].ID != catalogPolicy.ID || got[0].EffectScope != RuntimeEffectEphemeralSession {
		t.Fatalf("current owner policy did not replace activation policy: %+v", got)
	}
}

func TestRuntimeOperationPolicyForOperationUsesMostSpecificNonConflictingFact(t *testing.T) {
	stateChanging := false
	command := "go"
	policies := []RuntimeOperationPolicyFact{
		{
			ID: "kind-default", OwnerModuleID: RuntimeModuleShell, OperationKind: StepKindShellCommand,
			EffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
		},
		{
			ID: "read-only-command", OwnerModuleID: RuntimeModuleShell, OperationKind: StepKindShellCommand,
			ParameterMatches: []RuntimeOperationParameterMatch{
				{Name: "command", String: &command}, {Name: "stateChanging", Boolean: &stateChanging},
			},
			EffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
		},
	}
	operation := RuntimeOperation{Kind: StepKindShellCommand, Parameters: []OperationParameter{
		{Name: "command", String: &command}, {Name: "stateChanging", Boolean: &stateChanging},
	}}
	policy, found := RuntimeOperationPolicyForOperation(operation, policies)
	if !found || policy.ID != "read-only-command" || policy.EffectScope != RuntimeEffectNone {
		t.Fatalf("specific owner policy match = %+v found=%t", policy, found)
	}
}

func TestRuntimeOperationPolicyValidationRejectsDuplicateAndConflictingAuthority(t *testing.T) {
	stateChanging := false
	base := RuntimeOperationPolicyFact{
		ID: "shell/read-only", OwnerModuleID: RuntimeModuleShell, OperationKind: StepKindShellCommand,
		ParameterMatches: []RuntimeOperationParameterMatch{{Name: "stateChanging", Boolean: &stateChanging}},
		EffectScope:      RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}
	if err := ValidateRuntimeOperationPolicyFacts([]RuntimeOperationPolicyFact{base, base}); err == nil {
		t.Fatal("duplicate operation policy identity was accepted")
	}
	conflict := base
	conflict.ID = "shell/conflicting"
	conflict.EffectScope = RuntimeEffectWorkspaceDurable
	if err := ValidateRuntimeOperationPolicyFacts([]RuntimeOperationPolicyFact{base, conflict}); err == nil {
		t.Fatal("conflicting operation policy selector was accepted")
	}
	activation := RuntimeActivationContext{
		ProviderSelection: ProviderInferenceSelection{Target: ProviderModelTarget{ProviderID: "provider", ModelID: "model"}},
		OperationPolicies: []RuntimeOperationPolicyFact{base, base},
	}
	if err := activation.Validate(); err == nil {
		t.Fatal("activation normalization hid duplicate operation policy authority")
	}
	catalog := RuntimeOperationAdmissibility{
		PrerequisiteID: "shell-catalog", OwnerModuleID: RuntimeModuleShell,
		OperationKinds: []string{StepKindShellCommand}, OperationPolicies: []RuntimeOperationPolicyFact{base, base},
		ObservedRevision: "shell-v1",
	}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err == nil {
		t.Fatal("catalog normalization hid duplicate operation policy authority")
	}
}
