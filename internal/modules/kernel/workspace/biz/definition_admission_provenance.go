package workspace

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// verifyDefinitionClarificationProvenance turns caller-carried
// identity/binding values into a verified cross-owner fact. Self-consistency
// is necessary but not sufficient: before the Workspace private reducer/CAS
// runs, Decision must confirm the exact ticket/resolution identity from its
// own durable aggregate and resolution repository.
func (m *Biz) verifyDefinitionClarificationProvenance(ctx context.Context, intent workspaceevents.DefinitionAdmissionIntentCommand) error {
	if intent.ClarificationTicket == nil && intent.ClarificationResolution == nil {
		return nil
	}
	if m == nil || m.EventHub() == nil {
		return common.ErrConflict
	}
	ticketID := common.TicketID("")
	if intent.ClarificationTicket != nil {
		ticketID = intent.ClarificationTicket.RequestIdentity.TicketID
	}
	if intent.ClarificationResolution != nil {
		ticketID = intent.ClarificationResolution.ResolutionIdentity.TicketID
	}
	query := decisionevents.QueryWorkspaceDefinitionClarificationFactCommand{
		WorkspaceID: intent.Candidate.WorkspaceID,
		TicketID:    ticketID,
	}
	if err := query.Validate(); err != nil {
		return common.ErrConflict
	}
	event := magicevent.NewEventWithContext(
		decisionevents.TopicQueryWorkspaceDefinitionClarificationFact,
		ModuleID,
		decisionevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		query,
	)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), event.ID(), eventcontract.LaneScope{
		SubjectType: "decision_definition_clarification", SubjectID: string(ticketID), WorkspaceID: string(query.WorkspaceID),
	}))
	result := m.SendEvent(event)
	if result == nil {
		return fmt.Errorf("%w: Decision clarification provenance query returned no result", common.ErrConflict)
	}
	if err := result.Error(); err != nil {
		return fmt.Errorf("%w: Decision clarification provenance query: %v", common.ErrConflict, err)
	}
	value, err := result.Get()
	if err != nil {
		return fmt.Errorf("%w: Decision clarification provenance result: %v", common.ErrConflict, err)
	}
	provenance, ok := value.(decisionevents.QueryWorkspaceDefinitionClarificationFactResult)
	if !ok || provenance.ValidateFor(query) != nil || !provenance.Found || provenance.Ticket == nil {
		return fmt.Errorf("%w: Decision clarification provenance is absent or invalid", common.ErrConflict)
	}
	if intent.ClarificationTicket != nil {
		fact := intent.ClarificationTicket.Normalized()
		if err := fact.Validate(); err != nil || provenance.Resolution != nil || !provenance.Ticket.RequestIdentity.EqualNormalized(fact.RequestIdentity) ||
			provenance.Ticket.OpenBindingHash != fact.TicketBinding.ClarificationRequestBindingHash {
			return fmt.Errorf("%w: Decision persisted ticket does not match Workspace clarification fact", common.ErrConflict)
		}
		return nil
	}
	fact := intent.ClarificationResolution.Normalized()
	if err := fact.Validate(); err != nil || provenance.Resolution == nil {
		return fmt.Errorf("%w: Decision persisted resolution is absent or invalid", common.ErrConflict)
	}
	persisted := provenance.Resolution.Normalized()
	if persisted.ResolutionIdentity != fact.ResolutionIdentity ||
		persisted.ResolutionID != fact.ResolutionBinding.ResolutionID ||
		persisted.ResolutionType != fact.ResolutionBinding.ResolutionType ||
		persisted.ResolutionBindingHash != fact.ResolutionBinding.ResolutionBindingHash {
		return fmt.Errorf("%w: Decision persisted resolution does not match Workspace clarification fact", common.ErrConflict)
	}
	return nil
}
