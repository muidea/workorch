package ownerreadports

import (
	"context"
	"errors"
	"testing"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPublishCommandWithResponseRejectsWriteTopic(t *testing.T) {
	_, err := PublishCommandWithResponse(context.Background(), events.NewMemoryHub(), runevents.TopicRunCommandUpdate, "run", "run-001", "", "run-001", map[string]any{
		events.PayloadKeyRunID: "run-001",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("PublishCommandWithResponse err = %v, want invalid argument for write topic", err)
	}
}

func TestPublishCommandWithResponsePublishesReadTopic(t *testing.T) {
	hub := events.NewMemoryHub()
	hub.Subscribe(runevents.TopicRunCommandGet, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{
			Topic:        envelope.Topic + "/response",
			SourceModule: "test",
			Payload: map[string]any{
				"ok": true,
			},
		})
	})

	response, err := PublishCommandWithResponse(context.Background(), hub, runevents.TopicRunCommandGet, "run", "run-001", "", "run-001", map[string]any{
		events.PayloadKeyRunID: "run-001",
	})
	if err != nil {
		t.Fatalf("PublishCommandWithResponse err = %v", err)
	}
	if response.Payload["ok"] != true {
		t.Fatalf("response payload = %+v, want ok", response.Payload)
	}
}

func TestOwnerReadCommandsCarryWorkspaceScopeInEnvelopeAndPayload(t *testing.T) {
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
			name:      "results by run",
			topic:     resultevents.TopicResultCommandListByRun,
			wantRunID: string(runID),
			responsePayload: map[string]any{
				events.PayloadKeyItems: []*resultmodel.Result{},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ListResultsByRunOwnerCommand(ctx, hub, runID)
				return err
			},
		},
		{
			name:      "artifacts by run",
			topic:     resultevents.TopicArtifactCommandListByRun,
			wantRunID: string(runID),
			responsePayload: map[string]any{
				events.PayloadKeyItems: []*resultmodel.Artifact{},
			},
			invoke: func(ctx context.Context, hub events.Hub) error {
				_, err := ListArtifactsByRunOwnerCommand(ctx, hub, runID)
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
			hub := events.NewMemoryHub()
			hub.Subscribe(tt.topic, func(_ context.Context, envelope events.Envelope) error {
				if envelope.WorkspaceID != string(workspaceID) {
					t.Fatalf("envelope workspaceID = %q, want %s", envelope.WorkspaceID, workspaceID)
				}
				if tt.wantRunID != "" && envelope.RunID != tt.wantRunID {
					t.Fatalf("envelope runID = %q, want %s", envelope.RunID, tt.wantRunID)
				}
				if got, _ := envelope.Payload[events.PayloadKeyWorkspaceID].(string); got != string(workspaceID) {
					t.Fatalf("payload workspaceID = %q, want %s", got, workspaceID)
				}
				if tt.wantRunID != "" {
					if got, _ := envelope.Payload[events.PayloadKeyRunID].(string); got != tt.wantRunID {
						t.Fatalf("payload runID = %q, want %s", got, tt.wantRunID)
					}
				}
				return events.Response(events.Envelope{
					Topic:   envelope.Topic + "/response",
					Payload: tt.responsePayload,
				})
			})
			if err := tt.invoke(ctx, hub); err != nil {
				t.Fatalf("invoke err = %v", err)
			}
		})
	}
}
