package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Pure persisted-shape provenance fixture only. Both independent bindings are
// compiled from one complete typed batch so availability stays fail-closed;
// this does not exercise production eligible-closure scheduling or owner I/O.
func historicalAcceptanceProofFixture(t *testing.T) RuntimeLineageAssertionSources {
	t.Helper()
	sources := historicalBehaviorFixture(t)
	items := sources.Facts.DefinitionSeed.CoverageItems
	var current RuntimeCoverageItem
	for _, item := range items {
		if item.ID == "current" {
			current = item
		}
	}
	historyFocus := sources.Facts.Focus.Normalized()
	historyFocus.EvidenceDeclarationRef = historyFocus.CoverageEvidenceBindings[0].DeclarationRef
	historyFocus.RequiredEvidenceKinds, historyFocus.AcceptedStepKinds = []string{EvidenceKindStateChange}, []string{StepKindFileWrite}
	for _, item := range items {
		if item.ID == "history" {
			historyFocus.SliceID, historyFocus.AssessmentScope, historyFocus.AssessmentID, historyFocus.AssessmentText = item.SliceID, item.SourceScope, item.SourceID, item.TargetText
		}
	}
	currentFocus := CoverageFocus{ID: "ordinary-source", SliceID: current.SliceID, AssessmentScope: current.SourceScope, AssessmentID: current.SourceID, AssessmentText: current.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{current.ID}, RequiredEvidenceKinds: []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession},
		EvidenceDeclarationRef: CanonicalRuntimeDefinitionEvidenceDeclarationRef(current.SourceScope, current.SourceID), ValidationRecipeID: "recipe/validate", RequireAssessment: true, RequireExecutableBehaviorEvidence: true}
	sources.Facts.DefinitionFocuses = CompileRuntimeDefinitionFocuses([]CoverageFocus{historyFocus, currentFocus})
	requirement := runtimeValidationProofRequirementForTest(t, sources.Definition.Revision, current, currentFocus.EvidenceDeclarationRef, currentFocus.ValidationRecipeID, nil)
	manifest := sources.Semantic.Manifest.Clone()
	manifest.CoverageItems = NormalizeRuntimeCoverageItems(items)
	for index := range manifest.OwnerCatalogs {
		catalog := &manifest.OwnerCatalogs[index]
		if catalog.OwnerModuleID == RuntimeModuleShell {
			catalog.ValidationSurfaces = []RuntimeValidationSurface{bindRuntimeValidationSurfaceForTest(RuntimeValidationSurface{ID: currentFocus.ValidationRecipeID, OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
				Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "original-input-schema", InputBindingRef: "original-input", EffectScope: RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce}, requirement)}
		}
		catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	}
	var err error
	manifest.ProofObligations, err = compileRuntimeSemanticProofObligations(manifest.DefinitionRevision, CompileRuntimeDefinitionFocuses([]CoverageFocus{historyFocus, currentFocus}), manifest.CoverageItems, manifest.OwnerCatalogs)
	if err != nil {
		t.Fatal(err)
	}
	manifest = sealSemanticTestManifest(t, manifest)
	historicalProposal := sources.Semantic.BatchDocuments[0].Proposals[0].Clone()
	ordinaryProposal := RuntimeSemanticProofProposal{CoverageWorkItemID: current.ID, OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{"main.go"}}, RequiredEvidenceKinds: []string{EvidenceKindValidation},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession}, ValidationRecipeID: currentFocus.ValidationRecipeID, DeclarationRef: currentFocus.EvidenceDeclarationRef}
	order := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{"history", "current"}}
	requestID := "original-composite-request"
	batch := semanticContractTestPendingBatch(t, manifest, order, 0, requestID, order.CoverageOrder, order.CoverageOrder)
	document := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID: batch.BatchID, BatchSequence: batch.Sequence, BatchCoverageWorkItemIDs: order.CoverageOrder, Proposals: []RuntimeSemanticProofProposal{historicalProposal, ordinaryProposal}}
	feasibilities := []RuntimeSemanticProofFeasibility{semanticFeasibilityForTest(t, manifest, historicalProposal, []string{"main.go"}, "", nil),
		semanticFeasibilityForTest(t, manifest, ordinaryProposal, []string{"main.go"}, currentFocus.ValidationRecipeID, requirement.RequiredCheckRefs)}
	focuses, _, err := CompileRuntimeSemanticBindings(manifest, requestID, document, feasibilities)
	if err != nil {
		t.Fatal(err)
	}
	var historical, ordinary CoverageFocus
	for _, focus := range focuses {
		if focus.CoverageWorkItemIDs[0] == "history" {
			historical = focus
		} else {
			ordinary = focus
		}
	}
	batch.State, batch.DocumentHash = RuntimeSemanticPlanBatchAccepted, StableRuntimeSemanticPlanDocumentHash(document)
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	sources.Semantic = RuntimeSemanticPlanningLedger{SchemaVersion: RuntimeSemanticPlanningLedgerSchemaCurrent, Phase: RuntimeSemanticPlanningSealed, DefinitionSeed: sources.Facts.DefinitionSeed,
		Manifest: &manifest, StablePrefixHash: "original-prefix", LogicalRequestID: requestID, CoverageOrderDocument: &order, Document: &document, DocumentHash: batch.DocumentHash,
		Batches: []RuntimeSemanticPlanBatch{batch}, NextBatchSequence: 1, CompletedBatchIDs: []string{batch.BatchID}, BatchDocuments: []RuntimeSemanticPlanDocument{document},
		MergedDocument: &document, MergedDocumentHash: batch.DocumentHash, FeasibilityBatchID: StableRuntimeSemanticFeasibilityBatchID(manifest, document), Feasibilities: feasibilities, FeasibilitySetHash: StableRuntimeSemanticFeasibilitySetHash(feasibilities)}
	historyBinding := historical.CoverageEvidenceBindings[0]
	historyRef := RuntimeCWIBindingRefForCoverageBinding(historyBinding, manifest.Hash)
	sources.Facts.Focus, sources.Facts.RemainingFocuses = ordinary, nil
	sources.Facts.CompletedFocuses = []CoverageFocus{historical}
	for index := range sources.Facts.Coverage {
		row := &sources.Facts.Coverage[index]
		if row.ID == "history" {
			row.FocusID, row.CurrentBindingRef, row.BindingLineage = historical.ID, &historyRef, []RuntimeCWIBindingRef{historyRef}
		} else {
			ref := RuntimeCWIBindingRefForCoverageBinding(ordinary.CoverageEvidenceBindings[0], manifest.Hash)
			row.FocusID, row.CurrentBindingRef, row.BindingLineage, row.CWIConvergenceState = ordinary.ID, &ref, []RuntimeCWIBindingRef{ref}, RuntimeCWIConvergenceStateBound
		}
	}
	historyDispatch := &sources.Completed[0]
	historyDispatch.Transaction.FocusID, historyDispatch.CoverageBindings = historical.ID, historical.CoverageEvidenceBindings
	historyDispatch.Candidate.StateChange.Operations[0].Operation.BindingRefs = []string{historyBinding.ID}
	op := historyDispatch.Candidate.StateChange.Operations[0].Operation
	historyFact := &sources.Facts.Evidence.Accepted[0]
	historyFact.FocusID, historyFact.BindingRefs, historyFact.Revision = historical.ID, []string{historyBinding.ID}, runtimeEvidenceRevisionForSequence(1)
	historyExecution := &historyDispatch.OperationExecutions[0]
	historyExecution.OperationHash, historyExecution.EvidenceFactHash = StableRuntimeOperationHash(op), StableRuntimeEvidenceFactHash(*historyFact)
	historyExecution.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(*historyExecution)
	completion, err := CompileRuntimeHistoricalBehaviorCompletion(sources, "history", historyDispatch.ID)
	if err != nil {
		t.Fatal(err)
	}
	sources.Facts.Coverage[0].HistoricalBehavior = completion
	binding := ordinary.CoverageEvidenceBindings[0]
	proof, _ := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
	validation := RuntimeValidationObservation{RecipeID: proof.ValidationRecipeID, ObservationProtocolRef: proof.ValidationCheckAttestations[0].ObservationProtocolRef}
	for _, attestation := range proof.ValidationCheckAttestations {
		validation.ExecutedChecks = append(validation.ExecutedChecks, attestation.NativeCheckRef)
		validation.PassedChecks = append(validation.PassedChecks, attestation.NativeCheckRef)
	}
	operation := RuntimeOperation{ID: "ordinary-validation", Kind: RuntimeBlockKindManagedValidationSession, BindingRefs: []string{binding.ID}, TargetRefs: binding.TargetRefs}.Normalized()
	receipt := runtimeOperationReceiptLineageFixture(StableRuntimeOperationExecutionKey(sources.Facts.WorkspaceID, sources.Facts.RunID, "ordinary-dispatch", operation.ID), 1, operation.ID)
	receipt.TargetRefs, receipt.ObservedEffectScope = operation.TargetRefs, RuntimeEffectEphemeralSession
	receipt.Evidence.Validation, receipt.Evidence.Quality = &validation, common.EvidenceQualityValid
	dependencies := (RuntimeOwnerRevisionSet{DefinitionRevision: sources.Definition.Revision, Entries: []RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: "shell/ws", ResourceKind: "validation_input", ResourceID: "main.go", Revision: "actually-read-current-bytes", DependencyHash: "original-selector"}}}).WithHash()
	receipt.Evidence.OwnerRevisionSet = &dependencies
	receipt.Evidence.ApplicabilityDependencies = &RuntimeEvidenceApplicabilityDependencies{ProtocolRef: RuntimeEvidenceApplicabilityDependenciesV1, OwnerRevisionSet: dependencies}
	execution := runtimeOperationReceiptLineageExecution(receipt, OperationExecutionEvidenceCommitted)
	execution.ExpectedEffectScope, execution.OperationHash = RuntimeEffectEphemeralSession, StableRuntimeOperationHash(operation)
	fact := EvidenceFact{ID: common.EvidenceRecordID("runtime-receipt|" + execution.OwnerReceiptHash), Kind: EvidenceKindValidation, FocusID: ordinary.ID, Revision: runtimeEvidenceRevisionForSequence(2),
		SubjectRef: operation.ID, SourceRef: execution.OwnerReceiptRef, AuthoritySnapshotID: "ordinary-authority", DispatchID: "ordinary-dispatch", OperationID: operation.ID,
		BindingRefs: operation.BindingRefs, TargetRefs: operation.TargetRefs, CoverageWorkItemIDs: []common.CoverageWorkItemID{current.ID}, Summary: receipt.Evidence.Summary, ContentHash: receipt.Evidence.ContentHash,
		Quality: receipt.Evidence.Quality, OwnerRevisionSet: receipt.Evidence.OwnerRevisionSet, ApplicabilityDependencies: receipt.Evidence.ApplicabilityDependencies, Validation: receipt.Evidence.Validation}
	execution.EvidenceFactID, execution.EvidenceFactHash = fact.ID, StableRuntimeEvidenceFactHash(fact)
	execution.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(execution)
	sources.Facts.Evidence.Accepted = append(sources.Facts.Evidence.Accepted, fact)
	sources.Receipts = append(sources.Receipts, receipt)
	sources.ProcessedEventIDs = append(sources.ProcessedEventIDs, string(fact.ID))
	sources.Completed = append(sources.Completed, RuntimeDispatchRecord{ID: fact.DispatchID, WorkspaceID: sources.Facts.WorkspaceID, RunID: sources.Facts.RunID, State: RuntimeDispatchStateCommitted,
		Transaction: AuthorityTransaction{FocusID: ordinary.ID, DefinitionRevision: sources.Definition.Revision, AuthoritySnapshotID: fact.AuthoritySnapshotID}, CoverageBindings: ordinary.CoverageEvidenceBindings,
		Candidate: RuntimeCandidate{Validation: &ValidationCandidate{Operations: []ValidationOperation{{Operation: operation}}}}, CompletedOperationIDs: []string{operation.ID}, OperationExecutions: []RuntimeOperationExecutionRecord{execution}})
	cutoff := fact.Revision
	observation := RuntimeEvidenceApplicabilityObservation{Request: (RuntimeEvidenceApplicabilityRequest{ProtocolRef: RuntimeEvidenceApplicabilityObservationV1, IssuedAtStateRevision: 10, EvidenceRevision: cutoff,
		SourceEvidenceID: fact.ID, SourceEvidenceHash: StableRuntimeEvidenceFactHash(fact), SourceReceiptHash: StableRuntimeOperationReceiptHash(receipt), DispatchID: fact.DispatchID, OperationID: fact.OperationID}).WithID(), RecordedAtStateRevision: 11, CurrentDependencies: &dependencies}
	sources.Facts.EvidenceApplicabilityObservations = []RuntimeEvidenceApplicabilityObservation{observation}
	identity := RuntimeEquivalentAuthorityInputIdentity{DefinitionRevision: sources.Definition.Revision, FocusHash: StableRuntimeAuthorityFocusHash(ordinary), EvidenceRevision: cutoff, CapabilityStateHash: "original-capabilities",
		CatalogRevisionHash: StableRuntimeOperationAdmissibilityCatalogRevisionHash(nil), ApplicabilityObservationHash: StableRuntimeEvidenceApplicabilitySemanticHash(sources.Facts.EvidenceApplicabilityObservations)}
	assessment := RuntimeDispatchRecord{ID: "original-assessment", WorkspaceID: sources.Facts.WorkspaceID, RunID: sources.Facts.RunID, State: RuntimeDispatchStateCommitted,
		Transaction: AuthorityTransaction{State: ConvergenceStateNeedAssessment, FocusID: ordinary.ID, DefinitionRevision: sources.Definition.Revision, EvidenceRevision: cutoff, AuthorityInputID: StableRuntimeEquivalentAuthorityInputID(identity)},
		Candidate:   RuntimeCandidate{Assessment: &AssessmentCandidate{FocusID: ordinary.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{current.ID}}}, CoverageBindings: ordinary.CoverageEvidenceBindings,
		AssessmentAuthorityInput: &RuntimeAssessmentAuthorityInput{AuthorityInput: identity, Purpose: RuntimeAssessmentPurposeDefinitionProof, SourceSliceID: current.SliceID, SourceScope: current.SourceScope, SourceID: current.SourceID, SourceText: current.TargetText,
			Focus: ordinary, ApplicabilityObservations: CloneRuntimeEvidenceApplicabilityObservations(sources.Facts.EvidenceApplicabilityObservations)}}
	adjudication, err := DeriveRuntimeAssessmentAdjudication(assessment, sources.Facts, sources.Completed, nil, sources)
	if err != nil || adjudication.Disposition != AssessmentDispositionSatisfied {
		t.Fatalf("original ordinary adjudication: %v %+v", err, adjudication)
	}
	assessment.AssessmentAdjudication, assessment.AssessmentAdjudicationHash = &adjudication, StableRuntimeAssessmentAdjudicationHash(adjudication)
	sources.Completed = append(sources.Completed, assessment)
	sources.Facts.Assessments = []RuntimeAssessmentFact{{ID: assessment.ID, FocusID: ordinary.ID, SliceID: current.SliceID, Scope: current.SourceScope, RequirementID: current.SourceID, RequirementText: current.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{current.ID}, Disposition: adjudication.Disposition, AdjudicationHash: assessment.AssessmentAdjudicationHash}}
	return sources
}

func TestHistoricalAcceptanceProofReplaysIndependentAllOfWithoutChangingCredit(t *testing.T) {
	sources := historicalAcceptanceProofFixture(t)
	before, _ := json.Marshal(sources)
	proof, err := CompileRuntimeHistoricalAcceptanceProof(sources, "history", "original-assessment")
	if err != nil {
		t.Fatal(err)
	}
	if proof.AcceptanceBindingRef.BindingID == proof.HistoricalBindingRef.BindingID || len(proof.BehaviorEvidenceRefs) != 1 || len(proof.AcceptanceEvidenceRefs) != 1 || proof.AcceptanceEvidenceRefs[0] == proof.BehaviorEvidenceRefs[0] {
		t.Fatal("historical effect replaced independent ordinary acceptance")
	}
	after, _ := json.Marshal(sources)
	if !reflect.DeepEqual(before, after) || sources.Facts.Coverage[0].State != RuntimeCoverageItemStateOpen {
		t.Fatal("pure proof consumer changed sources or granted terminal credit")
	}
	var reload RuntimeLineageAssertionSources
	if err := json.Unmarshal(before, &reload); err != nil {
		t.Fatal(err)
	}
	repeated, err := CompileRuntimeHistoricalAcceptanceProof(reload, "history", "original-assessment")
	if err != nil || !reflect.DeepEqual(proof, repeated) {
		t.Fatalf("proof changed across reload: %v", err)
	}
	copy := proof.Clone()
	copy.AcceptanceEvidenceRefs[0], copy.BehaviorEvidenceRefs[0] = "changed", "changed"
	if reflect.DeepEqual(proof, copy) || proof.AcceptanceEvidenceRefs[0] == "changed" || proof.BehaviorEvidenceRefs[0] == "changed" {
		t.Fatal("proof clone aliases independent originals")
	}
}

func TestHistoricalAcceptanceProofRejectsClaimsStaleWeakAndFutureSources(t *testing.T) {
	for name, mutate := range map[string]func(*RuntimeLineageAssertionSources){
		"effect-only": func(s *RuntimeLineageAssertionSources) { s.Facts.Evidence.Accepted = s.Facts.Evidence.Accepted[:1] },
		"claimed credit": func(s *RuntimeLineageAssertionSources) {
			s.Facts.Coverage[1].State = RuntimeCoverageItemStateSatisfied
			s.Facts.Assessments = nil
		},
		"partial checks": func(s *RuntimeLineageAssertionSources) {
			s.Receipts[1].Evidence.Validation.PassedChecks = s.Receipts[1].Evidence.Validation.PassedChecks[:1]
		},
		"unprocessed ordinary":     func(s *RuntimeLineageAssertionSources) { s.ProcessedEventIDs = s.ProcessedEventIDs[:1] },
		"missing ordinary receipt": func(s *RuntimeLineageAssertionSources) { s.Receipts = s.Receipts[:1] },
		"future behavior": func(s *RuntimeLineageAssertionSources) {
			s.Facts.Evidence.Accepted[0].Revision = runtimeEvidenceRevisionForSequence(3)
		},
		"missing original observation": func(s *RuntimeLineageAssertionSources) {
			s.Completed[2].AssessmentAuthorityInput.ApplicabilityObservations = nil
		},
		"unconfirmed": func(s *RuntimeLineageAssertionSources) { s.Definition.SourceObligations = nil },
		"wrong consumer": func(s *RuntimeLineageAssertionSources) {
			s.Completed[2].Candidate.Assessment.CoverageWorkItemIDs = []common.CoverageWorkItemID{"history"}
		},
		"acquisition": func(s *RuntimeLineageAssertionSources) {
			s.Completed[2].AssessmentAuthorityInput.Purpose = RuntimeAssessmentPurposeOwnerFactAcquisition
		},
		"repair": func(s *RuntimeLineageAssertionSources) {
			s.Completed[2].RepairCommitment = &RuntimeDispatchRepairCommitment{}
		},
		"work": func(s *RuntimeLineageAssertionSources) {
			s.Completed[2].ImplementationWorkCommitment = &RuntimeImplementationWorkRound{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			sources := historicalAcceptanceProofFixture(t)
			mutate(&sources)
			if _, err := CompileRuntimeHistoricalAcceptanceProof(sources, "history", "original-assessment"); err == nil {
				t.Fatal("invalid source or producer claim supplied deferred acceptance")
			}
		})
	}
}

func TestHistoricalAcceptanceSettlementRequiresExactConsumerAndRetainsRevokedHistory(t *testing.T) {
	sources := historicalAcceptanceProofFixture(t)
	proof, err := CompileRuntimeHistoricalAcceptanceProof(sources, "history", "original-assessment")
	if err != nil {
		t.Fatal(err)
	}
	for index := range sources.Facts.Coverage {
		row := &sources.Facts.Coverage[index]
		row.State, row.CWIConvergenceState, row.LastAssessmentID = RuntimeCoverageItemStateSatisfied, RuntimeCWIConvergenceStateSettled, proof.AssessmentDispatchID
		if row.ID == "history" {
			row.HistoricalAcceptanceProofs = []RuntimeHistoricalAcceptanceProof{proof.Clone()}
			row.EvidenceRefs = normalizeStrings(append(append([]string(nil), proof.BehaviorEvidenceRefs...), proof.AcceptanceEvidenceRefs...))
		}
	}
	if err := ValidateRuntimeHistoricalBehaviorCompletions(sources); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(sources)
	var reload RuntimeLineageAssertionSources
	if err := json.Unmarshal(body, &reload); err != nil || ValidateRuntimeHistoricalBehaviorCompletions(reload) != nil {
		t.Fatalf("original settlement did not reload: %v", err)
	}
	cloned := (RuntimeStateRecord{Facts: sources.Facts}).Clone()
	cloned.Facts.Coverage[0].HistoricalAcceptanceProofs[0].AcceptanceEvidenceRefs[0] = "changed"
	if sources.Facts.Coverage[0].HistoricalAcceptanceProofs[0].AcceptanceEvidenceRefs[0] == "changed" {
		t.Fatal("raw clone aliases acceptance proof history")
	}
	// Losing ordinary current credit must not retain historical terminal credit.
	reload.Facts.Coverage[1].State, reload.Facts.Coverage[1].CWIConvergenceState, reload.Facts.Coverage[1].CurrentBindingRef = RuntimeCoverageItemStateOpen, RuntimeCWIConvergenceStateReopened, nil
	if ValidateRuntimeHistoricalBehaviorCompletions(reload) == nil {
		t.Fatal("stale ordinary consumer retained historical terminal credit")
	}
	history := &reload.Facts.Coverage[0]
	history.State, history.CWIConvergenceState, history.LastAssessmentID, history.EvidenceRefs = RuntimeCoverageItemStateOpen, RuntimeCWIConvergenceStateBound, "", nil
	if err := ValidateRuntimeHistoricalBehaviorCompletions(reload); err != nil {
		t.Fatalf("revocation destroyed immutable original proof history: %v", err)
	}
	if !reflect.DeepEqual(history.HistoricalAcceptanceProofs, sources.Facts.Coverage[0].HistoricalAcceptanceProofs) || !reflect.DeepEqual(history.HistoricalBehavior, sources.Facts.Coverage[0].HistoricalBehavior) {
		t.Fatal("revocation rewrote original behavior/assessment")
	}
	for name, mutate := range map[string]func(*RuntimeLineageAssertionSources){
		"deleted source": func(s *RuntimeLineageAssertionSources) { s.Facts.Coverage[0].HistoricalAcceptanceProofs = nil },
		"duplicate": func(s *RuntimeLineageAssertionSources) {
			s.Facts.Coverage[0].HistoricalAcceptanceProofs = append(s.Facts.Coverage[0].HistoricalAcceptanceProofs, proof)
		},
		"borrowed reference": func(s *RuntimeLineageAssertionSources) {
			s.Facts.Coverage[0].HistoricalAcceptanceProofs[0].AcceptanceEvidenceRefs = proof.BehaviorEvidenceRefs
		},
		"another binding": func(s *RuntimeLineageAssertionSources) {
			s.Facts.Coverage[1].CurrentBindingRef.BindingID = "replacement"
		},
		"another assessment": func(s *RuntimeLineageAssertionSources) { s.Facts.Coverage[1].LastAssessmentID = "replacement" },
	} {
		t.Run(name, func(t *testing.T) {
			var changed RuntimeLineageAssertionSources
			if err := json.Unmarshal(body, &changed); err != nil {
				t.Fatal(err)
			}
			mutate(&changed)
			if ValidateRuntimeHistoricalBehaviorCompletions(changed) == nil {
				t.Fatal("coordinated stale/partial settlement restored terminal credit")
			}
		})
	}
}

func TestHistoricalAcceptanceRejectsLocallyConsistentObservationWithoutOriginalSource(t *testing.T) {
	for _, name := range []string{"missing SM-2 prefix", "wrong original receipt", "unretained generation"} {
		t.Run(name, func(t *testing.T) {
			sources := historicalAcceptanceProofFixture(t)
			dispatch := &sources.Completed[2]
			authority := dispatch.AssessmentAuthorityInput
			switch name {
			case "missing SM-2 prefix":
				sources.Facts.EvidenceApplicabilityObservations = nil
			case "wrong original receipt":
				observation := &authority.ApplicabilityObservations[0]
				observation.Request.SourceReceiptHash = "another-owner-receipt"
				observation.Request = observation.Request.WithID()
				sources.Facts.EvidenceApplicabilityObservations = CloneRuntimeEvidenceApplicabilityObservations(authority.ApplicabilityObservations)
			case "unretained generation":
				authority.ApplicabilityReconciliationRequestHash = "unretained-generation"
				authority.ApplicabilityObservations[0].Request.ReconciliationRequestHash = "unretained-generation"
				authority.ApplicabilityObservations[0].Request = authority.ApplicabilityObservations[0].Request.WithID()
				sources.Facts.EvidenceApplicabilityObservations = CloneRuntimeEvidenceApplicabilityObservations(authority.ApplicabilityObservations)
			}
			authority.AuthorityInput.ApplicabilityObservationHash = StableRuntimeEvidenceApplicabilitySemanticHash(authority.ApplicabilityObservations)
			dispatch.Transaction.AuthorityInputID = StableRuntimeEquivalentAuthorityInputID(authority.AuthorityInput)
			// Recompute both adjudication ledgers: local hashes are consistent,
			// but cannot manufacture the original receipt/prefix/generation.
			adjudication, err := DeriveRuntimeAssessmentAdjudication(*dispatch, sources.Facts, sources.Completed[:2], nil, sources)
			if err != nil || adjudication.Disposition != AssessmentDispositionSatisfied {
				t.Fatalf("counterexample did not retain local satisfied shape: %v", err)
			}
			dispatch.AssessmentAdjudication = &adjudication
			dispatch.AssessmentAdjudicationHash = StableRuntimeAssessmentAdjudicationHash(adjudication)
			sources.Facts.Assessments[0].AdjudicationHash = dispatch.AssessmentAdjudicationHash
			if _, err := CompileRuntimeHistoricalAcceptanceProof(sources, "history", dispatch.ID); err == nil {
				t.Fatal("coordinated hashes substituted for original immutable source")
			}
		})
	}
}
