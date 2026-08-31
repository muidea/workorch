package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeTestCommitAcceptedReceiptEvidence advances a persisted accepted owner
// receipt through the same Orchestrator evidence transition used in production.
// Multi-operation governance fixtures remain on their active dispatch until
// the sibling operations have also reached a terminal evidence state.
func runtimeTestCommitAcceptedReceiptEvidence(t *testing.T, aggregate *RuntimeAggregate, operationID string) {
	t.Helper()
	if aggregate == nil || aggregate.ActiveDispatch == nil {
		t.Fatal("accepted receipt projection fixture has no active dispatch")
	}
	dispatch := aggregate.ActiveDispatch
	executionIndex := -1
	for index := range dispatch.OperationExecutions {
		if dispatch.OperationExecutions[index].OperationID == operationID {
			executionIndex = index
			break
		}
	}
	if executionIndex < 0 {
		t.Fatalf("accepted receipt projection has no execution for %q", operationID)
	}
	execution := dispatch.OperationExecutions[executionIndex]
	if execution.State != runtimecontract.OperationExecutionOwnerCommitted {
		t.Fatalf("accepted receipt projection execution state = %q", execution.State)
	}
	receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
		t.Fatalf("accepted receipt projection receipt = %+v found=%t", receipt, found)
	}
	operation, found := candidateOperationByID(dispatch.Candidate, operationID)
	if !found {
		t.Fatalf("accepted receipt projection has no operation %q", operationID)
	}
	repository := &durableRuntimeStateRepository{aggregate: *aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatalf("initialize accepted receipt projection controller: %v", err)
	}
	evidence, err := operationEvidenceFromPersistedReceipt(*aggregate, *dispatch, operation, receipt)
	if err != nil {
		t.Fatalf("derive accepted receipt projection: %v", err)
	}
	committed, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:        RuntimeInternalEventOperationEvidenceObserved,
		Scope:       RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Transaction: &RuntimeInternalTransactionPayload{ExecutionEvidence: &evidence},
	})
	if err != nil {
		t.Fatalf("commit accepted receipt projection: %v", err)
	}
	*aggregate = committed.Aggregate
}

// runtimeTestEnsureCurrentFocusBindings seals direct reducer fixtures with the
// same explicit binding shape produced by activationcore in production.
func runtimeTestEnsureCurrentFocusBindings(t *testing.T, aggregate *RuntimeAggregate) {
	t.Helper()
	if aggregate == nil {
		t.Fatal("nil Runtime aggregate fixture")
	}
	aggregate.Facts = runtimeTestFactsWithManagedValidationOwnerPreflight(aggregate.Facts)
	runtimeTestSealCurrentPolicies(&aggregate.Facts)
	aggregate.Facts = aggregate.Facts.Normalized()
	if aggregate.ExecutionBudget.Policy.PolicyRevision == "" {
		ledger, err := runtimecontract.NewRuntimeExecutionBudgetLedger(aggregate.Facts.ExecutionBudgetPolicy)
		if err != nil {
			t.Fatalf("initialize test execution budget: %v", err)
		}
		aggregate.ExecutionBudget = ledger
	}
	runtimeTestSealCurrentFocusBindings(t, aggregate)
	runtimeTestEnsureSemanticPlanningLedger(t, aggregate)
	provisionalFocusID := aggregate.Facts.Focus.ID
	runtimeTestProjectSemanticBindingPreimages(t, aggregate)
	bindCurrentFocusTargetSelection(&aggregate.Facts)
	aggregate.Facts = runtimeTestRebindSyntheticFocusScopedFacts(aggregate.Facts, provisionalFocusID)
	aggregate.Facts = runtimeTestRebindSyntheticEvidenceToCurrentSemanticFocus(aggregate.Facts)
	runtimeTestSealCanonicalCWILedger(t, aggregate)
	if aggregate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningSealed {
		aggregate.SemanticPlanning.SealedBindingSetHash = aggregate.Facts.ReconciliationContract.BindingSetHash
		if len(aggregate.SemanticPlanning.SealedLineageRefs) == 0 {
			t.Fatal("test semantic planning ledger has no sealed lineage")
		}
		aggregate.SemanticPlanning.SealedLineageRefs[len(aggregate.SemanticPlanning.SealedLineageRefs)-1] = aggregate.SemanticPlanning.SealedBindingSetHash
		if err := aggregate.SemanticPlanning.Validate(); err != nil {
			t.Fatalf("align test semantic planning binding set: %v", err)
		}
	}
	decision, err := ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatalf("reduce test fixture with sealed coverage binding: %v", err)
	}
	if decision.State == runtimecontract.ConvergenceStateFailed {
		t.Fatalf("test semantic binding projection reduced to failure: terminal=%+v focus=%+v", decision.Terminal, aggregate.Facts.Focus)
	}
	aggregate.Decision = decision
}

// runtimeTestProjectSemanticBindingPreimages replaces the legacy handwritten
// focus/binding shape with the exact output of the same pure compiler used by
// the production semantic seal. The helper exists only to migrate focused
// reducer fixtures; it does not create an alternate production admission path.
func runtimeTestProjectSemanticBindingPreimages(t *testing.T, aggregate *RuntimeAggregate) {
	t.Helper()
	if aggregate == nil {
		t.Fatal("test semantic binding projection requires an aggregate")
	}
	facts, _, err := runtimeTestProjectSemanticBindingPreimageFacts(*aggregate)
	if err != nil {
		t.Fatalf("project test semantic binding preimages: %v", err)
	}
	aggregate.Facts = facts
}

func runtimeTestProjectSemanticBindingPreimageFacts(aggregate RuntimeAggregate) (runtimecontract.RuntimeFacts, []runtimecontract.CoverageFocus, error) {
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed ||
		aggregate.SemanticPlanning.Manifest == nil || aggregate.SemanticPlanning.Document == nil {
		return runtimecontract.RuntimeFacts{}, nil, fmt.Errorf("test semantic binding projection requires a sealed ledger preimage")
	}
	ledger := aggregate.SemanticPlanning.Clone()
	focuses, _, err := runtimecontract.CompileRuntimeSemanticBindings(
		*ledger.Manifest, ledger.LogicalRequestID, *ledger.Document, ledger.Feasibilities,
	)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, nil, fmt.Errorf("compile test semantic binding preimages: %w", err)
	}
	selected := make(map[common.CoverageWorkItemID]struct{}, len(focuses))
	for _, focus := range focuses {
		for _, coverageID := range focus.CoverageWorkItemIDs {
			selected[coverageID] = struct{}{}
		}
	}
	facts := aggregate.Facts.Normalized()
	clearSelectedFocusBindings := func(focus runtimecontract.CoverageFocus) runtimecontract.CoverageFocus {
		focus = focus.Normalized()
		retained := make([]runtimecontract.CoverageEvidenceBinding, 0, len(focus.CoverageEvidenceBindings))
		for _, binding := range focus.CoverageEvidenceBindings {
			if _, replace := selected[binding.CoverageWorkItemID]; !replace {
				retained = append(retained, binding.Clone())
			}
		}
		focus.CoverageEvidenceBindings = retained
		return focus.Normalized()
	}
	facts.Focus = clearSelectedFocusBindings(facts.Focus)
	for index := range facts.RemainingFocuses {
		facts.RemainingFocuses[index] = clearSelectedFocusBindings(facts.RemainingFocuses[index])
	}
	for index := range facts.Coverage {
		item := &facts.Coverage[index]
		if _, replace := selected[item.ID]; !replace {
			continue
		}
		if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			return runtimecontract.RuntimeFacts{}, nil, fmt.Errorf("test semantic binding projection cannot rewrite settled CWI %q", item.ID)
		}
		item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateUnbound
		item.CurrentBindingRef = nil
		item.BindingLineage = nil
		item.ReopenLineage = nil
		item.FocusID = ""
	}
	facts.CWILedgerSchemaVersion = runtimecontract.RuntimeCWILedgerSchemaCurrent
	facts.ReconciliationContract = runtimecontract.RuntimeCWIReconciliationContract{}
	pending, err := semanticPendingFocuses(facts, ledger.PendingCoverageIDs, focuses, *ledger.Manifest)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, nil, fmt.Errorf("compile test semantic pending focuses: %w", err)
	}
	facts, err = sealRuntimeCWIBindings(facts, ledger.Manifest.Hash, focuses, pending)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, nil, fmt.Errorf("seal test semantic binding preimages: %w", err)
	}
	reconciliation, err := canonicalRuntimeCWIReconciliationContract(facts)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, nil, fmt.Errorf("compile test semantic reconciliation preimage: %w", err)
	}
	facts.ReconciliationContract = reconciliation
	return runtimeTestSeedSemanticTargetSnapshots(facts), focuses, nil
}

// runtimeTestSealCanonicalCWILedger projects direct post-semantic test facts
// into the same single CWI ledger shape that production obtains through the
// private semantic seal. It is test-only setup, not a second Runtime path.
func runtimeTestSealCanonicalCWILedger(t *testing.T, aggregate *RuntimeAggregate) {
	t.Helper()
	if aggregate == nil {
		return
	}
	facts := aggregate.Facts.Normalized()
	facts.Focus = runtimeTestAlignFocusTargetSurfaceFromBindings(facts.Focus)
	for index := range facts.RemainingFocuses {
		facts.RemainingFocuses[index] = runtimeTestAlignFocusTargetSurfaceFromBindings(facts.RemainingFocuses[index])
	}
	for index := range facts.CompletedFocuses {
		facts.CompletedFocuses[index] = runtimeTestAlignFocusTargetSurfaceFromBindings(facts.CompletedFocuses[index])
	}
	facts = facts.Normalized()
	focusByCoverage := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus)
	for _, focus := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		focus = focus.Normalized()
		for _, coverageID := range focus.CoverageWorkItemIDs {
			if _, duplicate := focusByCoverage[coverageID]; duplicate {
				t.Fatalf("test CWI ledger duplicates focus projection for %q", coverageID)
			}
			focusByCoverage[coverageID] = focus
		}
	}
	for index := range facts.Coverage {
		item := &facts.Coverage[index]
		focus, found := focusByCoverage[item.ID]
		if !found {
			t.Fatalf("test CWI ledger has no focus projection for %q", item.ID)
		}
		item.FocusID = focus.ID
		binding, bound := coverageBindingForRuntimeCWI(focus, item.ID)
		if !bound {
			if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
				t.Fatalf("test CWI ledger cannot synthesize settled lineage for %q", item.ID)
			}
			item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateUnbound
			item.CurrentBindingRef = nil
			item.BindingLineage = nil
			item.ReopenLineage = nil
			continue
		}
		manifestHash := ""
		if aggregate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningSealed && aggregate.SemanticPlanning.Manifest != nil {
			manifestHash = aggregate.SemanticPlanning.Manifest.Hash
		}
		if manifestHash == "" {
			// Pure reducer fixtures may exercise the canonical row projection
			// without constructing an aggregate semantic ledger. Operation/receipt
			// fixtures install the sealed ledger first and therefore cannot use this
			// test-only placeholder as semantic audit authority.
			manifestHash = "test-semantic-manifest/" + focus.ID
		}
		ref := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, manifestHash)
		item.CurrentBindingRef = &ref
		item.BindingLineage = []runtimecontract.RuntimeCWIBindingRef{ref}
		item.ReopenLineage = nil
		if item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateSettled
		} else {
			item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateBound
		}
	}
	facts.CWILedgerSchemaVersion = runtimecontract.RuntimeCWILedgerSchemaCurrent
	boundFocuses := make([]runtimecontract.CoverageFocus, 0, 1+len(facts.RemainingFocuses)+len(facts.CompletedFocuses))
	for _, focus := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		if len(focus.CoverageEvidenceBindings) != 0 {
			boundFocuses = append(boundFocuses, focus)
		}
	}
	if len(boundFocuses) == 0 {
		facts.ReconciliationContract = runtimecontract.RuntimeCWIReconciliationContract{}
	} else {
		contract, err := runtimecontract.CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, boundFocuses)
		if err != nil {
			t.Fatalf("compile test CWI reconciliation contract: %v", err)
		}
		facts.ReconciliationContract = contract
	}
	if err := runtimecontract.ValidateRuntimeCWILedger(facts); err != nil {
		t.Fatalf("validate test canonical CWI ledger: %v", err)
	}
	aggregate.Facts = facts
}

// runtimeTestSynchronizeCanonicalCWILedger projects a deliberately edited
// test focus/binding shape back into the persisted CWI rows before a fixture
// enters a controller or reducer. It is test construction only: production
// reaches the same shape exclusively through the private semantic seal/reopen
// transitions.
func runtimeTestSynchronizeCanonicalCWILedger(t *testing.T, aggregate *RuntimeAggregate) {
	t.Helper()
	runtimeTestSealCanonicalCWILedger(t, aggregate)
	if aggregate != nil && aggregate.SemanticPlanning.Phase == runtimecontract.RuntimeSemanticPlanningSealed && len(aggregate.SemanticPlanning.SealedLineageRefs) != 0 {
		aggregate.SemanticPlanning.SealedBindingSetHash = aggregate.Facts.ReconciliationContract.BindingSetHash
		aggregate.SemanticPlanning.SealedLineageRefs[len(aggregate.SemanticPlanning.SealedLineageRefs)-1] = aggregate.SemanticPlanning.SealedBindingSetHash
		if err := aggregate.SemanticPlanning.Validate(); err != nil {
			t.Fatalf("align synchronized test semantic planning binding set: %v", err)
		}
	}
}

func runtimeTestFactsWithCanonicalCWIProjection(t *testing.T, facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	t.Helper()
	runtimeTestSealCurrentPolicies(&facts)
	aggregate := RuntimeAggregate{Facts: facts}
	runtimeTestSynchronizeCanonicalCWILedger(t, &aggregate)
	return aggregate.Facts
}

func runtimeTestSealCurrentFocusBindings(t *testing.T, aggregate *RuntimeAggregate) {
	t.Helper()
	if aggregate == nil || len(aggregate.Facts.Focus.CoverageWorkItemIDs) == 0 {
		return
	}
	focus := aggregate.Facts.Focus.Normalized()
	definitionRevision := aggregate.Facts.DefinitionRevision
	if definitionRevision == "" {
		definitionRevision = aggregate.Definition.Revision
	}
	if definitionRevision == "" {
		definitionRevision = "test-definition"
	}
	// Direct fixtures declare their final proof constraints here before entering
	// the reducer. Production obtains the same fields from activation.
	validationRecipeID := focus.ValidationRecipeID
	if validationRecipeID == "" && hasRuntimeTestEvidenceKind(focus.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation) {
		for _, policy := range aggregate.Facts.Activation.EvidencePolicies {
			if normalized := policy.Normalized(); normalized.RecipeDeclared && normalized.ID != "" {
				validationRecipeID = normalized.ID
				break
			}
		}
		if validationRecipeID == "" {
			validationRecipeID = "test-runtime-validation-recipe"
		}
		focus.ValidationRecipeID = validationRecipeID
	}
	proofs := make([]runtimecontract.CoverageEvidenceProof, 0, len(focus.RequiredEvidenceKinds))
	for _, evidenceKind := range focus.RequiredEvidenceKinds {
		proof := runtimecontract.CoverageEvidenceProof{EvidenceKind: evidenceKind}
		for _, descriptor := range runtimecontract.RuntimeOperationDescriptors() {
			if runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(descriptor.Kind, evidenceKind) {
				proof.AllowedOperationKinds = append(proof.AllowedOperationKinds, descriptor.Kind)
			}
		}
		switch evidenceKind {
		case runtimecontract.EvidenceKindStateChange:
			proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable}
		case runtimecontract.EvidenceKindValidation:
			proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession}
			proof.ValidationRecipeID = validationRecipeID
			proof.RequiredCheckRefs = append([]string(nil), focus.RequiredCheckRefs...)
			proof.MetricConstraints = append([]string(nil), focus.MetricConstraints...)
			proof.ArtifactRefs = append([]string(nil), focus.ArtifactRefs...)
		default:
			t.Fatalf("test fixture has unsupported evidence kind %q", evidenceKind)
		}
		proofs = append(proofs, proof.Normalized())
	}
	bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(focus.CoverageWorkItemIDs))
	for _, coverageID := range focus.CoverageWorkItemIDs {
		binding := runtimecontract.CoverageEvidenceBinding{
			ID:                                   "binding/test/runtime/" + focus.ID + "/" + string(coverageID),
			CoverageWorkItemID:                   coverageID,
			FocusID:                              focus.ID,
			DefinitionRevision:                   definitionRevision,
			Proofs:                               append([]runtimecontract.CoverageEvidenceProof(nil), proofs...),
			TargetRefs:                           append([]string(nil), focus.TargetRefs...),
			TargetSelectorRef:                    focus.TargetSelectorRef,
			TargetSelectorOwnerRef:               focus.TargetSelectorOwnerRef,
			TargetSelectorSubjectRef:             focus.TargetSelectorSubjectRef,
			TargetSelectorCandidateRefs:          append([]string(nil), focus.TargetSelectorCandidateRefs...),
			ProspectiveTargetAllOfConstraintRefs: append([]string(nil), focus.ProspectiveTargetAllOfConstraintRefs...),
			ProspectiveTargetRefs:                append([]string(nil), focus.ProspectiveTargetRefs...),
			DeclarationRef:                       "test/runtime-reducer-fixture",
		}.Normalized()
		if len(binding.TargetRefs) == 0 && binding.TargetSelectorRef == "" && len(binding.ProspectiveTargetRefs) == 0 {
			binding.TargetSelectorRef = "test/runtime-owner-selector"
			binding.TargetSelectorOwnerRef = "test-runtime"
			binding.TargetSelectorSubjectRef = focus.ID
		}
		if binding.TargetSelectorRef != "" && binding.TargetSelectorOwnerRef == "" {
			binding.TargetSelectorOwnerRef = "test-runtime"
		}
		if binding.TargetSelectorRef != "" && binding.TargetSelectorSubjectRef == "" {
			binding.TargetSelectorSubjectRef = focus.ID
		}
		binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
		bindings = append(bindings, binding)
	}
	focus.CoverageEvidenceBindings = runtimecontract.NormalizeCoverageEvidenceBindings(bindings)
	focus = runtimeTestAlignFocusTargetSurfaceFromBindings(focus)
	aggregate.Facts.Focus = focus
	bindEvidence := func(values []runtimecontract.EvidenceFact) []runtimecontract.EvidenceFact {
		ret := append([]runtimecontract.EvidenceFact(nil), values...)
		for index := range ret {
			evidence := &ret[index]
			if evidence.FocusID == "" {
				evidence.FocusID = focus.ID
			}
			if len(evidence.BindingRefs) != 0 || len(evidence.CoverageWorkItemIDs) == 0 {
				continue
			}
			for _, coverageID := range evidence.CoverageWorkItemIDs {
				for _, binding := range focus.CoverageEvidenceBindings {
					if binding.CoverageWorkItemID != coverageID {
						continue
					}
					if _, found := runtimecontract.CoverageEvidenceProofForKind(binding, evidence.Kind); found {
						evidence.BindingRefs = append(evidence.BindingRefs, binding.ID)
					}
				}
			}
			if len(evidence.BindingRefs) != 0 {
				if evidence.DispatchID == "" {
					evidence.DispatchID = "dispatch/test/" + string(evidence.ID)
				}
				if evidence.OperationID == "" {
					evidence.OperationID = "operation/test/" + string(evidence.ID)
				}
			}
		}
		return ret
	}
	aggregate.Facts.Evidence.Accepted = bindEvidence(aggregate.Facts.Evidence.Accepted)
	aggregate.Facts.Evidence.Rejected = bindEvidence(aggregate.Facts.Evidence.Rejected)
}

// runtimeTestAlignFocusTargetSurfaceFromBindings aligns hand-written
// post-semantic fixtures with the production invariant that a sealed focus and
// every binding it contains carry the same target surface. The binding is the
// only semantic authority available in these fixtures; production code is not
// involved in this projection.
func runtimeTestAlignFocusTargetSurfaceFromBindings(focus runtimecontract.CoverageFocus) runtimecontract.CoverageFocus {
	focus = focus.Normalized()
	if len(focus.CoverageEvidenceBindings) == 0 {
		return focus
	}
	binding := focus.CoverageEvidenceBindings[0].Normalized()
	focus.TargetRefs = append([]string(nil), binding.TargetRefs...)
	focus.TargetSelectorRef = binding.TargetSelectorRef
	focus.TargetSelectorOwnerRef = binding.TargetSelectorOwnerRef
	focus.TargetSelectorSubjectRef = binding.TargetSelectorSubjectRef
	focus.TargetSelectorCandidateRefs = append([]string(nil), binding.TargetSelectorCandidateRefs...)
	focus.ProspectiveTargetAllOfConstraintRefs = append([]string(nil), binding.ProspectiveTargetAllOfConstraintRefs...)
	focus.ProspectiveTargetRefs = append([]string(nil), binding.ProspectiveTargetRefs...)
	return focus.Normalized()
}

func runtimeTestEnsureSemanticPlanningLedger(t *testing.T, aggregate *RuntimeAggregate) {
	t.Helper()
	if aggregate == nil {
		return
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
		// A number of reducer fixtures append a second focus after their base
		// facts were created. Keep the synthetic definition seed in lock-step so
		// the immutable semantic order can legitimately include that CWI. This is
		// test scaffolding only; production DefinitionSeed is immutable.
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
			item := runtimecontract.RuntimeCoverageItem{ID: fact.ID, SliceID: sliceID, SourceScope: sourceScope, SourceID: sourceID, TargetText: fact.TargetText, PrerequisiteCoverageWorkItemIDs: fact.PrerequisiteCoverageWorkItemIDs, ReopenDependencies: fact.ReopenDependencies}
			seed.CoverageItems = append(seed.CoverageItems, item)
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
			t.Fatalf("build test semantic planning ledger: %v", err)
		}
		aggregate.SemanticPlanning = ledger
	}
	if aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningSealed || aggregate.SemanticPlanning.Manifest == nil || aggregate.SemanticPlanning.Manifest.DefinitionSeedHash != aggregate.Facts.DefinitionSeed.Hash {
		ledger, err := runtimeTestBuildSealedSemanticPlanning(*aggregate)
		if err != nil {
			t.Fatalf("seal test semantic planning ledger: %v", err)
		}
		aggregate.SemanticPlanning = ledger
		aggregate.Facts = runtimeTestProjectSemanticOwnerCatalogFacts(aggregate.Facts, ledger)
	}
}

// runtimeTestProjectSemanticOwnerCatalogFacts installs the exact typed owner
// catalogs and freshness observations used as the semantic manifest preimage.
// Production receives these facts from capability-owner preflight. Focused
// tests use this helper so the manifest cannot outlive or self-invent a catalog
// that is absent from the current aggregate.
func runtimeTestProjectSemanticOwnerCatalogFacts(facts runtimecontract.RuntimeFacts, ledger runtimecontract.RuntimeSemanticPlanningLedger) runtimecontract.RuntimeFacts {
	if ledger.Manifest == nil {
		return facts
	}
	catalogs := append([]runtimecontract.RuntimeOperationAdmissibility(nil), ledger.Manifest.OwnerCatalogs...)
	catalogs = append(catalogs, ledger.Manifest.OwnerFactDependencyCatalogs...)
	if len(catalogs) == 0 {
		return facts
	}
	wanted := make(map[string]runtimecontract.RuntimeOperationAdmissibility, len(catalogs))
	wantedByPrerequisiteID := make(map[string]runtimecontract.RuntimeOperationAdmissibility, len(catalogs))
	for _, rawCatalog := range catalogs {
		catalog := rawCatalog.Normalized()
		wanted[catalog.PrerequisiteID+"\x00"+catalog.OwnerModuleID] = catalog
		wantedByPrerequisiteID[catalog.PrerequisiteID] = catalog
	}
	currentCatalogs := make([]runtimecontract.RuntimeOperationAdmissibility, 0, len(facts.Resources.OperationAdmissibilities)+len(wanted))
	for _, rawCatalog := range facts.Resources.OperationAdmissibilities {
		catalog := rawCatalog.Normalized()
		if _, replace := wanted[catalog.PrerequisiteID+"\x00"+catalog.OwnerModuleID]; replace {
			continue
		}
		currentCatalogs = append(currentCatalogs, catalog)
	}
	for _, catalog := range wanted {
		currentCatalogs = append(currentCatalogs, catalog)
	}
	facts.Resources.OperationAdmissibilities = currentCatalogs

	observations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(facts.Resources.PrerequisiteObservations)+len(wanted))
	observationByPrerequisite := make(map[string]runtimecontract.RuntimePrerequisiteObservation, len(facts.Resources.PrerequisiteObservations))
	for _, rawObservation := range facts.Resources.PrerequisiteObservations {
		observation := rawObservation.Normalized()
		if catalog, replace := wantedByPrerequisiteID[observation.PrerequisiteID]; replace {
			if observation.OwnerModuleID == catalog.OwnerModuleID && observation.ContentHash == catalog.SnapshotID && observation.Validate() == nil {
				observationByPrerequisite[observation.PrerequisiteID] = observation
				observations = append(observations, observation)
			}
			continue
		}
		observations = append(observations, observation)
	}
	for _, catalog := range wanted {
		if existing, found := observationByPrerequisite[catalog.PrerequisiteID]; found &&
			existing.OwnerModuleID == catalog.OwnerModuleID && existing.ContentHash == catalog.SnapshotID && existing.Validate() == nil {
			continue
		}
		revision := catalog.ObservedRevision
		if revision == "" {
			revision = catalog.SnapshotID
		}
		domain := runtimecontract.RuntimeResourceOwnerCatalog
		subjectRef := catalog.OwnerModuleID
		switch catalog.OwnerModuleID {
		case runtimecontract.RuntimeModuleFile:
			domain, subjectRef = runtimecontract.RuntimeResourceWorkspaceTree, "workspace_deliverable"
		case runtimecontract.RuntimeModuleGit:
			domain, subjectRef = runtimecontract.RuntimeResourceRepositoryState, "workspace_deliverable"
		case runtimecontract.RuntimeModuleLLM:
			domain, subjectRef = runtimecontract.RuntimeResourceProviderConfig, "provider"
		}
		observation := runtimecontract.RuntimePrerequisiteObservation{
			PrerequisiteID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
			OwnerModuleID: catalog.OwnerModuleID, ContentHash: catalog.SnapshotID,
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
				Domain: domain, OwnerModuleID: catalog.OwnerModuleID, SubjectRef: subjectRef, Revision: revision,
			}},
		}
		observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
		observations = append(observations, observation)
	}
	facts.Resources.PrerequisiteObservations = runtimecontract.NormalizeRuntimePrerequisiteObservations(observations)
	satisfied := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Satisfied)+len(wanted))
	satisfiedByID := make(map[string]runtimecontract.RuntimePrerequisite, len(facts.Prerequisites.Satisfied))
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if catalog, replace := wantedByPrerequisiteID[prerequisite.ID]; replace {
			if prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility && prerequisite.OwnerModuleID == catalog.OwnerModuleID {
				satisfiedByID[prerequisite.ID] = prerequisite
				satisfied = append(satisfied, prerequisite)
			}
			continue
		}
		satisfied = append(satisfied, prerequisite)
	}
	for _, catalog := range wanted {
		if prerequisite, found := satisfiedByID[catalog.PrerequisiteID]; found &&
			prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility && prerequisite.OwnerModuleID == catalog.OwnerModuleID {
			continue
		}
		satisfied = append(satisfied, runtimecontract.RuntimePrerequisite{
			ID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
			Scope: runtimecontract.RuntimePrerequisiteScopeGlobal, OwnerModuleID: catalog.OwnerModuleID,
			RequiredCapabilities: append([]string(nil), catalog.OperationKinds...), Reason: "test capability-owner preflight",
		})
	}
	facts.Prerequisites.Satisfied = satisfied
	retainedPending := facts.Prerequisites.Pending[:0]
	for _, prerequisite := range facts.Prerequisites.Pending {
		if _, sealed := wantedByPrerequisiteID[prerequisite.ID]; sealed {
			continue
		}
		retainedPending = append(retainedPending, prerequisite)
	}
	facts.Prerequisites.Pending = retainedPending
	return facts.Normalized()
}

// runtimeTestBuildSealedSemanticPlanning is test-only post-semantic setup.
// It keeps operation/reducer fixtures on the production invariant that an
// operations request follows a sealed semantic contract, without adding a
// production fallback that infers semantic authority from old focus fields.
func runtimeTestBuildSealedSemanticPlanning(aggregate RuntimeAggregate) (runtimecontract.RuntimeSemanticPlanningLedger, error) {
	seed := aggregate.Facts.DefinitionSeed.Clone()
	ledger, err := runtimecontract.NewRuntimeSemanticPlanningLedger(seed)
	if err != nil {
		return runtimecontract.RuntimeSemanticPlanningLedger{}, err
	}
	testTargets := runtimeTestSemanticTargets(aggregate.Facts, seed)
	sourceFocuses := runtimeTestSemanticSourceFocuses(aggregate.Facts, seed)
	catalogTargets := runtimeTestSemanticCatalogTargets(aggregate.Facts, seed, sourceFocuses, testTargets)
	operationKinds := []string{runtimecontract.StepKindFileWrite}
	selectorRefs := []string(nil)
	operationVariants := []runtimecontract.RuntimeOperationAdmissibilityVariant{{
		Kind: runtimecontract.StepKindFileWrite,
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
			runtimecontract.RuntimeOperationTargetPolicyExactTarget,
		},
	}}
	stateChangeOperationKind := runtimecontract.StepKindFileWrite
	stateChangeOperationVariantID := ""
	for _, candidateKind := range aggregate.Facts.Capabilities.StateChangeKinds {
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(candidateKind)
		if !found || descriptor.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
			!runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(candidateKind, runtimecontract.EvidenceKindStateChange) {
			continue
		}
		stateChangeOperationKind = runtimecontract.CanonicalRuntimeBlockKind(candidateKind)
		break
	}
	operationKinds = runtimeTestAppendUnique(operationKinds, stateChangeOperationKind)
	if contract, found := runtimecontract.RuntimeOperationContractForKind(stateChangeOperationKind); found && len(contract.Variants) != 0 {
		for _, variant := range contract.Variants {
			if !runtimecontract.RuntimeEffectIsDurable(variant.EffectScope) {
				continue
			}
			stateChangeOperationVariantID = variant.ID
			break
		}
		if stateChangeOperationVariantID == "" {
			return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("test semantic state-change kind %q has no durable variant", stateChangeOperationKind)
		}
		operationVariants = runtimeTestUpsertOperationVariant(operationVariants, runtimecontract.RuntimeOperationAdmissibilityVariant{
			Kind: stateChangeOperationKind, VariantID: stateChangeOperationVariantID,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		})
	}
	for _, source := range sourceFocuses {
		for _, convergence := range source.OwnerFactConvergences {
			if convergence.RequirePreexisting || convergence.AcquisitionOwnerModuleID != runtimecontract.RuntimeModuleFile {
				continue
			}
			operationKinds = runtimeTestAppendUnique(operationKinds, convergence.AcquisitionOperationKind)
			selectorRefs = runtimeTestAppendUnique(selectorRefs, convergence.AcquisitionTargetSelectorRef)
			operationVariants = runtimeTestUpsertOperationVariant(operationVariants, runtimecontract.RuntimeOperationAdmissibilityVariant{
				Kind: convergence.AcquisitionOperationKind, VariantID: convergence.AcquisitionOperationVariantID,
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{convergence.AcquisitionTargetPolicy},
				SelectorRefs:          []string{convergence.AcquisitionTargetSelectorRef},
				ConstraintRefs:        []string{"workspace-relative-path"},
			})
		}
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "test-semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: operationKinds, OperationVariants: operationVariants,
		TargetRefs: catalogTargets, SelectorRefs: selectorRefs, ConstraintRefs: []string{"workspace-relative-path"},
		ObservedRevision: "test-semantic-revision",
	}
	// A semantic manifest carries exactly one catalog per owner. Merge an
	// explicitly supplied File preflight into the synthetic baseline instead of
	// preserving two self-consistent snapshots for the same owner. The explicit
	// policies/selector constraints remain the authority for prospective create
	// fixtures; the baseline contributes only generic exact-target coverage for
	// reducer fixtures that did not declare a File catalog themselves.
	for _, rawOwnerCatalog := range aggregate.Facts.Resources.OperationAdmissibilities {
		ownerCatalog := rawOwnerCatalog.Normalized()
		if ownerCatalog.OwnerModuleID != runtimecontract.RuntimeModuleFile {
			continue
		}
		for _, kind := range ownerCatalog.OperationKinds {
			catalog.OperationKinds = runtimeTestAppendUnique(catalog.OperationKinds, kind)
		}
		for _, variant := range ownerCatalog.OperationVariants {
			catalog.OperationVariants = runtimeTestUpsertOperationVariant(catalog.OperationVariants, variant)
		}
		for _, targetRef := range ownerCatalog.TargetRefs {
			catalog.TargetRefs = runtimeTestAppendUnique(catalog.TargetRefs, targetRef)
		}
		for _, selectorRef := range ownerCatalog.SelectorRefs {
			catalog.SelectorRefs = runtimeTestAppendUnique(catalog.SelectorRefs, selectorRef)
		}
		for _, constraintRef := range ownerCatalog.ConstraintRefs {
			catalog.ConstraintRefs = runtimeTestAppendUnique(catalog.ConstraintRefs, constraintRef)
		}
		for _, recipeRef := range ownerCatalog.ValidationRecipeRefs {
			catalog.ValidationRecipeRefs = runtimeTestAppendUnique(catalog.ValidationRecipeRefs, recipeRef)
		}
		catalog.ValidationSurfaces = runtimecontract.NormalizeRuntimeValidationSurfaces(append(catalog.ValidationSurfaces, ownerCatalog.ValidationSurfaces...))
		if ownerCatalog.ObservedRevision != "" {
			catalog.ObservedRevision = ownerCatalog.ObservedRevision
		}
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	ownerCatalogs := []runtimecontract.RuntimeOperationAdmissibility{catalog}
	allowedOwnerModuleIDs := []string{runtimecontract.RuntimeModuleFile}
	allowedOperationKinds := append([]string(nil), operationKinds...)
	for _, rawOwnerCatalog := range aggregate.Facts.Resources.OperationAdmissibilities {
		ownerCatalog := rawOwnerCatalog.Normalized()
		if ownerCatalog.OwnerModuleID == "" || ownerCatalog.OwnerModuleID == runtimecontract.RuntimeModuleFile {
			continue
		}
		ownerCatalogs = append(ownerCatalogs, ownerCatalog)
		allowedOwnerModuleIDs = runtimeTestAppendUnique(allowedOwnerModuleIDs, ownerCatalog.OwnerModuleID)
		for _, operationKind := range ownerCatalog.OperationKinds {
			allowedOperationKinds = runtimeTestAppendUnique(allowedOperationKinds, operationKind)
		}
	}
	proofObligations, err := runtimeTestSemanticProofObligations(sourceFocuses, seed.CoverageItems, ownerCatalogs)
	if err != nil {
		return runtimecontract.RuntimeSemanticPlanningLedger{}, err
	}
	dependencyCatalogs := make([]runtimecontract.RuntimeOperationAdmissibility, 0)
	dependencyOwners := make(map[string]struct{})
	for _, obligation := range proofObligations {
		if obligation.OwnerFactAcquisition == nil || obligation.OwnerFactAcquisition.ProspectiveSatisfactionContractRef == "" {
			continue
		}
		owner := obligation.OwnerFactAcquisition.Requirement.OwnerModuleID
		if _, duplicate := dependencyOwners[owner]; duplicate {
			continue
		}
		var dependencyCatalog runtimecontract.RuntimeOperationAdmissibility
		for _, candidate := range aggregate.Facts.Resources.OperationAdmissibilities {
			candidate = candidate.Normalized()
			if candidate.OwnerModuleID == owner {
				dependencyCatalog = candidate
				break
			}
		}
		if dependencyCatalog.SnapshotID == "" {
			return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("build test semantic manifest: missing dependency catalog %q", owner)
		}
		dependencyCatalogs = append(dependencyCatalogs, dependencyCatalog)
		dependencyOwners[owner] = struct{}{}
	}
	operationContractRevision := aggregate.Facts.Activation.ExecutionContract.OperationContractRevision
	if operationContractRevision == "" {
		operationContractRevision = runtimecontract.RuntimeOperationContractRevision()
	}
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DefinitionRevision: aggregate.Facts.DefinitionRevision, DefinitionSeedHash: seed.Hash,
		OperationContractRevision: operationContractRevision,
		CoverageItems:             seed.CoverageItems,
		AllowedOwnerModuleIDs:     allowedOwnerModuleIDs,
		AllowedOperationKinds:     allowedOperationKinds,
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
			runtimecontract.RuntimeOperationTargetPolicyExactTarget,
			runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		},
		ProofObligations:            proofObligations,
		OwnerCatalogs:               ownerCatalogs,
		OwnerFactDependencyCatalogs: dependencyCatalogs,
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	coverageOrder, err := runtimeTestSemanticCoverageOrder(seed)
	if err != nil {
		return runtimecontract.RuntimeSemanticPlanningLedger{}, err
	}
	orderDocument := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion:      runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:       manifest.Hash,
		DefinitionSeedHash: seed.Hash,
		PlanStep:           runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder:      coverageOrder,
	}
	if err := orderDocument.ValidateAgainstManifest(manifest); err != nil {
		return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("build test semantic coverage order: %w", err)
	}
	activeCoverageIDs := append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...)
	if len(activeCoverageIDs) == 0 {
		if len(coverageOrder) == 0 {
			return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("test semantic focus has no coverage items")
		}
		activeCoverageIDs = []common.CoverageWorkItemID{coverageOrder[0]}
	}
	batch, err := semanticplan.CompileFocusBatch(
		manifest, orderDocument, 0, "test-semantic-request", activeCoverageIDs, coverageOrder, len(coverageOrder),
	)
	if err != nil {
		return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("build test semantic focus batch: %w", err)
	}
	for _, next := range []runtimecontract.RuntimeSemanticPlanBatchState{
		runtimecontract.RuntimeSemanticPlanBatchRequesting,
		runtimecontract.RuntimeSemanticPlanBatchDocumentComplete,
	} {
		batch, err = advanceRuntimeSemanticPlanBatch(batch, next)
		if err != nil {
			return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("advance test semantic batch: %w", err)
		}
	}
	selected := make(map[common.CoverageWorkItemID]struct{}, len(batch.CoverageWorkItemIDs))
	for _, id := range batch.CoverageWorkItemIDs {
		selected[id] = struct{}{}
	}
	obligationsByCoverage := make(map[common.CoverageWorkItemID]runtimecontract.RuntimeSemanticProofObligation, len(manifest.ProofObligations))
	for _, obligation := range manifest.ProofObligations {
		obligationsByCoverage[obligation.CoverageWorkItemID] = obligation
	}
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: seed.Hash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID:  batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
	}
	sourceByCoverage := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(sourceFocuses))
	for _, source := range sourceFocuses {
		for _, coverageID := range source.CoverageWorkItemIDs {
			sourceByCoverage[coverageID] = source.Normalized()
		}
	}
	type semanticFixtureProof struct {
		proposal runtimecontract.RuntimeSemanticProofProposal
		catalog  runtimecontract.RuntimeOperationAdmissibility
	}
	validationProof := func(item runtimecontract.RuntimeCoverageItem, targetRefs []string, declarationRef string) (semanticFixtureProof, error) {
		source := sourceByCoverage[item.ID]
		for _, rawOwnerCatalog := range manifest.OwnerCatalogs {
			ownerCatalog := rawOwnerCatalog.Normalized()
			for _, rawSurface := range ownerCatalog.ValidationSurfaces {
				surface := rawSurface.Normalized()
				if surface.Readiness != runtimecontract.RuntimeValidationSurfaceReady ||
					surface.ProofApplicability != runtimecontract.RuntimeValidationProofDirect ||
					(source.ValidationRecipeID != "" && surface.ID != source.ValidationRecipeID) {
					continue
				}
				target := runtimecontract.RuntimeOperationTargetAuthorization{
					Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: append([]string(nil), targetRefs...),
				}
				if source.TargetSelectorRef != "" && len(ownerCatalog.TargetRefs) == 0 {
					allowed, selectorRefs, _, _, policyErr := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(
						ownerCatalog, surface.OperationKind, nil, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
					)
					if policyErr == nil && allowed && runtimeStringsContain(selectorRefs, source.TargetSelectorRef) {
						target = runtimecontract.RuntimeOperationTargetAuthorization{
							Policy:               runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
							OwnerScopeTargetRefs: append([]string(nil), ownerCatalog.TargetRefs...),
							OwnerSelectorRefs:    []string{source.TargetSelectorRef},
						}
					}
				}
				return semanticFixtureProof{
					proposal: runtimecontract.RuntimeSemanticProofProposal{
						CoverageWorkItemID: item.ID, OwnerModuleID: ownerCatalog.OwnerModuleID,
						OperationKind:         surface.OperationKind,
						Target:                target,
						RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
						RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{surface.EffectScope},
						ValidationRecipeID:    surface.ID,
						DeclarationRef:        declarationRef,
					},
					catalog: ownerCatalog,
				}, nil
			}
		}
		return semanticFixtureProof{}, fmt.Errorf("test semantic validation proof for %q has no ready direct owner surface", item.ID)
	}
	stateChangeProof := func(item runtimecontract.RuntimeCoverageItem, targetRefs []string, declarationRef string) (semanticFixtureProof, error) {
		source := sourceByCoverage[item.ID]
		candidateKinds := append([]string(nil), source.AcceptedStepKinds...)
		if len(candidateKinds) == 0 {
			candidateKinds = append(candidateKinds, aggregate.Facts.Capabilities.StateChangeKinds...)
		}
		if len(candidateKinds) == 0 {
			candidateKinds = runtimeTestAppendUnique(candidateKinds, stateChangeOperationKind)
		}
		for _, candidateKind := range candidateKinds {
			candidateKind = runtimecontract.CanonicalRuntimeBlockKind(candidateKind)
			descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(candidateKind)
			if !found || !runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(candidateKind, runtimecontract.EvidenceKindStateChange) {
				continue
			}
			for _, rawOwnerCatalog := range manifest.OwnerCatalogs {
				ownerCatalog := rawOwnerCatalog.Normalized()
				catalogSupportsKind := false
				for _, operationKind := range ownerCatalog.OperationKinds {
					if runtimecontract.CanonicalRuntimeBlockKind(operationKind) == candidateKind {
						catalogSupportsKind = true
						break
					}
				}
				if ownerCatalog.OwnerModuleID != descriptor.OwnerModuleID || !catalogSupportsKind {
					continue
				}
				variantID := ""
				if contract, contractFound := runtimecontract.RuntimeOperationContractForKind(candidateKind); contractFound && len(contract.Variants) != 0 {
					for _, variant := range ownerCatalog.OperationVariants {
						if runtimecontract.CanonicalRuntimeBlockKind(variant.Kind) != candidateKind || variant.VariantID == "" {
							continue
						}
						if len(source.AcceptedOperationVariants) != 0 {
							accepted := false
							for _, declared := range source.AcceptedOperationVariants {
								if runtimecontract.CanonicalRuntimeBlockKind(declared.Kind) == candidateKind && declared.VariantID == variant.VariantID {
									accepted = true
									break
								}
							}
							if !accepted {
								continue
							}
						}
						variantID = variant.VariantID
						break
					}
					if variantID == "" {
						continue
					}
				}
				target := runtimecontract.RuntimeOperationTargetAuthorization{
					Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: append([]string(nil), targetRefs...),
				}
				if len(source.ProspectiveTargetRefs) != 0 {
					variantIDs := []string(nil)
					if variantID != "" {
						variantIDs = []string{variantID}
					}
					allowed, selectors, constraints, _, policyErr := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(
						ownerCatalog, candidateKind, variantIDs, runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
					)
					if policyErr != nil || !allowed || !runtimeStringsContain(selectors, source.TargetSelectorRef) || len(constraints) == 0 {
						continue
					}
					target = runtimecontract.RuntimeOperationTargetAuthorization{
						Policy:                    runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
						OwnerSelectorRefs:         []string{source.TargetSelectorRef},
						ProspectiveConstraintRefs: append([]string(nil), constraints...),
						ProspectiveTargetRefs:     append([]string(nil), source.ProspectiveTargetRefs...),
					}
				}
				return semanticFixtureProof{
					proposal: runtimecontract.RuntimeSemanticProofProposal{
						CoverageWorkItemID: item.ID, OwnerModuleID: ownerCatalog.OwnerModuleID,
						OperationKind: candidateKind, OperationVariantID: variantID,
						Target:                target,
						RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
						RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
						DeclarationRef:        declarationRef,
					},
					catalog: ownerCatalog,
				}, nil
			}
		}
		return semanticFixtureProof{}, fmt.Errorf("test semantic state-change proof for %q has no declaration-compatible owner catalog", item.ID)
	}
	feasibilities := make([]runtimecontract.RuntimeSemanticProofFeasibility, 0, len(batch.CoverageWorkItemIDs)*2)
	for idx, item := range seed.CoverageItems {
		if _, ok := selected[item.ID]; !ok {
			continue
		}
		targetRefs := runtimeTestSemanticTargetRefsForCoverage(aggregate.Facts, item.ID, testTargets[idx])
		if len(targetRefs) == 0 {
			return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("test semantic proof for %q has no target", item.ID)
		}
		obligation := obligationsByCoverage[item.ID]
		declarationRef := "test/semantic/" + string(item.ID)
		proofs := make([]semanticFixtureProof, 0, len(obligation.RequiredEvidenceKinds))
		if acquisition := obligation.OwnerFactAcquisition; acquisition != nil {
			proofs = append(proofs, semanticFixtureProof{
				proposal: runtimecontract.RuntimeSemanticProofProposal{
					CoverageWorkItemID: item.ID, OwnerModuleID: acquisition.AcquisitionOwnerModuleID,
					OperationKind: acquisition.AcquisitionOperationKind, OperationVariantID: acquisition.AcquisitionOperationVariantID,
					Target: runtimecontract.RuntimeOperationTargetAuthorization{
						Policy:                    acquisition.AcquisitionTargetPolicy,
						OwnerSelectorRefs:         []string{acquisition.AcquisitionTargetSelectorRef},
						ProspectiveConstraintRefs: append([]string(nil), acquisition.AcquisitionConstraintRefs...),
						ProspectiveTargetRefs:     append([]string(nil), targetRefs...),
					},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
					RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
					DeclarationRef:        declarationRef,
				},
				catalog: catalog,
			})
		} else {
			for _, evidenceKind := range obligation.RequiredEvidenceKinds {
				switch evidenceKind {
				case runtimecontract.EvidenceKindStateChange:
					proof, proofErr := stateChangeProof(item, targetRefs, declarationRef)
					if proofErr != nil {
						return runtimecontract.RuntimeSemanticPlanningLedger{}, proofErr
					}
					proofs = append(proofs, proof)
				case runtimecontract.EvidenceKindValidation:
					proof, proofErr := validationProof(item, targetRefs, declarationRef)
					if proofErr != nil {
						return runtimecontract.RuntimeSemanticPlanningLedger{}, proofErr
					}
					proofs = append(proofs, proof)
				default:
					return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("test semantic proof for %q has unsupported evidence kind %q", item.ID, evidenceKind)
				}
			}
		}
		for _, proof := range proofs {
			proposal := proof.proposal.Normalized()
			feasibilityTargets := append([]string(nil), proposal.Target.ExactTargetRefs...)
			if proposal.Target.Policy == runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope {
				feasibilityTargets = append([]string(nil), proposal.Target.OwnerScopeTargetRefs...)
			}
			if len(proposal.Target.ProspectiveTargetRefs) != 0 {
				feasibilityTargets = append([]string(nil), proposal.Target.ProspectiveTargetRefs...)
			}
			document.Proposals = append(document.Proposals, proposal)
			feasibility := runtimecontract.RuntimeSemanticProofFeasibility{
				SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
				ManifestHash:  manifest.Hash, DefinitionSeedHash: seed.Hash,
				CoverageWorkItemID: item.ID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal),
				OwnerModuleID: proof.catalog.OwnerModuleID, CatalogSnapshotID: proof.catalog.SnapshotID,
				Status: runtimecontract.RuntimeSemanticFeasibilityFeasible, ObservedRevision: proof.catalog.ObservedRevision, TargetRefs: feasibilityTargets,
				ValidationRecipeID: proposal.ValidationRecipeID,
			}
			if surface, found := runtimecontract.RuntimeValidationSurfaceForID(proof.catalog.ValidationSurfaces, proposal.ValidationRecipeID); found {
				feasibility.RequiredCheckRefs = append([]string(nil), surface.Normalized().CheckRefs...)
			}
			if acquisition := obligation.OwnerFactAcquisition; acquisition != nil && acquisition.AcquisitionTargetPolicy == runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget {
				feasibility.CatalogObservationID = "test-prospective-file-observation"
				delta, deltaErr := runtimecontract.NewRuntimeProspectiveWorkspaceManifestDelta(
					manifest, proposal, acquisition.ProspectiveSatisfactionContractRef, catalog,
					runtimecontract.WorkspaceManifest{Revision: catalog.ObservedRevision, TargetRefs: append([]string(nil), catalog.TargetRefs...)},
				)
				if deltaErr != nil {
					return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("build test prospective workspace delta: %w: proposal=%+v catalog=%+v", deltaErr, proposal, catalog)
				}
				var dependencyCatalog runtimecontract.RuntimeOperationAdmissibility
				for _, candidate := range manifest.OwnerFactDependencyCatalogs {
					if candidate.OwnerModuleID == acquisition.Requirement.OwnerModuleID {
						dependencyCatalog = candidate
						break
					}
				}
				if dependencyCatalog.SnapshotID == "" {
					return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("build test prospective satisfaction: missing dependency catalog %q", acquisition.Requirement.OwnerModuleID)
				}
				recipeRefs := runtimeTestProspectiveSatisfactionRecipeRefs(dependencyCatalog, acquisition.Requirement, acquisition.ProspectiveSatisfactionContractRef)
				if len(recipeRefs) == 0 {
					return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("build test prospective satisfaction: dependency catalog %q has no matching recipe", dependencyCatalog.OwnerModuleID)
				}
				satisfaction := runtimecontract.RuntimeOwnerFactProspectiveSatisfaction{
					SchemaVersion: runtimecontract.RuntimeOwnerFactProspectiveSatisfactionSchemaCurrent,
					WorkspaceID:   manifest.WorkspaceID, RunID: manifest.RunID, ManifestHash: manifest.Hash,
					DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID,
					ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal), Requirement: acquisition.Requirement,
					ProspectiveSatisfactionContractRef: acquisition.ProspectiveSatisfactionContractRef,
					OwnerModuleID:                      acquisition.Requirement.OwnerModuleID, CatalogSnapshotID: dependencyCatalog.SnapshotID,
					CatalogObservationID: "test-prospective-required-owner-observation", ObservedRevision: dependencyCatalog.ObservedRevision,
					ProspectiveWorkspaceManifestDeltaHash: runtimecontract.StableRuntimeProspectiveWorkspaceManifestDeltaHash(delta),
					Outcome:                               runtimecontract.RuntimeOwnerFactProspectiveWouldSatisfy, ProspectiveValidationRecipeRefs: recipeRefs,
				}
				if satisfactionErr := satisfaction.Validate(); satisfactionErr != nil {
					return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("build test prospective satisfaction: %w", satisfactionErr)
				}
				feasibility.ProspectiveWorkspaceManifestDelta = &delta
				observation := runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation{
					FeasibilityBatchID: "test-feasibility-batch", RequestHash: "test-prospective-request", Satisfaction: &satisfaction,
				}
				feasibility.OwnerFactProspectiveSatisfactionObservation = &observation
			}
			feasibilities = append(feasibilities, feasibility)
		}
	}
	if len(document.Proposals) < len(batch.CoverageWorkItemIDs) {
		return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("test semantic focus proposals do not cover batch")
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("build test semantic proof document: %w: proposals=%+v", err, document.Proposals)
	}
	document = document.Normalized()
	batch.DocumentHash = runtimecontract.StableRuntimeSemanticPlanDocumentHash(document)
	batch.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(batch)
	for _, next := range []runtimecontract.RuntimeSemanticPlanBatchState{
		runtimecontract.RuntimeSemanticPlanBatchFeasibilityPending,
		runtimecontract.RuntimeSemanticPlanBatchFeasibilityComplete,
		runtimecontract.RuntimeSemanticPlanBatchAccepted,
	} {
		batch, err = advanceRuntimeSemanticPlanBatch(batch, next)
		if err != nil {
			return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("accept test semantic batch: %w", err)
		}
	}
	merged := document.Clone()
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningSealed
	ledger.Manifest = &manifest
	ledger.CoverageOrderDocument = &orderDocument
	ledger.Batches = []runtimecontract.RuntimeSemanticPlanBatch{batch}
	ledger.BatchDocuments = []runtimecontract.RuntimeSemanticPlanDocument{document}
	batchFeasibility := runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{
		BatchID: batch.BatchID, BatchHash: batch.Hash,
		DocumentHash:  runtimecontract.StableRuntimeSemanticPlanDocumentHash(document),
		Feasibilities: append([]runtimecontract.RuntimeSemanticProofFeasibility(nil), feasibilities...),
	}
	batchFeasibility.Hash = runtimecontract.StableRuntimeSemanticPlanBatchFeasibilityResultHash(batchFeasibility)
	ledger.BatchFeasibilityResults = []runtimecontract.RuntimeSemanticPlanBatchFeasibilityResult{batchFeasibility}
	ledger.CompletedBatchIDs = []string{batch.BatchID}
	ledger.NextBatchSequence = 1
	ledger.PendingCoverageIDs = append([]common.CoverageWorkItemID(nil), batch.PendingCoverageWorkItemIDs...)
	ledger.StablePrefixHash = "test-semantic-stable-prefix"
	ledger.LogicalRequestID = "test-semantic-request"
	ledger.Document = &document
	ledger.DocumentHash = runtimecontract.StableRuntimeSemanticPlanDocumentHash(document)
	ledger.FeasibilityBatchID = runtimecontract.StableRuntimeSemanticFeasibilityBatchID(manifest, document)
	ledger.Feasibilities = feasibilities
	ledger.FeasibilitySetHash = runtimecontract.StableRuntimeSemanticFeasibilitySetHash(feasibilities)
	ledger.MergedDocument = &merged
	ledger.MergedDocumentHash = ledger.DocumentHash
	ledger.SealedBindingSetHash = "test-semantic-binding-set"
	ledger.SealedLineageRefs = []string{manifest.Hash, runtimecontract.StableRuntimeSemanticPlanCoverageOrderHash(orderDocument), ledger.DocumentHash, ledger.FeasibilityBatchID, ledger.FeasibilitySetHash, ledger.SealedBindingSetHash}
	if err := ledger.Validate(); err != nil {
		return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("test sealed semantic ledger validation: %w", err)
	}
	return ledger, nil
}

func runtimeTestSemanticSourceFocuses(facts runtimecontract.RuntimeFacts, seed runtimecontract.RuntimeDefinitionSeed) []runtimecontract.CoverageFocus {
	byCoverage := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(seed.CoverageItems))
	for _, declaration := range facts.DefinitionFocuses {
		focus := declaration.CoverageFocusDeclaration()
		for _, coverageID := range focus.CoverageWorkItemIDs {
			byCoverage[coverageID] = focus
		}
	}
	for _, raw := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		focus := raw.Normalized()
		for _, coverageID := range focus.CoverageWorkItemIDs {
			declaration, found := byCoverage[coverageID]
			if !found {
				byCoverage[coverageID] = focus
				continue
			}
			// DefinitionFocus intentionally has no generation-selected target
			// field. A prospective target belongs to the semantic proposal, so
			// retain it from the current generation without rewriting any
			// immutable declaration field projected above.
			declaration.ProspectiveTargetRefs = append([]string(nil), focus.ProspectiveTargetRefs...)
			byCoverage[coverageID] = declaration.Normalized()
		}
	}
	ret := make([]runtimecontract.CoverageFocus, 0, len(seed.CoverageItems))
	for _, item := range seed.CoverageItems {
		focus, found := byCoverage[item.ID]
		if !found {
			focus = runtimecontract.CoverageFocus{
				ID: "test-semantic-source/" + string(item.ID), CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID},
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			}
		}
		if focus.TargetSelectorRef == "" {
			for _, convergence := range focus.OwnerFactConvergences {
				if convergence.RequirePreexisting || convergence.TargetSelectorRef == "" {
					continue
				}
				focus.TargetSelectorRef = convergence.TargetSelectorRef
				focus.TargetSelectorOwnerRef = convergence.TargetSelectorOwnerRef
				focus.TargetSelectorSubjectRef = convergence.TargetSelectorSubjectRef
				break
			}
		}
		if len(focus.ProspectiveTargetRefs) != 0 {
			focus.RequireTargetGrounding = true
			if focus.TargetRole == "" {
				focus.TargetRole = "test_target"
			}
			if focus.TargetSelectorRef == "" {
				focus.TargetSelectorRef = "test/definition-selector/" + string(item.ID)
			}
			if focus.TargetSelectorOwnerRef == "" {
				focus.TargetSelectorOwnerRef = "test-definition"
			}
			if focus.TargetSelectorSubjectRef == "" {
				focus.TargetSelectorSubjectRef = string(item.ID)
			}
		}
		if len(focus.RequiredEvidenceKinds) == 0 {
			focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
		}
		if len(focus.AcceptedStepKinds) == 0 && len(focus.AcceptedOperationVariants) == 0 {
			for _, evidenceKind := range focus.RequiredEvidenceKinds {
				switch evidenceKind {
				case runtimecontract.EvidenceKindStateChange:
					for _, operationKind := range facts.Capabilities.StateChangeKinds {
						focus.AcceptedStepKinds = runtimeTestAppendUnique(focus.AcceptedStepKinds, operationKind)
					}
					if len(focus.AcceptedStepKinds) == 0 {
						focus.AcceptedStepKinds = runtimeTestAppendUnique(focus.AcceptedStepKinds, runtimecontract.StepKindFileWrite)
					}
				case runtimecontract.EvidenceKindValidation:
					for _, operationKind := range facts.Capabilities.ValidationKinds {
						focus.AcceptedStepKinds = runtimeTestAppendUnique(focus.AcceptedStepKinds, operationKind)
					}
					if len(facts.Capabilities.ValidationKinds) == 0 {
						focus.AcceptedStepKinds = runtimeTestAppendUnique(focus.AcceptedStepKinds, runtimecontract.RuntimeBlockKindManagedValidationSession)
					}
				}
			}
		}
		focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{item.ID}
		ret = append(ret, focus.Normalized())
	}
	return ret
}

func runtimeTestProspectiveSatisfactionRecipeRefs(catalog runtimecontract.RuntimeOperationAdmissibility, requirement runtimecontract.RuntimeOwnerFactRequirement, contractRef string) []string {
	ret := make([]string, 0)
	for _, surface := range catalog.Normalized().ValidationSurfaces {
		if surface.OwnerModuleID == requirement.OwnerModuleID &&
			surface.OperationKind == requirement.OperationKind &&
			surface.Readiness == runtimecontract.RuntimeValidationSurfacePrerequisiteRequired &&
			surface.ProspectiveSatisfactionContractRef == contractRef {
			ret = runtimeTestAppendUnique(ret, surface.ID)
		}
	}
	return ret
}

func runtimeTestSemanticProofObligations(sourceFocuses []runtimecontract.CoverageFocus, coverageItems []runtimecontract.RuntimeCoverageItem, catalogs []runtimecontract.RuntimeOperationAdmissibility) ([]runtimecontract.RuntimeSemanticProofObligation, error) {
	sourceByCoverage := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(sourceFocuses))
	for _, source := range sourceFocuses {
		for _, coverageID := range source.CoverageWorkItemIDs {
			sourceByCoverage[coverageID] = source.Normalized()
		}
	}
	catalogByOwner := make(map[string]runtimecontract.RuntimeOperationAdmissibility, len(catalogs))
	for _, catalog := range catalogs {
		catalogByOwner[catalog.OwnerModuleID] = catalog.Normalized()
	}
	ret := make([]runtimecontract.RuntimeSemanticProofObligation, 0, len(coverageItems))
	for _, item := range coverageItems {
		source := sourceByCoverage[item.ID]
		obligation := runtimecontract.RuntimeSemanticProofObligation{
			CoverageWorkItemID:                              item.ID,
			RequiredEvidenceKinds:                           append([]string(nil), source.RequiredEvidenceKinds...),
			DeclarationEvidenceRole:                         source.EvidenceRole,
			DeclarationTargetRole:                           source.TargetRole,
			DeclarationTargetDiscoverySources:               append([]string(nil), source.TargetDiscoverySources...),
			DeclarationTargetSelectorRef:                    source.TargetSelectorRef,
			DeclarationTargetSelectorOwnerRef:               source.TargetSelectorOwnerRef,
			DeclarationTargetSelectorSubjectRef:             source.TargetSelectorSubjectRef,
			DeclarationTargetSelectorCandidateRefs:          append([]string(nil), source.TargetSelectorCandidateRefs...),
			DeclarationProspectiveTargetAllOfConstraintRefs: append([]string(nil), source.ProspectiveTargetAllOfConstraintRefs...),
			RequireTargetGrounding:                          source.RequireTargetGrounding,
			RequireDirectEvidence:                           source.RequireDirectEvidence,
			RequireClaimEvidence:                            source.RequireClaimEvidence,
			RequireExecutableBehaviorEvidence:               source.RequireExecutableBehaviorEvidence,
			RequireCoverageTargets:                          source.RequireCoverageTargets,
			RequireAssessment:                               source.RequireAssessment,
			RequiredOwnerFacts:                              append([]runtimecontract.RuntimeOwnerFactRequirement(nil), source.RequiredOwnerFacts...),
			OwnerFactConvergences:                           runtimecontract.CloneRuntimeOwnerFactConvergences(source.OwnerFactConvergences),
			MetricConstraints:                               append([]string(nil), source.MetricConstraints...),
			ArtifactRefs:                                    append([]string(nil), source.ArtifactRefs...),
		}
		if len(runtimecontract.RuntimeMissingOwnerFactRequirements(source.RequiredOwnerFacts, catalogs)) != 0 {
			convergence, required := runtimecontract.ResolveRuntimeOwnerFactAcquisition(source.RequiredOwnerFacts, source.OwnerFactConvergences, catalogs)
			if required {
				catalog, found := catalogByOwner[convergence.AcquisitionOwnerModuleID]
				if !found {
					return nil, fmt.Errorf("test semantic obligation %q lacks acquisition owner catalog %q", item.ID, convergence.AcquisitionOwnerModuleID)
				}
				variantIDs := []string(nil)
				if convergence.AcquisitionOperationVariantID != "" {
					variantIDs = []string{convergence.AcquisitionOperationVariantID}
				}
				_, _, constraints, _, _ := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(catalog, convergence.AcquisitionOperationKind, variantIDs, convergence.AcquisitionTargetPolicy)
				var prospectiveTargetConstraint *runtimecontract.RuntimeProspectiveTargetSatisfactionConstraint
				if convergence.AcquisitionTargetPolicy == runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget {
					requiredOwnerCatalog, found := catalogByOwner[convergence.Requirement.OwnerModuleID]
					if !found {
						return nil, fmt.Errorf("test semantic obligation %q lacks prospective required-owner catalog %q", item.ID, convergence.Requirement.OwnerModuleID)
					}
					resolved, resolveErr := runtimecontract.ResolveRuntimeProspectiveTargetSatisfactionConstraint(requiredOwnerCatalog, convergence.Requirement, convergence.ProspectiveSatisfactionContractRef)
					if resolveErr != nil {
						return nil, fmt.Errorf("test semantic obligation %q prospective target constraint: %w", item.ID, resolveErr)
					}
					prospectiveTargetConstraint = &resolved
				}
				obligation.OwnerFactAcquisition = &runtimecontract.RuntimeSemanticOwnerFactAcquisition{
					Requirement:                             convergence.Requirement,
					ProspectiveSatisfactionContractRef:      convergence.ProspectiveSatisfactionContractRef,
					ProspectiveTargetSatisfactionConstraint: prospectiveTargetConstraint,
					DeclarationTargetSelectorRef:            convergence.TargetSelectorRef, DeclarationTargetSelectorOwnerRef: convergence.TargetSelectorOwnerRef, DeclarationTargetSelectorSubjectRef: convergence.TargetSelectorSubjectRef,
					AcquisitionOwnerModuleID: convergence.AcquisitionOwnerModuleID, AcquisitionOperationKind: convergence.AcquisitionOperationKind, AcquisitionOperationVariantID: convergence.AcquisitionOperationVariantID,
					AcquisitionTargetPolicy: convergence.AcquisitionTargetPolicy, AcquisitionTargetSelectorRef: convergence.AcquisitionTargetSelectorRef, AcquisitionConstraintRefs: constraints,
					OwnerCatalogSnapshotID: catalog.SnapshotID, OwnerCatalogObservedRevision: catalog.ObservedRevision,
					ReobserveOwnerModuleID: convergence.ReobserveOwnerModuleID, ReobserveFactKind: convergence.ReobserveFactKind, MaxAcquisitionAttempts: convergence.MaxAcquisitionAttempts,
				}
			}
		}
		ret = append(ret, obligation.Normalized())
	}
	return ret, nil
}

func runtimeTestAppendUnique(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func runtimeTestUpsertOperationVariant(values []runtimecontract.RuntimeOperationAdmissibilityVariant, value runtimecontract.RuntimeOperationAdmissibilityVariant) []runtimecontract.RuntimeOperationAdmissibilityVariant {
	for idx := range values {
		if values[idx].Kind != value.Kind || values[idx].VariantID != value.VariantID {
			continue
		}
		for _, policy := range value.AllowedTargetPolicies {
			found := false
			for _, current := range values[idx].AllowedTargetPolicies {
				if current == policy {
					found = true
					break
				}
			}
			if !found {
				values[idx].AllowedTargetPolicies = append(values[idx].AllowedTargetPolicies, policy)
			}
		}
		for _, selector := range value.SelectorRefs {
			values[idx].SelectorRefs = runtimeTestAppendUnique(values[idx].SelectorRefs, selector)
		}
		for _, constraint := range value.ConstraintRefs {
			values[idx].ConstraintRefs = runtimeTestAppendUnique(values[idx].ConstraintRefs, constraint)
		}
		return values
	}
	return append(values, value)
}

// runtimeTestSemanticTargets retains the declaration target from each direct
// test focus when one exists. The production semantic provider receives the
// same owner-admissible vocabulary; a synthetic target is used only when a
// focused fixture deliberately models a selector-only declaration.
func runtimeTestSemanticTargets(facts runtimecontract.RuntimeFacts, seed runtimecontract.RuntimeDefinitionSeed) []string {
	byCoverage := make(map[common.CoverageWorkItemID]string, len(seed.CoverageItems))
	for _, focus := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		focus = focus.Normalized()
		for _, coverageID := range focus.CoverageWorkItemIDs {
			if _, found := byCoverage[coverageID]; found {
				continue
			}
			switch {
			case len(focus.ProspectiveTargetRefs) != 0:
				byCoverage[coverageID] = focus.ProspectiveTargetRefs[0]
			case len(focus.TargetRefs) == 1:
				byCoverage[coverageID] = focus.TargetRefs[0]
			case len(focus.TargetSelectorCandidateRefs) != 0:
				byCoverage[coverageID] = focus.TargetSelectorCandidateRefs[0]
			}
		}
	}
	targets := make([]string, 0, len(seed.CoverageItems))
	for index, item := range seed.CoverageItems {
		target := byCoverage[item.ID]
		if target == "" {
			target = fmt.Sprintf("semantic-test-target-%d", index+1)
		}
		targets = append(targets, target)
	}
	return targets
}

func runtimeTestSemanticTargetRefsForCoverage(facts runtimecontract.RuntimeFacts, coverageID common.CoverageWorkItemID, fallback string) []string {
	for _, rawFocus := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		focus := rawFocus.Normalized()
		found := false
		for _, candidateID := range focus.CoverageWorkItemIDs {
			if candidateID == coverageID {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		switch {
		case len(focus.ProspectiveTargetRefs) != 0:
			return append([]string(nil), focus.ProspectiveTargetRefs...)
		case len(focus.TargetRefs) != 0:
			return append([]string(nil), focus.TargetRefs...)
		case len(focus.TargetSelectorCandidateRefs) != 0:
			return append([]string(nil), focus.TargetSelectorCandidateRefs...)
		}
	}
	for _, rawCatalog := range facts.Resources.OperationAdmissibilities {
		catalog := rawCatalog.Normalized()
		if catalog.OwnerModuleID == runtimecontract.RuntimeModuleFile && len(catalog.TargetRefs) != 0 {
			return append([]string(nil), catalog.TargetRefs...)
		}
	}
	if facts.Resources.Manifest != nil && len(facts.Resources.Manifest.TargetRefs) != 0 {
		return append([]string(nil), facts.Resources.Manifest.TargetRefs...)
	}
	if fallback == "" {
		return nil
	}
	return []string{fallback}
}

// runtimeTestSemanticCatalogTargets keeps the synthetic File catalog distinct
// from a prospective acquisition proposal. A generation-selected creation
// target is candidate input, not an already-existing owner target; exact
// terminal proposals still need their target published by the test catalog.
func runtimeTestSemanticCatalogTargets(facts runtimecontract.RuntimeFacts, seed runtimecontract.RuntimeDefinitionSeed, sources []runtimecontract.CoverageFocus, proposalTargets []string) []string {
	byCoverage := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(sources))
	for _, source := range sources {
		for _, coverageID := range source.CoverageWorkItemIDs {
			byCoverage[coverageID] = source.Normalized()
		}
	}
	targets := make([]string, 0, len(proposalTargets))
	for _, catalog := range facts.Resources.OperationAdmissibilities {
		catalog = catalog.Normalized()
		if catalog.OwnerModuleID == runtimecontract.RuntimeModuleFile {
			for _, target := range catalog.TargetRefs {
				targets = runtimeTestAppendUnique(targets, target)
			}
			break
		}
	}
	if facts.Resources.Manifest != nil {
		for _, target := range facts.Resources.Manifest.TargetRefs {
			targets = runtimeTestAppendUnique(targets, target)
		}
	}
	for index, item := range seed.CoverageItems {
		if index >= len(proposalTargets) {
			break
		}
		source := byCoverage[item.ID]
		prospective := len(source.ProspectiveTargetRefs) != 0
		if len(runtimecontract.RuntimeMissingOwnerFactRequirements(source.RequiredOwnerFacts, facts.Resources.OperationAdmissibilities)) != 0 {
			if convergence, required := runtimecontract.ResolveRuntimeOwnerFactAcquisition(source.RequiredOwnerFacts, source.OwnerFactConvergences, facts.Resources.OperationAdmissibilities); required {
				prospective = convergence.AcquisitionTargetPolicy == runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget
			}
		}
		if !prospective {
			targets = runtimeTestAppendUnique(targets, proposalTargets[index])
		}
	}
	return targets
}

// runtimeTestSemanticCoverageOrder derives a dependency-valid provider-order
// fixture without introducing an authority path in production.  The real
// order is provider-authored; this helper only makes the synthetic sealed
// ledger satisfy the same immutable order lineage contract as production.
func runtimeTestSemanticCoverageOrder(seed runtimecontract.RuntimeDefinitionSeed) ([]common.CoverageWorkItemID, error) {
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

func reduceRuntimeStateFixture(t *testing.T, facts runtimecontract.RuntimeFacts, prior *runtimecontract.AuthoritySnapshot) (runtimecontract.RuntimeDecision, error) {
	t.Helper()
	facts = runtimeTestFactsWithManagedValidationOwnerPreflight(facts)
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
	return ReduceRuntimeState(facts, prior)
}

// runtimeTestFactsWithManagedValidationOwnerPreflight keeps direct reducer
// fixtures on the production owner boundary: declaring validation evidence is
// not itself an executable validation authority.  The test must also contain
// the typed Shell preflight result that publishes a ready direct managed-
// validation surface.  State-change-only fixtures remain untouched.
func runtimeTestFactsWithManagedValidationOwnerPreflight(facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	const managedValidationPrerequisiteID = "operation-admissibility-shell-managed-validation"
	requiresValidation := false
	for _, kind := range facts.Focus.RequiredEvidenceKinds {
		if kind == runtimecontract.EvidenceKindValidation {
			requiresValidation = true
			break
		}
	}
	if !requiresValidation {
		return facts
	}
	readyCatalog := false
	for _, rawCatalog := range facts.Resources.OperationAdmissibilities {
		catalog := rawCatalog.Normalized()
		if catalog.OwnerModuleID != runtimecontract.RuntimeModuleShell {
			continue
		}
		for _, rawSurface := range catalog.ValidationSurfaces {
			surface := rawSurface.Normalized()
			if surface.OperationKind == runtimecontract.RuntimeBlockKindManagedValidationSession &&
				surface.Readiness == runtimecontract.RuntimeValidationSurfaceReady &&
				surface.ProofApplicability == runtimecontract.RuntimeValidationProofDirect {
				readyCatalog = true
				break
			}
		}
		if readyCatalog {
			break
		}
	}
	if !readyCatalog {
		facts = runtimeFactsWithManagedValidationRecipe(facts)
	}
	targetRefs := runtimeTestManagedValidationOwnerTargetRefs(facts)
	for index, rawCatalog := range facts.Resources.OperationAdmissibilities {
		catalog := rawCatalog.Normalized()
		if catalog.OwnerModuleID != runtimecontract.RuntimeModuleShell || catalog.PrerequisiteID != managedValidationPrerequisiteID {
			continue
		}
		for _, targetRef := range targetRefs {
			catalog.TargetRefs = runtimeTestAppendUnique(catalog.TargetRefs, targetRef)
		}
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
		facts.Resources.OperationAdmissibilities[index] = catalog
	}
	return facts
}

func runtimeTestManagedValidationOwnerTargetRefs(facts runtimecontract.RuntimeFacts) []string {
	focus := facts.Focus.Normalized()
	ret := append([]string(nil), focus.TargetRefs...)
	ret = append(ret, focus.ProspectiveTargetRefs...)
	ret = append(ret, focus.TargetSelectorCandidateRefs...)
	if len(ret) != 0 {
		return ret
	}
	coverageID := common.CoverageWorkItemID("")
	if len(focus.CoverageWorkItemIDs) != 0 {
		coverageID = focus.CoverageWorkItemIDs[0]
	}
	for index, item := range facts.DefinitionSeed.CoverageItems {
		if item.ID == coverageID {
			return []string{fmt.Sprintf("semantic-test-target-%d", index+1)}
		}
	}
	for index, item := range facts.Coverage {
		if item.ID == coverageID {
			return []string{fmt.Sprintf("semantic-test-target-%d", index+1)}
		}
	}
	return []string{"semantic-test-target-1"}
}

func runtimeOperationExecutionEnvelopeForTest(execution runtimecontract.RuntimeOperationExecutionRecord, grant *runtimecontract.RuntimeOperationGovernanceGrant) runtimecontract.RuntimeOperationExecutionEnvelope {
	ret := runtimecontract.RuntimeOperationExecutionEnvelope{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationHash: execution.OperationHash, ExpectedEffectScope: execution.ExpectedEffectScope,
		IdempotencyClass: execution.IdempotencyClass, AuthorityHash: execution.AuthorityHash,
		AdmissibilitySnapshotID: execution.AdmissibilitySnapshotID, AcquisitionBinding: execution.AcquisitionBinding,
	}
	if execution.ReplayProof != nil {
		proof := execution.ReplayProof.Clone()
		ret.ReplayProof = &proof
	}
	if grant != nil {
		value := grant.Normalized()
		ret.GovernanceGrant = &value
	}
	return ret
}

func runtimeTestFactsWithCurrentFocusBindings(t *testing.T, facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext) runtimecontract.RuntimeFacts {
	t.Helper()
	facts = runtimeTestFactsWithManagedValidationOwnerPreflight(facts)
	runtimeTestSealCurrentPolicies(&facts)
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
	aggregate := RuntimeAggregate{Facts: facts, Definition: definition}
	runtimeTestSealCurrentFocusBindings(t, &aggregate)
	runtimeTestEnsureSemanticPlanningLedger(t, &aggregate)
	provisionalFocusID := aggregate.Facts.Focus.ID
	runtimeTestProjectSemanticBindingPreimages(t, &aggregate)
	bindCurrentFocusTargetSelection(&aggregate.Facts)
	aggregate.Facts = runtimeTestRebindSyntheticFocusScopedFacts(aggregate.Facts, provisionalFocusID)
	aggregate.Facts = runtimeTestRebindSyntheticEvidenceToCurrentSemanticFocus(aggregate.Facts)
	aggregate.Facts.DefinitionFocuses = runtimeTestDefinitionFocuses(aggregate.Facts)
	runtimeTestSealCanonicalCWILedger(t, &aggregate)
	return aggregate.Facts
}

func runtimeTestRebindSyntheticFocusScopedFacts(facts runtimecontract.RuntimeFacts, provisionalFocusID string) runtimecontract.RuntimeFacts {
	if provisionalFocusID == "" || provisionalFocusID == facts.Focus.ID {
		return facts
	}
	if facts.Repair.FocusID == provisionalFocusID {
		facts.Repair.FocusID = facts.Focus.ID
	}
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == provisionalFocusID {
		directive := facts.RepairDirective.Normalized()
		directive.FocusID = facts.Focus.ID
		facts.RepairDirective = &directive
	}
	for index := range facts.Assessments {
		assessment := &facts.Assessments[index]
		if assessment.FocusID == provisionalFocusID && assessment.AdjudicationHash == "" {
			assessment.FocusID = facts.Focus.ID
		}
	}
	return facts
}

// runtimeTestRebindSyntheticEvidenceToCurrentSemanticFocus finishes the
// migration of direct reducer facts after their provider/owner semantic batch
// has replaced the provisional test binding. Only refs created by
// runtimeTestSealCurrentFocusBindings are rewritten; explicit or forged refs
// remain untouched for negative tests.
func runtimeTestRebindSyntheticEvidenceToCurrentSemanticFocus(facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	const provisionalBindingPrefix = "binding/test/runtime/"
	bindingsByCoverage := make(map[common.CoverageWorkItemID][]runtimecontract.CoverageEvidenceBinding, len(facts.Focus.CoverageEvidenceBindings))
	for _, rawBinding := range facts.Focus.CoverageEvidenceBindings {
		binding := rawBinding.Normalized()
		bindingsByCoverage[binding.CoverageWorkItemID] = append(bindingsByCoverage[binding.CoverageWorkItemID], binding)
	}
	rebind := func(values []runtimecontract.EvidenceFact) []runtimecontract.EvidenceFact {
		ret := append([]runtimecontract.EvidenceFact(nil), values...)
		for index := range ret {
			evidence := &ret[index]
			provisional := len(evidence.BindingRefs) == 0
			if len(evidence.BindingRefs) != 0 {
				provisional = true
				for _, ref := range evidence.BindingRefs {
					if len(ref) < len(provisionalBindingPrefix) || ref[:len(provisionalBindingPrefix)] != provisionalBindingPrefix {
						provisional = false
						break
					}
				}
			}
			if !provisional || len(evidence.CoverageWorkItemIDs) == 0 {
				continue
			}
			refs := make([]string, 0, len(evidence.CoverageWorkItemIDs))
			for _, coverageID := range evidence.CoverageWorkItemIDs {
				for _, binding := range bindingsByCoverage[coverageID] {
					if _, found := runtimecontract.CoverageEvidenceProofForKind(binding, evidence.Kind); found {
						refs = append(refs, binding.ID)
					}
				}
			}
			if len(refs) == 0 {
				continue
			}
			evidence.BindingRefs = runtimeTestNormalizeBindingRefs(refs)
			evidence.FocusID = facts.Focus.ID
			if evidence.DispatchID == "" {
				evidence.DispatchID = "dispatch/test/" + string(evidence.ID)
			}
			if evidence.OperationID == "" {
				evidence.OperationID = "operation/test/" + string(evidence.ID)
			}
		}
		return ret
	}
	facts.Evidence.Accepted = rebind(facts.Evidence.Accepted)
	facts.Evidence.Rejected = rebind(facts.Evidence.Rejected)
	return facts
}

func runtimeTestDefinitionFocuses(facts runtimecontract.RuntimeFacts) []runtimecontract.RuntimeDefinitionFocus {
	byCoverage := make(map[common.CoverageWorkItemID]runtimecontract.CoverageFocus, len(facts.DefinitionSeed.CoverageItems))
	for _, declaration := range facts.DefinitionFocuses {
		focus := declaration.CoverageFocusDeclaration()
		for _, coverageID := range focus.CoverageWorkItemIDs {
			byCoverage[coverageID] = focus
		}
	}
	for _, raw := range append(append([]runtimecontract.CoverageFocus{facts.Focus}, facts.RemainingFocuses...), facts.CompletedFocuses...) {
		focus := raw.Normalized()
		for _, coverageID := range focus.CoverageWorkItemIDs {
			if _, found := byCoverage[coverageID]; !found {
				byCoverage[coverageID] = focus
			}
		}
	}
	ret := make([]runtimecontract.RuntimeDefinitionFocus, 0, len(facts.DefinitionSeed.CoverageItems))
	for _, item := range facts.DefinitionSeed.CoverageItems {
		focus, found := byCoverage[item.ID]
		if !found {
			focus = runtimecontract.CoverageFocus{
				ID: "definition-test/" + string(item.ID), SliceID: item.SliceID,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			}
		}
		focus.ID = "definition-test/" + string(item.ID)
		focus.SliceID = item.SliceID
		focus.AssessmentScope = item.SourceScope
		focus.AssessmentID = item.SourceID
		focus.AssessmentText = item.TargetText
		focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{item.ID}
		if len(focus.RequiredEvidenceKinds) == 0 && !focus.RequireAssessment {
			focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
		}
		// Hand-written reducer fixtures often declare only the generation-side
		// selector ref or grounding flag. Complete the immutable declaration
		// identity explicitly before compiling it; production never synthesizes
		// these fields and receives all three from the admitted Profile.
		if focus.TargetSelectorRef == "" && len(focus.TargetDiscoverySources) != 0 {
			focus.TargetSelectorRef = "test/definition-selector/" + string(item.ID)
		}
		if focus.TargetSelectorRef != "" {
			if focus.TargetSelectorOwnerRef == "" {
				focus.TargetSelectorOwnerRef = "test-definition"
			}
			if focus.TargetSelectorSubjectRef == "" {
				focus.TargetSelectorSubjectRef = string(item.ID)
			}
		} else {
			focus.TargetSelectorOwnerRef = ""
			focus.TargetSelectorSubjectRef = ""
			focus.TargetSelectorCandidateRefs = nil
		}
		if focus.RequireTargetGrounding && focus.TargetRole == "" {
			focus.TargetRole = "test_target"
		}
		ret = append(ret, runtimecontract.CompileRuntimeDefinitionFocuses([]runtimecontract.CoverageFocus{focus})...)
	}
	return ret
}

// runtimeTestSealCurrentPolicies completes hand-written test fixtures through
// the same explicit constructors used by the fresh activation path. Runtime
// production validation never calls this helper and never fills persisted
// policy fields.
func runtimeTestSealCurrentPolicies(facts *runtimecontract.RuntimeFacts) {
	if facts == nil {
		return
	}
	if facts.Activation.ExecutionContract.OperationContractRevision == "" {
		facts.Activation.ExecutionContract.OperationContractRevision = runtimecontract.RuntimeOperationContractRevision()
	}
	if facts.Activation.ExecutionContract.StateChangePolicy == "" {
		facts.Activation.ExecutionContract.StateChangePolicy = runtimecontract.RuntimeStateChangeAllowedButNotRequired
	}
	convergence := runtimecontract.DefaultRuntimeConvergencePolicy()
	if facts.ConvergencePolicy.MaxRepairAttemptsPerFocus != 0 {
		convergence.MaxRepairAttemptsPerFocus = facts.ConvergencePolicy.MaxRepairAttemptsPerFocus
	}
	if facts.ConvergencePolicy.MaxValidationOnlyRetriesPerFocus != 0 {
		convergence.MaxValidationOnlyRetriesPerFocus = facts.ConvergencePolicy.MaxValidationOnlyRetriesPerFocus
	}
	if facts.ConvergencePolicy.MaxAcceptanceSliceWorkItems != 0 {
		convergence.MaxAcceptanceSliceWorkItems = facts.ConvergencePolicy.MaxAcceptanceSliceWorkItems
	}
	if facts.ConvergencePolicy.MaxFocuses != 0 {
		convergence.MaxFocuses = facts.ConvergencePolicy.MaxFocuses
	}
	if facts.ConvergencePolicy.MaxCandidateContextItems != 0 {
		convergence.MaxCandidateContextItems = facts.ConvergencePolicy.MaxCandidateContextItems
	}
	facts.ConvergencePolicy = convergence

	recovery := runtimecontract.DefaultRuntimeRecoveryPolicy()
	if facts.RecoveryPolicy.MaxProviderTransportAttempts != 0 {
		recovery.MaxProviderTransportAttempts = facts.RecoveryPolicy.MaxProviderTransportAttempts
	}
	if facts.RecoveryPolicy.MaxOperationRecoveryAttempts != 0 {
		recovery.MaxOperationRecoveryAttempts = facts.RecoveryPolicy.MaxOperationRecoveryAttempts
	}
	if facts.RecoveryPolicy.MaxCASReloadAttempts != 0 {
		recovery.MaxCASReloadAttempts = facts.RecoveryPolicy.MaxCASReloadAttempts
	}
	if facts.RecoveryPolicy.MaxTerminalDeliveryAttempts != 0 {
		recovery.MaxTerminalDeliveryAttempts = facts.RecoveryPolicy.MaxTerminalDeliveryAttempts
	}
	if facts.RecoveryPolicy.MaxDriveScheduleAttempts != 0 {
		recovery.MaxDriveScheduleAttempts = facts.RecoveryPolicy.MaxDriveScheduleAttempts
	}
	if facts.RecoveryPolicy.MaxPrerequisiteRefreshes != 0 {
		recovery.MaxPrerequisiteRefreshes = facts.RecoveryPolicy.MaxPrerequisiteRefreshes
	}
	facts.RecoveryPolicy = recovery
	facts.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(convergence, recovery)
}

func runtimeTestBindCandidateToFacts(t *testing.T, facts runtimecontract.RuntimeFacts, candidate runtimecontract.RuntimeCandidate) runtimecontract.RuntimeCandidate {
	t.Helper()
	return runtimeTestBindCandidateToCurrentFocus(t, RuntimeAggregate{Facts: facts}, candidate)
}

func hasRuntimeTestEvidenceKind(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

// runtimeTestBindCandidateToCurrentFocus is the scripted-provider fixture
// builder for the current sealed wire shape.
func runtimeTestBindCandidateToCurrentFocus(t *testing.T, aggregate RuntimeAggregate, candidate runtimecontract.RuntimeCandidate) runtimecontract.RuntimeCandidate {
	t.Helper()
	candidate = runtimeTestBindCandidateToFocusBindings(aggregate.Facts.Focus.CoverageEvidenceBindings, candidate)
	for _, operation := range candidateRuntimeOperations(candidate) {
		if len(operation.CoverageWorkItemIDs) != 0 {
			t.Fatalf("test fixture operation %q has no matching sealed binding", operation.ID)
		}
	}
	return candidate
}

// runtimeTestBindCandidateToFocusBindings converts fixture declarations into
// exact binding selections. Raw CWI IDs are never wire authority.
func runtimeTestBindCandidateToFocusBindings(bindings []runtimecontract.CoverageEvidenceBinding, candidate runtimecontract.RuntimeCandidate) runtimecontract.RuntimeCandidate {
	bind := func(operation *runtimecontract.RuntimeOperation) {
		if operation == nil || len(operation.BindingRefs) != 0 || len(operation.CoverageWorkItemIDs) == 0 {
			return
		}
		refs := make([]string, 0, len(operation.CoverageWorkItemIDs))
		for _, coverageID := range operation.CoverageWorkItemIDs {
			for _, binding := range bindings {
				if binding.CoverageWorkItemID != coverageID || !runtimeTestBindingAuthorizesOperation(binding, *operation) {
					continue
				}
				refs = append(refs, binding.ID)
			}
		}
		if len(refs) != 0 {
			operation.BindingRefs = runtimeTestNormalizeBindingRefs(refs)
		}
		// A fixture that cannot prove its declared CWI remains a non-credit
		// operation rather than borrowing a different binding.
		operation.CoverageWorkItemIDs = nil
	}
	if candidate.StateChange != nil {
		for index := range candidate.StateChange.Operations {
			bind(&candidate.StateChange.Operations[index].Operation)
		}
	}
	if candidate.Validation != nil {
		for index := range candidate.Validation.Operations {
			bind(&candidate.Validation.Operations[index].Operation)
		}
	}
	return candidate
}

func runtimeTestBindingAuthorizesOperation(binding runtimecontract.CoverageEvidenceBinding, operation runtimecontract.RuntimeOperation) bool {
	binding = binding.Normalized()
	operation = operation.Normalized()
	proofCapable := false
	for _, proof := range binding.Proofs {
		for _, kind := range proof.AllowedOperationKinds {
			if runtimecontract.CanonicalRuntimeBlockKind(kind) == runtimecontract.CanonicalRuntimeBlockKind(operation.Kind) {
				proofCapable = true
				break
			}
		}
		if proofCapable {
			break
		}
	}
	if !proofCapable {
		return false
	}
	authorization, err := runtimecontract.RuntimeOperationTargetAuthorizationFromCoverageEvidenceBinding(binding)
	if err != nil {
		return false
	}
	requireTargets := authorization.Policy != runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope ||
		len(authorization.OwnerScopeTargetRefs) != 0
	return runtimecontract.ValidateRuntimeOperationTargetsAgainstAuthorization(authorization, operation.TargetRefs, requireTargets) == nil
}

func runtimeTestNormalizeBindingRefs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

func attachRuntimeTestOwnerReceipt(t *testing.T, aggregate *RuntimeAggregate, evidence *ExecutionEvidence) {
	t.Helper()
	if aggregate == nil || aggregate.ActiveDispatch == nil || evidence == nil {
		t.Fatal("test owner receipt requires active dispatch and evidence")
	}
	var execution *runtimecontract.RuntimeOperationExecutionRecord
	for idx := range aggregate.ActiveDispatch.OperationExecutions {
		if aggregate.ActiveDispatch.OperationExecutions[idx].OperationID == evidence.OperationID {
			execution = &aggregate.ActiveDispatch.OperationExecutions[idx]
			break
		}
	}
	if execution == nil {
		t.Fatalf("operation %s has no sealed execution", evidence.OperationID)
	}
	if execution.Fence == 0 {
		execution.Fence = 1
	}
	var (
		targetRefs      []string
		sealedOperation runtimecontract.RuntimeOperation
		operationFound  bool
	)
	for _, operation := range candidateRuntimeOperations(aggregate.ActiveDispatch.Candidate) {
		if operation.ID == evidence.OperationID {
			targetRefs = append([]string(nil), operation.TargetRefs...)
			sealedOperation = operation.Normalized()
			operationFound = true
			break
		}
	}
	if !operationFound {
		t.Fatalf("operation %s is missing from sealed candidate", evidence.OperationID)
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: execution.ExpectedEffectScope,
		Changed: runtimecontract.RuntimeEffectIsDurable(execution.ExpectedEffectScope), TargetRefs: targetRefs, Evidence: evidence.Evidence,
	}
	if evidence.Status == ExecutionEvidenceStatusRejected {
		receipt.Outcome = runtimecontract.RuntimeOperationOutcomeRejected
		receipt.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
		receipt.Changed = false
		if evidence.Failure == nil {
			t.Fatalf("rejected evidence %q requires owner failure", evidence.OperationID)
		}
		receipt.Feedback = runtimeOperationFeedbackForTest(t, *aggregate, evidence.OperationID, *evidence.Failure)
	}
	// An explicitly supplied validation observation is an owner fact for this
	// test case and must survive receipt construction. Only a sealed,
	// proof-capable validation operation may receive the deterministic fixture
	// observation below; a file read or any other diagnostic operation must not
	// be promoted to a coverage-capable owner receipt by test infrastructure.
	if evidence.Kind == runtimecontract.EvidenceKindValidation && receipt.Evidence.Validation == nil {
		for _, ref := range sealedOperation.BindingRefs {
			binding, found := runtimecontract.CoverageEvidenceBindingForRef(aggregate.Facts.Focus.CoverageEvidenceBindings, ref)
			if !found {
				continue
			}
			proof, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
			if !found || !runtimecontract.RuntimeOperationKindSupportsCoverageEvidence(sealedOperation.Kind, runtimecontract.EvidenceKindValidation) {
				continue
			}
			receipt.Evidence.Validation = &runtimecontract.RuntimeValidationObservation{RecipeID: proof.ValidationRecipeID, PassedChecks: append([]string(nil), proof.RequiredCheckRefs...)}
			break
		}
	}
	receipt = receipt.Normalized()
	hash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	execution.State = runtimecontract.OperationExecutionOwnerCommitted
	if execution.ClaimedAt == "" {
		execution.ClaimedAt = runtimecontract.RuntimeOperationClaimMarkerInitial
	}
	execution.OwnerReceiptRef = "runtime-receipt/" + execution.ExecutionKey
	execution.OwnerReceiptHash = hash
	aggregate.OperationReceipts = append(aggregate.OperationReceipts, receipt)
	evidence.Receipt = &receipt
	evidence.ObservedEffectScope = receipt.ObservedEffectScope
	evidence.OwnerReceiptRef = execution.OwnerReceiptRef
	evidence.OwnerReceiptHash = hash
}
