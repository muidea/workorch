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
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"

	appservice "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestEventHubBackendRoutesWorkspaceCreateThroughCLICommand(t *testing.T) {
	ctx := context.Background()
	be, commandHub := newEventHubBackendWithDispatcher(t, ctx)

	ws, err := be.Workspace().Create(ctx, workorchmanagementcontract.WorkspaceCreateRequest{
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
		command, ok := event.Data().(workorchdevents.CLIWorkspaceCreateCommand)
		if ok && command.Request.Title == "eventhub workspace" {
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
		_, err := be.Workspace().Create(ctx, workorchmanagementcontract.WorkspaceCreateRequest{
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

func TestEventHubProjectionEventsBackendReceivesTypedProjection(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	defer rawHub.Terminate(ctx)

	be := NewEventHub(rawHub)
	received := make(chan projectioncontract.WorkspaceProjectionChanged, 1)
	statuses := make(chan port.SubscriptionStatus, 4)
	unsubscribe, err := be.ProjectionEvents().SubscribeWorkspaceProjection(
		ctx,
		func(_ context.Context, info projectioncontract.WorkspaceProjectionChanged) {
			received <- info
		},
		func(_ context.Context, status port.SubscriptionStatus) {
			statuses <- status
		},
	)
	if err != nil {
		t.Fatalf("subscribe projection events: %v", err)
	}
	defer unsubscribe()

	// Subscribe establishes live status before any projection data arrives.
	select {
	case status := <-statuses:
		if status.State != port.SubscriptionLive && status.State != port.SubscriptionConnecting {
			t.Fatalf("status = %+v, want connecting/live", status)
		}
	case <-time.After(200 * time.Millisecond):
		// status callback is best-effort for this test
	}

	projectionEvent := magicevent.NewEventWithContext(
		workorchdevents.TopicWorkspaceProjectionChanged,
		"workorchd/projection",
		workorchdevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		workorchdevents.WorkspaceProjectionChangedData{
			WorkspaceID:          "ws-projection",
			ChangeKind:           "detail",
			GlobalRevision:       7,
			WorkspaceRevision:    3,
			RuntimeStateRevision: 11,
			ActivityRevision:     5,
			ChangedSections: []projectioncontract.WorkspaceProjectionSection{
				projectioncontract.WorkspaceProjectionSectionRuntimeProgress,
				projectioncontract.WorkspaceProjectionSectionActivity,
			},
			ProjectionState: "ready",
			UpdatedAt:       "2026-07-15T12:00:00Z",
		},
	)
	rawHub.Post(projectionEvent)

	select {
	case info := <-received:
		if info.WorkspaceID != "ws-projection" {
			t.Fatalf("workspace = %q", info.WorkspaceID)
		}
		if info.Revision.WorkspaceRevision != 3 || info.Revision.RuntimeStateRevision != 11 || info.Revision.ActivityRevision != 5 {
			t.Fatalf("revision = %+v", info.Revision)
		}
		if len(info.ChangedSections) != 2 {
			t.Fatalf("changed sections = %+v", info.ChangedSections)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for typed projection event")
	}
}

func TestEventHubEventsBackendReceivesTypedEventStreamRecord(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	defer rawHub.Terminate(ctx)

	be := NewEventHub(rawHub)
	received := make(chan port.Event, 1)
	unsubscribe, err := be.Events().Subscribe(ctx, func(_ context.Context, evt port.Event) {
		if evt.Type == port.EventWorkspaceCreated {
			received <- evt
		}
	})
	if err != nil {
		t.Fatalf("subscribe events: %v", err)
	}
	defer unsubscribe()

	data, err := eventstreamevents.EventRecordDataFromModel(&eventstreammodel.EventRecord{
		ID:          "eventstream-workspace-created",
		Version:     recordpayload.WorkorchEventVersion,
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-eventstream",
		SubjectType: "workspace",
		SubjectID:   "ws-eventstream",
		Revision:    1,
		Payload:     workorcheventpayload.New(workorcheventpayload.StringField("title", "typed event stream")),
	})
	if err != nil {
		t.Fatalf("build event record data: %v", err)
	}
	event := magicevent.NewEventWithContext(
		eventstreamevents.TopicEventStreamRecordAppended,
		"blocks/eventstream",
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

func TestEventHubEventsBackendReceivesPostSealWorkspaceDeletedReceiptDirectly(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	defer rawHub.Terminate(ctx)

	be := NewEventHub(rawHub)
	received := make(chan port.Event, 1)
	unsubscribe, err := be.Events().Subscribe(ctx, func(_ context.Context, evt port.Event) {
		if evt.Type == port.EventWorkspaceDeleted {
			received <- evt
		}
	})
	if err != nil {
		t.Fatalf("subscribe events: %v", err)
	}
	defer unsubscribe()

	data := workspaceevents.WorkspaceDeletedData{
		WorkspaceID:       "ws-deleted-eventhub",
		RequestID:         "request-deleted-eventhub",
		Mode:              deletioncontract.ModeRetain,
		Disposition:       workspaceevents.DeleteWorkspaceCompleted,
		DeletionRevision:  7,
		WorkspaceRevision: 11,
		CompletedAt:       time.Date(2026, 8, 26, 3, 4, 5, 0, time.UTC),
	}
	data.DeletionID = workspaceevents.StableWorkspaceDeletionID(data.WorkspaceID, data.RequestID)
	rawHub.Post(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceDeleted,
		"blocks/workspace",
		workspaceevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		data,
	))

	select {
	case event := <-received:
		if event.ID != data.DeletionID || event.WorkspaceID != data.WorkspaceID || event.Revision != int64(data.WorkspaceRevision) || event.Payload["deletionId"] != data.DeletionID || event.Payload["disposition"] != string(workspaceevents.DeleteWorkspaceCompleted) {
			t.Fatalf("workspace deleted event=%+v", event)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for direct post-seal Workspace deleted receipt")
	}
}

func TestCLIWorkspaceSnapshotResultCarriesTypedSnapshot(t *testing.T) {
	want := workorchdevents.CLIWorkspaceSnapshotInfo{Workspace: workorchdevents.CLIWorkspaceInfo{ID: "ws-typed", State: common.WorkspaceStateActive}}
	result := workorchdevents.CLIWorkspaceSnapshotResult{Snapshot: want}
	got := result.Snapshot
	if got.Workspace.ID != "ws-typed" {
		t.Fatalf("typed resource = %+v, want workspace ws-typed", got)
	}
}

func TestEventHubRunRestartPublishesWorkspaceScopedFreshActivation(t *testing.T) {
	ctx := context.Background()
	be, commandHub := newEventHubBackendWithDispatcher(t, ctx)
	published := 0
	var captured workorchdevents.WorkspaceRunRequestedCommand
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, commandHub)
	observer.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(event magicevent.Event, result magicevent.Result) {
		published++
		captured, _ = event.Data().(workorchdevents.WorkspaceRunRequestedCommand)
		result.Set(workorchdevents.WorkspaceRunRequestedResult{WorkspaceID: "ws-restart-unified", RequestID: "request-restart-unified", Kind: "restart", Outcome: "reconciliation_requested", Reason: "current_run_continuation_accepted"}, nil)
	})
	defer observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested)

	if _, err := be.Run().Command(ctx, port.WorkspaceRunCommand{
		WorkspaceID: "ws-restart-unified",
		Intent:      port.RunCommandIntentRestart,
		RequestID:   "request-restart-unified",
		Reason:      "test_restart_unified",
		Source:      "test",
	}); err != nil {
		t.Fatalf("restart err=%v", err)
	}
	if published != 1 || captured.Intent != string(port.RunCommandIntentRestart) || captured.WorkspaceID != "ws-restart-unified" {
		t.Fatalf("published=%d command=%+v, want one workspace-scoped restart", published, captured)
	}
}

func TestEventHubRunResumeForExistingRunOmitsActivationOverrides(t *testing.T) {
	ctx := context.Background()
	be, commandHub := newEventHubBackendWithDispatcher(t, ctx)
	var captured workorchdevents.WorkspaceRunRequestedCommand
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, commandHub)
	observer.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(event magicevent.Event, result magicevent.Result) {
		captured, _ = event.Data().(workorchdevents.WorkspaceRunRequestedCommand)
		result.Set(workorchdevents.WorkspaceRunRequestedResult{WorkspaceID: "ws-resume-scoped", RequestID: "request-resume-scoped", Kind: "resume", Outcome: "reconciliation_requested", Reason: "current_run_continuation_accepted"}, nil)
	})
	defer observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested)

	if _, err := be.Run().Command(ctx, port.WorkspaceRunCommand{
		WorkspaceID: "ws-resume-scoped",
		Intent:      port.RunCommandIntentResume,
		RequestID:   "request-resume-scoped",
		Reason:      "user_requested",
		Source:      "test",
	}); err != nil {
		t.Fatalf("resume: %v", err)
	}

	if captured.WorkspaceID != "ws-resume-scoped" || captured.Intent != string(port.RunCommandIntentResume) || captured.RequestID != "request-resume-scoped" {
		t.Fatalf("resume command scope = %+v", captured)
	}
}

func TestEventHubRunResumeGeneratesRequestCorrelation(t *testing.T) {
	ctx := context.Background()
	be, commandHub := newEventHubBackendWithDispatcher(t, ctx)
	var captured workorchdevents.WorkspaceRunRequestedCommand
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, commandHub)
	observer.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(event magicevent.Event, result magicevent.Result) {
		captured, _ = event.Data().(workorchdevents.WorkspaceRunRequestedCommand)
		result.Set(workorchdevents.WorkspaceRunRequestedResult{WorkspaceID: "ws-resume-correlation", RequestID: captured.RequestID, Kind: "resume", Outcome: "reconciliation_requested", Reason: "current_run_continuation_accepted"}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested) })

	if _, err := be.Run().Command(ctx, port.WorkspaceRunCommand{
		WorkspaceID: "ws-resume-correlation",
		Intent:      port.RunCommandIntentResume,
		Source:      "tui",
	}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if captured.RequestID == "" || captured.Source != "tui" {
		t.Fatalf("resume correlation = %+v, want generated request id and tui source", captured)
	}
}

func TestEventHubRunCommandRequiresOwnerAdmissionResponse(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		subscribe func(magicevent.Hub) func()
		wantError string
	}{
		{
			name:      "missing owner admission response",
			wantError: "missing observer",
		},
		{
			name: "wrong response type",
			subscribe: func(hub magicevent.Hub) func() {
				return subscribeWorkspaceRunResult(t, hub, struct{ Accepted bool }{Accepted: true})
			},
			wantError: "invalid owner outcome",
		},
		{
			name: "owner outcome mismatch",
			subscribe: func(hub magicevent.Hub) func() {
				return subscribeWorkspaceRunResult(t, hub, workorchdevents.WorkspaceRunRequestedResult{})
			},
			wantError: "invalid owner outcome",
		},
		{
			name: "accepted",
			subscribe: func(hub magicevent.Hub) func() {
				observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, hub)
				observer.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(event magicevent.Event, result magicevent.Result) {
					command, _ := event.Data().(workorchdevents.WorkspaceRunRequestedCommand)
					result.Set(workorchdevents.WorkspaceRunRequestedResult{WorkspaceID: command.WorkspaceID, RequestID: command.RequestID, Kind: "resume", Outcome: "reconciliation_requested", Reason: "current_run_continuation_accepted"}, nil)
				})
				return func() { observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested) }
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
			_, err := be.Run().Command(ctx, port.WorkspaceRunCommand{
				WorkspaceID: "ws-run-command-ack",
				Intent:      port.RunCommandIntentResume,
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

	ws, err := be.Workspace().Create(ctx, workorchmanagementcontract.WorkspaceCreateRequest{Title: "snapshot workspace"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	snapshotBackend, ok := be.Workspace().(interface {
		Snapshot(context.Context, common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error)
	})
	if !ok {
		t.Fatal("workspace backend does not expose snapshot reader")
	}
	snapshot, err := snapshotBackend.Snapshot(ctx, common.WorkspaceID(ws.ID))
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
		if _, ok := event.Data().(workorchdevents.CLIWorkspaceSnapshotCommand); ok {
			return
		}
	}
	t.Fatal("workspace snapshot did not publish CLI command")
}

func TestEventHubRunCommandRejectsUnsupportedIntent(t *testing.T) {
	be := NewEventHub(nil)
	_, err := be.Run().Command(context.Background(), port.WorkspaceRunCommand{
		WorkspaceID: "ws-eventhub",
		Intent:      port.RunCommandIntent("unsupported"),
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported run command intent") {
		t.Fatalf("err = %v, want unsupported run command intent", err)
	}
}

func TestEventHubRunCommandRejectsInvalidIntentScopeBeforePublish(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  port.WorkspaceRunCommand
	}{
		{
			name: "restart with run scope",
			req: port.WorkspaceRunCommand{
				WorkspaceID: "ws-eventhub",
				Intent:      port.RunCommandIntentRestart,
			},
		},
		{
			name: "replan without run scope",
			req: port.WorkspaceRunCommand{
				WorkspaceID: "ws-eventhub",
				Intent:      port.RunCommandIntent("replan"),
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
		call func(port.Backend) error
	}{
		{
			name: "get decision",
			call: func(be port.Backend) error {
				_, err := be.Decision().Get(ctx, "", "ticket-eventhub")
				return err
			},
		},
		{
			name: "resolve decision",
			call: func(be port.Backend) error {
				_, err := be.Decision().Resolve(ctx, port.ResolveDecisionRequest{TicketID: "ticket-eventhub", ResolutionType: "accept"})
				return err
			},
		},
		{
			name: "batch resolve decisions",
			call: func(be port.Backend) error {
				_, err := be.Decision().BatchResolve(ctx, "", []common.TicketID{"ticket-eventhub"}, "accept", workorcheventpayload.Payload{})
				return err
			},
		},
		{
			name: "get approval",
			call: func(be port.Backend) error {
				_, err := be.Approval().Get(ctx, "", "approval-eventhub")
				return err
			},
		},
		{
			name: "approve approval",
			call: func(be port.Backend) error {
				_, err := be.Approval().Approve(ctx, "", "approval-eventhub")
				return err
			},
		},
		{
			name: "reject approval",
			call: func(be port.Backend) error {
				_, err := be.Approval().Reject(ctx, "", "approval-eventhub")
				return err
			},
		},
		{
			name: "batch resolve approvals",
			call: func(be port.Backend) error {
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

	ws, err := be.Workspace().Create(ctx, workorchmanagementcontract.WorkspaceCreateRequest{Title: "workspace object scope"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := be.Workspace().Get(ctx, common.WorkspaceID(ws.ID)); err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	snapshotBackend, ok := be.Workspace().(interface {
		Snapshot(context.Context, common.WorkspaceID) (*projectioncontract.WorkspaceSnapshot, error)
	})
	if !ok {
		t.Fatal("workspace backend does not expose snapshot reader")
	}
	if _, err := snapshotBackend.Snapshot(ctx, common.WorkspaceID(ws.ID)); err != nil {
		t.Fatalf("snapshot workspace: %v", err)
	}
	if err := be.Workspace().Delete(ctx, common.WorkspaceID(ws.ID), false); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}

	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.TopicCLIWorkspaceGet, common.WorkspaceID(ws.ID), string(ws.ID))
	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.TopicCLIWorkspaceSnapshot, common.WorkspaceID(ws.ID), string(ws.ID))
	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.TopicCLIWorkspaceDelete, common.WorkspaceID(ws.ID), string(ws.ID))
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
	if projectionEvent.Type != port.EventWorkspaceProjectionChanged || projectionEvent.WorkspaceID != "ws-1" || projectionEvent.Payload["changeKind"] != "detail" {
		t.Fatalf("projection event = %+v, want workspace projection changed", projectionEvent)
	}

	goalEvent := convertBackendEvent(backendEventData{
		Topic:       workspaceevents.TopicGoalDefined,
		SubjectID:   "goal-1",
		WorkspaceID: "ws-1",
		Payload:     recordpayload.GoalPayloadData{Text: "updated goal"}.EventPayload().Map(),
	})
	if goalEvent.Type != port.EventGoalDefined || goalEvent.WorkspaceID != "ws-1" || goalEvent.Payload["goalId"] != "goal-1" {
		t.Fatalf("goal event = %+v, want converted goal definition event", goalEvent)
	}
	dodEvent := convertBackendEvent(backendEventData{
		Topic:       workspaceevents.TopicDoDDefined,
		SubjectID:   "dod-1",
		WorkspaceID: "ws-1",
		Payload:     recordpayload.DoDPayloadData{Items: []string{"tests pass"}, State: common.DoDStateReady}.EventPayload().Map(),
	})
	if dodEvent.Type != port.EventDoDDefined || dodEvent.WorkspaceID != "ws-1" || dodEvent.Payload["doDId"] != "dod-1" {
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

	ws, err := be.Workspace().Create(ctx, workorchmanagementcontract.WorkspaceCreateRequest{Title: "definition refresh workspace"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	goal, err := be.Goal().Define(ctx, common.WorkspaceID(ws.ID), "initial goal")
	if err != nil {
		t.Fatalf("define goal: %v", err)
	}
	if _, err := be.Goal().Update(ctx, common.WorkspaceID(ws.ID), common.GoalID(goal.ID), "updated goal"); err != nil {
		t.Fatalf("update goal: %v", err)
	}
	dod, err := be.DoD().Define(ctx, common.WorkspaceID(ws.ID), []string{"initial dod"})
	if err != nil {
		t.Fatalf("define dod: %v", err)
	}
	if _, err := be.DoD().Update(ctx, common.WorkspaceID(ws.ID), common.DoDID(dod.ID), []string{"updated dod"}); err != nil {
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

	ws, err := be.Workspace().Create(ctx, workorchmanagementcontract.WorkspaceCreateRequest{Title: "definition scope workspace"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	goal, err := be.Goal().Define(ctx, common.WorkspaceID(ws.ID), "scoped goal")
	if err != nil {
		t.Fatalf("define goal: %v", err)
	}
	if _, err := be.Goal().Get(ctx, common.WorkspaceID(ws.ID), common.GoalID(goal.ID)); err != nil {
		t.Fatalf("get goal: %v", err)
	}
	if _, err := be.Goal().Update(ctx, common.WorkspaceID(ws.ID), common.GoalID(goal.ID), "updated scoped goal"); err != nil {
		t.Fatalf("update goal: %v", err)
	}
	dod, err := be.DoD().Define(ctx, common.WorkspaceID(ws.ID), []string{"scoped dod"})
	if err != nil {
		t.Fatalf("define dod: %v", err)
	}
	if _, err := be.DoD().Get(ctx, common.WorkspaceID(ws.ID), common.DoDID(dod.ID)); err != nil {
		t.Fatalf("get dod: %v", err)
	}
	if _, err := be.DoD().Update(ctx, common.WorkspaceID(ws.ID), common.DoDID(dod.ID), []string{"updated scoped dod"}); err != nil {
		t.Fatalf("update dod: %v", err)
	}

	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.TopicCLIGoalGet, common.WorkspaceID(ws.ID), string(goal.ID))
	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.TopicCLIGoalUpdate, common.WorkspaceID(ws.ID), string(goal.ID))
	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.TopicCLIDoDGet, common.WorkspaceID(ws.ID), string(dod.ID))
	assertCLICommandWorkspaceScope(t, commandHub, workorchdevents.TopicCLIDoDUpdate, common.WorkspaceID(ws.ID), string(dod.ID))
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

	assertCLICommandWorkspaceScope(t, recordedHub, workorchdevents.TopicCLIRunGet, "ws-run-read-scope", "run-read-scope")
	assertCLICommandWorkspaceScope(t, recordedHub, workorchdevents.TopicCLIRunList, "ws-run-read-scope", "")
	assertCLICommandWorkspaceScope(t, recordedHub, workorchdevents.TopicCLIRunStatus, "ws-run-read-scope", "run-read-scope")
}

func TestEventHubBackendWorkspaceListUsesProjectionViewReader(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	reader := &projectionOnlyWorkspaceViewReader{
		workspaces: []appmodelcore.Workspace{{
			ID:    "ws-projected-list",
			Title: "projected list",
			State: string(common.WorkspaceStateActive),
		}},
	}
	dispatcher := appservice.NewCLICommandDispatcher(&runtimecore.Session{}, rawHub, reader)
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

	ws, err := be.Workspace().Create(ctx, workorchmanagementcontract.WorkspaceCreateRequest{
		Title:                "memory workspace",
		DeliverableDirectory: "/tmp/memory-workspace",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	created, err := be.Memory().Create(ctx, common.WorkspaceID(ws.ID), &workorchmanagementcontract.MemoryMutation{
		MemoryID:   "memory-cli-1",
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
	if created == nil || created.ID != "memory-cli-1" || created.WorkspaceID != string(ws.ID) {
		t.Fatalf("created memory = %+v, want persisted item in workspace %s", created, ws.ID)
	}
	list, err := be.Memory().List(ctx, common.WorkspaceID(ws.ID))
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, want created memory", list)
	}
	search, err := be.Memory().Search(ctx, workorchquerycontract.MemorySearchQuery{WorkspaceID: common.WorkspaceID(ws.ID), Query: "eventhub"})
	if err != nil {
		t.Fatalf("search memory: %v", err)
	}
	if len(search) != 1 || search[0].ID != created.ID {
		t.Fatalf("search = %+v, want created memory", search)
	}
	got, err := be.Memory().Get(ctx, common.WorkspaceID(ws.ID), common.MemoryID(created.ID))
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if got == nil || got.Summary != "eventhub memory" {
		t.Fatalf("got = %+v, want created memory", got)
	}
	updated, err := be.Memory().Update(ctx, common.WorkspaceID(ws.ID), common.MemoryID(created.ID), &workorchmanagementcontract.MemoryMutation{Summary: "updated eventhub memory"})
	if err != nil {
		t.Fatalf("update memory: %v", err)
	}
	if updated == nil || updated.Summary != "updated eventhub memory" {
		t.Fatalf("updated = %+v, want updated summary", updated)
	}
	if err := be.Memory().Delete(ctx, common.WorkspaceID(ws.ID), common.MemoryID(created.ID)); err != nil {
		t.Fatalf("delete memory: %v", err)
	}
	list, err = be.Memory().List(ctx, common.WorkspaceID(ws.ID))
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

func newEventHubBackendWithDispatcher(t *testing.T, ctx context.Context) (port.Backend, magicevent.Hub) {
	t.Helper()
	runtime := testkit.NewRuntime("memory", "", nil)
	session := appservice.NewRuntimeSession(runtime)
	// The memory owner fixture still creates workspace-managed directories.
	// Keep those test artifacts under t.TempDir instead of the package cwd.
	session.SetRepositoryConfig("file", t.TempDir())
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
	observer.Subscribe(workorchdevents.TopicCLIRunGet, func(event magicevent.Event, response magicevent.Result) {
		if _, ok := event.Data().(workorchdevents.CLIRunGetCommand); !ok {
			t.Fatalf("run get command = %T", event.Data())
		}
		response.Set(workorchdevents.CLIRunGetResult{Run: projectioncontract.Run{ID: string(runID), WorkspaceID: string(workspaceID), State: string(common.RunStateActive)}}, nil)
	})
	observer.Subscribe(workorchdevents.TopicCLIRunList, func(event magicevent.Event, response magicevent.Result) {
		if _, ok := event.Data().(workorchdevents.CLIRunListCommand); !ok {
			t.Fatalf("run list command = %T", event.Data())
		}
		response.Set(workorchdevents.CLIRunListResult{Runs: []projectioncontract.Run{{ID: string(runID), WorkspaceID: string(workspaceID), State: string(common.RunStateActive)}}}, nil)
	})
	observer.Subscribe(workorchdevents.TopicCLIRunStatus, func(event magicevent.Event, response magicevent.Result) {
		if _, ok := event.Data().(workorchdevents.CLIRunStatusCommand); !ok {
			t.Fatalf("run status command = %T", event.Data())
		}
		response.Set(workorchdevents.CLIRunStatusResult{Status: projectioncontract.RunStatus{ID: string(runID), WorkspaceID: string(workspaceID), State: string(common.RunStateActive)}}, nil)
	})
	return func() {
		observer.Unsubscribe(workorchdevents.TopicCLIRunGet)
		observer.Unsubscribe(workorchdevents.TopicCLIRunList)
		observer.Unsubscribe(workorchdevents.TopicCLIRunStatus)
	}
}

func assertCLICommandWorkspaceScope(t *testing.T, hub magicevent.Hub, topic string, workspaceID common.WorkspaceID, subjectID string) {
	t.Helper()
	recordedHub, ok := hub.(*recordingEventHub)
	if !ok {
		t.Fatal("command hub does not expose sent events")
	}
	for _, event := range recordedHub.Sent() {
		if event.ID() != topic {
			continue
		}
		var actualWorkspace common.WorkspaceID
		var actualSubject string
		switch command := event.Data().(type) {
		case workorchdevents.CLIWorkspaceGetCommand:
			actualWorkspace, actualSubject = command.WorkspaceID, string(command.WorkspaceID)
		case workorchdevents.CLIWorkspaceSnapshotCommand:
			actualWorkspace, actualSubject = command.WorkspaceID, string(command.WorkspaceID)
		case workorchdevents.CLIWorkspaceDeleteCommand:
			actualWorkspace, actualSubject = command.WorkspaceID, string(command.WorkspaceID)
		case workorchdevents.CLIGoalGetCommand:
			actualWorkspace, actualSubject = command.WorkspaceID, string(command.GoalID)
		case workorchdevents.CLIGoalUpdateCommand:
			actualWorkspace, actualSubject = command.WorkspaceID, string(command.GoalID)
		case workorchdevents.CLIDoDGetCommand:
			actualWorkspace, actualSubject = command.WorkspaceID, string(command.DoDID)
		case workorchdevents.CLIDoDUpdateCommand:
			actualWorkspace, actualSubject = command.WorkspaceID, string(command.DoDID)
		case workorchdevents.CLIRunGetCommand:
			actualWorkspace, actualSubject = command.WorkspaceID, string(command.RunID)
		case workorchdevents.CLIRunListCommand:
			actualWorkspace = command.WorkspaceID
		case workorchdevents.CLIRunStatusCommand:
			actualWorkspace, actualSubject = command.WorkspaceID, string(command.RunID)
		default:
			t.Fatalf("topic %s command = %T", topic, event.Data())
		}
		if actualWorkspace != workspaceID || actualSubject != subjectID {
			t.Fatalf("%s command scope=%q subject=%q, want %q/%q", topic, actualWorkspace, actualSubject, workspaceID, subjectID)
		}
		return
	}
	t.Fatalf("published CLI command %s subject=%s not found", topic, subjectID)
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
	session *runtimecore.Session
}

func (r testWorkspaceViewReader) ListWorkspaceView(ctx context.Context) ([]appmodelcore.Workspace, error) {
	return appservice.ListWorkspacesWithSession(ctx, r.session.Borrow())
}

func (r testWorkspaceViewReader) ReadWorkspaceViewSnapshot(ctx context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error) {
	return appservice.ReadWorkspaceSnapshotWithSession(ctx, r.session.Borrow(), workspaceID)
}

func (r testWorkspaceViewReader) ReadWorkspaceActivity(context.Context, string) (projectioncontract.WorkspaceActivityFeed, bool, error) {
	return projectioncontract.WorkspaceActivityFeed{}, false, nil
}

type projectionOnlyWorkspaceViewReader struct {
	workspaces []appmodelcore.Workspace
	listCalls  int
}

func (r *projectionOnlyWorkspaceViewReader) ListWorkspaceView(context.Context) ([]appmodelcore.Workspace, error) {
	r.listCalls++
	return r.workspaces, nil
}

func (r *projectionOnlyWorkspaceViewReader) ReadWorkspaceViewSnapshot(context.Context, string) (readmodelcore.WorkspaceSnapshotState, error) {
	return readmodelcore.WorkspaceSnapshotState{}, common.ErrInvalidArgument
}

func (r *projectionOnlyWorkspaceViewReader) ReadWorkspaceActivity(context.Context, string) (projectioncontract.WorkspaceActivityFeed, bool, error) {
	return projectioncontract.WorkspaceActivityFeed{}, false, nil
}

func TestConvertBackendEventAddsBlockingSubjectIDs(t *testing.T) {
	created := convertBackendEvent(backendEventData{
		Topic:       workspaceevents.TopicWorkspaceCreated,
		WorkspaceID: "ws-created",
		Payload:     recordpayload.WorkspacePayloadData{Title: "created", DeliverableDirectory: "/tmp/created", State: "active"}.EventPayload().Map(),
	})
	if created.Type != port.EventWorkspaceCreated || created.WorkspaceID != "ws-created" {
		t.Fatalf("created event = %+v, want workspace created", created)
	}

	deleted := convertBackendEvent(backendEventData{
		Topic:       workspaceevents.TopicWorkspaceDeleted,
		WorkspaceID: "ws-deleted",
		Payload:     recordpayload.WorkspacePayloadData{Title: "deleted", DeliverableDirectory: "/tmp/deleted", State: "active"}.EventPayload().Map(),
	})
	if deleted.Type != port.EventWorkspaceDeleted || deleted.WorkspaceID != "ws-deleted" {
		t.Fatalf("deleted event = %+v, want workspace deleted", deleted)
	}

	decision := convertBackendEvent(backendEventData{
		Topic:       decisionevents.TopicTicketCreated,
		SubjectID:   "ticket-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload:     recordpayload.DecisionTicketPayloadData{Reason: "missing operator input", State: "open"}.EventPayload().Map(),
	})
	if decision.Type != port.EventDecisionCreated || decision.Payload["ticketId"] != "ticket-1" {
		t.Fatalf("decision event = %+v, want ticketId from subject", decision)
	}

	approval := convertBackendEvent(backendEventData{
		Topic:       approvalevents.TopicApprovalResolved,
		SubjectID:   "approval-1",
		WorkspaceID: "ws-1",
		RunID:       "run-1",
		Payload: recordpayload.ApprovalPayloadData{
			Reason: "approve shell", Action: "run command", State: "pending",
			Context: workorcheventpayload.New(workorcheventpayload.StringField("risk", "shell")),
		}.EventPayload().Map(),
	})
	if approval.Type != port.EventApprovalResolved || approval.Payload["approvalId"] != "approval-1" {
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
	if governance.Type != port.EventGovernanceResolved || governance.Payload["traceId"] != "trace-1" {
		t.Fatalf("governance event = %+v, want governance resolved with trace id", governance)
	}

}
