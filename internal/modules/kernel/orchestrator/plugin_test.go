package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	plugincommon "github.com/muidea/magicCommon/framework/plugin/common"
	magictask "github.com/muidea/magicCommon/task"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratorruntime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
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

func TestModuleTeardownCheckedRetainsBlockedOwnerAndRetriesWithoutDuplicateRelease(t *testing.T) {
	hub := magicevent.NewHub(8)
	background := &moduleLifecycleRecordingBackground{}
	runOwner := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	blocked := &moduleLifecycleBlockingRuntimeRepository{
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(blocked.release) }) }
	for topic, handler := range map[string]func(magicevent.Event, magicevent.Result){
		runevents.TopicRunCommandRuntimeRecoveryInventory: func(_ magicevent.Event, result magicevent.Result) {
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{}, nil)
		},
		runevents.TopicRunCommandWorkspaceRetirementInventory: func(_ magicevent.Event, result magicevent.Result) {
			result.Set(runevents.QueryWorkspaceRetirementInventoryResult{}, nil)
		},
		runevents.TopicRunCommandRuntimeStateRecord: func(ev magicevent.Event, result magicevent.Result) {
			command, ok := ev.Data().(runevents.QueryRuntimeStateRecordCommand)
			if !ok {
				result.Set(nil, cd.WrapError(cd.Unexpected, common.ErrInvalidArgument, "runtime query"))
				return
			}
			_, err := blocked.Load(ev.Context(), command.WorkspaceID, command.RunID)
			result.Set(nil, cd.WrapError(cd.NotFound, err, "runtime record"))
		},
		runevents.TopicRunCommandStoreRuntimeStateRecord: func(ev magicevent.Event, result magicevent.Result) {
			err := blocked.Save(ev.Context(), orchestratorruntime.RuntimeAggregate{}, 0)
			result.Set(nil, cd.WrapError(cd.Unexpected, err, "unexpected runtime write"))
		},
	} {
		if err := runOwner.Subscribe(topic, handler); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { runOwner.Unsubscribe(topic) })
	}

	module := New()
	if err := module.Configure(orchestratorbiz.Dependencies{
		Evidence:          memoryrepo.NewOrchestratorOwnerRepositorySet(),
		CommandHub:        hub,
		BackgroundRoutine: background,
	}); err != nil {
		t.Fatal(err)
	}
	originalBiz := module.bizPtr
	// Configure 使用测试拥有的依赖，不读取全局 initiator export。
	// 将真实 Module 而非 teardown delegate 注册到隔离的真实 manager；
	// pre-Setup 生命周期支持已显式配置的 plugin。
	manager := plugincommon.NewPluginMgr("orchestrator-checked-teardown-test")
	before := &moduleLifecycleReleaseProbe{id: "already-released", weight: 100}
	downstream := &moduleLifecycleReleaseProbe{id: "downstream-owner", weight: 40}
	for _, plugin := range []any{module, downstream, before} {
		if err := manager.Register(plugin); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		release()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := manager.TeardownChecked(ctx); err != nil {
			t.Errorf("cleanup teardown: %v", err)
		}
		hub.Terminate(ctx)
	})
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	if err := manager.Run(runCtx); err != nil {
		t.Fatalf("manager Run: %v", err)
	}

	queryDone := make(chan magicevent.Result, 1)
	go func() {
		queryDone <- hub.Send(magicevent.NewEventWithContext(
			orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt,
			"blocks/file", ModuleID, magicevent.NewHeader(), context.Background(),
			orchestratorevents.QueryRuntimeOperationReceiptCommand{
				WorkspaceID: "ws-checked-teardown", RunID: "run-checked-teardown",
				DispatchID: "dispatch-checked-teardown", OperationID: "operation-checked-teardown",
				ExecutionKey: "execution-checked-teardown", Fence: 1,
			},
		))
	}()
	select {
	case <-blocked.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("real Biz receipt handler did not reach blocking repository")
	}
	cancelRun()

	// 即使 framework Run context 已取消，独立停机预算仍须报告真实阻塞的 reader。
	// 第二次失败证明第一次没有将实际 Module 标记为 released。
	// 新阶段协议要求全部 Quiesce 成功后才进入任何最终释放；即使较高权重的 probe 也不能提前释放。
	for attempt := 1; attempt <= 2; attempt++ {
		stopCtx, cancelStop := context.WithTimeout(context.Background(), 20*time.Millisecond)
		err := manager.TeardownChecked(stopCtx)
		cancelStop()
		if err == nil || err.Code != cd.Unexpected || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("teardown attempt %d: err=%v, want guarded deadline failure", attempt, err)
		}
		if module.bizPtr != originalBiz {
			t.Fatalf("teardown attempt %d discarded or replaced original Biz", attempt)
		}
		if before.teardownCalls != 0 || downstream.teardownCalls != 0 {
			t.Fatalf("teardown attempt %d released wrong owners: before=%d downstream=%d",
				attempt, before.teardownCalls, downstream.teardownCalls)
		}
		select {
		case <-queryDone:
			t.Fatal("blocked owner read was treated as completed")
		default:
		}
	}

	release()
	select {
	case result := <-queryDone:
		if result == nil {
			t.Fatal("receipt query returned no result")
		}
		if _, err := result.Get(); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("released repository query: %v, want original not-found fact", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("receipt handler did not actually exit after repository unblock")
	}
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStop()
	if err := manager.TeardownChecked(stopCtx); err != nil {
		t.Fatalf("teardown retry after true handler exit: %v", err)
	}
	if module.bizPtr != nil || before.teardownCalls != 1 || downstream.teardownCalls != 1 {
		t.Fatalf("retry did not release exactly once: biz=%p before=%d downstream=%d",
			module.bizPtr, before.teardownCalls, downstream.teardownCalls)
	}
	if err := manager.TeardownChecked(stopCtx); err != nil {
		t.Fatalf("duplicate manager teardown: %v", err)
	}
	module.Teardown(stopCtx)
	if before.teardownCalls != 1 || downstream.teardownCalls != 1 || module.bizPtr != nil {
		t.Fatal("duplicate Teardown repeated a completed release")
	}
	if blocked.loadCalls.Load() != 1 || blocked.saveCalls.Load() != 0 {
		t.Fatalf("teardown caused Runtime I/O replay: loads=%d saves=%d", blocked.loadCalls.Load(), blocked.saveCalls.Load())
	}
}

type moduleLifecycleBlockingRuntimeRepository struct {
	entered     chan struct{}
	release     chan struct{}
	enteredOnce sync.Once
	loadCalls   atomic.Int64
	saveCalls   atomic.Int64
}

func (r *moduleLifecycleBlockingRuntimeRepository) Load(context.Context, common.WorkspaceID, common.RunID) (orchestratorruntime.RuntimeAggregate, error) {
	r.loadCalls.Add(1)
	r.enteredOnce.Do(func() { close(r.entered) })
	// 与现有 Biz 生命周期 fixture 相同，取消不是退出回执：
	// 已接纳的 owner read 只在显式解除阻塞后退出。
	<-r.release
	return orchestratorruntime.RuntimeAggregate{}, common.ErrNotFound
}

func (r *moduleLifecycleBlockingRuntimeRepository) Save(context.Context, orchestratorruntime.RuntimeAggregate, uint64) error {
	r.saveCalls.Add(1)
	return common.ErrInvalidArgument
}

type moduleLifecycleReleaseProbe struct {
	id            string
	weight        int
	teardownCalls int
}

func (p *moduleLifecycleReleaseProbe) ID() string                  { return p.id }
func (p *moduleLifecycleReleaseProbe) Weight() int                 { return p.weight }
func (*moduleLifecycleReleaseProbe) Run(context.Context) *cd.Error { return nil }
func (p *moduleLifecycleReleaseProbe) Teardown(context.Context)    { p.teardownCalls++ }

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
