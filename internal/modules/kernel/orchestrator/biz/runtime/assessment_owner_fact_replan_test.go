package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCanonicalAssessmentOwnerFactCannotOverrideOrHideProofQuality(t *testing.T) {
	requirement := runtimeTestSelectableValidationOwnerFact()

	complete, _ := assessmentGateFixture(t, "behavior is verified", "TestBehavior")
	complete.Facts.Focus.RequiredOwnerFacts = []runtimecontract.RuntimeOwnerFactRequirement{requirement}
	refs, satisfied := canonicalAssessmentEvidenceForCoverage(complete, "cwi-claim")
	if satisfied || len(refs) != 1 || refs[0] != "evidence-claim" {
		t.Fatalf("missing owner postcondition did not remain a distinct proof gap: refs=%v satisfied=%t", refs, satisfied)
	}
	if !canonicalAssessmentProofCompleteWithoutOwnerFacts(complete) {
		t.Fatal("complete sealed proof was not recognized independently from its owner postcondition")
	}

	incomplete, _ := assessmentGateFixture(t, "behavior is verified", "")
	incomplete.Facts.Focus.RequiredOwnerFacts = []runtimecontract.RuntimeOwnerFactRequirement{requirement}
	if canonicalAssessmentProofCompleteWithoutOwnerFacts(incomplete) {
		t.Fatal("missing proof quality was misclassified as an owner-fact-only gap")
	}
}

func TestCommitAssessmentReopensCurrentCWIWhenOwnerFactIsSoleGap(t *testing.T) {
	controller, repository, bootstrapFacts := prepareOwnerFactAssessment(t, false, runtimecontract.RuntimeBlockKindPatchApply)
	savesBefore := repository.saves

	progressed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if progressed.Decision.State == runtimecontract.ConvergenceStateComplete || progressed.Decision.State == runtimecontract.ConvergenceStateFailed {
		t.Fatalf("owner-fact repair closed or failed before its bounded semantic replan: %+v", progressed.Decision)
	}
	if progressed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || len(progressed.SemanticPlanning.History) != 1 {
		t.Fatalf("owner-fact gap did not retire exactly one semantic generation: %+v", progressed.SemanticPlanning)
	}
	history := progressed.SemanticPlanning.History[0]
	if history.Reason != runtimecontract.RuntimeSemanticPlanningRetirementCWIReopened || len(history.ReopenedCoverageIDs) != 1 || history.ReopenedCoverageIDs[0] != "cwi-v2" {
		t.Fatalf("semantic generation retirement lineage = %+v", history)
	}
	if progressed.Facts.Focus.ID != "semantic-pending/cwi-v2" || len(progressed.Facts.Focus.CoverageEvidenceBindings) != 0 {
		t.Fatalf("reopened CWI focus projection = %+v", progressed.Facts.Focus)
	}
	item := progressed.Facts.Coverage[0]
	if item.State != runtimecontract.RuntimeCoverageItemStateOpen || item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened || item.CurrentBindingRef != nil || len(item.ReopenLineage) != 1 {
		t.Fatalf("reopened CWI row = %+v", item)
	}
	if lineage := item.ReopenLineage[0]; lineage.Reason != runtimeCWIReopenReasonOwnerFactPostcondition || lineage.TriggerRef == "" {
		t.Fatalf("owner-fact reopen lineage = %+v", lineage)
	}
	assessment := progressed.Facts.Assessments[len(progressed.Facts.Assessments)-1]
	if assessment.ClaimedDisposition != runtimecontract.AssessmentDispositionSatisfied || assessment.Disposition != runtimecontract.AssessmentDispositionRepairableGap {
		t.Fatalf("producer assessment overrode canonical owner fact: %+v", assessment)
	}
	if progressed.ActiveDispatch != nil {
		t.Fatalf("assessment dispatch remained active after atomic CWI reopen: %+v", progressed.ActiveDispatch)
	}
	if len(progressed.Facts.Repair.NoProgressOperationShapes) != 1 ||
		progressed.Facts.Repair.NoProgressOperationShapes[0].Operation.Kind != runtimecontract.StepKindFileWrite {
		t.Fatalf("owner-fact reopen lost accepted operation anti-replay fact: %+v", progressed.Facts.Repair)
	}
	if err := progressed.Validate(); err != nil {
		t.Fatalf("replanned aggregate is invalid: %v", err)
	}

	// The new binding has a new semantic identity, but the same operation shape
	// remains blocked for this CWI until an owner-observable gap changes.
	successorFocus := bootstrapFacts.Focus.Normalized()
	successorFocus.ID = "semantic-focus-owner-fact-successor"
	successorBinding := successorFocus.CoverageEvidenceBindings[0].Clone()
	successorBinding.ID = "semantic-binding-owner-fact-successor"
	successorBinding.FocusID = successorFocus.ID
	successorBinding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(successorBinding)
	successorFocus.CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{successorBinding}
	resealed, err := sealRuntimeCWIBindings(progressed.Facts, "semantic-manifest-owner-fact-successor", []runtimecontract.CoverageFocus{successorFocus}, nil)
	if err != nil {
		t.Fatalf("seal successor owner-fact binding: %v", err)
	}
	if len(resealed.Repair.NoProgressOperationShapes) != 1 ||
		resealed.Repair.NoProgressOperationShapes[0].Hash != progressed.Facts.Repair.NoProgressOperationShapes[0].Hash ||
		resealed.Repair.FocusID != successorFocus.ID {
		t.Fatalf("successor semantic focus lost CWI anti-replay authority: %+v", resealed.Repair)
	}

	restarted, err := controller.initialize(context.Background(), bootstrapFacts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != savesBefore+1 || restarted.StateRevision != progressed.StateRevision || restarted.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || restarted.Facts.Focus.ID != progressed.Facts.Focus.ID {
		t.Fatalf("restart changed owner-fact replan authority: saves=%d before=%d restarted=%+v", repository.saves, savesBefore, restarted)
	}
}

func TestCommitAssessmentCompletesAfterOwnerPublishesSelectableValidationSurface(t *testing.T) {
	controller, _, _ := prepareOwnerFactAssessment(t, true)
	completed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Decision.State != runtimecontract.ConvergenceStateComplete || completed.Decision.Terminal == nil || completed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed {
		t.Fatalf("ready direct owner surface did not allow normal completion: %+v", completed)
	}
	if completed.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateSatisfied || completed.Facts.Coverage[0].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled || len(completed.Facts.Coverage[0].ReopenLineage) != 0 {
		t.Fatalf("completed owner-fact CWI row = %+v", completed.Facts.Coverage[0])
	}
}

func TestCommitAssessmentSettlesAcquisitionBeforeTerminalDefinitionProof(t *testing.T) {
	controller, repository, bootstrapFacts, definition := prepareTerminalValidationOwnerFactAcquisitionAssessment(t)
	savesBefore := repository.saves

	acquisition, found, err := currentRuntimeSemanticOwnerFactAcquisition(*repository.aggregate)
	if err != nil || !found {
		t.Fatalf("sealed owner-fact acquisition: found=%t acquisition=%+v err=%v", found, acquisition, err)
	}
	if acquisition.AcquisitionOperationKind != runtimecontract.StepKindFileWrite ||
		len(repository.aggregate.Facts.Focus.RequiredEvidenceKinds) != 1 || repository.aggregate.Facts.Focus.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("current generation is not the exact acquisition contract: acquisition=%+v focus=%+v", acquisition, repository.aggregate.Facts.Focus)
	}
	declaration := repository.aggregate.Facts.DefinitionFocuses[0].CoverageFocusDeclaration()
	if len(declaration.RequiredEvidenceKinds) != 1 || declaration.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation {
		t.Fatalf("acquisition overwrote terminal Definition evidence: %+v", declaration)
	}

	// Model the Shell owner's post-operation re-observation. This is owner data,
	// not an Orchestrator transition; commitAssessment may only consume it after
	// the private AssessmentObserved route invokes SM-2.
	shellCatalog := runtimeTestSelectableValidationCatalog(t)
	for index := range repository.aggregate.Facts.Resources.OperationAdmissibilities {
		if repository.aggregate.Facts.Resources.OperationAdmissibilities[index].OwnerModuleID == runtimecontract.RuntimeModuleShell {
			repository.aggregate.Facts.Resources.OperationAdmissibilities[index] = shellCatalog
			shellCatalog = runtimecontract.RuntimeOperationAdmissibility{}
			break
		}
	}
	if shellCatalog.OwnerModuleID != "" {
		repository.aggregate.Facts.Resources.OperationAdmissibilities = append(repository.aggregate.Facts.Resources.OperationAdmissibilities, shellCatalog)
	}
	repository.aggregate.Facts.Resources.OperationAdmissibilities = runtimecontract.NormalizeRuntimeOperationAdmissibilities(repository.aggregate.Facts.Resources.OperationAdmissibilities)

	progressed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if progressed.Decision.State == runtimecontract.ConvergenceStateComplete || progressed.Decision.State == runtimecontract.ConvergenceStateFailed {
		t.Fatalf("acquisition incorrectly selected a terminal Run state: %+v", progressed.Decision)
	}
	if progressed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || len(progressed.SemanticPlanning.History) != 1 {
		t.Fatalf("acquisition did not require one fresh terminal-proof generation: %+v", progressed.SemanticPlanning)
	}
	item := progressed.Facts.Coverage[0]
	if item.State != runtimecontract.RuntimeCoverageItemStateOpen || item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened || item.CurrentBindingRef != nil || len(item.ReopenLineage) != 1 {
		t.Fatalf("acquisition granted terminal CWI credit: %+v", item)
	}
	if item.ReopenLineage[0].Reason != runtimeCWIReopenReasonOwnerFactAcquisitionSettled || progressed.Facts.Focus.ID != "semantic-pending/cwi-v2" {
		t.Fatalf("acquisition successor lineage = %+v focus=%+v", item.ReopenLineage, progressed.Facts.Focus)
	}
	if progressed.DefinitionClarification != nil || progressed.Decision.State == runtimecontract.ConvergenceStateNeedGovernance {
		t.Fatalf("declared acquisition opened a Definition clarification: %+v", progressed.DefinitionClarification)
	}
	if len(progressed.Facts.Repair.NoProgressOperationShapes) != 1 ||
		progressed.Facts.Repair.NoProgressOperationShapes[0].FailureCode != runtimeCWIReopenReasonOwnerFactAcquisitionSettled ||
		progressed.Facts.Repair.NoProgressOperationShapes[0].Operation.Kind != runtimecontract.StepKindFileWrite {
		t.Fatalf("bounded acquisition attempt was not retained exactly once: %+v", progressed.Facts.Repair)
	}
	if err := progressed.Validate(); err != nil {
		t.Fatalf("acquisition successor aggregate is invalid: %v", err)
	}
	successorManifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		progressed.Facts.WorkspaceID,
		progressed.Facts.RunID,
		progressed.Definition,
		progressed.Facts.DefinitionSeed,
		progressed.Facts.DefinitionFocuses,
		progressed.Facts.Activation.ExecutionContract,
		progressed.Facts.Resources.OperationAdmissibilities,
		runtimecontract.RuntimeCWIsEligibleForPlanning(progressed.Facts),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(successorManifest.ProofObligations) != 1 || successorManifest.ProofObligations[0].OwnerFactAcquisition != nil ||
		len(successorManifest.ProofObligations[0].RequiredEvidenceKinds) != 1 || successorManifest.ProofObligations[0].RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation {
		t.Fatalf("successor generation did not restore terminal Definition proof: obligations=%+v missing=%+v catalogs=%+v", successorManifest.ProofObligations, runtimecontract.RuntimeMissingOwnerFactRequirements(progressed.Facts.DefinitionFocuses[0].RequiredOwnerFacts, progressed.Facts.Resources.OperationAdmissibilities), progressed.Facts.Resources.OperationAdmissibilities)
	}

	restarted, err := controller.initialize(context.Background(), bootstrapFacts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != savesBefore+1 || restarted.StateRevision != progressed.StateRevision ||
		restarted.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || restarted.Facts.Focus.ID != progressed.Facts.Focus.ID {
		t.Fatalf("restart changed acquisition settlement authority: saves=%d before=%d restarted=%+v", repository.saves, savesBefore, restarted)
	}
}

func TestAssessmentObservedUsesRetiredAcquisitionPurposeAfterReceiptResourceChange(t *testing.T) {
	_, repository, bootstrapFacts, definition := prepareTerminalValidationOwnerFactAcquisitionAssessment(t, true)
	before := *repository.aggregate
	if before.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || len(before.SemanticPlanning.History) != 1 {
		t.Fatalf("resource change did not retire exactly one semantic generation: %+v", before.SemanticPlanning)
	}
	history := before.SemanticPlanning.History[0]
	if history.Reason != runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated || history.Snapshot.Manifest == nil ||
		before.Facts.Coverage[0].CurrentBindingRef == nil ||
		before.Facts.Coverage[0].CurrentBindingRef.SemanticManifestHash != history.Snapshot.Manifest.Hash {
		t.Fatalf("current binding did not retain exact retired manifest lineage: history=%+v coverage=%+v", history, before.Facts.Coverage[0])
	}
	acquisition, found, err := currentRuntimeSemanticOwnerFactAcquisition(before)
	if err != nil || !found || acquisition.AcquisitionOperationKind != runtimecontract.StepKindFileWrite {
		t.Fatalf("retired acquisition purpose was not recovered: found=%t acquisition=%+v err=%v", found, acquisition, err)
	}

	// A stale binding hash with no active-sealed or immutable retired manifest
	// cannot silently fall back to Definition proof.
	stale := before
	stale.SemanticPlanning = stale.SemanticPlanning.Clone()
	stale.SemanticPlanning.History = nil
	if _, _, staleErr := currentRuntimeSemanticOwnerFactAcquisition(stale); !errors.Is(staleErr, common.ErrConflict) {
		t.Fatalf("missing retired acquisition lineage err=%v, want conflict", staleErr)
	}
	staleRepository := &controllerRepository{aggregate: &stale}
	staleController, err := NewRuntimeController(staleRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, staleErr := staleController.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:      RuntimeInternalEventAssessmentObserved,
		Scope:     RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"},
		Authority: &RuntimeInternalAuthorityPayload{},
	}); !errors.Is(staleErr, common.ErrConflict) {
		t.Fatalf("stale AssessmentObserved err=%v, want conflict", staleErr)
	}
	if staleRepository.saves != 0 || staleRepository.aggregate.StateRevision != before.StateRevision || len(staleRepository.aggregate.Facts.Assessments) != 0 {
		t.Fatalf("stale AssessmentObserved mutated authority: saves=%d aggregate=%+v", staleRepository.saves, staleRepository.aggregate)
	}

	// Fresh-process recovery must consume the same persisted current binding and
	// retired generation; it cannot require an in-memory acquisition marker.
	savesBeforeRestart := repository.saves
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := restarted.initialize(context.Background(), bootstrapFacts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != savesBeforeRestart || reloaded.StateRevision != before.StateRevision || len(reloaded.SemanticPlanning.History) != 1 {
		t.Fatalf("restart rewrote retired acquisition authority: saves=%d before=%d aggregate=%+v", repository.saves, savesBeforeRestart, reloaded)
	}

	result, err := restarted.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:      RuntimeInternalEventAssessmentObserved,
		Scope:     RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"},
		Authority: &RuntimeInternalAuthorityPayload{},
	})
	if err != nil {
		t.Fatal(err)
	}
	progressed := result.Aggregate
	item := progressed.Facts.Coverage[0]
	if progressed.Decision.State == runtimecontract.ConvergenceStateComplete || progressed.Decision.State == runtimecontract.ConvergenceStateFailed ||
		item.State != runtimecontract.RuntimeCoverageItemStateOpen || item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened ||
		item.CurrentBindingRef != nil || len(item.ReopenLineage) != 1 || item.ReopenLineage[0].Reason != runtimeCWIReopenReasonOwnerFactAcquisitionSettled {
		t.Fatalf("retired acquisition state_change closed terminal CWI instead of reopening it: decision=%+v item=%+v", progressed.Decision, item)
	}
	if progressed.Facts.Focus.ID != "semantic-pending/cwi-v2" || len(progressed.Facts.Repair.NoProgressOperationShapes) != 1 ||
		progressed.Facts.Repair.NoProgressOperationShapes[0].FailureCode != runtimeCWIReopenReasonOwnerFactAcquisitionSettled {
		t.Fatalf("terminal validation successor or acquisition anti-replay lineage was not restored: focus=%+v repair=%+v", progressed.Facts.Focus, progressed.Facts.Repair)
	}
	successor, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		progressed.Facts.WorkspaceID,
		progressed.Facts.RunID,
		progressed.Definition,
		progressed.Facts.DefinitionSeed,
		progressed.Facts.DefinitionFocuses,
		progressed.Facts.Activation.ExecutionContract,
		progressed.Facts.Resources.OperationAdmissibilities,
		runtimecontract.RuntimeCWIsEligibleForPlanning(progressed.Facts),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(successor.ProofObligations) != 1 || successor.ProofObligations[0].OwnerFactAcquisition != nil ||
		len(successor.ProofObligations[0].RequiredEvidenceKinds) != 1 || successor.ProofObligations[0].RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation {
		t.Fatalf("reopened successor did not restore terminal validation proof: %+v", successor.ProofObligations)
	}
	if err := progressed.Validate(); err != nil {
		t.Fatalf("retired-acquisition successor aggregate is invalid: %v", err)
	}

	// The same owner-private fact is stale after the first commit. It must not
	// append another assessment/reopen, consume another acquisition, or write CAS.
	savesAfterCommit := repository.saves
	if _, duplicateErr := restarted.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:      RuntimeInternalEventAssessmentObserved,
		Scope:     RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"},
		Authority: &RuntimeInternalAuthorityPayload{},
	}); !errors.Is(duplicateErr, common.ErrConflict) {
		t.Fatalf("duplicate AssessmentObserved err=%v, want conflict", duplicateErr)
	}
	if repository.saves != savesAfterCommit || len(repository.aggregate.Facts.Coverage[0].ReopenLineage) != 1 || len(repository.aggregate.Facts.Assessments) != 1 {
		t.Fatalf("duplicate AssessmentObserved mutated acquisition settlement: saves=%d before=%d aggregate=%+v", repository.saves, savesAfterCommit, repository.aggregate)
	}
}

func TestCommitAssessmentFailsClosedAfterSingleSealedOwnerFactAcquisition(t *testing.T) {
	controller, _, _ := prepareOwnerFactAssessment(t, false)
	failed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Decision.State != runtimecontract.ConvergenceStateFailed || failed.Decision.Terminal == nil || failed.Decision.Terminal.Code != "owner_fact_convergence_exhausted" {
		t.Fatalf("one-shot owner-fact acquisition did not fail closed: %+v", failed.Decision)
	}
	if len(failed.Facts.Coverage[0].ReopenLineage) != 0 || len(failed.Facts.Repair.NoProgressOperationShapes) != 1 ||
		failed.Facts.Repair.NoProgressOperationShapes[0].Operation.Kind != runtimecontract.StepKindFileWrite {
		t.Fatalf("one-shot acquisition created a second generation or lost its exact attempt: %+v", failed.Facts)
	}
}

func TestOwnerFactSemanticReplanBudgetCountsOnlyItsOwnBoundedLineage(t *testing.T) {
	facts := controllerFacts()
	facts.Focus.RequiredOwnerFacts = []runtimecontract.RuntimeOwnerFactRequirement{runtimeTestSelectableValidationOwnerFact()}
	facts.Focus.OwnerFactConvergences = []runtimecontract.RuntimeOwnerFactConvergence{runtimeTestSelectableValidationOwnerFactConvergence()}
	acquisition := facts.Focus.OwnerFactConvergences[0]
	facts.Coverage[0].ReopenLineage = []runtimecontract.RuntimeCWIReopenLineage{
		{Reason: "dependency_invalidated"},
	}
	if runtimeOwnerFactReplanBudgetExhausted(facts, acquisition) {
		t.Fatal("unrelated CWI reopen lineage consumed the owner-fact replan budget")
	}
	facts.Repair.NoProgressOperationShapes = []runtimecontract.RuntimeRejectedOperationShape{{
		Operation:        runtimecontract.RuntimeOperation{ID: "owner-fact-acquisition", Kind: acquisition.AcquisitionOperationKind},
		FailureSignature: runtimeOwnerFactAcquisitionFailureSignature(acquisition.Requirement),
	}}
	if !runtimeOwnerFactReplanBudgetExhausted(facts, acquisition) {
		t.Fatal("owner-fact replan budget did not fail closed at the declared limit")
	}
}

func TestCommitAssessmentFailsClosedAfterOwnerFactReplanBudget(t *testing.T) {
	controller, repository, _ := prepareOwnerFactAssessment(t, false, runtimecontract.RuntimeBlockKindPatchApply)
	item := &repository.aggregate.Facts.Coverage[0]
	if item.CurrentBindingRef == nil {
		t.Fatal("owner-fact budget fixture has no current binding")
	}
	prior := item.CurrentBindingRef.Clone()
	prior.BindingID = "binding/cwi-v2/prior-owner-fact-generation"
	prior.BindingHash = "binding-hash/prior-owner-fact-generation"
	prior.SemanticManifestHash = "semantic-manifest/prior-owner-fact-generation"
	item.BindingLineage = append([]runtimecontract.RuntimeCWIBindingRef{prior}, item.BindingLineage...)
	item.ReopenLineage = []runtimecontract.RuntimeCWIReopenLineage{{
		PreviousBindingRef: prior, TriggerRef: "assessment/prior-owner-fact-generation",
		Reason: runtimeCWIReopenReasonOwnerFactPostcondition,
	}}
	acquisition := repository.aggregate.Facts.Focus.OwnerFactConvergences[0]
	repository.aggregate.Facts.Repair.NoProgressOperationShapes = []runtimecontract.RuntimeRejectedOperationShape{{
		Operation:        runtimecontract.RuntimeOperation{ID: "prior-owner-fact-acquisition", Kind: acquisition.AcquisitionOperationKind},
		FailureSignature: runtimeOwnerFactAcquisitionFailureSignature(acquisition.Requirement),
	}}
	repository.aggregate.Facts.ConvergencePolicy.MaxRepairAttemptsPerFocus = 1
	policy := runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(repository.aggregate.Facts.ConvergencePolicy, repository.aggregate.Facts.RecoveryPolicy)
	repository.aggregate.Facts.ExecutionBudgetPolicy = policy
	repository.aggregate.ExecutionBudget.Policy = policy
	if err := repository.aggregate.Validate(); err != nil {
		t.Fatalf("owner-fact budget fixture is invalid: %v", err)
	}

	failed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Decision.State != runtimecontract.ConvergenceStateFailed || failed.Decision.Terminal == nil || failed.Decision.Terminal.Code != "owner_fact_convergence_exhausted" {
		t.Fatalf("owner-fact budget did not fail closed deterministically: %+v", failed.Decision)
	}
	if failed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed || len(failed.Facts.Coverage[0].ReopenLineage) != 1 || failed.Facts.Coverage[0].CurrentBindingRef == nil {
		t.Fatalf("budget exhaustion retired another binding/generation: %+v", failed)
	}
}

func prepareOwnerFactAssessment(t *testing.T, ownerFactReady bool, operationKinds ...string) (*RuntimeController, *controllerRepository, runtimecontract.RuntimeFacts) {
	t.Helper()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.RequireAssessment = true
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Capabilities.StateChangeKinds = append(facts.Capabilities.StateChangeKinds, runtimecontract.RuntimeBlockKindPatchApply)
	facts.Focus.RequiredOwnerFacts = []runtimecontract.RuntimeOwnerFactRequirement{runtimeTestSelectableValidationOwnerFact()}
	convergence := runtimeTestSelectableValidationOwnerFactConvergence()
	if len(operationKinds) > 0 {
		convergence.AcquisitionOperationKind = runtimecontract.CanonicalRuntimeBlockKind(operationKinds[0])
		if convergence.AcquisitionOperationKind == runtimecontract.RuntimeBlockKindPatchApply {
			convergence.AcquisitionOperationVariantID = "file_write"
		}
	}
	facts.Focus.OwnerFactConvergences = []runtimecontract.RuntimeOwnerFactConvergence{convergence}
	if ownerFactReady {
		facts.Resources.OperationAdmissibilities = append(facts.Resources.OperationAdmissibilities, runtimeTestSelectableValidationCatalog(t))
	}
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, controllerDefinition())
	bootstrapFacts := facts
	if _, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "owner-fact-state-change-prefix"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimeTestBindCandidateToFacts(t, facts, runtimecontract.RuntimeCandidate{
		StateChange: &runtimecontract.StateChangeCandidate{
			Operations: []runtimecontract.StateChangingOperation{{
				Operation: runtimecontract.RuntimeOperation{
					ID: "write-owner-fact", Kind: runtimecontract.StepKindFileWrite,
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("state"),
				},
			}},
		},
	})
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	evidence := ExecutionEvidence{
		EventID: "event-owner-fact", WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-owner-fact",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, repository.aggregate, &evidence)
	committed, err := controller.commitEvidence(context.Background(), evidence)
	if err != nil || committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("owner-fact fixture did not reach assessment: aggregate=%+v err=%v", committed.Aggregate, err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "owner-fact-assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	assessment := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: committed.Aggregate.Facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "producer claims the preparation is complete", EvidenceRefs: []string{"event-owner-fact"},
	}}
	if _, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, assessment); err != nil {
		t.Fatal(err)
	}
	return controller, repository, bootstrapFacts
}

func prepareTerminalValidationOwnerFactAcquisitionAssessment(t *testing.T, retireSemanticAfterReceipt ...bool) (*RuntimeController, *controllerRepository, runtimecontract.RuntimeFacts, runtimecontract.RuntimeDefinitionContext) {
	t.Helper()
	retireAfterReceipt := len(retireSemanticAfterReceipt) != 0 && retireSemanticAfterReceipt[0]
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.RequireAssessment = true
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Focus.RequiredOwnerFacts = []runtimecontract.RuntimeOwnerFactRequirement{runtimeTestSelectableValidationOwnerFact()}
	convergence := runtimeTestSelectableValidationOwnerFactConvergence()
	facts.Focus.OwnerFactConvergences = []runtimecontract.RuntimeOwnerFactConvergence{convergence}
	facts.Focus.TargetSelectorRef = convergence.TargetSelectorRef
	facts.Focus.TargetSelectorOwnerRef = convergence.TargetSelectorOwnerRef
	facts.Focus.TargetSelectorSubjectRef = convergence.TargetSelectorSubjectRef
	facts.Focus.TargetRole = "validation-input"
	facts.Focus.ProspectiveTargetRef = "runtime-owner-fact-validation_test.go"
	facts.Focus.RequireTargetGrounding = true
	fileCatalog := facts.Resources.OperationAdmissibilities[0]
	fileCatalog.SelectorRefs = []string{convergence.AcquisitionTargetSelectorRef}
	fileCatalog.ConstraintRefs = []string{"workspace-relative-path"}
	fileCatalog.OperationVariants = []runtimecontract.RuntimeOperationAdmissibilityVariant{{
		Kind: runtimecontract.StepKindFileWrite,
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
			runtimecontract.RuntimeOperationTargetPolicyExactTarget,
			runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		},
		SelectorRefs:   []string{convergence.AcquisitionTargetSelectorRef},
		ConstraintRefs: []string{"workspace-relative-path"},
	}}
	fileCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileCatalog)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{
		fileCatalog,
		runtimeTestProspectiveValidationDependencyCatalog(t, convergence.ProspectiveSatisfactionContractRef),
	}
	if retireAfterReceipt {
		for _, catalog := range facts.Resources.OperationAdmissibilities {
			prerequisite := runtimecontract.RuntimePrerequisite{
				ID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
				OwnerModuleID: catalog.OwnerModuleID, RequiredCapabilities: append([]string(nil), catalog.OperationKinds...),
			}
			facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, prerequisite)
			facts.Resources.PrerequisiteObservations = append(
				facts.Resources.PrerequisiteObservations,
				runtimeTestAssessmentSemanticCatalogObservation(catalog, "workspace-tree-owner-fact-acquisition-v1"),
			)
		}
	}
	facts.DefinitionFocuses = nil
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
	bootstrapFacts := facts
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	if _, err := controller.initialize(context.Background(), facts, definition, nil); err != nil {
		t.Fatal(err)
	}
	semantic, err := controller.EnsureSemanticContractSealedForOperationTest(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if semantic.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(semantic.Facts.Prerequisites.Pending) != 1 {
		t.Fatalf("owner-fact acquisition did not request exact target grounding: %+v", semantic.Decision)
	}
	prerequisite := semantic.Facts.Prerequisites.Pending[0]
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: prerequisite.ID, OwnerModuleID: prerequisite.OwnerModuleID, TargetRole: prerequisite.SubjectRef,
		SelectorRef: prerequisite.TargetSelectorRef, SelectorOwnerRef: prerequisite.TargetSelectorOwnerRef, SelectorSubjectRef: prerequisite.TargetSelectorSubjectRef,
		SelectorCandidateRefs: append([]string(nil), prerequisite.TargetSelectorCandidateRefs...), Disposition: runtimecontract.RuntimeTargetSelectionCreate,
		ProspectiveTargetRef: prerequisite.ProspectiveTargetRef, ObservedRevision: "workspace-tree-owner-fact-acquisition-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	grounded, err := controller.commitPrerequisite(context.Background(), "ws-v2", "run-v2", PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSelection: &selection})
	if err != nil {
		t.Fatal(err)
	}
	if grounded.Decision.State == runtimecontract.ConvergenceStateFailed {
		t.Fatalf("owner-fact acquisition target grounding failed: terminal=%+v", grounded.Decision.Terminal)
	}
	opened, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "owner-fact-acquisition-prefix")
	if err != nil {
		t.Fatal(err)
	}
	target := opened.Facts.Focus.ProspectiveTargetRef
	if target == "" {
		t.Fatalf("owner-fact acquisition focus has no prospective target: %+v", opened.Facts.Focus)
	}
	candidate := runtimeTestBindCandidateToCurrentFocus(t, opened, runtimecontract.RuntimeCandidate{
		StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
			Operation: runtimecontract.RuntimeOperation{
				ID: "write-owner-fact-acquisition", Kind: runtimecontract.StepKindFileWrite,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: runtimeFileWriteParameters("validation surface"),
			},
		}}},
	})
	if retireAfterReceipt {
		if err := RecordFixtureCandidateFeasibilities(context.Background(), controller, "ws-v2", "run-v2", 0, candidate); err != nil {
			t.Fatal(err)
		}
	}
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	evidence := ExecutionEvidence{
		EventID: "event-owner-fact-acquisition", WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-owner-fact-acquisition",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, repository.aggregate, &evidence)
	if retireAfterReceipt {
		runtimeTestAttachReceiptResourceChanges(t, repository.aggregate, &evidence, []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", BeforeRevision: "workspace-tree-owner-fact-acquisition-v1", AfterRevision: "workspace-tree-owner-fact-acquisition-v2",
		}})
	}
	committed, err := controller.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatalf("owner-fact acquisition evidence commit: %v", err)
	}
	if retireAfterReceipt {
		if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(committed.Aggregate.Facts.Prerequisites.Pending) == 0 {
			t.Fatalf("owner-fact acquisition resource change did not require catalog refresh: %+v", committed.Aggregate)
		}
		for len(repository.aggregate.Facts.Prerequisites.Pending) != 0 {
			prerequisite := repository.aggregate.Facts.Prerequisites.Pending[0]
			catalog, found := runtimeTestAssessmentCatalogForPrerequisite(facts.Resources.OperationAdmissibilities, prerequisite.ID)
			if !found {
				t.Fatalf("no original catalog for refreshed prerequisite %+v", prerequisite)
			}
			if prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleShell {
				catalog = runtimeTestSelectableValidationCatalog(t)
				catalog.PrerequisiteID = prerequisite.ID
			} else {
				catalog.ObservedRevision = "workspace-tree-owner-fact-acquisition-v2"
			}
			catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
			observation := runtimeTestAssessmentSemanticCatalogObservation(catalog, "workspace-tree-owner-fact-acquisition-v2")
			refreshed, refreshErr := controller.commitPrerequisite(context.Background(), "ws-v2", "run-v2", PrerequisiteResult{
				PrerequisiteID: prerequisite.ID, OperationAdmissibility: &catalog, Observation: &observation,
			})
			if refreshErr != nil {
				t.Fatalf("refresh owner-fact acquisition catalog %q: %v", prerequisite.ID, refreshErr)
			}
			committed.Aggregate = refreshed
		}
	}
	if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("owner-fact acquisition did not reach assessment: aggregate=%+v err=%v", committed.Aggregate, err)
	}
	if retireAfterReceipt {
		if committed.Aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || len(committed.Aggregate.SemanticPlanning.History) != 1 {
			t.Fatalf("catalog refresh did not retire owner-fact acquisition semantic generation: %+v", committed.Aggregate.SemanticPlanning)
		}
		err = beginRuntimeAssessmentAttemptWithoutSemanticReseal(t, controller, "owner-fact-acquisition-assessment-prefix")
	} else {
		_, err = controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "owner-fact-acquisition-assessment-prefix")
	}
	if err != nil {
		t.Fatal(err)
	}
	assessment := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: committed.Aggregate.Facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "producer observed the acquisition receipt", EvidenceRefs: []string{"event-owner-fact-acquisition"},
	}}
	if _, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, assessment); err != nil {
		t.Fatal(err)
	}
	return controller, repository, bootstrapFacts, definition
}

func beginRuntimeAssessmentAttemptWithoutSemanticReseal(t *testing.T, controller *RuntimeController, stablePrefixHash string) error {
	t.Helper()
	aggregate, err := controller.Load(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		return err
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
			runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil),
		),
	)
	if err != nil {
		return err
	}
	_, session, err := controller.ensureCandidateRequestSession(
		context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeProviderDocumentOperations,
		stablePrefixHash, manifest.Hash, "",
	)
	if err != nil {
		return err
	}
	if session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
		physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
		if _, err := controller.markCandidateRequestSubmitted(context.Background(), "ws-v2", "run-v2", session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
			return err
		}
		if _, err := controller.completeCandidateRequestDocument(context.Background(), "ws-v2", "run-v2", session.LogicalRequestID, physicalCallID); err != nil {
			return err
		}
	}
	_, err = controller.beginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, stablePrefixHash, manifest, session.LogicalRequestID)
	return err
}

func runtimeTestAssessmentCatalogForPrerequisite(catalogs []runtimecontract.RuntimeOperationAdmissibility, prerequisiteID string) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	for _, catalog := range catalogs {
		catalog = catalog.Normalized()
		if catalog.PrerequisiteID == prerequisiteID {
			return catalog, true
		}
	}
	return runtimecontract.RuntimeOperationAdmissibility{}, false
}

func runtimeTestAssessmentSemanticCatalogObservation(catalog runtimecontract.RuntimeOperationAdmissibility, workspaceRevision string) runtimecontract.RuntimePrerequisiteObservation {
	catalog = catalog.Normalized()
	dependencies := []runtimecontract.RuntimeFreshnessDependency{{
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", Revision: workspaceRevision,
	}}
	if catalog.OwnerModuleID != runtimecontract.RuntimeModuleFile {
		dependencies = append(dependencies, runtimecontract.RuntimeFreshnessDependency{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: catalog.OwnerModuleID,
			SubjectRef: "owner-fact-validation-policy", Revision: catalog.ObservedRevision,
		})
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: catalog.OwnerModuleID, ContentHash: catalog.SnapshotID, Dependencies: dependencies,
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func runtimeTestAttachReceiptResourceChanges(t *testing.T, aggregate *RuntimeAggregate, evidence *ExecutionEvidence, changes []runtimecontract.RuntimeResourceChange) {
	t.Helper()
	if aggregate == nil || aggregate.ActiveDispatch == nil || evidence == nil || evidence.Receipt == nil {
		t.Fatal("resource-change receipt fixture requires active dispatch and owner receipt")
	}
	receipt := evidence.Receipt.Clone()
	receipt.ResourceChanges = append([]runtimecontract.RuntimeResourceChange(nil), changes...)
	receipt = receipt.Normalized()
	hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	for index := range aggregate.OperationReceipts {
		if aggregate.OperationReceipts[index].ExecutionKey == receipt.ExecutionKey && aggregate.OperationReceipts[index].Fence == receipt.Fence {
			aggregate.OperationReceipts[index] = receipt
		}
	}
	for index := range aggregate.ActiveDispatch.OperationExecutions {
		if aggregate.ActiveDispatch.OperationExecutions[index].OperationID == receipt.OperationID {
			aggregate.ActiveDispatch.OperationExecutions[index].OwnerReceiptHash = hash
		}
	}
	evidence.Receipt = &receipt
	evidence.OwnerReceiptHash = hash
}

func runtimeTestSelectableValidationOwnerFact() runtimecontract.RuntimeOwnerFactRequirement {
	return runtimecontract.RuntimeOwnerFactRequirement{
		OwnerModuleID: runtimecontract.RuntimeModuleShell, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
	}
}

func runtimeTestSelectableValidationOwnerFactConvergence() runtimecontract.RuntimeOwnerFactConvergence {
	return runtimecontract.RuntimeOwnerFactConvergence{
		Requirement: runtimeTestSelectableValidationOwnerFact(), MissingTrigger: runtimecontract.RuntimeOwnerFactMissingTrigger,
		AcquisitionOwnerModuleID: runtimecontract.RuntimeModuleFile, AcquisitionOperationKind: runtimecontract.StepKindFileWrite,
		AcquisitionTargetPolicy: runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, AcquisitionTargetSelectorRef: "workspace-relative-new-file-target",
		ProspectiveSatisfactionContractRef: "shell/selectable-validation-surface/prospective-workspace-manifest/v1",
		TargetSelectorRef:                  "file/workspace_deliverable", TargetSelectorOwnerRef: "test-profile", TargetSelectorSubjectRef: "validation_preparation",
		ReobserveOwnerModuleID: runtimecontract.RuntimeModuleShell, ReobserveFactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		MaxAcquisitionAttempts: 1,
	}
}

func runtimeTestSelectableValidationCatalog(t *testing.T) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	surface := runtimecontract.RuntimeValidationSurface{
		ID: "recipe/owner-fact-ready", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		Readiness:     runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "schema/owner-fact-v1", InputBindingRef: "workspace/owner-fact-v1",
		CheckRefs: []string{"check/owner-fact"}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
		EffectScope: runtimecontract.RuntimeEffectNone, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation-admissibility-shell-owner-fact", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeRefs: []string{surface.ID},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{surface}, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{{ID: surface.ID, Command: "true"}},
		ObservedRevision: "shell-owner-fact-v1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatalf("owner-fact test catalog: %v", err)
	}
	return catalog
}

func runtimeTestProspectiveValidationDependencyCatalog(t *testing.T, contractRef string) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	surface := runtimecontract.RuntimeValidationSurface{
		ID: "recipe/owner-fact-prospective", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		Readiness:     runtimecontract.RuntimeValidationSurfacePrerequisiteRequired, ReadinessReason: "applicable workspace input required",
		InputSchemaHash: "schema/owner-fact-prospective-v1", PrerequisiteRef: "workspace/owner-fact-validation-input",
		ProspectiveSatisfactionContractRef: contractRef,
		ProspectiveTargetSatisfactionConstraint: &runtimecontract.RuntimeProspectiveTargetSatisfactionConstraint{
			SchemaVersion: runtimecontract.RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
			OwnerModuleID: runtimecontract.RuntimeModuleShell, ProspectiveSatisfactionContractRef: contractRef,
			ValidationSurfaceID: "recipe/owner-fact-prospective", ValidationRecipeID: "recipe/owner-fact-prospective",
			InputSchemaHash: "schema/owner-fact-prospective-v1", CatalogObservedRevision: "shell-owner-fact-policy-v1",
			AllOf: []runtimecontract.RuntimeProspectiveTargetPredicate{{Kind: runtimecontract.RuntimeProspectiveTargetPredicatePathSuffix, Value: "_test.go"}},
		},
		CheckRefs: []string{"check/owner-fact-prospective"}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
		EffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation-admissibility-shell-owner-fact", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds:     []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{surface},
		ObservedRevision:   "shell-owner-fact-policy-v1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatalf("prospective owner-fact dependency catalog: %v", err)
	}
	return catalog
}
