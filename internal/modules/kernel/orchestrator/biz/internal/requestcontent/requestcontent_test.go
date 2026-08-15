package requestcontent

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSemanticPlanRequestContentCarriesLatestCatalogFactsWithoutRawMessage(t *testing.T) {
	manifest := requestContentManifest(t)
	prefix := "candidate-prefix-stable"
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OriginalClass: "semantic_feasibility", OriginalCode: "operation_target_exists", OriginalMessage: "private owner text must not escape",
		Signature: "file/operation-target-exists", TargetRefs: []string{"existing.go"}, ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2",
	}
	violation := runtimecontract.RuntimeSemanticContractViolation{Kind: runtimecontract.RuntimeSemanticContractViolationProposalConstraint, Path: "$.proposals[0].target.policy", CoverageWorkItemID: "cwi-1", ProposalIndex: 0, Rule: runtimecontract.RuntimeSemanticConstraintAllowedValue, Actual: "prospective_owner_target", Allowed: []string{"exact_target"}}
	diagnostic.SemanticViolations = []runtimecontract.RuntimeSemanticContractViolation{violation}
	directive := runtimecontract.RuntimeSemanticCorrectionDirective{SchemaVersion: runtimecontract.RuntimeSemanticCorrectionDirectiveSchemaCurrent, PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, StablePrefixHash: prefix, SourceRejectionHash: runtimecontract.StableRuntimeSemanticPlanCorrectionHash(*diagnostic), Attempt: 1, Violations: []runtimecontract.RuntimeSemanticContractViolation{violation}}
	directive.Hash = runtimecontract.StableRuntimeSemanticCorrectionDirectiveHash(directive)
	content, err := BuildSemanticPlanRequestContent(SemanticPlanRequestInput{Manifest: manifest, StablePrefixHash: prefix, Attempt: 1, PreviousRejection: diagnostic, CorrectionDirective: &directive, RemainingRecoveryAttempt: 1})
	if err != nil {
		t.Fatal(err)
	}
	if content.PreviousObservedRevision != "tree-v2" || len(content.CatalogSnapshotIDs) != 1 || len(content.ConstraintRefs) == 0 || !content.ExistingTarget.ExistingTargetObserved {
		t.Fatalf("semantic request content lost current owner facts: %+v", content)
	}
	if strings.Contains(strings.Join([]string{content.PreviousFailureClass, content.PreviousFailureCode, content.PreviousFailureSignature}, "|"), diagnostic.OriginalMessage) {
		t.Fatal("raw owner message entered semantic request content")
	}
	hash1, err := content.StableHash()
	if err != nil {
		t.Fatal(err)
	}
	contentCopy := content
	hash2, err := contentCopy.StableHash()
	if err != nil || hash1 != hash2 {
		t.Fatalf("volatile request hash is not deterministic: %q/%q err=%v", hash1, hash2, err)
	}
}

func TestSemanticPlanRequestContentRejectsLineageOnInitialAttempt(t *testing.T) {
	manifest := requestContentManifest(t)
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleFile, OriginalClass: "semantic", OriginalCode: "bad", OriginalMessage: "bad", Signature: "bad"}
	if _, err := BuildSemanticPlanRequestContent(SemanticPlanRequestInput{Manifest: manifest, StablePrefixHash: "prefix", PreviousRejection: diagnostic}); err == nil {
		t.Fatal("initial semantic request accepted correction lineage")
	}
}

func TestContentHashAndTailArePureAndBounded(t *testing.T) {
	if ContentHash("same") != ContentHash("same") || ContentHash("same") == ContentHash("different") {
		t.Fatal("content hash is not deterministic")
	}
	if got := ContentTail("abcdef", 3); got != "def" {
		t.Fatalf("content tail = %q", got)
	}
	if got := ContentTail("abcdef", 0); got != "" {
		t.Fatalf("zero content tail = %q", got)
	}
}

func requestContentManifest(t *testing.T) runtimecontract.RuntimeSemanticPlanManifest {
	t.Helper()
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "catalog-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply}, TargetRefs: []string{"existing.go"}, SelectorRefs: []string{"new-target"}, ConstraintRefs: []string{"path", "existing-target"}, ObservedRevision: "tree-v2",
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{
			{Kind: runtimecontract.StepKindFileWrite, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget}, SelectorRefs: []string{"new-target"}, ConstraintRefs: []string{"path"}},
			{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "text_replace", AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope}, ConstraintRefs: []string{"existing-target"}},
		},
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := runtimecontract.RuntimeSemanticPlanManifest{SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent, WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", DefinitionSeedHash: "seed-1", OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(), CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "change", Order: 1}}, AllowedOwnerModuleIDs: []string{runtimecontract.RuntimeModuleFile}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply}, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget}, OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{catalog}}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	return manifest
}
