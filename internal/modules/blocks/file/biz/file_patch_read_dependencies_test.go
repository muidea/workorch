package file

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFilePatchReadDependenciesUseActualInputAndRecoveryPostcondition(t *testing.T) {
	for _, variant := range []string{patchTypeFileWrite, patchTypeTextReplace, patchTypeUnifiedDiff} {
		t.Run(variant, func(t *testing.T) {
			workspace := t.TempDir()
			before, after := []byte("原文\r\nbefore\n"), []byte("原文\r\nafter\n")
			if variant == patchTypeUnifiedDiff {
				before, after = []byte("原文\nbefore\n"), []byte("原文\nafter\n")
			}
			path := filepath.Join(workspace, "input.txt")
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			beforeDigest, afterDigest := sha256.Sum256(before), sha256.Sum256(after)
			beforeHash, afterHash := hex.EncodeToString(beforeDigest[:]), hex.EncodeToString(afterDigest[:])
			request := PatchApplyRequest{WorkspaceDirectory: workspace, TargetRef: "input.txt", PatchType: variant, BaseHash: beforeHash,
				Content: string(after), ContentPresent: true, OldText: "before", NewText: "after", NewTextPresent: true,
				Diff:        "--- a/input.txt\n+++ b/input.txt\n@@ -1,2 +1,2 @@\n 原文\n-before\n+after\n",
				ReplayProof: &runtimecontract.RuntimeOperationReplayProof{OwnerModuleID: ModuleID, OperationID: "patch", TargetRefs: []string{"input.txt"}, BeforeHash: beforeHash, AfterHash: afterHash, AdmissibilitySnapshotID: "snapshot"},
			}
			result, err := NewRuntimeAdapter().ApplyPatch(request)
			if err != nil || result.patchRead == nil || *result.patchRead != (filePatchReadObservation{TargetRef: "input.txt", ContentHash: beforeHash, Size: int64(len(before)), Purpose: "patch_input"}) {
				t.Fatalf("actual mutation read: %v %+v", err, result.patchRead)
			}
			body, err := os.ReadFile(path)
			if err != nil || !reflect.DeepEqual(body, after) {
				t.Fatalf("mutation changed original byte grammar: %v %q", err, body)
			}
			operation := runtimecontract.RuntimeOperation{ID: "patch", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"input.txt"}}
			scope := runtimecontract.RuntimeOperationExecutionContext{WorkspaceID: "ws", Transaction: runtimecontract.AuthorityTransaction{DefinitionRevision: "definition"}}
			dependencies, err := filePatchOwnerDependencies(scope, operation, result.patchRead)
			if err != nil || dependencies.Entries[0].Revision != beforeHash {
				t.Fatalf("mutation dependency did not retain original input: %v", err)
			}
			preimage, err := json.Marshal(struct {
				WorkspaceID   string
				OperationHash string
				Observation   struct {
					TargetRef, ContentHash string
					Size                   int64
					Purpose                string
				}
			}{"ws", runtimecontract.StableRuntimeOperationHash(operation), struct {
				TargetRef, ContentHash string
				Size                   int64
				Purpose                string
			}{"input.txt", beforeHash, int64(len(before)), "patch_input"}})
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(append([]byte("file-patch-read-dependency/v1\x00"), preimage...))
			if dependencies.Entries[0].DependencyHash != hex.EncodeToString(digest[:]) {
				t.Fatal("dependency differs from independent original-byte selector preimage")
			}
			for _, identity := range []string{"definition", "workspace", "target", "hash", "purpose", "size"} {
				changedScope, observation := scope, *result.patchRead
				switch identity {
				case "definition":
					changedScope.Transaction.DefinitionRevision = " definition"
				case "workspace":
					changedScope.WorkspaceID = " ws"
				case "target":
					observation.TargetRef = " input.txt"
				case "hash":
					observation.ContentHash = " " + beforeHash
				case "purpose":
					observation.Purpose = "historical_before_hash"
				case "size":
					observation.Size = -1
				}
				if set, err := filePatchOwnerDependencies(changedScope, operation, &observation); err == nil || set != nil {
					t.Fatalf("invalid actual-read identity accepted: %s", identity)
				}
			}
			changedOperation := operation
			changedOperation.ID = "other-operation"
			changedDependencies, err := filePatchOwnerDependencies(scope, changedOperation, result.patchRead)
			if err != nil || changedDependencies.SetHash == dependencies.SetHash {
				t.Fatal("read selector not bound to sealed operation")
			}
			replayed, err := NewRuntimeAdapter().ApplyPatch(request)
			if err != nil || replayed.patchRead == nil || replayed.patchRead.ContentHash != afterHash || replayed.patchRead.Purpose != "replay_postcondition" || replayed.BeforeHash != beforeHash {
				t.Fatalf("recovery manufactured historical BeforeHash as current read: %v %+v", err, replayed)
			}
			recoveryDependencies, err := filePatchOwnerDependencies(scope, operation, replayed.patchRead)
			if err != nil || recoveryDependencies.Entries[0].Revision != afterHash || recoveryDependencies.SetHash == dependencies.SetHash {
				t.Fatalf("recovery read not distinguished from original input: %v", err)
			}
			// 失败发生在完整读取之后：保留实际输入，不借 BaseHash 或成功产物。
			request.ReplayProof = nil
			request.BaseHash = "wrong"
			request.Content = "different-not-postcondition"
			failed, err := NewRuntimeAdapter().ApplyPatch(request)
			if err == nil || failed.patchRead == nil || failed.patchRead.ContentHash != afterHash || failed.BeforeHash != "" {
				t.Fatalf("rejected mutation lost actual input read: %v %+v", err, failed)
			}
		})
	}
	t.Run("no-op and rejected binary preserve exact read", func(t *testing.T) {
		workspace := t.TempDir()
		path := filepath.Join(workspace, "input.bin")
		for _, body := range [][]byte{[]byte("same\r\n"), {0, 0xff, '\n'}} {
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(body)
			hash := hex.EncodeToString(digest[:])
			result, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{WorkspaceDirectory: workspace, TargetRef: "input.bin", PatchType: patchTypeFileWrite, Content: string(body), ContentPresent: true, BaseHash: hash})
			if result.patchRead == nil || result.patchRead.ContentHash != hash || result.patchRead.Size != int64(len(body)) {
				t.Fatal("no-op/binary path lost actual byte observation")
			}
			if body[0] == 0 && err == nil || body[0] != 0 && err != nil {
				t.Fatalf("original no-op/binary gate changed: %v", err)
			}
			if actual, err := os.ReadFile(path); err != nil || !reflect.DeepEqual(actual, body) {
				t.Fatal("no-op/binary path changed original bytes")
			}
		}
	})
	t.Run("absence and unread failure", func(t *testing.T) {
		workspace := t.TempDir()
		created, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{WorkspaceDirectory: workspace, TargetRef: "new.txt", PatchType: patchTypeFileWrite, Content: "new", ContentPresent: true, CreateOnly: true})
		if err != nil || created.patchRead == nil || created.patchRead.Purpose != "patch_absence" {
			t.Fatalf("creation lost observed absence: %v", err)
		}
		failed, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{WorkspaceDirectory: workspace, TargetRef: "missing.txt", PatchType: patchTypeTextReplace, OldText: "old", NewText: "new", NewTextPresent: true})
		if err == nil || failed.patchRead == nil || failed.patchRead.Purpose != "patch_absence" {
			t.Fatal("missing target lost observed absence")
		}
		if err := os.Mkdir(filepath.Join(workspace, "directory"), 0700); err != nil {
			t.Fatal(err)
		}
		unread, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{WorkspaceDirectory: workspace, TargetRef: "directory", PatchType: patchTypeFileWrite, Content: "new", ContentPresent: true})
		if err == nil || unread.patchRead != nil {
			t.Fatal("read error manufactured absence or content proof")
		}
		if set, err := filePatchOwnerDependencies(runtimecontract.RuntimeOperationExecutionContext{}, runtimecontract.RuntimeOperation{}, nil); err != nil || set != nil {
			t.Fatal("missing observation manufactured empty dependency proof")
		}
	})
}

func TestFileCreationDependenciesDistinguishAbsenceEmptyBytesAndRecovery(t *testing.T) {
	for _, variant := range []string{patchTypeFileWrite, patchTypeUnifiedDiff} {
		t.Run(variant, func(t *testing.T) {
			workspace := t.TempDir()
			request := PatchApplyRequest{WorkspaceDirectory: workspace, TargetRef: "new.txt", PatchType: variant, Content: "new\n", ContentPresent: true, CreateOnly: true,
				Diff: "--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1 @@\n+new\n"}
			result, err := NewRuntimeAdapter().ApplyPatch(request)
			if err != nil || result.patchRead == nil || result.BeforeHash != "" {
				t.Fatalf("create: %v %+v", err, result)
			}
			absenceDigest := sha256.Sum256([]byte("file-target-existence/v1\x00absent"))
			absenceHash := hex.EncodeToString(absenceDigest[:])
			if *result.patchRead != (filePatchReadObservation{TargetRef: "new.txt", ContentHash: absenceHash, Purpose: "patch_absence"}) {
				t.Fatalf("absence identity: %+v", result.patchRead)
			}
			scope := runtimecontract.RuntimeOperationExecutionContext{WorkspaceID: "ws", Transaction: runtimecontract.AuthorityTransaction{DefinitionRevision: "definition"}}
			operation := runtimecontract.RuntimeOperation{ID: "create", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"new.txt"}}
			dependencies, err := filePatchOwnerDependencies(scope, operation, result.patchRead)
			if err != nil || len(dependencies.Entries) != 1 || dependencies.Entries[0].ResourceKind != "workspace_target_existence" || dependencies.Entries[0].Revision != absenceHash {
				t.Fatalf("absence source: %v %+v", err, dependencies)
			}
			preimage, err := json.Marshal(struct {
				WorkspaceID   string
				OperationHash string
				Observation   struct {
					TargetRef, ContentHash string
					Size                   int64
					Purpose                string
				}
			}{"ws", runtimecontract.StableRuntimeOperationHash(operation), struct {
				TargetRef, ContentHash string
				Size                   int64
				Purpose                string
			}{"new.txt", absenceHash, 0, "patch_absence"}})
			if err != nil {
				t.Fatal(err)
			}
			selector := sha256.Sum256(append([]byte("file-patch-read-dependency/v1\x00"), preimage...))
			if dependencies.Entries[0].DependencyHash != hex.EncodeToString(selector[:]) {
				t.Fatal("absence selector differs from independent oracle")
			}
			for _, targets := range [][]string{nil, {"other.txt"}, {"new.txt", "other.txt"}} {
				changed := operation.Clone()
				changed.TargetRefs = targets
				if set, err := filePatchOwnerDependencies(scope, changed, result.patchRead); err == nil || set != nil {
					t.Fatalf("absence attached to wrong sealed target: %v", targets)
				}
			}
			empty, err := filePatchOwnerDependencies(scope, operation, observedFilePatchRead("new.txt", nil, "patch_input"))
			if err != nil || empty.SetHash == dependencies.SetHash || empty.Entries[0].ResourceKind == dependencies.Entries[0].ResourceKind {
				t.Fatal("absence confused with empty content")
			}
			request.ReplayProof = &runtimecontract.RuntimeOperationReplayProof{OwnerModuleID: ModuleID, OperationID: "create", TargetRefs: []string{"new.txt"}, AfterHash: result.AfterHash, AdmissibilitySnapshotID: "snapshot"}
			replayed, err := NewRuntimeAdapter().ApplyPatch(request)
			if err != nil || replayed.patchRead == nil || replayed.patchRead.Purpose != "replay_postcondition" {
				t.Fatalf("recovery: %v %+v", err, replayed)
			}
			recovery, err := filePatchOwnerDependencies(scope, operation, replayed.patchRead)
			if err != nil || recovery.SetHash == dependencies.SetHash || recovery.Entries[0].Revision != result.AfterHash {
				t.Fatal("recovery fabricated original absence")
			}
			for _, field := range []string{"hash", "size"} {
				changed := *result.patchRead
				if field == "hash" {
					changed.ContentHash = result.AfterHash
				} else {
					changed.Size = 1
				}
				if set, err := filePatchOwnerDependencies(scope, operation, &changed); err == nil || set != nil {
					t.Fatalf("invalid absence %s accepted", field)
				}
			}
			encoded, err := json.Marshal(dependencies)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded runtimecontract.RuntimeOwnerRevisionSet
			if err := json.Unmarshal(encoded, &reloaded); err != nil || !reflect.DeepEqual(dependencies, &reloaded) {
				t.Fatalf("absence reload: %v", err)
			}
		})
	}
}
