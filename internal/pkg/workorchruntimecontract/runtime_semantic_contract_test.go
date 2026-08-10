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

func TestRuntimeDefinitionSeedHashKeepsReopenEncodingAndAddsPrerequisiteIdentity(t *testing.T) {
	item := RuntimeCoverageItem{
		ID: "cwi-001", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-001",
		ReopenDependencies: []RuntimeCoverageReopenDependency{{CoverageWorkItemID: "cwi-002", TriggerEvidenceKinds: []string{EvidenceKindStateChange}}},
	}
	base := RuntimeDefinitionSeed{
		SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: "definition-1", PolicyHash: "policy-1",
		CoverageItems: []RuntimeCoverageItem{item, {ID: "cwi-002", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-002"}},
		Slices:        []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001", "cwi-002"}}},
	}
	base.Hash = StableRuntimeDefinitionSeedHash(base)
	withoutNewRelation := base.Clone()
	withoutNewRelation.CoverageItems[0].PrerequisiteCoverageWorkItemIDs = nil
	withoutNewRelation.Hash = StableRuntimeDefinitionSeedHash(withoutNewRelation)
	if withoutNewRelation.Hash != base.Hash {
		t.Fatalf("empty prerequisite relation changed legacy reopen hash: %q != %q", withoutNewRelation.Hash, base.Hash)
	}
	withPrerequisite := base.Clone()
	withPrerequisite.CoverageItems[0].PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-002"}
	withPrerequisite.Hash = StableRuntimeDefinitionSeedHash(withPrerequisite)
	if withPrerequisite.Hash == base.Hash {
		t.Fatal("prerequisite relation was omitted from definition seed hash")
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
		PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "batch-1", BatchSequence: 0,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		Proposals:                []RuntimeSemanticProofProposal{semanticContractTestProposal("cwi-001")},
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

func TestRuntimeSemanticDocumentRejectsValidationOnOwnerWithoutRecipe(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	gitCatalog := semanticTestOwnerCatalog("semantic-git-catalog", RuntimeModuleGit, []string{StepKindGitWrite})
	gitCatalog.SelectorRefs = []string{"workspace-repository"}
	gitCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(gitCatalog)
	manifest.AllowedOwnerModuleIDs = append(manifest.AllowedOwnerModuleIDs, RuntimeModuleGit)
	manifest.AllowedOperationKinds = append(manifest.AllowedOperationKinds, StepKindGitWrite)
	manifest.AllowedTargetPolicies = append(manifest.AllowedTargetPolicies, RuntimeOperationTargetPolicyExistingOwnerScope)
	manifest.OwnerCatalogs = append(manifest.OwnerCatalogs, gitCatalog)
	manifest.CoverageItems = append(manifest.CoverageItems, RuntimeCoverageItem{ID: "cwi-002", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-002", TargetText: "commit", Order: 2})
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	proposal := RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-002", OwnerModuleID: RuntimeModuleGit, OperationKind: StepKindGitWrite,
		Target:                RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"workspace-repository"}},
		RequiredEvidenceKinds: []string{EvidenceKindValidation}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession},
		ValidationRecipeID: "software-dev-validation-evidence", RequiredCheckRefs: []string{"check"}, DeclarationRef: "dod-002",
	}
	document := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "batch-1", BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-002"}, Proposals: []RuntimeSemanticProofProposal{proposal}}
	if err := document.ValidateAgainstManifest(manifest); err == nil || !strings.Contains(err.Error(), "operationKind") {
		t.Fatalf("validation proposal for recipe-less owner error = %v", err)
	}
}

func TestRuntimeSemanticProposalRejectsDurableValidationEffect(t *testing.T) {
	proposal := semanticContractTestProposal("cwi-001")
	proposal.RequiredEvidenceKinds = []string{EvidenceKindValidation}
	proposal.RequiredEffectScopes = []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}
	proposal.ValidationRecipeID = "recipe/validation"
	proposal.RequiredCheckRefs = []string{"check"}
	if err := proposal.Validate(); err == nil || !strings.Contains(err.Error(), "requiredEffectScopes") {
		t.Fatalf("durable validation effect error = %v", err)
	}
}

func TestRuntimeSemanticProposalRejectsShellCommandAsValidationProof(t *testing.T) {
	proposal := semanticContractTestProposal("cwi-001")
	proposal.OwnerModuleID = RuntimeModuleShell
	proposal.OperationKind = StepKindShellCommand
	proposal.RequiredEvidenceKinds = []string{EvidenceKindValidation}
	proposal.RequiredEffectScopes = []RuntimeEffectScope{RuntimeEffectEphemeralSession}
	proposal.ValidationRecipeID = "recipe/validation"
	proposal.RequiredCheckRefs = []string{"check"}
	if err := proposal.Validate(); err == nil || !strings.Contains(err.Error(), "operationKind") {
		t.Fatalf("shell command validation proof error = %v", err)
	}
}

func TestRuntimeSemanticPlanCoverageOrderMustRespectDependencies(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems = []RuntimeCoverageItem{
		{ID: "cwi-001", SliceID: "slice-1", SourceScope: "test", SourceID: "one"},
		{ID: "cwi-002", SliceID: "slice-1", SourceScope: "test", SourceID: "two", PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}},
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

func TestRuntimeSemanticPlanDocumentRejectsImplicitProofBatchStep(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash,
		BatchID:            "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		Proposals: []RuntimeSemanticProofProposal{semanticContractTestProposal("cwi-001")},
	}
	if err := document.ValidateAgainstManifest(manifest); err == nil || !strings.Contains(err.Error(), "invalid step") {
		t.Fatalf("implicit proof-batch plan step error = %v", err)
	}
}

func TestRuntimeSemanticPlanBatchDocumentMayCoverOnlyDeclaredBatch(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems = append(manifest.CoverageItems, RuntimeCoverageItem{ID: "cwi-002", SliceID: "slice-1", SourceScope: "test", SourceID: "two"})
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID:  "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
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
		OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: manifest.OwnerCatalogs[0].SnapshotID, Status: RuntimeSemanticFeasibilityInfeasible, ObservedRevision: "workspace-1",
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

func TestRuntimeSemanticPlanningFailedValidatesActiveBatchFeasibilityLineage(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	seed := RuntimeDefinitionSeed{
		SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: manifest.DefinitionRevision,
		PolicyHash: "policy-1", CoverageItems: manifest.CoverageItems,
		Slices: []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}}},
	}
	seed.Hash = StableRuntimeDefinitionSeedHash(seed)
	order := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{"cwi-001"},
	}
	proposal := semanticContractTestProposal("cwi-001")
	batch, err := CompileRuntimeSemanticPlanBatch(manifest, order, 0, "request-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	batch, err = AdvanceRuntimeSemanticPlanBatch(batch, RuntimeSemanticPlanBatchRequesting)
	if err != nil {
		t.Fatal(err)
	}
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID: batch.BatchID, BatchSequence: batch.Sequence, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		Proposals: []RuntimeSemanticProofProposal{proposal},
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	batch, err = AdvanceRuntimeSemanticPlanBatch(batch, RuntimeSemanticPlanBatchDocumentComplete)
	if err != nil {
		t.Fatal(err)
	}
	batch.DocumentHash = StableRuntimeSemanticPlanDocumentHash(document)
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	feasibility := RuntimeSemanticProofFeasibility{
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID,
		ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID,
		CatalogSnapshotID: manifest.OwnerCatalogs[0].SnapshotID, Status: RuntimeSemanticFeasibilityInfeasible, ObservedRevision: "workspace-1",
		OriginalDiagnostic: semanticContractTestDiagnostic(),
	}
	if err := feasibility.ValidateForProposal(manifest, proposal); err != nil {
		t.Fatal(err)
	}
	ledger := RuntimeSemanticPlanningLedger{
		SchemaVersion: RuntimeSemanticPlanningLedgerSchemaCurrent, Phase: RuntimeSemanticPlanningFailed,
		DefinitionSeed: seed, Manifest: &manifest, Attempt: 0, StablePrefixHash: "prefix-1", LogicalRequestID: "request-1",
		CoverageOrderDocument: &order, Document: &order, DocumentHash: StableRuntimeSemanticPlanDocumentHash(order), ActiveBatchID: batch.BatchID,
		NextBatchSequence: 1,
		BatchDocuments:    []RuntimeSemanticPlanDocument{document}, Batches: []RuntimeSemanticPlanBatch{batch},
		FeasibilityBatchID: StableRuntimeSemanticFeasibilityBatchID(manifest, document),
		Feasibilities:      []RuntimeSemanticProofFeasibility{feasibility},
		FeasibilitySetHash: StableRuntimeSemanticFeasibilitySetHash([]RuntimeSemanticProofFeasibility{feasibility}),
		Failure:            semanticContractTestDiagnostic(),
	}
	if err := ledger.Validate(); err != nil {
		t.Fatalf("failed active-batch ledger rejected: %v", err)
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

func TestRuntimeSemanticProposalTargetRefsExcludeProspectiveIdentity(t *testing.T) {
	proposal := semanticContractTestProposal("cwi-001")
	proposal.Target = RuntimeOperationTargetAuthorization{
		Policy:                    RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		OwnerSelectorRefs:         []string{"workspace-validation-target"},
		ProspectiveTargetRef:      "go-test-memory-usage",
		ProspectiveConstraintRefs: []string{"bounded-timeout"},
	}
	if refs := RuntimeSemanticProposalTargetRefs(proposal); len(refs) != 0 {
		t.Fatalf("prospective target was treated as existing target refs: %+v", refs)
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
	order := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{"cwi-001"},
	}
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	batch, err := CompileRuntimeSemanticPlanFocusBatch(manifest, order, 0, "request-1", []common.CoverageWorkItemID{"cwi-001"}, []common.CoverageWorkItemID{"cwi-001"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []RuntimeSemanticPlanBatchState{
		RuntimeSemanticPlanBatchRequesting,
		RuntimeSemanticPlanBatchDocumentComplete,
	} {
		batch, err = AdvanceRuntimeSemanticPlanBatch(batch, next)
		if err != nil {
			t.Fatal(err)
		}
	}
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID: batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		Proposals:                []RuntimeSemanticProofProposal{proposal},
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	batch.DocumentHash = StableRuntimeSemanticPlanDocumentHash(document)
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	for _, next := range []RuntimeSemanticPlanBatchState{
		RuntimeSemanticPlanBatchFeasibilityPending,
		RuntimeSemanticPlanBatchFeasibilityComplete,
		RuntimeSemanticPlanBatchAccepted,
	} {
		batch, err = AdvanceRuntimeSemanticPlanBatch(batch, next)
		if err != nil {
			t.Fatal(err)
		}
	}
	feasibility := RuntimeSemanticProofFeasibility{
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID,
		ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: manifest.OwnerCatalogs[0].SnapshotID,
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
	orderHash := StableRuntimeSemanticPlanCoverageOrderHash(order)
	merged := document.Clone()
	ledger := RuntimeSemanticPlanningLedger{
		SchemaVersion: RuntimeSemanticPlanningLedgerSchemaCurrent, Phase: RuntimeSemanticPlanningSealed,
		DefinitionSeed: seed, Manifest: &manifest, Attempt: 0, StablePrefixHash: "prefix-1", LogicalRequestID: "request-1",
		CoverageOrderDocument: &order, Document: &document, DocumentHash: documentHash, FeasibilityBatchID: batchID,
		Batches: []RuntimeSemanticPlanBatch{batch}, BatchDocuments: []RuntimeSemanticPlanDocument{document},
		CompletedBatchIDs: []string{batch.BatchID}, NextBatchSequence: 1, MergedDocument: &merged, MergedDocumentHash: documentHash,
		Feasibilities: []RuntimeSemanticProofFeasibility{feasibility}, FeasibilitySetHash: feasibilityHash,
		SealedBindingSetHash: "binding-set-1", SealedLineageRefs: []string{manifest.Hash, orderHash, documentHash, batchID, feasibilityHash, "binding-set-1"},
	}
	if err := ledger.Validate(); err != nil {
		t.Fatal(err)
	}
	nonFeasible := ledger.Clone()
	nonFeasible.Feasibilities[0].Status = RuntimeSemanticFeasibilityInfeasible
	nonFeasible.Feasibilities[0].OriginalDiagnostic = semanticContractTestDiagnostic()
	nonFeasible.FeasibilitySetHash = StableRuntimeSemanticFeasibilitySetHash(nonFeasible.Feasibilities)
	if err := nonFeasible.Validate(); err == nil {
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
