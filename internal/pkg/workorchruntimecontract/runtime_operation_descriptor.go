package workorchruntimecontract

import (
	"sort"
	"strings"
)

// RuntimeOperationEffectClass classifies the neutral technical effect of an operation kind.
// Descriptor effect is kind-level; capability-sealed overrides (e.g. MCP read_only) may narrow
// concrete side-effect decisions but never expand a non_mutating descriptor into mutating.
type RuntimeOperationEffectClass string

const (
	RuntimeOperationEffectNonMutating    RuntimeOperationEffectClass = "non_mutating"
	RuntimeOperationEffectWorkspaceState RuntimeOperationEffectClass = "workspace_state"
	RuntimeOperationEffectRemoteState    RuntimeOperationEffectClass = "remote_state"
	RuntimeOperationEffectUnknown        RuntimeOperationEffectClass = "unknown"
)

// RuntimeOperationDescriptor is the single shared technical contract for a concrete operation kind.
// It does not register executors and does not restore a generic RuntimeAdapter registry.
type RuntimeOperationDescriptor struct {
	Kind            string
	OwnerModuleID   string
	EffectClass     RuntimeOperationEffectClass
	AllowedActions  []RuntimeAction
	CapabilityClass RuntimeCapabilityClass
}

// RuntimeOperationDescriptors returns the complete normalized, stably sorted descriptor inventory.
// Callers must not mutate the returned slice or its nested AllowedActions.
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

// FeasibleOperationKinds returns the stably sorted intersection of required accepted kinds,
// sealed RuntimeCapabilities, and descriptor action/capability compatibility.
// Unknown or non-executable kinds are excluded. MCP tool calls may remain feasible at kind
// level when sealed into ValidationKinds; concrete side-effect safety still uses sealed MCP
// capability metadata at candidate validation time.
func FeasibleOperationKinds(required RequiredOutputContract, capabilities RuntimeCapabilities) []string {
	required = required.Normalized()
	capabilities = RuntimeCapabilities{
		StateChangeKinds: normalizeStrings(capabilities.StateChangeKinds),
		ValidationKinds:  normalizeStrings(capabilities.ValidationKinds),
		GovernanceKinds:  normalizeStrings(capabilities.GovernanceKinds),
	}
	accepted := required.AcceptedKinds
	if len(accepted) == 0 {
		return nil
	}
	capabilitySet := stringSet(capabilityKindsForRequiredOutput(required.Kind, capabilities))
	if len(capabilitySet) == 0 {
		return nil
	}
	wantAction, ok := runtimeActionForRequiredOutput(required.Kind)
	if !ok {
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
		if !runtimeDescriptorAllowsAction(descriptor, wantAction) {
			continue
		}
		if required.Kind == RequiredOutputKindValidation {
			// Kind-level feasibility only admits non-mutating descriptors, plus MCP tool
			// calls which may be narrowed by sealed read_only capability metadata later.
			if descriptor.EffectClass != RuntimeOperationEffectNonMutating && canonical != StepKindMCPToolCall {
				continue
			}
		}
		seen[canonical] = struct{}{}
		ret = append(ret, canonical)
	}
	sort.Strings(ret)
	return ret
}

func capabilityKindsForRequiredOutput(kind RequiredOutputKind, capabilities RuntimeCapabilities) []string {
	switch kind {
	case RequiredOutputKindStateChange:
		return capabilities.StateChangeKinds
	case RequiredOutputKindValidation:
		return capabilities.ValidationKinds
	case RequiredOutputKindAssessment:
		// Assessment is LLM-produced; no capability-block operation kinds.
		return nil
	default:
		return nil
	}
}

func runtimeActionForRequiredOutput(kind RequiredOutputKind) (RuntimeAction, bool) {
	switch kind {
	case RequiredOutputKindStateChange:
		return RuntimeActionProduceStateChange, true
	case RequiredOutputKindValidation:
		return RuntimeActionProduceValidation, true
	case RequiredOutputKindAssessment:
		return RuntimeActionProduceAssessment, true
	default:
		return "", false
	}
}

func runtimeDescriptorAllowsAction(descriptor RuntimeOperationDescriptor, action RuntimeAction) bool {
	for _, allowed := range descriptor.AllowedActions {
		if allowed == action {
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
	value.EffectClass = RuntimeOperationEffectClass(strings.TrimSpace(string(value.EffectClass)))
	value.CapabilityClass = RuntimeCapabilityClass(strings.TrimSpace(string(value.CapabilityClass)))
	if len(value.AllowedActions) > 0 {
		value.AllowedActions = append([]RuntimeAction(nil), value.AllowedActions...)
	}
	return value
}

func operationEffectHasWorkspaceSideEffects(effect RuntimeOperationEffectClass) bool {
	switch effect {
	case RuntimeOperationEffectNonMutating:
		return false
	case RuntimeOperationEffectWorkspaceState, RuntimeOperationEffectRemoteState, RuntimeOperationEffectUnknown:
		return true
	default:
		return true
	}
}

var (
	runtimeOperationDescriptorInventory []RuntimeOperationDescriptor
	runtimeOperationDescriptorByKind    map[string]RuntimeOperationDescriptor
)

func init() {
	inventory := []RuntimeOperationDescriptor{
		{
			Kind: StepKindFileList, OwnerModuleID: RuntimeModuleFile,
			EffectClass:     RuntimeOperationEffectNonMutating,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassValidation,
		},
		{
			Kind: StepKindFileRead, OwnerModuleID: RuntimeModuleFile,
			EffectClass:     RuntimeOperationEffectNonMutating,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassValidation,
		},
		{
			Kind: StepKindFileEdit, OwnerModuleID: RuntimeModuleFile,
			EffectClass:     RuntimeOperationEffectWorkspaceState,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceStateChange},
			CapabilityClass: RuntimeCapabilityClassStateChange,
		},
		{
			Kind: RuntimeBlockKindPatchApply, OwnerModuleID: RuntimeModuleFile,
			EffectClass:     RuntimeOperationEffectWorkspaceState,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceStateChange},
			CapabilityClass: RuntimeCapabilityClassStateChange,
		},
		{
			Kind: RuntimeBlockKindPatchPreview, OwnerModuleID: RuntimeModuleFile,
			EffectClass:     RuntimeOperationEffectNonMutating,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassValidation,
		},
		{
			Kind: RuntimeBlockKindPatchRevert, OwnerModuleID: RuntimeModuleFile,
			EffectClass:     RuntimeOperationEffectWorkspaceState,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceStateChange},
			CapabilityClass: RuntimeCapabilityClassStateChange,
		},
		{
			Kind: StepKindGitRead, OwnerModuleID: RuntimeModuleGit,
			EffectClass:     RuntimeOperationEffectNonMutating,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassValidation,
		},
		{
			Kind: StepKindGitWrite, OwnerModuleID: RuntimeModuleGit,
			EffectClass:     RuntimeOperationEffectWorkspaceState,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceStateChange},
			CapabilityClass: RuntimeCapabilityClassStateChange,
		},
		{
			Kind: StepKindShellCommand, OwnerModuleID: RuntimeModuleShell,
			EffectClass:     RuntimeOperationEffectWorkspaceState,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceStateChange, RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassStateChange,
		},
		{
			// Managed validation is Shell-owned non-mutating validation. Candidate kind stays
			// validation/managed_session; dispatch routes through the Shell EventHub executor.
			Kind: RuntimeBlockKindManagedValidationSession, OwnerModuleID: RuntimeModuleShell,
			EffectClass:     RuntimeOperationEffectNonMutating,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassValidation,
		},
		{
			Kind: StepKindSSHCommand, OwnerModuleID: RuntimeModuleSSH,
			EffectClass:     RuntimeOperationEffectRemoteState,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceStateChange, RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassStateChange,
		},
		{
			Kind: StepKindMCPToolCall, OwnerModuleID: RuntimeModuleMCP,
			EffectClass:     RuntimeOperationEffectWorkspaceState,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceStateChange, RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassStateChange,
		},
		{
			Kind: StepKindMCPResourceRead, OwnerModuleID: RuntimeModuleMCP,
			EffectClass:     RuntimeOperationEffectNonMutating,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassValidation,
		},
		{
			Kind: StepKindMCPPromptGet, OwnerModuleID: RuntimeModuleMCP,
			EffectClass:     RuntimeOperationEffectNonMutating,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassValidation,
		},
		{
			Kind: StepKindWebSearch, OwnerModuleID: RuntimeModuleWeb,
			EffectClass:     RuntimeOperationEffectNonMutating,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassValidation,
		},
		{
			Kind: StepKindWebFetch, OwnerModuleID: RuntimeModuleWeb,
			EffectClass:     RuntimeOperationEffectNonMutating,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassValidation,
		},
		{
			// Governance / assessment kinds are not capability-block operations. They remain
			// in the inventory for verifier completeness and unknown-kind rejection of aliases.
			Kind: RuntimeBlockKindDecisionRequest, OwnerModuleID: RuntimeModuleDecision,
			EffectClass:     RuntimeOperationEffectNonMutating,
			AllowedActions:  []RuntimeAction{RuntimeActionRequestGovernance},
			CapabilityClass: RuntimeCapabilityClassGovernance,
		},
		{
			Kind: RuntimeBlockKindApprovalRequest, OwnerModuleID: RuntimeModuleDecision,
			EffectClass:     RuntimeOperationEffectNonMutating,
			AllowedActions:  []RuntimeAction{RuntimeActionRequestGovernance},
			CapabilityClass: RuntimeCapabilityClassGovernance,
		},
		{
			Kind: RuntimeBlockKindDoDAssessment, OwnerModuleID: RuntimeModuleOrchestrator,
			EffectClass:     RuntimeOperationEffectNonMutating,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceAssessment},
			CapabilityClass: RuntimeCapabilityClassValidation,
		},
		{
			Kind: RuntimeBlockKindMemoryWrite, OwnerModuleID: RuntimeModuleOrchestrator,
			EffectClass:     RuntimeOperationEffectWorkspaceState,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceStateChange},
			CapabilityClass: RuntimeCapabilityClassStateChange,
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
