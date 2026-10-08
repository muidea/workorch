package runtime

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
	previous          *RuntimeAggregate
	loadErr           error
	loads             int
}

type transitionCoordinatorValidatedRepository struct {
	transitionCoordinatorRepository
	validatedSaves int
}

func (r *transitionCoordinatorValidatedRepository) SaveValidated(_ context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	r.validatedSaves++
	r.lastAggregate = aggregate
	r.lastExpected = expectedRevision
	return r.saveErr
}

func (r *transitionCoordinatorRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	r.loads++
	if r.loadErr != nil {
		return RuntimeAggregate{}, r.loadErr
	}
	if r.previous == nil {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	return *r.previous, nil
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

func TestRuntimeTransitionCoordinatorRejectsRevisionOverflowBeforeCAS(t *testing.T) {
	repository := &transitionCoordinatorRepository{}
	coordinator, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := stateChangeAggregate(t)
	aggregate.StateRevision = 0
	if err := coordinator.Commit(context.Background(), aggregate, ^uint64(0)); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("revision 溢出必须拒绝: %v", err)
	}
	if repository.saves != 0 {
		t.Fatalf("revision 溢出触发了持久化: %d", repository.saves)
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

func TestRuntimeTransitionCoordinatorRequiresIndependentScopedPreimage(t *testing.T) {
	for _, scenario := range []string{"missing", "scope", "revision", "invariant", "valid"} {
		t.Run(scenario, func(t *testing.T) {
			previous := stateChangeAggregate(t)
			next := previous
			next.StateRevision++
			repository := &transitionCoordinatorRepository{previous: &previous}
			switch scenario {
			case "missing":
				repository.previous = nil
			case "scope":
				previous.Facts.RunID = "wrong-run"
			case "revision":
				previous.StateRevision++
			case "invariant":
				previous.Decision.AuthoritySnapshot.RunID = "wrong-run"
			}
			coordinator, err := NewRuntimeTransitionCoordinator(repository)
			if err != nil {
				t.Fatal(err)
			}
			err = coordinator.Commit(context.Background(), next, next.StateRevision-1)
			if scenario == "valid" {
				if err != nil || repository.saves != 1 || repository.lastExpected != next.StateRevision-1 {
					t.Fatalf("有效前像未精确提交一次: err=%v saves=%d", err, repository.saves)
				}
			} else {
				if err == nil || repository.saves != 0 {
					t.Fatalf("无效前像进入 CAS: err=%v saves=%d", err, repository.saves)
				}
				switch scenario {
				case "missing":
					if !errors.Is(err, common.ErrNotFound) {
						t.Fatalf("Load 缺失未原样传播: %v", err)
					}
				case "scope":
					if !errors.Is(err, common.ErrConflict) {
						t.Fatalf("跨 scope 前像未拒绝: %v", err)
					}
				case "revision":
					var conflict *RuntimeCASConflictError
					if !errors.As(err, &conflict) || conflict.ExpectedRevision != next.StateRevision-1 || conflict.ObservedRevision != previous.StateRevision {
						t.Fatalf("独立 revision conflict 缺精确 expected/observed: %v", err)
					}
				}
			}
			if repository.loads != 1 {
				t.Fatalf("独立前像 Load 次数=%d", repository.loads)
			}
		})
	}
}

func TestRuntimeTransitionCoordinatorUsesValidatedRepositoryOnlyAfterInvariantCheck(t *testing.T) {
	repository := &transitionCoordinatorValidatedRepository{}
	coordinator, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := stateChangeAggregate(t)

	invalid := aggregate
	invalid.Decision.AuthoritySnapshot.RunID = "different-run"
	if err := coordinator.Commit(context.Background(), invalid, invalid.StateRevision-1); err == nil {
		t.Fatal("invalid aggregate was accepted")
	}
	if repository.validatedSaves != 0 || repository.saves != 0 {
		t.Fatalf("invalid aggregate reached repository: validated=%d ordinary=%d", repository.validatedSaves, repository.saves)
	}

	if err := coordinator.Commit(context.Background(), aggregate, aggregate.StateRevision-1); err != nil {
		t.Fatal(err)
	}
	if repository.validatedSaves != 1 || repository.saves != 0 {
		t.Fatalf("save paths: validated=%d ordinary=%d", repository.validatedSaves, repository.saves)
	}
}
