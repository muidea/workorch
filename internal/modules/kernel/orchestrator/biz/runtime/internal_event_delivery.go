package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// RuntimeInternalDeliveryPayload contains only SM-4 reconciliation, recovery,
// drive and terminal-delivery facts. It cannot rebuild SM-2 authority or carry
// an SM-3 operation executor.
type RuntimeInternalDeliveryPayload struct {
	ReconciliationRequest *runtimecontract.RuntimeCWIReconciliationRequest
	FailureFact           *runtimecontract.FailureFact

	DeliveryAcknowledgement        *runtimecontract.RuntimeDeliveryAcknowledgement
	RecoveryBudgetKind             runtimecontract.RuntimeRecoveryBudgetKind
	DriveLease                     *runtimecontract.RuntimeDriveLease
	RecoverableWait                *runtimecontract.RuntimeRecoverableWaitRecord
	RecoverableWaitID              string
	RecoverableWaitProbeResultHash string
	LeaseGeneration                uint64
	LeaseNow                       string
	TerminationKind                runtimecontract.RuntimeTerminationKind
	RequestedBy                    string
	Reason                         string
}

// SM-4 facts drive persisted delivery/recovery work. They may schedule from
// already persisted authority but cannot rebuild SM-2 or SM-3 state.
const (
	RuntimeInternalEventReconciliationRequested      RuntimeInternalEventKind = "reconciliation_requested"
	RuntimeInternalEventDriveFailureObserved         RuntimeInternalEventKind = "drive_failure_observed"
	RuntimeInternalEventTerminalDeliveryAcknowledged RuntimeInternalEventKind = "terminal_delivery_ack_observed"
	RuntimeInternalEventRecoveryBudgetConsumed       RuntimeInternalEventKind = "recovery_budget_consumed"
	RuntimeInternalEventDriveLeaseScheduled          RuntimeInternalEventKind = "drive_lease_scheduled"
	RuntimeInternalEventDriveLeaseClaimed            RuntimeInternalEventKind = "drive_lease_claimed"
	RuntimeInternalEventDriveLeaseReleased           RuntimeInternalEventKind = "drive_lease_released"
	RuntimeInternalEventDriveLeaseRecovered          RuntimeInternalEventKind = "drive_lease_recovered"
	RuntimeInternalEventRecoverableWaitOpened        RuntimeInternalEventKind = "recoverable_wait_opened"
	RuntimeInternalEventRecoverableWaitProbeDue      RuntimeInternalEventKind = "recoverable_wait_probe_due"
	RuntimeInternalEventRecoverableWaitProbeDeferred RuntimeInternalEventKind = "recoverable_wait_probe_deferred"
	RuntimeInternalEventRecoverableWaitExhausted     RuntimeInternalEventKind = "recoverable_wait_exhausted"
	RuntimeInternalEventRecoverableWaitRetired       RuntimeInternalEventKind = "recoverable_wait_retired"
	RuntimeInternalEventTerminationRequested         RuntimeInternalEventKind = "termination_requested"
)
