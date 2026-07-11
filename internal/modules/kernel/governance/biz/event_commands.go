package governance

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func eventResolveDecision(hub magicevent.Hub) ResolveDecisionFunc {
	return func(ctx context.Context, ticketID common.TicketID, resolutionType string, payload map[string]any) error {
		if !decisioncommon.IsSupportedResolutionType(resolutionType) {
			resolutionType = decisioncommon.ResolutionTypeProvideMissingInfo
		}
		workspaceID := stringFromPayload(payload, events.PayloadKeyWorkspaceID)
		runID := stringFromPayload(payload, events.PayloadKeyRunID)
		stepID := stringFromPayload(payload, events.PayloadKeyStepID)
		_, err := sendOwnerMagicCommand[decisionevents.EnsureResolvedCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.CommandDestination, "decision_ticket", string(ticketID), common.WorkspaceID(workspaceID), common.RunID(runID), common.StepID(stepID), decisionevents.EnsureResolvedCommand{
			ResolutionID: common.ResolutionID("resolution-" + string(ticketID) + "-auto-governance"),
			TicketID:     ticketID,
			WorkspaceID:  common.WorkspaceID(workspaceID),
			RunID:        common.RunID(runID),
			StepID:       common.StepID(stepID),
			Type:         resolutionType,
			Payload:      payload,
		})
		return err
	}
}

func eventResolveApproval(hub magicevent.Hub) ResolveApprovalFunc {
	return func(ctx context.Context, approvalID common.ApprovalID) error {
		workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
		_, err := sendOwnerMagicCommand[approvalevents.EnsureApprovedCommand, approvalevents.ApprovalCommandResult](ctx, hub, approvalevents.TopicApprovalCommandEnsureApproved, approvalevents.CommandDestination, "approval", string(approvalID), workspaceID, "", "", approvalevents.EnsureApprovedCommand{
			ApprovalID:  approvalID,
			WorkspaceID: workspaceID,
		})
		return err
	}
}

func eventResumeRun(hub magicevent.Hub) ResumeRunFunc {
	return func(ctx context.Context, runID common.RunID) error {
		workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
		_, err := sendOwnerMagicCommand[orchestratorevents.ResumeRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandResumeRun, orchestratorevents.CommandDestination, "run", string(runID), workspaceID, runID, "", orchestratorevents.ResumeRunCommand{
			WorkspaceID: workspaceID,
			RunID:       runID,
		})
		return err
	}
}

func stringFromPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}
