package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// RuntimeEffectScope is the neutral technical effect of a concrete operation.
// Core never infers scope from business domains or product-specific tool names.
type RuntimeEffectScope string

const (
	RuntimeEffectNone             RuntimeEffectScope = "none"
	RuntimeEffectEphemeralSession RuntimeEffectScope = "ephemeral_session"
	RuntimeEffectWorkspaceDurable RuntimeEffectScope = "workspace_durable"
	RuntimeEffectRemoteDurable    RuntimeEffectScope = "remote_durable"
	RuntimeEffectGovernanceOnly   RuntimeEffectScope = "governance_only"
	RuntimeEffectUnknown          RuntimeEffectScope = "unknown"
)

// RuntimeOperationIdempotencyClass describes whether replaying an ExecutionKey is safe.
type RuntimeOperationIdempotencyClass string

const (
	RuntimeOperationIdempotencySafeToReplay RuntimeOperationIdempotencyClass = "safe_to_replay"
	RuntimeOperationIdempotencyAtMostOnce   RuntimeOperationIdempotencyClass = "at_most_once"
	RuntimeOperationIdempotencyUnknown      RuntimeOperationIdempotencyClass = "unknown"
	RuntimeOperationIdempotencyOwnerManaged RuntimeOperationIdempotencyClass = "owner_managed"
)

// ValidRuntimeEffectScope reports whether scope is a recognized neutral value.
func ValidRuntimeEffectScope(scope RuntimeEffectScope) bool {
	switch RuntimeEffectScope(strings.TrimSpace(string(scope))) {
	case RuntimeEffectNone, RuntimeEffectEphemeralSession, RuntimeEffectWorkspaceDurable,
		RuntimeEffectRemoteDurable, RuntimeEffectGovernanceOnly, RuntimeEffectUnknown:
		return true
	default:
		return false
	}
}

// NormalizeRuntimeEffectScope trims and validates scope; empty becomes unknown.
func NormalizeRuntimeEffectScope(scope RuntimeEffectScope) RuntimeEffectScope {
	scope = RuntimeEffectScope(strings.TrimSpace(string(scope)))
	if scope == "" {
		return RuntimeEffectUnknown
	}
	if !ValidRuntimeEffectScope(scope) {
		return RuntimeEffectUnknown
	}
	return scope
}

// validSealedRuntimeEffectScope reports whether an execution authority carries
// one explicit, recognized and resolved effect. RuntimeEffectUnknown is useful
// while acquiring owner policy, but it cannot survive into a sealed execution
// record or an owner command envelope.
func validSealedRuntimeEffectScope(scope RuntimeEffectScope) bool {
	raw := string(scope)
	normalized := NormalizeRuntimeEffectScope(scope)
	return raw != "" && raw == string(normalized) && normalized != RuntimeEffectUnknown
}

func validSealedRuntimeOperationIdempotencyClass(value RuntimeOperationIdempotencyClass) bool {
	raw := string(value)
	normalized := NormalizeRuntimeOperationIdempotencyClass(value)
	return raw != "" && raw == string(normalized) && normalized != RuntimeOperationIdempotencyUnknown
}

// RuntimeEffectIsDurable reports whether the scope is a durable state-changing effect.
func RuntimeEffectIsDurable(scope RuntimeEffectScope) bool {
	switch NormalizeRuntimeEffectScope(scope) {
	case RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable:
		return true
	default:
		return false
	}
}

// RuntimeEffectIsNonDurable reports whether the scope is none or ephemeral_session.
func RuntimeEffectIsNonDurable(scope RuntimeEffectScope) bool {
	switch NormalizeRuntimeEffectScope(scope) {
	case RuntimeEffectNone, RuntimeEffectEphemeralSession:
		return true
	default:
		return false
	}
}

// RuntimeEffectIsStateChanging reports durable effects that may close state-change requirements.
// ephemeral_session is not durable and cannot close durable state-change.
func RuntimeEffectIsStateChanging(scope RuntimeEffectScope) bool {
	return RuntimeEffectIsDurable(scope)
}

// NormalizeRuntimeEffectScopes trims, drops empties/duplicates, and sorts stably via normalizeStrings.
func NormalizeRuntimeEffectScopes(values []RuntimeEffectScope) []RuntimeEffectScope {
	if len(values) == 0 {
		return nil
	}
	raw := make([]string, 0, len(values))
	for _, value := range values {
		scope := NormalizeRuntimeEffectScope(value)
		if scope == "" {
			continue
		}
		raw = append(raw, string(scope))
	}
	raw = normalizeStrings(raw)
	if len(raw) == 0 {
		return nil
	}
	ret := make([]RuntimeEffectScope, len(raw))
	for idx, value := range raw {
		ret[idx] = RuntimeEffectScope(value)
	}
	return ret
}

// SameRuntimeEffectScopes reports order-sensitive equality after normalization.
func SameRuntimeEffectScopes(left []RuntimeEffectScope, right []RuntimeEffectScope) bool {
	left = NormalizeRuntimeEffectScopes(left)
	right = NormalizeRuntimeEffectScopes(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

// RuntimeEffectScopesAsStrings converts scopes to strings for hashing / projection.
func RuntimeEffectScopesAsStrings(values []RuntimeEffectScope) []string {
	values = NormalizeRuntimeEffectScopes(values)
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, len(values))
	for idx, value := range values {
		ret[idx] = string(value)
	}
	return ret
}

// RuntimeEffectScopesFromStrings parses strings into scopes (unknown values become unknown).
func RuntimeEffectScopesFromStrings(values []string) []RuntimeEffectScope {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeEffectScope, 0, len(values))
	for _, value := range values {
		ret = append(ret, NormalizeRuntimeEffectScope(RuntimeEffectScope(value)))
	}
	return NormalizeRuntimeEffectScopes(ret)
}

// ValidateEffectScopesCompatibleWithState enforces INV-02 convergence-state/effect compatibility.
func ValidateEffectScopesCompatibleWithState(state ConvergenceState, scopes []RuntimeEffectScope) error {
	scopes = NormalizeRuntimeEffectScopes(scopes)
	policy, ok := RuntimeConvergenceEffectPolicyFor(state)
	if !ok {
		return fmt.Errorf("unknown runtime convergence state %q", state)
	}
	if policy.ExecutableOperationsAllowed {
		if state == ConvergenceStateNeedStateChange && len(scopes) == 0 {
			return fmt.Errorf("produce_state_change requires at least one durable effect scope")
		}
		for _, scope := range scopes {
			if !policy.AllowsOperationEffect(scope) {
				return fmt.Errorf("%s forbids effect scope %q", state, scope)
			}
		}
		return nil
	}
	switch state {
	case ConvergenceStateNeedGovernance:
		for _, scope := range scopes {
			if scope != RuntimeEffectGovernanceOnly {
				return fmt.Errorf("request_governance requires governance_only effect, got %q", scope)
			}
		}
		return nil
	default:
		if len(scopes) != 0 {
			return fmt.Errorf("convergence state %s cannot carry operation effect scopes", state)
		}
		return nil
	}
}

// EffectScopeSatisfiesAccepted reports whether observed is in the accepted set.
func EffectScopeSatisfiesAccepted(observed RuntimeEffectScope, accepted []RuntimeEffectScope) bool {
	observed = NormalizeRuntimeEffectScope(observed)
	for _, scope := range NormalizeRuntimeEffectScopes(accepted) {
		if scope == observed {
			return true
		}
	}
	return false
}

// EffectScopeMeetsRequired reports whether at least one required scope is present in observed.
func EffectScopeMeetsRequired(observed []RuntimeEffectScope, required []RuntimeEffectScope) bool {
	required = NormalizeRuntimeEffectScopes(required)
	if len(required) == 0 {
		return true
	}
	set := stringSet(RuntimeEffectScopesAsStrings(observed))
	for _, scope := range required {
		if _, ok := set[string(scope)]; ok {
			return true
		}
	}
	return false
}

func NormalizeRuntimeOperationIdempotencyClass(value RuntimeOperationIdempotencyClass) RuntimeOperationIdempotencyClass {
	value = RuntimeOperationIdempotencyClass(strings.TrimSpace(string(value)))
	switch value {
	case RuntimeOperationIdempotencySafeToReplay, RuntimeOperationIdempotencyAtMostOnce,
		RuntimeOperationIdempotencyUnknown, RuntimeOperationIdempotencyOwnerManaged:
		return value
	case "":
		return RuntimeOperationIdempotencyUnknown
	default:
		return RuntimeOperationIdempotencyUnknown
	}
}
