package governanceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ResolveDecision(ctx context.Context, hub magicevent.Hub, ticket *decisionmodel.Ticket, resolutionType string, payload map[string]any) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	if ticket == nil || ticket.ID == "" {
		return nil, nil, common.ErrInvalidArgument
	}
	ticketID := string(ticket.ID)
	resolutionID := "resolution-" + string(ticket.WorkspaceID) + "-" + ticketID + "-" + resolutionType
	response, err := sendGovernanceMagicCommand[decisionevents.EnsureResolvedCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandEnsureResolved, decisionevents.CommandDestination, "decision_ticket", ticketID, ticket.WorkspaceID, ticket.RunID, decisionevents.EnsureResolvedCommand{
		ResolutionID:       common.ResolutionID(resolutionID),
		TicketID:           ticket.ID,
		WorkspaceID:        ticket.WorkspaceID,
		RunID:              ticket.RunID,
		StepID:             ticket.StepID,
		Type:               resolutionType,
		Payload:            cloneMap(payload),
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

func stringPayloadValue(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}
