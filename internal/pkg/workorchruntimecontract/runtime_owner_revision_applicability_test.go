package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeOwnerRevisionApplicabilityIsExactAndResourceScoped(t *testing.T) {
	dependency, current := ownerDependencyUnionFixture()
	for _, scenario := range []string{"exact", "unrelated_change", "unrelated_removed", "missing", "revision", "snapshot", "fence", "selector", "owner", "resource"} {
		t.Run(scenario, func(t *testing.T) {
			observed := current.Normalized()
			matched := -1
			for idx, entry := range observed.Entries {
				if entry.ResourceID == dependency.Entries[0].ResourceID && entry.OwnerInstanceID == dependency.Entries[0].OwnerInstanceID {
					matched = idx
				}
			}
			if matched < 0 {
				t.Fatal("fixture 未包含实际依赖")
			}
			switch scenario {
			case "unrelated_change":
				other := 1 - matched
				observed.Entries[other].Revision = "unrelated-new-revision"
				observed.Entries[other].DependencyHash = "unrelated-new-content"
			case "unrelated_removed":
				observed.Entries = []RuntimeOwnerRevisionRef{observed.Entries[matched]}
			case "missing":
				observed.Entries = append(observed.Entries[:matched], observed.Entries[matched+1:]...)
			case "revision":
				observed.Entries[matched].Revision = "changed"
			case "snapshot":
				observed.Entries[matched].SnapshotID = "changed"
			case "fence":
				observed.Entries[matched].Fence++
			case "selector":
				observed.Entries[matched].DependencyHash = "changed"
			case "owner":
				observed.Entries[matched].OwnerInstanceID = "another-owner"
			case "resource":
				observed.Entries[matched].ResourceID = "another-resource"
			}
			observed = observed.WithHash()
			before, _ := json.Marshal([]RuntimeOwnerRevisionSet{dependency, observed})
			var reloaded []RuntimeOwnerRevisionSet
			if err := json.Unmarshal(before, &reloaded); err != nil {
				t.Fatal(err)
			}
			for _, inputs := range [][]RuntimeOwnerRevisionSet{{dependency, observed}, reloaded} {
				for range 2 {
					got, err := RuntimeOwnerRevisionSetMatchesCurrent(inputs[0], inputs[1])
					want := scenario == "exact" || scenario == "unrelated_change" || scenario == "unrelated_removed"
					if err != nil || got != want {
						t.Fatalf("逐资源适用性不符合独立场景: got=%t want=%t err=%v", got, want, err)
					}
				}
			}
			after, _ := json.Marshal([]RuntimeOwnerRevisionSet{dependency, observed})
			if string(before) != string(after) {
				t.Fatal("适用性比较改写原始集合")
			}
		})
	}
	empty, err := CompileRuntimeOwnerRevisionSetUnion(dependency.DefinitionRevision)
	if err != nil {
		t.Fatal(err)
	}
	if matched, err := RuntimeOwnerRevisionSetMatchesCurrent(empty, current); err != nil || !matched {
		t.Fatalf("空实际依赖不应借用无关 owner 状态: %t %v", matched, err)
	}
	if matched, err := RuntimeOwnerRevisionSetMatchesCurrent(dependency, empty); err != nil || matched {
		t.Fatalf("缺实际依赖不得匹配: %t %v", matched, err)
	}
}

func TestRuntimeOwnerRevisionApplicabilityRejectsInvalidSources(t *testing.T) {
	dependency, current := ownerDependencyUnionFixture()
	for _, scenario := range []string{"definition", "hash", "unhashed_body", "raw", "order", "duplicate", "conflict", "incomplete"} {
		for _, side := range []string{"dependencies", "current"} {
			t.Run(side+"/"+scenario, func(t *testing.T) {
				inputs := []RuntimeOwnerRevisionSet{dependency.Normalized(), current.Normalized()}
				idx := 0
				if side == "current" {
					idx = 1
				}
				invalid := &inputs[idx]
				switch scenario {
				case "definition":
					invalid.DefinitionRevision = "other-definition"
					*invalid = invalid.WithHash()
				case "hash":
					invalid.SetHash = "forged"
				case "unhashed_body":
					invalid.Entries[0].DependencyHash = "unhashed"
				case "raw":
					invalid.Entries[0].ResourceID = " " + invalid.Entries[0].ResourceID
				case "order":
					*invalid = current.Normalized()
					invalid.Entries[0], invalid.Entries[1] = invalid.Entries[1], invalid.Entries[0]
				case "duplicate", "conflict":
					invalid.Entries = append(invalid.Entries, invalid.Entries[0])
					if scenario == "conflict" {
						invalid.Entries[len(invalid.Entries)-1].Revision = "conflicting-revision"
					}
					*invalid = invalid.WithHash()
				case "incomplete":
					invalid.Entries[0].OwnerKind = ""
					*invalid = invalid.WithHash()
				}
				if matched, err := RuntimeOwnerRevisionSetMatchesCurrent(inputs[0], inputs[1]); err == nil || matched {
					t.Fatalf("非法输入不得以不匹配或匹配冒充合法比较: %t %v", matched, err)
				}
			})
		}
	}
}

func TestRuntimeOwnerRevisionApplicabilityDoesNotAliasEmbeddedSeparators(t *testing.T) {
	dependency, _ := ownerDependencyUnionFixture()
	dependency.Entries[0].OwnerKind, dependency.Entries[0].OwnerInstanceID = "capability\x00file", "owner"
	dependency = dependency.WithHash()
	current := dependency.Normalized()
	current.Entries[0].OwnerKind, current.Entries[0].OwnerInstanceID = "capability", "file\x00owner"
	current = current.WithHash()
	if matched, err := RuntimeOwnerRevisionSetMatchesCurrent(dependency, current); err != nil || matched {
		t.Fatalf("独立字段资源键发生 NUL 别名: %t %v", matched, err)
	}
}

func FuzzRuntimeOwnerRevisionApplicability(f *testing.F) {
	f.Add("owner", "resource", "revision", "dependency", uint64(2), true)
	f.Add("owner\x00other", "原资源", "revision", "dependency", uint64(0), false)
	f.Fuzz(func(t *testing.T, owner, resource, revision, hash string, fence uint64, mutate bool) {
		dependency := (RuntimeOwnerRevisionSet{DefinitionRevision: "definition", Entries: []RuntimeOwnerRevisionRef{{
			OwnerKind: "capability", OwnerInstanceID: owner, ResourceKind: "fact", ResourceID: resource,
			SnapshotID: "snapshot", Revision: revision, Fence: fence, DependencyHash: hash,
		}}}).WithHash()
		current := dependency.Normalized()
		if mutate {
			current.Entries[0].Fence ^= 1
		}
		current = current.WithHash()
		got, err := RuntimeOwnerRevisionSetMatchesCurrent(dependency, current)
		if dependency.Validate() != nil || current.Validate() != nil {
			if err == nil || got {
				t.Fatal("非法来源未拒绝")
			}
			return
		}
		// oracle 不使用生产资源键／matcher：逐项比较全部原字段。
		want := reflect.DeepEqual(dependency.Entries[0], current.Entries[0])
		if err != nil || got != want {
			t.Fatalf("完整字段 oracle 不等价: got=%t want=%t err=%v", got, want, err)
		}
	})
}
