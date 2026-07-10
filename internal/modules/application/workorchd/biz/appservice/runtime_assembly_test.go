package appservice

import (
	"testing"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimeports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestManagedRuntimeFromEventHubBuildsRuntimeWithEventCollector(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports := session.Bootstrap.Exports()
	runtime := runtimeports.NewManagedRuntimeFromEventHub(exports.Infrastructure.EventHub, exports.Infrastructure.CommandHub)
	if runtime == nil {
		t.Fatal("expected runtime")
	}

	if err := exports.Infrastructure.EventHub.Publish(ctx, events.Envelope{
		Version:     events.WorkorchEventVersion,
		Topic:       runevents.TopicRunStateChanged,
		SubjectType: "run",
		SubjectID:   "run-assembly",
		RunID:       "run-assembly",
		Payload:     map[string]any{events.PayloadKeyState: "running"},
	}); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	published := runtime.PublishedWorkorchEvents()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic() != runevents.TopicRunStateChanged {
		t.Fatalf("topic = %s, want %s", published[0].Topic(), runevents.TopicRunStateChanged)
	}
}

func TestManagedRuntimeFromEventHubReturnsNilWhenRequiredHubMissing(t *testing.T) {
	runtime := runtimeports.NewManagedRuntimeFromEventHub(nil, nil)
	if runtime != nil {
		t.Fatalf("runtime = %#v, want nil", runtime)
	}
}
