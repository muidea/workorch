package governance

import (
	"context"
	"fmt"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func eventResolveDecision(hub events.Hub) ResolveDecisionFunc {
	return func(ctx context.Context, ticketID common.TicketID, resolutionType string, payload map[string]any) error {
		if !decisioncommon.IsSupportedResolutionType(resolutionType) {
			resolutionType = decisioncommon.ResolutionTypeProvideMissingInfo
		}
		workspaceID := stringFromPayload(payload, events.PayloadKeyWorkspaceID)
		runID := stringFromPayload(payload, events.PayloadKeyRunID)
		stepID := stringFromPayload(payload, events.PayloadKeyStepID)
		return publishGovernanceCommand(ctx, hub, events.TopicDecisionCommandEnsureResolved, "decision_ticket", string(ticketID), map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
			events.PayloadKeyResolutionID:     "resolution-" + string(ticketID) + "-auto-governance",
			events.PayloadKeyType:             resolutionType,
			events.PayloadKeyPayload:          payload,
			events.PayloadKeyWorkspaceID:      workspaceID,
			events.PayloadKeyRunID:            runID,
			events.PayloadKeyStepID:           stepID,
		}, workspaceID, runID, stepID)
	}
}

func eventResolveApproval(hub events.Hub) ResolveApprovalFunc {
	return func(ctx context.Context, approvalID common.ApprovalID) error {
		workspaceID := string(common.RuntimeWorkspaceIDFromContext(ctx))
		return publishGovernanceCommand(ctx, hub, events.TopicApprovalCommandEnsureApproved, "approval", string(approvalID), map[string]any{
			events.PayloadKeyApprovalID:  string(approvalID),
			events.PayloadKeyWorkspaceID: workspaceID,
		}, workspaceID, "", "")
	}
}

func eventResumeRun(hub events.Hub) ResumeRunFunc {
	return func(ctx context.Context, runID common.RunID) error {
		workspaceID := string(common.RuntimeWorkspaceIDFromContext(ctx))
		return publishGovernanceCommand(ctx, hub, events.TopicRunCommandEnsureResumed, "run", string(runID), map[string]any{
			events.PayloadKeyWorkspaceID: workspaceID,
			events.PayloadKeyRunID:       string(runID),
		}, workspaceID, string(runID), "")
	}
}

func publishGovernanceCommand(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, payload map[string]any, workspaceID string, runID string, stepID string) error {
	if hub == nil || topic == "" || subjectID == "" {
		return common.ErrInvalidArgument
	}
	err := hub.Publish(ctx, events.Envelope{
		Topic:        topic,
		SourceModule: ModuleID,
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		StepID:       stepID,
		Payload:      payload,
	})
	if _, ok := events.ResponseEnvelope(err); ok {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("governance command %s returned no response", topic)
}

func stringFromPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}
