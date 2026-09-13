package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestModuleRunBindsFrameworkContextToOwnerLifecycle(t *testing.T) {
	hub := magicevent.NewHub(8)
	background := &moduleLifecycleRecordingBackground{}
	runOwner := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	runOwner.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{}, nil)
		}
	})
	runOwner.Subscribe(runevents.TopicRunCommandWorkspaceRetirementInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryWorkspaceRetirementInventoryResult{}, nil)
		}
	})
	t.Cleanup(func() {
		runOwner.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory)
		runOwner.Unsubscribe(runevents.TopicRunCommandWorkspaceRetirementInventory)
		hub.Terminate(context.Background())
	})

	module := New()
	if err := module.Configure(orchestratorbiz.Dependencies{
		Evidence:          memoryrepo.NewOrchestratorOwnerRepositorySet(),
		CommandHub:        hub,
		BackgroundRoutine: background,
	}); err != nil {
		t.Fatal(err)
	}
	runCtx, cancelRun := context.WithCancel(context.Background())
	if err := module.Run(runCtx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	baselineAsyncCalls := background.asyncCalls.Load()
	cancelRun()

	data := runevents.RunStateChangedData{
		WorkspaceID: "ws-framework-context", RunID: "run-framework-context",
		WorkspaceRunGeneration: 1, RunRevision: 1, State: common.RunStateActive,
	}
	data.CausationID = runevents.RunStateChangedCausationID(data.RunID, data.WorkspaceRunGeneration, data.RunRevision, 0)
	hub.Send(magicevent.NewEventWithContext(
		runevents.TopicRunStateChanged, runevents.CommandDestination, runevents.NotificationDestination,
		magicevent.NewHeader(), context.Background(), data,
	))
	if calls := background.asyncCalls.Load(); calls != baselineAsyncCalls {
		t.Fatalf("cancelled framework lifecycle accepted %d new background drives", calls-baselineAsyncCalls)
	}

	module.Teardown(context.Background())
	if module.bizPtr != nil {
		t.Fatal("Teardown retained the Orchestrator Biz")
	}
}

func TestModuleSetupRejectsMissingBackgroundRoutine(t *testing.T) {
	hub := magicevent.NewHub(1)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	if err := New().Setup(context.Background(), hub, nil); err == nil {
		t.Fatal("Setup accepted a nil BackgroundRoutine")
	}
}

func TestModuleLifecycleWeightsKeepStartupAndTeardownDependenciesIndependent(t *testing.T) {
	module := New()
	if module.Weight() != 30 {
		t.Fatalf("Orchestrator startup weight = %d, want 30 before Run", module.Weight())
	}
	if module.TeardownWeight() != 90 {
		t.Fatalf("Orchestrator teardown weight = %d, want 90 before Run teardown", module.TeardownWeight())
	}
}

type moduleLifecycleRecordingBackground struct {
	asyncCalls atomic.Int64
}

func (b *moduleLifecycleRecordingBackground) AsyncTask(value magictask.Task) error {
	if value != nil {
		b.asyncCalls.Add(1)
	}
	return nil
}

func (b *moduleLifecycleRecordingBackground) SyncTask(value magictask.Task) error {
	if value != nil {
		value.Run()
	}
	return nil
}

func (b *moduleLifecycleRecordingBackground) SyncTaskWithTimeOut(value magictask.Task, _ time.Duration) error {
	return b.SyncTask(value)
}

func (b *moduleLifecycleRecordingBackground) AsyncFunction(value func()) error {
	if value != nil {
		b.asyncCalls.Add(1)
	}
	return nil
}

func (b *moduleLifecycleRecordingBackground) SyncFunction(value func()) error {
	if value != nil {
		value()
	}
	return nil
}

func (b *moduleLifecycleRecordingBackground) SyncFunctionWithTimeOut(value func(), _ time.Duration) error {
	return b.SyncFunction(value)
}

func (*moduleLifecycleRecordingBackground) Timer(context.Context, magictask.Task, time.Duration, time.Duration) error {
	return nil
}

func (*moduleLifecycleRecordingBackground) Shutdown(context.Context) bool { return true }

func (b *moduleLifecycleRecordingBackground) AsyncTaskContext(ctx context.Context, value magictask.Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.AsyncTask(value)
}
