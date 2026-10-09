package runtimeprofiledecode

import (
	"encoding/json"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeProfileLineageAssertionShapeAndTypedDecoder(t *testing.T) {
	rule := map[string]any{"ref": "receipt", "kind": "owner_operation_receipt", "consumptionPoint": "execution", "ownerModuleId": "blocks/file", "operationKind": runtimecontract.RuntimeBlockKindPatchApply, "operationVariantId": "file_write", "requiredEffectScope": string(runtimecontract.RuntimeEffectWorkspaceDurable)}
	requirement := map[string]any{"id": "requirement-1", "scope": "dodItem", "requiredEvidenceKinds": []any{"state_change"}, "acceptedStepKinds": []any{runtimecontract.StepKindFileWrite}, "lineageAssertions": []any{rule}}
	profiles := map[string]any{"profile": map[string]any{"evidenceRequirements": []any{requirement}}}
	if err := ValidateInputShape(map[string]any{"profileContracts": profiles}); err != nil {
		t.Fatal(err)
	}
	decoded := Decode(profiles)
	want := []runtimecontract.RuntimeLineageAssertionRule{{Ref: "receipt", Kind: runtimecontract.RuntimeLineageOwnerOperationReceipt, ConsumptionPoint: runtimecontract.RuntimeProofPhaseExecution, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.RuntimeBlockKindPatchApply, OperationVariantID: "file_write", RequiredEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable}}
	if len(decoded) != 1 || len(decoded[0].EvidenceRequirements) != 1 || !reflect.DeepEqual(decoded[0].EvidenceRequirements[0].LineageAssertions, want) {
		t.Fatalf("decoder 丢失封存规则: %+v", decoded)
	}
	if err := runtimecontract.ValidateRuntimeProfileContracts(decoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"Kind", "unknown", "phase", "ConsumptionPoint"} {
		rule[field] = "unexpected"
		if err := ValidateInputShape(map[string]any{"profileContracts": profiles}); err == nil {
			t.Fatalf("未知 rule 字段被接受: %s", field)
		}
		delete(rule, field)
	}
	rule["minCompletedRounds"] = 1.5
	if err := ValidateInputShape(map[string]any{"profileContracts": profiles}); err == nil {
		t.Fatal("非整数 round 数被接受")
	}
	delete(rule, "minCompletedRounds")
	delete(rule, "consumptionPoint")
	if err := runtimecontract.ValidateRuntimeProfileContracts(Decode(profiles)); err == nil {
		t.Fatal("缺消费时点的 guard 被接受")
	}
	rule["consumptionPoint"] = "unknown"
	if err := runtimecontract.ValidateRuntimeProfileContracts(Decode(profiles)); err == nil {
		t.Fatal("未知消费时点的 guard 被接受")
	}
	rule["consumptionPoint"] = "execution"
	rule["kind"] = "unknown"
	if err := runtimecontract.ValidateRuntimeProfileContracts(Decode(profiles)); err == nil {
		t.Fatal("未知 guard 被接受")
	}
}

func TestRuntimeProfileWithoutLineageRulesPreservesExactPersistenceIdentity(t *testing.T) {
	input := map[string]any{"profile": map[string]any{"evidenceRequirements": []any{map[string]any{"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"state_change"}, "acceptedStepKinds": []any{runtimecontract.StepKindFileWrite}}}}}
	decoded := Decode(input)
	if len(decoded) != 1 || decoded[0].EvidenceRequirements[0].LineageAssertions != nil {
		t.Fatal("未声明 guard 的 Profile 不应引入会被 omitempty 丢弃的空切片身份")
	}
	data, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded []runtimecontract.RuntimeProfileContract
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, reloaded) || runtimecontract.StableRuntimeProfileContractsHash(decoded) != runtimecontract.StableRuntimeProfileContractsHash(reloaded) {
		t.Fatal("未 opt-in Profile 在 file reopen 后漂移，exact owner replay 不能改用弱对等")
	}
}
