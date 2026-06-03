package appservice

import (
	"context"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type eventHubReadModelWorkspaceReader struct {
	hub events.Hub
}

func (r eventHubReadModelWorkspaceReader) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return readWorkspaceByOwnerCommand(ctx, r.hub, id)
}

func (r eventHubReadModelWorkspaceReader) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	return listWorkspacesByOwnerCommand(ctx, r.hub)
}

type eventHubReadModelGoalReader struct {
	hub events.Hub
}

func (r eventHubReadModelGoalReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	return listGoalsByWorkspaceOwnerCommand(ctx, r.hub, workspaceID)
}

type eventHubReadModelDoDReader struct {
	hub events.Hub
}

func (r eventHubReadModelDoDReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	return listDoDsByWorkspaceOwnerCommand(ctx, r.hub, workspaceID)
}

type eventHubReadModelRunReader struct {
	hub events.Hub
}

func (r eventHubReadModelRunReader) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	return readRunByOwnerCommand(ctx, r.hub, id)
}

func (r eventHubReadModelRunReader) List(ctx context.Context) ([]*runmodel.Run, error) {
	return listRunsByOwnerCommand(ctx, r.hub)
}

func (r eventHubReadModelRunReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	return listRunsByWorkspaceOwnerCommand(ctx, r.hub, workspaceID)
}

type eventHubReadModelResultReader struct {
	hub events.Hub
}

func (r eventHubReadModelResultReader) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	return listResultsByWorkspaceOwnerCommand(ctx, r.hub, workspaceID)
}

func (r eventHubReadModelResultReader) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	return listResultsByRunOwnerCommand(ctx, r.hub, runID)
}

type eventHubReadModelStepReader struct {
	hub events.Hub
}

func (r eventHubReadModelStepReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	return listStepsByRunOwnerCommand(ctx, r.hub, runID)
}

type eventHubReadModelRuntimeCheckpointReader struct {
	hub events.Hub
}

func (r eventHubReadModelRuntimeCheckpointReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.RuntimeCheckpoint, error) {
	return listRuntimeCheckpointsByRunOwnerCommand(ctx, r.hub, runID)
}

type eventHubReadModelArtifactReader struct {
	hub events.Hub
}

func (r eventHubReadModelArtifactReader) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	return listArtifactsByWorkspaceOwnerCommand(ctx, r.hub, workspaceID)
}

func (r eventHubReadModelArtifactReader) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	return listArtifactsByRunOwnerCommand(ctx, r.hub, runID)
}

func (r eventHubReadModelArtifactReader) ListByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	return listArtifactsByResultOwnerCommand(ctx, r.hub, resultID)
}

type eventHubReadModelArtifactContinuationReader struct {
	hub events.Hub
}

func (r eventHubReadModelArtifactContinuationReader) ListByRun(ctx context.Context, runID common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	return listArtifactContinuationsByRunOwnerCommand(ctx, r.hub, runID)
}

type eventHubReadModelEventRecordReader struct {
	hub events.Hub
}

func (r eventHubReadModelEventRecordReader) List(ctx context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	return listEventRecordsByOwnerCommand(ctx, r.hub, query)
}

type eventHubReadModelMemoryRecallTraceReader struct {
	hub events.Hub
}

func (r eventHubReadModelMemoryRecallTraceReader) List(ctx context.Context, query repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, r.hub, events.TopicMemoryRecallTraceCommandList, "memory_recall_trace", "list", query.WorkspaceID, query.RunID, map[string]any{
		events.PayloadKeyMemoryRecallTraceQuery: query,
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryRecallTraces].([]*memorymodel.MemoryRecallTrace)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

type eventHubReadModelToolCallReader struct {
	hub events.Hub
}

func (r eventHubReadModelToolCallReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.ToolCall, error) {
	return listToolCallsByRunOwnerCommand(ctx, r.hub, runID)
}

type eventHubReadModelPatchReader struct {
	hub events.Hub
}

func (r eventHubReadModelPatchReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Patch, error) {
	return listPatchesByRunOwnerCommand(ctx, r.hub, runID)
}

type eventHubReadModelValidationResultReader struct {
	hub events.Hub
}

func (r eventHubReadModelValidationResultReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.ValidationResult, error) {
	return listValidationResultsByRunOwnerCommand(ctx, r.hub, runID)
}

type eventHubReadModelMCPCapabilityReader struct {
	hub events.Hub
}

func (r eventHubReadModelMCPCapabilityReader) List(ctx context.Context) ([]*mcpmodel.Capability, error) {
	if r.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, r.hub, events.TopicMCPCommandListCapabilities, "mcp_capability", "all", "", "", map[string]any{})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyCapabilities].([]*mcpmodel.Capability)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

type eventHubReadModelAutoGovernanceTraceReader struct {
	hub events.Hub
}

func (r eventHubReadModelAutoGovernanceTraceReader) List(ctx context.Context, query repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	return listAutoGovernanceTracesByOwnerCommand(ctx, r.hub, query)
}

type eventHubReadModelDecisionTicketReader struct {
	hub events.Hub
}

func (r eventHubReadModelDecisionTicketReader) Get(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	return readDecisionTicketByOwnerCommand(ctx, r.hub, ticketID)
}

func (r eventHubReadModelDecisionTicketReader) List(ctx context.Context) ([]*decisionmodel.Ticket, error) {
	return listDecisionTicketsByOwnerCommand(ctx, r.hub)
}

func (r eventHubReadModelDecisionTicketReader) ListByRunID(ctx context.Context, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	return listDecisionTicketsByRunOwnerCommand(ctx, r.hub, runID)
}

func (r eventHubReadModelDecisionTicketReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	return listDecisionTicketsByWorkspaceOwnerCommand(ctx, r.hub, workspaceID)
}

type eventHubReadModelDecisionResolutionReader struct {
	hub events.Hub
}

func (r eventHubReadModelDecisionResolutionReader) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	return readDecisionResolutionByTicketOwnerCommand(ctx, r.hub, ticketID)
}

type eventHubReadModelApprovalReader struct {
	hub events.Hub
}

func (r eventHubReadModelApprovalReader) Get(ctx context.Context, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return readApprovalByOwnerCommand(ctx, r.hub, approvalID)
}

func (r eventHubReadModelApprovalReader) List(ctx context.Context) ([]*approvalmodel.Approval, error) {
	return listApprovalsByOwnerCommand(ctx, r.hub)
}

func (r eventHubReadModelApprovalReader) ListByRunID(ctx context.Context, runID common.RunID) ([]*approvalmodel.Approval, error) {
	return listApprovalsByRunOwnerCommand(ctx, r.hub, runID)
}

func (r eventHubReadModelApprovalReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	return listApprovalsByWorkspaceOwnerCommand(ctx, r.hub, workspaceID)
}
