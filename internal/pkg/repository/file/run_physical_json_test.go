package file

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/filecommit"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// All exported fields are populated to catch generated layout drift, including
// raw invalid strings, empty collections and fields added to the Run contract.
func populateRunJSON(v reflect.Value, parents map[reflect.Type]bool) {
	if !v.CanSet() || parents[v.Type()] {
		return
	}
	parents[v.Type()] = true
	defer delete(parents, v.Type())
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			populateRunJSON(v.Field(i), parents)
		}
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		populateRunJSON(v.Elem(), parents)
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		populateRunJSON(v.Index(0), parents)
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
		k, value := reflect.New(v.Type().Key()).Elem(), reflect.New(v.Type().Elem()).Elem()
		populateRunJSON(k, parents)
		populateRunJSON(value, parents)
		v.SetMapIndex(k, value)
	case reflect.String:
		v.SetString(" original <&\"\n中文\u2028\xff ")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(7)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(9)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.25)
	}
}

func assertRunPhysicalJSON(t *testing.T, selected run.Run) {
	t.Helper()
	// Raw fixture encoding is intentionally independent of admission/fence checks.
	selected.ID = "a\n中文"
	if selected.RuntimeState != nil {
		selected.RuntimeStateHash = rc.StableRuntimeStateRecordHash(*selected.RuntimeState)
		if selected.RuntimeLifecycleFence != nil {
			fence := *selected.RuntimeLifecycleFence
			selected.RuntimeLifecycleFence = &fence
			selected.RuntimeLifecycleFence.RuntimeStateHash = selected.RuntimeStateHash
		}
	} else {
		selected.RuntimeStateHash = ""
		selected.RuntimeLifecycleFence = nil
	}
	other := selected
	other.WorkspaceID = "foreign-workspace"
	other.RuntimeState = &rc.RuntimeStateRecord{ProcessedEventIDs: []string{}}
	items := map[common.RunID]run.Run{"z<>&\xff": other, "a\n中文": selected, "nil-state": {}}
	want, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	got, err := marshalRunRuntimeWrite(items, &selected)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("raw Run map JSON differs (want %d bytes got %d)", len(want), len(got))
	}
	after, _ := json.Marshal(items)
	if !bytes.Equal(want, after) {
		t.Fatal("encoder changed original")
	}
}

func TestRunPhysicalJSONExactLayout(t *testing.T) {
	assertRunPhysicalJSON(t, run.Run{})
	var filled run.Run
	populateRunJSON(reflect.ValueOf(&filled).Elem(), map[reflect.Type]bool{})
	assertRunPhysicalJSON(t, filled)
	filled.ExecutionIntentReceipts = []run.WorkspaceExecutionIntentReceipt{}
	filled.RetiredReplacementIntents = []run.ReplacementIntent{}
	assertRunPhysicalJSON(t, filled)
	for i := 0; i < 4; i++ {
		t.Run("isolated", func(t *testing.T) { t.Parallel(); assertRunPhysicalJSON(t, filled) })
	}
}

func TestRunPhysicalJSONOtherRunEncodingFailure(t *testing.T) {
	selected := run.Run{ID: "selected", RuntimeState: &rc.RuntimeStateRecord{}}
	bad := run.Run{RuntimeState: &rc.RuntimeStateRecord{Facts: rc.RuntimeFacts{Activation: rc.RuntimeActivationContext{Memories: []rc.RuntimeMemoryContext{{VectorScore: math.NaN()}}}}}}
	if _, err := marshalRunRuntimeWrite(map[common.RunID]run.Run{"selected": selected, "other": bad}, &selected); err == nil {
		t.Fatal("unencodable unrelated state was silently omitted")
	}
}

func TestRunPhysicalJSONPublicationFailures(t *testing.T) {
	for _, published := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-rename", true: "after-rename"}[published], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runs.json")
			defer EvictMapFileCache(path)
			old := run.Run{ID: "run-001", RuntimeState: &rc.RuntimeStateRecord{StateRevision: 1}}
			oldItems := map[common.RunID]run.Run{old.ID: old}
			if err := saveMap(path, oldItems); err != nil {
				t.Fatal(err)
			}
			oldBody, _ := os.ReadFile(path)
			next := run.CloneRun(&old)
			next.RuntimeState.StateRevision++
			items := map[common.RunID]run.Run{next.ID: *next}
			body, err := marshalRunRuntimeWrite(items, next)
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("injected atomic publication failure")
			err = publishMapBytes(path, items, body, 0o644, func(path string, data []byte, mode os.FileMode) (filecommit.Outcome, error) {
				if published {
					if err := os.WriteFile(path, data, mode); err != nil {
						return filecommit.Outcome{}, err
					}
				}
				return filecommit.Outcome{Published: published}, failure
			})
			if !errors.Is(err, failure) {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want := oldBody
			if published {
				want = body
			}
			if !bytes.Equal(want, got) {
				t.Fatal("canonical image changed outside publication outcome")
			}
			loaded, err := loadMap[common.RunID, run.Run](path)
			if err != nil {
				t.Fatal(err)
			}
			wantRevision := uint64(1)
			if published {
				wantRevision++
			}
			if loaded[old.ID].RuntimeState.StateRevision != wantRevision {
				t.Fatal("cache retained wrong image")
			}
		})
	}
}

func TestRunPhysicalJSONCASColdIsolationAndTamper(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runs.json")
	defer EvictMapFileCache(path)
	repo := NewRunRepository(path)
	for i, ws := range []common.WorkspaceID{"ws-a", "ws-b"} {
		item := &run.Run{ID: run.WorkspaceRunID(uint64(i + 1)), WorkspaceID: ws, State: common.RunStateActive, RuntimeState: &rc.RuntimeStateRecord{StateRevision: 1}}
		if err := repo.Create(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	all, err := loadRunMap(path)
	if err != nil {
		t.Fatal(err)
	}
	var selected, other run.Run
	for _, v := range all {
		if v.WorkspaceID == "ws-a" {
			selected = v
		} else {
			other = v
		}
	}
	next := run.CloneRun(&selected)
	next.RuntimeState.StateRevision++
	next.RuntimeState.ProcessedEventIDs = []string{" original <&\n中文> "}
	if err := repo.CompareAndSwapRuntimeState(ctx, next, selected.Revision, 1); err != nil {
		t.Fatal(err)
	}
	// Mutating the caller and later detached reads must not alter the cache.
	next.RuntimeState.ProcessedEventIDs[0] = "caller mutation"
	for i := 0; i < 2; i++ {
		EvictMapFileCache(path)
		cold, err := NewRunRepository(path).GetByWorkspaceID(ctx, selected.WorkspaceID, selected.ID)
		if err != nil {
			t.Fatal(err)
		}
		if cold.RuntimeState.ProcessedEventIDs[0] != " original <&\n中文> " {
			t.Fatal("stored state aliases caller")
		}
		cold.RuntimeState.ProcessedEventIDs[0] = "read mutation"
	}
	untouched, err := repo.GetByWorkspaceID(ctx, other.WorkspaceID, other.ID)
	if err != nil || !reflect.DeepEqual(untouched, &other) {
		t.Fatal("other Workspace changed", err)
	}
	if err := repo.CompareAndSwapRuntimeState(ctx, run.CloneRun(&selected), selected.Revision, 1); !errors.Is(err, common.ErrConflict) {
		t.Fatal("stale write accepted", err)
	}
	items, err := loadRunMap(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := items[selected.ID]
	tampered.RuntimeState.StateRevision++
	items[selected.ID] = tampered
	// Deliberately simulate external corruption, not a normal write.
	body, _ := json.Marshal(items)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	EvictMapFileCache(path)
	if _, err := NewRunRepository(path).GetByWorkspaceID(ctx, selected.WorkspaceID, selected.ID); !errors.Is(err, common.ErrConflict) {
		t.Fatal("cold corrupted state accepted", err)
	}
}

func runPhysicalArchive(tb testing.TB) run.Run {
	tb.Helper()
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		tb.Skip("explicit fixed read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	var archive map[string]run.Run
	if err := json.Unmarshal(body, &archive); err != nil || len(archive) != 1 {
		tb.Fatalf("single-run archive: %v", err)
	}
	for _, item := range archive {
		return item
	}
	panic("empty archive")
}

func TestRunPhysicalJSONFixedArchive(t *testing.T) { assertRunPhysicalJSON(t, runPhysicalArchive(t)) }

func BenchmarkRunPhysicalEncoding(b *testing.B) {
	item := runPhysicalArchive(b)
	items := map[common.RunID]run.Run{item.ID: item}
	for _, single := range []bool{false, true} {
		name := "hash-and-map"
		if single {
			name = "same-bytes"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var err error
				if single {
					_, err = marshalRunRuntimeWrite(items, &item)
				} else {
					_, err = run.RuntimeStateWriteHash(&item)
					if err == nil {
						_, err = json.Marshal(items)
					}
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
