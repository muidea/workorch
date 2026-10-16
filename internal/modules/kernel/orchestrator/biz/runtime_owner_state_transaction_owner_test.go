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
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeOwnerTransactionFaultRepository struct {
	runtimeStateRepository
	loads, rereads, saves int
	after                 bool
	failure               error
}

func (r *runtimeOwnerTransactionFaultRepository) LoadOwnerStateOriginal(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtimecontract.RuntimeStateRecord, runtime.RuntimeAggregate, error) {
	r.loads++
	return r.runtimeStateRepository.LoadOwnerStateOriginal(ctx, workspaceID, runID)
}
func (r *runtimeOwnerTransactionFaultRepository) LoadTransitionPreimage(ctx context.Context, request runtime.RuntimeTransitionPreimageRequest) (runtime.RuntimeTransitionPreimage, error) {
	r.rereads++
	return r.runtimeStateRepository.LoadTransitionPreimage(ctx, request)
}
func (r *runtimeOwnerTransactionFaultRepository) SaveValidated(ctx context.Context, aggregate runtime.RuntimeAggregate, expected uint64) error {
	r.saves++
	if !r.after {
		return r.failure
	}
	if err := r.runtimeStateRepository.SaveValidated(ctx, aggregate, expected); err != nil {
		return err
	}
	return r.failure
}

func TestRuntimeOwnerStateTransactionRealOwnerCASAndRestart(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_cas", true: "after_cas_response_loss"}[after], func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "runs.json")
			runs := filerepository.NewRunRepository(path)
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
			failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed", Signature: "exact-failure", Message: "preserved owner input"}
			event := runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventFailureObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: before.Facts.WorkspaceID, RunID: before.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{FailureFact: &failure}}
			interrupted := errors.New("owner CAS interruption")
			fault := &runtimeOwnerTransactionFaultRepository{runtimeStateRepository: repository, after: after, failure: interrupted}
			controller, _ := runtime.NewRuntimeController(fault)
			if _, err := controller.ApplyInternalEvent(ctx, event); !errors.Is(err, interrupted) {
				t.Fatalf("interruption lost: %v", err)
			}
			if fault.loads != 1 || fault.rereads != 1 || fault.saves != 1 {
				t.Fatalf("actual owner reads/CAS=%d/%d/%d", fault.loads, fault.rereads, fault.saves)
			}
			if err := module.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			// Reconstruct both the file repository and the actual Run owner.
			freshModule := runmodule.New()
			if err := freshModule.Configure(runOwnerTestDependencies(filerepository.NewRunRepository(path), nil, nil, hub)); err != nil {
				t.Fatal(err)
			}
			if err := freshModule.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = freshModule.Stop(ctx) })
			fresh := runtimeStateRepository{hub: hub}
			controller, _ = runtime.NewRuntimeController(fresh)
			result, err := controller.ApplyInternalEvent(ctx, event)
			if err != nil || result.Aggregate.StateRevision != 2 {
				t.Fatalf("re-drive not exactly once: revision=%d err=%v", result.Aggregate.StateRevision, err)
			}
			loaded, err := fresh.Load(ctx, before.Facts.WorkspaceID, before.Facts.RunID)
			if err != nil || loaded.StateRevision != 2 || !reflect.DeepEqual(loaded.ExecutionBudget, before.ExecutionBudget) || !reflect.DeepEqual(loaded.OperationReceipts, before.OperationReceipts) {
				t.Fatalf("restart changed budget/effects: %v", err)
			}
			duplicate, err := controller.ApplyInternalEvent(ctx, event)
			if err != nil || !reflect.DeepEqual(duplicate.Aggregate, loaded) {
				t.Fatalf("duplicate rewrote committed fact: %v", err)
			}
		})
	}
}
