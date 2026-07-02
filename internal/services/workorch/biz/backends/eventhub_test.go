package backend

import (
	"context"
	"errors"
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
		ID:                 "run-1",
		WorkspaceID:        "ws-1",
		GoalID:             "goal-1",
		DoDID:              "dod-1",
		DefinitionRevision: "goal:goal-1|dod:dod-1",
		State:              common.RunStateRunning,
		ExecutionContext: map[string]any{
			"goal":               map[string]any{"text": "goal text"},
			"dod":                map[string]any{"items": []any{"dod item"}},
			"operatorInputQueue": []map[string]any{{"content": "resume"}},
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
	if resource["DefinitionRevision"] != "goal:goal-1|dod:dod-1" {
		t.Fatalf("DefinitionRevision = %q, want preserved run definition revision", resource["DefinitionRevision"])
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

func TestDecodeResultResourceUsesTypedPayloadBeforeJSONFallback(t *testing.T) {
	want := WorkspaceSnapshotInfo{
		Workspace: WorkspaceInfo{ID: "ws-typed", State: common.WorkspaceStateRunning},
	}
	got, err := decodeResultResource[WorkspaceSnapshotInfo](events.CLIResultPayload{
		TypedResource: &want,
		Resource: map[string]any{
			"cannotJSONMarshal": make(chan struct{}),
		},
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
	be, eventHub := newEventHubBackendWithDispatcher(t, ctx)
	unsubscribe := subscribeWorkspaceRunAccepted(t, eventHub)
	defer unsubscribe()

	if _, err := be.Run().Command(ctx, WorkspaceRunCommand{
		WorkspaceID:        "ws-restart-unified",
		Intent:             RunCommandIntentRestart,
		RequestID:          "request-restart-unified",
		Reason:             "test_restart_unified",
		Source:             "test",
		Priority:           7,
		DefinitionRevision: "rev-restart-unified",
		DecisionID:         "ticket-restart-unified",
		ExecutionContextPatch: map[string]any{
			"latestOperatorInput": map[string]any{"source": "test", "content": "restart"},
		},
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

	publishedHub, ok := eventHub.(interface{ Published() []events.Envelope })
	if !ok {
		t.Fatal("event hub does not expose published envelopes")
	}
	var sawUnified bool
	const legacyRestartTopic = "/workorchd/command/workspace-restart/requested"
	for _, envelope := range publishedHub.Published() {
		if envelope.Topic == legacyRestartTopic {
			t.Fatalf("published legacy restart topic: %+v", envelope)
		}
		if envelope.Topic != workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested {
			continue
		}
		if envelope.WorkspaceID != "ws-restart-unified" ||
			envelope.SubjectType != "workspace-run" ||
			envelope.Payload[events.PayloadKeyIntent] != string(RunCommandIntentRestart) ||
			envelope.Payload[events.PayloadKeyRequestID] != "request-restart-unified" ||
			envelope.Payload[events.PayloadKeyReason] != "test_restart_unified" ||
			envelope.Payload[events.PayloadKeySource] != "test" ||
			envelope.Payload[events.PayloadKeyDefinitionRevision] != "rev-restart-unified" ||
			envelope.Payload[events.PayloadKeyDecisionID] != "ticket-restart-unified" ||
			envelope.Payload[events.PayloadKeyPriority] != 7 ||
			envelope.Payload[events.PayloadKeyGoalID] != "goal-restart-unified" ||
			envelope.Payload[events.PayloadKeyDoDID] != "dod-restart-unified" ||
			envelope.Payload[events.PayloadKeyLLMProviderID] != "provider-restart-unified" ||
			envelope.Payload[events.PayloadKeySoulID] != "soul-restart-unified" {
			t.Fatalf("workspace run restart envelope = %+v", envelope)
		}
		if bindings, _ := envelope.Payload["soulBindings"].(map[string]string); bindings["validation"] != "soul-validation-restart-unified" {
			t.Fatalf("restart envelope soulBindings = %+v, want validation binding", envelope.Payload["soulBindings"])
		}
		if got := stringSliceValue(envelope.Payload, "skillIds"); len(got) != 1 || got[0] != "skill-restart-unified" {
			t.Fatalf("restart envelope skillIds = %+v", envelope.Payload["skillIds"])
		}
		if got := stringSliceValue(envelope.Payload, "mcpServerIds"); len(got) != 1 || got[0] != "mcp-restart-unified" {
			t.Fatalf("restart envelope mcpServerIds = %+v", envelope.Payload["mcpServerIds"])
		}
		if got := stringSliceValue(envelope.Payload, "capabilityIds"); len(got) != 1 || got[0] != "mcp-restart-unified:tool:validate" {
			t.Fatalf("restart envelope capabilityIds = %+v", envelope.Payload["capabilityIds"])
		}
		if got := stringSliceValue(envelope.Payload, "memoryIds"); len(got) != 1 || got[0] != "memory-restart-unified" {
			t.Fatalf("restart envelope memoryIds = %+v", envelope.Payload["memoryIds"])
		}
		if patch, _ := envelope.Payload[events.PayloadKeyExecutionContextPatch].(map[string]any); patch == nil {
			t.Fatalf("restart envelope missing execution context patch: %+v", envelope.Payload)
		}
		sawUnified = true
	}
	if !sawUnified {
		t.Fatal("restart did not publish unified workspace-run command")
	}
}

func TestEventHubRunCommandRequiresSchedulerAcceptedResponse(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		subscribe func(events.Hub) func()
		wantError string
	}{
		{
			name:      "missing scheduler response",
			wantError: "returned no accepted response",
		},
		{
			name: "wrong response topic",
			subscribe: func(hub events.Hub) func() {
				return hub.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(_ context.Context, envelope events.Envelope) error {
					return events.Response(events.Envelope{
						Topic:        envelope.Topic + "/done",
						SourceModule: "workorchd/scheduler",
						Payload:      map[string]any{"accepted": true},
					})
				})
			},
			wantError: "unexpected response topic",
		},
		{
			name: "not accepted",
			subscribe: func(hub events.Hub) func() {
				return hub.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(_ context.Context, envelope events.Envelope) error {
					return events.Response(events.Envelope{
						Topic:        envelope.Topic + "/accepted",
						SourceModule: "workorchd/scheduler",
						Payload:      map[string]any{"accepted": false},
					})
				})
			},
			wantError: "was not accepted",
		},
		{
			name: "wrong response owner",
			subscribe: func(hub events.Hub) func() {
				return hub.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(_ context.Context, envelope events.Envelope) error {
					return events.Response(events.Envelope{
						Topic:        envelope.Topic + "/accepted",
						SourceModule: "other-owner",
						Payload:      map[string]any{"accepted": true},
					})
				})
			},
			wantError: "returned response from other-owner",
		},
		{
			name: "accepted",
			subscribe: func(hub events.Hub) func() {
				return subscribeWorkspaceRunAccepted(t, hub)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hub := events.NewMemoryHub()
			if tc.subscribe != nil {
				unsubscribe := tc.subscribe(hub)
				defer unsubscribe()
			}
			be := NewEventHub(hub)
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

func TestEventHubRunCommandRejectsUnsupportedIntent(t *testing.T) {
	hub := events.NewMemoryHub()
	be := NewEventHub(hub)
	_, err := be.Run().Command(context.Background(), WorkspaceRunCommand{
		WorkspaceID:   "ws-eventhub",
		ExpectedRunID: "run-001",
		Intent:        RunCommandIntent("unsupported"),
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported run command intent") {
		t.Fatalf("err = %v, want unsupported run command intent", err)
	}
	if len(hub.Published()) != 0 {
		t.Fatalf("published = %+v, want no scheduler command for unsupported intent", hub.Published())
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
			hub := events.NewMemoryHub()
			be := NewEventHub(hub)
			_, err := be.Run().Command(context.Background(), tc.req)
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
			if len(hub.Published()) != 0 {
				t.Fatalf("published = %+v, want no scheduler command for invalid intent scope", hub.Published())
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
			hub := events.NewMemoryHub()
			be := NewEventHub(hub)
			err := tc.call(be)
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
			if len(hub.Published()) != 0 {
				t.Fatalf("published = %+v, want no CLI command for invalid governance scope", hub.Published())
			}
		})
	}
}

func TestEventHubWorkspaceObjectCommandsCarryWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	be, eventHub := newEventHubBackendWithDispatcher(t, ctx)

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

	assertCLICommandWorkspaceScope(t, eventHub, events.CommandWorkspaceGet, ws.ID, string(ws.ID))
	assertCLICommandWorkspaceScope(t, eventHub, events.CommandWorkspaceSnapshot, ws.ID, string(ws.ID))
	assertCLICommandWorkspaceScope(t, eventHub, events.CommandWorkspaceDelete, ws.ID, string(ws.ID))
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

func TestEventHubGoalAndDoDGetUpdateCommandsCarryWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	be, eventHub := newEventHubBackendWithDispatcher(t, ctx)

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

	assertCLICommandWorkspaceScope(t, eventHub, events.CommandGoalGet, ws.ID, string(goal.ID))
	assertCLICommandWorkspaceScope(t, eventHub, events.CommandGoalUpdate, ws.ID, string(goal.ID))
	assertCLICommandWorkspaceScope(t, eventHub, events.CommandDoDGet, ws.ID, string(dod.ID))
	assertCLICommandWorkspaceScope(t, eventHub, events.CommandDoDUpdate, ws.ID, string(dod.ID))
}

func TestEventHubRunReadCommandsCarryWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	eventHub := events.NewMemoryHub()
	unsubscribe := subscribeRunReadCLIResponses(t, eventHub, "ws-run-read-scope", "run-read-scope")
	defer unsubscribe()
	be := NewEventHub(eventHub)

	if _, err := be.Run().Get(ctx, "ws-run-read-scope", "run-read-scope"); err != nil {
		t.Fatalf("get run: %v", err)
	}
	if _, err := be.Run().List(ctx, "ws-run-read-scope"); err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if _, err := be.Run().Status(ctx, "ws-run-read-scope", "run-read-scope"); err != nil {
		t.Fatalf("run status: %v", err)
	}

	assertCLICommandWorkspaceScope(t, eventHub, events.CommandRunGet, "ws-run-read-scope", "run-read-scope")
	assertCLICommandWorkspaceScope(t, eventHub, events.CommandRunList, "ws-run-read-scope", "")
	assertCLICommandWorkspaceScope(t, eventHub, events.CommandRunStatus, "ws-run-read-scope", "run-read-scope")
}

func TestEventHubBackendWorkspaceListUsesProjectionViewReader(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(32)
	eventHub := events.NewMagicHubWithHubOwner(rawHub, "workorchd/test")
	reader := &projectionOnlyWorkspaceViewReader{
		workspaces: []appservice.Workspace{{
			ID:    "ws-projected-list",
			Title: "projected list",
			State: string(common.WorkspaceStateActive),
		}},
	}
	dispatcher := appservice.NewCLICommandDispatcher(&appservice.RuntimeSession{}, eventHub, reader)
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("run dispatcher: %v", err)
	}
	t.Cleanup(dispatcher.Close)
	be := NewEventHub(eventHub)

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

func subscribeWorkspaceRunAccepted(t *testing.T, hub events.Hub) func() {
	t.Helper()
	return hub.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{
			Topic:        envelope.Topic + "/accepted",
			SourceModule: "workorchd/scheduler",
			SubjectType:  envelope.SubjectType,
			SubjectID:    envelope.SubjectID,
			WorkspaceID:  envelope.WorkspaceID,
			RunID:        envelope.RunID,
			Payload:      map[string]any{"accepted": true},
		})
	})
}

func subscribeRunReadCLIResponses(t *testing.T, hub events.Hub, workspaceID common.WorkspaceID, runID common.RunID) func() {
	t.Helper()
	return hub.Subscribe(workorchdevents.TopicCLICommand, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := events.DecodeCLICommandPayload(envelope.Payload)
		if !ok {
			t.Fatalf("decode CLI command payload failed: %+v", envelope.Payload)
		}
		payload := events.CLIResultPayload{
			CorrelationID: cmd.CorrelationID,
			CommandID:     cmd.CommandID,
			OK:            true,
			WorkspaceID:   cmd.WorkspaceID,
			RunID:         cmd.RunID,
			OccurredAt:    time.Now().UTC(),
		}
		run := RunInfo{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
		switch cmd.Command {
		case events.CommandRunGet:
			payload.TypedResource = run
			payload.Resource = resourceMap(run)
		case events.CommandRunList:
			payload.TypedResources = []any{run}
			payload.Resources = []map[string]any{resourceMap(run)}
		case events.CommandRunStatus:
			status := RunStatusInfo{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
			payload.TypedResource = status
			payload.Resource = resourceMap(status)
		default:
			return nil
		}
		return events.Response(events.Envelope{
			Topic:        workorchdevents.TopicCLIResult,
			SourceModule: "workorchd/cli",
			SubjectType:  envelope.SubjectType,
			SubjectID:    envelope.SubjectID,
			WorkspaceID:  envelope.WorkspaceID,
			RunID:        envelope.RunID,
			Payload:      events.CLIResultPayloadMap(payload),
		})
	})
}

func assertCLICommandWorkspaceScope(t *testing.T, hub events.Hub, command string, workspaceID common.WorkspaceID, subjectID string) {
	t.Helper()
	publishedHub, ok := hub.(interface{ Published() []events.Envelope })
	if !ok {
		t.Fatal("event hub does not expose published envelopes")
	}
	for _, envelope := range publishedHub.Published() {
		if envelope.Topic != workorchdevents.TopicCLICommand {
			continue
		}
		cmd, ok := events.DecodeCLICommandPayload(envelope.Payload)
		if !ok || cmd.Command != command || cmd.SubjectID != subjectID {
			continue
		}
		if envelope.WorkspaceID != string(workspaceID) || cmd.WorkspaceID != workspaceID {
			t.Fatalf("%s command scope envelope=%q payload=%q, want %q", command, envelope.WorkspaceID, cmd.WorkspaceID, workspaceID)
		}
		return
	}
	t.Fatalf("published CLI command %s subject=%s not found", command, subjectID)
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
		Payload:     events.DecisionTicketPayload("missing operator input", "open"),
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

func TestConvertEnvelopeTreatsContractViolationAsProjectionChange(t *testing.T) {
	for _, topic := range []string{
		orchestratorevents.TopicOrchestratorContractViolationRecorded,
		orchestratorevents.TopicOrchestratorContractViolationRetryRequested,
		orchestratorevents.TopicOrchestratorContractViolationEscalated,
	} {
		t.Run(topic, func(t *testing.T) {
			evt := convertEnvelope(events.Envelope{
				Topic:       topic,
				WorkspaceID: "ws-violation",
				RunID:       "run-violation",
				StepID:      "run-violation/step-1",
				Payload: map[string]any{
					"violatedRule":   "state_changing_required_missing",
					"recoverability": "retry_same_contract",
				},
			})
			if evt.Type != EventWorkspaceProjectionChanged ||
				evt.WorkspaceID != "ws-violation" ||
				evt.RunID != "run-violation" {
				t.Fatalf("event = %+v, want workspace projection changed for contract violation", evt)
			}
			if evt.Payload["violatedRule"] != "state_changing_required_missing" {
				t.Fatalf("payload = %+v, want violation summary preserved", evt.Payload)
			}
		})
	}
}
