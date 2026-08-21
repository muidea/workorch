package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

var (
	runtimeFileBenchmarkSelection runtimecontract.RuntimeTargetSelection
	runtimeFileBenchmarkInventory runtimecontract.RuntimeOperationAdmissibility
	runtimeFileBenchmarkSnapshot  runtimecontract.TargetSnapshot
	runtimeFileBenchmarkManifest  runtimecontract.WorkspaceManifest
	runtimeFileBenchmarkPatch     RuntimeFileOperationResult
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

func BenchmarkRuntimeAdapterTargetSnapshotLargeFile(b *testing.B) {
	workspace := b.TempDir()
	content := strings.Repeat("0123456789abcdef\n", 3000)
	if err := os.WriteFile(filepath.Join(workspace, "large.txt"), []byte(content), 0o644); err != nil {
		b.Fatal(err)
	}
	adapter := NewRuntimeAdapter()
	b.ReportAllocs()
	b.SetBytes(int64(len(content)))
	b.ResetTimer()
	for range b.N {
		snapshot, err := adapter.AcquireTargetSnapshot(context.Background(), workspace, "large.txt", defaultMaxReadBytes)
		if err != nil || !snapshot.Complete() || snapshot.ContentHash == "" {
			b.Fatalf("snapshot = %+v err=%v", snapshot, err)
		}
		runtimeFileBenchmarkSnapshot = snapshot
	}
}

func BenchmarkRuntimeAdapterWorkspaceManifestLargeDirectory(b *testing.B) {
	workspace := b.TempDir()
	for idx := range 400 {
		ref := filepath.Join(workspace, fmt.Sprintf("group-%02d", idx%20), fmt.Sprintf("file-%03d.txt", idx))
		if err := os.MkdirAll(filepath.Dir(ref), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(ref, []byte(fmt.Sprintf("file %d\n", idx)), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	adapter := NewRuntimeAdapter()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		manifest, err := adapter.AcquireWorkspaceManifest(workspace)
		if err != nil || manifest.Revision == "" || len(manifest.TargetRefs) != 400 {
			b.Fatalf("manifest targets=%d revision=%q err=%v", len(manifest.TargetRefs), manifest.Revision, err)
		}
		runtimeFileBenchmarkManifest = manifest
	}
}

func BenchmarkRuntimeAdapterPatchApplyReplayLargeFile(b *testing.B) {
	workspace := b.TempDir()
	const targetRef = "large.txt"
	before := []byte("before\n" + strings.Repeat("payload\n", 6000))
	after := []byte("after\n" + strings.Repeat("payload\n", 6000))
	if err := os.WriteFile(filepath.Join(workspace, targetRef), after, 0o644); err != nil {
		b.Fatal(err)
	}
	proof := runtimecontract.RuntimeOperationReplayProof{
		OwnerModuleID: ModuleID, OperationID: "patch-1", TargetRefs: []string{targetRef},
		BeforeHash: sha256Hex(before), AfterHash: sha256Hex(after), AdmissibilitySnapshotID: "admissibility-benchmark",
	}
	adapter := NewRuntimeAdapter()
	b.ReportAllocs()
	b.SetBytes(int64(len(after)))
	b.ResetTimer()
	for range b.N {
		result, err := adapter.ApplyPatch(PatchApplyRequest{
			WorkspaceDirectory: workspace, TargetRef: targetRef, PatchType: patchTypeTextReplace,
			OldText: "before", NewText: "after", NewTextPresent: true, ReplayProof: &proof,
		})
		if err != nil || result.AfterHash != proof.AfterHash || result.BeforeHash != proof.BeforeHash {
			b.Fatalf("patch replay = %+v err=%v", result, err)
		}
		runtimeFileBenchmarkPatch = result
	}
}
