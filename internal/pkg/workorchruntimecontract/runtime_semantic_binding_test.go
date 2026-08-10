package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCompileRuntimeSemanticBindingsSealsOneBindingPerCWI(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	proposal := semanticContractTestProposal("cwi-001")
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID: "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		Proposals: []RuntimeSemanticProofProposal{proposal},
	}
	fact := RuntimeSemanticProofFeasibility{
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID,
		CatalogSnapshotID: manifest.OwnerCatalogs[0].SnapshotID, Status: RuntimeSemanticFeasibilityFeasible, ObservedRevision: "workspace-1", TargetRefs: []string{"main.go"},
	}
	focuses, contract, err := CompileRuntimeSemanticBindings(manifest, document, []RuntimeSemanticProofFeasibility{fact})
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 1 || len(focuses[0].CoverageWorkItemIDs) != 1 || len(focuses[0].CoverageEvidenceBindings) != 1 ||
		focuses[0].ProspectiveTargetRef != "main.go" || focuses[0].TargetSelectorRef != "file/workspace-relative-create" {
		t.Fatalf("compiled focuses = %+v", focuses)
	}
	if len(contract.Bindings) != 1 || contract.BindingSetHash == "" || contract.Bindings[0].BindingHash != focuses[0].CoverageEvidenceBindings[0].Hash {
		t.Fatalf("compiled reconciliation = %+v", contract)
	}
}

func TestCompileRuntimeSemanticBindingsRejectsPartialOrNonFeasibleBatch(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	proposal := semanticContractTestProposal("cwi-001")
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID: "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		Proposals: []RuntimeSemanticProofProposal{proposal},
	}
	if _, _, err := CompileRuntimeSemanticBindings(manifest, document, nil); err == nil {
		t.Fatal("partial feasibility batch was sealed")
	}
	fact := RuntimeSemanticProofFeasibility{
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID,
		CatalogSnapshotID: manifest.OwnerCatalogs[0].SnapshotID, Status: RuntimeSemanticFeasibilityInfeasible, ObservedRevision: "workspace-1",
		OriginalDiagnostic: semanticContractTestDiagnostic(),
	}
	if _, _, err := CompileRuntimeSemanticBindings(manifest, document, []RuntimeSemanticProofFeasibility{fact}); err == nil {
		t.Fatal("non-feasible owner batch was sealed")
	}
}

func TestCompileRuntimeSemanticBindingsAllowsEmptyProspectiveOwnerFacts(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	proposal := semanticContractTestProposal("cwi-001")
	document := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, Proposals: []RuntimeSemanticProofProposal{proposal}}
	fact := RuntimeSemanticProofFeasibility{SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: manifest.OwnerCatalogs[0].SnapshotID, Status: RuntimeSemanticFeasibilityFeasible, ObservedRevision: "workspace-1"}
	focuses, _, err := CompileRuntimeSemanticBindings(manifest, document, []RuntimeSemanticProofFeasibility{fact})
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 1 || focuses[0].ProspectiveTargetRef != "main.go" {
		t.Fatalf("compiled focus = %+v", focuses)
	}
}
