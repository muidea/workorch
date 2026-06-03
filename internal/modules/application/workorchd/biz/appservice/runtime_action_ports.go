package appservice

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type eventHubActionPort struct {
	hub events.Hub
}

func newEventHubActionPort(hub events.Hub) *eventHubActionPort {
	return &eventHubActionPort{hub: hub}
}

func (p *eventHubActionPort) EnsureWorkspace(ctx context.Context, workspaceID common.WorkspaceID, title string) error {
	if p == nil || p.hub == nil || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, events.TopicWorkspaceCommandEnsure, "workspace", string(workspaceID), workspaceID, "", "", map[string]any{
		events.PayloadKeyWorkspaceID:          string(workspaceID),
		events.PayloadKeyTitle:                title,
		events.PayloadKeyDeliverableDirectory: "workspaces/" + string(workspaceID),
	})
}

func (p *eventHubActionPort) EnsureRun(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	if p == nil || p.hub == nil || runID == "" || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, events.TopicRunCommandEnsure, "run", string(runID), workspaceID, runID, "", map[string]any{
		events.PayloadKeyRunID:       string(runID),
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
}

func (p *eventHubActionPort) EnsureRunStarted(ctx context.Context, runID common.RunID) error {
	if p == nil || p.hub == nil || runID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, events.TopicRunCommandStart, "run", string(runID), "", runID, "", map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
}

func (p *eventHubActionPort) EnsureRunBlockedOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	if p == nil || p.hub == nil || runID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, events.TopicOrchestratorCommandBlockDecision, "run", string(runID), "", runID, "", map[string]any{
		events.PayloadKeyRunID:            string(runID),
		events.PayloadKeyBlockingTicketID: string(ticketID),
	})
}

func (p *eventHubActionPort) EnsureRunBlockedOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if p == nil || p.hub == nil || runID == "" || approvalID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, events.TopicOrchestratorCommandBlockApproval, "run", string(runID), "", runID, "", map[string]any{
		events.PayloadKeyRunID:              string(runID),
		events.PayloadKeyBlockingApprovalID: string(approvalID),
	})
}

func (p *eventHubActionPort) EnsureRunResumed(ctx context.Context, runID common.RunID) error {
	if p == nil || p.hub == nil || runID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, events.TopicRunCommandEnsureResumed, "run", string(runID), "", runID, "", map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
}

func (p *eventHubActionPort) EnsureTicket(
	ctx context.Context,
	ticketID common.TicketID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	if p == nil || p.hub == nil || ticketID == "" || workspaceID == "" || runID == "" || stepID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, events.TopicDecisionCommandEnsureTicket, "decision_ticket", string(ticketID), workspaceID, runID, stepID, map[string]any{
		events.PayloadKeyBlockingTicketID: string(ticketID),
		events.PayloadKeyWorkspaceID:      string(workspaceID),
		events.PayloadKeyRunID:            string(runID),
		events.PayloadKeyStepID:           string(stepID),
		events.PayloadKeyReason:           reason,
	})
}

func (p *eventHubActionPort) EnsureResolvedTicket(
	ctx context.Context,
	resolutionID common.ResolutionID,
	ticketID common.TicketID,
	resolutionType string,
	payload map[string]any,
) error {
	if p == nil || p.hub == nil || resolutionID == "" || ticketID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, events.TopicDecisionCommandEnsureResolved, "decision_ticket", string(ticketID), "", "", "", map[string]any{
		events.PayloadKeyResolutionID:     string(resolutionID),
		events.PayloadKeyBlockingTicketID: string(ticketID),
		events.PayloadKeyType:             resolutionType,
		events.PayloadKeyPayload:          cloneActionPayload(payload),
	})
}

func (p *eventHubActionPort) EnsureApproval(
	ctx context.Context,
	approvalID common.ApprovalID,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	stepID common.StepID,
	reason string,
) error {
	if p == nil || p.hub == nil || approvalID == "" || workspaceID == "" || runID == "" || stepID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, events.TopicApprovalCommandEnsure, "approval", string(approvalID), workspaceID, runID, stepID, map[string]any{
		events.PayloadKeyApprovalID:  string(approvalID),
		events.PayloadKeyWorkspaceID: string(workspaceID),
		events.PayloadKeyRunID:       string(runID),
		events.PayloadKeyStepID:      string(stepID),
		events.PayloadKeyReason:      reason,
	})
}

func (p *eventHubActionPort) EnsureApprovalApproved(ctx context.Context, approvalID common.ApprovalID) error {
	if p == nil || p.hub == nil || approvalID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, events.TopicApprovalCommandEnsureApproved, "approval", string(approvalID), "", "", "", map[string]any{
		events.PayloadKeyApprovalID: string(approvalID),
	})
}

func (p *eventHubActionPort) EnsureApprovalRejected(ctx context.Context, approvalID common.ApprovalID) error {
	if p == nil || p.hub == nil || approvalID == "" {
		return common.ErrInvalidArgument
	}
	return p.publish(ctx, events.TopicApprovalCommandEnsureRejected, "approval", string(approvalID), "", "", "", map[string]any{
		events.PayloadKeyApprovalID: string(approvalID),
	})
}

func (p *eventHubActionPort) publish(ctx context.Context, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, payload map[string]any) error {
	if p == nil || p.hub == nil || topic == "" || subjectID == "" {
		return common.ErrInvalidArgument
	}
	err := p.hub.Publish(ctx, events.Envelope{
		Topic:        topic,
		SourceModule: "workorchd/appservice/runtime-actions",
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		StepID:       string(stepID),
		Payload:      payload,
	})
	if _, ok := events.ResponseEnvelope(err); ok {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("runtime action command %s returned no response", topic)
}

func cloneActionPayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
