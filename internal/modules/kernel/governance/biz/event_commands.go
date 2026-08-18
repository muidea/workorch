package governance

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const (
	resolutionPayloadWorkspaceID = "workspaceId"
	resolutionPayloadRunID       = "runId"
	resolutionPayloadStepID      = "stepId"
)

func eventResolveDecision(send func(magicevent.Event) magicevent.Result) ResolveDecisionFunc {
	return func(ctx context.Context, ticketID common.TicketID, resolutionType string, payload workorcheventpayload.Payload) error {
		if !decisioncommon.IsSupportedResolutionType(resolutionType) {
			resolutionType = decisioncommon.ResolutionTypeProvideMissingInfo
		}
		workspaceID := stringFromPayload(payload, resolutionPayloadWorkspaceID)
		runID := stringFromPayload(payload, resolutionPayloadRunID)
		stepID := stringFromPayload(payload, resolutionPayloadStepID)
		if send == nil {
			return common.ErrInvalidArgument
		}
		command := decisionevents.EnsureResolvedCommand{
			ResolutionID: autoGovernanceResolutionID(common.WorkspaceID(workspaceID), ticketID),
			TicketID:     ticketID,
			WorkspaceID:  common.WorkspaceID(workspaceID),
			RunID:        common.RunID(runID),
			StepID:       common.StepID(stepID),
			Type:         resolutionType,
			Payload:      payload.Clone().Fields,
		}
		event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandEnsureResolved, governancecommon.ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		bindOwnerLane(event, decisionevents.Contracts(), decisionevents.TopicDecisionCommandEnsureResolved, "decision_ticket", string(ticketID), common.WorkspaceID(workspaceID), common.RunID(runID), common.StepID(stepID))
		_, err := decisionResponse(send(event), decisionevents.TopicDecisionCommandEnsureResolved)
		return err
	}
}

func eventResolveApproval(send func(magicevent.Event) magicevent.Result) ResolveApprovalFunc {
	return func(ctx context.Context, approvalID common.ApprovalID) error {
		workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
		if send == nil {
			return common.ErrInvalidArgument
		}
		command := approvalevents.EnsureApprovedCommand{
			ApprovalID:  approvalID,
			WorkspaceID: workspaceID,
			Context: workorcheventpayload.New(
				workorcheventpayload.StringField("resolvedBy", autoGovernanceActor),
				workorcheventpayload.StringField("source", autoGovernanceActor),
			).Fields,
		}
		event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandEnsureApproved, governancecommon.ModuleID, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		bindOwnerLane(event, approvalevents.Contracts(), approvalevents.TopicApprovalCommandEnsureApproved, "approval", string(approvalID), workspaceID, "", "")
		_, err := approvalResponse(send(event), approvalevents.TopicApprovalCommandEnsureApproved)
		return err
	}
}

func stringFromPayload(payload workorcheventpayload.Payload, key string) string {
	value, _ := payload.String(key)
	return value
}
