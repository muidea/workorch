package workorchruntimecontract

import "strings"

// Extended failure taxonomy for RVC-R5/R13. Existing FailureClass values remain valid.
// New classes separate provider, owner, CAS, contract, and delivery failures so budgets
// and reducer disposition stay independent (INV budgets).
const (
	FailureClassCandidateSchemaFailure   FailureClass = "candidate_schema_failure"
	FailureClassCandidateContractFailure FailureClass = "candidate_contract_failure"
	// FailureClassCandidateInfeasible is a typed owner feasibility rejection.
	// It never by itself authorizes an SM-2 state-changing repair; that requires
	// a current owner-grounded failure target in the Runtime aggregate.
	FailureClassCandidateInfeasible       FailureClass = "candidate_infeasible"
	FailureClassProviderTransient         FailureClass = "provider_transient"
	FailureClassProviderAuthentication    FailureClass = "provider_authentication"
	FailureClassProviderConfiguration     FailureClass = "provider_configuration"
	FailureClassCapabilityUnavailable     FailureClass = "capability_unavailable"
	FailureClassOwnerTransientUnavailable FailureClass = "owner_transient_unavailable"
	FailureClassExecutionRejected         FailureClass = "execution_rejected"
	FailureClassOperationOutcomeUnknown   FailureClass = "operation_outcome_unknown"
	FailureClassGovernanceRequired        FailureClass = "governance_required"
	FailureClassCASConflict               FailureClass = "cas_conflict"
	FailureClassRuntimeInvariantDefect    FailureClass = "runtime_invariant_defect"
	FailureClassProjectionDeliveryFailure FailureClass = "projection_delivery_failure"
	FailureClassEffectMismatch            FailureClass = "effect_mismatch"
	FailureClassExecutionBudgetExhausted  FailureClass = "execution_budget_exhausted"
	FailureClassRecoverableWaitExpired    FailureClass = "recoverable_wait_expired"
	FailureClassRecoverableWaitExhausted  FailureClass = "recoverable_wait_exhausted"
)

// NormalizeFailureClass trims the class string.
func NormalizeFailureClass(class FailureClass) FailureClass {
	return FailureClass(strings.TrimSpace(string(class)))
}
