package workorchruntimecontract

import (
	"reflect"
	"sort"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeOperationDescriptorsAreStableCompleteAndUnique(t *testing.T) {
	descriptors := RuntimeOperationDescriptors()
	if len(descriptors) == 0 {
		t.Fatal("expected non-empty operation descriptor inventory")
	}
	kinds := make([]string, 0, len(descriptors))
	seen := map[string]struct{}{}
	for _, descriptor := range descriptors {
		if descriptor.Kind == "" || descriptor.OwnerModuleID == "" || descriptor.EffectClass == "" || descriptor.CapabilityClass == "" {
			t.Fatalf("descriptor incomplete: %+v", descriptor)
		}
		if len(descriptor.AllowedActions) == 0 {
			t.Fatalf("descriptor %q requires allowed actions", descriptor.Kind)
		}
		if _, ok := seen[descriptor.Kind]; ok {
			t.Fatalf("duplicate descriptor kind %q", descriptor.Kind)
		}
		seen[descriptor.Kind] = struct{}{}
		kinds = append(kinds, descriptor.Kind)
	}
	sorted := append([]string(nil), kinds...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(kinds, sorted) {
		t.Fatalf("descriptor inventory is not stably sorted by kind:\n got %v\nwant %v", kinds, sorted)
	}

	// Mutation of returned inventory must not affect global authority.
	descriptors[0].Kind = "mutated"
	descriptors[0].AllowedActions[0] = RuntimeActionFail
	again, ok := RuntimeOperationDescriptorForKind(kinds[0])
	if !ok || again.Kind != kinds[0] || again.AllowedActions[0] == RuntimeActionFail {
		t.Fatalf("RuntimeOperationDescriptors must return defensive copies, got %+v", again)
	}
}

func TestRuntimeOperationDescriptorForKindCanonicalAndUnknown(t *testing.T) {
	descriptor, ok := RuntimeOperationDescriptorForKind(RuntimeBlockKindManagedValidationSession)
	if !ok {
		t.Fatal("managed validation must have a descriptor")
	}
	if descriptor.OwnerModuleID != RuntimeModuleShell {
		t.Fatalf("managed validation owner = %q, want %s", descriptor.OwnerModuleID, RuntimeModuleShell)
	}
	if descriptor.EffectClass != RuntimeOperationEffectNonMutating {
		t.Fatalf("managed validation effect = %q, want non_mutating", descriptor.EffectClass)
	}
	if descriptor.CapabilityClass != RuntimeCapabilityClassValidation {
		t.Fatalf("managed validation capability class = %q", descriptor.CapabilityClass)
	}
	if !runtimeDescriptorAllowsAction(descriptor, RuntimeActionProduceValidation) {
		t.Fatal("managed validation must allow produce_validation")
	}
	if runtimeDescriptorAllowsAction(descriptor, RuntimeActionProduceStateChange) {
		t.Fatal("managed validation must not allow produce_state_change")
	}

	// Canonical aliases still resolve through descriptor authority.
	if got := RuntimeModuleForOperationKind("patch/request"); got != RuntimeModuleFile {
		t.Fatalf("RuntimeModuleForOperationKind(patch/request) = %q, want %s", got, RuntimeModuleFile)
	}
	if _, ok := RuntimeOperationDescriptorForKind("unknown/kind"); ok {
		t.Fatal("unknown kind must not resolve a descriptor")
	}
	if RuntimeModuleForOperationKind("unknown/kind") != "" {
		t.Fatal("unknown kind must have empty owner module")
	}
}

func TestOperationSideEffectsDeriveFromDescriptor(t *testing.T) {
	if OperationHasWorkspaceSideEffects(RuntimeOperation{Kind: RuntimeBlockKindManagedValidationSession}) {
		t.Fatal("managed validation must be non-mutating")
	}
	if !OperationHasWorkspaceSideEffects(RuntimeOperation{Kind: StepKindShellCommand}) {
		t.Fatal("shell/command must remain side-effecting at kind level")
	}
	if !OperationHasWorkspaceSideEffects(RuntimeOperation{Kind: "totally/unknown"}) {
		t.Fatal("unknown kinds must default to side-effecting")
	}
	if OperationHasWorkspaceSideEffects(RuntimeOperation{Kind: StepKindFileRead}) {
		t.Fatal("file/read must be non-mutating")
	}
	if !OperationHasWorkspaceSideEffects(RuntimeOperation{Kind: RuntimeBlockKindPatchApply}) {
		t.Fatal("patch/apply must be side-effecting")
	}
}

func TestFeasibleOperationKindsIntersectAcceptedCapabilitiesAndEffect(t *testing.T) {
	required := RequiredOutputContract{
		Kind:                RequiredOutputKindValidation,
		AcceptedKinds:       []string{StepKindShellCommand, RuntimeBlockKindManagedValidationSession, StepKindFileRead, "unknown/kind"},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}
	capabilities := RuntimeCapabilities{
		ValidationKinds: []string{StepKindShellCommand, RuntimeBlockKindManagedValidationSession, StepKindFileRead},
	}
	got := FeasibleOperationKinds(required, capabilities)
	want := []string{StepKindFileRead, RuntimeBlockKindManagedValidationSession}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FeasibleOperationKinds = %v, want %v", got, want)
	}

	// shell/command remains non-feasible for validation despite being sealed into ValidationKinds.
	if OperationHasWorkspaceSideEffects(RuntimeOperation{Kind: StepKindShellCommand}) == false {
		t.Fatal("shell feasibility exclusion depends on side-effecting kind effect")
	}

	stateRequired := RequiredOutputContract{
		Kind:                RequiredOutputKindStateChange,
		AcceptedKinds:       []string{RuntimeBlockKindPatchApply, StepKindFileRead},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}
	stateCaps := RuntimeCapabilities{StateChangeKinds: []string{RuntimeBlockKindPatchApply, StepKindFileRead}}
	got = FeasibleOperationKinds(stateRequired, stateCaps)
	want = []string{RuntimeBlockKindPatchApply}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("state-change FeasibleOperationKinds = %v, want %v", got, want)
	}

	if kinds := FeasibleOperationKinds(required, RuntimeCapabilities{}); kinds != nil {
		t.Fatalf("empty capabilities must yield no feasible kinds, got %v", kinds)
	}
}

func TestRuntimeModuleForOperationKindMatchesDescriptorInventory(t *testing.T) {
	for _, descriptor := range RuntimeOperationDescriptors() {
		// Governance/assessment inventory kinds still expose owner via descriptor.
		if got := RuntimeModuleForOperationKind(descriptor.Kind); got != descriptor.OwnerModuleID {
			t.Fatalf("RuntimeModuleForOperationKind(%q) = %q, want %q", descriptor.Kind, got, descriptor.OwnerModuleID)
		}
	}
}
