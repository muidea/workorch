package workspace

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const definitionSpecificationCommandSource = definitionAdmissionCommandSource

func definitionSpecificationTopics() []string {
	return []string{workspaceevents.TopicWorkspaceDerivationReserve, workspaceevents.TopicWorkspaceDerivationComplete, workspaceevents.TopicWorkspaceAuthoringSpecificationReserve, workspaceevents.TopicWorkspaceAuthoringSpecificationComplete, workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, workspaceevents.TopicWorkspaceAuthoringDiagnosticQuery, workspaceevents.TopicWorkspaceAuthoringSuccessAuditQuery, workspaceevents.TopicWorkspacePreparationDiagnosticRecord, workspaceevents.TopicWorkspacePreparationDiagnosticQuery, workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery, workspaceevents.TopicWorkspaceDefinitionAuthoringCreate, workspaceevents.TopicWorkspaceDefinitionAuthoringSave, workspaceevents.TopicWorkspaceAuthoringConfirm, workspaceevents.TopicWorkspaceAuthoringAdmit, workspaceevents.TopicWorkspaceSpecificationBegin, workspaceevents.TopicWorkspaceSpecificationReserve, workspaceevents.TopicWorkspaceSpecificationComplete}
}

func (m *Biz) handleDefinitionSpecificationCommand(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || m == nil || m.workspaces == nil || (ev.Source() != definitionSpecificationCommandSource && ((ev.ID() != workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery || ev.Source() != "blocks/file") && ev.ID() != workspaceevents.TopicWorkspaceAuthoringDiagnosticQuery)) {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	// File verification synchronously queries this Workspace owner. Perform it
	// before the deletion ingress read lock, or a waiting writer could block the
	// nested query while this handler waits for File. A direct internal call
	// cannot set VerifiedResources through an EventHub payload.
	resourcesVerified := false
	var confirmationProof *verifiedAuthoringResources
	var admissionProof *verifiedAuthoringResources
	var derivationReserve *workspaceevents.ReserveDefinitionDerivationCommand
	if ev.ID() == workspaceevents.TopicWorkspaceDerivationReserve {
		if command, ok := ev.Data().(workspaceevents.ReserveDefinitionDerivationCommand); ok {
			command.Resources = workspacemodel.CloneDefinitionAuthoringResources(command.Resources)
			derivationReserve = &command
			if len(command.Resources) != 0 {
				if err := m.verifyAuthoringDerivationResources(ev.Context(), command); err != nil {
					result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
					return
				}
				resourcesVerified = true
			}
		}
	}
	if ev.ID() == workspaceevents.TopicWorkspaceAuthoringConfirm {
		if command, ok := ev.Data().(workspaceevents.ConfirmDefinitionAuthoringCommand); ok && command.Validate() == nil {
			proof, err := m.verifyCurrentAuthoringResources(ev.Context(), command.WorkspaceID, command.DerivationPermitID)
			if err != nil {
				result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
				return
			}
			confirmationProof = &proof
		}
	}
	if ev.ID() == workspaceevents.TopicWorkspaceAuthoringAdmit {
		if command, ok := ev.Data().(workspaceevents.AdmitDefinitionAuthoringCommand); ok && command.Validate() == nil && command.Candidate.AuthoringRef != nil {
			proof, err := m.verifyCurrentAuthoringResources(ev.Context(), command.Candidate.WorkspaceID, command.Candidate.AuthoringRef.DerivationPermitID)
			if err != nil {
				result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
				return
			}
			admissionProof = &proof
		}
	}
	m.deletionIngressMu.RLock()
	defer m.deletionIngressMu.RUnlock()
	var reply any
	var err error
	switch ev.ID() {
	case workspaceevents.TopicWorkspaceDerivationReserve:
		if derivationReserve == nil {
			err = common.ErrInvalidArgument
		} else {
			var transition definitionSpecificationTransition
			transition, err = m.submitDefinitionSpecificationInternalEvent(ev.Context(), definitionSpecificationInternalEvent{WorkspaceID: derivationReserve.WorkspaceID, DeriveReserve: derivationReserve, VerifiedResources: resourcesVerified})
			if err == nil {
				reply = *transition.Derivation
			}
		}
	case workspaceevents.TopicWorkspaceDerivationComplete:
		cmd, ok := ev.Data().(workspaceevents.CompleteDefinitionDerivationCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			var transition definitionSpecificationTransition
			transition, err = m.submitDefinitionSpecificationInternalEvent(ev.Context(), definitionSpecificationInternalEvent{WorkspaceID: cmd.WorkspaceID, DeriveComplete: &cmd})
			if err == nil {
				reply = *transition.Derivation
			}
		}
	case workspaceevents.TopicWorkspaceAuthoringSpecificationReserve:
		cmd, ok := ev.Data().(workspaceevents.ReserveAuthoringSpecificationCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			var transition definitionSpecificationTransition
			transition, err = m.submitDefinitionSpecificationInternalEvent(ev.Context(), definitionSpecificationInternalEvent{WorkspaceID: cmd.WorkspaceID, AuthoringSpecReserve: &cmd})
			if err == nil {
				reply = *transition.AuthoringSpecification
			}
		}
	case workspaceevents.TopicWorkspaceAuthoringSpecificationComplete:
		cmd, ok := ev.Data().(workspaceevents.CompleteAuthoringSpecificationCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			var transition definitionSpecificationTransition
			transition, err = m.submitDefinitionSpecificationInternalEvent(ev.Context(), definitionSpecificationInternalEvent{WorkspaceID: cmd.WorkspaceID, AuthoringSpecComplete: &cmd})
			if err == nil {
				reply = *transition.AuthoringSpecification
			}
		}
	case workspaceevents.TopicWorkspaceDefinitionAuthoringQuery:
		cmd, ok := ev.Data().(workspaceevents.QueryDefinitionAuthoringCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			reply, err = m.queryDefinitionAuthoring(ev.Context(), cmd)
		}
	case workspaceevents.TopicWorkspaceAuthoringDiagnosticQuery:
		cmd, ok := ev.Data().(workspaceevents.QueryAuthoringDiagnosticCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			reply, err = m.queryAuthoringDiagnostic(ev.Context(), cmd)
		}
	case workspaceevents.TopicWorkspaceAuthoringSuccessAuditQuery:
		cmd, ok := ev.Data().(workspaceevents.QueryAuthoringSuccessAuditCommand)
		if !ok || ev.Source() != definitionSpecificationCommandSource {
			err = common.ErrInvalidArgument
		} else {
			reply, err = m.queryAuthoringSuccessAudit(ev.Context(), cmd)
		}
	case workspaceevents.TopicWorkspacePreparationDiagnosticRecord:
		cmd, ok := ev.Data().(workspaceevents.RecordPreparationDiagnosticCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			cmd = cmd.DeepClone()
			var transition definitionSpecificationTransition
			transition, err = m.submitDefinitionSpecificationInternalEvent(ev.Context(), definitionSpecificationInternalEvent{WorkspaceID: cmd.Diagnostic.WorkspaceID, PreparationDiagnostic: &cmd})
			if err == nil {
				reply = transition.PreparationDiagnostic.DeepClone()
			}
		}
	case workspaceevents.TopicWorkspacePreparationDiagnosticQuery:
		cmd, ok := ev.Data().(workspaceevents.QueryPreparationDiagnosticCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			reply, err = m.queryPreparationDiagnostic(ev.Context(), cmd)
		}
	case workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery:
		cmd, ok := ev.Data().(workspaceevents.QueryAuthoringResourceScopeCommand)
		if !ok || ev.Source() != "blocks/file" {
			err = common.ErrInvalidArgument
		} else {
			var view workspaceevents.QueryDefinitionAuthoringResult
			view, err = m.queryDefinitionAuthoring(ev.Context(), workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: cmd.WorkspaceID})
			if err == nil {
				reply, err = workspaceevents.AuthoringResourceScopeFor(view, cmd)
			}
		}
	case workspaceevents.TopicWorkspaceDefinitionAuthoringCreate:
		cmd, ok := ev.Data().(workspaceevents.CreateDefinitionAuthoringCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			reply, err = m.createDefinitionAuthoring(ev.Context(), cmd)
		}
	case workspaceevents.TopicWorkspaceDefinitionAuthoringSave:
		cmd, ok := ev.Data().(workspaceevents.SaveDefinitionAuthoringCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			var transition definitionSpecificationTransition
			transition, err = m.submitDefinitionSpecificationInternalEvent(ev.Context(), definitionSpecificationInternalEvent{WorkspaceID: cmd.Input.WorkspaceID, Authoring: &cmd})
			if err == nil {
				reply = *transition.Authoring
			}
		}
	case workspaceevents.TopicWorkspaceAuthoringConfirm:
		cmd, ok := ev.Data().(workspaceevents.ConfirmDefinitionAuthoringCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			reply, err = m.submitDefinitionAuthoringConfirmationInternalEvent(ev.Context(), definitionAuthoringConfirmationInternalEvent{Command: cmd, VerifiedResources: confirmationProof})
		}
	case workspaceevents.TopicWorkspaceAuthoringAdmit:
		cmd, ok := ev.Data().(workspaceevents.AdmitDefinitionAuthoringCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			reply, err = m.submitDefinitionAuthoringAdmissionWithProof(ev.Context(), cmd, admissionProof)
		}
	case workspaceevents.TopicWorkspaceSpecificationBegin:
		cmd, ok := ev.Data().(workspaceevents.BeginDefinitionSpecificationCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			reply, err = m.beginDefinitionSpecification(ev.Context(), cmd)
		}
	case workspaceevents.TopicWorkspaceSpecificationReserve:
		cmd, ok := ev.Data().(workspaceevents.ReserveDefinitionSpecificationRequestCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			reply, err = m.reserveDefinitionSpecificationRequest(ev.Context(), cmd)
		}
	case workspaceevents.TopicWorkspaceSpecificationComplete:
		cmd, ok := ev.Data().(workspaceevents.CompleteDefinitionSpecificationRequestCommand)
		if !ok {
			err = common.ErrInvalidArgument
		} else {
			reply, err = m.completeDefinitionSpecificationRequest(ev.Context(), cmd)
		}
	default:
		err = common.ErrInvalidArgument
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(reply, nil)
}

type definitionSpecificationObservation struct {
	workspace         *workspacemodel.Workspace
	goal              *workspacemodel.Goal
	dod               *workspacemodel.DoD
	template          *workspacemodel.WorkspaceTemplate
	configurationHash string
}

// Call only while both owner authoring locks are held. Formal definitions and
// resolved template are owner reads, never cross-owner service/repo access.
func (m *Biz) observeDefinitionSpecification(ctx context.Context, workspaceID common.WorkspaceID) (definitionSpecificationObservation, error) {
	if m == nil || m.workspaces == nil || m.goals == nil || m.dods == nil || m.templates == nil {
		return definitionSpecificationObservation{}, common.ErrInvalidArgument
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return definitionSpecificationObservation{}, err
	}
	w, err := m.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return definitionSpecificationObservation{}, err
	}
	if err = workspaceDeletionRejectsMutation(w); err != nil {
		return definitionSpecificationObservation{}, err
	}
	if w.DefinitionAuthoring != nil || w.DefinitionAuthoringConfirmation != nil {
		return definitionSpecificationObservation{}, common.ErrConflict
	}
	goals, err := m.goals.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return definitionSpecificationObservation{}, err
	}
	goal, _, err := currentGoalDefinition(goals, workspaceID)
	if err != nil {
		return definitionSpecificationObservation{}, err
	}
	dods, err := m.dods.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return definitionSpecificationObservation{}, err
	}
	dod, _, err := currentDoDDefinition(dods, workspaceID)
	if err != nil {
		return definitionSpecificationObservation{}, err
	}
	if goal.WorkspaceID != workspaceID || dod.WorkspaceID != workspaceID || goal.DefinitionRevision == "" || dod.DefinitionRevision == "" || len(dod.Items) == 0 {
		return definitionSpecificationObservation{}, common.ErrConflict
	}
	template, err := m.resolvedTemplate(ctx, w.TemplateID)
	if err != nil {
		return definitionSpecificationObservation{}, err
	}
	if template.ID != w.TemplateID {
		return definitionSpecificationObservation{}, common.ErrConflict
	}
	return definitionSpecificationObservation{workspace: w, goal: goal, dod: dod, template: template, configurationHash: workspacemodel.WorkspaceConfigurationHash(w)}, nil
}
func sameSpecificationCandidateDefinition(snapshot workspacemodel.DefinitionAdmissionCandidate, observation definitionSpecificationObservation) bool {
	// Snapshot uses canonical Goal/DoD and template identities, not a caller
	// supplied textual hash or guessed revision.
	if snapshot.WorkspaceID != observation.workspace.ID || snapshot.GoalInput.CandidateItemRef != "goal:"+string(observation.goal.ID) || snapshot.GoalInput.SourceIdentity != "goal:"+string(observation.goal.ID)+"@"+observation.goal.DefinitionRevision || snapshot.GoalInput.Text != strings.TrimSpace(observation.goal.Text) || len(snapshot.DoDInputs) != len(observation.dod.Items) || snapshot.WorkspaceTemplateSnapshot.TemplateRef != observation.template.ID || snapshot.WorkspaceTemplateSnapshot.SnapshotHash != workspacemodel.WorkspaceTemplateSnapshotHash(observation.template) || snapshot.ResolvedProjectWorkspaceDirectory != observation.workspace.DeliverableDirectory {
		return false
	}
	for idx, item := range snapshot.DoDInputs {
		// DoD inputs are sorted by stable ref; each canonical item maps its index.
		if item.CandidateItemRef != fmt.Sprintf("dod:%s:item:%06d", observation.dod.ID, idx+1) || item.SourceIdentity != "dod:"+string(observation.dod.ID)+"@"+observation.dod.DefinitionRevision || item.Text != strings.TrimSpace(observation.dod.Items[idx]) {
			return false
		}
	}
	return true
}
func specEpochFor(observation definitionSpecificationObservation) (workspacemodel.DefinitionSpecificationRequestLedger, int, error) {
	var ledger workspacemodel.DefinitionSpecificationRequestLedger
	if observation.workspace.DefinitionSpecificationLedger != nil {
		ledger = observation.workspace.DefinitionSpecificationLedger.Clone()
		if err := ledger.Validate(observation.workspace.ID); err != nil {
			return ledger, -1, fmt.Errorf("%w: invalid persisted specification ledger: %v", common.ErrConflict, err)
		}
		for index := 0; index < int(ledger.Count); index++ {
			epoch := ledger.Epochs[index]
			if epoch.GoalRevision == observation.goal.DefinitionRevision && epoch.DoDRevision == observation.dod.DefinitionRevision {
				return ledger, index, nil
			}
		}
	}
	return ledger, -1, nil
}
func newerSpecificationFormalRevisions(goalRevision, dodRevision string, previous workspacemodel.DefinitionSpecificationEpoch) bool {
	goal, goalOK := definitionRevisionSequence("goal", goalRevision)
	previousGoal, previousGoalOK := definitionRevisionSequence("goal", previous.GoalRevision)
	dod, dodOK := definitionRevisionSequence("dod", dodRevision)
	previousDoD, previousDoDOK := definitionRevisionSequence("dod", previous.DoDRevision)
	return goalOK && previousGoalOK && dodOK && previousDoDOK && goal >= previousGoal && dod >= previousDoD && (goal > previousGoal || dod > previousDoD)
}

func validateCurrentSpecEpoch(observation definitionSpecificationObservation, epoch workspacemodel.DefinitionSpecificationEpoch) error {
	if epoch.GoalRevision != observation.goal.DefinitionRevision || epoch.DoDRevision != observation.dod.DefinitionRevision || epoch.TemplateHash != workspacemodel.WorkspaceTemplateSnapshotHash(observation.template) || epoch.ConfigurationHash != observation.configurationHash || !sameSpecificationCandidateDefinition(epoch.Snapshot, observation) || observation.workspace.Revision < epoch.WorkspaceRevision {
		return fmt.Errorf("%w: specification input became stale", common.ErrConflict)
	}
	if observation.workspace.StartupInput != nil && (epoch.Snapshot.BaseDefinitionRevision != observation.workspace.StartupInput.DefinitionRevision || epoch.Snapshot.BaseStartupInputHash != observation.workspace.StartupInput.StartupInputHash) {
		return common.ErrConflict
	}
	if observation.workspace.StartupInput == nil && (epoch.Snapshot.BaseDefinitionRevision != "" || epoch.Snapshot.BaseStartupInputHash != "") {
		return common.ErrConflict
	}
	for _, archived := range observation.workspace.DefinitionAdmissionCandidates {
		if archived.CandidateID == epoch.Snapshot.CandidateID && archived.CandidateRevision >= epoch.Snapshot.CandidateRevision {
			return fmt.Errorf("%w: compilation input no longer precedes the current candidate", common.ErrConflict)
		}
	}
	return nil
}
func (m *Biz) beginDefinitionSpecification(ctx context.Context, cmd workspaceevents.BeginDefinitionSpecificationCommand) (workspaceevents.BeginDefinitionSpecificationResult, error) {
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		return workspaceevents.BeginDefinitionSpecificationResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	transition, err := m.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: cmd.Candidate.WorkspaceID, Begin: &cmd})
	if err != nil {
		return workspaceevents.BeginDefinitionSpecificationResult{}, err
	}
	if transition.Begin == nil {
		return workspaceevents.BeginDefinitionSpecificationResult{}, common.ErrConflict
	}
	return *transition.Begin, nil
}

func specificationRequestRecord(epoch *workspacemodel.DefinitionSpecificationEpoch, id workspacemodel.DefinitionSpecificationRequestIdentity) (*workspacemodel.DefinitionSpecificationRequestRecord, error) {
	for n := 0; n < int(epoch.Count); n++ {
		record := &epoch.Records[n]
		if record.Identity.LogicalRequestID == id.LogicalRequestID {
			if record.Identity != id {
				return nil, common.ErrConflict
			}
			return record, nil
		}
	}
	return nil, common.ErrNotFound
}
func storedSpecificationProposal(epoch *workspacemodel.DefinitionSpecificationEpoch, hash string) *workspacemodel.GoalSpecification {
	for n := 0; n < int(epoch.Count); n++ {
		r := epoch.Records[n]
		if r.Identity.Phase == workspacemodel.DefinitionSpecificationProposal && r.State == workspacemodel.DefinitionSpecificationCompleted && r.Completion != nil && r.Completion.Proposal != nil && r.Completion.Proposal.ContentHash() == hash {
			return r.Completion.Proposal
		}
	}
	return nil
}
func specRequestView(r workspacemodel.DefinitionSpecificationRequestRecord, epoch workspacemodel.DefinitionSpecificationEpoch, permit bool) workspaceevents.DefinitionSpecificationRequestResult {
	response := workspaceevents.DefinitionSpecificationRequestResult{Identity: r.Identity, Consumed: epoch.Count, Remaining: workspacemodel.DefinitionSpecificationRequestLimit - epoch.Count}
	switch {
	case permit:
		response.Disposition = workspaceevents.DefinitionSpecificationPermitted
		response.PermitID = r.PermitID
	case r.State == workspacemodel.DefinitionSpecificationCompleted:
		response.Disposition = workspaceevents.DefinitionSpecificationAlreadyCompleted
		completed := r.Completion.Clone()
		response.Completed = &completed
	default:
		response.Disposition = workspaceevents.DefinitionSpecificationAlreadyReserved
	}
	return response
}
func (m *Biz) reserveDefinitionSpecificationRequest(ctx context.Context, cmd workspaceevents.ReserveDefinitionSpecificationRequestCommand) (workspaceevents.DefinitionSpecificationRequestResult, error) {
	if err := cmd.Validate(); err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	cmd.Identity = cmd.Identity.Normalized()
	transition, err := m.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: cmd.Identity.WorkspaceID, Reserve: &cmd})
	if err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, err
	}
	if transition.Request == nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, common.ErrConflict
	}
	return *transition.Request, nil
}
func (m *Biz) completeDefinitionSpecificationRequest(ctx context.Context, cmd workspaceevents.CompleteDefinitionSpecificationRequestCommand) (workspaceevents.DefinitionSpecificationRequestResult, error) {
	if err := cmd.Validate(); err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	cmd.Identity = cmd.Identity.Normalized()
	cmd.Completion = cmd.Completion.Normalized()
	transition, err := m.submitDefinitionSpecificationInternalEvent(ctx, definitionSpecificationInternalEvent{WorkspaceID: cmd.Identity.WorkspaceID, Complete: &cmd})
	if err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, err
	}
	if transition.Request == nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, common.ErrConflict
	}
	return *transition.Request, nil
}
func (m *Biz) queryDefinitionSpecificationRequest(ctx context.Context, cmd workspaceevents.QueryDefinitionSpecificationRequestCommand) (workspaceevents.DefinitionSpecificationRequestResult, error) {
	if err := cmd.Validate(); err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	id := cmd.Identity.Normalized()
	if m == nil || m.manager == nil || m.workspaces == nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, common.ErrInvalidArgument
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	m.templateMutationMu.Lock()
	defer m.templateMutationMu.Unlock()
	bound, err := common.BindRuntimeWorkspaceContext(ctx, id.WorkspaceID)
	if err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, err
	}
	o, err := m.observeDefinitionSpecification(bound, id.WorkspaceID)
	if err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, err
	}
	ledger, idx, err := specEpochFor(o)
	if err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, err
	}
	if idx < 0 {
		return workspaceevents.DefinitionSpecificationRequestResult{}, common.ErrConflict
	}
	epoch := &ledger.Epochs[idx]
	if err := validateCurrentSpecEpoch(o, *epoch); err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, err
	}
	if id.CandidateID != epoch.Snapshot.CandidateID || id.CandidateRevision != epoch.Snapshot.CandidateRevision || id.CandidateHash != epoch.Snapshot.CandidateHash || id.CompilationInputHash != epoch.CompilationInputHash {
		return workspaceevents.DefinitionSpecificationRequestResult{}, common.ErrConflict
	}
	record, err := specificationRequestRecord(epoch, id)
	if err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, err
	}
	return specRequestView(*record, *epoch, false), nil
}

func (m *Biz) validateCompilationAdmission(ctx context.Context, current *workspacemodel.Workspace, command workspaceevents.DefinitionAdmissionIntentCommand) error {
	candidate := command.Candidate
	if current == nil || candidate.WorkspaceID != current.ID {
		return common.ErrConflict
	}
	for _, archived := range current.DefinitionAdmissionCandidates {
		if archived.CandidateID == candidate.CandidateID && archived.CandidateRevision == candidate.CandidateRevision && archived.CandidateHash != candidate.CandidateHash {
			return common.ErrConflict
		}
	}
	// A completed archive is replayed by its sealed candidate hash. Rechecking
	// live owner facts would rewrite history after an unrelated definition edit.
	for _, archived := range current.DefinitionAdmissionCandidates {
		if archived.CandidateID == candidate.CandidateID && archived.CandidateRevision == candidate.CandidateRevision && archived.CandidateHash == candidate.CandidateHash {
			return nil
		}
	}
	ledger := current.DefinitionSpecificationLedger
	if ledger == nil {
		if candidate.CompilationRef != nil {
			return common.ErrConflict
		}
		return nil
	}
	if err := ledger.Validate(current.ID); err != nil {
		return fmt.Errorf("%w: invalid specification ledger: %v", common.ErrConflict, err)
	}
	for index := int(ledger.Count) - 1; index >= 0; index-- {
		epoch := ledger.Epochs[index]
		if epoch.Snapshot.CandidateID != candidate.CandidateID || epoch.Snapshot.CandidateRevision != candidate.CandidateRevision {
			continue
		}
		if candidate.CompilationRef == nil {
			return fmt.Errorf("%w: pre-seal compiled candidate cannot bypass review", common.ErrConflict)
		}
		if index != int(ledger.Count)-1 {
			return fmt.Errorf("%w: compiled candidate references stale formal definition", common.ErrConflict)
		}
		if epoch.Snapshot.CandidateHash != candidate.CompilationRef.InputCandidateHash || epoch.CompilationInputHash != candidate.CompilationRef.CompilationInputHash {
			return common.ErrConflict
		}
		// submitDefinitionAdmissionIntent holds definitionMu and the existing
		// current-template path holds templateMutationMu through its CAS.
		if !command.CurrentTemplate {
			return fmt.Errorf("%w: compiled admission requires current Template fence", common.ErrConflict)
		}
		observation, err := m.observeDefinitionSpecification(ctx, current.ID)
		if err != nil {
			return err
		}
		if err := validateCurrentSpecEpoch(observation, epoch); err != nil {
			return err
		}
		return verifyCompiledSpecificationCandidate(candidate, epoch)
	}
	if candidate.CompilationRef != nil {
		return common.ErrConflict
	}
	// A current owner compilation epoch cannot be side-stepped by submitting
	// another candidate revision or ID before its completed review is admitted.
	if ledger.Count > 0 {
		latest := ledger.Epochs[ledger.Count-1]
		observation, err := m.observeDefinitionSpecification(ctx, current.ID)
		if err != nil {
			return err
		}
		if observation.goal.DefinitionRevision == latest.GoalRevision && observation.dod.DefinitionRevision == latest.DoDRevision {
			return fmt.Errorf("%w: active specification compilation requires exact reviewed candidate", common.ErrConflict)
		}
	}
	return nil
}

func verifyCompiledSpecificationCandidate(candidate workspacemodel.DefinitionAdmissionCandidate, epoch workspacemodel.DefinitionSpecificationEpoch) error {
	ref := candidate.CompilationRef
	if ref == nil || ref.Validate() != nil || candidate.Specification == nil || ref.InputCandidateHash != epoch.Snapshot.CandidateHash || ref.CompilationInputHash != epoch.CompilationInputHash || candidate.WorkspaceID != epoch.Snapshot.WorkspaceID || candidate.CandidateID != epoch.Snapshot.CandidateID || candidate.CandidateRevision != epoch.Snapshot.CandidateRevision {
		return common.ErrConflict
	}
	plain := candidate.Clone()
	plain.Specification = nil
	plain.CompilationRef = nil
	plain = plain.WithHash()
	if plain.CandidateHash != epoch.Snapshot.CandidateHash || candidate.Specification.ContentHash() != ref.ProposalHash {
		return common.ErrConflict
	}
	var proposal *workspacemodel.GoalSpecification
	proposalIndex := -1
	for n := 0; n < int(epoch.Count); n++ {
		r := epoch.Records[n]
		if r.PermitID == ref.ProposalPermitID && r.State == workspacemodel.DefinitionSpecificationCompleted && r.Identity.Phase == workspacemodel.DefinitionSpecificationProposal && r.Completion != nil && r.Completion.Proposal != nil && r.Completion.Proposal.ContentHash() == ref.ProposalHash {
			proposal = r.Completion.Proposal
			proposalIndex = n
			break
		}
	}
	if proposal == nil {
		return common.ErrConflict
	}
	spec := candidate.Specification.Clone()
	spec.Reviews = nil
	if !reflect.DeepEqual(spec.Normalized(), proposal.Normalized()) {
		return common.ErrConflict
	}
	// A later successfully completed proposal supersedes earlier proposals in
	// this epoch. Reusing an obsolete proposal cannot produce an admission.
	for n := proposalIndex + 1; n < int(epoch.Count); n++ {
		r := epoch.Records[n]
		if r.Identity.Phase == workspacemodel.DefinitionSpecificationProposal && r.State == workspacemodel.DefinitionSpecificationCompleted && r.Completion != nil && r.Completion.Proposal != nil {
			return common.ErrConflict
		}
	}
	kinds := []string{workspacemodel.GoalReviewDimensionCoverage, workspacemodel.GoalReviewNegativePath, workspacemodel.GoalReviewRegression, workspacemodel.GoalReviewResourceEnvironment, workspacemodel.GoalReviewTraceability, workspacemodel.GoalReviewVerifierFeasibility}
	for index, kind := range kinds {
		found := false
		for n := 0; n < int(epoch.Count); n++ {
			r := epoch.Records[n]
			if r.PermitID != ref.CriticPermitIDs[index] || r.Identity.Phase != workspacemodel.DefinitionSpecificationCritic || r.Identity.ReviewKind != kind || r.Identity.ProposalHash != ref.ProposalHash || r.State != workspacemodel.DefinitionSpecificationCompleted || r.Completion == nil || r.Completion.Review == nil {
				continue
			}
			for _, review := range candidate.Specification.Reviews {
				if review.Kind == kind && reflect.DeepEqual(review, *r.Completion.Review) {
					found = true
					break
				}
			}
		}
		if !found {
			return common.ErrConflict
		}
	}
	return nil
}
