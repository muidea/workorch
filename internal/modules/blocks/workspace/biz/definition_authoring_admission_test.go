package workspace

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	activationcore "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	runowner "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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
		verification.EvidenceRequirementID = "*"
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
	confirmation := seedReviewedAuthoringConfirmationWithAdmissionBlocker(t, owners, "ws-authoring-blocked")
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
	if err != nil || first.AdmissionResult.Disposition != workspace.DefinitionAdmissionClarificationRequired || first.AdmissionResult.ViolationCode != workspace.DefinitionAdmissionProfileContractInvalid || first.StartupInput != nil || first.ActivationAdmission != nil {
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

func TestAuthoringVerificationApplicabilityBlocksWrongDeclarationBeforeRun(t *testing.T) {
	for _, tt := range []struct{ name, id, scope string }{{"wrongid", "proof", "dodItem"}, {"wrongscope", "*", "deliverable"}, {"otherdod", "dod-002", "dodItem"}} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer func() { owners.close() }()
			confirmation := seedReviewedAuthoringConfirmationWithDeclaration(t, owners, common.WorkspaceID("ws-"+tt.name), true, true, tt.id, tt.scope)
			before, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); err == nil {
				t.Fatal("wrong declaration materialized formal definitions")
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, root)
			if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation}); err == nil {
				t.Fatal("reopen bypassed declaration applicability")
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
			if err != nil || !reflect.DeepEqual(before, stored) {
				t.Fatalf("rejected declaration changed owner state: %+v %v", stored, err)
			}
			goals, goalErr := owners.repositories.Goals.ListByWorkspaceID(ctx, confirmation.WorkspaceID)
			dods, dodErr := owners.repositories.DoDs.ListByWorkspaceID(ctx, confirmation.WorkspaceID)
			runs, runErr := owners.repositories.Runs.List(ctx)
			if goalErr != nil || dodErr != nil || runErr != nil || len(goals) != 0 || len(dods) != 0 || len(runs) != 0 || stored.DefinitionAuthoringConfirmation != nil || len(stored.DefinitionAdmissionCandidates) != 0 || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.ActiveRunID != "" {
				t.Fatalf("invalid declaration gained formal authority: goals=%d dods=%d runs=%d errors=%v/%v/%v", len(goals), len(dods), len(runs), goalErr, dodErr, runErr)
			}
		})
	}
}

// Real static bootstrap and Workspace/Run Start; no SM-2/3/4 execution or credit.
func TestAuthoringApplicableDeclarationsSurviveProductionBootstrapAndExplicitStart(t *testing.T) {
	for _, mode := range []string{"wildcard", "exact"} {
		t.Run(mode, func(t *testing.T) { testAuthoringApplicableBootstrapAndStart(t, mode) })
	}
}

func testAuthoringApplicableBootstrapAndStart(t *testing.T, mode string) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	confirmation := seedReviewedAuthoringConfirmationWithTemplate(t, owners, "ws-applicable-start", true, true, "*", "dodItem", func(template *workspace.WorkspaceTemplate) {
		hints := template.ContextHints.Map()
		generic := hints["profileContracts"].(map[string]any)["generic"].(map[string]any)
		declaration := generic["evidenceRequirements"].([]any)[0].(map[string]any)
		fact := map[string]any{"ownerModuleId": runtimecontract.RuntimeModuleShell, "factKind": string(runtimecontract.RuntimeOwnerFactSelectableValidationSurface), "operationKind": runtimecontract.RuntimeBlockKindManagedValidationSession}
		declaration["requiredOwnerFacts"] = []any{fact}
		declaration["ownerFactConvergences"] = []any{map[string]any{"ownerModuleId": runtimecontract.RuntimeModuleShell, "factKind": string(runtimecontract.RuntimeOwnerFactSelectableValidationSurface), "operationKind": runtimecontract.RuntimeBlockKindManagedValidationSession, "requirePreexisting": true}}
		if mode == "exact" {
			exact := make([]any, 0, 3)
			for n := 1; n <= 3; n++ {
				value := make(map[string]any, len(declaration))
				for key, item := range declaration {
					value[key] = item
				}
				value["id"] = fmt.Sprintf("dod-%03d", n)
				exact = append(exact, value)
			}
			// Exact proof and wildcard constraints both survive; no fallback replaces either.
			declaration["requireDirectEvidence"] = true
			delete(declaration, "requiredOwnerFacts")
			delete(declaration, "ownerFactConvergences")
			generic["evidenceRequirements"] = append(exact, declaration)
		}

		generic["bootstrapRecipes"] = []any{map[string]any{"id": "init", "requiredGoalTerms": []any{"交付"}, "artifacts": []any{map[string]any{"id": "artifact", "targetRole": "artifact", "allowedOperationKinds": []any{runtimecontract.StepKindFileWrite}}}}}
		payload, err := workorcheventpayload.FromMap(hints)
		if err != nil {
			t.Fatal(err)
		}
		template.ContextHints = payload
	})
	confirmed, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: confirmation})
	if err != nil {
		t.Fatal(err)
	}
	current, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, current)
	admitted, err := owners.workspace.submitDefinitionAuthoringAdmission(ctx, workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: candidate, ExpectedWorkspaceRevision: current.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(current)})
	if err != nil || admitted.StartupInput == nil {
		t.Fatalf("applicable reviewed source was blocked: %+v %v", admitted, err)
	}
	policy := &runtimecontract.RuntimeAcceptanceSlicePolicy{SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent, MaxWorkItems: 4, MaxFocuses: 16, MaxCandidateContextItems: 4, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed}
	bootstrap, err := activationcore.RuntimeBootstrap(&confirmed.Goal, &confirmed.DoD, admitted.StartupInput.DefinitionRevision, admitted.StartupInput.StartupInputHash, nil, runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "fixture", ModelID: "fixture"}}, ExecutionContract: runtimecontract.RuntimeExecutionContract{StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired, AcceptanceSlicePolicy: policy, ProfileContracts: candidate.RuntimeProfileContracts}})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range bootstrap.CoverageItems {
		if item.SourceScope == "dodItem" {
			count++
			if item.SourceID != fmt.Sprintf("dod-%03d", count) || string(item.ID) == "cwi-001" {
				t.Fatalf("source/CWI order conflated: %+v", item)
			}
		}
	}
	if count != 3 {
		t.Fatalf("missing confirmed sources: %d", count)
	}
	for _, focus := range bootstrap.DefinitionFocuses {
		if focus.AssessmentScope != "dodItem" {
			continue
		}
		if len(focus.RequiredEvidenceKinds) != 1 || focus.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation || len(focus.RequiredOwnerFacts) == 0 || len(focus.AcceptedStepKinds) == 0 {
			t.Fatalf("declared proof contribution lost: %+v", focus)
		}
		if mode == "exact" && !focus.RequireDirectEvidence {
			t.Fatalf("exact declaration shadowed wildcard quality constraint: %+v", focus)
		}
	}
	runs, err := owners.repositories.Runs.List(ctx)
	if err != nil || len(runs) != 0 {
		t.Fatal("confirmation/bootstrap automatically started Run")
	}
	run := runowner.New()
	if err := run.Configure(runowner.Dependencies{OwnerRepositories: owners.repositories.RunOwner, EventHub: owners.hub}); err != nil {
		t.Fatal(err)
	}
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	defer run.Stop(ctx)
	result, err := owners.workspace.executeExecutionIntent(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandExecutionIntent, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: confirmation.WorkspaceID, RequestID: "explicit-start", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: bootstrap}))
	if err != nil || result.RunID == "" {
		t.Fatalf("explicit Start did not reach real Run owner: %+v %v", result, err)
	}
	runs, err = owners.repositories.Runs.List(ctx)
	if err != nil || len(runs) != 1 {
		t.Fatalf("explicit Start cardinality: %d %v", len(runs), err)
	}
}
