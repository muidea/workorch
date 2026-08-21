package governanceports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

const definitionClarificationSource = "application/workorchd"

func OpenWorkspaceDefinitionClarification(ctx context.Context, hub magicevent.Hub, command decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand) (decisionevents.OpenWorkspaceDefinitionClarificationRequestResult, error) {
	if hub == nil || command.WorkspaceID == "" {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicOpenWorkspaceDefinitionClarification, definitionClarificationSource, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "decision_ticket", SubjectID: string(command.TicketID), WorkspaceID: string(command.WorkspaceID)}))
	return ownerevent.Decision[decisionevents.OpenWorkspaceDefinitionClarificationRequestResult](hub.Send(event), event.ID())
}

func ResolveWorkspaceDefinitionClarificationByAdmission(ctx context.Context, hub magicevent.Hub, command decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand) (decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult, error) {
	if hub == nil || command.WorkspaceID == "" {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicResolveWorkspaceDefinitionClarificationByAdmission, definitionClarificationSource, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "decision_ticket", SubjectID: string(command.TicketID), WorkspaceID: string(command.WorkspaceID)}))
	return ownerevent.Decision[decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult](hub.Send(event), event.ID())
}

func ReconcileWorkspaceDefinitionClarificationCandidate(ctx context.Context, hub magicevent.Hub, command decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand) (decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult, error) {
	command = command.Normalized()
	if hub == nil || command.OldRequest.WorkspaceID == "" {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicReconcileWorkspaceDefinitionClarificationCandidate, definitionClarificationSource, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "decision_ticket", SubjectID: string(command.OldRequest.TicketID), WorkspaceID: string(command.OldRequest.WorkspaceID)}))
	return ownerevent.Decision[decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult](hub.Send(event), event.ID())
}
