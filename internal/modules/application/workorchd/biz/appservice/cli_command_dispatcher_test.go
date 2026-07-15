package appservice

import (
	"context"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
)

func TestCLICommandDispatcherRunIsIdempotent(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	reader := &staticCLIWorkspaceViewReader{workspaces: []Workspace{{ID: "ws-cli-idempotent"}}}
	dispatcher := NewCLICommandDispatcher(&RuntimeSession{}, hub, reader)
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if result := sendCLIWorkspaceList(t, hub); len(result.Workspaces) != 1 || reader.listCalls != 1 {
		t.Fatalf("first result = %+v list calls = %d, want one handler", result, reader.listCalls)
	}
	dispatcher.Close()
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("Run after Close: %v", err)
	}
	if result := sendCLIWorkspaceList(t, hub); len(result.Workspaces) != 1 || reader.listCalls != 2 {
		t.Fatalf("second result = %+v list calls = %d, want one handler per command", result, reader.listCalls)
	}
}

func sendCLIWorkspaceList(t *testing.T, hub magicevent.Hub) workorchdevents.CLIWorkspaceListResult {
	t.Helper()
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceList, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), workorchdevents.CLIWorkspaceListCommand{})
	response := hub.Send(event)
	if response == nil || response.Error() != nil {
		t.Fatalf("send workspace list response = %+v", response)
	}
	data, err := response.Get()
	if err != nil {
		t.Fatalf("get workspace list result: %v", err)
	}
	result, ok := data.(workorchdevents.CLIWorkspaceListResult)
	if !ok {
		t.Fatalf("workspace list result = %T", data)
	}
	return result
}

func TestCLIWorkspaceSnapshotCommandProjectsDetailView(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	dispatcher := NewCLICommandDispatcher(&RuntimeSession{}, hub, &staticCLIWorkspaceViewReader{snapshot: WorkspaceSnapshot{
		Workspace:      Workspace{ID: "ws-cli-detail", ActiveRunID: "run-cli-active", ContextHints: map[string]any{"large": "profile"}, EvidencePolicies: []map[string]any{{"large": "policy"}}},
		Runs:           []Run{{ID: "run-cli-active", WorkspaceID: "ws-cli-detail", State: "running"}},
		LLMUsageTraces: []LLMUsageTrace{{ID: "usage-cli", RunID: "run-cli-active", ContextBudget: mustReadModelEventPayload(t, map[string]any{"failureSignature": strings.Repeat("x", 2048)})}},
	}})
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("run dispatcher: %v", err)
	}
	t.Cleanup(dispatcher.Close)

	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceSnapshot, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), workorchdevents.CLIWorkspaceSnapshotCommand{WorkspaceID: "ws-cli-detail"})
	response := hub.Send(event)
	if response == nil || response.Error() != nil {
		t.Fatalf("snapshot response = %+v", response)
	}
	data, err := response.Get()
	if err != nil {
		t.Fatalf("snapshot result: %v", err)
	}
	result, ok := data.(workorchdevents.CLIWorkspaceSnapshotResult)
	if !ok {
		t.Fatalf("snapshot result = %T", data)
	}
	if len(result.Snapshot.Runs) != 1 {
		t.Fatalf("runs = %+v", result.Snapshot.Runs)
	}
	signature, _ := result.Snapshot.LLMUsageTraces[0].ContextBudget.String("failureSignature")
	if len(signature) > workspaceSnapshotTraceProjectionStringLimitForTest() {
		t.Fatalf("failureSignature length = %d", len(signature))
	}
}

func TestCLIDecisionResolvePublishesSchedulerContinuation(t *testing.T) {
	session, tickets, _ := newCountingDecisionRuntimeSession(t)
	const workspaceID = common.WorkspaceID("ws-cli-governance-scheduler")
	const runID = common.RunID("run-cli-governance-scheduler")
	const ticketID = common.TicketID("ticket-cli-governance-scheduler")
	if err := tickets.Create(context.Background(), &decisionmodel.Ticket{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateOpen, Reason: "need replan"}); err != nil {
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
	dispatcher := NewCLICommandDispatcher(session, commandHub, nil)
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("run dispatcher: %v", err)
	}
	t.Cleanup(dispatcher.Close)
	command := workorchdevents.CLIDecisionResolveCommand{RequestID: "cmd-cli-replan", Request: workorchport.ResolveDecisionRequest{WorkspaceID: workspaceID, TicketID: ticketID, ResolutionType: decisioncommon.ResolutionTypeReplan, Payload: workorcheventpayload.New(workorcheventpayload.StringField("operator", "tester"))}}
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIDecisionResolve, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), command)
	response := commandHub.Send(event)
	if response == nil || response.Error() != nil {
		t.Fatalf("decision response = %+v", response)
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		t.Fatalf("decision result: %v", resultErr)
	}
	result, ok := data.(workorchdevents.CLIDecisionResolveResult)
	if !ok || result.Decision.ID != ticketID {
		t.Fatalf("decision = %+v", data)
	}
	if schedulerCommand.WorkspaceID != workspaceID || schedulerCommand.RunID != runID || schedulerCommand.Intent != "replan" || schedulerCommand.Reason != "decision_replan_resolved" || schedulerCommand.DecisionID != string(ticketID) || schedulerCommand.Source != "cli" {
		t.Fatalf("scheduler command = %#v", schedulerCommand)
	}
}

func TestCLIWorkspaceCommandsRejectMissingScope(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	dispatcher := NewCLICommandDispatcher(&RuntimeSession{}, hub, &staticCLIWorkspaceViewReader{})
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("run dispatcher: %v", err)
	}
	t.Cleanup(dispatcher.Close)
	for _, command := range []struct {
		topic string
		value any
	}{
		{workorchdevents.TopicCLIWorkspaceGet, workorchdevents.CLIWorkspaceGetCommand{}},
		{workorchdevents.TopicCLIWorkspaceSnapshot, workorchdevents.CLIWorkspaceSnapshotCommand{}},
		{workorchdevents.TopicCLIWorkspaceDelete, workorchdevents.CLIWorkspaceDeleteCommand{}},
	} {
		event := magicevent.NewEventWithContext(command.topic, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), command.value)
		response := hub.Send(event)
		if response == nil || response.Error() == nil {
			t.Fatalf("topic %s response = %+v, want error", command.topic, response)
		}
	}
}

func TestCLIWorkspaceListCommandUsesWorkspaceViewReader(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	reader := &staticCLIWorkspaceViewReader{workspaces: []Workspace{{ID: "ws-projection-list", Title: "projection list", State: "active"}}}
	dispatcher := NewCLICommandDispatcher(&RuntimeSession{}, hub, reader)
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("run dispatcher: %v", err)
	}
	t.Cleanup(dispatcher.Close)
	result := sendCLIWorkspaceList(t, hub)
	if reader.listCalls != 1 || len(result.Workspaces) != 1 || result.Workspaces[0].ID != "ws-projection-list" {
		t.Fatalf("result = %+v calls = %d", result, reader.listCalls)
	}
}

func TestCLIWorkspaceSnapshotCommandUsesProjectionViewReaderAndOmitsEventPayload(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	reader := &staticCLIWorkspaceViewReader{snapshot: WorkspaceSnapshot{Workspace: Workspace{ID: "ws-projection-snapshot"}, EventRecords: []EventRecord{{ID: "event-large-payload", Topic: "runtime.event", SubjectType: "run", WorkspaceID: "ws-projection-snapshot", Payload: mustReadModelEventPayload(t, map[string]any{"state": "active", "failureReason": "provider unavailable"}), PayloadRef: "event://payload/event-large-payload"}}}}
	dispatcher := NewCLICommandDispatcher(&RuntimeSession{}, hub, reader)
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("run dispatcher: %v", err)
	}
	t.Cleanup(dispatcher.Close)
	event := magicevent.NewEventWithContext(workorchdevents.TopicCLIWorkspaceSnapshot, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), workorchdevents.CLIWorkspaceSnapshotCommand{WorkspaceID: "ws-projection-snapshot"})
	response := hub.Send(event)
	if response == nil || response.Error() != nil {
		t.Fatalf("snapshot response = %+v", response)
	}
	data, err := response.Get()
	if err != nil {
		t.Fatalf("snapshot result: %v", err)
	}
	result := data.(workorchdevents.CLIWorkspaceSnapshotResult)
	if reader.snapshotCalls != 1 || reader.lastSnapshotWorkspaceID != "ws-projection-snapshot" {
		t.Fatalf("snapshot calls = %d workspace = %q", reader.snapshotCalls, reader.lastSnapshotWorkspaceID)
	}
	if len(result.Snapshot.EventRecords) != 1 {
		t.Fatalf("eventRecords = %+v", result.Snapshot.EventRecords)
	}
	state, _ := result.Snapshot.EventRecords[0].Payload.String("state")
	if state != string(common.RunStateActive) || result.Snapshot.EventRecords[0].PayloadRef != "event://payload/event-large-payload" || result.Snapshot.EventRecords[0].Summary != "provider unavailable" {
		t.Fatalf("event record = %+v", result.Snapshot.EventRecords[0])
	}
}

func workspaceSnapshotTraceProjectionStringLimitForTest() int { return 1024 + len("...[truncated]") }

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

func (r *staticCLIWorkspaceViewReader) ReadWorkspaceActivity(context.Context, string) (WorkspaceActivityFeed, bool, error) {
	return WorkspaceActivityFeed{}, false, nil
}
