package runtime

import (
	"context"
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
	controller, repository, bootstrapFacts := prepareOwnerFactAssessment(t, false)
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

func TestOwnerFactSemanticReplanBudgetCountsOnlyItsOwnBoundedLineage(t *testing.T) {
	facts := controllerFacts()
	facts.Focus.RequiredOwnerFacts = []runtimecontract.RuntimeOwnerFactRequirement{runtimeTestSelectableValidationOwnerFact()}
	facts.ConvergencePolicy.MaxRepairAttemptsPerFocus = 2
	facts.Coverage[0].ReopenLineage = []runtimecontract.RuntimeCWIReopenLineage{
		{Reason: "dependency_invalidated"},
		{Reason: runtimeCWIReopenReasonOwnerFactPostcondition},
	}
	if runtimeOwnerFactReplanBudgetExhausted(facts) {
		t.Fatal("unrelated CWI reopen lineage consumed the owner-fact replan budget")
	}
	facts.Coverage[0].ReopenLineage = append(facts.Coverage[0].ReopenLineage, runtimecontract.RuntimeCWIReopenLineage{Reason: runtimeCWIReopenReasonOwnerFactPostcondition})
	if !runtimeOwnerFactReplanBudgetExhausted(facts) {
		t.Fatal("owner-fact replan budget did not fail closed at the declared limit")
	}
}

func TestCommitAssessmentFailsClosedAfterOwnerFactReplanBudget(t *testing.T) {
	controller, repository, _ := prepareOwnerFactAssessment(t, false)
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
	repository.aggregate.Facts.ConvergencePolicy.MaxRepairAttemptsPerFocus = 1
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

func prepareOwnerFactAssessment(t *testing.T, ownerFactReady bool) (*RuntimeController, *controllerRepository, runtimecontract.RuntimeFacts) {
	t.Helper()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Focus.RequireAssessment = true
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Focus.RequiredOwnerFacts = []runtimecontract.RuntimeOwnerFactRequirement{runtimeTestSelectableValidationOwnerFact()}
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

func runtimeTestSelectableValidationOwnerFact() runtimecontract.RuntimeOwnerFactRequirement {
	return runtimecontract.RuntimeOwnerFactRequirement{
		OwnerModuleID: runtimecontract.RuntimeModuleShell, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
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
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{surface}, ObservedRevision: "shell-owner-fact-v1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatalf("owner-fact test catalog: %v", err)
	}
	return catalog
}
