package run

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type heldRunRecoveryBackground struct {
	magictask.BackgroundRoutine
	mu     sync.Mutex
	tasks  []magictask.Task
	reject error
}

func (b *heldRunRecoveryBackground) AsyncTaskContext(_ context.Context, task magictask.Task) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.reject != nil {
		return b.reject
	}
	b.tasks = append(b.tasks, task)
	return nil
}
func (b *heldRunRecoveryBackground) complete() {
	b.mu.Lock()
	tasks := b.tasks
	b.tasks = nil
	b.mu.Unlock()
	for _, task := range tasks {
		task.Run()
	}
}

func TestRunLifecycleCanceledQueuedRecoveryIsNotCompletedUntilCallbackReturns(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-queued-recovery")
	runs := memory.NewRunRepository()
	predecessor := replacementRecoveryTestRun(t, "ws-queued-recovery", "request-queued")
	predecessor.ReplacementIntent.Recovery.DueAt = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(16)
	defer hub.Terminate(context.Background())
	background := &heldRunRecoveryBackground{}
	owner := New()
	if err := owner.Configure(runBizTestDependencies(runs, nil, nil, hub, background)); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if len(background.tasks) != 1 {
		t.Fatalf("queued callbacks=%d", len(background.tasks))
	}
	key := replacementRecoveryTaskKey(predecessor.WorkspaceID, predecessor.ID)
	acquiredTopics := append([]string(nil), owner.topics...)
	if len(acquiredTopics) != 24 { // Complete history and exact original-receipt queries.
		t.Fatalf("unexpected acquired topic inventory: %v", acquiredTopics)
	}
	owner.BeginShutdown()
	for attempt := 0; attempt < 2; attempt++ {
		budget, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		err := owner.Stop(budget)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Stop treated queued callback as done: %v", err)
		}
		if owner.runs == nil || owner.replacementRecoveryCancel == nil || owner.replacementRecoveryCtx == nil || owner.replacementRecoveryTasks[key] == 0 || !reflect.DeepEqual(owner.topics, acquiredTopics) || owner.stopping {
			t.Fatal("timeout discarded dependencies/handles/ledger or disabled drain")
		}
	}
	owner.scheduleReplacementRecovery(predecessor.WorkspaceID, predecessor.ID)
	if len(background.tasks) != 1 {
		t.Fatal("shutdown accepted new recovery submission")
	}
	stored, err := runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil || !reflect.DeepEqual(stored.ReplacementIntent, predecessor.ReplacementIntent) {
		t.Fatalf("shutdown changed SM-1 lease: %+v %v", stored, err)
	}
	background.complete()
	if err := owner.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := owner.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored, err = runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil || !reflect.DeepEqual(stored.ReplacementIntent, predecessor.ReplacementIntent) {
		t.Fatalf("canceled callback changed durable recovery: %+v %v", stored, err)
	}
}

func TestRunLifecycleRejectedRecoverySubmissionDoesNotLeakInflight(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-rejected-recovery")
	runs := memory.NewRunRepository()
	predecessor := replacementRecoveryTestRun(t, "ws-rejected-recovery", "request-rejected")
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(16)
	defer hub.Terminate(context.Background())
	background := &heldRunRecoveryBackground{reject: common.ErrConflict}
	owner := New()
	if err := owner.Configure(runBizTestDependencies(runs, nil, nil, hub, background)); err != nil {
		t.Fatal(err)
	}
	owner.scheduleReplacementRecovery(predecessor.WorkspaceID, predecessor.ID)
	if len(owner.replacementRecoveryTasks) != 0 {
		t.Fatal("rejected task retained mounted wakeup")
	}
	owner.BeginShutdown()
	if err := owner.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := owner.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRunLifecycleShutdownAdmitsOnlyValidatedCompletionNotNewExecution(t *testing.T) {
	ctx, owner, ownerSet := newRunPrivilegedSourceTestOwner(t, "workspace-source-terminal")
	seed := runLifecycleSourceTestTerminalRun()
	if err := ownerSet.Runs().Create(ctx, seed); err != nil {
		t.Fatal(err)
	}
	cmd := terminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{
		WorkspaceID: seed.WorkspaceID, RunID: seed.ID, TerminalDeliveryID: "terminal-source-terminal",
		AuthoritySnapshotID: "authority-source-terminal", DefinitionRevision: "definition-source-terminal",
		EvidenceRevision: "evidence-source-terminal", DispatchID: "dispatch-source-terminal",
		Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
	}, "cause-source-terminal")
	owner.BeginShutdown()
	if err := owner.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Even the legitimate Workspace source and a fully valid sealed intent are
	// fresh admission, not an EnterDrain fallback.
	bootstrap := restartTestBootstrap(t, seed.WorkspaceID)
	execute := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: seed.WorkspaceID, RequestID: "restart-after-shutdown", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	}
	if err := execute.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.executeWorkspaceExecutionIntent(magicevent.NewEventWithContext(runevents.TopicRunCommandWorkspaceExecutionIntent, workspaceExecutionIntentRunSource, ModuleID, magicevent.NewHeader(), ctx, execute)); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("valid new execution passed shutdown: %v", err)
	}
	for _, source := range []string{"blocks/workspace", "kernel/orchestrator"} {
		if source == "kernel/orchestrator" {
			for _, topic := range []string{runevents.TopicRunCommandWorkspaceExecutionIntent, runevents.TopicRunFactContinuationEligible} {
				ev := magicevent.NewEventWithContext(topic, source, ModuleID, magicevent.NewHeader(), ctx, cmd)
				var err error
				if topic == runevents.TopicRunCommandWorkspaceExecutionIntent {
					_, err = owner.executeWorkspaceExecutionIntent(ev)
				} else {
					_, err = owner.executeLifecycleCommand(ev)
				}
				if !errors.Is(err, basebiz.ErrLifecycleClosing) {
					t.Fatalf("source-wide drain authorized new execution: %v", err)
				}
			}
			continue
		}
		ev := magicevent.NewEventWithContext(runevents.TopicRunFactTerminalDelivery, source, ModuleID, magicevent.NewHeader(), ctx, cmd)
		if _, err := owner.executeLifecycleCommand(ev); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("drain skipped source validation: %v", err)
		}
	}
	ev := magicevent.NewEventWithContext(runevents.TopicRunFactTerminalDelivery, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx, cmd)
	for attempt := 0; attempt < 2; attempt++ {
		result, err := owner.executeLifecycleCommand(ev)
		if err != nil || result.Run.State != common.RunStateCompleted || result.DeliveryAck == nil || result.Transitioned != (attempt == 0) {
			t.Fatalf("validated terminal retry %d: %+v %v", attempt, result, err)
		}
	}
}

func TestRunLifecycleUnknownPanicRemainsUnknown(t *testing.T) {
	sentinel := &struct{}{}
	defer func() {
		if recover() != sentinel {
			t.Fatal("unknown panic was hidden")
		}
	}()
	_ = checkedOwnerSubscription(func() { panic(sentinel) })
}

func TestRunLifecycleNestedSendRetainsParentContextThroughShutdown(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-nested-drain")
	runs := memory.NewRunRepository()
	predecessor := replacementRecoveryTestRun(t, "ws-nested-drain", "request-nested")
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(16)
	owner := New()
	if err := owner.Configure(runBizTestDependencies(runs, nil, nil, hub, nil)); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	if err := observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, func(ev magicevent.Event, result magicevent.Result) {
		// The Run task is already accepted. Its synchronous child must retain
		// the same Hub ancestor lane and still read the owner after shutdown.
		owner.BeginShutdown()
		child := magicevent.NewEventWithContext(runevents.TopicRunCommandGet, runtimeLifecycleFactSource, ModuleID, magicevent.NewHeader(), ev.Context(), runevents.GetRunCommand{WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID})
		child.BindLaneKey(ev.LaneKey())
		read := hub.Send(child)
		if read == nil || read.Error() != nil {
			t.Errorf("nested drain read: %v", read)
			result.Set(nil, nil)
			return
		}
		result.Set(orchestratorevents.RequestRuntimeTerminationResult{TerminalDeliveryID: "terminal-nested", WorkspaceReleased: true, StateRevision: 1}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination)
		owner.Stop(context.Background())
		hub.Terminate(context.Background())
	}()
	leave, err := owner.enterOwnerCommand(false)
	if err != nil {
		t.Fatal(err)
	}
	defer leave()
	done := make(chan error, 1)
	go func() {
		done <- owner.deliverClaimedReplacementTermination(ctx, predecessor.ID, *predecessor.ReplacementIntent, 1)
	}()
	select {
	case <-done: // The incomplete fake acknowledgement may fail SM-1 validation;
		// this test proves only nested Send completion/context and drain admission.
	case <-time.After(time.Second):
		t.Fatal("nested Send lost ancestor context or held commandMu")
	}
	leave()
}
