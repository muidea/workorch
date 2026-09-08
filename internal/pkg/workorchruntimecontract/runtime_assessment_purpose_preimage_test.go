package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestAssessmentPurposePreimageIsIsolatedFromOtherSemanticPurposes(t *testing.T) {
	definition, definitionBindings := assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	repair, repairBindings := assessmentPurposeGenerationForTest(t, "cwi-repair", "repair-request", true)
	for _, repairActive := range []bool{true, false} {
		name := "historical_repair"
		active, historical := definition.Clone(), repair.Clone()
		if repairActive {
			name, active, historical = "current_repair", repair.Clone(), definition.Clone()
		}
		t.Run(name, func(t *testing.T) {
			active.History = []RuntimeSemanticPlanningGeneration{{
				RetiredAtStateRevision: 2, Reason: RuntimeSemanticPlanningRetirementResourceInvalidated,
				InvalidationSignatures: []string{"resource-change"}, PrerequisiteIDs: []string{historical.Manifest.OwnerCatalogs[0].PrerequisiteID}, Snapshot: historical,
			}}
			if err := active.Validate(); err != nil {
				t.Fatalf("mixed semantic ledger fixture: %v", err)
			}
			before, err := json.Marshal(active)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded RuntimeSemanticPlanningLedger
			if err := json.Unmarshal(before, &reloaded); err != nil {
				t.Fatal(err)
			}
			for _, ledger := range []RuntimeSemanticPlanningLedger{active, reloaded} {
				preimage, err := ResolveRuntimeAssessmentPurposePreimage(ledger, definitionBindings)
				if err != nil || preimage.Purpose != RuntimeAssessmentPurposeDefinitionProof || preimage.SemanticManifestHash != definition.Manifest.Hash {
					t.Fatalf("unrelated repair changed exact definition assessment: %+v err=%v", preimage, err)
				}
				if _, err := ResolveRuntimeAssessmentPurposePreimage(ledger, repairBindings); err == nil || !strings.Contains(err.Error(), "validation_failure_repair") {
					t.Fatalf("repair-only proof became an assessment: %v", err)
				}
				// Both candidate construction above and accepted-attempt replay below
				// must use the same exact-binding purpose derivation.
				manifest := RuntimeCandidateContractManifest{Bindings: definitionBindings, AssessmentPurpose: preimage.Purpose, AssessmentPurposeManifestHash: preimage.SemanticManifestHash}
				dispatch := RuntimeDispatchRecord{ID: "assessment", Candidate: RuntimeCandidate{Assessment: &AssessmentCandidate{}}, AssessmentAuthorityInput: &RuntimeAssessmentAuthorityInput{Purpose: preimage.Purpose}}
				attempts := []RuntimeCandidateAttemptRecord{{State: RuntimeCandidateAttemptStateAccepted, DispatchID: dispatch.ID, LogicalRequestID: "assessment-request"}}
				sessions := []RuntimeCandidateRequestSession{{LogicalRequestID: "assessment-request", CandidateManifest: &manifest}}
				if err := ValidateRuntimeAcceptedAssessmentPurposePreimages(ledger, &dispatch, nil, attempts, sessions); err != nil {
					t.Fatalf("historical assessment replay rejected unrelated repair: %v", err)
				}
				manifest.AssessmentPurposeManifestHash = repair.Manifest.Hash
				if err := ValidateRuntimeAcceptedAssessmentPurposePreimages(ledger, &dispatch, nil, attempts, sessions); err == nil {
					t.Fatal("assessment accepted another generation's manifest identity")
				}
			}
			after, err := json.Marshal(active)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("purpose lookup mutated immutable generations: %v", err)
			}
		})
	}
}

func TestAssessmentPurposePreimageRejectsMissingDuplicateAndDriftedProofs(t *testing.T) {
	base, bindings := assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	for _, scenario := range []string{"missing", "duplicate", "binding_drift", "document_drift", "owner_drift", "manifest_drift"} {
		t.Run(scenario, func(t *testing.T) {
			ledger, requested := base.Clone(), CloneCoverageEvidenceBindings(bindings)
			switch scenario {
			case "missing":
				ledger.BatchDocuments = nil
			case "duplicate":
				ledger.History = []RuntimeSemanticPlanningGeneration{{
					RetiredAtStateRevision: 2, Reason: RuntimeSemanticPlanningRetirementResourceInvalidated,
					InvalidationSignatures: []string{"resource-change"}, PrerequisiteIDs: []string{base.Manifest.OwnerCatalogs[0].PrerequisiteID}, Snapshot: base.Clone(),
				}}
			case "binding_drift":
				requested[0].CoverageWorkItemID = "cwi-repair"
			case "document_drift":
				ledger.BatchDocuments[0].Proposals[0].Target.ProspectiveTargetRefs = []string{"different.txt"}
			case "owner_drift":
				ledger.Feasibilities[0].OwnerModuleID = RuntimeModuleShell
			case "manifest_drift":
				ledger.Manifest.ProofObligations[0].DeclarationRef = "other-declaration"
			}
			if _, err := ResolveRuntimeAssessmentPurposePreimage(ledger, requested); err == nil {
				t.Fatal("invalid semantic preimage accepted")
			}
		})
	}
}

// Builds persisted-shape semantic fixtures with the production pure compiler.
// It is not a transition fixture; the Orchestrator mainline tests exercise the
// owner-private events and opaque CAS that produce these records in production.
func assessmentPurposeGenerationForTest(t *testing.T, coverageID common.CoverageWorkItemID, requestID string, repair bool) (RuntimeSemanticPlanningLedger, []CoverageEvidenceBinding) {
	t.Helper()
	definition := RuntimeDefinitionContext{
		StartupInputHash: "startup-purpose", Revision: "definition-purpose", GoalID: "goal-purpose", GoalDefinition: "deliver behavior",
		DoDID: "dod-purpose", DoDDefinition: "behavior verified", CapabilityContract: []string{StepKindFileWrite, RuntimeBlockKindPatchApply, RuntimeBlockKindManagedValidationSession},
	}
	execution := RuntimeExecutionContract{OperationContractRevision: RuntimeOperationContractRevision(), StateChangePolicy: RuntimeStateChangeAllowedButNotRequired}
	items := []RuntimeCoverageItem{
		{ID: "cwi-source", SliceID: "slice-source", SourceScope: "deliverable", SourceID: "source", TargetText: "deliver behavior", Order: 1},
		{ID: "cwi-repair", SliceID: "slice-repair", SourceScope: "dodItem", SourceID: "behavior", TargetText: "behavior verified", Order: 2},
	}
	slices := []RuntimeAcceptanceSlice{{ID: "slice-source", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}}, {ID: "slice-repair", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-repair"}}}
	seed, err := CompileRuntimeDefinitionSeed(definition, RuntimeActivationContext{ExecutionContract: execution}, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	focuses := semanticDefinitionFocuses(items)
	convergence := testValidationFailureRepairConvergence()
	for index := range focuses {
		if focuses[index].CoverageWorkItemIDs[0] == "cwi-repair" {
			focuses[index].RequiredEvidenceKinds = []string{EvidenceKindValidation}
			focuses[index].ValidationFailureRepairConvergence = &convergence
		}
	}
	focuses = NormalizeRuntimeDefinitionFocuses(focuses)
	catalog := semanticValidationRepairFileCatalog()
	shellCatalog := semanticTestOwnerCatalog("purpose-shell-catalog", RuntimeModuleShell, []string{RuntimeBlockKindManagedValidationSession})
	shellCatalog.ValidationRecipes = []RuntimeValidationRecipe{{ID: "purpose-validation", Command: "go", Args: []string{"test", "./..."}}}
	shellCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(shellCatalog)
	catalogs := []RuntimeOperationAdmissibility{catalog, shellCatalog}
	var manifest RuntimeSemanticPlanManifest
	if repair {
		manifest, err = CompileRuntimeSemanticPlanManifestForCoverageWithValidationFailureRepair("ws-purpose", "run-purpose", definition, seed, focuses, execution, catalogs, []common.CoverageWorkItemID{coverageID}, RuntimeValidationFailureRepairGenerationInput{
			CoverageWorkItemID: coverageID, FailedFocusID: "definition-focus-" + string(coverageID), FailureSignature: "validation-failure",
			TriggerOwnerModuleID: RuntimeModuleShell, TriggerOperationKind: RuntimeBlockKindManagedValidationSession,
			TriggerDiagnosticClass: ValidationDiagnosticBuildFailed, TriggerEvidenceTargetType: ValidationEvidenceTargetFailureSurface, TriggerOwnerDiagnosticCode: "shell_exit_nonzero",
		})
	} else {
		manifest, err = CompileRuntimeSemanticPlanManifestForCoverage("ws-purpose", "run-purpose", definition, seed, focuses, execution, catalogs, []common.CoverageWorkItemID{coverageID})
	}
	if err != nil {
		t.Fatal(err)
	}
	proposal := RuntimeSemanticProofProposal{
		CoverageWorkItemID: coverageID, OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite,
		Target:                RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, OwnerSelectorRefs: []string{"workspace-relative-new-file-target"}, ProspectiveConstraintRefs: []string{"new-target-requires-owner-selector", "workspace-relative-path"}, ProspectiveTargetRefs: []string{string(coverageID) + ".txt"}},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, DeclarationRef: RuntimeSemanticCurrentDeclarationRef(manifest.ProofObligations[0]),
	}
	order := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: seed.Hash, PlanStep: RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{coverageID}}
	batch := semanticContractTestPendingBatch(t, manifest, order, 0, requestID, order.CoverageOrder, order.CoverageOrder)
	document := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: seed.Hash, PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: batch.BatchID, BatchSequence: batch.Sequence, BatchCoverageWorkItemIDs: order.CoverageOrder, Proposals: []RuntimeSemanticProofProposal{proposal}}
	batch.State, batch.DocumentHash = RuntimeSemanticPlanBatchAccepted, StableRuntimeSemanticPlanDocumentHash(document)
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	fact := RuntimeSemanticProofFeasibility{SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: seed.Hash, CoverageWorkItemID: coverageID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: RuntimeModuleFile, CatalogSnapshotID: catalog.SnapshotID, Status: RuntimeSemanticFeasibilityFeasible, ObservedRevision: catalog.ObservedRevision, TargetRefs: proposal.Target.ProspectiveTargetRefs}
	compiled, _, err := CompileRuntimeSemanticBindings(manifest, requestID, document, []RuntimeSemanticProofFeasibility{fact})
	if err != nil {
		t.Fatal(err)
	}
	ledger := RuntimeSemanticPlanningLedger{
		SchemaVersion: RuntimeSemanticPlanningLedgerSchemaCurrent, Phase: RuntimeSemanticPlanningSealed, DefinitionSeed: seed,
		Manifest: &manifest, StablePrefixHash: "stable-purpose-prefix", LogicalRequestID: requestID,
		CoverageOrderDocument: &order, Document: &document, DocumentHash: batch.DocumentHash,
		Batches: []RuntimeSemanticPlanBatch{batch}, NextBatchSequence: 1, CompletedBatchIDs: []string{batch.BatchID}, BatchDocuments: []RuntimeSemanticPlanDocument{document},
		MergedDocument: &document, MergedDocumentHash: batch.DocumentHash, FeasibilityBatchID: StableRuntimeSemanticFeasibilityBatchID(manifest, document),
		Feasibilities: []RuntimeSemanticProofFeasibility{fact}, FeasibilitySetHash: StableRuntimeSemanticFeasibilitySetHash([]RuntimeSemanticProofFeasibility{fact}),
		SealedBindingSetHash: "test-binding-set", SealedLineageRefs: []string{manifest.Hash, StableRuntimeSemanticPlanCoverageOrderHash(order), batch.DocumentHash, StableRuntimeSemanticFeasibilityBatchID(manifest, document), StableRuntimeSemanticFeasibilitySetHash([]RuntimeSemanticProofFeasibility{fact}), "test-binding-set"},
	}
	if err := ledger.Validate(); err != nil {
		t.Fatalf("semantic purpose fixture: %v", err)
	}
	return ledger, compiled[0].CoverageEvidenceBindings
}
