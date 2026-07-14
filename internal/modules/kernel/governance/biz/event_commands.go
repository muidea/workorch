package governance

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func eventResolveDecision(hub magicevent.Hub) ResolveDecisionFunc {
	return func(ctx context.Context, ticketID common.TicketID, resolutionType string, payload workorcheventpayload.Payload) error {
		if !decisioncommon.IsSupportedResolutionType(resolutionType) {
			resolutionType = decisioncommon.ResolutionTypeProvideMissingInfo
		}
		workspaceID := stringFromPayload(payload, events.PayloadKeyWorkspaceID)
		runID := stringFromPayload(payload, events.PayloadKeyRunID)
		stepID := stringFromPayload(payload, events.PayloadKeyStepID)
		if hub == nil {
			return common.ErrInvalidArgument
		}
		command := decisionevents.EnsureResolvedCommand{
			ResolutionID: common.ResolutionID("resolution-" + string(ticketID) + "-auto-governance"),
			TicketID:     ticketID,
			WorkspaceID:  common.WorkspaceID(workspaceID),
			RunID:        common.RunID(runID),
			StepID:       common.StepID(stepID),
			Type:         resolutionType,
			Payload:      payload.Clone().Fields,
		}
		event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandEnsureResolved, governancecommon.ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		bindOwnerLane(event, decisionevents.TopicDecisionCommandEnsureResolved, "decision_ticket", string(ticketID), common.WorkspaceID(workspaceID), common.RunID(runID), common.StepID(stepID))
		_, err := decisionResponse(hub.Send(event), decisionevents.TopicDecisionCommandEnsureResolved)
		return err
	}
}

func eventResolveApproval(hub magicevent.Hub) ResolveApprovalFunc {
	return func(ctx context.Context, approvalID common.ApprovalID) error {
		workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
		if hub == nil {
			return common.ErrInvalidArgument
		}
		command := approvalevents.EnsureApprovedCommand{
			ApprovalID:  approvalID,
			WorkspaceID: workspaceID,
		}
		event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandEnsureApproved, governancecommon.ModuleID, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		bindOwnerLane(event, approvalevents.TopicApprovalCommandEnsureApproved, "approval", string(approvalID), workspaceID, "", "")
		_, err := approvalResponse(hub.Send(event), approvalevents.TopicApprovalCommandEnsureApproved)
		return err
	}
}

func eventResumeRun(hub magicevent.Hub) ResumeRunFunc {
	return func(ctx context.Context, runID common.RunID) error {
		workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
		if hub == nil {
			return common.ErrInvalidArgument
		}
		command := orchestratorevents.ResumeRunCommand{
			WorkspaceID: workspaceID,
			RunID:       runID,
		}
		event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandResumeRun, governancecommon.ModuleID, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		bindOwnerLane(event, orchestratorevents.TopicOrchestratorCommandResumeRun, "run", string(runID), workspaceID, runID, "")
		_, err := orchestratorResponse(hub.Send(event), orchestratorevents.TopicOrchestratorCommandResumeRun)
		return err
	}
}

func stringFromPayload(payload workorcheventpayload.Payload, key string) string {
	value, _ := payload.String(key)
	return value
}
