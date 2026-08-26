package governanceports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

const definitionClarificationSource = "application/workorchd"

func OpenWorkspaceDefinitionClarification(ctx context.Context, hub magicevent.Hub, command decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand) (decisionevents.OpenWorkspaceDefinitionClarificationRequestResult, error) {
	command = command.Normalized()
	if hub == nil || command.WorkspaceID == "" {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, common.ErrInvalidArgument
	}
	if err := command.Validate(); err != nil {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicOpenWorkspaceDefinitionClarification, definitionClarificationSource, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "decision_ticket", SubjectID: string(command.TicketID), WorkspaceID: string(command.WorkspaceID)}))
	result, err := ownerevent.Decision[decisionevents.OpenWorkspaceDefinitionClarificationRequestResult](hub.Send(event), event.ID())
	if err != nil {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, err
	}
	result = result.Normalized()
	if err := result.ValidateFor(command); err != nil {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, fmt.Errorf("%w: Decision owner returned invalid workspace Definition clarification result: %v", common.ErrConflict, err)
	}
	return result, nil
}

func ResolveWorkspaceDefinitionClarificationByAdmission(ctx context.Context, hub magicevent.Hub, command decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand) (decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult, error) {
	command = command.Normalized()
	if hub == nil || command.WorkspaceID == "" {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, common.ErrInvalidArgument
	}
	if err := command.Validate(); err != nil {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicResolveWorkspaceDefinitionClarificationByAdmission, definitionClarificationSource, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "decision_ticket", SubjectID: string(command.TicketID), WorkspaceID: string(command.WorkspaceID)}))
	result, err := ownerevent.Decision[decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult](hub.Send(event), event.ID())
	if err != nil {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, err
	}
	result = result.Normalized()
	if err := result.ValidateFor(command); err != nil {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, fmt.Errorf("%w: Decision owner returned invalid workspace Definition clarification resolution: %v", common.ErrConflict, err)
	}
	return result, nil
}

func ReconcileWorkspaceDefinitionClarificationCandidate(ctx context.Context, hub magicevent.Hub, command decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand) (decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult, error) {
	command = command.Normalized()
	if hub == nil || command.OldRequest.WorkspaceID == "" {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrInvalidArgument
	}
	if err := command.Validate(); err != nil {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	event := magicevent.NewEventWithContext(decisionevents.TopicReconcileWorkspaceDefinitionClarificationCandidate, definitionClarificationSource, decisionevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "decision_ticket", SubjectID: string(command.OldRequest.TicketID), WorkspaceID: string(command.OldRequest.WorkspaceID)}))
	result, err := ownerevent.Decision[decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult](hub.Send(event), event.ID())
	if err != nil {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, err
	}
	result = result.Normalized()
	if err := result.ValidateFor(command); err != nil {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, fmt.Errorf("%w: Decision owner returned invalid workspace Definition clarification reconciliation: %v", common.ErrConflict, err)
	}
	return result, nil
}
