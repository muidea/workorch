package runtime

import (
	"context"
	"fmt"
	"strings"
	"testing"

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
	if sealed.ActiveDispatch == nil || sealed.ActiveDispatch.Transaction != repository.aggregate.Decision.AuthorityTransaction() || sealed.ActiveDispatch.CandidateHash != runtimecontract.StableRuntimeCandidateHash(candidate) {
		t.Fatalf("sealed dispatch = %+v", sealed.ActiveDispatch)
	}
	stepInput := sealed.ActiveDispatch.StepInput()
	if stepInput.DispatchID == "" || stepInput.Transaction != sealed.ActiveDispatch.Transaction || stepInput.CandidateHash != sealed.ActiveDispatch.CandidateHash {
		t.Fatalf("dispatch step input did not persist complete transaction: %+v", stepInput)
	}
	evidence := ExecutionEvidence{
		EventID: "event-state-change-1", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted, SourceRef: "patch/1",
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Disposition != CommitDispositionCommitted || committed.Aggregate.EvidenceSequence != 1 || committed.Aggregate.Facts.EvidenceRevision != "evidence-v1-00000000000000000001" {
		t.Fatalf("commit result = %+v", committed)
	}
	if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation || committed.Aggregate.ActiveDispatch != nil || len(committed.Aggregate.CompletedDispatches) != 1 {
		t.Fatalf("aggregate did not advance atomically: %+v", committed.Aggregate)
	}
	stateRevision := committed.Aggregate.StateRevision
	duplicate, err := coordinator.CommitEvidence(context.Background(), evidence)
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
		testDispatchCoverageBinding("binding/a", "cwi-a", facts.Focus.ID, facts.DefinitionRevision, runtimecontract.EvidenceKindValidation, runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeBlockKindManagedValidationSession, "recipe-a", "check-a", "main.go"),
		testDispatchCoverageBinding("binding/b", "cwi-b", facts.Focus.ID, facts.DefinitionRevision, runtimecontract.EvidenceKindValidation, runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeBlockKindManagedValidationSession, "recipe-b", "check-b", "main.go"),
	}
	operation := runtimecontract.RuntimeOperation{ID: "validate-a", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, BindingRefs: []string{"binding/a"}, TargetRefs: []string{"main.go"}}
	receipt := &runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "exec/ws-1/run-1/dispatch/validate-a", Fence: 1, OperationID: "validate-a", Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectNone, TargetRefs: []string{"main.go"},
		Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &runtimecontract.RuntimeValidationObservation{RecipeID: "recipe-a", ExecutedChecks: []string{"check-a"}, PassedChecks: []string{"check-a"}}},
	}
	refs, credit, diagnostic := coverageEvidenceCreditForReceipt(facts, operation, runtimecontract.EvidenceKindValidation, receipt)
	if len(refs) != 1 || refs[0] != "binding/a" || len(credit) != 1 || credit[0] != "cwi-a" || diagnostic != "" {
		t.Fatalf("exact validation proof credit = refs=%v credit=%v diagnostic=%q", refs, credit, diagnostic)
	}

	// A real, accepted receipt that reports a check owned by another binding is
	// retained by the caller but cannot close either CWI through this operation.
	receipt.Evidence.Validation = &runtimecontract.RuntimeValidationObservation{RecipeID: "recipe-b", ExecutedChecks: []string{"check-b"}, PassedChecks: []string{"check-b"}}
	_, credit, diagnostic = coverageEvidenceCreditForReceipt(facts, operation, runtimecontract.EvidenceKindValidation, receipt)
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
	_, credit, diagnostic = coverageEvidenceCreditForReceipt(stateFacts, stateOperation, runtimecontract.EvidenceKindStateChange, stateReceipt)
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
	_, credit, diagnostic = coverageEvidenceCreditForReceipt(stateFacts, stateOperation, runtimecontract.EvidenceKindStateChange, stateReceipt)
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
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "patch-1",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "owner receipt evidence", Quality: common.EvidenceQualityValid},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	// This represents a malformed internal-event producer.  The receipt is
	// already persisted, so the attempted replacement must neither enter the
	// evidence ledger nor influence SM-2 quality/coverage.
	evidence.Evidence = runtimecontract.RuntimeOperationEvidence{Summary: "forged event evidence", Quality: common.EvidenceQualityInconclusive, ZeroWork: true}
	committed, err := coordinator.CommitEvidence(context.Background(), evidence)
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
	result, err := coordinator.CommitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-stale-1", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-old",
		AuthoritySnapshotID: "authority-old", OperationID: "patch-old", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != CommitDispositionStale || len(result.Aggregate.Diagnostics) != 1 || result.Aggregate.Decision.Focus.ID != currentFocus || result.Aggregate.Facts.EvidenceRevision != currentRevision || len(result.Aggregate.Facts.Evidence.Accepted) != 0 {
		t.Fatalf("stale evidence changed authority facts: %+v", result.Aggregate)
	}
	again, err := coordinator.CommitEvidence(context.Background(), ExecutionEvidence{
		EventID: "event-stale-1", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-old",
		AuthoritySnapshotID: "authority-old", OperationID: "patch-old", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	})
	if err != nil || again.Disposition != CommitDispositionDuplicate || len(again.Aggregate.Diagnostics) != 1 {
		t.Fatalf("duplicate stale evidence = %+v err=%v", again, err)
	}
}

func TestReopenCoverageDependenciesForAcceptedEvidencePreservesLineage(t *testing.T) {
	coverage := []runtimecontract.RuntimeCoverageItemFact{
		{ID: "cwi-source", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"state-before"}, ReopenDependencies: []runtimecontract.RuntimeCoverageReopenDependency{{CoverageWorkItemID: "cwi-validation", TriggerEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}}},
		{ID: "cwi-validation", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"validation-before"}, LastAssessmentID: "assessment-before"},
		{ID: "cwi-other", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"other-before"}},
	}
	evidence := runtimecontract.EvidenceFact{ID: "state-after", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}}
	updated, reopened := reopenCoverageDependenciesForEvidence(coverage, evidence)
	if len(reopened) != 1 || reopened[0] != "cwi-validation" {
		t.Fatalf("reopened coverage = %v, want cwi-validation", reopened)
	}
	byID := map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItemFact{}
	for _, item := range updated {
		byID[item.ID] = item
	}
	dependent := byID["cwi-validation"]
	if dependent.State != runtimecontract.RuntimeCoverageItemStateOpen || dependent.LastAssessmentID != "" || len(dependent.EvidenceRefs) != 1 || dependent.EvidenceRefs[0] != "validation-before" || len(dependent.ReopenedByEvidenceRefs) != 1 || dependent.ReopenedByEvidenceRefs[0] != "state-after" {
		t.Fatalf("dependent after reopen = %+v", dependent)
	}
	if byID["cwi-other"].State != runtimecontract.RuntimeCoverageItemStateSatisfied || byID["cwi-source"].State != runtimecontract.RuntimeCoverageItemStateSatisfied {
		t.Fatalf("unrelated/source coverage changed: %+v", byID)
	}
	updated, reopened = reopenCoverageDependenciesForEvidence(updated, runtimecontract.EvidenceFact{ID: "validation-after", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}})
	byID = map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItemFact{}
	for _, item := range updated {
		byID[item.ID] = item
	}
	if len(reopened) != 0 || byID["cwi-validation"].State != runtimecontract.RuntimeCoverageItemStateOpen {
		t.Fatalf("non-trigger evidence reopened coverage: reopened=%v coverage=%+v", reopened, updated)
	}
}

func TestRequeueReopenedCoverageFocusesRestoresCompletedFocusAheadOfPendingQueue(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Focus:            runtimecontract.CoverageFocus{ID: "focus-source", SliceID: "slice-source", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}},
		RemainingFocuses: []runtimecontract.CoverageFocus{{ID: "focus-pending", SliceID: "slice-pending", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-pending"}}},
		CompletedFocuses: []runtimecontract.CoverageFocus{
			{ID: "focus-validation", SliceID: "slice-validation", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-validation"}},
			{ID: "focus-unrelated", SliceID: "slice-unrelated", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-unrelated"}},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{
			{ID: "cwi-source", State: runtimecontract.RuntimeCoverageItemStateOpen}, {ID: "cwi-pending", State: runtimecontract.RuntimeCoverageItemStateOpen},
			{ID: "cwi-validation", State: runtimecontract.RuntimeCoverageItemStateOpen}, {ID: "cwi-unrelated", State: runtimecontract.RuntimeCoverageItemStateSatisfied},
		},
	}
	updated := requeueReopenedCoverageFocuses(facts, []common.CoverageWorkItemID{"cwi-validation"})
	if len(updated.RemainingFocuses) != 2 || updated.RemainingFocuses[0].ID != "focus-validation" || updated.RemainingFocuses[1].ID != "focus-pending" || len(updated.CompletedFocuses) != 1 || updated.CompletedFocuses[0].ID != "focus-unrelated" {
		t.Fatalf("requeued focus queues = current=%+v remaining=%+v completed=%+v", updated.Focus, updated.RemainingFocuses, updated.CompletedFocuses)
	}
	// Normalization models the persisted/reloaded aggregate shape; the queue
	// must retain the same reactivation ordering across restart.
	reloaded := updated.Normalized()
	if len(reloaded.RemainingFocuses) != 2 || reloaded.RemainingFocuses[0].ID != "focus-validation" || len(reloaded.CompletedFocuses) != 1 || reloaded.CompletedFocuses[0].ID != "focus-unrelated" {
		t.Fatalf("reloaded focus queues = %+v", reloaded)
	}
}

func TestDispatchCommitAtomicallyReopensAndRequeuesCompletedDependency(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{
		{ID: "cwi-1", SliceID: "slice-source", State: runtimecontract.RuntimeCoverageItemStateOpen, ReopenDependencies: []runtimecontract.RuntimeCoverageReopenDependency{{CoverageWorkItemID: "cwi-validation", TriggerEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}}},
		{ID: "cwi-validation", SliceID: "slice-validation", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"validation-before"}, LastAssessmentID: "assessment-before"},
	}
	aggregate.Facts.Focus.SliceID = "slice-source"
	aggregate.Facts.CompletedFocuses = []runtimecontract.CoverageFocus{{ID: "focus-validation", SliceID: "slice-validation", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-validation"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}}}
	decision, err := ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	runtimeTestEnsureCurrentFocusBindings(t, &aggregate)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimeTestBindCandidateToCurrentFocus(t, aggregate, stateChangeCandidate())
	sealed := sealCandidateForTest(t, repository, candidate)
	evidence := ExecutionEvidence{EventID: "event-source-after-validation", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted, SourceRef: "patch/1"}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Aggregate.StateRevision <= sealed.StateRevision || len(committed.Aggregate.Facts.RemainingFocuses) != 1 || committed.Aggregate.Facts.RemainingFocuses[0].ID != "focus-validation" || len(committed.Aggregate.Facts.CompletedFocuses) != 0 {
		t.Fatalf("dependency reopen was not atomically requeued: %+v", committed.Aggregate.Facts)
	}
	for _, item := range committed.Aggregate.Facts.Coverage {
		if item.ID == "cwi-validation" && (item.State != runtimecontract.RuntimeCoverageItemStateOpen || item.LastAssessmentID != "" || len(item.ReopenedByEvidenceRefs) != 1 || item.ReopenedByEvidenceRefs[0] != evidence.EventID) {
			t.Fatalf("reopened dependent coverage = %+v", item)
		}
	}
	// The repository load is the restart boundary used by the coordinator. The
	// persisted queue and reopen lineage must survive it unchanged.
	reloaded, err := repository.Load(context.Background(), "ws-1", "run-1")
	if err != nil || len(reloaded.Facts.RemainingFocuses) != 1 || reloaded.Facts.RemainingFocuses[0].ID != "focus-validation" {
		t.Fatalf("reloaded dependency queue = %+v err=%v", reloaded.Facts, err)
	}
	duplicate, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != committed.Aggregate.StateRevision {
		t.Fatalf("duplicate dependency evidence changed aggregate: %+v err=%v", duplicate, err)
	}
}

func TestRejectedValidationEvidenceAtomicallyReturnsToStateChange(t *testing.T) {
	aggregate := validationAggregate(t)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	commitEvidence := ExecutionEvidence{
		EventID: "event-validation-failed", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "validate-1", Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Signature: "build-failed", Message: "main redeclared", TargetRefs: []string{"main.go"}},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &commitEvidence)
	result, err := coordinator.CommitEvidence(context.Background(), commitEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("validation failure decision = %+v, want atomic state change", result.Aggregate.Decision)
	}
	if result.Aggregate.Facts.Resources.FailureTarget == nil || result.Aggregate.Facts.Resources.FailureTarget.Signature != "build-failed" ||
		len(result.Aggregate.Facts.Resources.FailureTarget.TargetRefs) != 1 || result.Aggregate.Facts.Resources.FailureTarget.TargetRefs[0] != "main.go" {
		t.Fatalf("validation failure target was not persisted from owner receipt: %+v", result.Aggregate.Facts.Resources.FailureTarget)
	}
	if result.Aggregate.Facts.RepairDirective == nil || len(result.Aggregate.Facts.RepairDirective.TargetRefs) != 1 || result.Aggregate.Facts.RepairDirective.TargetRefs[0] != "main.go" {
		t.Fatalf("repair directive is not grounded to owner failure target: %+v", result.Aggregate.Facts.RepairDirective)
	}
}

func TestRejectedValidationEntryDoesNotBootstrapUngroundedFailureTarget(t *testing.T) {
	aggregate := validationAggregate(t)
	aggregate.Facts.Focus.TargetRefs = nil
	aggregate.Facts.Focus.TargetRole = "deliverable"
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	candidate := validationCandidate()
	candidate.Validation.Operations[0].Operation.TargetRefs = []string{"validation-entry.go"}
	sealed := sealCandidateForTest(t, repository, candidate)
	evidence := ExecutionEvidence{
		EventID: "event-validation-entry-failed", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "validate-1", Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Signature: "validation-entry-failed", Message: "check failed", TargetRefs: []string{"validation-entry.go"}},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Aggregate.Facts.Failure == nil || len(committed.Aggregate.Facts.Failure.TargetRefs) != 0 || committed.Aggregate.Facts.Resources.FailureTarget != nil {
		t.Fatalf("validation entry was promoted into failure-target authority: facts=%+v", committed.Aggregate.Facts)
	}
	if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("ungrounded validation failure requested a candidate: decision=%+v", committed.Aggregate.Decision)
	}
}

func TestRejectedValidationScopeMemberDoesNotBecomeFailureTarget(t *testing.T) {
	aggregate := validationAggregate(t)
	aggregate.Facts.Focus.TargetRefs = nil
	aggregate.Facts.Focus.TargetRole = "deliverable"
	aggregate.Facts.Focus.RequireTargetGrounding = true
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs:  []string{"implementation.go", "support.go"}, CandidateTargetRefs: []string{"implementation.go", "support.go"},
		CandidateCount: 2, ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	aggregate.Facts.Resources.TargetSelection = &selection
	bindCurrentFocusTargetSelection(&aggregate.Facts)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	candidate := validationCandidate()
	candidate.Validation.Operations[0].Operation.TargetRefs = []string{"implementation.go"}
	sealed := sealCandidateForTest(t, repository, candidate)
	evidence := ExecutionEvidence{
		EventID: "event-validation-scope-member-failed", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "validate-1", Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Signature: "validation-scope-member-failed", Message: "check failed", TargetRefs: []string{"implementation.go"}},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Aggregate.Facts.Failure == nil || len(committed.Aggregate.Facts.Failure.TargetRefs) != 0 || committed.Aggregate.Facts.Resources.FailureTarget != nil {
		t.Fatalf("File scope member became failure-target authority: facts=%+v", committed.Aggregate.Facts)
	}
	if committed.Aggregate.Facts.RepairDirective != nil && len(committed.Aggregate.Facts.RepairDirective.TargetRefs) != 0 {
		t.Fatalf("File scope member became a concrete repair directive target: %+v", committed.Aggregate.Facts.RepairDirective)
	}
	if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation || len(committed.Aggregate.Decision.RequiredOutput.TargetRefs) != 0 {
		t.Fatalf("scope-backed validation failure did not remain an unbound validation gap: decision=%+v", committed.Aggregate.Decision)
	}
	if committed.Aggregate.Facts.Resources.TargetSelection == nil || committed.Aggregate.Facts.Resources.TargetSelection.Disposition != runtimecontract.RuntimeTargetSelectionScope {
		t.Fatalf("scope feasibility inventory was not retained: %+v", committed.Aggregate.Facts.Resources.TargetSelection)
	}
	if len(committed.Aggregate.Facts.Prerequisites.Pending) != 0 || len(committed.Aggregate.Facts.Resources.TargetSnapshots) != 0 {
		t.Fatalf("scope was promoted into concrete target snapshots: prerequisites=%+v snapshots=%+v", committed.Aggregate.Facts.Prerequisites.Pending, committed.Aggregate.Facts.Resources.TargetSnapshots)
	}
}

func TestRejectedValidationOwnerObservationBindsCompatibleTarget(t *testing.T) {
	aggregate := validationAggregate(t)
	aggregate.Facts.Focus.TargetRefs = nil
	aggregate.Facts.Focus.TargetRole = "implementation"
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	addCurrentMCPFailureObservationFreshness(t, &aggregate, "mcp-server-v1")
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	candidate := validationCandidate()
	candidate.Validation.Operations[0].Operation.TargetRefs = []string{"validation-entry.go"}
	sealed := sealCandidateForTest(t, repository, candidate)
	evidence := ExecutionEvidence{
		EventID: "event-observed-target-failed", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "validate-1", Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassBuildFailure, Signature: "mcp-observed-failure", Message: "capability observed failure",
			TargetRefs: []string{"implementation.go"},
			TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
				OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation",
				ObservedRevision: "mcp-server-v1", TargetRefs: []string{"implementation.go"},
			},
		},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	failureTarget := committed.Aggregate.Facts.Resources.FailureTarget
	if committed.Aggregate.Facts.Failure == nil || failureTarget == nil || failureTarget.TargetObservation == nil ||
		len(failureTarget.TargetRefs) != 1 || failureTarget.TargetRefs[0] != "implementation.go" ||
		failureTarget.TargetObservation.SubjectRef != "server-a" || failureTarget.TargetObservation.ObservedRevision != "mcp-server-v1" {
		t.Fatalf("owner observation was not persisted as repair authority: facts=%+v", committed.Aggregate.Facts)
	}
	if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("owner observation bypassed snapshot prerequisite: decision=%+v", committed.Aggregate.Decision)
	}
	var snapshotPrerequisite runtimecontract.RuntimePrerequisite
	for _, prerequisite := range committed.Aggregate.Facts.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleFile && prerequisite.SubjectRef == "implementation.go" {
			snapshotPrerequisite = prerequisite
			break
		}
	}
	if snapshotPrerequisite.ID == "" {
		t.Fatalf("owner observation did not request a current file snapshot: %+v", committed.Aggregate.Facts.Prerequisites.Pending)
	}
	revision := committed.Aggregate.StateRevision
	duplicate, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != revision ||
		duplicate.Aggregate.Facts.EvidenceRevision != committed.Aggregate.Facts.EvidenceRevision ||
		len(duplicate.Aggregate.Facts.Evidence.Rejected) != len(committed.Aggregate.Facts.Evidence.Rejected) {
		t.Fatalf("duplicate observed failure changed authority: result=%+v err=%v", duplicate, err)
	}
	// A restarted controller must reuse the committed owner observation and
	// sealed dispatch lineage. Completing the fresh snapshot is a new SM-2
	// prerequisite transition, never a replay of the rejected MCP operation.
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	afterSnapshot, err := restarted.CommitPrerequisite(context.Background(), "ws-1", "run-1", PrerequisiteResult{
		PrerequisiteID: snapshotPrerequisite.ID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: "implementation.go", ContentHash: "implementation-v1", ContentState: runtimecontract.SnapshotContentStateComplete},
	})
	if err != nil || afterSnapshot.ActiveDispatch != nil || len(afterSnapshot.CompletedDispatches) != 1 ||
		afterSnapshot.CompletedDispatches[0].ID != sealed.ActiveDispatch.ID || afterSnapshot.Facts.Resources.FailureTarget == nil ||
		len(afterSnapshot.Facts.Resources.TargetSnapshots) != 1 || afterSnapshot.Facts.Resources.TargetSnapshots[0].TargetRef != "implementation.go" {
		t.Fatalf("restart prerequisite continuation replayed or lost owner binding: aggregate=%+v err=%v", afterSnapshot, err)
	}
}

func TestOwnerFailureObservationRetargetsExplicitFocusAfterSnapshotRestart(t *testing.T) {
	aggregate := validationAggregate(t)
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetRole = "implementation"
	addCurrentMCPFailureObservationFreshness(t, &aggregate, "mcp-retarget-v1")
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	oldAuthority := sealed.Decision.AuthoritySnapshot.ID
	evidence := ExecutionEvidence{
		EventID: "event-owner-retarget", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "validate-1",
		Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassBuildFailure, Signature: "observed-implementation-failure", Message: "owner observed implementation repair target",
			TargetRefs: []string{"implementation.go"},
			TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
				OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-retarget", TargetRole: "implementation",
				ObservedRevision: "mcp-retarget-v1", TargetRefs: []string{"implementation.go"},
			},
		},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil || committed.Aggregate.Facts.Resources.FailureTarget == nil ||
		!sameRuntimeTargetRefs(GroundedRepairTargets(committed.Aggregate.Facts), []string{"implementation.go"}) {
		t.Fatalf("owner retarget was not committed: aggregate=%+v err=%v", committed.Aggregate, err)
	}
	var snapshot runtimecontract.RuntimePrerequisite
	for _, prerequisite := range committed.Aggregate.Facts.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.SubjectRef == "implementation.go" {
			snapshot = prerequisite
		}
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.SubjectRef == "main.go" {
			t.Fatalf("retarget incorrectly reacquired superseded focus target: %+v", committed.Aggregate.Facts.Prerequisites.Pending)
		}
	}
	if snapshot.ID == "" || committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("retarget did not wait for the new grounded snapshot: decision=%+v pending=%+v", committed.Aggregate.Decision, committed.Aggregate.Facts.Prerequisites.Pending)
	}

	// The persisted aggregate is the restart boundary.  Completing the new
	// owner snapshot must issue a new authority for implementation.go, not
	// revive the old Focus target or rejected validation dispatch.
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := restarted.CommitPrerequisite(context.Background(), "ws-1", "run-1", PrerequisiteResult{
		PrerequisiteID: snapshot.ID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: "implementation.go", ContentHash: "implementation-v1", ContentState: runtimecontract.SnapshotContentStateComplete},
	})
	if err != nil || resumed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange ||
		!sameRuntimeTargetRefs(resumed.Decision.RequiredOutput.TargetRefs, []string{"implementation.go"}) ||
		resumed.Decision.AuthoritySnapshot.ID == oldAuthority || resumed.Decision.AuthoritySnapshot.Attempt != 0 {
		t.Fatalf("retarget restart did not issue a fresh implementation authority: aggregate=%+v err=%v", resumed, err)
	}
	oldCandidate := stateChangeCandidate()
	if admission := EvaluateCandidateAdmission(resumed, oldCandidate); admission.Accepted() {
		t.Fatalf("superseded Focus target remained admissible after retarget: %+v", admission)
	}
	newCandidate := stateChangeCandidate()
	newCandidate.StateChange.Operations[0].Operation = patchApplyTestOperation("patch-implementation", "implementation.go")
	if admission := EvaluateCandidateAdmission(resumed, newCandidate); !admission.Accepted() {
		t.Fatalf("retargeted owner target was not admissible: %+v", admission)
	}
}

func addCurrentMCPFailureObservationFreshness(t *testing.T, aggregate *RuntimeAggregate, revision string) {
	t.Helper()
	if aggregate == nil {
		t.Fatal("runtime aggregate is required")
	}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:mcp", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, SelectorRefs: []string{"server-a"}, ObservedRevision: revision,
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, RequiredCapabilities: []string{runtimecontract.StepKindMCPToolCall},
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp_server_catalog", Revision: revision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	aggregate.Facts.Resources.OperationAdmissibilities = append(aggregate.Facts.Resources.OperationAdmissibilities, inventory)
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, observation)
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, prerequisite)
}

func TestValidationFailureTargetObservationRequiresCurrentOwnerFreshness(t *testing.T) {
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
	if !validationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("failure target observation with current owner freshness dependency was not grounded")
	}
}

func TestValidationFailureTargetObservationAcceptsOnlyStructuredShellProtocol(t *testing.T) {
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
	if !validationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("content-bound Shell observation with current owner freshness was not grounded")
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
	aggregate := validationAggregate(t)
	aggregate.Facts.Focus.TargetRefs = nil
	aggregate.Facts.Focus.TargetRole = "implementation"
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	candidate := validationCandidate()
	candidate.Validation.Operations[0].Operation.TargetRefs = []string{"validation-entry.go"}
	sealed := sealCandidateForTest(t, repository, candidate)
	evidence := ExecutionEvidence{
		EventID: "event-observed-target-role-mismatch", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "validate-1", Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassBuildFailure, Signature: "mcp-observed-role-mismatch", Message: "capability observed failure",
			TargetRefs: []string{"implementation.go"},
			TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
				OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "validation-entry",
				ObservedRevision: "mcp-server-v1", TargetRefs: []string{"implementation.go"},
			},
		},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.CommitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Aggregate.Facts.Failure == nil || len(committed.Aggregate.Facts.Failure.TargetRefs) != 0 ||
		committed.Aggregate.Facts.Resources.FailureTarget != nil || committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("mismatched owner role became repair authority: facts=%+v decision=%+v", committed.Aggregate.Facts, committed.Aggregate.Decision)
	}
}

func TestRejectedTextReplaceConsumesRepairBudgetAndRequiresFreshTargetSnapshot(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.ConvergencePolicy = runtimecontract.RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 3}
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
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "text_replace_not_found",
			Signature: "patch-text-replace-not-found", Message: "oldText not found", TargetRefs: []string{"main.go"},
		},
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	rejected, err := coordinator.CommitEvidence(context.Background(), evidence)
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
	duplicate, err := coordinator.CommitEvidence(context.Background(), evidence)
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
	refreshed, err := controller.CommitPrerequisite(context.Background(), "ws-1", "run-1", PrerequisiteResult{
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
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &commitEvidence)
	result, err := second.CommitEvidence(context.Background(), commitEvidence)
	if err != nil || result.Disposition != CommitDispositionCommitted {
		t.Fatalf("restart commit = %+v err=%v", result, err)
	}
}

func TestDispatchCoordinatorCommitsMultiOperationDispatchOnlyAfterAllOperations(t *testing.T) {
	repository := &durableRuntimeStateRepository{aggregate: stateChangeAggregate(t)}
	coordinator, _ := NewDispatchCoordinator(repository)
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		{Operation: patchApplyTestOperation("write-1", "main.go")},
		{Operation: func() runtimecontract.RuntimeOperation {
			operation := patchApplyTestOperation("write-2", "other.go")
			// The exact main.go binding must never be reused by unrelated work.
			operation.BindingRefs = nil
			operation.DependsOn = []string{"write-1"}
			return operation
		}()},
	}}}
	sealed := sealCandidateForTest(t, repository, candidate)
	firstEvidence := ExecutionEvidence{
		EventID: "event-write-1", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-1",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &firstEvidence)
	first, err := coordinator.CommitEvidence(context.Background(), firstEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if first.DispatchCompleted || first.Aggregate.ActiveDispatch == nil || len(first.Aggregate.ActiveDispatch.CompletedOperationIDs) != 1 {
		t.Fatalf("first operation prematurely completed dispatch: %+v", first)
	}
	if first.Aggregate.Decision.AuthoritySnapshot.ID != sealed.Decision.AuthoritySnapshot.ID {
		t.Fatalf("decision changed before dispatch completion")
	}
	secondEvidence := ExecutionEvidence{
		EventID: "event-write-2", WorkspaceID: "ws-1", RunID: "run-1", DispatchID: sealed.ActiveDispatch.ID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, OperationID: "write-2",
		Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &secondEvidence)
	second, err := coordinator.CommitEvidence(context.Background(), secondEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if !second.DispatchCompleted || second.Aggregate.ActiveDispatch != nil || len(second.Aggregate.CompletedDispatches) != 1 {
		t.Fatalf("second operation did not complete dispatch: %+v", second)
	}
}

func TestDispatchCommitAdvancesToNextCoverageFocusDeterministically(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	aggregate.Facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
		ID: "focus-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRefs: []string{"other.go"},
	}}
	aggregate.Facts.Coverage = runtimecontract.InitialRuntimeCoverageFacts([]runtimecontract.RuntimeCoverageItem{{ID: "cwi-1"}, {ID: "cwi-2"}})
	runtimeTestEnsureCurrentFocusBindings(t, &aggregate)
	decision, err := ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	runtimeTestEnsureCurrentFocusBindings(t, &aggregate)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, _ := NewDispatchCoordinator(repository)
	candidate := runtimeTestBindCandidateToCurrentFocus(t, aggregate, stateChangeCandidate())
	sealed := sealCandidateForTest(t, repository, candidate)
	commitEvidence := ExecutionEvidence{
		EventID: "event-focus-1", WorkspaceID: "ws-1", RunID: "run-1",
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: "patch-1", Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &commitEvidence)
	result, err := coordinator.CommitEvidence(context.Background(), commitEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.Facts.Focus.ID != "focus-2" || len(result.Aggregate.Facts.RemainingFocuses) != 0 ||
		result.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || result.Aggregate.Decision.Focus.ID != "focus-2" {
		t.Fatalf("next focus aggregate = %+v", result.Aggregate)
	}
	if len(result.Aggregate.Facts.Evidence.Accepted) != 1 || len(result.Aggregate.Facts.Evidence.Accepted[0].CoverageWorkItemIDs) != 1 ||
		result.Aggregate.Facts.Evidence.Accepted[0].CoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatalf("focus-bound evidence = %+v", result.Aggregate.Facts.Evidence.Accepted)
	}
}

func stateChangeAggregate(t *testing.T) RuntimeAggregate {
	t.Helper()
	facts := baseDispatchFacts()
	aggregate := RuntimeAggregate{StateRevision: 1, Definition: dispatchDefinition(), Facts: facts}
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
	var dispatch SealedDispatch
	if runtimeCandidateHasManagedValidation(candidate) {
		recipes, snapshotErr := runtimecontract.RuntimeCandidateManagedValidationRecipeSnapshot(manifest, candidate)
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		dispatch, err = sealDispatchWithActivationAndFeasibilities(aggregate.Decision, candidate, aggregate.Facts.Activation, recipes, nil)
	} else {
		dispatch, err = SealDispatchWithActivation(aggregate.Decision, candidate, aggregate.Facts.Activation)
	}
	if err != nil {
		t.Fatal(err)
	}
	transaction := aggregate.Decision.AuthorityTransaction()
	stablePrefixHash := "test-sealed-dispatch"
	logicalRequestID := runtimecontract.StableRuntimeCandidateRequestSessionID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction, stablePrefixHash, manifest.Hash, "test-sealed-dispatch")
	aggregate.CandidateRequestSessions = append(aggregate.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{
		LogicalRequestID: logicalRequestID, Transaction: transaction, StablePrefixHash: stablePrefixHash, ManifestHash: manifest.Hash,
		CorrectionHash: "test-sealed-dispatch", State: runtimecontract.RuntimeCandidateRequestSessionDocumentComplete, NextSequence: 1,
		LastPhysicalCallID: runtimecontract.StableRuntimeCandidatePhysicalCallID(logicalRequestID, 1), ProviderSubmitted: true,
		LastOutcome: runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete},
	})
	aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
		AuthorityContractID: runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction),
		Transaction:         transaction,
		Attempt:             0,
		StablePrefixHash:    stablePrefixHash,
		ManifestHash:        manifest.Hash,
		LogicalRequestID:    logicalRequestID,
		State:               runtimecontract.RuntimeCandidateAttemptStateAccepted,
	})
	aggregate.ActiveDispatch = &dispatch
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	repository.aggregate = aggregate
	return aggregate
}

func validationAggregate(t *testing.T) RuntimeAggregate {
	t.Helper()
	facts := runtimeFactsWithManagedValidationRecipe(baseDispatchFacts())
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{ID: "accepted-state-change", Kind: runtimecontract.EvidenceKindStateChange, Revision: facts.EvidenceRevision}}
	aggregate := RuntimeAggregate{StateRevision: 1, Definition: dispatchDefinition(), Facts: facts}
	runtimeTestEnsureCurrentFocusBindings(t, &aggregate)
	// This fixture starts after an already-accepted durable change. Model that
	// fact in the current C12 shape instead of relying on the retired
	// last-focus empty-CWI shortcut: a binding reference is what lets the
	// reducer classify the next required proof as validation.
	if len(aggregate.Facts.Focus.CoverageEvidenceBindings) != 1 {
		t.Fatalf("validation fixture requires one current-focus binding: %+v", aggregate.Facts.Focus)
	}
	aggregate.Facts.Evidence.Accepted[0].CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...)
	aggregate.Facts.Evidence.Accepted[0].BindingRefs = []string{aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID}
	decision, err := ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatalf("reduce binding-shaped validation fixture: %v", err)
	}
	aggregate.Decision = decision
	return aggregate
}

func dispatchDefinition() runtimecontract.RuntimeDefinitionContext {
	return runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal",
		DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"patch/apply", "validation/managed_session"},
	}
}

func baseDispatchFacts() runtimecontract.RuntimeFacts {
	return runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-v1-00000000000000000000",
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
		ID: "patch-1", Kind: "patch/apply", BindingRefs: []string{"binding/test/runtime/focus-1/cwi-1"}, TargetRefs: []string{"main.go"}, Parameters: patchApplyTestParameters("main.go"),
	}}}}}
}

func patchApplyTestOperation(id string, targetRef string) runtimecontract.RuntimeOperation {
	return runtimecontract.RuntimeOperation{
		ID: id, Kind: runtimecontract.RuntimeBlockKindPatchApply,
		BindingRefs: []string{"binding/test/runtime/focus-1/cwi-1"}, TargetRefs: []string{targetRef}, Parameters: patchApplyTestParameters(targetRef),
	}
}

func patchApplyTestParameters(_ string) []runtimecontract.OperationParameter {
	patchType := "file_write"
	content := "updated\n"
	return []runtimecontract.OperationParameter{
		{Name: "patchType", String: &patchType},
		{Name: "content", String: &content},
	}
}

func validationCandidate() runtimecontract.RuntimeCandidate {
	return runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "validate-1", Kind: "validation/managed_session", BindingRefs: []string{"binding/test/runtime/focus-1/cwi-1"}, TargetRefs: []string{"main.go"},
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
			ID: "write-1", Kind: "file/write", BindingRefs: []string{"binding/test/runtime/focus-v2/cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("state"),
		},
	}}}}
	dispatch, err := SealDispatchWithActivation(decision, candidate, facts.Activation)
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
	if dispatch.OperationExecutions[0].ExecutionKey == "" {
		t.Fatal("execution key required")
	}
}
