package file

import (
	"os"
	"path/filepath"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

var (
	runtimeFileBenchmarkSelection runtimecontract.RuntimeTargetSelection
	runtimeFileBenchmarkInventory runtimecontract.RuntimeOperationAdmissibility
)

func BenchmarkRuntimeAdapterTargetCatalogFeasibility(b *testing.B) {
	workspace := b.TempDir()
	const targetRef = "main.go"
	const original = "package main\n\n// before\n"
	if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte(original), 0o644); err != nil {
		b.Fatal(err)
	}
	patchType, oldText, newText := patchTypeTextReplace, "before", "after"
	operation := runtimecontract.RuntimeOperation{
		ID: "benchmark-patch", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{targetRef},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldText},
			{Name: "newText", String: &newText},
		},
	}
	adapter := NewRuntimeAdapter()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		selection, err := adapter.AcquireRuntimeTargetSelection(
			workspace, "target-selection-benchmark", "deliverable",
			runtimeFileTargetSelectorUniqueWorkspaceEntry, "", 4,
		)
		if err != nil || selection.Disposition != runtimecontract.RuntimeTargetSelectionSelected {
			b.Fatalf("target selection = %+v err=%v", selection, err)
		}
		inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
			workspace, "admissibility-benchmark", []string{runtimecontract.RuntimeBlockKindPatchApply}, []string{targetRef}, 4,
		)
		if err != nil {
			b.Fatal(err)
		}
		if failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, operation); failure != nil {
			b.Fatalf("feasibility failure = %+v", failure)
		}
		runtimeFileBenchmarkSelection = selection
		runtimeFileBenchmarkInventory = inventory
	}
	b.StopTimer()
	content, err := os.ReadFile(filepath.Join(workspace, targetRef))
	if err != nil || string(content) != original {
		b.Fatalf("read-only preflight mutated target content=%q err=%v", content, err)
	}
}
