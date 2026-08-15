package decision

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (m *Biz) publishTicketCreated(ctx context.Context, ticket *decisionmodel.Ticket) error {
	if m == nil || m.EventHub() == nil || ticket == nil {
		return nil
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicTicketCreated, modulecommon.ModuleID, decisionevents.NotificationDestination, magicevent.NewHeader(), ctx, ticketChangedData(ticket))
	m.bindDecisionNotificationLane(event, ticket)
	m.PostEvent(event)
	return nil
}

func (m *Biz) recordAndPublishResolvedTicket(ctx context.Context, ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution, transitioned bool) error {
	if err := m.recordDecisionResolutionEvidence(ctx, ticket, resolution); err != nil {
		return err
	}
	if !transitioned {
		return nil
	}
	if m == nil || m.EventHub() == nil || ticket == nil || resolution == nil {
		return nil
	}
	data := decisionevents.TicketResolvedData{
		Ticket:             ticketChangedData(ticket),
		ResolutionID:       resolution.ID,
		ResolutionType:     resolution.Type,
		ResolutionPayload:  resolution.Payload.Clone().Fields,
		ContinuationAction: resolution.ContinuationAction,
		ResumeRunID:        resolution.ResumeRunID,
		SuccessorRunID:     resolution.SuccessorRunID,
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicTicketResolved, modulecommon.ModuleID, decisionevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	m.bindDecisionNotificationLane(event, ticket)
	m.PostEvent(event)
	return nil
}

func ticketChangedData(ticket *decisionmodel.Ticket) decisionevents.TicketChangedData {
	if ticket == nil {
		return decisionevents.TicketChangedData{}
	}
	return decisionevents.TicketChangedData{
		TicketID:     ticket.ID,
		WorkspaceID:  ticket.WorkspaceID,
		RunID:        ticket.RunID,
		StepID:       ticket.StepID,
		Reason:       ticket.Reason,
		DecisionKind: ticket.DecisionKind,
		State:        ticket.State,
	}
}

func (m *Biz) bindDecisionNotificationLane(event magicevent.Event, ticket *decisionmodel.Ticket) {
	if event == nil || ticket == nil {
		return
	}
	event.BindLaneKey(events.EventHubLaneKeyFor(decisionevents.Contracts(), event.ID(), eventcontract.LaneScope{
		SubjectType: "decision_ticket",
		SubjectID:   string(ticket.ID),
		WorkspaceID: string(ticket.WorkspaceID),
		RunID:       string(ticket.RunID),
		StepID:      string(ticket.StepID),
	}))
}
