package semanticplan

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestBatchLookupsReturnDetachedValues(t *testing.T) {
	document := runtimecontract.RuntimeSemanticPlanDocument{BatchID: "batch-1"}
	batch := runtimecontract.RuntimeSemanticPlanBatch{BatchID: "batch-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}
	gotDocument, ok := DocumentByBatchID([]runtimecontract.RuntimeSemanticPlanDocument{document}, "batch-1")
	if !ok || gotDocument.BatchID != document.BatchID {
		t.Fatalf("document lookup = %+v/%t", gotDocument, ok)
	}
	gotBatch, ok := BatchByID([]runtimecontract.RuntimeSemanticPlanBatch{batch}, "batch-1")
	if !ok || gotBatch.BatchID != batch.BatchID {
		t.Fatalf("batch lookup = %+v/%t", gotBatch, ok)
	}
	gotBatch.CoverageWorkItemIDs[0] = "changed"
	if batch.CoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatal("batch lookup exposed caller-owned slice")
	}
	if _, ok := BatchByID([]runtimecontract.RuntimeSemanticPlanBatch{batch}, ""); ok {
		t.Fatal("empty batch identity matched")
	}
}

func TestExistingTargetContextCarriesRevisionAndOwnerAlternatives(t *testing.T) {
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "catalog-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{
			{Kind: runtimecontract.StepKindFileWrite, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget}, SelectorRefs: []string{"new-target"}, ConstraintRefs: []string{"path"}},
			{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "text_replace", AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope}, ConstraintRefs: []string{"existing-target"}},
		},
		TargetRefs: []string{"mcp-lsp-bridge/server.go"}, SelectorRefs: []string{"new-target"}, ConstraintRefs: []string{"path", "existing-target"}, ObservedRevision: "tree-v2",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent, WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", DefinitionSeedHash: "seed-1", OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems:         []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "change", Order: 1}},
		AllowedOwnerModuleIDs: []string{runtimecontract.RuntimeModuleFile}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply}, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget}, ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{CoverageWorkItemID: "cwi-1", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}}, OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleFile, OriginalClass: "semantic_feasibility", OriginalCode: "operation_target_exists", OriginalMessage: "must not be rendered", Signature: "file/operation-target-exists", TargetRefs: []string{"mcp-lsp-bridge/server.go"}, ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2"}
	context, err := BuildExistingTargetContext(manifest, diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if !context.ExistingTargetObserved || context.ObservedRevision != "tree-v2" || context.CatalogSnapshotID != catalog.SnapshotID || context.CatalogObservedRevision != "tree-v2" {
		t.Fatalf("existing target context lost revision identity: %+v", context)
	}
	if len(context.ExistingOperationVariants) != 1 || context.ExistingOperationVariants[0] != runtimecontract.RuntimeBlockKindPatchApply+"/text_replace" {
		t.Fatalf("existing replacement alternatives = %v", context.ExistingOperationVariants)
	}
	if contains(context.ExistingOperationKinds, runtimecontract.StepKindFileWrite) || !contains(context.ConstraintRefs, "existing-target") {
		t.Fatalf("create-only operation leaked into existing alternatives: %+v", context)
	}
	if contains([]string{context.FailureClass, context.FailureCode, context.FailureSignature}, diagnostic.OriginalMessage) {
		t.Fatal("raw owner message entered typed replan context")
	}
}

func TestValidateManifestIdentityUsesCanonicalCompilation(t *testing.T) {
	definition := runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{runtimecontract.StepKindFileWrite}}
	activation := runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"}}, ExecutionContract: runtimecontract.RuntimeExecutionContract{
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		StateChangePolicy:         runtimecontract.RuntimeStateChangeAllowedButNotRequired,
	}}
	// Keep the fixture independent from provider details; the contract helper
	// validates the execution selection and catalog identity.
	items := []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "change", Order: 1}}
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(definition, activation, items, []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility("catalog-file", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileWrite}, []string{"main.go"}, []string{"new-target"}, []string{"path"}, "tree-v1")
	if err != nil {
		t.Fatal(err)
	}
	catalog.OperationVariants = []runtimecontract.RuntimeOperationAdmissibilityVariant{{Kind: runtimecontract.StepKindFileWrite, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget}, SelectorRefs: []string{"new-target"}, ConstraintRefs: []string{"path"}}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	focuses := runtimecontract.CompileRuntimeDefinitionFocuses([]runtimecontract.CoverageFocus{{
		ID: "focus-1", SliceID: "slice-1", AssessmentScope: "dodItem", AssessmentID: "dod-1",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequireAssessment: true,
	}})
	identity := ManifestIdentity{WorkspaceID: "ws-1", RunID: "run-1", Definition: definition, DefinitionSeed: seed, DefinitionFocuses: focuses, Execution: activation.ExecutionContract, Admissibilities: []runtimecontract.RuntimeOperationAdmissibility{catalog}, EligibleCoverage: []common.CoverageWorkItemID{"cwi-1"}}
	manifest, err := CompileManifest(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifestIdentity(identity, manifest); err != nil {
		t.Fatal(err)
	}
	stale := manifest.Clone()
	stale.DefinitionRevision = "definition-stale"
	stale.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(stale)
	if err := ValidateManifestIdentity(identity, stale); err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale manifest error = %v", err)
	}
}
