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
