package appservice

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/events"
)

func TestManagedRuntimeAssemblyFromExportsBuildsRuntimeAndCollector(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports := session.Bootstrap.Exports()
	assembly := managedRuntimeAssemblyFromExports(exports)
	if assembly.runtime == nil {
		t.Fatal("expected runtime")
	}
	if assembly.collector == nil {
		t.Fatal("expected collector")
	}

	if err := exports.Infrastructure.EventHub.Publish(ctx, events.Envelope{
		Version:     events.Phase1EventVersion,
		Topic:       events.TopicRunStateChanged,
		SubjectType: "run",
		SubjectID:   "run-assembly",
		RunID:       "run-assembly",
		Payload:     map[string]any{events.PayloadKeyState: "running"},
	}); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	published := assembly.collector.PublishedPhase1Events()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic() != events.TopicRunStateChanged {
		t.Fatalf("topic = %s, want %s", published[0].Topic(), events.TopicRunStateChanged)
	}
}
