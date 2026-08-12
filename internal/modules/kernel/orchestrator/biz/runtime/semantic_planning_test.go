package runtime

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func semanticCoverageOrderExpectedDocument(aggregate RuntimeAggregate, manifestHash, stablePrefixHash string) runtimecontract.RuntimeExpectedDocumentContract {
	return runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:  runtimecontract.RuntimeProviderDocumentSemanticPlan,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		ManifestHash:  manifestHash, DefinitionSeedHash: aggregate.Facts.DefinitionSeed.Hash,
		StablePrefixHash: stablePrefixHash, MaxOutputTokens: 4096, MaxRecoveryAttempts: 1,
	}
}

type semanticResponseLossRepository struct {
	RuntimeStateRepository
	marker string
	match  func(RuntimeAggregate) bool
	fired  bool
}

type semanticFailOnceRepository struct {
	RuntimeStateRepository
	err    error
	failed bool
}

func (r *semanticFailOnceRepository) Save(ctx context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	if !r.failed {
		r.failed = true
		return r.err
	}
	return r.RuntimeStateRepository.Save(ctx, aggregate, expectedRevision)
}

func (r *semanticResponseLossRepository) Save(ctx context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	matched := !r.fired && r.match != nil && r.match(aggregate)
	if err := r.RuntimeStateRepository.Save(ctx, aggregate, expectedRevision); err != nil {
		return err
	}
	if matched {
		r.fired = true
		panic(r.marker)
	}
	return nil
}

func TestRuntimeSemanticPlanningPrivateTransitionSequenceIsPersistedAndIdempotent(t *testing.T) {
	controller, repository, aggregate := newSemanticPlanningController(t)
	manifest, document, feasibility := semanticPlanningFixture(t, aggregate)
	proposal := document.Proposals[0]
	order := document.Clone()
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID = ""
	order.BatchSequence = 0
	order.BatchCoverageWorkItemIDs = nil
	order.CoverageOrder = []common.CoverageWorkItemID{proposal.CoverageWorkItemID}
	order.Proposals = nil
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}

	apply := func(event RuntimeInternalEvent) RuntimeAggregate {
		t.Helper()
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
		return aggregate
	}

	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractRequired})
	if repository.saves != 1 { // initialize only; duplicate required is read-only.
		t.Fatalf("required duplicate saves = %d", repository.saves)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"})
	requestRevision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"})
	if aggregate.StateRevision != requestRevision {
		t.Fatal("duplicate request changed aggregate revision")
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: "semantic-request-1"})
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, order, 0, "proof-request-1", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	proof := document.Clone()
	proof.PlanStep = runtimecontract.RuntimeSemanticPlanStepProofBatch
	proof.BatchID = batch.BatchID
	proof.BatchSequence = batch.Sequence
	proof.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	batchID := aggregate.SemanticPlanning.FeasibilityBatchID
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: batchID, SemanticFeasibility: &feasibility})
	observationRevision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: batchID, SemanticFeasibility: &feasibility})
	if aggregate.StateRevision != observationRevision {
		t.Fatal("duplicate owner observation changed aggregate revision")
	}
	savesBeforeSeal := repository.saves
	acceptedBatch := aggregate.SemanticPlanning.Batches[0]
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, acceptedBatch, proof)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, SemanticPlanMergedDocument: &merged})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	if repository.saves != savesBeforeSeal+2 {
		t.Fatalf("semantic merge+seal saves = %d, want two CAS saves after %d", repository.saves, savesBeforeSeal)
	}
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed {
		t.Fatalf("semantic phase = %q", aggregate.SemanticPlanning.Phase)
	}
	if aggregate.Facts.ReconciliationContract.BindingSetHash == "" || aggregate.SemanticPlanning.SealedBindingSetHash != aggregate.Facts.ReconciliationContract.BindingSetHash || len(aggregate.Facts.Focus.CoverageEvidenceBindings) != 1 {
		t.Fatalf("semantic authority was not sealed atomically: ledger=%+v facts=%+v", aggregate.SemanticPlanning, aggregate.Facts)
	}
	sealedRevision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	if aggregate.StateRevision != sealedRevision || repository.saves != savesBeforeSeal+2 {
		t.Fatalf("duplicate semantic seal changed authority: revision=%d saves=%d", aggregate.StateRevision, repository.saves)
	}
	reloaded, err := controller.Load(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SemanticPlanning.FeasibilitySetHash != aggregate.SemanticPlanning.FeasibilitySetHash || reloaded.SemanticPlanning.SealedBindingSetHash != aggregate.Facts.ReconciliationContract.BindingSetHash {
		t.Fatalf("semantic lineage changed across reload: %+v", reloaded.SemanticPlanning)
	}
}

func TestRuntimeSemanticBatchDocumentReplayIsIdempotent(t *testing.T) {
	controller, repository, aggregate := newSemanticPlanningController(t)
	manifest, order, feasibility := semanticPlanningFixture(t, aggregate)
	proposal := order.Proposals[0]
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID = ""
	order.BatchSequence = 0
	order.BatchCoverageWorkItemIDs = nil
	order.CoverageOrder = []common.CoverageWorkItemID{manifest.CoverageItems[0].ID}
	order.Proposals = nil
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply := func(event RuntimeInternalEvent) {
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "batch-prefix", LogicalRequestID: "order-request"})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: "order-request"})
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, order, 0, "batch-request", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := controller.compileSemanticPlanBatch(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = result
	proof := order.Clone()
	proof.PlanStep = runtimecontract.RuntimeSemanticPlanStepProofBatch
	proof.BatchID = batch.BatchID
	proof.BatchSequence = batch.Sequence
	proof.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
	proof.CoverageOrder = nil
	proof.Proposals = []runtimecontract.RuntimeSemanticProofProposal{proposal}
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID})
	revision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID})
	if aggregate.StateRevision != revision || repository.saves == 0 {
		t.Fatalf("replayed batch document changed durable state: revision=%d saves=%d", aggregate.StateRevision, repository.saves)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility})
	accepted := aggregate.SemanticPlanning.Batches[0]
	batchResult := runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{
		BatchID: accepted.BatchID, BatchHash: accepted.Hash,
		DocumentHash:  accepted.DocumentHash,
		Feasibilities: []runtimecontract.RuntimeSemanticProofFeasibility{feasibility},
	}
	batchResult.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(batchResult)
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchFeasibilityObserved, SemanticPlanBatchFeasibilityResult: &batchResult})
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, accepted, proof)
	if err != nil {
		t.Fatal(err)
	}
	responseLoss := &semanticResponseLossRepository{
		RuntimeStateRepository: repository, marker: "semantic-merged-document-response-loss",
		match: func(candidate RuntimeAggregate) bool { return candidate.SemanticPlanning.MergedDocumentHash != "" },
	}
	controller.repository = responseLoss
	controller.transitions.repository = responseLoss
	func() {
		defer func() {
			if recovered := recover(); recovered != "semantic-merged-document-response-loss" {
				t.Fatalf("unexpected merged document response-loss marker: %v", recovered)
			}
		}()
		_, _ = controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
			Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			SemanticPlanMergedDocument: &merged,
		})
		t.Fatal("expected merged document response loss after durable CAS")
	}()
	aggregate = *repository.aggregate
	restartedController, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	controller = restartedController
	mergeRevision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, SemanticPlanMergedDocument: &merged})
	if aggregate.StateRevision != mergeRevision {
		t.Fatal("replayed merged semantic document changed durable state")
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed || len(aggregate.Facts.ReconciliationContract.Bindings) != 1 {
		t.Fatalf("batch replay recovery did not reach atomic semantic seal: %+v", aggregate.SemanticPlanning)
	}
}

func TestRuntimeSemanticBatchCorrectionRejectsUnclassifiedDiagnostic(t *testing.T) {
	controller, _, aggregate := newSemanticPlanningController(t)
	manifest, order, _ := semanticPlanningFixture(t, aggregate)
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID = ""
	order.BatchSequence = 0
	order.BatchCoverageWorkItemIDs = nil
	order.CoverageOrder = []common.CoverageWorkItemID{manifest.CoverageItems[0].ID}
	order.Proposals = nil
	apply := func(event RuntimeInternalEvent) {
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "batch-correction-prefix", LogicalRequestID: "order-request"})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: "order-request"})
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, order, 0, "batch-request", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: "semantic_plan", OriginalCode: "invalid_json", OriginalMessage: "batch document was malformed",
		Signature: "semantic-plan/batch/invalid-json",
	}
	directive := semanticPlanningCorrectionDirective(manifest, aggregate.SemanticPlanning.StablePrefixHash, runtimecontract.RuntimeSemanticPlanStepProofBatch, diagnostic)
	expected := runtimecontract.RuntimeExpectedDocumentContract{DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan, PlanStep: directive.PlanStep, ManifestHash: directive.ManifestHash, DefinitionSeedHash: directive.DefinitionSeedHash, CorrectionAttempt: 1}
	if _, err := controller.requireSemanticPlanCorrection(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.SemanticPlanning.StablePrefixHash, "correction-request", expected, diagnostic); err == nil {
		t.Fatal("unclassified batch diagnostic created correction authority")
	}
}

func TestRuntimeSemanticBatchCorrectionRejectsCoverageOutsideActiveBatch(t *testing.T) {
	// The out-of-batch CWI must be part of the immutable Definition seed and
	// canonical ledger. Appending it to a provider manifest would only test the
	// manifest-identity fence, not proof-batch scope.
	controller, _, aggregate, manifest, order, _, _ := newTwoItemSemanticBatchFixture(t)
	apply := func(event RuntimeInternalEvent) {
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "batch-scope-prefix", LogicalRequestID: "order-request"})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: "order-request"})
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, order, 0, "batch-request", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: "semantic_plan", OriginalCode: "invalid_document", OriginalMessage: "batch document was invalid",
		Signature: "semantic-plan/batch/invalid-document", FieldViolations: []runtimecontract.RuntimeFieldViolation{{Path: "$.proposals", Actual: "invalid"}},
		SemanticViolations: []runtimecontract.RuntimeSemanticContractViolation{{
			Kind: runtimecontract.RuntimeSemanticContractViolationProofBatchDocument, Path: "$.proposals",
			RequiredCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2-second"},
		}},
	}
	directive := semanticPlanningCorrectionDirective(manifest, aggregate.SemanticPlanning.StablePrefixHash, runtimecontract.RuntimeSemanticPlanStepProofBatch, diagnostic)
	expected := runtimecontract.RuntimeExpectedDocumentContract{DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan, PlanStep: directive.PlanStep, ManifestHash: directive.ManifestHash, DefinitionSeedHash: directive.DefinitionSeedHash, CorrectionAttempt: 1}
	if _, err := controller.requireSemanticPlanCorrection(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.SemanticPlanning.StablePrefixHash, "correction-request", expected, diagnostic); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("out-of-batch correction error = %v", err)
	}
}

func TestRuntimeSemanticPlanningBindsProposalCorrectionToActiveBatch(t *testing.T) {
	ledger := runtimecontract.RuntimeSemanticPlanningLedger{
		ActiveBatchID: "batch-1",
		Batches: []runtimecontract.RuntimeSemanticPlanBatch{{
			BatchID: "batch-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		}},
	}
	directive := runtimecontract.RuntimeSemanticCorrectionDirective{
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		Violations: []runtimecontract.RuntimeSemanticContractViolation{{
			Kind: runtimecontract.RuntimeSemanticContractViolationProposalConstraint, Path: "$.proposals[0].requiredEffectScopes",
			CoverageWorkItemID: "cwi-001", ProposalIndex: 0, Rule: runtimecontract.RuntimeSemanticConstraintNonEmpty,
			Actual: "empty_array", Required: []string{"at least one effect scope"},
		}},
	}
	if err := validateSemanticPlanCorrectionScope(ledger, directive); err != nil {
		t.Fatal(err)
	}
	directive.Violations[0].CoverageWorkItemID = "cwi-outside"
	if err := validateSemanticPlanCorrectionScope(ledger, directive); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("out-of-batch proposal correction error = %v", err)
	}
}

func TestRuntimeSemanticPlanningRejectsInvalidStaleAndThirdAttemptTransitions(t *testing.T) {
	controller, _, aggregate := newSemanticPlanningController(t)
	manifest, document, _ := semanticPlanningFixture(t, aggregate)
	ctx := context.Background()
	if _, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanEvaluated, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticDocument: &document, LogicalRequestID: "missing-request",
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("document without request error = %v", err)
	}
	manifest.DefinitionSeedHash = "stale-seed"
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	if _, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanRequestStarted, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1",
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale manifest error = %v", err)
	}
	manifest, document, _ = semanticPlanningFixture(t, aggregate)

	// A correction must be tied to an already-issued proof batch. The previous
	// fixture manufactured a prerequisite CWI in the manifest, which is now
	// correctly rejected because it does not exist in the Definition seed.
	apply := func(event RuntimeInternalEvent) RuntimeAggregate {
		t.Helper()
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(ctx, event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
		return aggregate
	}
	apply(RuntimeInternalEvent{
		Kind:             RuntimeInternalEventSemanticPlanRequestStarted,
		SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1",
	})
	order := document.Clone()
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID = ""
	order.BatchSequence = 0
	order.BatchCoverageWorkItemIDs = nil
	order.CoverageOrder = []common.CoverageWorkItemID{manifest.CoverageItems[0].ID}
	order.Proposals = nil
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: "semantic-request-1"})
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, order, 0, "proof-request-1", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	rejection := semanticPlanningDiagnostic()
	rejection.FieldViolations = []runtimecontract.RuntimeFieldViolation{{Path: "$.proposals[0].requiredEffectScopes", Actual: "empty_array", Required: []string{"workspace_durable"}}}
	rejection.SemanticViolations = []runtimecontract.RuntimeSemanticContractViolation{{
		Kind: runtimecontract.RuntimeSemanticContractViolationProposalConstraint,
		Path: "$.proposals[0].requiredEffectScopes", CoverageWorkItemID: manifest.CoverageItems[0].ID,
		ProposalIndex: 0, Rule: runtimecontract.RuntimeSemanticConstraintNonEmpty,
		Actual: "empty_array", Required: []string{"workspace_durable"},
	}}
	directive := semanticPlanningCorrectionDirective(manifest, "semantic-prefix-1", runtimecontract.RuntimeSemanticPlanStepProofBatch, rejection)
	expectedCorrection := runtimecontract.RuntimeExpectedDocumentContract{DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan, PlanStep: directive.PlanStep, ManifestHash: directive.ManifestHash, DefinitionSeedHash: directive.DefinitionSeedHash, CorrectionAttempt: 1}
	beforeCorrection := aggregate.StateRevision
	corrected, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanCorrectionRequired, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-2", ExpectedDocument: &expectedCorrection, SemanticDiagnostic: &rejection,
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate = corrected.Aggregate
	if corrected.Aggregate.SemanticPlanning.Attempt != 1 || len(corrected.Aggregate.SemanticPlanning.Rejections) != 1 || corrected.Aggregate.StateRevision != beforeCorrection+1 {
		t.Fatalf("correction lineage = %+v", corrected.Aggregate.SemanticPlanning)
	}
	replayed, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanCorrectionRequired, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-2", ExpectedDocument: &expectedCorrection, SemanticDiagnostic: &rejection,
	})
	if err != nil || replayed.Aggregate.StateRevision != corrected.Aggregate.StateRevision {
		t.Fatalf("duplicate correction changed state: revision=%d err=%v", replayed.Aggregate.StateRevision, err)
	}
	recovered, err := controller.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || recovered.SemanticPlanning.CorrectionDirective == nil || recovered.SemanticPlanning.CorrectionDirective.Hash != directive.Hash {
		t.Fatalf("persisted correction did not survive reload: %+v err=%v", recovered.SemanticPlanning.CorrectionDirective, err)
	}
	if _, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanCorrectionRequired, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-3", ExpectedDocument: &expectedCorrection, SemanticDiagnostic: &rejection,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("third semantic request error = %v", err)
	}
	correctedProof := document.Clone()
	correctedProof.BatchID = batch.BatchID
	correctedProof.BatchSequence = batch.Sequence
	correctedProof.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
	accepted, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanEvaluated, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		LogicalRequestID: "semantic-request-2", SemanticDocument: &correctedProof,
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted ||
		accepted.Aggregate.SemanticPlanning.Attempt != 1 || accepted.Aggregate.SemanticPlanning.CorrectionDirective == nil ||
		len(accepted.Aggregate.SemanticPlanning.Rejections) != 1 {
		t.Fatalf("corrected proof document lost bounded correction lineage: %+v", accepted.Aggregate.SemanticPlanning)
	}
}

func TestRuntimeSemanticPlanningFailurePreservesOwnerDiagnosticFacts(t *testing.T) {
	controller, _, aggregate := newSemanticPlanningController(t)
	manifest, document, feasibility := semanticPlanningFixture(t, aggregate)
	proposal := document.Proposals[0]
	order := document.Clone()
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID = ""
	order.BatchSequence = 0
	order.BatchCoverageWorkItemIDs = nil
	order.CoverageOrder = []common.CoverageWorkItemID{proposal.CoverageWorkItemID}
	order.Proposals = nil
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	apply := func(event RuntimeInternalEvent) RuntimeAggregate {
		t.Helper()
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatal(err)
		}
		aggregate = result.Aggregate
		return aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: "semantic-request-1"})
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, order, 0, "proof-request-1", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	proof := document.Clone()
	proof.PlanStep = runtimecontract.RuntimeSemanticPlanStepProofBatch
	proof.BatchID = batch.BatchID
	proof.BatchSequence = batch.Sequence
	proof.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	ownerDiagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OriginalClass: "feasibility", OriginalCode: "target_outside_scope", OriginalMessage: "  target is outside owner scope\n", Signature: "file/target-outside-scope",
	}
	feasibility.Status = runtimecontract.RuntimeSemanticFeasibilityInfeasible
	feasibility.OriginalDiagnostic = &ownerDiagnostic
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility})
	failure := semanticPlanningDiagnostic()
	failure.OriginalCode = "validation_recipe_exhausted"
	failure.OriginalMessage = "validation recipe cannot execute in workspace"
	failure.Signature = "shell/validation-recipe-exhausted"
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractFailed, SemanticDiagnostic: &failure})
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed || len(aggregate.SemanticPlanning.Feasibilities) != 1 || aggregate.SemanticPlanning.Feasibilities[0].OriginalDiagnostic == nil || aggregate.SemanticPlanning.Feasibilities[0].OriginalDiagnostic.OriginalMessage != ownerDiagnostic.OriginalMessage {
		t.Fatalf("failed semantic ledger lost owner facts: %+v", aggregate.SemanticPlanning)
	}
	if aggregate.Facts.Failure == nil || aggregate.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible {
		t.Fatalf("semantic owner failure was misclassified: %+v", aggregate.Facts.Failure)
	}
	if aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || aggregate.Decision.Terminal == nil || aggregate.Decision.Terminal.Code != "semantic_contract_failed" {
		t.Fatalf("semantic failure did not seal terminal decision: %+v", aggregate.Decision)
	}
}

func TestRuntimeConvergenceProgressSignatureTracksSemanticPlanningLineage(t *testing.T) {
	controller, _, aggregate := newSemanticPlanningController(t)
	manifest, _, _ := semanticPlanningFixture(t, aggregate)
	before := RuntimeConvergenceProgressSignature(aggregate)
	started, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanRequestStarted, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if before == RuntimeConvergenceProgressSignature(started.Aggregate) {
		t.Fatal("semantic request lineage did not change convergence signature")
	}
}

func TestRuntimeSemanticStaleFactPrecedesBoundedCatalogRefreshAndSurvivesReload(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	catalog, prerequisite, observation := semanticPlanningCatalogFixture()
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	aggregate, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, document, feasibility := semanticPlanningFixture(t, aggregate)
	proposal := document.Proposals[0]
	order := document.Clone()
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID = ""
	order.BatchSequence = 0
	order.BatchCoverageWorkItemIDs = nil
	order.CoverageOrder = []common.CoverageWorkItemID{proposal.CoverageWorkItemID}
	order.Proposals = nil
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	feasibility.CatalogSnapshotID = catalog.SnapshotID
	feasibility.CatalogObservationID = observation.ObservationID
	feasibility.Status = runtimecontract.RuntimeSemanticFeasibilityStale
	feasibility.ObservedRevision = "file-revision-2"
	feasibility.OriginalDiagnostic = &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, OriginalClass: "semantic_feasibility",
		OriginalCode: "semantic_catalog_stale", OriginalMessage: "owner catalog advanced while probing",
		Signature: "semantic-feasibility/file/stale", ExpectedRevision: "file-revision-1", ObservedRevision: "file-revision-2",
	}

	apply := func(event RuntimeInternalEvent) RuntimeAggregate {
		t.Helper()
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, applyErr := controller.ApplyInternalEvent(ctx, event)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", event.Kind, applyErr)
		}
		aggregate = result.Aggregate
		return aggregate
	}
	expected := semanticCoverageOrderExpectedDocument(aggregate, manifest.Hash, "semantic-prefix-1")
	withSession, session, err := controller.ensureCandidateRequestSessionWithExpected(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentSemanticPlan, "semantic-prefix-1", manifest.Hash, "", &expected)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = withSession
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: session.LogicalRequestID})
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
	aggregate, err = controller.markCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, 1, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.completeCandidateRequestDocument(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: session.LogicalRequestID})
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, order, 0, "proof-request-1", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	proof := document.Clone()
	proof.PlanStep = runtimecontract.RuntimeSemanticPlanStepProofBatch
	proof.BatchID = batch.BatchID
	proof.BatchSequence = batch.Sequence
	proof.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
	proof.CoverageOrder = nil
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	batchID := aggregate.SemanticPlanning.FeasibilityBatchID
	beforeObservationRevision := aggregate.StateRevision
	conflictRepository := &semanticFailOnceRepository{RuntimeStateRepository: repository, err: common.ErrConflict}
	conflictController, err := NewRuntimeController(conflictRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conflictController.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticFeasibilityObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticBatchID: batchID, SemanticFeasibility: &feasibility,
	}); err == nil {
		t.Fatal("semantic feasibility unexpectedly committed through injected CAS conflict")
	}
	if repository.aggregate.StateRevision != beforeObservationRevision || len(repository.aggregate.SemanticPlanning.Feasibilities) != 0 {
		t.Fatalf("semantic feasibility CAS conflict left partial facts: %+v", repository.aggregate.SemanticPlanning)
	}
	observationLoss := &semanticResponseLossRepository{
		RuntimeStateRepository: repository, marker: "semantic-feasibility-response-loss",
		match: func(candidate RuntimeAggregate) bool {
			return candidate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningFeasibilityRequired && len(candidate.SemanticPlanning.Feasibilities) == 1
		},
	}
	observationController, err := NewRuntimeController(observationLoss)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "semantic-feasibility-response-loss" {
				t.Fatalf("unexpected semantic feasibility response-loss marker: %v", recovered)
			}
		}()
		_, _ = observationController.ApplyInternalEvent(ctx, RuntimeInternalEvent{
			Kind: RuntimeInternalEventSemanticFeasibilityObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			SemanticBatchID: batchID, SemanticFeasibility: &feasibility,
		})
		t.Fatal("expected semantic feasibility response loss after durable CAS")
	}()
	if !observationLoss.fired {
		t.Fatal("semantic feasibility response-loss CAS point was not reached")
	}
	aggregate = *repository.aggregate
	observationRevision := aggregate.StateRevision
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFeasibilityRequired ||
		len(aggregate.SemanticPlanning.Feasibilities) != 1 || len(aggregate.Facts.Resources.OperationAdmissibilities) != 1 ||
		len(aggregate.PrerequisiteInvalidations) != 0 {
		t.Fatalf("stale factual CAS was not isolated from catalog invalidation: %+v", aggregate)
	}

	savesAfterObservation := repository.saves
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: batchID, SemanticFeasibility: &feasibility})
	if aggregate.StateRevision != observationRevision || repository.saves != savesAfterObservation {
		t.Fatal("response-loss semantic feasibility replay was not idempotent")
	}
	refreshLoss := &semanticResponseLossRepository{
		RuntimeStateRepository: repository, marker: "semantic-catalog-refresh-response-loss",
		match: func(candidate RuntimeAggregate) bool {
			return candidate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningRequired && len(candidate.SemanticPlanning.FeasibilityGaps) == 1
		},
	}
	refreshController, err := NewRuntimeController(refreshLoss)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "semantic-catalog-refresh-response-loss" {
				t.Fatalf("unexpected semantic refresh response-loss marker: %v", recovered)
			}
		}()
		_, _ = refreshController.ApplyInternalEvent(ctx, RuntimeInternalEvent{
			Kind: RuntimeInternalEventSemanticContractRequired, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			SemanticBatchID: batchID, SemanticRefreshGeneration: 0,
		})
		t.Fatal("expected semantic catalog refresh response loss after durable CAS")
	}()
	if !refreshLoss.fired {
		t.Fatal("semantic catalog refresh response-loss CAS point was not reached")
	}
	aggregate = *repository.aggregate
	if aggregate.StateRevision != observationRevision+1 || aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired ||
		aggregate.SemanticPlanning.CatalogRefreshAttempts != 1 || len(aggregate.SemanticPlanning.FeasibilityGaps) != 1 ||
		len(aggregate.Facts.Resources.OperationAdmissibilities) != 0 || len(aggregate.Facts.Resources.PrerequisiteObservations) != 0 ||
		len(aggregate.Facts.Prerequisites.Pending) != 1 || aggregate.Facts.Recovery.PrerequisiteRefreshes != 1 ||
		len(aggregate.CandidateAttempts) != 0 || aggregate.ActiveDispatch != nil {
		t.Fatalf("semantic stale did not atomically retire only catalog authority: %+v", aggregate)
	}
	gap := aggregate.SemanticPlanning.FeasibilityGaps[0]
	if gap.Feasibilities[0].OriginalDiagnostic == nil || gap.Feasibilities[0].OriginalDiagnostic.OriginalMessage != "owner catalog advanced while probing" ||
		len(aggregate.PrerequisiteInvalidations) != 1 || aggregate.PrerequisiteInvalidations[0].Stale == nil {
		t.Fatalf("semantic stale lineage was not preserved: gap=%+v invalidations=%+v", gap, aggregate.PrerequisiteInvalidations)
	}
	saves := repository.saves
	revision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractRequired, SemanticBatchID: batchID, SemanticRefreshGeneration: 0})
	if aggregate.StateRevision != revision || repository.saves != saves {
		t.Fatalf("duplicate semantic stale refresh changed state: revision=%d saves=%d", aggregate.StateRevision, repository.saves)
	}
	reloaded, err := controller.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SemanticPlanning.CatalogRefreshAttempts != 1 || len(reloaded.SemanticPlanning.FeasibilityGaps) != 1 ||
		reloaded.SemanticPlanning.FeasibilityGaps[0].Feasibilities[0].OriginalDiagnostic.OriginalMessage != "owner catalog advanced while probing" {
		t.Fatalf("semantic catalog refresh lineage changed after reload: %+v", reloaded.SemanticPlanning)
	}
	newCatalog := catalog
	newCatalog.ObservedRevision = "file-revision-2"
	newCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(newCatalog)
	newObservation := observation
	newObservation.ContentHash = newCatalog.SnapshotID
	newObservation.Dependencies[0].Revision = newCatalog.ObservedRevision
	newObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(newObservation)
	aggregate, err = controller.commitPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &newCatalog, Observation: &newObservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	newManifest := manifest.Clone()
	newManifest.OwnerCatalogs = []runtimecontract.RuntimeOperationAdmissibility{newCatalog}
	newManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(newManifest)
	if newManifest.Hash == manifest.Hash {
		t.Fatal("new catalog revision did not change semantic manifest identity")
	}
	newExpected := semanticCoverageOrderExpectedDocument(aggregate, newManifest.Hash, "semantic-prefix-1")
	withNewSession, newSession, err := controller.ensureCandidateRequestSessionWithExpected(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentSemanticPlan, "semantic-prefix-1", newManifest.Hash, "", &newExpected)
	if err != nil {
		t.Fatal(err)
	}
	oldSessionTerminal := false
	for _, persisted := range withNewSession.CandidateRequestSessions {
		if persisted.LogicalRequestID == session.LogicalRequestID && persisted.State == runtimecontract.RuntimeCandidateRequestSessionTerminal &&
			persisted.LastOutcome.Code == "semantic_catalog_invalidated" {
			oldSessionTerminal = true
		}
	}
	if newSession.LogicalRequestID == session.LogicalRequestID || len(withNewSession.CandidateRequestSessions) != 2 || !oldSessionTerminal {
		t.Fatalf("fresh catalog reused or rewrote old semantic request session: old=%+v new=%+v", session, newSession)
	}
}

func TestRuntimeSemanticPrerequisiteRequiredFailsClosedWithoutRetiringCatalog(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	catalog, prerequisite, observation := semanticPlanningCatalogFixture()
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	aggregate, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, document, feasibility := semanticPlanningFixture(t, aggregate)
	proposal := document.Proposals[0]
	order := document.Clone()
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID = ""
	order.BatchSequence = 0
	order.BatchCoverageWorkItemIDs = nil
	order.CoverageOrder = []common.CoverageWorkItemID{proposal.CoverageWorkItemID}
	order.Proposals = nil
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply := func(event RuntimeInternalEvent) {
		t.Helper()
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, applyErr := controller.ApplyInternalEvent(ctx, event)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", event.Kind, applyErr)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: "semantic-request-1"})
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, order, 0, "proof-request-1", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	proof := document.Clone()
	proof.PlanStep = runtimecontract.RuntimeSemanticPlanStepProofBatch
	proof.BatchID = batch.BatchID
	proof.BatchSequence = batch.Sequence
	proof.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
	proof.CoverageOrder = nil
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	feasibility.CatalogSnapshotID = catalog.SnapshotID
	feasibility.CatalogObservationID = observation.ObservationID
	feasibility.Status = runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired
	feasibility.ObservedRevision = catalog.ObservedRevision
	feasibility.OriginalDiagnostic = &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OriginalClass: "semantic_feasibility", OriginalCode: "workspace_manifest_unavailable",
		OriginalMessage: "workspace manifest could not be observed", Signature: "semantic-feasibility/file/manifest-unavailable",
		ExpectedRevision: catalog.ObservedRevision,
	}
	batchID := aggregate.SemanticPlanning.FeasibilityBatchID
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: batchID, SemanticFeasibility: &feasibility})
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFeasibilityRequired ||
		len(aggregate.SemanticPlanning.Feasibilities) != 1 ||
		aggregate.SemanticPlanning.Feasibilities[0].Status != runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired {
		t.Fatalf("prerequisite-required owner observation was not persisted: %+v", aggregate.SemanticPlanning)
	}
	if _, refreshErr := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticContractRequired, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticBatchID: batchID, SemanticRefreshGeneration: 0,
	}); refreshErr == nil {
		t.Fatal("prerequisite-required semantic fact unexpectedly entered catalog refresh")
	}
	if len(aggregate.PrerequisiteInvalidations) != 0 || len(aggregate.Facts.Resources.OperationAdmissibilities) != 1 ||
		len(aggregate.Facts.Resources.PrerequisiteObservations) != 1 || aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 ||
		len(aggregate.Facts.Prerequisites.Pending) != 0 {
		t.Fatalf("prerequisite-required fact retired or budgeted catalog state: facts=%+v invalidations=%+v", aggregate.Facts, aggregate.PrerequisiteInvalidations)
	}
}

func TestRuntimeSemanticCatalogReacquisitionRetiresOnlyCurrentGeneration(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate, catalog, prerequisite, observation := sealedSemanticPlanningAggregate(t)
	oldLedger := aggregate.SemanticPlanning.Clone()

	changed, err := applyReceiptResourceChangeInvalidation(&aggregate, "dispatch-retired", "operation-retired", runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "execution/semantic-retired", Fence: 1, OperationID: "operation-retired",
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", BeforeRevision: catalog.ObservedRevision, AfterRevision: "file-revision-2",
		}},
	})
	if err != nil || !changed || !semanticCatalogPrerequisitePending(aggregate, prerequisite.ID) {
		t.Fatalf("receipt invalidation did not reopen current semantic catalog acquisition: changed=%t err=%v aggregate=%+v", changed, err, aggregate)
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	repository.aggregate = &aggregate

	newCatalog := catalog.Normalized()
	newCatalog.ObservedRevision = "file-revision-2"
	newCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(newCatalog)
	newObservation := observation
	newObservation.ContentHash = newCatalog.SnapshotID
	newObservation.Dependencies[0].Revision = newCatalog.ObservedRevision
	newObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(newObservation)
	updated, err := controller.commitPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &newCatalog, Observation: &newObservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || len(updated.SemanticPlanning.History) != 1 ||
		updated.SemanticPlanning.History[0].Reason != runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated ||
		updated.SemanticPlanning.History[0].Snapshot.Manifest == nil || updated.SemanticPlanning.History[0].Snapshot.Manifest.Hash != oldLedger.Manifest.Hash {
		t.Fatalf("same-focus catalog refresh did not preserve and retire only the old generation: %+v", updated.SemanticPlanning)
	}
	operationPending := false
	for _, pending := range updated.Facts.Prerequisites.Pending {
		if pending.ID == prerequisite.ID {
			operationPending = true
		}
	}
	if len(updated.Facts.Resources.OperationAdmissibilities) != 1 || updated.Facts.Resources.OperationAdmissibilities[0].SnapshotID != newCatalog.SnapshotID || operationPending {
		t.Fatalf("reacquired catalog was not retained as the only current owner fact: %+v", updated.Facts)
	}
	if semanticCatalogPrerequisitePending(updated, prerequisite.ID) {
		t.Fatal("retired generation still treats an already reacquired catalog as pending")
	}

	// Historical invalidations keep their audit record. They must not by
	// themselves retire a later generation that has no matching pending
	// acquisition.
	updated.SemanticPlanning = oldLedger
	if semanticCatalogPrerequisitePending(updated, prerequisite.ID) {
		t.Fatal("historical invalidation was treated as a current catalog refresh")
	}
}

func TestRuntimeSemanticCatalogReacquisitionResealsCoverageProofLineageBeforeCredit(t *testing.T) {
	for _, testCase := range []struct {
		name                string
		currentTarget       string
		wantDisposition     runtimecontract.RuntimeCoverageProofLineageDisposition
		wantCurrentCredit   bool
		wantMissingEvidence bool
	}{
		{
			name:              "equivalent proof",
			currentTarget:     "target.txt",
			wantDisposition:   runtimecontract.RuntimeCoverageProofLineageDispositionEquivalent,
			wantCurrentCredit: true,
		},
		{
			name:                "incompatible target",
			currentTarget:       "other.txt",
			wantDisposition:     runtimecontract.RuntimeCoverageProofLineageDispositionIncompatible,
			wantCurrentCredit:   false,
			wantMissingEvidence: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			controller, repository, aggregate, catalog, prerequisite, observation := sealedSemanticPlanningAggregate(t)
			oldBinding := aggregate.Facts.Focus.CoverageEvidenceBindings[0]
			accepted := seedAcceptedCoverageProofLineageForSemanticReseal(t, &aggregate, oldBinding)

			changed, err := applyReceiptResourceChangeInvalidation(&aggregate, "dispatch-lineage-reseal", "operation-lineage-reseal", runtimecontract.RuntimeOperationReceipt{
				ExecutionKey: "execution/lineage-reseal", Fence: 1, OperationID: "operation-lineage-reseal",
				ResourceChanges: []runtimecontract.RuntimeResourceChange{{
					Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					SubjectRef: "workspace_deliverable", BeforeRevision: catalog.ObservedRevision, AfterRevision: "file-revision-2",
				}},
			})
			if err != nil || !changed || !semanticCatalogPrerequisitePending(aggregate, prerequisite.ID) {
				t.Fatalf("receipt invalidation did not reopen semantic acquisition: changed=%t err=%v aggregate=%+v", changed, err, aggregate)
			}
			facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil, aggregate.CoverageProofLineage)
			if err != nil {
				t.Fatal(err)
			}
			aggregate.Facts, aggregate.Decision = facts, decision
			aggregate.StateRevision++
			if err := aggregate.Validate(); err != nil {
				t.Fatal(err)
			}
			repository.aggregate = &aggregate

			currentCatalog := catalog.Normalized()
			currentCatalog.TargetRefs = []string{testCase.currentTarget}
			currentCatalog.ObservedRevision = "file-revision-2"
			currentCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(currentCatalog)
			currentObservation := observation.Normalized()
			currentObservation.Dependencies = append([]runtimecontract.RuntimeFreshnessDependency(nil), currentObservation.Dependencies...)
			currentObservation.ContentHash = currentCatalog.SnapshotID
			currentObservation.Dependencies[0].Revision = currentCatalog.ObservedRevision
			currentObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(currentObservation)
			reacquired, err := controller.commitPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, PrerequisiteResult{
				PrerequisiteID: prerequisite.ID, OperationAdmissibility: &currentCatalog, Observation: &currentObservation,
			})
			if err != nil {
				t.Fatal(err)
			}
			if reacquired.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || len(reacquired.CoverageProofLineage) != 1 {
				t.Fatalf("reacquisition advanced before a re-seal: phase=%s lineage=%+v", reacquired.SemanticPlanning.Phase, reacquired.CoverageProofLineage)
			}
			if got := reacquired.Facts.Evidence.Accepted[0].BindingRefs; len(got) != 1 || got[0] != oldBinding.ID {
				t.Fatalf("reacquisition rewrote immutable source evidence: %v", got)
			}

			resealed := resealSemanticPlanningGenerationForLineageTest(t, controller, reacquired, currentCatalog, currentObservation, testCase.currentTarget)
			if len(resealed.CoverageProofLineage) != 2 {
				t.Fatalf("re-seal lineage rows = %+v", resealed.CoverageProofLineage)
			}
			resolution := resealed.CoverageProofLineage[1]
			if resolution.Disposition != testCase.wantDisposition {
				t.Fatalf("resolution disposition = %q, want %q; resolution=%+v", resolution.Disposition, testCase.wantDisposition, resolution)
			}
			if got := runtimeEvidenceAppliesToCurrentCoverageWithLineage(resealed.Facts, accepted, resealed.CoverageProofLineage); got != testCase.wantCurrentCredit {
				t.Fatalf("current proof credit = %t, want %t; lineage=%+v", got, testCase.wantCurrentCredit, resealed.CoverageProofLineage)
			}
			missing := missingEvidenceKinds(resealed.Facts, resealed.CoverageProofLineage)
			if (len(missing) != 0) != testCase.wantMissingEvidence {
				t.Fatalf("missing evidence = %v, wantMissing=%t", missing, testCase.wantMissingEvidence)
			}
		})
	}
}

func TestRuntimeSemanticCatalogReacquisitionWithUnchangedIdentityKeepsGeneration(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate, catalog, prerequisite, observation := sealedSemanticPlanningAggregate(t)
	beforeLedger := aggregate.SemanticPlanning.Clone()
	changed, err := applyReceiptResourceChangeInvalidation(&aggregate, "dispatch-same-catalog", "operation-same-catalog", runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "execution/semantic-same-catalog", Fence: 1, OperationID: "operation-same-catalog",
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", BeforeRevision: catalog.ObservedRevision, AfterRevision: "file-revision-2",
		}},
	})
	if err != nil || !changed || !semanticCatalogPrerequisitePending(aggregate, prerequisite.ID) {
		t.Fatalf("same-identity receipt invalidation did not open reacquisition: changed=%t err=%v aggregate=%+v", changed, err, aggregate)
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	repository.aggregate = &aggregate

	// The owner re-observes a new resource revision, but its admissibility
	// vocabulary/snapshot is unchanged.  The semantic generation may therefore
	// be reused; only the freshness observation identity changes.
	newObservation := observation
	newObservation.Dependencies[0].Revision = "file-revision-2"
	newObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(newObservation)
	updated, err := controller.commitPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &catalog, Observation: &newObservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.SemanticPlanning.Phase != beforeLedger.Phase || len(updated.SemanticPlanning.History) != len(beforeLedger.History) ||
		updated.SemanticPlanning.Manifest == nil || updated.SemanticPlanning.Manifest.Hash != beforeLedger.Manifest.Hash {
		t.Fatalf("unchanged catalog identity unexpectedly retired semantic generation: before=%+v after=%+v", beforeLedger, updated.SemanticPlanning)
	}
	if semanticCatalogPrerequisitePending(updated, prerequisite.ID) || len(updated.Facts.Resources.PrerequisiteObservations) != 1 ||
		updated.Facts.Resources.PrerequisiteObservations[0].ObservationID != newObservation.ObservationID {
		t.Fatalf("same-identity reacquisition did not leave one fresh current observation: facts=%+v", updated.Facts)
	}
}

func TestRuntimeSemanticCatalogInvalidationLineageExcludesRetiredSignatures(t *testing.T) {
	_, _, aggregate, _, prerequisite, _ := sealedSemanticPlanningAggregate(t)
	oldLedger := aggregate.SemanticPlanning.Clone()
	aggregate.SemanticPlanning.History = []runtimecontract.RuntimeSemanticPlanningGeneration{{
		RetiredAtStateRevision: 2, Reason: runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated,
		InvalidationSignatures: []string{"invalidation-old"}, PrerequisiteIDs: []string{prerequisite.ID}, Snapshot: oldLedger,
	}}
	aggregate.PrerequisiteInvalidations = []runtimecontract.RuntimePrerequisiteInvalidationRecord{
		{Signature: "invalidation-old", PrerequisiteIDs: []string{prerequisite.ID}, ObservationIDs: []string{"observation-old"}},
		{Signature: "invalidation-current", PrerequisiteIDs: []string{prerequisite.ID}, ObservationIDs: []string{"observation-current"}},
	}
	ids, signatures := semanticCatalogInvalidationLineage(aggregate, *oldLedger.Manifest, []RuntimeSemanticCatalogFreshnessGap{{PrerequisiteID: prerequisite.ID}})
	if len(ids) != 1 || ids[0] != prerequisite.ID || len(signatures) != 1 || signatures[0] != "invalidation-current" {
		t.Fatalf("historical invalidation signature leaked into current generation lineage: ids=%v signatures=%v", ids, signatures)
	}
}

func TestRuntimeSemanticCatalogInvalidationEventRejectsUnknownRetiredManifest(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate, _, _, _ := sealedSemanticPlanningAggregate(t)
	aggregate.Facts.Resources.PrerequisiteObservations = nil
	repository.aggregate = &aggregate
	beforeRevision := aggregate.StateRevision
	if _, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticCatalogInvalidated, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ManifestHash: "semantic-manifest-unknown", LogicalRequestID: aggregate.SemanticPlanning.LogicalRequestID,
	}); err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unknown retired semantic manifest was accepted: %v", err)
	}
	if repository.aggregate.StateRevision != beforeRevision {
		t.Fatalf("rejected semantic invalidation changed state revision: before=%d after=%d", beforeRevision, repository.aggregate.StateRevision)
	}
	if _, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticCatalogInvalidated, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ManifestHash: aggregate.SemanticPlanning.Manifest.Hash, LogicalRequestID: "semantic-request-from-another-generation",
	}); err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("semantic invalidation with a mismatched logical generation was accepted: %v", err)
	}
	if repository.aggregate.StateRevision != beforeRevision {
		t.Fatalf("mismatched semantic invalidation changed state revision: before=%d after=%d", beforeRevision, repository.aggregate.StateRevision)
	}
	retired, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticCatalogInvalidated, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ManifestHash: aggregate.SemanticPlanning.Manifest.Hash, LogicalRequestID: aggregate.SemanticPlanning.LogicalRequestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	revision := retired.Aggregate.StateRevision
	duplicate, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticCatalogInvalidated, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		ManifestHash: aggregate.SemanticPlanning.Manifest.Hash, LogicalRequestID: aggregate.SemanticPlanning.LogicalRequestID,
	})
	if err != nil || duplicate.Aggregate.StateRevision != revision {
		t.Fatalf("matching semantic invalidation was not idempotent: err=%v duplicate=%+v", err, duplicate.Aggregate)
	}
}

func TestSemanticManifestCatalogFreshnessGapKeepsOwnerScopedObservationIdentity(t *testing.T) {
	_, _, aggregate, catalog, _, observation := sealedSemanticPlanningAggregate(t)
	wrongOwner := observation
	wrongOwner.OwnerModuleID = runtimecontract.RuntimeModuleGit
	aggregate.Facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{wrongOwner}
	gaps := SemanticManifestCatalogFreshnessGaps(aggregate, *aggregate.SemanticPlanning.Manifest)
	if len(gaps) != 1 || gaps[0].ManifestObservationID != "" || gaps[0].CurrentObservationID != "" {
		t.Fatalf("cross-owner observation was treated as current catalog identity: catalog=%+v observation=%+v gaps=%+v", catalog, wrongOwner, gaps)
	}
	wrongKind := observation
	wrongKind.Kind = runtimecontract.PrerequisiteKindCapability
	wrongKind.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(wrongKind)
	aggregate.Facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{wrongKind}
	gaps = SemanticManifestCatalogFreshnessGaps(aggregate, *aggregate.SemanticPlanning.Manifest)
	if len(gaps) != 1 || gaps[0].ManifestObservationID != "" || gaps[0].CurrentObservationID != "" {
		t.Fatalf("wrong-kind observation was treated as semantic catalog lineage: catalog=%+v observation=%+v gaps=%+v", catalog, wrongKind, gaps)
	}
}

func TestRuntimeSemanticCatalogInvalidationNeverRetiresActiveDispatch(t *testing.T) {
	_, _, aggregate, catalog, _, _ := sealedSemanticPlanningAggregate(t)
	changedCatalog := catalog.Normalized()
	changedCatalog.ObservedRevision = "file-revision-2"
	changedCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(changedCatalog)
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{changedCatalog}
	aggregate.ActiveDispatch = &SealedDispatch{ID: "dispatch-active", State: DispatchStateSealed}
	before := aggregate.SemanticPlanning.Clone()
	retired, err := resetSemanticPlanningForInvalidatedCatalog(&aggregate, runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated)
	if err != nil || retired || aggregate.SemanticPlanning.Phase != before.Phase || len(aggregate.SemanticPlanning.History) != len(before.History) {
		t.Fatalf("active dispatch semantic retirement = retired:%t err:%v ledger:%+v", retired, err, aggregate.SemanticPlanning)
	}
}

func TestRuntimeSemanticRetirementPreservesPriorProviderDocumentOutcome(t *testing.T) {
	ledger := runtimecontract.RuntimeSemanticPlanningLedger{}
	// Build the sealed ledger through the package fixture so the retirement
	// helper is tested against a complete generation, not a hand-written shape.
	_, _, aggregate, _, prerequisite, _ := sealedSemanticPlanningAggregate(t)
	ledger = aggregate.SemanticPlanning.Clone()
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:  runtimecontract.RuntimeProviderDocumentSemanticPlan,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepProofBatch,
		ManifestHash:  ledger.Manifest.Hash, DefinitionSeedHash: ledger.DefinitionSeed.Hash,
		LogicalRequestID: "old-provider-request", StablePrefixHash: "old-provider-prefix",
		BatchID: ledger.Document.BatchID, BatchSequence: ledger.Document.BatchSequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), ledger.Document.BatchCoverageWorkItemIDs...),
		CoverageOrderHash:        runtimecontract.StableRuntimeSemanticPlanCoverageOrderHash(*ledger.CoverageOrderDocument),
		MaxOutputTokens:          1024, MaxRecoveryAttempts: 1,
	}
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	outcome := runtimecontract.RuntimeProviderDocumentOutcome{
		Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: expected.Hash,
		ResponseHash: "provider-response-old", ResponseBytes: 128, Code: "max_output_tokens", Signature: "provider/incomplete",
	}
	aggregate.CandidateRequestSessions = []runtimecontract.RuntimeCandidateRequestSession{{
		LogicalRequestID: expected.LogicalRequestID, DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan,
		ExpectedDocument: &expected, Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-old"},
		StablePrefixHash: expected.StablePrefixHash, ManifestHash: expected.ManifestHash,
		State: runtimecontract.RuntimeCandidateRequestSessionRecoveryPending, NextSequence: 2, IncompleteOutcomes: 1,
		LastDocumentOutcome: &outcome, LastOutcome: runtimecontract.RuntimeCandidateRequestOutcome{
			Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentIncomplete, Code: outcome.Code, Signature: outcome.Signature,
		},
	}}
	if err := retireSemanticPlanningGeneration(&aggregate, runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated, []RuntimeSemanticCatalogFreshnessGap{{PrerequisiteID: prerequisite.ID}}); err != nil {
		t.Fatal(err)
	}
	if got := aggregate.CandidateRequestSessions[0].LastDocumentOutcome; got == nil || got.ResponseHash != outcome.ResponseHash || got.Code != outcome.Code {
		t.Fatalf("retirement discarded prior provider document outcome: %+v", got)
	}
	if aggregate.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
		aggregate.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
		t.Fatalf("retirement did not close the old provider session: %+v", aggregate.CandidateRequestSessions[0])
	}
}

func TestRuntimeSemanticCrossOwnerPartialBatchIsOrderIndependentAndRecoversAfterResponseLoss(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	facts.Capabilities.StateChangeKinds = append(facts.Capabilities.StateChangeKinds, runtimecontract.StepKindGitWrite)
	secondItem := runtimecontract.RuntimeCoverageItemFact{
		ID: "cwi-v2-second", SliceID: "slice-v2", SourceScope: "dodItem", SourceID: "dod-v2-second",
		TargetText: "deliver second", State: runtimecontract.RuntimeCoverageItemStateOpen,
	}
	facts.Coverage = append(facts.Coverage, secondItem)
	facts.Slices[0].CoverageWorkItemIDs = append(facts.Slices[0].CoverageWorkItemIDs, secondItem.ID)
	facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
		ID: "focus-v2-second", SliceID: secondItem.SliceID,
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{secondItem.ID},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AcceptedStepKinds:     []string{runtimecontract.StepKindGitWrite},
		TargetRefs:            []string{"workspace_deliverable"},
	}}
	facts.DefinitionSeed.CoverageItems = append(facts.DefinitionSeed.CoverageItems, runtimecontract.RuntimeCoverageItem{
		ID: secondItem.ID, SliceID: secondItem.SliceID, SourceScope: secondItem.SourceScope, SourceID: secondItem.SourceID, TargetText: secondItem.TargetText,
	})
	facts.DefinitionSeed.Slices = facts.Slices
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	definition := controllerDefinition()
	definition.CapabilityContract = append(definition.CapabilityContract, runtimecontract.StepKindGitWrite)
	fileCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"first.txt"}, ObservedRevision: "file-revision-1",
	}
	fileCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileCatalog)
	gitCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "semantic-git", OwnerModuleID: runtimecontract.RuntimeModuleGit,
		OperationKinds: []string{runtimecontract.StepKindGitWrite}, TargetRefs: []string{"workspace_deliverable"}, ObservedRevision: "repository-revision-1",
	}
	gitCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(gitCatalog)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{fileCatalog, gitCatalog}
	aggregate, err := controller.initialize(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		aggregate.Definition,
		aggregate.Facts.DefinitionSeed,
		aggregate.Facts.Activation.ExecutionContract,
		aggregate.Facts.Resources.OperationAdmissibilities,
		runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts),
	)
	if err != nil {
		t.Fatal(err)
	}
	order := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{manifest.CoverageItems[0].ID, manifest.CoverageItems[1].ID},
	}
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	proposals := []runtimecontract.RuntimeSemanticProofProposal{
		{
			CoverageWorkItemID: manifest.CoverageItems[0].ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite,
			Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"first.txt"}},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			DeclarationRef:        "semantic/" + string(manifest.CoverageItems[0].ID),
		},
	}
	activeProposal := proposals[0]
	activeFeasibility := runtimecontract.RuntimeSemanticProofFeasibility{
		SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: activeProposal.CoverageWorkItemID,
		ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(activeProposal), OwnerModuleID: activeProposal.OwnerModuleID,
		CatalogSnapshotID: fileCatalog.SnapshotID, Status: runtimecontract.RuntimeSemanticFeasibilityFeasible,
		ObservedRevision: fileCatalog.ObservedRevision, TargetRefs: activeProposal.Target.ExactTargetRefs,
	}
	apply := func(active *RuntimeController, event RuntimeInternalEvent) RuntimeAggregate {
		t.Helper()
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, applyErr := active.ApplyInternalEvent(ctx, event)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", event.Kind, applyErr)
		}
		aggregate = result.Aggregate
		return aggregate
	}
	apply(controller, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"})
	apply(controller, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: "semantic-request-1"})
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, order, 0, "proof-request-1", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	proof := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID: batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
		Proposals:                []runtimecontract.RuntimeSemanticProofProposal{activeProposal},
	}
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply(controller, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID})
	apply(controller, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	batchID := aggregate.SemanticPlanning.FeasibilityBatchID
	responseLoss := &semanticResponseLossRepository{
		RuntimeStateRepository: repository, marker: "semantic-partial-batch-response-loss",
		match: func(candidate RuntimeAggregate) bool { return len(candidate.SemanticPlanning.Feasibilities) == 1 },
	}
	lostController, err := NewRuntimeController(responseLoss)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "semantic-partial-batch-response-loss" {
				t.Fatalf("unexpected partial-batch response-loss marker: %v", recovered)
			}
		}()
		_, _ = lostController.ApplyInternalEvent(ctx, RuntimeInternalEvent{
			Kind: RuntimeInternalEventSemanticFeasibilityObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			SemanticBatchID: batchID, SemanticFeasibility: &activeFeasibility,
		})
		t.Fatal("expected partial semantic batch response loss after durable CAS")
	}()
	aggregate = *repository.aggregate
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	revision := aggregate.StateRevision
	apply(restarted, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: batchID, SemanticFeasibility: &activeFeasibility})
	if aggregate.StateRevision != revision {
		t.Fatal("restart replay changed the committed out-of-order Git feasibility observation")
	}
	conflictingFile := activeFeasibility.Clone()
	conflictingFile.TargetRefs = []string{"different_file_target"}
	if _, conflictErr := restarted.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticFeasibilityObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticBatchID: batchID, SemanticFeasibility: &conflictingFile,
	}); !errors.Is(conflictErr, common.ErrConflict) {
		t.Fatalf("conflicting Git feasibility error = %v, want conflict", conflictErr)
	}
	if repository.aggregate.StateRevision != revision {
		t.Fatalf("conflicting Git feasibility changed durable revision: got %d want %d", repository.aggregate.StateRevision, revision)
	}
	if len(aggregate.SemanticPlanning.Feasibilities) != 1 || aggregate.SemanticPlanning.FeasibilitySetHash != runtimecontract.StableRuntimeSemanticFeasibilitySetHash([]runtimecontract.RuntimeSemanticProofFeasibility{activeFeasibility}) {
		t.Fatalf("partial semantic batch did not resume at the missing owner fact: %+v", aggregate.SemanticPlanning)
	}
	accepted := aggregate.SemanticPlanning.Batches[0]
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, accepted, proof)
	if err != nil {
		t.Fatal(err)
	}
	apply(restarted, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, SemanticPlanMergedDocument: &merged})
	apply(restarted, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed || len(aggregate.Facts.ReconciliationContract.Bindings) != 1 {
		t.Fatalf("recovered semantic focus did not seal current binding set: %+v", aggregate)
	}
	if len(aggregate.Facts.RemainingFocuses) != 1 || len(aggregate.Facts.RemainingFocuses[0].CoverageEvidenceBindings) != 0 || aggregate.Facts.RemainingFocuses[0].CoverageWorkItemIDs[0] != secondItem.ID {
		t.Fatalf("later cross-owner focus was not retained pending: %+v", aggregate.Facts.RemainingFocuses)
	}
}

func TestRuntimeSemanticMultiBatchFeasibilityMergeRecoversAfterResponseLoss(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate, manifest, order, proposals, feasibilities := newTwoItemSemanticBatchFixture(t)
	apply := func(event RuntimeInternalEvent) {
		t.Helper()
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(ctx, event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "multi-batch-prefix", LogicalRequestID: "order-request"})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: "order-request"})
	// First transaction is scoped to the active focus only. The second CWI is
	// retained as an inert pending placeholder and cannot enter this proof
	// document or owner-fact set.
	firstBatch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, order, 0, "batch-request-0", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, firstBatch)
	if err != nil {
		t.Fatal(err)
	}
	firstDocument := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch, BatchID: firstBatch.BatchID, BatchSequence: firstBatch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), firstBatch.CoverageWorkItemIDs...),
		Proposals:                []runtimecontract.RuntimeSemanticProofProposal{proposals[0]},
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &firstDocument, LogicalRequestID: firstBatch.RequestLogicalID})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibilities[0]})
	firstAccepted := aggregate.SemanticPlanning.Batches[0]
	firstMerged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, firstAccepted, firstDocument)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, SemanticPlanMergedDocument: &firstMerged})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	if len(aggregate.Facts.RemainingFocuses) != 1 || len(aggregate.Facts.RemainingFocuses[0].CoverageEvidenceBindings) != 0 {
		t.Fatalf("first focus admitted later CWI as authority: %+v", aggregate.Facts.RemainingFocuses)
	}
	// Simulate the SM-4 evidence commit that satisfies the first CWI and
	// advances SM-2. This is persisted through the same owner-local helpers;
	// no provider or capability owner is involved in selecting the next focus.
	if err := runtimeTestAdvanceSemanticFocus(t, repository, &aggregate); err != nil {
		t.Fatal(err)
	}
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningOrderAccepted ||
		aggregate.Facts.Focus.ID != "semantic-pending/cwi-v2-second" {
		t.Fatalf("next focus did not reopen retained order lineage: phase=%s focus=%+v", aggregate.SemanticPlanning.Phase, aggregate.Facts.Focus)
	}
	secondBatch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, order, aggregate.SemanticPlanning.NextBatchSequence, "batch-request-1", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, secondBatch)
	if err != nil {
		t.Fatal(err)
	}
	secondDocument := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch, BatchID: secondBatch.BatchID, BatchSequence: secondBatch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), secondBatch.CoverageWorkItemIDs...),
		Proposals:                []runtimecontract.RuntimeSemanticProofProposal{proposals[1]},
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &secondDocument, LogicalRequestID: secondBatch.RequestLogicalID})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibilities[1]})
	secondAccepted := aggregate.SemanticPlanning.Batches[1]
	secondMerged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, secondAccepted, secondDocument)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, SemanticPlanMergedDocument: &secondMerged})
	// Lose the response after the second seal has been durably committed. The
	// restarted owner must observe the same cumulative reconciliation lineage.
	responseLoss := &semanticResponseLossRepository{RuntimeStateRepository: repository, marker: "semantic-multi-batch-seal-response-loss", match: func(candidate RuntimeAggregate) bool {
		return candidate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningSealed && len(candidate.Facts.ReconciliationContract.Bindings) == 2
	}}
	controller.repository = responseLoss
	controller.transitions.repository = responseLoss
	func() {
		defer func() {
			if recovered := recover(); recovered != "semantic-multi-batch-seal-response-loss" {
				t.Fatalf("unexpected feasibility merge response-loss marker: %v", recovered)
			}
		}()
		_, _ = controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID})
		t.Fatal("expected second semantic seal response loss after durable CAS")
	}()
	aggregate = *repository.aggregate
	controller, err = NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	revision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	if aggregate.StateRevision != revision {
		t.Fatal("replayed second semantic seal changed durable revision")
	}
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed || len(aggregate.Facts.ReconciliationContract.Bindings) != 2 {
		t.Fatalf("multi-batch focus recovery did not seal: %+v", aggregate.SemanticPlanning)
	}
}

// Catalog invalidation retires only the authority that remains eligible for a
// new semantic generation. A CWI already settled by a receipt remains out of
// the provider-facing plan and retains its immutable binding/evidence audit.
func TestRuntimeSemanticCatalogInvalidationDoesNotReplanSettledCWI(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate, manifest, order, proposals, feasibilities := newTwoItemSemanticBatchFixture(t)
	aggregate = sealFirstTwoItemSemanticBatch(t, controller, aggregate, manifest, order, proposals, feasibilities)
	if err := runtimeTestAdvanceSemanticFocus(t, repository, &aggregate); err != nil {
		t.Fatal(err)
	}

	settledID := common.CoverageWorkItemID("cwi-v2")
	pendingID := common.CoverageWorkItemID("cwi-v2-second")
	settled, found := runtimeCoverageByID(aggregate.Facts.Coverage)[settledID]
	if !found || settled.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled || settled.CurrentBindingRef == nil {
		t.Fatalf("first CWI was not settled before catalog invalidation: %+v", aggregate.Facts.Coverage)
	}
	settledBinding := settled.CurrentBindingRef.Clone()
	if aggregate.Facts.Focus.ID != "semantic-pending/"+string(pendingID) || len(aggregate.Facts.CompletedFocuses) != 1 {
		t.Fatalf("fixture did not advance to the second pending CWI: focus=%+v completed=%+v", aggregate.Facts.Focus, aggregate.Facts.CompletedFocuses)
	}

	// A missing current catalog observation makes the active semantic generation
	// stale. The old manifest remains in history, but the settled CWI must not be
	// moved back into the new generation's pending projection.
	aggregate.Facts.Resources.PrerequisiteObservations = nil
	repository.aggregate = &aggregate
	beforeSessions := len(aggregate.CandidateRequestSessions)
	beforeEvidence := len(aggregate.Facts.Evidence.Accepted)
	beforeDispatches := len(aggregate.CompletedDispatches)
	result, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:             RuntimeInternalEventSemanticCatalogInvalidated,
		WorkspaceID:      aggregate.Facts.WorkspaceID,
		RunID:            aggregate.Facts.RunID,
		ManifestHash:     aggregate.SemanticPlanning.Manifest.Hash,
		LogicalRequestID: aggregate.SemanticPlanning.LogicalRequestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate = result.Aggregate

	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || len(aggregate.SemanticPlanning.History) != 1 {
		t.Fatalf("catalog invalidation did not retire exactly the active generation: %+v", aggregate.SemanticPlanning)
	}
	settled, found = runtimeCoverageByID(aggregate.Facts.Coverage)[settledID]
	if !found || settled.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled ||
		settled.State != runtimecontract.RuntimeCoverageItemStateSatisfied || settled.CurrentBindingRef == nil ||
		!runtimecontract.SameRuntimeCWIBindingRef(*settled.CurrentBindingRef, settledBinding) {
		t.Fatalf("catalog invalidation rewrote settled CWI authority: %+v", settled)
	}
	if len(aggregate.Facts.CompletedFocuses) != 1 || aggregate.Facts.CompletedFocuses[0].CoverageWorkItemIDs[0] != settledID ||
		aggregate.Facts.Focus.ID != "semantic-pending/"+string(pendingID) || len(aggregate.Facts.Focus.CoverageEvidenceBindings) != 0 {
		t.Fatalf("catalog invalidation rebuilt settled/pending focus projection: focus=%+v completed=%+v", aggregate.Facts.Focus, aggregate.Facts.CompletedFocuses)
	}
	if len(aggregate.CandidateRequestSessions) != beforeSessions || len(aggregate.Facts.Evidence.Accepted) != beforeEvidence ||
		len(aggregate.CompletedDispatches) != beforeDispatches || aggregate.ActiveDispatch != nil {
		t.Fatalf("catalog invalidation created an operation/provider side effect: sessions=%+v evidence=%+v dispatches=%+v active=%+v", aggregate.CandidateRequestSessions, aggregate.Facts.Evidence.Accepted, aggregate.CompletedDispatches, aggregate.ActiveDispatch)
	}
	if bindings := aggregate.Facts.ReconciliationContract.Bindings; len(bindings) != 1 || bindings[0].CoverageWorkItemID != settledID || bindings[0].BindingID != settledBinding.BindingID {
		t.Fatalf("current reconciliation contract did not retain only settled CWI binding: %+v", aggregate.Facts.ReconciliationContract)
	}

	eligible := runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts)
	if !sameOrderedSemanticCoverageIDs(eligible, []common.CoverageWorkItemID{pendingID}) {
		t.Fatalf("new semantic generation eligibility = %v, want only %q", eligible, pendingID)
	}
	freshManifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Definition, aggregate.Facts.DefinitionSeed,
		aggregate.Facts.Activation.ExecutionContract, aggregate.Facts.Resources.OperationAdmissibilities, eligible,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(freshManifest.CoverageItems) != 1 || freshManifest.CoverageItems[0].ID != pendingID {
		t.Fatalf("fresh semantic manifest reintroduced settled CWI: %+v", freshManifest.CoverageItems)
	}
}

// sealFirstTwoItemSemanticBatch leaves the first CWI under a sealed binding and
// the second as the persisted pending projection from the same semantic order.
// It is test construction only; all mutations still go through the private
// controller event/CAS boundary used in production.
func sealFirstTwoItemSemanticBatch(
	t *testing.T,
	controller *RuntimeController,
	aggregate RuntimeAggregate,
	manifest runtimecontract.RuntimeSemanticPlanManifest,
	order runtimecontract.RuntimeSemanticPlanDocument,
	proposals []runtimecontract.RuntimeSemanticProofProposal,
	feasibilities []runtimecontract.RuntimeSemanticProofFeasibility,
) RuntimeAggregate {
	t.Helper()
	if controller == nil || len(proposals) < 1 || len(feasibilities) < 1 {
		t.Fatal("two-item semantic batch fixture is incomplete")
	}
	ctx := context.Background()
	apply := func(event RuntimeInternalEvent) {
		t.Helper()
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(ctx, event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
	}

	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "settled-cwi-catalog-prefix", LogicalRequestID: "settled-cwi-order-request"})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: "settled-cwi-order-request"})
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, order, 0, "settled-cwi-batch-request", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID:  batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
		Proposals:                []runtimecontract.RuntimeSemanticProofProposal{proposals[0]},
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &document, LogicalRequestID: batch.RequestLogicalID})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibilities[0]})
	accepted := aggregate.SemanticPlanning.Batches[0]
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, accepted, document)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, SemanticPlanMergedDocument: &merged})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	return aggregate
}

func newTwoItemSemanticBatchFixture(t *testing.T) (*RuntimeController, *controllerRepository, RuntimeAggregate, runtimecontract.RuntimeSemanticPlanManifest, runtimecontract.RuntimeSemanticPlanDocument, []runtimecontract.RuntimeSemanticProofProposal, []runtimecontract.RuntimeSemanticProofFeasibility) {
	t.Helper()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	second := runtimecontract.RuntimeCoverageItemFact{ID: "cwi-v2-second", SliceID: facts.Slices[0].ID, SourceScope: "dodItem", SourceID: "dod-v2-second", TargetText: "deliver second", State: runtimecontract.RuntimeCoverageItemStateOpen}
	facts.Coverage = append(facts.Coverage, second)
	facts.Slices[0].CoverageWorkItemIDs = append(facts.Slices[0].CoverageWorkItemIDs, second.ID)
	facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
		ID: "focus-v2-second", SliceID: second.SliceID,
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{second.ID},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AcceptedStepKinds:     []string{runtimecontract.StepKindFileWrite},
		TargetRefs:            []string{"target.txt"},
	}}
	facts.DefinitionSeed.CoverageItems = append(facts.DefinitionSeed.CoverageItems, runtimecontract.RuntimeCoverageItem{ID: second.ID, SliceID: second.SliceID, SourceScope: second.SourceScope, SourceID: second.SourceID, TargetText: second.TargetText})
	facts.DefinitionSeed.Slices = facts.Slices
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	catalog, prerequisite, observation := semanticPlanningCatalogFixture()
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	aggregate, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		aggregate.Definition,
		aggregate.Facts.DefinitionSeed,
		aggregate.Facts.Activation.ExecutionContract,
		aggregate.Facts.Resources.OperationAdmissibilities,
		runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts),
	)
	if err != nil {
		t.Fatal(err)
	}
	baseProposal := runtimecontract.RuntimeSemanticProofProposal{CoverageWorkItemID: manifest.CoverageItems[0].ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite, Target: runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"target.txt"}}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, DeclarationRef: "semantic/" + string(manifest.CoverageItems[0].ID)}
	baseFeasibility := runtimecontract.RuntimeSemanticProofFeasibility{SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: baseProposal.CoverageWorkItemID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(baseProposal), OwnerModuleID: baseProposal.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID, Status: runtimecontract.RuntimeSemanticFeasibilityFeasible, ObservedRevision: catalog.ObservedRevision, TargetRefs: baseProposal.Target.ExactTargetRefs}
	secondProposal := baseProposal.Clone()
	secondProposal.CoverageWorkItemID = second.ID
	secondProposal.DeclarationRef = "semantic/" + string(second.ID)
	proposals := []runtimecontract.RuntimeSemanticProofProposal{baseProposal, secondProposal}
	secondFeasibility := baseFeasibility.Clone()
	secondFeasibility.CoverageWorkItemID = second.ID
	secondFeasibility.ProposalHash = runtimecontract.StableRuntimeSemanticProofProposalHash(secondProposal)
	order := runtimecontract.RuntimeSemanticPlanDocument{SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{manifest.CoverageItems[0].ID, second.ID}}
	return controller, repository, aggregate, manifest, order, proposals, []runtimecontract.RuntimeSemanticProofFeasibility{baseFeasibility, secondFeasibility}
}

func semanticPlanningCatalogFixture() (runtimecontract.RuntimeOperationAdmissibility, runtimecontract.RuntimePrerequisite, runtimecontract.RuntimePrerequisiteObservation) {
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"target.txt"}, ObservedRevision: "file-revision-1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite},
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: catalog.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return catalog, prerequisite, observation
}

func sealedSemanticPlanningAggregate(t *testing.T) (*RuntimeController, *controllerRepository, RuntimeAggregate, runtimecontract.RuntimeOperationAdmissibility, runtimecontract.RuntimePrerequisite, runtimecontract.RuntimePrerequisiteObservation) {
	t.Helper()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	catalog, prerequisite, observation := semanticPlanningCatalogFixture()
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	aggregate, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, document, feasibility := semanticPlanningFixture(t, aggregate)
	order := document.Clone()
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID = ""
	order.BatchSequence = 0
	order.BatchCoverageWorkItemIDs = nil
	order.CoverageOrder = []common.CoverageWorkItemID{manifest.CoverageItems[0].ID}
	order.Proposals = nil
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply := func(event RuntimeInternalEvent) {
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, applyErr := controller.ApplyInternalEvent(context.Background(), event)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", event.Kind, applyErr)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "semantic-retirement-prefix", LogicalRequestID: "semantic-retirement-request"})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: "semantic-retirement-request"})
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(manifest, order, 0, "semantic-retirement-batch", aggregate.Facts.Focus.CoverageWorkItemIDs, aggregate.SemanticPlanning.PendingCoverageIDs, 1)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	proof := document.Clone()
	proof.PlanStep = runtimecontract.RuntimeSemanticPlanStepProofBatch
	proof.BatchID = batch.BatchID
	proof.BatchSequence = batch.Sequence
	proof.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
	proof.CoverageOrder = nil
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	feasibility.CatalogSnapshotID = catalog.SnapshotID
	feasibility.CatalogObservationID = observation.ObservationID
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility})
	accepted := aggregate.SemanticPlanning.Batches[0]
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, accepted, proof)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, SemanticPlanMergedDocument: &merged})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	return controller, repository, aggregate, catalog, prerequisite, observation
}

// seedAcceptedCoverageProofLineageForSemanticReseal prepares a receipt that
// was accepted under the source binding before a later catalog re-acquisition.
// The test then drives the new binding through the normal semantic seal path;
// it never rewrites the source evidence to make it appear current.
func seedAcceptedCoverageProofLineageForSemanticReseal(t *testing.T, aggregate *RuntimeAggregate, binding runtimecontract.CoverageEvidenceBinding) runtimecontract.EvidenceFact {
	t.Helper()
	if aggregate == nil || aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed {
		t.Fatal("coverage proof lineage source requires a sealed semantic aggregate")
	}
	binding = binding.Normalized()
	if _, found := runtimecontract.CoverageEvidenceBindingForRef(aggregate.Facts.Focus.CoverageEvidenceBindings, binding.ID); !found {
		t.Fatalf("source binding %q is not current before re-acquisition", binding.ID)
	}

	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey:        "execution/semantic-lineage-source",
		Fence:               1,
		OperationID:         "operation-semantic-lineage-source",
		Outcome:             runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		Changed:             true,
		TargetRefs:          append([]string(nil), binding.TargetRefs...),
		Evidence: runtimecontract.RuntimeOperationEvidence{
			Quality: common.EvidenceQualityValid,
		},
	}.Normalized()
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	fact := runtimecontract.EvidenceFact{
		ID:                  "evidence-semantic-lineage-source",
		Kind:                runtimecontract.EvidenceKindStateChange,
		SubjectRef:          "semantic-lineage-source",
		AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
		DispatchID:          "dispatch-semantic-lineage-source",
		OperationID:         receipt.OperationID,
		Revision:            nextEvidenceRevision(aggregate.EvidenceSequence + 1),
		Summary:             "accepted source proof before semantic re-seal",
		TargetRefs:          append([]string(nil), binding.TargetRefs...),
		BindingRefs:         []string{binding.ID},
		CoverageWorkItemIDs: []common.CoverageWorkItemID{binding.CoverageWorkItemID},
		Quality:             common.EvidenceQualityValid,
	}
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, SealedDispatch{
		ID: fact.DispatchID,
		OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{
			OperationID:      receipt.OperationID,
			ExecutionKey:     receipt.ExecutionKey,
			Fence:            receipt.Fence,
			OwnerReceiptHash: receiptHash,
			State:            runtimecontract.OperationExecutionEvidenceCommitted,
		}},
	})
	aggregate.OperationReceipts = append(aggregate.OperationReceipts, receipt)
	aggregate.Facts.Evidence.Accepted = append(aggregate.Facts.Evidence.Accepted, fact)
	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	// The source receipt is audit evidence only. It cannot directly mark the
	// CWI satisfied: only the normal SM-2 evidence/assessment transition may
	// settle the canonical ledger. Keeping it open lets this fixture prove that
	// a new binding resolves old evidence before any credit is considered.
	if err := appendAcceptedCoverageProofLineage(aggregate, fact, receipt); err != nil {
		t.Fatalf("append source coverage proof lineage: %v", err)
	}
	return fact
}

// resealSemanticPlanningGenerationForLineageTest follows the same private
// order, bounded-batch, owner-feasibility, merge and seal sequence as Runtime.
// It keeps the source evidence untouched so the test observes only the SM-2
// current-binding resolution written by semantic contract sealing.
func resealSemanticPlanningGenerationForLineageTest(t *testing.T, controller *RuntimeController, aggregate RuntimeAggregate, catalog runtimecontract.RuntimeOperationAdmissibility, observation runtimecontract.RuntimePrerequisiteObservation, target string) RuntimeAggregate {
	t.Helper()
	if controller == nil || aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || len(aggregate.Facts.Focus.CoverageWorkItemIDs) != 1 {
		t.Fatalf("semantic re-seal requires one pending current focus: %+v", aggregate)
	}
	catalog = catalog.Normalized()
	observation = observation.Normalized()
	coverageID := aggregate.Facts.Focus.CoverageWorkItemIDs[0]
	manifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		aggregate.Definition,
		aggregate.Facts.DefinitionSeed,
		aggregate.Facts.Activation.ExecutionContract,
		aggregate.Facts.Resources.OperationAdmissibilities,
		runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts),
	)
	if err != nil {
		t.Fatalf("compile new semantic manifest: %v", err)
	}
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID:    coverageID,
		OwnerModuleID:         runtimecontract.RuntimeModuleFile,
		OperationKind:         runtimecontract.StepKindFileWrite,
		Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{target}},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:        "semantic/" + string(coverageID),
	}
	order := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion:      runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:       manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep:           runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder:      []common.CoverageWorkItemID{coverageID},
	}
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatalf("new semantic coverage order: %v", err)
	}
	apply := func(event RuntimeInternalEvent) {
		t.Helper()
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{
		Kind:             RuntimeInternalEventSemanticPlanRequestStarted,
		SemanticManifest: &manifest,
		StablePrefixHash: "semantic-lineage-reseal-prefix",
		LogicalRequestID: "semantic-lineage-reseal-order",
	})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &order, LogicalRequestID: "semantic-lineage-reseal-order"})
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest,
		order,
		0,
		"semantic-lineage-reseal-batch",
		aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs,
		1,
	)
	if err != nil {
		t.Fatalf("compile re-seal semantic batch: %v", err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatalf("persist re-seal semantic batch: %v", err)
	}
	proof := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion:            runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:             manifest.Hash,
		DefinitionSeedHash:       manifest.DefinitionSeedHash,
		PlanStep:                 runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID:                  batch.BatchID,
		BatchSequence:            batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
		Proposals:                []runtimecontract.RuntimeSemanticProofProposal{proposal},
	}
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatalf("new semantic proof batch: %v", err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	feasibility := runtimecontract.RuntimeSemanticProofFeasibility{
		SchemaVersion:        runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
		ManifestHash:         manifest.Hash,
		DefinitionSeedHash:   manifest.DefinitionSeedHash,
		CoverageWorkItemID:   coverageID,
		ProposalHash:         runtimecontract.StableRuntimeSemanticProofProposalHash(proposal),
		OwnerModuleID:        proposal.OwnerModuleID,
		CatalogSnapshotID:    catalog.SnapshotID,
		CatalogObservationID: observation.ObservationID,
		Status:               runtimecontract.RuntimeSemanticFeasibilityFeasible,
		ObservedRevision:     catalog.ObservedRevision,
		TargetRefs:           []string{target},
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility})
	accepted := aggregate.SemanticPlanning.Batches[0]
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, accepted, proof)
	if err != nil {
		t.Fatalf("merge re-seal semantic proof batch: %v", err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, SemanticPlanMergedDocument: &merged})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	return aggregate
}

func newSemanticPlanningController(t *testing.T) (*RuntimeController, *controllerRepository, RuntimeAggregate) {
	t.Helper()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return controller, repository, aggregate
}

func semanticPlanningFixture(t *testing.T, aggregate RuntimeAggregate) (runtimecontract.RuntimeSemanticPlanManifest, runtimecontract.RuntimeSemanticPlanDocument, runtimecontract.RuntimeSemanticProofFeasibility) {
	t.Helper()
	manifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		aggregate.Definition,
		aggregate.Facts.DefinitionSeed,
		aggregate.Facts.Activation.ExecutionContract,
		aggregate.Facts.Resources.OperationAdmissibilities,
		runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts),
	)
	if err != nil {
		t.Fatal(err)
	}
	catalog := manifest.OwnerCatalogs[0]
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-v2", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite,
		Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"target.txt"}},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef: "semantic/cwi-v2",
	}
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID: "batch-fixture", BatchSequence: 0,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{proposal.CoverageWorkItemID},
		Proposals:                []runtimecontract.RuntimeSemanticProofProposal{proposal},
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	feasibility := runtimecontract.RuntimeSemanticProofFeasibility{
		SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID,
		ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID,
		CatalogSnapshotID: catalog.SnapshotID, Status: runtimecontract.RuntimeSemanticFeasibilityFeasible, ObservedRevision: catalog.ObservedRevision, TargetRefs: []string{"target.txt"},
	}
	return manifest, document, feasibility
}

func semanticPlanningDiagnostic() runtimecontract.RuntimeOriginalDiagnostic {
	return runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
		OriginalClass: "contract", OriginalCode: "semantic_plan_rejected", OriginalMessage: "proposal is outside the manifest", Signature: "semantic-plan/rejected-1",
	}
}

func TestRuntimeCapabilitiesWithSemanticProofsAddsValidationOperation(t *testing.T) {
	focus := runtimecontract.CoverageFocus{CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{{Proofs: []runtimecontract.CoverageEvidenceProof{{EvidenceKind: runtimecontract.EvidenceKindValidation, AllowedOperationKinds: []string{runtimecontract.StepKindMCPToolCall}}}}}}
	got := runtimeCapabilitiesWithSemanticProofs(runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}}, []runtimecontract.CoverageFocus{focus})
	if !containsString(got.ValidationKinds, runtimecontract.StepKindMCPToolCall) || !containsString(got.ValidationKinds, runtimecontract.RuntimeBlockKindManagedValidationSession) {
		t.Fatalf("semantic validation capabilities = %+v", got.ValidationKinds)
	}
}

func semanticPlanningCorrectionDirective(manifest runtimecontract.RuntimeSemanticPlanManifest, prefix string, step runtimecontract.RuntimeSemanticPlanStep, rejection runtimecontract.RuntimeOriginalDiagnostic) runtimecontract.RuntimeSemanticCorrectionDirective {
	directive := runtimecontract.RuntimeSemanticCorrectionDirective{
		SchemaVersion: runtimecontract.RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      step, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		StablePrefixHash: prefix, SourceRejectionHash: runtimecontract.StableRuntimeSemanticPlanCorrectionHash(rejection), Attempt: 1,
		Violations: append([]runtimecontract.RuntimeSemanticContractViolation(nil), rejection.SemanticViolations...),
	}
	directive.Hash = runtimecontract.StableRuntimeSemanticCorrectionDirectiveHash(directive)
	return directive
}

func sameOrderedSemanticCoverageIDs(left, right []common.CoverageWorkItemID) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

// runtimeTestAdvanceSemanticFocus models the Orchestrator-owned SM-4 evidence
// commit that closes the current focus and opens the next focus-local semantic
// transaction.  It intentionally uses only typed aggregate facts; no provider
// document or capability-owner state is used to choose the next focus.
func runtimeTestAdvanceSemanticFocus(t *testing.T, repository *controllerRepository, aggregate *RuntimeAggregate) error {
	t.Helper()
	if repository == nil || aggregate == nil {
		return common.ErrInvalidArgument
	}
	if len(aggregate.Facts.Focus.CoverageWorkItemIDs) == 0 || len(aggregate.Facts.Focus.CoverageEvidenceBindings) == 0 {
		return fmt.Errorf("semantic focus fixture has no current binding")
	}
	focus := aggregate.Facts.Focus.Normalized()
	bindingRefs := make([]string, 0, len(focus.CoverageEvidenceBindings))
	for _, binding := range focus.CoverageEvidenceBindings {
		bindingRefs = append(bindingRefs, binding.ID)
	}
	evidenceID := common.EvidenceRecordID("semantic-focus-advance")
	fact := runtimecontract.EvidenceFact{
		ID: evidenceID, Kind: runtimecontract.EvidenceKindStateChange,
		SubjectRef: "semantic-test-operation", AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
		DispatchID: "semantic-test-dispatch", OperationID: "semantic-test-operation",
		Revision: nextEvidenceRevision(aggregate.EvidenceSequence + 1), Summary: "semantic focus proof committed",
		TargetRefs: append([]string(nil), focus.TargetRefs...), BindingRefs: bindingRefs,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), focus.CoverageWorkItemIDs...),
		Quality:             common.EvidenceQualityValid,
	}
	aggregate.Facts.Evidence.Accepted = append(aggregate.Facts.Evidence.Accepted, fact)
	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	for idx := range aggregate.Facts.Coverage {
		for _, coverageID := range focus.CoverageWorkItemIDs {
			if aggregate.Facts.Coverage[idx].ID == coverageID {
				aggregate.Facts.Coverage[idx].State = runtimecontract.RuntimeCoverageItemStateSatisfied
			}
		}
	}
	advanced, err := advanceRuntimeCWIFocus(aggregate.Facts)
	if err != nil {
		return err
	}
	aggregate.Facts = advanced
	if err := prepareSemanticPlanningForAdvancedFocus(aggregate); err != nil {
		return err
	}
	previousSnapshot := aggregate.Decision.AuthoritySnapshot
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &previousSnapshot)
	if err != nil {
		return err
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	expectedRevision := aggregate.StateRevision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return err
	}
	if err := repository.Save(context.Background(), *aggregate, expectedRevision); err != nil {
		return err
	}
	return nil
}
