package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type definitionAdmissionInternalEventKind string

const (
	definitionAdmissionInternalEventCandidateSubmitted              definitionAdmissionInternalEventKind = "candidate_submitted"
	definitionAdmissionInternalEventCandidatePromoted               definitionAdmissionInternalEventKind = "candidate_promoted"
	definitionAdmissionInternalEventClarificationResolutionObserved definitionAdmissionInternalEventKind = "clarification_resolution_observed"
)

// definitionAdmissionInternalEvent is the only Workspace-owner input allowed
// to select a pre-seal admission reducer. External commands carry immutable
// authoring facts; they never carry a target activation-admission state.
type definitionAdmissionInternalEvent struct {
	Kind        definitionAdmissionInternalEventKind
	WorkspaceID common.WorkspaceID

	CandidateSubmission     *workspaceevents.SubmitDefinitionAdmissionCandidateCommand
	CandidatePromotion      *workspaceevents.PromoteDefinitionAdmissionCandidateCommand
	ClarificationResolution *workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionCommand
}

type definitionAdmissionTransition struct {
	Next *workspacemodel.Workspace
	Noop bool

	CandidateSubmissionResult     *workspaceevents.SubmitDefinitionAdmissionCandidateResult
	CandidatePromotionResult      *workspaceevents.PromoteDefinitionAdmissionCandidateResult
	ClarificationResolutionResult *workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult
}

func normalizeDefinitionAdmissionInternalEvent(event definitionAdmissionInternalEvent) (definitionAdmissionInternalEvent, error) {
	if event.WorkspaceID == "" {
		return definitionAdmissionInternalEvent{}, common.ErrInvalidArgument
	}
	payloadCount := 0
	for _, present := range []bool{event.CandidateSubmission != nil, event.CandidatePromotion != nil, event.ClarificationResolution != nil} {
		if present {
			payloadCount++
		}
	}
	if payloadCount != 1 {
		return definitionAdmissionInternalEvent{}, common.ErrInvalidArgument
	}
	switch event.Kind {
	case definitionAdmissionInternalEventCandidateSubmitted:
		if event.CandidateSubmission == nil {
			return definitionAdmissionInternalEvent{}, common.ErrInvalidArgument
		}
		command := *event.CandidateSubmission
		command.Candidate = command.Candidate.Normalized()
		if err := command.Candidate.Validate(); err != nil || command.Candidate.WorkspaceID != event.WorkspaceID {
			if err != nil {
				return definitionAdmissionInternalEvent{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
			}
			return definitionAdmissionInternalEvent{}, common.ErrInvalidArgument
		}
		event.CandidateSubmission = &command
	case definitionAdmissionInternalEventCandidatePromoted:
		if event.CandidatePromotion == nil {
			return definitionAdmissionInternalEvent{}, common.ErrInvalidArgument
		}
		command := *event.CandidatePromotion
		command.CandidateID = strings.TrimSpace(command.CandidateID)
		command.CandidateHash = strings.TrimSpace(command.CandidateHash)
		if command.WorkspaceID != event.WorkspaceID || command.CandidateID == "" || command.CandidateRevision == 0 || command.CandidateHash == "" || command.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
			return definitionAdmissionInternalEvent{}, common.ErrInvalidArgument
		}
		event.CandidatePromotion = &command
	case definitionAdmissionInternalEventClarificationResolutionObserved:
		if event.ClarificationResolution == nil {
			return definitionAdmissionInternalEvent{}, common.ErrInvalidArgument
		}
		command := *event.ClarificationResolution
		command.CandidateID = strings.TrimSpace(command.CandidateID)
		command.CandidateHash = strings.TrimSpace(command.CandidateHash)
		command.ResolutionBindingHash = strings.TrimSpace(command.ResolutionBindingHash)
		command.DefinitionRevision = strings.TrimSpace(command.DefinitionRevision)
		command.StartupInputHash = strings.TrimSpace(command.StartupInputHash)
		if command.WorkspaceID != event.WorkspaceID || command.CandidateID == "" || command.CandidateRevision == 0 || command.CandidateHash == "" || command.BlockingTicketID == "" || command.ResolutionID == "" || command.ResolutionBindingHash == "" || command.DefinitionRevision == "" || command.StartupInputHash == "" {
			return definitionAdmissionInternalEvent{}, common.ErrInvalidArgument
		}
		event.ClarificationResolution = &command
	default:
		return definitionAdmissionInternalEvent{}, common.ErrInvalidArgument
	}
	return event, nil
}

func reduceDefinitionAdmissionInternalEvent(current *workspacemodel.Workspace, event definitionAdmissionInternalEvent) (definitionAdmissionTransition, error) {
	if current == nil || current.ID != event.WorkspaceID {
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	switch event.Kind {
	case definitionAdmissionInternalEventCandidateSubmitted:
		return reduceDefinitionAdmissionCandidateSubmission(current, *event.CandidateSubmission)
	case definitionAdmissionInternalEventCandidatePromoted:
		return reduceDefinitionAdmissionCandidatePromotion(current, *event.CandidatePromotion)
	case definitionAdmissionInternalEventClarificationResolutionObserved:
		return reduceDefinitionClarificationResolution(current, *event.ClarificationResolution)
	default:
		return definitionAdmissionTransition{}, common.ErrInvalidArgument
	}
}

func reduceDefinitionAdmissionCandidateSubmission(current *workspacemodel.Workspace, command workspaceevents.SubmitDefinitionAdmissionCandidateCommand) (definitionAdmissionTransition, error) {
	candidate := command.Candidate
	currentRevision := uint64(0)
	for _, existing := range current.DefinitionAdmissionCandidates {
		if existing.CandidateID != candidate.CandidateID {
			continue
		}
		if existing.CandidateRevision == candidate.CandidateRevision {
			if existing.CandidateHash != candidate.CandidateHash {
				return definitionAdmissionTransition{}, common.ErrConflict
			}
			result := workspaceevents.SubmitDefinitionAdmissionCandidateResult{Candidate: existing.Clone(), Disposition: workspaceevents.DefinitionAdmissionCandidateDuplicate}
			return definitionAdmissionTransition{Noop: true, CandidateSubmissionResult: &result}, nil
		}
		if existing.CandidateRevision > currentRevision {
			currentRevision = existing.CandidateRevision
		}
	}
	if command.ExpectedCandidateRevision != currentRevision || candidate.CandidateRevision != currentRevision+1 {
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	next := *current
	next.DefinitionAdmissionCandidates = append(cloneDefinitionAdmissionCandidates(current.DefinitionAdmissionCandidates), candidate.Clone())
	result := workspaceevents.SubmitDefinitionAdmissionCandidateResult{Candidate: candidate.Clone(), Disposition: workspaceevents.DefinitionAdmissionCandidateCreated}
	return definitionAdmissionTransition{Next: &next, CandidateSubmissionResult: &result}, nil
}

func reduceDefinitionAdmissionCandidatePromotion(current *workspacemodel.Workspace, command workspaceevents.PromoteDefinitionAdmissionCandidateCommand) (definitionAdmissionTransition, error) {
	candidate, isCurrent := currentDefinitionAdmissionCandidate(current.DefinitionAdmissionCandidates, command.CandidateID)
	if candidate == nil || !isCurrent || candidate.CandidateRevision != command.CandidateRevision || candidate.CandidateHash != command.CandidateHash {
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	if err := command.AdmissionResult.ValidateFor(*candidate); err != nil {
		return definitionAdmissionTransition{}, fmt.Errorf("%w: %v", common.ErrConflict, err)
	}
	if current.StartupInput != nil || current.ActivationAdmission != nil {
		if samePromotion(current, *candidate, command.AdmissionResult, command.BlockingTicketID) {
			result := workspaceevents.PromoteDefinitionAdmissionCandidateResult{
				DefinitionRevision: current.StartupInput.DefinitionRevision, StartupInputHash: current.StartupInput.StartupInputHash,
				CandidateItemToItemRefMappingHash: current.StartupInput.CandidateItemToItemRefMappingHash,
				StartupInput:                      current.StartupInput.Clone(), ActivationAdmission: current.ActivationAdmission.Normalized(), Duplicate: true,
			}
			return definitionAdmissionTransition{Noop: true, CandidatePromotionResult: &result}, nil
		}
		if candidate.BaseDefinitionRevision == "" || candidate.BaseStartupInputHash == "" || current.StartupInput == nil || candidate.BaseDefinitionRevision != current.StartupInput.DefinitionRevision || candidate.BaseStartupInputHash != current.StartupInput.StartupInputHash {
			return definitionAdmissionTransition{}, common.ErrConflict
		}
	}
	startup := startupInputFromCandidate(*candidate, command.AdmissionResult)
	if err := startup.Validate(); err != nil {
		return definitionAdmissionTransition{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	admission := workspacemodel.WorkspaceActivationAdmission{
		WorkspaceID: command.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash,
		DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible,
	}
	if command.BlockingTicketID != "" {
		admission.State = workspacemodel.WorkspaceActivationPendingClarificationResolution
		admission.BlockingTicketID = command.BlockingTicketID
	}
	if err := admission.ValidateCandidate(startup, *candidate); err != nil {
		return definitionAdmissionTransition{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	next := *current
	next.StartupInput = &startup
	next.ActivationAdmission = &admission
	result := workspaceevents.PromoteDefinitionAdmissionCandidateResult{
		DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash,
		CandidateItemToItemRefMappingHash: startup.CandidateItemToItemRefMappingHash,
		StartupInput:                      startup.Clone(), ActivationAdmission: admission.Normalized(),
	}
	return definitionAdmissionTransition{Next: &next, CandidatePromotionResult: &result}, nil
}

func reduceDefinitionClarificationResolution(current *workspacemodel.Workspace, command workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionCommand) (definitionAdmissionTransition, error) {
	if current.StartupInput == nil || current.ActivationAdmission == nil {
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	admission := current.ActivationAdmission.Normalized()
	if admission.WorkspaceID != command.WorkspaceID || admission.CandidateID != command.CandidateID || admission.CandidateRevision != command.CandidateRevision || admission.CandidateHash != command.CandidateHash || admission.BlockingTicketID != command.BlockingTicketID || admission.DefinitionRevision != command.DefinitionRevision || admission.StartupInputHash != command.StartupInputHash {
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	if admission.State == workspacemodel.WorkspaceActivationEligible {
		if admission.ClarificationResolutionID == command.ResolutionID && admission.ClarificationResolutionBindingHash == command.ResolutionBindingHash {
			result := workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{ActivationAdmission: admission, Transitioned: false}
			return definitionAdmissionTransition{Noop: true, ClarificationResolutionResult: &result}, nil
		}
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	if admission.State != workspacemodel.WorkspaceActivationPendingClarificationResolution {
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	admission.State = workspacemodel.WorkspaceActivationEligible
	admission.ClarificationResolutionID = command.ResolutionID
	admission.ClarificationResolutionBindingHash = command.ResolutionBindingHash
	candidate, isCurrent := currentDefinitionAdmissionCandidate(current.DefinitionAdmissionCandidates, admission.CandidateID)
	if candidate == nil || !isCurrent || candidate.CandidateRevision != admission.CandidateRevision || candidate.CandidateHash != admission.CandidateHash {
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	if err := admission.ValidateCandidate(*current.StartupInput, *candidate); err != nil {
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	next := *current
	next.ActivationAdmission = &admission
	result := workspaceevents.AcknowledgeWorkspaceDefinitionClarificationResolutionResult{ActivationAdmission: admission, Transitioned: true}
	return definitionAdmissionTransition{Next: &next, ClarificationResolutionResult: &result}, nil
}

func (m *Biz) submitDefinitionAdmissionInternalEvent(ctx context.Context, event definitionAdmissionInternalEvent) (definitionAdmissionTransition, error) {
	if m == nil || m.workspaces == nil {
		return definitionAdmissionTransition{}, common.ErrInvalidArgument
	}
	event, err := normalizeDefinitionAdmissionInternalEvent(event)
	if err != nil {
		return definitionAdmissionTransition{}, err
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, event.WorkspaceID)
	if err != nil {
		return definitionAdmissionTransition{}, err
	}
	for attempt := 0; attempt < 4; attempt++ {
		current, err := m.workspaces.Get(bound, event.WorkspaceID)
		if err != nil {
			return definitionAdmissionTransition{}, err
		}
		transition, err := reduceDefinitionAdmissionInternalEvent(current, event)
		if err != nil {
			return definitionAdmissionTransition{}, err
		}
		if transition.Noop {
			return transition, nil
		}
		if transition.Next == nil {
			return definitionAdmissionTransition{}, common.ErrInvalidArgument
		}
		if err := m.workspaces.CompareAndSwap(bound, transition.Next, current.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return definitionAdmissionTransition{}, err
		}
		return transition, nil
	}
	return definitionAdmissionTransition{}, common.ErrConflict
}
