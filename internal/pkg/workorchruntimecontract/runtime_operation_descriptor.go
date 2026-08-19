package workorchruntimecontract

import (
	"sort"
	"strings"
)

// RuntimeOperationTargetCardinality is the sole descriptor-level declaration
// of an operation's outer TargetRefs shape. Parameters must not duplicate this
// identity; owner adapters receive the already sealed outer refs.
type RuntimeOperationTargetCardinality string

const (
	RuntimeOperationTargetCardinalityZero RuntimeOperationTargetCardinality = "zero"
	RuntimeOperationTargetCardinalityOne  RuntimeOperationTargetCardinality = "one"
	RuntimeOperationTargetCardinalityMany RuntimeOperationTargetCardinality = "many"
)

// RuntimeOperationDescriptor is the single shared technical contract for a concrete operation kind.
// It does not register executors and does not restore a generic RuntimeAdapter registry.
// DefaultEffectScope is the conservative kind-level default; concrete
// ExpectedEffectScope may be replaced only by a capability-owner policy fact.
type RuntimeOperationDescriptor struct {
	Kind                 string
	OwnerModuleID        string
	DefaultEffectScope   RuntimeEffectScope
	AllowedStates        []ConvergenceState
	CapabilityClass      RuntimeCapabilityClass
	IdempotencyClass     RuntimeOperationIdempotencyClass
	RequiresOwnerReceipt bool
	TargetCardinality    RuntimeOperationTargetCardinality
}

// RuntimeOperationDescriptors returns the complete normalized, stably sorted descriptor inventory.
// Callers must not mutate the returned slice or its nested AllowedStates.
func RuntimeOperationDescriptors() []RuntimeOperationDescriptor {
	return cloneRuntimeOperationDescriptors(runtimeOperationDescriptorInventory)
}

// RuntimeOperationDescriptorForKind returns the descriptor for a canonical operation kind.
// Unknown kinds return ok=false and must be treated as non-executable with unknown effect.
func RuntimeOperationDescriptorForKind(kind string) (RuntimeOperationDescriptor, bool) {
	canonical := CanonicalRuntimeBlockKind(kind)
	if canonical == "" {
		return RuntimeOperationDescriptor{}, false
	}
	descriptor, ok := runtimeOperationDescriptorByKind[canonical]
	if !ok {
		return RuntimeOperationDescriptor{}, false
	}
	return cloneRuntimeOperationDescriptor(descriptor), true
}

// FeasibleOperationKinds returns the stably sorted descriptor-only intersection of
// required accepted kinds and sealed RuntimeCapabilities. It intentionally excludes
// dynamic-effect kinds because their effect cannot be proven from a kind alone.
// Call FeasibleOperationKindsWithActivation on the Runtime mainline.
func FeasibleOperationKinds(state ConvergenceState, required RequiredOutputContract, capabilities RuntimeCapabilities) []string {
	return feasibleOperationKinds(state, required, capabilities, nil)
}

// FeasibleOperationKindsWithActivation additionally consumes activation- or
// catalog-sealed owner operation policies. A dynamic-effect kind is exposed
// only when one typed fact is compatible with the reducer action.
func FeasibleOperationKindsWithActivation(state ConvergenceState, required RequiredOutputContract, capabilities RuntimeCapabilities, operationPolicies []RuntimeOperationPolicyFact) []string {
	return feasibleOperationKinds(state, required, capabilities, operationPolicies)
}

// FeasibleOperationKindsWithOwnerValidationRecipes further narrows the
// descriptor/capability intersection by the current Shell-owner-confirmed
// managed validation recipes. It never runs a command or selects an action;
// it prevents an empty recipe selection set from being presented as an
// executable candidate kind.
func FeasibleOperationKindsWithOwnerValidationRecipes(state ConvergenceState, required RequiredOutputContract, capabilities RuntimeCapabilities, operationPolicies []RuntimeOperationPolicyFact, recipes []RuntimeValidationRecipe) []string {
	ret := FeasibleOperationKindsWithActivation(state, required, capabilities, operationPolicies)
	if state != ConvergenceStateNeedValidation {
		return ret
	}
	if len(NormalizeRuntimeValidationRecipes(recipes)) != 0 {
		return ret
	}
	filtered := make([]string, 0, len(ret))
	for _, kind := range ret {
		if CanonicalRuntimeBlockKind(kind) != RuntimeBlockKindManagedValidationSession {
			filtered = append(filtered, kind)
		}
	}
	return nilIfEmpty(filtered)
}

func feasibleOperationKinds(state ConvergenceState, required RequiredOutputContract, capabilities RuntimeCapabilities, operationPolicies []RuntimeOperationPolicyFact) []string {
	required = required.Normalized()
	capabilities = RuntimeCapabilities{
		StateChangeKinds: canonicalizeOperationKinds(capabilities.StateChangeKinds),
		ValidationKinds:  canonicalizeOperationKinds(capabilities.ValidationKinds),
		GovernanceKinds:  canonicalizeOperationKinds(capabilities.GovernanceKinds),
	}
	accepted := required.AcceptedKinds
	if len(accepted) == 0 {
		return nil
	}
	capabilitySet := stringSet(capabilityKindsForConvergenceState(state, capabilities))
	if len(capabilitySet) == 0 {
		return nil
	}
	if state != ConvergenceStateNeedStateChange && state != ConvergenceStateNeedValidation {
		return nil
	}
	ret := make([]string, 0, len(accepted))
	seen := make(map[string]struct{}, len(accepted))
	for _, kind := range accepted {
		canonical := CanonicalRuntimeBlockKind(kind)
		if canonical == "" {
			continue
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		if _, ok := capabilitySet[canonical]; !ok {
			continue
		}
		descriptor, ok := RuntimeOperationDescriptorForKind(canonical)
		if !ok {
			continue
		}
		if !runtimeDescriptorAllowsState(descriptor, state) {
			continue
		}
		// Action + effect feasibility (INV-02). A kind whose descriptor cannot
		// prove the effect must be backed by an explicit owner policy fact.
		if !OperationEffectCompatibleWithPolicies(canonical, state, operationPolicies) {
			continue
		}
		seen[canonical] = struct{}{}
		ret = append(ret, canonical)
	}
	sort.Strings(ret)
	return ret
}

func canonicalizeOperationKinds(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		canonical := CanonicalRuntimeBlockKind(value)
		if canonical == "" {
			continue
		}
		ret = append(ret, canonical)
	}
	return normalizeStrings(ret)
}

func capabilityKindsForConvergenceState(state ConvergenceState, capabilities RuntimeCapabilities) []string {
	switch state {
	case ConvergenceStateNeedStateChange:
		return capabilities.StateChangeKinds
	case ConvergenceStateNeedValidation:
		return capabilities.ValidationKinds
	case ConvergenceStateNeedAssessment:
		// Assessment is LLM-produced; no capability-block operation kinds.
		return nil
	default:
		return nil
	}
}

func runtimeDescriptorAllowsState(descriptor RuntimeOperationDescriptor, state ConvergenceState) bool {
	for _, allowed := range descriptor.AllowedStates {
		if allowed == state {
			return true
		}
	}
	return false
}

func cloneRuntimeOperationDescriptors(values []RuntimeOperationDescriptor) []RuntimeOperationDescriptor {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeOperationDescriptor, len(values))
	for idx, value := range values {
		ret[idx] = cloneRuntimeOperationDescriptor(value)
	}
	return ret
}

func cloneRuntimeOperationDescriptor(value RuntimeOperationDescriptor) RuntimeOperationDescriptor {
	value.Kind = strings.TrimSpace(value.Kind)
	value.OwnerModuleID = strings.TrimSpace(value.OwnerModuleID)
	value.DefaultEffectScope = NormalizeRuntimeEffectScope(value.DefaultEffectScope)
	value.CapabilityClass = RuntimeCapabilityClass(strings.TrimSpace(string(value.CapabilityClass)))
	value.IdempotencyClass = NormalizeRuntimeOperationIdempotencyClass(value.IdempotencyClass)
	value.TargetCardinality = RuntimeOperationTargetCardinality(strings.TrimSpace(string(value.TargetCardinality)))
	if len(value.AllowedStates) > 0 {
		value.AllowedStates = append([]ConvergenceState(nil), value.AllowedStates...)
	}
	return value
}

var (
	runtimeOperationDescriptorInventory []RuntimeOperationDescriptor
	runtimeOperationDescriptorByKind    map[string]RuntimeOperationDescriptor
)

func init() {
	// RequiresOwnerReceipt=true for all capability-block operations that perform side effects
	// or observations that commit evidence (RVC-R5 type gate; production path still optional).
	inventory := []RuntimeOperationDescriptor{
		{
			Kind: StepKindFileList, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindFileRead, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true, TargetCardinality: RuntimeOperationTargetCardinalityOne,
		},
		{
			Kind: StepKindFileEdit, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedStateChange}, CapabilityClass: RuntimeCapabilityClassStateChange,
			RequiresOwnerReceipt: true, TargetCardinality: RuntimeOperationTargetCardinalityOne,
		},
		{
			// Historical durable file mutation kind used by fixtures/capability contracts.
			Kind: StepKindFileWrite, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedStateChange}, CapabilityClass: RuntimeCapabilityClassStateChange,
			RequiresOwnerReceipt: true, TargetCardinality: RuntimeOperationTargetCardinalityOne,
		},
		{
			Kind: RuntimeBlockKindPatchApply, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedStateChange}, CapabilityClass: RuntimeCapabilityClassStateChange,
			RequiresOwnerReceipt: true, TargetCardinality: RuntimeOperationTargetCardinalityOne,
		},
		{
			Kind: RuntimeBlockKindPatchPreview, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true, TargetCardinality: RuntimeOperationTargetCardinalityOne,
		},
		{
			Kind: RuntimeBlockKindPatchRevert, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedStateChange}, CapabilityClass: RuntimeCapabilityClassStateChange,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindGitRead, OwnerModuleID: RuntimeModuleGit,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindGitWrite, OwnerModuleID: RuntimeModuleGit,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedStateChange}, CapabilityClass: RuntimeCapabilityClassStateChange,
			RequiresOwnerReceipt: true,
		},
		{
			// The conservative kind default is durable. The Shell owner may publish
			// a parameter-matched non-durable policy in its current catalog.
			Kind: StepKindShellCommand, OwnerModuleID: RuntimeModuleShell,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyUnknown,
			AllowedStates:   []ConvergenceState{ConvergenceStateNeedStateChange, ConvergenceStateNeedValidation},
			CapabilityClass: RuntimeCapabilityClassStateChange, RequiresOwnerReceipt: true,
			TargetCardinality: RuntimeOperationTargetCardinalityMany,
		},
		{
			// Managed validation is Shell-owned non-mutating validation. Candidate kind stays
			// validation/managed_session; dispatch routes through the Shell EventHub executor.
			Kind: RuntimeBlockKindManagedValidationSession, OwnerModuleID: RuntimeModuleShell,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindSSHCommand, OwnerModuleID: RuntimeModuleSSH,
			DefaultEffectScope: RuntimeEffectRemoteDurable, IdempotencyClass: RuntimeOperationIdempotencyUnknown,
			AllowedStates:   []ConvergenceState{ConvergenceStateNeedStateChange, ConvergenceStateNeedValidation},
			CapabilityClass: RuntimeCapabilityClassStateChange, RequiresOwnerReceipt: true,
		},
		{
			// Dynamic tool calls remain unknown until an owner policy fact
			// supplies the exact parameter-bound effect/replay contract.
			Kind: StepKindMCPToolCall, OwnerModuleID: RuntimeModuleMCP,
			DefaultEffectScope: RuntimeEffectUnknown, IdempotencyClass: RuntimeOperationIdempotencyUnknown,
			AllowedStates:   []ConvergenceState{ConvergenceStateNeedStateChange, ConvergenceStateNeedValidation},
			CapabilityClass: RuntimeCapabilityClassStateChange, RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindMCPResourceRead, OwnerModuleID: RuntimeModuleMCP,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindMCPPromptGet, OwnerModuleID: RuntimeModuleMCP,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindWebSearch, OwnerModuleID: RuntimeModuleWeb,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindWebFetch, OwnerModuleID: RuntimeModuleWeb,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			// Governance / assessment kinds are not capability-block operations. They remain
			// in the inventory for verifier completeness and unknown-kind rejection of aliases.
			Kind: RuntimeBlockKindDecisionRequest, OwnerModuleID: RuntimeModuleDecision,
			DefaultEffectScope: RuntimeEffectGovernanceOnly, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedGovernance}, CapabilityClass: RuntimeCapabilityClassGovernance,
			RequiresOwnerReceipt: false,
		},
		{
			Kind: RuntimeBlockKindApprovalRequest, OwnerModuleID: RuntimeModuleApproval,
			DefaultEffectScope: RuntimeEffectGovernanceOnly, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedGovernance}, CapabilityClass: RuntimeCapabilityClassGovernance,
			RequiresOwnerReceipt: false,
		},
		{
			Kind: RuntimeBlockKindDoDAssessment, OwnerModuleID: RuntimeModuleOrchestrator,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedAssessment}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: false,
		},
		{
			Kind: RuntimeBlockKindMemoryWrite, OwnerModuleID: RuntimeModuleOrchestrator,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			AllowedStates: []ConvergenceState{ConvergenceStateNeedStateChange}, CapabilityClass: RuntimeCapabilityClassStateChange,
			RequiresOwnerReceipt: true,
		},
	}
	sort.SliceStable(inventory, func(i, j int) bool {
		return inventory[i].Kind < inventory[j].Kind
	})
	byKind := make(map[string]RuntimeOperationDescriptor, len(inventory))
	for _, descriptor := range inventory {
		if _, exists := byKind[descriptor.Kind]; exists {
			panic("duplicate runtime operation descriptor kind: " + descriptor.Kind)
		}
		byKind[descriptor.Kind] = cloneRuntimeOperationDescriptor(descriptor)
	}
	runtimeOperationDescriptorInventory = inventory
	runtimeOperationDescriptorByKind = byKind
}
