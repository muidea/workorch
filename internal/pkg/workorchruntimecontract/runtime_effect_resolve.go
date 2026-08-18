package workorchruntimecontract

import (
	"fmt"
)

// ResolveExpectedEffectScope derives the concrete sealed effect for an
// operation. Kind-level DefaultEffectScope is the baseline; an exact
// capability-owner policy fact may override it.
// Core never branches on an owner, protocol, tool name, risk label, language
// stack, or business domain.
func ResolveExpectedEffectScope(operation RuntimeOperation, policies []RuntimeOperationPolicyFact) RuntimeEffectScope {
	operation = operation.Normalized()
	kind := CanonicalRuntimeBlockKind(operation.Kind)
	descriptor, ok := RuntimeOperationDescriptorForKind(kind)
	if !ok {
		return RuntimeEffectUnknown
	}
	if policy, found := RuntimeOperationPolicyForOperation(operation, policies); found {
		return policy.EffectScope
	}
	return NormalizeRuntimeEffectScope(descriptor.DefaultEffectScope)
}

// ResolveOperationIdempotencyClass returns the sealed replay contract for a
// concrete operation. An exact owner policy is authoritative when present;
// otherwise the technical kind descriptor supplies the conservative default.
func ResolveOperationIdempotencyClass(operation RuntimeOperation, policies []RuntimeOperationPolicyFact) RuntimeOperationIdempotencyClass {
	operation = operation.Normalized()
	kind := CanonicalRuntimeBlockKind(operation.Kind)
	descriptor, ok := RuntimeOperationDescriptorForKind(kind)
	if !ok {
		return RuntimeOperationIdempotencyUnknown
	}
	if policy, found := RuntimeOperationPolicyForOperation(operation, policies); found {
		return policy.IdempotencyClass
	}
	return NormalizeRuntimeOperationIdempotencyClass(descriptor.IdempotencyClass)
}

// ValidateOperationEffectAgainstRequired enforces INV-02 for a single operation
// against the sealed RequiredOutput effect contract.
func ValidateOperationEffectAgainstRequired(state ConvergenceState, operation RuntimeOperation, required RequiredOutputContract, policies []RuntimeOperationPolicyFact) (RuntimeEffectScope, error) {
	required = required.Normalized()
	scope := ResolveExpectedEffectScope(operation, policies)
	if state != ConvergenceStateNeedStateChange && state != ConvergenceStateNeedValidation && state != ConvergenceStateNeedGovernance {
		return scope, nil
	}
	policy, ok := RuntimeConvergenceEffectPolicyFor(state)
	if !ok {
		return scope, fmt.Errorf("operation %q has no effect policy for convergence state %q", operation.ID, state)
	}
	switch state {
	case ConvergenceStateNeedValidation, ConvergenceStateNeedStateChange:
		if !policy.AllowsOperationEffect(scope) {
			if state == ConvergenceStateNeedStateChange {
				return scope, fmt.Errorf("operation %q expected effect %q is not a durable effect and cannot satisfy %s", operation.ID, scope, state)
			}
			return scope, fmt.Errorf("operation %q expected effect %q cannot satisfy %s", operation.ID, scope, state)
		}
		if len(required.AcceptedEffectScopes) > 0 && !EffectScopeSatisfiesAccepted(scope, required.AcceptedEffectScopes) {
			return scope, fmt.Errorf("operation %q expected effect %q is not in accepted effect scopes", operation.ID, scope)
		}
	case ConvergenceStateNeedGovernance:
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
func ValidateCandidateEffectCompatibility(state ConvergenceState, operations []RuntimeOperation, required RequiredOutputContract, policies []RuntimeOperationPolicyFact) error {
	required = required.Normalized()
	if len(operations) == 0 {
		return fmt.Errorf("runtime candidate requires at least one operation")
	}
	if state != ConvergenceStateNeedStateChange && state != ConvergenceStateNeedValidation && state != ConvergenceStateNeedGovernance {
		return nil
	}
	if err := ValidateRequiredOutputForState(state, required); err != nil {
		return err
	}

	var observed []RuntimeEffectScope
	for _, operation := range operations {
		scope, err := ValidateOperationEffectAgainstRequired(state, operation, required, policies)
		if err != nil {
			return err
		}
		observed = append(observed, scope)
	}

	switch state {
	case ConvergenceStateNeedStateChange:
		if !EffectScopeMeetsRequired(observed, required.RequiredEffectScopes) {
			return fmt.Errorf("produce_state_change candidate durable effects %v do not meet required effect scopes %v",
				RuntimeEffectScopesAsStrings(observed), RuntimeEffectScopesAsStrings(required.RequiredEffectScopes))
		}
	case ConvergenceStateNeedValidation:
		// Every operation was checked against the same non-durable policy above.
	}
	return nil
}

// OperationEffectCompatibleWithPolicies is the owner-fact-aware feasibility
// gate. A kind with an unknown or incompatible descriptor default is feasible
// only when at least one current typed policy declares a compatible effect.
// The result never authorizes a concrete operation; seal repeats the exact
// parameter-matched policy check.
func OperationEffectCompatibleWithPolicies(kind string, state ConvergenceState, policies []RuntimeOperationPolicyFact) bool {
	kind = CanonicalRuntimeBlockKind(kind)
	if !OperationEffectCompatibleWithStateDescriptor(kind, state) {
		return false
	}
	effectPolicy, ok := RuntimeConvergenceEffectPolicyFor(state)
	if !ok {
		return false
	}
	if effectPolicy.AllowsOperationEffect(RuntimeOperationDescriptorEffectScope(kind)) {
		return true
	}
	for _, operationPolicy := range NormalizeRuntimeOperationPolicyFacts(policies) {
		if operationPolicy.OperationKind != kind {
			continue
		}
		if effectPolicy.AllowsOperationEffect(operationPolicy.EffectScope) {
			return true
		}
	}
	return false
}

func RuntimeOperationDescriptorEffectScope(kind string) RuntimeEffectScope {
	descriptor, ok := RuntimeOperationDescriptorForKind(kind)
	if !ok {
		return RuntimeEffectUnknown
	}
	return NormalizeRuntimeEffectScope(descriptor.DefaultEffectScope)
}

func OperationEffectCompatibleWithStateDescriptor(kind string, state ConvergenceState) bool {
	descriptor, ok := RuntimeOperationDescriptorForKind(kind)
	return ok && runtimeDescriptorAllowsState(descriptor, state)
}
