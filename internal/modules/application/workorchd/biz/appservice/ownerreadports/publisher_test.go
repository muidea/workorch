package ownerreadports

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerReadCommandsCarryWorkspaceScopeInMagicEvent(t *testing.T) {
	const workspaceID = common.WorkspaceID("ws-scope-owner-read")
	const runID = common.RunID("run-scope-owner-read")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	tests := []struct {
		name            string
		topic           string
		wantRunID       string
		responsePayload map[string]any
		invoke          func(context.Context, events.Hub) error
	}{
		{
			name:      "run get",
			topic:     runevents.TopicRunCommandGet,
			wantRunID: string(runID),
			responsePayload: map[string]any{
				events.PayloadKeyRun: &runmodel.Run{ID: runID, WorkspaceID: workspaceID},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ReadRunByOwnerCommand(ctx, hub, runID)
				return err
			},
		},
		{
			name:      "steps by run",
			topic:     runevents.TopicRunCommandListStepsByRun,
			wantRunID: string(runID),
			responsePayload: map[string]any{
				events.PayloadKeySteps: []*runmodel.Step{},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ListStepsByRunOwnerCommand(ctx, hub, runID)
				return err
			},
		},
		{
			name:      "step summaries by run",
			topic:     runevents.TopicRunCommandListStepSummariesByRun,
			wantRunID: string(runID),
			responsePayload: map[string]any{
				events.PayloadKeySteps: []*runmodel.Step{},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ListStepSummariesByRunOwnerCommand(ctx, hub, runID)
				return err
			},
		},
		{
			name:      "checkpoints by run",
			topic:     runevents.TopicRunCommandListCheckpointsByRun,
			wantRunID: string(runID),
			responsePayload: map[string]any{
				events.PayloadKeyCheckpoints: []*runmodel.RuntimeCheckpoint{},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ListRuntimeCheckpointsByRunOwnerCommand(ctx, hub, runID)
				return err
			},
		},
		{
			name:      "tool calls by run",
			topic:     orchestratorevents.TopicOrchestratorCommandListToolCallsByRun,
			wantRunID: string(runID),
			responsePayload: map[string]any{
				events.PayloadKeyItems: []*runmodel.ToolCall{},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ListToolCallsByRunOwnerCommand(ctx, hub, runID)
				return err
			},
		},
		{
			name:      "artifact continuations by run",
			topic:     orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun,
			wantRunID: string(runID),
			responsePayload: map[string]any{
				events.PayloadKeyItems: []*runtimecontract.ArtifactContinuation{},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ListArtifactContinuationsByRunOwnerCommand(ctx, hub, runID)
				return err
			},
		},
		{
			name:      "coverage work items by run",
			topic:     orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun,
			wantRunID: string(runID),
			responsePayload: map[string]any{
				events.PayloadKeyItems: []*runtimecontract.CoverageWorkItem{},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ListCoverageWorkItemsByRunOwnerCommand(ctx, hub, runID)
				return err
			},
		},
		{
			name:      "decision ticket get",
			topic:     decisionevents.TopicDecisionCommandGetTicket,
			wantRunID: "",
			responsePayload: map[string]any{
				events.PayloadKeyTicket: &decisionmodel.Ticket{ID: "ticket-scope-owner-read", WorkspaceID: workspaceID, RunID: runID},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ReadDecisionTicketByOwnerCommand(ctx, hub, "ticket-scope-owner-read")
				return err
			},
		},
		{
			name:      "decision tickets by run",
			topic:     decisionevents.TopicDecisionCommandListTicketsByRun,
			wantRunID: string(runID),
			responsePayload: map[string]any{
				events.PayloadKeyTickets: []*decisionmodel.Ticket{},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ListDecisionTicketsByRunOwnerCommand(ctx, hub, runID)
				return err
			},
		},
		{
			name:      "approval get",
			topic:     approvalevents.TopicApprovalCommandGet,
			wantRunID: "",
			responsePayload: map[string]any{
				events.PayloadKeyApproval: &approvalmodel.Approval{ID: "approval-scope-owner-read", WorkspaceID: workspaceID, RunID: runID},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ReadApprovalByOwnerCommand(ctx, hub, "approval-scope-owner-read")
				return err
			},
		},
		{
			name:      "approvals by run",
			topic:     approvalevents.TopicApprovalCommandListByRun,
			wantRunID: string(runID),
			responsePayload: map[string]any{
				events.PayloadKeyItems: []*approvalmodel.Approval{},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ListApprovalsByRunOwnerCommand(ctx, hub, runID)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawHub := magicevent.NewHub(16)
			hub := events.NewMagicHubWithHubOwner(rawHub, "ownerreadports/test")
			defer hub.Terminate(context.Background())
			subscribeOwnerReadMagicResponse(t, rawHub, tt.topic, workspaceID, common.RunID(tt.wantRunID), tt.responsePayload)
			if err := tt.invoke(ctx, hub); err != nil {
				t.Fatalf("invoke err = %v", err)
			}
		})
	}
}

func TestResultOwnerReadCommandsCarryWorkspaceScopeInMagicEvent(t *testing.T) {
	const workspaceID = common.WorkspaceID("ws-scope-result-owner-read")
	const runID = common.RunID("run-scope-result-owner-read")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	rawHub := magicevent.NewHub(16)
	hub := events.NewMagicHubWithHub(rawHub)

	subscribeResultReadCommand(t, rawHub, resultevents.TopicResultCommandListByRun, func(ev magicevent.Event, response magicevent.Result) {
		cmd, ok := ev.Data().(resultevents.ListResultsByRunCommand)
		if !ok {
			t.Fatalf("result command type = %T", ev.Data())
		}
		if cmd.RunID != runID {
			t.Fatalf("result command runID = %q, want %s", cmd.RunID, runID)
		}
		if got := common.RuntimeWorkspaceIDFromContext(ev.Context()); got != workspaceID {
			t.Fatalf("result command workspace context = %q, want %s", got, workspaceID)
		}
		response.Set(resultevents.ResultCommandResult{}, nil)
	})
	subscribeResultReadCommand(t, rawHub, resultevents.TopicArtifactCommandListByRun, func(ev magicevent.Event, response magicevent.Result) {
		cmd, ok := ev.Data().(resultevents.ListArtifactsByRunCommand)
		if !ok {
			t.Fatalf("artifact command type = %T", ev.Data())
		}
		if cmd.RunID != runID {
			t.Fatalf("artifact command runID = %q, want %s", cmd.RunID, runID)
		}
		if got := common.RuntimeWorkspaceIDFromContext(ev.Context()); got != workspaceID {
			t.Fatalf("artifact command workspace context = %q, want %s", got, workspaceID)
		}
		response.Set(resultevents.ResultCommandResult{}, nil)
	})

	if _, err := ListResultsByRunOwnerCommand(ctx, hub, runID); err != nil {
		t.Fatalf("ListResultsByRunOwnerCommand err = %v", err)
	}
	if _, err := ListArtifactsByRunOwnerCommand(ctx, hub, runID); err != nil {
		t.Fatalf("ListArtifactsByRunOwnerCommand err = %v", err)
	}
}

func subscribeResultReadCommand(t *testing.T, hub magicevent.Hub, topic string, handle func(magicevent.Event, magicevent.Result)) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(resultevents.CommandDestination, hub)
	observer.Subscribe(topic, handle)
}

func subscribeOwnerReadMagicResponse(t *testing.T, hub magicevent.Hub, topic string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) {
	t.Helper()
	destination, ok := ownerReadMagicDestination(topic)
	if !ok {
		return
	}
	observer := magicevent.NewSimpleObserver(destination, hub)
	observer.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		if got := common.RuntimeWorkspaceIDFromContext(ev.Context()); got != workspaceID {
			t.Fatalf("magic command workspace context = %q, want %s", got, workspaceID)
		}
		switch topic {
		case runevents.TopicRunCommandGet:
			cmd, ok := ev.Data().(runevents.GetRunCommand)
			if !ok {
				t.Fatalf("run get command type = %T", ev.Data())
			}
			if cmd.WorkspaceID != workspaceID || cmd.RunID != runID {
				t.Fatalf("run get command = %+v, want %s/%s", cmd, workspaceID, runID)
			}
			result.Set(runevents.RunCommandResult{Run: payload[events.PayloadKeyRun].(*runmodel.Run)}, nil)
		case runevents.TopicRunCommandListStepsByRun, runevents.TopicRunCommandListStepSummariesByRun:
			cmd, ok := ev.Data().(runevents.ListStepsByRunCommand)
			if !ok {
				t.Fatalf("step list command type = %T", ev.Data())
			}
			if cmd.WorkspaceID != workspaceID || cmd.RunID != runID {
				t.Fatalf("step list command = %+v, want %s/%s", cmd, workspaceID, runID)
			}
			result.Set(runevents.RunCommandResult{Steps: payload[events.PayloadKeySteps].([]*runmodel.Step)}, nil)
		case runevents.TopicRunCommandListCheckpointsByRun:
			cmd, ok := ev.Data().(runevents.ListRuntimeCheckpointsByRunCommand)
			if !ok {
				t.Fatalf("checkpoint list command type = %T", ev.Data())
			}
			if cmd.WorkspaceID != workspaceID || cmd.RunID != runID {
				t.Fatalf("checkpoint list command = %+v, want %s/%s", cmd, workspaceID, runID)
			}
			result.Set(runevents.RunCommandResult{Checkpoints: payload[events.PayloadKeyCheckpoints].([]*runmodel.RuntimeCheckpoint)}, nil)
		case orchestratorevents.TopicOrchestratorCommandListToolCallsByRun:
			cmd, ok := ev.Data().(orchestratorevents.ListToolCallsByRunCommand)
			if !ok {
				t.Fatalf("tool call list command type = %T", ev.Data())
			}
			if cmd.WorkspaceID != workspaceID || cmd.RunID != runID {
				t.Fatalf("tool call list command = %+v, want %s/%s", cmd, workspaceID, runID)
			}
			result.Set(orchestratorevents.OrchestratorCommandResult{ToolCalls: payload[events.PayloadKeyItems].([]*runmodel.ToolCall)}, nil)
		case orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun:
			cmd, ok := ev.Data().(orchestratorevents.ListArtifactContinuationsByRunCommand)
			if !ok {
				t.Fatalf("artifact continuation list command type = %T", ev.Data())
			}
			if cmd.WorkspaceID != workspaceID || cmd.RunID != runID {
				t.Fatalf("artifact continuation list command = %+v, want %s/%s", cmd, workspaceID, runID)
			}
			result.Set(orchestratorevents.OrchestratorCommandResult{ArtifactContinuations: payload[events.PayloadKeyItems].([]*runtimecontract.ArtifactContinuation)}, nil)
		case orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun:
			cmd, ok := ev.Data().(orchestratorevents.ListCoverageWorkItemsByRunCommand)
			if !ok {
				t.Fatalf("coverage work item list command type = %T", ev.Data())
			}
			if cmd.WorkspaceID != workspaceID || cmd.RunID != runID {
				t.Fatalf("coverage work item list command = %+v, want %s/%s", cmd, workspaceID, runID)
			}
			result.Set(orchestratorevents.OrchestratorCommandResult{CoverageWorkItems: payload[events.PayloadKeyItems].([]*runtimecontract.CoverageWorkItem)}, nil)
		case decisionevents.TopicDecisionCommandGetTicket:
			cmd, ok := ev.Data().(decisionevents.GetTicketCommand)
			if !ok {
				t.Fatalf("ticket get command type = %T", ev.Data())
			}
			if cmd.WorkspaceID != workspaceID || cmd.TicketID == "" {
				t.Fatalf("ticket get command = %+v, want workspace %s", cmd, workspaceID)
			}
			result.Set(decisionevents.DecisionCommandResult{Ticket: payload[events.PayloadKeyTicket].(*decisionmodel.Ticket)}, nil)
		case decisionevents.TopicDecisionCommandListTicketsByRun:
			cmd, ok := ev.Data().(decisionevents.ListTicketsByRunCommand)
			if !ok {
				t.Fatalf("ticket list command type = %T", ev.Data())
			}
			if cmd.WorkspaceID != workspaceID || cmd.RunID != runID {
				t.Fatalf("ticket list command = %+v, want %s/%s", cmd, workspaceID, runID)
			}
			result.Set(decisionevents.DecisionCommandResult{Tickets: payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)}, nil)
		case approvalevents.TopicApprovalCommandGet:
			cmd, ok := ev.Data().(approvalevents.GetApprovalCommand)
			if !ok {
				t.Fatalf("approval get command type = %T", ev.Data())
			}
			if cmd.WorkspaceID != workspaceID || cmd.ApprovalID == "" {
				t.Fatalf("approval get command = %+v, want workspace %s", cmd, workspaceID)
			}
			result.Set(approvalevents.ApprovalCommandResult{Approval: payload[events.PayloadKeyApproval].(*approvalmodel.Approval)}, nil)
		case approvalevents.TopicApprovalCommandListByRun:
			cmd, ok := ev.Data().(approvalevents.ListApprovalsByRunCommand)
			if !ok {
				t.Fatalf("approval list command type = %T", ev.Data())
			}
			if cmd.WorkspaceID != workspaceID || cmd.RunID != runID {
				t.Fatalf("approval list command = %+v, want %s/%s", cmd, workspaceID, runID)
			}
			result.Set(approvalevents.ApprovalCommandResult{Approvals: payload[events.PayloadKeyItems].([]*approvalmodel.Approval)}, nil)
		default:
			t.Fatalf("unhandled magic owner read topic %s", topic)
		}
	})
}

func ownerReadMagicDestination(topic string) (string, bool) {
	switch topic {
	case runevents.TopicRunCommandGet,
		runevents.TopicRunCommandListStepsByRun,
		runevents.TopicRunCommandListStepSummariesByRun,
		runevents.TopicRunCommandListCheckpointsByRun:
		return runevents.CommandDestination, true
	case orchestratorevents.TopicOrchestratorCommandListToolCallsByRun,
		orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun,
		orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun:
		return orchestratorevents.CommandDestination, true
	case decisionevents.TopicDecisionCommandGetTicket,
		decisionevents.TopicDecisionCommandListTicketsByRun:
		return decisionevents.CommandDestination, true
	case approvalevents.TopicApprovalCommandGet,
		approvalevents.TopicApprovalCommandListByRun:
		return approvalevents.CommandDestination, true
	default:
		return "", false
	}
}
