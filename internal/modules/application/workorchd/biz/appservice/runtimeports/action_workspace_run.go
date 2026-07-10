package runtimeports

import (
	"context"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
)

func (p *EventHubActionPort) EnsureWorkspace(ctx context.Context, workspaceID common.WorkspaceID, title string) error {
	if p == nil || p.hub == nil || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.send(ctx, workspaceevents.TopicWorkspaceCommandEnsure, workspaceevents.CommandDestination, "workspace", string(workspaceID), workspaceID, "", "", workspaceevents.EnsureWorkspaceCommand{
		WorkspaceID:          workspaceID,
		Title:                title,
		DeliverableDirectory: "workspaces/" + string(workspaceID),
	})
}

func (p *EventHubActionPort) EnsureRun(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	if p == nil || p.hub == nil || runID == "" || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.send(ctx, runevents.TopicRunCommandEnsure, runevents.CommandDestination, "run", string(runID), workspaceID, runID, "", runevents.EnsureRunCommand{RunID: runID, WorkspaceID: workspaceID})
}

func (p *EventHubActionPort) EnsureRunStarted(ctx context.Context, runID common.RunID) error {
	if p == nil || p.hub == nil || runID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.send(ctx, orchestratorevents.TopicOrchestratorCommandStartRun, orchestratorevents.CommandDestination, "run", string(runID), workspaceID, runID, "", orchestratorevents.StartRunCommand{RunID: runID})
}

func (p *EventHubActionPort) EnsureRunBlockedOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	if p == nil || p.hub == nil || runID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.send(ctx, orchestratorevents.TopicOrchestratorCommandBlockDecision, orchestratorevents.CommandDestination, "run", string(runID), workspaceID, runID, "", orchestratorevents.BlockDecisionCommand{RunID: runID, TicketID: ticketID})
}

func (p *EventHubActionPort) EnsureRunBlockedOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if p == nil || p.hub == nil || runID == "" || approvalID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.send(ctx, orchestratorevents.TopicOrchestratorCommandBlockApproval, orchestratorevents.CommandDestination, "run", string(runID), workspaceID, runID, "", orchestratorevents.BlockApprovalCommand{RunID: runID, ApprovalID: approvalID})
}

func (p *EventHubActionPort) EnsureRunResumed(ctx context.Context, runID common.RunID) error {
	if p == nil || p.hub == nil || runID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.send(ctx, orchestratorevents.TopicOrchestratorCommandResumeRun, orchestratorevents.CommandDestination, "run", string(runID), workspaceID, runID, "", orchestratorevents.ResumeRunCommand{WorkspaceID: workspaceID, RunID: runID})
}
