package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeImplementationWorkCapabilityProjection(t *testing.T) {
	_, focuses, profiles := definitionImplementationWorkFixture(t)
	before, _ := json.Marshal(focuses)
	got, err := CompileRuntimeImplementationWorkCapabilityRequirements(focuses, profiles)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].OwnerModuleID != RuntimeModuleFile || got[0].CapabilityClass != RuntimeCapabilityClassStateChange ||
		!reflect.DeepEqual(got[0].RequiredCapabilities, []string{StepKindFileWrite, RuntimeBlockKindPatchApply}) {
		t.Fatalf("unexpected work inventory: %#v", got)
	}
	duplicate := append(CloneRuntimeDefinitionFocuses(focuses), focuses[0].Clone())
	duplicate[1].ID = "focus-2"
	again, err := CompileRuntimeImplementationWorkCapabilityRequirements(duplicate, profiles)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatalf("inventory is not deduplicated: %#v, %v", again, err)
	}
	got[0].RequiredCapabilities[0] = "mutated"
	after, _ := json.Marshal(focuses)
	if string(before) != string(after) {
		t.Fatal("inventory projection mutated Definition")
	}
	profiles[0].EvidenceRequirements[0].ID = "unmatched-source"
	focuses[0].ImplementationWorkPolicy = nil
	got, err = CompileRuntimeImplementationWorkCapabilityRequirements(focuses, profiles)
	if err != nil || len(got) != 0 {
		t.Fatalf("unmatched Profile enlisted work inventory: %#v, %v", got, err)
	}
	focuses[0].ImplementationWorkPolicy = &RuntimeImplementationWorkPolicy{}
	if _, err := CompileRuntimeImplementationWorkCapabilityRequirements(focuses, profiles); err == nil {
		t.Fatal("injected work policy accepted")
	}
}

func TestRuntimeImplementationWorkCapabilityRequirementsRejectIncompleteProjection(t *testing.T) {
	_, focuses, profiles := definitionImplementationWorkFixture(t)
	work, err := CompileRuntimeImplementationWorkCapabilityRequirements(focuses, profiles)
	if err != nil {
		t.Fatal(err)
	}
	base := RuntimeBootstrap{DefinitionFocuses: focuses, Activation: RuntimeActivationContext{ExecutionContract: RuntimeExecutionContract{ProfileContracts: profiles}},
		Definition: RuntimeDefinitionContext{CapabilityContract: append([]string(nil), work[0].RequiredCapabilities...)},
		Requirements: append(work, RuntimeRequirement{Kind: PrerequisiteKindOperationAdmissibility, Scope: RuntimePrerequisiteScopeGlobal,
			OwnerModuleID: RuntimeModuleFile, RequiredCapabilities: append([]string(nil), work[0].RequiredCapabilities...)})}
	validate := func(b RuntimeBootstrap) error {
		return ValidateRuntimeImplementationWorkCapabilityRequirements(b.DefinitionFocuses, b.Activation.ExecutionContract.ProfileContracts, b.Definition.CapabilityContract, b.Requirements)
	}
	if err := validate(base); err != nil {
		t.Fatal(err)
	}
	var replay RuntimeBootstrap
	raw, _ := json.Marshal(base)
	if err := json.Unmarshal(raw, &replay); err != nil || validate(replay) != nil {
		t.Fatalf("current-version replay lost projection: %v", err)
	}
	for name, mutate := range map[string]func(*RuntimeBootstrap){
		"missing inventory":         func(b *RuntimeBootstrap) { b.Definition.CapabilityContract = nil },
		"missing capability":        func(b *RuntimeBootstrap) { b.Requirements = b.Requirements[1:] },
		"missing admissibility":     func(b *RuntimeBootstrap) { b.Requirements = b.Requirements[:1] },
		"wrong owner":               func(b *RuntimeBootstrap) { b.Requirements[0].OwnerModuleID = RuntimeModuleShell },
		"wrong class":               func(b *RuntimeBootstrap) { b.Requirements[0].CapabilityClass = RuntimeCapabilityClassValidation },
		"wrong admissibility owner": func(b *RuntimeBootstrap) { b.Requirements[1].OwnerModuleID = RuntimeModuleShell },
		"missing operation": func(b *RuntimeBootstrap) {
			b.Requirements[1].RequiredCapabilities = b.Requirements[1].RequiredCapabilities[:1]
		},
		"focus scoped": func(b *RuntimeBootstrap) {
			b.Requirements[0].Scope = RuntimePrerequisiteScopeFocus
			b.Requirements[0].FocusID = "focus-1"
		},
		"unknown scope":   func(b *RuntimeBootstrap) { b.Requirements[0].Scope = "unknown" },
		"source omission": func(b *RuntimeBootstrap) { b.DefinitionFocuses[0].ImplementationWorkPolicy = nil },
	} {
		t.Run(name, func(t *testing.T) {
			changed := base.Clone()
			mutate(&changed)
			if err := validate(changed); err == nil {
				t.Fatal("incomplete or broadened projection accepted")
			}
		})
	}
}
