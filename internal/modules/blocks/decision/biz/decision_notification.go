package decision

import (
	"context"
	"fmt"

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
	return m.withWorkspaceActiveDelivery(ctx, ticket.WorkspaceID, func(guarded context.Context) error {
		event := magicevent.NewEventWithContext(decisionevents.TopicTicketCreated, modulecommon.ModuleID, decisionevents.NotificationDestination, magicevent.NewHeader(), guarded, ticketChangedData(ticket))
		m.bindDecisionNotificationLane(event, ticket)
		m.PostEvent(event)
		return nil
	})
}

func (m *Biz) ensureTicketCreatedNotification(ctx context.Context, ticket *decisionmodel.Ticket) error {
	if ticket == nil {
		return nil
	}
	if ticket.CreatedNotificationPublishedAt != "" {
		return nil
	}
	if err := m.publishTicketCreated(ctx, ticket); err != nil {
		return err
	}
	updated, err := m.service.MarkTicketCreatedNotificationPublished(ctx, ticket)
	if err != nil {
		return err
	}
	*ticket = *updated
	return nil
}

func (m *Biz) recordAndPublishResolvedTicket(ctx context.Context, ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution, transitioned bool) error {
	runtimeProposal := ticket != nil && ticket.DecisionKind == "runtime_governance" && ticket.RuntimeGovernanceProposal != nil &&
		ticket.RuntimeGovernanceProposal.State == decisionmodel.RuntimeGovernanceResolutionProposalCommitted
	workspacePreSealClarification := isWorkspacePreSealDefinitionClarification(ticket)
	requestContractRepair := ticket != nil && ticket.DefinitionClarification != nil && ticket.DefinitionClarification.RequestContractRepairHash != "" && resolution != nil && resolution.Type == decisionevents.DefinitionClarificationResolutionRequestContractRepaired
	// A pre-seal Definition clarification is workspace-scoped and deliberately
	// has no Run. Its dedicated resolution result plus TicketResolved
	// notification are the complete delivery contract. Sending it through the
	// run-scoped Orchestrator evidence command would make a valid admission
	// resolution fail after Decision has already persisted it.
	if !workspacePreSealClarification && !requestContractRepair && (!runtimeProposal || ticket.RuntimeGovernanceProposal.EvidenceRecordedAt == "") {
		if err := m.recordDecisionResolutionEvidence(ctx, ticket, resolution); err != nil {
			return err
		}
		if runtimeProposal {
			updated, err := m.service.MarkRuntimeGovernanceEvidenceRecorded(ctx, ticket)
			if err != nil {
				return err
			}
			*ticket = *updated
		}
	}
	shouldPublish := transitioned
	if runtimeProposal {
		shouldPublish = shouldPublish || ticket.RuntimeGovernanceProposal.ResolutionNotificationPublishedAt == ""
	} else if ticket != nil {
		shouldPublish = shouldPublish || ticket.ResolutionNotificationPublishedAt == ""
	}
	if !shouldPublish {
		return nil
	}
	if m == nil || m.EventHub() == nil || ticket == nil || resolution == nil {
		return nil
	}
	data := decisionevents.TicketResolvedData{
		NotificationID:     decisionResolvedNotificationID(ticket, resolution),
		Ticket:             ticketChangedData(ticket),
		ResolutionID:       resolution.ID,
		ResolutionType:     resolution.Type,
		ResolutionPayload:  resolution.Payload.Clone().Fields,
		ContinuationAction: resolution.ContinuationAction,
		ResumeRunID:        resolution.ResumeRunID,
		SuccessorRunID:     resolution.SuccessorRunID,
	}
	if err := m.withWorkspaceActiveDelivery(ctx, ticket.WorkspaceID, func(guarded context.Context) error {
		event := magicevent.NewEventWithContext(decisionevents.TopicTicketResolved, modulecommon.ModuleID, decisionevents.NotificationDestination, magicevent.NewHeader(), guarded, data)
		m.bindDecisionNotificationLane(event, ticket)
		m.PostEvent(event)
		return nil
	}); err != nil {
		return err
	}
	if runtimeProposal {
		updated, err := m.service.MarkRuntimeGovernanceNotificationPublished(ctx, ticket)
		if err != nil {
			return err
		}
		*ticket = *updated
	} else {
		updated, err := m.service.MarkResolutionNotificationPublished(ctx, ticket)
		if err != nil {
			return err
		}
		*ticket = *updated
	}
	return nil
}

func isWorkspacePreSealDefinitionClarification(ticket *decisionmodel.Ticket) bool {
	return ticket != nil &&
		ticket.DecisionKind == "workspace_definition_clarification" &&
		ticket.RunID == "" &&
		ticket.StepID == "" &&
		ticket.DefinitionClarification != nil &&
		ticket.DefinitionClarification.Class == decisionmodel.DefinitionClarificationWorkspacePreSeal
}

func ticketChangedData(ticket *decisionmodel.Ticket) decisionevents.TicketChangedData {
	if ticket == nil {
		return decisionevents.TicketChangedData{}
	}
	return decisionevents.TicketChangedData{
		NotificationID: decisionCreatedNotificationID(ticket),
		TicketID:       ticket.ID,
		WorkspaceID:    ticket.WorkspaceID,
		RunID:          ticket.RunID,
		StepID:         ticket.StepID,
		Reason:         ticket.Reason,
		DecisionKind:   ticket.DecisionKind,
		State:          ticket.State,
	}
}

func decisionCreatedNotificationID(ticket *decisionmodel.Ticket) string {
	if ticket == nil || ticket.WorkspaceID == "" || ticket.ID == "" {
		return ""
	}
	return fmt.Sprintf("decision-notification/created/%s/%s", ticket.WorkspaceID, ticket.ID)
}

func decisionResolvedNotificationID(ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution) string {
	if ticket == nil || resolution == nil || ticket.WorkspaceID == "" || ticket.ID == "" || resolution.ID == "" {
		return ""
	}
	return fmt.Sprintf("decision-notification/resolved/%s/%s/%s", ticket.WorkspaceID, ticket.ID, resolution.ID)
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
