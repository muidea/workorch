package workorchruntimecontract

import "strings"

// RuntimeMCPCapabilitiesForState is an MCP provider-projection adapter. It
// does not author Runtime effect authority: each capability is filtered by an
// exact subject-bound generic owner policy, with the technical descriptor as
// the conservative fallback.
func RuntimeMCPCapabilitiesForState(capabilities []RuntimeMCPCapabilityContext, policies []RuntimeOperationPolicyFact, state ConvergenceState) []RuntimeMCPCapabilityContext {
	effectPolicy, ok := RuntimeConvergenceEffectPolicyFor(state)
	if !ok || !effectPolicy.ExecutableOperationsAllowed {
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
		if !ok || !runtimeDescriptorAllowsState(descriptor, state) {
			continue
		}
		scope := NormalizeRuntimeEffectScope(descriptor.DefaultEffectScope)
		if ownerPolicy, found := runtimeOperationPolicyForSubject(policies, kind, string(capability.ID)); found {
			scope = ownerPolicy.EffectScope
		}
		if !effectPolicy.AllowsOperationEffect(scope) {
			continue
		}
		ret = append(ret, capability)
	}
	return ret
}

func runtimeOperationPolicyForSubject(policies []RuntimeOperationPolicyFact, kind, subjectRef string) (RuntimeOperationPolicyFact, bool) {
	kind = CanonicalRuntimeBlockKind(kind)
	subjectRef = strings.TrimSpace(subjectRef)
	for _, policy := range NormalizeRuntimeOperationPolicyFacts(policies) {
		if policy.OperationKind == kind && policy.SubjectRef == subjectRef {
			return policy, true
		}
	}
	return RuntimeOperationPolicyFact{}, false
}

// SealMCPCapabilityEffect normalizes owner metadata for provider projection.
// It never derives effect/replay semantics from risk labels or tool names.
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
