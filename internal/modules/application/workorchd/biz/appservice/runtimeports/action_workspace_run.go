package runtimeports

import (
	"context"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (p *EventHubActionPort) EnsureWorkspace(ctx context.Context, workspaceID common.WorkspaceID, title string) error {
	if p == nil || p.hub == nil || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, workspaceevents.TopicWorkspaceCommandEnsure, "workspace", string(workspaceID), workspaceID, "", "", map[string]any{
		events.PayloadKeyCommand: workspacecontracts.EnsureWorkspaceCommand{
			WorkspaceID:          workspaceID,
			Title:                title,
			DeliverableDirectory: "workspaces/" + string(workspaceID),
		},
	})
}

func (p *EventHubActionPort) EnsureRun(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	if p == nil || p.hub == nil || runID == "" || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, runevents.TopicRunCommandEnsure, "run", string(runID), workspaceID, runID, "", map[string]any{
		events.PayloadKeyCommand: runcontracts.CreateRunCommand{RunID: runID, WorkspaceID: workspaceID},
	})
}

func (p *EventHubActionPort) EnsureRunStarted(ctx context.Context, runID common.RunID) error {
	if p == nil || p.hub == nil || runID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, orchestratorevents.TopicOrchestratorCommandStartRun, "run", string(runID), workspaceID, runID, "", map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
}

func (p *EventHubActionPort) EnsureRunBlockedOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	if p == nil || p.hub == nil || runID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, orchestratorevents.TopicOrchestratorCommandBlockDecision, "run", string(runID), workspaceID, runID, "", map[string]any{
		events.PayloadKeyCommand: runcontracts.BlockOnDecisionCommand{RunID: runID, TicketID: ticketID},
	})
}

func (p *EventHubActionPort) EnsureRunBlockedOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if p == nil || p.hub == nil || runID == "" || approvalID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, orchestratorevents.TopicOrchestratorCommandBlockApproval, "run", string(runID), workspaceID, runID, "", map[string]any{
		events.PayloadKeyCommand: runcontracts.BlockOnApprovalCommand{RunID: runID, ApprovalID: approvalID},
	})
}

func (p *EventHubActionPort) EnsureRunResumed(ctx context.Context, runID common.RunID) error {
	if p == nil || p.hub == nil || runID == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, orchestratorevents.TopicOrchestratorCommandResumeRun, "run", string(runID), workspaceID, runID, "", map[string]any{
		events.PayloadKeyCommand: runcontracts.ResumeRunCommand{WorkspaceID: workspaceID, RunID: runID},
	})
}
