package appservice

import (
	"context"
	"errors"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
)

func TestCLICommandDispatcherRunIsIdempotent(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	reader := &staticCLIWorkspaceViewReader{workspaces: []Workspace{{ID: "ws-cli-idempotent"}}}
	dispatcher := NewCLICommandDispatcher(&RuntimeSession{}, nil, hub, reader)
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if result := sendCLIListCommand(t, hub, "cmd-idempotent-1"); !result.OK || reader.listCalls != 1 {
		t.Fatalf("first result = %+v list calls = %d, want one handler", result, reader.listCalls)
	}
	dispatcher.Close()
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("Run after Close: %v", err)
	}
	if result := sendCLIListCommand(t, hub, "cmd-idempotent-2"); !result.OK || reader.listCalls != 2 {
		t.Fatalf("second result = %+v list calls = %d, want one handler per command", result, reader.listCalls)
	}
}

func sendCLIListCommand(t *testing.T, hub magicevent.Hub, commandID string) workorchdevents.CLIResult {
	t.Helper()
	command := workorchdevents.CLICommand{
		CommandID:     commandID,
		CorrelationID: commandID + "/correlation",
		Command:       workorchdevents.CommandWorkspaceList,
		SubjectType:   "workspace",
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLICommand, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), command)
	response := hub.Send(event)
	if response == nil || response.Error() != nil {
		t.Fatalf("send CLI list command response = %+v", response)
	}
	data, err := response.Get()
	if err != nil {
		t.Fatalf("get CLI list result: %v", err)
	}
	result, ok := data.(workorchdevents.CLIResult)
	if !ok {
		t.Fatalf("CLI list result = %T, want CLIResult", data)
	}
	return result
}

func TestCLIWorkspaceSnapshotCommandProjectsDetailView(t *testing.T) {
	dispatcher := &CLICommandDispatcher{
		viewReader: &staticCLIWorkspaceViewReader{
			snapshot: WorkspaceSnapshot{
				Workspace: Workspace{
					ID:               "ws-cli-detail",
					ActiveRunID:      "run-cli-active",
					ContextHints:     map[string]any{"large": "profile"},
					EvidencePolicies: []map[string]any{{"large": "policy"}},
				},
				Runs: []Run{{
					ID:               "run-cli-active",
					WorkspaceID:      "ws-cli-detail",
					State:            "running",
					ExecutionContext: map[string]any{"coverageWorkItems": []any{map[string]any{"large": strings.Repeat("x", 2048)}}},
				}},
				LLMUsageTraces: []LLMUsageTrace{{
					ID:    "usage-cli",
					RunID: "run-cli-active",
					ContextBudget: map[string]any{
						"failureSignature": strings.Repeat("x", 2048),
					},
				}},
			},
		},
	}

	resource, _, err := dispatcher.handleWorkspaceCommand(context.Background(), clicore.Command{
		Command:     clicore.CommandWorkspaceSnapshot,
		WorkspaceID: "ws-cli-detail",
		SubjectID:   "ws-cli-detail",
	})
	if err != nil {
		t.Fatalf("handle workspace snapshot: %v", err)
	}
	workspace := resource["workspace"].(Workspace)
	if workspace.ContextHints != nil || workspace.EvidencePolicies != nil {
		t.Fatalf("workspace = %+v, want detail projection without heavy template fields", workspace)
	}
	runs := resource["runs"].([]map[string]any)
	if len(runs) != 1 || runs[0]["executionContext"] != nil {
		t.Fatalf("runs = %+v, want compact detail run projection", runs)
	}
	traces := resource["llmUsageTraces"].([]LLMUsageTrace)
	signature := traces[0].ContextBudget["failureSignature"].(string)
	if len(signature) > workspaceSnapshotTraceProjectionStringLimitForTest() {
		t.Fatalf("failureSignature length = %d, want detail projection bounded", len(signature))
	}
}

func TestCLIDecisionResolvePublishesSchedulerContinuation(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	hub, err := session.RequiredEventHub()
	if err != nil {
		t.Fatalf("session event hub: %v", err)
	}
	const workspaceID = common.WorkspaceID("ws-cli-governance-scheduler")
	const runID = common.RunID("run-cli-governance-scheduler")
	const ticketID = common.TicketID("ticket-cli-governance-scheduler")
	if err := tickets.Create(context.Background(), &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		State:       common.DecisionTicketStateOpen,
		Reason:      "need replan",
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	commandHub, err := session.RequiredCommandHub()
	if err != nil {
		t.Fatalf("session command hub: %v", err)
	}
	var schedulerCommand workorchdevents.WorkspaceRunRequestedCommand
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, commandHub)
	observer.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(ev magicevent.Event, result magicevent.Result) {
		schedulerCommand, _ = ev.Data().(workorchdevents.WorkspaceRunRequestedCommand)
		result.Set(workorchdevents.WorkspaceRunRequestedResult{Accepted: true}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested) })

	dispatcher := NewCLICommandDispatcher(session, hub, commandHub, nil)
	resource, _, err := dispatcher.handleGovernanceCommand(context.Background(), clicore.Command{
		CommandID:   "cmd-cli-replan",
		WorkspaceID: workspaceID,
		RunID:       runID,
		SubjectID:   string(ticketID),
		Command:     clicore.CommandDecisionResolve,
		Payload: map[string]any{
			"resolutionType": decisioncommon.ResolutionTypeReplan,
			"payload":        map[string]any{"operator": "tester"},
		},
	})
	if err != nil {
		t.Fatalf("handle governance command: %v", err)
	}
	if resource["id"] != string(ticketID) {
		t.Fatalf("resource id = %v, want %s", resource["id"], ticketID)
	}
	if publishedHub, ok := hub.(interface{ Published() []events.Envelope }); ok {
		for _, envelope := range publishedHub.Published() {
			if envelope.Topic == orchestratorevents.TopicOrchestratorCommandResumeRun {
				t.Fatal("CLI decision resolve published direct orchestrator resume; want scheduler command only")
			}
		}
	}
	if schedulerCommand.WorkspaceID != workspaceID || schedulerCommand.RunID != runID {
		t.Fatalf("scheduler command scope = workspace:%q run:%q, want %s/%s", schedulerCommand.WorkspaceID, schedulerCommand.RunID, workspaceID, runID)
	}
	if schedulerCommand.Intent != "replan" ||
		schedulerCommand.Reason != "decision_replan_resolved" ||
		schedulerCommand.DecisionID != string(ticketID) ||
		schedulerCommand.Source != "cli" {
		t.Fatalf("scheduler command = %#v, want cli replan continuation", schedulerCommand)
	}
}

func TestCLIWorkspaceObjectCommandsRequireMatchingWorkspaceScope(t *testing.T) {
	dispatcher := &CLICommandDispatcher{viewReader: &staticCLIWorkspaceViewReader{}}
	for _, tc := range []struct {
		name        string
		command     string
		workspaceID string
		subjectID   string
		wantErr     error
	}{
		{
			name:      "snapshot missing workspace scope",
			command:   clicore.CommandWorkspaceSnapshot,
			subjectID: "ws-scope",
			wantErr:   common.ErrInvalidArgument,
		},
		{
			name:        "snapshot mismatched workspace scope",
			command:     clicore.CommandWorkspaceSnapshot,
			workspaceID: "ws-a",
			subjectID:   "ws-b",
			wantErr:     common.ErrConflict,
		},
		{
			name:        "get missing subject",
			command:     clicore.CommandWorkspaceGet,
			workspaceID: "ws-scope",
			wantErr:     common.ErrInvalidArgument,
		},
		{
			name:        "delete mismatched workspace scope",
			command:     clicore.CommandWorkspaceDelete,
			workspaceID: "ws-a",
			subjectID:   "ws-b",
			wantErr:     common.ErrConflict,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := dispatcher.handleWorkspaceCommand(context.Background(), clicore.Command{
				Command:     tc.command,
				WorkspaceID: common.WorkspaceID(tc.workspaceID),
				SubjectID:   tc.subjectID,
			})
			if err == nil || !errors.Is(err, tc.wantErr) {
				t.Fatalf("handle workspace command err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestCLIRunCommandsRequireWorkspaceScope(t *testing.T) {
	dispatcher := &CLICommandDispatcher{}
	for _, tc := range []struct {
		name        string
		command     string
		workspaceID string
		runID       string
		subjectID   string
		wantErr     error
	}{
		{
			name:    "activate missing workspace",
			command: clicore.CommandRunActivate,
			wantErr: common.ErrInvalidArgument,
		},
		{
			name:    "list missing workspace",
			command: clicore.CommandRunList,
			wantErr: common.ErrInvalidArgument,
		},
		{
			name:      "resume missing workspace",
			command:   clicore.CommandRunResume,
			subjectID: "run-a",
			wantErr:   common.ErrInvalidArgument,
		},
		{
			name:        "get missing run subject",
			command:     clicore.CommandRunGet,
			workspaceID: "ws-a",
			wantErr:     common.ErrInvalidArgument,
		},
		{
			name:        "status mismatched run subject",
			command:     clicore.CommandRunStatus,
			workspaceID: "ws-a",
			runID:       "run-a",
			subjectID:   "run-b",
			wantErr:     common.ErrConflict,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := dispatcher.handleRunCommand(context.Background(), clicore.Command{
				Command:     tc.command,
				WorkspaceID: common.WorkspaceID(tc.workspaceID),
				RunID:       common.RunID(tc.runID),
				SubjectID:   tc.subjectID,
			})
			if err == nil || !errors.Is(err, tc.wantErr) {
				t.Fatalf("handle run command err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestCLIWorkspaceListCommandUsesWorkspaceViewReader(t *testing.T) {
	reader := &staticCLIWorkspaceViewReader{
		workspaces: []Workspace{{
			ID:    "ws-projection-list",
			Title: "projection list",
			State: "active",
		}},
	}
	dispatcher := &CLICommandDispatcher{viewReader: reader}

	_, resources, err := dispatcher.handleWorkspaceCommand(context.Background(), clicore.Command{
		Command: clicore.CommandWorkspaceList,
	})
	if err != nil {
		t.Fatalf("handle workspace list: %v", err)
	}
	if reader.listCalls != 1 {
		t.Fatalf("list calls = %d, want 1", reader.listCalls)
	}
	if len(resources) != 1 || resources[0]["id"] != "ws-projection-list" {
		t.Fatalf("resources = %+v, want projection workspace list", resources)
	}
}

func TestCLIWorkspaceSnapshotCommandUsesProjectionViewReaderAndOmitsEventPayload(t *testing.T) {
	reader := &staticCLIWorkspaceViewReader{
		snapshot: WorkspaceSnapshot{
			Workspace: Workspace{ID: "ws-projection-snapshot"},
			EventRecords: []EventRecord{{
				ID:             "event-large-payload",
				Topic:          "runtime.event",
				WorkspaceID:    "ws-projection-snapshot",
				Payload:        map[string]any{"large": strings.Repeat("x", 4096)},
				PayloadSummary: map[string]any{"state": "running"},
				PayloadRef:     "event://payload/event-large-payload",
			}},
		},
	}
	dispatcher := &CLICommandDispatcher{viewReader: reader}

	resource, _, err := dispatcher.handleWorkspaceCommand(context.Background(), clicore.Command{
		Command:     clicore.CommandWorkspaceSnapshot,
		WorkspaceID: "ws-projection-snapshot",
		SubjectID:   "ws-projection-snapshot",
	})
	if err != nil {
		t.Fatalf("handle workspace snapshot: %v", err)
	}
	if reader.snapshotCalls != 1 || reader.lastSnapshotWorkspaceID != "ws-projection-snapshot" {
		t.Fatalf("snapshot calls = %d workspace = %q, want projection reader called once for workspace", reader.snapshotCalls, reader.lastSnapshotWorkspaceID)
	}
	eventRecords := resource["eventRecords"].([]map[string]any)
	if len(eventRecords) != 1 {
		t.Fatalf("eventRecords = %+v, want one projected record", eventRecords)
	}
	if _, ok := eventRecords[0]["payload"]; ok {
		t.Fatalf("event record resource includes raw payload: %+v", eventRecords[0])
	}
	if eventRecords[0]["payloadRef"] != "event://payload/event-large-payload" {
		t.Fatalf("event record payloadRef = %v, want raw ref retained", eventRecords[0]["payloadRef"])
	}
	if summary, _ := eventRecords[0]["payloadSummary"].(map[string]any); summary["state"] != "running" {
		t.Fatalf("event record payloadSummary = %+v, want summary retained", eventRecords[0]["payloadSummary"])
	}
	typed, ok := clicore.TypedResource(resource).(port.WorkspaceSnapshotInfo)
	if !ok {
		t.Fatalf("typed resource = %T, want WorkspaceSnapshotInfo", clicore.TypedResource(resource))
	}
	if len(typed.EventRecords) != 1 {
		t.Fatalf("typed event records = %+v, want one projected event", typed.EventRecords)
	}
	if typed.EventRecords[0].Payload != nil {
		t.Fatalf("typed event record payload = %+v, want snapshot DTO without raw payload", typed.EventRecords[0].Payload)
	}
	if typed.EventRecords[0].PayloadRef != "event://payload/event-large-payload" ||
		typed.EventRecords[0].PayloadSummary["state"] != "running" {
		t.Fatalf("typed event record = %+v, want summary/ref retained", typed.EventRecords[0])
	}
}

func workspaceSnapshotTraceProjectionStringLimitForTest() int {
	return 1024 + len("...[truncated]")
}

type staticCLIWorkspaceViewReader struct {
	workspaces              []Workspace
	snapshot                WorkspaceSnapshot
	listCalls               int
	snapshotCalls           int
	lastSnapshotWorkspaceID string
}

func (r *staticCLIWorkspaceViewReader) ListWorkspaceView(context.Context) ([]Workspace, error) {
	r.listCalls++
	return r.workspaces, nil
}

func (r *staticCLIWorkspaceViewReader) ReadWorkspaceViewSnapshot(_ context.Context, workspaceID string) (WorkspaceSnapshot, error) {
	r.snapshotCalls++
	r.lastSnapshotWorkspaceID = workspaceID
	return r.snapshot, nil
}
