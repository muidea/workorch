package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// 保留原 string 集合转换路径，独立对照 canonical 快路径的完整输出。
func normalizeCoverageWorkItemIDsReference(values []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	stringsValue := make([]string, 0, len(values))
	for _, value := range values {
		stringsValue = append(stringsValue, string(value))
	}
	stringsValue = normalizeStringsReference(stringsValue)
	ret := make([]common.CoverageWorkItemID, 0, len(stringsValue))
	for _, value := range stringsValue {
		ret = append(ret, common.CoverageWorkItemID(value))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func normalizeCoverageWorkItemIDsPriorAllocationPath(values []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	stringsValue := make([]string, 0, len(values))
	for _, value := range values {
		stringsValue = append(stringsValue, string(value))
	}
	stringsValue = normalizeStrings(stringsValue)
	ret := make([]common.CoverageWorkItemID, 0, len(stringsValue))
	for _, value := range stringsValue {
		ret = append(ret, common.CoverageWorkItemID(value))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func assertCoverageWorkItemIDsCanonicalEquivalent(t testing.TB, values []common.CoverageWorkItemID) {
	t.Helper()
	before, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	want := normalizeCoverageWorkItemIDsReference(values)
	original := slices.Clone(values)
	for range 2 {
		got := normalizeCoverageWorkItemIDs(values)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("CWI 规范化与原算法不同: input=%q got=%q want=%q", values, got, want)
		}
		if len(got) > 0 {
			got[0] = "caller-change"
		}
	}
	after, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || !reflect.DeepEqual(values, original) {
		t.Fatal("CWI 规范化共享或改写原输入 bytes/backing array")
	}
	var reload []common.CoverageWorkItemID
	if err := json.Unmarshal(before, &reload); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalizeCoverageWorkItemIDs(reload), normalizeCoverageWorkItemIDsReference(reload)) {
		t.Fatal("CWI 重载后规范化与原算法不同")
	}
}

func TestCoverageWorkItemIDsCanonicalFastPathPreservesOriginalValues(t *testing.T) {
	for _, values := range [][]common.CoverageWorkItemID{
		nil, {}, {""}, {" "}, {"cwi/a"}, {"cwi/a", "cwi/b", "cwi/c"},
		{"cwi/b", "cwi/a"}, {"cwi/a", "cwi/a"}, {" cwi/a", "cwi/a "},
		{" cwi/a ", "cwi/b"}, {"", "cwi/a", " "}, {"中文", "目标"},
		{"a\x00b", "a\xff"}, {common.CoverageWorkItemID("a" + strings.Repeat("正文", 1024)), "z"},
	} {
		assertCoverageWorkItemIDsCanonicalEquivalent(t, values)
	}
	values := []common.CoverageWorkItemID{"cwi/a", "cwi/b", "cwi/c", "cwi/d"}
	current := testing.AllocsPerRun(20, func() { _ = normalizeCoverageWorkItemIDs(values) })
	reference := testing.AllocsPerRun(20, func() { _ = normalizeCoverageWorkItemIDsPriorAllocationPath(values) })
	if current >= reference {
		t.Fatalf("canonical CWI 集合未减少中间复制: current=%v reference=%v", current, reference)
	}
}

func FuzzCoverageWorkItemIDsCanonicalMatchesOriginalPath(f *testing.F) {
	for _, value := range []string{"", "cwi/a\x00cwi/b", "cwi/b\x00cwi/a\x00cwi/a", " cwi/a \x00 cwi/b", "中文\x00目标", "a\xff\x00b"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, encoded string) {
		var values []common.CoverageWorkItemID
		for _, value := range strings.Split(encoded, "\x00") {
			values = append(values, common.CoverageWorkItemID(value))
		}
		assertCoverageWorkItemIDsCanonicalEquivalent(t, values)
	})
}
