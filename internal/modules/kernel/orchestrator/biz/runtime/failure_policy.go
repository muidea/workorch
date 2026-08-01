package runtime

import (
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// FailureDisposition is the Orchestrator owner-local state-machine policy for
// a typed failure fact. It is intentionally not a shared runtime contract:
// capability owners report facts, while SM-2/SM-4 choose recovery, governance
// and terminal transitions.
type FailureDisposition string

const (
	FailureDispositionSameContractRetry FailureDisposition = "same_contract_retry"
	FailureDispositionProviderRetry     FailureDisposition = "provider_transport_retry"
	FailureDispositionConfigurationGov  FailureDisposition = "configuration_governance"
	FailureDispositionGovernance        FailureDisposition = "governance"
	FailureDispositionSameFocusRepair   FailureDisposition = "same_focus_repair"
	FailureDispositionOperationRecovery FailureDisposition = "operation_recovery"
	FailureDispositionCASReload         FailureDisposition = "cas_reload"
	FailureDispositionDeliveryRetry     FailureDisposition = "delivery_retry"
	FailureDispositionBoundedFail       FailureDisposition = "bounded_fail"
	FailureDispositionTerminalDefect    FailureDisposition = "terminal_contract_defect"
)

// FailureClassDisposition is the sole owner-local mapping from a factual
// failure class to an SM-2/SM-4 disposition. Unknown classes fail closed as a
// bounded terminal path; they never acquire an implicit retry budget.
func FailureClassDisposition(class runtimecontract.FailureClass) FailureDisposition {
	switch runtimecontract.FailureClass(strings.TrimSpace(string(class))) {
	case runtimecontract.FailureClassCandidateSchemaFailure, runtimecontract.FailureClassCandidateContractFailure,
		runtimecontract.FailureClassCandidateExhausted:
		return FailureDispositionSameContractRetry
	case runtimecontract.FailureClassProviderTransient:
		return FailureDispositionProviderRetry
	case runtimecontract.FailureClassProviderAuthentication, runtimecontract.FailureClassProviderConfiguration:
		return FailureDispositionConfigurationGov
	case runtimecontract.FailureClassCapabilityUnavailable, runtimecontract.FailureClassGovernanceRequired,
		runtimecontract.FailureClassGovernanceDependency, runtimecontract.FailureClassGovernanceRejected:
		return FailureDispositionGovernance
	case runtimecontract.FailureClassOwnerTransientUnavailable:
		return FailureDispositionOperationRecovery
	case runtimecontract.FailureClassCandidateInfeasible, runtimecontract.FailureClassExecutionRejected, runtimecontract.FailureClassExecutionFailure,
		runtimecontract.FailureClassBuildFailure, runtimecontract.FailureClassEffectMismatch:
		return FailureDispositionSameFocusRepair
	case runtimecontract.FailureClassOperationOutcomeUnknown:
		// An unknown owner outcome is not an external dependency. Only a separately
		// sealed dependency may enter governance.
		return FailureDispositionBoundedFail
	case runtimecontract.FailureClassCASConflict:
		return FailureDispositionCASReload
	case runtimecontract.FailureClassProjectionDeliveryFailure:
		return FailureDispositionDeliveryRetry
	case runtimecontract.FailureClassRuntimeInvariantDefect, runtimecontract.FailureClassRuntimeContractMisalignment:
		return FailureDispositionTerminalDefect
	case runtimecontract.FailureClassNoProgressExhausted, runtimecontract.FailureClassCoverageAssessmentIncomplete:
		return FailureDispositionBoundedFail
	default:
		return FailureDispositionBoundedFail
	}
}

// FailureClassIsBusinessTerminal reports whether a drive failure can become a
// business Run terminal. CAS/transport/operation recovery/delivery failures
// remain owner-local re-drive work regardless of their recovery ledger state.
func FailureClassIsBusinessTerminal(class runtimecontract.FailureClass) bool {
	switch FailureClassDisposition(class) {
	case FailureDispositionCASReload, FailureDispositionDeliveryRetry,
		FailureDispositionProviderRetry, FailureDispositionOperationRecovery:
		return false
	default:
		return true
	}
}
