package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"testing"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

var runtimeCandidateRequestContentTestStores sync.Map

func runtimeProviderFeedbackForTest(t *testing.T, command llmevents.RequestRuntimeCandidateCommand, failure runtimecontract.RuntimeCandidateFailure) *runtimecontract.RuntimeFeedback {
	t.Helper()
	category := runtimecontract.RuntimeFeedbackCategoryProviderProtocol
	switch failure.Class {
	case runtimecontract.RuntimeCandidateFailureClassProviderAuthentication:
		category = runtimecontract.RuntimeFeedbackCategoryProviderAuthentication
	case runtimecontract.RuntimeCandidateFailureClassProviderConfiguration:
		category = runtimecontract.RuntimeFeedbackCategoryProviderConfiguration
	case runtimecontract.RuntimeCandidateFailureClassProviderTransient:
		category = runtimecontract.RuntimeFeedbackCategoryProviderTransient
	}
	if failure.Code == "" {
		failure.Code = string(category)
	}
	if failure.Signature == "" {
		failure.Signature = "test-" + string(category)
	}
	scope := runtimecontract.RuntimeFeedbackScope{WorkspaceID: command.WorkspaceID, RunID: command.RunID}
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
		scope.AuthoritySnapshotID = command.Transaction.AuthoritySnapshotID
	}
	feedback, err := runtimecontract.RuntimeFeedbackFromCandidateFailure(scope, runtimecontract.RuntimeFeedbackDomainProviderRequest, category, runtimecontract.RuntimeModuleLLM, failure)
	if err != nil {
		t.Fatal(err)
	}
	return &feedback
}

func runtimePreflightFeedbackForTest(t *testing.T, command llmevents.PreflightRuntimeCandidateCommand, reason string) runtimecontract.RuntimeFeedback {
	t.Helper()
	feedback := runtimecontract.RuntimeFeedback{
		Domain: runtimecontract.RuntimeFeedbackDomainProviderPreflight, StableCategory: runtimecontract.RuntimeFeedbackCategoryAccepted,
		WorkspaceID: command.WorkspaceID, RunID: command.RunID,
	}
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
		feedback.AuthoritySnapshotID = command.Transaction.AuthoritySnapshotID
	}
	if reason != "" {
		feedback.StableCategory = runtimecontract.RuntimeFeedbackCategoryContextIntegrity
		feedback.OriginalDiagnostic = &runtimecontract.RuntimeOriginalDiagnostic{
			SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
			OriginalClass: "context_integrity", OriginalCode: "candidate_context_integrity", OriginalMessage: reason, Signature: "test-context-integrity",
		}
	}
	if err := feedback.Validate(); err != nil {
		t.Fatal(err)
	}
	return feedback
}

func runtimeTestAggregateWithSemanticPlanning(t *testing.T, aggregate runtime.RuntimeAggregate) runtime.RuntimeAggregate {
	t.Helper()
	if aggregate.Facts.DefinitionSeed.SchemaVersion == "" {
		items := make([]runtimecontract.RuntimeCoverageItem, 0, len(aggregate.Facts.Coverage))
		slicesByID := map[string][]common.CoverageWorkItemID{}
		for _, fact := range aggregate.Facts.Coverage {
			sliceID := fact.SliceID
			if sliceID == "" {
				sliceID = "slice-test-" + string(fact.ID)
			}
			sourceScope, sourceID := fact.SourceScope, fact.SourceID
			if sourceScope == "" {
				sourceScope = "test"
			}
			if sourceID == "" {
				sourceID = string(fact.ID)
			}
			items = append(items, runtimecontract.RuntimeCoverageItem{ID: fact.ID, SliceID: sliceID, SourceScope: sourceScope, SourceID: sourceID, TargetText: fact.TargetText, PrerequisiteCoverageWorkItemIDs: fact.PrerequisiteCoverageWorkItemIDs, ReopenDependencies: fact.ReopenDependencies})
			slicesByID[sliceID] = append(slicesByID[sliceID], fact.ID)
		}
		if len(items) == 0 {
			sliceID := aggregate.Facts.Focus.SliceID
			if sliceID == "" {
				sliceID = "slice-test"
			}
			for _, coverageID := range aggregate.Facts.Focus.CoverageWorkItemIDs {
				items = append(items, runtimecontract.RuntimeCoverageItem{ID: coverageID, SliceID: sliceID, SourceScope: "test", SourceID: string(coverageID)})
				slicesByID[sliceID] = append(slicesByID[sliceID], coverageID)
			}
		}
		slices := make([]runtimecontract.RuntimeAcceptanceSlice, 0, len(slicesByID))
		for sliceID, coverageIDs := range slicesByID {
			slices = append(slices, runtimecontract.RuntimeAcceptanceSlice{ID: sliceID, CoverageWorkItemIDs: coverageIDs})
		}
		aggregate.Facts.DefinitionSeed = runtimecontract.RuntimeDefinitionSeed{
			SchemaVersion: runtimecontract.RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: aggregate.Facts.DefinitionRevision,
			PolicyHash: "policy-test", CoverageItems: items, Slices: slices,
		}
		aggregate.Facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(aggregate.Facts.DefinitionSeed)
	} else {
		// Test fixtures often append a queued focus after constructing the base
		// DefinitionSeed. Keep the immutable seed in lock-step before building
		// semantic order lineage; production DefinitionSeed remains immutable.
		seed := aggregate.Facts.DefinitionSeed.Clone()
		known := make(map[common.CoverageWorkItemID]struct{}, len(seed.CoverageItems))
		for _, item := range seed.CoverageItems {
			known[item.ID] = struct{}{}
		}
		sliceIndex := make(map[string]int, len(seed.Slices))
		for idx, slice := range seed.Slices {
			sliceIndex[slice.ID] = idx
		}
		changed := false
		for _, fact := range aggregate.Facts.Coverage {
			if _, exists := known[fact.ID]; exists {
				continue
			}
			sliceID := fact.SliceID
			if sliceID == "" {
				sliceID = "slice-test-" + string(fact.ID)
			}
			sourceScope, sourceID := fact.SourceScope, fact.SourceID
			if sourceScope == "" {
				sourceScope = "test"
			}
			if sourceID == "" {
				sourceID = string(fact.ID)
			}
			seed.CoverageItems = append(seed.CoverageItems, runtimecontract.RuntimeCoverageItem{
				ID: fact.ID, SliceID: sliceID, SourceScope: sourceScope, SourceID: sourceID,
				TargetText: fact.TargetText, PrerequisiteCoverageWorkItemIDs: fact.PrerequisiteCoverageWorkItemIDs,
				ReopenDependencies: fact.ReopenDependencies,
			})
			known[fact.ID] = struct{}{}
			if idx, exists := sliceIndex[sliceID]; exists {
				seed.Slices[idx].CoverageWorkItemIDs = append(seed.Slices[idx].CoverageWorkItemIDs, fact.ID)
			} else {
				seed.Slices = append(seed.Slices, runtimecontract.RuntimeAcceptanceSlice{ID: sliceID, CoverageWorkItemIDs: []common.CoverageWorkItemID{fact.ID}})
				sliceIndex[sliceID] = len(seed.Slices) - 1
			}
			changed = true
		}
		if changed {
			seed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(seed)
			aggregate.Facts.DefinitionSeed = seed
		}
	}
	if aggregate.SemanticPlanning.SchemaVersion == "" {
		ledger, err := runtimecontract.NewRuntimeSemanticPlanningLedger(aggregate.Facts.DefinitionSeed)
		if err != nil {
			t.Fatal(err)
		}
		aggregate.SemanticPlanning = ledger
	}
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed {
		aggregate.SemanticPlanning = runtimeTestSealedSemanticPlanning(t, aggregate, aggregate.SemanticPlanning)
	}
	return aggregate
}

// runtimeTestPersistSealedSemanticAuthority upgrades an already initialized
// post-semantic operation fixture through the same RuntimeStateRepository CAS
// boundary used by production. It is intentionally test-only: semantic-drive
// tests must execute the real proposal/feasibility/seal sequence instead.
func runtimeTestPersistSealedSemanticAuthority(
	t *testing.T,
	ctx context.Context,
	repository runtime.RuntimeStateRepository,
	aggregate runtime.RuntimeAggregate,
) runtime.RuntimeAggregate {
	t.Helper()
	if repository == nil {
		t.Fatal("runtime test semantic authority repository is nil")
	}
	next := runtimeTestAggregateWithSemanticPlanning(t, aggregate)
	if next.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed {
		t.Fatalf("runtime test semantic authority phase = %q", next.SemanticPlanning.Phase)
	}
	next.StateRevision = aggregate.StateRevision + 1
	if err := repository.Save(ctx, next, aggregate.StateRevision); err != nil {
		t.Fatalf("persist runtime test semantic authority: %v", err)
	}
	return next
}

func runtimeTestInitializeOperationFixture(
	t *testing.T,
	ctx context.Context,
	owner *Biz,
	repository runtime.RuntimeStateRepository,
	facts runtimecontract.RuntimeFacts,
	definition runtimecontract.RuntimeDefinitionContext,
	requirements []runtimecontract.RuntimeRequirement,
) runtime.RuntimeAggregate {
	t.Helper()
	initialized, err := owner.InitializeRuntime(ctx, facts, definition, requirements)
	if err != nil {
		t.Fatalf("initialize runtime operation fixture: %v", err)
	}
	return runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, initialized)
}

// runtimeTestPersistBootstrappedOperationFixture prepares a Run-backed test
// that starts from a RuntimeBootstrap but only exercises the post-semantic
// operation/recovery mainline. Fresh semantic-drive tests must not use it.
func runtimeTestPersistBootstrappedOperationFixture(
	t *testing.T,
	ctx context.Context,
	hub magicevent.Hub,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	bootstrap runtimecontract.RuntimeBootstrap,
) runtime.RuntimeAggregate {
	t.Helper()
	if hub == nil {
		t.Fatal("runtime test operation fixture hub is nil")
	}
	facts, err := bootstrap.InitialFacts(workspaceID, runID)
	if err != nil {
		t.Fatalf("build runtime operation fixture facts: %v", err)
	}
	repository := runtimeStateRepository{hub: hub}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatalf("configure runtime operation fixture repository: %v", err)
	}
	return runtimeTestInitializeOperationFixture(t, ctx, owner, repository, facts, bootstrap.Definition, bootstrap.Requirements)
}

// Most orchestrator drive fixtures exercise the post-semantic operations
// mainline. Seal a synthetic, internally valid semantic ledger so those tests
// do not accidentally become semantic-provider fixtures. Dedicated semantic
// planning tests initialize the controller directly and retain the required
// phase.
func runtimeTestSealedSemanticPlanning(t *testing.T, aggregate runtime.RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger) runtimecontract.RuntimeSemanticPlanningLedger {
	t.Helper()
	testTargets := make([]string, 0, len(aggregate.Facts.DefinitionSeed.CoverageItems))
	for idx := range aggregate.Facts.DefinitionSeed.CoverageItems {
		testTargets = append(testTargets, fmt.Sprintf("fixture-semantic-%d.txt", idx+1))
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "test-semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: testTargets, ObservedRevision: "test-semantic-revision"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	operationContractRevision := aggregate.Facts.Activation.ExecutionContract.OperationContractRevision
	if operationContractRevision == "" {
		operationContractRevision = runtimecontract.RuntimeOperationContractRevision()
	}
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DefinitionRevision: aggregate.Facts.DefinitionRevision, DefinitionSeedHash: aggregate.Facts.DefinitionSeed.Hash,
		OperationContractRevision: operationContractRevision,
		CoverageItems:             aggregate.Facts.DefinitionSeed.CoverageItems,
		AllowedOwnerModuleIDs:     []string{runtimecontract.RuntimeModuleFile}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		OwnerCatalogs:         []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	coverageOrder, err := runtimeTestSemanticCoverageOrderForSupport(aggregate.Facts.DefinitionSeed)
	if err != nil {
		t.Fatalf("build test semantic coverage order: %v", err)
	}
	orderDocument := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: coverageOrder,
	}
	if err := orderDocument.ValidateAgainstManifest(manifest); err != nil {
		t.Fatalf("validate test semantic coverage order: %v", err)
	}
	activeCoverageIDs := append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...)
	if len(activeCoverageIDs) == 0 {
		if len(coverageOrder) == 0 {
			t.Fatal("test semantic fixture has no coverage order")
		}
		activeCoverageIDs = []common.CoverageWorkItemID{coverageOrder[0]}
	}
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, orderDocument, 0, "test-semantic-request", activeCoverageIDs, coverageOrder, len(coverageOrder),
	)
	if err != nil {
		t.Fatalf("compile test semantic focus batch: %v", err)
	}
	for _, next := range []runtimecontract.RuntimeSemanticPlanBatchState{
		runtimecontract.RuntimeSemanticPlanBatchRequesting,
		runtimecontract.RuntimeSemanticPlanBatchDocumentComplete,
	} {
		batch, err = runtimecontract.AdvanceRuntimeSemanticPlanBatch(batch, next)
		if err != nil {
			t.Fatalf("advance test semantic focus batch: %v", err)
		}
	}
	selected := make(map[common.CoverageWorkItemID]struct{}, len(batch.CoverageWorkItemIDs))
	for _, id := range batch.CoverageWorkItemIDs {
		selected[id] = struct{}{}
	}
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID:  batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
	}
	feasibilities := make([]runtimecontract.RuntimeSemanticProofFeasibility, 0, len(batch.CoverageWorkItemIDs))
	for idx, item := range manifest.CoverageItems {
		if _, ok := selected[item.ID]; !ok {
			continue
		}
		target := fmt.Sprintf("fixture-semantic-%d.txt", idx+1)
		proposal := runtimecontract.RuntimeSemanticProofProposal{
			CoverageWorkItemID: item.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite,
			Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{target}},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			DeclarationRef: "test/semantic-fixture/" + string(item.ID),
		}
		document.Proposals = append(document.Proposals, proposal)
		feasibilities = append(feasibilities, runtimecontract.RuntimeSemanticProofFeasibility{
			SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
			ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
			CoverageWorkItemID: item.ID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal),
			OwnerModuleID: runtimecontract.RuntimeModuleFile, CatalogSnapshotID: catalog.SnapshotID, Status: runtimecontract.RuntimeSemanticFeasibilityFeasible,
			ObservedRevision: "test-semantic-revision", TargetRefs: []string{target},
		})
	}
	if len(document.Proposals) != len(batch.CoverageWorkItemIDs) {
		t.Fatal("test semantic focus proposals do not cover batch")
	}
	document = document.Normalized()
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatalf("validate test semantic proof document: %v", err)
	}
	batch.DocumentHash = runtimecontract.StableRuntimeSemanticPlanDocumentHash(document)
	batch.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(batch)
	for _, next := range []runtimecontract.RuntimeSemanticPlanBatchState{
		runtimecontract.RuntimeSemanticPlanBatchFeasibilityPending,
		runtimecontract.RuntimeSemanticPlanBatchFeasibilityComplete,
		runtimecontract.RuntimeSemanticPlanBatchAccepted,
	} {
		batch, err = runtimecontract.AdvanceRuntimeSemanticPlanBatch(batch, next)
		if err != nil {
			t.Fatalf("accept test semantic focus batch: %v", err)
		}
	}
	merged := document.Clone()
	_, reconciliation, err := runtimecontract.CompileRuntimeSemanticBindings(manifest, document, feasibilities)
	if err != nil {
		t.Fatalf("compile test semantic bindings: %v", err)
	}
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningSealed
	ledger.Manifest = &manifest
	ledger.CoverageOrderDocument = &orderDocument
	ledger.Batches = []runtimecontract.RuntimeSemanticPlanBatch{batch}
	ledger.BatchDocuments = []runtimecontract.RuntimeSemanticPlanDocument{document}
	ledger.CompletedBatchIDs = []string{batch.BatchID}
	ledger.NextBatchSequence = 1
	ledger.PendingCoverageIDs = append([]common.CoverageWorkItemID(nil), batch.PendingCoverageWorkItemIDs...)
	ledger.StablePrefixHash = runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	ledger.LogicalRequestID = "test-semantic-request"
	ledger.Document = &document
	ledger.DocumentHash = runtimecontract.StableRuntimeSemanticPlanDocumentHash(document)
	ledger.FeasibilityBatchID = runtimecontract.StableRuntimeSemanticFeasibilityBatchID(manifest, document)
	ledger.Feasibilities = feasibilities
	ledger.FeasibilitySetHash = runtimecontract.StableRuntimeSemanticFeasibilitySetHash(feasibilities)
	ledger.MergedDocument = &merged
	ledger.MergedDocumentHash = ledger.DocumentHash
	ledger.SealedBindingSetHash = reconciliation.BindingSetHash
	ledger.SealedLineageRefs = []string{manifest.Hash, runtimecontract.StableRuntimeSemanticPlanCoverageOrderHash(orderDocument), ledger.DocumentHash, ledger.FeasibilityBatchID, ledger.FeasibilitySetHash, ledger.SealedBindingSetHash}
	if err := ledger.Validate(); err != nil {
		t.Fatalf("build sealed semantic planning fixture: %v", err)
	}
	return ledger.Normalized()
}

// runtimeTestSemanticCoverageOrderForSupport builds a deterministic,
// dependency-valid provider-order fixture. It is test scaffolding only: the
// production Orchestrator accepts the provider-authored order and never
// derives one from this helper.
func runtimeTestSemanticCoverageOrderForSupport(seed runtimecontract.RuntimeDefinitionSeed) ([]common.CoverageWorkItemID, error) {
	items := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItem, len(seed.CoverageItems))
	for _, item := range seed.CoverageItems {
		items[item.ID] = item
	}
	state := make(map[common.CoverageWorkItemID]uint8, len(items))
	order := make([]common.CoverageWorkItemID, 0, len(items))
	var visit func(common.CoverageWorkItemID) error
	visit = func(id common.CoverageWorkItemID) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("test semantic coverage dependency cycle at %q", id)
		case 2:
			return nil
		}
		item, found := items[id]
		if !found {
			return fmt.Errorf("test semantic coverage dependency %q is not in seed", id)
		}
		state[id] = 1
		for _, prerequisiteID := range item.PrerequisiteCoverageWorkItemIDs {
			if err := visit(prerequisiteID); err != nil {
				return err
			}
		}
		state[id] = 2
		order = append(order, id)
		return nil
	}
	for _, item := range seed.CoverageItems {
		if err := visit(item.ID); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// runtimeCandidateRequestContentTestStore gives all Biz instances wired to
// one test EventHub the same opaque content repository. The production
// equivalent is process wiring to the durable repository; this helper keeps
// restart fixtures from replacing that repository while leaving raw provider
// documents subject to the normal C4/C5 persist-and-reload path.
func runtimeCandidateRequestContentTestStore(hub magicevent.Hub) repository.ArtifactContinuationRepository {
	if hub == nil {
		return memory.NewArtifactContinuationRepository()
	}
	store, _ := runtimeCandidateRequestContentTestStores.LoadOrStore(hub, memory.NewArtifactContinuationRepository())
	return store.(repository.ArtifactContinuationRepository)
}

// ensureRuntimeCandidatePreflightTestResponder makes test fakes state their
// rendered-request contract explicitly. Production always uses the LLM Block
// handler; this responder never calls a provider and only exists on test hubs
// that otherwise mock the candidate command directly.
func ensureRuntimeCandidatePreflightTestResponder(hub magicevent.Hub) {
	if hub == nil {
		return
	}
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	observer.Subscribe(llmevents.TopicPreflightRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(llmevents.PreflightRuntimeCandidateCommand)
		if !ok {
			return
		}
		feedback := runtimecontract.RuntimeFeedback{
			Domain: runtimecontract.RuntimeFeedbackDomainProviderPreflight, StableCategory: runtimecontract.RuntimeFeedbackCategoryAccepted,
			WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		}
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
			feedback.AuthoritySnapshotID = command.Transaction.AuthoritySnapshotID
		}
		result.Set(llmevents.PreflightRuntimeCandidateResult{Transaction: command.Transaction, DocumentKind: command.DocumentKind, Feedback: feedback}, nil)
	})
}

func newRuntimeTestOwner() *Biz {
	return newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{})
}

func newRuntimeTestOwnerWithEvidence(evidence RuntimeEvidenceRepositories) *Biz {
	return &Biz{evidence: evidence}
}

func (m *Biz) ConfigureRuntimeRepository(repository runtime.RuntimeStateRepository) error {
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		return err
	}
	m.runtimeController = controller
	return nil
}

func (m *Biz) RuntimeRepositoryConfigured() bool {
	return m != nil && m.runtimeController != nil
}

func (m *Biz) loadRuntime(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	if m == nil || m.runtimeController == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return m.runtimeController.Load(ctx, workspaceID, runID)
}

func (m *Biz) QueryRuntimeOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID, operationID, executionKey string, fence uint64, grant *runtimecontract.RuntimeOperationGovernanceGrant) (runtimecontract.RuntimeOperationReceipt, bool, error) {
	if m == nil || m.runtimeController == nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, common.ErrInvalidArgument
	}
	return m.runtimeController.QueryOperationReceipt(ctx, workspaceID, runID, dispatchID, operationID, executionKey, fence, grant)
}

func (m *Biz) EvidenceRepositories() RuntimeEvidenceRepositories {
	if m == nil {
		return RuntimeEvidenceRepositories{}
	}
	return m.evidence
}

func newTestBiz(owner *Biz, hub magicevent.Hub, background magictask.BackgroundRoutine) *Biz {
	ensureRuntimeCandidatePreflightTestResponder(hub)
	// C4/C5 fixtures need the same opaque content carrier supplied by
	// production wiring. This is only test setup: a complete scripted provider
	// document still must persist and be decoded via the request-session path.
	if owner == nil {
		owner = newRuntimeTestOwner()
	}
	if owner.evidence.ArtifactContinuations == nil {
		owner.evidence.ArtifactContinuations = runtimeCandidateRequestContentTestStore(hub)
	}
	owner.Base = basebiz.New(ModuleID, hub, background)
	return owner
}

// runtimeCandidateResultForTest is the only scripted-provider success
// builder for the orchestrator package. It deliberately encodes the provider
// wire schema, rather than marshaling RuntimeCandidate: RuntimeCandidate is
// an internal DTO and its field layout is not the provider response contract.
// C5 must consume DocumentContent just like production does; Candidate remains
// only as an optional assertion aid for test bodies.
func runtimeCandidateResultForTest(t *testing.T, command llmevents.RequestRuntimeCandidateCommand, candidate runtimecontract.RuntimeCandidate) llmevents.RequestRuntimeCandidateResult {
	t.Helper()
	candidate = bindRuntimeTestCandidateToManifest(command.CandidateManifest, candidate)
	document, err := runtimecontract.EncodeRuntimeCandidatePayloadWithManifest(command.CandidateManifest, candidate)
	if err != nil {
		// Never Fatal inside an EventHub observer: that leaves hub.Send blocked.
		// Surface the encode/membership failure as a typed complete rejection.
		return llmevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
			Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_contract_invalid",
				Signature: "candidate-contract-invalid", Reason: err.Error(),
			}),
		}
	}
	return llmevents.RequestRuntimeCandidateResult{
		Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
		ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
		DocumentOutcome:      runtimeCompleteDocumentOutcome(command.ExpectedDocument, document),
		DocumentContent:      document,
	}
}

func runtimeCompleteDocumentOutcome(expected *runtimecontract.RuntimeExpectedDocumentContract, document []byte) *runtimecontract.RuntimeProviderDocumentOutcome {
	return &runtimecontract.RuntimeProviderDocumentOutcome{
		Disposition:          runtimecontract.RuntimeProviderDocumentComplete,
		ExpectedDocumentHash: runtimeExpectedDocumentHash(expected),
		ResponseHash:         runtimecontract.StableRuntimeProviderResponseHash(document), ResponseBytes: int64(len(document)),
	}
}

func runtimeExpectedDocumentHash(expected *runtimecontract.RuntimeExpectedDocumentContract) string {
	if expected == nil {
		return ""
	}
	return expected.Normalized().Hash
}

// bindRuntimeTestCandidateToManifest builds the current provider wire shape
// for scripted test providers. Real provider JSON must name only sealed
// bindingRefs and raw CWI IDs remain a strict rejection.
func bindRuntimeTestCandidateToManifest(manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) runtimecontract.RuntimeCandidate {
	bind := func(operation *runtimecontract.RuntimeOperation) {
		if operation == nil || len(operation.CoverageWorkItemIDs) == 0 || len(operation.BindingRefs) != 0 {
			return
		}
		allowed := runtimeTestManifestBindingRefsForOperation(manifest, operation.Kind)
		if len(allowed) == 0 {
			return
		}
		refs := make([]string, 0, len(operation.CoverageWorkItemIDs))
		for _, coverageID := range operation.CoverageWorkItemIDs {
			for _, binding := range manifest.Bindings {
				if binding.CoverageWorkItemID != coverageID || !runtimeTestStringIncluded(allowed, binding.ID) {
					continue
				}
				refs = append(refs, binding.ID)
			}
		}
		refs = runtimeTestNormalizeStrings(refs)
		if len(refs) == 0 {
			return
		}
		operation.BindingRefs = refs
		operation.CoverageWorkItemIDs = nil
	}
	if candidate.StateChange != nil {
		for idx := range candidate.StateChange.Operations {
			bind(&candidate.StateChange.Operations[idx].Operation)
		}
	}
	if candidate.Validation != nil {
		for idx := range candidate.Validation.Operations {
			bind(&candidate.Validation.Operations[idx].Operation)
		}
	}
	return candidate
}

func runtimeTestManifestBindingRefsForOperation(manifest runtimecontract.RuntimeCandidateContractManifest, kind string) []string {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, operation := range manifest.Normalized().Operations {
		if operation.Kind == kind {
			return append([]string(nil), operation.AllowedBindingRefs...)
		}
	}
	return nil
}

func runtimeTestStringIncluded(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func runtimeTestNormalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

// runtimeTestBootstrapWithBindings upgrades an in-process fixture to the
// current schema's explicit proof shape. It is intentionally test-only: the
// production activation compiler must be the sole producer of bindings and
// fails closed when profile/template declarations are absent.
func runtimeTestBootstrapWithBindings(bootstrap runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	definitionRevision := bootstrap.Definition.Revision
	for focusIndex := range bootstrap.Focuses {
		focus := &bootstrap.Focuses[focusIndex]
		if len(focus.CoverageWorkItemIDs) == 0 || len(focus.CoverageEvidenceBindings) != 0 {
			continue
		}
		if len(focus.AcceptedStepKinds) == 0 {
			for _, kind := range focus.RequiredEvidenceKinds {
				switch kind {
				case runtimecontract.EvidenceKindStateChange:
					focus.AcceptedStepKinds = append(focus.AcceptedStepKinds, runtimecontract.StepKindFileEdit)
				case runtimecontract.EvidenceKindValidation:
					focus.AcceptedStepKinds = append(focus.AcceptedStepKinds, runtimecontract.RuntimeBlockKindManagedValidationSession)
				}
			}
		}
		if focus.ValidationRecipeID == "" {
			for _, kind := range focus.RequiredEvidenceKinds {
				if kind == runtimecontract.EvidenceKindValidation {
					// Direct in-process fixtures must name an exact validation
					// recipe just as activationcore requires in production. This is
					// test data, not an inferred production recipe.
					focus.ValidationRecipeID = "test-runtime-validation-recipe"
					break
				}
			}
		}
		proofs := make([]runtimecontract.CoverageEvidenceProof, 0, len(focus.RequiredEvidenceKinds))
		for _, kind := range focus.RequiredEvidenceKinds {
			proof := runtimecontract.CoverageEvidenceProof{EvidenceKind: kind}
			if kind == runtimecontract.EvidenceKindStateChange {
				proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}
			} else if kind == runtimecontract.EvidenceKindValidation {
				proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}
				proof.ValidationRecipeID = focus.ValidationRecipeID
				proof.RequiredCheckRefs = append([]string(nil), focus.RequiredCheckRefs...)
			}
			for _, operationKind := range focus.AcceptedStepKinds {
				if runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(operationKind, kind) {
					proof.AllowedOperationKinds = append(proof.AllowedOperationKinds, operationKind)
				}
			}
			proofs = append(proofs, proof)
		}
		bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(focus.CoverageWorkItemIDs))
		for _, coverageID := range focus.CoverageWorkItemIDs {
			binding := runtimecontract.CoverageEvidenceBinding{
				ID: "binding/test/" + focus.ID + "/" + string(coverageID), CoverageWorkItemID: coverageID,
				FocusID: focus.ID, DefinitionRevision: definitionRevision,
				Proofs:     append([]runtimecontract.CoverageEvidenceProof(nil), proofs...),
				TargetRefs: append([]string(nil), focus.TargetRefs...), TargetSelectorRef: focus.TargetSelectorRef,
				TargetSelectorOwnerRef: focus.TargetSelectorOwnerRef, TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef,
				ProspectiveTargetRef: focus.ProspectiveTargetRef, DeclarationRef: "test/runtime-fixture",
			}.Normalized()
			if len(binding.TargetRefs) == 0 && binding.TargetSelectorRef == "" && binding.ProspectiveTargetRef == "" {
				binding.TargetSelectorRef = "test/fixture-owner-selector"
				binding.TargetSelectorOwnerRef = "test-fixture"
				binding.TargetSelectorSubjectRef = focus.ID
			}
			if binding.TargetSelectorRef != "" && binding.TargetSelectorOwnerRef == "" {
				binding.TargetSelectorOwnerRef = "test-fixture"
			}
			if binding.TargetSelectorRef != "" && binding.TargetSelectorSubjectRef == "" {
				binding.TargetSelectorSubjectRef = focus.ID
			}
			binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
			bindings = append(bindings, binding)
		}
		focus.CoverageEvidenceBindings = runtimecontract.NormalizeCoverageEvidenceBindings(bindings)
	}
	for index := range bootstrap.CoverageItems {
		if bootstrap.CoverageItems[index].SourceScope == "" {
			bootstrap.CoverageItems[index].SourceScope = "test"
		}
		if bootstrap.CoverageItems[index].SourceID == "" {
			bootstrap.CoverageItems[index].SourceID = string(bootstrap.CoverageItems[index].ID)
		}
	}
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(bootstrap.Definition, bootstrap.Activation, bootstrap.CoverageItems, bootstrap.Slices)
	if err == nil {
		bootstrap.DefinitionSeed = seed
	}
	return bootstrap
}

// runtimeTestBootstrapWithReboundBindings is for fixtures that deliberately
// change a Focus proof declaration after starting from the shared bootstrap.
// A new proof declaration must get a new immutable binding; retaining the
// previous binding would model precisely the stale binding drift that V6
// rejects in production.
func runtimeTestBootstrapWithReboundBindings(bootstrap runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	for index := range bootstrap.Focuses {
		bootstrap.Focuses[index].CoverageEvidenceBindings = nil
	}
	return runtimeTestBootstrapWithBindings(bootstrap)
}

// runtimeTestFactsWithReboundBindings applies the same test-only declaration
// compiler to a direct RuntimeFacts fixture after that fixture changes its
// focus contract. Production facts never take this path: activation compiles
// bindings once, before the Run owner persists the bootstrap.
func runtimeTestFactsWithReboundBindings(facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext) runtimecontract.RuntimeFacts {
	bootstrap := runtimecontract.RuntimeBootstrap{
		Definition: definition,
		Focuses:    []runtimecontract.CoverageFocus{facts.Focus},
	}
	bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
	if len(bootstrap.Focuses) == 1 {
		facts.Focus = bootstrap.Focuses[0]
	}
	return facts
}

func runtimeCandidateDocumentForTest(t *testing.T, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) []byte {
	t.Helper()
	candidate = bindRuntimeTestCandidateToManifest(manifest, candidate)
	document, err := runtimecontract.EncodeRuntimeCandidatePayloadWithManifest(manifest, candidate)
	if err != nil {
		t.Fatalf("encode test provider document from sealed manifest: %v", err)
	}
	return document
}
