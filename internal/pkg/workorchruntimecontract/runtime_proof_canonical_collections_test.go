package workorchruntimecontract

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func runtimeEffectScopesNormalizeReference(values []RuntimeEffectScope) []RuntimeEffectScope {
	if len(values) == 0 {
		return nil
	}
	raw := make([]string, 0, len(values))
	for _, value := range values {
		scope := NormalizeRuntimeEffectScope(value)
		if scope != "" {
			raw = append(raw, string(scope))
		}
	}
	raw = normalizeStrings(raw)
	if len(raw) == 0 {
		return nil
	}
	ret := make([]RuntimeEffectScope, len(raw))
	for index, value := range raw {
		ret[index] = RuntimeEffectScope(value)
	}
	return ret
}

func runtimeOperationVariantsNormalizeReference(values []RuntimeOperationVariantRef) []RuntimeOperationVariantRef {
	byKey := map[string]RuntimeOperationVariantRef{}
	for _, value := range values {
		value.Kind = CanonicalRuntimeBlockKind(value.Kind)
		value.VariantID = strings.TrimSpace(value.VariantID)
		if value.Kind != "" && value.VariantID != "" {
			byKey[value.Kind+"|"+value.VariantID] = value
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeOperationVariantRef, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byKey[key])
	}
	return ret
}

func TestRuntimeProofCanonicalCollectionsMatchReferenceAndDetach(t *testing.T) {
	for _, values := range [][]RuntimeEffectScope{nil, {}, {RuntimeEffectEphemeralSession}, {RuntimeEffectNone, RuntimeEffectRemoteDurable}, {"", "bad", RuntimeEffectUnknown}, {" none ", RuntimeEffectNone, RuntimeEffectWorkspaceDurable}, {RuntimeEffectWorkspaceDurable, RuntimeEffectNone}} {
		before := append([]RuntimeEffectScope(nil), values...)
		got := NormalizeRuntimeEffectScopes(values)
		if !reflect.DeepEqual(got, runtimeEffectScopesNormalizeReference(values)) {
			t.Fatal("effect scopes 改变 reference 语义")
		}
		if len(got) > 0 {
			got[0] = "changed"
		}
		if !reflect.DeepEqual(before, append([]RuntimeEffectScope(nil), values...)) {
			t.Fatal("effect scopes 返回别名")
		}
	}
	for _, values := range [][]RuntimeOperationVariantRef{nil, {}, {{Kind: StepKindFileWrite, VariantID: "a"}}, {{Kind: StepKindFileWrite, VariantID: "a"}, {Kind: StepKindFileWrite, VariantID: "b"}}, {{Kind: " file/write ", VariantID: " a "}, {Kind: StepKindFileWrite, VariantID: "a"}}, {{Kind: StepKindFileWrite, VariantID: "b"}, {Kind: StepKindFileWrite, VariantID: "a"}}, {{}, {Kind: "unknown", VariantID: "a|b"}}} {
		before := append([]RuntimeOperationVariantRef(nil), values...)
		got := NormalizeRuntimeOperationVariantRefs(values)
		if !reflect.DeepEqual(got, runtimeOperationVariantsNormalizeReference(values)) {
			t.Fatal("operation variants 改变顺序、去重或空值 reference 语义")
		}
		if len(got) > 0 {
			got[0].VariantID = "changed"
		}
		if !reflect.DeepEqual(before, append([]RuntimeOperationVariantRef(nil), values...)) {
			t.Fatal("operation variants 返回别名")
		}
	}
}

func FuzzRuntimeProofCanonicalCollections(f *testing.F) {
	for _, value := range []string{"", "none", " workspace_durable ", "file/write", "unknown|a"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		scopes := []RuntimeEffectScope{RuntimeEffectScope(value), RuntimeEffectNone, RuntimeEffectScope(value)}
		if !reflect.DeepEqual(NormalizeRuntimeEffectScopes(scopes), runtimeEffectScopesNormalizeReference(scopes)) {
			t.Fatal("effect reference 不等价")
		}
		variants := []RuntimeOperationVariantRef{{Kind: value, VariantID: value}, {Kind: StepKindFileWrite, VariantID: value}}
		if !reflect.DeepEqual(NormalizeRuntimeOperationVariantRefs(variants), runtimeOperationVariantsNormalizeReference(variants)) {
			t.Fatal("variant reference 不等价")
		}
	})
}

func BenchmarkRuntimeProofCanonicalCollections(b *testing.B) {
	for _, test := range []struct {
		name      string
		normalize func([]RuntimeEffectScope) []RuntimeEffectScope
	}{{"effect_reference", runtimeEffectScopesNormalizeReference}, {"effect_canonical", NormalizeRuntimeEffectScopes}} {
		b.Run(test.name, func(b *testing.B) {
			values := []RuntimeEffectScope{RuntimeEffectEphemeralSession}
			b.ReportAllocs()
			for b.Loop() {
				test.normalize(values)
			}
		})
	}
}
