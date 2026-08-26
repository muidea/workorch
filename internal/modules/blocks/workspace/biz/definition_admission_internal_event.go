package workspace

import (
	"context"
	"errors"
	"fmt"

	definitionadmission "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/definitionadmission"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// definitionAdmissionInternalEvent is the only Workspace-owner input allowed
// to select a pre-seal admission transition. Its public source carries one
// immutable candidate intent and, at most, one immutable Decision-owner fact.
type definitionAdmissionInternalEvent struct {
	WorkspaceID common.WorkspaceID
	Intent      *workspaceevents.DefinitionAdmissionIntentCommand
}

type definitionAdmissionTransition struct {
	Next   *workspacemodel.Workspace
	Noop   bool
	Result *workspaceevents.DefinitionAdmissionIntentResult
}

func normalizeDefinitionAdmissionInternalEvent(event definitionAdmissionInternalEvent) (definitionAdmissionInternalEvent, error) {
	if event.WorkspaceID == "" || event.Intent == nil {
		return definitionAdmissionInternalEvent{}, common.ErrInvalidArgument
	}
	intent := event.Intent.Normalized()
	if err := intent.Validate(); err != nil || intent.Candidate.WorkspaceID != event.WorkspaceID {
		if err != nil {
			return definitionAdmissionInternalEvent{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
		}
		return definitionAdmissionInternalEvent{}, common.ErrInvalidArgument
	}
	event.Intent = &intent
	return event, nil
}

func reduceDefinitionAdmissionInternalEvent(current *workspacemodel.Workspace, event definitionAdmissionInternalEvent) (definitionAdmissionTransition, error) {
	if current == nil || current.ID != event.WorkspaceID || event.Intent == nil {
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	if err := workspaceDeletionRejectsMutation(current); err != nil {
		return definitionAdmissionTransition{}, err
	}
	if _, _, err := current.ValidateActivationAdmissionPersistence(); err != nil {
		return definitionAdmissionTransition{}, fmt.Errorf("%w: invalid current definition admission: %v", common.ErrConflict, err)
	}
	return reduceDefinitionAdmissionIntent(current, *event.Intent)
}

func reduceDefinitionAdmissionIntent(current *workspacemodel.Workspace, command workspaceevents.DefinitionAdmissionIntentCommand) (definitionAdmissionTransition, error) {
	candidate := command.Candidate
	admissionResult, err := definitionadmission.Compile(candidate)
	if err != nil {
		return definitionAdmissionTransition{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	next, candidateDisposition, candidateChanged, err := persistDefinitionAdmissionCandidate(current, command)
	if err != nil {
		return definitionAdmissionTransition{}, err
	}
	stored, isCurrent := currentDefinitionAdmissionCandidate(next.DefinitionAdmissionCandidates, candidate.CandidateID)
	if stored == nil || !isCurrent || stored.CandidateRevision != candidate.CandidateRevision || stored.CandidateHash != candidate.CandidateHash {
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	priorClarification := outstandingDefinitionClarification(&next, candidate)

	if admissionResult.Disposition == workspacemodel.DefinitionAdmissionClarificationRequired {
		if command.ClarificationTicket != nil || command.ClarificationResolution != nil {
			return definitionAdmissionTransition{}, common.ErrConflict
		}
		result := definitionAdmissionIntentResult(candidate, admissionResult, candidateDisposition, workspaceevents.DefinitionAdmissionIntentClarificationRequired, priorClarification, nil, nil)
		return definitionAdmissionTransitionFor(next, candidateChanged, result)
	}
	if admissionResult.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		return definitionAdmissionTransition{}, common.ErrConflict
	}

	startup, activation, bound := definitionActivationForCandidate(&next, candidate)
	if bound {
		switch activation.State {
		case workspacemodel.WorkspaceActivationEligible:
			if command.ClarificationTicket != nil {
				return definitionAdmissionTransition{}, common.ErrConflict
			}
			if command.ClarificationResolution != nil {
				if err := command.ClarificationResolution.ValidateForEligible(candidate, admissionResult, startup, activation); err != nil {
					return definitionAdmissionTransition{}, common.ErrConflict
				}
			}
			result := definitionAdmissionIntentResult(candidate, admissionResult, candidateDisposition, workspaceevents.DefinitionAdmissionIntentEligible, priorClarification, &startup, &activation)
			return definitionAdmissionTransitionFor(next, candidateChanged, result)
		case workspacemodel.WorkspaceActivationPendingClarificationResolution:
			if priorClarification == nil {
				return definitionAdmissionTransition{}, common.ErrConflict
			}
			if command.ClarificationTicket != nil {
				if err := command.ClarificationTicket.ValidateFor(*priorClarification); err != nil || command.ClarificationTicket.TicketBinding.TicketID != activation.BlockingTicketID {
					return definitionAdmissionTransition{}, common.ErrConflict
				}
			}
			if command.ClarificationResolution != nil {
				if err := command.ClarificationResolution.ValidateFor(*priorClarification, candidate, admissionResult, startup, activation); err != nil {
					return definitionAdmissionTransition{}, common.ErrConflict
				}
				activation.State = workspacemodel.WorkspaceActivationEligible
				activation.ClarificationResolutionID = command.ClarificationResolution.ResolutionBinding.ResolutionID
				activation.ClarificationResolutionBindingHash = command.ClarificationResolution.ResolutionBinding.ResolutionBindingHash
				if err := activation.ValidateCandidate(startup, candidate); err != nil {
					return definitionAdmissionTransition{}, common.ErrConflict
				}
				next.ActivationAdmission = &activation
				result := definitionAdmissionIntentResult(candidate, admissionResult, candidateDisposition, workspaceevents.DefinitionAdmissionIntentEligible, priorClarification, &startup, &activation)
				return definitionAdmissionTransitionFor(next, true, result)
			}
			result := definitionAdmissionIntentResult(candidate, admissionResult, candidateDisposition, workspaceevents.DefinitionAdmissionIntentClarificationResolutionRequired, priorClarification, &startup, &activation)
			return definitionAdmissionTransitionFor(next, candidateChanged, result)
		default:
			return definitionAdmissionTransition{}, common.ErrConflict
		}
	}

	if command.ClarificationResolution != nil {
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	if priorClarification != nil {
		if command.ClarificationTicket == nil {
			result := definitionAdmissionIntentResult(candidate, admissionResult, candidateDisposition, workspaceevents.DefinitionAdmissionIntentClarificationTicketRequired, priorClarification, nil, nil)
			return definitionAdmissionTransitionFor(next, candidateChanged, result)
		}
		if err := command.ClarificationTicket.ValidateFor(*priorClarification); err != nil {
			return definitionAdmissionTransition{}, common.ErrConflict
		}
		promoted, startup, activation, err := promoteDefinitionAdmission(&next, candidate, admissionResult, command.ClarificationTicket.TicketBinding.TicketID)
		if err != nil {
			return definitionAdmissionTransition{}, err
		}
		result := definitionAdmissionIntentResult(candidate, admissionResult, candidateDisposition, workspaceevents.DefinitionAdmissionIntentClarificationResolutionRequired, priorClarification, &startup, &activation)
		return definitionAdmissionTransitionFor(promoted, true, result)
	}
	if command.ClarificationTicket != nil {
		return definitionAdmissionTransition{}, common.ErrConflict
	}
	promoted, startup, activation, err := promoteDefinitionAdmission(&next, candidate, admissionResult, "")
	if err != nil {
		return definitionAdmissionTransition{}, err
	}
	result := definitionAdmissionIntentResult(candidate, admissionResult, candidateDisposition, workspaceevents.DefinitionAdmissionIntentEligible, nil, &startup, &activation)
	return definitionAdmissionTransitionFor(promoted, true, result)
}

func persistDefinitionAdmissionCandidate(current *workspacemodel.Workspace, command workspaceevents.DefinitionAdmissionIntentCommand) (workspacemodel.Workspace, workspaceevents.DefinitionAdmissionCandidateDisposition, bool, error) {
	candidate := command.Candidate
	currentRevision := uint64(0)
	lineageID := ""
	seenRevisions := map[uint64]struct{}{}
	duplicateCandidate := false
	for _, existing := range current.DefinitionAdmissionCandidates {
		existing = existing.Normalized()
		if err := existing.Validate(); err != nil {
			return workspacemodel.Workspace{}, "", false, common.ErrConflict
		}
		if lineageID == "" {
			lineageID = existing.CandidateID
		}
		if existing.CandidateID != lineageID {
			return workspacemodel.Workspace{}, "", false, common.ErrConflict
		}
		if _, duplicate := seenRevisions[existing.CandidateRevision]; duplicate {
			return workspacemodel.Workspace{}, "", false, common.ErrConflict
		}
		seenRevisions[existing.CandidateRevision] = struct{}{}
		if existing.CandidateRevision == candidate.CandidateRevision {
			if existing.CandidateID != candidate.CandidateID || existing.CandidateHash != candidate.CandidateHash {
				return workspacemodel.Workspace{}, "", false, common.ErrConflict
			}
			duplicateCandidate = true
		}
		if existing.CandidateRevision > currentRevision {
			currentRevision = existing.CandidateRevision
		}
	}
	if currentRevision != uint64(len(seenRevisions)) {
		return workspacemodel.Workspace{}, "", false, common.ErrConflict
	}
	if lineageID != "" && candidate.CandidateID != lineageID {
		return workspacemodel.Workspace{}, "", false, common.ErrConflict
	}
	if duplicateCandidate {
		return *current, workspaceevents.DefinitionAdmissionCandidateDuplicate, false, nil
	}
	if command.ExpectedCandidateRevision != currentRevision || candidate.CandidateRevision != currentRevision+1 {
		return workspacemodel.Workspace{}, "", false, common.ErrConflict
	}
	if current.StartupInput == nil {
		if candidate.BaseDefinitionRevision != "" || candidate.BaseStartupInputHash != "" {
			return workspacemodel.Workspace{}, "", false, common.ErrConflict
		}
	} else if candidate.BaseDefinitionRevision != current.StartupInput.DefinitionRevision || candidate.BaseStartupInputHash != current.StartupInput.StartupInputHash {
		return workspacemodel.Workspace{}, "", false, common.ErrConflict
	}
	next := *current
	next.DefinitionAdmissionCandidates = append(cloneDefinitionAdmissionCandidates(current.DefinitionAdmissionCandidates), candidate.Clone())
	return next, workspaceevents.DefinitionAdmissionCandidateCreated, true, nil
}

func outstandingDefinitionClarification(current *workspacemodel.Workspace, candidate workspacemodel.DefinitionAdmissionCandidate) *workspaceevents.DefinitionAdmissionClarificationBinding {
	if current == nil {
		return nil
	}
	eligibleBaseline := uint64(0)
	if current.ActivationAdmission != nil {
		activation := current.ActivationAdmission.Normalized()
		if activation.State == workspacemodel.WorkspaceActivationEligible && activation.CandidateID == candidate.CandidateID {
			eligibleBaseline = activation.CandidateRevision
		}
	}
	var selected *workspaceevents.DefinitionAdmissionClarificationBinding
	for _, existing := range current.DefinitionAdmissionCandidates {
		if existing.CandidateID != candidate.CandidateID || existing.CandidateRevision <= eligibleBaseline || existing.CandidateRevision >= candidate.CandidateRevision || (selected != nil && existing.CandidateRevision <= selected.Candidate.CandidateRevision) {
			continue
		}
		admission, err := definitionadmission.Compile(existing)
		if err != nil || admission.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired {
			continue
		}
		binding := workspaceevents.DefinitionAdmissionClarificationBinding{Candidate: existing.Clone(), AdmissionResult: admission}
		selected = &binding
	}
	return selected
}

func definitionActivationForCandidate(current *workspacemodel.Workspace, candidate workspacemodel.DefinitionAdmissionCandidate) (workspacemodel.WorkspaceStartupInputContract, workspacemodel.WorkspaceActivationAdmission, bool) {
	if current == nil || current.StartupInput == nil || current.ActivationAdmission == nil {
		return workspacemodel.WorkspaceStartupInputContract{}, workspacemodel.WorkspaceActivationAdmission{}, false
	}
	startup := current.StartupInput.Clone()
	activation := current.ActivationAdmission.Normalized()
	if activation.CandidateID != candidate.CandidateID || activation.CandidateRevision != candidate.CandidateRevision || activation.CandidateHash != candidate.CandidateHash {
		return workspacemodel.WorkspaceStartupInputContract{}, workspacemodel.WorkspaceActivationAdmission{}, false
	}
	return startup, activation, true
}

func promoteDefinitionAdmission(current *workspacemodel.Workspace, candidate workspacemodel.DefinitionAdmissionCandidate, admissionResult workspacemodel.DefinitionAdmissionResult, blockingTicketID common.TicketID) (workspacemodel.Workspace, workspacemodel.WorkspaceStartupInputContract, workspacemodel.WorkspaceActivationAdmission, error) {
	if current == nil {
		return workspacemodel.Workspace{}, workspacemodel.WorkspaceStartupInputContract{}, workspacemodel.WorkspaceActivationAdmission{}, common.ErrInvalidArgument
	}
	if current.StartupInput != nil || current.ActivationAdmission != nil {
		if current.StartupInput == nil || current.ActivationAdmission == nil || candidate.BaseDefinitionRevision == "" || candidate.BaseStartupInputHash == "" || candidate.BaseDefinitionRevision != current.StartupInput.DefinitionRevision || candidate.BaseStartupInputHash != current.StartupInput.StartupInputHash {
			return workspacemodel.Workspace{}, workspacemodel.WorkspaceStartupInputContract{}, workspacemodel.WorkspaceActivationAdmission{}, common.ErrConflict
		}
	}
	startup := startupInputFromCandidate(candidate, admissionResult)
	if err := startup.Validate(); err != nil {
		return workspacemodel.Workspace{}, workspacemodel.WorkspaceStartupInputContract{}, workspacemodel.WorkspaceActivationAdmission{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	activation := workspacemodel.WorkspaceActivationAdmission{
		WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash,
		DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible,
	}
	if blockingTicketID != "" {
		activation.State = workspacemodel.WorkspaceActivationPendingClarificationResolution
		activation.BlockingTicketID = blockingTicketID
	}
	if err := activation.ValidateCandidate(startup, candidate); err != nil {
		return workspacemodel.Workspace{}, workspacemodel.WorkspaceStartupInputContract{}, workspacemodel.WorkspaceActivationAdmission{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	next := *current
	next.StartupInput = &startup
	next.ActivationAdmission = &activation
	return next, startup, activation, nil
}

func definitionAdmissionIntentResult(candidate workspacemodel.DefinitionAdmissionCandidate, admissionResult workspacemodel.DefinitionAdmissionResult, candidateDisposition workspaceevents.DefinitionAdmissionCandidateDisposition, disposition workspaceevents.DefinitionAdmissionIntentDisposition, prior *workspaceevents.DefinitionAdmissionClarificationBinding, startup *workspacemodel.WorkspaceStartupInputContract, activation *workspacemodel.WorkspaceActivationAdmission) workspaceevents.DefinitionAdmissionIntentResult {
	result := workspaceevents.DefinitionAdmissionIntentResult{
		Candidate: candidate.Clone(), AdmissionResult: admissionResult, CandidateDisposition: candidateDisposition, Disposition: disposition,
	}
	if prior != nil {
		binding := prior.Clone()
		result.PriorClarification = &binding
	}
	if startup != nil {
		value := startup.Clone()
		result.StartupInput = &value
	}
	if activation != nil {
		value := activation.Normalized()
		result.ActivationAdmission = &value
	}
	return result.Normalized()
}

func definitionAdmissionTransitionFor(next workspacemodel.Workspace, changed bool, result workspaceevents.DefinitionAdmissionIntentResult) (definitionAdmissionTransition, error) {
	if err := result.Validate(); err != nil {
		return definitionAdmissionTransition{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	if changed {
		return definitionAdmissionTransition{Next: &next, Result: &result}, nil
	}
	return definitionAdmissionTransition{Noop: true, Result: &result}, nil
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
