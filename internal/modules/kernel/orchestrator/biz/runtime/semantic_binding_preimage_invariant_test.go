package runtime

import (
	"context"
	"errors"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeSemanticBindingPreimageSurvivesAssessmentCompletion(t *testing.T) {
	controller, repository, _ := prepareOwnerFactAssessment(t, true)
	before := *repository.aggregate
	if err := validateRuntimeSemanticBindingPreimages(before); err != nil {
		activeBatches, activeResults := len(before.SemanticPlanning.Batches), len(before.SemanticPlanning.BatchFeasibilityResults)
		historyBatches, historyResults := 0, 0
		for _, generation := range before.SemanticPlanning.History {
			historyBatches += len(generation.Snapshot.Batches)
			historyResults += len(generation.Snapshot.BatchFeasibilityResults)
		}
		t.Fatalf("pre-assessment semantic binding preimage (phase=%s active=%d/%d history=%d/%d): %v", before.SemanticPlanning.Phase, activeBatches, activeResults, historyBatches, historyResults, err)
	}
	completed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		activeBatches, activeResults := len(before.SemanticPlanning.Batches), len(before.SemanticPlanning.BatchFeasibilityResults)
		historyBatches, historyResults := 0, 0
		for _, generation := range before.SemanticPlanning.History {
			historyBatches += len(generation.Snapshot.Batches)
			historyResults += len(generation.Snapshot.BatchFeasibilityResults)
		}
		t.Fatalf("commit assessment lost semantic binding preimage (phase=%s active=%d/%d history=%d/%d): %v", before.SemanticPlanning.Phase, activeBatches, activeResults, historyBatches, historyResults, err)
	}
	if err := validateRuntimeSemanticBindingPreimages(completed); err != nil {
		t.Fatalf("completed assessment semantic binding preimage: %v", err)
	}
}

func TestRuntimeSemanticBindingPreimageRejectsCoordinatedProjectionRewrite(t *testing.T) {
	newSealed := func(t *testing.T) RuntimeAggregate {
		t.Helper()
		repository := &durableRuntimeStateRepository{aggregate: runtimeTestSemanticPreimageStateChangeAggregate(t)}
		return sealCandidateForTest(t, repository, stateChangeCandidate())
	}

	t.Run("focus CWI and reconciliation", func(t *testing.T) {
		aggregate := runtimeTestSemanticPreimageStateChangeAggregate(t)
		if err := validateRuntimeSemanticBindingPreimages(aggregate); err != nil {
			t.Fatalf("valid semantic preimage rejected: %v", err)
		}
		tampered := runtimeTestRewriteSemanticBindingProjections(t, aggregate, true, false, false, false)
		runtimeTestRequireSemanticBindingPreimageViolation(t, validateRuntimeSemanticBindingPreimages(tampered), "coordinated focus/CWI/reconciliation rewrite")
	})

	t.Run("candidate request manifest", func(t *testing.T) {
		aggregate := newSealed(t)
		if err := validateRuntimeSemanticBindingPreimages(aggregate); err != nil {
			t.Fatalf("valid semantic preimage rejected: %v", err)
		}
		tampered := runtimeTestRewriteSemanticBindingProjections(t, aggregate, false, true, false, false)
		runtimeTestRequireSemanticBindingPreimageViolation(t, validateRuntimeSemanticBindingPreimages(tampered), "candidate request projection rewrite")
	})

	t.Run("accepted attempt", func(t *testing.T) {
		aggregate := newSealed(t)
		tampered := runtimeTestRewriteSemanticBindingProjections(t, aggregate, false, false, true, false)
		runtimeTestRequireSemanticBindingPreimageViolation(t, validateRuntimeSemanticBindingPreimages(tampered), "accepted-attempt projection rewrite")
	})

	t.Run("active dispatch", func(t *testing.T) {
		aggregate := newSealed(t)
		tampered := runtimeTestRewriteSemanticBindingProjections(t, aggregate, false, false, false, true)
		runtimeTestRequireSemanticBindingPreimageViolation(t, validateRuntimeSemanticBindingPreimages(tampered), "active-dispatch projection rewrite")
	})

	t.Run("all downstream copies and self hashes", func(t *testing.T) {
		aggregate := newSealed(t)
		tampered := runtimeTestRewriteSemanticBindingProjections(t, aggregate, true, true, true, true)
		runtimeTestRequireSemanticBindingPreimageViolation(t, validateRuntimeSemanticBindingPreimages(tampered), "coordinated downstream rewrite")
	})

	t.Run("accepted batch generation request", func(t *testing.T) {
		aggregate := runtimeTestSemanticPreimageStateChangeAggregate(t)
		if len(aggregate.SemanticPlanning.Batches) != 1 || len(aggregate.SemanticPlanning.BatchFeasibilityResults) != 1 {
			t.Fatalf("semantic generation rewrite fixture is incomplete: %+v", aggregate.SemanticPlanning)
		}
		tampered := aggregate
		tampered.SemanticPlanning = aggregate.SemanticPlanning.Clone()
		batch := tampered.SemanticPlanning.Batches[0].Clone()
		batch.RequestLogicalID += "-coordinated-rewrite"
		batch.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(batch)
		tampered.SemanticPlanning.Batches[0] = batch
		result := tampered.SemanticPlanning.BatchFeasibilityResults[0].Normalized()
		result.BatchHash = batch.Hash
		result.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
		tampered.SemanticPlanning.BatchFeasibilityResults[0] = result
		runtimeTestRequireSemanticBindingPreimageViolation(t, validateRuntimeSemanticBindingPreimages(tampered), "coordinated accepted-batch generation rewrite")
	})
}

func TestRuntimeSemanticBindingPreimageRequiresArchivedHistoryOwnerFacts(t *testing.T) {
	_, _, aggregate, _, _, _ := sealedSemanticPlanningAggregate(t)
	if err := validateRuntimeSemanticBindingPreimages(aggregate); err != nil {
		t.Fatalf("valid current semantic preimage rejected: %v", err)
	}
	// Once a batch is no longer the current working set, only the exact typed
	// batch result may replay its owner facts. Focus/lineage projections alone
	// must not reconstruct that earlier authority.
	aggregate.SemanticPlanning.Document = nil
	aggregate.SemanticPlanning.DocumentHash = ""
	aggregate.SemanticPlanning.Feasibilities = nil
	aggregate.SemanticPlanning.FeasibilitySetHash = ""
	runtimeTestRequireSemanticBindingPreimageViolation(t, validateRuntimeSemanticBindingPreimages(aggregate), "archived owner-fact removal")
}

func runtimeTestRequireSemanticBindingPreimageViolation(t *testing.T, err error, mutation string) {
	t.Helper()
	if err == nil || !errors.Is(err, errRuntimeSemanticBindingPreimageViolation) {
		t.Fatalf("%s was not rejected by the semantic provider/owner preimage invariant: %v", mutation, err)
	}
}

func runtimeTestSemanticPreimageStateChangeAggregate(t *testing.T) RuntimeAggregate {
	t.Helper()
	facts := baseDispatchFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	aggregate := RuntimeAggregate{StateRevision: 1, Definition: dispatchDefinition(), Facts: facts}
	runtimeTestEnsureCurrentFocusBindings(t, &aggregate)
	return aggregate
}

func runtimeTestRewriteSemanticBindingProjections(
	t *testing.T,
	aggregate RuntimeAggregate,
	rewriteFacts bool,
	rewriteSession bool,
	rewriteAttempt bool,
	rewriteDispatch bool,
) RuntimeAggregate {
	t.Helper()
	source := aggregate.Facts.Focus.Normalized()
	if len(source.CoverageEvidenceBindings) != 1 {
		t.Fatalf("semantic rewrite fixture has %d focus bindings, want one", len(source.CoverageEvidenceBindings))
	}
	binding := source.CoverageEvidenceBindings[0].Clone()
	binding.DeclarationRef += "/coordinated-rewrite"
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)

	if rewriteFacts {
		facts := aggregate.Facts.Normalized()
		focus := facts.Focus.Normalized()
		focus.CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{binding}
		facts.Focus = focus.Normalized()
		ref := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, aggregate.SemanticPlanning.Manifest.Hash)
		for index := range facts.Coverage {
			item := &facts.Coverage[index]
			if item.ID != binding.CoverageWorkItemID {
				continue
			}
			item.CurrentBindingRef = &ref
			for lineageIndex := range item.BindingLineage {
				if item.BindingLineage[lineageIndex].BindingID == binding.ID {
					item.BindingLineage[lineageIndex] = ref
				}
			}
		}
		reconciliation, err := runtimecontract.CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, []runtimecontract.CoverageFocus{facts.Focus})
		if err != nil {
			t.Fatalf("compile coordinated reconciliation rewrite: %v", err)
		}
		facts.ReconciliationContract = reconciliation
		aggregate.Facts = facts
		aggregate.SemanticPlanning.SealedBindingSetHash = reconciliation.BindingSetHash
		if count := len(aggregate.SemanticPlanning.SealedLineageRefs); count != 0 {
			aggregate.SemanticPlanning.SealedLineageRefs[count-1] = reconciliation.BindingSetHash
		}
	}

	if rewriteSession {
		if len(aggregate.CandidateRequestSessions) == 0 || aggregate.CandidateRequestSessions[len(aggregate.CandidateRequestSessions)-1].CandidateManifest == nil {
			t.Fatal("semantic rewrite fixture has no candidate request manifest")
		}
		index := len(aggregate.CandidateRequestSessions) - 1
		session := aggregate.CandidateRequestSessions[index].Clone()
		manifest := session.CandidateManifest.Clone()
		manifest.Bindings = []runtimecontract.CoverageEvidenceBinding{binding}
		manifest.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)
		session.CandidateManifest = &manifest
		session.ManifestHash = manifest.Hash
		aggregate.CandidateRequestSessions = append([]runtimecontract.RuntimeCandidateRequestSession(nil), aggregate.CandidateRequestSessions...)
		aggregate.CandidateRequestSessions[index] = session
	}

	if rewriteAttempt {
		if len(aggregate.CandidateAttempts) == 0 {
			t.Fatal("semantic rewrite fixture has no accepted attempt")
		}
		index := len(aggregate.CandidateAttempts) - 1
		attempt := aggregate.CandidateAttempts[index].Clone()
		attempt.AdmissionCoverageBindings = []runtimecontract.CoverageEvidenceBinding{binding}
		attempt.AdmissionCoverageBindingHash = runtimecontract.StableRuntimeCoverageEvidenceBindingSnapshotHash(attempt.AdmissionCoverageBindings)
		aggregate.CandidateAttempts = append([]runtimecontract.RuntimeCandidateAttemptRecord(nil), aggregate.CandidateAttempts...)
		aggregate.CandidateAttempts[index] = attempt
	}

	if rewriteDispatch {
		if aggregate.ActiveDispatch == nil {
			t.Fatal("semantic rewrite fixture has no active dispatch")
		}
		dispatch := *aggregate.ActiveDispatch
		dispatch.CoverageBindings = []runtimecontract.CoverageEvidenceBinding{binding}
		dispatch.CoverageBindingHash = runtimecontract.StableRuntimeCoverageEvidenceBindingSnapshotHash(dispatch.CoverageBindings)
		dispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(sealedDispatchToRuntimeRecord(dispatch))
		aggregate.ActiveDispatch = &dispatch
	}
	return aggregate
}
