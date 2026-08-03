package runtime

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
	RuntimeInternalEventTerminationRequested         RuntimeInternalEventKind = "termination_requested"
)
