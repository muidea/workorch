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
	Template    *workspacemodel.WorkspaceTemplate
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
	if event.Intent.CurrentTemplate {
		baseline := uint64(0)
		if current.ActivationAdmission != nil {
			baseline = current.ActivationAdmission.CandidateRevision
		}
		for _, archived := range current.DefinitionAdmissionCandidates {
			if archived.CandidateRevision > baseline && archived.Validate() != nil {
				return definitionAdmissionTransition{}, fmt.Errorf("%w: unresolved admission candidate has no current clarification contract", common.ErrConflict)
			}
		}
		if event.Template == nil || event.Template.ID != current.TemplateID ||
			workspacemodel.WorkspaceTemplateSnapshotHash(event.Template) != event.Intent.Candidate.WorkspaceTemplateSnapshot.SnapshotHash {
			return definitionAdmissionTransition{}, fmt.Errorf("%w: current Template changed during admission", common.ErrConflict)
		}
		latest, _ := currentDefinitionAdmissionCandidate(current.DefinitionAdmissionCandidates, event.Intent.Candidate.CandidateID)
		exactReplay := latest != nil && latest.CandidateHash == event.Intent.Candidate.CandidateHash && latest.CandidateRevision == event.Intent.Candidate.CandidateRevision
		if !exactReplay && (event.Intent.ExpectedWorkspaceRevision == 0 || event.Intent.ExpectedWorkspaceRevision != current.Revision) {
			return definitionAdmissionTransition{}, common.ErrConflict
		}
		if err := validateArchivedAdmissionIdentity(current); err != nil {
			return definitionAdmissionTransition{}, err
		}
		var err error
		current, err = workspacemodel.WorkspaceForCurrentTemplate(current, event.Template)
		if err != nil {
			return definitionAdmissionTransition{}, err
		}
		if event.Intent.ExpectedConfigurationHash == "" || event.Intent.ExpectedConfigurationHash != workspacemodel.WorkspaceConfigurationHash(current) {
			return definitionAdmissionTransition{}, fmt.Errorf("%w: Workspace configuration changed during admission", common.ErrConflict)
		}
	} else if _, _, err := current.ValidateActivationAdmissionPersistence(); err != nil {
		return definitionAdmissionTransition{}, fmt.Errorf("%w: invalid current definition admission: %v", common.ErrConflict, err)
	}
	return reduceDefinitionAdmissionIntent(current, *event.Intent)
}

// Old admission is a lineage anchor, not an executable configuration. Check
// scope and identity without running today's compiler against archived input.
// Partial/missing/conflicting records remain corruption, never migration input.
func validateArchivedAdmissionIdentity(current *workspacemodel.Workspace) error {
	if current.StartupInput == nil && current.ActivationAdmission == nil {
		return nil
	}
	if current.StartupInput == nil || current.ActivationAdmission == nil {
		return common.ErrConflict
	}
	s, a := current.StartupInput, current.ActivationAdmission
	if s.WorkspaceID != current.ID || a.WorkspaceID != current.ID || s.StartupInputHash == "" ||
		a.StartupInputHash != s.StartupInputHash || a.DefinitionRevision != s.DefinitionRevision {
		return common.ErrConflict
	}
	matched := 0
	for _, candidate := range current.DefinitionAdmissionCandidates {
		if candidate.WorkspaceID == current.ID && candidate.CandidateID == a.CandidateID &&
			candidate.CandidateRevision == a.CandidateRevision && candidate.CandidateHash == a.CandidateHash {
			matched++
		}
	}
	if matched != 1 {
		return common.ErrConflict
	}
	if a.State != workspacemodel.WorkspaceActivationEligible {
		// An unresolved ticket remains an active owner obligation. It cannot be
		// treated as settled archive or silently cleared by a configuration edit.
		if _, _, err := current.ValidateActivationAdmissionPersistence(); err != nil {
			return common.ErrConflict
		}
	}
	return nil
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
	if current.ActivationAdmission != nil && current.ActivationAdmission.State == workspacemodel.WorkspaceActivationPendingClarificationResolution && priorClarification == nil {
		return definitionAdmissionTransition{}, common.ErrConflict
	}

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
	latestHasSpecification := false
	for _, existing := range current.DefinitionAdmissionCandidates {
		if !command.CurrentTemplate {
			existing = existing.Normalized()
		}
		if !command.CurrentTemplate || existing.CandidateRevision == candidate.CandidateRevision {
			if err := existing.Validate(); err != nil {
				return workspacemodel.Workspace{}, "", false, common.ErrConflict
			}
		}
		if existing.WorkspaceID != current.ID || existing.CandidateID == "" || existing.CandidateRevision == 0 || existing.CandidateHash == "" {
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
			latestHasSpecification = existing.Specification != nil
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
	// The latest immutable authoring attempt cannot lose a submitted
	// specification by bypassing the Application adapter. Archived legacy
	// candidates remain readable; only a new successor is subject to this gate.
	if latestHasSpecification && candidate.Specification == nil {
		return workspacemodel.Workspace{}, "", false, common.ErrConflict
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
	// Provenance validation also reads the resolved Template when an active
	// compilation epoch exists. Keep that read and the admission CAS under the
	// same owner mutation fence, even on a legacy CurrentTemplate=false intent.
	// The caller already holds definitionMu, so the lock order remains
	// definitionMu -> templateMutationMu across every admission path.
	m.templateMutationMu.Lock()
	defer m.templateMutationMu.Unlock()
	if event.Intent.CurrentTemplate {
		if m.templates == nil {
			return definitionAdmissionTransition{}, common.ErrInvalidArgument
		}
		event.Template, err = m.resolvedTemplate(ctx, event.Intent.Candidate.WorkspaceTemplateSnapshot.TemplateRef)
		if err != nil {
			return definitionAdmissionTransition{}, err
		}
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
		if err := m.validateCompilationAdmission(bound, current, *event.Intent); err != nil {
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
		m.publishWorkspaceUpdated(bound, transition.Next)
		return transition, nil
	}
	return definitionAdmissionTransition{}, common.ErrConflict
}
