package governanceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventvalue"
)

func ResolveDecision(ctx context.Context, hub magicevent.Hub, ticket *decisionmodel.Ticket, resolutionType string, payload map[string]any) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	if ticket == nil || ticket.ID == "" {
		return nil, nil, common.ErrInvalidArgument
	}
	ticketID := string(ticket.ID)
	resolutionID := "resolution-" + string(ticket.WorkspaceID) + "-" + ticketID + "-" + resolutionType
	resolutionPayload, err := eventvalue.FromMap(payload)
	if err != nil {
		return nil, nil, err
	}
	response, err := sendGovernanceMagicCommand[decisionevents.EnsureResolvedCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.CommandDestination, "decision_ticket", ticketID, ticket.WorkspaceID, ticket.RunID, decisionevents.EnsureResolvedCommand{
		ResolutionID:       common.ResolutionID(resolutionID),
		TicketID:           ticket.ID,
		WorkspaceID:        ticket.WorkspaceID,
		RunID:              ticket.RunID,
		StepID:             ticket.StepID,
		Type:               resolutionType,
		Payload:            resolutionPayload,
		ContinuationAction: stringPayloadValue(payload, "continuationAction"),
		ResumeRunID:        common.RunID(stringPayloadValue(payload, "resumeRunId")),
		SuccessorRunID:     common.RunID(stringPayloadValue(payload, "successorRunId")),
	})
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

func stringPayloadValue(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}
