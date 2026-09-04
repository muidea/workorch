package workorchruntimecontract

import (
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeDriveLeaseState is the persisted SM-4 execution permission state.
// It is an owner-neutral DTO: only kernel/orchestrator decides its transition.
type RuntimeDriveLeaseState string

const (
	RuntimeDriveLeaseScheduled RuntimeDriveLeaseState = "scheduled"
	RuntimeDriveLeaseRunning   RuntimeDriveLeaseState = "running"
)

// RuntimeDriveLeaseReason is a stable recovery category. It deliberately
// preserves neither a raw error message nor a provider/owner-specific retry
// decision; the original diagnostic remains in its producing fact lineage.
type RuntimeDriveLeaseReason string

const (
	RuntimeDriveLeaseReasonPersistedPending RuntimeDriveLeaseReason = "persisted_pending"
	RuntimeDriveLeaseReasonCASReload        RuntimeDriveLeaseReason = "cas_reload"
	RuntimeDriveLeaseReasonOwnerTransient   RuntimeDriveLeaseReason = "owner_transient"
	RuntimeDriveLeaseReasonProviderRetry    RuntimeDriveLeaseReason = "provider_retry"
	RuntimeDriveLeaseReasonOperationRetry   RuntimeDriveLeaseReason = "operation_retry"
	RuntimeDriveLeaseReasonDeliveryRetry    RuntimeDriveLeaseReason = "delivery_retry"
	RuntimeDriveLeaseReasonScheduleRetry    RuntimeDriveLeaseReason = "schedule_retry"
	RuntimeDriveLeaseReasonRecoverableWait  RuntimeDriveLeaseReason = "recoverable_wait"
)

// RuntimeDriveLease is the Orchestrator-owned, persisted SM-4 retry identity.
// A process-local singleflight key or timer is only an optimization for this
// record and cannot create a retry independently.
type RuntimeDriveLease struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	AggregateRevision  uint64
	RecoveryGeneration uint64
	Reason             RuntimeDriveLeaseReason
	FailureSignature   string
	// RecoveryRequestID binds a budget-authorized successor back to the exact
	// persisted recovery record that created it. Canonical terminal-delivery,
	// CAS-reload and owner-transient successors require it; unrelated technical
	// lease reasons leave it empty.
	RecoveryRequestID string
	DueAt             string
	State             RuntimeDriveLeaseState
}

func (l RuntimeDriveLease) Normalized() RuntimeDriveLease {
	l.FailureSignature = strings.TrimSpace(l.FailureSignature)
	l.RecoveryRequestID = strings.TrimSpace(l.RecoveryRequestID)
	l.DueAt = strings.TrimSpace(l.DueAt)
	return l
}

func (l RuntimeDriveLease) Validate() error {
	l = l.Normalized()
	if l.WorkspaceID == "" || l.RunID == "" || l.AggregateRevision == 0 || l.RecoveryGeneration == 0 || l.FailureSignature == "" {
		return fmt.Errorf("runtime drive lease requires scope, revision, generation and failure signature")
	}
	switch l.Reason {
	case RuntimeDriveLeaseReasonPersistedPending,
		RuntimeDriveLeaseReasonCASReload,
		RuntimeDriveLeaseReasonOwnerTransient,
		RuntimeDriveLeaseReasonProviderRetry,
		RuntimeDriveLeaseReasonOperationRetry,
		RuntimeDriveLeaseReasonDeliveryRetry,
		RuntimeDriveLeaseReasonScheduleRetry,
		RuntimeDriveLeaseReasonRecoverableWait:
	default:
		return fmt.Errorf("runtime drive lease has unsupported reason %q", l.Reason)
	}
	switch l.State {
	case RuntimeDriveLeaseScheduled, RuntimeDriveLeaseRunning:
	default:
		return fmt.Errorf("runtime drive lease has unsupported state %q", l.State)
	}
	canonicalTerminalDelivery := l.Reason == RuntimeDriveLeaseReasonDeliveryRetry && l.FailureSignature == "terminal-delivery"
	nonterminalRecovery := l.Reason == RuntimeDriveLeaseReasonCASReload || l.Reason == RuntimeDriveLeaseReasonOwnerTransient
	if l.RecoveryRequestID != "" && !canonicalTerminalDelivery && !nonterminalRecovery {
		return fmt.Errorf("runtime drive lease recovery request has unsupported reason")
	}
	if nonterminalRecovery && l.RecoveryRequestID == "" {
		return fmt.Errorf("runtime nonterminal recovery drive lease requires recovery request")
	}
	if _, err := time.Parse(time.RFC3339Nano, l.DueAt); err != nil {
		return fmt.Errorf("runtime drive lease dueAt must be RFC3339: %w", err)
	}
	return nil
}

func (l RuntimeDriveLease) Clone() RuntimeDriveLease { return l.Normalized() }

// RuntimeDriveLeasePresent is the canonical full-struct presence test for an
// embedded (non-pointer) lease marker. Checking only selected fields can make
// a partially populated retired authority disappear from validation.
func RuntimeDriveLeasePresent(lease RuntimeDriveLease) bool {
	return lease.Normalized() != (RuntimeDriveLease{})
}

// SameRuntimeDriveLeaseIdentity compares the immutable retry identity used by
// a timer before it may claim the persisted lease. State is intentionally not
// included: a successful claim changes scheduled -> running at a new revision.
func SameRuntimeDriveLeaseIdentity(left, right RuntimeDriveLease) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.WorkspaceID == right.WorkspaceID &&
		left.RunID == right.RunID &&
		left.AggregateRevision == right.AggregateRevision &&
		left.RecoveryGeneration == right.RecoveryGeneration &&
		left.Reason == right.Reason &&
		left.FailureSignature == right.FailureSignature &&
		left.RecoveryRequestID == right.RecoveryRequestID &&
		left.DueAt == right.DueAt
}
