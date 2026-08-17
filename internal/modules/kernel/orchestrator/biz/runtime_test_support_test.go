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
	if aggregate.Facts.Resources.ProviderCeiling == nil {
		aggregate.Facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
			Selection:                aggregate.Facts.Activation.ProviderSelection,
			ContextWindowTokens:      128000,
			ProviderMaxOutputTokens:  16384,
			EffectiveMaxOutputTokens: 8192,
			MaxInputTokens:           119808,
			MaxBytes:                 128 * 1024,
			CapabilityRevision:       "capability-1",
			ConfigurationRevision:    "config-1",
		}
	}
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
	// Direct fixtures frequently mutate their source focus after constructing a
	// base aggregate. Re-project the immutable declaration before the fixture
	// enters production validation; production creates it exactly once in
	// RuntimeBootstrap.InitialFacts.
	queue := append([]runtimecontract.CoverageFocus{aggregate.Facts.Focus}, aggregate.Facts.RemainingFocuses...)
	queue = append(queue, aggregate.Facts.CompletedFocuses...)
	items := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItem, len(aggregate.Facts.DefinitionSeed.CoverageItems))
	for _, item := range aggregate.Facts.DefinitionSeed.CoverageItems {
		items[item.ID] = item
	}
	for index := range queue {
		if queue[index].SliceID == "" && len(queue[index].CoverageWorkItemIDs) == 1 {
			queue[index].SliceID = items[queue[index].CoverageWorkItemIDs[0]].SliceID
		}
	}
	aggregate.Facts.DefinitionFocuses = runtimecontract.CompileRuntimeDefinitionFocuses(queue)
	if aggregate.Facts.CWILedgerSchemaVersion == "" {
		// Test-only post-semantic fixtures begin from the same unbound activation
		// shape as production. Do not let an older fixture's hand-written binding
		// turn into an implicit activation authority.
		aggregate.Facts = runtimeTestFactsWithoutSemanticBindings(aggregate.Facts)
		initialized, err := runtimecontract.InitializeRuntimeCWILedgerForActivation(aggregate.Facts)
		if err != nil {
			t.Fatalf("initialize test canonical CWI ledger: %v", err)
		}
		aggregate.Facts = initialized
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
		aggregate.Facts = runtimeTestProjectSealedSemanticFacts(t, aggregate.Facts, aggregate.SemanticPlanning)
		facts, decision, err := runtime.ReduceRuntimeStateWithDirective(aggregate.Facts, nil)
		if err != nil {
			t.Fatalf("reduce test canonical semantic facts: %v", err)
		}
		aggregate.Facts = facts
		aggregate.Decision = decision
	}
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("test canonical semantic aggregate: %v", err)
	}
	return aggregate
}

func runtimeTestFactsWithoutSemanticBindings(facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	facts.Focus.CoverageEvidenceBindings = nil
	for index := range facts.RemainingFocuses {
		facts.RemainingFocuses[index].CoverageEvidenceBindings = nil
	}
	for index := range facts.CompletedFocuses {
		facts.CompletedFocuses[index].CoverageEvidenceBindings = nil
	}
	return facts.Normalized()
}

// runtimeTestProjectSealedSemanticFacts is test scaffolding for operation
// fixtures. It projects an already compiled semantic document through the
// same canonical fields that production's private seal transition writes;
// it is never a production activation or recovery path.
func runtimeTestProjectSealedSemanticFacts(t *testing.T, facts runtimecontract.RuntimeFacts, ledger runtimecontract.RuntimeSemanticPlanningLedger) runtimecontract.RuntimeFacts {
	t.Helper()
	if facts.CWILedgerSchemaVersion != runtimecontract.RuntimeCWILedgerSchemaCurrent || ledger.Manifest == nil || ledger.Document == nil {
		t.Fatalf("test semantic projection requires initialized ledger and complete document")
	}
	sourceFocuses := facts.DefinitionFocuses
	focuses, contract, err := runtimecontract.CompileRuntimeSemanticBindingsWithSourceFocuses(*ledger.Manifest, ledger.LogicalRequestID, *ledger.Document, ledger.Feasibilities, sourceFocuses)
	if err != nil {
		t.Fatalf("compile test semantic bindings for canonical projection: %v", err)
	}
	eligible := runtimecontract.RuntimeCWIsEligibleForPlanning(facts)
	if len(focuses) == 0 || len(focuses) != len(ledger.Document.BatchCoverageWorkItemIDs) || len(focuses) > len(eligible) {
		t.Fatalf("test semantic projection must bind the accepted batch exactly once: focuses=%d batch=%d eligible=%d", len(focuses), len(ledger.Document.BatchCoverageWorkItemIDs), len(eligible))
	}
	eligibleByID := make(map[common.CoverageWorkItemID]struct{}, len(eligible))
	for _, coverageID := range eligible {
		eligibleByID[coverageID] = struct{}{}
	}
	sealedByID := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(focuses))
	coverageIndex := make(map[common.CoverageWorkItemID]int, len(facts.Coverage))
	for index, item := range facts.Coverage {
		coverageIndex[item.ID] = index
	}
	sourceByCoverage := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(facts.Coverage))
	for _, source := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		source = source.Normalized()
		for _, coverageID := range source.CoverageWorkItemIDs {
			sourceByCoverage[coverageID] = source
		}
	}
	for focusIndex := range focuses {
		focus := focuses[focusIndex].Normalized()
		if len(focus.CoverageWorkItemIDs) != 1 || len(focus.CoverageEvidenceBindings) != 1 {
			t.Fatalf("test semantic focus is not one-CWI canonical projection: %+v", focus)
		}
		coverageID := focus.CoverageWorkItemIDs[0]
		if _, eligible := eligibleByID[coverageID]; !eligible {
			t.Fatalf("test semantic projection binds ineligible coverage %q", coverageID)
		}
		if _, duplicate := sealedByID[coverageID]; duplicate {
			t.Fatalf("test semantic projection duplicates coverage %q", coverageID)
		}
		if source, found := sourceByCoverage[coverageID]; found {
			// Semantic binding owns the executable proof. The original focus still
			// owns profile-declared target discovery context, which remains a
			// prerequisite input rather than a binding or credit authority.
			focus.TargetRole = source.TargetRole
			focus.TargetDiscoverySources = append([]string(nil), source.TargetDiscoverySources...)
			focus.TargetSelectorOwnerRef = source.TargetSelectorOwnerRef
			focus.TargetSelectorSubjectRef = source.TargetSelectorSubjectRef
			focus.TargetSelectorCandidateRefs = append([]string(nil), source.TargetSelectorCandidateRefs...)
			focus.RequireTargetGrounding = source.RequireTargetGrounding
			focus = focus.Normalized()
		}
		index, found := coverageIndex[coverageID]
		if !found {
			t.Fatalf("test semantic focus references unknown coverage %q", coverageID)
		}
		binding := focus.CoverageEvidenceBindings[0]
		ref := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, ledger.Manifest.Hash)
		item := &facts.Coverage[index]
		item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateBound
		item.CurrentBindingRef = &ref
		item.BindingLineage = append(item.BindingLineage, ref)
		item.FocusID = focus.ID
		if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			item.State = runtimecontract.RuntimeCoverageItemStateOpen
		}
		// The projected queue below uses focuses, not sealedByID. Keep the
		// source-declared grounding facts on that exact focus instance so the
		// normal prerequisite dispatcher can acquire File's selection.
		focuses[focusIndex] = focus
		sealedByID[coverageID] = focus
	}
	pendingByID := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(eligible)-len(sealedByID))
	for _, coverageID := range eligible {
		if _, sealed := sealedByID[coverageID]; sealed {
			continue
		}
		source, found := sourceByCoverage[coverageID]
		if !found {
			t.Fatalf("test semantic projection has no source focus for pending coverage %q", coverageID)
		}
		source.ID = "semantic-pending/" + string(coverageID)
		source.CoverageWorkItemIDs = []common.CoverageWorkItemID{coverageID}
		source.CoverageEvidenceBindings = nil
		pendingByID[coverageID] = source.Normalized()
		index := coverageIndex[coverageID]
		item := &facts.Coverage[index]
		if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			t.Fatalf("test semantic projection cannot return settled coverage %q to pending", coverageID)
		}
		item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStatePlanning
		item.CurrentBindingRef = nil
	}
	queue := append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...)
	retained := make([]runtimecontract.CoverageFocus, 0, len(queue))
	currentReplaced := false
	for queueIndex, source := range queue {
		source = source.Normalized()
		remainingIDs := make([]common.CoverageWorkItemID, 0, len(source.CoverageWorkItemIDs))
		for _, coverageID := range source.CoverageWorkItemIDs {
			if _, replaced := sealedByID[coverageID]; replaced {
				if queueIndex == 0 {
					currentReplaced = true
				}
				continue
			}
			if _, replaced := pendingByID[coverageID]; replaced {
				continue
			}
			remainingIDs = append(remainingIDs, coverageID)
		}
		if len(remainingIDs) == 0 {
			continue
		}
		source.CoverageWorkItemIDs = remainingIDs
		bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(source.CoverageEvidenceBindings))
		for _, binding := range source.CoverageEvidenceBindings {
			if _, sealed := sealedByID[binding.CoverageWorkItemID]; sealed {
				continue
			}
			if _, pending := pendingByID[binding.CoverageWorkItemID]; !pending {
				bindings = append(bindings, binding)
			}
		}
		source.CoverageEvidenceBindings = bindings
		retained = append(retained, source.Normalized())
	}
	if !currentReplaced {
		t.Fatalf("test semantic projection does not bind the current eligible focus")
	}
	pending := make([]runtimecontract.CoverageFocus, 0, len(pendingByID))
	for _, coverageID := range eligible {
		if source, found := pendingByID[coverageID]; found {
			pending = append(pending, source)
		}
	}
	projected := append(append(append([]runtimecontract.CoverageFocus(nil), focuses...), pending...), retained...)
	if len(projected) == 0 {
		t.Fatal("test semantic projection has no current focus")
	}
	facts.Focus = projected[0]
	facts.RemainingFocuses = append([]runtimecontract.CoverageFocus(nil), projected[1:]...)

	focusByCoverage := make(map[common.CoverageWorkItemID]string, len(facts.Coverage))
	boundFocuses := make([]runtimecontract.CoverageFocus, 0, len(projected)+len(facts.CompletedFocuses))
	for _, source := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		source = source.Normalized()
		for _, coverageID := range source.CoverageWorkItemIDs {
			if previous, duplicate := focusByCoverage[coverageID]; duplicate {
				t.Fatalf("test semantic projection duplicates coverage %q in focus %q and %q", coverageID, previous, source.ID)
			}
			focusByCoverage[coverageID] = source.ID
		}
		if len(source.CoverageEvidenceBindings) != 0 {
			boundFocuses = append(boundFocuses, source)
		}
	}
	for index := range facts.Coverage {
		focusID, found := focusByCoverage[facts.Coverage[index].ID]
		if !found {
			t.Fatalf("test semantic projection leaves coverage %q without a focus", facts.Coverage[index].ID)
		}
		facts.Coverage[index].FocusID = focusID
	}
	if len(boundFocuses) == len(focuses) && len(facts.CompletedFocuses) == 0 {
		facts.ReconciliationContract = contract
	} else {
		currentContract, compileErr := runtimecontract.CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, boundFocuses)
		if compileErr != nil {
			t.Fatalf("compile test canonical reconciliation contract: %v", compileErr)
		}
		facts.ReconciliationContract = currentContract
	}
	// Mirror the production semantic-seal boundary: continuations that were
	// derived for the pre-seal focus cannot remain authority after SM-2 replaces
	// that focus with its CWI-scoped semantic projection. The subsequent reducer
	// reconstructs any still-applicable repair directive for the new focus.
	facts.RepairDirective = nil
	facts.Feasibility = nil
	facts.Failure = nil
	return facts.Normalized()
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
	return runtimeTestPersistSealedSemanticAuthorityWithProviderCeiling(t, ctx, repository, aggregate, true)
}

func runtimeTestPersistSealedSemanticAuthorityWithoutProviderCeiling(
	t *testing.T,
	ctx context.Context,
	repository runtime.RuntimeStateRepository,
	aggregate runtime.RuntimeAggregate,
) runtime.RuntimeAggregate {
	return runtimeTestPersistSealedSemanticAuthorityWithProviderCeiling(t, ctx, repository, aggregate, false)
}

func runtimeTestPersistSealedSemanticAuthorityWithProviderCeiling(
	t *testing.T,
	ctx context.Context,
	repository runtime.RuntimeStateRepository,
	aggregate runtime.RuntimeAggregate,
	retainProviderCeiling bool,
) runtime.RuntimeAggregate {
	t.Helper()
	if repository == nil {
		t.Fatal("runtime test semantic authority repository is nil")
	}
	next := runtimeTestAggregateWithSemanticPlanning(t, aggregate)
	if !retainProviderCeiling {
		next.Facts.Resources.ProviderCeiling = nil
	}
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
type runtimeTestSemanticFixtureSpec struct {
	proposal         runtimecontract.RuntimeSemanticProofProposal
	feasibilityRefs  []string
	validationChecks []string
	selectorRefs     []string
	constraintRefs   []string
	targetRefs       []string
}

// runtimeTestSealedSemanticPlanning builds a minimal valid semantic fact set
// from the fixture's declared CWI focus. It intentionally does not invent a
// File-only target or operation: post-semantic runtime tests need their
// original owner, kind, target policy and validation facts to remain intact.
func runtimeTestSealedSemanticPlanning(t *testing.T, aggregate runtime.RuntimeAggregate, ledger runtimecontract.RuntimeSemanticPlanningLedger) runtimecontract.RuntimeSemanticPlanningLedger {
	t.Helper()
	eligible := runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts)
	if len(eligible) == 0 {
		t.Fatal("test semantic fixture has no CWI eligible for planning")
	}
	fixtureSpecs := runtimeTestSemanticFixtureSpecs(t, aggregate, eligible)
	catalogs := runtimeTestSemanticFixtureCatalogs(t, fixtureSpecs)
	coverageItems := runtimeTestSemanticFixtureCoverageItems(t, aggregate.Facts.DefinitionSeed, eligible)
	operationContractRevision := aggregate.Facts.Activation.ExecutionContract.OperationContractRevision
	if operationContractRevision == "" {
		operationContractRevision = runtimecontract.RuntimeOperationContractRevision()
	}
	allowedOwners := make([]string, 0, len(catalogs))
	allowedKinds := make([]string, 0, len(catalogs))
	allowedPolicies := make([]runtimecontract.RuntimeOperationTargetPolicy, 0, len(catalogs))
	for _, spec := range fixtureSpecs {
		allowedOwners = append(allowedOwners, spec.proposal.OwnerModuleID)
		allowedKinds = append(allowedKinds, spec.proposal.OperationKind)
		allowedPolicies = append(allowedPolicies, spec.proposal.Target.Policy)
	}
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DefinitionRevision: aggregate.Facts.DefinitionRevision, DefinitionSeedHash: aggregate.Facts.DefinitionSeed.Hash,
		OperationContractRevision: operationContractRevision,
		CoverageItems:             coverageItems,
		AllowedOwnerModuleIDs:     runtimeTestNormalizeStrings(allowedOwners),
		AllowedOperationKinds:     runtimeTestNormalizeStrings(allowedKinds),
		AllowedTargetPolicies:     runtimeTestNormalizeTargetPolicies(allowedPolicies),
		OwnerCatalogs:             catalogs,
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	coverageOrder, err := runtimeTestSemanticCoverageOrderForItems(coverageItems)
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
	activeCoverageIDs := runtimeTestSemanticActiveCoverageIDs(aggregate.Facts, eligible)
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
	specsByCoverage := make(map[common.CoverageWorkItemID][]runtimeTestSemanticFixtureSpec, len(fixtureSpecs))
	for _, spec := range fixtureSpecs {
		coverageID := spec.proposal.CoverageWorkItemID
		specsByCoverage[coverageID] = append(specsByCoverage[coverageID], spec)
	}
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID:  batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
	}
	feasibilities := make([]runtimecontract.RuntimeSemanticProofFeasibility, 0, len(fixtureSpecs))
	for _, coverageID := range batch.CoverageWorkItemIDs {
		specs, found := specsByCoverage[coverageID]
		if !found {
			t.Fatalf("test semantic batch references unknown fixture coverage %q", coverageID)
		}
		for _, spec := range specs {
			proposal := spec.proposal
			document.Proposals = append(document.Proposals, proposal)
			catalog := runtimeTestSemanticFixtureCatalogForOwner(t, catalogs, proposal.OwnerModuleID)
			feasibilities = append(feasibilities, runtimecontract.RuntimeSemanticProofFeasibility{
				SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
				ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
				CoverageWorkItemID: coverageID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal),
				OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID, Status: runtimecontract.RuntimeSemanticFeasibilityFeasible,
				ObservedRevision: catalog.ObservedRevision, TargetRefs: append([]string(nil), spec.feasibilityRefs...),
				ValidationRecipeID: proposal.ValidationRecipeID, RequiredCheckRefs: append([]string(nil), spec.validationChecks...),
			})
		}
	}
	if len(document.Proposals) < len(batch.CoverageWorkItemIDs) {
		t.Fatal("test semantic focus proposals do not cover every batch CWI")
	}
	document = document.Normalized()
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatalf("validate test semantic proof document: %v policies=%v proposals=%+v", err, manifest.AllowedTargetPolicies, document.Proposals)
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
	sourceFocuses := append([]runtimecontract.CoverageFocus{aggregate.Facts.Focus}, aggregate.Facts.RemainingFocuses...)
	sourceFocuses = append(sourceFocuses, aggregate.Facts.CompletedFocuses...)
	_, reconciliation, err := runtimecontract.CompileRuntimeSemanticBindingsWithSourceFocuses(manifest, "test-semantic-request", document, feasibilities, sourceFocuses)
	if err != nil {
		t.Fatalf("compile test semantic bindings: %v proposals=%+v", err, document.Proposals)
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

func runtimeTestSemanticFixtureSpecs(t *testing.T, aggregate runtime.RuntimeAggregate, eligible []common.CoverageWorkItemID) []runtimeTestSemanticFixtureSpec {
	t.Helper()
	sources := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(aggregate.Facts.Coverage))
	for _, source := range append(append([]runtimecontract.CoverageFocus{aggregate.Facts.Focus}, aggregate.Facts.RemainingFocuses...), aggregate.Facts.CompletedFocuses...) {
		source = source.Normalized()
		for _, coverageID := range source.CoverageWorkItemIDs {
			if previous, duplicate := sources[coverageID]; duplicate {
				t.Fatalf("test semantic fixture duplicates coverage %q in focus %q and %q", coverageID, previous.ID, source.ID)
			}
			sources[coverageID] = source
		}
	}
	specs := make([]runtimeTestSemanticFixtureSpec, 0, len(eligible)*2)
	for _, coverageID := range eligible {
		source, found := sources[coverageID]
		if !found {
			t.Fatalf("test semantic fixture has no source focus for coverage %q", coverageID)
		}
		evidenceKinds, required := runtimeTestSemanticFixtureEvidenceKinds(source)
		for _, evidenceKind := range evidenceKinds {
			kind, ok := runtimeTestSemanticFixtureOperation(aggregate, source, evidenceKind)
			if !ok {
				if required {
					t.Fatalf("test semantic fixture coverage %q has no declared operation for required evidence %q", coverageID, evidenceKind)
				}
				continue
			}
			descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
			if !found {
				t.Fatalf("test semantic fixture operation %q has no descriptor", kind)
			}
			variantID := runtimeTestSemanticFixtureVariant(t, source, kind)
			target, feasibilityRefs, selectorRefs, constraintRefs, catalogTargets := runtimeTestSemanticFixtureTarget(aggregate.Facts, source, coverageID, evidenceKind)
			proposal := runtimecontract.RuntimeSemanticProofProposal{
				CoverageWorkItemID: coverageID,
				OwnerModuleID:      descriptor.OwnerModuleID,
				OperationKind:      kind,
				OperationVariantID: variantID,
				Target:             target,
				RequiredEvidenceKinds: []string{
					evidenceKind,
				},
				DeclarationRef: "test/semantic-fixture/" + string(coverageID),
			}
			checks := []string(nil)
			switch evidenceKind {
			case runtimecontract.EvidenceKindStateChange:
				proposal.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}
			case runtimecontract.EvidenceKindValidation:
				proposal.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}
				proposal.ValidationRecipeID = source.ValidationRecipeID
				if proposal.ValidationRecipeID == "" {
					proposal.ValidationRecipeID = "test-runtime-validation-recipe"
				}
				checks = append([]string(nil), source.RequiredCheckRefs...)
				if len(checks) == 0 {
					checks = []string{proposal.ValidationRecipeID}
				}
				proposal.RequiredCheckRefs = append([]string(nil), checks...)
			default:
				t.Fatalf("test semantic fixture selected unsupported evidence kind %q", evidenceKind)
			}
			specs = append(specs, runtimeTestSemanticFixtureSpec{
				proposal: proposal.Normalized(), feasibilityRefs: feasibilityRefs,
				validationChecks: checks, selectorRefs: selectorRefs, constraintRefs: constraintRefs,
				targetRefs: catalogTargets,
			})
		}
	}
	if len(specs) == 0 {
		t.Fatal("test semantic fixture has no proof proposals")
	}
	return specs
}

func runtimeTestSemanticFixtureEvidenceKinds(source runtimecontract.CoverageFocus) ([]string, bool) {
	source = source.Normalized()
	ret := make([]string, 0, 2)
	for _, evidenceKind := range []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation} {
		if runtimeTestStringIncluded(source.RequiredEvidenceKinds, evidenceKind) {
			ret = append(ret, evidenceKind)
		}
	}
	if len(ret) != 0 {
		return ret, true
	}
	// A source focus without an independent proof declaration still needs one
	// bounded fixture proof. Prefer a state change and fall back to validation
	// only when that is the sole declared capability.
	return []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}, false
}

func runtimeTestSemanticFixtureOperation(aggregate runtime.RuntimeAggregate, source runtimecontract.CoverageFocus, evidenceKind string) (string, bool) {
	source = source.Normalized()
	candidates := append([]string(nil), source.AcceptedStepKinds...)
	if len(candidates) == 0 {
		candidates = append(candidates, aggregate.Facts.Capabilities.StateChangeKinds...)
		candidates = append(candidates, aggregate.Facts.Capabilities.ValidationKinds...)
		candidates = append(candidates, aggregate.Definition.CapabilityContract...)
	}
	candidates = runtimeTestNormalizeStrings(candidates)
	for _, candidate := range candidates {
		kind := runtimecontract.CanonicalRuntimeBlockKind(candidate)
		if kind == "" || !runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(kind, evidenceKind) {
			continue
		}
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
		if !found || descriptor.OwnerModuleID == "" {
			continue
		}
		return kind, true
	}
	return "", false
}

func runtimeTestSemanticFixtureVariant(t *testing.T, source runtimecontract.CoverageFocus, kind string) string {
	t.Helper()
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, ref := range source.Normalized().AcceptedOperationVariants {
		if ref.Kind == kind {
			return ref.VariantID
		}
	}
	contract, found := runtimecontract.RuntimeOperationContractForKind(kind)
	if !found || len(contract.Variants) == 0 {
		return ""
	}
	t.Fatalf("test semantic fixture operation %q has static variants but source focus %q does not declare one", kind, source.ID)
	return ""
}

func runtimeTestSemanticFixtureTarget(
	facts runtimecontract.RuntimeFacts,
	source runtimecontract.CoverageFocus,
	coverageID common.CoverageWorkItemID,
	evidenceKind string,
) (runtimecontract.RuntimeOperationTargetAuthorization, []string, []string, []string, []string) {
	source = source.Normalized()
	targets := append([]string(nil), source.TargetRefs...)
	if len(targets) == 0 && facts.Resources.TargetSelection != nil {
		targets = append(targets, facts.Resources.TargetSelection.TargetRefs...)
	}
	selector := source.TargetSelectorRef
	if selector == "" {
		selector = "test/fixture-owner-selector/" + string(coverageID)
	}
	if source.ProspectiveTargetRef != "" && evidenceKind == runtimecontract.EvidenceKindStateChange {
		constraint := "test/fixture-prospective-target/" + string(coverageID)
		return runtimecontract.RuntimeOperationTargetAuthorization{
			Policy:                    runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			OwnerSelectorRefs:         []string{selector},
			ProspectiveConstraintRefs: []string{constraint},
			ProspectiveTargetRef:      source.ProspectiveTargetRef,
		}, []string{source.ProspectiveTargetRef}, []string{selector}, []string{constraint}, nil
	}
	if source.ProspectiveTargetRef != "" && evidenceKind == runtimecontract.EvidenceKindValidation {
		return runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{source.ProspectiveTargetRef},
		}, []string{source.ProspectiveTargetRef}, nil, nil, []string{source.ProspectiveTargetRef}
	}
	if len(targets) == 0 && source.TargetSelectorRef != "" {
		return runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{selector},
		}, nil, []string{selector}, nil, nil
	}
	if len(targets) == 0 {
		targets = append(targets, source.TargetSelectorCandidateRefs...)
	}
	if len(targets) != 0 {
		return runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: targets,
		}, targets, nil, nil, targets
	}
	return runtimecontract.RuntimeOperationTargetAuthorization{
		Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{selector},
	}, nil, []string{selector}, nil, nil
}

func runtimeTestSemanticFixtureCatalogs(t *testing.T, specs []runtimeTestSemanticFixtureSpec) []runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	byOwner := make(map[string]*runtimecontract.RuntimeOperationAdmissibility, len(specs))
	order := make([]string, 0, len(specs))
	variantIndex := make(map[string]map[string]int, len(specs))
	for _, spec := range specs {
		proposal := spec.proposal.Normalized()
		catalog := byOwner[proposal.OwnerModuleID]
		if catalog == nil {
			catalog = &runtimecontract.RuntimeOperationAdmissibility{
				PrerequisiteID:   "test-semantic-" + proposal.OwnerModuleID,
				OwnerModuleID:    proposal.OwnerModuleID,
				ObservedRevision: "test-semantic-revision-" + proposal.OwnerModuleID,
			}
			byOwner[proposal.OwnerModuleID] = catalog
			order = append(order, proposal.OwnerModuleID)
			variantIndex[proposal.OwnerModuleID] = make(map[string]int)
		}
		catalog.OperationKinds = append(catalog.OperationKinds, proposal.OperationKind)
		catalog.TargetRefs = append(catalog.TargetRefs, spec.targetRefs...)
		catalog.SelectorRefs = append(catalog.SelectorRefs, spec.selectorRefs...)
		catalog.ConstraintRefs = append(catalog.ConstraintRefs, spec.constraintRefs...)
		if proposal.ValidationRecipeID != "" {
			catalog.ValidationRecipeRefs = append(catalog.ValidationRecipeRefs, proposal.ValidationRecipeID)
			catalog.ValidationRecipeRefs = append(catalog.ValidationRecipeRefs, spec.validationChecks...)
		}
		// Publish one explicit policy fact per kind/variant. This matters when a
		// single owner catalog mixes a discriminated kind (for example patch/apply)
		// with a non-variant kind: once any variant facts exist, the broad catalog
		// union is vocabulary only and cannot authorize the other kind.
		key := proposal.OperationKind + "|" + proposal.OperationVariantID
		if index, found := variantIndex[proposal.OwnerModuleID][key]; found {
			catalog.OperationVariants[index].AllowedTargetPolicies = append(catalog.OperationVariants[index].AllowedTargetPolicies, proposal.Target.Policy)
			continue
		}
		catalog.OperationVariants = append(catalog.OperationVariants, runtimecontract.RuntimeOperationAdmissibilityVariant{
			Kind: proposal.OperationKind, VariantID: proposal.OperationVariantID,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{proposal.Target.Policy},
			SelectorRefs:          append([]string(nil), spec.selectorRefs...),
			ConstraintRefs:        append([]string(nil), spec.constraintRefs...),
		})
		variantIndex[proposal.OwnerModuleID][key] = len(catalog.OperationVariants) - 1
	}
	ret := make([]runtimecontract.RuntimeOperationAdmissibility, 0, len(order))
	for _, owner := range order {
		catalog := byOwner[owner]
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
		if err := catalog.Validate(); err != nil {
			t.Fatalf("build test semantic owner catalog %q: %v", owner, err)
		}
		ret = append(ret, catalog.Normalized())
	}
	return ret
}

func runtimeTestSemanticFixtureCatalogForOwner(t *testing.T, catalogs []runtimecontract.RuntimeOperationAdmissibility, owner string) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	for _, catalog := range catalogs {
		if catalog.OwnerModuleID == owner {
			return catalog
		}
	}
	t.Fatalf("test semantic fixture has no catalog for owner %q", owner)
	return runtimecontract.RuntimeOperationAdmissibility{}
}

func runtimeTestSemanticFixtureCoverageItems(t *testing.T, seed runtimecontract.RuntimeDefinitionSeed, eligible []common.CoverageWorkItemID) []runtimecontract.RuntimeCoverageItem {
	t.Helper()
	wanted := make(map[common.CoverageWorkItemID]struct{}, len(eligible))
	for _, coverageID := range eligible {
		wanted[coverageID] = struct{}{}
	}
	items := make([]runtimecontract.RuntimeCoverageItem, 0, len(eligible))
	for _, item := range seed.CoverageItems {
		if _, found := wanted[item.ID]; !found {
			continue
		}
		items = append(items, item)
		delete(wanted, item.ID)
	}
	if len(wanted) != 0 {
		for coverageID := range wanted {
			t.Fatalf("test semantic fixture eligible coverage %q is absent from the definition seed", coverageID)
		}
	}
	return items
}

func runtimeTestSemanticActiveCoverageIDs(facts runtimecontract.RuntimeFacts, eligible []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	eligibleByID := make(map[common.CoverageWorkItemID]struct{}, len(eligible))
	for _, coverageID := range eligible {
		eligibleByID[coverageID] = struct{}{}
	}
	ret := make([]common.CoverageWorkItemID, 0, len(facts.Focus.CoverageWorkItemIDs))
	for _, coverageID := range facts.Focus.Normalized().CoverageWorkItemIDs {
		if _, found := eligibleByID[coverageID]; found {
			ret = append(ret, coverageID)
		}
	}
	return ret
}

func runtimeTestNormalizeTargetPolicies(values []runtimecontract.RuntimeOperationTargetPolicy) []runtimecontract.RuntimeOperationTargetPolicy {
	seen := make(map[runtimecontract.RuntimeOperationTargetPolicy]struct{}, len(values))
	ret := make([]runtimecontract.RuntimeOperationTargetPolicy, 0, len(values))
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

// runtimeTestSemanticCoverageOrderForSupport builds a deterministic,
// dependency-valid provider-order fixture. It is test scaffolding only: the
// production Orchestrator accepts the provider-authored order and never
// derives one from this helper.
func runtimeTestSemanticCoverageOrderForSupport(seed runtimecontract.RuntimeDefinitionSeed) ([]common.CoverageWorkItemID, error) {
	return runtimeTestSemanticCoverageOrderForItems(seed.CoverageItems)
}

func runtimeTestSemanticCoverageOrderForItems(values []runtimecontract.RuntimeCoverageItem) ([]common.CoverageWorkItemID, error) {
	items := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeCoverageItem, len(values))
	for _, item := range values {
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
	for _, item := range values {
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
			refs := make([]string, 0)
			for _, authorization := range operation.BindingAuthorizations {
				refs = append(refs, authorization.AllowedBindingRefs...)
			}
			return runtimeTestNormalizeStrings(refs)
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

// runtimeTestBootstrapWithBindings prepares only the declarative input that a
// semantic plan may later bind. The historical name is retained for existing
// test callers, but a fresh bootstrap intentionally carries no proof binding.
func runtimeTestBootstrapWithBindings(bootstrap runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	for focusIndex := range bootstrap.Focuses {
		focus := &bootstrap.Focuses[focusIndex]
		if len(focus.CoverageWorkItemIDs) == 0 {
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
		focus.CoverageEvidenceBindings = nil
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

// runtimeTestBootstrapWithReboundBindings clears any obsolete test-only proof
// data. The later semantic fixture seal creates the replacement binding.
func runtimeTestBootstrapWithReboundBindings(bootstrap runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	for index := range bootstrap.Focuses {
		bootstrap.Focuses[index].CoverageEvidenceBindings = nil
	}
	return runtimeTestBootstrapWithBindings(bootstrap)
}

// runtimeTestFactsWithReboundBindings keeps direct post-semantic fixtures
// explicit. Production never calls this helper: its binding comes only from
// the Orchestrator semantic seal.
func runtimeTestFactsWithReboundBindings(facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext) runtimecontract.RuntimeFacts {
	focus := facts.Focus.Normalized()
	definitionRevision := definition.Normalized().Revision
	if definitionRevision == "" {
		definitionRevision = facts.DefinitionRevision
	}
	if definitionRevision == "" {
		definitionRevision = "test-definition"
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
				focus.ValidationRecipeID = "test-runtime-validation-recipe"
				break
			}
		}
	}
	proofs := make([]runtimecontract.CoverageEvidenceProof, 0, len(focus.RequiredEvidenceKinds))
	for _, kind := range focus.RequiredEvidenceKinds {
		proof := runtimecontract.CoverageEvidenceProof{EvidenceKind: kind}
		switch kind {
		case runtimecontract.EvidenceKindStateChange:
			proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}
		case runtimecontract.EvidenceKindValidation:
			proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}
			proof.ValidationRecipeID = focus.ValidationRecipeID
			proof.RequiredCheckRefs = append([]string(nil), focus.RequiredCheckRefs...)
		}
		for _, operationKind := range focus.AcceptedStepKinds {
			if runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(operationKind, kind) {
				proof.AllowedOperationKinds = append(proof.AllowedOperationKinds, operationKind)
			}
		}
		proofs = append(proofs, proof.Normalized())
	}
	bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(focus.CoverageWorkItemIDs))
	for _, coverageID := range focus.CoverageWorkItemIDs {
		binding := runtimecontract.CoverageEvidenceBinding{
			ID: "binding/test/" + focus.ID + "/" + string(coverageID), CoverageWorkItemID: coverageID,
			FocusID: focus.ID, DefinitionRevision: definitionRevision,
			Proofs:     append([]runtimecontract.CoverageEvidenceProof(nil), proofs...),
			TargetRefs: append([]string(nil), focus.TargetRefs...), TargetSelectorRef: focus.TargetSelectorRef,
			TargetSelectorOwnerRef: focus.TargetSelectorOwnerRef, TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef,
			TargetSelectorCandidateRefs: append([]string(nil), focus.TargetSelectorCandidateRefs...),
			ProspectiveTargetRef:        focus.ProspectiveTargetRef, DeclarationRef: "test/runtime-fixture",
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
	facts.Focus = focus
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
