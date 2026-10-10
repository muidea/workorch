package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func ownerDependencyUnionFixture() (RuntimeOwnerRevisionSet, RuntimeOwnerRevisionSet) {
	file := RuntimeOwnerRevisionRef{OwnerKind: "capability", OwnerInstanceID: "file-owner", ResourceKind: "file", ResourceID: "workspace/source", Revision: "content-1", DependencyHash: "read-source"}
	recipe := RuntimeOwnerRevisionRef{OwnerKind: "capability", OwnerInstanceID: "validation-owner", ResourceKind: "recipe", ResourceID: "recipe-1", SnapshotID: "catalog-1", Fence: 2, DependencyHash: "recipe-input"}
	return (RuntimeOwnerRevisionSet{DefinitionRevision: "definition-1", Entries: []RuntimeOwnerRevisionRef{file}}).WithHash(),
		(RuntimeOwnerRevisionSet{DefinitionRevision: "definition-1", Entries: []RuntimeOwnerRevisionRef{recipe, file}}).WithHash()
}

func TestRuntimeOwnerRevisionSetUnionPreservesExactDependencies(t *testing.T) {
	first, second := ownerDependencyUnionFixture()
	before, err := json.Marshal([]RuntimeOwnerRevisionSet{first, second})
	if err != nil {
		t.Fatal(err)
	}
	var reloaded []RuntimeOwnerRevisionSet
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	for _, inputs := range [][]RuntimeOwnerRevisionSet{{first, second}, reloaded} {
		for _, order := range [][]RuntimeOwnerRevisionSet{inputs, {inputs[1], inputs[0], inputs[1]}} {
			got, err := CompileRuntimeOwnerRevisionSetUnion("definition-1", order...)
			if err != nil || !reflect.DeepEqual(got, second) {
				t.Fatalf("exact union = %+v, want %+v: %v", got, second, err)
			}
			got.Entries[0].DependencyHash = "changed"
			after, err := json.Marshal(inputs)
			if err != nil || string(after) != string(before) {
				t.Fatal("union aliased or changed source dependencies")
			}
		}
	}
	empty, err := CompileRuntimeOwnerRevisionSetUnion("definition-1")
	if err != nil || empty.Validate() != nil || len(empty.Entries) != 0 {
		t.Fatalf("empty union invented an owner dependency: %+v %v", empty, err)
	}
}

func TestRuntimeOwnerRevisionSetUnionRejectsConflictingAndAlteredSources(t *testing.T) {
	first, second := ownerDependencyUnionFixture()
	for _, change := range []struct {
		name   string
		mutate func(*RuntimeOwnerRevisionRef)
	}{
		{"revision", func(e *RuntimeOwnerRevisionRef) { e.Revision = "content-2" }},
		{"snapshot", func(e *RuntimeOwnerRevisionRef) { e.SnapshotID = "new-snapshot" }},
		{"fence", func(e *RuntimeOwnerRevisionRef) { e.Fence++ }},
		{"selector", func(e *RuntimeOwnerRevisionRef) { e.DependencyHash = "different-read" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			conflict := first.Normalized()
			change.mutate(&conflict.Entries[0])
			conflict = conflict.WithHash()
			if _, err := CompileRuntimeOwnerRevisionSetUnion("definition-1", first, conflict); err == nil {
				t.Fatal("union selected one conflicting owner dependency")
			}
		})
	}
	for _, change := range []struct {
		name   string
		mutate func(*RuntimeOwnerRevisionSet)
	}{
		{"definition", func(s *RuntimeOwnerRevisionSet) { s.DefinitionRevision = "other"; *s = s.WithHash() }},
		{"duplicate", func(s *RuntimeOwnerRevisionSet) { s.Entries = append(s.Entries, s.Entries[0]); *s = s.WithHash() }},
		{"hash", func(s *RuntimeOwnerRevisionSet) { s.SetHash = "self-reported" }},
		{"body", func(s *RuntimeOwnerRevisionSet) { s.Entries[0].Revision = "unhashed-change" }},
		{"raw whitespace", func(s *RuntimeOwnerRevisionSet) { s.Entries[0].ResourceID = " " + s.Entries[0].ResourceID }},
		{"raw order", func(s *RuntimeOwnerRevisionSet) { s.Entries[0], s.Entries[1] = s.Entries[1], s.Entries[0] }},
		{"missing owner", func(s *RuntimeOwnerRevisionSet) { s.Entries[0].OwnerInstanceID = ""; *s = s.WithHash() }},
	} {
		t.Run(change.name, func(t *testing.T) {
			invalid := second.Normalized()
			change.mutate(&invalid)
			if _, err := CompileRuntimeOwnerRevisionSetUnion("definition-1", invalid); err == nil {
				t.Fatal("union normalized away or admitted an invalid source")
			}
		})
	}
	for _, definition := range []string{"", " definition-1", "definition-1 "} {
		if _, err := CompileRuntimeOwnerRevisionSetUnion(definition, first); err == nil {
			t.Fatal("union accepted a missing or altered definition identity")
		}
	}
}

func TestRuntimeOwnerRevisionResourceKeyDoesNotAliasEmbeddedSeparators(t *testing.T) {
	first, _ := ownerDependencyUnionFixture()
	left, right := first.Entries[0], first.Entries[0]
	left.OwnerKind, left.OwnerInstanceID = "capability\x00file", "owner"
	right.OwnerKind, right.OwnerInstanceID = "capability", "file\x00owner"
	set := (RuntimeOwnerRevisionSet{DefinitionRevision: first.DefinitionRevision, Entries: []RuntimeOwnerRevisionRef{left, right}}).WithHash()
	if err := set.Validate(); err != nil {
		t.Fatalf("distinct typed resource keys collided: %v", err)
	}
	got, err := CompileRuntimeOwnerRevisionSetUnion(first.DefinitionRevision, set)
	if err != nil || len(got.Entries) != 2 {
		t.Fatalf("union collapsed distinct typed resource keys: %+v %v", got, err)
	}
	reversed := (RuntimeOwnerRevisionSet{DefinitionRevision: first.DefinitionRevision, Entries: []RuntimeOwnerRevisionRef{right, left}}).WithHash()
	if !reflect.DeepEqual(set, reversed) {
		t.Fatal("embedded separators made canonical order depend on source order")
	}
}

func FuzzRuntimeOwnerRevisionSetUnion(f *testing.F) {
	f.Add("file-owner", "workspace/file", "validation-owner", "recipe", uint64(2))
	f.Add("capability\x00file", "owner", "capability", "file\x00owner", uint64(0))
	f.Fuzz(func(t *testing.T, leftOwner, leftResource, rightOwner, rightResource string, fence uint64) {
		values := []RuntimeOwnerRevisionRef{
			{OwnerKind: "capability", OwnerInstanceID: leftOwner, ResourceKind: "fact", ResourceID: leftResource, Revision: "revision-1", Fence: fence, DependencyHash: "exact-read"},
			{OwnerKind: "capability", OwnerInstanceID: rightOwner, ResourceKind: "fact", ResourceID: rightResource, Revision: "revision-1", Fence: fence, DependencyHash: "exact-read"},
		}
		sets := make([]RuntimeOwnerRevisionSet, 0, 2)
		for _, value := range values {
			sets = append(sets, (RuntimeOwnerRevisionSet{DefinitionRevision: "definition-1", Entries: []RuntimeOwnerRevisionRef{value}}).WithHash())
		}
		// 独立 oracle 逐字段表示资源键，不调用生产 union/key helper。
		unique := make(map[[4]string]RuntimeOwnerRevisionRef)
		for _, set := range sets {
			entry := set.Entries[0]
			if strings.TrimSpace(entry.OwnerInstanceID) == "" || strings.TrimSpace(entry.ResourceID) == "" {
				if _, err := CompileRuntimeOwnerRevisionSetUnion("definition-1", sets...); err == nil {
					t.Fatal("empty owner/resource was admitted")
				}
				return
			}
			unique[[4]string{entry.OwnerKind, entry.OwnerInstanceID, entry.ResourceKind, entry.ResourceID}] = entry
		}
		expected := RuntimeOwnerRevisionSet{DefinitionRevision: "definition-1"}
		for _, entry := range unique {
			expected.Entries = append(expected.Entries, entry)
		}
		sort.Slice(expected.Entries, func(i, j int) bool {
			a, b := expected.Entries[i], expected.Entries[j]
			if a.OwnerInstanceID != b.OwnerInstanceID {
				return a.OwnerInstanceID < b.OwnerInstanceID
			}
			return a.ResourceID < b.ResourceID
		})
		expected.SetHash = stableFormalValueHash("runtime-owner-revision-set", struct {
			DefinitionRevision string
			Entries            []RuntimeOwnerRevisionRef
		}{expected.DefinitionRevision, expected.Entries})
		for _, inputs := range [][]RuntimeOwnerRevisionSet{sets, {sets[1], sets[0], sets[0]}} {
			got, err := CompileRuntimeOwnerRevisionSetUnion("definition-1", inputs...)
			if err != nil || !reflect.DeepEqual(got, expected) {
				t.Fatalf("union differs from field-wise oracle: %+v want %+v: %v", got, expected, err)
			}
		}
	})
}
