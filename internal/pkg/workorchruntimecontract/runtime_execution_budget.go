package workorchruntimecontract

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeExecutionBudgetSchemaCurrent       = "workorch.runtime_execution_budget"
	RuntimeExecutionBudgetPolicyV1            = "runtime-execution-budget/default-v1"
	RuntimeBudgetAmountLimit            int64 = 1 << 60
)

type RuntimeBudgetScope string

const (
	RuntimeBudgetScopeWorkspace RuntimeBudgetScope = "workspace"
	RuntimeBudgetScopeRun       RuntimeBudgetScope = "run"
)

type RuntimeBudgetRestartDisposition string

const (
	RuntimeBudgetRestartFreshFromSuccessorPolicy RuntimeBudgetRestartDisposition = "fresh_from_successor_policy"
	RuntimeBudgetRestartZero                     RuntimeBudgetRestartDisposition = "zero"
	RuntimeBudgetRestartExplicitLimit            RuntimeBudgetRestartDisposition = "explicit_limit"
)

type RuntimeBudgetExhaustionDisposition string

const (
	RuntimeBudgetExhaustionRecoverableIfReconcilable RuntimeBudgetExhaustionDisposition = "recoverable_wait_if_reconcilable"
	RuntimeBudgetExhaustionTerminal                  RuntimeBudgetExhaustionDisposition = "terminal"
)

type RuntimeBudgetDimension string

const (
	RuntimeBudgetDimensionProviderCall          RuntimeBudgetDimension = "provider_call"
	RuntimeBudgetDimensionProviderInputToken    RuntimeBudgetDimension = "provider_input_token"
	RuntimeBudgetDimensionProviderOutputToken   RuntimeBudgetDimension = "provider_output_token"
	RuntimeBudgetDimensionWholeDocumentRecovery RuntimeBudgetDimension = "whole_document_recovery"
	RuntimeBudgetDimensionCandidateCorrection   RuntimeBudgetDimension = "candidate_correction"
	RuntimeBudgetDimensionSemanticGeneration    RuntimeBudgetDimension = "semantic_generation"
	RuntimeBudgetDimensionSemanticCorrection    RuntimeBudgetDimension = "semantic_correction"
	RuntimeBudgetDimensionCapabilityOperation   RuntimeBudgetDimension = "capability_operation"
	RuntimeBudgetDimensionOperationRecovery     RuntimeBudgetDimension = "operation_recovery"
	RuntimeBudgetDimensionRecoveryProbe         RuntimeBudgetDimension = "recovery_probe"
	RuntimeBudgetDimensionSameFocusRepair       RuntimeBudgetDimension = "same_focus_repair"
	RuntimeBudgetDimensionReasoningEffort       RuntimeBudgetDimension = "reasoning_effort"
)

type RuntimeBudgetDimensionPolicy struct {
	Dimension             RuntimeBudgetDimension
	Unit                  string
	Limit                 int64
	MaxIncrement          int64
	ExhaustionDisposition RuntimeBudgetExhaustionDisposition
}

func (p RuntimeBudgetDimensionPolicy) Normalized() RuntimeBudgetDimensionPolicy {
	p.Dimension = RuntimeBudgetDimension(strings.TrimSpace(string(p.Dimension)))
	p.Unit = strings.TrimSpace(p.Unit)
	p.ExhaustionDisposition = RuntimeBudgetExhaustionDisposition(strings.TrimSpace(string(p.ExhaustionDisposition)))
	return p
}

func (p RuntimeBudgetDimensionPolicy) Validate() error {
	p = p.Normalized()
	if !validRuntimeBudgetDimension(p.Dimension) || p.Unit == "" || p.Limit <= 0 || p.Limit > RuntimeBudgetAmountLimit ||
		p.MaxIncrement <= 0 || p.MaxIncrement > p.Limit {
		return fmt.Errorf("runtime budget dimension policy is incomplete or out of range")
	}
	switch p.ExhaustionDisposition {
	case RuntimeBudgetExhaustionRecoverableIfReconcilable, RuntimeBudgetExhaustionTerminal:
		return nil
	default:
		return fmt.Errorf("runtime budget dimension %q has invalid exhaustion disposition", p.Dimension)
	}
}

func validRuntimeBudgetDimension(d RuntimeBudgetDimension) bool {
	switch d {
	case RuntimeBudgetDimensionProviderCall, RuntimeBudgetDimensionProviderInputToken, RuntimeBudgetDimensionProviderOutputToken,
		RuntimeBudgetDimensionWholeDocumentRecovery, RuntimeBudgetDimensionCandidateCorrection, RuntimeBudgetDimensionSemanticGeneration,
		RuntimeBudgetDimensionSemanticCorrection, RuntimeBudgetDimensionCapabilityOperation, RuntimeBudgetDimensionOperationRecovery,
		RuntimeBudgetDimensionRecoveryProbe, RuntimeBudgetDimensionSameFocusRepair, RuntimeBudgetDimensionReasoningEffort:
		return true
	default:
		return false
	}
}

type RuntimeExecutionBudgetPolicy struct {
	SchemaVersion      string
	PolicyRevision     string
	Scope              RuntimeBudgetScope
	Dimensions         []RuntimeBudgetDimensionPolicy
	RestartDisposition RuntimeBudgetRestartDisposition
	PolicyHash         string
}

func (p RuntimeExecutionBudgetPolicy) Normalized() RuntimeExecutionBudgetPolicy {
	p.SchemaVersion = strings.TrimSpace(p.SchemaVersion)
	p.PolicyRevision = strings.TrimSpace(p.PolicyRevision)
	p.Scope = RuntimeBudgetScope(strings.TrimSpace(string(p.Scope)))
	p.RestartDisposition = RuntimeBudgetRestartDisposition(strings.TrimSpace(string(p.RestartDisposition)))
	p.Dimensions = append([]RuntimeBudgetDimensionPolicy(nil), p.Dimensions...)
	for idx := range p.Dimensions {
		p.Dimensions[idx] = p.Dimensions[idx].Normalized()
	}
	sort.SliceStable(p.Dimensions, func(i, j int) bool { return p.Dimensions[i].Dimension < p.Dimensions[j].Dimension })
	p.PolicyHash = strings.TrimSpace(p.PolicyHash)
	return p
}

func (p RuntimeExecutionBudgetPolicy) Clone() RuntimeExecutionBudgetPolicy { return p.Normalized() }

func (p RuntimeExecutionBudgetPolicy) Validate() error {
	p = p.Normalized()
	if p.SchemaVersion != RuntimeExecutionBudgetSchemaCurrent || p.PolicyRevision == "" || len(p.Dimensions) == 0 || p.PolicyHash == "" {
		return fmt.Errorf("runtime execution budget policy is incomplete")
	}
	if p.Scope != RuntimeBudgetScopeWorkspace && p.Scope != RuntimeBudgetScopeRun {
		return fmt.Errorf("runtime execution budget policy has invalid scope")
	}
	switch p.RestartDisposition {
	case RuntimeBudgetRestartFreshFromSuccessorPolicy, RuntimeBudgetRestartZero, RuntimeBudgetRestartExplicitLimit:
	default:
		return fmt.Errorf("runtime execution budget policy has invalid restart disposition")
	}
	seen := map[RuntimeBudgetDimension]struct{}{}
	for _, dimension := range p.Dimensions {
		if err := dimension.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[dimension.Dimension]; duplicate {
			return fmt.Errorf("runtime execution budget dimension %q is duplicated", dimension.Dimension)
		}
		seen[dimension.Dimension] = struct{}{}
	}
	if p.PolicyHash != stableRuntimeExecutionBudgetPolicyHashNormalized(p) {
		return fmt.Errorf("runtime execution budget policy hash mismatch")
	}
	return nil
}

func StableRuntimeExecutionBudgetPolicyHash(policy RuntimeExecutionBudgetPolicy) string {
	return stableRuntimeExecutionBudgetPolicyHashNormalized(policy.Normalized())
}

// 仅复用本次调用已规范化的 detached policy；不缓存预算校验或跨调用输入。
func stableRuntimeExecutionBudgetPolicyHashNormalized(p RuntimeExecutionBudgetPolicy) string {
	h := newRuntimeHashWriter()
	writeHashString(h, "runtime-execution-budget-policy")
	writeHashString(h, p.SchemaVersion)
	writeHashString(h, p.PolicyRevision)
	writeHashString(h, string(p.Scope))
	writeHashString(h, string(p.RestartDisposition))
	for _, dimension := range p.Dimensions {
		writeHashString(h, string(dimension.Dimension))
		writeHashString(h, dimension.Unit)
		writeHashString(h, fmt.Sprintf("%d", dimension.Limit))
		writeHashString(h, fmt.Sprintf("%d", dimension.MaxIncrement))
		writeHashString(h, string(dimension.ExhaustionDisposition))
	}
	return "runtime-budget-policy-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func DefaultRuntimeExecutionBudgetPolicy() RuntimeExecutionBudgetPolicy {
	return RuntimeExecutionBudgetPolicyForRuntimePolicies(DefaultRuntimeConvergencePolicy(), DefaultRuntimeRecoveryPolicy())
}

// RuntimeExecutionBudgetPolicyForRuntimePolicies compiles the unified view
// policy from the independently authoritative repair and recovery policies.
// The corresponding dimensions are references to those sub-ledgers; they are
// not a second source of repair/recovery consumption authority.
func RuntimeExecutionBudgetPolicyForRuntimePolicies(convergence RuntimeConvergencePolicy, recovery RuntimeRecoveryPolicy) RuntimeExecutionBudgetPolicy {
	convergence = convergence.Normalized()
	recovery = recovery.Normalized()
	const providerCalls int64 = 4096
	policy := RuntimeExecutionBudgetPolicy{
		SchemaVersion:      RuntimeExecutionBudgetSchemaCurrent,
		PolicyRevision:     RuntimeExecutionBudgetPolicyV1,
		Scope:              RuntimeBudgetScopeRun,
		RestartDisposition: RuntimeBudgetRestartFreshFromSuccessorPolicy,
		Dimensions: []RuntimeBudgetDimensionPolicy{
			{Dimension: RuntimeBudgetDimensionProviderCall, Unit: "call", Limit: providerCalls, MaxIncrement: 1, ExhaustionDisposition: RuntimeBudgetExhaustionTerminal},
			{Dimension: RuntimeBudgetDimensionProviderInputToken, Unit: "token", Limit: providerCalls * 131072, MaxIncrement: 131072, ExhaustionDisposition: RuntimeBudgetExhaustionTerminal},
			{Dimension: RuntimeBudgetDimensionProviderOutputToken, Unit: "token", Limit: providerCalls * 131072, MaxIncrement: 131072, ExhaustionDisposition: RuntimeBudgetExhaustionTerminal},
			{Dimension: RuntimeBudgetDimensionWholeDocumentRecovery, Unit: "attempt", Limit: 512, MaxIncrement: 1, ExhaustionDisposition: RuntimeBudgetExhaustionTerminal},
			{Dimension: RuntimeBudgetDimensionCandidateCorrection, Unit: "attempt", Limit: 128, MaxIncrement: 1, ExhaustionDisposition: RuntimeBudgetExhaustionTerminal},
			{Dimension: RuntimeBudgetDimensionSemanticGeneration, Unit: "generation", Limit: 2048, MaxIncrement: 1, ExhaustionDisposition: RuntimeBudgetExhaustionTerminal},
			{Dimension: RuntimeBudgetDimensionSemanticCorrection, Unit: "attempt", Limit: 128, MaxIncrement: 1, ExhaustionDisposition: RuntimeBudgetExhaustionTerminal},
			{Dimension: RuntimeBudgetDimensionCapabilityOperation, Unit: "operation", Limit: 8192, MaxIncrement: 1, ExhaustionDisposition: RuntimeBudgetExhaustionRecoverableIfReconcilable},
			{Dimension: RuntimeBudgetDimensionOperationRecovery, Unit: "attempt", Limit: int64(recovery.MaxOperationRecoveryAttempts), MaxIncrement: 1, ExhaustionDisposition: RuntimeBudgetExhaustionRecoverableIfReconcilable},
			{Dimension: RuntimeBudgetDimensionRecoveryProbe, Unit: "probe", Limit: 512, MaxIncrement: 1, ExhaustionDisposition: RuntimeBudgetExhaustionTerminal},
			{Dimension: RuntimeBudgetDimensionSameFocusRepair, Unit: "attempt", Limit: int64(convergence.MaxRepairAttemptsPerFocus), MaxIncrement: 1, ExhaustionDisposition: RuntimeBudgetExhaustionTerminal},
			{Dimension: RuntimeBudgetDimensionReasoningEffort, Unit: "request", Limit: providerCalls, MaxIncrement: 1, ExhaustionDisposition: RuntimeBudgetExhaustionTerminal},
		},
	}
	policy.PolicyHash = StableRuntimeExecutionBudgetPolicyHash(policy)
	return policy.Normalized()
}

// ValidateRuntimeExecutionBudgetPolicySources rejects drift between the
// unified policy and the two source-owned budget policies it projects.
func ValidateRuntimeExecutionBudgetPolicySources(policy RuntimeExecutionBudgetPolicy, convergence RuntimeConvergencePolicy, recovery RuntimeRecoveryPolicy) error {
	policy = policy.Normalized()
	convergence = convergence.Normalized()
	recovery = recovery.Normalized()
	checks := []struct {
		dimension RuntimeBudgetDimension
		limit     int64
	}{
		{dimension: RuntimeBudgetDimensionOperationRecovery, limit: int64(recovery.MaxOperationRecoveryAttempts)},
		{dimension: RuntimeBudgetDimensionSameFocusRepair, limit: int64(convergence.MaxRepairAttemptsPerFocus)},
	}
	for _, check := range checks {
		dimension, found := RuntimeBudgetDimensionPolicyFor(policy, check.dimension)
		if !found || dimension.Limit != check.limit || dimension.MaxIncrement != 1 {
			return fmt.Errorf("runtime execution budget dimension %q does not match its authoritative source policy", check.dimension)
		}
	}
	return nil
}

func runtimeBudgetDimensionUsesSourceLedger(dimension RuntimeBudgetDimension) bool {
	switch dimension {
	case RuntimeBudgetDimensionOperationRecovery, RuntimeBudgetDimensionSameFocusRepair:
		return true
	default:
		return false
	}
}

func SameRuntimeExecutionBudgetPolicy(left, right RuntimeExecutionBudgetPolicy) bool {
	l, r := left.Normalized(), right.Normalized()
	return l.PolicyHash != "" && l.PolicyHash == r.PolicyHash
}

func RuntimeBudgetDimensionPolicyFor(policy RuntimeExecutionBudgetPolicy, dimension RuntimeBudgetDimension) (RuntimeBudgetDimensionPolicy, bool) {
	// 同 dimension 的稳定排序保持原先后；只读查询不复制随后丢弃的整份 policy。
	for _, value := range policy.Dimensions {
		value = value.Normalized()
		if value.Dimension == dimension {
			return value, true
		}
	}
	return RuntimeBudgetDimensionPolicy{}, false
}

type RuntimeBudgetAllocationState string

const (
	RuntimeBudgetAllocationReserved  RuntimeBudgetAllocationState = "reserved"
	RuntimeBudgetAllocationConfirmed RuntimeBudgetAllocationState = "confirmed"
	RuntimeBudgetAllocationReleased  RuntimeBudgetAllocationState = "released"
	RuntimeBudgetAllocationUnknown   RuntimeBudgetAllocationState = "unknown"
	RuntimeBudgetAllocationExhausted RuntimeBudgetAllocationState = "exhausted"
)

type RuntimeBudgetAllocation struct {
	AllocationID         string
	Dimension            RuntimeBudgetDimension
	Unit                 string
	LogicalConsumptionID string
	ScopeRef             string
	PolicyRevision       string
	LimitAtAllocation    int64
	ReservedAmount       int64
	MaxIncrement         int64
	State                RuntimeBudgetAllocationState
	OwnerFactRef         string
	AllocationHash       string
}

func (a RuntimeBudgetAllocation) Normalized() RuntimeBudgetAllocation {
	a.AllocationID = strings.TrimSpace(a.AllocationID)
	a.Dimension = RuntimeBudgetDimension(strings.TrimSpace(string(a.Dimension)))
	a.Unit = strings.TrimSpace(a.Unit)
	a.LogicalConsumptionID = strings.TrimSpace(a.LogicalConsumptionID)
	a.ScopeRef = strings.TrimSpace(a.ScopeRef)
	a.PolicyRevision = strings.TrimSpace(a.PolicyRevision)
	a.State = RuntimeBudgetAllocationState(strings.TrimSpace(string(a.State)))
	a.OwnerFactRef = strings.TrimSpace(a.OwnerFactRef)
	a.AllocationHash = strings.TrimSpace(a.AllocationHash)
	return a
}

func (a RuntimeBudgetAllocation) Validate(policy RuntimeExecutionBudgetPolicy) error {
	a = a.Normalized()
	dimension, found := RuntimeBudgetDimensionPolicyFor(policy, a.Dimension)
	if !found || a.AllocationID == "" || a.Unit != dimension.Unit || a.LogicalConsumptionID == "" || a.ScopeRef == "" ||
		a.PolicyRevision != strings.TrimSpace(policy.PolicyRevision) || a.LimitAtAllocation != dimension.Limit || a.MaxIncrement != dimension.MaxIncrement ||
		a.ReservedAmount <= 0 || a.ReservedAmount > a.MaxIncrement || a.AllocationHash == "" {
		return fmt.Errorf("runtime budget allocation is incomplete or does not match policy")
	}
	switch a.State {
	case RuntimeBudgetAllocationReserved:
		if a.OwnerFactRef != "" {
			return fmt.Errorf("reserved runtime budget allocation cannot carry owner fact")
		}
	case RuntimeBudgetAllocationConfirmed, RuntimeBudgetAllocationReleased, RuntimeBudgetAllocationUnknown, RuntimeBudgetAllocationExhausted:
		if a.OwnerFactRef == "" {
			return fmt.Errorf("terminal runtime budget allocation requires owner fact")
		}
	default:
		return fmt.Errorf("runtime budget allocation has invalid state %q", a.State)
	}
	if a.AllocationID != StableRuntimeBudgetAllocationID(a) || a.AllocationHash != StableRuntimeBudgetAllocationHash(a) {
		return fmt.Errorf("runtime budget allocation identity mismatch")
	}
	return nil
}

func StableRuntimeBudgetAllocationID(allocation RuntimeBudgetAllocation) string {
	a := allocation.Normalized()
	h := newRuntimeHashWriter()
	writeHashString(h, "runtime-budget-allocation")
	writeHashString(h, a.PolicyRevision)
	writeHashString(h, string(a.Dimension))
	writeHashString(h, a.LogicalConsumptionID)
	writeHashString(h, a.ScopeRef)
	return "runtime-budget-allocation-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func StableRuntimeBudgetAllocationHash(allocation RuntimeBudgetAllocation) string {
	a := allocation.Normalized()
	h := newRuntimeHashWriter()
	writeHashString(h, "runtime-budget-allocation-value")
	writeHashString(h, a.AllocationID)
	writeHashString(h, string(a.Dimension))
	writeHashString(h, a.Unit)
	writeHashString(h, a.LogicalConsumptionID)
	writeHashString(h, a.ScopeRef)
	writeHashString(h, a.PolicyRevision)
	writeHashString(h, fmt.Sprintf("%d", a.LimitAtAllocation))
	writeHashString(h, fmt.Sprintf("%d", a.ReservedAmount))
	writeHashString(h, fmt.Sprintf("%d", a.MaxIncrement))
	writeHashString(h, string(a.State))
	writeHashString(h, a.OwnerFactRef)
	return "runtime-budget-allocation-value-" + hex.EncodeToString(h.Sum(nil))[:20]
}

type RuntimeBudgetConsumptionFact struct {
	ConsumptionID         string
	AllocationID          string
	Owner                 string
	LogicalConsumptionID  string
	PhysicalConsumptionID string
	Unit                  string
	ConfirmedAmount       int64
	UnknownAmount         int64
	Currency              string
	MonetaryAmount        int64
	OwnerResultRef        string
	OwnerResultHash       string
	ConsumptionHash       string
}

func (f RuntimeBudgetConsumptionFact) Normalized() RuntimeBudgetConsumptionFact {
	f.ConsumptionID = strings.TrimSpace(f.ConsumptionID)
	f.AllocationID = strings.TrimSpace(f.AllocationID)
	f.Owner = strings.TrimSpace(f.Owner)
	f.LogicalConsumptionID = strings.TrimSpace(f.LogicalConsumptionID)
	f.PhysicalConsumptionID = strings.TrimSpace(f.PhysicalConsumptionID)
	f.Unit = strings.TrimSpace(f.Unit)
	f.Currency = strings.ToUpper(strings.TrimSpace(f.Currency))
	f.OwnerResultRef = strings.TrimSpace(f.OwnerResultRef)
	f.OwnerResultHash = strings.TrimSpace(f.OwnerResultHash)
	f.ConsumptionHash = strings.TrimSpace(f.ConsumptionHash)
	return f
}

func (f RuntimeBudgetConsumptionFact) Validate(allocation RuntimeBudgetAllocation) error {
	f = f.Normalized()
	a := allocation.Normalized()
	if f.ConsumptionID == "" || f.AllocationID != a.AllocationID || f.Owner == "" || f.LogicalConsumptionID != a.LogicalConsumptionID ||
		f.PhysicalConsumptionID == "" || f.Unit != a.Unit || f.ConfirmedAmount < 0 || f.UnknownAmount < 0 ||
		f.ConfirmedAmount > RuntimeBudgetAmountLimit || f.UnknownAmount > RuntimeBudgetAmountLimit ||
		(f.ConfirmedAmount > 0 && f.UnknownAmount > 0) || f.ConfirmedAmount+f.UnknownAmount > a.ReservedAmount ||
		f.MonetaryAmount < 0 || (f.MonetaryAmount > 0 && f.Currency == "") || f.OwnerResultRef == "" || f.OwnerResultHash == "" {
		return fmt.Errorf("runtime budget consumption fact is incomplete or exceeds its allocation")
	}
	if f.ConsumptionID != StableRuntimeBudgetConsumptionID(f) || f.ConsumptionHash != StableRuntimeBudgetConsumptionHash(f) {
		return fmt.Errorf("runtime budget consumption identity mismatch")
	}
	return nil
}

func StableRuntimeBudgetConsumptionID(fact RuntimeBudgetConsumptionFact) string {
	f := fact.Normalized()
	h := newRuntimeHashWriter()
	writeHashString(h, "runtime-budget-consumption")
	writeHashString(h, f.AllocationID)
	writeHashString(h, f.Owner)
	writeHashString(h, f.PhysicalConsumptionID)
	return "runtime-budget-consumption-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func StableRuntimeBudgetConsumptionHash(fact RuntimeBudgetConsumptionFact) string {
	f := fact.Normalized()
	h := newRuntimeHashWriter()
	writeHashString(h, "runtime-budget-consumption-value")
	writeHashString(h, f.ConsumptionID)
	writeHashString(h, f.AllocationID)
	writeHashString(h, f.Owner)
	writeHashString(h, f.LogicalConsumptionID)
	writeHashString(h, f.PhysicalConsumptionID)
	writeHashString(h, f.Unit)
	writeHashString(h, fmt.Sprintf("%d", f.ConfirmedAmount))
	writeHashString(h, fmt.Sprintf("%d", f.UnknownAmount))
	writeHashString(h, f.Currency)
	writeHashString(h, fmt.Sprintf("%d", f.MonetaryAmount))
	writeHashString(h, f.OwnerResultRef)
	writeHashString(h, f.OwnerResultHash)
	return "runtime-budget-consumption-value-" + hex.EncodeToString(h.Sum(nil))[:20]
}

type RuntimeBudgetReconciliationStatus string

const (
	RuntimeBudgetReconciliationPending      RuntimeBudgetReconciliationStatus = "pending"
	RuntimeBudgetReconciliationConfirmed    RuntimeBudgetReconciliationStatus = "confirmed"
	RuntimeBudgetReconciliationReleased     RuntimeBudgetReconciliationStatus = "released"
	RuntimeBudgetReconciliationStillUnknown RuntimeBudgetReconciliationStatus = "still_unknown"
	RuntimeBudgetReconciliationFailed       RuntimeBudgetReconciliationStatus = "failed"
)

type RuntimeBudgetReconciliationRecord struct {
	ReconciliationID   string
	AllocationID       string
	ProbeKind          string
	ProbeOwner         string
	ProbeRequestHash   string
	ExpectedOwnerFence string
	Status             RuntimeBudgetReconciliationStatus
	OwnerResultHash    string
	ReconciliationHash string
}

func (r RuntimeBudgetReconciliationRecord) Normalized() RuntimeBudgetReconciliationRecord {
	r.ReconciliationID = strings.TrimSpace(r.ReconciliationID)
	r.AllocationID = strings.TrimSpace(r.AllocationID)
	r.ProbeKind = strings.TrimSpace(r.ProbeKind)
	r.ProbeOwner = strings.TrimSpace(r.ProbeOwner)
	r.ProbeRequestHash = strings.TrimSpace(r.ProbeRequestHash)
	r.ExpectedOwnerFence = strings.TrimSpace(r.ExpectedOwnerFence)
	r.Status = RuntimeBudgetReconciliationStatus(strings.TrimSpace(string(r.Status)))
	r.OwnerResultHash = strings.TrimSpace(r.OwnerResultHash)
	r.ReconciliationHash = strings.TrimSpace(r.ReconciliationHash)
	return r
}

func (r RuntimeBudgetReconciliationRecord) Validate(allocations []RuntimeBudgetAllocation) error {
	r = r.Normalized()
	if r.ReconciliationID == "" || r.AllocationID == "" || r.ProbeKind == "" || r.ProbeOwner == "" || r.ProbeRequestHash == "" || r.ExpectedOwnerFence == "" || r.ReconciliationHash == "" {
		return fmt.Errorf("runtime budget reconciliation is incomplete")
	}
	allocation, found := RuntimeBudgetAllocationForID(allocations, r.AllocationID)
	if !found {
		return fmt.Errorf("runtime budget reconciliation has no allocation")
	}
	switch r.Status {
	case RuntimeBudgetReconciliationPending:
		if allocation.State != RuntimeBudgetAllocationUnknown {
			return fmt.Errorf("pending runtime budget reconciliation has no unknown allocation")
		}
		if r.OwnerResultHash != "" {
			return fmt.Errorf("pending runtime budget reconciliation cannot carry result")
		}
	case RuntimeBudgetReconciliationStillUnknown:
		if allocation.State != RuntimeBudgetAllocationUnknown || r.OwnerResultHash == "" {
			return fmt.Errorf("still-unknown runtime budget reconciliation has no matching owner result")
		}
	case RuntimeBudgetReconciliationConfirmed:
		if allocation.State != RuntimeBudgetAllocationConfirmed {
			return fmt.Errorf("confirmed runtime budget reconciliation has no confirmed allocation")
		}
		if r.OwnerResultHash == "" {
			return fmt.Errorf("completed runtime budget reconciliation requires owner result")
		}
	case RuntimeBudgetReconciliationReleased:
		if allocation.State != RuntimeBudgetAllocationReleased {
			return fmt.Errorf("released runtime budget reconciliation has no released allocation")
		}
		if r.OwnerResultHash == "" {
			return fmt.Errorf("completed runtime budget reconciliation requires owner result")
		}
	case RuntimeBudgetReconciliationFailed:
		if allocation.State != RuntimeBudgetAllocationExhausted {
			return fmt.Errorf("failed runtime budget reconciliation has no exhausted allocation")
		}
		if r.OwnerResultHash == "" {
			return fmt.Errorf("completed runtime budget reconciliation requires owner result")
		}
	default:
		return fmt.Errorf("runtime budget reconciliation has invalid status")
	}
	if r.ReconciliationID != StableRuntimeBudgetReconciliationID(r) || r.ReconciliationHash != StableRuntimeBudgetReconciliationHash(r) {
		return fmt.Errorf("runtime budget reconciliation identity mismatch")
	}
	return nil
}

func StableRuntimeBudgetReconciliationID(record RuntimeBudgetReconciliationRecord) string {
	r := record.Normalized()
	h := newRuntimeHashWriter()
	writeHashString(h, "runtime-budget-reconciliation")
	writeHashString(h, r.AllocationID)
	writeHashString(h, r.ProbeKind)
	writeHashString(h, r.ProbeOwner)
	writeHashString(h, r.ProbeRequestHash)
	writeHashString(h, r.ExpectedOwnerFence)
	return "runtime-budget-reconciliation-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func StableRuntimeBudgetReconciliationHash(record RuntimeBudgetReconciliationRecord) string {
	r := record.Normalized()
	h := newRuntimeHashWriter()
	writeHashString(h, "runtime-budget-reconciliation-value")
	writeHashString(h, r.ReconciliationID)
	writeHashString(h, r.AllocationID)
	writeHashString(h, r.ProbeKind)
	writeHashString(h, r.ProbeOwner)
	writeHashString(h, r.ProbeRequestHash)
	writeHashString(h, r.ExpectedOwnerFence)
	writeHashString(h, string(r.Status))
	writeHashString(h, r.OwnerResultHash)
	return "runtime-budget-reconciliation-value-" + hex.EncodeToString(h.Sum(nil))[:20]
}

type RuntimeExecutionBudgetLedger struct {
	Policy          RuntimeExecutionBudgetPolicy
	Allocations     []RuntimeBudgetAllocation
	Consumptions    []RuntimeBudgetConsumptionFact
	Reconciliations []RuntimeBudgetReconciliationRecord
}

func NewRuntimeExecutionBudgetLedger(policy RuntimeExecutionBudgetPolicy) (RuntimeExecutionBudgetLedger, error) {
	policy = policy.Normalized()
	if err := policy.Validate(); err != nil {
		return RuntimeExecutionBudgetLedger{}, err
	}
	return RuntimeExecutionBudgetLedger{Policy: policy.Clone()}, nil
}

func (l RuntimeExecutionBudgetLedger) Clone() RuntimeExecutionBudgetLedger {
	l.Policy = l.Policy.Clone()
	l.Allocations = append([]RuntimeBudgetAllocation(nil), l.Allocations...)
	for idx := range l.Allocations {
		l.Allocations[idx] = l.Allocations[idx].Normalized()
	}
	sort.SliceStable(l.Allocations, func(i, j int) bool { return l.Allocations[i].AllocationID < l.Allocations[j].AllocationID })
	l.Consumptions = append([]RuntimeBudgetConsumptionFact(nil), l.Consumptions...)
	for idx := range l.Consumptions {
		l.Consumptions[idx] = l.Consumptions[idx].Normalized()
	}
	sort.SliceStable(l.Consumptions, func(i, j int) bool { return l.Consumptions[i].ConsumptionID < l.Consumptions[j].ConsumptionID })
	l.Reconciliations = append([]RuntimeBudgetReconciliationRecord(nil), l.Reconciliations...)
	for idx := range l.Reconciliations {
		l.Reconciliations[idx] = l.Reconciliations[idx].Normalized()
	}
	sort.SliceStable(l.Reconciliations, func(i, j int) bool {
		return l.Reconciliations[i].ReconciliationID < l.Reconciliations[j].ReconciliationID
	})
	return l
}

func (l RuntimeExecutionBudgetLedger) Validate() error {
	l = l.Clone()
	if err := l.Policy.Validate(); err != nil {
		return err
	}
	allocations := map[string]RuntimeBudgetAllocation{}
	for _, allocation := range l.Allocations {
		if runtimeBudgetDimensionUsesSourceLedger(allocation.Dimension) {
			return fmt.Errorf("runtime budget dimension %q must be derived from its authoritative source ledger", allocation.Dimension)
		}
		if err := allocation.Validate(l.Policy); err != nil {
			return err
		}
		if _, duplicate := allocations[allocation.AllocationID]; duplicate {
			return fmt.Errorf("runtime budget allocation %q is duplicated", allocation.AllocationID)
		}
		allocations[allocation.AllocationID] = allocation
	}
	consumptionsByAllocation := map[string]int{}
	physicalOwners := map[string]string{}
	seenConsumptions := map[string]struct{}{}
	for _, consumption := range l.Consumptions {
		allocation, found := allocations[consumption.AllocationID]
		if !found {
			return fmt.Errorf("runtime budget consumption has no allocation")
		}
		if err := consumption.Validate(allocation); err != nil {
			return err
		}
		if _, duplicate := seenConsumptions[consumption.ConsumptionID]; duplicate {
			return fmt.Errorf("runtime budget consumption %q is duplicated", consumption.ConsumptionID)
		}
		seenConsumptions[consumption.ConsumptionID] = struct{}{}
		consumptionsByAllocation[consumption.AllocationID]++
		physicalKey := consumption.Owner + "\x00" + consumption.PhysicalConsumptionID + "\x00" + string(allocation.Dimension)
		if other, duplicate := physicalOwners[physicalKey]; duplicate && other != consumption.AllocationID {
			return fmt.Errorf("runtime physical consumption belongs to multiple allocations")
		}
		physicalOwners[physicalKey] = consumption.AllocationID
	}
	for _, allocation := range l.Allocations {
		count := consumptionsByAllocation[allocation.AllocationID]
		switch allocation.State {
		case RuntimeBudgetAllocationReserved, RuntimeBudgetAllocationReleased:
			if count != 0 {
				return fmt.Errorf("runtime budget allocation state %q cannot carry consumption", allocation.State)
			}
		case RuntimeBudgetAllocationConfirmed:
			if count != 1 {
				return fmt.Errorf("confirmed runtime budget allocation requires one consumption")
			}
			fact := runtimeBudgetConsumptionForAllocation(l.Consumptions, allocation.AllocationID)
			if fact.UnknownAmount != 0 || allocation.OwnerFactRef != fact.OwnerResultRef {
				return fmt.Errorf("confirmed runtime budget allocation carries unknown consumption")
			}
		case RuntimeBudgetAllocationUnknown, RuntimeBudgetAllocationExhausted:
			if count != 1 {
				return fmt.Errorf("unknown or exhausted runtime budget allocation requires one consumption")
			}
			fact := runtimeBudgetConsumptionForAllocation(l.Consumptions, allocation.AllocationID)
			if fact.UnknownAmount <= 0 || fact.ConfirmedAmount != 0 || allocation.OwnerFactRef != fact.OwnerResultRef {
				return fmt.Errorf("unknown or exhausted runtime budget allocation has invalid consumption")
			}
		}
	}
	seenReconciliations := map[string]struct{}{}
	for _, record := range l.Reconciliations {
		if err := record.Validate(l.Allocations); err != nil {
			return err
		}
		if _, duplicate := seenReconciliations[record.ReconciliationID]; duplicate {
			return fmt.Errorf("runtime budget reconciliation %q is duplicated", record.ReconciliationID)
		}
		seenReconciliations[record.ReconciliationID] = struct{}{}
	}
	_, err := BuildRuntimeExecutionBudgetView("workspace-validation", "run-validation", 1, l, "", "")
	return err
}

func RuntimeBudgetAllocationForID(values []RuntimeBudgetAllocation, allocationID string) (RuntimeBudgetAllocation, bool) {
	allocationID = strings.TrimSpace(allocationID)
	for _, value := range values {
		value = value.Normalized()
		if value.AllocationID == allocationID {
			return value, true
		}
	}
	return RuntimeBudgetAllocation{}, false
}

func runtimeBudgetConsumptionForAllocation(values []RuntimeBudgetConsumptionFact, allocationID string) RuntimeBudgetConsumptionFact {
	for _, value := range values {
		if value.Normalized().AllocationID == allocationID {
			return value.Normalized()
		}
	}
	return RuntimeBudgetConsumptionFact{}
}

type RuntimeBudgetAllocationSpec struct {
	Dimension            RuntimeBudgetDimension
	LogicalConsumptionID string
	ScopeRef             string
	ReservedAmount       int64
}

type RuntimeBudgetExhaustedError struct {
	Dimension RuntimeBudgetDimension
	Required  int64
	Remaining int64
}

func (e *RuntimeBudgetExhaustedError) Error() string {
	if e == nil {
		return "runtime execution budget exhausted"
	}
	return fmt.Sprintf("runtime execution budget %s exhausted: required=%d remaining=%d", e.Dimension, e.Required, e.Remaining)
}

func ReserveRuntimeBudgetAllocations(ledger RuntimeExecutionBudgetLedger, specs []RuntimeBudgetAllocationSpec) (RuntimeExecutionBudgetLedger, []RuntimeBudgetAllocation, bool, error) {
	ledger = ledger.Clone()
	if err := ledger.Validate(); err != nil {
		return RuntimeExecutionBudgetLedger{}, nil, false, err
	}
	if len(specs) == 0 {
		return ledger, nil, true, nil
	}
	created := make([]RuntimeBudgetAllocation, 0, len(specs))
	allDuplicate := true
	for _, spec := range specs {
		spec.Dimension = RuntimeBudgetDimension(strings.TrimSpace(string(spec.Dimension)))
		spec.LogicalConsumptionID = strings.TrimSpace(spec.LogicalConsumptionID)
		spec.ScopeRef = strings.TrimSpace(spec.ScopeRef)
		dimension, found := RuntimeBudgetDimensionPolicyFor(ledger.Policy, spec.Dimension)
		if !found || runtimeBudgetDimensionUsesSourceLedger(spec.Dimension) || spec.LogicalConsumptionID == "" || spec.ScopeRef == "" || spec.ReservedAmount <= 0 || spec.ReservedAmount > dimension.MaxIncrement {
			return RuntimeExecutionBudgetLedger{}, nil, false, common.ErrInvalidArgument
		}
		allocation := RuntimeBudgetAllocation{
			Dimension: spec.Dimension, Unit: dimension.Unit, LogicalConsumptionID: spec.LogicalConsumptionID, ScopeRef: spec.ScopeRef,
			PolicyRevision: ledger.Policy.PolicyRevision, LimitAtAllocation: dimension.Limit,
			ReservedAmount: spec.ReservedAmount, MaxIncrement: dimension.MaxIncrement, State: RuntimeBudgetAllocationReserved,
		}
		allocation.AllocationID = StableRuntimeBudgetAllocationID(allocation)
		allocation.AllocationHash = StableRuntimeBudgetAllocationHash(allocation)
		if existing, found := RuntimeBudgetAllocationForID(ledger.Allocations, allocation.AllocationID); found {
			// Only replay of the exact still-reserved authorization is a
			// duplicate. A confirmed/unknown/released/exhausted allocation is a
			// terminal value and must never authorize another external action.
			if existing.AllocationHash != allocation.AllocationHash {
				return RuntimeExecutionBudgetLedger{}, nil, false, fmt.Errorf("runtime budget allocation identity conflicts")
			}
			created = append(created, existing)
			continue
		}
		remaining, err := RuntimeBudgetRemaining(ledger, spec.Dimension)
		if err != nil {
			return RuntimeExecutionBudgetLedger{}, nil, false, err
		}
		if remaining < spec.ReservedAmount {
			return RuntimeExecutionBudgetLedger{}, nil, false, &RuntimeBudgetExhaustedError{Dimension: spec.Dimension, Required: spec.ReservedAmount, Remaining: remaining}
		}
		ledger.Allocations = append(ledger.Allocations, allocation)
		created = append(created, allocation)
		allDuplicate = false
	}
	ledger = ledger.Clone()
	if err := ledger.Validate(); err != nil {
		return RuntimeExecutionBudgetLedger{}, nil, false, err
	}
	return ledger, created, allDuplicate, nil
}

type RuntimeBudgetConsumptionJoin struct {
	AllocationID          string
	Owner                 string
	PhysicalConsumptionID string
	ConfirmedAmount       int64
	UnknownAmount         int64
	Currency              string
	MonetaryAmount        int64
	OwnerResultRef        string
	OwnerResultHash       string
}

func JoinRuntimeBudgetConsumption(ledger RuntimeExecutionBudgetLedger, join RuntimeBudgetConsumptionJoin) (RuntimeExecutionBudgetLedger, RuntimeBudgetConsumptionFact, bool, error) {
	ledger = ledger.Clone()
	allocation, found := RuntimeBudgetAllocationForID(ledger.Allocations, join.AllocationID)
	if !found {
		return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, common.ErrInvalidArgument
	}
	fact := RuntimeBudgetConsumptionFact{
		AllocationID: allocation.AllocationID, Owner: strings.TrimSpace(join.Owner), LogicalConsumptionID: allocation.LogicalConsumptionID,
		PhysicalConsumptionID: strings.TrimSpace(join.PhysicalConsumptionID), Unit: allocation.Unit,
		ConfirmedAmount: join.ConfirmedAmount, UnknownAmount: join.UnknownAmount, Currency: join.Currency,
		MonetaryAmount: join.MonetaryAmount, OwnerResultRef: join.OwnerResultRef, OwnerResultHash: join.OwnerResultHash,
	}
	fact.ConsumptionID = StableRuntimeBudgetConsumptionID(fact)
	fact.ConsumptionHash = StableRuntimeBudgetConsumptionHash(fact)
	if err := fact.Validate(allocation); err != nil {
		return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, err
	}
	for _, existing := range ledger.Consumptions {
		if existing.ConsumptionID != fact.ConsumptionID {
			continue
		}
		if existing.ConsumptionHash == fact.ConsumptionHash {
			return ledger, existing, false, nil
		}
		// An unknown physical consumption is deliberately conservative, not a
		// terminal value.  Exact owner reconciliation may replace it with the
		// confirmed fact for the same allocation/owner/physical identity.  No
		// other value rewrite is legal.
		if allocation.State != RuntimeBudgetAllocationUnknown || existing.UnknownAmount <= 0 || fact.UnknownAmount != 0 {
			return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, fmt.Errorf("conflicting duplicate runtime budget consumption")
		}
		break
	}
	if allocation.State != RuntimeBudgetAllocationReserved && allocation.State != RuntimeBudgetAllocationUnknown {
		return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, fmt.Errorf("runtime budget allocation state %q cannot accept consumption", allocation.State)
	}
	if allocation.State == RuntimeBudgetAllocationUnknown {
		old := runtimeBudgetConsumptionForAllocation(ledger.Consumptions, allocation.AllocationID)
		if old.ConsumptionID != fact.ConsumptionID {
			return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, fmt.Errorf("runtime budget unknown allocation requires exact reconciliation identity")
		}
		for idx := range ledger.Consumptions {
			if ledger.Consumptions[idx].ConsumptionID == old.ConsumptionID {
				ledger.Consumptions[idx] = fact
			}
		}
	} else {
		ledger.Consumptions = append(ledger.Consumptions, fact)
	}
	for idx := range ledger.Allocations {
		if ledger.Allocations[idx].AllocationID != allocation.AllocationID {
			continue
		}
		if fact.UnknownAmount > 0 {
			ledger.Allocations[idx].State = RuntimeBudgetAllocationUnknown
		} else {
			ledger.Allocations[idx].State = RuntimeBudgetAllocationConfirmed
		}
		ledger.Allocations[idx].OwnerFactRef = fact.OwnerResultRef
		ledger.Allocations[idx].AllocationHash = StableRuntimeBudgetAllocationHash(ledger.Allocations[idx])
	}
	ledger = ledger.Clone()
	if err := ledger.Validate(); err != nil {
		return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, err
	}
	return ledger, fact, true, nil
}

// ReleaseRuntimeBudgetAllocation records owner-grounded proof that the
// reserved physical consumption did not occur. It intentionally accepts only
// a still-reserved allocation; unknown consumption needs exact owner
// reconciliation and cannot be erased by a local timeout.
func ReleaseRuntimeBudgetAllocation(ledger RuntimeExecutionBudgetLedger, allocationID, ownerFactRef string) (RuntimeExecutionBudgetLedger, bool, error) {
	ledger = ledger.Clone()
	allocationID = strings.TrimSpace(allocationID)
	ownerFactRef = strings.TrimSpace(ownerFactRef)
	if allocationID == "" || ownerFactRef == "" {
		return RuntimeExecutionBudgetLedger{}, false, common.ErrInvalidArgument
	}
	for idx := range ledger.Allocations {
		allocation := ledger.Allocations[idx].Normalized()
		if allocation.AllocationID != allocationID {
			continue
		}
		if allocation.State == RuntimeBudgetAllocationReleased && allocation.OwnerFactRef == ownerFactRef {
			return ledger, false, nil
		}
		if allocation.State != RuntimeBudgetAllocationReserved {
			return RuntimeExecutionBudgetLedger{}, false, fmt.Errorf("runtime budget allocation state %q cannot be released without reconciliation", allocation.State)
		}
		allocation.State = RuntimeBudgetAllocationReleased
		allocation.OwnerFactRef = ownerFactRef
		allocation.AllocationHash = StableRuntimeBudgetAllocationHash(allocation)
		ledger.Allocations[idx] = allocation
		ledger = ledger.Clone()
		if err := ledger.Validate(); err != nil {
			return RuntimeExecutionBudgetLedger{}, false, err
		}
		return ledger, true, nil
	}
	return RuntimeExecutionBudgetLedger{}, false, common.ErrNotFound
}

// NewRuntimeBudgetReconciliationRecord seals the exact probe needed to
// reconcile one unknown allocation. It does not mutate the ledger.
func NewRuntimeBudgetReconciliationRecord(ledger RuntimeExecutionBudgetLedger, allocationID, probeKind, probeOwner, probeRequestHash, expectedOwnerFence string) (RuntimeBudgetReconciliationRecord, error) {
	ledger = ledger.Clone()
	if err := ledger.Validate(); err != nil {
		return RuntimeBudgetReconciliationRecord{}, err
	}
	record := RuntimeBudgetReconciliationRecord{
		AllocationID: strings.TrimSpace(allocationID), ProbeKind: strings.TrimSpace(probeKind), ProbeOwner: strings.TrimSpace(probeOwner),
		ProbeRequestHash: strings.TrimSpace(probeRequestHash), ExpectedOwnerFence: strings.TrimSpace(expectedOwnerFence),
		Status: RuntimeBudgetReconciliationPending,
	}
	record.ReconciliationID = StableRuntimeBudgetReconciliationID(record)
	record.ReconciliationHash = StableRuntimeBudgetReconciliationHash(record)
	if err := record.Validate(ledger.Allocations); err != nil {
		return RuntimeBudgetReconciliationRecord{}, err
	}
	return record.Normalized(), nil
}

// OpenRuntimeBudgetReconciliation appends one pending reconciliation after an
// external outcome has become unknown. Exact replay is a no-op; another
// active reconciliation for the same allocation is a conflict.
func OpenRuntimeBudgetReconciliation(ledger RuntimeExecutionBudgetLedger, record RuntimeBudgetReconciliationRecord) (RuntimeExecutionBudgetLedger, bool, error) {
	ledger = ledger.Clone()
	if err := ledger.Validate(); err != nil {
		return RuntimeExecutionBudgetLedger{}, false, err
	}
	record = record.Normalized()
	if record.Status != RuntimeBudgetReconciliationPending {
		return RuntimeExecutionBudgetLedger{}, false, common.ErrInvalidArgument
	}
	if err := record.Validate(ledger.Allocations); err != nil {
		return RuntimeExecutionBudgetLedger{}, false, err
	}
	for _, existing := range ledger.Reconciliations {
		existing = existing.Normalized()
		if existing.ReconciliationID == record.ReconciliationID {
			if existing.ReconciliationHash != record.ReconciliationHash {
				return RuntimeExecutionBudgetLedger{}, false, fmt.Errorf("runtime budget reconciliation identity conflicts")
			}
			return ledger, false, nil
		}
		if existing.AllocationID == record.AllocationID && (existing.Status == RuntimeBudgetReconciliationPending || existing.Status == RuntimeBudgetReconciliationStillUnknown) {
			return RuntimeExecutionBudgetLedger{}, false, fmt.Errorf("runtime budget allocation already has an active reconciliation")
		}
	}
	ledger.Reconciliations = append(ledger.Reconciliations, record)
	ledger = ledger.Clone()
	if err := ledger.Validate(); err != nil {
		return RuntimeExecutionBudgetLedger{}, false, err
	}
	return ledger, true, nil
}

// MarkRuntimeBudgetReconciliationStillUnknown records that the exact owner
// probe completed without resolving whether the physical consumption
// occurred. It preserves the conservative unknown allocation and is
// idempotent only for the same owner result.
func MarkRuntimeBudgetReconciliationStillUnknown(ledger RuntimeExecutionBudgetLedger, reconciliationID, ownerResultHash string) (RuntimeExecutionBudgetLedger, bool, error) {
	ledger = ledger.Clone()
	if err := ledger.Validate(); err != nil {
		return RuntimeExecutionBudgetLedger{}, false, err
	}
	reconciliationID = strings.TrimSpace(reconciliationID)
	ownerResultHash = strings.TrimSpace(ownerResultHash)
	if reconciliationID == "" || ownerResultHash == "" {
		return RuntimeExecutionBudgetLedger{}, false, common.ErrInvalidArgument
	}
	for idx := range ledger.Reconciliations {
		record := ledger.Reconciliations[idx].Normalized()
		if record.ReconciliationID != reconciliationID {
			continue
		}
		if record.Status == RuntimeBudgetReconciliationStillUnknown {
			if record.OwnerResultHash == ownerResultHash {
				return ledger, false, nil
			}
			return RuntimeExecutionBudgetLedger{}, false, fmt.Errorf("conflicting duplicate runtime budget still-unknown result")
		}
		if record.Status != RuntimeBudgetReconciliationPending {
			return RuntimeExecutionBudgetLedger{}, false, fmt.Errorf("runtime budget reconciliation state %q cannot remain unknown", record.Status)
		}
		allocation, found := RuntimeBudgetAllocationForID(ledger.Allocations, record.AllocationID)
		if !found || allocation.State != RuntimeBudgetAllocationUnknown {
			return RuntimeExecutionBudgetLedger{}, false, fmt.Errorf("runtime budget still-unknown reconciliation requires an unknown allocation")
		}
		record.Status = RuntimeBudgetReconciliationStillUnknown
		record.OwnerResultHash = ownerResultHash
		record.ReconciliationHash = StableRuntimeBudgetReconciliationHash(record)
		ledger.Reconciliations[idx] = record
		ledger = ledger.Clone()
		if err := ledger.Validate(); err != nil {
			return RuntimeExecutionBudgetLedger{}, false, err
		}
		return ledger, true, nil
	}
	return RuntimeExecutionBudgetLedger{}, false, common.ErrNotFound
}

// ConfirmRuntimeBudgetReconciliation atomically replaces the unknown
// consumption for the same owner/physical identity and completes its pending
// reconciliation record. No intermediate ledger shape is returned.
func ConfirmRuntimeBudgetReconciliation(ledger RuntimeExecutionBudgetLedger, reconciliationID string, join RuntimeBudgetConsumptionJoin) (RuntimeExecutionBudgetLedger, RuntimeBudgetConsumptionFact, bool, error) {
	ledger = ledger.Clone()
	if err := ledger.Validate(); err != nil {
		return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, err
	}
	reconciliationID = strings.TrimSpace(reconciliationID)
	idx := -1
	var record RuntimeBudgetReconciliationRecord
	for current := range ledger.Reconciliations {
		if ledger.Reconciliations[current].ReconciliationID == reconciliationID {
			idx = current
			record = ledger.Reconciliations[current].Normalized()
			break
		}
	}
	if idx < 0 {
		return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, common.ErrNotFound
	}
	join.AllocationID = strings.TrimSpace(join.AllocationID)
	if join.AllocationID != record.AllocationID || join.ConfirmedAmount <= 0 || join.UnknownAmount != 0 || strings.TrimSpace(join.OwnerResultHash) == "" {
		return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, common.ErrInvalidArgument
	}
	if record.Status == RuntimeBudgetReconciliationConfirmed {
		existing := runtimeBudgetConsumptionForAllocation(ledger.Consumptions, record.AllocationID)
		if existing.Owner == strings.TrimSpace(join.Owner) && existing.PhysicalConsumptionID == strings.TrimSpace(join.PhysicalConsumptionID) &&
			existing.ConfirmedAmount == join.ConfirmedAmount && existing.UnknownAmount == 0 && existing.OwnerResultRef == strings.TrimSpace(join.OwnerResultRef) &&
			existing.OwnerResultHash == strings.TrimSpace(join.OwnerResultHash) && record.OwnerResultHash == strings.TrimSpace(join.OwnerResultHash) {
			return ledger, existing, false, nil
		}
		return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, fmt.Errorf("conflicting duplicate runtime budget reconciliation result")
	}
	if record.Status != RuntimeBudgetReconciliationPending && record.Status != RuntimeBudgetReconciliationStillUnknown {
		return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, fmt.Errorf("runtime budget reconciliation state %q cannot be confirmed", record.Status)
	}
	// Remove the pending record only inside this local value so the existing
	// join primitive can validate the final allocation/consumption pair. The
	// completed record is restored before the value can escape this function.
	ledger.Reconciliations = append(append([]RuntimeBudgetReconciliationRecord(nil), ledger.Reconciliations[:idx]...), ledger.Reconciliations[idx+1:]...)
	joined, fact, changed, err := JoinRuntimeBudgetConsumption(ledger, join)
	if err != nil {
		return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, err
	}
	if !changed {
		return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, fmt.Errorf("runtime budget reconciliation did not replace unknown consumption")
	}
	record.Status = RuntimeBudgetReconciliationConfirmed
	record.OwnerResultHash = strings.TrimSpace(join.OwnerResultHash)
	record.ReconciliationHash = StableRuntimeBudgetReconciliationHash(record)
	joined.Reconciliations = append(joined.Reconciliations, record)
	joined = joined.Clone()
	if err := joined.Validate(); err != nil {
		return RuntimeExecutionBudgetLedger{}, RuntimeBudgetConsumptionFact{}, false, err
	}
	return joined, fact, true, nil
}

// ReleaseRuntimeBudgetReconciliation atomically applies owner proof that an
// unknown physical consumption did not occur. The conservative unknown fact
// is removed only in the same value that closes the exact reconciliation and
// releases its allocation.
func ReleaseRuntimeBudgetReconciliation(ledger RuntimeExecutionBudgetLedger, reconciliationID, ownerResultRef, ownerResultHash string) (RuntimeExecutionBudgetLedger, bool, error) {
	ledger = ledger.Clone()
	if err := ledger.Validate(); err != nil {
		return RuntimeExecutionBudgetLedger{}, false, err
	}
	reconciliationID = strings.TrimSpace(reconciliationID)
	ownerResultRef = strings.TrimSpace(ownerResultRef)
	ownerResultHash = strings.TrimSpace(ownerResultHash)
	if reconciliationID == "" || ownerResultRef == "" || ownerResultHash == "" {
		return RuntimeExecutionBudgetLedger{}, false, common.ErrInvalidArgument
	}
	reconciliationIdx := -1
	var record RuntimeBudgetReconciliationRecord
	for idx := range ledger.Reconciliations {
		if ledger.Reconciliations[idx].ReconciliationID == reconciliationID {
			reconciliationIdx = idx
			record = ledger.Reconciliations[idx].Normalized()
			break
		}
	}
	if reconciliationIdx < 0 {
		return RuntimeExecutionBudgetLedger{}, false, common.ErrNotFound
	}
	allocationIdx := -1
	for idx := range ledger.Allocations {
		if ledger.Allocations[idx].AllocationID == record.AllocationID {
			allocationIdx = idx
			break
		}
	}
	if allocationIdx < 0 {
		return RuntimeExecutionBudgetLedger{}, false, common.ErrNotFound
	}
	allocation := ledger.Allocations[allocationIdx].Normalized()
	if record.Status == RuntimeBudgetReconciliationReleased {
		if record.OwnerResultHash == ownerResultHash && allocation.State == RuntimeBudgetAllocationReleased && allocation.OwnerFactRef == ownerResultRef {
			return ledger, false, nil
		}
		return RuntimeExecutionBudgetLedger{}, false, fmt.Errorf("conflicting duplicate runtime budget reconciliation release")
	}
	if (record.Status != RuntimeBudgetReconciliationPending && record.Status != RuntimeBudgetReconciliationStillUnknown) || allocation.State != RuntimeBudgetAllocationUnknown {
		return RuntimeExecutionBudgetLedger{}, false, fmt.Errorf("runtime budget reconciliation state %q cannot release allocation %q", record.Status, allocation.State)
	}
	consumptions := make([]RuntimeBudgetConsumptionFact, 0, len(ledger.Consumptions))
	removed := 0
	for _, fact := range ledger.Consumptions {
		if fact.AllocationID == allocation.AllocationID {
			removed++
			continue
		}
		consumptions = append(consumptions, fact)
	}
	if removed != 1 {
		return RuntimeExecutionBudgetLedger{}, false, fmt.Errorf("runtime budget reconciliation release requires one unknown consumption")
	}
	ledger.Consumptions = consumptions
	allocation.State = RuntimeBudgetAllocationReleased
	allocation.OwnerFactRef = ownerResultRef
	allocation.AllocationHash = StableRuntimeBudgetAllocationHash(allocation)
	ledger.Allocations[allocationIdx] = allocation
	record.Status = RuntimeBudgetReconciliationReleased
	record.OwnerResultHash = ownerResultHash
	record.ReconciliationHash = StableRuntimeBudgetReconciliationHash(record)
	ledger.Reconciliations[reconciliationIdx] = record
	ledger = ledger.Clone()
	if err := ledger.Validate(); err != nil {
		return RuntimeExecutionBudgetLedger{}, false, err
	}
	return ledger, true, nil
}

// FailRuntimeBudgetReconciliation closes a pending reconciliation and seals
// its allocation as exhausted while retaining the unknown consumption
// conservatively. It is used when the wait's deadline or guard is terminal,
// never to recover remaining budget.
func FailRuntimeBudgetReconciliation(ledger RuntimeExecutionBudgetLedger, reconciliationID, ownerResultHash string) (RuntimeExecutionBudgetLedger, bool, error) {
	ledger = ledger.Clone()
	if err := ledger.Validate(); err != nil {
		return RuntimeExecutionBudgetLedger{}, false, err
	}
	reconciliationID = strings.TrimSpace(reconciliationID)
	ownerResultHash = strings.TrimSpace(ownerResultHash)
	if reconciliationID == "" || ownerResultHash == "" {
		return RuntimeExecutionBudgetLedger{}, false, common.ErrInvalidArgument
	}
	for idx := range ledger.Reconciliations {
		record := ledger.Reconciliations[idx].Normalized()
		if record.ReconciliationID != reconciliationID {
			continue
		}
		if record.Status == RuntimeBudgetReconciliationFailed && record.OwnerResultHash == ownerResultHash {
			return ledger, false, nil
		}
		if record.Status != RuntimeBudgetReconciliationPending && record.Status != RuntimeBudgetReconciliationStillUnknown {
			return RuntimeExecutionBudgetLedger{}, false, fmt.Errorf("runtime budget reconciliation state %q cannot fail", record.Status)
		}
		allocationIdx := -1
		for current := range ledger.Allocations {
			if ledger.Allocations[current].AllocationID == record.AllocationID {
				allocationIdx = current
				break
			}
		}
		if allocationIdx < 0 || ledger.Allocations[allocationIdx].State != RuntimeBudgetAllocationUnknown {
			return RuntimeExecutionBudgetLedger{}, false, fmt.Errorf("runtime budget reconciliation failure requires an unknown allocation")
		}
		ledger.Allocations[allocationIdx].State = RuntimeBudgetAllocationExhausted
		ledger.Allocations[allocationIdx].AllocationHash = StableRuntimeBudgetAllocationHash(ledger.Allocations[allocationIdx])
		record.Status = RuntimeBudgetReconciliationFailed
		record.OwnerResultHash = ownerResultHash
		record.ReconciliationHash = StableRuntimeBudgetReconciliationHash(record)
		ledger.Reconciliations[idx] = record
		ledger = ledger.Clone()
		if err := ledger.Validate(); err != nil {
			return RuntimeExecutionBudgetLedger{}, false, err
		}
		return ledger, true, nil
	}
	return RuntimeExecutionBudgetLedger{}, false, common.ErrNotFound
}

func RuntimeBudgetRemaining(ledger RuntimeExecutionBudgetLedger, dimension RuntimeBudgetDimension) (int64, error) {
	dimensionPolicy, found := RuntimeBudgetDimensionPolicyFor(ledger.Policy, dimension)
	if !found {
		return 0, common.ErrInvalidArgument
	}
	var unavailable int64
	for _, allocation := range ledger.Allocations {
		allocation = allocation.Normalized()
		if allocation.Dimension != dimension {
			continue
		}
		switch allocation.State {
		case RuntimeBudgetAllocationReserved:
			unavailable += allocation.ReservedAmount
		case RuntimeBudgetAllocationConfirmed, RuntimeBudgetAllocationUnknown, RuntimeBudgetAllocationExhausted:
			fact := runtimeBudgetConsumptionForAllocation(ledger.Consumptions, allocation.AllocationID)
			unavailable += fact.ConfirmedAmount + fact.UnknownAmount
		}
		if unavailable > dimensionPolicy.Limit {
			return 0, fmt.Errorf("runtime budget dimension %q exceeds its limit", dimension)
		}
	}
	return dimensionPolicy.Limit - unavailable, nil
}

func RuntimeProviderBudgetAllocationSpecs(ledger RuntimeExecutionBudgetLedger, documentKind RuntimeProviderDocumentKind, correctionAttempt, sequence int, logicalRequestID, physicalCallID string) ([]RuntimeBudgetAllocationSpec, error) {
	if correctionAttempt < 0 || correctionAttempt > 1 || sequence <= 0 || strings.TrimSpace(logicalRequestID) == "" || strings.TrimSpace(physicalCallID) == "" {
		return nil, common.ErrInvalidArgument
	}
	dimensions := []RuntimeBudgetDimension{RuntimeBudgetDimensionProviderCall, RuntimeBudgetDimensionProviderInputToken, RuntimeBudgetDimensionProviderOutputToken, RuntimeBudgetDimensionReasoningEffort}
	switch documentKind {
	case RuntimeProviderDocumentSemanticPlan:
		// A whole-document recovery is another physical call for the same
		// sealed logical document. It consumes provider/reasoning resources and
		// the recovery dimension, but it must not count the same semantic
		// generation or correction attempt again.
		if sequence == 1 {
			dimensions = append(dimensions, RuntimeBudgetDimensionSemanticGeneration)
			if correctionAttempt == 1 {
				dimensions = append(dimensions, RuntimeBudgetDimensionSemanticCorrection)
			}
		}
	case RuntimeProviderDocumentOperations:
		if correctionAttempt == 1 && sequence == 1 {
			dimensions = append(dimensions, RuntimeBudgetDimensionCandidateCorrection)
		}
	default:
		return nil, common.ErrInvalidArgument
	}
	if sequence > 1 {
		dimensions = append(dimensions, RuntimeBudgetDimensionWholeDocumentRecovery)
	}
	ret := make([]RuntimeBudgetAllocationSpec, 0, len(dimensions))
	for _, dimension := range dimensions {
		policy, found := RuntimeBudgetDimensionPolicyFor(ledger.Policy, dimension)
		if !found {
			return nil, fmt.Errorf("runtime provider budget dimension %q is absent", dimension)
		}
		amount := int64(1)
		if dimension == RuntimeBudgetDimensionProviderInputToken || dimension == RuntimeBudgetDimensionProviderOutputToken {
			amount = policy.MaxIncrement
		}
		ret = append(ret, RuntimeBudgetAllocationSpec{Dimension: dimension, LogicalConsumptionID: logicalRequestID, ScopeRef: physicalCallID, ReservedAmount: amount})
	}
	return ret, nil
}

type RuntimeExecutionBudgetDimensionView struct {
	Dimension RuntimeBudgetDimension `json:"dimension"`
	Unit      string                 `json:"unit"`
	Limit     int64                  `json:"limit"`
	Confirmed int64                  `json:"confirmed"`
	Reserved  int64                  `json:"reserved"`
	Unknown   int64                  `json:"unknown"`
	Remaining int64                  `json:"remaining"`
	Exhausted bool                   `json:"exhausted"`
}

type RuntimeExecutionBudgetView struct {
	WorkspaceID        common.WorkspaceID                    `json:"workspaceId"`
	RunID              common.RunID                          `json:"runId"`
	PolicyRevision     string                                `json:"policyRevision"`
	StateRevision      uint64                                `json:"stateRevision"`
	Dimensions         []RuntimeExecutionBudgetDimensionView `json:"dimensions"`
	ActiveWaitID       string                                `json:"activeWaitId,omitempty"`
	TerminalReasonCode string                                `json:"terminalReasonCode,omitempty"`
	ViewHash           string                                `json:"viewHash"`
}

// RuntimeExecutionBudgetSourceUsage is a read-only snapshot of consumption
// whose authority already lives in another aggregate sub-ledger. It is used
// only while deriving RuntimeExecutionBudgetView and is never persisted as a
// parallel counter.
type RuntimeExecutionBudgetSourceUsage struct {
	OperationRecoveryAttempts int64
	SameFocusRepairAttempts   int64
}

func RuntimeExecutionBudgetSourceUsageFromFacts(facts RuntimeFacts) RuntimeExecutionBudgetSourceUsage {
	facts = facts.Normalized()
	return RuntimeExecutionBudgetSourceUsage{
		OperationRecoveryAttempts: int64(facts.Recovery.OperationRecoveryAttempts),
		SameFocusRepairAttempts:   int64(facts.Repair.RepairAttempts),
	}
}

func (u RuntimeExecutionBudgetSourceUsage) Validate(policy RuntimeExecutionBudgetPolicy) error {
	values := []struct {
		dimension RuntimeBudgetDimension
		amount    int64
	}{
		{dimension: RuntimeBudgetDimensionOperationRecovery, amount: u.OperationRecoveryAttempts},
		{dimension: RuntimeBudgetDimensionSameFocusRepair, amount: u.SameFocusRepairAttempts},
	}
	for _, value := range values {
		dimension, found := RuntimeBudgetDimensionPolicyFor(policy, value.dimension)
		if !found || value.amount < 0 || value.amount > dimension.Limit {
			return fmt.Errorf("runtime execution budget source usage for %q is out of range", value.dimension)
		}
	}
	return nil
}

func (u RuntimeExecutionBudgetSourceUsage) confirmed(dimension RuntimeBudgetDimension) int64 {
	switch dimension {
	case RuntimeBudgetDimensionOperationRecovery:
		return u.OperationRecoveryAttempts
	case RuntimeBudgetDimensionSameFocusRepair:
		return u.SameFocusRepairAttempts
	default:
		return 0
	}
}

func (v RuntimeExecutionBudgetView) Clone() RuntimeExecutionBudgetView {
	v.Dimensions = append([]RuntimeExecutionBudgetDimensionView(nil), v.Dimensions...)
	return v
}

func BuildRuntimeExecutionBudgetView(workspaceID common.WorkspaceID, runID common.RunID, stateRevision uint64, ledger RuntimeExecutionBudgetLedger, activeWaitID, terminalReasonCode string, sourceUsage ...RuntimeExecutionBudgetSourceUsage) (RuntimeExecutionBudgetView, error) {
	ledger = ledger.Clone()
	if workspaceID == "" || runID == "" || stateRevision == 0 || ledger.Policy.PolicyRevision == "" {
		return RuntimeExecutionBudgetView{}, common.ErrInvalidArgument
	}
	if len(sourceUsage) > 1 {
		return RuntimeExecutionBudgetView{}, common.ErrInvalidArgument
	}
	usage := RuntimeExecutionBudgetSourceUsage{}
	if len(sourceUsage) == 1 {
		usage = sourceUsage[0]
		if err := usage.Validate(ledger.Policy); err != nil {
			return RuntimeExecutionBudgetView{}, err
		}
	}
	view := RuntimeExecutionBudgetView{WorkspaceID: workspaceID, RunID: runID, PolicyRevision: ledger.Policy.PolicyRevision, StateRevision: stateRevision, ActiveWaitID: strings.TrimSpace(activeWaitID), TerminalReasonCode: strings.TrimSpace(terminalReasonCode)}
	for _, dimension := range ledger.Policy.Dimensions {
		item := RuntimeExecutionBudgetDimensionView{Dimension: dimension.Dimension, Unit: dimension.Unit, Limit: dimension.Limit, Confirmed: usage.confirmed(dimension.Dimension)}
		for _, allocation := range ledger.Allocations {
			if allocation.Dimension != dimension.Dimension {
				continue
			}
			switch allocation.State {
			case RuntimeBudgetAllocationReserved:
				item.Reserved += allocation.ReservedAmount
			case RuntimeBudgetAllocationConfirmed:
				item.Confirmed += runtimeBudgetConsumptionForAllocation(ledger.Consumptions, allocation.AllocationID).ConfirmedAmount
			case RuntimeBudgetAllocationUnknown, RuntimeBudgetAllocationExhausted:
				item.Unknown += runtimeBudgetConsumptionForAllocation(ledger.Consumptions, allocation.AllocationID).UnknownAmount
			}
		}
		used := item.Confirmed + item.Reserved + item.Unknown
		if used < 0 || used > item.Limit {
			return RuntimeExecutionBudgetView{}, fmt.Errorf("runtime execution budget view exceeds dimension %q", item.Dimension)
		}
		item.Remaining = item.Limit - used
		item.Exhausted = item.Remaining == 0
		view.Dimensions = append(view.Dimensions, item)
	}
	view.ViewHash = StableRuntimeExecutionBudgetViewHash(view)
	return view, nil
}

func StableRuntimeExecutionBudgetViewHash(view RuntimeExecutionBudgetView) string {
	h := newRuntimeHashWriter()
	writeHashString(h, "runtime-execution-budget-view")
	writeHashString(h, string(view.WorkspaceID))
	writeHashString(h, string(view.RunID))
	writeHashString(h, view.PolicyRevision)
	writeHashString(h, fmt.Sprintf("%d", view.StateRevision))
	for _, dimension := range view.Dimensions {
		writeHashString(h, string(dimension.Dimension))
		writeHashString(h, dimension.Unit)
		writeHashString(h, fmt.Sprintf("%d", dimension.Limit))
		writeHashString(h, fmt.Sprintf("%d", dimension.Confirmed))
		writeHashString(h, fmt.Sprintf("%d", dimension.Reserved))
		writeHashString(h, fmt.Sprintf("%d", dimension.Unknown))
		writeHashString(h, fmt.Sprintf("%d", dimension.Remaining))
		writeHashString(h, fmt.Sprintf("%t", dimension.Exhausted))
	}
	writeHashString(h, strings.TrimSpace(view.ActiveWaitID))
	writeHashString(h, strings.TrimSpace(view.TerminalReasonCode))
	return "runtime-budget-view-" + hex.EncodeToString(h.Sum(nil))[:20]
}
