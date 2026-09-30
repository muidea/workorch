package biz

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governanceports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	definitioncontract "github.com/muidea/workorch/internal/pkg/workorchdefinitioncontract"
)

// ConfirmAndAdmitDefinitionAuthoring never creates a Run. If any candidate
// verification lacks an exact Profile declaration, confirmation is durable but
// admission remains blocked; the user must revise and review the proposal.
func (b *Biz) ConfirmAndAdmitDefinitionAuthoring(ctx context.Context, command workspaceevents.ConfirmDefinitionAuthoringCommand) (workspaceevents.AdmitDefinitionAuthoringResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringConfirm, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceAuthoringConfirm, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	confirmation := b.SendEvent(ev)
	if confirmation == nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
	}
	if err := confirmation.Error(); err != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, err
	}
	value, err := confirmation.Get()
	if err != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, err
	}
	confirmed, ok := value.(workspaceevents.ConfirmDefinitionAuthoringResult)
	if !ok || confirmed.ValidateFor(command) != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
	}
	current, readErr := workspaceports.ReadWorkspaceRecord(ctx, b.EventHub(), command.WorkspaceID)
	if readErr != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, readErr
	}
	view, queryErr := b.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: command.WorkspaceID})
	if queryErr != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, queryErr
	}
	if current.Revision != view.WorkspaceRevision || view.Record == nil || view.Confirmation == nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
	}
	current.DefinitionAuthoring = view.Record
	current.DefinitionAuthoringConfirmation = view.Confirmation
	current.DefinitionAuthoringHistory = view.History
	if view.AdmissionCandidate != nil {
		return b.replayConfirmedDefinitionAuthoringAdmission(ctx, command, current)
	}
	if len(current.DefinitionAdmissionCandidates) != len(current.DefinitionAuthoringHistory) {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
	}
	template, templateErr := workspaceports.ReadWorkspaceTemplateRecord(ctx, b.EventHub(), current.TemplateID)
	if templateErr != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, templateErr
	}
	candidate, candidateErr := activationports.AuthoringAdmissionCandidate(current, template)
	if candidateErr != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, candidateErr
	}
	return b.AdmitConfirmedDefinitionAuthoring(ctx, current, candidate)
}

func (b *Biz) replayConfirmedDefinitionAuthoringAdmission(ctx context.Context, confirmation workspaceevents.ConfirmDefinitionAuthoringCommand, current *workspace.Workspace) (workspaceevents.AdmitDefinitionAuthoringResult, error) {
	if current == nil || current.ID != confirmation.WorkspaceID || current.DefinitionAuthoring == nil || current.DefinitionAuthoringConfirmation == nil || len(current.DefinitionAdmissionCandidates) != len(current.DefinitionAuthoringHistory)+1 || current.ValidateDefinitionAuthoringHistory() != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
	}
	journal := current.DefinitionAuthoringConfirmation
	if journal.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || journal.InputHash != confirmation.InputHash || journal.DerivationPermitID != confirmation.DerivationPermitID || journal.ProposalHash != confirmation.ProposalHash || journal.SpecificationHash != confirmation.SpecificationHash {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
	}
	candidate := current.DefinitionAdmissionCandidates[len(current.DefinitionAdmissionCandidates)-1].Clone()
	if candidate.AuthoringRef == nil || candidate.AuthoringRef.InputHash != confirmation.InputHash || candidate.AuthoringRef.DerivationPermitID != confirmation.DerivationPermitID || candidate.AuthoringRef.ProposalHash != confirmation.ProposalHash || candidate.AuthoringRef.SpecificationHash != confirmation.SpecificationHash {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
	}
	if _, _, err := current.ValidateActivationAdmissionPersistence(); err != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, err
	}
	return b.AdmitConfirmedDefinitionAuthoring(ctx, current, candidate)
}

func (b *Biz) AdmitConfirmedDefinitionAuthoring(ctx context.Context, current *workspace.Workspace, candidate workspace.DefinitionAdmissionCandidate) (workspaceevents.AdmitDefinitionAuthoringResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || current == nil || candidate.WorkspaceID != current.ID {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrInvalidArgument
	}
	command := workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: candidate.Clone(), ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)}
	if err := command.Validate(); err != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, err
	}
	admitted, err := b.sendAuthoringAdmission(ctx, command)
	if err != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, err
	}
	if admitted.AdmissionResult.Disposition == workspace.DefinitionAdmissionClarificationRequired {
		binding := workspaceevents.DefinitionAdmissionClarificationBinding{Candidate: admitted.Candidate, AdmissionResult: admitted.AdmissionResult}
		identity, err := binding.DecisionRequestIdentity()
		if err != nil {
			return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
		}
		if admitted.PriorClarification != nil {
			old, oldErr := admitted.PriorClarification.DecisionRequestIdentity()
			if oldErr != nil {
				return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
			}
			if _, err := governanceports.OpenWorkspaceDefinitionClarification(ctx, b.EventHub(), decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(old)); err != nil {
				return workspaceevents.AdmitDefinitionAuthoringResult{}, err
			}
			if _, err := governanceports.ReconcileWorkspaceDefinitionClarificationCandidate(ctx, b.EventHub(), decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand{
				OldRequest: decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(old), NewRequest: decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(identity),
			}); err != nil {
				return workspaceevents.AdmitDefinitionAuthoringResult{}, err
			}
		} else if _, err := governanceports.OpenWorkspaceDefinitionClarification(ctx, b.EventHub(), decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(identity)); err != nil {
			return workspaceevents.AdmitDefinitionAuthoringResult{}, err
		}
		admitted.BlockingTicketID = identity.TicketID
		if admitted.ValidateFor(command) != nil {
			return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
		}
		return admitted, nil
	}
	if admitted.PriorClarification == nil {
		return admitted, nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		if admitted.PriorClarification == nil {
			return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
		}
		identity, identityErr := admitted.PriorClarification.DecisionRequestIdentity()
		if identityErr != nil {
			return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
		}
		opened, openErr := governanceports.OpenWorkspaceDefinitionClarification(ctx, b.EventHub(), decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(identity))
		if openErr != nil {
			return workspaceevents.AdmitDefinitionAuthoringResult{}, openErr
		}
		if admitted.StartupInput == nil {
			fact := workspaceevents.WorkspaceDefinitionClarificationTicketFact{RequestIdentity: identity, TicketBinding: opened.BindingValue()}
			if fact.ValidateFor(*admitted.PriorClarification) != nil {
				return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
			}
			command.ClarificationTicket = &fact
			admitted, err = b.sendAuthoringAdmission(ctx, command)
			if err != nil {
				return workspaceevents.AdmitDefinitionAuthoringResult{}, err
			}
			command.ClarificationTicket = nil
			continue
		}
		if admitted.ActivationAdmission == nil || opened.TicketID != admitted.ActivationAdmission.BlockingTicketID || admitted.ActivationAdmission.State != workspace.WorkspaceActivationPendingClarificationResolution && admitted.ActivationAdmission.State != workspace.WorkspaceActivationEligible {
			return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
		}
		resolve := decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
			TicketID: opened.TicketID, WorkspaceID: candidate.WorkspaceID,
			OldCandidateID: admitted.PriorClarification.Candidate.CandidateID, OldCandidateRevision: admitted.PriorClarification.Candidate.CandidateRevision, OldCandidateHash: admitted.PriorClarification.Candidate.CandidateHash,
			NewCandidateID: admitted.Candidate.CandidateID, NewCandidateRevision: admitted.Candidate.CandidateRevision, NewCandidateHash: admitted.Candidate.CandidateHash,
			AdmissionResultHash: admitted.AdmissionResult.AdmissionResultHash, DefinitionRevision: admitted.StartupInput.DefinitionRevision, StartupInputHash: admitted.StartupInput.StartupInputHash,
		}
		resolved, resolveErr := governanceports.ResolveWorkspaceDefinitionClarificationByAdmission(ctx, b.EventHub(), resolve)
		if resolveErr != nil {
			return workspaceevents.AdmitDefinitionAuthoringResult{}, resolveErr
		}
		fact := workspaceevents.WorkspaceDefinitionClarificationResolutionFact{ResolutionIdentity: definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity(resolve), ResolutionBinding: resolved.BindingValue()}
		if fact.ValidateFor(*admitted.PriorClarification, admitted.Candidate, admitted.AdmissionResult, *admitted.StartupInput, *admitted.ActivationAdmission) != nil {
			return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
		}
		command.ClarificationResolution = &fact
		return b.sendAuthoringAdmission(ctx, command)
	}
	return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
}

func (b *Biz) sendAuthoringAdmission(ctx context.Context, command workspaceevents.AdmitDefinitionAuthoringCommand) (workspaceevents.AdmitDefinitionAuthoringResult, error) {
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringAdmit, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceAuthoringAdmit, eventcontract.LaneScope{WorkspaceID: string(command.Candidate.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, err
	}
	value, err := result.Get()
	if err != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, err
	}
	admitted, ok := value.(workspaceevents.AdmitDefinitionAuthoringResult)
	if !ok || admitted.ValidateFor(command) != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, fmt.Errorf("%w: authoring admission owner result mismatch", common.ErrConflict)
	}
	return admitted, nil
}
