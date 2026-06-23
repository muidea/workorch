package appservice

import (
	"context"
	"errors"
	"strings"
	"testing"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestCLICommandDispatcherRunIsIdempotent(t *testing.T) {
	hub := &countingCLIHub{}
	dispatcher := NewCLICommandDispatcher(&RuntimeSession{}, hub, nil)
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if hub.subscribeCount != 1 {
		t.Fatalf("subscribe count = %d, want 1", hub.subscribeCount)
	}
	dispatcher.Close()
	if hub.unsubscribeCount != 1 {
		t.Fatalf("unsubscribe count = %d, want 1", hub.unsubscribeCount)
	}
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("Run after Close: %v", err)
	}
	if hub.subscribeCount != 2 {
		t.Fatalf("subscribe count after rerun = %d, want 2", hub.subscribeCount)
	}
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

	resource, _, err := dispatcher.handleWorkspaceCommand(context.Background(), events.CLICommandPayload{
		Command:     events.CommandWorkspaceSnapshot,
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

	directResumePublished := false
	hub.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, func(context.Context, events.Envelope) error {
		directResumePublished = true
		return events.Response(events.Envelope{Topic: orchestratorevents.TopicOrchestratorCommandResumeRun + "/response"})
	})
	var schedulerEnvelope events.Envelope
	hub.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(_ context.Context, envelope events.Envelope) error {
		schedulerEnvelope = envelope
		return events.Response(events.Envelope{
			Topic:        envelope.Topic + "/accepted",
			SourceModule: "workorchd/scheduler",
			Payload:      map[string]any{"accepted": true},
		})
	})

	dispatcher := NewCLICommandDispatcher(session, hub, nil)
	resource, _, err := dispatcher.handleGovernanceCommand(context.Background(), events.CLICommandPayload{
		CommandID:   "cmd-cli-replan",
		WorkspaceID: workspaceID,
		RunID:       runID,
		SubjectID:   string(ticketID),
		Command:     events.CommandDecisionResolve,
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
	if directResumePublished {
		t.Fatal("CLI decision resolve published direct orchestrator resume; want scheduler command only")
	}
	if schedulerEnvelope.Topic != workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested {
		t.Fatalf("scheduler envelope topic = %q, want workspace run command", schedulerEnvelope.Topic)
	}
	if schedulerEnvelope.WorkspaceID != string(workspaceID) || schedulerEnvelope.RunID != string(runID) {
		t.Fatalf("scheduler envelope scope = workspace:%q run:%q, want %s/%s", schedulerEnvelope.WorkspaceID, schedulerEnvelope.RunID, workspaceID, runID)
	}
	if schedulerEnvelope.Payload[events.PayloadKeyIntent] != "replan" ||
		schedulerEnvelope.Payload[events.PayloadKeyReason] != "decision_replan_resolved" ||
		schedulerEnvelope.Payload[events.PayloadKeyDecisionID] != string(ticketID) ||
		schedulerEnvelope.Payload[events.PayloadKeySource] != "cli" {
		t.Fatalf("scheduler payload = %#v, want cli replan continuation", schedulerEnvelope.Payload)
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
			command:   events.CommandWorkspaceSnapshot,
			subjectID: "ws-scope",
			wantErr:   common.ErrInvalidArgument,
		},
		{
			name:        "snapshot mismatched workspace scope",
			command:     events.CommandWorkspaceSnapshot,
			workspaceID: "ws-a",
			subjectID:   "ws-b",
			wantErr:     common.ErrConflict,
		},
		{
			name:        "get missing subject",
			command:     events.CommandWorkspaceGet,
			workspaceID: "ws-scope",
			wantErr:     common.ErrInvalidArgument,
		},
		{
			name:        "delete mismatched workspace scope",
			command:     events.CommandWorkspaceDelete,
			workspaceID: "ws-a",
			subjectID:   "ws-b",
			wantErr:     common.ErrConflict,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := dispatcher.handleWorkspaceCommand(context.Background(), events.CLICommandPayload{
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
			command: events.CommandRunActivate,
			wantErr: common.ErrInvalidArgument,
		},
		{
			name:    "list missing workspace",
			command: events.CommandRunList,
			wantErr: common.ErrInvalidArgument,
		},
		{
			name:      "resume missing workspace",
			command:   events.CommandRunResume,
			subjectID: "run-a",
			wantErr:   common.ErrInvalidArgument,
		},
		{
			name:        "get missing run subject",
			command:     events.CommandRunGet,
			workspaceID: "ws-a",
			wantErr:     common.ErrInvalidArgument,
		},
		{
			name:        "status mismatched run subject",
			command:     events.CommandRunStatus,
			workspaceID: "ws-a",
			runID:       "run-a",
			subjectID:   "run-b",
			wantErr:     common.ErrConflict,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := dispatcher.handleRunCommand(context.Background(), events.CLICommandPayload{
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

	_, resources, err := dispatcher.handleWorkspaceCommand(context.Background(), events.CLICommandPayload{
		Command: events.CommandWorkspaceList,
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

func workspaceSnapshotTraceProjectionStringLimitForTest() int {
	return 1024 + len("...[truncated]")
}

type staticCLIWorkspaceViewReader struct {
	workspaces []Workspace
	snapshot   WorkspaceSnapshot
	listCalls  int
}

func (r *staticCLIWorkspaceViewReader) ListWorkspaceView(context.Context) ([]Workspace, error) {
	r.listCalls++
	return r.workspaces, nil
}

func (r *staticCLIWorkspaceViewReader) ReadWorkspaceViewSnapshot(context.Context, string) (WorkspaceSnapshot, error) {
	return r.snapshot, nil
}

type countingCLIHub struct {
	subscribeCount   int
	unsubscribeCount int
}

func (h *countingCLIHub) Publish(context.Context, events.Envelope) error {
	return nil
}

func (h *countingCLIHub) Subscribe(string, events.Handler) func() {
	h.subscribeCount++
	return func() {
		h.unsubscribeCount++
	}
}
