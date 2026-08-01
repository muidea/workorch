package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ScheduleDriveLease records the only durable authority that may later cause
// an SM-4 recovery drive. A duplicate for the same aggregate revision and
// failure signature is a pure read: it must not create another timer chain.
func (c *RuntimeController) ScheduleDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, requested runtimecontract.RuntimeDriveLease) (RuntimeAggregate, runtimecontract.RuntimeDriveLease, bool, error) {
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
	if current := aggregate.DriveLease; current != nil &&
		current.AggregateRevision == aggregate.StateRevision &&
		current.Reason == requested.Reason &&
		current.FailureSignature == requested.FailureSignature &&
		(current.State == runtimecontract.RuntimeDriveLeaseScheduled || current.State == runtimecontract.RuntimeDriveLeaseRunning) {
		return aggregate, current.Clone(), false, nil
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

// ClaimDriveLease consumes a due persisted schedule. A timer never creates a
// retry: it may only move the exact lease it was given from scheduled to
// running. A stale or early timer performs no drive and no side effect.
func (c *RuntimeController) ClaimDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, requested runtimecontract.RuntimeDriveLease, now string) (RuntimeAggregate, runtimecontract.RuntimeDriveLease, bool, error) {
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
	if current == nil || current.State != runtimecontract.RuntimeDriveLeaseScheduled || !runtimecontract.SameRuntimeDriveLeaseIdentity(*current, requested) {
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

// ReleaseDriveLease clears the running lease after its drive returned. It is
// generation-fenced, so an old worker cannot remove a newer retry schedule.
func (c *RuntimeController) ReleaseDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, generation uint64) (RuntimeAggregate, bool, error) {
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

// RecoverDriveLease converts an interrupted running lease into a new scheduled
// generation. It is called only by startup recovery after the aggregate was
// loaded; a live timer never guesses this transition.
func (c *RuntimeController) RecoverDriveLease(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dueAt string) (RuntimeAggregate, runtimecontract.RuntimeDriveLease, bool, error) {
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
