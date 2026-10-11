package grounding

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 原完整投影路径作为独立 oracle，不能调用新实现推导期待值。
func currentFocusTargetSelectionReference(facts runtimecontract.RuntimeFacts) *runtimecontract.RuntimeTargetSelection {
	facts = facts.Normalized()
	if facts.Resources.TargetSelection == nil {
		return nil
	}
	selection := facts.Resources.TargetSelection.Normalized()
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if prerequisite.ID == selection.PrerequisiteID && prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSelection && prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus && prerequisite.FocusID == facts.Focus.ID {
			return &selection
		}
	}
	return nil
}

func focusTargetProjectionFixture() runtimecontract.RuntimeFacts {
	selection := runtimecontract.RuntimeTargetSelection{PrerequisiteID: " selection ", OwnerModuleID: " owner ", SnapshotID: " snapshot ",
		TargetRole: " delivery ", SelectorRef: " selector ", SelectorOwnerRef: " profile ", SelectorSubjectRef: " subject ",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{" b.go ", "a.go", "a.go"}, CandidateTargetRefs: []string{" b.go ", "a.go"}, SelectorCandidateRefs: []string{" c ", "c"}, CandidateCount: -1, ObservedRevision: " revision "}
	return runtimecontract.RuntimeFacts{Focus: runtimecontract.CoverageFocus{ID: " focus "}, Resources: runtimecontract.RuntimeResourceFacts{TargetSelection: &selection},
		Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{{ID: " selection ", Kind: " target_selection ", Scope: " focus ", FocusID: " focus "}}}}
}

func TestCurrentFocusTargetProjectionMatchesCompleteReference(t *testing.T) {
	for _, scenario := range []string{"matching", "absent", "empty-id", "kind", "scope", "global-focus", "focus", "duplicate-first-rejected", "duplicate-first-accepted", "unrelated-first", "empty-prerequisite", "json-reload"} {
		t.Run(scenario, func(t *testing.T) {
			facts := focusTargetProjectionFixture()
			// 使用实际 enum 文本，不让 fixture 的猜测常量形成共同错误 oracle。
			facts.Prerequisites.Satisfied[0].Kind = runtimecontract.PrerequisiteKindTargetSelection
			facts.Prerequisites.Satisfied[0].Scope = runtimecontract.RuntimePrerequisiteScopeFocus
			switch scenario {
			case "absent":
				facts.Resources.TargetSelection = nil
			case "empty-id":
				facts.Resources.TargetSelection.PrerequisiteID = " "
				facts.Prerequisites.Satisfied[0].ID = " "
			case "kind":
				facts.Prerequisites.Satisfied[0].Kind = "other"
			case "scope":
				facts.Prerequisites.Satisfied[0].Scope = "other"
			case "global-focus":
				facts.Prerequisites.Satisfied[0].Scope = ""
			case "focus":
				facts.Prerequisites.Satisfied[0].FocusID = "stale"
			case "duplicate-first-rejected":
				first := facts.Prerequisites.Satisfied[0]
				first.FocusID = "stale"
				facts.Prerequisites.Satisfied = append([]runtimecontract.RuntimePrerequisite{first}, facts.Prerequisites.Satisfied...)
			case "duplicate-first-accepted":
				last := facts.Prerequisites.Satisfied[0]
				last.Scope = "other"
				facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, last)
			case "unrelated-first":
				facts.Prerequisites.Satisfied = append([]runtimecontract.RuntimePrerequisite{{ID: "other", Kind: runtimecontract.PrerequisiteKindTargetSelection}}, facts.Prerequisites.Satisfied...)
			case "empty-prerequisite":
				facts.Prerequisites.Satisfied[0].ID = " "
			case "json-reload":
				body, _ := json.Marshal(facts)
				if err := json.Unmarshal(body, &facts); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := json.Marshal(facts)
			want, got := currentFocusTargetSelectionReference(facts), CurrentFocusTargetSelection(facts)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("完整旧路径投影不一致：got=%+v want=%+v", got, want)
			}
			after, _ := json.Marshal(facts)
			if string(before) != string(after) {
				t.Fatal("纯投影改写原 facts")
			}
			if got != nil {
				got.TargetRefs[0] = "caller"
				got.SelectorCandidateRefs[0] = "caller"
				if !reflect.DeepEqual(CurrentFocusTargetSelection(facts), want) {
					t.Fatal("返回集合共享原 owner 事实")
				}
			}
		})
	}
	facts := focusTargetProjectionFixture()
	facts.Prerequisites.Satisfied[0].Kind = runtimecontract.PrerequisiteKindTargetSelection
	facts.Prerequisites.Satisfied[0].Scope = runtimecontract.RuntimePrerequisiteScopeFocus
	if CurrentFocusTargetSelection(facts) == nil {
		t.Fatal("正例没有实际命中")
	}
	original := testing.AllocsPerRun(20, func() { _ = currentFocusTargetSelectionReference(facts) })
	current := testing.AllocsPerRun(20, func() { _ = CurrentFocusTargetSelection(facts) })
	if current >= original {
		t.Fatalf("未减少无关 facts 复制：current=%v original=%v", current, original)
	}
}

func FuzzCurrentFocusTargetProjectionReference(f *testing.F) {
	f.Add("selection", "focus", string(runtimecontract.PrerequisiteKindTargetSelection), string(runtimecontract.RuntimePrerequisiteScopeFocus), "focus", false)
	f.Add(" ", "", "other", "global", "stale", true)
	f.Fuzz(func(t *testing.T, id, focus, kind, scope, prerequisiteFocus string, duplicate bool) {
		facts := focusTargetProjectionFixture()
		facts.Focus.ID = focus
		facts.Resources.TargetSelection.PrerequisiteID = id
		facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{{ID: id, Kind: runtimecontract.PrerequisiteKind(kind), Scope: runtimecontract.RuntimePrerequisiteScope(scope), FocusID: prerequisiteFocus}}
		if duplicate {
			facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, runtimecontract.RuntimePrerequisite{ID: id, Kind: runtimecontract.PrerequisiteKindTargetSelection, Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: focus})
		}
		before, _ := json.Marshal(facts)
		if got, want := CurrentFocusTargetSelection(facts), currentFocusTargetSelectionReference(facts); !reflect.DeepEqual(got, want) {
			t.Fatalf("投影不一致：got=%+v want=%+v", got, want)
		}
		after, _ := json.Marshal(facts)
		if string(before) != string(after) {
			t.Fatal("纯投影改写原输入")
		}
	})
}

func BenchmarkCurrentFocusTargetProjection(b *testing.B) {
	facts := focusTargetProjectionFixture()
	facts.Prerequisites.Satisfied[0].Kind = runtimecontract.PrerequisiteKindTargetSelection
	facts.Prerequisites.Satisfied[0].Scope = runtimecontract.RuntimePrerequisiteScopeFocus
	for index := range 32 {
		facts.Coverage = append(facts.Coverage, runtimecontract.RuntimeCoverageItemFact{ID: common.CoverageWorkItemID(string(rune('a' + index)))})
	}
	for _, entry := range []struct {
		name    string
		project func(runtimecontract.RuntimeFacts) *runtimecontract.RuntimeTargetSelection
	}{{"reference", currentFocusTargetSelectionReference}, {"current", CurrentFocusTargetSelection}} {
		b.Run(entry.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = entry.project(facts)
			}
		})
	}
}
