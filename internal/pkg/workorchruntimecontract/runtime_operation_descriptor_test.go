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
		if descriptor.Kind == "" || descriptor.OwnerModuleID == "" || descriptor.DefaultEffectScope == "" || descriptor.CapabilityClass == "" {
			t.Fatalf("descriptor incomplete: %+v", descriptor)
		}
		if len(descriptor.AllowedStates) == 0 {
			t.Fatalf("descriptor %q requires allowed states", descriptor.Kind)
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
	descriptors[0].AllowedStates[0] = ConvergenceStateFailed
	again, ok := RuntimeOperationDescriptorForKind(kinds[0])
	if !ok || again.Kind != kinds[0] || again.AllowedStates[0] == ConvergenceStateFailed {
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
	if descriptor.DefaultEffectScope != RuntimeEffectEphemeralSession || descriptor.IdempotencyClass != RuntimeOperationIdempotencyAtMostOnce {
		t.Fatalf("managed validation descriptor = %+v, want ephemeral/at-most-once", descriptor)
	}
	if descriptor.CapabilityClass != RuntimeCapabilityClassValidation {
		t.Fatalf("managed validation capability class = %q", descriptor.CapabilityClass)
	}
	if !runtimeDescriptorAllowsState(descriptor, ConvergenceStateNeedValidation) {
		t.Fatal("managed validation must allow produce_validation")
	}
	if runtimeDescriptorAllowsState(descriptor, ConvergenceStateNeedStateChange) {
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
	fileWrite, ok := RuntimeOperationDescriptorForKind(StepKindFileWrite)
	if !ok || fileWrite.TargetCardinality != RuntimeOperationTargetCardinalityOne {
		t.Fatalf("file/write descriptor must retain only its technical target shape: %+v", fileWrite)
	}
	patch, ok := RuntimeOperationDescriptorForKind(RuntimeBlockKindPatchApply)
	if !ok || patch.TargetCardinality != RuntimeOperationTargetCardinalityOne {
		t.Fatalf("patch/apply descriptor technical target shape = %+v", patch)
	}
	shell, ok := RuntimeOperationDescriptorForKind(StepKindShellCommand)
	if !ok || shell.TargetCardinality != RuntimeOperationTargetCardinalityMany {
		t.Fatalf("shell/command descriptor technical target shape = %+v", shell)
	}
	git, ok := RuntimeOperationDescriptorForKind(StepKindGitWrite)
	if !ok || git.OwnerModuleID != RuntimeModuleGit {
		t.Fatalf("git/write descriptor = %+v", git)
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
		AcceptedKinds:       []string{StepKindShellCommand, RuntimeBlockKindManagedValidationSession, StepKindFileRead, "unknown/kind"},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}
	capabilities := RuntimeCapabilities{
		ValidationKinds: []string{StepKindShellCommand, RuntimeBlockKindManagedValidationSession, StepKindFileRead},
	}
	got := FeasibleOperationKinds(ConvergenceStateNeedValidation, required, capabilities)
	// shell/command has no activation-sealed concrete non-durable effect, so it is
	// absent from the candidate request. Validation uses a descriptor explicitly
	// marked non-durable or a concrete sealed capability instead.
	want := []string{StepKindFileRead, RuntimeBlockKindManagedValidationSession}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FeasibleOperationKinds = %v, want %v", got, want)
	}

	// Arbitrary shell/command remains side-effecting for every admitted shape.
	if !OperationHasWorkspaceSideEffects(RuntimeOperation{Kind: StepKindShellCommand}) {
		t.Fatal("shell/command must remain side-effecting")
	}

	stateRequired := RequiredOutputContract{
		AcceptedKinds:       []string{RuntimeBlockKindPatchApply, StepKindFileRead},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}
	stateCaps := RuntimeCapabilities{StateChangeKinds: []string{RuntimeBlockKindPatchApply, StepKindFileRead}}
	got = FeasibleOperationKinds(ConvergenceStateNeedStateChange, stateRequired, stateCaps)
	want = []string{RuntimeBlockKindPatchApply}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("state-change FeasibleOperationKinds = %v, want %v", got, want)
	}

	if kinds := FeasibleOperationKinds(ConvergenceStateNeedValidation, required, RuntimeCapabilities{}); kinds != nil {
		t.Fatalf("empty capabilities must yield no feasible kinds, got %v", kinds)
	}

	mcpRequired := RequiredOutputContract{
		AcceptedKinds:       []string{StepKindMCPToolCall},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	}
	mcpCaps := RuntimeCapabilities{ValidationKinds: []string{StepKindMCPToolCall}}
	if got := FeasibleOperationKindsWithActivation(ConvergenceStateNeedValidation, mcpRequired, mcpCaps, nil); len(got) != 0 {
		t.Fatalf("MCP without explicit effect must not be feasible, got %v", got)
	}
	if got := FeasibleOperationKindsWithActivation(ConvergenceStateNeedValidation, mcpRequired, mcpCaps, []RuntimeOperationPolicyFact{
		mcpOperationPolicyForTest("read", "srv", "inspect", RuntimeEffectNone, RuntimeOperationIdempotencySafeToReplay),
	}); !reflect.DeepEqual(got, []string{StepKindMCPToolCall}) {
		t.Fatalf("explicit non-durable MCP capability must be feasible, got %v", got)
	}
	if got := FeasibleOperationKindsWithActivation(ConvergenceStateNeedValidation, mcpRequired, mcpCaps, []RuntimeOperationPolicyFact{
		mcpOperationPolicyForTest("write", "srv", "apply", RuntimeEffectWorkspaceDurable, RuntimeOperationIdempotencyAtMostOnce),
	}); len(got) != 0 {
		t.Fatalf("durable MCP capability must not be validation feasible, got %v", got)
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
