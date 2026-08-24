package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeDefinitionSeedIsStableAndContainsNoProofAuthority(t *testing.T) {
	definition := RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "deliver",
		DoDID: "dod-1", DoDDefinition: "1. works", CapabilityContract: []string{StepKindFileEdit},
	}
	activation := RuntimeActivationContext{ExecutionContract: RuntimeExecutionContract{
		OperationContractRevision: RuntimeOperationContractRevision(),
		StateChangePolicy:         RuntimeStateChangeAllowedButNotRequired,
	}}
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
	definition := RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "deliver",
		DoDID: "dod-1", DoDDefinition: "works",
		CapabilityContract: []string{StepKindFileWrite, RuntimeBlockKindManagedValidationSession, RuntimeBlockKindDecisionRequest},
	}
	activation := RuntimeActivationContext{ExecutionContract: RuntimeExecutionContract{
		OperationContractRevision: RuntimeOperationContractRevision(),
		StateChangePolicy:         RuntimeStateChangeAllowedButNotRequired,
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
	catalogs[1].ValidationRecipes = []RuntimeValidationRecipe{{ID: "recipe/current-validation", Command: "go", Args: []string{"test", "./..."}}}
	catalogs[1].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalogs[1])
	catalogs[0].SelectorRefs = []string{"file/workspace-relative-create"}
	catalogs[0].ConstraintRefs = []string{"workspace-relative-path"}
	catalogs[0].OperationVariants = []RuntimeOperationAdmissibilityVariant{{
		Kind: StepKindFileWrite,
		AllowedTargetPolicies: []RuntimeOperationTargetPolicy{
			RuntimeOperationTargetPolicyExactTarget,
			RuntimeOperationTargetPolicyExistingOwnerScope,
			RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		},
		SelectorRefs: []string{"file/workspace-relative-create"}, ConstraintRefs: []string{"workspace-relative-path"},
	}}
	catalogs[0].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalogs[0])
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
	withoutApplicableRecipe := append([]RuntimeOperationAdmissibility(nil), catalogs...)
	withoutApplicableRecipe[1].ValidationRecipeRefs = nil
	withoutApplicableRecipe[1].ValidationRecipes = nil
	withoutApplicableRecipe[1].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(withoutApplicableRecipe[1])
	manifest, err = CompileRuntimeSemanticPlanManifest("ws-1", "run-1", definition, seed, activation.ExecutionContract, withoutApplicableRecipe)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.AllowedOperationKinds) != 1 || manifest.AllowedOperationKinds[0] != StepKindFileWrite ||
		len(manifest.AllowedOwnerModuleIDs) != 1 || manifest.AllowedOwnerModuleIDs[0] != RuntimeModuleFile ||
		len(manifest.OwnerCatalogs) != 1 || manifest.OwnerCatalogs[0].OwnerModuleID != RuntimeModuleFile {
		t.Fatalf("recipe-less managed validation remained provider-selectable: %+v", manifest)
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

func TestCompileRuntimeSemanticPlanManifestOmitsSettledPrerequisiteFromProviderOrder(t *testing.T) {
	definition := RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "deliver", DoDID: "dod-1", DoDDefinition: "works",
		CapabilityContract: []string{StepKindFileWrite},
	}
	execution := RuntimeExecutionContract{
		OperationContractRevision: RuntimeOperationContractRevision(),
		StateChangePolicy:         RuntimeStateChangeAllowedButNotRequired,
	}
	activation := RuntimeActivationContext{ExecutionContract: execution}
	items := []RuntimeCoverageItem{
		{ID: "cwi-source", SliceID: "slice-1", SourceScope: "deliverable", SourceID: "source", Order: 1},
		{ID: "cwi-dependent", SliceID: "slice-2", SourceScope: "deliverable", SourceID: "dependent", Order: 2, PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}},
	}
	slices := []RuntimeAcceptanceSlice{
		{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}},
		{ID: "slice-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-dependent"}},
	}
	seed, err := CompileRuntimeDefinitionSeed(definition, activation, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	catalog := semanticTestOwnerCatalog("file-catalog", RuntimeModuleFile, []string{StepKindFileWrite})
	manifest, err := CompileRuntimeSemanticPlanManifestForCoverage("ws-1", "run-1", definition, seed, execution, []RuntimeOperationAdmissibility{catalog}, []common.CoverageWorkItemID{"cwi-dependent"})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.CoverageItems) != 1 || manifest.CoverageItems[0].ID != "cwi-dependent" || len(manifest.CoverageItems[0].PrerequisiteCoverageWorkItemIDs) != 0 {
		t.Fatalf("eligible manifest retained settled predecessor: %+v", manifest.CoverageItems)
	}
	order := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{"cwi-dependent"},
	}
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatalf("dependent-only coverage order: %v", err)
	}
	if len(seed.CoverageItems[1].PrerequisiteCoverageWorkItemIDs) != 1 {
		t.Fatal("manifest projection mutated complete Definition seed graph")
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

func TestRuntimeSemanticDocumentRequiresOneBoundedProofPerEvidenceKind(t *testing.T) {
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
	if err := duplicate.ValidateAgainstManifest(manifest); err == nil || !strings.Contains(err.Error(), "duplicates proof kind") {
		t.Fatalf("duplicate proposal error = %v", err)
	}
	missing := document.Clone()
	missing.Proposals = nil
	if err := missing.ValidateAgainstManifest(manifest); err == nil || !strings.Contains(err.Error(), "does not cover") {
		t.Fatalf("missing proposal error = %v", err)
	}
	escaped := document.Clone()
	escaped.Proposals[0].Target.OwnerSelectorRefs = []string{"file/unsealed-selector"}
	if err := escaped.ValidateAgainstManifest(manifest); err == nil || !strings.Contains(err.Error(), "target.ownerSelectorRefs") {
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

func TestRuntimeSemanticDocumentAllowsIndependentProofsForOneCWI(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	shellCatalog := semanticTestOwnerCatalog("semantic-shell-catalog", RuntimeModuleShell, []string{StepKindShellCommand, RuntimeBlockKindManagedValidationSession})
	shellCatalog.SelectorRefs = []string{"file/workspace-relative-create"}
	shellCatalog.ValidationRecipeRefs = []string{"recipe/validate"}
	shellCatalog.OperationVariants = []RuntimeOperationAdmissibilityVariant{{
		Kind: StepKindShellCommand,
		AllowedTargetPolicies: []RuntimeOperationTargetPolicy{
			RuntimeOperationTargetPolicyExactTarget,
			RuntimeOperationTargetPolicyExistingOwnerScope,
		},
	}}
	shellCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(shellCatalog)
	manifest.AllowedOwnerModuleIDs = append(manifest.AllowedOwnerModuleIDs, RuntimeModuleShell)
	manifest.AllowedOperationKinds = append(manifest.AllowedOperationKinds, StepKindShellCommand, RuntimeBlockKindManagedValidationSession)
	manifest.AllowedTargetPolicies = append(manifest.AllowedTargetPolicies, RuntimeOperationTargetPolicyExistingOwnerScope)
	manifest.OwnerCatalogs = append(manifest.OwnerCatalogs, shellCatalog)
	manifest = sealSemanticTestManifest(t, manifest)

	stateChange := semanticContractTestProposal("cwi-001")
	validation := RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-001", OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		Target:                RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"file/workspace-relative-create"}},
		RequiredEvidenceKinds: []string{EvidenceKindValidation}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession},
		ValidationRecipeID: "recipe/validate", DeclarationRef: stateChange.DeclarationRef,
	}
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "batch-1", BatchSequence: 0,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, Proposals: []RuntimeSemanticProofProposal{stateChange, validation},
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatalf("independent state-change and validation proofs for one CWI must be valid: %v", err)
	}
}

func TestRuntimeSemanticDocumentRejectsValidationOnOwnerWithoutRecipe(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	gitCatalog := semanticTestOwnerCatalog("semantic-git-catalog", RuntimeModuleGit, []string{StepKindGitWrite})
	gitCatalog.SelectorRefs = []string{"workspace-repository"}
	gitCatalog.OperationVariants = []RuntimeOperationAdmissibilityVariant{{
		Kind: StepKindGitWrite, VariantID: "commit",
		AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExactTarget, RuntimeOperationTargetPolicyExistingOwnerScope},
		SelectorRefs:          []string{"workspace-repository"},
	}}
	gitCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(gitCatalog)
	manifest.AllowedOwnerModuleIDs = append(manifest.AllowedOwnerModuleIDs, RuntimeModuleGit)
	manifest.AllowedOperationKinds = append(manifest.AllowedOperationKinds, StepKindGitWrite)
	manifest.AllowedTargetPolicies = append(manifest.AllowedTargetPolicies, RuntimeOperationTargetPolicyExistingOwnerScope)
	manifest.OwnerCatalogs = append(manifest.OwnerCatalogs, gitCatalog)
	manifest.CoverageItems = append(manifest.CoverageItems, RuntimeCoverageItem{ID: "cwi-002", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-002", TargetText: "commit", Order: 2})
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	proposal := RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-002", OwnerModuleID: RuntimeModuleGit, OperationKind: StepKindGitWrite, OperationVariantID: "commit",
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

func TestRuntimeSemanticProposalTargetRefsIncludesProspectiveIdentityForOwnerProbe(t *testing.T) {
	proposal := semanticContractTestProposal("cwi-001")
	proposal.Target = RuntimeOperationTargetAuthorization{
		Policy:                    RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		OwnerSelectorRefs:         []string{"workspace-validation-target"},
		ProspectiveTargetRef:      "go-test-memory-usage",
		ProspectiveConstraintRefs: []string{"bounded-timeout"},
	}
	if refs := RuntimeSemanticProposalTargetRefs(proposal); len(refs) != 1 || refs[0] != "go-test-memory-usage" {
		t.Fatalf("prospective target was not preserved for owner feasibility: %+v", refs)
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
		Status: RuntimeSemanticFeasibilityFeasible, ObservedRevision: "workspace-1", TargetRefs: []string{proposal.Target.ProspectiveTargetRef},
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
	catalog.OperationVariants = []RuntimeOperationAdmissibilityVariant{{
		Kind: StepKindFileWrite,
		AllowedTargetPolicies: []RuntimeOperationTargetPolicy{
			RuntimeOperationTargetPolicyExactTarget,
			RuntimeOperationTargetPolicyExistingOwnerScope,
			RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		},
		SelectorRefs: []string{"file/workspace-relative-create"}, ConstraintRefs: []string{"workspace-relative-path"},
	}}
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

// sealSemanticTestManifest recreates the immutable catalog and manifest
// identities after a test intentionally changes its fixture facts.
func sealSemanticTestManifest(t *testing.T, manifest RuntimeSemanticPlanManifest) RuntimeSemanticPlanManifest {
	t.Helper()
	manifest = manifest.Normalized()
	for index := range manifest.OwnerCatalogs {
		manifest.OwnerCatalogs[index].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(manifest.OwnerCatalogs[index])
	}
	manifest = manifest.Normalized()
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		t.Fatalf("seal semantic test manifest: %v", err)
	}
	return manifest
}

func semanticContractTestProposal(id common.CoverageWorkItemID) RuntimeSemanticProofProposal {
	return RuntimeSemanticProofProposal{
		CoverageWorkItemID: id, OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite,
		Target: RuntimeOperationTargetAuthorization{
			Policy:                    RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			OwnerSelectorRefs:         []string{"file/workspace-relative-create"},
			ProspectiveConstraintRefs: []string{"workspace-relative-path"}, ProspectiveTargetRef: "new-file.go",
		},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		DeclarationRef: "semantic-plan/dod-001",
	}
}

// semanticSealedLedgerFixture is intentionally assembled through the same
// pure batch transitions used by the Orchestrator.  History tests therefore
// exercise the real sealed-generation shape instead of a permissive partial
// DTO that could hide a missing lineage invariant.
func semanticSealedLedgerFixture(t *testing.T) RuntimeSemanticPlanningLedger {
	t.Helper()
	definition := RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
		Revision: "definition-history", GoalID: "goal-history", GoalDefinition: "deliver",
		DoDID: "dod-history", DoDDefinition: "works", CapabilityContract: []string{StepKindFileWrite},
	}
	activation := RuntimeActivationContext{ExecutionContract: RuntimeExecutionContract{
		OperationContractRevision: RuntimeOperationContractRevision(),
		StateChangePolicy:         RuntimeStateChangeAllowedButNotRequired,
	}}
	items := []RuntimeCoverageItem{{ID: "cwi-history", SliceID: "slice-history", SourceScope: "dodItem", SourceID: "dod-history", TargetText: "works", Order: 1}}
	slices := []RuntimeAcceptanceSlice{{ID: "slice-history", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-history"}}}
	seed, err := CompileRuntimeDefinitionSeed(definition, activation, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	catalog := semanticTestOwnerCatalog("semantic-history-catalog", RuntimeModuleFile, []string{StepKindFileWrite})
	catalog.TargetRefs = []string{"main.go"}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest, err := CompileRuntimeSemanticPlanManifest("ws-history", "run-history", definition, seed, activation.ExecutionContract, []RuntimeOperationAdmissibility{catalog})
	if err != nil {
		t.Fatal(err)
	}
	proposal := RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-history", OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite,
		Target:                RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"main.go"}},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		DeclarationRef: "semantic-history/cwi-history",
	}
	order := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{"cwi-history"},
	}
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	batch, err := CompileRuntimeSemanticPlanFocusBatch(manifest, order, 0, "semantic-history-request", []common.CoverageWorkItemID{"cwi-history"}, []common.CoverageWorkItemID{"cwi-history"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if batch, err = AdvanceRuntimeSemanticPlanBatch(batch, RuntimeSemanticPlanBatchRequesting); err != nil {
		t.Fatal(err)
	}
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-history"}, Proposals: []RuntimeSemanticProofProposal{proposal},
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	batch.DocumentHash = StableRuntimeSemanticPlanDocumentHash(document)
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	for _, next := range []RuntimeSemanticPlanBatchState{
		RuntimeSemanticPlanBatchDocumentComplete,
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
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID,
		CatalogSnapshotID: catalog.SnapshotID, Status: RuntimeSemanticFeasibilityFeasible, ObservedRevision: catalog.ObservedRevision, TargetRefs: []string{"main.go"},
	}
	feasibilityHash := StableRuntimeSemanticFeasibilitySetHash([]RuntimeSemanticProofFeasibility{feasibility})
	documentHash := StableRuntimeSemanticPlanDocumentHash(document)
	batchID := StableRuntimeSemanticFeasibilityBatchID(manifest, document)
	orderHash := StableRuntimeSemanticPlanCoverageOrderHash(order)
	merged := document.Clone()
	ledger := RuntimeSemanticPlanningLedger{
		SchemaVersion: RuntimeSemanticPlanningLedgerSchemaCurrent, Phase: RuntimeSemanticPlanningSealed, DefinitionSeed: seed,
		Manifest: &manifest, StablePrefixHash: "semantic-history-prefix", LogicalRequestID: "semantic-history-request",
		CoverageOrderDocument: &order, Document: &document, DocumentHash: documentHash,
		Batches: []RuntimeSemanticPlanBatch{batch}, ActiveBatchID: "", NextBatchSequence: 1,
		CompletedBatchIDs: []string{batch.BatchID}, BatchDocuments: []RuntimeSemanticPlanDocument{document},
		MergedDocument: &merged, MergedDocumentHash: documentHash, FeasibilityBatchID: batchID,
		Feasibilities: []RuntimeSemanticProofFeasibility{feasibility}, FeasibilitySetHash: feasibilityHash,
		SealedBindingSetHash: "binding-history", SealedLineageRefs: []string{manifest.Hash, orderHash, documentHash, batchID, feasibilityHash, "binding-history"},
	}
	if err := ledger.Validate(); err != nil {
		t.Fatalf("sealed semantic fixture invalid: %v", err)
	}
	return ledger
}

func TestRuntimeSemanticPlanningHistoryRetainsBoundedDetachedGenerations(t *testing.T) {
	ledger := semanticSealedLedgerFixture(t)
	history := RuntimeSemanticPlanningGeneration{
		RetiredAtStateRevision: 2, Reason: RuntimeSemanticPlanningRetirementResourceInvalidated,
		InvalidationSignatures: []string{"resource-change-history"}, PrerequisiteIDs: []string{"semantic-history-catalog"}, Snapshot: ledger,
	}
	ledger.History = []RuntimeSemanticPlanningGeneration{history}
	if err := ledger.Validate(); err != nil {
		t.Fatal(err)
	}
	cloned := ledger.Clone()
	cloned.History[0].Snapshot.Manifest.OwnerCatalogs[0].TargetRefs[0] = "mutated.go"
	cloned.History[0].Snapshot.Feasibilities[0].TargetRefs[0] = "mutated.go"
	if ledger.History[0].Snapshot.Manifest.OwnerCatalogs[0].TargetRefs[0] != "main.go" || ledger.History[0].Snapshot.Feasibilities[0].TargetRefs[0] != "main.go" {
		t.Fatal("semantic generation clone aliases nested owner facts")
	}

	nested := ledger.Clone()
	nested.History = []RuntimeSemanticPlanningGeneration{{
		RetiredAtStateRevision: 2, Reason: RuntimeSemanticPlanningRetirementResourceInvalidated,
		InvalidationSignatures: []string{"resource-change-history"}, PrerequisiteIDs: []string{"semantic-history-catalog"}, Snapshot: ledger,
	}}
	nested.History[0].Snapshot.History = []RuntimeSemanticPlanningGeneration{history}
	if err := nested.Validate(); err == nil {
		t.Fatal("nested semantic generation history was accepted")
	}

	duplicate := ledger.Clone()
	duplicate.History = []RuntimeSemanticPlanningGeneration{history, history}
	if err := duplicate.Validate(); err == nil {
		t.Fatal("duplicate semantic generation history was accepted")
	}
	duplicateManifest := ledger.Clone()
	secondGeneration := history
	secondGeneration.RetiredAtStateRevision = 3
	// Change a generation-local field while keeping the snapshot otherwise
	// valid.  The duplicate check must therefore reject the second history entry
	// because its sealed manifest + logical-request identity is unchanged,
	// rather than because a hand-edited document hash made the snapshot
	// malformed.
	secondGeneration.Snapshot.StablePrefixHash = "different-prefix-under-same-generation"
	duplicateManifest.History = []RuntimeSemanticPlanningGeneration{history, secondGeneration}
	if err := duplicateManifest.Validate(); err == nil {
		t.Fatal("same semantic generation identity was accepted twice")
	}

	// A resource rollback may recreate an older catalog snapshot.  It is a new
	// semantic request generation only when the request identity is also new;
	// preserving both entries keeps the old authority immutable while allowing
	// the fresh request to converge.
	rollback := ledger.Clone()
	rollbackSnapshot := ledger.Clone()
	rollbackSnapshot.LogicalRequestID = "rollback-request-under-same-catalog"
	rollbackSnapshot.History = nil
	rollbackGeneration := RuntimeSemanticPlanningGeneration{
		RetiredAtStateRevision: 3, Reason: RuntimeSemanticPlanningRetirementResourceInvalidated,
		InvalidationSignatures: []string{"resource-change-rollback"}, PrerequisiteIDs: []string{"semantic-history-catalog"},
		Snapshot: rollbackSnapshot,
	}
	rollback.History = []RuntimeSemanticPlanningGeneration{history, rollbackGeneration}
	if err := rollback.Validate(); err != nil {
		t.Fatalf("distinct logical generation under a recreated catalog was rejected: %v", err)
	}
	duplicateInvalidation := rollback.Clone()
	duplicateInvalidation.History[1].InvalidationSignatures = []string{"resource-change-history"}
	if err := duplicateInvalidation.Validate(); err == nil {
		t.Fatal("an invalidation signature was attached to more than one semantic generation")
	}
	duplicatePrerequisite := ledger.Clone()
	duplicatePrerequisite.History = []RuntimeSemanticPlanningGeneration{history}
	duplicatePrerequisite.History[0].PrerequisiteIDs = []string{"semantic-history-catalog", "semantic-history-catalog"}
	if err := duplicatePrerequisite.Validate(); err == nil {
		t.Fatal("duplicate semantic generation prerequisite lineage was accepted")
	}

	tooMany := ledger.Clone()
	tooMany.History = make([]RuntimeSemanticPlanningGeneration, RuntimeSemanticPlanningHistoryLimit+1)
	for idx := range tooMany.History {
		tooMany.History[idx] = history
		tooMany.History[idx].RetiredAtStateRevision = uint64(idx + 2)
	}
	if err := tooMany.Validate(); err == nil {
		t.Fatal("semantic generation history exceeded its bound without rejection")
	}
}

func TestRuntimeSemanticPlanningHistoryRejectsSeedAndReasonDrift(t *testing.T) {
	ledger := semanticSealedLedgerFixture(t)
	history := RuntimeSemanticPlanningGeneration{
		RetiredAtStateRevision: 2, Reason: RuntimeSemanticPlanningRetirementCatalogRefresh,
		InvalidationSignatures: []string{"catalog-refresh-history"}, PrerequisiteIDs: []string{"semantic-history-catalog"}, Snapshot: ledger,
	}
	withHistory := ledger.Clone()
	withHistory.History = []RuntimeSemanticPlanningGeneration{history}
	if err := withHistory.Validate(); err != nil {
		t.Fatal(err)
	}
	badReason := withHistory.Clone()
	badReason.History[0].Reason = RuntimeSemanticPlanningRetirementReason("not-a-retirement")
	if err := badReason.Validate(); err == nil {
		t.Fatal("unsupported semantic retirement reason was accepted")
	}
	badSeed := withHistory.Clone()
	badSeed.DefinitionSeed.PolicyHash = "different-policy"
	badSeed.DefinitionSeed.Hash = StableRuntimeDefinitionSeedHash(badSeed.DefinitionSeed)
	if err := badSeed.Validate(); err == nil {
		t.Fatal("semantic history with a mismatched definition seed was accepted")
	}
}

func TestRuntimeSemanticPlanningEligibleClosureHistoryIsTypedAndDetached(t *testing.T) {
	ledger := semanticSealedLedgerFixture(t)
	coverageID := ledger.DefinitionSeed.CoverageItems[0].ID
	history := RuntimeSemanticPlanningGeneration{
		RetiredAtStateRevision: 2,
		Reason:                 RuntimeSemanticPlanningRetirementEligibleClosureChanged,
		ChangedCoverageIDs:     []common.CoverageWorkItemID{coverageID}, ClosureChangeTriggerRefs: []string{"binding-prerequisite"},
		Snapshot: ledger,
	}
	if err := history.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := history.Clone()
	clone.ChangedCoverageIDs[0] = "mutated"
	clone.ClosureChangeTriggerRefs[0] = "mutated"
	if history.ChangedCoverageIDs[0] != coverageID || history.ClosureChangeTriggerRefs[0] != "binding-prerequisite" {
		t.Fatal("eligible closure history clone aliases typed lineage")
	}
	invalid := history
	invalid.ChangedCoverageIDs = []common.CoverageWorkItemID{coverageID, coverageID}
	if err := invalid.Validate(); err == nil {
		t.Fatal("duplicate eligible closure lineage was accepted")
	}
	invalid = history
	invalid.ClosureChangeTriggerRefs = nil
	if err := invalid.Validate(); err == nil {
		t.Fatal("eligible closure history without prerequisite binding refs was accepted")
	}
}
