package runtimeprofiledecode

import (
	"encoding/json"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeProfileConfirmedBindingRepairVersionDecoder(t *testing.T) {
	rule := map[string]any{"ref": "original-repair", "kind": "repair_then_fresh_validation", "consumptionPoint": "execution", "repairHistoryContractRef": runtimecontract.RuntimeConfirmedBindingRepairProofV1}
	profiles := map[string]any{"profile": map[string]any{"evidenceRequirements": []any{map[string]any{"id": "dod-1", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{runtimecontract.RuntimeBlockKindManagedValidationSession}, "lineageAssertions": []any{rule}}}}}
	if err := ValidateInputShape(map[string]any{"profileContracts": profiles}); err != nil {
		t.Fatal(err)
	}
	decoded := Decode(profiles)
	if runtimecontract.ValidateRuntimeProfileContracts(decoded) != nil || decoded[0].EvidenceRequirements[0].LineageAssertions[0].RepairHistoryContractRef != runtimecontract.RuntimeConfirmedBindingRepairProofV1 {
		t.Fatal("decoder lost confirmed repair identity")
	}
	for _, value := range []any{nil, 42, []any{}, map[string]any{}} {
		rule["repairHistoryContractRef"] = value
		if ValidateInputShape(map[string]any{"profileContracts": profiles}) == nil {
			t.Fatal("invalid repair contract type accepted")
		}
	}
	rule["repairHistoryContractRef"] = "runtime/confirmed-binding-repair-proof/v0"
	if runtimecontract.ValidateRuntimeProfileContracts(Decode(profiles)) == nil {
		t.Fatal("unknown repair version accepted")
	}
	delete(rule, "repairHistoryContractRef")
	if runtimecontract.ValidateRuntimeProfileContracts(Decode(profiles)) != nil || Decode(profiles)[0].EvidenceRequirements[0].LineageAssertions[0].RepairHistoryContractRef != "" {
		t.Fatal("legacy implicitly opted in")
	}
}

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

func TestRuntimeProfileTerminalProofVersionAndExactSourceDecoder(t *testing.T) {
	rule := map[string]any{"ref": "original-receipt", "kind": "terminal_owner_operation_receipt", "consumptionPoint": "post_terminal_delivery", "terminalContractRef": runtimecontract.RuntimeTerminalReceiptProofContract, "receiptSourceScope": "deliverable", "receiptSourceId": "final-material", "ownerModuleId": "blocks/file", "operationKind": runtimecontract.RuntimeBlockKindPatchApply, "operationVariantId": "file_write", "requiredEffectScope": string(runtimecontract.RuntimeEffectWorkspaceDurable)}
	requirement := map[string]any{"id": "original-dod", "scope": "dodItem", "requiredEvidenceKinds": []any{"state_change"}, "acceptedStepKinds": []any{runtimecontract.StepKindFileWrite}, "lineageAssertions": []any{rule}}
	profiles := map[string]any{"profile": map[string]any{"evidenceRequirements": []any{requirement}}}
	if err := ValidateInputShape(map[string]any{"profileContracts": profiles}); err != nil {
		t.Fatal(err)
	}
	decoded := Decode(profiles)
	if err := runtimecontract.ValidateRuntimeProfileContracts(decoded); err != nil {
		t.Fatal(err)
	}
	got := decoded[0].EvidenceRequirements[0].LineageAssertions[0]
	if got.TerminalContractRef != runtimecontract.RuntimeTerminalReceiptProofContract || got.ReceiptSourceScope != "deliverable" || got.ReceiptSourceID != "final-material" {
		t.Fatal("typed decoder lost version/source identity")
	}
	for _, field := range []string{"terminalContractRef", "receiptSourceScope", "receiptSourceId"} {
		prior := rule[field]
		for _, invalid := range []any{nil, 42, []any{}, map[string]any{}} {
			rule[field] = invalid
			if ValidateInputShape(map[string]any{"profileContracts": profiles}) == nil {
				t.Fatalf("terminal extension type accepted: %s=%v", field, invalid)
			}
		}
		delete(rule, field)
		if runtimecontract.ValidateRuntimeProfileContracts(Decode(profiles)) == nil {
			t.Fatalf("missing terminal identity accepted: %s", field)
		}
		rule[field] = prior
	}
}

func TestRuntimeProfileTerminalSnapshotExactDualSourceDecoder(t *testing.T) {
	rule := map[string]any{"ref": "original-snapshot", "kind": "terminal_owner_snapshot", "consumptionPoint": "post_terminal_delivery", "terminalContractRef": runtimecontract.RuntimeTerminalSnapshotProofContract, "receiptSourceScope": "deliverable", "receiptSourceId": "repository", "validationSourceScope": "dodItem", "validationSourceId": "behavior", "ownerModuleId": "blocks/git", "operationKind": runtimecontract.StepKindGitWrite, "operationVariantId": "commit", "requiredEffectScope": string(runtimecontract.RuntimeEffectWorkspaceDurable)}
	requirement := map[string]any{"id": "original-dod", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{runtimecontract.RuntimeBlockKindManagedValidationSession}, "lineageAssertions": []any{rule}}
	profiles := map[string]any{"profile": map[string]any{"evidenceRequirements": []any{requirement}}}
	if err := ValidateInputShape(map[string]any{"profileContracts": profiles}); err != nil {
		t.Fatal(err)
	}
	decoded := Decode(profiles)
	if err := runtimecontract.ValidateRuntimeProfileContracts(decoded); err != nil {
		t.Fatal(err)
	}
	got := decoded[0].EvidenceRequirements[0].LineageAssertions[0]
	if got.ValidationSourceScope != "dodItem" || got.ValidationSourceID != "behavior" || got.TerminalContractRef != runtimecontract.RuntimeTerminalSnapshotProofContract {
		t.Fatal("dual exact source lost")
	}
	for _, field := range []string{"validationSourceScope", "validationSourceId"} {
		original := rule[field]
		for _, invalid := range []any{nil, 42, []any{}, map[string]any{}} {
			rule[field] = invalid
			if ValidateInputShape(map[string]any{"profileContracts": profiles}) == nil {
				t.Fatal("snapshot null/type extension accepted")
			}
		}
		delete(rule, field)
		if runtimecontract.ValidateRuntimeProfileContracts(Decode(profiles)) == nil {
			t.Fatal("snapshot missing source accepted")
		}
		rule[field] = original
	}
}

func TestRuntimeProfileCrossSourceExecutionVersionDecoder(t *testing.T) {
	rule := map[string]any{"ref": "original-work", "kind": "completed_work_rounds", "consumptionPoint": "execution", "minCompletedRounds": 2, "executionSourceContractRef": runtimecontract.RuntimeDoDSourceExecutionProof}
	profiles := map[string]any{"profile": map[string]any{"evidenceRequirements": []any{map[string]any{"id": "dod-001", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{runtimecontract.RuntimeBlockKindManagedValidationSession}, "lineageAssertions": []any{rule}}}}}
	if err := ValidateInputShape(map[string]any{"profileContracts": profiles}); err != nil {
		t.Fatal(err)
	}
	decoded := Decode(profiles)
	if err := runtimecontract.ValidateRuntimeProfileContracts(decoded); err != nil {
		t.Fatal(err)
	}
	if decoded[0].EvidenceRequirements[0].LineageAssertions[0].ExecutionSourceContractRef != runtimecontract.RuntimeDoDSourceExecutionProof {
		t.Fatal("decoder lost opt-in source contract")
	}
	for _, value := range []any{nil, 1, []any{}, map[string]any{}} {
		rule["executionSourceContractRef"] = value
		if ValidateInputShape(map[string]any{"profileContracts": profiles}) == nil {
			t.Fatal("invalid opt-in field type accepted")
		}
	}
	rule["executionSourceContractRef"] = "runtime/cross-source-execution-proof/v0"
	if runtimecontract.ValidateRuntimeProfileContracts(Decode(profiles)) == nil {
		t.Fatal("unsupported source contract version accepted")
	}
	rule["executionSourceContractRef"] = runtimecontract.RuntimeDoDSourceExecutionProof
	rule["consumptionPoint"] = "post_terminal_delivery"
	if runtimecontract.ValidateRuntimeProfileContracts(Decode(profiles)) == nil {
		t.Fatal("execution mapping escaped consumption phase")
	}
}

func TestRuntimeProfileReceiptChoicesRejectTruncationAndShapeDrift(t *testing.T) {
	choices := []any{map[string]any{"kind": "file/write"}, map[string]any{"kind": "patch/apply", "variantId": "text_replace"}}
	rule := map[string]any{"receiptChoiceContractRef": runtimecontract.RuntimeTerminalReceiptChoiceV1, "receiptOperationChoices": choices}
	if err := ValidateObject(rule, "lineageAssertion", "rule"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []any{nil, []any{}, choices[:1], append(append([]any{}, choices...), choices[0], choices[1], choices[0]), []any{choices[0], map[string]any{"kind": "file/write", "unknown": "field"}}} {
		rule["receiptOperationChoices"] = invalid
		if ValidateObject(rule, "lineageAssertion", "rule") == nil {
			t.Fatal("invalid raw operation list accepted", invalid)
		}
	}
}
