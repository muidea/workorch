package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// 原 map／排序路径独立保留，不借用新的位集合或固定输出顺序。
func referenceSemanticTargetPolicies(values []RuntimeOperationTargetPolicy) []RuntimeOperationTargetPolicy {
	seen := map[RuntimeOperationTargetPolicy]struct{}{}
	for _, value := range values {
		value = RuntimeOperationTargetPolicy(strings.TrimSpace(string(value)))
		switch value {
		case RuntimeOperationTargetPolicyExactTarget, RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget:
			seen[value] = struct{}{}
		}
	}
	ret := make([]RuntimeOperationTargetPolicy, 0, len(seen))
	for value := range seen {
		ret = append(ret, value)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i] < ret[j] })
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func assertSemanticTargetPoliciesEquivalent(t testing.TB, values []RuntimeOperationTargetPolicy) {
	t.Helper()
	before := append([]RuntimeOperationTargetPolicy(nil), values...)
	want := referenceSemanticTargetPolicies(values)
	got := normalizeRuntimeSemanticTargetPolicies(values)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("policy normalization differs: %q != %q", got, want)
	}
	if len(values) != len(before) || (len(values) != 0 && !reflect.DeepEqual(values, before)) {
		t.Fatal("normalization changed original policy bytes")
	}
	if len(got) > 0 {
		got[0] = "caller-change"
		if !reflect.DeepEqual(normalizeRuntimeSemanticTargetPolicies(values), want) ||
			!reflect.DeepEqual(referenceSemanticTargetPolicies(values), want) {
			t.Fatal("returned policies share input or later-call state")
		}
	}
}

func TestSemanticTargetPoliciesClosedSetPreservesOriginalNormalization(t *testing.T) {
	vocabulary := []RuntimeOperationTargetPolicy{
		RuntimeOperationTargetPolicyExactTarget,
		RuntimeOperationTargetPolicyExistingOwnerScope,
		RuntimeOperationTargetPolicyProspectiveOwnerTarget,
	}
	assertSemanticTargetPoliciesEquivalent(t, nil)
	assertSemanticTargetPoliciesEquivalent(t, []RuntimeOperationTargetPolicy{})
	for mask := 0; mask < 8; mask++ {
		values := []RuntimeOperationTargetPolicy{"", "unsupported", "\x00\xff原文"}
		for index := len(vocabulary) - 1; index >= 0; index-- {
			if mask&(1<<index) != 0 {
				values = append(values, vocabulary[index], RuntimeOperationTargetPolicy(" "+string(vocabulary[index])+"\r\n"), vocabulary[index])
			}
		}
		assertSemanticTargetPoliciesEquivalent(t, values)
		body, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		var reloaded []RuntimeOperationTargetPolicy
		if err := json.Unmarshal(body, &reloaded); err != nil {
			t.Fatal(err)
		}
		assertSemanticTargetPoliciesEquivalent(t, reloaded)
	}
	values := append([]RuntimeOperationTargetPolicy(nil), vocabulary...)
	_ = normalizeRuntimeSemanticTargetPolicies(values)
	values[0] = "later-unsupported"
	assertSemanticTargetPoliciesEquivalent(t, values)
}

func TestSemanticTargetPoliciesClosedSetReducesAllocations(t *testing.T) {
	values := []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyProspectiveOwnerTarget, RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyExactTarget}
	old := testing.AllocsPerRun(100, func() { _ = referenceSemanticTargetPolicies(values) })
	current := testing.AllocsPerRun(100, func() { _ = normalizeRuntimeSemanticTargetPolicies(values) })
	if current >= old {
		t.Fatalf("closed set did not reduce allocations: %f >= %f", current, old)
	}
}

func FuzzSemanticTargetPoliciesClosedSetMatchesOriginal(f *testing.F) {
	for _, value := range []string{"", "exact_target", "existing_owner_scope\x00prospective_owner_target\x00exact_target", "  exact_target\r\n\x00\xff原文"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		values := []RuntimeOperationTargetPolicy{}
		for _, part := range strings.Split(raw, "\x00") {
			values = append(values, RuntimeOperationTargetPolicy(part))
		}
		assertSemanticTargetPoliciesEquivalent(t, values)
	})
}

func BenchmarkSemanticTargetPoliciesClosedSet(b *testing.B) {
	values := []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyProspectiveOwnerTarget, RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyExactTarget}
	for _, reference := range []bool{false, true} {
		name := "closed_set"
		if reference {
			name = "reference"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if reference {
					_ = referenceSemanticTargetPolicies(values)
				} else {
					_ = normalizeRuntimeSemanticTargetPolicies(values)
				}
			}
		})
	}
}
