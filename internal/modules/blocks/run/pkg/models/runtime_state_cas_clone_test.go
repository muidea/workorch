package run

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 保留优化前完整深拷贝算法，独立校准物理写集和规范化语义。
func referenceOpaqueRuntimeStateCAS(current, next *Run, revision uint64) error {
	if current == nil || next == nil || current.ID == "" || current.WorkspaceID == "" ||
		current.ID != next.ID || current.WorkspaceID != next.WorkspaceID || current.Revision != next.Revision ||
		current.WorkspaceRunGeneration == 0 || current.WorkspaceRunGeneration != next.WorkspaceRunGeneration || next.RuntimeState == nil {
		return common.ErrConflict
	}
	currentRevision := uint64(0)
	if current.RuntimeState != nil {
		currentRevision = current.RuntimeState.StateRevision
	}
	if currentRevision != revision || next.RuntimeState.StateRevision != revision+1 {
		return common.ErrConflict
	}
	expected := CloneRun(current)
	expected.RuntimeState = cloneOpaqueRuntimeState(next.RuntimeState)
	expected.RuntimeLifecycleFence = cloneOpaqueRuntimeLifecycleFence(next.RuntimeLifecycleFence)
	if !reflect.DeepEqual(expected, next) {
		return common.ErrConflict
	}
	return nil
}

func opaqueCASFixture() (*Run, *Run) {
	current := &Run{
		ID: "run-001", WorkspaceID: "workspace-001", Revision: 7, WorkspaceRunGeneration: 1,
		GoalID: "goal-001", DoDID: "dod-001", State: common.RunStateActive,
		RuntimeState:              &runtimecontract.RuntimeStateRecord{StateRevision: 3, ProcessedEventIDs: []string{" original ", " original "}},
		RetiredReplacementIntents: []ReplacementIntent{},
	}
	next := CloneRun(current)
	next.RuntimeState = &runtimecontract.RuntimeStateRecord{StateRevision: 4, ProcessedEventIDs: []string{}}
	return current, next
}

func TestOpaqueRuntimeStateCASCloneElisionPreservesEveryRunFieldBoundary(t *testing.T) {
	current, next := opaqueCASFixture()
	if err := ValidateOpaqueRuntimeStateCAS(current, next, 3); err != nil {
		t.Fatalf("opaque replacement rejected: %v", err)
	}
	for idx := 0; idx < reflect.TypeFor[Run]().NumField(); idx++ {
		field := reflect.TypeFor[Run]().Field(idx)
		if field.Name == "RuntimeState" || field.Name == "RuntimeLifecycleFence" {
			continue
		}
		t.Run(field.Name, func(t *testing.T) {
			current, next := opaqueCASFixture()
			value := reflect.ValueOf(next).Elem().Field(idx)
			switch value.Kind() {
			case reflect.String:
				value.SetString(value.String() + "changed")
			case reflect.Uint64:
				value.SetUint(value.Uint() + 1)
			case reflect.Pointer:
				value.Set(reflect.New(value.Type().Elem()))
			case reflect.Slice:
				value.Set(reflect.MakeSlice(value.Type(), 1, 1))
			case reflect.Struct:
				value.Field(0).SetString("changed")
			default:
				t.Fatalf("new Run field needs a boundary mutation: %s", field.Name)
			}
			if referenceOpaqueRuntimeStateCAS(current, next, 3) != common.ErrConflict ||
				ValidateOpaqueRuntimeStateCAS(current, next, 3) != common.ErrConflict {
				t.Fatalf("non-opaque Run field %s escaped the original CAS boundary", field.Name)
			}
		})
	}
}

func TestOpaqueRuntimeStateCASCloneElisionMatchesReferenceAndPreservesInputs(t *testing.T) {
	for _, variant := range []string{"replace", "first", "missing_next_state", "wrong_revision", "scope", "nil_current", "nil_next", "canonical_fence", "raw_fence"} {
		t.Run(variant, func(t *testing.T) {
			current, next := opaqueCASFixture()
			revision := uint64(3)
			switch variant {
			case "first":
				current.RuntimeState = nil
				next.RuntimeState.StateRevision = 1
				revision = 0
			case "missing_next_state":
				next.RuntimeState = nil
			case "wrong_revision":
				revision++
			case "scope":
				next.WorkspaceID = "other-workspace"
			case "nil_current":
				current = nil
			case "nil_next":
				next = nil
			case "canonical_fence", "raw_fence":
				next.RuntimeLifecycleFence = &runtimecontract.RuntimeOpaqueLifecycleFence{RuntimeStateRef: "state"}
				if variant == "raw_fence" {
					next.RuntimeLifecycleFence.RuntimeStateRef = " state "
				}
			}
			beforeCurrent, err := json.Marshal(current)
			if err != nil {
				t.Fatal(err)
			}
			beforeNext, err := json.Marshal(next)
			if err != nil {
				t.Fatal(err)
			}
			want := referenceOpaqueRuntimeStateCAS(current, next, revision)
			for range 2 {
				if got := ValidateOpaqueRuntimeStateCAS(current, next, revision); got != want {
					t.Fatalf("reference mismatch: got %v, want %v", got, want)
				}
			}
			afterCurrent, err := json.Marshal(current)
			if err != nil {
				t.Fatal(err)
			}
			afterNext, err := json.Marshal(next)
			if err != nil {
				t.Fatal(err)
			}
			if string(beforeCurrent) != string(afterCurrent) || string(beforeNext) != string(afterNext) {
				t.Fatal("CAS validation mutated raw inputs")
			}
		})
	}
}

func BenchmarkOpaqueRuntimeStateCASCloneElision(b *testing.B) {
	current, next := opaqueCASFixture()
	current.RuntimeState.ProcessedEventIDs = make([]string, 16384)
	for idx := range current.RuntimeState.ProcessedEventIDs {
		current.RuntimeState.ProcessedEventIDs[idx] = "immutable-old-ledger-entry"
	}
	for _, implementation := range []struct {
		name  string
		check func(*Run, *Run, uint64) error
	}{{"reference", referenceOpaqueRuntimeStateCAS}, {"elided", ValidateOpaqueRuntimeStateCAS}} {
		b.Run(implementation.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := implementation.check(current, next, 3); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
