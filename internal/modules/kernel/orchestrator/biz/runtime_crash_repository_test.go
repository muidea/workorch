package orchestrator

import (
	"context"
	"errors"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
)

type runtimeCrashPersistenceProbe struct {
	saves         int
	persisted     int
	lastExpected  uint64
	lastAggregate runtime.RuntimeAggregate
	saveErr       error
}

func (r *runtimeCrashPersistenceProbe) Load(context.Context, common.WorkspaceID, common.RunID) (runtime.RuntimeAggregate, error) {
	return runtime.RuntimeAggregate{}, common.ErrNotFound
}

func (r *runtimeCrashPersistenceProbe) Save(_ context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	r.saves++
	return r.persist(aggregate, expectedRevision)
}

func (r *runtimeCrashPersistenceProbe) persist(aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.persisted++
	r.lastExpected = expectedRevision
	r.lastAggregate = aggregate
	return nil
}

type runtimeCrashValidatedPersistenceProbe struct {
	runtimeCrashPersistenceProbe
	validatedSaves int
}

func (r *runtimeCrashValidatedPersistenceProbe) SaveValidated(_ context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	r.validatedSaves++
	return r.persist(aggregate, expectedRevision)
}

func TestRuntimeSaveCrashRepositoryPreservesCoordinatorPersistenceBoundary(t *testing.T) {
	ctx := context.Background()
	for _, validated := range []bool{false, true} {
		name := "ordinary"
		if validated {
			name = "validated"
		}
		for _, after := range []bool{false, true} {
			cut := "before"
			if after {
				cut = "after"
			}
			t.Run(name+"/"+cut, func(t *testing.T) {
				probe := &runtimeCrashValidatedPersistenceProbe{}
				var delegate runtime.RuntimeStateRepository = &probe.runtimeCrashPersistenceProbe
				if validated {
					delegate = probe
				}
				aggregate := runtimeAggregate(t, 1)
				crash := &runtimeSaveCrashRepository{
					delegate: delegate, after: after, marker: "crash-cut",
					match: func(current runtime.RuntimeAggregate) bool {
						return current.StateRevision == aggregate.StateRevision
					},
				}
				coordinator, err := runtime.NewRuntimeTransitionCoordinator(crash)
				if err != nil {
					t.Fatal(err)
				}
				invalid := aggregate
				invalid.Decision.AuthoritySnapshot.RunID = "another-run"
				if err := coordinator.Commit(ctx, invalid, 0); err == nil || crash.fired || probe.saves != 0 || probe.validatedSaves != 0 {
					t.Fatalf("非法 aggregate 越过 invariant gate: err=%v crash=%v saves=%d validated=%d", err, crash.fired, probe.saves, probe.validatedSaves)
				}
				func() {
					defer func() {
						if recovered := recover(); recovered != crash.marker {
							t.Fatalf("崩溃切点丢失: %v", recovered)
						}
					}()
					if err := coordinator.Commit(ctx, aggregate, 0); err != nil {
						t.Fatal(err)
					}
					t.Fatal("匹配切点没有触发崩溃")
				}()
				wantPersisted := 0
				if after {
					wantPersisted = 1
				}
				if !crash.fired || probe.persisted != wantPersisted {
					t.Fatalf("CAS 前后切点改变: fired=%v persisted=%d want=%d", crash.fired, probe.persisted, wantPersisted)
				}
				if err := coordinator.Commit(ctx, aggregate, 0); err != nil {
					t.Fatal(err)
				}
				wantPersisted++
				if probe.persisted != wantPersisted || probe.lastExpected != 0 || probe.lastAggregate.StateRevision != aggregate.StateRevision {
					t.Fatalf("重复调用改变持久身份: %+v", probe)
				}
				if validated && (probe.validatedSaves != wantPersisted || probe.saves != 0) {
					t.Fatalf("wrapper 未保留已校验写入路径: %+v", probe)
				}
				if !validated && (probe.saves != wantPersisted || probe.validatedSaves != 0) {
					t.Fatalf("普通 repository 写入路径改变: %+v", probe)
				}
			})
		}
	}
}

func TestRuntimeSaveCrashRepositoryRetainsPersistenceFailureAndDirectSave(t *testing.T) {
	ctx := context.Background()
	probe := &runtimeCrashValidatedPersistenceProbe{}
	probe.saveErr = errors.New("CAS 未提交")
	crash := &runtimeSaveCrashRepository{delegate: probe, after: true, marker: "after-commit", match: func(runtime.RuntimeAggregate) bool { return true }}
	coordinator, err := runtime.NewRuntimeTransitionCoordinator(crash)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := runtimeAggregate(t, 1)
	if err := coordinator.Commit(ctx, aggregate, 0); !errors.Is(err, probe.saveErr) || crash.fired || probe.persisted != 0 {
		t.Fatalf("失败 CAS 被当作已提交崩溃: err=%v fired=%v persisted=%d", err, crash.fired, probe.persisted)
	}
	crash.match = nil
	probe.saveErr = nil
	if err := crash.Save(ctx, aggregate, 0); err != nil {
		t.Fatal(err)
	}
	if probe.saves != 1 || probe.validatedSaves != 1 || probe.persisted != 1 {
		t.Fatalf("直接 Save 绕过普通 repository 校验入口: %+v", probe)
	}
}
