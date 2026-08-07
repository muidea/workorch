package runtime

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &document, LogicalRequestID: "semantic-request-1"})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	batchID := aggregate.SemanticPlanning.FeasibilityBatchID
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: batchID, SemanticFeasibility: &feasibility})
	observationRevision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: batchID, SemanticFeasibility: &feasibility})
	if aggregate.StateRevision != observationRevision {
		t.Fatal("duplicate owner observation changed aggregate revision")
	}
	savesBeforeSeal := repository.saves
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	if repository.saves != savesBeforeSeal+1 {
		t.Fatalf("semantic seal saves = %d, want one atomic save after %d", repository.saves, savesBeforeSeal)
	}
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed {
		t.Fatalf("semantic phase = %q", aggregate.SemanticPlanning.Phase)
	}
	if aggregate.Facts.ReconciliationContract.BindingSetHash == "" || aggregate.SemanticPlanning.SealedBindingSetHash != aggregate.Facts.ReconciliationContract.BindingSetHash || len(aggregate.Facts.Focus.CoverageEvidenceBindings) != 1 {
		t.Fatalf("semantic authority was not sealed atomically: ledger=%+v facts=%+v", aggregate.SemanticPlanning, aggregate.Facts)
	}
	sealedRevision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	if aggregate.StateRevision != sealedRevision || repository.saves != savesBeforeSeal+1 {
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
	batch, err := runtimecontract.CompileRuntimeSemanticPlanBatch(manifest, order, 0, "batch-request", 1)
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
		DocumentHash:  runtimecontract.StableRuntimeSemanticPlanDocumentHash(proof),
		Feasibilities: []runtimecontract.RuntimeSemanticProofFeasibility{feasibility},
	}
	batchResult.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(batchResult)
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchFeasibilityObserved, SemanticPlanBatchFeasibilityResult: &batchResult})
	merged, err := runtimecontract.MergeRuntimeSemanticPlanBatchDocuments(manifest, order, []runtimecontract.RuntimeSemanticPlanDocument{proof})
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
	mergeFeasibilityRevision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchFeasibilitiesMerged, SemanticPlanMergedFeasibilities: []runtimecontract.RuntimeSemanticProofFeasibility{feasibility}})
	if aggregate.StateRevision != mergeFeasibilityRevision {
		t.Fatal("replayed merged feasibility changed durable state")
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed || len(aggregate.Facts.ReconciliationContract.Bindings) != 1 {
		t.Fatalf("batch replay recovery did not reach atomic semantic seal: %+v", aggregate.SemanticPlanning)
	}
}

func TestRuntimeSemanticBatchCorrectionKeepsBatchLineage(t *testing.T) {
	controller, _, aggregate := newSemanticPlanningController(t)
	manifest, order, _ := semanticPlanningFixture(t, aggregate)
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
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
	batch, err := runtimecontract.CompileRuntimeSemanticPlanBatch(manifest, order, 0, "batch-request", 1)
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
	correctionHash := runtimecontract.StableRuntimeSemanticPlanCorrectionHash(diagnostic)
	corrected, err := controller.requireSemanticPlanCorrection(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.SemanticPlanning.StablePrefixHash, "correction-request", correctionHash, diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if corrected.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningCorrectionRequired || corrected.SemanticPlanning.ActiveBatchID != batch.BatchID || corrected.SemanticPlanning.Attempt != 1 {
		t.Fatalf("batch correction lost active lineage: %+v", corrected.SemanticPlanning)
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

	manifest, _, _ = semanticPlanningFixture(t, aggregate)
	started, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanRequestStarted, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	rejection := semanticPlanningDiagnostic()
	correctionHash := runtimecontract.StableRuntimeSemanticPlanCorrectionHash(rejection)
	corrected, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanCorrectionRequired, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-2", CorrectionHash: correctionHash, SemanticDiagnostic: &rejection,
	})
	if err != nil {
		t.Fatal(err)
	}
	if corrected.Aggregate.SemanticPlanning.Attempt != 1 || len(corrected.Aggregate.SemanticPlanning.Rejections) != 1 || corrected.Aggregate.StateRevision != started.Aggregate.StateRevision+1 {
		t.Fatalf("correction lineage = %+v", corrected.Aggregate.SemanticPlanning)
	}
	if _, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanCorrectionRequired, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-3", CorrectionHash: correctionHash, SemanticDiagnostic: &rejection,
	}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("third semantic request error = %v", err)
	}
}

func TestRuntimeSemanticPlanningFailurePreservesOwnerDiagnosticFacts(t *testing.T) {
	controller, _, aggregate := newSemanticPlanningController(t)
	manifest, document, feasibility := semanticPlanningFixture(t, aggregate)
	ctx := context.Background()
	apply := func(event RuntimeInternalEvent) RuntimeAggregate {
		t.Helper()
		event.WorkspaceID, event.RunID = aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
		result, err := controller.ApplyInternalEvent(ctx, event)
		if err != nil {
			t.Fatal(err)
		}
		aggregate = result.Aggregate
		return aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, SemanticManifest: &manifest, StablePrefixHash: "semantic-prefix-1", LogicalRequestID: "semantic-request-1"})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &document, LogicalRequestID: "semantic-request-1"})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	ownerDiagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OriginalClass: "feasibility", OriginalCode: "target_outside_scope", OriginalMessage: "  target is outside owner scope\n", Signature: "file/target-outside-scope",
	}
	feasibility.Status = runtimecontract.RuntimeSemanticFeasibilityInfeasible
	feasibility.OriginalDiagnostic = &ownerDiagnostic
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility})
	failure := semanticPlanningDiagnostic()
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractFailed, SemanticDiagnostic: &failure})
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed || len(aggregate.SemanticPlanning.Feasibilities) != 1 || aggregate.SemanticPlanning.Feasibilities[0].OriginalDiagnostic == nil || aggregate.SemanticPlanning.Feasibilities[0].OriginalDiagnostic.OriginalMessage != ownerDiagnostic.OriginalMessage {
		t.Fatalf("failed semantic ledger lost owner facts: %+v", aggregate.SemanticPlanning)
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
	withSession, session, err := controller.ensureCandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentSemanticPlan, "semantic-prefix-1", manifest.Hash, "")
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &document, LogicalRequestID: session.LogicalRequestID})
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
	withNewSession, newSession, err := controller.ensureCandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentSemanticPlan, "semantic-prefix-1", newManifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	oldSessionComplete := false
	for _, persisted := range withNewSession.CandidateRequestSessions {
		if persisted.LogicalRequestID == session.LogicalRequestID && persisted.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
			oldSessionComplete = true
		}
	}
	if newSession.LogicalRequestID == session.LogicalRequestID || len(withNewSession.CandidateRequestSessions) != 2 || !oldSessionComplete {
		t.Fatalf("fresh catalog reused or rewrote old semantic request session: old=%+v new=%+v", session, newSession)
	}
}

func TestRuntimeSemanticPrerequisiteRequiredRetiresCatalogWithoutInventingStaleRevision(t *testing.T) {
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &document, LogicalRequestID: "semantic-request-1"})
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
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractRequired, SemanticBatchID: batchID, SemanticRefreshGeneration: 0})
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || len(aggregate.PrerequisiteInvalidations) != 1 ||
		aggregate.PrerequisiteInvalidations[0].Stale != nil || aggregate.PrerequisiteInvalidations[0].RefreshDiagnostic == nil ||
		aggregate.PrerequisiteInvalidations[0].RefreshDiagnostic.OriginalMessage != "workspace manifest could not be observed" ||
		len(aggregate.Facts.Prerequisites.Pending) != 1 || aggregate.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("prerequisite-required semantic gap was not preserved as its own refresh fact: %+v", aggregate)
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
	facts.DefinitionSeed.CoverageItems = append(facts.DefinitionSeed.CoverageItems, runtimecontract.RuntimeCoverageItem{
		ID: secondItem.ID, SliceID: secondItem.SliceID, SourceScope: secondItem.SourceScope, SourceID: secondItem.SourceID, TargetText: secondItem.TargetText,
	})
	facts.DefinitionSeed.Slices = facts.Slices
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	definition := controllerDefinition()
	definition.CapabilityContract = append(definition.CapabilityContract, runtimecontract.StepKindGitWrite)
	aggregate, err := controller.initialize(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
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
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DefinitionRevision: aggregate.Facts.DefinitionRevision, DefinitionSeedHash: aggregate.Facts.DefinitionSeed.Hash,
		OperationContractRevision: aggregate.Facts.Activation.ExecutionContract.OperationContractRevision,
		CoverageItems:             aggregate.Facts.DefinitionSeed.CoverageItems,
		AllowedOwnerModuleIDs: []string{
			runtimecontract.RuntimeModuleFile,
			runtimecontract.RuntimeModuleGit,
		},
		AllowedOperationKinds: []string{
			runtimecontract.StepKindFileWrite,
			runtimecontract.StepKindGitWrite,
		},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{
			fileCatalog,
			gitCatalog,
		},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	proposals := []runtimecontract.RuntimeSemanticProofProposal{
		{
			CoverageWorkItemID: manifest.CoverageItems[0].ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite,
			Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"first.txt"}},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			DeclarationRef:        "semantic/" + string(manifest.CoverageItems[0].ID),
		},
		{
			CoverageWorkItemID: manifest.CoverageItems[1].ID, OwnerModuleID: runtimecontract.RuntimeModuleGit, OperationKind: runtimecontract.StepKindGitWrite,
			Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"workspace_deliverable"}},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			DeclarationRef:        "semantic/" + string(manifest.CoverageItems[1].ID),
		},
	}
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, Proposals: proposals,
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
	apply(controller, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &document, LogicalRequestID: "semantic-request-1"})
	apply(controller, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
	batchID := aggregate.SemanticPlanning.FeasibilityBatchID
	feasibilities := make([]runtimecontract.RuntimeSemanticProofFeasibility, 0, 2)
	for _, proposal := range proposals {
		catalog := fileCatalog
		if proposal.OwnerModuleID == runtimecontract.RuntimeModuleGit {
			catalog = gitCatalog
		}
		feasibilities = append(feasibilities, runtimecontract.RuntimeSemanticProofFeasibility{
			SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash,
			DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID,
			ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID,
			CatalogSnapshotID: catalog.SnapshotID, Status: runtimecontract.RuntimeSemanticFeasibilityFeasible,
			ObservedRevision: catalog.ObservedRevision, TargetRefs: proposal.Target.ExactTargetRefs,
		})
	}
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
			SemanticBatchID: batchID, SemanticFeasibility: &feasibilities[1],
		})
		t.Fatal("expected partial semantic batch response loss after durable CAS")
	}()
	aggregate = *repository.aggregate
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	revision := aggregate.StateRevision
	apply(restarted, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: batchID, SemanticFeasibility: &feasibilities[1]})
	if aggregate.StateRevision != revision {
		t.Fatal("restart replay changed the committed out-of-order Git feasibility observation")
	}
	conflictingGit := feasibilities[1].Clone()
	conflictingGit.TargetRefs = []string{"different_repository_target"}
	if _, conflictErr := restarted.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticFeasibilityObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticBatchID: batchID, SemanticFeasibility: &conflictingGit,
	}); !errors.Is(conflictErr, common.ErrConflict) {
		t.Fatalf("conflicting Git feasibility error = %v, want conflict", conflictErr)
	}
	if repository.aggregate.StateRevision != revision {
		t.Fatalf("conflicting Git feasibility changed durable revision: got %d want %d", repository.aggregate.StateRevision, revision)
	}
	apply(restarted, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: batchID, SemanticFeasibility: &feasibilities[0]})
	if len(aggregate.SemanticPlanning.Feasibilities) != 2 || aggregate.SemanticPlanning.FeasibilitySetHash != runtimecontract.StableRuntimeSemanticFeasibilitySetHash(feasibilities) {
		t.Fatalf("partial semantic batch did not resume at the missing owner fact: %+v", aggregate.SemanticPlanning)
	}
	apply(restarted, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed || len(aggregate.Facts.ReconciliationContract.Bindings) != 2 {
		t.Fatalf("recovered semantic fan-out did not seal the complete binding set: %+v", aggregate)
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
	for sequence := 0; sequence < 2; sequence++ {
		batch, err := runtimecontract.CompileRuntimeSemanticPlanBatchFromOffset(manifest, order, sequence, sequence, fmt.Sprintf("batch-request-%d", sequence), 1)
		if err != nil {
			t.Fatal(err)
		}
		aggregate, err = controller.compileSemanticPlanBatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
		if err != nil {
			t.Fatal(err)
		}
		document := runtimecontract.RuntimeSemanticPlanDocument{
			SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
			PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch, BatchID: batch.BatchID, BatchSequence: batch.Sequence,
			BatchCoverageWorkItemIDs: batch.CoverageWorkItemIDs, CoverageOrder: order.CoverageOrder,
			Proposals: []runtimecontract.RuntimeSemanticProofProposal{proposals[sequence]},
		}
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, SemanticDocument: &document, LogicalRequestID: batch.RequestLogicalID})
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired})
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibilities[sequence]})
		accepted := aggregate.SemanticPlanning.Batches[sequence]
		result := runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{BatchID: accepted.BatchID, BatchHash: accepted.Hash, DocumentHash: runtimecontract.StableRuntimeSemanticPlanDocumentHash(document), Feasibilities: []runtimecontract.RuntimeSemanticProofFeasibility{feasibilities[sequence]}}
		result.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
		apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchFeasibilityObserved, SemanticPlanBatchFeasibilityResult: &result})
	}
	merged, err := runtimecontract.MergeRuntimeSemanticPlanBatchDocuments(manifest, order, aggregate.SemanticPlanning.BatchDocuments)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, SemanticPlanMergedDocument: &merged})
	mergedFacts, err := runtimecontract.MergeRuntimeSemanticPlanBatchFeasibilityResults(manifest, merged, aggregate.SemanticPlanning.Batches, aggregate.SemanticPlanning.BatchFeasibilityResults)
	if err != nil {
		t.Fatal(err)
	}
	responseLoss := &semanticResponseLossRepository{RuntimeStateRepository: repository, marker: "semantic-multi-batch-feasibility-merge-response-loss", match: func(candidate RuntimeAggregate) bool {
		return candidate.SemanticPlanning.FeasibilitySetHash == runtimecontract.StableRuntimeSemanticFeasibilitySetHash(mergedFacts)
	}}
	controller.repository = responseLoss
	controller.transitions.repository = responseLoss
	func() {
		defer func() {
			if recovered := recover(); recovered != "semantic-multi-batch-feasibility-merge-response-loss" {
				t.Fatalf("unexpected feasibility merge response-loss marker: %v", recovered)
			}
		}()
		_, _ = controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchFeasibilitiesMerged, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, SemanticPlanMergedFeasibilities: mergedFacts})
		t.Fatal("expected feasibility merge response loss after durable CAS")
	}()
	aggregate = *repository.aggregate
	controller, err = NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	revision := aggregate.StateRevision
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchFeasibilitiesMerged, SemanticPlanMergedFeasibilities: mergedFacts})
	if aggregate.StateRevision != revision {
		t.Fatal("replayed multi-batch feasibility merge changed durable revision")
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed})
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed || len(aggregate.Facts.ReconciliationContract.Bindings) != 2 {
		t.Fatalf("multi-batch feasibility recovery did not seal: %+v", aggregate.SemanticPlanning)
	}
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
	facts.DefinitionSeed.CoverageItems = append(facts.DefinitionSeed.CoverageItems, runtimecontract.RuntimeCoverageItem{ID: second.ID, SliceID: second.SliceID, SourceScope: second.SourceScope, SourceID: second.SourceID, TargetText: second.TargetText})
	facts.DefinitionSeed.Slices = facts.Slices
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	aggregate, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"target.txt"}, ObservedRevision: "file-revision-1"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := runtimecontract.RuntimeSemanticPlanManifest{SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, DefinitionRevision: aggregate.Facts.DefinitionRevision, DefinitionSeedHash: aggregate.Facts.DefinitionSeed.Hash, OperationContractRevision: aggregate.Facts.Activation.ExecutionContract.OperationContractRevision, CoverageItems: aggregate.Facts.DefinitionSeed.CoverageItems, AllowedOwnerModuleIDs: []string{runtimecontract.RuntimeModuleFile}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget}, OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{catalog}}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
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
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"target.txt"}, ObservedRevision: "file-revision-1"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DefinitionRevision: aggregate.Facts.DefinitionRevision, DefinitionSeedHash: aggregate.Facts.DefinitionSeed.Hash,
		OperationContractRevision: aggregate.Facts.Activation.ExecutionContract.OperationContractRevision,
		CoverageItems:             aggregate.Facts.DefinitionSeed.CoverageItems,
		AllowedOwnerModuleIDs:     []string{runtimecontract.RuntimeModuleFile},
		AllowedOperationKinds:     []string{runtimecontract.StepKindFileWrite},
		AllowedTargetPolicies:     []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		OwnerCatalogs:             []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-v2", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite,
		Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"target.txt"}},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef: "semantic/cwi-v2",
	}
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, Proposals: []runtimecontract.RuntimeSemanticProofProposal{proposal},
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	feasibility := runtimecontract.RuntimeSemanticProofFeasibility{
		SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID,
		ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID,
		CatalogSnapshotID: "catalog-file-1", Status: runtimecontract.RuntimeSemanticFeasibilityFeasible, ObservedRevision: "workspace-revision-1", TargetRefs: []string{"target.txt"},
	}
	return manifest, document, feasibility
}

func semanticPlanningDiagnostic() runtimecontract.RuntimeOriginalDiagnostic {
	return runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
		OriginalClass: "contract", OriginalCode: "semantic_plan_rejected", OriginalMessage: "proposal is outside the manifest", Signature: "semantic-plan/rejected-1",
	}
}
