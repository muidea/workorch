package backend

import (
	"context"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	appservice "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestEventHubBackendRoutesWorkspaceCreateThroughCLICommand(t *testing.T) {
	ctx := context.Background()
	be, eventHub := newEventHubBackendWithDispatcher(t, ctx)

	ws, err := be.Workspace().Create(ctx, CreateWorkspaceRequest{
		Title:                "eventhub workspace",
		Description:          "created through cli command event",
		DeliverableDirectory: "/tmp/eventhub-workspace",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if ws == nil || ws.ID == "" {
		t.Fatalf("workspace = %+v, want id", ws)
	}

	publishedHub, ok := eventHub.(interface{ Published() []events.Envelope })
	if !ok {
		t.Fatal("event hub does not expose published envelopes")
	}
	var sawCommand, sawResult bool
	for _, envelope := range publishedHub.Published() {
		switch envelope.Topic {
		case workorchdevents.TopicCLICommand:
			command, ok := events.DecodeCLICommandPayload(envelope.Payload)
			if ok && command.Command == events.CommandWorkspaceCreate {
				sawCommand = true
			}
		case workorchdevents.TopicCLIResult:
			result, ok := events.DecodeCLIResultPayload(envelope.Payload)
			if ok && result.OK && result.CommandID != "" {
				sawResult = true
			}
		}
	}
	if !sawCommand {
		t.Fatalf("published command=%v, want workspace create command", sawCommand)
	}
	if sawResult {
		t.Fatalf("published result=%v, want synchronous response without duplicate CLI result event", sawResult)
	}
}

func TestEventHubBackendCLICommandUsesAsyncDispatchWithoutSendTimeout(t *testing.T) {
	ctx := context.Background()
	be, _ := newEventHubBackendWithDispatcher(t, ctx)

	done := make(chan error, 1)
	go func() {
		_, err := be.Workspace().Create(ctx, CreateWorkspaceRequest{
			Title:                "eventhub sync response workspace",
			DeliverableDirectory: "/tmp/eventhub-sync-response-workspace",
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("create workspace: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("workspace create blocked waiting for nested CLI result event")
	}
}

func TestEventHubEventsBackendReceivesProjectionAcrossHubWrappers(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	defer rawHub.Terminate(ctx)

	subscriberHub := events.NewMagicHubWithHubOwner(rawHub, "workorch/tui")
	publisherHub := events.NewMagicHubWithHubOwner(rawHub, "workorchd/projection")
	be := NewEventHub(subscriberHub)
	received := make(chan Event, 1)
	unsubscribe, err := be.Events().Subscribe(ctx, func(_ context.Context, evt Event) {
		if evt.Type == EventWorkspaceProjectionChanged {
			received <- evt
		}
	})
	if err != nil {
		t.Fatalf("subscribe events: %v", err)
	}
	defer unsubscribe()

	if err := publisherHub.Publish(ctx, events.Envelope{
		Topic:       workorchdevents.TopicWorkspaceProjectionChanged,
		WorkspaceID: "ws-projection",
		Payload:     map[string]any{"changeKind": "detail"},
	}); err != nil {
		t.Fatalf("publish projection event: %v", err)
	}

	select {
	case evt := <-received:
		if evt.WorkspaceID != "ws-projection" || evt.Payload["changeKind"] != "detail" {
			t.Fatalf("event = %+v, want projection detail for ws-projection", evt)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for projection event across hub wrappers")
	}
}

func TestEventHubRunResourceMapCompactsExecutionContext(t *testing.T) {
	large := strings.Repeat("x", 1024*1024)
	resource := resourceMap(RunInfo{
		ID:          "run-1",
		WorkspaceID: "ws-1",
		GoalID:      "goal-1",
		DoDID:       "dod-1",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			"goal":               map[string]any{"text": "goal text"},
			"dod":                map[string]any{"items": []any{"dod item"}},
			"operatorInputQueue": []map[string]any{{"content": "continue"}},
			"processEvidenceTimeline": []any{
				map[string]any{"content": large},
			},
		},
	})

	contextMap, ok := resource["ExecutionContext"].(map[string]any)
	if !ok || contextMap == nil {
		t.Fatalf("ExecutionContext = %+v, want compact context", resource["ExecutionContext"])
	}
	if _, ok := contextMap["processEvidenceTimeline"]; ok {
		t.Fatalf("ExecutionContext contains processEvidenceTimeline, want heavy runtime fields omitted")
	}
	goal, _ := contextMap["goal"].(map[string]any)
	if goal["text"] != "goal text" {
		t.Fatalf("goal = %+v, want preserved display context", goal)
	}
	queue, _ := contextMap["operatorInputQueue"].([]map[string]any)
	if len(queue) != 1 {
		t.Fatalf("operatorInputQueue = %+v, want preserved operator input queue", contextMap["operatorInputQueue"])
	}
}

func TestEventHubSnapshotResourceMapCompactsRunExecutionContext(t *testing.T) {
	resource := resourceMap(WorkspaceSnapshotInfo{
		Workspace: WorkspaceInfo{ID: "ws-1", State: common.WorkspaceStateRunning},
		Runs: []RunInfo{{
			ID:          "run-1",
			State:       common.RunStateRunning,
			GoalID:      "goal-1",
			DoDID:       "dod-1",
			WorkspaceID: "ws-1",
			ExecutionContext: map[string]any{
				"goal":              map[string]any{"text": "goal text"},
				"coverageWorkItems": []any{map[string]any{"large": strings.Repeat("x", 1024)}},
			},
		}},
	})

	runs, _ := resource["Runs"].([]map[string]any)
	if len(runs) != 1 {
		t.Fatalf("Runs = %+v, want one compact run", resource["Runs"])
	}
	contextMap, _ := runs[0]["ExecutionContext"].(map[string]any)
	if _, ok := contextMap["coverageWorkItems"]; ok {
		t.Fatalf("run context contains coverageWorkItems, want compact snapshot run context")
	}
	goal, _ := contextMap["goal"].(map[string]any)
	if goal["text"] != "goal text" {
		t.Fatalf("goal = %+v, want preserved run goal context", goal)
	}
}

func TestEventHubBackendReadsWorkspaceSnapshotThroughCLICommand(t *testing.T) {
	ctx := context.Background()
	be, eventHub := newEventHubBackendWithDispatcher(t, ctx)

	ws, err := be.Workspace().Create(ctx, CreateWorkspaceRequest{Title: "snapshot workspace"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	snapshotBackend, ok := be.Workspace().(interface {
		Snapshot(context.Context, common.WorkspaceID) (*WorkspaceSnapshotInfo, error)
	})
	if !ok {
		t.Fatal("workspace backend does not expose snapshot reader")
	}
	snapshot, err := snapshotBackend.Snapshot(ctx, ws.ID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot == nil || snapshot.Workspace.ID != ws.ID {
		t.Fatalf("snapshot = %+v, want workspace %s", snapshot, ws.ID)
	}
	publishedHub, ok := eventHub.(interface{ Published() []events.Envelope })
	if !ok {
		t.Fatal("event hub does not expose published envelopes")
	}
	for _, envelope := range publishedHub.Published() {
		if envelope.Topic != workorchdevents.TopicCLICommand {
			continue
		}
		command, ok := events.DecodeCLICommandPayload(envelope.Payload)
		if ok && command.Command == events.CommandWorkspaceSnapshot {
			return
		}
	}
	t.Fatal("workspace snapshot did not publish CLI command")
}

func TestConvertEnvelopeMapsGoalAndDoDEvents(t *testing.T) {
	projectionEvent := convertEnvelope(events.Envelope{
		Topic:       workorchdevents.TopicWorkspaceProjectionChanged,
		WorkspaceID: "ws-1",
		Payload: map[string]any{
			"globalRevision":    float64(2),
			"workspaceRevision": float64(1),
			"changeKind":        "detail",
		},
	})
	if projectionEvent.Type != EventWorkspaceProjectionChanged || projectionEvent.WorkspaceID != "ws-1" || projectionEvent.Payload["changeKind"] != "detail" {
		t.Fatalf("projection event = %+v, want workspace projection changed", projectionEvent)
	}

	goalEvent := convertEnvelope(events.Envelope{
		Topic:       workspaceevents.TopicGoalDefined,
		SubjectID:   "goal-1",
		WorkspaceID: "ws-1",
		Payload:     events.GoalPayload("updated goal", "", "", "", ""),
	})
	if goalEvent.Type != EventGoalDefined || goalEvent.WorkspaceID != "ws-1" || goalEvent.Payload["goalId"] != "goal-1" {
		t.Fatalf("goal event = %+v, want converted goal definition event", goalEvent)
	}
	dodEvent := convertEnvelope(events.Envelope{
		Topic:       workspaceevents.TopicDoDDefined,
		SubjectID:   "dod-1",
		WorkspaceID: "ws-1",
		Payload:     events.DoDPayload([]string{"tests pass"}, common.DoDStateReady, "", "", "", ""),
	})
	if dodEvent.Type != EventDoDDefined || dodEvent.WorkspaceID != "ws-1" || dodEvent.Payload["doDId"] != "dod-1" {
		t.Fatalf("dod event = %+v, want converted dod definition event", dodEvent)
	}
}

func TestEventHubGoalAndDoDUpdatesPublishDefinitionEvents(t *testing.T) {
	ctx := context.Background()
	be, eventHub := newEventHubBackendWithDispatcher(t, ctx)

	ws, err := be.Workspace().Create(ctx, CreateWorkspaceRequest{Title: "definition refresh workspace"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	goal, err := be.Goal().Define(ctx, ws.ID, "initial goal")
	if err != nil {
		t.Fatalf("define goal: %v", err)
	}
	if _, err := be.Goal().Update(ctx, ws.ID, goal.ID, "updated goal"); err != nil {
		t.Fatalf("update goal: %v", err)
	}
	dod, err := be.DoD().Define(ctx, ws.ID, []string{"initial dod"})
	if err != nil {
		t.Fatalf("define dod: %v", err)
	}
	if _, err := be.DoD().Update(ctx, ws.ID, dod.ID, []string{"updated dod"}); err != nil {
		t.Fatalf("update dod: %v", err)
	}

	publishedHub, ok := eventHub.(interface{ Published() []events.Envelope })
	if !ok {
		t.Fatal("event hub does not expose published envelopes")
	}
	var sawUpdatedGoal, sawUpdatedDoD bool
	for _, envelope := range publishedHub.Published() {
		switch envelope.Topic {
		case workspaceevents.TopicGoalDefined:
			if envelope.SubjectID == string(goal.ID) && envelope.WorkspaceID == string(ws.ID) && envelope.Payload[events.PayloadKeyText] == "- updated goal" {
				sawUpdatedGoal = true
			}
		case workspaceevents.TopicDoDDefined:
			items, _ := envelope.Payload[events.PayloadKeyItems].([]string)
			if envelope.SubjectID == string(dod.ID) && envelope.WorkspaceID == string(ws.ID) && len(items) == 1 && items[0] == "updated dod" {
				sawUpdatedDoD = true
			}
		}
	}
	if !sawUpdatedGoal || !sawUpdatedDoD {
		t.Fatalf("published updated goal=%v dod=%v, want both definition update events", sawUpdatedGoal, sawUpdatedDoD)
	}
}

func TestEventHubBackendManagesWorkspaceMemoryThroughCLICommands(t *testing.T) {
	ctx := context.Background()
	be, eventHub := newEventHubBackendWithDispatcher(t, ctx)

	ws, err := be.Workspace().Create(ctx, CreateWorkspaceRequest{
		Title:                "memory workspace",
		DeliverableDirectory: "/tmp/memory-workspace",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	created, err := be.Memory().Create(ctx, ws.ID, &MemoryItemInfo{
		ID:         "memory-cli-1",
		SourceType: "user",
		MemoryType: "project_context",
		Summary:    "eventhub memory",
		Content:    "created through eventhub backend",
		Tags:       []string{"eventhub"},
		Importance: 50,
		Confidence: 50,
		Visibility: "workspace",
	})
	if err != nil {
		t.Fatalf("create memory: %v", err)
	}
	if created == nil || created.ID != "memory-cli-1" || created.WorkspaceID != ws.ID {
		t.Fatalf("created memory = %+v, want persisted item in workspace %s", created, ws.ID)
	}
	list, err := be.Memory().List(ctx, ws.ID)
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, want created memory", list)
	}
	search, err := be.Memory().Search(ctx, ws.ID, MemorySearchRequest{Query: "eventhub"})
	if err != nil {
		t.Fatalf("search memory: %v", err)
	}
	if len(search) != 1 || search[0].ID != created.ID {
		t.Fatalf("search = %+v, want created memory", search)
	}
	got, err := be.Memory().Get(ctx, ws.ID, created.ID)
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got == nil || got.Summary != "eventhub memory" {
		t.Fatalf("got = %+v, want created memory", got)
	}
	updated, err := be.Memory().Update(ctx, ws.ID, created.ID, &MemoryItemInfo{Summary: "updated eventhub memory"})
	if err != nil {
		t.Fatalf("update memory: %v", err)
	}
	if updated == nil || updated.Summary != "updated eventhub memory" {
		t.Fatalf("updated = %+v, want updated summary", updated)
	}
	if err := be.Memory().Delete(ctx, ws.ID, created.ID); err != nil {
		t.Fatalf("delete memory: %v", err)
	}
	list, err = be.Memory().List(ctx, ws.ID)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("list after delete = %+v, want empty", list)
	}

	publishedHub, ok := eventHub.(interface{ Published() []events.Envelope })
	if !ok {
		t.Fatal("event hub does not expose published envelopes")
	}
	var sawCreate, sawUpdate, sawDelete bool
	for _, envelope := range publishedHub.Published() {
		switch envelope.Topic {
		case memoryevents.TopicMemoryCreated:
			sawCreate = true
		case memoryevents.TopicMemoryUpdated:
			sawUpdate = true
		case memoryevents.TopicMemoryDeleted:
			sawDelete = true
		}
	}
	if !sawCreate || !sawUpdate || !sawDelete {
		t.Fatalf("memory events create=%v update=%v delete=%v, want all", sawCreate, sawUpdate, sawDelete)
	}
}

func newEventHubBackendWithDispatcher(t *testing.T, ctx context.Context) (Backend, events.Hub) {
	t.Helper()
	session, err := appservice.NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() { _ = session.Close(ctx) })
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	dispatcher := appservice.NewCLICommandDispatcher(session, exports.Infrastructure.EventHub, testWorkspaceViewReader{session: session})
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("run dispatcher: %v", err)
	}
	t.Cleanup(dispatcher.Close)
	return NewEventHub(exports.Infrastructure.EventHub), exports.Infrastructure.EventHub
}

type testWorkspaceViewReader struct {
	session *appservice.RuntimeSession
}

func (r testWorkspaceViewReader) ListWorkspaceView(ctx context.Context) ([]appservice.Workspace, error) {
	return appservice.ListWorkspacesWithSession(ctx, r.session.Borrow())
}

func (r testWorkspaceViewReader) ReadWorkspaceViewSnapshot(ctx context.Context, workspaceID string) (appservice.WorkspaceSnapshot, error) {
	return appservice.ReadWorkspaceSnapshotWithSession(ctx, r.session.Borrow(), workspaceID)
}

func TestConvertEnvelopeAddsBlockingSubjectIDs(t *testing.T) {
	created := convertEnvelope(events.Envelope{
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-created",
		Payload:     events.WorkspacePayload("created", "", "/tmp/created", "", "active", ""),
	})
	if created.Type != EventWorkspaceCreated || created.WorkspaceID != "ws-created" {
		t.Fatalf("created event = %+v, want workspace created", created)
	}

	deleted := convertEnvelope(events.Envelope{
		Topic:       workspaceevents.TopicWorkspaceDeleted,
		WorkspaceID: "ws-deleted",
		Payload:     events.WorkspacePayload("deleted", "", "/tmp/deleted", "", "active", ""),
	})
	if deleted.Type != EventWorkspaceDeleted || deleted.WorkspaceID != "ws-deleted" {
		t.Fatalf("deleted event = %+v, want workspace deleted", deleted)
	}

	decision := convertEnvelope(events.Envelope{
		Topic:       decisionevents.TopicTicketCreated,
		SubjectID:   "ticket-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     events.DecisionTicketPayload("missing city", "open"),
	})
	if decision.Type != EventDecisionCreated || decision.Payload["ticketId"] != "ticket-1" {
		t.Fatalf("decision event = %+v, want ticketId from subject", decision)
	}

	approval := convertEnvelope(events.Envelope{
		Topic:       approvalevents.TopicApprovalCreated,
		SubjectID:   "approval-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     events.ApprovalPayload("approve shell", "run command", map[string]any{"risk": "shell"}, "pending"),
	})
	if approval.Type != EventApprovalCreated || approval.Payload["approvalId"] != "approval-1" {
		t.Fatalf("approval event = %+v, want approvalId from subject", approval)
	}

	governance := convertEnvelope(events.Envelope{
		Topic:       governanceevents.TopicGovernanceAutoResolved,
		SubjectID:   "trace-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"itemType": "approval",
			"itemId":   "approval-1",
			"action":   "approve",
			"status":   "resolved",
		},
	})
	if governance.Type != EventGovernanceResolved || governance.Payload["traceId"] != "trace-1" {
		t.Fatalf("governance event = %+v, want governance resolved with trace id", governance)
	}

	artifact := convertEnvelope(events.Envelope{
		Topic:       orchestratorevents.TopicArtifactContinuationChunkRecorded,
		SubjectID:   "artifact-continuation-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: map[string]any{
			"state":     "chunking",
			"targetRef": "deliverable.txt",
		},
	})
	if artifact.Type != EventArtifactContinuationChanged || artifact.Payload["artifactContinuationId"] != "artifact-continuation-1" {
		t.Fatalf("artifact continuation event = %+v, want artifact continuation id from subject", artifact)
	}
}
