package run

import (
	"context"
	"fmt"

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
	return o.publish(ctx, events.TopicOrchestratorCommandStartRun, runID, nil)
}

func (o eventOrchestrator) BlockOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	return o.publish(ctx, events.TopicOrchestratorCommandBlockDecision, runID, map[string]any{events.PayloadKeyBlockingTicketID: string(ticketID)})
}

func (o eventOrchestrator) BlockOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	return o.publish(ctx, events.TopicOrchestratorCommandBlockApproval, runID, map[string]any{events.PayloadKeyBlockingApprovalID: string(approvalID)})
}

func (o eventOrchestrator) ResumeRun(ctx context.Context, runID common.RunID) error {
	return o.publish(ctx, events.TopicOrchestratorCommandResumeRun, runID, nil)
}

func (o eventOrchestrator) ResumeRunWithExecutionContextPatch(ctx context.Context, runID common.RunID, patch map[string]any) error {
	return o.publish(ctx, events.TopicOrchestratorCommandResumeRun, runID, map[string]any{events.PayloadKeyExecutionContextPatch: patch})
}

func (o eventOrchestrator) ValidateRun(ctx context.Context, runID common.RunID) error {
	return o.publish(ctx, events.TopicOrchestratorCommandValidateRun, runID, nil)
}

func (o eventOrchestrator) CompleteRun(ctx context.Context, runID common.RunID) error {
	return o.publish(ctx, events.TopicOrchestratorCommandCompleteRun, runID, nil)
}

func (o eventOrchestrator) CancelRun(ctx context.Context, runID common.RunID) error {
	return o.publish(ctx, events.TopicOrchestratorCommandCancelRun, runID, nil)
}

func (o eventOrchestrator) SupersedeRun(ctx context.Context, runID common.RunID) error {
	return o.publish(ctx, events.TopicOrchestratorCommandSupersedeRun, runID, nil)
}

func (o eventOrchestrator) FailRun(ctx context.Context, runID common.RunID, reason string) error {
	return o.publish(ctx, events.TopicOrchestratorCommandFailRun, runID, map[string]any{events.PayloadKeyFailureReason: reason})
}

func (o eventOrchestrator) publish(ctx context.Context, topic string, runID common.RunID, payload map[string]any) error {
	if o.hub == nil || runID == "" || topic == "" {
		return common.ErrInvalidArgument
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload[events.PayloadKeyRunID] = string(runID)
	err := o.hub.Publish(ctx, events.Envelope{
		Topic:        topic,
		SourceModule: ModuleID,
		SubjectType:  "run",
		SubjectID:    string(runID),
		RunID:        string(runID),
		Payload:      payload,
	})
	if _, ok := events.ResponseEnvelope(err); ok {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("orchestrator command %s returned no response", topic)
}
