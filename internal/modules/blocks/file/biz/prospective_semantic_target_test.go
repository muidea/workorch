package file

import (
	"os"
	"path/filepath"
	"testing"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileProspectiveSemanticTargetSetAdmissionIsReadOnlyForNestedMissingTargets(t *testing.T) {
	workspace := t.TempDir()
	targetRefs := []string{"docs/lsp_tools.md", "lsp_tools.go"}
	command := fileevents.ProbeRuntimeSemanticProofFeasibilityCommand{
		WorkspaceDeliverableDirectory: workspace,
		Proposal: runtimecontract.RuntimeSemanticProofProposal{
			CoverageWorkItemID: "cwi-repair",
			OwnerModuleID:      runtimecontract.RuntimeModuleFile,
			OperationKind:      runtimecontract.StepKindFileWrite,
			Target: runtimecontract.RuntimeOperationTargetAuthorization{
				Policy:                runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
				OwnerSelectorRefs:     []string{runtimeFileSelectorWorkspaceRelativeNewTarget},
				ProspectiveTargetRefs: targetRefs,
			},
		},
	}
	observation := workspaceManifestObservation{
		manifest: runtimecontract.WorkspaceManifest{Revision: "workspace-empty"},
	}
	if err := fileProspectiveSemanticTargetAdmission(command, observation); err != nil {
		t.Fatalf("nested prospective target set was not feasible: %v", err)
	}
	for _, targetRef := range append(targetRefs, "docs") {
		if _, err := os.Lstat(filepath.Join(workspace, filepath.FromSlash(targetRef))); !os.IsNotExist(err) {
			t.Fatalf("prospective admission mutated %q: %v", targetRef, err)
		}
	}

	if err := os.WriteFile(filepath.Join(workspace, "lsp_tools.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	observation.manifest.TargetRefs = []string{"lsp_tools.go"}
	observation.entryCount = 1
	if err := fileProspectiveSemanticTargetAdmission(command, observation); err == nil {
		t.Fatal("prospective target set accepted a target that already exists")
	}
}
