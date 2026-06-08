package governanceports

import (
	"context"

	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ResolveDecision(ctx context.Context, hub events.Hub, ticket *decisionmodel.Ticket, resolutionType string, payload map[string]any) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	if ticket == nil || ticket.ID == "" {
		return nil, nil, common.ErrInvalidArgument
	}
	ticketID := string(ticket.ID)
	resolutionID := "resolution-" + string(ticket.WorkspaceID) + "-" + ticketID + "-" + resolutionType
	response, err := ownercommandports.PublishWithResponse(ctx, hub, decisionevents.TopicDecisionCommandEnsureResolved, "decision_ticket", ticketID, ticket.WorkspaceID, ticket.RunID, map[string]any{
		events.PayloadKeyCommand: decisioncontracts.ResolveTicketCommand{
			ResolutionID: common.ResolutionID(resolutionID),
			TicketID:     ticket.ID,
			WorkspaceID:  ticket.WorkspaceID,
			RunID:        ticket.RunID,
			StepID:       ticket.StepID,
			Type:         resolutionType,
			Payload:      cloneMap(payload),
		},
	})
	if err != nil {
		return nil, nil, err
	}
	item, ok := response.Payload[events.PayloadKeyTicket].(*decisionmodel.Ticket)
	if !ok || item == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	resolution, ok := response.Payload[events.PayloadKeyResolution].(*decisionmodel.Resolution)
	if !ok || resolution == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return item, resolution, nil
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
