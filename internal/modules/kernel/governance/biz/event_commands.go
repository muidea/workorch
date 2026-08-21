package governance

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
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
		if send == nil || ticketID == "" || workspaceID == "" || runID == "" {
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
		response, err := decisionResponse[decisionevents.EnsureResolvedResult](send(event), decisionevents.TopicDecisionCommandEnsureResolved)
		if err != nil {
			return err
		}
		if response.Ticket == nil || response.Resolution == nil ||
			response.Ticket.ID != command.TicketID || response.Ticket.WorkspaceID != command.WorkspaceID || response.Ticket.RunID != command.RunID ||
			response.Resolution.ID != command.ResolutionID || response.Resolution.TicketID != command.TicketID || response.Resolution.Type != command.Type {
			return fmt.Errorf("%w: Decision ensure-resolved returned mismatched owner identity", common.ErrConflict)
		}
		return nil
	}
}

func eventResolveApproval(send func(magicevent.Event) magicevent.Result) ResolveApprovalFunc {
	return func(ctx context.Context, approvalID common.ApprovalID, runID common.RunID) error {
		workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
		if send == nil || approvalID == "" || workspaceID == "" || runID == "" {
			return common.ErrInvalidArgument
		}
		command := approvalevents.EnsureApprovedCommand{
			ApprovalID:  approvalID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			Principal: approvalmodel.ApprovalResolutionPrincipal{
				Kind: approvalmodel.ApprovalResolutionPrincipalAutoGovernance, SubjectID: governancecommon.ModuleID,
			},
			ResolutionContext: workorcheventpayload.New(workorcheventpayload.StringField("source", autoGovernanceActor)).Fields,
		}
		event := magicevent.NewEventWithContext(approvalevents.TopicApprovalCommandEnsureApproved, governancecommon.ModuleID, approvalevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		bindOwnerLane(event, approvalevents.Contracts(), approvalevents.TopicApprovalCommandEnsureApproved, "approval", string(approvalID), workspaceID, runID, "")
		response, err := approvalResponse[approvalevents.EnsureApprovedResult](send(event), approvalevents.TopicApprovalCommandEnsureApproved)
		if err != nil {
			return err
		}
		if response.Approval == nil || response.Approval.ID != command.ApprovalID || response.Approval.WorkspaceID != command.WorkspaceID || response.Approval.RunID != command.RunID ||
			response.Approval.State != common.ApprovalStateApproved || !approvalResolvedByGovernance(response.Approval) {
			return fmt.Errorf("%w: Approval ensure-approved returned mismatched owner identity or terminal state", common.ErrConflict)
		}
		return nil
	}
}

func stringFromPayload(payload workorcheventpayload.Payload, key string) string {
	value, _ := payload.String(key)
	return value
}
