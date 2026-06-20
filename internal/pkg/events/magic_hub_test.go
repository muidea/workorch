package events

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	runtimeeventdefs "github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent/eventdefs"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
)

func TestMagicHubPublishesThroughMagicCommonHub(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	var received Envelope
	unsubscribe := hub.Subscribe(runevents.TopicRunStateChanged, func(_ context.Context, envelope Envelope) error {
		received = envelope
		return nil
	})
	defer unsubscribe()

	err := hub.Publish(context.Background(), Envelope{
		Topic: runevents.TopicRunStateChanged,
		RunID: "run-1",
		Payload: map[string]any{
			PayloadKeyState: "completed",
		},
	})
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if received.Topic != runevents.TopicRunStateChanged {
		t.Fatalf("received topic = %s, want %s", received.Topic, runevents.TopicRunStateChanged)
	}
	if received.ID == "" || received.OccurredAt == "" || received.Version == "" {
		t.Fatalf("received envelope was not prepared: %+v", received)
	}
}

func TestMagicHubAssignsWorkspaceRevision(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	if err := hub.Publish(context.Background(), Envelope{Topic: workspaceevents.TopicWorkspaceStateChanged, WorkspaceID: "ws-a"}); err != nil {
		t.Fatalf("publish first workspace event: %v", err)
	}
	if err := hub.PublishAsync(context.Background(), Envelope{Topic: runevents.TopicRunStateChanged, WorkspaceID: "ws-a", RunID: "run-a"}); err != nil {
		t.Fatalf("publish second workspace event: %v", err)
	}
	if err := hub.Publish(context.Background(), Envelope{Topic: workspaceevents.TopicWorkspaceCreated, WorkspaceID: "ws-b"}); err != nil {
		t.Fatalf("publish other workspace event: %v", err)
	}
	if err := hub.Publish(context.Background(), Envelope{Topic: workorchdevents.TopicCLICommand}); err != nil {
		t.Fatalf("publish global event: %v", err)
	}

	published := hub.Published()
	if len(published) != 4 {
		t.Fatalf("published count = %d, want 4", len(published))
	}
	if published[0].Revision != 1 || published[1].Revision != 2 || published[2].Revision != 1 || published[3].Revision != 0 {
		t.Fatalf("revisions = %d/%d/%d/%d, want workspace-scoped monotonic revisions and zero global revision",
			published[0].Revision,
			published[1].Revision,
			published[2].Revision,
			published[3].Revision,
		)
	}
}

func TestMagicHubSupportsWildcardSubscription(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	var topics []string
	unsubscribe := hub.Subscribe("*", func(_ context.Context, envelope Envelope) error {
		topics = append(topics, envelope.Topic)
		return nil
	})
	defer unsubscribe()

	if err := hub.Publish(context.Background(), Envelope{Topic: workspaceevents.TopicWorkspaceCreated}); err != nil {
		t.Fatalf("publish workspace event: %v", err)
	}
	if err := hub.Publish(context.Background(), Envelope{Topic: runevents.TopicRunCreated}); err != nil {
		t.Fatalf("publish run event: %v", err)
	}
	if len(topics) != 2 {
		t.Fatalf("topics count = %d, want 2: %+v", len(topics), topics)
	}
}

func TestMagicHubUnsubscribeStopsDelivery(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	var calls int
	unsubscribe := hub.Subscribe(approvalevents.TopicApprovalResolved, func(_ context.Context, _ Envelope) error {
		calls++
		return nil
	})
	unsubscribe()

	if err := hub.Publish(context.Background(), Envelope{Topic: approvalevents.TopicApprovalResolved}); err != nil {
		t.Fatalf("publish after unsubscribe: %v", err)
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
}

func TestMagicHubUnsubscribeDoesNotBlockOnUnderlyingHub(t *testing.T) {
	hub := NewMagicHubWithHub(blockingUnsubscribeHub{})
	unsubscribe := hub.Subscribe(approvalevents.TopicApprovalResolved, func(_ context.Context, _ Envelope) error {
		return nil
	})

	done := make(chan struct{})
	go func() {
		unsubscribe()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(defaultMagicHubUnsubscribeTimeout + 200*time.Millisecond):
		t.Fatal("unsubscribe blocked on underlying magicCommon hub")
	}
}

func TestMagicHubSubscribeDoesNotBlockOnUnderlyingHub(t *testing.T) {
	hub := NewMagicHubWithHub(blockingSubscribeHub{})

	done := make(chan struct{})
	go func() {
		unsubscribe := hub.Subscribe(approvalevents.TopicApprovalResolved, func(_ context.Context, _ Envelope) error {
			return nil
		})
		unsubscribe()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(defaultMagicHubSubscribeTimeout + 200*time.Millisecond):
		t.Fatal("subscribe blocked on underlying magicCommon hub")
	}
}

func TestMagicHubReturnsHandlerError(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	wantErr := errors.New("handler failed")
	unsubscribe := hub.Subscribe(decisionevents.TopicTicketResolved, func(_ context.Context, _ Envelope) error {
		return wantErr
	})
	defer unsubscribe()

	err := hub.Publish(context.Background(), Envelope{Topic: decisionevents.TopicTicketResolved})
	if err == nil {
		t.Fatalf("Publish returned nil, want error")
	}
	if !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("Publish returned unexpected error: %v", err)
	}
}

func TestMagicHubPublishCanReturnHandlerResponse(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	want := Envelope{Topic: workorchdevents.TopicCLIResult, Payload: map[string]any{"ok": true}}
	unsubscribe := hub.Subscribe(workorchdevents.TopicCLICommand, func(_ context.Context, _ Envelope) error {
		return Response(want)
	})
	defer unsubscribe()

	err := hub.Publish(context.Background(), Envelope{Topic: workorchdevents.TopicCLICommand})
	response, ok := ResponseEnvelope(err)
	if !ok {
		t.Fatalf("Publish err = %v, want response envelope", err)
	}
	if response.Topic != workorchdevents.TopicCLIResult || response.Payload["ok"] != true {
		t.Fatalf("response = %+v, want CLI result payload", response)
	}
}

func TestMagicHubPublishedDoesNotRetainHeavyCLIResultPayload(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	large := strings.Repeat("x", 1024*1024)
	err := hub.Publish(context.Background(), Envelope{
		Topic:       workorchdevents.TopicCLIResult,
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"correlationId": "corr-1",
			"commandId":     "cmd-1",
			"ok":            true,
			"occurredAt":    time.Now().UTC().Format(time.RFC3339Nano),
			"workspaceId":   "ws-1",
			"runId":         "run-1",
			"resource":      map[string]any{"large": large},
			"resources":     []map[string]any{{"large": large}},
			"snapshot":      map[string]any{"large": large},
		},
	})
	if err != nil {
		t.Fatalf("publish CLI result: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("published count = %d, want 1", len(published))
	}
	payload := published[0].Payload
	if payload["resource"] != nil || payload["resources"] != nil || payload["snapshot"] != nil {
		t.Fatalf("published payload retained heavy result fields: %+v", payload)
	}
	if payload["correlationId"] != "corr-1" || payload["commandId"] != "cmd-1" || payload["ok"] != true {
		t.Fatalf("published payload lost audit fields: %+v", payload)
	}
	if payload["resourceSummary"] == nil || payload["resourcesSummary"] == nil || payload["snapshotSummary"] == nil {
		t.Fatalf("published payload missing result summaries: %+v", payload)
	}
}

func TestMagicHubWildcardObserverDoesNotSwallowHandlerResponse(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	unsubscribeAll := hub.Subscribe("*", func(_ context.Context, _ Envelope) error {
		return nil
	})
	defer unsubscribeAll()
	unsubscribeCommand := hub.Subscribe(orchestratorevents.TopicOrchestratorCommandBlockApproval, func(_ context.Context, envelope Envelope) error {
		return Response(Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{"ok": true}})
	})
	defer unsubscribeCommand()

	err := hub.Publish(context.Background(), Envelope{Topic: orchestratorevents.TopicOrchestratorCommandBlockApproval})
	response, ok := ResponseEnvelope(err)
	if !ok {
		t.Fatalf("Publish err = %v, want response envelope", err)
	}
	if response.Payload["ok"] != true {
		t.Fatalf("response = %+v, want ok payload", response)
	}
}

type blockingUnsubscribeHub struct{}

func (blockingUnsubscribeHub) Subscribe(string, magicevent.Observer) {}

func (blockingUnsubscribeHub) Unsubscribe(string, magicevent.Observer) {
	select {}
}

func (blockingUnsubscribeHub) Post(magicevent.Event) {}

func (blockingUnsubscribeHub) Send(magicevent.Event) magicevent.Result {
	return nil
}

func (blockingUnsubscribeHub) Terminate(_ context.Context) {}

type blockingSubscribeHub struct{}

func (blockingSubscribeHub) Subscribe(string, magicevent.Observer) {
	select {}
}

func (blockingSubscribeHub) Unsubscribe(string, magicevent.Observer) {}

func (blockingSubscribeHub) Post(magicevent.Event) {}

func (blockingSubscribeHub) Send(magicevent.Event) magicevent.Result {
	return nil
}

func (blockingSubscribeHub) Terminate(_ context.Context) {}

type recordingLaneHub struct {
	events []magicevent.Event
	lanes  []string
}

func (h *recordingLaneHub) Subscribe(string, magicevent.Observer) {}

func (h *recordingLaneHub) Unsubscribe(string, magicevent.Observer) {}

func (h *recordingLaneHub) Post(event magicevent.Event) {
	h.events = append(h.events, event)
	h.lanes = append(h.lanes, event.LaneKey())
}

func (h *recordingLaneHub) Send(event magicevent.Event) magicevent.Result {
	h.events = append(h.events, event)
	h.lanes = append(h.lanes, event.LaneKey())
	return nil
}

func (h *recordingLaneHub) Terminate(_ context.Context) {}

func TestMagicHubPublishWithoutSubscribersSucceeds(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	if err := hub.Publish(context.Background(), Envelope{Topic: runevents.TopicStepDispatched}); err != nil {
		t.Fatalf("Publish without subscribers returned error: %v", err)
	}
	published := hub.Published()
	if len(published) != 1 || published[0].Topic != runevents.TopicStepDispatched {
		t.Fatalf("published = %+v, want one step dispatched event", published)
	}
}

func TestMagicHubUsesTopicLanesForScopedNotificationEvents(t *testing.T) {
	rawHub := &recordingLaneHub{}
	hub := NewMagicHubWithHub(rawHub)

	if err := hub.Publish(context.Background(), Envelope{Topic: workspaceevents.TopicWorkspaceStateChanged, WorkspaceID: "ws-a"}); err != nil {
		t.Fatalf("publish workspace a event: %v", err)
	}
	if err := hub.Publish(context.Background(), Envelope{Topic: runevents.TopicRunStateChanged, WorkspaceID: "ws-a", RunID: "run-001"}); err != nil {
		t.Fatalf("publish run event: %v", err)
	}
	if len(rawHub.lanes) != 2 || rawHub.lanes[0] != "topic/"+workspaceevents.TopicWorkspaceStateChanged || rawHub.lanes[1] != "topic/"+runevents.TopicRunStateChanged {
		t.Fatalf("lanes = %v, want topic lanes for scoped notifications", rawHub.lanes)
	}
}

func TestMagicHubFallsBackToTopicLaneForGlobalEvents(t *testing.T) {
	rawHub := &recordingLaneHub{}
	hub := NewMagicHubWithHub(rawHub)

	if err := hub.Publish(context.Background(), Envelope{Topic: workorchdevents.TopicCLICommand, WorkspaceID: "ws-a", RunID: "run-001"}); err != nil {
		t.Fatalf("publish global event: %v", err)
	}
	if len(rawHub.lanes) != 1 || rawHub.lanes[0] != "topic/"+workorchdevents.TopicCLICommand {
		t.Fatalf("lanes = %v, want topic fallback lane", rawHub.lanes)
	}
}

func TestMagicHubUsesTopicLaneForCLIResultWithScope(t *testing.T) {
	rawHub := &recordingLaneHub{}
	hub := NewMagicHubWithHub(rawHub)

	if err := hub.Publish(context.Background(), Envelope{Topic: workorchdevents.TopicCLIResult, WorkspaceID: "ws-a", RunID: "run-001"}); err != nil {
		t.Fatalf("publish scoped CLI result event: %v", err)
	}
	if len(rawHub.lanes) != 1 || rawHub.lanes[0] != "topic/"+workorchdevents.TopicCLIResult {
		t.Fatalf("lanes = %v, want CLI result topic lane", rawHub.lanes)
	}
}

func TestMagicHubUsesQueryLaneForScopedReadCommands(t *testing.T) {
	rawHub := &recordingLaneHub{}
	hub := NewMagicHubWithHub(rawHub)

	if err := hub.Publish(context.Background(), Envelope{
		Topic:       runevents.TopicRunCommandListStepsByRun,
		WorkspaceID: "ws-a",
		RunID:       "run-001",
	}); err != nil {
		t.Fatalf("publish scoped read command: %v", err)
	}
	if len(rawHub.lanes) != 1 {
		t.Fatalf("lanes = %v, want one query lane", rawHub.lanes)
	}
	want := "query/run/command/list-steps-by-run/workspace/ws-a/run/run-001"
	if rawHub.lanes[0] != want {
		t.Fatalf("lane = %q, want %q", rawHub.lanes[0], want)
	}
}

func TestMagicHubKeepsRunOwnerWriteCommandsOnRunStateLane(t *testing.T) {
	rawHub := &recordingLaneHub{}
	hub := NewMagicHubWithHub(rawHub)

	if err := hub.Publish(context.Background(), Envelope{
		Topic:       runevents.TopicRunCommandUpdate,
		WorkspaceID: "ws-a",
		RunID:       "run-001",
	}); err != nil {
		t.Fatalf("publish run owner write command: %v", err)
	}
	if len(rawHub.lanes) != 1 || rawHub.lanes[0] != "state/run/ws-a/run-001" {
		t.Fatalf("lanes = %v, want run state write command lane", rawHub.lanes)
	}
}

func TestMagicHubKeepsOrchestratorCommandsOnOrchestratorStateLane(t *testing.T) {
	rawHub := &recordingLaneHub{}
	hub := NewMagicHubWithHub(rawHub)

	if err := hub.Publish(context.Background(), Envelope{
		Topic:       orchestratorevents.TopicOrchestratorCommandResumeRun,
		WorkspaceID: "ws-a",
		RunID:       "run-001",
	}); err != nil {
		t.Fatalf("publish orchestrator command: %v", err)
	}
	want := "state/orchestrator/workspace/ws-a/run/run-001"
	if len(rawHub.lanes) != 1 || rawHub.lanes[0] != want {
		t.Fatalf("lanes = %v, want %s", rawHub.lanes, want)
	}
}

func TestMagicHubKeepsOrchestratorDispatchRequestsOnOrchestratorStateLane(t *testing.T) {
	rawHub := &recordingLaneHub{}
	hub := NewMagicHubWithHub(rawHub)

	if err := hub.Publish(context.Background(), Envelope{
		Topic:       orchestratorevents.TopicOrchestratorDispatchStepRequested,
		WorkspaceID: "ws-a",
		RunID:       "run-001",
		StepID:      "run-001-step-2",
	}); err != nil {
		t.Fatalf("publish orchestrator dispatch request: %v", err)
	}
	want := "state/orchestrator/workspace/ws-a/run/run-001"
	if len(rawHub.lanes) != 1 || rawHub.lanes[0] != want {
		t.Fatalf("lanes = %v, want %s", rawHub.lanes, want)
	}
}

func TestMagicHubUsesExecuteLaneForRuntimeBlockCommands(t *testing.T) {
	rawHub := &recordingLaneHub{}
	hub := NewMagicHubWithHub(rawHub)

	if err := hub.Publish(context.Background(), Envelope{
		Topic:       runtimeeventdefs.RuntimeBlockExecuteTopic("mcp/tool_call"),
		WorkspaceID: "ws-a",
		RunID:       "run-001",
		StepID:      "run-001-step-2",
	}); err != nil {
		t.Fatalf("publish runtime block command: %v", err)
	}
	want := "exec/block/mcp-tool-call/workspace/ws-a/step/run-001-step-2"
	if len(rawHub.lanes) != 1 || rawHub.lanes[0] != want {
		t.Fatalf("lanes = %v, want %s", rawHub.lanes, want)
	}
}

func TestMagicHubUsesRuntimeExecutionStateLaneForAcceptedEvents(t *testing.T) {
	rawHub := &recordingLaneHub{}
	hub := NewMagicHubWithHub(rawHub)

	if err := hub.Publish(context.Background(), Envelope{
		Topic:       runtimeeventdefs.TopicRuntimeBlockExecuteAccepted,
		WorkspaceID: "ws-a",
		RunID:       "run-001",
		StepID:      "run-001-step-2",
	}); err != nil {
		t.Fatalf("publish runtime accepted event: %v", err)
	}
	want := "state/runtime-exec/workspace/ws-a/run/run-001/step/run-001-step-2"
	if len(rawHub.lanes) != 1 || rawHub.lanes[0] != want {
		t.Fatalf("lanes = %v, want %s", rawHub.lanes, want)
	}
}

func TestMagicHubUsesOrchestratorStateLaneForRuntimeResultEvents(t *testing.T) {
	for _, topic := range []string{
		runtimeeventdefs.TopicRuntimeBlockExecuteCompleted,
		runtimeeventdefs.TopicRuntimeBlockExecuteFailed,
	} {
		t.Run(topic, func(t *testing.T) {
			rawHub := &recordingLaneHub{}
			hub := NewMagicHubWithHub(rawHub)

			if err := hub.Publish(context.Background(), Envelope{
				Topic:       topic,
				WorkspaceID: "ws-a",
				RunID:       "run-001",
				StepID:      "run-001-step-2",
			}); err != nil {
				t.Fatalf("publish runtime result event: %v", err)
			}
			want := "state/orchestrator/workspace/ws-a/run/run-001"
			if len(rawHub.lanes) != 1 || rawHub.lanes[0] != want {
				t.Fatalf("lanes = %v, want %s", rawHub.lanes, want)
			}
		})
	}
}

func TestMagicHubKeepsRuntimeResultLanesWorkspaceScoped(t *testing.T) {
	rawHub := &recordingLaneHub{}
	hub := NewMagicHubWithHub(rawHub)

	for _, workspaceID := range []string{"ws-a", "ws-b"} {
		if err := hub.Publish(context.Background(), Envelope{
			Topic:       runtimeeventdefs.TopicRuntimeBlockExecuteCompleted,
			WorkspaceID: workspaceID,
			RunID:       "run-001",
			StepID:      "run-001-step-2",
		}); err != nil {
			t.Fatalf("publish runtime result event for %s: %v", workspaceID, err)
		}
	}
	want := []string{
		"state/orchestrator/workspace/ws-a/run/run-001",
		"state/orchestrator/workspace/ws-b/run/run-001",
	}
	if len(rawHub.lanes) != len(want) {
		t.Fatalf("lanes = %v, want %v", rawHub.lanes, want)
	}
	for idx := range want {
		if rawHub.lanes[idx] != want[idx] {
			t.Fatalf("lanes = %v, want %v", rawHub.lanes, want)
		}
	}
}

func TestMagicHubKeepsWorkspaceLocalRunStateLanesIndependent(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	unsubscribeWrite := hub.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, func(_ context.Context, envelope Envelope) error {
		if envelope.WorkspaceID != "ws-a" || envelope.RunID != "run-001" {
			t.Fatalf("write scope = %s/%s, want ws-a/run-001", envelope.WorkspaceID, envelope.RunID)
		}
		close(writeStarted)
		<-releaseWrite
		return Response(Envelope{Topic: envelope.Topic + "/response"})
	})
	defer unsubscribeWrite()

	unsubscribeCreate := hub.Subscribe(runevents.TopicRunCommandCreate, func(_ context.Context, envelope Envelope) error {
		if envelope.WorkspaceID != "ws-b" || envelope.RunID != "run-001" {
			t.Fatalf("create scope = %s/%s, want ws-b/run-001", envelope.WorkspaceID, envelope.RunID)
		}
		return Response(Envelope{
			Topic:   envelope.Topic + "/response",
			Payload: map[string]any{"created": true},
		})
	})
	defer unsubscribeCreate()

	writeDone := make(chan error, 1)
	go func() {
		writeDone <- hub.Publish(context.Background(), Envelope{
			Topic:       orchestratorevents.TopicOrchestratorCommandResumeRun,
			WorkspaceID: "ws-a",
			RunID:       "run-001",
		})
	}()

	select {
	case <-writeStarted:
	case <-time.After(time.Second):
		t.Fatal("workspace A write command did not start")
	}

	createDone := make(chan error, 1)
	go func() {
		createDone <- hub.Publish(context.Background(), Envelope{
			Topic:       runevents.TopicRunCommandCreate,
			WorkspaceID: "ws-b",
			RunID:       "run-001",
		})
	}()

	select {
	case err := <-createDone:
		response, ok := ResponseEnvelope(err)
		if !ok {
			t.Fatalf("create err = %v, want response envelope", err)
		}
		if response.Payload["created"] != true {
			t.Fatalf("create response = %+v, want created payload", response)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("workspace B run create blocked behind workspace A run state lane")
	}

	close(releaseWrite)
	select {
	case err := <-writeDone:
		if _, ok := ResponseEnvelope(err); err != nil && !ok {
			t.Fatalf("write command returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("workspace A write command did not finish")
	}
}

func TestMagicHubDoesNotBlockRunOwnerWriteBehindActiveOrchestratorLane(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	orchestratorStarted := make(chan struct{})
	releaseOrchestrator := make(chan struct{})
	unsubscribeOrchestrator := hub.Subscribe(orchestratorevents.TopicOrchestratorCommandFailRun, func(_ context.Context, envelope Envelope) error {
		if envelope.WorkspaceID != "ws-a" || envelope.RunID != "run-001" {
			t.Fatalf("orchestrator scope = %s/%s, want ws-a/run-001", envelope.WorkspaceID, envelope.RunID)
		}
		close(orchestratorStarted)
		<-releaseOrchestrator
		return Response(Envelope{Topic: envelope.Topic + "/response"})
	})
	defer unsubscribeOrchestrator()

	unsubscribeRunUpdate := hub.Subscribe(runevents.TopicRunCommandUpdate, func(_ context.Context, envelope Envelope) error {
		if envelope.WorkspaceID != "ws-a" || envelope.RunID != "run-001" {
			t.Fatalf("run update scope = %s/%s, want ws-a/run-001", envelope.WorkspaceID, envelope.RunID)
		}
		return Response(Envelope{
			Topic:   envelope.Topic + "/response",
			Payload: map[string]any{"updated": true},
		})
	})
	defer unsubscribeRunUpdate()

	orchestratorDone := make(chan error, 1)
	go func() {
		orchestratorDone <- hub.Publish(context.Background(), Envelope{
			Topic:       orchestratorevents.TopicOrchestratorCommandFailRun,
			WorkspaceID: "ws-a",
			RunID:       "run-001",
		})
	}()

	select {
	case <-orchestratorStarted:
	case <-time.After(time.Second):
		t.Fatal("orchestrator command did not start")
	}

	updateDone := make(chan error, 1)
	go func() {
		updateDone <- hub.Publish(context.Background(), Envelope{
			Topic:       runevents.TopicRunCommandUpdate,
			WorkspaceID: "ws-a",
			RunID:       "run-001",
		})
	}()

	select {
	case err := <-updateDone:
		response, ok := ResponseEnvelope(err)
		if !ok {
			t.Fatalf("run update err = %v, want response envelope", err)
		}
		if response.Payload["updated"] != true {
			t.Fatalf("run update response = %+v, want updated payload", response)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("run owner update blocked behind active orchestrator lane")
	}

	close(releaseOrchestrator)
	select {
	case err := <-orchestratorDone:
		if _, ok := ResponseEnvelope(err); err != nil && !ok {
			t.Fatalf("orchestrator command returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("orchestrator command did not finish")
	}
}

func TestMagicHubReadCommandIsNotBlockedByActiveRunWriteLane(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	unsubscribeWrite := hub.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, func(_ context.Context, envelope Envelope) error {
		if envelope.RunID != "run-001" {
			t.Fatalf("write run ID = %q, want run-001", envelope.RunID)
		}
		close(writeStarted)
		<-releaseWrite
		return Response(Envelope{Topic: envelope.Topic + "/response"})
	})
	defer unsubscribeWrite()
	unsubscribeRead := hub.Subscribe(runevents.TopicRunCommandListStepsByRun, func(_ context.Context, envelope Envelope) error {
		if envelope.RunID != "run-001" {
			t.Fatalf("read run ID = %q, want run-001", envelope.RunID)
		}
		return Response(Envelope{
			Topic:   envelope.Topic + "/response",
			Payload: map[string]any{"ok": true},
		})
	})
	defer unsubscribeRead()

	writeDone := make(chan error, 1)
	go func() {
		writeDone <- hub.Publish(context.Background(), Envelope{
			Topic:       orchestratorevents.TopicOrchestratorCommandResumeRun,
			WorkspaceID: "ws-a",
			RunID:       "run-001",
		})
	}()

	select {
	case <-writeStarted:
	case <-time.After(time.Second):
		t.Fatal("write command did not start")
	}

	readDone := make(chan error, 1)
	go func() {
		readDone <- hub.Publish(context.Background(), Envelope{
			Topic:       runevents.TopicRunCommandListStepsByRun,
			WorkspaceID: "ws-a",
			RunID:       "run-001",
		})
	}()

	select {
	case err := <-readDone:
		response, ok := ResponseEnvelope(err)
		if !ok {
			t.Fatalf("read err = %v, want response envelope", err)
		}
		if response.Payload["ok"] != true {
			t.Fatalf("read response = %+v, want ok payload", response)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("read command blocked behind active run write lane")
	}

	close(releaseWrite)
	select {
	case err := <-writeDone:
		if _, ok := ResponseEnvelope(err); err != nil && !ok {
			t.Fatalf("write command returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("write command did not finish")
	}
}

func TestMagicHubSupportsSubscribeWorkorch(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	var calls int
	unsubscribers := SubscribeWorkorch(hub, func(_ context.Context, event WorkorchEvent) error {
		calls++
		if event.Topic() != runevents.TopicRunStateChanged {
			t.Fatalf("topic = %s, want %s", event.Topic(), runevents.TopicRunStateChanged)
		}
		return nil
	})
	defer func() {
		for _, unsubscribe := range unsubscribers {
			if unsubscribe != nil {
				unsubscribe()
			}
		}
	}()

	if err := hub.Publish(context.Background(), Envelope{
		Version: WorkorchEventVersion,
		Topic:   runevents.TopicRunStateChanged,
		RunID:   "run-1",
		Payload: RunPayload("", "", "running", "", "", ""),
	}); err != nil {
		t.Fatalf("publish event: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestMagicHubDeliversSameTopicToMultipleSubscribers(t *testing.T) {
	hub := NewMagicHub()
	defer hub.Terminate(context.Background())

	var firstCalls int
	var secondCalls int
	unsubscribeFirst := hub.Subscribe(runevents.TopicRunStateChanged, func(_ context.Context, _ Envelope) error {
		firstCalls++
		return nil
	})
	defer unsubscribeFirst()
	unsubscribeSecond := hub.Subscribe(runevents.TopicRunStateChanged, func(_ context.Context, _ Envelope) error {
		secondCalls++
		return nil
	})
	defer unsubscribeSecond()

	if err := hub.Publish(context.Background(), Envelope{Topic: runevents.TopicRunStateChanged}); err != nil {
		t.Fatalf("publish event: %v", err)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("calls = %d/%d, want 1/1", firstCalls, secondCalls)
	}
}

func TestMagicHubPublishesAcrossWrappersSharingSameUnderlyingHub(t *testing.T) {
	rawHub := magicevent.NewHub(defaultMagicHubCapacity)
	defer rawHub.Terminate(context.Background())

	publisher := NewMagicHubWithHubOwner(rawHub, "test/publisher")
	subscriber := NewMagicHubWithHubOwner(rawHub, "test/subscriber")

	var received Envelope
	unsubscribe := subscriber.Subscribe(workspaceevents.TopicWorkspaceCreated, func(_ context.Context, envelope Envelope) error {
		received = envelope
		return nil
	})
	defer unsubscribe()

	if err := publisher.Publish(context.Background(), Envelope{Topic: workspaceevents.TopicWorkspaceCreated}); err != nil {
		t.Fatalf("publish across wrappers returned error: %v", err)
	}
	if received.Topic != workspaceevents.TopicWorkspaceCreated {
		t.Fatalf("received topic = %s, want %s", received.Topic, workspaceevents.TopicWorkspaceCreated)
	}
}

func TestMagicHubRevisionIsSharedAcrossWrappers(t *testing.T) {
	rawHub := magicevent.NewHub(defaultMagicHubCapacity)
	defer rawHub.Terminate(context.Background())

	first := NewMagicHubWithHubOwner(rawHub, "test/revision-first")
	second := NewMagicHubWithHubOwner(rawHub, "test/revision-second")

	if err := first.Publish(context.Background(), Envelope{Topic: workspaceevents.TopicWorkspaceStateChanged, WorkspaceID: "ws-shared"}); err != nil {
		t.Fatalf("publish first wrapper event: %v", err)
	}
	if err := second.Publish(context.Background(), Envelope{Topic: runevents.TopicRunStateChanged, WorkspaceID: "ws-shared", RunID: "run-shared"}); err != nil {
		t.Fatalf("publish second wrapper event: %v", err)
	}

	firstPublished := first.Published()
	secondPublished := second.Published()
	if len(firstPublished) != 1 || len(secondPublished) != 1 {
		t.Fatalf("published counts = %d/%d, want 1/1", len(firstPublished), len(secondPublished))
	}
	if firstPublished[0].Revision != 1 || secondPublished[0].Revision != 2 {
		t.Fatalf("shared wrapper revisions = %d/%d, want 1/2", firstPublished[0].Revision, secondPublished[0].Revision)
	}
}

func TestMagicHubDeliversSameTopicAcrossMultipleWrappers(t *testing.T) {
	rawHub := magicevent.NewHub(defaultMagicHubCapacity)
	defer rawHub.Terminate(context.Background())

	first := NewMagicHubWithHubOwner(rawHub, "test/first")
	second := NewMagicHubWithHubOwner(rawHub, "test/second")
	publisher := NewMagicHubWithHubOwner(rawHub, "test/publisher")

	var firstCalls int
	var secondCalls int
	unsubscribeFirst := first.Subscribe(workspaceevents.TopicWorkspaceCreated, func(_ context.Context, _ Envelope) error {
		firstCalls++
		return nil
	})
	defer unsubscribeFirst()
	unsubscribeSecond := second.Subscribe(workspaceevents.TopicWorkspaceCreated, func(_ context.Context, _ Envelope) error {
		secondCalls++
		return nil
	})
	defer unsubscribeSecond()

	if err := publisher.Publish(context.Background(), Envelope{Topic: workspaceevents.TopicWorkspaceCreated}); err != nil {
		t.Fatalf("publish across wrappers returned error: %v", err)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("calls = %d/%d, want 1/1", firstCalls, secondCalls)
	}
}

func TestMagicHubWildcardSubscriptionsAreWrapperLocal(t *testing.T) {
	rawHub := magicevent.NewHub(defaultMagicHubCapacity)
	defer rawHub.Terminate(context.Background())

	first := NewMagicHubWithHubOwner(rawHub, "test/first")
	second := NewMagicHubWithHubOwner(rawHub, "test/second")

	var firstCalls int
	var secondCalls int
	unsubscribeFirst := first.Subscribe("*", func(_ context.Context, _ Envelope) error {
		firstCalls++
		return nil
	})
	defer unsubscribeFirst()
	unsubscribeSecond := second.Subscribe("*", func(_ context.Context, _ Envelope) error {
		secondCalls++
		return nil
	})
	defer unsubscribeSecond()

	if err := first.Publish(context.Background(), Envelope{Topic: workspaceevents.TopicWorkspaceCreated}); err != nil {
		t.Fatalf("publish local wildcard returned error: %v", err)
	}
	if firstCalls != 1 || secondCalls != 0 {
		t.Fatalf("wildcard calls = %d/%d, want 1/0", firstCalls, secondCalls)
	}
}

func TestMagicHubObserverIDUsesStableOwnerName(t *testing.T) {
	hub := NewMagicHubWithOwner("workorchd/http")
	defer hub.Terminate(context.Background())

	unsubscribe := hub.Subscribe(workspaceevents.TopicWorkspaceCreated, func(_ context.Context, _ Envelope) error {
		return nil
	})
	defer unsubscribe()

	observer := hub.observers[workspaceevents.TopicWorkspaceCreated]
	if observer == nil {
		t.Fatal("workspace observer was not registered")
	}
	want := "/_workorch_observer/workorchd/http/workspace/created"
	if observer.ID() != want {
		t.Fatalf("observer ID = %q, want %q", observer.ID(), want)
	}
}
