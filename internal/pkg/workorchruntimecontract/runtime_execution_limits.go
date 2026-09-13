package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const RuntimeExecutionLimitsV1 = "runtime-execution-limits/v1"

// RuntimeExecutionLimits narrows physical budgets and optionally bounds proof
// batches and durable dispatch seals per original CWI. It authorizes no work
// and creates no second ledger.
type RuntimeExecutionLimits struct {
	ProtocolRef             string
	MaxProviderCalls        int
	MaxCapabilityOperations int
	// Zero means no additional per-CWI bound; all existing budgets still apply.
	MaxProofBatchesPerCWI int `json:",omitempty"`
	// Counts original durable dispatch seals, including failed/invalidated work.
	// Governance continuation of an existing seal does not reserve again.
	MaxDurableDispatchesPerCWI int `json:",omitempty"`
}

func (l RuntimeExecutionLimits) Validate() error {
	if l.ProtocolRef != RuntimeExecutionLimitsV1 {
		return fmt.Errorf("runtime execution limits require the current protocol")
	}
	baseline := DefaultRuntimeExecutionBudgetPolicy()
	operations, _ := RuntimeBudgetDimensionPolicyFor(baseline, RuntimeBudgetDimensionCapabilityOperation)
	if l.MaxDurableDispatchesPerCWI < 0 || int64(l.MaxDurableDispatchesPerCWI) > operations.Limit {
		return fmt.Errorf("runtime durable dispatch limit must be in 1..%d when specified", operations.Limit)
	}
	if l.MaxProofBatchesPerCWI < 0 || l.MaxProofBatchesPerCWI > RuntimeSemanticPlanningHistoryLimit+1 {
		return fmt.Errorf("runtime proof batch limit must be in 1..%d when specified", RuntimeSemanticPlanningHistoryLimit+1)
	}
	for _, bound := range l.dimensions() {
		policy, found := RuntimeBudgetDimensionPolicyFor(baseline, bound.dimension)
		if !found || bound.limit < 1 || bound.limit > policy.Limit {
			return fmt.Errorf("runtime execution limit %q must be in 1..%d", bound.dimension, policy.Limit)
		}
	}
	return nil
}

func (l RuntimeExecutionLimits) dimensions() []struct {
	dimension RuntimeBudgetDimension
	limit     int64
} {
	return []struct {
		dimension RuntimeBudgetDimension
		limit     int64
	}{
		{RuntimeBudgetDimensionProviderCall, int64(l.MaxProviderCalls)},
		{RuntimeBudgetDimensionCapabilityOperation, int64(l.MaxCapabilityOperations)},
	}
}

func SameRuntimeExecutionLimits(left, right *RuntimeExecutionLimits) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func StableRuntimeExecutionLimitsHash(limits RuntimeExecutionLimits) string {
	h := sha256.New()
	writeHashString(h, limits.ProtocolRef)
	writeHashString(h, fmt.Sprint(limits.MaxProviderCalls))
	writeHashString(h, fmt.Sprint(limits.MaxCapabilityOperations))
	writeHashString(h, fmt.Sprint(limits.MaxProofBatchesPerCWI))
	writeHashString(h, fmt.Sprint(limits.MaxDurableDispatchesPerCWI))
	return "runtime-execution-limits-" + hex.EncodeToString(h.Sum(nil))
}

// RuntimeExecutionBudgetPolicyWithLimits compiles a sealed declaration into
// the existing budget policy. Consumers continue reserving in that one ledger.
func RuntimeExecutionBudgetPolicyWithLimits(convergence RuntimeConvergencePolicy, recovery RuntimeRecoveryPolicy, limits *RuntimeExecutionLimits) (RuntimeExecutionBudgetPolicy, error) {
	policy := RuntimeExecutionBudgetPolicyForRuntimePolicies(convergence, recovery)
	if limits == nil {
		return policy, nil
	}
	if err := limits.Validate(); err != nil {
		return RuntimeExecutionBudgetPolicy{}, err
	}
	for _, bound := range limits.dimensions() {
		for idx := range policy.Dimensions {
			if policy.Dimensions[idx].Dimension == bound.dimension {
				policy.Dimensions[idx].Limit = bound.limit
			}
		}
	}
	policy.PolicyHash = StableRuntimeExecutionBudgetPolicyHash(policy)
	return policy, policy.Validate()
}

// ValidateRuntimeExecutionLimitsBinding is checked at bootstrap and every
// facts reload. Rehashing a different policy cannot bypass the startup limits.
func ValidateRuntimeExecutionLimitsBinding(policy RuntimeExecutionBudgetPolicy, limits *RuntimeExecutionLimits) error {
	if limits == nil {
		return nil
	}
	if err := limits.Validate(); err != nil {
		return err
	}
	if policy.Scope != RuntimeBudgetScopeRun || policy.RestartDisposition != RuntimeBudgetRestartFreshFromSuccessorPolicy {
		return fmt.Errorf("runtime execution limits require Run-scoped successor policy")
	}
	baseline := DefaultRuntimeExecutionBudgetPolicy()
	for _, bound := range limits.dimensions() {
		actual, found := RuntimeBudgetDimensionPolicyFor(policy, bound.dimension)
		expected, _ := RuntimeBudgetDimensionPolicyFor(baseline, bound.dimension)
		expected.Limit = bound.limit
		if !found || actual != expected {
			return fmt.Errorf("runtime execution budget %q differs from sealed limits", bound.dimension)
		}
	}
	return nil
}
