package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func historicalBehaviorFixture(t *testing.T) RuntimeLineageAssertionSources {
	t.Helper()
	definition, execution, seed := purposeEndpointFixture(t)
	var item RuntimeCoverageItem
	for _, candidate := range seed.CoverageItems {
		if candidate.ID == "history" {
			item = candidate
		}
	}
	semantic, binding := historicalBehaviorSemanticFixture(t, definition, seed, item)
	ref := RuntimeCWIBindingRefForCoverageBinding(binding, semantic.Manifest.Hash)
	facts := RuntimeFacts{WorkspaceID: "ws", RunID: "run", DefinitionRevision: definition.Revision, DefinitionSeed: seed,
		CWILedgerSchemaVersion: RuntimeCWILedgerSchemaCurrent, Activation: RuntimeActivationContext{ExecutionContract: execution}, Coverage: InitialRuntimeCoverageFacts(seed.CoverageItems)}
	if facts.Coverage[0].ID != item.ID {
		facts.Coverage[0], facts.Coverage[1] = facts.Coverage[1], facts.Coverage[0]
	}
	facts.Coverage[0].CWIConvergenceState, facts.Coverage[0].CurrentBindingRef = RuntimeCWIConvergenceStateBound, &ref
	facts.Coverage[0].FocusID = binding.FocusID
	facts.Coverage[0].BindingLineage = []RuntimeCWIBindingRef{ref}
	facts.Coverage[1].CWIConvergenceState, facts.Coverage[1].FocusID = RuntimeCWIConvergenceStateUnbound, "acceptance-focus"
	facts.Focus = CoverageFocus{ID: binding.FocusID, CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, CoverageEvidenceBindings: []CoverageEvidenceBinding{binding}}
	facts.RemainingFocuses = []CoverageFocus{{ID: "acceptance-focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"current"}}}
	operation := RuntimeOperation{ID: "effect", Kind: StepKindFileWrite, BindingRefs: []string{binding.ID}, TargetRefs: binding.TargetRefs}.Normalized()
	receipt := runtimeOperationReceiptLineageFixture(StableRuntimeOperationExecutionKey(facts.WorkspaceID, facts.RunID, "dispatch", operation.ID), 1, operation.ID)
	receipt.TargetRefs = append([]string(nil), operation.TargetRefs...)
	dependency := historicalEffectPurposeFixture()
	dependency.OwnerRevisionSet.DefinitionRevision = definition.Revision
	*dependency.OwnerRevisionSet = dependency.OwnerRevisionSet.WithHash()
	dependency.ApplicabilityDependencies.OwnerRevisionSet.DefinitionRevision = definition.Revision
	dependency.ApplicabilityDependencies.OwnerRevisionSet = dependency.ApplicabilityDependencies.OwnerRevisionSet.WithHash()
	dependency.ApplicabilityDependencies.OperationKind = operation.Kind
	receipt.Evidence.Quality = common.EvidenceQualityValid
	receipt.Evidence.OwnerRevisionSet, receipt.Evidence.ApplicabilityDependencies = dependency.OwnerRevisionSet, dependency.ApplicabilityDependencies
	exec := runtimeOperationReceiptLineageExecution(receipt, OperationExecutionEvidenceCommitted)
	exec.OperationHash = StableRuntimeOperationHash(operation)
	ownerEvidence := receipt.Evidence
	fact := EvidenceFact{ID: common.EvidenceRecordID("runtime-receipt|" + exec.OwnerReceiptHash), Kind: EvidenceKindStateChange, FocusID: binding.FocusID,
		SubjectRef: operation.ID, SourceRef: exec.OwnerReceiptRef, AuthoritySnapshotID: "authority", DispatchID: "dispatch", OperationID: operation.ID,
		BindingRefs: operation.BindingRefs, TargetRefs: receipt.TargetRefs, CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID},
		Summary: ownerEvidence.Summary, ContentHash: ownerEvidence.ContentHash, Quality: ownerEvidence.Quality, OwnerRevisionSet: ownerEvidence.OwnerRevisionSet,
		ApplicabilityDependencies: ownerEvidence.ApplicabilityDependencies}
	exec.EvidenceFactID, exec.EvidenceFactHash = fact.ID, StableRuntimeEvidenceFactHash(fact)
	exec.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(exec)
	facts.Evidence.Accepted = []EvidenceFact{fact}
	dispatch := RuntimeDispatchRecord{ID: "dispatch", WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, State: RuntimeDispatchStateCommitted,
		Transaction:      AuthorityTransaction{State: ConvergenceStateNeedStateChange, FocusID: binding.FocusID, DefinitionRevision: definition.Revision, AuthoritySnapshotID: fact.AuthoritySnapshotID},
		CoverageBindings: []CoverageEvidenceBinding{binding}, CompletedOperationIDs: []string{operation.ID}, OperationExecutions: []RuntimeOperationExecutionRecord{exec},
		Candidate: RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: operation}}}}}
	return RuntimeLineageAssertionSources{Definition: definition, Facts: facts, Semantic: semantic, Completed: []RuntimeDispatchRecord{dispatch}, Receipts: []RuntimeOperationReceipt{receipt}, ProcessedEventIDs: []string{string(fact.ID)}}
}

// Persisted-shape accepted-batch fixture, compiled by the same pure semantic
// compiler as production. No local binding hash stands in for owner acceptance.
func historicalBehaviorSemanticFixture(t *testing.T, definition RuntimeDefinitionContext, seed RuntimeDefinitionSeed, item RuntimeCoverageItem) (RuntimeSemanticPlanningLedger, CoverageEvidenceBinding) {
	t.Helper()
	manifest, _, _, source := semanticIndependentProofSetFixture(t)
	manifest.WorkspaceID, manifest.RunID = "ws", "run"
	manifest.DefinitionRevision, manifest.DefinitionSeedHash = definition.Revision, seed.Hash
	manifest.CoverageItems = []RuntimeCoverageItem{item}
	source.ID, source.CoverageWorkItemIDs = "historical-focus", []common.CoverageWorkItemID{item.ID}
	source.EvidenceDeclarationRef = CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID)
	source.RequiredEvidenceKinds, source.AcceptedStepKinds = []string{EvidenceKindStateChange}, []string{StepKindFileWrite}
	source.ValidationRecipeID, source.RequiredCheckRefs = "", nil
	manifest = sealSemanticTestManifestWithSource(t, manifest, source)
	proposal := RuntimeSemanticProofProposal{CoverageWorkItemID: item.ID, OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite,
		Target:                RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"main.go"}},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, DeclarationRef: source.EvidenceDeclarationRef}
	order := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: seed.Hash,
		PlanStep: RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{item.ID}}
	requestID := "original-historical-request"
	batch := semanticContractTestPendingBatch(t, manifest, order, 0, requestID, order.CoverageOrder, order.CoverageOrder)
	document := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: seed.Hash,
		PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: batch.BatchID, BatchSequence: batch.Sequence, BatchCoverageWorkItemIDs: order.CoverageOrder, Proposals: []RuntimeSemanticProofProposal{proposal}}
	feasibilities := []RuntimeSemanticProofFeasibility{semanticFeasibilityForTest(t, manifest, proposal, proposal.Target.ExactTargetRefs, "", nil)}
	compiled, _, err := CompileRuntimeSemanticBindings(manifest, requestID, document, feasibilities)
	if err != nil {
		t.Fatal(err)
	}
	batch.State, batch.DocumentHash = RuntimeSemanticPlanBatchAccepted, StableRuntimeSemanticPlanDocumentHash(document)
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	ledger := RuntimeSemanticPlanningLedger{SchemaVersion: RuntimeSemanticPlanningLedgerSchemaCurrent, Phase: RuntimeSemanticPlanningSealed,
		DefinitionSeed: seed, Manifest: &manifest, StablePrefixHash: "original-stable-prefix", LogicalRequestID: requestID,
		CoverageOrderDocument: &order, Document: &document, DocumentHash: batch.DocumentHash, Batches: []RuntimeSemanticPlanBatch{batch},
		NextBatchSequence: 1, CompletedBatchIDs: []string{batch.BatchID}, BatchDocuments: []RuntimeSemanticPlanDocument{document},
		MergedDocument: &document, MergedDocumentHash: batch.DocumentHash, FeasibilityBatchID: StableRuntimeSemanticFeasibilityBatchID(manifest, document),
		Feasibilities: feasibilities, FeasibilitySetHash: StableRuntimeSemanticFeasibilitySetHash(feasibilities)}
	if _, err := ResolveRuntimeAssessmentPurposePreimage(ledger, compiled[0].CoverageEvidenceBindings); err != nil {
		t.Fatalf("accepted historical semantic preimage: %v", err)
	}
	return ledger, compiled[0].CoverageEvidenceBindings[0]
}

func TestHistoricalBehaviorUnlocksOnlyExactConsumerWithoutTerminalCredit(t *testing.T) {
	sources := historicalBehaviorFixture(t)
	before, _ := json.Marshal(sources)
	completion, err := CompileRuntimeHistoricalBehaviorCompletion(sources, "history", "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(sources)
	if !reflect.DeepEqual(before, after) || sources.Facts.Coverage[0].State != RuntimeCoverageItemStateOpen {
		t.Fatal("pure completion compiler changed sources or granted terminal credit")
	}
	sources.Facts.Coverage[0].HistoricalBehavior = completion
	if err := ValidateRuntimeHistoricalBehaviorCompletions(sources); err != nil {
		t.Fatal(err)
	}
	row := sources.Facts.Coverage[0]
	if !RuntimeCWIPrerequisiteReadyForConsumer(sources.Facts, row, "current") || RuntimeCWIPrerequisiteReadyForConsumer(sources.Facts, row, "other") {
		t.Fatal("historical progress did not unlock only original exact acceptance source")
	}
	if got := RuntimeCWIsEligibleForPlanning(sources.Facts); !reflect.DeepEqual(got, []common.CoverageWorkItemID{"current"}) {
		t.Fatalf("planning consumed historical terminal credit: %v", got)
	}
	sources.Facts.Focus, sources.Facts.RemainingFocuses[0] = sources.Facts.RemainingFocuses[0], sources.Facts.Focus
	if blocked := runtimeCurrentFocusBlockedPrerequisites(sources.Facts); len(blocked) != 0 {
		t.Fatalf("final admission disagrees with exact planning prerequisite: %v", blocked)
	}
	body, _ := json.Marshal(sources)
	var reload RuntimeLineageAssertionSources
	if json.Unmarshal(body, &reload) != nil || ValidateRuntimeHistoricalBehaviorCompletions(reload) != nil || !reflect.DeepEqual(reload, sources) {
		t.Fatal("completion source changed on JSON restart")
	}
	raw := (RuntimeStateRecord{Facts: sources.Facts}).Clone()
	raw.Facts.Coverage[0].HistoricalBehavior.EvidenceRefs[0] = "changed"
	normalized := sources.Facts.Normalized()
	for index := range normalized.Coverage {
		if normalized.Coverage[index].HistoricalBehavior != nil {
			normalized.Coverage[index].HistoricalBehavior.Source.AcceptanceItemRef = "changed"
		}
	}
	if sources.Facts.Coverage[0].HistoricalBehavior.EvidenceRefs[0] == "changed" || completion.Source.AcceptanceItemRef == "changed" {
		t.Fatal("completion clone aliases original source")
	}
}

func TestHistoricalBehaviorRejectsPartialWeakUnconfirmedAndCoordinatedLineage(t *testing.T) {
	for name, mutate := range map[string]func(*RuntimeLineageAssertionSources){
		"partial":     func(s *RuntimeLineageAssertionSources) { s.Completed[0].CompletedOperationIDs = nil },
		"active":      func(s *RuntimeLineageAssertionSources) { s.Active = &s.Completed[0]; s.Completed = nil },
		"invalidated": func(s *RuntimeLineageAssertionSources) { s.Completed[0].State = RuntimeDispatchStateInvalidated },
		"owner only": func(s *RuntimeLineageAssertionSources) {
			s.Completed[0].OperationExecutions[0].State = OperationExecutionOwnerCommitted
		},
		"no receipt":        func(s *RuntimeLineageAssertionSources) { s.Receipts = nil },
		"wrong fence":       func(s *RuntimeLineageAssertionSources) { s.Receipts[0].Fence++ },
		"duplicate receipt": func(s *RuntimeLineageAssertionSources) { s.Receipts = append(s.Receipts, s.Receipts[0]) },
		"zero work":         func(s *RuntimeLineageAssertionSources) { s.Receipts[0].Evidence.ZeroWork = true },
		"unprocessed":       func(s *RuntimeLineageAssertionSources) { s.ProcessedEventIDs = nil },
		"no confirmation":   func(s *RuntimeLineageAssertionSources) { s.Definition.SourceObligations = nil },
		"effect as credit":  func(s *RuntimeLineageAssertionSources) { s.Facts.Coverage[0].State = RuntimeCoverageItemStateSatisfied },
		"missing evidence":  func(s *RuntimeLineageAssertionSources) { s.Facts.Evidence.Accepted = nil },
		"different source": func(s *RuntimeLineageAssertionSources) {
			s.Completed[0].CoverageBindings[0].HistoricalAcceptance.AcceptanceChecksHash = runtimeHistoricalAcceptanceChecksHash(nil)
			s.Completed[0].CoverageBindings[0].Hash = StableCoverageEvidenceBindingHash(s.Completed[0].CoverageBindings[0])
			s.Facts.Coverage[0].CurrentBindingRef.BindingHash = s.Completed[0].CoverageBindings[0].Hash
		},
		"no semantic": func(s *RuntimeLineageAssertionSources) { s.Semantic = RuntimeSemanticPlanningLedger{} },
		"semantic startup substitution": func(s *RuntimeLineageAssertionSources) {
			s.Semantic.DefinitionSeed.Hash = "another-startup-seed"
		},
		"no accepted document":      func(s *RuntimeLineageAssertionSources) { s.Semantic.BatchDocuments = nil },
		"missing owner feasibility": func(s *RuntimeLineageAssertionSources) { s.Semantic.Feasibilities = nil },
		"different original manifest": func(s *RuntimeLineageAssertionSources) {
			s.Facts.Coverage[0].CurrentBindingRef.SemanticManifestHash = "replacement"
		},
		"rehashed binding": func(s *RuntimeLineageAssertionSources) {
			s.Completed[0].CoverageBindings[0].ID = "replacement"
			s.Completed[0].CoverageBindings[0].Hash = StableCoverageEvidenceBindingHash(s.Completed[0].CoverageBindings[0])
			*s.Facts.Coverage[0].CurrentBindingRef = RuntimeCWIBindingRefForCoverageBinding(s.Completed[0].CoverageBindings[0], s.Semantic.Manifest.Hash)
		},
		"repair": func(s *RuntimeLineageAssertionSources) {
			s.Completed[0].RepairCommitment = &RuntimeDispatchRepairCommitment{}
		},
		"work": func(s *RuntimeLineageAssertionSources) {
			s.Completed[0].ImplementationWorkCommitment = &RuntimeImplementationWorkRound{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			sources := historicalBehaviorFixture(t)
			mutate(&sources)
			if _, err := CompileRuntimeHistoricalBehaviorCompletion(sources, "history", "dispatch"); err == nil {
				t.Fatal("invalid historical completion accepted")
			}
		})
	}
	sources := historicalBehaviorFixture(t)
	completion, err := CompileRuntimeHistoricalBehaviorCompletion(sources, "history", "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	sources.Facts.Coverage[0].HistoricalBehavior = completion
	completion.EvidenceRefs = append(completion.EvidenceRefs, completion.EvidenceRefs[0])
	if ValidateRuntimeHistoricalBehaviorCompletions(sources) == nil || RuntimeCWIPrerequisiteReadyForConsumer(sources.Facts, sources.Facts.Coverage[0], "current") {
		t.Fatal("self-consistent completion label bypassed independently compiled member set")
	}
}
