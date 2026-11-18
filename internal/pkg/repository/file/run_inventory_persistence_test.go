package file

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestColdRunInventoryRetainsOriginalBytesAndMaterializesOnlyOnAggregateRead(t *testing.T) {
	for _, indent := range []bool{false, true} {
		t.Run(map[bool]string{false: "compact", true: "indented"}[indent], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "run.json")
			original := run.Run{ID: "run", WorkspaceID: "ws", Revision: 8, WorkspaceRunGeneration: 3, State: common.RunStateFailed, RuntimeState: &rc.RuntimeStateRecord{StateRevision: 7, ProcessedEventIDs: []string{" <&\"quoted\\\n 中文 ", "second"}}}
			original.RuntimeStateHash = rc.StableRuntimeStateRecordHash(*original.RuntimeState)
			var raw []byte
			if indent {
				raw, _ = json.MarshalIndent(map[common.RunID]run.Run{original.ID: original}, "", "  ")
			} else {
				raw, _ = json.Marshal(map[common.RunID]run.Run{original.ID: original})
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			repo := NewRunRepository(path)
			facts, err := repo.ListInventory(context.Background(), "ws")
			if err != nil || len(facts) != 1 || !facts[0].HasRuntimeState || facts[0].RuntimeStateRevision != 7 {
				t.Fatalf("cold inventory: %+v %v", facts, err)
			}
			cache := mapCacheEntry(path)
			cache.mu.RLock()
			_, lazy := cache.items.(runInventoryPhysicalMap)
			cache.mu.RUnlock()
			if !lazy {
				t.Fatal("inventory decoded historical Runtime graph")
			}
			// Concurrent narrow/full readers share the original file publication lock.
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					if _, err := repo.ListInventory(context.Background(), ""); err != nil {
						t.Error(err)
					}
					if _, err := repo.GetByWorkspaceID(context.Background(), "ws", "run"); err != nil {
						t.Error(err)
					}
				})
			}
			wg.Wait()
			item, err := repo.GetByWorkspaceID(context.Background(), "ws", "run")
			if err != nil || !reflect.DeepEqual(*item.RuntimeState, *original.RuntimeState) {
				t.Fatalf("aggregate materialization: %v", err)
			}
			item.RuntimeState.ProcessedEventIDs[0] = "consumer mutation"
			next, err := repo.GetByWorkspaceID(context.Background(), "ws", "run")
			if err != nil || next.RuntimeState.ProcessedEventIDs[0] != original.RuntimeState.ProcessedEventIDs[0] {
				t.Fatal("materialized source aliased caller")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(raw, after) {
				t.Fatal("read rewrote physical source")
			}
			if foreign, err := repo.ListInventory(context.Background(), "other"); err != nil || len(foreign) != 0 {
				t.Fatal("inventory crossed workspace")
			}
		})
	}
}

func TestColdRunInventoryRejectsOriginalStateCorruptionBeforeProjection(t *testing.T) {
	for _, scenario := range []string{"hash", "body", "unknown-run-field", "truncated", "missing-state"} {
		t.Run(scenario, func(t *testing.T) {
			original := run.Run{ID: "run", WorkspaceID: "ws", RuntimeState: &rc.RuntimeStateRecord{StateRevision: 1, ProcessedEventIDs: []string{"original"}}}
			original.RuntimeStateHash = rc.StableRuntimeStateRecordHash(*original.RuntimeState)
			switch scenario {
			case "hash":
				original.RuntimeStateHash = "wrong"
			case "body":
				original.RuntimeState.ProcessedEventIDs[0] = "changed"
			case "missing-state":
				original.RuntimeState = nil
			}
			raw, _ := json.Marshal(map[common.RunID]run.Run{original.ID: original})
			if scenario == "unknown-run-field" {
				raw = bytes.Replace(raw, []byte(`"Revision":`), []byte(`"UnknownAuthority":true,"Revision":`), 1)
			}
			if scenario == "truncated" {
				raw = raw[:len(raw)-2]
			}
			path := filepath.Join(t.TempDir(), "run.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewRunRepository(path).ListInventory(context.Background(), ""); err == nil {
				t.Fatal("invalid original entered lifecycle inventory")
			}
		})
	}
}

func TestExactRunReadKeepsUnselectedHistoryOpaque(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.json")
	originals := map[common.RunID]run.Run{}
	for _, id := range []common.RunID{"selected", "history"} {
		item := run.Run{ID: id, WorkspaceID: "ws", State: common.RunStateFailed, RuntimeState: &rc.RuntimeStateRecord{StateRevision: 3}}
		item.RuntimeStateHash = rc.StableRuntimeStateRecordHash(*item.RuntimeState)
		originals[id] = item
	}
	raw, _ := json.Marshal(originals)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	repo := NewRunRepository(path)
	if _, err := repo.GetByWorkspaceID(context.Background(), "foreign", "selected"); err != common.ErrNotFound {
		t.Fatalf("scope=%v", err)
	}
	base, err := repo.GetRuntimeStateWriteBase(context.Background(), "ws", "selected")
	if err != nil || base.RuntimeStateRevision != 3 || base.Run == nil || base.Run.RuntimeState != nil {
		t.Fatalf("selected write base=%+v %v", base, err)
	}
	if _, err := repo.GetByWorkspaceID(context.Background(), "ws", "selected"); err != nil {
		t.Fatal(err)
	}
	cache := mapCacheEntry(path)
	cache.mu.RLock()
	images, ok := cache.items.(runInventoryPhysicalMap)
	if !ok || images["selected"].aggregate == nil || images["history"].aggregate != nil || len(images["history"].raw) == 0 {
		t.Error("exact read materialized historical graph")
	}
	cache.mu.RUnlock()
	items, err := repo.List(context.Background())
	if err != nil || len(items) != 2 {
		t.Fatalf("explicit full read: %d %v", len(items), err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(raw, after) {
		t.Fatal("materialization rewrote original")
	}
}

func TestCompareAndSwapRuntimeStateKeepsHistoricalRunsUnmaterializedAndByteCanonical(t *testing.T) {
	ctx := context.Background()
	for _, indentCold := range []bool{false, true} {
		name := "compact"
		if indentCold {
			name = "indented"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runs.json")
			defer EvictMapFileCache(path)

			seedRepo := NewRunRepository(path)
			hist := &run.Run{
				ID:           run.WorkspaceRunID(1),
				WorkspaceID:  "ws-history",
				State:        common.RunStateCompleted,
				RuntimeState: &rc.RuntimeStateRecord{StateRevision: 5, ProcessedEventIDs: []string{"hist <&>\n中文"}},
			}
			active := &run.Run{
				ID:           run.WorkspaceRunID(1),
				WorkspaceID:  "ws-active",
				State:        common.RunStateActive,
				RuntimeState: &rc.RuntimeStateRecord{StateRevision: 1, ProcessedEventIDs: []string{"init"}},
			}
			// Give each run a unique ID across workspaces via distinct generation indices.
			hist.ID = "run-history-1"
			active.ID = "run-active-1"
			if err := seedRepo.Create(ctx, hist); err != nil {
				t.Fatalf("create hist: %v", err)
			}
			if err := seedRepo.Create(ctx, active); err != nil {
				t.Fatalf("create active: %v", err)
			}
			histSnapshot := cloneRun(hist)

			if indentCold {
				all, err := loadRunMap(path)
				if err != nil {
					t.Fatal(err)
				}
				indented, err := json.MarshalIndent(all, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, indented, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			EvictMapFileCache(path)

			repo := NewRunRepository(path)
			wsRuns, err := repo.ListByWorkspaceID(ctx, "ws-active")
			if err != nil {
				t.Fatalf("ListByWorkspaceID: %v", err)
			}
			if len(wsRuns) != 1 || wsRuns[0].ID != active.ID {
				t.Fatalf("wsRuns = %+v, want only %q", wsRuns, active.ID)
			}

			cache := mapCacheEntry(path)
			cache.mu.RLock()
			images, ok := cache.items.(runInventoryPhysicalMap)
			if !ok || images[hist.ID].aggregate != nil || len(images[hist.ID].raw) == 0 || images[active.ID].aggregate == nil {
				cache.mu.RUnlock()
				t.Fatalf("cold ListByWorkspaceID improperly materialized history or missed active: ok=%v histAgg=%v activeAgg=%v", ok, images[hist.ID].aggregate != nil, images[active.ID].aggregate != nil)
			}
			cache.mu.RUnlock()

			current := wsRuns[0]
			for step := uint64(1); step <= 3; step++ {
				next := cloneRun(current)
				next.RuntimeState.StateRevision = step + 1
				next.RuntimeState.ProcessedEventIDs = append(next.RuntimeState.ProcessedEventIDs, string([]rune{'s', rune('0' + step)})+" <&>\n中文")
				if err := repo.CompareAndSwapRuntimeState(ctx, next, current.Revision, step); err != nil {
					t.Fatalf("CAS step %d: %v", step, err)
				}
				current = next

				cache.mu.RLock()
				images, ok = cache.items.(runInventoryPhysicalMap)
				histAggNil := images[hist.ID].aggregate == nil
				histCanonical := images[hist.ID].rawCanonical
				cache.mu.RUnlock()
				if !ok || !histAggNil || !histCanonical {
					t.Fatalf("CAS step %d unexpected cache state: physical=%v histAggNil=%v histCanonical=%v", step, ok, histAggNil, histCanonical)
				}

				gotBytes, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				expectedRef := cloneRun(current)
				expectedItems := map[common.RunID]run.Run{
					hist.ID:   *cloneRun(histSnapshot),
					active.ID: *expectedRef,
				}
				wantBytes, err := marshalRunRuntimeWrite(expectedItems, expectedRef)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(gotBytes, wantBytes) {
					t.Fatalf("CAS step %d on-disk JSON differs from canonical marshalRunRuntimeWrite (got %d bytes, want %d)", step, len(gotBytes), len(wantBytes))
				}
			}

			EvictMapFileCache(path)
			reloaded, err := NewRunRepository(path).List(ctx)
			if err != nil || len(reloaded) != 2 {
				t.Fatalf("cold List after CAS: len=%d err=%v", len(reloaded), err)
			}
			for _, item := range reloaded {
				switch item.ID {
				case hist.ID:
					if !reflect.DeepEqual(item, histSnapshot) {
						t.Fatalf("historical Run mutated:\ngot  %+v\nwant %+v", item, histSnapshot)
					}
				case active.ID:
					if !reflect.DeepEqual(item, current) {
						t.Fatalf("active Run mismatch:\ngot  %+v\nwant %+v", item, current)
					}
				default:
					t.Fatalf("unexpected Run ID %q", item.ID)
				}
			}
		})
	}
}
