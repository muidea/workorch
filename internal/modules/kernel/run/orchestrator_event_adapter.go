package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runbiz "github.com/muidea/workorch/internal/modules/kernel/run/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type eventOrchestrator struct {
	hub events.Hub
}

func NewEventOrchestrator(hub events.Hub) runbiz.Orchestrator {
	return eventOrchestrator{hub: hub}
}

func (o eventOrchestrator) StartRun(ctx context.Context, runID common.RunID) error {
	return o.publish(ctx, orchestratorevents.TopicOrchestratorCommandStartRun, runID, orchestratorevents.StartRunCommand{RunID: runID})
}

func (o eventOrchestrator) BlockOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	return o.publish(ctx, orchestratorevents.TopicOrchestratorCommandBlockDecision, runID, orchestratorevents.BlockDecisionCommand{RunID: runID, TicketID: ticketID})
}

func (o eventOrchestrator) BlockOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	return o.publish(ctx, orchestratorevents.TopicOrchestratorCommandBlockApproval, runID, orchestratorevents.BlockApprovalCommand{RunID: runID, ApprovalID: approvalID})
}

func (o eventOrchestrator) ResumeRun(ctx context.Context, runID common.RunID) error {
	return o.publish(ctx, orchestratorevents.TopicOrchestratorCommandResumeRun, runID, orchestratorevents.ResumeRunCommand{RunID: runID, WorkspaceID: common.RuntimeWorkspaceIDFromContext(ctx)})
}

func (o eventOrchestrator) ResumeRunWithExecutionContextPatch(ctx context.Context, runID common.RunID, patch map[string]any) error {
	return o.publish(ctx, orchestratorevents.TopicOrchestratorCommandResumeRun, runID, orchestratorevents.ResumeRunCommand{RunID: runID, WorkspaceID: common.RuntimeWorkspaceIDFromContext(ctx), ExecutionContextPatch: cloneMap(patch)})
}

func (o eventOrchestrator) ValidateRun(ctx context.Context, runID common.RunID) error {
	return o.publish(ctx, orchestratorevents.TopicOrchestratorCommandValidateRun, runID, orchestratorevents.ValidateRunCommand{RunID: runID})
}

func (o eventOrchestrator) CompleteRun(ctx context.Context, runID common.RunID) error {
	return o.publish(ctx, orchestratorevents.TopicOrchestratorCommandCompleteRun, runID, orchestratorevents.CompleteRunCommand{RunID: runID})
}

func (o eventOrchestrator) CancelRun(ctx context.Context, runID common.RunID) error {
	return o.publish(ctx, orchestratorevents.TopicOrchestratorCommandCancelRun, runID, orchestratorevents.CancelRunCommand{RunID: runID})
}

func (o eventOrchestrator) SupersedeRun(ctx context.Context, runID common.RunID) error {
	return o.publish(ctx, orchestratorevents.TopicOrchestratorCommandSupersedeRun, runID, orchestratorevents.SupersedeRunCommand{RunID: runID})
}

func (o eventOrchestrator) FailRun(ctx context.Context, runID common.RunID, reason string) error {
	return o.publish(ctx, orchestratorevents.TopicOrchestratorCommandFailRun, runID, orchestratorevents.FailRunCommand{RunID: runID, Reason: reason})
}

type rawMagicHubProvider interface {
	RawHub() magicevent.Hub
}

func (o eventOrchestrator) publish(ctx context.Context, topic string, runID common.RunID, command any) error {
	if o.hub == nil || runID == "" || topic == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	provider, ok := o.hub.(rawMagicHubProvider)
	if !ok || provider.RawHub() == nil {
		return common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(topic, ModuleID, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(events.EventHubLaneKey(events.Envelope{
		Topic:       topic,
		SubjectType: "run",
		SubjectID:   string(runID),
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))
	result := provider.RawHub().Send(ev)
	if result == nil {
		return common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return err
	}
	_, err := result.Get()
	return err
}
