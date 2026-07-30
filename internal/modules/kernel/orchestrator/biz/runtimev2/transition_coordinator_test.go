package runtimev2

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

type transitionCoordinatorRepository struct {
	saves             int
	lastAggregate     RuntimeAggregate
	lastExpected      uint64
	saveErr           error
	mutateBeforeError bool
}

func (r *transitionCoordinatorRepository) Load(context.Context, common.WorkspaceID, common.RunID) (RuntimeAggregate, error) {
	return RuntimeAggregate{}, common.ErrNotFound
}

func (r *transitionCoordinatorRepository) Save(_ context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	r.saves++
	if r.saveErr != nil && !r.mutateBeforeError {
		return r.saveErr
	}
	r.lastAggregate = aggregate
	r.lastExpected = expectedRevision
	return r.saveErr
}

func TestRuntimeTransitionCoordinatorRequiresExactNextRevision(t *testing.T) {
	repository := &transitionCoordinatorRepository{}
	coordinator, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := stateChangeAggregate(t)

	for _, revision := range []uint64{aggregate.StateRevision - 1, aggregate.StateRevision + 1} {
		candidate := aggregate
		candidate.StateRevision = revision
		err := coordinator.Commit(context.Background(), candidate, aggregate.StateRevision-1)
		if !errors.Is(err, common.ErrConflict) {
			t.Fatalf("revision %d error = %v, want conflict", revision, err)
		}
	}
	if repository.saves != 0 {
		t.Fatalf("invalid revisions reached repository Save %d times", repository.saves)
	}
}

func TestRuntimeTransitionCoordinatorRejectsInvalidAggregateBeforeCAS(t *testing.T) {
	repository := &transitionCoordinatorRepository{}
	coordinator, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := stateChangeAggregate(t)
	aggregate.Decision.AuthoritySnapshot.RunID = "different-run"

	if err := coordinator.Commit(context.Background(), aggregate, aggregate.StateRevision-1); err == nil {
		t.Fatal("invalid aggregate was accepted")
	}
	if repository.saves != 0 {
		t.Fatalf("invalid aggregate reached repository Save %d times", repository.saves)
	}
}

func TestRuntimeTransitionCoordinatorDoesNotPromoteUntypedConflictToCAS(t *testing.T) {
	repository := &transitionCoordinatorRepository{saveErr: common.ErrConflict}
	coordinator, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := stateChangeAggregate(t)

	err = coordinator.Commit(context.Background(), aggregate, aggregate.StateRevision-1)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Commit error = %v, want conflict", err)
	}
	var conflict *RuntimeCASConflictError
	if errors.As(err, &conflict) {
		t.Fatalf("untyped conflict was incorrectly promoted to CAS conflict: %+v", conflict)
	}
	if repository.saves != 1 {
		t.Fatalf("repository Save calls = %d, want 1", repository.saves)
	}
	if repository.lastAggregate.StateRevision != 0 {
		t.Fatalf("repository exposed partial committed state: %+v", repository.lastAggregate)
	}
}

func TestRuntimeTransitionCoordinatorCommitsValidatedAggregateOnce(t *testing.T) {
	repository := &transitionCoordinatorRepository{}
	coordinator, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := stateChangeAggregate(t)

	if err := coordinator.Commit(context.Background(), aggregate, aggregate.StateRevision-1); err != nil {
		t.Fatal(err)
	}
	if repository.saves != 1 || repository.lastExpected != aggregate.StateRevision-1 {
		t.Fatalf("Save calls=%d expectedRevision=%d", repository.saves, repository.lastExpected)
	}
	if repository.lastAggregate.StateRevision != aggregate.StateRevision {
		t.Fatalf("saved aggregate revision=%d, want %d", repository.lastAggregate.StateRevision, aggregate.StateRevision)
	}
}
