package workorchruntimecontract

import "strings"

// Extended failure taxonomy for RVC-R5/R13. Existing FailureClass values remain valid.
// New classes separate provider, owner, CAS, contract, and delivery failures so budgets
// and reducer disposition stay independent (INV budgets).
const (
	FailureClassCandidateSchemaFailure   FailureClass = "candidate_schema_failure"
	FailureClassCandidateContractFailure FailureClass = "candidate_contract_failure"
	// FailureClassCandidateInfeasible is a typed owner feasibility rejection
	// promoted by the orchestrator only when activation policy requires a
	// same-focus repair instead of another candidate under the old authority.
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
)

// RuntimeFailureDisposition is the reducer/drive disposition for a failure class.
type RuntimeFailureDisposition string

const (
	RuntimeFailureDispositionSameContractRetry RuntimeFailureDisposition = "same_contract_retry"
	RuntimeFailureDispositionProviderRetry     RuntimeFailureDisposition = "provider_transport_retry"
	RuntimeFailureDispositionConfigurationGov  RuntimeFailureDisposition = "configuration_governance"
	RuntimeFailureDispositionGovernance        RuntimeFailureDisposition = "governance"
	RuntimeFailureDispositionSameFocusRepair   RuntimeFailureDisposition = "same_focus_repair"
	RuntimeFailureDispositionOperationRecovery RuntimeFailureDisposition = "operation_recovery"
	RuntimeFailureDispositionCASReload         RuntimeFailureDisposition = "cas_reload"
	RuntimeFailureDispositionDeliveryRetry     RuntimeFailureDisposition = "delivery_retry"
	RuntimeFailureDispositionBoundedFail       RuntimeFailureDisposition = "bounded_fail"
	RuntimeFailureDispositionTerminalDefect    RuntimeFailureDisposition = "terminal_contract_defect"
)

// FailureClassDisposition returns the canonical reducer disposition for a failure class.
func FailureClassDisposition(class FailureClass) RuntimeFailureDisposition {
	switch FailureClass(strings.TrimSpace(string(class))) {
	case FailureClassCandidateSchemaFailure, FailureClassCandidateContractFailure,
		FailureClassCandidateExhausted:
		return RuntimeFailureDispositionSameContractRetry
	case FailureClassProviderTransient:
		return RuntimeFailureDispositionProviderRetry
	case FailureClassProviderAuthentication, FailureClassProviderConfiguration:
		return RuntimeFailureDispositionConfigurationGov
	case FailureClassCapabilityUnavailable, FailureClassGovernanceRequired,
		FailureClassGovernanceDependency, FailureClassGovernanceRejected:
		return RuntimeFailureDispositionGovernance
	case FailureClassOwnerTransientUnavailable:
		return RuntimeFailureDispositionOperationRecovery
	case FailureClassCandidateInfeasible, FailureClassExecutionRejected, FailureClassExecutionFailure,
		FailureClassBuildFailure, FailureClassEffectMismatch:
		return RuntimeFailureDispositionSameFocusRepair
	case FailureClassOperationOutcomeUnknown:
		return RuntimeFailureDispositionGovernance
	case FailureClassCASConflict:
		return RuntimeFailureDispositionCASReload
	case FailureClassProjectionDeliveryFailure:
		return RuntimeFailureDispositionDeliveryRetry
	case FailureClassRuntimeInvariantDefect, FailureClassRuntimeContractMisalignment:
		return RuntimeFailureDispositionTerminalDefect
	case FailureClassNoProgressExhausted, FailureClassCoverageAssessmentIncomplete:
		return RuntimeFailureDispositionBoundedFail
	default:
		return RuntimeFailureDispositionBoundedFail
	}
}

// FailureClassIsBusinessTerminal reports whether the class may terminal a business Run
// after budgets are exhausted (CAS/owner transient/delivery must not).
func FailureClassIsBusinessTerminal(class FailureClass) bool {
	switch FailureClassDisposition(class) {
	case RuntimeFailureDispositionCASReload, RuntimeFailureDispositionDeliveryRetry,
		RuntimeFailureDispositionProviderRetry, RuntimeFailureDispositionOperationRecovery:
		return false
	default:
		return true
	}
}

// NormalizeFailureClass trims the class string.
func NormalizeFailureClass(class FailureClass) FailureClass {
	return FailureClass(strings.TrimSpace(string(class)))
}
