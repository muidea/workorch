package runtimecore

import (
	"context"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func newEventCollectorTestHub(t *testing.T) magicevent.Hub {
	t.Helper()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	return hub
}

func publishEventCollectorRecord(t *testing.T, hub magicevent.Hub, topic, subjectID string, payload map[string]any) {
	t.Helper()
	value, err := workorcheventpayload.FromMap(payload)
	if err != nil {
		t.Fatal(err)
	}
	record, err := eventstreamevents.EventRecordDataFromModel(&eventstreammodel.EventRecord{
		ID: common.EventRecordID("event-" + subjectID), Version: events.WorkorchEventVersion, Topic: topic,
		SubjectID: subjectID, RunID: common.RunID("run-1"), Payload: value,
	})
	if err != nil {
		t.Fatal(err)
	}
	hub.Post(magicevent.NewEventWithContext(eventstreamevents.TopicEventStreamRecordAppended, "blocks/eventstream", eventstreamevents.NotificationDestination, magicevent.NewHeader(), context.Background(), eventstreamevents.EventRecordAppendedData{Record: record}))
	time.Sleep(10 * time.Millisecond)
}

func TestEventCollectorCollectsOnlyValidRecordsAndCloses(t *testing.T) {
	hub := newEventCollectorTestHub(t)
	collector := newEventCollector(hub)
	publishEventCollectorRecord(t, hub, runevents.TopicRunStateChanged, "run-1", map[string]any{events.PayloadKeyState: "active"})
	publishEventCollectorRecord(t, hub, runevents.TopicRunStateChanged, "run-invalid", map[string]any{events.PayloadKeyState: 1})
	if published := collector.PublishedEventRecords(); len(published) != 1 || published[0].SubjectID != "run-1" {
		t.Fatalf("published = %+v, want one valid run record", published)
	}
	collector.Close()
	publishEventCollectorRecord(t, hub, approvalevents.TopicApprovalResolved, "approval-1", map[string]any{events.PayloadKeyState: "approved"})
	if published := collector.PublishedEventRecords(); len(published) != 1 {
		t.Fatalf("published after close = %d, want 1", len(published))
	}
}

func TestEventCollectorReturnsDeepCopiedRecords(t *testing.T) {
	hub := newEventCollectorTestHub(t)
	collector := newEventCollector(hub)
	defer collector.Close()
	publishEventCollectorRecord(t, hub, approvalevents.TopicApprovalResolved, "approval-1", map[string]any{
		events.PayloadKeyReason: "approval", events.PayloadKeyAction: "approve", events.PayloadKeyState: "approved",
		events.PayloadKeyContext: map[string]any{"nested": map[string]any{"value": "original"}},
	})
	first := collector.PublishedEventRecords()
	contextValue := first[0].Payload.Map()[events.PayloadKeyContext].(map[string]any)
	contextValue["nested"].(map[string]any)["value"] = "mutated"
	second := collector.PublishedEventRecords()
	if got := second[0].Payload.Map()[events.PayloadKeyContext].(map[string]any)["nested"].(map[string]any)["value"]; got != "original" {
		t.Fatalf("deep clone value = %v, want original", got)
	}
}
