package workspace

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// definitionSpecificationInternalEvent is an owner-private, typed mutation
// intent. No EventHub caller can directly select a ledger transition.
type definitionSpecificationInternalEvent struct {
	WorkspaceID           common.WorkspaceID
	Authoring             *workspaceevents.SaveDefinitionAuthoringCommand
	DeriveReserve         *workspaceevents.ReserveDefinitionDerivationCommand
	VerifiedResources     bool
	DeriveComplete        *workspaceevents.CompleteDefinitionDerivationCommand
	AuthoringSpecReserve  *workspaceevents.ReserveAuthoringSpecificationCommand
	AuthoringSpecComplete *workspaceevents.CompleteAuthoringSpecificationCommand
	PreparationDiagnostic *workspaceevents.RecordPreparationDiagnosticCommand
	Begin                 *workspaceevents.BeginDefinitionSpecificationCommand
	Reserve               *workspaceevents.ReserveDefinitionSpecificationRequestCommand
	Complete              *workspaceevents.CompleteDefinitionSpecificationRequestCommand
}

type definitionSpecificationTransition struct {
	Authoring              *workspaceevents.SaveDefinitionAuthoringResult
	Derivation             *workspaceevents.DefinitionDerivationRequestResult
	AuthoringSpecification *workspaceevents.AuthoringSpecificationRequestResult
	PreparationDiagnostic  *workspaceevents.RecordPreparationDiagnosticResult
	Next                   *workspacemodel.Workspace
	Begin                  *workspaceevents.BeginDefinitionSpecificationResult
	Request                *workspaceevents.DefinitionSpecificationRequestResult
}

func (e definitionSpecificationInternalEvent) validate() error {
	count := 0
	if e.VerifiedResources && (e.DeriveReserve == nil || len(e.DeriveReserve.Resources) == 0) {
		return common.ErrInvalidArgument
	}
	if e.DeriveReserve != nil {
		count++
		if e.DeriveReserve.Validate() != nil || e.DeriveReserve.WorkspaceID != e.WorkspaceID {
			return common.ErrInvalidArgument
		}
	}
	if e.DeriveComplete != nil {
		count++
		if e.DeriveComplete.Validate() != nil || e.DeriveComplete.WorkspaceID != e.WorkspaceID {
			return common.ErrInvalidArgument
		}
	}
	if e.AuthoringSpecReserve != nil {
		count++
		if e.AuthoringSpecReserve.Validate() != nil || e.AuthoringSpecReserve.WorkspaceID != e.WorkspaceID {
			return common.ErrInvalidArgument
		}
	}
	if e.AuthoringSpecComplete != nil {
		count++
		if e.AuthoringSpecComplete.Validate() != nil || e.AuthoringSpecComplete.WorkspaceID != e.WorkspaceID {
			return common.ErrInvalidArgument
		}
	}
	if e.PreparationDiagnostic != nil {
		count++
		if e.PreparationDiagnostic.Validate() != nil || e.PreparationDiagnostic.Diagnostic.WorkspaceID != e.WorkspaceID {
			return common.ErrInvalidArgument
		}
	}
	if e.Authoring != nil {
		count++
		if e.Authoring.Validate() != nil || e.Authoring.Input.WorkspaceID != e.WorkspaceID {
			return common.ErrInvalidArgument
		}
	}
	if e.Begin != nil {
		count++
		if e.Begin.Validate() != nil || e.Begin.Candidate.WorkspaceID != e.WorkspaceID {
			return common.ErrInvalidArgument
		}
	}
	if e.Reserve != nil {
		count++
		if e.Reserve.Validate() != nil || e.Reserve.Identity.WorkspaceID != e.WorkspaceID {
			return common.ErrInvalidArgument
		}
	}
	if e.Complete != nil {
		count++
		if e.Complete.Validate() != nil || e.Complete.Identity.WorkspaceID != e.WorkspaceID {
			return common.ErrInvalidArgument
		}
	}
	if count != 1 || e.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	return nil
}

// This dispatcher alone performs ledger CAS. Owner Goal/DoD and Template locks
// cover fact observation, pure reduction and persistence on every retry.
func (m *Biz) submitDefinitionSpecificationInternalEvent(ctx context.Context, event definitionSpecificationInternalEvent) (definitionSpecificationTransition, error) {
	if m == nil || m.manager == nil || m.workspaces == nil {
		return definitionSpecificationTransition{}, common.ErrInvalidArgument
	}
	if err := event.validate(); err != nil {
		return definitionSpecificationTransition{}, err
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	m.templateMutationMu.Lock()
	defer m.templateMutationMu.Unlock()
	bound, err := common.BindRuntimeWorkspaceContext(ctx, event.WorkspaceID)
	if err != nil {
		return definitionSpecificationTransition{}, err
	}
	for attempt := 0; attempt < 4; attempt++ {
		var observation definitionSpecificationObservation
		var err error
		if event.Authoring != nil {
			current, readErr := m.workspaces.Get(bound, event.WorkspaceID)
			if readErr != nil {
				return definitionSpecificationTransition{}, readErr
			}
			if replay, matched, err := replayDefinitionAuthoringSave(current, *event.Authoring); matched || err != nil {
				return replay, err
			}
		}
		if event.PreparationDiagnostic != nil {
			current, readErr := m.workspaces.Get(bound, event.WorkspaceID)
			if readErr != nil {
				return definitionSpecificationTransition{}, readErr
			}
			if replay, matched, replayErr := replayPreparationDiagnostic(current, *event.PreparationDiagnostic); matched || replayErr != nil {
				return replay, replayErr
			}
		}
		if event.Authoring != nil || event.DeriveReserve != nil || event.DeriveComplete != nil || event.AuthoringSpecReserve != nil || event.AuthoringSpecComplete != nil || event.PreparationDiagnostic != nil {
			observation, err = m.observeDefinitionAuthoring(bound, event.WorkspaceID)
		} else {
			observation, err = m.observeDefinitionSpecification(bound, event.WorkspaceID)
		}
		if err != nil {
			return definitionSpecificationTransition{}, err
		}
		if event.Authoring != nil || event.DeriveReserve != nil || event.AuthoringSpecReserve != nil || event.PreparationDiagnostic != nil {
			templateID := observation.workspace.TemplateID
			if event.Authoring != nil {
				templateID = event.Authoring.Input.Normalized().TemplateID
			}
			observation.template, err = m.resolvedTemplate(bound, templateID)
			if err != nil {
				return definitionSpecificationTransition{}, err
			}
		}
		transition, err := reduceDefinitionSpecificationInternalEvent(observation, event)
		if err != nil {
			return definitionSpecificationTransition{}, err
		}
		if transition.Next == nil {
			return transition, nil
		}
		if err := validateWorkspaceAuthoringMappingOutcomes(transition.Next); err != nil {
			return definitionSpecificationTransition{}, err
		}
		if err := m.workspaces.CompareAndSwap(bound, transition.Next, observation.workspace.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return definitionSpecificationTransition{}, err
		}
		if definitionSpecificationChangesAuthoringRead(event) {
			m.publishWorkspaceUpdated(bound, transition.Next)
		}
		return transition, nil
	}
	return definitionSpecificationTransition{}, common.ErrConflict
}

func definitionSpecificationChangesAuthoringRead(event definitionSpecificationInternalEvent) bool {
	return event.Authoring != nil || event.DeriveReserve != nil || event.DeriveComplete != nil ||
		event.AuthoringSpecReserve != nil || event.AuthoringSpecComplete != nil || event.PreparationDiagnostic != nil
}

// The reducer is pure: all mutable owner facts are captured by observation;
// it neither calls a provider nor reads/writes repositories or EventHub.
func reduceDefinitionSpecificationInternalEvent(o definitionSpecificationObservation, e definitionSpecificationInternalEvent) (definitionSpecificationTransition, error) {
	if err := validateWorkspaceAuthoringMappingOutcomes(o.workspace); err != nil {
		return definitionSpecificationTransition{}, err
	}
	transition, err := reduceObservedDefinitionSpecificationInternalEvent(o, e)
	if err != nil {
		return definitionSpecificationTransition{}, err
	}
	if transition.Next != nil {
		if err := validateWorkspaceAuthoringMappingOutcomes(transition.Next); err != nil {
			return definitionSpecificationTransition{}, err
		}
	}
	return transition, nil
}

func reduceObservedDefinitionSpecificationInternalEvent(o definitionSpecificationObservation, e definitionSpecificationInternalEvent) (definitionSpecificationTransition, error) {
	switch {
	case e.PreparationDiagnostic != nil:
		return reducePreparationDiagnostic(o, *e.PreparationDiagnostic)
	case e.AuthoringSpecReserve != nil || e.AuthoringSpecComplete != nil:
		return reduceAuthoringSpecification(o, e)
	case e.DeriveReserve != nil || e.DeriveComplete != nil:
		return reduceDefinitionDerivation(o, e)
	case e.Authoring != nil:
		return reduceDefinitionAuthoring(o, *e.Authoring)
	case e.Begin != nil:
		return reduceDefinitionSpecificationBegin(o, *e.Begin)
	case e.Reserve != nil:
		return reduceDefinitionSpecificationReserve(o, *e.Reserve)
	case e.Complete != nil:
		return reduceDefinitionSpecificationComplete(o, *e.Complete)
	default:
		return definitionSpecificationTransition{}, common.ErrInvalidArgument
	}
}

func reduceDefinitionSpecificationBegin(o definitionSpecificationObservation, cmd workspaceevents.BeginDefinitionSpecificationCommand) (definitionSpecificationTransition, error) {
	c := cmd.Candidate
	if o.configurationHash != cmd.ExpectedConfigurationHash || !sameSpecificationCandidateDefinition(c, o) || c.WorkspaceTemplateSnapshot.SnapshotHash != workspacemodel.WorkspaceTemplateSnapshotHash(o.template) {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if o.workspace.StartupInput == nil {
		if c.BaseDefinitionRevision != "" || c.BaseStartupInputHash != "" {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
	} else if c.BaseDefinitionRevision != o.workspace.StartupInput.DefinitionRevision || c.BaseStartupInputHash != o.workspace.StartupInput.StartupInputHash {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	current, validLineage := currentDefinitionAdmissionCandidate(o.workspace.DefinitionAdmissionCandidates, c.CandidateID)
	if len(o.workspace.DefinitionAdmissionCandidates) > 0 && (!validLineage || current == nil || c.CandidateRevision != current.CandidateRevision+1) {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if len(o.workspace.DefinitionAdmissionCandidates) == 0 && c.CandidateRevision != 1 {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	ledger, epochIdx, err := specEpochFor(o)
	if err != nil {
		return definitionSpecificationTransition{}, err
	}
	if epochIdx >= 0 {
		epoch := ledger.Epochs[epochIdx]
		inputHash := workspacemodel.DefinitionSpecificationCompilationInputHash(c.CandidateHash, cmd.Resources, cmd.Dimensions)
		if epoch.Snapshot.CandidateHash != c.CandidateHash || epoch.CompilationInputHash != inputHash || epoch.TemplateHash != workspacemodel.WorkspaceTemplateSnapshotHash(o.template) || epoch.ConfigurationHash != o.configurationHash {
			return definitionSpecificationTransition{}, fmt.Errorf("%w: same formal revision has another sealed compilation input", common.ErrConflict)
		}
		if err := validateCurrentSpecEpoch(o, epoch); err != nil {
			return definitionSpecificationTransition{}, err
		}
		sealed := epoch.Clone()
		result := workspaceevents.BeginDefinitionSpecificationResult{WorkspaceID: c.WorkspaceID, CandidateHash: c.CandidateHash, CompilationInputHash: epoch.CompilationInputHash, GoalRevision: epoch.GoalRevision, DoDRevision: epoch.DoDRevision, Snapshot: sealed.Snapshot, Resources: sealed.Resources, Dimensions: sealed.Dimensions, WorkspaceRevision: epoch.WorkspaceRevision}
		return definitionSpecificationTransition{Begin: &result}, nil
	}
	if o.workspace.Revision != cmd.ExpectedWorkspaceRevision {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if ledger.Count > 0 {
		previous := ledger.Epochs[ledger.Count-1]
		if !newerSpecificationFormalRevisions(o.goal.DefinitionRevision, o.dod.DefinitionRevision, previous) {
			return definitionSpecificationTransition{}, fmt.Errorf("%w: specification formal definition revision did not advance", common.ErrConflict)
		}
	}
	if ledger.Count == workspacemodel.DefinitionSpecificationEpochLimit {
		return definitionSpecificationTransition{}, fmt.Errorf("%w: specification epoch history exhausted (limit %d)", common.ErrConflict, workspacemodel.DefinitionSpecificationEpochLimit)
	}
	seed := (workspacemodel.GoalSpecification{Resources: cmd.Resources, Dimensions: cmd.Dimensions}).Clone()
	epoch := workspacemodel.DefinitionSpecificationEpoch{GoalRevision: o.goal.DefinitionRevision, DoDRevision: o.dod.DefinitionRevision, Snapshot: c.Clone(), Resources: seed.Resources, Dimensions: seed.Dimensions, CompilationInputHash: workspacemodel.DefinitionSpecificationCompilationInputHash(c.CandidateHash, seed.Resources, seed.Dimensions), TemplateHash: c.WorkspaceTemplateSnapshot.SnapshotHash, ConfigurationHash: o.configurationHash, WorkspaceRevision: o.workspace.Revision}
	ledger.Epochs[ledger.Count] = epoch
	ledger.Count++
	next := *o.workspace
	next.DefinitionSpecificationLedger = &ledger
	sealed := epoch.Clone()
	result := workspaceevents.BeginDefinitionSpecificationResult{WorkspaceID: c.WorkspaceID, CandidateHash: c.CandidateHash, CompilationInputHash: epoch.CompilationInputHash, GoalRevision: epoch.GoalRevision, DoDRevision: epoch.DoDRevision, Snapshot: sealed.Snapshot, Resources: sealed.Resources, Dimensions: sealed.Dimensions, WorkspaceRevision: o.workspace.Revision}
	return definitionSpecificationTransition{Next: &next, Begin: &result}, nil
}

func reduceDefinitionSpecificationReserve(o definitionSpecificationObservation, cmd workspaceevents.ReserveDefinitionSpecificationRequestCommand) (definitionSpecificationTransition, error) {
	id := cmd.Identity.Normalized()
	ledger, idx, err := specEpochFor(o)
	if err != nil {
		return definitionSpecificationTransition{}, err
	}
	if idx < 0 {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	epoch := &ledger.Epochs[idx]
	if err := validateCurrentSpecEpoch(o, *epoch); err != nil {
		return definitionSpecificationTransition{}, err
	}
	if id.CandidateID != epoch.Snapshot.CandidateID || id.CandidateRevision != epoch.Snapshot.CandidateRevision || id.CandidateHash != epoch.Snapshot.CandidateHash || id.CompilationInputHash != epoch.CompilationInputHash {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if record, err := specificationRequestRecord(epoch, id); err == nil {
		result := specRequestView(*record, *epoch, false)
		return definitionSpecificationTransition{Request: &result}, nil
	} else if !errors.Is(err, common.ErrNotFound) {
		return definitionSpecificationTransition{}, err
	}
	if epoch.Count >= workspacemodel.DefinitionSpecificationRequestLimit {
		return definitionSpecificationTransition{}, fmt.Errorf("%w: specification request budget exhausted", common.ErrConflict)
	}
	count := 0
	for n := 0; n < int(epoch.Count); n++ {
		other := epoch.Records[n].Identity
		if other.Phase == id.Phase && other.ReviewKind == id.ReviewKind {
			count++
		}
	}
	if count >= workspacemodel.DefinitionSpecificationAttemptLimit {
		return definitionSpecificationTransition{}, fmt.Errorf("%w: specification phase attempt budget exhausted", common.ErrConflict)
	}
	if id.Phase == workspacemodel.DefinitionSpecificationCritic && storedSpecificationProposal(epoch, id.ProposalHash) == nil {
		return definitionSpecificationTransition{}, fmt.Errorf("%w: critic has no completed owner proposal", common.ErrConflict)
	}
	record := workspacemodel.DefinitionSpecificationRequestRecord{Identity: id, PermitID: id.PermitID(), State: workspacemodel.DefinitionSpecificationReserved}
	epoch.Records[epoch.Count] = record
	epoch.Count++
	next := *o.workspace
	next.DefinitionSpecificationLedger = &ledger
	result := specRequestView(record, *epoch, true)
	return definitionSpecificationTransition{Next: &next, Request: &result}, nil
}

func reduceDefinitionSpecificationComplete(o definitionSpecificationObservation, cmd workspaceevents.CompleteDefinitionSpecificationRequestCommand) (definitionSpecificationTransition, error) {
	id := cmd.Identity.Normalized()
	completion := cmd.Completion.Normalized()
	ledger, idx, err := specEpochFor(o)
	if err != nil {
		return definitionSpecificationTransition{}, err
	}
	if idx < 0 {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	epoch := &ledger.Epochs[idx]
	if err := validateCurrentSpecEpoch(o, *epoch); err != nil {
		return definitionSpecificationTransition{}, err
	}
	if id.CandidateHash != epoch.Snapshot.CandidateHash || id.CompilationInputHash != epoch.CompilationInputHash || id.CandidateID != epoch.Snapshot.CandidateID || id.CandidateRevision != epoch.Snapshot.CandidateRevision {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	record, err := specificationRequestRecord(epoch, id)
	if err != nil || record.PermitID != cmd.PermitID {
		return definitionSpecificationTransition{}, common.ErrConflict
	}
	if record.State == workspacemodel.DefinitionSpecificationCompleted {
		if !reflect.DeepEqual(*record.Completion, completion) {
			return definitionSpecificationTransition{}, common.ErrConflict
		}
		result := specRequestView(*record, *epoch, false)
		return definitionSpecificationTransition{Request: &result}, nil
	}
	proposal := storedSpecificationProposal(epoch, id.ProposalHash)
	if err := completion.ValidateFor(id, epoch.Snapshot, epoch.Resources, epoch.Dimensions, proposal); err != nil {
		return definitionSpecificationTransition{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	record.State = workspacemodel.DefinitionSpecificationCompleted
	record.Completion = &completion
	next := *o.workspace
	next.DefinitionSpecificationLedger = &ledger
	result := specRequestView(*record, *epoch, false)
	return definitionSpecificationTransition{Next: &next, Request: &result}, nil
}
