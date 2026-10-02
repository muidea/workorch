package workorchruntimecontract

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func normalizeStringsReference(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func TestNormalizeStringsPreservesCanonicalDetachedValues(t *testing.T) {
	cases := [][]string{nil, {}, {""}, {" "}, {"a"}, {"a", "b"}, {"b", "a"}, {"a", "a"}, {" a", "a "}, {" a ", "b"}, {"", "a", " "}, {"中文", "目标"}}
	for _, values := range cases {
		before := append([]string(nil), values...)
		got := normalizeStrings(values)
		want := normalizeStringsReference(values)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("规范化改变: input=%q got=%q want=%q", values, got, want)
		}
		if len(got) != 0 {
			got[0] = "changed"
			if !reflect.DeepEqual(append([]string(nil), values...), before) {
				t.Fatal("规范化返回值共享输入 backing array")
			}
		}
	}
}

func FuzzNormalizeStringsMatchesReference(f *testing.F) {
	for _, seed := range []string{"", "a\x00b", "b\x00a\x00a", " a \x00 b", "中文\x00目标"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, encoded string) {
		values := strings.Split(encoded, "\x00")
		if !reflect.DeepEqual(normalizeStrings(values), normalizeStringsReference(values)) {
			t.Fatalf("规范化改变: %q", values)
		}
	})
}

func BenchmarkNormalizeStringsCanonical(b *testing.B) {
	values := []string{"binding/a", "binding/b", "binding/c", "binding/d"}
	b.ReportAllocs()
	for b.Loop() {
		normalizeStrings(values)
	}
}
