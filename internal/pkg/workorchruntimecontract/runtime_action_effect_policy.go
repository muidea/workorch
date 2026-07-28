package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// RuntimeActionEffectPolicy is the sealed, owner-neutral effect contract for a
// Runtime action. It is a pure value: consumers may validate candidates and
// receipts with it, but it does not choose an action or mutate Runtime state.
type RuntimeActionEffectPolicy struct {
	Action                        RuntimeAction
	ExecutableOperationsAllowed   bool
	AllowedOperationEffects       []RuntimeEffectScope
	AllowedAcceptedReceiptEffects []RuntimeEffectScope
}

// RuntimeActionEffectPolicyFor returns the single effect policy consumed by
// RequiredOutput, candidate manifests, provider rendering, seal admission and
// receipt evidence adjudication.
func RuntimeActionEffectPolicyFor(action RuntimeAction) (RuntimeActionEffectPolicy, bool) {
	var policy RuntimeActionEffectPolicy
	switch action {
	case RuntimeActionProduceStateChange:
		policy = RuntimeActionEffectPolicy{
			Action:                        action,
			ExecutableOperationsAllowed:   true,
			AllowedOperationEffects:       []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
			AllowedAcceptedReceiptEffects: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		}
	case RuntimeActionProduceValidation:
		policy = RuntimeActionEffectPolicy{
			Action:                        action,
			ExecutableOperationsAllowed:   true,
			AllowedOperationEffects:       []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession},
			AllowedAcceptedReceiptEffects: []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession},
		}
	case RuntimeActionProduceAssessment, RuntimeActionRequestGovernance,
		RuntimeActionAcquirePrerequisite, RuntimeActionComplete, RuntimeActionFail:
		policy = RuntimeActionEffectPolicy{Action: action}
	default:
		return RuntimeActionEffectPolicy{}, false
	}
	return policy.Normalized(), true
}

func (p RuntimeActionEffectPolicy) Normalized() RuntimeActionEffectPolicy {
	p.Action = RuntimeAction(strings.TrimSpace(string(p.Action)))
	p.AllowedOperationEffects = NormalizeRuntimeEffectScopes(p.AllowedOperationEffects)
	p.AllowedAcceptedReceiptEffects = NormalizeRuntimeEffectScopes(p.AllowedAcceptedReceiptEffects)
	if !p.ExecutableOperationsAllowed {
		p.AllowedOperationEffects = nil
		p.AllowedAcceptedReceiptEffects = nil
	}
	return p
}

func (p RuntimeActionEffectPolicy) Validate() error {
	p = p.Normalized()
	expected, ok := RuntimeActionEffectPolicyFor(p.Action)
	if !ok {
		return fmt.Errorf("unknown runtime action effect policy %q", p.Action)
	}
	if p.ExecutableOperationsAllowed != expected.ExecutableOperationsAllowed ||
		!SameRuntimeEffectScopes(p.AllowedOperationEffects, expected.AllowedOperationEffects) ||
		!SameRuntimeEffectScopes(p.AllowedAcceptedReceiptEffects, expected.AllowedAcceptedReceiptEffects) {
		return fmt.Errorf("runtime action effect policy for %q does not match the canonical policy", p.Action)
	}
	return nil
}

func (p RuntimeActionEffectPolicy) AllowsOperationEffect(scope RuntimeEffectScope) bool {
	p = p.Normalized()
	return p.ExecutableOperationsAllowed && EffectScopeSatisfiesAccepted(scope, p.AllowedOperationEffects)
}

func (p RuntimeActionEffectPolicy) AllowsAcceptedReceiptEffect(scope RuntimeEffectScope) bool {
	p = p.Normalized()
	return p.ExecutableOperationsAllowed && EffectScopeSatisfiesAccepted(scope, p.AllowedAcceptedReceiptEffects)
}

// RuntimeActionEffectInstruction is the provider-facing statement derived from
// the same sealed policy used at candidate and receipt boundaries.
func RuntimeActionEffectInstruction(action RuntimeAction) string {
	policy, ok := RuntimeActionEffectPolicyFor(action)
	if !ok {
		return ""
	}
	allowed := strings.Join(RuntimeEffectScopesAsStrings(policy.AllowedOperationEffects), " or ")
	switch action {
	case RuntimeActionProduceStateChange:
		return fmt.Sprintf("Every operation must have a sealed %s effect compatible with requiredEffectScopes; non-durable and unknown operations are forbidden.", allowed)
	case RuntimeActionProduceValidation:
		return fmt.Sprintf("Every operation must have a sealed %s effect; durable and unknown effects are forbidden.", strings.Replace(allowed, "ephemeral_session", "explicitly accepted ephemeral_session", 1))
	case RuntimeActionProduceAssessment:
		return "Executable operations are forbidden; assess only current-run accepted evidence."
	case RuntimeActionRequestGovernance:
		return "Executable operations are forbidden; return only the sealed governance dependency."
	default:
		return ""
	}
}
