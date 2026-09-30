package workspace

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func authoringAdmissionCandidateFromCurrentConfiguration(t *testing.T, current *workspace.Workspace) workspace.DefinitionAdmissionCandidate {
	t.Helper()
	if current == nil || current.DefinitionAuthoring == nil || current.DefinitionAuthoringConfirmation == nil {
		t.Fatal("confirmed authoring input is missing")
	}
	goal, dod, specification, err := workspace.ConfirmedAuthoringCandidateInputs(current.DefinitionAuthoring, *current.DefinitionAuthoringConfirmation, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := workspace.ConfirmedAuthoringAdmissionRef(current.DefinitionAuthoring, *current.DefinitionAuthoringConfirmation, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := definitionconfiguration.Compile(current, &current.DefinitionAuthoring.TemplateSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	refs := make([]string, len(dod))
	for index := range dod {
		refs[index] = dod[index].CandidateItemRef
	}
	sort.Strings(refs)
	lineage := "authoring-" + workspace.StableDefinitionAuthoringConfirmationID(current.ID, ref.InputHash, ref.DerivationPermitID, ref.SpecificationHash)
	if len(current.DefinitionAdmissionCandidates) > 0 {
		lineage = current.DefinitionAdmissionCandidates[0].CandidateID
	}
	candidate := workspace.DefinitionAdmissionCandidate{
		SchemaVersion: workspace.DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: current.ID,
		CandidateID: lineage, CandidateRevision: uint64(len(current.DefinitionAuthoringHistory) + 1),
		GoalInput: goal, DoDInputs: dod,
		WorkspaceTemplateSnapshot: workspace.DefinitionTemplateSnapshot{TemplateRef: current.TemplateID, Revision: "template:" + current.DefinitionAuthoringConfirmation.TemplateSnapshotHash, SnapshotHash: current.DefinitionAuthoringConfirmation.TemplateSnapshotHash},
		AdmissionPolicyBindings:   bindings.PolicyBindings, RuntimeProfileContracts: bindings.ProfileContracts,
		RequiredOwnerFactBindings: bindings.RequiredOwnerFacts, OwnerFactConvergenceBindings: bindings.OwnerFactConvergences,
		RuntimeProfileContractsHash: bindings.ProfileContractsHash,
		AdmissionCompilerVersion:    workspace.DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: current.DeliverableDirectory,
		ProposedRelationInputs: []workspace.DefinitionProposedRelationInput{{RelationRef: "terminal-all-of", RelationKind: "all_of", SourceItemRefs: refs, TargetItemRefs: []string{goal.CandidateItemRef}}},
		Specification:          &specification, AuthoringRef: &ref,
	}
	candidate = candidate.WithHash()
	if err := candidate.Validate(); err != nil {
		t.Fatal(err)
	}
	return candidate
}

func TestAuthoringAdmissionSealsExactPersistedReviewsWithoutRun(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	confirmation := seedReviewedAuthoringConfirmationWithProof(t, owners, "ws-authoring-admit", true)
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	_, _, reviewed, err := workspace.ConfirmedAuthoringCandidateInputs(current.DefinitionAuthoring, *current.DefinitionAuthoringConfirmation, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	spec := reviewed.Clone()
	for index := range spec.Verifications {
		verification := &spec.Verifications[index]
		verification.Status = workspace.GoalVerificationDeclared
		verification.ProfileID = "generic"
		verification.EvidenceRequirementID = "proof"
		verification.Scope = "dodItem"
	}
	// The owner must reject even a fully self-consistent candidate if the
	// application rewrites the reviewed verifications after their six reviews.
	tampered := spec.Clone()
	for index := range tampered.Verifications {
		tampered.Verifications[index].Declaration = "另一份未审查的声明"
	}
	for index := range tampered.Reviews {
		tampered.Reviews[index].SpecificationHash = tampered.ContentHash()
	}
	if tampered.AdmissionHash() == confirmation.SpecificationHash {
		t.Fatal("tampered specification kept reviewed identity")
	}
	base := authoringAdmissionCandidateFromCurrentConfiguration(t, current)
	tamperedCandidate := base.Clone()
	tamperedCandidate.Specification = &tampered
	tamperedCandidate.AuthoringRef = &workspace.DefinitionAuthoringAdmissionRef{InputHash: base.AuthoringRef.InputHash, DerivationPermitID: base.AuthoringRef.DerivationPermitID, ProposalHash: tampered.ContentHash(), SpecificationHash: tampered.AdmissionHash()}
	tamperedCandidate = tamperedCandidate.WithHash()
	tamperedCommand := workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: tamperedCandidate, ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)}
	if err := tamperedCommand.Validate(); err != nil {
		t.Fatalf("self-consistent counterfeit should pass structural validation: %v", err)
	}
	if _, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, tamperedCommand); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("owner accepted rewritten PASS provenance: %v", err)
	}
	command := workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: base, ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	first, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, command)
	if err != nil || first.AdmissionResult.Disposition != workspace.DefinitionAdmissionAdmittedAllOf || first.StartupInput == nil || first.ActivationAdmission == nil || first.ActivationAdmission.State != workspace.WorkspaceActivationEligible {
		t.Fatalf("exact reviewed candidate was not admitted: %+v %v", first, err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || stored.ActiveRunID != "" || stored.StartupInput == nil || stored.StartupInput.SpecificationHash != reviewed.AdmissionHash() || stored.ActivationAdmission == nil || !reflect.DeepEqual(stored.DefinitionAdmissionCandidates[0].Specification.Normalized(), reviewed.Normalized()) {
		t.Fatalf("admission lost reviewed source or auto-started Run: %+v %v", stored, err)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	second, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, command)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("file-backed duplicate admission changed the result: %+v %+v %v", first, second, err)
	}
	if _, present, err := stored.ValidateActivationAdmissionPersistence(); err != nil || !present {
		t.Fatalf("persisted authoring admission failed provenance validation: %v", err)
	}
	confirmedAgain, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation})
	if err != nil || confirmedAgain.WorkspaceRevision != first.WorkspaceRevision {
		t.Fatalf("confirmation response-loss replay changed admitted Workspace: %+v %v", confirmedAgain, err)
	}
}

func TestAuthoringAdmissionRejectsForgedConfigurationBindings(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	confirmation := seedReviewedAuthoringConfirmationWithProof(t, owners, "ws-authoring-forged-config", true)
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, current)
	for name, modify := range map[string]func(*workspace.DefinitionAdmissionCandidate){
		"profile": func(candidate *workspace.DefinitionAdmissionCandidate) {
			candidate.RuntimeProfileContracts[0].EvidenceRequirements[0].AcceptedStepKinds = []string{"validation/managed_session", "provider-asserted-proof"}
			candidate.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(candidate.RuntimeProfileContracts)
		},
		"policy": func(candidate *workspace.DefinitionAdmissionCandidate) {
			candidate.AdmissionPolicyBindings = []workspace.DefinitionAdmissionPolicyBinding{{PolicyRef: "profile:forged", Revision: "runtime-profile-contract-v1", SnapshotHash: "self-consistent"}}
		},
		"owner-fact": func(candidate *workspace.DefinitionAdmissionCandidate) {
			candidate.RequiredOwnerFactBindings = []workspace.DefinitionRequiredOwnerFactBinding{{SourceRef: "forged-proof", Requirement: runtimecontract.RuntimeOwnerFactRequirement{OwnerModuleID: runtimecontract.RuntimeModuleShell, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession}}}
		},
		"owner-fact-convergence": func(candidate *workspace.DefinitionAdmissionCandidate) {
			candidate.OwnerFactConvergenceBindings = []workspace.DefinitionOwnerFactConvergenceBinding{{SourceRef: "forged-proof", Convergence: runtimecontract.RuntimeOwnerFactConvergence{Requirement: runtimecontract.RuntimeOwnerFactRequirement{OwnerModuleID: runtimecontract.RuntimeModuleShell, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession}, RequirePreexisting: true}}}
		},
		"execution-policy": func(candidate *workspace.DefinitionAdmissionCandidate) {
			candidate.AdmissionPolicyBindings = append(candidate.AdmissionPolicyBindings, workspace.DefinitionAdmissionPolicyBinding{PolicyRef: "execution-limits:forged", Revision: "runtime-execution-limits-v1", SnapshotHash: "self-consistent"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			counterfeit := candidate.Clone()
			modify(&counterfeit)
			counterfeit = counterfeit.WithHash()
			command := workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: counterfeit, ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)}
			if err := command.Validate(); err != nil {
				t.Fatalf("counterfeit must be structurally valid: %v", err)
			}
			if _, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, command); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("owner trusted applicant-supplied %s configuration: %v", name, err)
			}
		})
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil {
		t.Fatalf("counterfeit created admission state: %+v %v", stored, err)
	}
}

func TestAuthoringAdmissionBlockedCandidatePersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer owners.close()
	confirmation := seedReviewedAuthoringConfirmation(t, owners, "ws-authoring-blocked")
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, current)
	command := workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: candidate, ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	first, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, command)
	if err != nil || first.AdmissionResult.Disposition != workspace.DefinitionAdmissionClarificationRequired || first.StartupInput != nil || first.ActivationAdmission != nil {
		t.Fatalf("blocked candidate gained startup: %+v %v", first, err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, current.ID)
	if err != nil || len(stored.DefinitionAdmissionCandidates) != 1 || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.ActiveRunID != "" {
		t.Fatalf("blocked candidate was not persisted safely: %+v %v", stored, err)
	}
	if _, present, err := stored.ValidateActivationAdmissionPersistence(); err != nil || present {
		t.Fatalf("blocked candidate was considered eligible: %v %t", err, present)
	}
	owners.close()
	owners = openFileDefinitionAdmissionOwners(t, root)
	replayed, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, command)
	if err != nil || !reflect.DeepEqual(first, replayed) {
		t.Fatalf("blocked candidate replay changed outcome: %+v %+v %v", first, replayed, err)
	}
	if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); err != nil {
		t.Fatalf("confirmed definition could not replay after blocked admission: %v", err)
	}
}

func TestAuthoringDraftCannotUseLegacyAdmission(t *testing.T) {
	fixture, close := openSpecificationLedgerFixture(t, t.TempDir())
	defer close()
	w, err := fixture.owner.workspaces.Get(fixture.ctx, fixture.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: w.ID, Revision: 1, TemplateID: w.TemplateID, GoalText: "尚未确认的新目标"}
	w.DefinitionAuthoring = &workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshotHash: fixture.candidate.WorkspaceTemplateSnapshot.SnapshotHash}
	for _, currentTemplate := range []bool{false, true} {
		command := workspaceevents.DefinitionAdmissionIntentCommand{Candidate: fixture.candidate, CurrentTemplate: currentTemplate}
		transition, err := reduceDefinitionAdmissionInternalEvent(w, definitionAdmissionInternalEvent{WorkspaceID: w.ID, Intent: &command})
		if !errors.Is(err, common.ErrConflict) || transition.Next != nil || transition.Result != nil {
			t.Fatalf("draft bypassed confirmation: %+v %v", transition, err)
		}
	}
	if w.StartupInput != nil || w.ActivationAdmission != nil {
		t.Fatal("rejected input changed authority")
	}
}
