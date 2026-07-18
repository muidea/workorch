package workorchruntimecontract

import (
	"sort"
	"strings"
)

// RuntimeOperationDescriptor is the single shared technical contract for a concrete operation kind.
// It does not register executors and does not restore a generic RuntimeAdapter registry.
// DefaultEffectScope is the kind-level default; concrete ExpectedEffectScope may still be
// narrowed by sealed capability metadata and operation parameters (shell stateChanging, MCP risk).
type RuntimeOperationDescriptor struct {
	Kind                 string
	OwnerModuleID        string
	DefaultEffectScope   RuntimeEffectScope
	AllowedActions       []RuntimeAction
	CapabilityClass      RuntimeCapabilityClass
	IdempotencyClass     RuntimeOperationIdempotencyClass
	RequiresOwnerReceipt bool
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

// FeasibleOperationKinds returns the stably sorted descriptor-only intersection of
// required accepted kinds and sealed RuntimeCapabilities. It intentionally excludes
// MCP tool calls because their effect is capability-specific and cannot be proven from
// a kind alone. Call FeasibleOperationKindsWithActivation on the Runtime V2 mainline.
func FeasibleOperationKinds(required RequiredOutputContract, capabilities RuntimeCapabilities) []string {
	return feasibleOperationKinds(required, capabilities, nil)
}

// FeasibleOperationKindsWithActivation additionally consumes the activation-sealed
// MCP capability inventory. This is the only feasibility path that may expose
// mcp/tool_call, and only when at least one enabled explicit capability effect is
// compatible with the reducer action.
func FeasibleOperationKindsWithActivation(required RequiredOutputContract, capabilities RuntimeCapabilities, mcpCapabilities []RuntimeMCPCapabilityContext) []string {
	return feasibleOperationKinds(required, capabilities, mcpCapabilities)
}

func feasibleOperationKinds(required RequiredOutputContract, capabilities RuntimeCapabilities, mcpCapabilities []RuntimeMCPCapabilityContext) []string {
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
		// Action + effect feasibility (INV-02). MCP has no kind-level effect; it
		// must be backed by an explicit activation-sealed capability scope.
		if !OperationEffectCompatibleWithSealedCapabilities(canonical, wantAction, capabilities, mcpCapabilities) {
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
	value.DefaultEffectScope = NormalizeRuntimeEffectScope(value.DefaultEffectScope)
	value.CapabilityClass = RuntimeCapabilityClass(strings.TrimSpace(string(value.CapabilityClass)))
	value.IdempotencyClass = NormalizeRuntimeOperationIdempotencyClass(value.IdempotencyClass)
	if len(value.AllowedActions) > 0 {
		value.AllowedActions = append([]RuntimeAction(nil), value.AllowedActions...)
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
			AllowedActions: []RuntimeAction{RuntimeActionProduceValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindFileRead, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedActions: []RuntimeAction{RuntimeActionProduceValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindFileEdit, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			AllowedActions: []RuntimeAction{RuntimeActionProduceStateChange}, CapabilityClass: RuntimeCapabilityClassStateChange,
			RequiresOwnerReceipt: true,
		},
		{
			// Historical durable file mutation kind used by fixtures/capability contracts.
			Kind: StepKindFileWrite, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			AllowedActions: []RuntimeAction{RuntimeActionProduceStateChange}, CapabilityClass: RuntimeCapabilityClassStateChange,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: RuntimeBlockKindPatchApply, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			AllowedActions: []RuntimeAction{RuntimeActionProduceStateChange}, CapabilityClass: RuntimeCapabilityClassStateChange,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: RuntimeBlockKindPatchPreview, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedActions: []RuntimeAction{RuntimeActionProduceValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: RuntimeBlockKindPatchRevert, OwnerModuleID: RuntimeModuleFile,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			AllowedActions: []RuntimeAction{RuntimeActionProduceStateChange}, CapabilityClass: RuntimeCapabilityClassStateChange,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindGitRead, OwnerModuleID: RuntimeModuleGit,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedActions: []RuntimeAction{RuntimeActionProduceValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindGitWrite, OwnerModuleID: RuntimeModuleGit,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			AllowedActions: []RuntimeAction{RuntimeActionProduceStateChange}, CapabilityClass: RuntimeCapabilityClassStateChange,
			RequiresOwnerReceipt: true,
		},
		{
			// shell/command default is durable; concrete ExpectedEffectScope is narrowed by
			// sealed stateChanging parameter at seal time (RVC-R6).
			Kind: StepKindShellCommand, OwnerModuleID: RuntimeModuleShell,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyUnknown,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceStateChange, RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassStateChange, RequiresOwnerReceipt: true,
		},
		{
			// Managed validation is Shell-owned non-mutating validation. Candidate kind stays
			// validation/managed_session; dispatch routes through the Shell EventHub executor.
			Kind: RuntimeBlockKindManagedValidationSession, OwnerModuleID: RuntimeModuleShell,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedActions: []RuntimeAction{RuntimeActionProduceValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindSSHCommand, OwnerModuleID: RuntimeModuleSSH,
			DefaultEffectScope: RuntimeEffectRemoteDurable, IdempotencyClass: RuntimeOperationIdempotencyUnknown,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceStateChange, RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassStateChange, RequiresOwnerReceipt: true,
		},
		{
			// MCP tool_call default is durable/unknown until sealed capability risk narrows it.
			Kind: StepKindMCPToolCall, OwnerModuleID: RuntimeModuleMCP,
			DefaultEffectScope: RuntimeEffectUnknown, IdempotencyClass: RuntimeOperationIdempotencyUnknown,
			AllowedActions:  []RuntimeAction{RuntimeActionProduceStateChange, RuntimeActionProduceValidation},
			CapabilityClass: RuntimeCapabilityClassStateChange, RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindMCPResourceRead, OwnerModuleID: RuntimeModuleMCP,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedActions: []RuntimeAction{RuntimeActionProduceValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindMCPPromptGet, OwnerModuleID: RuntimeModuleMCP,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedActions: []RuntimeAction{RuntimeActionProduceValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindWebSearch, OwnerModuleID: RuntimeModuleWeb,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedActions: []RuntimeAction{RuntimeActionProduceValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			Kind: StepKindWebFetch, OwnerModuleID: RuntimeModuleWeb,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedActions: []RuntimeAction{RuntimeActionProduceValidation}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: true,
		},
		{
			// Governance / assessment kinds are not capability-block operations. They remain
			// in the inventory for verifier completeness and unknown-kind rejection of aliases.
			Kind: RuntimeBlockKindDecisionRequest, OwnerModuleID: RuntimeModuleDecision,
			DefaultEffectScope: RuntimeEffectGovernanceOnly, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedActions: []RuntimeAction{RuntimeActionRequestGovernance}, CapabilityClass: RuntimeCapabilityClassGovernance,
			RequiresOwnerReceipt: false,
		},
		{
			Kind: RuntimeBlockKindApprovalRequest, OwnerModuleID: RuntimeModuleDecision,
			DefaultEffectScope: RuntimeEffectGovernanceOnly, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedActions: []RuntimeAction{RuntimeActionRequestGovernance}, CapabilityClass: RuntimeCapabilityClassGovernance,
			RequiresOwnerReceipt: false,
		},
		{
			Kind: RuntimeBlockKindDoDAssessment, OwnerModuleID: RuntimeModuleOrchestrator,
			DefaultEffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
			AllowedActions: []RuntimeAction{RuntimeActionProduceAssessment}, CapabilityClass: RuntimeCapabilityClassValidation,
			RequiresOwnerReceipt: false,
		},
		{
			Kind: RuntimeBlockKindMemoryWrite, OwnerModuleID: RuntimeModuleOrchestrator,
			DefaultEffectScope: RuntimeEffectWorkspaceDurable, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
			AllowedActions: []RuntimeAction{RuntimeActionProduceStateChange}, CapabilityClass: RuntimeCapabilityClassStateChange,
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
