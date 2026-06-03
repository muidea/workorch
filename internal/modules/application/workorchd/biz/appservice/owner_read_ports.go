package appservice

import (
	"context"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func readWorkspaceByOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandGet, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyWorkspace].(*workspacemodel.Workspace)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func listWorkspacesByOwnerCommand(ctx context.Context, hub events.Hub) ([]*workspacemodel.Workspace, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandList, "workspace", "all", "", "", map[string]any{})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyWorkspaces].([]*workspacemodel.Workspace)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func readSoulByOwnerCommand(ctx context.Context, hub events.Hub, soulID common.SoulID) (*soulmodel.Soul, error) {
	if hub == nil || soulID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicSoulCommandGet, "soul", string(soulID), "", "", map[string]any{
		events.PayloadKeySoulID: string(soulID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeySoul].(*soulmodel.Soul)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func readGoalByOwnerCommand(ctx context.Context, hub events.Hub, goalID common.GoalID) (*workspacemodel.Goal, error) {
	if hub == nil || goalID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandGetGoal, "goal", string(goalID), "", "", map[string]any{
		events.PayloadKeyGoalID: string(goalID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyGoal].(*workspacemodel.Goal)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func listGoalsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandListGoalsByWS, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyGoals].([]*workspacemodel.Goal)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func readDoDByOwnerCommand(ctx context.Context, hub events.Hub, dodID common.DoDID) (*workspacemodel.DoD, error) {
	if hub == nil || dodID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandGetDoD, "dod", string(dodID), "", "", map[string]any{
		events.PayloadKeyDoDID: string(dodID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyDoD].(*workspacemodel.DoD)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func listDoDsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicWorkspaceCommandListDoDsByWS, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyDoDs].([]*workspacemodel.DoD)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func readRunByOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) (*runmodel.Run, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyRunID: string(runID),
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicRunCommandGet, "run", string(runID), workspaceID, runID, payload)
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyRun].(*runmodel.Run)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func listRunsByOwnerCommand(ctx context.Context, hub events.Hub) ([]*runmodel.Run, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicRunCommandList, "run", "all", "", "", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyRuns].([]*runmodel.Run)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listRunsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyRuns].([]*runmodel.Run)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func readStepByOwnerCommand(ctx context.Context, hub events.Hub, stepID common.StepID) (*runmodel.Step, error) {
	if hub == nil || stepID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicRunCommandGetStep, "step", string(stepID), "", "", map[string]any{
		events.PayloadKeyStepID: string(stepID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyStep].(*runmodel.Step)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func listStepsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.Step, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicRunCommandListStepsByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeySteps].([]*runmodel.Step)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func readRuntimeCheckpointByOwnerCommand(ctx context.Context, hub events.Hub, checkpointID common.CheckpointID) (*runmodel.RuntimeCheckpoint, error) {
	if hub == nil || checkpointID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicRunCommandGetRuntimeCheckpoint, "runtime_checkpoint", string(checkpointID), "", "", map[string]any{
		events.PayloadKeyCheckpointID: string(checkpointID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyCheckpoint].(*runmodel.RuntimeCheckpoint)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func listRuntimeCheckpointsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.RuntimeCheckpoint, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicRunCommandListCheckpointsByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyCheckpoints].([]*runmodel.RuntimeCheckpoint)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func readSkillByOwnerCommand(ctx context.Context, hub events.Hub, skillID common.SkillID) (*skillmodel.Skill, error) {
	if hub == nil || skillID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicSkillCommandGet, "skill", string(skillID), "", "", map[string]any{
		events.PayloadKeySkillID: string(skillID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeySkill].(*skillmodel.Skill)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func listSkillsByOwnerCommand(ctx context.Context, hub events.Hub) ([]*skillmodel.Skill, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicSkillCommandList, "skill", "all", "", "", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeySkills].([]*skillmodel.Skill)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func readResultByOwnerCommand(ctx context.Context, hub events.Hub, resultID common.ResultID) (*resultmodel.Result, error) {
	if hub == nil || resultID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicResultCommandGet, "result", string(resultID), "", "", map[string]any{
		events.PayloadKeyResultID: string(resultID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyResult].(*resultmodel.Result)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func listResultsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicResultCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Result)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listResultsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*resultmodel.Result, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicResultCommandListByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Result)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listArtifactsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicArtifactCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Artifact)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listArtifactsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*resultmodel.Artifact, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicArtifactCommandListByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Artifact)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listArtifactsByResultOwnerCommand(ctx context.Context, hub events.Hub, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	if hub == nil || resultID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicArtifactCommandListByResult, "result", string(resultID), "", "", map[string]any{
		events.PayloadKeyResultID: string(resultID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*resultmodel.Artifact)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listArtifactContinuationsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicOrchestratorCommandListArtifactContinuationsByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*orchestratormodel.ArtifactContinuation)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listEventRecordsByOwnerCommand(ctx context.Context, hub events.Hub, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicEventStreamCommandList, "event_record", "list", query.WorkspaceID, query.RunID, map[string]any{
		events.PayloadKeyQuery: query,
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*eventstreammodel.EventRecord)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listToolCallsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.ToolCall, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicOrchestratorCommandListToolCallsByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*runmodel.ToolCall)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listPatchesByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.Patch, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicOrchestratorCommandListPatchesByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*runmodel.Patch)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listValidationResultsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.ValidationResult, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicOrchestratorCommandListValidationResultsByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*runmodel.ValidationResult)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listAutoGovernanceTracesByOwnerCommand(ctx context.Context, hub events.Hub, query repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicGovernanceAutoCommandListTraces, "auto_governance_trace", "list", query.WorkspaceID, query.RunID, map[string]any{
		events.PayloadKeyQuery: query,
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*governancemodel.AutoGovernanceTrace)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listDecisionTicketsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicDecisionCommandListTicketsByWS, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func readDecisionTicketByOwnerCommand(ctx context.Context, hub events.Hub, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	if hub == nil || ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyBlockingTicketID: string(ticketID),
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicDecisionCommandGetTicket, "decision_ticket", string(ticketID), workspaceID, "", payload)
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyTicket].(*decisionmodel.Ticket)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func listDecisionTicketsByOwnerCommand(ctx context.Context, hub events.Hub) ([]*decisionmodel.Ticket, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicDecisionCommandListTickets, "decision_ticket", "all", "", "", map[string]any{})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listDecisionTicketsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyRunID: string(runID),
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicDecisionCommandListTicketsByRun, "run", string(runID), workspaceID, runID, payload)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func readDecisionResolutionByTicketOwnerCommand(ctx context.Context, hub events.Hub, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	if hub == nil || ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicDecisionCommandGetResolution, "ticket", string(ticketID), "", "", map[string]any{
		events.PayloadKeyBlockingTicketID: string(ticketID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyResolution].(*decisionmodel.Resolution)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func readApprovalByOwnerCommand(ctx context.Context, hub events.Hub, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	if hub == nil || approvalID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyApprovalID: string(approvalID),
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicApprovalCommandGet, "approval", string(approvalID), workspaceID, "", payload)
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyApproval].(*approvalmodel.Approval)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func listApprovalsByOwnerCommand(ctx context.Context, hub events.Hub) ([]*approvalmodel.Approval, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicApprovalCommandList, "approval", "all", "", "", map[string]any{})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*approvalmodel.Approval)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listApprovalsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*approvalmodel.Approval, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	payload := map[string]any{
		events.PayloadKeyRunID: string(runID),
	}
	if workspaceID != "" {
		payload[events.PayloadKeyWorkspaceID] = string(workspaceID)
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicApprovalCommandListByRun, "run", string(runID), workspaceID, runID, payload)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*approvalmodel.Approval)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

func listApprovalsByWorkspaceOwnerCommand(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicApprovalCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*approvalmodel.Approval)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}
