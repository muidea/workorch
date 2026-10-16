package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
)

// Keep the actual complete-load path while faulting only the CAS return lane.
type runtimePreimageOwnerFaultRepository struct {
	runtimeStateRepository
	after bool
	err   error
}

type runtimePreimageOwnerInterleavedRepository struct {
	runtimeStateRepository
	concurrent runtime.RuntimeAggregate
}

func (r runtimePreimageOwnerInterleavedRepository) LoadTransitionPreimage(ctx context.Context, request runtime.RuntimeTransitionPreimageRequest) (runtime.RuntimeTransitionPreimage, error) {
	product, err := r.runtimeStateRepository.LoadTransitionPreimage(ctx, request)
	if err != nil {
		return runtime.RuntimeTransitionPreimage{}, err
	}
	// A separate successful owner CAS lands after the original complete read.
	if err := r.runtimeStateRepository.Save(ctx, r.concurrent, r.concurrent.StateRevision-1); err != nil {
		return runtime.RuntimeTransitionPreimage{}, err
	}
	return product, nil
}

func (r runtimePreimageOwnerFaultRepository) SaveValidated(ctx context.Context, aggregate runtime.RuntimeAggregate, expected uint64) error {
	if !r.after {
		return r.err
	}
	if err := r.runtimeStateRepository.SaveValidated(ctx, aggregate, expected); err != nil {
		return err
	}
	return r.err
}

func TestRuntimeTransitionPreimageRealOwnerCASRecovery(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_cas", true: "after_cas_response_loss"}[after], func(t *testing.T) {
			ctx := context.Background()
			runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
			if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
				t.Fatal(err)
			}
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			module := runmodule.New()
			if err := module.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
				t.Fatal(err)
			}
			if err := module.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = module.Stop(ctx) })
			repository := runtimeStateRepository{hub: hub}
			before := runtimeAggregate(t, 1)
			if err := repository.Save(ctx, before, 0); err != nil {
				t.Fatal(err)
			}
			next := before.CloneRaw()
			next.StateRevision++
			interrupted := errors.New("exact CAS interruption")
			coordinator, err := runtime.NewRuntimeTransitionCoordinator(runtimePreimageOwnerFaultRepository{runtimeStateRepository: repository, after: after, err: interrupted})
			if err != nil {
				t.Fatal(err)
			}
			if err := coordinator.Commit(ctx, next, 1); !errors.Is(err, interrupted) {
				t.Fatalf("fault not preserved: %v", err)
			}
			// Fresh adapter/coordinator cannot borrow a success credential. It
			// must see the actual owner revision, including a lost CAS response.
			fresh := runtimeStateRepository{hub: hub}
			coordinator, err = runtime.NewRuntimeTransitionCoordinator(fresh)
			if err != nil {
				t.Fatal(err)
			}
			err = coordinator.Commit(ctx, next, 1)
			if after {
				var conflict *runtime.RuntimeCASConflictError
				if !errors.As(err, &conflict) || conflict.ExpectedRevision != 1 || conflict.ObservedRevision != 2 {
					t.Fatalf("fresh owner reload lost exact committed revision: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			loaded, err := fresh.Load(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
			if err != nil || loaded.StateRevision != 2 || !reflect.DeepEqual(loaded.ExecutionBudget, before.ExecutionBudget) || !reflect.DeepEqual(loaded.OperationReceipts, before.OperationReceipts) {
				t.Fatalf("recovery altered revision/budget/effect: %v", err)
			}
			loaded.StateRevision++
			if err := coordinator.Commit(ctx, loaded, 2); err != nil {
				t.Fatalf("subsequent independent commit failed: %v", err)
			}
		})
	}
}

func TestRuntimeTransitionPreimageOwnerCASStillFencesInterleavedWrite(t *testing.T) {
	ctx := context.Background()
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := runmodule.New()
	if err := module.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	repository := runtimeStateRepository{hub: hub}
	before := runtimeAggregate(t, 1)
	if err := repository.Save(ctx, before, 0); err != nil {
		t.Fatal(err)
	}
	next := before.CloneRaw()
	next.StateRevision++
	concurrent := next.CloneRaw()
	concurrent.ProcessedEventIDs = append(concurrent.ProcessedEventIDs, "independent-owner-cas")
	coordinator, err := runtime.NewRuntimeTransitionCoordinator(runtimePreimageOwnerInterleavedRepository{runtimeStateRepository: repository, concurrent: concurrent})
	if err != nil {
		t.Fatal(err)
	}
	err = coordinator.Commit(ctx, next, 1)
	var conflict *runtime.RuntimeCASConflictError
	if !errors.As(err, &conflict) || conflict.ExpectedRevision != 1 || conflict.ObservedRevision != 2 {
		t.Fatalf("complete-load product bypassed owner CAS: %v", err)
	}
	loaded, err := repository.Load(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
	if err != nil || !reflect.DeepEqual(loaded.ProcessedEventIDs, concurrent.ProcessedEventIDs) {
		t.Fatalf("losing Commit overwrote concurrent original: %v", err)
	}
}
