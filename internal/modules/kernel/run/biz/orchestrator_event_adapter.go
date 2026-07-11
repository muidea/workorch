package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/run/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type eventOrchestrator struct {
	hub magicevent.Hub
}

func (o eventOrchestrator) StartRun(ctx context.Context, runID common.RunID) error {
	return publishOrchestratorCommand(ctx, o.hub, orchestratorevents.TopicOrchestratorCommandStartRun, runID, orchestratorevents.StartRunCommand{RunID: runID})
}

func (o eventOrchestrator) BlockOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	return publishOrchestratorCommand(ctx, o.hub, orchestratorevents.TopicOrchestratorCommandBlockDecision, runID, orchestratorevents.BlockDecisionCommand{RunID: runID, TicketID: ticketID})
}

func (o eventOrchestrator) BlockOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	return publishOrchestratorCommand(ctx, o.hub, orchestratorevents.TopicOrchestratorCommandBlockApproval, runID, orchestratorevents.BlockApprovalCommand{RunID: runID, ApprovalID: approvalID})
}

func (o eventOrchestrator) ResumeRun(ctx context.Context, runID common.RunID) error {
	return publishOrchestratorCommand(ctx, o.hub, orchestratorevents.TopicOrchestratorCommandResumeRun, runID, orchestratorevents.ResumeRunCommand{RunID: runID, WorkspaceID: common.RuntimeWorkspaceIDFromContext(ctx)})
}

func (o eventOrchestrator) ResumeRunWithExecutionContextPatch(ctx context.Context, runID common.RunID, patch map[string]any) error {
	return publishOrchestratorCommand(ctx, o.hub, orchestratorevents.TopicOrchestratorCommandResumeRun, runID, orchestratorevents.ResumeRunCommand{RunID: runID, WorkspaceID: common.RuntimeWorkspaceIDFromContext(ctx), ExecutionContextPatch: cloneExecutionContext(patch)})
}

func (o eventOrchestrator) ValidateRun(ctx context.Context, runID common.RunID) error {
	return publishOrchestratorCommand(ctx, o.hub, orchestratorevents.TopicOrchestratorCommandValidateRun, runID, orchestratorevents.ValidateRunCommand{RunID: runID})
}

func (o eventOrchestrator) CompleteRun(ctx context.Context, runID common.RunID) error {
	return publishOrchestratorCommand(ctx, o.hub, orchestratorevents.TopicOrchestratorCommandCompleteRun, runID, orchestratorevents.CompleteRunCommand{RunID: runID})
}

func (o eventOrchestrator) CancelRun(ctx context.Context, runID common.RunID) error {
	return publishOrchestratorCommand(ctx, o.hub, orchestratorevents.TopicOrchestratorCommandCancelRun, runID, orchestratorevents.CancelRunCommand{RunID: runID})
}

func (o eventOrchestrator) SupersedeRun(ctx context.Context, runID common.RunID) error {
	return publishOrchestratorCommand(ctx, o.hub, orchestratorevents.TopicOrchestratorCommandSupersedeRun, runID, orchestratorevents.SupersedeRunCommand{RunID: runID})
}

func (o eventOrchestrator) FailRun(ctx context.Context, runID common.RunID, reason string) error {
	return publishOrchestratorCommand(ctx, o.hub, orchestratorevents.TopicOrchestratorCommandFailRun, runID, orchestratorevents.FailRunCommand{RunID: runID, Reason: reason})
}

func publishOrchestratorCommand[Command any](ctx context.Context, hub magicevent.Hub, topic string, runID common.RunID, command Command) error {
	if hub == nil || runID == "" || topic == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	ev := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{
		SubjectType: "run",
		SubjectID:   string(runID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))
	result := hub.Send(ev)
	if result == nil {
		return common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return err
	}
	_, err := result.Get()
	return err
}
