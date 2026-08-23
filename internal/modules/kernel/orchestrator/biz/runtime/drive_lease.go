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
	} else if expectedRunningGeneration != 0 {
		return aggregate, runtimecontract.RuntimeDriveLease{}, false, nil
	}
	expectedRevision := aggregate.StateRevision
	generation := uint64(1)
	if aggregate.DriveLease != nil {
		generation = aggregate.DriveLease.RecoveryGeneration + 1
	}
	aggregate.StateRevision++
	lease := requested
	lease.AggregateRevision = aggregate.StateRevision
	lease.RecoveryGeneration = generation
	lease.State = runtimecontract.RuntimeDriveLeaseScheduled
	aggregate.DriveLease = &lease
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, fmt.Errorf("schedule runtime drive lease: %w", err)
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, err
	}
	return aggregate, lease.Clone(), true, nil
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
		if err := aggregate.Validate(); err != nil {
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
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, fmt.Errorf("claim runtime drive lease: %w", err)
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeDriveLease{}, false, err
	}
	return aggregate, claimed.Clone(), true, nil
}

// releaseDriveLease clears the running lease after its drive returned. It is
// generation-fenced, so an old worker cannot remove a newer retry schedule.
func (c *driveDeliveryComponent) releaseDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, generation uint64) (RuntimeAggregate, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || generation == 0 {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	if aggregate.DriveLease == nil || aggregate.DriveLease.State != runtimecontract.RuntimeDriveLeaseRunning || aggregate.DriveLease.RecoveryGeneration != generation {
		return aggregate, false, nil
	}
	expectedRevision := aggregate.StateRevision
	aggregate.DriveLease = nil
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
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
	if err := aggregate.Validate(); err != nil {
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
