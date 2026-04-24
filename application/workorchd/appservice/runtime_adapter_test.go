package appservice

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type testHub struct {
	subscribers map[string][]events.Handler
}

func newTestHub() *testHub {
	return &testHub{subscribers: map[string][]events.Handler{}}
}

func (h *testHub) Publish(ctx context.Context, envelope events.Envelope) error {
	for _, handler := range h.subscribers[envelope.Topic] {
		if handler == nil {
			continue
		}
		if err := handler(ctx, envelope); err != nil {
			return err
		}
	}
	return nil
}

func (h *testHub) Subscribe(topic string, handler events.Handler) func() {
	h.subscribers[topic] = append(h.subscribers[topic], handler)
	return func() {}
}

func TestManagedRuntimePublishedWorkorchEventsUsesCollectorRatherThanHubPublishedMethod(t *testing.T) {
	hub := newTestHub()
	runtime := NewManagedRuntime(NewRuntimeAdapter(RuntimeDependencies{}), hub)
	defer runtime.Close()

	if err := hub.Publish(context.Background(), events.Envelope{
		Version:     events.WorkorchEventVersion,
		Topic:       events.TopicRunStateChanged,
		SubjectType: "run",
		SubjectID:   "run-1",
		RunID:       "run-1",
		Payload:     map[string]any{events.PayloadKeyState: "running"},
	}); err != nil {
		t.Fatalf("publish run event: %v", err)
	}

	published := runtime.PublishedWorkorchEvents()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic() != events.TopicRunStateChanged {
		t.Fatalf("topic = %s, want %s", published[0].Topic(), events.TopicRunStateChanged)
	}
	if published[0].Envelope().Payload[events.PayloadKeyState] != "running" {
		t.Fatalf("payload state = %v, want running", published[0].Envelope().Payload[events.PayloadKeyState])
	}
}

func TestManagedRuntimeCloseStopsFurtherCollection(t *testing.T) {
	hub := newTestHub()
	runtime := NewManagedRuntime(NewRuntimeAdapter(RuntimeDependencies{}), hub)

	runtime.Close()

	if err := hub.Publish(context.Background(), events.Envelope{
		Version:     events.WorkorchEventVersion,
		Topic:       events.TopicApprovalCreated,
		SubjectType: "approval",
		SubjectID:   "approval-1",
		Payload:     map[string]any{events.PayloadKeyState: "pending"},
	}); err != nil {
		t.Fatalf("publish approval event: %v", err)
	}

	if published := runtime.PublishedWorkorchEvents(); len(published) != 0 {
		t.Fatalf("len(published) = %d, want 0", len(published))
	}
}

func TestNewManagedRuntimeReturnsNilWhenRuntimeMissing(t *testing.T) {
	if runtime := NewManagedRuntime(nil, newTestHub()); runtime != nil {
		t.Fatalf("runtime = %#v, want nil", runtime)
	}
}

func TestRuntimeAdapterReturnsInvalidArgumentWhenDependenciesMissing(t *testing.T) {
	ctx := context.Background()
	runtime := NewRuntimeAdapter(RuntimeDependencies{})

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "get workspace", run: func() error { _, err := runtime.GetWorkspaceByID(ctx, "ws"); return err }},
		{name: "ensure workspace", run: func() error { return runtime.EnsureWorkspace(ctx, "ws", "title") }},
		{name: "get run", run: func() error { _, err := runtime.GetRun(ctx, "run"); return err }},
		{name: "ensure run", run: func() error { return runtime.EnsureRun(ctx, "run", "ws") }},
		{name: "ensure run started", run: func() error { return runtime.EnsureRunStarted(ctx, "run") }},
		{name: "ensure run blocked on decision", run: func() error { return runtime.EnsureRunBlockedOnDecision(ctx, "run", "ticket") }},
		{name: "ensure run blocked on approval", run: func() error { return runtime.EnsureRunBlockedOnApproval(ctx, "run", "approval") }},
		{name: "ensure run resumed", run: func() error { return runtime.EnsureRunResumed(ctx, "run") }},
		{name: "get step", run: func() error { _, err := runtime.GetStep(ctx, "run/step-1"); return err }},
		{name: "ensure ticket", run: func() error { return runtime.EnsureTicket(ctx, "ticket", "ws", "run", "run/manual-gate", "decision") }},
		{name: "get ticket", run: func() error { _, err := runtime.GetTicket(ctx, "ticket"); return err }},
		{name: "get resolution", run: func() error { _, err := runtime.GetResolutionByTicketID(ctx, "ticket"); return err }},
		{name: "ensure resolved ticket", run: func() error { return runtime.EnsureResolvedTicket(ctx, "resolution", "ticket", "approve_path", nil) }},
		{name: "ensure approval", run: func() error {
			return runtime.EnsureApproval(ctx, "approval", "ws", "run", "run/manual-approval", "approval")
		}},
		{name: "get approval", run: func() error { _, err := runtime.GetApproval(ctx, "approval"); return err }},
		{name: "ensure approval approved", run: func() error { return runtime.EnsureApprovalApproved(ctx, "approval") }},
		{name: "ensure approval rejected", run: func() error { return runtime.EnsureApprovalRejected(ctx, "approval") }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
		})
	}
}

func TestRuntimeAdapterNilReceiverReturnsInvalidArgument(t *testing.T) {
	var runtime *runtimeAdapter

	if _, err := runtime.GetRun(context.Background(), "run"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("get run err = %v, want invalid argument", err)
	}
	if err := runtime.EnsureRun(context.Background(), "run", "ws"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ensure run err = %v, want invalid argument", err)
	}
}
