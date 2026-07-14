package governanceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func ResolveDecision(ctx context.Context, hub magicevent.Hub, ticket *decisionmodel.Ticket, resolutionType string, payload workorcheventpayload.Payload) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	if ticket == nil || ticket.ID == "" {
		return nil, nil, common.ErrInvalidArgument
	}
	ticketID := string(ticket.ID)
	resolutionID := "resolution-" + string(ticket.WorkspaceID) + "-" + ticketID + "-" + resolutionType
	if hub == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandEnsureResolved, "application/workorchd/appservice", decisionevents.CommandDestination, magicevent.NewHeader(), ctx, decisionevents.EnsureResolvedCommand{
		ResolutionID:       common.ResolutionID(resolutionID),
		TicketID:           ticket.ID,
		WorkspaceID:        ticket.WorkspaceID,
		RunID:              ticket.RunID,
		StepID:             ticket.StepID,
		Type:               resolutionType,
		Payload:            payload.Clone().Fields,
		ContinuationAction: stringPayloadValue(payload, "continuationAction"),
		ResumeRunID:        common.RunID(stringPayloadValue(payload, "resumeRunId")),
		SuccessorRunID:     common.RunID(stringPayloadValue(payload, "successorRunId")),
	})
	ownerevent.BindLane(event, decisionevents.TopicDecisionCommandEnsureResolved, "decision_ticket", ticketID, ticket.WorkspaceID, ticket.RunID, ticket.StepID)
	response, err := ownerevent.Decision(hub.Send(event), decisionevents.TopicDecisionCommandEnsureResolved)
	if err != nil {
		return nil, nil, err
	}
	if response.Ticket == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	if response.Resolution == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return response.Ticket, response.Resolution, nil
}

func stringPayloadValue(payload workorcheventpayload.Payload, key string) string {
	value, _ := payload.String(key)
	return value
}
