package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/requestcontent"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
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

func TestSemanticPlanActiveCoverageFairlyDrainsLargeProviderOrder(t *testing.T) {
	const coverageCount = 64
	providerOrder := make([]common.CoverageWorkItemID, 0, coverageCount)
	for index := 0; index < coverageCount; index++ {
		prefix := "z"
		if index%2 != 0 {
			prefix = "a"
		}
		providerOrder = append(providerOrder, common.CoverageWorkItemID(fmt.Sprintf("cwi-%s-%02d", prefix, index)))
	}
	order := runtimecontract.RuntimeSemanticPlanDocument{
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: append([]common.CoverageWorkItemID(nil), providerOrder...),
	}
	facts := runtimecontract.RuntimeFacts{Focus: runtimecontract.CoverageFocus{
		ID: "focus-current", CoverageWorkItemIDs: []common.CoverageWorkItemID{providerOrder[17]},
	}}
	if active := SemanticPlanActiveCoverageIDsForDrive(facts, order, providerOrder); len(active) != 1 || active[0] != providerOrder[17] {
		t.Fatalf("current focus lost its authority fence: active=%v", active)
	}

	// Once the execution focus is no longer in pending semantic lineage, the
	// fallback cursor must take the first remaining provider-authored identity.
	// Repeating that projection proves every item advances exactly once without
	// lexical-ID sorting or starvation.
	facts.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-retired-current"}
	pending := append([]common.CoverageWorkItemID(nil), providerOrder...)
	for index, want := range providerOrder {
		active := SemanticPlanActiveCoverageIDsForDrive(facts, order, pending)
		if len(active) != 1 || active[0] != want {
			t.Fatalf("provider-order cursor %d = %v, want %s", index, active, want)
		}
		pending = pending[1:]
	}
	if active := SemanticPlanActiveCoverageIDsForDrive(facts, order, pending); len(active) != 0 {
		t.Fatalf("drained provider order retained active coverage: %v", active)
	}
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
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
		return aggregate
	}

	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractRequired, Authority: &RuntimeInternalAuthorityPayload{
		// initialize only; duplicate required is read-only.
	}})
	if repository.saves != 1 {
		t.Fatalf("required duplicate saves = %d", repository.saves)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"}})
	requestRevision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"}})
	if aggregate.StateRevision != requestRevision {
		t.Fatal("duplicate request changed aggregate revision")
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "semantic-request-1"}})
	batch, err := semanticplan.CompileFocusBatch(
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	batchID := aggregate.SemanticPlanning.FeasibilityBatchID
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: batchID, SemanticFeasibility: &feasibility}})
	observationRevision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: batchID, SemanticFeasibility: &feasibility}})
	if aggregate.StateRevision != observationRevision {
		t.Fatal("duplicate owner observation changed aggregate revision")
	}
	savesBeforeSeal := repository.saves
	acceptedBatch := aggregate.SemanticPlanning.Batches[0]
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, acceptedBatch, proof)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &merged}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
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
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "batch-prefix", LogicalRequestID: "order-request"}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "order-request"}})
	batch, err := semanticplan.CompileFocusBatch(
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID}})
	revision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID}})
	if aggregate.StateRevision != revision || repository.saves == 0 {
		t.Fatalf("replayed batch document changed durable state: revision=%d saves=%d", aggregate.StateRevision, repository.saves)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility}})
	accepted := aggregate.SemanticPlanning.Batches[0]
	batchResult := runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{
		BatchID: accepted.BatchID, BatchHash: accepted.Hash,
		DocumentHash:  accepted.DocumentHash,
		Feasibilities: []runtimecontract.RuntimeSemanticProofFeasibility{feasibility},
	}
	batchResult.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(batchResult)
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchFeasibilityObserved, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanBatchFeasibilityResult: &batchResult}})
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
			Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &merged},
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &merged}})
	if aggregate.StateRevision != mergeRevision {
		t.Fatal("replayed merged semantic document changed durable state")
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
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
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "batch-correction-prefix", LogicalRequestID: "order-request"}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "order-request"}})
	batch, err := semanticplan.CompileFocusBatch(
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
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "batch-scope-prefix", LogicalRequestID: "order-request"}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "order-request"}})
	batch, err := semanticplan.CompileFocusBatch(
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
		Kind: RuntimeInternalEventSemanticPlanEvaluated, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &document, LogicalRequestID: "missing-request"},
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("document without request error = %v", err)
	}
	manifest.DefinitionSeedHash = "stale-seed"
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	if _, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanRequestStarted, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"},
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale manifest error = %v", err)
	}
	manifest, document, _ = semanticPlanningFixture(t, aggregate)

	// A correction must be tied to an already-issued proof batch. The previous
	// fixture manufactured a prerequisite CWI in the manifest, which is now
	// correctly rejected because it does not exist in the Definition seed.
	apply := func(event RuntimeInternalEvent) RuntimeAggregate {
		t.Helper()
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(ctx, event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
		return aggregate
	}
	apply(RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"},
	})
	order := document.Clone()
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID = ""
	order.BatchSequence = 0
	order.BatchCoverageWorkItemIDs = nil
	order.CoverageOrder = []common.CoverageWorkItemID{manifest.CoverageItems[0].ID}
	order.Proposals = nil
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "semantic-request-1"}})
	batch, err := semanticplan.CompileFocusBatch(
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
		Kind: RuntimeInternalEventSemanticPlanCorrectionRequired, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &RuntimeInternalTransactionPayload{StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-2", ExpectedDocument: &expectedCorrection, SemanticDiagnostic: &rejection},
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate = corrected.Aggregate
	if corrected.Aggregate.SemanticPlanning.Attempt != 1 || len(corrected.Aggregate.SemanticPlanning.Rejections) != 1 || corrected.Aggregate.StateRevision != beforeCorrection+1 {
		t.Fatalf("correction lineage = %+v", corrected.Aggregate.SemanticPlanning)
	}
	replayed, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanCorrectionRequired, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &RuntimeInternalTransactionPayload{StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-2", ExpectedDocument: &expectedCorrection, SemanticDiagnostic: &rejection},
	})
	if err != nil || replayed.Aggregate.StateRevision != corrected.Aggregate.StateRevision {
		t.Fatalf("duplicate correction changed state: revision=%d err=%v", replayed.Aggregate.StateRevision, err)
	}
	recovered, err := controller.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || recovered.SemanticPlanning.CorrectionDirective == nil || recovered.SemanticPlanning.CorrectionDirective.Hash != directive.Hash {
		t.Fatalf("persisted correction did not survive reload: %+v err=%v", recovered.SemanticPlanning.CorrectionDirective, err)
	}
	if _, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanCorrectionRequired, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &RuntimeInternalTransactionPayload{StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-3", ExpectedDocument: &expectedCorrection, SemanticDiagnostic: &rejection},
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("third semantic request error = %v", err)
	}
	correctedProof := document.Clone()
	correctedProof.BatchID = batch.BatchID
	correctedProof.BatchSequence = batch.Sequence
	correctedProof.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
	accepted, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanEvaluated, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &RuntimeInternalTransactionPayload{LogicalRequestID: "semantic-request-2", SemanticDocument: &correctedProof},
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

func TestRuntimeSemanticFeasibilityRejectsProviderCorrectionAfterOwnerFactsBegin(t *testing.T) {
	controller, repository, aggregate := newSemanticPlanningController(t)
	manifest, proof, _ := semanticPlanningFixture(t, aggregate)
	ctx := context.Background()
	apply := func(event RuntimeInternalEvent) RuntimeAggregate {
		t.Helper()
		event.Scope = RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}
		result, err := controller.ApplyInternalEvent(ctx, event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
		return aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{
		SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-feasibility", LogicalRequestID: "semantic-order-request",
	}})
	order := proof.Clone()
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID = ""
	order.BatchSequence = 0
	order.BatchCoverageWorkItemIDs = nil
	order.CoverageOrder = []common.CoverageWorkItemID{manifest.CoverageItems[0].ID}
	order.Proposals = nil
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{
		SemanticDocument: &order, LogicalRequestID: "semantic-order-request",
	}})
	batch, err := semanticplan.CompileFocusBatch(
		manifest, order, 0, "semantic-proof-request", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	proof.BatchID = batch.BatchID
	proof.BatchSequence = batch.Sequence
	proof.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{
		SemanticDocument: &proof, LogicalRequestID: "semantic-proof-request",
	}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFeasibilityRequired {
		t.Fatalf("phase = %q, want feasibility required", aggregate.SemanticPlanning.Phase)
	}
	beforeRevision := aggregate.StateRevision
	beforeAttempts := len(aggregate.CandidateAttempts)
	rejection := semanticPlanningDiagnostic()
	rejection.OriginalCode = "semantic_owner_proposal_constraint"
	rejection.OriginalMessage = "proposal omitted its required durable effect scope"
	rejection.SemanticViolations = []runtimecontract.RuntimeSemanticContractViolation{{
		Kind: runtimecontract.RuntimeSemanticContractViolationProposalConstraint,
		Path: "$.proposals[0].requiredEffectScopes", CoverageWorkItemID: manifest.CoverageItems[0].ID,
		ProposalIndex: 0, Rule: runtimecontract.RuntimeSemanticConstraintRelational,
		Actual: "empty_array", Required: []string{string(runtimecontract.RuntimeEffectWorkspaceDurable)},
	}}
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan, PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, CorrectionAttempt: 1,
	}
	event := RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanCorrectionRequired, Scope: RuntimeInternalEventScope{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
	}, Transaction: &RuntimeInternalTransactionPayload{
		StablePrefixHash: "semantic-prefix-feasibility", LogicalRequestID: "semantic-proof-correction",
		ExpectedDocument: &expected, SemanticDiagnostic: &rejection,
	}}
	if _, err := controller.ApplyInternalEvent(ctx, event); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("feasibility-stage provider correction error = %v, want conflict", err)
	}
	reloaded, err := repository.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	ledger := reloaded.SemanticPlanning
	active, found := runtimeSemanticBatch(ledger.Batches, ledger.ActiveBatchID)
	if !found || ledger.Phase != runtimecontract.RuntimeSemanticPlanningFeasibilityRequired || ledger.Attempt != 0 ||
		active.State != runtimecontract.RuntimeSemanticPlanBatchFeasibilityPending || active.DocumentHash == "" ||
		len(ledger.BatchDocuments) != 1 || ledger.FeasibilityBatchID == "" || len(ledger.Feasibilities) != 0 || ledger.FeasibilitySetHash != "" ||
		len(reloaded.CandidateAttempts) != beforeAttempts || reloaded.ActiveDispatch != nil || reloaded.StateRevision != beforeRevision {
		t.Fatalf("rejected feasibility correction changed owner state: ledger=%+v batch=%+v", ledger, active)
	}
}

func TestRuntimeSemanticCoverageOrderCorrectionPersistsAndReplaysIdempotently(t *testing.T) {
	controller, _, aggregate := newSemanticPlanningController(t)
	manifest, _, _ := semanticPlanningFixture(t, aggregate)
	ctx := context.Background()
	started, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:  RuntimeInternalEventSemanticPlanRequestStarted,
		Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Transaction: &RuntimeInternalTransactionPayload{
			SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-coverage", LogicalRequestID: "semantic-coverage-request-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate = started.Aggregate
	required := make([]common.CoverageWorkItemID, 0, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		required = append(required, item.ID)
	}
	rejection := semanticPlanningDiagnostic()
	rejection.FieldViolations = []runtimecontract.RuntimeFieldViolation{{Path: "$.proposals", Actual: "missing_required"}}
	rejection.SemanticViolations = []runtimecontract.RuntimeSemanticContractViolation{{
		Kind: runtimecontract.RuntimeSemanticContractViolationCoverageOrderDocument, Path: "$.proposals", Actual: "missing_required",
		RequiredCoverageWorkItemIDs: required,
	}}
	directive := semanticPlanningCorrectionDirective(manifest, "semantic-prefix-coverage", runtimecontract.RuntimeSemanticPlanStepCoverageOrder, rejection)
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan, PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, CorrectionAttempt: 1,
	}
	event := RuntimeInternalEvent{
		Kind:  RuntimeInternalEventSemanticPlanCorrectionRequired,
		Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Transaction: &RuntimeInternalTransactionPayload{
			StablePrefixHash: "semantic-prefix-coverage", LogicalRequestID: "semantic-coverage-request-2",
			ExpectedDocument: &expected, SemanticDiagnostic: &rejection,
		},
	}
	corrected, err := controller.ApplyInternalEvent(ctx, event)
	if err != nil {
		t.Fatal(err)
	}
	if corrected.Aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningCorrectionRequired ||
		corrected.Aggregate.SemanticPlanning.CorrectionDirective == nil ||
		corrected.Aggregate.SemanticPlanning.CorrectionDirective.Hash != directive.Hash {
		t.Fatalf("coverage-order correction was not persisted: %+v", corrected.Aggregate.SemanticPlanning)
	}
	replayed, err := controller.ApplyInternalEvent(ctx, event)
	if err != nil || replayed.Aggregate.StateRevision != corrected.Aggregate.StateRevision {
		t.Fatalf("duplicate coverage-order correction changed state: revision=%d want=%d err=%v", replayed.Aggregate.StateRevision, corrected.Aggregate.StateRevision, err)
	}
	reloaded, err := controller.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || reloaded.SemanticPlanning.CorrectionDirective == nil || reloaded.SemanticPlanning.CorrectionDirective.Hash != directive.Hash {
		t.Fatalf("coverage-order correction did not survive reload: %+v err=%v", reloaded.SemanticPlanning.CorrectionDirective, err)
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
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatal(err)
		}
		aggregate = result.Aggregate
		return aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "semantic-request-1"}})
	batch, err := semanticplan.CompileFocusBatch(
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	ownerDiagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OriginalClass: "feasibility", OriginalCode: "target_outside_scope", OriginalMessage: "  target is outside owner scope\n", Signature: "file/target-outside-scope",
	}
	feasibility.Status = runtimecontract.RuntimeSemanticFeasibilityInfeasible
	feasibility.OriginalDiagnostic = &ownerDiagnostic
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility}})
	failure := semanticPlanningDiagnostic()
	failure.OriginalCode = "validation_recipe_exhausted"
	failure.OriginalMessage = "validation recipe cannot execute in workspace"
	failure.Signature = "shell/validation-recipe-exhausted"
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractFailed, Authority: &RuntimeInternalAuthorityPayload{
		SemanticFailureCode: "semantic_owner_feasibility_infeasible", SemanticDiagnostic: &failure,
	}})
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
		Kind: RuntimeInternalEventSemanticPlanRequestStarted, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"},
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
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
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
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: session.LogicalRequestID}})
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
	aggregate, err = controller.markCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, 1, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.completeCandidateRequestDocument(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: session.LogicalRequestID}})
	batch, err := semanticplan.CompileFocusBatch(
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	batchID := aggregate.SemanticPlanning.FeasibilityBatchID
	beforeObservationRevision := aggregate.StateRevision
	conflictRepository := &semanticFailOnceRepository{RuntimeStateRepository: repository, err: common.ErrConflict}
	conflictController, err := NewRuntimeController(conflictRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conflictController.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticFeasibilityObserved, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: batchID, SemanticFeasibility: &feasibility},
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
			Kind: RuntimeInternalEventSemanticFeasibilityObserved, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: batchID, SemanticFeasibility: &feasibility},
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: batchID, SemanticFeasibility: &feasibility}})
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
			Kind: RuntimeInternalEventSemanticContractRequired, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: batchID, SemanticRefreshGeneration: 0},
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractRequired, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: batchID, SemanticRefreshGeneration: 0}})
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
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
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
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, applyErr := controller.ApplyInternalEvent(ctx, event)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", event.Kind, applyErr)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "semantic-request-1"}})
	batch, err := semanticplan.CompileFocusBatch(
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: batchID, SemanticFeasibility: &feasibility}})
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFeasibilityRequired ||
		len(aggregate.SemanticPlanning.Feasibilities) != 1 ||
		aggregate.SemanticPlanning.Feasibilities[0].Status != runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired {
		t.Fatalf("prerequisite-required owner observation was not persisted: %+v", aggregate.SemanticPlanning)
	}
	if _, refreshErr := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticContractRequired, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: batchID, SemanticRefreshGeneration: 0},
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

func TestRuntimeCatalogStaleBindingRetirementPreemptsOperationAuthorityAcrossRestart(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate, catalog, prerequisite, observation := sealedSemanticPlanningAggregate(t)
	coverageID := aggregate.Facts.Coverage[0].ID
	oldBinding := aggregate.Facts.Coverage[0].CurrentBindingRef.Clone()
	oldFocusTargetRefs := append([]string(nil), aggregate.Facts.Focus.TargetRefs...)
	oldTargetSelection := aggregate.Facts.Resources.TargetSelection

	changed, err := applyReceiptResourceChangeInvalidation(&aggregate, "dispatch-owner-catalog-stale", "operation-owner-catalog-stale", runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "execution/owner-catalog-stale", Fence: 1, OperationID: "operation-owner-catalog-stale",
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", BeforeRevision: catalog.ObservedRevision, AfterRevision: "file-revision-stale-successor",
		}},
	})
	if err != nil || !changed || !semanticCatalogPrerequisitePending(aggregate, prerequisite.ID) {
		t.Fatalf("owner stale fact did not reopen catalog acquisition: changed=%t err=%v aggregate=%+v", changed, err, aggregate)
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

	freshCatalog := catalog.Normalized()
	freshCatalog.ObservedRevision = "file-revision-stale-successor"
	freshCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(freshCatalog)
	freshObservation := observation.Normalized()
	freshObservation.ContentHash = freshCatalog.SnapshotID
	freshObservation.Dependencies = append([]runtimecontract.RuntimeFreshnessDependency(nil), freshObservation.Dependencies...)
	freshObservation.Dependencies[0].Revision = freshCatalog.ObservedRevision
	freshObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(freshObservation)
	reacquired, err := controller.commitPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &freshCatalog, Observation: &freshObservation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reacquired.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired ||
		len(reacquired.SemanticPlanning.History) != 1 || reacquired.SemanticPlanning.Manifest != nil ||
		!RuntimeCWIBindingRetirementPending(reacquired) || SemanticOperationAuthorityReady(reacquired) {
		t.Fatalf("retired catalog generation remained usable before explicit binding retirement: %+v", reacquired.SemanticPlanning)
	}
	item := runtimeCoverageByID(reacquired.Facts.Coverage)[coverageID]
	if item.CurrentBindingRef == nil || !runtimecontract.SameRuntimeCWIBindingRef(*item.CurrentBindingRef, oldBinding) ||
		len(runtimecontract.RuntimeCWIsEligibleForPlanning(reacquired.Facts)) != 0 {
		t.Fatalf("catalog retirement implicitly rewrote its bound CWI: %+v", item)
	}

	savesBeforeRestart := repository.saves
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := restarted.Load(ctx, reacquired.Facts.WorkspaceID, reacquired.Facts.RunID)
	if err != nil || repository.saves != savesBeforeRestart || !RuntimeCWIBindingRetirementPending(reloaded) || SemanticOperationAuthorityReady(reloaded) {
		t.Fatalf("restart changed the pending stale-binding fence: aggregate=%+v saves=%d/%d err=%v", reloaded, repository.saves, savesBeforeRestart, err)
	}
	retirement, found := NextRuntimeCWIBindingRetirementFact(reloaded)
	if !found || !runtimecontract.SameRuntimeCWIBindingRef(retirement.CurrentBindingRef, oldBinding) {
		t.Fatalf("restart lost the typed owner-local binding retirement: fact=%+v found=%t", retirement, found)
	}
	committed, err := restarted.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:      RuntimeInternalEventCWIBindingRetired,
		Scope:     RuntimeInternalEventScope{WorkspaceID: reloaded.Facts.WorkspaceID, RunID: reloaded.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{CWIBindingRetirement: &retirement},
	})
	if err != nil {
		t.Fatal(err)
	}
	retired := committed.Aggregate
	item = runtimeCoverageByID(retired.Facts.Coverage)[coverageID]
	eligible := runtimecontract.RuntimeCWIsEligibleForPlanning(retired.Facts)
	if item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStatePlanning || item.CurrentBindingRef != nil ||
		len(item.BindingRetirementLineage) != 1 || len(eligible) != 1 || eligible[0] != item.ID ||
		RuntimeCWIBindingRetirementPending(retired) || SemanticOperationAuthorityReady(retired) ||
		retired.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || retired.SemanticPlanning.Manifest != nil ||
		len(retired.SemanticPlanning.History) != 1 || retired.ActiveDispatch != nil ||
		len(retired.Facts.Focus.CoverageEvidenceBindings) != 0 || !sameRuntimeTargetRefs(retired.Facts.Focus.TargetRefs, oldFocusTargetRefs) ||
		!reflect.DeepEqual(retired.Facts.Resources.TargetSelection, oldTargetSelection) {
		t.Fatalf("typed retirement did not reopen exactly one CWI without reseal/target synthesis: item=%+v eligible=%v aggregate=%+v", item, eligible, retired)
	}
	if err := retired.Validate(); err != nil {
		t.Fatalf("reopened stale-binding aggregate violates fresh-load invariant: %v", err)
	}

	revision, saves := retired.StateRevision, repository.saves
	duplicate, err := restarted.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:      RuntimeInternalEventCWIBindingRetired,
		Scope:     RuntimeInternalEventScope{WorkspaceID: retired.Facts.WorkspaceID, RunID: retired.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{CWIBindingRetirement: &retirement},
	})
	if err != nil || duplicate.Aggregate.StateRevision != revision || repository.saves != saves {
		t.Fatalf("duplicate stale-binding retirement changed authority: aggregate=%+v saves=%d/%d err=%v", duplicate.Aggregate, repository.saves, saves, err)
	}
}

func TestRuntimeSemanticCatalogReacquisitionKeepsAcceptedEffectBindingLocked(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate, catalog, prerequisite, observation := sealedSemanticPlanningAggregate(t)
	oldBinding := aggregate.Facts.Focus.CoverageEvidenceBindings[0].Clone()
	if err := runtimeTestCommitSemanticStateChangeReceipt(t, repository, &aggregate); err != nil {
		t.Fatal(err)
	}

	changed, err := applyReceiptResourceChangeInvalidation(&aggregate, "dispatch-lineage-lock", "operation-lineage-lock", runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "execution/lineage-lock", Fence: 1, OperationID: "operation-lineage-lock",
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", BeforeRevision: catalog.ObservedRevision, AfterRevision: "file-revision-2",
		}},
	})
	if err != nil || !changed || !semanticCatalogPrerequisitePending(aggregate, prerequisite.ID) {
		t.Fatalf("receipt invalidation did not open catalog reacquisition: changed=%t err=%v", changed, err)
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil, aggregate.CoverageProofLineage)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	aggregate.StateRevision++
	repository.aggregate = &aggregate

	currentCatalog := catalog.Normalized()
	currentCatalog.TargetRefs = []string{"other.txt"}
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
	item := runtimeCoverageByID(reacquired.Facts.Coverage)[oldBinding.CoverageWorkItemID]
	if item.CurrentBindingRef == nil || item.CurrentBindingRef.BindingID != oldBinding.ID || len(item.BindingRetirementLineage) != 0 {
		t.Fatalf("accepted-effect binding was replaced during reacquisition: %+v", item)
	}
	if got := reacquired.Facts.Evidence.Accepted[0].BindingRefs; len(got) != 1 || got[0] != oldBinding.ID {
		t.Fatalf("reacquisition rewrote source evidence: %v", got)
	}
	if eligible := runtimecontract.RuntimeCWIsEligibleForPlanning(reacquired.Facts); len(eligible) != 0 {
		t.Fatalf("accepted-effect CWI became eligible for implicit replan: %v", eligible)
	}
	if retirement, found := NextRuntimeCWIBindingRetirementFact(reacquired); found {
		t.Fatalf("accepted-effect binding produced retirement fact: %+v", retirement)
	}
}

func TestSemanticOperationAuthorityKeepsLockedCurrentCWIAheadOfLaterPlanning(t *testing.T) {
	_, repository, aggregate, _, _, _ := sealedSemanticPlanningAggregate(t)
	if err := runtimeTestCommitSemanticStateChangeReceipt(t, repository, &aggregate); err != nil {
		t.Fatal(err)
	}
	aggregate.Facts.Coverage = append(aggregate.Facts.Coverage, runtimecontract.RuntimeCoverageItemFact{
		ID: "cwi-later", SliceID: "slice-later",
		State:               runtimecontract.RuntimeCoverageItemStateOpen,
		CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStateUnbound,
	})
	aggregate.Facts.DefinitionSeed.CoverageItems = append(aggregate.Facts.DefinitionSeed.CoverageItems, runtimecontract.RuntimeCoverageItem{
		ID: "cwi-later", SliceID: "slice-later", Order: len(aggregate.Facts.DefinitionSeed.CoverageItems) + 1,
	})
	aggregate.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningRequired

	if eligible := runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts); len(eligible) != 1 || eligible[0] != "cwi-later" {
		t.Fatalf("later planning eligibility = %v", eligible)
	}
	if !SemanticOperationAuthorityReady(aggregate) {
		t.Fatal("later eligible CWI preempted the execution-locked current binding")
	}

	aggregate.Facts.Evidence.Accepted = nil
	aggregate.CompletedDispatches = nil
	aggregate.OperationReceipts = nil
	if SemanticOperationAuthorityReady(aggregate) {
		t.Fatal("unlocked current binding bypassed required semantic planning")
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
		Kind: RuntimeInternalEventSemanticCatalogInvalidated, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{ManifestHash: "semantic-manifest-unknown", LogicalRequestID: aggregate.SemanticPlanning.LogicalRequestID},
	}); err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unknown retired semantic manifest was accepted: %v", err)
	}
	if repository.aggregate.StateRevision != beforeRevision {
		t.Fatalf("rejected semantic invalidation changed state revision: before=%d after=%d", beforeRevision, repository.aggregate.StateRevision)
	}
	if _, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticCatalogInvalidated, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{ManifestHash: aggregate.SemanticPlanning.Manifest.Hash, LogicalRequestID: "semantic-request-from-another-generation"},
	}); err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("semantic invalidation with a mismatched logical generation was accepted: %v", err)
	}
	if repository.aggregate.StateRevision != beforeRevision {
		t.Fatalf("mismatched semantic invalidation changed state revision: before=%d after=%d", beforeRevision, repository.aggregate.StateRevision)
	}
	retired, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticCatalogInvalidated, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{ManifestHash: aggregate.SemanticPlanning.Manifest.Hash, LogicalRequestID: aggregate.SemanticPlanning.LogicalRequestID},
	})
	if err != nil {
		t.Fatal(err)
	}
	revision := retired.Aggregate.StateRevision
	duplicate, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticCatalogInvalidated, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{ManifestHash: aggregate.SemanticPlanning.Manifest.Hash, LogicalRequestID: aggregate.SemanticPlanning.LogicalRequestID},
	})
	if err != nil || duplicate.Aggregate.StateRevision != revision {
		t.Fatalf("matching semantic invalidation was not idempotent: err=%v duplicate=%+v", err, duplicate.Aggregate)
	}
}

func TestRuntimeSemanticCatalogInvalidationRequiresSeparateNoEffectBindingRetirement(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate, _, _, _ := sealedSemanticPlanningAggregate(t)
	coverageID := aggregate.Facts.Coverage[0].ID
	current := aggregate.Facts.Coverage[0].CurrentBindingRef.Clone()
	manifestHash := aggregate.SemanticPlanning.Manifest.Hash
	logicalRequestID := aggregate.SemanticPlanning.LogicalRequestID
	aggregate.Facts.Resources.PrerequisiteObservations = nil
	repository.aggregate = &aggregate

	retired, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticCatalogInvalidated, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{ManifestHash: manifestHash, LogicalRequestID: logicalRequestID},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := runtimeCoverageByID(retired.Aggregate.Facts.Coverage)[coverageID]
	if item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound || item.CurrentBindingRef == nil ||
		!runtimecontract.SameRuntimeCWIBindingRef(*item.CurrentBindingRef, current) || len(item.BindingRetirementLineage) != 0 {
		t.Fatalf("catalog invalidation mutated canonical binding: %+v", item)
	}
	fact, found := NextRuntimeCWIBindingRetirementFact(retired.Aggregate)
	if !found || fact.CoverageWorkItemID != coverageID || !runtimecontract.SameRuntimeCWIBindingRef(fact.CurrentBindingRef, current) {
		t.Fatalf("no-effect binding retirement fact = %+v, found=%v", fact, found)
	}
	retirement, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventCWIBindingRetired, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{CWIBindingRetirement: &fact},
	})
	if err != nil {
		t.Fatal(err)
	}
	item = runtimeCoverageByID(retirement.Aggregate.Facts.Coverage)[coverageID]
	if item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStatePlanning || item.CurrentBindingRef != nil ||
		len(item.BindingRetirementLineage) != 1 || item.BindingRetirementLineage[0].CausationRef != fact.CausationRef {
		t.Fatalf("explicit binding retirement did not update exact CWI: %+v", item)
	}
	if retirement.Aggregate.Facts.Focus.ID != "semantic-pending/"+string(coverageID) || len(retirement.Aggregate.Facts.Focus.CoverageEvidenceBindings) != 0 {
		t.Fatalf("explicit binding retirement focus = %+v", retirement.Aggregate.Facts.Focus)
	}
	revision := retirement.Aggregate.StateRevision
	duplicate, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventCWIBindingRetired, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{CWIBindingRetirement: &fact},
	})
	if err != nil || duplicate.Aggregate.StateRevision != revision {
		t.Fatalf("duplicate binding retirement changed revision: err=%v aggregate=%+v", err, duplicate.Aggregate)
	}
}

func TestRuntimeCWIBindingRetirementSupersedesReconciliationBeforeRestartScheduling(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate, _, _, _ := sealedSemanticPlanningAggregate(t)
	contract := aggregate.Facts.ReconciliationContract.Clone()
	request := runtimecontract.RuntimeCWIReconciliationRequest{
		RequestID:                 "reconcile-before-binding-retirement",
		WorkspaceID:               aggregate.Facts.WorkspaceID,
		RunID:                     aggregate.Facts.RunID,
		Scope:                     runtimecontract.RuntimeCWIReconciliationScopeCurrentRun,
		DefinitionRevision:        contract.DefinitionRevision,
		BindingSetHash:            contract.BindingSetHash,
		Bindings:                  contract.Bindings,
		WorkspaceBaselineRevision: "tree-before-binding-retirement",
		WorkspaceBaselineRef:      "workspace-tree/tree-before-binding-retirement",
		Generation:                1,
	}
	sealed, changed, err := controller.ensureReconciliationRequest(ctx, request)
	if err != nil || !changed {
		t.Fatalf("seal reconciliation before binding retirement = changed=%v err=%v", changed, err)
	}
	batch := reconciliationBatchForRequest(t, request, runtimecontract.RuntimeCWIReconciliationObservationSatisfied)
	sealed.Reconciliation.Batch = &batch
	for _, binding := range request.Bindings {
		observations := make([]runtimecontract.RuntimeCWIReconciliationObservation, 0, len(binding.OwnerModuleIDs))
		for _, observation := range batch.Observations {
			if observation.CoverageWorkItemID == binding.CoverageWorkItemID && observation.BindingID == binding.BindingID {
				observations = append(observations, observation)
			}
		}
		sealed.Reconciliation.Results = append(sealed.Reconciliation.Results, runtimecontract.RuntimeCWIReconciliationResult{
			RequestID: request.RequestID, WorkspaceID: request.WorkspaceID, RunID: request.RunID,
			Generation: request.Generation, BatchHash: batch.Hash, CoverageItemID: binding.CoverageWorkItemID,
			BindingID: binding.BindingID, Disposition: runtimecontract.RuntimeCWIReconciliationDispositionPrerequisite,
			ObservationRef: runtimecontract.StableRuntimeCWIReconciliationObservationSetRef(observations),
		})
	}
	sealed.Reconciliation = sealed.Reconciliation.Normalized()
	if err := sealed.Reconciliation.Validate(); err != nil {
		t.Fatalf("complete reconciliation fixture: %v", err)
	}
	manifestHash := sealed.SemanticPlanning.Manifest.Hash
	logicalRequestID := sealed.SemanticPlanning.LogicalRequestID
	sealed.Facts.Resources.PrerequisiteObservations = nil
	repository.aggregate = &sealed

	invalidated, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticCatalogInvalidated, Scope: RuntimeInternalEventScope{WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{ManifestHash: manifestHash, LogicalRequestID: logicalRequestID},
	})
	if err != nil {
		t.Fatal(err)
	}
	fact, found := NextRuntimeCWIBindingRetirementFact(invalidated.Aggregate)
	if !found {
		t.Fatal("catalog invalidation did not schedule the explicit no-effect binding retirement")
	}
	retired, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventCWIBindingRetired, Scope: RuntimeInternalEventScope{WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{CWIBindingRetirement: &fact},
	})
	if err != nil {
		t.Fatal(err)
	}
	if retired.Aggregate.Reconciliation.Request != nil || retired.Aggregate.Reconciliation.Batch != nil || retired.Aggregate.Reconciliation.Results != nil {
		t.Fatalf("binding retirement retained active reconciliation transport: %+v", retired.Aggregate.Reconciliation)
	}
	if len(retired.Aggregate.Reconciliation.History) != 1 || !retired.Aggregate.Reconciliation.History[0].Superseded ||
		retired.Aggregate.Reconciliation.History[0].Request.RequestID != request.RequestID ||
		retired.Aggregate.Reconciliation.History[0].Batch == nil ||
		retired.Aggregate.Reconciliation.History[0].Batch.Hash != batch.Hash ||
		len(retired.Aggregate.Reconciliation.History[0].Results) != len(request.Bindings) {
		t.Fatalf("binding retirement did not archive the old request in the same CAS: %+v", retired.Aggregate.Reconciliation.History)
	}
	retirementRevision := retired.Aggregate.StateRevision
	retirementSaves := repository.saves

	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(ctx, sealed.Facts.WorkspaceID, sealed.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StateRevision != retirementRevision || loaded.Reconciliation.Request != nil || repository.saves != retirementSaves {
		t.Fatalf("restart changed or resurrected superseded reconciliation: aggregate=%+v saves=%d/%d", loaded.Reconciliation, repository.saves, retirementSaves)
	}
	if _, changed, err := restarted.ensureReconciliationRequest(ctx, request); !errors.Is(err, common.ErrConflict) || changed {
		t.Fatalf("restart reactivated superseded request: changed=%v err=%v", changed, err)
	}

	if len(loaded.Facts.ReconciliationContract.Bindings) != 0 || loaded.Facts.Coverage[0].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStatePlanning {
		t.Fatalf("restart made an unsealed binding set eligible for reconciliation: facts=%+v", loaded.Facts)
	}
}

func TestRuntimeSemanticCatalogInvalidationCannotRetireEffectBearingBinding(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate, _, _, _ := sealedSemanticPlanningAggregate(t)
	coverageID := aggregate.Facts.Coverage[0].ID
	binding := aggregate.Facts.Coverage[0].CurrentBindingRef.Clone()
	if err := runtimeTestCommitSemanticStateChangeReceipt(t, repository, &aggregate); err != nil {
		t.Fatal(err)
	}
	manifestHash := aggregate.SemanticPlanning.Manifest.Hash
	logicalRequestID := aggregate.SemanticPlanning.LogicalRequestID
	aggregate.Facts.Resources.PrerequisiteObservations = nil
	repository.aggregate = &aggregate

	retired, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticCatalogInvalidated, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{ManifestHash: manifestHash, LogicalRequestID: logicalRequestID},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := runtimeCoverageByID(retired.Aggregate.Facts.Coverage)[coverageID]
	if item.CurrentBindingRef == nil || !runtimecontract.SameRuntimeCWIBindingRef(*item.CurrentBindingRef, binding) {
		t.Fatalf("effect-bearing binding changed during catalog invalidation: %+v", item)
	}
	if fact, found := NextRuntimeCWIBindingRetirementFact(retired.Aggregate); found {
		t.Fatalf("effect-bearing binding produced retirement fact: %+v", fact)
	}
	direct := RuntimeCWIBindingRetirementFact{
		CoverageWorkItemID: coverageID, CurrentBindingRef: binding,
		CausationRef: "forced-retirement", Reason: "must be rejected", ExpectedStateRevision: retired.Aggregate.StateRevision,
	}
	beforeRevision := repository.aggregate.StateRevision
	if _, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventCWIBindingRetired, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{CWIBindingRetirement: &direct},
	}); err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("effect-bearing binding retirement error = %v", err)
	}
	if repository.aggregate.StateRevision != beforeRevision {
		t.Fatalf("rejected effect-bearing retirement changed revision: before=%d after=%d", beforeRevision, repository.aggregate.StateRevision)
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

func TestRuntimeSemanticRetirementClearsPriorProviderDocumentFraming(t *testing.T) {
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
	outcome := runtimecontract.RuntimeProviderDocumentFraming{
		Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: expected.Hash,
		ResponseHash: "provider-response-old", ResponseBytes: 128, Code: "max_output_tokens", Signature: "provider/incomplete",
	}
	aggregate.CandidateRequestSessions = []runtimecontract.RuntimeCandidateRequestSession{{
		LogicalRequestID: expected.LogicalRequestID, DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan,
		ExpectedDocument: &expected, Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-old"},
		StablePrefixHash: expected.StablePrefixHash, ManifestHash: expected.ManifestHash,
		State: runtimecontract.RuntimeCandidateRequestSessionRecoveryPending, NextSequence: 2, IncompleteOutcomes: 1,
		LastDocumentFraming: &outcome, LastOutcome: runtimecontract.RuntimeCandidateRequestOutcome{
			Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentIncomplete, Code: outcome.Code, Signature: outcome.Signature,
		},
	}}
	if err := retireSemanticPlanningGeneration(&aggregate, runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated, []RuntimeSemanticCatalogFreshnessGap{{PrerequisiteID: prerequisite.ID}}); err != nil {
		t.Fatal(err)
	}
	if got := aggregate.CandidateRequestSessions[0].LastDocumentFraming; got != nil {
		t.Fatalf("stale retirement retained live provider document framing: %+v", got)
	}
	if aggregate.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
		aggregate.CandidateRequestSessions[0].LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale {
		t.Fatalf("retirement did not close the old provider session: %+v", aggregate.CandidateRequestSessions[0])
	}
}

func TestRuntimeSemanticRetirementRejectsUnsettledSubmittedRequestWithoutMutation(t *testing.T) {
	_, _, aggregate, _, prerequisite, _ := sealedSemanticPlanningAggregate(t)
	ledger := aggregate.SemanticPlanning.Clone()
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:  runtimecontract.RuntimeProviderDocumentSemanticPlan,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepProofBatch,
		ManifestHash:  ledger.Manifest.Hash, DefinitionSeedHash: ledger.DefinitionSeed.Hash,
		LogicalRequestID: "submitted-semantic-request", StablePrefixHash: "submitted-semantic-prefix",
		BatchID: ledger.Document.BatchID, BatchSequence: ledger.Document.BatchSequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), ledger.Document.BatchCoverageWorkItemIDs...),
		CoverageOrderHash:        runtimecontract.StableRuntimeSemanticPlanCoverageOrderHash(*ledger.CoverageOrderDocument),
		MaxOutputTokens:          1024, MaxRecoveryAttempts: 1,
	}
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(expected.LogicalRequestID, 1)
	session := runtimecontract.RuntimeCandidateRequestSession{
		LogicalRequestID: expected.LogicalRequestID, DocumentKind: expected.DocumentKind, ExpectedDocument: &expected,
		Transaction:      runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "submitted-semantic-authority"},
		StablePrefixHash: expected.StablePrefixHash, ManifestHash: expected.ManifestHash,
		State: runtimecontract.RuntimeCandidateRequestSessionPrepared, NextSequence: 1,
		LastPhysicalCallID: physicalCallID, ProviderSubmitted: true,
	}
	if err := session.Validate(); err != nil {
		t.Fatal(err)
	}
	aggregate.CandidateRequestSessions = []runtimecontract.RuntimeCandidateRequestSession{session}
	if err := reserveCandidateRequestExecutionBudget(&aggregate, session, 1, physicalCallID); err != nil {
		t.Fatal(err)
	}
	beforeLedger := aggregate.SemanticPlanning.Clone()
	beforeSession := aggregate.CandidateRequestSessions[0].Clone()
	beforeBudget := aggregate.ExecutionBudget.Clone()
	beforeRevision := aggregate.StateRevision

	err := retireSemanticPlanningGeneration(&aggregate, runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated, []RuntimeSemanticCatalogFreshnessGap{{PrerequisiteID: prerequisite.ID}})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("retirement err = %v, want unresolved-submission conflict", err)
	}
	if aggregate.StateRevision != beforeRevision || !reflect.DeepEqual(aggregate.SemanticPlanning, beforeLedger) ||
		!reflect.DeepEqual(aggregate.CandidateRequestSessions[0], beforeSession) || !reflect.DeepEqual(aggregate.ExecutionBudget, beforeBudget) {
		t.Fatalf("rejected retirement mutated owner state: semantic=%+v session=%+v budget=%+v", aggregate.SemanticPlanning, aggregate.CandidateRequestSessions[0], aggregate.ExecutionBudget)
	}
	// The persisted marker and reservation remain a valid restart boundary: a
	// fresh owner can still classify this exact physical call rather than lose it
	// behind a stale-generation terminal.
	if err := aggregate.CandidateRequestSessions[0].Validate(); err != nil {
		t.Fatalf("submitted session is not reloadable: %v", err)
	}
	if err := aggregate.ExecutionBudget.Validate(); err != nil {
		t.Fatalf("submitted budget is not reloadable: %v", err)
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
	for _, catalog := range facts.Resources.OperationAdmissibilities {
		prerequisite := runtimecontract.RuntimePrerequisite{
			ID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
			OwnerModuleID: catalog.OwnerModuleID, RequiredCapabilities: append([]string(nil), catalog.OperationKinds...),
		}
		observation := runtimecontract.RuntimePrerequisiteObservation{
			PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
			ContentHash: catalog.SnapshotID,
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
				Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: catalog.OwnerModuleID,
				SubjectRef: "semantic-test-catalog", Revision: catalog.ObservedRevision,
			}},
		}
		observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
		facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, prerequisite)
		facts.Resources.PrerequisiteObservations = append(facts.Resources.PrerequisiteObservations, observation)
	}
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
	aggregate, err := controller.initialize(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		aggregate.Definition,
		aggregate.Facts.DefinitionSeed,
		aggregate.Facts.DefinitionFocuses,
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
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, applyErr := active.ApplyInternalEvent(ctx, event)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", event.Kind, applyErr)
		}
		aggregate = result.Aggregate
		return aggregate
	}
	apply(controller, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"}})
	apply(controller, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "semantic-request-1"}})
	batch, err := semanticplan.CompileFocusBatch(
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
	apply(controller, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID}})
	apply(controller, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
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
			Kind: RuntimeInternalEventSemanticFeasibilityObserved, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: batchID, SemanticFeasibility: &activeFeasibility},
		})
		t.Fatal("expected partial semantic batch response loss after durable CAS")
	}()
	aggregate = *repository.aggregate
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	revision := aggregate.StateRevision
	apply(restarted, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: batchID, SemanticFeasibility: &activeFeasibility}})
	if aggregate.StateRevision != revision {
		t.Fatal("restart replay changed the committed out-of-order Git feasibility observation")
	}
	conflictingFile := activeFeasibility.Clone()
	conflictingFile.TargetRefs = []string{"different_file_target"}
	if _, conflictErr := restarted.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticFeasibilityObserved, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: batchID, SemanticFeasibility: &conflictingFile},
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
	apply(restarted, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &merged}})
	apply(restarted, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
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
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(ctx, event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
	}
	archiveActiveBatchFeasibility := func() {
		t.Helper()
		batch, found := runtimeSemanticBatch(aggregate.SemanticPlanning.Batches, aggregate.SemanticPlanning.ActiveBatchID)
		if !found || batch.State != runtimecontract.RuntimeSemanticPlanBatchAccepted {
			t.Fatalf("semantic batch was not accepted before feasibility archive: %+v", aggregate.SemanticPlanning)
		}
		document, found := runtimeSemanticBatchDocument(aggregate.SemanticPlanning.BatchDocuments, batch.BatchID)
		if !found {
			t.Fatalf("accepted semantic batch %q has no exact document", batch.BatchID)
		}
		result := runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{
			BatchID: batch.BatchID, BatchHash: batch.Hash,
			DocumentHash:  runtimecontract.StableRuntimeSemanticPlanDocumentHash(document),
			Feasibilities: append([]runtimecontract.RuntimeSemanticProofFeasibility(nil), aggregate.SemanticPlanning.Feasibilities...),
		}
		result.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchFeasibilityObserved, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanBatchFeasibilityResult: &result}})
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "multi-batch-prefix", LogicalRequestID: "order-request"}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "order-request"}})
	// First transaction is scoped to the active focus only. The second CWI is
	// retained as an inert pending placeholder and cannot enter this proof
	// document or owner-fact set.
	firstBatch, err := semanticplan.CompileFocusBatch(
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &firstDocument, LogicalRequestID: firstBatch.RequestLogicalID}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibilities[0]}})
	archiveActiveBatchFeasibility()
	firstAccepted := aggregate.SemanticPlanning.Batches[0]
	firstMerged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, firstAccepted, firstDocument)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &firstMerged}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
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
	// The immutable order still contains the settled predecessor. Replaying the
	// owner-private projection event must narrow it to the current executable
	// closure instead of rejecting the settled audit lineage as in run-087.
	projectedRevision := aggregate.StateRevision
	apply(RuntimeInternalEvent{
		Kind: RuntimeInternalEventEligibleFocusProjected, Authority: &RuntimeInternalAuthorityPayload{SemanticOrderManifestHash: manifest.Hash,
			SemanticCoverageOrder: append([]common.CoverageWorkItemID(nil), order.CoverageOrder...)},
	})
	if aggregate.StateRevision != projectedRevision || aggregate.Facts.Focus.CoverageWorkItemIDs[0] != "cwi-v2-second" {
		t.Fatalf("settled predecessor projection changed current closure: revision=%d want=%d focus=%+v", aggregate.StateRevision, projectedRevision, aggregate.Facts.Focus)
	}
	secondBatch, err := semanticplan.CompileFocusBatch(
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &secondDocument, LogicalRequestID: secondBatch.RequestLogicalID}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibilities[1]}})
	archiveActiveBatchFeasibility()
	secondAccepted := aggregate.SemanticPlanning.Batches[1]
	secondMerged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, secondAccepted, secondDocument)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &secondMerged}})
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
		_, _ = controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{}})
		t.Fatal("expected second semantic seal response loss after durable CAS")
	}()
	aggregate = *repository.aggregate
	controller, err = NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	revision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
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
		Kind: RuntimeInternalEventSemanticCatalogInvalidated, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID,
			RunID: aggregate.Facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{ManifestHash: aggregate.SemanticPlanning.Manifest.Hash,
			LogicalRequestID: aggregate.SemanticPlanning.LogicalRequestID},
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
		aggregate.Facts.DefinitionFocuses,
		aggregate.Facts.Activation.ExecutionContract, aggregate.Facts.Resources.OperationAdmissibilities, eligible,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(freshManifest.CoverageItems) != 1 || freshManifest.CoverageItems[0].ID != pendingID {
		t.Fatalf("fresh semantic manifest reintroduced settled CWI: %+v", freshManifest.CoverageItems)
	}
}

func TestRuntimeSemanticSettlementRetiresGenerationWhenDependencyBecomesEligible(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	dependent := runtimecontract.RuntimeCoverageItemFact{
		ID: "cwi-dependent", SliceID: facts.Slices[0].ID, SourceScope: "dodItem", SourceID: "dod-dependent",
		TargetText: "deliver dependent coverage", State: runtimecontract.RuntimeCoverageItemStateOpen,
		PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
	}
	facts.Coverage = append(facts.Coverage, dependent)
	facts.Slices[0].CoverageWorkItemIDs = append(facts.Slices[0].CoverageWorkItemIDs, dependent.ID)
	facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
		ID: "focus-dependent", SliceID: dependent.SliceID, CoverageWorkItemIDs: []common.CoverageWorkItemID{dependent.ID},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"target.txt"},
	}}
	facts.DefinitionSeed.CoverageItems = append(facts.DefinitionSeed.CoverageItems, runtimecontract.RuntimeCoverageItem{
		ID: dependent.ID, SliceID: dependent.SliceID, SourceScope: dependent.SourceScope, SourceID: dependent.SourceID,
		TargetText: dependent.TargetText, Order: 2, PrerequisiteCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), dependent.PrerequisiteCoverageWorkItemIDs...),
	})
	facts.DefinitionSeed.Slices = facts.Slices
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	catalog, prerequisite, observation := semanticPlanningCatalogFixture()
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
	aggregate, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, document, feasibility := semanticPlanningFixture(t, aggregate)
	if len(manifest.CoverageItems) != 1 || manifest.CoverageItems[0].ID != "cwi-v2" {
		t.Fatalf("initial eligible manifest = %+v, want only prerequisite CWI", manifest.CoverageItems)
	}
	order := document.Clone()
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID = ""
	order.BatchSequence = 0
	order.BatchCoverageWorkItemIDs = nil
	order.CoverageOrder = []common.CoverageWorkItemID{"cwi-v2"}
	order.Proposals = nil

	apply := func(event RuntimeInternalEvent) {
		t.Helper()
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, applyErr := controller.ApplyInternalEvent(ctx, event)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", event.Kind, applyErr)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "dependency-prefix", LogicalRequestID: "dependency-order-request"}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "dependency-order-request"}})
	batch, err := semanticplan.CompileFocusBatch(manifest, order, 0, "dependency-batch-request", aggregate.Facts.Focus.CoverageWorkItemIDs, aggregate.SemanticPlanning.PendingCoverageIDs, 1)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	document.BatchID = batch.BatchID
	document.BatchSequence = batch.Sequence
	document.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &document, LogicalRequestID: batch.RequestLogicalID}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility}})
	accepted := aggregate.SemanticPlanning.Batches[0]
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, accepted, document)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &merged}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
	if err := runtimeTestAdvanceSemanticFocus(t, repository, &aggregate); err != nil {
		t.Fatal(err)
	}

	if aggregate.Facts.Focus.CoverageWorkItemIDs[0] != dependent.ID || aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || len(aggregate.SemanticPlanning.History) != 1 {
		t.Fatalf("dependency unlock did not retire semantic generation: focus=%+v ledger=%+v", aggregate.Facts.Focus, aggregate.SemanticPlanning)
	}
	history := aggregate.SemanticPlanning.History[0]
	if history.Reason != runtimecontract.RuntimeSemanticPlanningRetirementEligibleClosureChanged || len(history.ChangedCoverageIDs) != 1 || history.ChangedCoverageIDs[0] != dependent.ID || len(history.ClosureChangeTriggerRefs) != 1 {
		t.Fatalf("dependency unlock history = %+v", history)
	}
	if len(aggregate.Facts.Focus.CoverageEvidenceBindings) != 0 || aggregate.ActiveDispatch != nil {
		t.Fatalf("dependency unlock fabricated execution authority: focus=%+v dispatch=%+v", aggregate.Facts.Focus, aggregate.ActiveDispatch)
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
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(ctx, event)
		if err != nil {
			t.Fatalf("apply %s: %v", event.Kind, err)
		}
		aggregate = result.Aggregate
	}

	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "settled-cwi-catalog-prefix", LogicalRequestID: "settled-cwi-order-request"}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "settled-cwi-order-request"}})
	batch, err := semanticplan.CompileFocusBatch(
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &document, LogicalRequestID: batch.RequestLogicalID}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibilities[0]}})
	accepted := aggregate.SemanticPlanning.Batches[0]
	acceptedDocument, found := runtimeSemanticBatchDocument(aggregate.SemanticPlanning.BatchDocuments, accepted.BatchID)
	if !found {
		t.Fatalf("accepted semantic batch %q has no exact document", accepted.BatchID)
	}
	batchResult := runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{
		BatchID: accepted.BatchID, BatchHash: accepted.Hash,
		DocumentHash:  runtimecontract.StableRuntimeSemanticPlanDocumentHash(acceptedDocument),
		Feasibilities: append([]runtimecontract.RuntimeSemanticProofFeasibility(nil), aggregate.SemanticPlanning.Feasibilities...),
	}
	batchResult.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(batchResult)
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchFeasibilityObserved, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanBatchFeasibilityResult: &batchResult}})
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, accepted, document)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &merged}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
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
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
	aggregate, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		aggregate.Definition,
		aggregate.Facts.DefinitionSeed,
		aggregate.Facts.DefinitionFocuses,
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
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
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
		event.Scope.WorkspaceID, event.Scope.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, applyErr := controller.ApplyInternalEvent(context.Background(), event)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", event.Kind, applyErr)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "semantic-retirement-prefix", LogicalRequestID: "semantic-retirement-request"}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "semantic-retirement-request"}})
	batch, err := semanticplan.CompileFocusBatch(manifest, order, 0, "semantic-retirement-batch", aggregate.Facts.Focus.CoverageWorkItemIDs, aggregate.SemanticPlanning.PendingCoverageIDs, 1)
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	feasibility.CatalogSnapshotID = catalog.SnapshotID
	feasibility.CatalogObservationID = observation.ObservationID
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility}})
	accepted := aggregate.SemanticPlanning.Batches[0]
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, accepted, proof)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &merged}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
	return controller, repository, aggregate, catalog, prerequisite, observation
}

func newSemanticPlanningController(t *testing.T) (*RuntimeController, *controllerRepository, RuntimeAggregate) {
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
	return controller, repository, aggregate
}

func semanticPlanningFixture(t *testing.T, aggregate RuntimeAggregate) (runtimecontract.RuntimeSemanticPlanManifest, runtimecontract.RuntimeSemanticPlanDocument, runtimecontract.RuntimeSemanticProofFeasibility) {
	t.Helper()
	manifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		aggregate.Definition,
		aggregate.Facts.DefinitionSeed,
		aggregate.Facts.DefinitionFocuses,
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
	observation, err := requestcontent.ProjectRuntimeSemanticCorrectionObservation(&rejection)
	if err != nil {
		observation = &runtimecontract.RuntimeSemanticCorrectionObservation{
			SchemaVersion: runtimecontract.RuntimeSemanticCorrectionObservationSchemaCurrent,
			OwnerModuleID: rejection.OwnerModuleID, Class: rejection.OriginalClass, Code: rejection.OriginalCode,
			Signature: rejection.Signature, SemanticViolations: append([]runtimecontract.RuntimeSemanticContractViolation(nil), rejection.SemanticViolations...),
		}
	}
	directive := runtimecontract.RuntimeSemanticCorrectionDirective{
		SchemaVersion: runtimecontract.RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      step, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		StablePrefixHash: prefix, SourceObservationHash: runtimecontract.StableRuntimeSemanticCorrectionObservationHash(*observation), Attempt: 1,
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

// runtimeTestCommitSemanticStateChangeReceipt records an effect-bearing
// semantic proof through a real sealed operation, owner receipt and SM-3
// evidence commit. It deliberately stops before assessment so tests can prove
// that the accepted effect locks its owning binding.
func runtimeTestCommitSemanticStateChangeReceipt(t *testing.T, repository *controllerRepository, aggregate *RuntimeAggregate) error {
	t.Helper()
	if repository == nil || aggregate == nil {
		return common.ErrInvalidArgument
	}
	if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite {
		controller, err := NewRuntimeController(repository)
		if err != nil {
			return err
		}
		for _, prerequisite := range aggregate.Facts.Prerequisites.Pending {
			if prerequisite.Kind != runtimecontract.PrerequisiteKindTargetSnapshot {
				continue
			}
			snapshot := runtimecontract.TargetSnapshot{
				TargetRef: prerequisite.SubjectRef, Content: "semantic test target",
				ContentHash: "semantic-test-snapshot/" + prerequisite.SubjectRef, ContentState: runtimecontract.SnapshotContentStateComplete,
			}
			observation := runtimecontract.RuntimePrerequisiteObservation{
				PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: snapshot.ContentHash,
				Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
					Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					SubjectRef: snapshot.TargetRef, Revision: snapshot.ContentHash,
				}},
			}
			observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
			resumed, commitErr := controller.commitPrerequisite(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, PrerequisiteResult{
				PrerequisiteID: prerequisite.ID, TargetSnapshot: &snapshot, Observation: &observation,
			})
			if commitErr != nil {
				return commitErr
			}
			*aggregate = resumed
		}
	}
	if len(aggregate.Facts.Focus.CoverageWorkItemIDs) == 0 || len(aggregate.Facts.Focus.CoverageEvidenceBindings) == 0 {
		return fmt.Errorf("semantic focus fixture has no current binding")
	}
	focus := aggregate.Facts.Focus.Normalized()
	targetRefs := append([]string(nil), focus.TargetRefs...)
	if len(targetRefs) == 0 {
		for _, binding := range focus.CoverageEvidenceBindings {
			targetRefs = append(targetRefs, binding.TargetRefs...)
		}
	}
	operation := runtimecontract.RuntimeOperation{
		ID: "semantic-test-operation-" + fmt.Sprintf("%d", aggregate.StateRevision), Kind: runtimecontract.StepKindFileWrite,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), focus.CoverageWorkItemIDs...),
		TargetRefs:          targetRefs,
		Parameters:          runtimeFileWriteParameters("semantic focus proof committed"),
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: operation}}}}
	durable := &durableRuntimeStateRepository{aggregate: *aggregate}
	sealed := sealCandidateForTest(t, durable, candidate)
	operationID := runtimeTestActiveOperationID(t, sealed, 0)
	evidence := ExecutionEvidence{
		EventID:     "semantic-test-owner-receipt-" + sealed.ActiveDispatch.ID,
		WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID,
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: operationID, Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &durable.aggregate, &evidence)
	coordinator, err := NewDispatchCoordinator(durable)
	if err != nil {
		return err
	}
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		return err
	}
	*aggregate = committed.Aggregate
	value := *aggregate
	repository.aggregate = &value
	return nil
}

// runtimeTestAdvanceSemanticFocus completes the assessment after the real
// receipt above. The production reducer then owns CWI settlement and focus
// advancement; the test never injects an orphan EvidenceFact or directly
// edits convergence state.
func runtimeTestAdvanceSemanticFocus(t *testing.T, repository *controllerRepository, aggregate *RuntimeAggregate) error {
	t.Helper()
	if err := runtimeTestCommitSemanticStateChangeReceipt(t, repository, aggregate); err != nil {
		return err
	}
	if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedAssessment {
		controller, controllerErr := NewRuntimeController(repository)
		if controllerErr != nil {
			return controllerErr
		}
		prefix := "semantic-test-assessment-" + fmt.Sprintf("%d", aggregate.StateRevision)
		if _, beginErr := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, prefix); beginErr != nil {
			return beginErr
		}
		assessment := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
			FocusID: aggregate.Facts.Focus.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...),
			Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "semantic focus proof satisfied",
			EvidenceRefs: runtimecontract.AcceptedEvidenceRefsForFocus(aggregate.Facts),
		}}
		if _, acceptErr := controller.AcceptCandidateAttempt(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, assessment); acceptErr != nil {
			return acceptErr
		}
		assessed, assessmentErr := controller.commitAssessment(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
		if assessmentErr != nil {
			return assessmentErr
		}
		*aggregate = assessed
	}
	value := *aggregate
	repository.aggregate = &value
	return nil
}
