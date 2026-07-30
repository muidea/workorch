package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// RuntimeConvergenceEffectPolicy is the sealed, owner-neutral effect contract
// for a canonical convergence state. It is a pure value: consumers may
// validate candidates and receipts with it, but it does not choose a state or
// mutate Runtime state.
type RuntimeConvergenceEffectPolicy struct {
	State                         ConvergenceState
	ExecutableOperationsAllowed   bool
	AllowedOperationEffects       []RuntimeEffectScope
	AllowedAcceptedReceiptEffects []RuntimeEffectScope
}

// RuntimeConvergenceEffectPolicyFor returns the single effect policy consumed by
// RequiredOutput, candidate manifests, provider rendering, seal admission and
// receipt evidence adjudication.
func RuntimeConvergenceEffectPolicyFor(state ConvergenceState) (RuntimeConvergenceEffectPolicy, bool) {
	var policy RuntimeConvergenceEffectPolicy
	switch state {
	case ConvergenceStateNeedStateChange:
		policy = RuntimeConvergenceEffectPolicy{
			State:                         state,
			ExecutableOperationsAllowed:   true,
			AllowedOperationEffects:       []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
			AllowedAcceptedReceiptEffects: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable, RuntimeEffectRemoteDurable},
		}
	case ConvergenceStateNeedValidation:
		policy = RuntimeConvergenceEffectPolicy{
			State:                         state,
			ExecutableOperationsAllowed:   true,
			AllowedOperationEffects:       []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession},
			AllowedAcceptedReceiptEffects: []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession},
		}
	case ConvergenceStateNeedAssessment, ConvergenceStateNeedGovernance,
		ConvergenceStateNeedPrerequisite, ConvergenceStateComplete, ConvergenceStateFailed:
		policy = RuntimeConvergenceEffectPolicy{State: state}
	default:
		return RuntimeConvergenceEffectPolicy{}, false
	}
	return policy.Normalized(), true
}

func (p RuntimeConvergenceEffectPolicy) Normalized() RuntimeConvergenceEffectPolicy {
	p.State = ConvergenceState(strings.TrimSpace(string(p.State)))
	p.AllowedOperationEffects = NormalizeRuntimeEffectScopes(p.AllowedOperationEffects)
	p.AllowedAcceptedReceiptEffects = NormalizeRuntimeEffectScopes(p.AllowedAcceptedReceiptEffects)
	if !p.ExecutableOperationsAllowed {
		p.AllowedOperationEffects = nil
		p.AllowedAcceptedReceiptEffects = nil
	}
	return p
}

func (p RuntimeConvergenceEffectPolicy) Validate() error {
	p = p.Normalized()
	expected, ok := RuntimeConvergenceEffectPolicyFor(p.State)
	if !ok {
		return fmt.Errorf("unknown runtime convergence effect policy %q", p.State)
	}
	if p.ExecutableOperationsAllowed != expected.ExecutableOperationsAllowed ||
		!SameRuntimeEffectScopes(p.AllowedOperationEffects, expected.AllowedOperationEffects) ||
		!SameRuntimeEffectScopes(p.AllowedAcceptedReceiptEffects, expected.AllowedAcceptedReceiptEffects) {
		return fmt.Errorf("runtime convergence effect policy for %q does not match the canonical policy", p.State)
	}
	return nil
}

func (p RuntimeConvergenceEffectPolicy) AllowsOperationEffect(scope RuntimeEffectScope) bool {
	p = p.Normalized()
	return p.ExecutableOperationsAllowed && EffectScopeSatisfiesAccepted(scope, p.AllowedOperationEffects)
}

func (p RuntimeConvergenceEffectPolicy) AllowsAcceptedReceiptEffect(scope RuntimeEffectScope) bool {
	p = p.Normalized()
	return p.ExecutableOperationsAllowed && EffectScopeSatisfiesAccepted(scope, p.AllowedAcceptedReceiptEffects)
}

// RuntimeConvergenceEffectInstruction is the provider-facing statement derived from
// the same sealed policy used at candidate and receipt boundaries.
func RuntimeConvergenceEffectInstruction(state ConvergenceState) string {
	policy, ok := RuntimeConvergenceEffectPolicyFor(state)
	if !ok {
		return ""
	}
	allowed := strings.Join(RuntimeEffectScopesAsStrings(policy.AllowedOperationEffects), " or ")
	switch state {
	case ConvergenceStateNeedStateChange:
		return fmt.Sprintf("Every operation must have a sealed %s effect compatible with requiredEffectScopes; non-durable and unknown operations are forbidden.", allowed)
	case ConvergenceStateNeedValidation:
		return fmt.Sprintf("Every operation must have a sealed %s effect; durable and unknown effects are forbidden.", strings.Replace(allowed, "ephemeral_session", "explicitly accepted ephemeral_session", 1))
	case ConvergenceStateNeedAssessment:
		return "Executable operations are forbidden; assess only current-run accepted evidence."
	case ConvergenceStateNeedGovernance:
		return "Executable operations are forbidden; return only the sealed governance dependency."
	default:
		return ""
	}
}
