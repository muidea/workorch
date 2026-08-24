package workspaceports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Definition admission is part of the application Runtime authority path,
// matching execution-intent and clarification ingress. It must not use the
// generic CRUD/read-port source identity.
const definitionAdmissionSourceModule = "application/workorchd"

func SubmitDefinitionAdmissionCandidate(ctx context.Context, hub magicevent.Hub, candidate workspacemodel.DefinitionAdmissionCandidate, expectedRevision uint64) (workspaceevents.SubmitDefinitionAdmissionCandidateResult, error) {
	if hub == nil || candidate.WorkspaceID == "" {
		return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, common.ErrInvalidArgument
	}
	candidate = candidate.WithHash()
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandSubmitDefinitionAdmissionCandidate, definitionAdmissionSourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.SubmitDefinitionAdmissionCandidateCommand{Candidate: candidate, ExpectedCandidateRevision: expectedRevision})
	ownerevent.BindLane(event, workspaceevents.Contracts(), event.ID(), "workspace", string(candidate.WorkspaceID), candidate.WorkspaceID, "", "")
	value, err := ownerevent.WorkspaceOwner[workspaceevents.SubmitDefinitionAdmissionCandidateResult](hub.Send(event), event.ID())
	if err != nil {
		return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, err
	}
	if err := value.Candidate.Validate(); err != nil || value.Candidate.CandidateHash != candidate.CandidateHash {
		return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, fmt.Errorf("%w: Workspace owner returned invalid definition candidate", common.ErrConflict)
	}
	if err := value.AdmissionResult.ValidateFor(value.Candidate); err != nil {
		return workspaceevents.SubmitDefinitionAdmissionCandidateResult{}, fmt.Errorf("%w: Workspace owner returned invalid definition admission result", common.ErrConflict)
	}
	return value, nil
}

func PromoteDefinitionAdmissionCandidate(ctx context.Context, hub magicevent.Hub, candidate workspacemodel.DefinitionAdmissionCandidate, blockingTicketID common.TicketID) (workspaceevents.PromoteDefinitionAdmissionCandidateResult, error) {
	if hub == nil || candidate.WorkspaceID == "" {
		return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandPromoteDefinitionAdmissionCandidate, definitionAdmissionSourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.PromoteDefinitionAdmissionCandidateCommand{
		WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision,
		CandidateHash: candidate.CandidateHash, BlockingTicketID: blockingTicketID,
	})
	ownerevent.BindLane(event, workspaceevents.Contracts(), event.ID(), "workspace", string(candidate.WorkspaceID), candidate.WorkspaceID, "", "")
	value, err := ownerevent.WorkspaceOwner[workspaceevents.PromoteDefinitionAdmissionCandidateResult](hub.Send(event), event.ID())
	if err != nil {
		return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, err
	}
	if err := value.StartupInput.Validate(); err != nil || value.StartupInputHash != value.StartupInput.StartupInputHash || value.DefinitionRevision != value.StartupInput.DefinitionRevision {
		return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, fmt.Errorf("%w: Workspace owner returned invalid promoted startup input", common.ErrConflict)
	}
	if err := value.ActivationAdmission.Validate(value.StartupInput); err != nil {
		return workspaceevents.PromoteDefinitionAdmissionCandidateResult{}, fmt.Errorf("%w: Workspace owner returned invalid activation admission", common.ErrConflict)
	}
	return value, nil
}

func AcknowledgeWorkspaceDefinitionClarification(ctx context.Context, hub magicevent.Hub, command workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionCommand) (workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult, error) {
	if hub == nil || command.WorkspaceID == "" {
		return workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandAcknowledgeDefinitionClarification, definitionAdmissionSourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), event.ID(), "workspace", string(command.WorkspaceID), command.WorkspaceID, "", "")
	return ownerevent.WorkspaceOwner[workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult](hub.Send(event), event.ID())
}
