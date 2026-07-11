package appservice

import (
	"testing"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimeports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestManagedRuntimeFromCommandHubBuildsRuntimeWithEventCollector(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)
	runtime := runtimeports.NewManagedRuntimeFromCommandHub(exports.Infrastructure.CommandHub)
	if runtime == nil {
		t.Fatal("expected runtime")
	}

	if err := publishManagedRuntimeRecord(ctx, exports.Infrastructure.CommandHub, managedRuntimeRecordFixture{
		Version:     events.WorkorchEventVersion,
		Topic:       runevents.TopicRunStateChanged,
		SubjectType: "run",
		SubjectID:   "run-assembly",
		RunID:       "run-assembly",
		Payload:     map[string]any{events.PayloadKeyState: "running"},
	}); err != nil {
		t.Fatalf("publish event record: %v", err)
	}

	published := runtime.PublishedEventRecords()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic != runevents.TopicRunStateChanged {
		t.Fatalf("topic = %s, want %s", published[0].Topic, runevents.TopicRunStateChanged)
	}
}

func TestManagedRuntimeFromCommandHubReturnsNilWhenRequiredHubMissing(t *testing.T) {
	runtime := runtimeports.NewManagedRuntimeFromCommandHub(nil)
	if runtime != nil {
		t.Fatalf("runtime = %#v, want nil", runtime)
	}
}
