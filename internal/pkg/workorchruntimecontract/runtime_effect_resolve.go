package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// ResolveExpectedEffectScope derives the concrete sealed effect for an operation.
// Kind-level DefaultEffectScope is the baseline; sealed capability metadata and
// operation parameters may narrow (never expand) it. Core never infers scope from
// business domains, language stacks, or product-specific tool names.
func ResolveExpectedEffectScope(operation RuntimeOperation, capabilities []RuntimeMCPCapabilityContext) RuntimeEffectScope {
	operation = operation.Normalized()
	kind := CanonicalRuntimeBlockKind(operation.Kind)
	descriptor, ok := RuntimeOperationDescriptorForKind(kind)
	if !ok {
		return RuntimeEffectUnknown
	}
	scope := NormalizeRuntimeEffectScope(descriptor.DefaultEffectScope)

	switch kind {
	case StepKindShellCommand:
		return resolveShellExpectedEffectScope(operation, scope)
	case RuntimeBlockKindManagedValidationSession:
		// Managed validation is always non-durable observation.
		return RuntimeEffectNone
	case StepKindSSHCommand:
		return resolveSSHExpectedEffectScope(operation, scope)
	case StepKindMCPToolCall:
		return resolveMCPToolCallExpectedEffectScope(operation, capabilities, scope)
	default:
		return scope
	}
}

// ResolveOperationIdempotencyClass returns the sealed replay contract for a
// concrete operation. Descriptor metadata is authoritative for fixed kinds;
// mcp/tool_call must match the exact enabled server/tool capability because its
// replay semantics are owner supplied rather than inferred from names or risk.
func ResolveOperationIdempotencyClass(operation RuntimeOperation, capabilities []RuntimeMCPCapabilityContext) RuntimeOperationIdempotencyClass {
	operation = operation.Normalized()
	kind := CanonicalRuntimeBlockKind(operation.Kind)
	descriptor, ok := RuntimeOperationDescriptorForKind(kind)
	if !ok {
		return RuntimeOperationIdempotencyUnknown
	}
	if kind != StepKindMCPToolCall {
		return NormalizeRuntimeOperationIdempotencyClass(descriptor.IdempotencyClass)
	}
	serverID := strings.TrimSpace(operationStringParameter(operation.Parameters, "serverId"))
	tool := strings.TrimSpace(operationStringParameter(operation.Parameters, "tool"))
	if serverID == "" || tool == "" {
		return RuntimeOperationIdempotencyUnknown
	}
	for _, raw := range capabilities {
		capability := SealMCPCapabilityEffect(raw)
		if !capability.Enabled || strings.TrimSpace(string(capability.ServerID)) != serverID || strings.TrimSpace(capability.Name) != tool {
			continue
		}
		if CanonicalRuntimeBlockKind(capability.OperationKind) != StepKindMCPToolCall {
			continue
		}
		return NormalizeRuntimeOperationIdempotencyClass(RuntimeOperationIdempotencyClass(capability.IdempotencyClass))
	}
	return RuntimeOperationIdempotencyUnknown
}

func resolveShellExpectedEffectScope(operation RuntimeOperation, defaultScope RuntimeEffectScope) RuntimeEffectScope {
	// Explicit stateChanging=false narrows durable kind default to none.
	// Explicit stateChanging=true keeps durable (workspace).
	// Missing stateChanging: keep kind default (durable) — seal still requires
	// action/effect compatibility, so validation will reject it via durable gate.
	if flag, ok := operationBooleanParameter(operation.Parameters, "stateChanging"); ok {
		if !flag {
			return RuntimeEffectNone
		}
		return RuntimeEffectWorkspaceDurable
	}
	return NormalizeRuntimeEffectScope(defaultScope)
}

func resolveSSHExpectedEffectScope(operation RuntimeOperation, defaultScope RuntimeEffectScope) RuntimeEffectScope {
	if flag, ok := operationBooleanParameter(operation.Parameters, "stateChanging"); ok {
		if !flag {
			return RuntimeEffectNone
		}
		return RuntimeEffectRemoteDurable
	}
	return NormalizeRuntimeEffectScope(defaultScope)
}

func resolveMCPToolCallExpectedEffectScope(operation RuntimeOperation, capabilities []RuntimeMCPCapabilityContext, defaultScope RuntimeEffectScope) RuntimeEffectScope {
	serverID := strings.TrimSpace(operationStringParameter(operation.Parameters, "serverId"))
	tool := strings.TrimSpace(operationStringParameter(operation.Parameters, "tool"))
	if serverID == "" || tool == "" || len(capabilities) == 0 {
		// Unknown / unmatched capability: cannot prove effect.
		return RuntimeEffectUnknown
	}
	for _, capability := range capabilities {
		if !capability.Enabled {
			continue
		}
		if strings.TrimSpace(string(capability.ServerID)) != serverID {
			continue
		}
		// Registry kind may be "tool"; operation kind is mcp/tool_call.
		capKind := strings.TrimSpace(capability.Kind)
		if capKind != "" {
			canonical := CanonicalRuntimeBlockKind(capKind)
			if canonical != "" && canonical != StepKindMCPToolCall && !strings.EqualFold(capKind, "tool") {
				continue
			}
		}
		if strings.TrimSpace(capability.Name) != tool {
			continue
		}
		// Effect is capability-owner metadata sealed during activation. Risk labels and
		// tool names are advisory registry data, not Runtime V2 effect authority.
		// Missing or invalid metadata deliberately remains unknown.
		return NormalizeRuntimeEffectScope(RuntimeEffectScope(capability.EffectScope))
	}
	return RuntimeEffectUnknown
}

func operationBooleanParameter(parameters []OperationParameter, name string) (bool, bool) {
	name = strings.TrimSpace(name)
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter.Name) == name && parameter.Boolean != nil {
			return *parameter.Boolean, true
		}
	}
	return false, false
}

// ValidateOperationEffectAgainstRequired enforces INV-02 for a single operation
// against the sealed RequiredOutput effect contract.
func ValidateOperationEffectAgainstRequired(operation RuntimeOperation, required RequiredOutputContract, capabilities []RuntimeMCPCapabilityContext) (RuntimeEffectScope, error) {
	required = required.Normalized()
	scope := ResolveExpectedEffectScope(operation, capabilities)
	action, ok := runtimeActionForRequiredOutput(required.Kind)
	if !ok {
		return scope, nil
	}
	policy, ok := RuntimeActionEffectPolicyFor(action)
	if !ok {
		return scope, fmt.Errorf("operation %q has no effect policy for action %q", operation.ID, action)
	}
	switch action {
	case RuntimeActionProduceValidation, RuntimeActionProduceStateChange:
		if !policy.AllowsOperationEffect(scope) {
			if action == RuntimeActionProduceStateChange {
				return scope, fmt.Errorf("operation %q expected effect %q is not a durable effect and cannot satisfy %s", operation.ID, scope, action)
			}
			return scope, fmt.Errorf("operation %q expected effect %q cannot satisfy %s", operation.ID, scope, action)
		}
		if len(required.AcceptedEffectScopes) > 0 && !EffectScopeSatisfiesAccepted(scope, required.AcceptedEffectScopes) {
			return scope, fmt.Errorf("operation %q expected effect %q is not in accepted effect scopes", operation.ID, scope)
		}
	case RuntimeActionRequestGovernance:
		if scope != RuntimeEffectGovernanceOnly {
			return scope, fmt.Errorf("operation %q expected effect %q cannot satisfy request_governance", operation.ID, scope)
		}
	}
	return scope, nil
}

// ValidateCandidateEffectCompatibility enforces forward/reverse effect gates before seal.
// - produce_state_change: every operation durable; candidate effects match RequiredEffectScopes
// - produce_validation: every operation non-durable; no durable/unknown
// - unknown effect never accepted
func ValidateCandidateEffectCompatibility(operations []RuntimeOperation, required RequiredOutputContract, capabilities []RuntimeMCPCapabilityContext) error {
	required = required.Normalized()
	if len(operations) == 0 {
		return fmt.Errorf("runtime candidate requires at least one operation")
	}
	action, ok := runtimeActionForRequiredOutput(required.Kind)
	if !ok {
		return nil
	}
	if err := required.Validate(); err != nil {
		return err
	}

	var observed []RuntimeEffectScope
	for _, operation := range operations {
		scope, err := ValidateOperationEffectAgainstRequired(operation, required, capabilities)
		if err != nil {
			return err
		}
		observed = append(observed, scope)
	}

	switch action {
	case RuntimeActionProduceStateChange:
		if !EffectScopeMeetsRequired(observed, required.RequiredEffectScopes) {
			return fmt.Errorf("produce_state_change candidate durable effects %v do not meet required effect scopes %v",
				RuntimeEffectScopesAsStrings(observed), RuntimeEffectScopesAsStrings(required.RequiredEffectScopes))
		}
	case RuntimeActionProduceValidation:
		// Every operation was checked against the same non-durable policy above.
	}
	return nil
}

// OperationEffectCompatibleWithAction is the descriptor-only feasibility probe used
// when no concrete sealed MCP capability inventory is available. It is deliberately
// conservative: dual-use Shell/SSH and MCP operations are not validation-feasible
// until a concrete sealed operation/capability proves a non-durable effect.
func OperationEffectCompatibleWithAction(kind string, action RuntimeAction, capabilities RuntimeCapabilities) bool {
	kind = CanonicalRuntimeBlockKind(kind)
	descriptor, ok := RuntimeOperationDescriptorForKind(kind)
	if !ok {
		return false
	}
	if !runtimeDescriptorAllowsAction(descriptor, action) {
		return false
	}
	policy, ok := RuntimeActionEffectPolicyFor(action)
	return ok && policy.AllowsOperationEffect(descriptor.DefaultEffectScope)
}

// OperationEffectCompatibleWithSealedCapabilities is the activation-aware
// feasibility gate. A MCP kind is feasible only when activation carries an enabled,
// explicit capability effect compatible with the current reducer action. Returning a
// kind does not authorize an arbitrary tool: candidate seal still matches the exact
// server/tool capability and repeats the effect gate.
func OperationEffectCompatibleWithSealedCapabilities(kind string, action RuntimeAction, capabilities RuntimeCapabilities, mcpCapabilities []RuntimeMCPCapabilityContext) bool {
	kind = CanonicalRuntimeBlockKind(kind)
	if kind != StepKindMCPToolCall {
		return OperationEffectCompatibleWithAction(kind, action, capabilities)
	}
	if !OperationEffectCompatibleWithActionDescriptor(kind, action) {
		return false
	}
	policy, ok := RuntimeActionEffectPolicyFor(action)
	if !ok {
		return false
	}
	for _, capability := range mcpCapabilities {
		if !capability.Enabled || CanonicalRuntimeBlockKind(capability.OperationKind) != StepKindMCPToolCall {
			continue
		}
		scope := NormalizeRuntimeEffectScope(RuntimeEffectScope(capability.EffectScope))
		if policy.AllowsOperationEffect(scope) {
			return true
		}
	}
	return false
}

// RuntimeMCPCapabilitiesForAction returns the enabled capability metadata that is
// safe to expose to an action-specific candidate renderer. In particular, a
// validation request never sees durable or unknown mcp/tool_call capabilities;
// a state-change request never sees non-durable tool capabilities. Resource/prompt
// capability kinds retain their descriptor-defined non-durable effect.
func RuntimeMCPCapabilitiesForAction(capabilities []RuntimeMCPCapabilityContext, action RuntimeAction) []RuntimeMCPCapabilityContext {
	policy, ok := RuntimeActionEffectPolicyFor(action)
	if !ok || !policy.ExecutableOperationsAllowed {
		return nil
	}
	ret := make([]RuntimeMCPCapabilityContext, 0, len(capabilities))
	for _, raw := range capabilities {
		capability := SealMCPCapabilityEffect(raw)
		if !capability.Enabled {
			continue
		}
		kind := CanonicalRuntimeBlockKind(capability.OperationKind)
		if kind == "" {
			continue
		}
		descriptor, ok := RuntimeOperationDescriptorForKind(kind)
		if !ok || !runtimeDescriptorAllowsAction(descriptor, action) {
			continue
		}
		scope := NormalizeRuntimeEffectScope(descriptor.DefaultEffectScope)
		if kind == StepKindMCPToolCall {
			scope = NormalizeRuntimeEffectScope(RuntimeEffectScope(capability.EffectScope))
		}
		if !policy.AllowsOperationEffect(scope) {
			continue
		}
		ret = append(ret, capability)
	}
	return ret
}

func OperationEffectCompatibleWithActionDescriptor(kind string, action RuntimeAction) bool {
	descriptor, ok := RuntimeOperationDescriptorForKind(kind)
	return ok && runtimeDescriptorAllowsAction(descriptor, action)
}

// SealMCPCapabilityEffect normalizes capability-owner metadata at activation time.
// Registry Kind=tool maps to mcp/tool_call. EffectScope and IdempotencyClass must be
// supplied by the owner-facing registry contract; Runtime V2 never derives them from
// risk labels or MCP tool names. Missing metadata is explicitly sealed as unknown.
func SealMCPCapabilityEffect(capability RuntimeMCPCapabilityContext) RuntimeMCPCapabilityContext {
	capability.Kind = strings.TrimSpace(capability.Kind)
	capability.Name = strings.TrimSpace(capability.Name)
	capability.RiskLevel = strings.TrimSpace(capability.RiskLevel)
	if strings.TrimSpace(capability.OperationKind) == "" {
		capability.OperationKind = normalizeMCPRegistryKindToOperationKind(capability.Kind)
	} else {
		capability.OperationKind = CanonicalRuntimeBlockKind(capability.OperationKind)
		if _, ok := RuntimeOperationDescriptorForKind(capability.OperationKind); !ok {
			capability.OperationKind = normalizeMCPRegistryKindToOperationKind(capability.Kind)
		}
	}
	capability.EffectScope = string(NormalizeRuntimeEffectScope(RuntimeEffectScope(capability.EffectScope)))
	capability.IdempotencyClass = string(NormalizeRuntimeOperationIdempotencyClass(RuntimeOperationIdempotencyClass(capability.IdempotencyClass)))
	return capability
}
