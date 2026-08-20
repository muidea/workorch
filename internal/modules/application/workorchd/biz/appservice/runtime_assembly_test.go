package appservice

import (
	"context"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type managedRuntimeRecordFixture struct {
	ID           string
	Version      string
	Topic        string
	OccurredAt   string
	SourceModule string
	SubjectType  string
	SubjectID    string
	WorkspaceID  string
	RunID        string
	StepID       string
	Revision     int64
	Payload      map[string]any
}

func publishManagedRuntimeRecord(ctx context.Context, hub magicevent.Hub, record managedRuntimeRecordFixture) error {
	if record.ID == "" {
		record.ID = "event-" + record.SubjectID
	}
	if record.OccurredAt == "" {
		record.OccurredAt = "2026-08-20T00:00:00Z"
	}
	if record.SourceModule == "" {
		record.SourceModule = "blocks/run"
	}
	data, err := eventstreamevents.EventRecordDataFromModel(&eventstreammodel.EventRecord{
		ID:             common.EventRecordID(record.ID),
		Version:        record.Version,
		Topic:          record.Topic,
		OccurredAt:     record.OccurredAt,
		SourceModule:   record.SourceModule,
		SubjectType:    record.SubjectType,
		SubjectID:      record.SubjectID,
		WorkspaceID:    common.WorkspaceID(record.WorkspaceID),
		RunID:          common.RunID(record.RunID),
		StepID:         common.StepID(record.StepID),
		StreamPosition: 1,
		Revision:       record.Revision,
		Payload:        mustManagedRuntimePayload(record.Payload),
	})
	if err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(
		eventstreamevents.TopicEventStreamRecordAppended,
		"blocks/eventstream",
		eventstreamevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		eventstreamevents.EventRecordAppendedData{Record: data},
	)
	hub.Post(event)
	time.Sleep(10 * time.Millisecond)
	return nil
}

func mustManagedRuntimePayload(input map[string]any) workorcheventpayload.Payload {
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		panic(err)
	}
	return payload
}

func TestManagedRuntimeFromCommandHubBuildsRuntimeWithEventCollector(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)
	runtime := runtimecore.NewManagedRuntime(exports.Infrastructure.CommandHub)
	if runtime == nil {
		t.Fatal("expected runtime")
	}

	if err := publishManagedRuntimeRecord(ctx, exports.Infrastructure.CommandHub, managedRuntimeRecordFixture{
		Version:     recordpayload.WorkorchEventVersion,
		Topic:       runevents.TopicRunStateChanged,
		SubjectType: "run",
		SubjectID:   "run-assembly",
		RunID:       "run-assembly",
		Payload:     map[string]any{recordpayload.PayloadKeyState: "running"},
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
	runtime := runtimecore.NewManagedRuntime(nil)
	if runtime != nil {
		t.Fatalf("runtime = %#v, want nil", runtime)
	}
}
