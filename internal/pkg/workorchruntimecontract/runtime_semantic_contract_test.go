package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeDefinitionSeedIsStableAndContainsNoProofAuthority(t *testing.T) {
	definition := RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "deliver",
		DoDID: "dod-1", DoDDefinition: "1. works", CapabilityContract: []string{StepKindFileEdit},
	}
	activation := RuntimeActivationContext{ExecutionContract: RuntimeExecutionContract{OperationContractRevision: RuntimeOperationContractRevision()}}
	items := []RuntimeCoverageItem{{ID: "cwi-001", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "works", Order: 1}}
	slices := []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}}}
	seed, err := CompileRuntimeDefinitionSeed(definition, activation, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileRuntimeSemanticPlanManifest("ws-1", "run-1", definition, seed, activation.ExecutionContract, nil); err == nil || !strings.Contains(err.Error(), "no sealed admissibility catalog") {
		t.Fatalf("semantic manifest accepted missing owner catalogs: %v", err)
	}
	if seed.Hash == "" || seed.Hash != StableRuntimeDefinitionSeedHash(seed) || seed.DefinitionRevision != definition.Revision {
		t.Fatalf("seed = %+v", seed)
	}
	permuted, err := CompileRuntimeDefinitionSeed(definition, activation, append([]RuntimeCoverageItem(nil), items...), append([]RuntimeAcceptanceSlice(nil), slices...))
	if err != nil || permuted.Hash != seed.Hash {
		t.Fatalf("permuted seed = %+v err=%v", permuted, err)
	}
	changed := seed.Clone()
	changed.CoverageItems[0].TargetText = "different"
	changed.Hash = StableRuntimeDefinitionSeedHash(changed)
	if changed.Hash == seed.Hash {
		t.Fatal("definition seed hash ignored CWI semantics")
	}
}

func TestCompileRuntimeSemanticPlanManifestUsesOnlySealedCapabilityVocabulary(t *testing.T) {
	definition := RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "deliver",
		DoDID: "dod-1", DoDDefinition: "works",
		CapabilityContract: []string{StepKindFileWrite, RuntimeBlockKindManagedValidationSession, RuntimeBlockKindDecisionRequest},
	}
	activation := RuntimeActivationContext{ExecutionContract: RuntimeExecutionContract{
		OperationContractRevision: RuntimeOperationContractRevision(),
	}}
	items := []RuntimeCoverageItem{{ID: "cwi-001", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "works", Order: 1}}
	slices := []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}}}
	seed, err := CompileRuntimeDefinitionSeed(definition, activation, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	catalogs := []RuntimeOperationAdmissibility{
		semanticTestOwnerCatalog("file-catalog", RuntimeModuleFile, []string{StepKindFileWrite}),
		semanticTestOwnerCatalog("shell-catalog", RuntimeModuleShell, []string{RuntimeBlockKindManagedValidationSession}),
	}
	manifest, err := CompileRuntimeSemanticPlanManifest("ws-1", "run-1", definition, seed, activation.ExecutionContract, catalogs)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Hash == "" || manifest.DefinitionSeedHash != seed.Hash || len(manifest.AllowedOperationKinds) != 2 ||
		manifest.AllowedOperationKinds[0] != StepKindFileWrite || manifest.AllowedOperationKinds[1] != RuntimeBlockKindManagedValidationSession {
		t.Fatalf("semantic manifest = %+v", manifest)
	}
	if len(manifest.AllowedOwnerModuleIDs) != 2 || manifest.AllowedOwnerModuleIDs[0] != RuntimeModuleFile || manifest.AllowedOwnerModuleIDs[1] != RuntimeModuleShell {
		t.Fatalf("semantic owners = %v", manifest.AllowedOwnerModuleIDs)
	}
	if !containsSemanticTargetPolicy(manifest.AllowedTargetPolicies, RuntimeOperationTargetPolicyProspectiveOwnerTarget) {
		t.Fatalf("file/write did not expose prospective owner target: %v", manifest.AllowedTargetPolicies)
	}

	withoutCreate := definition
	withoutCreate.CapabilityContract = []string{RuntimeBlockKindManagedValidationSession}
	seed, err = CompileRuntimeDefinitionSeed(withoutCreate, activation, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err = CompileRuntimeSemanticPlanManifest("ws-1", "run-1", withoutCreate, seed, activation.ExecutionContract, catalogs[1:])
	if err != nil {
		t.Fatal(err)
	}
	if containsSemanticTargetPolicy(manifest.AllowedTargetPolicies, RuntimeOperationTargetPolicyProspectiveOwnerTarget) {
		t.Fatalf("non-create vocabulary exposed prospective target: %v", manifest.AllowedTargetPolicies)
	}
}

func semanticTestOwnerCatalog(id, owner string, kinds []string) RuntimeOperationAdmissibility {
	catalog := RuntimeOperationAdmissibility{PrerequisiteID: id, OwnerModuleID: owner, OperationKinds: kinds, ObservedRevision: id + "-revision"}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	return catalog
}

func containsSemanticTargetPolicy(values []RuntimeOperationTargetPolicy, wanted RuntimeOperationTargetPolicy) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestRuntimeSemanticDocumentRequiresOneBoundedProposalPerCWI(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		Proposals: []RuntimeSemanticProofProposal{semanticContractTestProposal("cwi-001")},
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	duplicate := document.Clone()
	duplicate.Proposals = append(duplicate.Proposals, duplicate.Proposals[0])
	if err := duplicate.ValidateAgainstManifest(manifest); err == nil || !strings.Contains(err.Error(), "duplicates coverage") {
		t.Fatalf("duplicate proposal error = %v", err)
	}
	missing := document.Clone()
	missing.Proposals = nil
	if err := missing.ValidateAgainstManifest(manifest); err == nil || !strings.Contains(err.Error(), "does not cover") {
		t.Fatalf("missing proposal error = %v", err)
	}
	escaped := document.Clone()
	escaped.Proposals[0].Target.OwnerSelectorRefs = []string{"file/unsealed-selector"}
	if err := escaped.ValidateAgainstManifest(manifest); err == nil || !strings.Contains(err.Error(), "outside owner catalog") {
		t.Fatalf("proposal escaped the sealed owner vocabulary: %v", err)
	}
	emptyInventory := manifest.Clone()
	emptyInventory.OwnerCatalogs[0].TargetRefs = nil
	emptyInventory.OwnerCatalogs[0].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(emptyInventory.OwnerCatalogs[0])
	emptyInventory.Hash = StableRuntimeSemanticPlanManifestHash(emptyInventory)
	emptyDocument := document.Clone()
	emptyDocument.ManifestHash = emptyInventory.Hash
	if err := emptyDocument.ValidateAgainstManifest(emptyInventory); err != nil {
		t.Fatalf("prospective target must be admitted by sealed selector/constraint before owner feasibility: %v", err)
	}
}

func TestRuntimeSemanticPlanCoverageOrderMustRespectDependencies(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems = []RuntimeCoverageItem{
		{ID: "cwi-001", SliceID: "slice-1", SourceScope: "test", SourceID: "one"},
		{ID: "cwi-002", SliceID: "slice-1", SourceScope: "test", SourceID: "two", ReopenDependencies: []RuntimeCoverageReopenDependency{{CoverageWorkItemID: "cwi-001"}}},
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageOrder: []common.CoverageWorkItemID{"cwi-002", "cwi-001"},
		Proposals:     []RuntimeSemanticProofProposal{semanticContractTestProposal("cwi-001"), semanticContractTestProposal("cwi-002")},
	}
	if err := document.ValidateAgainstManifest(manifest); err == nil {
		t.Fatal("coverage order violating dependency was accepted")
	}
}

func TestRuntimeSemanticPlanBatchDocumentMayCoverOnlyDeclaredBatch(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems = append(manifest.CoverageItems, RuntimeCoverageItem{ID: "cwi-002", SliceID: "slice-1", SourceScope: "test", SourceID: "two"})
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		BatchID: "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, CoverageOrder: []common.CoverageWorkItemID{"cwi-001", "cwi-002"},
		Proposals: []RuntimeSemanticProofProposal{semanticContractTestProposal("cwi-001")},
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatalf("partial batch document rejected: %v", err)
	}
}

func TestRuntimeSemanticCoverageOrderStepHasNoProofProposals(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{"cwi-001"},
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatalf("coverage-order step rejected: %v", err)
	}
}

func TestRuntimeSemanticFeasibilityRetainsOwnerDiagnosticWithoutSelectingAction(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	proposal := semanticContractTestProposal("cwi-001")
	fact := RuntimeSemanticProofFeasibility{
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal),
		OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: "catalog-1", Status: RuntimeSemanticFeasibilityInfeasible, ObservedRevision: "workspace-1",
		OriginalDiagnostic: &RuntimeOriginalDiagnostic{
			SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleFile,
			OriginalClass: "feasibility", OriginalCode: "target_outside_scope", OriginalMessage: "outside workspace",
			Signature: "file/target-outside-scope", TargetRefs: []string{"main.go"},
		},
	}
	if err := fact.ValidateForProposal(manifest, proposal); err != nil {
		t.Fatal(err)
	}
	cloned := fact.Clone()
	if cloned.OriginalDiagnostic == nil || cloned.OriginalDiagnostic.OriginalMessage != "outside workspace" {
		t.Fatalf("cloned feasibility = %+v", cloned)
	}
	feasible := fact.Clone()
	feasible.Status = RuntimeSemanticFeasibilityFeasible
	if err := feasible.ValidateForProposal(manifest, proposal); err == nil {
		t.Fatal("feasible observation retained a failure diagnostic")
	}
}

func TestRuntimeSemanticPlanningLedgerRequiredShapeIsStrict(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	seed := RuntimeDefinitionSeed{
		SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: manifest.DefinitionRevision,
		PolicyHash: "policy-1", CoverageItems: manifest.CoverageItems,
		Slices: []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}}},
	}
	seed.Hash = StableRuntimeDefinitionSeedHash(seed)
	ledger, err := NewRuntimeSemanticPlanningLedger(seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := ledger.Clone()
	invalid.Manifest = &manifest
	if err := invalid.Validate(); err == nil {
		t.Fatal("required ledger accepted downstream manifest")
	}
}

func TestRuntimeSemanticManifestAndProposalHashesCoverAuthorityInputs(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	changedManifest := manifest.Clone()
	changedManifest.AllowedOwnerModuleIDs = append(changedManifest.AllowedOwnerModuleIDs, RuntimeModuleShell)
	changedManifest.Hash = StableRuntimeSemanticPlanManifestHash(changedManifest)
	if changedManifest.Hash == manifest.Hash {
		t.Fatal("semantic manifest hash ignored owner catalog")
	}
	changedCatalog := manifest.Clone()
	changedCatalog.OwnerCatalogs[0].ObservedRevision = "file-revision-2"
	changedCatalog.OwnerCatalogs[0].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(changedCatalog.OwnerCatalogs[0])
	changedCatalog.Hash = StableRuntimeSemanticPlanManifestHash(changedCatalog)
	if changedCatalog.Hash == manifest.Hash {
		t.Fatal("semantic manifest hash ignored owner catalog revision")
	}

	proposal := semanticContractTestProposal("cwi-001")
	changedProposal := proposal.Clone()
	changedProposal.Target.ProspectiveConstraintRefs = append(changedProposal.Target.ProspectiveConstraintRefs, "regular-file")
	if StableRuntimeSemanticProofProposalHash(changedProposal) == StableRuntimeSemanticProofProposalHash(proposal) {
		t.Fatal("semantic proposal hash ignored target constraints")
	}
}

func TestRuntimeSemanticProposalRejectsOwnerKindMismatch(t *testing.T) {
	proposal := semanticContractTestProposal("cwi-001")
	proposal.OwnerModuleID = RuntimeModuleShell
	if err := proposal.Validate(); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("owner-kind mismatch error = %v", err)
	}
}

func TestRuntimeSemanticCorrectionHashRetainsOriginalDiagnosticIdentity(t *testing.T) {
	diagnostic := *semanticContractTestDiagnostic()
	base := StableRuntimeSemanticPlanCorrectionHash(diagnostic)
	if base == "" {
		t.Fatal("semantic correction hash is empty")
	}
	changed := diagnostic
	changed.OriginalMessage = "different original diagnostic"
	if StableRuntimeSemanticPlanCorrectionHash(changed) == base {
		t.Fatal("semantic correction hash ignored original diagnostic mutation")
	}
}

func TestRuntimeSemanticPlanningSealedRequiresCompleteFeasibleOwnerBatch(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	proposal := semanticContractTestProposal("cwi-001")
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, Proposals: []RuntimeSemanticProofProposal{proposal},
	}
	feasibility := RuntimeSemanticProofFeasibility{
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID,
		ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: "catalog-1",
		Status: RuntimeSemanticFeasibilityFeasible, ObservedRevision: "workspace-1", TargetRefs: []string{"main.go"},
	}
	seed := RuntimeDefinitionSeed{
		SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: manifest.DefinitionRevision,
		PolicyHash: "policy-1", CoverageItems: manifest.CoverageItems,
		Slices: []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}}},
	}
	seed.Hash = StableRuntimeDefinitionSeedHash(seed)
	documentHash := StableRuntimeSemanticPlanDocumentHash(document)
	batchID := StableRuntimeSemanticFeasibilityBatchID(manifest, document)
	feasibilityHash := StableRuntimeSemanticFeasibilitySetHash([]RuntimeSemanticProofFeasibility{feasibility})
	ledger := RuntimeSemanticPlanningLedger{
		SchemaVersion: RuntimeSemanticPlanningLedgerSchemaCurrent, Phase: RuntimeSemanticPlanningSealed,
		DefinitionSeed: seed, Manifest: &manifest, Attempt: 0, StablePrefixHash: "prefix-1", LogicalRequestID: "request-1",
		Document: &document, DocumentHash: documentHash, FeasibilityBatchID: batchID,
		Feasibilities: []RuntimeSemanticProofFeasibility{feasibility}, FeasibilitySetHash: feasibilityHash,
		SealedBindingSetHash: "binding-set-1", SealedLineageRefs: []string{"request-1", documentHash, batchID, feasibilityHash},
	}
	if err := ledger.Validate(); err != nil {
		t.Fatal(err)
	}
	nonFeasible := ledger.Clone()
	nonFeasible.Feasibilities[0].Status = RuntimeSemanticFeasibilityInfeasible
	nonFeasible.Feasibilities[0].OriginalDiagnostic = semanticContractTestDiagnostic()
	nonFeasible.FeasibilitySetHash = StableRuntimeSemanticFeasibilitySetHash(nonFeasible.Feasibilities)
	if err := nonFeasible.Validate(); err == nil || !strings.Contains(err.Error(), "non-feasible") {
		t.Fatalf("non-feasible sealed ledger error = %v", err)
	}
}

func semanticContractTestDiagnostic() *RuntimeOriginalDiagnostic {
	return &RuntimeOriginalDiagnostic{
		SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleFile,
		OriginalClass: "feasibility", OriginalCode: "target_outside_scope", OriginalMessage: "outside workspace",
		Signature: "file/target-outside-scope",
	}
}

func semanticContractTestManifest(t *testing.T) RuntimeSemanticPlanManifest {
	t.Helper()
	catalog := semanticTestOwnerCatalog("semantic-file-catalog", RuntimeModuleFile, []string{StepKindFileWrite})
	catalog.TargetRefs = []string{"main.go"}
	catalog.SelectorRefs = []string{"file/workspace-relative-create"}
	catalog.ConstraintRefs = []string{"workspace-relative-path"}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := RuntimeSemanticPlanManifest{
		SchemaVersion: RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", DefinitionSeedHash: "seed-1",
		OperationContractRevision: RuntimeOperationContractRevision(),
		CoverageItems:             []RuntimeCoverageItem{{ID: "cwi-001", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "works", Order: 1}},
		AllowedOwnerModuleIDs:     []string{RuntimeModuleFile}, AllowedOperationKinds: []string{StepKindFileWrite},
		AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		OwnerCatalogs:         []RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func semanticContractTestProposal(id common.CoverageWorkItemID) RuntimeSemanticProofProposal {
	return RuntimeSemanticProofProposal{
		CoverageWorkItemID: id, OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite,
		Target: RuntimeOperationTargetAuthorization{
			Policy:                    RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			OwnerSelectorRefs:         []string{"file/workspace-relative-create"},
			ProspectiveConstraintRefs: []string{"workspace-relative-path"}, ProspectiveTargetRef: "main.go",
		},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		DeclarationRef: "semantic-plan/dod-001",
	}
}
