package backend

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	appservice "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestEventHubBackendRoutesWorkspaceCreateThroughCLICommand(t *testing.T) {
	ctx := context.Background()
	be, commandHub := newEventHubBackendWithDispatcher(t, ctx)

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

	recordedHub, ok := commandHub.(*recordingEventHub)
	if !ok {
		t.Fatal("command hub does not expose sent events")
	}
	var sawCommand bool
	for _, event := range recordedHub.Sent() {
		command, ok := event.Data().(workorchdevents.CLICommand)
		if ok && command.Command == workorchdevents.CommandWorkspaceCreate && command.CreateWorkspace != nil {
			sawCommand = true
		}
	}
	if !sawCommand {
		t.Fatalf("published command=%v, want workspace create command", sawCommand)
	}
}

func TestEventHubBackendCLICommandUsesSynchronousTypedResult(t *testing.T) {
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
		t.Fatal("workspace create blocked waiting for typed command result")
	}
}

func TestEventHubEventsBackendReceivesProjectionAcrossHubWrappers(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	defer rawHub.Terminate(ctx)

	be := NewEventHub(rawHub)
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

	projectionEvent := magicevent.NewEventWithContext(workorchdevents.TopicWorkspaceProjectionChanged, "workorchd/projection", workorchdevents.NotificationDestination, magicevent.NewHeader(), ctx, workorchdevents.WorkspaceProjectionChangedData{WorkspaceID: "ws-projection", ChangeKind: "detail"})
	rawHub.Post(projectionEvent)

	select {
	case evt := <-received:
		if evt.WorkspaceID != "ws-projection" || evt.Payload["changeKind"] != "detail" {
			t.Fatalf("event = %+v, want projection detail for ws-projection", evt)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for projection event across hub wrappers")
	}
}

func TestEventHubEventsBackendReceivesTypedEventStreamRecord(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	defer rawHub.Terminate(ctx)

	be := NewEventHub(rawHub)
	received := make(chan Event, 1)
	unsubscribe, err := be.Events().Subscribe(ctx, func(_ context.Context, evt Event) {
		if evt.Type == EventWorkspaceCreated {
			received <- evt
		}
	})
	if err != nil {
		t.Fatalf("subscribe events: %v", err)
	}
	defer unsubscribe()

	data, err := eventstreamevents.EventRecordDataFromModel(&eventstreammodel.EventRecord{
		ID:          "eventstream-workspace-created",
		Version:     events.WorkorchEventVersion,
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-eventstream",
		SubjectType: "workspace",
		SubjectID:   "ws-eventstream",
		Revision:    1,
		Payload:     map[string]any{"title": "typed event stream"},
	})
	if err != nil {
		t.Fatalf("build event record data: %v", err)
	}
	event := magicevent.NewEventWithContext(
		eventstreamevents.TopicEventStreamRecordAppended,
		"kernel/eventstream",
		eventstreamevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		eventstreamevents.EventRecordAppendedData{Record: data},
	)
	result := rawHub.Send(event)
	if result == nil || result.Error() != nil {
		t.Fatalf("publish typed event stream record: %v", result)
	}

	select {
	case evt := <-received:
		if evt.ID != "eventstream-workspace-created" || evt.WorkspaceID != "ws-eventstream" || evt.Payload["title"] != "typed event stream" {
			t.Fatalf("event = %+v, want typed workspace event", evt)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for typed event stream record")
	}
}

func TestDecodeResultResourceUsesTypedPayloadBeforeJSONFallback(t *testing.T) {
	want := WorkspaceSnapshotInfo{
		Workspace: WorkspaceInfo{ID: "ws-typed", State: common.WorkspaceStateActive},
	}
	got, err := decodeResultResource[WorkspaceSnapshotInfo](workorchdevents.CLIResult{
		WorkspaceSnapshot: &want,
	}, nil)
	if err != nil {
		t.Fatalf("decode typed resource: %v", err)
	}
	if got == nil || got.Workspace.ID != "ws-typed" {
		t.Fatalf("typed resource = %+v, want workspace ws-typed", got)
	}
}

func TestEventHubRunRestartPublishesUnifiedWorkspaceRunCommand(t *testing.T) {
	ctx := context.Background()
	be, commandHub := newEventHubBackendWithDispatcher(t, ctx)
	var captured workorchdevents.WorkspaceRunRequestedCommand
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, commandHub)
	observer.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(event magicevent.Event, result magicevent.Result) {
		captured, _ = event.Data().(workorchdevents.WorkspaceRunRequestedCommand)
		result.Set(workorchdevents.WorkspaceRunRequestedResult{Accepted: true}, nil)
	})
	defer observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested)

	if _, err := be.Run().Command(ctx, WorkspaceRunCommand{
		WorkspaceID:        "ws-restart-unified",
		Intent:             RunCommandIntentRestart,
		RequestID:          "request-restart-unified",
		Reason:             "test_restart_unified",
		Source:             "test",
		Priority:           7,
		DefinitionRevision: "rev-restart-unified",
		DecisionID:         "ticket-restart-unified",
		Activation: ActivateRunRequest{
			GoalID:        "goal-restart-unified",
			DoDID:         "dod-restart-unified",
			LLMProviderID: "provider-restart-unified",
			SoulID:        "soul-restart-unified",
			SoulBindings: map[string]common.SoulID{
				"validation": "soul-validation-restart-unified",
			},
			SkillIDs:      []common.SkillID{"skill-restart-unified"},
			MCPServerIDs:  []common.MCPServerID{"mcp-restart-unified"},
			CapabilityIDs: []common.MCPCapabilityID{"mcp-restart-unified:tool:validate"},
			MemoryIDs:     []common.MemoryID{"memory-restart-unified"},
		},
	}); err != nil {
		t.Fatalf("restart: %v", err)
	}

	if captured.WorkspaceID != "ws-restart-unified" || captured.Intent != string(RunCommandIntentRestart) || captured.RequestID != "request-restart-unified" || captured.Reason != "test_restart_unified" || captured.Source != "test" || captured.DefinitionRevision != "rev-restart-unified" || captured.DecisionID != "ticket-restart-unified" || captured.Priority != 7 || captured.GoalID != "goal-restart-unified" || captured.DoDID != "dod-restart-unified" || captured.LLMProviderID != "provider-restart-unified" || captured.SoulID != "soul-restart-unified" {
		t.Fatalf("workspace run restart command = %+v", captured)
	}
	if captured.SoulBindings["validation"] != "soul-validation-restart-unified" || len(captured.SkillIDs) != 1 || len(captured.MCPServerIDs) != 1 || len(captured.CapabilityIDs) != 1 || len(captured.MemoryIDs) != 1 {
		t.Fatalf("workspace run activation contract = %+v", captured)
	}
}

func TestEventHubRunCommandRequiresSchedulerAcceptedResponse(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		subscribe func(magicevent.Hub) func()
		wantError string
	}{
		{
			name:      "missing scheduler response",
			wantError: "missing observer",
		},
		{
			name: "wrong response type",
			subscribe: func(hub magicevent.Hub) func() {
				return subscribeWorkspaceRunResult(t, hub, struct{ Accepted bool }{Accepted: true})
			},
			wantError: "was not accepted",
		},
		{
			name: "not accepted",
			subscribe: func(hub magicevent.Hub) func() {
				return subscribeWorkspaceRunResult(t, hub, workorchdevents.WorkspaceRunRequestedResult{Accepted: false})
			},
			wantError: "was not accepted",
		},
		{
			name: "accepted",
			subscribe: func(hub magicevent.Hub) func() {
				return subscribeWorkspaceRunResult(t, hub, workorchdevents.WorkspaceRunRequestedResult{Accepted: true})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rawHub := magicevent.NewHub(16)
			defer rawHub.Terminate(ctx)
			if tc.subscribe != nil {
				unsubscribe := tc.subscribe(rawHub)
				defer unsubscribe()
			}
			be := NewEventHub(rawHub)
			_, err := be.Run().Command(ctx, WorkspaceRunCommand{
				WorkspaceID: "ws-run-command-ack",
				Intent:      RunCommandIntentResume,
				RequestID:   "request-run-command-ack",
				Source:      "test",
			})
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("run command: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("run command err = %v, want containing %q", err, tc.wantError)
			}
		})
	}
}

func TestEventHubBackendReadsWorkspaceSnapshotThroughCLICommand(t *testing.T) {
	ctx := context.Background()
	be, commandHub := newEventHubBackendWithDispatcher(t, ctx)

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
	recordedHub, ok := commandHub.(*recordingEventHub)
	if !ok {
		t.Fatal("command hub does not expose sent events")
	}
	for _, event := range recordedHub.Sent() {
		command, ok := event.Data().(workorchdevents.CLICommand)
		if ok && command.Command == workorchdevents.CommandWorkspaceSnapshot {
			return
		}
	}
	t.Fatal("workspace snapshot did not publish CLI command")
}

func TestEventHubRunCommandRejectsUnsupportedIntent(t *testing.T) {
	be := NewEventHub(nil)
	_, err := be.Run().Command(context.Background(), WorkspaceRunCommand{
		WorkspaceID:   "ws-eventhub",
		ExpectedRunID: "run-001",
		Intent:        RunCommandIntent("unsupported"),
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported run command intent") {
		t.Fatalf("err = %v, want unsupported run command intent", err)
	}
}

func TestEventHubRunCommandRejectsInvalidIntentScopeBeforePublish(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  WorkspaceRunCommand
	}{
		{
			name: "restart with run scope",
			req: WorkspaceRunCommand{
				WorkspaceID:   "ws-eventhub",
				ExpectedRunID: "run-eventhub",
				Intent:        RunCommandIntentRestart,
			},
		},
		{
			name: "replan without run scope",
			req: WorkspaceRunCommand{
				WorkspaceID: "ws-eventhub",
				Intent:      RunCommandIntentReplan,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			be := NewEventHub(nil)
			_, err := be.Run().Command(context.Background(), tc.req)
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
		})
	}
}

func TestEventHubGovernanceObjectCommandsRequireWorkspaceScopeBeforePublish(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		call func(Backend) error
	}{
		{
			name: "get decision",
			call: func(be Backend) error {
				_, err := be.Decision().Get(ctx, "", "ticket-eventhub")
				return err
			},
		},
		{
			name: "resolve decision",
			call: func(be Backend) error {
				_, err := be.Decision().Resolve(ctx, ResolveDecisionRequest{TicketID: "ticket-eventhub", ResolutionType: "accept"})
				return err
			},
		},
		{
			name: "batch resolve decisions",
			call: func(be Backend) error {
				_, err := be.Decision().BatchResolve(ctx, "", []common.TicketID{"ticket-eventhub"}, "accept", nil)
				return err
			},
		},
		{
			name: "get approval",
			call: func(be Backend) error {
				_, err := be.Approval().Get(ctx, "", "approval-eventhub")
				return err
			},
		},
		{
			name: "approve approval",
			call: func(be Backend) error {
				_, err := be.Approval().Approve(ctx, "", "approval-eventhub")
				return err
			},
		},
		{
			name: "reject approval",
			call: func(be Backend) error {
				_, err := be.Approval().Reject(ctx, "", "approval-eventhub")
				return err
			},
		},
		{
			name: "batch resolve approvals",
			call: func(be Backend) error {
				_, err := be.Approval().BatchResolve(ctx, "", []common.ApprovalID{"approval-eventhub"}, "approve")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			be := NewEventHub(nil)
			err := tc.call(be)
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
		})
	}
}

func TestEventHubWorkspaceObjectCommandsCarryWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	be, commandHub := newEventHubBackendWithDispatcher(t, ctx)

	ws, err := be.Workspace().Create(ctx, CreateWorkspaceRequest{Title: "workspace object scope"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := be.Workspace().Get(ctx, ws.ID); err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	snapshotBackend, ok := be.Workspace().(interface {
		Snapshot(context.Context, common.WorkspaceID) (*WorkspaceSnapshotInfo, error)
	})
	if !ok {
		t.Fatal("workspace backend does not expose snapshot reader")
	}
	if _, err := snapshotBackend.Snapshot(ctx, ws.ID); err != nil {
		t.Fatalf("snapshot workspace: %v", err)
	}
	if err := be.Workspace().Delete(ctx, ws.ID, false); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}

	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.CommandWorkspaceGet, ws.ID, string(ws.ID))
	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.CommandWorkspaceSnapshot, ws.ID, string(ws.ID))
	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.CommandWorkspaceDelete, ws.ID, string(ws.ID))
}

func TestConvertEnvelopeMapsGoalAndDoDEvents(t *testing.T) {
	projectionEvent := convertBackendEvent(backendEventData{
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

	goalEvent := convertBackendEvent(backendEventData{
		Topic:       workspaceevents.TopicGoalDefined,
		SubjectID:   "goal-1",
		WorkspaceID: "ws-1",
		Payload:     events.GoalPayload("updated goal", "", "", "", ""),
	})
	if goalEvent.Type != EventGoalDefined || goalEvent.WorkspaceID != "ws-1" || goalEvent.Payload["goalId"] != "goal-1" {
		t.Fatalf("goal event = %+v, want converted goal definition event", goalEvent)
	}
	dodEvent := convertBackendEvent(backendEventData{
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
	be, commandHub := newEventHubBackendWithDispatcher(t, ctx)
	goalUpdated := make(chan struct{}, 1)
	dodUpdated := make(chan struct{}, 1)
	const observerID = "/workspace/consumers/workorch/backend-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, commandHub)
	observer.Subscribe(workspaceevents.TopicGoalDefined, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(workspaceevents.GoalDefinedData); ok && data.Text == "- updated goal" {
			goalUpdated <- struct{}{}
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	observer.Subscribe(workspaceevents.TopicDoDDefined, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(workspaceevents.DoDDefinedData); ok && len(data.Items) == 1 && data.Items[0] == "updated dod" {
			dodUpdated <- struct{}{}
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	defer observer.Unsubscribe(workspaceevents.TopicGoalDefined)
	defer observer.Unsubscribe(workspaceevents.TopicDoDDefined)

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

	select {
	case <-goalUpdated:
	case <-time.After(time.Second):
		t.Fatal("updated goal notification not received")
	}
	select {
	case <-dodUpdated:
	case <-time.After(time.Second):
		t.Fatal("updated DoD notification not received")
	}
}

func TestEventHubGoalAndDoDGetUpdateCommandsCarryWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	be, commandHub := newEventHubBackendWithDispatcher(t, ctx)

	ws, err := be.Workspace().Create(ctx, CreateWorkspaceRequest{Title: "definition scope workspace"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	goal, err := be.Goal().Define(ctx, ws.ID, "scoped goal")
	if err != nil {
		t.Fatalf("define goal: %v", err)
	}
	if _, err := be.Goal().Get(ctx, ws.ID, goal.ID); err != nil {
		t.Fatalf("get goal: %v", err)
	}
	if _, err := be.Goal().Update(ctx, ws.ID, goal.ID, "updated scoped goal"); err != nil {
		t.Fatalf("update goal: %v", err)
	}
	dod, err := be.DoD().Define(ctx, ws.ID, []string{"scoped dod"})
	if err != nil {
		t.Fatalf("define dod: %v", err)
	}
	if _, err := be.DoD().Get(ctx, ws.ID, dod.ID); err != nil {
		t.Fatalf("get dod: %v", err)
	}
	if _, err := be.DoD().Update(ctx, ws.ID, dod.ID, []string{"updated scoped dod"}); err != nil {
		t.Fatalf("update dod: %v", err)
	}

	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.CommandGoalGet, ws.ID, string(goal.ID))
	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.CommandGoalUpdate, ws.ID, string(goal.ID))
	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.CommandDoDGet, ws.ID, string(dod.ID))
	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.CommandDoDUpdate, ws.ID, string(dod.ID))
}

func TestEventHubRunReadCommandsCarryWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(ctx) })
	recordedHub := &recordingEventHub{Hub: rawHub}
	unsubscribe := subscribeRunReadCLIResponses(t, rawHub, "ws-run-read-scope", "run-read-scope")
	defer unsubscribe()
	be := NewEventHub(recordedHub)

	if _, err := be.Run().Get(ctx, "ws-run-read-scope", "run-read-scope"); err != nil {
		t.Fatalf("get run: %v", err)
	}
	if _, err := be.Run().List(ctx, "ws-run-read-scope"); err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if _, err := be.Run().Status(ctx, "ws-run-read-scope", "run-read-scope"); err != nil {
		t.Fatalf("run status: %v", err)
	}

	assertCLICommandWorkspaceScope(t, recordedHub, workorchdevents.CommandRunGet, "ws-run-read-scope", "run-read-scope")
	assertCLICommandWorkspaceScope(t, recordedHub, workorchdevents.CommandRunList, "ws-run-read-scope", "")
	assertCLICommandWorkspaceScope(t, recordedHub, workorchdevents.CommandRunStatus, "ws-run-read-scope", "run-read-scope")
}

func TestEventHubBackendWorkspaceListUsesProjectionViewReader(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	reader := &projectionOnlyWorkspaceViewReader{
		workspaces: []appservice.Workspace{{
			ID:    "ws-projected-list",
			Title: "projected list",
			State: string(common.WorkspaceStateActive),
		}},
	}
	dispatcher := appservice.NewCLICommandDispatcher(&appservice.RuntimeSession{}, rawHub, reader)
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("run dispatcher: %v", err)
	}
	t.Cleanup(dispatcher.Close)
	be := NewEventHub(rawHub)

	items, err := be.Workspace().List(ctx)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if reader.listCalls != 1 {
		t.Fatalf("view reader list calls = %d, want 1", reader.listCalls)
	}
	if len(items) != 1 || items[0].ID != "ws-projected-list" {
		t.Fatalf("items = %+v, want projection workspace list", items)
	}
}

func TestEventHubBackendManagesWorkspaceMemoryThroughCLICommands(t *testing.T) {
	ctx := context.Background()
	be, commandHub := newEventHubBackendWithDispatcher(t, ctx)
	const observerID = "/memory/consumers/workorch/backend-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, commandHub)
	notifications := make(chan string, 3)
	memoryTopics := []string{memoryevents.TopicMemoryCreated, memoryevents.TopicMemoryUpdated, memoryevents.TopicMemoryDeleted}
	for _, topic := range memoryTopics {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			if _, ok := event.Data().(memoryevents.MemoryChangedData); ok {
				notifications <- topic
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	t.Cleanup(func() {
		for _, topic := range memoryTopics {
			observer.Unsubscribe(topic)
		}
	})

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

	var sawCreate, sawUpdate, sawDelete bool
	for range memoryTopics {
		var topic string
		select {
		case topic = <-notifications:
		case <-time.After(time.Second):
			t.Fatalf("memory notifications create=%v update=%v delete=%v", sawCreate, sawUpdate, sawDelete)
		}
		switch topic {
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

func newEventHubBackendWithDispatcher(t *testing.T, ctx context.Context) (Backend, magicevent.Hub) {
	t.Helper()
	runtime := testkit.NewRuntime("memory", "", nil)
	session := appservice.NewRuntimeSession(runtime)
	t.Cleanup(func() { _ = session.Close(ctx) })
	var err error
	exports, err := testkit.SessionExports(session)
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	dispatcher := appservice.NewCLICommandDispatcher(session, exports.Infrastructure.CommandHub, testWorkspaceViewReader{session: session})
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("run dispatcher: %v", err)
	}
	t.Cleanup(dispatcher.Close)
	commandHub := &recordingEventHub{Hub: exports.Infrastructure.CommandHub}
	return NewEventHub(commandHub), commandHub
}

func subscribeWorkspaceRunResult(t *testing.T, hub magicevent.Hub, response any) func() {
	t.Helper()
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, hub)
	observer.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(response, nil)
	})
	return func() { observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested) }
}

func subscribeRunReadCLIResponses(t *testing.T, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID) func() {
	t.Helper()
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, hub)
	observer.Subscribe(workorchdevents.TopicCLICommand, func(event magicevent.Event, response magicevent.Result) {
		cmd, ok := event.Data().(workorchdevents.CLICommand)
		if !ok {
			t.Fatalf("decode CLI command failed: %T", event.Data())
		}
		result := workorchdevents.CLIResult{
			CorrelationID: cmd.CorrelationID,
			CommandID:     cmd.CommandID,
			OK:            true,
			WorkspaceID:   cmd.WorkspaceID,
			RunID:         cmd.RunID,
			OccurredAt:    time.Now().UTC(),
		}
		run := RunInfo{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}
		switch cmd.Command {
		case workorchdevents.CommandRunGet:
			result.Run = &run
		case workorchdevents.CommandRunList:
			result.Runs = []RunInfo{run}
		case workorchdevents.CommandRunStatus:
			status := RunStatusInfo{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}
			result.RunStatus = &status
		default:
			return
		}
		response.Set(result, nil)
	})
	return func() { observer.Unsubscribe(workorchdevents.TopicCLICommand) }
}

func assertCLICommandWorkspaceScope(t *testing.T, hub magicevent.Hub, command string, workspaceID common.WorkspaceID, subjectID string) {
	t.Helper()
	recordedHub, ok := hub.(*recordingEventHub)
	if !ok {
		t.Fatal("command hub does not expose sent events")
	}
	for _, event := range recordedHub.Sent() {
		cmd, ok := event.Data().(workorchdevents.CLICommand)
		if !ok || cmd.Command != command || cmd.SubjectID != subjectID {
			continue
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("%s command workspace scope=%q, want %q", command, cmd.WorkspaceID, workspaceID)
		}
		return
	}
	t.Fatalf("published CLI command %s subject=%s not found", command, subjectID)
}

type recordingEventHub struct {
	magicevent.Hub
	mu   sync.Mutex
	sent []magicevent.Event
}

func (h *recordingEventHub) Send(event magicevent.Event) magicevent.Result {
	h.mu.Lock()
	h.sent = append(h.sent, event)
	h.mu.Unlock()
	return h.Hub.Send(event)
}

func (h *recordingEventHub) Sent() []magicevent.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]magicevent.Event(nil), h.sent...)
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

type projectionOnlyWorkspaceViewReader struct {
	workspaces []appservice.Workspace
	listCalls  int
}

func (r *projectionOnlyWorkspaceViewReader) ListWorkspaceView(context.Context) ([]appservice.Workspace, error) {
	r.listCalls++
	return r.workspaces, nil
}

func (r *projectionOnlyWorkspaceViewReader) ReadWorkspaceViewSnapshot(context.Context, string) (appservice.WorkspaceSnapshot, error) {
	return appservice.WorkspaceSnapshot{}, common.ErrInvalidArgument
}

func TestConvertBackendEventAddsBlockingSubjectIDs(t *testing.T) {
	created := convertBackendEvent(backendEventData{
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-created",
		Payload:     events.WorkspacePayload("created", "", "/tmp/created", "", "active", ""),
	})
	if created.Type != EventWorkspaceCreated || created.WorkspaceID != "ws-created" {
		t.Fatalf("created event = %+v, want workspace created", created)
	}

	deleted := convertBackendEvent(backendEventData{
		Topic:       workspaceevents.TopicWorkspaceDeleted,
		WorkspaceID: "ws-deleted",
		Payload:     events.WorkspacePayload("deleted", "", "/tmp/deleted", "", "active", ""),
	})
	if deleted.Type != EventWorkspaceDeleted || deleted.WorkspaceID != "ws-deleted" {
		t.Fatalf("deleted event = %+v, want workspace deleted", deleted)
	}

	decision := convertBackendEvent(backendEventData{
		Topic:       decisionevents.TopicTicketCreated,
		SubjectID:   "ticket-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     events.DecisionTicketPayload("missing operator input", "open"),
	})
	if decision.Type != EventDecisionCreated || decision.Payload["ticketId"] != "ticket-1" {
		t.Fatalf("decision event = %+v, want ticketId from subject", decision)
	}

	approval := convertBackendEvent(backendEventData{
		Topic:       approvalevents.TopicApprovalCreated,
		SubjectID:   "approval-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     events.ApprovalPayload("approve shell", "run command", map[string]any{"risk": "shell"}, "pending"),
	})
	if approval.Type != EventApprovalCreated || approval.Payload["approvalId"] != "approval-1" {
		t.Fatalf("approval event = %+v, want approvalId from subject", approval)
	}

	governance := convertBackendEvent(backendEventData{
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

	artifact := convertBackendEvent(backendEventData{
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
