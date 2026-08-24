package git

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCapabilityStateReceiptStoreIsDurableAndIdempotent(t *testing.T) {
	root := t.TempDir()
	store, err := NewCapabilityStateReceiptStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StoreIfAbsent("ws", runtimecontract.RuntimeCapabilityStateSnapshot{AcquisitionKey: "invalid"}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("incomplete capability-state snapshot err=%v, want invalid argument", err)
	}
	snapshot := runtimecontract.RuntimeCapabilityStateSnapshot{
		RequirementID: "capability_state:git", AcquisitionKey: "capability-state/ws/run/requirement/0",
		OwnerModuleID: "git", CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
		ObservedRevision: "repo-absent", ContentHash: "hash-1", Disposition: runtimecontract.RuntimeCapabilityStateReady,
		QueryReceiptRef: "capability-state-receipt/capability-state/ws/run/requirement/0",
	}
	snapshot.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(snapshot)
	workspaceID := common.WorkspaceID("ws")
	persisted, err := store.StoreIfAbsent(workspaceID, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(root, "internal", "workspaces", string(workspaceID), "runtime", "git-capability-state-receipts")
	entries, err := os.ReadDir(wantDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("workspace receipt directory entries=%d err=%v, want one durable receipt", len(entries), err)
	}
	// Reopen the same owner data directory to model process restart.
	reopened, err := NewCapabilityStateReceiptStore(root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, found, err := reopened.Load(workspaceID, snapshot.AcquisitionKey)
	if err != nil || !found || !reflect.DeepEqual(loaded, persisted) {
		t.Fatalf("durable receipt = %+v found=%v err=%v, want %+v", loaded, found, err, persisted)
	}
	changed := snapshot
	changed.ContentHash = "hash-2"
	changed.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(changed)
	if _, err := reopened.StoreIfAbsent(workspaceID, changed); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("same acquisition key with changed snapshot err=%v, want conflict", err)
	}
	again, err := reopened.StoreIfAbsent(workspaceID, snapshot)
	if err != nil || !reflect.DeepEqual(again, persisted) {
		t.Fatalf("exact duplicate acquisition was not idempotent: %+v err=%v", again, err)
	}

	parallel := snapshot
	parallel.AcquisitionKey = "capability-state/ws/run/requirement/parallel"
	parallel.QueryReceiptRef = "capability-state-receipt/" + parallel.AcquisitionKey
	parallel.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(parallel)
	parallelChanged := parallel
	parallelChanged.ContentHash = "hash-parallel-changed"
	parallelChanged.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(parallelChanged)
	secondStore, err := NewCapabilityStateReceiptStore(root)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan struct {
		snapshot runtimecontract.RuntimeCapabilityStateSnapshot
		err      error
	}, 2)
	var wait sync.WaitGroup
	for _, input := range []struct {
		store    *CapabilityStateReceiptStore
		snapshot runtimecontract.RuntimeCapabilityStateSnapshot
	}{
		{store: store, snapshot: parallel},
		{store: secondStore, snapshot: parallelChanged},
	} {
		wait.Add(1)
		go func(store *CapabilityStateReceiptStore, input runtimecontract.RuntimeCapabilityStateSnapshot) {
			defer wait.Done()
			<-start
			value, storeErr := store.StoreIfAbsent(workspaceID, input)
			results <- struct {
				snapshot runtimecontract.RuntimeCapabilityStateSnapshot
				err      error
			}{snapshot: value, err: storeErr}
		}(input.store, input.snapshot)
	}
	close(start)
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	var winner runtimecontract.RuntimeCapabilityStateSnapshot
	for result := range results {
		if result.err == nil {
			successes++
			winner = result.snapshot
			continue
		}
		if errors.Is(result.err, common.ErrConflict) {
			conflicts++
			continue
		}
		t.Fatalf("parallel acquisition failed unexpectedly: %v", result.err)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("parallel same-key acquisition successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
	loadedParallel, found, err := store.Load(workspaceID, parallel.AcquisitionKey)
	if err != nil || !found || !reflect.DeepEqual(loadedParallel, winner) {
		t.Fatalf("parallel durable winner=%+v loaded=%+v found=%t err=%v", winner, loadedParallel, found, err)
	}
}

func TestCapabilityStateReceiptStoreRejectsCrossWorkspaceScope(t *testing.T) {
	store, err := NewCapabilityStateReceiptStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := runtimecontract.RuntimeCapabilityStateSnapshot{
		RequirementID: "capability_state:git", AcquisitionKey: "capability-state/ws-a/run/requirement/0",
		OwnerModuleID: "git", CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
		ObservedRevision: "repo-absent", ContentHash: "hash-1", Disposition: runtimecontract.RuntimeCapabilityStateReady,
		QueryReceiptRef: "capability-state-receipt/capability-state/ws-a/run/requirement/0",
	}
	snapshot.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(snapshot)
	if _, err := store.StoreIfAbsent("ws-b", snapshot); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("cross-workspace StoreIfAbsent err=%v, want invalid argument", err)
	}
	if _, _, err := store.Load("../ws-a", snapshot.AcquisitionKey); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("path-like workspace Load err=%v, want invalid argument", err)
	}
}

func TestCapabilityStateReceiptStoreRejectsUnknownPersistedFields(t *testing.T) {
	root := t.TempDir()
	store, err := NewCapabilityStateReceiptStore(root)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := common.WorkspaceID("ws")
	acquisitionKey := "capability-state/ws/run/requirement/unknown"
	path, err := store.path(workspaceID, acquisitionKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"AcquisitionKey":"capability-state/ws/run/requirement/unknown","retiredRevision":"v1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load(workspaceID, acquisitionKey); err == nil || !strings.Contains(err.Error(), `unknown field "retiredRevision"`) {
		t.Fatalf("Load error = %v, want strict retired-field rejection", err)
	}
}
