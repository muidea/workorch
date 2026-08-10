package workorchruntimecontract_test

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeSemanticProofFeasibilityCommandRequiresExactOwnerAndCatalog(t *testing.T) {
	command := semanticFeasibilityCommandFixture(t)
	if err := runtimecontract.ValidateRuntimeSemanticProofFeasibilityRequest(command, runtimecontract.RuntimeModuleFile); err != nil {
		t.Fatal(err)
	}
	if err := runtimecontract.ValidateRuntimeSemanticProofFeasibilityRequest(command, runtimecontract.RuntimeModuleShell); err == nil || !strings.Contains(err.Error(), "does not match receiver") {
		t.Fatalf("owner mismatch error = %v", err)
	}
	outside := command
	outside.Proposal.Target.ProspectiveConstraintRefs = []string{"outside-owner-contract"}
	if err := runtimecontract.ValidateRuntimeSemanticProofFeasibilityRequest(outside, runtimecontract.RuntimeModuleFile); err == nil || !strings.Contains(err.Error(), "outside owner catalog") {
		t.Fatalf("catalog escape error = %v", err)
	}
}

func TestRuntimeSemanticProofFeasibilityResultPreservesOriginalDiagnostic(t *testing.T) {
	command := semanticFeasibilityCommandFixture(t)
	diagnostic := runtimecontract.RuntimeSemanticFeasibilityDiagnostic(runtimecontract.RuntimeModuleFile, "target_exists", "the exact owner message", "workspace-1", "workspace-2", []string{"main.go"})
	diagnostic.OriginalMessage = "  the exact owner message  "
	response, err := runtimecontract.NewRuntimeSemanticProofFeasibilityObservation(command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, "workspace-2", []string{"main.go"}, diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if response.FeasibilityBatchID != command.FeasibilityBatchID || response.Observation.OriginalDiagnostic == diagnostic ||
		response.Observation.OriginalDiagnostic.OriginalMessage != "  the exact owner message  " || response.Observation.OriginalDiagnostic.ExpectedRevision != "workspace-1" {
		t.Fatalf("semantic feasibility response = %+v", response)
	}
	if _, err := runtimecontract.NewRuntimeSemanticProofFeasibilityObservation(command, runtimecontract.RuntimeSemanticFeasibilityFeasible, "workspace-2", nil, diagnostic); err == nil {
		t.Fatal("feasible result accepted failure diagnostic")
	}
}

func TestRuntimeSemanticProofFeasibilityRejectsForgedSealedOwnerFacts(t *testing.T) {
	command := semanticFeasibilityCommandFixture(t)
	valid, err := runtimecontract.NewRuntimeSemanticProofFeasibilityObservation(
		command, runtimecontract.RuntimeSemanticFeasibilityFeasible, "workspace-2", []string{"main.go"}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	wrongCatalog := valid.Observation.Clone()
	wrongCatalog.CatalogSnapshotID = "other-catalog"
	if err := wrongCatalog.ValidateForProposal(command.Manifest, command.Proposal); err == nil || !strings.Contains(err.Error(), "catalog identity") {
		t.Fatalf("catalog substitution error = %v", err)
	}
	forgedTarget := valid.Observation.Clone()
	forgedTarget.TargetRefs = []string{"outside.go"}
	if err := forgedTarget.ValidateForProposal(command.Manifest, command.Proposal); err == nil || !strings.Contains(err.Error(), "target does not match") {
		t.Fatalf("prospective target substitution error = %v", err)
	}

	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "shell-catalog", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		TargetRefs:     []string{"main.go"}, SelectorRefs: []string{"workspace-validation-target"},
		ValidationRecipeRefs: []string{"recipe/test"}, ObservedRevision: "shell-revision",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", DefinitionSeedHash: "seed-1",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems:             []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "works", Order: 1}},
		AllowedOwnerModuleIDs:     []string{runtimecontract.RuntimeModuleShell},
		AllowedOperationKinds:     []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		AllowedTargetPolicies:     []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope},
		OwnerCatalogs:             []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-1", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"workspace-validation-target"},
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
		ValidationRecipeID:    "recipe/test", DeclarationRef: "semantic/dod-1",
	}
	base := runtimecontract.RuntimeSemanticProofFeasibility{
		SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal),
		OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID,
		Status: runtimecontract.RuntimeSemanticFeasibilityFeasible, ObservedRevision: catalog.ObservedRevision,
		ValidationRecipeID: proposal.ValidationRecipeID, RequiredCheckRefs: []string{"recipe/test"},
	}
	if err := base.ValidateForProposal(manifest, proposal); err != nil {
		t.Fatal(err)
	}
	selectorEscape := base.Clone()
	selectorEscape.TargetRefs = []string{"outside.go"}
	if err := selectorEscape.ValidateForProposal(manifest, proposal); err == nil || !strings.Contains(err.Error(), "target is outside") {
		t.Fatalf("selector-only target escape error = %v", err)
	}
	forgedCheck := base.Clone()
	forgedCheck.RequiredCheckRefs = []string{"unsealed-check"}
	if err := forgedCheck.ValidateForProposal(manifest, proposal); err == nil || !strings.Contains(err.Error(), "check is outside") {
		t.Fatalf("check substitution error = %v", err)
	}
}

func semanticFeasibilityCommandFixture(t *testing.T) runtimecontract.RuntimeSemanticProofFeasibilityRequest {
	t.Helper()
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", DefinitionSeedHash: "seed-1",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems:             []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "works", Order: 1}},
		AllowedOwnerModuleIDs:     []string{runtimecontract.RuntimeModuleFile}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
	}
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-1", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, OwnerSelectorRefs: []string{"workspace-relative-new-file-target"},
			ProspectiveConstraintRefs: []string{"workspace-relative-path"}, ProspectiveTargetRef: "main.go",
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, DeclarationRef: "semantic/dod-1",
	}
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility("semantic-catalog-file", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileWrite}, []string{"main.go"},
		[]string{"workspace-relative-new-file-target"}, []string{"workspace-relative-path"}, "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	manifest.OwnerCatalogs = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	return runtimecontract.RuntimeSemanticProofFeasibilityRequest{
		WorkspaceID: "ws-1", RunID: "run-1", FeasibilityBatchID: "batch-1", WorkspaceDeliverableDirectory: t.TempDir(),
		Manifest: manifest, Proposal: proposal, OwnerCatalog: catalog,
	}
}
