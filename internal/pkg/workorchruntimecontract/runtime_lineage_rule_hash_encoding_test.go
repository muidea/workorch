package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func runtimeLineageRuleHashEncodingReference(rule RuntimeLineageAssertionRule) string {
	payload, _ := json.Marshal(rule)
	digest := sha256.Sum256(append([]byte("runtime-lineage-rule/v1\x00"), payload...))
	return hex.EncodeToString(digest[:])
}

func TestLineageRuleHashPreservesCompleteOriginalEncoding(t *testing.T) {
	values := []RuntimeLineageAssertionRule{
		{},
		{Ref: "原始规则\r\n\x00\xff", Kind: RuntimeLineageOwnerOperationReceipt, ConsumptionPoint: RuntimeProofPhaseExecution, OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite, OperationVariantID: "原始variant", RequiredEffectScope: RuntimeEffectWorkspaceDurable},
		{Ref: strings.Repeat("长规则", 8192), Kind: RuntimeLineageCompletedWorkRounds, ConsumptionPoint: RuntimeProofPhaseExecution, MinCompletedRounds: 2},
	}
	for _, rule := range values {
		before := rule
		want := runtimeLineageRuleHashEncodingReference(rule)
		for range 2 {
			if got := StableRuntimeLineageAssertionRuleHash(rule); got != want {
				t.Fatalf("原 lineage rule 完整编码漂移：got=%s want=%s", got, want)
			}
		}
		if !reflect.DeepEqual(before, rule) {
			t.Fatal("hash 改写原始规则")
		}
		body, err := json.Marshal(rule)
		if err != nil {
			t.Fatal(err)
		}
		var reloaded RuntimeLineageAssertionRule
		if err := json.Unmarshal(body, &reloaded); err != nil {
			t.Fatal(err)
		}
		if got := StableRuntimeLineageAssertionRuleHash(reloaded); got != runtimeLineageRuleHashEncodingReference(reloaded) || utf8.ValidString(rule.Ref) && got != want {
			t.Fatal("JSON 重载改变完整规则编码")
		}
	}
	base := RuntimeLineageAssertionRule{Ref: "rule", Kind: RuntimeLineageOwnerOperationReceipt, ConsumptionPoint: RuntimeProofPhaseExecution, OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite, RequiredEffectScope: RuntimeEffectWorkspaceDurable}
	for _, scenario := range []string{"ref", "kind", "phase", "rounds", "owner", "operation", "variant", "effect"} {
		changed := base
		switch scenario {
		case "ref":
			changed.Ref += "changed"
		case "kind":
			changed.Kind = RuntimeLineageCompletedWorkRounds
		case "phase":
			changed.ConsumptionPoint = RuntimeProofPhasePostTerminalDelivery
		case "rounds":
			changed.MinCompletedRounds = 2
		case "owner":
			changed.OwnerModuleID = RuntimeModuleShell
		case "operation":
			changed.OperationKind = StepKindFileEdit
		case "variant":
			changed.OperationVariantID = "create"
		case "effect":
			changed.RequiredEffectScope = RuntimeEffectNone
		}
		if StableRuntimeLineageAssertionRuleHash(changed) == StableRuntimeLineageAssertionRuleHash(base) || StableRuntimeLineageAssertionRuleHash(changed) != runtimeLineageRuleHashEncodingReference(changed) {
			t.Fatalf("字段变动没有按原编码重新绑定：%s", scenario)
		}
	}
}

func FuzzLineageRuleHashEncoding(f *testing.F) {
	for _, value := range []string{"", "中文\r\n\x00", "invalid\xff", strings.Repeat("x", 1024)} {
		f.Add(value, value, value, value, value, value, value, 2)
	}
	f.Fuzz(func(t *testing.T, ref, kind, phase, owner, operation, variant, effect string, rounds int) {
		rule := RuntimeLineageAssertionRule{Ref: ref, Kind: RuntimeLineageAssertionKind(kind), ConsumptionPoint: RuntimeProofPhase(phase), MinCompletedRounds: rounds, OwnerModuleID: owner, OperationKind: operation, OperationVariantID: variant, RequiredEffectScope: RuntimeEffectScope(effect)}
		if got, want := StableRuntimeLineageAssertionRuleHash(rule), runtimeLineageRuleHashEncodingReference(rule); got != want {
			t.Fatalf("任意原始规则编码漂移：got=%s want=%s", got, want)
		}
	})
}

var runtimeLineageRuleHashEncodingSink string

func BenchmarkLineageRuleHashEncoding(b *testing.B) {
	for _, size := range []struct {
		name string
		ref  string
	}{{"bounded", "owner-receipt"}, {"long", strings.Repeat("完整原文", 8192)}} {
		rule := RuntimeLineageAssertionRule{Ref: size.ref, Kind: RuntimeLineageOwnerOperationReceipt, ConsumptionPoint: RuntimeProofPhaseExecution, OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite, RequiredEffectScope: RuntimeEffectWorkspaceDurable}
		for _, implementation := range []struct {
			name string
			hash func(RuntimeLineageAssertionRule) string
		}{{"reference", runtimeLineageRuleHashEncodingReference}, {"current", StableRuntimeLineageAssertionRuleHash}} {
			b.Run(size.name+"/"+implementation.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					runtimeLineageRuleHashEncodingSink = implementation.hash(rule)
				}
			})
		}
	}
}
