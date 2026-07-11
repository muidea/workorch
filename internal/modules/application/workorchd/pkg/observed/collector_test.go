package observed

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/pkg/events"
)

type collectorTestHub struct {
	raw magicevent.Hub
}

func newCollectorTestHub(t *testing.T) *collectorTestHub {
	t.Helper()
	hub := &collectorTestHub{raw: magicevent.NewHub(32)}
	t.Cleanup(func() { hub.raw.Terminate(context.Background()) })
	return hub
}

func (h *collectorTestHub) Publish(ctx context.Context, record eventRecordFixture) error {
	data, err := eventRecordDataFromFixture(record)
	if err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(
		eventstreamevents.TopicEventStreamRecordAppended,
		"kernel/eventstream",
		eventstreamevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		eventstreamevents.EventRecordAppendedData{Record: data},
	)
	result := h.raw.Send(event)
	if result == nil {
		return fmt.Errorf("record appended event returned no result")
	}
	if resultErr := result.Error(); resultErr != nil {
		if strings.Contains(resultErr.Message, "missing observer") {
			return nil
		}
		return resultErr
	}
	return nil
}

func TestCollectorCollectsSubscribedWorkorchEvents(t *testing.T) {
	hub := newCollectorTestHub(t)
	collector := NewCollector(hub.raw)
	defer collector.Close()

	if err := hub.Publish(context.Background(), eventRecordFixture{
		Version:     events.WorkorchEventVersion,
		Topic:       runevents.TopicRunStateChanged,
		SubjectType: "run",
		SubjectID:   "run-1",
		RunID:       "run-1",
		Payload:     map[string]any{events.PayloadKeyState: "running"},
	}); err != nil {
		t.Fatalf("publish run event: %v", err)
	}
	if err := hub.Publish(context.Background(), eventRecordFixture{
		Topic:       "/unrelated/topic",
		SubjectType: "other",
		SubjectID:   "other-1",
		Payload:     map[string]any{events.PayloadKeyState: "ignored"},
	}); err != nil {
		t.Fatalf("publish unrelated event: %v", err)
	}

	published := collector.PublishedEventRecords()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic != runevents.TopicRunStateChanged {
		t.Fatalf("topic = %s, want %s", published[0].Topic, runevents.TopicRunStateChanged)
	}
	if payload := published[0].Payload.Map(); payload[events.PayloadKeyState] != "running" {
		t.Fatalf("payload state = %v, want running", payload[events.PayloadKeyState])
	}
}

func TestCollectorCloseStopsFurtherCollection(t *testing.T) {
	hub := newCollectorTestHub(t)
	collector := NewCollector(hub.raw)
	collector.Close()

	if err := hub.Publish(context.Background(), eventRecordFixture{
		Version:     events.WorkorchEventVersion,
		Topic:       approvalevents.TopicApprovalCreated,
		SubjectType: "approval",
		SubjectID:   "approval-1",
		Payload:     map[string]any{events.PayloadKeyState: "pending"},
	}); err != nil {
		t.Fatalf("publish approval event: %v", err)
	}

	if published := collector.PublishedEventRecords(); len(published) != 0 {
		t.Fatalf("len(published) = %d, want 0", len(published))
	}
}

func TestCollectorSkipsInvalidWorkorchPayload(t *testing.T) {
	hub := newCollectorTestHub(t)
	collector := NewCollector(hub.raw)
	defer collector.Close()

	if err := hub.Publish(context.Background(), eventRecordFixture{
		Version:     events.WorkorchEventVersion,
		Topic:       runevents.TopicRunStateChanged,
		SubjectType: "run",
		SubjectID:   "run-1",
		RunID:       "run-1",
		Payload: map[string]any{
			events.PayloadKeyState: 123,
		},
	}); err != nil {
		t.Fatalf("publish invalid run event: %v", err)
	}

	if published := collector.PublishedEventRecords(); len(published) != 0 {
		t.Fatalf("len(published) = %d, want 0", len(published))
	}
}

func TestCollectorPublishedEventRecordsReturnsDeepClonedPayloads(t *testing.T) {
	hub := newCollectorTestHub(t)
	collector := NewCollector(hub.raw)
	defer collector.Close()

	if err := hub.Publish(context.Background(), eventRecordFixture{
		Version:     events.WorkorchEventVersion,
		Topic:       runevents.TopicStepStateChanged,
		SubjectType: "step",
		SubjectID:   "run-1/step-1",
		RunID:       "run-1",
		StepID:      "run-1/step-1",
		Payload: events.StepPayload(
			"llm/workspace_execution",
			"completed",
			map[string]any{
				"workspaceId": "ws-1",
				"items":       []any{"first", map[string]any{"nested": "original"}},
			},
			map[string]any{
				"status": "completed",
			},
		),
	}); err != nil {
		t.Fatalf("publish step event: %v", err)
	}

	first := collector.PublishedEventRecords()
	if len(first) != 1 {
		t.Fatalf("len(first) = %d, want 1", len(first))
	}

	input := first[0].Payload.Map()[events.PayloadKeyInput].(map[string]any)
	input["workspaceId"] = "ws-mutated"
	items := input["items"].([]any)
	items[0] = "changed"
	items[1].(map[string]any)["nested"] = "mutated"
	output := first[0].Payload.Map()[events.PayloadKeyOutput].(map[string]any)
	output["status"] = "failed"

	second := collector.PublishedEventRecords()
	if len(second) != 1 {
		t.Fatalf("len(second) = %d, want 1", len(second))
	}

	secondInput := second[0].Payload.Map()[events.PayloadKeyInput].(map[string]any)
	if secondInput["workspaceId"] != "ws-1" {
		t.Fatalf("workspaceId = %v, want ws-1", secondInput["workspaceId"])
	}
	secondItems := secondInput["items"].([]any)
	if secondItems[0] != "first" {
		t.Fatalf("items[0] = %v, want first", secondItems[0])
	}
	if secondItems[1].(map[string]any)["nested"] != "original" {
		t.Fatalf("nested item = %v, want original", secondItems[1].(map[string]any)["nested"])
	}
	secondOutput := second[0].Payload.Map()[events.PayloadKeyOutput].(map[string]any)
	if secondOutput["status"] != "completed" {
		t.Fatalf("status = %v, want completed", secondOutput["status"])
	}
}

func TestCollectorKeepsBoundedRecentEvents(t *testing.T) {
	hub := newCollectorTestHub(t)
	collector := NewCollector(hub.raw)
	defer collector.Close()

	for idx := 0; idx < maxCollectedEventRecords+3; idx++ {
		runID := "run-bounded-" + strconv.Itoa(idx)
		if err := hub.Publish(context.Background(), eventRecordFixture{
			Version:     events.WorkorchEventVersion,
			Topic:       runevents.TopicRunStateChanged,
			SubjectType: "run",
			SubjectID:   runID,
			RunID:       runID,
			Payload:     map[string]any{events.PayloadKeyState: "running"},
		}); err != nil {
			t.Fatalf("publish event %d: %v", idx, err)
		}
	}

	published := collector.PublishedEventRecords()
	if len(published) != maxCollectedEventRecords {
		t.Fatalf("len(published) = %d, want %d", len(published), maxCollectedEventRecords)
	}
	if published[0].SubjectID != "run-bounded-3" {
		t.Fatalf("first event = %q, want oldest events evicted", published[0].SubjectID)
	}
}
