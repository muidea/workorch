package runtime

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type durableRuntimeStateRepository struct {
	aggregate RuntimeAggregate
}

func (r *durableRuntimeStateRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	return r.aggregate, nil
}

func (r *durableRuntimeStateRepository) Save(_ context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	if r.aggregate.StateRevision != expectedRevision {
		return fmt.Errorf("%w: runtime aggregate revision changed", common.ErrConflict)
	}
	r.aggregate = aggregate
	return nil
}

func TestDispatchCommitAdvancesEvidenceAndReducerExactlyOnce(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	candidate := stateChangeCandidate()
	sealed := sealCandidateForTest(t, repository, candidate)
	if sealed.ActiveDispatch == nil || sealed.ActiveDispatch.Transaction != repository.aggregate.Decision.AuthorityTransaction() || sealed.ActiveDispatch.CandidateHash != runtimecontract.StableRuntimeCandidateHash(sealed.ActiveDispatch.Candidate) {
		t.Fatalf("sealed dispatch = %+v", sealed.ActiveDispatch)
	}
	stepInput := sealed.ActiveDispatch.StepInput()
	if stepInput.DispatchID == "" || stepInput.Transaction != sealed.ActiveDispatch.Transaction || stepInput.CandidateHash != sealed.ActiveDispatch.CandidateHash {
		t.Fatalf("dispatch step input did not persist complete transaction: %+v", stepInput)
	}
	evidence := ExecutionEvidence{
		EventID: "event-state-change-1", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted, SourceRef: "patch/1",
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Disposition != CommitDispositionCommitted || committed.Aggregate.EvidenceSequence != 1 || committed.Aggregate.Facts.EvidenceRevision != "runtime-evidence-00000000000000000001" {
		t.Fatalf("commit result = %+v", committed)
	}
	if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation || committed.Aggregate.ActiveDispatch != nil || len(committed.Aggregate.CompletedDispatches) != 1 {
		t.Fatalf("aggregate did not advance atomically: %+v", committed.Aggregate)
	}
	stateRevision := committed.Aggregate.StateRevision
	duplicate, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != stateRevision || len(duplicate.Aggregate.Facts.Evidence.Accepted) != 1 {
		t.Fatalf("duplicate commit changed state: %+v", duplicate)
	}
}

// C12/T23-T26: only the ref sealed on this operation can receive CWI credit,
// and only if the owner fact satisfies that binding's exact proof. A generic
// validation success and an unrelated real file mutation remain audit facts,
// never alternate coverage authorities.
func TestCoverageEvidenceCreditRequiresExactSealedBindingAndOwnerReceipt(t *testing.T) {
	facts := baseDispatchFacts()
	facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-a", "cwi-b"}
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{
		{ID: "cwi-a", State: runtimecontract.RuntimeCoverageItemStateOpen},
		{ID: "cwi-b", State: runtimecontract.RuntimeCoverageItemStateOpen},
	}
	facts.Focus.CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{
		testDispatchCoverageBinding("binding/a", "cwi-a", facts.Focus.ID, facts.DefinitionRevision, runtimecontract.EvidenceKindValidation, runtimecontract.RuntimeEffectEphemeralSession, runtimecontract.RuntimeBlockKindManagedValidationSession, "recipe-a", "check-a", "main.go"),
		testDispatchCoverageBinding("binding/b", "cwi-b", facts.Focus.ID, facts.DefinitionRevision, runtimecontract.EvidenceKindValidation, runtimecontract.RuntimeEffectEphemeralSession, runtimecontract.RuntimeBlockKindManagedValidationSession, "recipe-b", "check-b", "main.go"),
	}
	operation := runtimecontract.RuntimeOperation{ID: "validate-a", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, BindingRefs: []string{"binding/a"}, TargetRefs: []string{"main.go"}}
	receipt := &runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "exec/ws-1/run-1/dispatch/validate-a", Fence: 1, OperationID: "validate-a", Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectEphemeralSession, TargetRefs: []string{"main.go"},
		Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &runtimecontract.RuntimeValidationObservation{RecipeID: "recipe-a", ExecutedChecks: []string{"check-a"}, PassedChecks: []string{"check-a"}}},
	}
	refs, credit, diagnostic := coverageEvidenceCreditForReceipt(facts.Focus.CoverageEvidenceBindings, operation, runtimecontract.EvidenceKindValidation, receipt)
	if len(refs) != 1 || refs[0] != "binding/a" || len(credit) != 1 || credit[0] != "cwi-a" || diagnostic != "" {
		t.Fatalf("exact validation proof credit = refs=%v credit=%v diagnostic=%q", refs, credit, diagnostic)
	}

	// A real, accepted receipt that reports a check owned by another binding is
	// retained by the caller but cannot close either CWI through this operation.
	receipt.Evidence.Validation = &runtimecontract.RuntimeValidationObservation{RecipeID: "recipe-b", ExecutedChecks: []string{"check-b"}, PassedChecks: []string{"check-b"}}
	_, credit, diagnostic = coverageEvidenceCreditForReceipt(facts.Focus.CoverageEvidenceBindings, operation, runtimecontract.EvidenceKindValidation, receipt)
	if len(credit) != 0 || diagnostic == "" {
		t.Fatalf("foreign generic validation received credit=%v diagnostic=%q", credit, diagnostic)
	}

	// An unrelated durable go.mod mutation is still an owner-observed fact, but
	// it cannot borrow the main.go binding's target or state-change proof.
	stateFacts := baseDispatchFacts()
	stateFacts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	stateFacts.Focus.CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{
		testDispatchCoverageBinding("binding/main", "cwi-1", stateFacts.Focus.ID, stateFacts.DefinitionRevision, runtimecontract.EvidenceKindStateChange, runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeBlockKindPatchApply, "", "", "main.go"),
	}
	stateOperation := runtimecontract.RuntimeOperation{ID: "rewrite-mod", Kind: runtimecontract.RuntimeBlockKindPatchApply, BindingRefs: []string{"binding/main"}, TargetRefs: []string{"go.mod"}}
	stateReceipt := &runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "exec/ws-1/run-1/dispatch/rewrite-mod", Fence: 1, OperationID: "rewrite-mod", Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true, TargetRefs: []string{"go.mod"}, Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "updated go.mod"},
	}
	_, credit, diagnostic = coverageEvidenceCreditForReceipt(stateFacts.Focus.CoverageEvidenceBindings, stateOperation, runtimecontract.EvidenceKindStateChange, stateReceipt)
	if len(credit) != 0 || diagnostic == "" {
		t.Fatalf("unrelated durable target received credit=%v diagnostic=%q", credit, diagnostic)
	}

	// Even if a stale or corrupted aggregate bypasses candidate admission, a
	// single receipt cannot close two independently sealed CWI bindings.
	secondBinding := stateFacts.Focus.CoverageEvidenceBindings[0]
	secondBinding.ID = "binding/other"
	secondBinding.CoverageWorkItemID = "cwi-other"
	secondBinding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(secondBinding)
	stateFacts.Focus.CoverageEvidenceBindings = append(stateFacts.Focus.CoverageEvidenceBindings, secondBinding)
	stateOperation.TargetRefs = []string{"main.go"}
	stateOperation.BindingRefs = []string{"binding/main", "binding/other"}
	stateReceipt.TargetRefs = []string{"main.go"}
	_, credit, diagnostic = coverageEvidenceCreditForReceipt(stateFacts.Focus.CoverageEvidenceBindings, stateOperation, runtimecontract.EvidenceKindStateChange, stateReceipt)
	if len(credit) != 0 || !strings.Contains(diagnostic, "exactly one coverage evidence binding") {
		t.Fatalf("multi-binding receipt received credit=%v diagnostic=%q", credit, diagnostic)
	}
}

func TestDispatchCommitUsesPersistedReceiptEvidenceRatherThanEventCopy(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	runtimeTestEnsureCurrentFocusBindings(t, &aggregate)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, runtimeTestBindCandidateToCurrentFocus(t, aggregate, stateChangeCandidate()))
	evidence := ExecutionEvidence{
		EventID: "event-receipt-is-authority", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: runtimeTestActiveOperationID(t, sealed, 0),
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "owner receipt evidence", Quality: common.EvidenceQualityValid},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	// This represents a malformed internal-event producer.  The receipt is
	// already persisted, so the attempted replacement must neither enter the
	// evidence ledger nor influence SM-2 quality/coverage.
	evidence.Evidence = runtimecontract.RuntimeOperationEvidence{Summary: "forged event evidence", Quality: common.EvidenceQualityInconclusive, ZeroWork: true}
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(committed.Aggregate.Facts.Evidence.Accepted) != 1 {
		t.Fatalf("accepted evidence = %+v", committed.Aggregate.Facts.Evidence.Accepted)
	}
	fact := committed.Aggregate.Facts.Evidence.Accepted[0]
	if fact.Summary != "owner receipt evidence" || fact.Quality != common.EvidenceQualityValid || fact.ZeroWork {
		t.Fatalf("event copy replaced persisted owner receipt: %+v", fact)
	}
}

func testDispatchCoverageBinding(id string, coverageID common.CoverageWorkItemID, focusID, definitionRevision, evidenceKind string, scope runtimecontract.RuntimeEffectScope, operationKind, recipeID, checkRef, targetRef string) runtimecontract.CoverageEvidenceBinding {
	proof := runtimecontract.CoverageEvidenceProof{
		EvidenceKind: evidenceKind, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{scope}, AllowedOperationKinds: []string{operationKind},
		ValidationRecipeID: recipeID,
	}
	if checkRef != "" {
		proof.RequiredCheckRefs = []string{checkRef}
	}
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: id, CoverageWorkItemID: coverageID, FocusID: focusID, DefinitionRevision: definitionRevision,
		Proofs: []runtimecontract.CoverageEvidenceProof{proof}, TargetRefs: []string{targetRef}, DeclarationRef: "test/dispatch-binding",
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	return binding
}

func TestStaleEvidenceOnlyEntersDiagnosticLineage(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
	coordinator, _ := NewDispatchCoordinator(repository)
	currentFocus := repository.aggregate.Decision.Focus.ID
	currentRevision := repository.aggregate.Facts.EvidenceRevision
	result, err := coordinator.commitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-stale-1", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-old",
		AuthoritySnapshotID: "authority-old", OperationID: "patch-old", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != CommitDispositionStale || len(result.Aggregate.Diagnostics) != 1 || result.Aggregate.Decision.Focus.ID != currentFocus || result.Aggregate.Facts.EvidenceRevision != currentRevision || len(result.Aggregate.Facts.Evidence.Accepted) != 0 {
		t.Fatalf("stale evidence changed authority facts: %+v", result.Aggregate)
	}
	again, err := coordinator.commitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-stale-1", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-old",
		AuthoritySnapshotID: "authority-old", OperationID: "patch-old", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil || again.Disposition != CommitDispositionDuplicate || len(again.Aggregate.Diagnostics) != 1 {
		t.Fatalf("duplicate stale evidence = %+v err=%v", again, err)
	}
}

func TestReopenRuntimeCWIsPreservesAuditAndCanonicalQueueProjection(t *testing.T) {
	facts := canonicalReopenFixtureFacts(t)
	updated, reopened, err := reopenRuntimeCWIs(facts, []runtimeCWIReopenTrigger{{
		CoverageWorkItemID: "cwi-validation", TriggerRef: "state-after", Reason: "accepted state-change invalidated dependent validation",
	}})
	if err != nil || len(reopened) != 1 || reopened[0] != "cwi-validation" {
		t.Fatalf("canonical reopen = facts=%+v reopened=%v err=%v", updated, reopened, err)
	}
	byID := runtimeCoverageByID(updated.Coverage)
	dependent := byID["cwi-validation"]
	if dependent.State != runtimecontract.RuntimeCoverageItemStateOpen || dependent.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened ||
		dependent.CurrentBindingRef != nil || dependent.LastAssessmentID != "" || len(dependent.EvidenceRefs) != 1 || dependent.EvidenceRefs[0] != "validation-before" ||
		len(dependent.ReopenedByEvidenceRefs) != 1 || dependent.ReopenedByEvidenceRefs[0] != "state-after" || len(dependent.ReopenLineage) != 1 {
		t.Fatalf("dependent after canonical reopen = %+v", dependent)
	}
	if byID["cwi-unrelated"].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled || byID["cwi-source"].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateUnbound {
		t.Fatalf("unrelated/source CWI changed: %+v", byID)
	}
	contract, err := canonicalRuntimeCWIReconciliationContract(updated)
	if err != nil {
		t.Fatal(err)
	}
	updated.ReconciliationContract = contract
	if err := runtimecontract.ValidateRuntimeCWILedger(updated); err != nil {
		t.Fatalf("reopened canonical ledger is invalid: %v", err)
	}
}

func TestReopenRuntimeCWIsRequeuesCompletedFocusAheadOfPendingQueue(t *testing.T) {
	facts := canonicalReopenFixtureFacts(t)
	updated, reopened, err := reopenRuntimeCWIs(facts, []runtimeCWIReopenTrigger{{
		CoverageWorkItemID: "cwi-validation", TriggerRef: "state-after", Reason: "accepted state-change invalidated dependent validation",
	}})
	if err != nil || len(reopened) != 1 {
		t.Fatalf("canonical reopen = reopened=%v err=%v", reopened, err)
	}
	if len(updated.RemainingFocuses) != 2 || updated.RemainingFocuses[0].ID != "semantic-pending/cwi-validation" || updated.RemainingFocuses[1].ID != "focus-pending" ||
		len(updated.CompletedFocuses) != 1 || updated.CompletedFocuses[0].ID != "focus-unrelated" {
		t.Fatalf("canonical requeued focus queues = current=%+v remaining=%+v completed=%+v", updated.Focus, updated.RemainingFocuses, updated.CompletedFocuses)
	}
	// Normalization models the persisted/reloaded aggregate shape; the pending
	// focus remains a stable CWI projection rather than a recursive old ID.
	reloaded := updated.Normalized()
	if len(reloaded.RemainingFocuses) != 2 || reloaded.RemainingFocuses[0].ID != "semantic-pending/cwi-validation" || len(reloaded.CompletedFocuses) != 1 || reloaded.CompletedFocuses[0].ID != "focus-unrelated" {
		t.Fatalf("reloaded canonical focus queues = %+v", reloaded)
	}
}

func canonicalReopenFixtureFacts(t *testing.T) runtimecontract.RuntimeFacts {
	t.Helper()
	newBoundFocus := func(id, sliceID string, coverageID common.CoverageWorkItemID, target string) (runtimecontract.CoverageFocus, runtimecontract.RuntimeCWIBindingRef) {
		focus := runtimecontract.CoverageFocus{
			ID: id, SliceID: sliceID, CoverageWorkItemIDs: []common.CoverageWorkItemID{coverageID},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{target},
		}
		binding := runtimecontract.CoverageEvidenceBinding{
			ID: "binding/test/" + id + "/" + string(coverageID), CoverageWorkItemID: coverageID,
			FocusID: id, DefinitionRevision: "definition-reopen",
			Proofs: []runtimecontract.CoverageEvidenceProof{{
				EvidenceKind: runtimecontract.EvidenceKindStateChange, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
				AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			}},
			TargetRefs: []string{target}, DeclarationRef: "test/canonical-reopen",
		}.Normalized()
		binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
		focus.CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{binding}
		return focus, runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, "manifest-reopen")
	}
	validationFocus, validationRef := newBoundFocus("focus-validation", "slice-validation", "cwi-validation", "validation.txt")
	otherFocus, otherRef := newBoundFocus("focus-unrelated", "slice-other", "cwi-unrelated", "other.txt")
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-reopen", RunID: "run-reopen", DefinitionRevision: "definition-reopen", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		CWILedgerSchemaVersion: runtimecontract.RuntimeCWILedgerSchemaCurrent,
		Focus:                  runtimecontract.CoverageFocus{ID: "focus-source", SliceID: "slice-source", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}},
		RemainingFocuses:       []runtimecontract.CoverageFocus{{ID: "focus-pending", SliceID: "slice-pending", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-pending"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}},
		CompletedFocuses:       []runtimecontract.CoverageFocus{validationFocus, otherFocus},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{
			{ID: "cwi-source", SliceID: "slice-source", State: runtimecontract.RuntimeCoverageItemStateOpen, CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStateUnbound, FocusID: "focus-source"},
			{ID: "cwi-pending", SliceID: "slice-pending", State: runtimecontract.RuntimeCoverageItemStateOpen, CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStateUnbound, FocusID: "focus-pending"},
			{ID: "cwi-validation", SliceID: "slice-validation", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"validation-before"}, LastAssessmentID: "assessment-before", CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStateSettled, CurrentBindingRef: &validationRef, BindingLineage: []runtimecontract.RuntimeCWIBindingRef{validationRef}, FocusID: "focus-validation"},
			{ID: "cwi-unrelated", SliceID: "slice-other", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"other-before"}, CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStateSettled, CurrentBindingRef: &otherRef, BindingLineage: []runtimecontract.RuntimeCWIBindingRef{otherRef}, FocusID: "focus-unrelated"},
		},
	}
	contract, err := runtimecontract.CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, []runtimecontract.CoverageFocus{validationFocus, otherFocus})
	if err != nil {
		t.Fatal(err)
	}
	facts.ReconciliationContract = contract
	if err := runtimecontract.ValidateRuntimeCWILedger(facts); err != nil {
		t.Fatalf("canonical reopen fixture ledger: %v", err)
	}
	return facts
}

func TestDispatchCommitAtomicallyReopensAndRequeuesCompletedDependency(t *testing.T) {
	dependency := runtimecontract.RuntimeCoverageReopenDependency{CoverageWorkItemID: "cwi-validation", TriggerEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}
	aggregate := validationOnlyAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Focus = runtimecontract.CoverageFocus{
			ID: "focus-validation", SliceID: "slice-validation", AssessmentScope: "dodItem", AssessmentID: "dod-validation", AssessmentText: "validation remains current",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-validation"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, TargetRefs: []string{"validation.txt"},
		}
		aggregate.Facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
			ID: "focus-source", SliceID: "slice-source", AssessmentScope: "dodItem", AssessmentID: "dod-source", AssessmentText: "source state is current",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"main.go"},
		}}
		aggregate.Facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{
			{ID: "cwi-validation", SliceID: "slice-validation", SourceScope: "dodItem", SourceID: "dod-validation", TargetText: "validation remains current", State: runtimecontract.RuntimeCoverageItemStateOpen},
			{ID: "cwi-source", SliceID: "slice-source", SourceScope: "dodItem", SourceID: "dod-source", TargetText: "source state is current", State: runtimecontract.RuntimeCoverageItemStateOpen, ReopenDependencies: []runtimecontract.RuntimeCoverageReopenDependency{dependency}},
		}
		aggregate.Facts.DefinitionSeed = runtimecontract.RuntimeDefinitionSeed{
			SchemaVersion: runtimecontract.RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: aggregate.Facts.DefinitionRevision, PolicyHash: "policy-test",
			CoverageItems: []runtimecontract.RuntimeCoverageItem{
				{ID: "cwi-validation", SliceID: "slice-validation", SourceScope: "dodItem", SourceID: "dod-validation", TargetText: "validation remains current", Order: 0},
				{ID: "cwi-source", SliceID: "slice-source", SourceScope: "dodItem", SourceID: "dod-source", TargetText: "source state is current", Order: 1, ReopenDependencies: []runtimecontract.RuntimeCoverageReopenDependency{dependency}},
			},
			Slices: []runtimecontract.RuntimeAcceptanceSlice{
				{ID: "slice-validation", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-validation"}},
				{ID: "slice-source", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}},
			},
		}
		aggregate.Facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(aggregate.Facts.DefinitionSeed)
		aggregate.Facts.DefinitionFocuses = runtimeTestDefinitionFocuses(aggregate.Facts)
	})
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	validationCandidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "validate-dependency", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-validation"}, TargetRefs: []string{"validation.txt"}, Parameters: runtimeManagedValidationParameters(),
	}}}}}
	sealedValidation := sealCandidateForTest(t, repository, validationCandidate)
	validationProof, found := runtimecontract.CoverageEvidenceProofForKind(sealedValidation.Facts.Focus.CoverageEvidenceBindings[0], runtimecontract.EvidenceKindValidation)
	if !found {
		t.Fatal("sealed validation dependency has no exact validation proof")
	}
	validationEvidence := ExecutionEvidence{
		EventID: "event-validation-before-source", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealedValidation.ActiveDispatch.ID,
		AuthoritySnapshotID: sealedValidation.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: runtimeTestActiveOperationID(t, sealedValidation, 0),
		Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusAccepted,
		Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &runtimecontract.RuntimeValidationObservation{
			RecipeID: validationProof.ValidationRecipeID, ExecutedChecks: append([]string(nil), validationProof.RequiredCheckRefs...), PassedChecks: append([]string(nil), validationProof.RequiredCheckRefs...),
		}},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &validationEvidence)
	validated, err := coordinator.commitEvidence(context.Background(), validationEvidence)
	if err != nil || validated.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("validation dependency receipt did not reach assessment: result=%+v err=%v", validated, err)
	}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	assessed, err := runtimeTestCommitSatisfiedCoverageAssessment(context.Background(), controller, validated.Aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if len(assessed.Facts.Focus.CoverageWorkItemIDs) != 1 || assessed.Facts.Focus.CoverageWorkItemIDs[0] != "cwi-source" || len(assessed.Facts.CompletedFocuses) != 1 {
		t.Fatalf("sealed validation assessment did not advance to source focus: %+v", assessed.Facts)
	}
	// Focus advancement intentionally exposes an unbound semantic placeholder.
	// Complete the next private SM-2 semantic batch from the retained immutable
	// order before SM-3 is allowed to admit the source operation.
	assessed = runtimeTestSealPendingStateChangeFocus(t, controller, assessed)
	if assessed.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(assessed.Facts.Prerequisites.Pending) != 1 ||
		assessed.Facts.Prerequisites.Pending[0].Kind != runtimecontract.PrerequisiteKindTargetSnapshot {
		t.Fatalf("sealed source binding did not request its File-owned target snapshot: %+v", assessed)
	}
	assessed, err = controller.commitPrerequisite(context.Background(), assessed.Facts.WorkspaceID, assessed.Facts.RunID, PrerequisiteResult{
		PrerequisiteID: assessed.Facts.Prerequisites.Pending[0].ID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{
			TargetRef: "main.go", Content: "test semantic target", ContentHash: "test-semantic-snapshot/main.go", ContentState: runtimecontract.SnapshotContentStateComplete,
		},
	})
	if err != nil || assessed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("File target snapshot did not unlock sealed source operation: aggregate=%+v err=%v", assessed, err)
	}
	stateCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "patch-source", Kind: runtimecontract.RuntimeBlockKindPatchApply,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}, TargetRefs: []string{"main.go"}, Parameters: patchApplyTestParameters("main.go"),
	}}}}}
	sealed := sealCandidateForTest(t, repository, stateCandidate)
	evidence := ExecutionEvidence{EventID: "event-source-after-validation", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted, SourceRef: "patch/1"}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	canonicalEvidenceID := "runtime-receipt|" + runtimecontract.StableRuntimeOperationReceiptHash(*evidence.Receipt)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Aggregate.StateRevision <= sealed.StateRevision || len(committed.Aggregate.Facts.RemainingFocuses) != 1 ||
		len(committed.Aggregate.Facts.RemainingFocuses[0].CoverageWorkItemIDs) != 1 || committed.Aggregate.Facts.RemainingFocuses[0].CoverageWorkItemIDs[0] != "cwi-validation" || len(committed.Aggregate.Facts.CompletedFocuses) != 0 {
		t.Fatalf("dependency reopen was not atomically requeued: %+v", committed.Aggregate.Facts)
	}
	for _, item := range committed.Aggregate.Facts.Coverage {
		if item.ID == "cwi-validation" && (item.State != runtimecontract.RuntimeCoverageItemStateOpen || item.LastAssessmentID != "" || len(item.ReopenedByEvidenceRefs) != 1 || item.ReopenedByEvidenceRefs[0] != canonicalEvidenceID) {
			t.Fatalf("reopened dependent coverage = %+v", item)
		}
	}
	// The repository load is the restart boundary used by the coordinator. The
	// persisted queue and reopen lineage must survive it unchanged.
	reloaded, err := repository.Load(context.Background(), "ws-1", "run-1")
	if err != nil || len(reloaded.Facts.RemainingFocuses) != 1 || len(reloaded.Facts.RemainingFocuses[0].CoverageWorkItemIDs) != 1 || reloaded.Facts.RemainingFocuses[0].CoverageWorkItemIDs[0] != "cwi-validation" {
		t.Fatalf("reloaded dependency queue = %+v err=%v", reloaded.Facts, err)
	}
	duplicate, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != committed.Aggregate.StateRevision {
		t.Fatalf("duplicate dependency evidence changed aggregate: %+v err=%v", duplicate, err)
	}
}

func TestManagedValidationReceiptRejectsDirectDiagnosticTargetsAtCommitAndFreshLoad(t *testing.T) {
	for _, test := range []struct {
		name       string
		targetRefs []string
	}{
		{name: "current focus", targetRefs: []string{"main.go"}},
		{name: "validation entry", targetRefs: []string{"validation-entry.sh"}},
		{name: "sealed scope member", targetRefs: []string{"support.go"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &durableRuntimeStateRepository{aggregate: validationAggregate(t)}
			baselineReceiptCount := len(repository.aggregate.OperationReceipts)
			sealed := sealCandidateForTest(t, repository, validationCandidate())
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			operationID := runtimeTestActiveOperationID(t, sealed, 0)
			claimed, err := controller.claimOperationExecution(context.Background(), sealed.Facts.WorkspaceID, sealed.Facts.RunID, operationID)
			if err != nil {
				t.Fatal(err)
			}
			execution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, operationID)
			if !found {
				t.Fatal("claimed managed-validation execution is missing")
			}
			failure := runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassBuildFailure, Code: "invalid_direct_targets",
				Signature: "invalid-direct-managed-validation-targets", Message: "managed validation failed",
				TargetRefs: append([]string(nil), test.targetRefs...),
			}
			receipt := runtimecontract.RuntimeOperationReceipt{
				ExecutionKey: execution.ExecutionKey, OperationID: operationID,
				Outcome: runtimecontract.RuntimeOperationOutcomeRejected, ObservedEffectScope: runtimecontract.RuntimeEffectEphemeralSession,
				Feedback: runtimeOperationFeedbackForTest(t, claimed, operationID, failure),
			}
			if _, _, err := controller.commitOperationReceipt(context.Background(), claimed.Facts.WorkspaceID, claimed.Facts.RunID, claimed.ActiveDispatch.ID, execution.Fence, runtimecontract.RuntimeModuleShell, receipt); err == nil ||
				!strings.Contains(err.Error(), "managed-validation receipt cannot carry direct diagnostic target refs") {
				t.Fatalf("receipt commit error = %v", err)
			}
			if len(repository.aggregate.OperationReceipts) != baselineReceiptCount || repository.aggregate.Facts.Resources.FailureTarget != nil {
				t.Fatalf("invalid receipt changed Runtime authority: %+v", repository.aggregate)
			}

			invalid := repository.aggregate
			evidence := ExecutionEvidence{
				EventID: "invalid-managed-validation-targets", WorkspaceID: invalid.Facts.WorkspaceID, RunID: invalid.Facts.RunID,
				DispatchID: invalid.ActiveDispatch.ID, AuthoritySnapshotID: invalid.ActiveDispatch.Transaction.AuthoritySnapshotID,
				OperationID: operationID, Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusRejected,
				Failure: &failure,
			}
			attachRuntimeTestOwnerReceipt(t, &invalid, &evidence)
			if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "managed-validation receipt cannot carry direct diagnostic target refs") {
				t.Fatalf("fresh-load invalid receipt error = %v", err)
			}
		})
	}
}

func TestRejectedManagedValidationCrossOwnerTargetObservationFailsClosed(t *testing.T) {
	repository, coordinator, evidence := rejectedManagedValidationTargetObservationFixture(t, runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation",
		ObservedRevision: "mcp-server-v1", TargetRefs: []string{"implementation.go"},
	})
	if err := repository.aggregate.Validate(); err == nil || !strings.Contains(err.Error(), "validation-candidate receipt cannot carry a direct target observation") {
		t.Fatalf("fresh-load cross-owner target observation error = %v", err)
	}
	if _, err := coordinator.commitEvidence(context.Background(), evidence); err == nil ||
		!strings.Contains(err.Error(), "validation-candidate receipt cannot carry a direct target observation") {
		t.Fatalf("cross-owner target observation admission error = %v", err)
	}
	if repository.aggregate.Facts.Resources.FailureTarget != nil || len(repository.aggregate.ProcessedEventIDs) != 1 {
		t.Fatalf("cross-owner target observation changed Runtime authority: %+v", repository.aggregate)
	}
}

func TestRejectedManagedValidationDirectShellTargetObservationFailsClosed(t *testing.T) {
	repository, coordinator, evidence := rejectedManagedValidationTargetObservationFixture(t, runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", TargetRole: "implementation",
		ObservedRevision: "shell-policy-v1", TargetRefs: []string{"implementation.go"},
		DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed,
		EvidenceRefs:    []string{"workorch://shell/failure-target/sha256/test"}, ContentHash: strings.Repeat("a", 64),
	})
	if err := repository.aggregate.Validate(); err == nil || !strings.Contains(err.Error(), "validation-candidate receipt cannot carry a direct target observation") {
		t.Fatalf("fresh-load direct Shell target observation error = %v", err)
	}
	if _, err := coordinator.commitEvidence(context.Background(), evidence); err == nil ||
		!strings.Contains(err.Error(), "validation-candidate receipt cannot carry a direct target observation") {
		t.Fatalf("direct Shell target observation admission error = %v", err)
	}
	if repository.aggregate.Facts.Resources.FailureTarget != nil || len(repository.aggregate.ProcessedEventIDs) != 1 {
		t.Fatalf("direct Shell target observation changed Runtime authority: %+v", repository.aggregate)
	}
}

func rejectedManagedValidationTargetObservationFixture(t *testing.T, observation runtimecontract.RuntimeFailureTargetObservation) (*durableRuntimeStateRepository, *DispatchCoordinator, ExecutionEvidence) {
	t.Helper()
	aggregate := validationAggregate(t)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	evidence := ExecutionEvidence{
		EventID: "event-invalid-direct-target-observation", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassBuildFailure, Signature: "invalid-direct-target-observation", Message: "invalid direct target observation",
			TargetRefs: []string{"implementation.go"}, TargetObservation: &observation,
		},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	return repository, coordinator, evidence
}

func TestValidationFailureTargetObservationRequiresPromotedTargetAuthority(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{Focus: runtimecontract.CoverageFocus{TargetRole: "implementation", RequireTargetGrounding: true}}
	failure := runtimecontract.FailureFact{
		TargetRefs: []string{"implementation.go"},
		TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
			OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "mcp-server-v1", TargetRefs: []string{"implementation.go"},
		},
	}
	if validationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("failure target observation without a current owner freshness dependency was grounded")
	}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{{
		OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", Revision: "mcp-server-v1",
		}},
	}}
	if validationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("raw owner freshness observation recreated target authority")
	}
	facts.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: "failure", TargetRefs: []string{"implementation.go"}}
	if !validationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("promoted failure target authority was not grounded")
	}
}

func TestValidationFailureTargetObservationRejectsDirectShellAuthority(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Focus: runtimecontract.CoverageFocus{TargetRole: "implementation", RequireTargetGrounding: true},
		Resources: runtimecontract.RuntimeResourceFacts{PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{{
			OwnerModuleID: runtimecontract.RuntimeModuleShell,
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
				Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", Revision: "shell-policy-v1",
			}},
		}}},
	}
	failure := runtimecontract.FailureFact{
		TargetRefs:           []string{"implementation.go"},
		ValidationDiagnostic: &runtimecontract.ValidationFailureDiagnostic{Class: runtimecontract.ValidationDiagnosticBuildFailed},
		TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
			OwnerModuleID: runtimecontract.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", TargetRole: "implementation", ObservedRevision: "shell-policy-v1",
			TargetRefs: []string{"implementation.go"}, DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed,
			EvidenceRefs: []string{"workorch://shell/failure-target/sha256/test"}, ContentHash: fmt.Sprintf("%064x", 1),
		},
	}
	if validationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("content-bound Shell observation bypassed the validation-observation and owner-confirmation path")
	}
	failure.TargetObservation.SubjectRef = "command_target"
	if validationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("Shell command target became repair authority")
	}
	failure.TargetObservation.SubjectRef = "shell_runtime_policy"
	failure.ValidationDiagnostic = nil
	if validationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("Shell observation without matching owner diagnostic became repair authority")
	}
}

func TestRejectedValidationOwnerObservationRejectsMismatchedRole(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Focus: runtimecontract.CoverageFocus{TargetRole: "implementation", RequireTargetGrounding: true},
		Resources: runtimecontract.RuntimeResourceFacts{PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{{
			OwnerModuleID: runtimecontract.RuntimeModuleMCP,
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
				Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
				SubjectRef: "mcp_server_catalog", Revision: "mcp-server-v1",
			}},
		}}},
	}
	failure := runtimecontract.FailureFact{
		TargetRefs: []string{"implementation.go"},
		TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
			OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "validation-entry",
			ObservedRevision: "mcp-server-v1", TargetRefs: []string{"implementation.go"},
		},
	}
	if validationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("mismatched owner role became repair authority")
	}
}

func TestRejectedTextReplaceConsumesRepairBudgetAndRequiresFreshTargetSnapshot(t *testing.T) {
	aggregate := stateChangeAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Focus.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
		aggregate.Facts.Focus.AcceptedOperationVariants = []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "text_replace"}}
		catalog := runtimecontract.RuntimeOperationAdmissibility{
			PrerequisiteID: "test-text-replace-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
				Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "text_replace",
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
				ConstraintRefs:        []string{"workspace-relative-path"},
			}},
			TargetRefs: []string{"main.go"}, ConstraintRefs: []string{"workspace-relative-path"}, ObservedRevision: "text-replace-file-v1",
		}
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
		aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	})
	convergencePolicy := runtimecontract.DefaultRuntimeConvergencePolicy()
	convergencePolicy.MaxRepairAttemptsPerFocus = 3
	aggregate.Facts.ConvergencePolicy = convergencePolicy
	aggregate.Facts.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(aggregate.Facts.ConvergencePolicy, aggregate.Facts.RecoveryPolicy)
	ledger, err := runtimecontract.NewRuntimeExecutionBudgetLedger(aggregate.Facts.ExecutionBudgetPolicy)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.ExecutionBudget = ledger
	aggregate.Facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "main.go", Content: "package main\n", ContentHash: "old-hash", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	oldText := "func missing() {}"
	newText := "func present() {}"
	patchType := "text_replace"
	targetRef := "main.go"
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "patch-1", Kind: runtimecontract.RuntimeBlockKindPatchApply,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{targetRef},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText},
		},
	}}}}}
	sealed := sealCandidateForTest(t, repository, candidate)
	evidence := ExecutionEvidence{
		EventID: "event-patch-conflict", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "text_replace_not_found",
			Signature: "patch-text-replace-not-found", Message: "oldText not found", TargetRefs: []string{"main.go"},
		},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	rejected, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Aggregate.Facts.Repair.RepairAttempts != 1 || rejected.Aggregate.Facts.Repair.ActiveGap == nil {
		t.Fatalf("repair ledger = %+v", rejected.Aggregate.Facts.Repair)
	}
	if rejected.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(rejected.Aggregate.Facts.Prerequisites.Pending) != 1 {
		t.Fatalf("rejected patch decision = %+v prerequisites=%+v", rejected.Aggregate.Decision, rejected.Aggregate.Facts.Prerequisites.Pending)
	}
	prerequisite := rejected.Aggregate.Facts.Prerequisites.Pending[0]
	if prerequisite.Kind != runtimecontract.PrerequisiteKindTargetSnapshot || prerequisite.SubjectRef != "main.go" || len(rejected.Aggregate.Facts.Resources.TargetSnapshots) != 0 {
		t.Fatalf("fresh snapshot prerequisite = %+v resources=%+v", prerequisite, rejected.Aggregate.Facts.Resources)
	}
	revision := rejected.Aggregate.StateRevision
	duplicate, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != revision || duplicate.Aggregate.Facts.Repair.RepairAttempts != 1 {
		t.Fatalf("duplicate rejection changed repair authority: %+v", duplicate)
	}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := controller.commitPrerequisite(context.Background(), "ws-1", "run-1", PrerequisiteResult{
		PrerequisiteID: prerequisite.ID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{
			TargetRef: "main.go", Content: "package main\nfunc present() {}\n", ContentHash: "fresh-hash", ContentState: runtimecontract.SnapshotContentStateComplete,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(refreshed.Facts.Prerequisites.Pending) != 0 ||
		len(refreshed.Facts.Resources.TargetSnapshots) != 1 || refreshed.Facts.Resources.TargetSnapshots[0].ContentHash != "fresh-hash" {
		t.Fatalf("fresh snapshot did not return to same-focus repair: %+v", refreshed)
	}
}

func TestCoordinatorRestartRecoversPersistedDispatchWithoutProcessState(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
	sealed := sealCandidateForTest(t, repository, stateChangeCandidate())
	second, _ := NewDispatchCoordinator(repository)
	commitEvidence := ExecutionEvidence{
		EventID: "event-after-restart", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &commitEvidence)
	result, err := second.commitEvidence(context.Background(), commitEvidence)
	if err != nil || result.Disposition != CommitDispositionCommitted {
		t.Fatalf("restart commit = %+v err=%v", result, err)
	}
}

func TestDispatchCoordinatorCommitsMultiOperationDispatchOnlyAfterAllOperations(t *testing.T) {
	// This dispatch has two CWI-crediting state changes. Each owner receipt is an
	// immutable proof input, but neither may bypass the separately owned
	// assessment transition. SM-3 also keeps the dispatch active until every
	// sealed operation has committed its owner receipt.
	facts := baseDispatchFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Focus.SliceID = "slice-1"
	facts.Focus.AssessmentScope = "dodItem"
	facts.Focus.AssessmentID = "dod-1"
	facts.Focus.AssessmentText = "deliver cwi 1"
	facts.Coverage[0].SliceID = facts.Focus.SliceID
	facts.Coverage[0].SourceScope = facts.Focus.AssessmentScope
	facts.Coverage[0].SourceID = facts.Focus.AssessmentID
	facts.Coverage[0].TargetText = facts.Focus.AssessmentText
	facts.DefinitionSeed = runtimecontract.RuntimeDefinitionSeed{
		SchemaVersion: runtimecontract.RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: facts.DefinitionRevision, PolicyHash: "policy-test",
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "deliver cwi 1"}},
		Slices:        []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}},
	}
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
	aggregate := RuntimeAggregate{StateRevision: 1, Definition: dispatchDefinition(), Facts: facts}
	runtimeTestEnsureCurrentFocusBindings(t, &aggregate)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		{Operation: patchApplyTestOperation("write-1", "main.go")},
		{Operation: func() runtimecontract.RuntimeOperation {
			operation := patchApplyTestOperation("write-2", "main.go")
			// Give the second sealed operation a distinct execution shape while
			// retaining the exact proof binding required by the manifest.
			otherContent := "second update\n"
			for index := range operation.Parameters {
				if operation.Parameters[index].Name == "content" {
					operation.Parameters[index].String = &otherContent
				}
			}
			return operation
		}()},
	}}}
	sealed := sealCandidateForTest(t, repository, candidate)
	firstOperationID := runtimeTestActiveOperationID(t, sealed, 0)
	secondOperationID := runtimeTestActiveOperationID(t, sealed, 1)
	firstEvidence := ExecutionEvidence{
		EventID: "event-write-1", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: firstOperationID,
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &firstEvidence)
	first, err := coordinator.commitEvidence(context.Background(), firstEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if first.DispatchCompleted || first.Aggregate.ActiveDispatch == nil || len(first.Aggregate.ActiveDispatch.CompletedOperationIDs) != 1 {
		t.Fatalf("first operation prematurely completed dispatch: %+v", first)
	}
	if first.Aggregate.Decision.AuthoritySnapshot.ID != sealed.Decision.AuthoritySnapshot.ID {
		t.Fatalf("decision changed before dispatch completion")
	}
	for _, coverage := range first.Aggregate.Facts.Coverage {
		if coverage.ID == "cwi-1" && (coverage.State != runtimecontract.RuntimeCoverageItemStateOpen || coverage.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound) {
			t.Fatalf("partial dispatch receipt bypassed assessment-owned CWI settlement: %+v", coverage)
		}
	}
	if err := first.Aggregate.Validate(); err != nil {
		t.Fatalf("first multi-operation receipt left an invalid aggregate: %v", err)
	}
	secondEvidence := ExecutionEvidence{
		EventID: "event-write-2", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: secondOperationID,
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &secondEvidence)
	second, err := coordinator.commitEvidence(context.Background(), secondEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if !second.DispatchCompleted || second.Aggregate.ActiveDispatch != nil || len(second.Aggregate.CompletedDispatches) != 1 {
		t.Fatalf("second operation did not complete dispatch: %+v", second)
	}
	if second.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment || second.Aggregate.EvidenceSequence != 2 || len(second.Aggregate.Facts.Evidence.Accepted) != 2 {
		t.Fatalf("completed multi-operation receipt set did not reach assessment with a contiguous evidence ledger: %+v", second.Aggregate)
	}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	assessed, err := runtimeTestCommitSatisfiedCoverageAssessment(context.Background(), controller, second.Aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if assessed.EvidenceSequence != 3 || len(assessed.Facts.Assessments) != 1 || len(assessed.Facts.Coverage) != 1 ||
		assessed.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateSatisfied || assessed.Facts.Coverage[0].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled {
		t.Fatalf("sealed assessment did not settle the multi-operation proof set exactly once: %+v", assessed)
	}
}

func TestDispatchCommitAdvancesToNextCoverageFocusDeterministically(t *testing.T) {
	aggregate := stateChangeAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
		aggregate.Facts.Focus.SliceID = "slice-1"
		aggregate.Facts.Focus.AssessmentScope = "dodItem"
		aggregate.Facts.Focus.AssessmentID = "dod-1"
		aggregate.Facts.Focus.AssessmentText = "deliver cwi 1"
		aggregate.Facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
			ID: "focus-2", SliceID: "slice-2", AssessmentScope: "dodItem", AssessmentID: "dod-2", AssessmentText: "deliver cwi 2",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"other.go"},
		}}
		aggregate.Facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{
			{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "deliver cwi 1", State: runtimecontract.RuntimeCoverageItemStateOpen},
			{ID: "cwi-2", SliceID: "slice-2", SourceScope: "dodItem", SourceID: "dod-2", TargetText: "deliver cwi 2", State: runtimecontract.RuntimeCoverageItemStateOpen},
		}
		aggregate.Facts.DefinitionSeed = runtimecontract.RuntimeDefinitionSeed{
			SchemaVersion: runtimecontract.RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: aggregate.Facts.DefinitionRevision, PolicyHash: "policy-test",
			CoverageItems: []runtimecontract.RuntimeCoverageItem{
				{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "deliver cwi 1"},
				{ID: "cwi-2", SliceID: "slice-2", SourceScope: "dodItem", SourceID: "dod-2", TargetText: "deliver cwi 2"},
			},
			Slices: []runtimecontract.RuntimeAcceptanceSlice{
				{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
				{ID: "slice-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}},
			},
		}
		aggregate.Facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(aggregate.Facts.DefinitionSeed)
	})
	initialFocusID := aggregate.Facts.Focus.ID
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	candidate := runtimeTestBindCandidateToCurrentFocus(t, aggregate, stateChangeCandidate())
	sealed := sealCandidateForTest(t, repository, candidate)
	commitEvidence := ExecutionEvidence{
		EventID: "event-focus-1", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &commitEvidence)
	result, err := coordinator.commitEvidence(context.Background(), commitEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.Facts.Focus.ID != initialFocusID || result.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment || result.Aggregate.EvidenceSequence != 1 {
		t.Fatalf("state-change receipt bypassed mandatory assessment before focus advance: %+v", result.Aggregate)
	}
	if len(result.Aggregate.Facts.Evidence.Accepted) != 1 || len(result.Aggregate.Facts.Evidence.Accepted[0].CoverageWorkItemIDs) != 1 ||
		result.Aggregate.Facts.Evidence.Accepted[0].CoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatalf("focus-bound evidence = %+v", result.Aggregate.Facts.Evidence.Accepted)
	}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	assessed, err := runtimeTestCommitSatisfiedCoverageAssessment(context.Background(), controller, result.Aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if len(assessed.Facts.Focus.CoverageWorkItemIDs) != 1 || assessed.Facts.Focus.CoverageWorkItemIDs[0] != "cwi-2" || len(assessed.Facts.RemainingFocuses) != 0 ||
		assessed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || assessed.Decision.Focus.ID != assessed.Facts.Focus.ID || assessed.EvidenceSequence != 2 {
		t.Fatalf("sealed assessment did not advance deterministically to the next focus: %+v", assessed)
	}
}

func stateChangeAggregate(t *testing.T) RuntimeAggregate {
	return stateChangeAggregateWithPreparation(t, nil)
}

// runtimeTestSealPendingStateChangeFocus drives the production private-event
// sequence for the next proof batch retained by an already accepted semantic
// order. It is deliberately narrower than rebuilding a test semantic ledger:
// the original manifest, order, owner catalog and pending identity remain the
// only authority for the second focus.
func runtimeTestSealPendingStateChangeFocus(t *testing.T, controller *RuntimeController, aggregate RuntimeAggregate) RuntimeAggregate {
	t.Helper()
	if controller == nil || aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningOrderAccepted ||
		aggregate.SemanticPlanning.Manifest == nil || aggregate.SemanticPlanning.CoverageOrderDocument == nil ||
		len(aggregate.Facts.Focus.CoverageWorkItemIDs) != 1 || len(aggregate.Facts.Focus.TargetRefs) != 1 {
		t.Fatalf("pending semantic state-change fixture is incomplete: focus=%+v ledger=%+v", aggregate.Facts.Focus, aggregate.SemanticPlanning)
	}
	manifest := aggregate.SemanticPlanning.Manifest.Clone()
	order := aggregate.SemanticPlanning.CoverageOrderDocument.Clone()
	coverageID := aggregate.Facts.Focus.CoverageWorkItemIDs[0]
	operationKind := runtimecontract.RuntimeBlockKindPatchApply
	variantID := "file_write"
	var catalog runtimecontract.RuntimeOperationAdmissibility
	for _, raw := range manifest.OwnerCatalogs {
		candidate := raw.Normalized()
		if candidate.OwnerModuleID != runtimecontract.RuntimeModuleFile {
			continue
		}
		for _, kind := range candidate.OperationKinds {
			if runtimecontract.CanonicalRuntimeBlockKind(kind) == operationKind {
				catalog = candidate
				break
			}
		}
		if catalog.SnapshotID != "" {
			break
		}
	}
	if catalog.SnapshotID == "" {
		t.Fatal("retained semantic manifest has no exact File state-change catalog")
	}
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: coverageID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKind: operationKind, OperationVariantID: variantID,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: append([]string(nil), aggregate.Facts.Focus.TargetRefs...),
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:        "test/semantic/" + string(coverageID),
	}.Normalized()
	batch, err := semanticplan.CompileFocusBatch(
		manifest, order, aggregate.SemanticPlanning.NextBatchSequence, "dispatch-reopen-source-batch",
		aggregate.Facts.Focus.CoverageWorkItemIDs, aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	apply := func(event RuntimeInternalEvent) {
		t.Helper()
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, applyErr := controller.ApplyInternalEvent(context.Background(), event)
		if applyErr != nil {
			t.Fatalf("apply pending semantic %s: %v", event.Kind, applyErr)
		}
		aggregate = result.Aggregate
	}
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch, BatchID: batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
		Proposals:                []runtimecontract.RuntimeSemanticProofProposal{proposal},
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &document, LogicalRequestID: batch.RequestLogicalID}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	feasibility := runtimecontract.RuntimeSemanticProofFeasibility{
		SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: coverageID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal),
		OwnerModuleID: catalog.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID,
		Status: runtimecontract.RuntimeSemanticFeasibilityFeasible, ObservedRevision: catalog.ObservedRevision,
		TargetRefs: append([]string(nil), aggregate.Facts.Focus.TargetRefs...),
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility}})
	accepted := aggregate.SemanticPlanning.Batches[len(aggregate.SemanticPlanning.Batches)-1]
	acceptedDocument, found := runtimeSemanticBatchDocument(aggregate.SemanticPlanning.BatchDocuments, accepted.BatchID)
	if !found {
		t.Fatalf("accepted pending semantic batch %q has no exact document", accepted.BatchID)
	}
	batchResult := runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{
		BatchID: accepted.BatchID, BatchHash: accepted.Hash,
		DocumentHash: runtimecontract.StableRuntimeSemanticPlanDocumentHash(acceptedDocument), Feasibilities: append([]runtimecontract.RuntimeSemanticProofFeasibility(nil), aggregate.SemanticPlanning.Feasibilities...),
	}
	batchResult.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(batchResult)
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchFeasibilityObserved, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanBatchFeasibilityResult: &batchResult}})
	accepted = aggregate.SemanticPlanning.Batches[len(aggregate.SemanticPlanning.Batches)-1]
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, accepted, document)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &merged}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
	if len(aggregate.Facts.Focus.CoverageEvidenceBindings) != 1 || aggregate.Facts.Focus.CoverageEvidenceBindings[0].CoverageWorkItemID != coverageID {
		t.Fatalf("pending semantic batch did not seal exact current binding: %+v", aggregate.Facts.Focus)
	}
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange && aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("pending semantic state-change seal decision = %+v prerequisites=%+v", aggregate.Decision, aggregate.Facts.Prerequisites)
	}
	return aggregate
}

// stateChangeAggregateWithPreparation lets focused fixtures declare their
// immutable Definition-focus target and proof inputs before SM-2 seals the
// semantic provider/owner preimage. Runtime repair facts may be added later,
// but no caller may rewrite Focus or its CoverageEvidenceBinding post-seal.
func stateChangeAggregateWithPreparation(t *testing.T, prepare func(*RuntimeAggregate)) RuntimeAggregate {
	t.Helper()
	facts := baseDispatchFacts()
	aggregate := RuntimeAggregate{StateRevision: 1, Definition: dispatchDefinition(), Facts: facts}
	if prepare != nil {
		prepare(&aggregate)
	}
	aggregate.Facts.DefinitionFocuses = nil
	aggregate.Facts.DefinitionFocuses = runtimeTestDefinitionFocuses(aggregate.Facts)
	runtimeTestEnsureCurrentFocusBindings(t, &aggregate)
	return aggregate
}

func TestRuntimeAggregateRejectsTerminalDecisionWithPendingCandidateAttempt(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	transaction := aggregate.Decision.AuthorityTransaction()
	aggregate.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{
		AuthorityContractID: runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction),
		Transaction:         transaction,
		Attempt:             0,
		StablePrefixHash:    "pending-prefix",
		State:               runtimecontract.RuntimeCandidateAttemptStatePending,
	}}

	for _, terminal := range []runtimecontract.RuntimeDecision{
		sealedTerminalDecision(t, aggregate.Facts, runtimecontract.ConvergenceStateFailed, runtimecontract.ConvergenceStateFailed, "failed"),
		sealedTerminalDecision(t, aggregate.Facts, runtimecontract.ConvergenceStateComplete, runtimecontract.ConvergenceStateComplete, "complete"),
	} {
		aggregate.Decision = terminal
		if err := aggregate.Validate(); err == nil {
			t.Fatalf("terminal decision %+v accepted a pending candidate attempt", terminal)
		}
	}
}

func TestRuntimeAggregateRequiresReducerTransitionAfterCandidateAttemptOneFailure(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction)
	failure := &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_replays_rejected_shape", Reason: "replayed shape"}
	aggregate.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{
		{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, StablePrefixHash: "stable-prefix", State: runtimecontract.RuntimeCandidateAttemptStateFailed, Failure: failure},
		{AuthorityContractID: contractID, Transaction: transaction, Attempt: 1, StablePrefixHash: "stable-prefix", State: runtimecontract.RuntimeCandidateAttemptStateFailed, Failure: failure},
	}
	bindRuntimeCandidateAttemptForTest(t, &aggregate, 0)
	bindRuntimeCandidateAttemptForTest(t, &aggregate, 1)
	aggregate.CandidateAttempts[0].Failure = &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid", Reason: "required operations field is missing",
		Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
			Path: "$.operations", Actual: "missing_operations", Required: []string{"operations"},
			ManifestHash: aggregate.CandidateAttempts[0].ManifestHash,
			Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField},
		},
	}
	aggregate.CandidateAttempts[0].Correction = (&runtimecontract.RuntimeCandidateCorrectionDirective{
		Disposition:  runtimecontract.RuntimeCandidateCorrectionCorrectableOnce,
		Objectives:   []runtimecontract.RuntimeCandidateCorrectionObjective{runtimecontract.RuntimeCandidateCorrectionSupplyRequiredField},
		ManifestHash: aggregate.CandidateAttempts[0].ManifestHash,
		Witness: &runtimecontract.RuntimeCandidateCorrectionWitness{
			Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField,
			Path:      "$.operations", RejectedValue: "field_absent", AlternativeValue: "field_present",
		},
	}).Clone()
	aggregate.CandidateAttempts[1].Correction = (&runtimecontract.RuntimeCandidateCorrectionDirective{
		Disposition:  runtimecontract.RuntimeCandidateCorrectionRejectedFinal,
		ManifestHash: aggregate.CandidateAttempts[1].ManifestHash,
		TerminalCode: "candidate_retry_exhausted",
	}).Clone()
	if err := aggregate.Validate(); err == nil {
		t.Fatal("same-authority attempt 1 failure was accepted without a reducer transition")
	}
	aggregate.Facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateExhausted, Code: "candidate_retry_exhausted", Signature: "candidate-retry-exhausted", Message: "replayed shape"}
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("reduced exhausted candidate state rejected: %v", err)
	}
}

func sealedTerminalDecision(t *testing.T, facts runtimecontract.RuntimeFacts, state runtimecontract.ConvergenceState, _ runtimecontract.ConvergenceState, code string) runtimecontract.RuntimeDecision {
	t.Helper()
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: state, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{},
		Terminal:       &runtimecontract.RuntimeTerminalDecision{Code: code},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

// sealCandidateForTest builds a persisted post-seal fixture without retaining a
// production alternate dispatch entry point. Production code must use the
// RuntimeController candidate-attempt admission path.
func sealCandidateForTest(t *testing.T, repository *durableRuntimeStateRepository, candidate runtimecontract.RuntimeCandidate) RuntimeAggregate {
	t.Helper()
	aggregate := repository.aggregate
	bound := runtimeTestBindCandidateToCurrentFocus(t, aggregate, candidate)
	manifest, err := BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	feasibilities := runtimeTestCurrentCandidateFeasibilities(t, aggregate, manifest, bound)
	return sealCandidateWithFeasibilitiesForTest(t, repository, candidate, feasibilities)
}

func sealCandidateWithFeasibilitiesForTest(t *testing.T, repository *durableRuntimeStateRepository, candidate runtimecontract.RuntimeCandidate, feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord) RuntimeAggregate {
	t.Helper()
	aggregate := repository.aggregate
	candidate = runtimeTestBindCandidateToCurrentFocus(t, aggregate, candidate)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
			runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	repairCommitment, err := deriveConfirmedRepairAdmissionCommitment(aggregate, manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	repairCommitmentHash := ""
	if repairCommitment != nil {
		repairCommitmentHash = runtimecontract.StableRuntimeDispatchRepairCommitmentHash(*repairCommitment)
	}
	admissionTargetAuthorizations, err := runtimecontract.RuntimeDispatchOperationTargetAuthorizationsFromManifest(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	admissionValidationRecipes, err := runtimecontract.RuntimeCandidateManagedValidationRecipeSnapshot(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	effectiveActivation := runtimecontract.RuntimeActivationForFacts(aggregate.Facts)
	dispatch, err := SealDispatchWithManifestAndPreSealAuthority(
		aggregate.Decision,
		candidate,
		effectiveActivation,
		manifest,
		feasibilities,
		aggregate.Facts.Resources.OperationAdmissibilities,
		aggregate.Facts.Resources.PrerequisiteObservations,
		RuntimeDispatchPreSealAuthority{RepairCommitment: cloneRuntimeDispatchRepairCommitment(repairCommitment)},
	)
	if err != nil {
		t.Fatal(err)
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	stablePrefixHash := "test-sealed-dispatch"
	logicalRequestID := runtimecontract.StableRuntimeCandidateRequestSessionID(runtimecontract.RuntimeProviderDocumentOperations, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction, stablePrefixHash, manifest.Hash, "test-sealed-dispatch")
	manifestCopy := manifest.Clone()
	aggregate.CandidateRequestSessions = append(aggregate.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{
		LogicalRequestID: logicalRequestID, DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, Transaction: transaction, StablePrefixHash: stablePrefixHash, ManifestHash: manifest.Hash,
		CandidateManifest:    &manifestCopy,
		RepairCommitment:     cloneRuntimeDispatchRepairCommitment(repairCommitment),
		RepairCommitmentHash: repairCommitmentHash,
		CorrectionHash:       "test-sealed-dispatch", State: runtimecontract.RuntimeCandidateRequestSessionDocumentComplete, NextSequence: 1,
		LastPhysicalCallID: runtimecontract.StableRuntimeCandidatePhysicalCallID(logicalRequestID, 1), ProviderSubmitted: true,
		LastOutcome: runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete},
	})
	attemptRecord := runtimecontract.RuntimeCandidateAttemptRecord{
		AuthorityContractID: runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction),
		Transaction:         transaction,
		Attempt:             0,
		StablePrefixHash:    stablePrefixHash,
		ManifestHash:        manifest.Hash,
		LogicalRequestID:    logicalRequestID,
		State:               runtimecontract.RuntimeCandidateAttemptStateAccepted,
		CandidateHash:       dispatch.CandidateHash,
		DispatchID:          dispatch.ID,
		AdmissionTargetAuthorizations: runtimecontract.CloneRuntimeDispatchOperationTargetAuthorizations(
			admissionTargetAuthorizations,
		),
		TargetAuthorizationHash: runtimecontract.StableRuntimeDispatchOperationTargetAuthorizationsHash(
			admissionTargetAuthorizations,
		),
		AdmissionCoverageBindings:    runtimecontract.CloneCoverageEvidenceBindings(manifest.Bindings),
		AdmissionCoverageBindingHash: runtimecontract.StableRuntimeCoverageEvidenceBindingSnapshotHash(manifest.Bindings),
		AdmissionValidationRecipes:   runtimecontract.CloneRuntimeValidationRecipes(admissionValidationRecipes),
		AdmissionRepairCommitment:    cloneRuntimeDispatchRepairCommitment(repairCommitment),
		RepairCommitmentHash:         repairCommitmentHash,
		DispatchAuthorityHash:        dispatch.DispatchAuthorityHash,
	}
	if len(admissionValidationRecipes) > 0 {
		attemptRecord.ValidationRecipeHash = runtimecontract.StableRuntimeValidationRecipeSnapshotHash(admissionValidationRecipes)
	}
	aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, attemptRecord)
	aggregate.ActiveDispatch = &dispatch
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	repository.aggregate = aggregate
	return aggregate
}

func validationAggregate(t *testing.T) RuntimeAggregate {
	return validationAggregateWithPreparation(t, nil)
}

// validationAggregateWithPreparation lets focused integration fixtures install
// their immutable Definition-focus inputs and owner preflight facts before the
// semantic contract is sealed. Post-seal Focus or binding rewrites would have
// no provider/owner preimage and are intentionally rejected by aggregate
// validation.
func validationAggregateWithPreparation(t *testing.T, prepare func(*RuntimeAggregate)) RuntimeAggregate {
	return validationAggregatePrepared(t, prepare, true)
}

func validationOnlyAggregateWithPreparation(t *testing.T, prepare func(*RuntimeAggregate)) RuntimeAggregate {
	return validationAggregatePrepared(t, prepare, false)
}

func validationAggregatePrepared(t *testing.T, prepare func(*RuntimeAggregate), commitStateChange bool) RuntimeAggregate {
	t.Helper()
	// Seal the semantic/candidate authority only after the Shell-owned recipe
	// inventory is present. Reusing stateChangeAggregate would retain a sealed
	// manifest created before that owner fact and make the validation recipe
	// unavailable by construction.
	validationFacts := baseDispatchFacts()
	validationFacts.Coverage[0].SliceID = "slice-validation"
	validationFacts.Coverage[0].SourceScope = "dodItem"
	validationFacts.Coverage[0].SourceID = "dod-item-1"
	validationFacts.Coverage[0].TargetText = "validation succeeds"
	validationFacts.Focus.SliceID = "slice-validation"
	validationFacts.Focus.AssessmentScope = "dodItem"
	validationFacts.Focus.AssessmentID = "dod-item-1"
	validationFacts.Focus.AssessmentText = "validation succeeds"
	if !commitStateChange {
		validationFacts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	}
	initial := RuntimeAggregate{
		StateRevision: 1,
		Definition:    dispatchDefinition(),
		Facts:         validationFacts,
	}
	if prepare != nil {
		prepare(&initial)
	}
	runtimeTestEnsureCurrentFocusBindings(t, &initial)
	if !commitStateChange {
		if initial.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
			t.Fatalf("validation-only fixture did not require validation: %+v", initial.Decision)
		}
		return initial
	}
	repository := &durableRuntimeStateRepository{aggregate: initial}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	stateCandidate := stateChangeCandidate()
	if len(initial.Facts.Focus.TargetRefs) != 0 {
		targetRef := initial.Facts.Focus.TargetRefs[0]
		stateCandidate.StateChange.Operations[0].Operation.TargetRefs = []string{targetRef}
		stateCandidate.StateChange.Operations[0].Operation.Parameters = patchApplyTestParameters(targetRef)
	}
	sealed := sealCandidateForTest(t, repository, stateCandidate)
	evidence := ExecutionEvidence{
		EventID: "validation-fixture-state-change", WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID,
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindStateChange,
		Status: ExecutionEvidenceStatusAccepted, SourceRef: "validation-fixture/state-change",
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatalf("commit production state-change validation fixture: %v", err)
	}
	if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation || committed.Aggregate.ActiveDispatch != nil {
		t.Fatalf("production state-change fixture did not require validation: %+v", committed.Aggregate.Decision)
	}
	// The state-change receipt is immutable authority needed by validation
	// fixtures, but it predates the repair cycle exercised by their tests. Keep
	// the cycle-local progress ledger empty, as it was at the original
	// need_validation fixture boundary.
	aggregate := committed.Aggregate
	aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: aggregate.Facts.Focus.ID,
		SliceID: aggregate.Facts.Focus.SliceID,
	}
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("reset validation fixture repair cycle: %v", err)
	}
	return aggregate
}

func dispatchDefinition() runtimecontract.RuntimeDefinitionContext {
	return runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal",
		DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply", "validation/managed_session"},
	}
}

func baseDispatchFacts() runtimecontract.RuntimeFacts {
	return runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "runtime-evidence-00000000000000000000",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, TargetRefs: []string{"main.go"},
		},
		Coverage:     []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{"patch/apply"}, ValidationKinds: []string{"validation/managed_session"}},
	}
}

func stateChangeCandidate() runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "patch-1", Kind: "patch/apply", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"}, Parameters: patchApplyTestParameters("main.go"),
	}}}}}
}

func patchApplyTestOperation(id string, targetRef string) runtimecontract.RuntimeOperation {
	return runtimecontract.RuntimeOperation{
		ID: id, Kind: runtimecontract.RuntimeBlockKindPatchApply,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{targetRef}, Parameters: patchApplyTestParameters(targetRef),
	}
}

func patchApplyTestParameters(targetRef string) []runtimecontract.OperationParameter {
	patchType := "file_write"
	content := "updated\n"
	baseHash := "test-semantic-snapshot/" + targetRef
	return []runtimecontract.OperationParameter{
		{Name: "patchType", String: &patchType},
		{Name: "content", String: &content},
		{Name: "baseHash", String: &baseHash},
	}
}

func validationCandidate() runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "validate-1", Kind: "validation/managed_session", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		Parameters: runtimeManagedValidationParameters(),
	}}}}}
}

func runtimeCandidateHasManagedValidation(candidate runtimecontract.RuntimeCandidate) bool {
	if candidate.StateChange != nil {
		for _, operation := range candidate.StateChange.Operations {
			if runtimecontract.CanonicalRuntimeBlockKind(operation.Operation.Kind) == runtimecontract.RuntimeBlockKindManagedValidationSession {
				return true
			}
		}
	}
	if candidate.Validation != nil {
		for _, operation := range candidate.Validation.Operations {
			if runtimecontract.CanonicalRuntimeBlockKind(operation.Operation.Kind) == runtimecontract.RuntimeBlockKindManagedValidationSession {
				return true
			}
		}
	}
	return false
}

func TestAdjudicateEvidenceKindRejectsDurableMismatch(t *testing.T) {
	decision := runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange}
	// Owner reports none while seal expected durable.
	_, err := adjudicateEvidenceKind(decision, runtimecontract.RuntimeEffectWorkspaceDurable, true, ExecutionEvidence{
		EventID: "e1", OperationID: "op-1", Status: ExecutionEvidenceStatusAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectNone, Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "noop"},
	})
	if err == nil {
		t.Fatal("non-durable observed under state-change must fail adjudication")
	}
}

func TestSealDispatchCreatesOperationExecutionRecords(t *testing.T) {
	facts := runtimeTestFactsWithCurrentFocusBindings(t, controllerFacts(), controllerDefinition())
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("state"),
		},
	}}}}
	candidate = runtimeTestBindCandidateToFacts(t, facts, candidate)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
			runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	feasibilities := runtimeTestCurrentCandidateFeasibilities(t, RuntimeAggregate{Facts: facts}, manifest, candidate)
	dispatch, err := SealDispatchWithManifestAndOwnerAuthority(
		decision,
		candidate,
		runtimecontract.RuntimeActivationForFacts(facts),
		manifest,
		feasibilities,
		facts.Resources.OperationAdmissibilities,
		facts.Resources.PrerequisiteObservations,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(dispatch.OperationExecutions) != 1 {
		t.Fatalf("executions = %+v", dispatch.OperationExecutions)
	}
	if dispatch.OperationExecutions[0].State != runtimecontract.OperationExecutionSealed {
		t.Fatalf("state = %s", dispatch.OperationExecutions[0].State)
	}
	if dispatch.OperationExecutions[0].ExpectedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable {
		t.Fatalf("expected scope = %s", dispatch.OperationExecutions[0].ExpectedEffectScope)
	}
	if dispatch.OperationExecutions[0].IdempotencyClass != runtimecontract.RuntimeOperationIdempotencyAtMostOnce {
		t.Fatalf("sealed idempotency = %s", dispatch.OperationExecutions[0].IdempotencyClass)
	}
	if dispatch.OperationExecutions[0].ExecutionKey == "" {
		t.Fatal("execution key required")
	}
	if dispatch.OperationExecutions[0].AdmissibilitySnapshotID == "" || len(dispatch.OperationAdmissibilities) != 1 ||
		dispatch.OperationAdmissibilities[0].SnapshotID != dispatch.OperationExecutions[0].AdmissibilitySnapshotID {
		t.Fatalf("execution lost current feasible owner catalog: execution=%+v catalogs=%+v", dispatch.OperationExecutions[0], dispatch.OperationAdmissibilities)
	}
}
