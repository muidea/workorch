package biz

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	executionusecase "github.com/muidea/workorch/internal/modules/application/workorchd/biz/execution"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type stopDrainSubmitter struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int64
}

func (s *stopDrainSubmitter) SubmitWorkspaceRun(_ context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	if s.calls.Add(1) == 1 {
		close(s.entered)
	}
	<-s.release
	return workorchdevents.WorkspaceRunRequestedResult{
		WorkspaceID: command.WorkspaceID,
		RunID:       "run-stop-drain",
		RequestID:   command.RequestID,
		Kind:        runtimecontract.RuntimeExecutionIntentKind(command.Intent),
		Outcome:     runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		Reason:      runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
	}, nil
}

func TestApplicationStopDrainsDirectEventHubExecutionAdmission(t *testing.T) {
	hub := magicevent.NewHub(16)
	background := magictask.NewBackgroundRoutine(16)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	t.Cleanup(func() {
		hub.Terminate(ctx)
		background.Shutdown(ctx)
	})

	submitter := &stopDrainSubmitter{entered: make(chan struct{}), release: make(chan struct{})}
	projection := &laneOrderingProjection{}
	admissions := newExecutionAdmissionLifecycle()
	b := &Biz{
		Base:       basebiz.New(ModuleID, hub, background),
		execution:  executionusecase.New(submitter, projection),
		admissions: admissions,
	}
	if err := b.subscribeCommand(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, b.handleWorkspaceRunRequested); err != nil {
		t.Fatal(err)
	}

	firstResult := make(chan magicevent.Result, 1)
	go func() {
		firstResult <- sendDirectExecutionAdmission(ctx, hub, "ws-stop-drain-active", "request-stop-drain-active")
	}()
	select {
	case <-submitter.entered:
	case <-ctx.Done():
		t.Fatalf("direct EventHub admission did not enter: %v", ctx.Err())
	}

	stopDone := make(chan struct{})
	go func() {
		b.Stop(context.Background())
		close(stopDone)
	}()
	for !admissions.stopping() {
		select {
		case <-ctx.Done():
			t.Fatalf("Stop did not reject new admissions: %v", ctx.Err())
		default:
			time.Sleep(time.Millisecond)
		}
	}

	rejected := sendDirectExecutionAdmission(ctx, hub, "ws-stop-drain-rejected", "request-stop-drain-rejected")
	if rejected == nil || rejected.Error() == nil || !errors.Is(rejected.Error().Cause, common.ErrConflict) {
		t.Fatalf("new admission during Stop result = %#v, want conflict", rejected)
	}
	if got := submitter.calls.Load(); got != 1 {
		t.Fatalf("owner submit calls during Stop = %d, want only the already-entered admission", got)
	}
	select {
	case <-stopDone:
		t.Fatal("Stop returned before the already-entered EventHub admission drained")
	default:
	}

	close(submitter.release)
	select {
	case result := <-firstResult:
		if result == nil || result.Error() != nil {
			t.Fatalf("already-entered admission result = %#v, want success", result)
		}
	case <-ctx.Done():
		t.Fatalf("already-entered admission did not finish: %v", ctx.Err())
	}
	select {
	case <-stopDone:
	case <-ctx.Done():
		t.Fatalf("Stop did not return after admission drain: %v", ctx.Err())
	}

	// Stop remains idempotent after every dependency and subscription is gone.
	b.Stop(context.Background())
}

type authoringStopSubscriptionHub struct {
	magicevent.Hub
	unsubscribed int
}

func (h *authoringStopSubscriptionHub) Unsubscribe(topic string, observer magicevent.Observer) *cd.Error {
	if topic == workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested {
		h.unsubscribed++
	}
	return h.Hub.Unsubscribe(topic, observer)
}

func TestApplicationStopAfterAuthoringTimeoutStillUnsubscribes(t *testing.T) {
	hub := &authoringStopSubscriptionHub{Hub: magicevent.NewHub(16)}
	defer hub.Terminate(context.Background())
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil), admissions: newExecutionAdmissionLifecycle(), authoringTasks: newAuthoringTaskLifecycle()}
	if err := b.subscribeCommand(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, b.handleWorkspaceRunRequested); err != nil {
		t.Fatal(err)
	}
	_, release, err := b.authoringTasks.accept(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	b.Stop(stopCtx)
	if hub.unsubscribed != 0 || !b.admissions.stopping() {
		t.Fatal("未排空调用就解除订阅或没有关闭新 admission")
	}
	release()
	b.Stop(context.Background())
	b.Stop(context.Background())
	if hub.unsubscribed != 1 {
		t.Fatalf("重复 Stop 未完成唯一订阅清理: %d", hub.unsubscribed)
	}
}

func TestApplicationStopWithoutSetupIsSafe(t *testing.T) {
	b := New()
	b.Stop(context.Background())
	b.Stop(context.Background())
	var nilBiz *Biz
	nilBiz.Stop(context.Background())
}

func sendDirectExecutionAdmission(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, requestID string) magicevent.Result {
	topic := workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested
	event := magicevent.NewEventWithContext(topic, "test/workorchd", workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: workspaceID,
		Intent:      string(runtimecontract.RuntimeExecutionIntentResume),
		RequestID:   requestID,
		Source:      "test",
	})
	event.BindLaneKey(executionAdmissionLaneKey(topic, workspaceID))
	return hub.Send(event)
}
