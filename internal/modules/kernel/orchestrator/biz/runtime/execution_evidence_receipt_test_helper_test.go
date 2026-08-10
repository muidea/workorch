package runtime

import (
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeTestEnsureCurrentFocusBindings seals direct reducer fixtures with the
// same explicit binding shape produced by activationcore in production.
func runtimeTestEnsureCurrentFocusBindings(t *testing.T, aggregate *RuntimeAggregate) {
	t.Helper()
	runtimeTestSealCurrentFocusBindings(t, aggregate)
	runtimeTestEnsureSemanticPlanningLedger(t, aggregate)
	decision, err := ReduceRuntimeState(aggregate.Facts, nil)
	if err != nil {
		t.Fatalf("reduce test fixture with sealed coverage binding: %v", err)
	}
	aggregate.Decision = decision
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
			ID:                       "binding/test/runtime/" + focus.ID + "/" + string(coverageID),
			CoverageWorkItemID:       coverageID,
			FocusID:                  focus.ID,
			DefinitionRevision:       definitionRevision,
			Proofs:                   append([]runtimecontract.CoverageEvidenceProof(nil), proofs...),
			TargetRefs:               append([]string(nil), focus.TargetRefs...),
			TargetSelectorRef:        focus.TargetSelectorRef,
			TargetSelectorOwnerRef:   focus.TargetSelectorOwnerRef,
			TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef,
			ProspectiveTargetRef:     focus.ProspectiveTargetRef,
			DeclarationRef:           "test/runtime-reducer-fixture",
		}.Normalized()
		if len(binding.TargetRefs) == 0 && binding.TargetSelectorRef == "" && binding.ProspectiveTargetRef == "" {
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
	aggregate.Facts.Focus = focus
	bindEvidence := func(values []runtimecontract.EvidenceFact) []runtimecontract.EvidenceFact {
		ret := append([]runtimecontract.EvidenceFact(nil), values...)
		for index := range ret {
			evidence := &ret[index]
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
	}
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
	testTargets := make([]string, 0, len(seed.CoverageItems))
	for idx := range seed.CoverageItems {
		testTargets = append(testTargets, fmt.Sprintf("semantic-test-target-%d", idx+1))
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
		DefinitionRevision: aggregate.Facts.DefinitionRevision, DefinitionSeedHash: seed.Hash,
		OperationContractRevision: operationContractRevision,
		CoverageItems:             seed.CoverageItems,
		AllowedOwnerModuleIDs:     []string{runtimecontract.RuntimeModuleFile},
		AllowedOperationKinds:     []string{runtimecontract.StepKindFileWrite},
		AllowedTargetPolicies:     []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		OwnerCatalogs:             []runtimecontract.RuntimeOperationAdmissibility{catalog},
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
	batch, err := runtimecontract.CompileRuntimeSemanticPlanFocusBatch(
		manifest, orderDocument, 0, "test-semantic-request", activeCoverageIDs, coverageOrder, len(coverageOrder),
	)
	if err != nil {
		return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("build test semantic focus batch: %w", err)
	}
	for _, next := range []runtimecontract.RuntimeSemanticPlanBatchState{
		runtimecontract.RuntimeSemanticPlanBatchRequesting,
		runtimecontract.RuntimeSemanticPlanBatchDocumentComplete,
	} {
		batch, err = runtimecontract.AdvanceRuntimeSemanticPlanBatch(batch, next)
		if err != nil {
			return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("advance test semantic batch: %w", err)
		}
	}
	selected := make(map[common.CoverageWorkItemID]struct{}, len(batch.CoverageWorkItemIDs))
	for _, id := range batch.CoverageWorkItemIDs {
		selected[id] = struct{}{}
	}
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: seed.Hash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID:  batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
	}
	feasibilities := make([]runtimecontract.RuntimeSemanticProofFeasibility, 0, len(batch.CoverageWorkItemIDs))
	for idx, item := range seed.CoverageItems {
		if _, ok := selected[item.ID]; !ok {
			continue
		}
		target := fmt.Sprintf("semantic-test-target-%d", idx+1)
		proposal := runtimecontract.RuntimeSemanticProofProposal{
			CoverageWorkItemID: item.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OperationKind:         runtimecontract.StepKindFileWrite,
			Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{target}},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			DeclarationRef:        "test/semantic/" + string(item.ID),
		}
		document.Proposals = append(document.Proposals, proposal)
		feasibilities = append(feasibilities, runtimecontract.RuntimeSemanticProofFeasibility{
			SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
			ManifestHash:  manifest.Hash, DefinitionSeedHash: seed.Hash,
			CoverageWorkItemID: item.ID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal),
			OwnerModuleID: runtimecontract.RuntimeModuleFile, CatalogSnapshotID: catalog.SnapshotID,
			Status: runtimecontract.RuntimeSemanticFeasibilityFeasible, ObservedRevision: "test-semantic-revision", TargetRefs: []string{target},
		})
	}
	if len(document.Proposals) != len(batch.CoverageWorkItemIDs) {
		return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("test semantic focus proposals do not cover batch")
	}
	document = document.Normalized()
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("build test semantic proof document: %w", err)
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
			return runtimecontract.RuntimeSemanticPlanningLedger{}, fmt.Errorf("accept test semantic batch: %w", err)
		}
	}
	merged := document.Clone()
	ledger.Phase = runtimecontract.RuntimeSemanticPlanningSealed
	ledger.Manifest = &manifest
	ledger.CoverageOrderDocument = &orderDocument
	ledger.Batches = []runtimecontract.RuntimeSemanticPlanBatch{batch}
	ledger.BatchDocuments = []runtimecontract.RuntimeSemanticPlanDocument{document}
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
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{Revision: facts.DefinitionRevision})
	return ReduceRuntimeState(facts, prior)
}

func runtimeTestFactsWithCurrentFocusBindings(t *testing.T, facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext) runtimecontract.RuntimeFacts {
	t.Helper()
	aggregate := RuntimeAggregate{Facts: facts, Definition: definition}
	runtimeTestSealCurrentFocusBindings(t, &aggregate)
	runtimeTestEnsureSemanticPlanningLedger(t, &aggregate)
	return aggregate.Facts
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
	if len(binding.TargetRefs) != 0 {
		return sameRuntimeTargetRefs(binding.TargetRefs, operation.TargetRefs)
	}
	if binding.ProspectiveTargetRef != "" {
		return sameRuntimeTargetRefs([]string{binding.ProspectiveTargetRef}, operation.TargetRefs)
	}
	return binding.TargetSelectorRef != ""
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
	execution.OwnerReceiptRef = "runtime-receipt/" + execution.ExecutionKey
	execution.OwnerReceiptHash = hash
	aggregate.OperationReceipts = append(aggregate.OperationReceipts, receipt)
	evidence.Receipt = &receipt
	evidence.ObservedEffectScope = receipt.ObservedEffectScope
	evidence.OwnerReceiptRef = execution.OwnerReceiptRef
	evidence.OwnerReceiptHash = hash
}
