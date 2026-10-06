package workspace

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// definitionAuthoringConfirmationInternalEvent is the sole owner-private
// selector of the confirmation journal. The public command names an exact
// persisted result; it cannot submit Goal/DoD after-images or eligibility.
type definitionAuthoringConfirmationInternalEvent struct {
	Command           workspaceevents.ConfirmDefinitionAuthoringCommand
	VerifiedResources *verifiedAuthoringResources
}

func (m *Biz) submitDefinitionAuthoringConfirmationInternalEvent(ctx context.Context, event definitionAuthoringConfirmationInternalEvent) (workspaceevents.ConfirmDefinitionAuthoringResult, error) {
	cmd := event.Command
	if m == nil || m.manager == nil || m.workspaces == nil || m.goals == nil || m.dods == nil || m.templates == nil || ctx == nil || cmd.Validate() != nil {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, common.ErrInvalidArgument
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	m.templateMutationMu.Lock()
	defer m.templateMutationMu.Unlock()
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
	}
	for attempt := 0; attempt < 4; attempt++ {
		current, err := m.workspaces.Get(bound, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
		}
		if err := workspaceDeletionRejectsMutation(current); err != nil {
			return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
		}
		if current.DefinitionAuthoringConfirmation != nil {
			if err := validateConfirmationResourceProof(current, event.VerifiedResources); err != nil {
				return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
			}
			return m.driveDefinitionAuthoringConfirmation(bound, cmd, current)
		}
		journal, err := m.prepareDefinitionAuthoringConfirmationWithProof(bound, current, cmd, event.VerifiedResources)
		if err != nil {
			return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
		}
		next := *current
		next.DefinitionAuthoringConfirmation = &journal
		if err := m.workspaces.CompareAndSwap(bound, &next, current.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
		}
		return m.driveDefinitionAuthoringConfirmation(bound, cmd, &next)
	}
	return workspaceevents.ConfirmDefinitionAuthoringResult{}, common.ErrConflict
}

func (m *Biz) prepareDefinitionAuthoringConfirmation(ctx context.Context, current *workspace.Workspace, cmd workspaceevents.ConfirmDefinitionAuthoringCommand) (workspace.DefinitionAuthoringConfirmation, error) {
	return m.prepareDefinitionAuthoringConfirmationWithProof(ctx, current, cmd, nil)
}

func (m *Biz) prepareDefinitionAuthoringConfirmationWithProof(ctx context.Context, current *workspace.Workspace, cmd workspaceevents.ConfirmDefinitionAuthoringCommand, proof *verifiedAuthoringResources) (workspace.DefinitionAuthoringConfirmation, error) {
	if current == nil || current.ID != cmd.WorkspaceID || current.Revision != cmd.ExpectedWorkspaceRevision || current.DefinitionAuthoring == nil || current.StartupInput != nil || current.ActivationAdmission != nil || current.ActiveRunID != "" || current.DefinitionSpecificationLedger != nil || len(current.DefinitionAdmissionCandidates) > 0 && !authoringSuccessorDraft(current) {
		return workspace.DefinitionAuthoringConfirmation{}, common.ErrConflict
	}
	record := current.DefinitionAuthoring
	if err := record.Validate(); err != nil || record.InputHash != cmd.InputHash {
		return workspace.DefinitionAuthoringConfirmation{}, common.ErrConflict
	}
	derivation, specification, err := record.ConfirmableSpecification(cmd.DerivationPermitID, cmd.ProposalHash)
	if err != nil || specification.AdmissionHash() != cmd.SpecificationHash {
		return workspace.DefinitionAuthoringConfirmation{}, common.ErrConflict
	}
	if len(derivation.Resources) != 0 && (proof == nil || !proof.matches(current, derivation)) {
		return workspace.DefinitionAuthoringConfirmation{}, fmt.Errorf("%w: resource-bound confirmation requires current File verification", common.ErrConflict)
	}
	template, err := m.resolvedTemplate(ctx, current.TemplateID)
	if err != nil || workspace.WorkspaceTemplateSnapshotHash(template) != record.TemplateSnapshotHash {
		return workspace.DefinitionAuthoringConfirmation{}, common.ErrConflict
	}
	if err := validateAuthoringVerificationAgainstSealedTemplate(record.TemplateSnapshotHash, derivation, specification); err != nil {
		return workspace.DefinitionAuthoringConfirmation{}, err
	}
	goals, err := m.goals.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspace.DefinitionAuthoringConfirmation{}, err
	}
	dods, err := m.dods.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspace.DefinitionAuthoringConfirmation{}, err
	}
	if len(goals) != 0 || len(dods) != 0 {
		if !authoringSuccessorDraft(current) || len(goals) != len(current.DefinitionAuthoringHistory) || len(dods) != len(current.DefinitionAuthoringHistory) {
			return workspace.DefinitionAuthoringConfirmation{}, common.ErrConflict
		}
		last := current.DefinitionAuthoringHistory[len(current.DefinitionAuthoringHistory)-1].Confirmation
		if currentGoalLeaf(goals, current.ID) == nil || currentDoDLeaf(dods, current.ID) == nil || !reflect.DeepEqual(*currentGoalLeaf(goals, current.ID), last.Goal) || !reflect.DeepEqual(*currentDoDLeaf(dods, current.ID), last.DoD) {
			return workspace.DefinitionAuthoringConfirmation{}, common.ErrConflict
		}
	}
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil {
		return workspace.DefinitionAuthoringConfirmation{}, err
	}
	texts := make([]string, 0, len(items))
	for _, item := range items {
		texts = append(texts, item.Text)
	}
	identity := workspace.StableDefinitionAuthoringConfirmationID(cmd.WorkspaceID, cmd.InputHash, cmd.DerivationPermitID, cmd.SpecificationHash)
	sequence := uint64(len(current.DefinitionAuthoringHistory) + 1)
	journal := workspace.DefinitionAuthoringConfirmation{
		Phase: workspace.DefinitionAuthoringConfirmationPrepared, InputHash: cmd.InputHash,
		DerivationPermitID: cmd.DerivationPermitID, ProposalHash: cmd.ProposalHash,
		SpecificationHash: cmd.SpecificationHash, TemplateSnapshotHash: record.TemplateSnapshotHash,
		ResourceSnapshotHash: derivation.ResourceSnapshotHash,
		ConfigurationHash:    workspace.WorkspaceConfigurationHash(current), FormalSequence: sequence,
		Goal: workspace.Goal{ID: common.GoalID("authoring-goal-" + identity), WorkspaceID: current.ID, Text: record.Input.Normalized().GoalText, DefinitionRevision: definitionRevisionForText("goal", sequence, record.Input.GoalText), LineageState: "active"},
		DoD:  workspace.DoD{ID: common.DoDID("authoring-dod-" + identity), WorkspaceID: current.ID, Items: texts, State: common.DoDStateReady, DefinitionRevision: definitionRevisionForItems("dod", sequence, texts), LineageState: "active"},
	}
	if sequence > 1 {
		previous := current.DefinitionAuthoringHistory[len(current.DefinitionAuthoringHistory)-1].Confirmation
		journal.PreviousInputHash = previous.InputHash
		journal.Goal.PreviousGoalID = previous.Goal.ID
		journal.Goal.PreviousDefinitionHash = definitionHashForText(previous.Goal.Text)
		journal.DoD.PreviousDoDID = previous.DoD.ID
		journal.DoD.PreviousDefinitionHash = definitionHashForItems(previous.DoD.Items)
		if journal.ValidateSuccessor(previous) != nil {
			return workspace.DefinitionAuthoringConfirmation{}, common.ErrConflict
		}
	}
	if err := journal.Validate(record, current.ID); err != nil || journal.ConfigurationHash == "" {
		return workspace.DefinitionAuthoringConfirmation{}, common.ErrConflict
	}
	return journal, nil
}

func validateConfirmationResourceProof(current *workspace.Workspace, proof *verifiedAuthoringResources) error {
	if current == nil || current.DefinitionAuthoring == nil || current.DefinitionAuthoringConfirmation == nil {
		return common.ErrConflict
	}
	journal := current.DefinitionAuthoringConfirmation
	for _, request := range current.DefinitionAuthoring.DerivationRequests {
		if request.PermitID != journal.DerivationPermitID {
			continue
		}
		if journal.ResourceSnapshotHash != request.ResourceSnapshotHash || (len(request.Resources) != 0 && (proof == nil || !proof.matches(current, request))) {
			return common.ErrConflict
		}
		return nil
	}
	return common.ErrConflict
}

// Each cutpoint reads the persisted journal and verifies the exact after-image.
// A failed write remains replayable; a conflicting row is never overwritten.
func (m *Biz) driveDefinitionAuthoringConfirmation(ctx context.Context, cmd workspaceevents.ConfirmDefinitionAuthoringCommand, initial *workspace.Workspace) (workspaceevents.ConfirmDefinitionAuthoringResult, error) {
	if initial == nil || initial.DefinitionAuthoring == nil || initial.DefinitionAuthoringConfirmation == nil {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, common.ErrConflict
	}
	journal := initial.DefinitionAuthoringConfirmation.Clone()
	if journal.InputHash != cmd.InputHash || journal.DerivationPermitID != cmd.DerivationPermitID || journal.ProposalHash != cmd.ProposalHash || journal.SpecificationHash != cmd.SpecificationHash || journal.Validate(initial.DefinitionAuthoring, initial.ID) != nil || workspace.WorkspaceConfigurationHash(initial) != journal.ConfigurationHash {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, common.ErrConflict
	}
	if err := validateConfirmationAdmissionReplay(initial, journal); err != nil {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
	}
	if len(initial.DefinitionAuthoringHistory) > 0 && journal.ValidateSuccessor(initial.DefinitionAuthoringHistory[len(initial.DefinitionAuthoringHistory)-1].Confirmation) != nil {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, common.ErrConflict
	}
	if journal.Phase == workspace.DefinitionAuthoringConfirmationPrepared {
		template, err := m.resolvedTemplate(ctx, initial.TemplateID)
		if err != nil || workspace.WorkspaceTemplateSnapshotHash(template) != journal.TemplateSnapshotHash {
			return workspaceevents.ConfirmDefinitionAuthoringResult{}, common.ErrConflict
		}
		derivation, specification, err := initial.DefinitionAuthoring.ConfirmableSpecification(journal.DerivationPermitID, journal.ProposalHash)
		if err != nil {
			return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
		}
		if err := validateAuthoringVerificationAgainstSealedTemplate(journal.TemplateSnapshotHash, derivation, specification); err != nil {
			return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
		}
	}
	if err := m.ensureConfirmationGoal(ctx, journal.Goal); err != nil {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
	}
	if err := m.ensureConfirmationDoD(ctx, journal.DoD); err != nil {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
	}
	current, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil || current == nil || current.DefinitionAuthoringConfirmation == nil || !reflect.DeepEqual(*current.DefinitionAuthoringConfirmation, journal) || workspace.WorkspaceConfigurationHash(current) != journal.ConfigurationHash {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, common.ErrConflict
	}
	if err := workspaceDeletionRejectsMutation(current); err != nil {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
	}
	if journal.Phase == workspace.DefinitionAuthoringConfirmationPrepared {
		updated := *current
		completed := journal.Clone()
		completed.Phase = workspace.DefinitionAuthoringConfirmationMaterialized
		updated.DefinitionAuthoringConfirmation = &completed
		if err := m.workspaces.CompareAndSwap(ctx, &updated, current.Revision); err != nil {
			return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
		}
		current = &updated
	}
	result := workspaceevents.ConfirmDefinitionAuthoringResult{WorkspaceID: cmd.WorkspaceID, InputHash: cmd.InputHash, ProposalHash: cmd.ProposalHash, SpecificationHash: cmd.SpecificationHash, Goal: *workspace.CloneGoal(&journal.Goal), DoD: *workspace.CloneDoD(&journal.DoD), WorkspaceRevision: current.Revision}
	if err := result.ValidateFor(cmd); err != nil {
		return workspaceevents.ConfirmDefinitionAuthoringResult{}, err
	}
	return result, nil
}

func validateConfirmationAdmissionReplay(current *workspace.Workspace, journal workspace.DefinitionAuthoringConfirmation) error {
	if current == nil || current.ValidateDefinitionAuthoringHistory() != nil || len(current.DefinitionAdmissionCandidates) > len(current.DefinitionAuthoringHistory)+1 {
		return common.ErrConflict
	}
	if len(current.DefinitionAuthoringHistory) > 0 {
		if journal.ValidateSuccessor(current.DefinitionAuthoringHistory[len(current.DefinitionAuthoringHistory)-1].Confirmation) != nil {
			return common.ErrConflict
		}
		if len(current.DefinitionAdmissionCandidates) == len(current.DefinitionAuthoringHistory) {
			if current.StartupInput == nil && current.ActivationAdmission == nil && current.ActiveRunID == "" {
				return nil
			}
			return common.ErrConflict
		}
		if journal.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || current.ActiveRunID != "" {
			return common.ErrConflict
		}
		_, _, err := current.ValidateActivationAdmissionPersistence()
		return err
	}
	if current.StartupInput == nil && current.ActivationAdmission == nil && len(current.DefinitionAdmissionCandidates) <= 1 {
		if current.ActiveRunID != "" {
			return common.ErrConflict
		}
		if len(current.DefinitionAdmissionCandidates) == 0 {
			return nil
		}
		candidate := current.DefinitionAdmissionCandidates[0]
		if _, _, err := current.ValidateActivationAdmissionPersistence(); err != nil {
			return common.ErrConflict
		}
		ref, err := workspace.ConfirmedAuthoringAdmissionRef(current.DefinitionAuthoring, journal, current.ID)
		if journal.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || err != nil || candidate.AuthoringRef == nil || *candidate.AuthoringRef != ref || candidate.Validate() != nil {
			return common.ErrConflict
		}
		return nil
	}
	if journal.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || len(current.DefinitionAdmissionCandidates) != 1 {
		return common.ErrConflict
	}
	candidate, present, err := current.ValidateActivationAdmissionPersistence()
	if err != nil || !present || candidate.AuthoringRef == nil || current.ActivationAdmission.State != workspace.WorkspaceActivationEligible {
		return common.ErrConflict
	}
	ref, err := workspace.ConfirmedAuthoringAdmissionRef(current.DefinitionAuthoring, journal, current.ID)
	if err != nil || ref != *candidate.AuthoringRef {
		return common.ErrConflict
	}
	return nil
}

func (m *Biz) ensureConfirmationGoal(ctx context.Context, expected workspace.Goal) error {
	items, err := m.goals.ListByWorkspaceID(ctx, expected.WorkspaceID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		if expected.PreviousGoalID != "" || expected.PreviousDefinitionHash != "" || expectedDefinitionSequence(expected.DefinitionRevision, "goal") != 1 {
			return common.ErrConflict
		}
		if err := m.goals.Create(ctx, &expected); err != nil {
			return err
		}
		return nil
	}
	if leaf := currentGoalLeaf(items, expected.WorkspaceID); leaf != nil && reflect.DeepEqual(*leaf, expected) && len(items) == int(expectedDefinitionSequence(expected.DefinitionRevision, "goal")) {
		return nil
	}
	if len(items) != int(expectedDefinitionSequence(expected.DefinitionRevision, "goal"))-1 || expected.PreviousGoalID == "" || expected.PreviousDefinitionHash == "" {
		return common.ErrConflict
	}
	previous := currentGoalLeaf(items, expected.WorkspaceID)
	if previous == nil || previous.ID != expected.PreviousGoalID || definitionHashForText(previous.Text) != expected.PreviousDefinitionHash {
		return common.ErrConflict
	}
	return m.goals.Create(ctx, &expected)
}

func (m *Biz) ensureConfirmationDoD(ctx context.Context, expected workspace.DoD) error {
	items, err := m.dods.ListByWorkspaceID(ctx, expected.WorkspaceID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		if expected.PreviousDoDID != "" || expected.PreviousDefinitionHash != "" || expectedDefinitionSequence(expected.DefinitionRevision, "dod") != 1 {
			return common.ErrConflict
		}
		if err := m.dods.Create(ctx, workspace.CloneDoD(&expected)); err != nil {
			return err
		}
		return nil
	}
	if leaf := currentDoDLeaf(items, expected.WorkspaceID); leaf != nil && reflect.DeepEqual(*leaf, expected) && len(items) == int(expectedDefinitionSequence(expected.DefinitionRevision, "dod")) {
		return nil
	}
	if len(items) != int(expectedDefinitionSequence(expected.DefinitionRevision, "dod"))-1 || expected.PreviousDoDID == "" || expected.PreviousDefinitionHash == "" {
		return common.ErrConflict
	}
	previous := currentDoDLeaf(items, expected.WorkspaceID)
	if previous == nil || previous.ID != expected.PreviousDoDID || definitionHashForItems(previous.Items) != expected.PreviousDefinitionHash {
		return common.ErrConflict
	}
	return m.dods.Create(ctx, workspace.CloneDoD(&expected))
}

func expectedDefinitionSequence(revision, prefix string) uint64 {
	sequence, _ := definitionRevisionSequence(prefix, revision)
	return sequence
}
