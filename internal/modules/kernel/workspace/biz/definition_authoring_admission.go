package workspace

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
)

// The legacy candidate ingress cannot treat a self-consistent but unpersisted
// set of PASS reviews as authority. Admission enters the shared owner-private
// dispatcher; its source check runs under the definition and Template locks.
func (m *Biz) submitDefinitionAuthoringAdmission(ctx context.Context, command workspaceevents.AdmitDefinitionAuthoringCommand) (workspaceevents.AdmitDefinitionAuthoringResult, error) {
	return m.submitDefinitionAuthoringAdmissionWithProof(ctx, command, nil)
}

func (m *Biz) submitDefinitionAuthoringAdmissionWithProof(ctx context.Context, command workspaceevents.AdmitDefinitionAuthoringCommand, proof *verifiedAuthoringResources) (workspaceevents.AdmitDefinitionAuthoringResult, error) {
	if m == nil || m.manager == nil || ctx == nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrInvalidArgument
	}
	if err := command.Validate(); err != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	transition, err := m.submitDefinitionAdmissionInternalEvent(ctx, definitionAdmissionInternalEvent{WorkspaceID: command.Candidate.WorkspaceID, Authoring: &command, VerifiedResources: proof})
	if err != nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, err
	}
	if transition.AuthoringResult == nil {
		return workspaceevents.AdmitDefinitionAuthoringResult{}, common.ErrConflict
	}
	return *transition.AuthoringResult, nil
}

func (m *Biz) verifyConfirmedAuthoringAdmission(ctx context.Context, current *workspacemodel.Workspace, candidate workspacemodel.DefinitionAdmissionCandidate, command workspaceevents.AdmitDefinitionAuthoringCommand, proof *verifiedAuthoringResources) error {
	if current == nil || current.DefinitionAuthoring == nil || current.DefinitionAuthoringConfirmation == nil || current.DefinitionAuthoringConfirmation.Phase != workspacemodel.DefinitionAuthoringConfirmationMaterialized || current.ActiveRunID != "" || current.DefinitionSpecificationLedger != nil || candidate.AuthoringRef == nil || candidate.CompilationRef != nil || candidate.BaseDefinitionRevision != "" || candidate.BaseStartupInputHash != "" || candidate.CandidateRevision != uint64(len(current.DefinitionAuthoringHistory)+1) || current.ValidateDefinitionAuthoringHistory() != nil {
		return common.ErrConflict
	}
	if err := validateWorkspaceAuthoringMappingOutcomes(current); err != nil {
		return err
	}
	if _, _, err := current.ValidateActivationAdmissionPersistence(); err != nil {
		return common.ErrConflict
	}
	if len(current.DefinitionAuthoringHistory) == 0 {
		if candidate.CandidateID != "authoring-"+workspacemodel.StableDefinitionAuthoringConfirmationID(current.ID, candidate.AuthoringRef.InputHash, candidate.AuthoringRef.DerivationPermitID, candidate.AuthoringRef.SpecificationHash) {
			return common.ErrConflict
		}
	} else if candidate.CandidateID != current.DefinitionAdmissionCandidates[0].CandidateID || current.DefinitionAuthoringConfirmation.ValidateSuccessor(current.DefinitionAuthoringHistory[len(current.DefinitionAuthoringHistory)-1].Confirmation) != nil {
		return common.ErrConflict
	}
	journal := *current.DefinitionAuthoringConfirmation
	if journal.Validate(current.DefinitionAuthoring, current.ID) != nil || candidate.AuthoringRef.InputHash != journal.InputHash || candidate.AuthoringRef.DerivationPermitID != journal.DerivationPermitID || candidate.AuthoringRef.ProposalHash != journal.ProposalHash || candidate.AuthoringRef.SpecificationHash != journal.SpecificationHash || command.ExpectedConfigurationHash != journal.ConfigurationHash || workspacemodel.WorkspaceConfigurationHash(current) != journal.ConfigurationHash {
		return common.ErrConflict
	}
	derivation, _, err := current.DefinitionAuthoring.ConfirmableSpecification(journal.DerivationPermitID, journal.ProposalHash)
	if err != nil || journal.ResourceSnapshotHash != derivation.ResourceSnapshotHash || (len(derivation.Resources) != 0 && (proof == nil || !proof.matches(current, derivation))) {
		return fmt.Errorf("%w: resource-bound admission requires current File verification", common.ErrConflict)
	}
	goal, dod, spec, err := workspacemodel.ConfirmedAuthoringCandidateInputs(current.DefinitionAuthoring, journal, current.ID)
	if err != nil || candidate.Specification == nil || candidate.GoalInput != goal || !reflect.DeepEqual(candidate.DoDInputs, workspacemodel.DefinitionAdmissionCandidate{DoDInputs: dod}.Normalized().DoDInputs) || !reflect.DeepEqual(candidate.Specification.Normalized(), spec.Normalized()) {
		return fmt.Errorf("%w: candidate differs from original six reviews", common.ErrConflict)
	}
	goals, err := m.goals.ListByWorkspaceID(ctx, current.ID)
	if err != nil || len(goals) != len(current.DefinitionAuthoringHistory)+1 || currentGoalLeaf(goals, current.ID) == nil || !reflect.DeepEqual(*currentGoalLeaf(goals, current.ID), journal.Goal) {
		return common.ErrConflict
	}
	dods, err := m.dods.ListByWorkspaceID(ctx, current.ID)
	if err != nil || len(dods) != len(current.DefinitionAuthoringHistory)+1 || currentDoDLeaf(dods, current.ID) == nil || !reflect.DeepEqual(*currentDoDLeaf(dods, current.ID), journal.DoD) {
		return common.ErrConflict
	}
	template, err := m.resolvedTemplate(ctx, current.TemplateID)
	if err != nil || workspacemodel.WorkspaceTemplateSnapshotHash(template) != journal.TemplateSnapshotHash || candidate.WorkspaceTemplateSnapshot.TemplateRef != current.TemplateID || candidate.WorkspaceTemplateSnapshot.SnapshotHash != journal.TemplateSnapshotHash || candidate.WorkspaceTemplateSnapshot.Revision != "template:"+journal.TemplateSnapshotHash || candidate.ResolvedProjectWorkspaceDirectory != current.DeliverableDirectory {
		return common.ErrConflict
	}
	bindings, err := definitionconfiguration.Compile(current, template)
	if err != nil {
		return fmt.Errorf("%w: current Workspace and Template configuration is invalid: %v", common.ErrConflict, err)
	}
	if err := definitionadmission.ValidateAuthoringVerificationApplicability(spec, dod, bindings.ProfileContracts); err != nil {
		return fmt.Errorf("%w: confirmed verification is not applicable to current profiles: %v", common.ErrConflict, err)
	}
	fromOwner := workspacemodel.DefinitionAdmissionCandidate{
		AdmissionPolicyBindings:      bindings.PolicyBindings,
		RuntimeProfileContracts:      bindings.ProfileContracts,
		RequiredOwnerFactBindings:    bindings.RequiredOwnerFacts,
		OwnerFactConvergenceBindings: bindings.OwnerFactConvergences,
	}.Normalized()
	candidate = candidate.Normalized()
	for _, comparison := range []struct {
		name  string
		equal bool
	}{
		{"policy", reflect.DeepEqual(candidate.AdmissionPolicyBindings, fromOwner.AdmissionPolicyBindings)},
		{"profile", reflect.DeepEqual(candidate.RuntimeProfileContracts, fromOwner.RuntimeProfileContracts)},
		{"owner facts", reflect.DeepEqual(candidate.RequiredOwnerFactBindings, fromOwner.RequiredOwnerFactBindings)},
		{"owner fact convergence", reflect.DeepEqual(candidate.OwnerFactConvergenceBindings, fromOwner.OwnerFactConvergenceBindings)},
		{"profile hash", candidate.RuntimeProfileContractsHash == bindings.ProfileContractsHash},
	} {
		if !comparison.equal {
			return fmt.Errorf("%w: candidate %s differs from current Workspace and Template", common.ErrConflict, comparison.name)
		}
	}
	if candidate.RuntimeProfileContractsHash == "" || candidate.AdmissionCompilerVersion != workspacemodel.DefinitionAdmissionCompilerVersionCurrent || len(candidate.ProposedRelationInputs) != 1 {
		return common.ErrConflict
	}
	relation := candidate.ProposedRelationInputs[0]
	if relation.RelationKind != "all_of" || relation.RelationRef != "terminal-all-of" || len(relation.SourceItemRefs) != len(dod) || len(relation.TargetItemRefs) != 1 || relation.TargetItemRefs[0] != goal.CandidateItemRef {
		return common.ErrConflict
	}
	expectedRefs := make([]string, len(dod))
	for index, item := range dod {
		expectedRefs[index] = item.CandidateItemRef
	}
	sort.Strings(expectedRefs)
	if !reflect.DeepEqual(relation.SourceItemRefs, expectedRefs) {
		return fmt.Errorf("%w: all-of source refs mismatch", common.ErrConflict)
	}
	return nil
}
