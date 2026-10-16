package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// scheduleDriveLease records the only durable authority that may later cause
// an SM-4 recovery drive. A duplicate for the same aggregate revision and
// failure signature is a pure read: it must not create another timer chain.
//
// expectedRunningGeneration is non-zero only when the worker holding that
// exact running lease is atomically rescheduling itself after another failed
// attempt. This generation fence prevents a stale worker (or an unrelated
// duplicate scheduler) from replacing a newer lease while still allowing the
// same retry identity to advance running -> scheduled without a release gap.
// Administrative termination is the one transition that owner-locally
// retires a running lease before that worker returns. If its stale in-flight
// owner delivery then fails, the same scheduling CAS may create only the
// canonical terminal-delivery successor from the already-persisted terminal.
func (c *driveDeliveryComponent) scheduleDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, requested runtimecontract.RuntimeDriveLease, expectedRunningGeneration uint64) (RuntimeAggregate, runtimecontract.RuntimeDriveLease, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, common.ErrInvalidArgument
	}
	requested = requested.Normalized()
	if requested.WorkspaceID != workspaceID || requested.RunID != runID || requested.Reason == "" || requested.FailureSignature == "" || requested.DueAt == "" {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, err
	}
	terminalDelivery := requested.Reason == runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry && requested.FailureSignature == "terminal-delivery"
	if terminalDelivery && TerminalDeliveryFullyAcknowledged(aggregate) {
		// Completion retires only an exact running caller. It neither needs nor
		// mints another recovery authorization, and a stale generation cannot clear
		// a scheduled/newer successor.
		if aggregate.DriveLease == nil {
			return aggregate, runtimecontract.RuntimeDriveLease{}, false, nil
		}
		if aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseRunning &&
			aggregate.DriveLease.RecoveryGeneration == expectedRunningGeneration {
			expectedRevision := aggregate.StateRevision
			aggregate.DriveLease = nil
			aggregate.StateRevision++
			if err := c.validateAggregate(aggregate); err != nil {
				return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, fmt.Errorf("retire fully acknowledged runtime drive lease: %w", err)
			}
			if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
				return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, err
			}
			return aggregate, runtimecontract.RuntimeDriveLease{}, true, nil
		}
		return aggregate, aggregate.DriveLease.Clone(), false, nil
	}
	if requested.Reason == runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry && requested.FailureSignature == "terminal-delivery" {
		authorization := aggregate.Facts.Recovery.Normalized().TerminalDeliveryRequest
		if requested.RecoveryRequestID == "" || authorization.RequestID != requested.RecoveryRequestID ||
			authorization.FailedLeaseGeneration != expectedRunningGeneration {
			return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, fmt.Errorf("%w: terminal delivery lease requires exact persisted recovery request", common.ErrConflict)
		}
	} else if requested.RecoveryRequestID != "" {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, fmt.Errorf("%w: request-bound nonterminal lease requires its atomic recovery event", common.ErrConflict)
	}
	if current := aggregate.DriveLease; current != nil {
		if current.State == runtimecontract.RuntimeDriveLeaseRunning {
			if expectedRunningGeneration != current.RecoveryGeneration {
				return aggregate, current.Clone(), false, nil
			}
			// The exact running worker may renew with the same identity (the
			// lifecycle-delivery case) or replace it with the newly classified
			// retry identity. Both are a single owner-local CAS below.
		} else {
			if expectedRunningGeneration != 0 {
				return aggregate, current.Clone(), false, nil
			}
			// One current scheduled generation already owns the successor. A
			// duplicate reads it back, while a stale/different scheduler cannot
			// replace its exact handoff identity. Only the worker that later claims
			// that generation may atomically renew from running above.
			if current.AggregateRevision == aggregate.StateRevision && current.State == runtimecontract.RuntimeDriveLeaseScheduled {
				return aggregate, current.Clone(), false, nil
			}
		}
	}
	retiredRunningGeneration := aggregate.DriveLease == nil && expectedRunningGeneration != 0 &&
		retiredRunningLeaseMayScheduleTerminalDeliverySuccessor(aggregate, requested, expectedRunningGeneration)
	authorizedMissingTerminalGeneration := aggregate.DriveLease == nil && expectedRunningGeneration != 0 &&
		missingTerminalDeliveryLeaseMayBeMaterialized(aggregate, requested, expectedRunningGeneration)
	if aggregate.DriveLease == nil && expectedRunningGeneration != 0 && !retiredRunningGeneration && !authorizedMissingTerminalGeneration {
		return aggregate, runtimecontract.RuntimeDriveLease{}, false, nil
	}
	expectedRevision := aggregate.StateRevision
	generation := uint64(1)
	if aggregate.DriveLease != nil {
		generation = aggregate.DriveLease.RecoveryGeneration + 1
	} else if retiredRunningGeneration || authorizedMissingTerminalGeneration {
		generation = expectedRunningGeneration + 1
		if generation == 0 {
			return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, fmt.Errorf("%w: Runtime drive lease generation exhausted", common.ErrConflict)
		}
	}
	aggregate.StateRevision++
	lease := requested
	lease.AggregateRevision = aggregate.StateRevision
	lease.RecoveryGeneration = generation
	lease.State = runtimecontract.RuntimeDriveLeaseScheduled
	aggregate.DriveLease = &lease
	if retiredRunningGeneration {
		aggregate.Facts.Recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID = requested.RecoveryRequestID
	}
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, fmt.Errorf("schedule runtime drive lease: %w", err)
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, err
	}
	return aggregate, lease.Clone(), true, nil
}

// missingTerminalDeliveryLeaseMayBeMaterialized covers the crash/stale-claim
// cut after a scheduled terminal generation G=F+1 was cleared because another
// owner acknowledgement advanced the aggregate revision. The current stable
// request R(F), pending delivery and absence of an intervention are sufficient
// durable proof to recreate that same generation; no new budget is consumed.
// The administrative retired-running marker remains a separate authorization
// for creating G+1 after termination retired a running G.
func missingTerminalDeliveryLeaseMayBeMaterialized(aggregate RuntimeAggregate, requested runtimecontract.RuntimeDriveLease, expectedFailedGeneration uint64) bool {
	if aggregate.DriveLease != nil || expectedFailedGeneration == 0 || TerminalDeliveryFullyAcknowledged(aggregate) ||
		requested.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry || requested.FailureSignature != "terminal-delivery" ||
		aggregate.Facts.Recovery.Normalized().TerminalDeliveryIntervention.Validate() == nil {
		return false
	}
	recovery := aggregate.Facts.Recovery.Normalized()
	authorization := recovery.TerminalDeliveryRequest
	if authorization.Validate() != nil || authorization.FailedLeaseGeneration != expectedFailedGeneration ||
		authorization.RequestID != requested.RecoveryRequestID {
		return false
	}
	retired := recovery.RetiredTerminalDeliveryLease
	if !runtimecontract.RuntimeDriveLeasePresent(retired) {
		return true
	}
	if retired.Validate() != nil || retired.State != runtimecontract.RuntimeDriveLeaseRunning ||
		expectedFailedGeneration < retired.RecoveryGeneration {
		return false
	}
	// The retired authority is an exact one-time handoff only for its first
	// successor. Once that marker was consumed, it becomes historical lineage:
	// a later durable request R(G) may rematerialize G+1 after a crash or a
	// concurrent acknowledgement cleared the lease, without pretending the old
	// retired generation itself failed again.
	firstRequestID := runtimecontract.StableRuntimeTerminalDeliveryRecoveryRequestID(authorization.TerminalDeliveryID, retired.RecoveryGeneration)
	return recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID == firstRequestID
}

// retiredRunningLeaseMayScheduleTerminalDeliverySuccessor is deliberately
// narrower than general terminal detection. Only administrative termination
// clears a concurrently running generation as part of its owner-private CAS;
// the returning worker may therefore replace that retired authority solely
// with the bounded terminal-delivery identity. All other missing-generation
// requests remain stale no-ops.
func retiredRunningLeaseMayScheduleTerminalDeliverySuccessor(aggregate RuntimeAggregate, requested runtimecontract.RuntimeDriveLease, expectedRunningGeneration uint64) bool {
	retired := aggregate.Facts.Recovery.Normalized().RetiredTerminalDeliveryLease
	return aggregate.DriveLease == nil && AdministrativeTerminationAwaitsTerminalDelivery(aggregate) && expectedRunningGeneration != 0 &&
		requested.Reason == runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry &&
		requested.FailureSignature == "terminal-delivery" && retired.Validate() == nil &&
		retired.State == runtimecontract.RuntimeDriveLeaseRunning && retired.RecoveryGeneration == expectedRunningGeneration &&
		aggregate.Facts.Recovery.Normalized().RetiredTerminalDeliveryLeaseRecoveryRequestID == ""
}

// claimDriveLease consumes a due persisted schedule. A timer never creates a
// retry: it may only move the exact lease it was given from scheduled to
// running. A stale or early timer performs no drive and no side effect.
func (c *driveDeliveryComponent) claimDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, requested runtimecontract.RuntimeDriveLease, now string) (RuntimeAggregate, runtimecontract.RuntimeDriveLease, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, common.ErrInvalidArgument
	}
	requested = requested.Normalized()
	if requested.WorkspaceID != workspaceID || requested.RunID != runID || requested.Validate() != nil || strings.TrimSpace(now) == "" {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, common.ErrInvalidArgument
	}
	if _, err := runtimeDriveLeaseTime(now); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, err
	}
	current := aggregate.DriveLease
	if current == nil || current.State != runtimecontract.RuntimeDriveLeaseScheduled {
		return aggregate, runtimecontract.RuntimeDriveLease{}, false, nil
	}
	if !runtimecontract.SameRuntimeDriveLeaseIdentity(*current, requested) {
		// A diagnostic owner transition may atomically reissue the same stable
		// successor at a newer generation. The older timer cannot claim it, but it
		// may hand the authoritative current generation back to the sole gate owner
		// for re-mounting. Unrelated or future timer identities remain inert.
		if requested.RecoveryGeneration < current.RecoveryGeneration &&
			requested.WorkspaceID == current.WorkspaceID && requested.RunID == current.RunID &&
			requested.Reason == current.Reason && requested.FailureSignature == current.FailureSignature {
			return aggregate, current.Clone(), false, nil
		}
		return aggregate, runtimecontract.RuntimeDriveLease{}, false, nil
	}
	if aggregate.StateRevision != current.AggregateRevision {
		// The lease belongs to an older aggregate. Consume only the stale local
		// schedule; a later owner-local transition may establish its own lease.
		expectedRevision := aggregate.StateRevision
		aggregate.DriveLease = nil
		aggregate.StateRevision++
		if err := c.validateAggregate(aggregate); err != nil {
			return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, fmt.Errorf("expire stale runtime drive lease: %w", err)
		}
		if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
			return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, err
		}
		return aggregate, runtimecontract.RuntimeDriveLease{}, false, nil
	}
	due, _ := runtimeDriveLeaseTime(current.DueAt)
	nowTime, _ := runtimeDriveLeaseTime(now)
	if nowTime.Before(due) {
		return aggregate, current.Clone(), false, nil
	}
	expectedRevision := aggregate.StateRevision
	aggregate.StateRevision++
	claimed := current.Clone()
	claimed.State = runtimecontract.RuntimeDriveLeaseRunning
	claimed.AggregateRevision = aggregate.StateRevision
	aggregate.DriveLease = &claimed
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, fmt.Errorf("claim runtime drive lease: %w", err)
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, err
	}
	return aggregate, claimed.Clone(), true, nil
}

// releaseDriveLease clears the exact scheduled or running lease after its
// authority is no longer actionable. It is generation-fenced, so an old
// worker or timer cannot remove a newer retry schedule.
func (c *driveDeliveryComponent) releaseDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, generation uint64) (RuntimeAggregate, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || generation == 0 {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	if aggregate.DriveLease == nil || aggregate.DriveLease.RecoveryGeneration != generation {
		return aggregate, false, nil
	}
	expectedRevision := aggregate.StateRevision
	aggregate.DriveLease = nil
	aggregate.StateRevision++
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, false, fmt.Errorf("release runtime drive lease: %w", err)
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, false, err
	}
	return aggregate, true, nil
}

// recoverDriveLease converts an interrupted running lease into a new scheduled
// generation. It is called only by startup recovery after the aggregate was
// loaded; a live timer never guesses this transition.
func (c *driveDeliveryComponent) recoverDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dueAt string) (RuntimeAggregate, runtimecontract.RuntimeDriveLease, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(dueAt) == "" {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, common.ErrInvalidArgument
	}
	if _, err := runtimeDriveLeaseTime(dueAt); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, err
	}
	if aggregate.DriveLease == nil {
		return aggregate, runtimecontract.RuntimeDriveLease{}, false, nil
	}
	if aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseScheduled {
		return aggregate, aggregate.DriveLease.Clone(), false, nil
	}
	expectedRevision := aggregate.StateRevision
	aggregate.StateRevision++
	lease := aggregate.DriveLease.Clone()
	lease.State = runtimecontract.RuntimeDriveLeaseScheduled
	lease.AggregateRevision = aggregate.StateRevision
	lease.RecoveryGeneration++
	lease.DueAt = strings.TrimSpace(dueAt)
	aggregate.DriveLease = &lease
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, fmt.Errorf("recover runtime drive lease: %w", err)
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, err
	}
	return aggregate, lease.Clone(), true, nil
}

func runtimeDriveLeaseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("runtime drive lease timestamp: %w", err)
	}
	return parsed, nil
}
