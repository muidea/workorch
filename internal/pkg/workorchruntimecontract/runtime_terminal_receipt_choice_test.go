package workorchruntimecontract

import (
	"encoding/json"
	"testing"
)

func TestRuntimeTerminalReceiptChoiceClosedOwnerInventoryAndHash(t *testing.T) {
	rule := RuntimeLineageAssertionRule{Ref: "receipt-choice", Kind: RuntimeLineageTerminalOwnerReceipt, ConsumptionPoint: RuntimeProofPhasePostTerminalDelivery, TerminalContractRef: RuntimeTerminalReceiptProofContract, ReceiptSourceScope: "deliverable", ReceiptSourceID: "source_change", OwnerModuleID: RuntimeModuleFile, RequiredEffectScope: RuntimeEffectWorkspaceDurable, ReceiptChoiceContractRef: RuntimeTerminalReceiptChoiceV1, ReceiptOperationChoices: [4]RuntimeOperationVariantRef{{Kind: StepKindFileWrite}, {Kind: RuntimeBlockKindPatchApply, VariantID: "file_write"}, {Kind: RuntimeBlockKindPatchApply, VariantID: "text_replace"}, {Kind: RuntimeBlockKindPatchApply, VariantID: "unified_diff"}}}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, choice := range rule.ReceiptOperationChoices {
		if !runtimeTerminalReceiptOperationMatches(rule, choice.Kind, choice.VariantID) {
			t.Fatal("exact alternative not matched")
		}
	}
	for _, choice := range []RuntimeOperationVariantRef{{Kind: StepKindFileRead}, {Kind: RuntimeBlockKindPatchApply, VariantID: "unknown"}, {Kind: StepKindGitWrite, VariantID: "commit"}} {
		if runtimeTerminalReceiptOperationMatches(rule, choice.Kind, choice.VariantID) {
			t.Fatal("unrelated operation matched")
		}
	}
	for name, change := range map[string]func(*RuntimeLineageAssertionRule){
		"unknown version": func(r *RuntimeLineageAssertionRule) { r.ReceiptChoiceContractRef += "-unknown" },
		"wrong owner":     func(r *RuntimeLineageAssertionRule) { r.OwnerModuleID = RuntimeModuleGit },
		"mixed exact":     func(r *RuntimeLineageAssertionRule) { r.OperationKind = StepKindFileWrite },
		"execution":       func(r *RuntimeLineageAssertionRule) { r.ConsumptionPoint = RuntimeProofPhaseExecution },
		"duplicate":       func(r *RuntimeLineageAssertionRule) { r.ReceiptOperationChoices[1] = r.ReceiptOperationChoices[0] },
		"hole":            func(r *RuntimeLineageAssertionRule) { r.ReceiptOperationChoices[1] = RuntimeOperationVariantRef{} },
		"unknown variant": func(r *RuntimeLineageAssertionRule) { r.ReceiptOperationChoices[1].VariantID = "unknown" },
		"wrong effect":    func(r *RuntimeLineageAssertionRule) { r.RequiredEffectScope = RuntimeEffectNone },
	} {
		t.Run(name, func(t *testing.T) {
			copy := rule
			change(&copy)
			if copy.Validate() == nil || StableRuntimeLineageAssertionRuleHash(copy) == StableRuntimeLineageAssertionRuleHash(rule) {
				t.Fatal("unsupported choice accepted or hash unchanged")
			}
		})
	}
	raw, _ := json.Marshal(rule)
	var loaded RuntimeLineageAssertionRule
	if err := json.Unmarshal(raw, &loaded); err != nil || loaded.Validate() != nil || loaded != rule {
		t.Fatal("choice changed after reload")
	}
}

func TestRuntimeTerminalReceiptChoicesPersistedJSONCannotDropAlternatives(t *testing.T) {
	for _, raw := range []string{
		`{"ReceiptOperationChoices":[{},{},{},{},{}]}`,
		`{"receiptOperationChoices":[{},{},{},{},{}]}`,
		`{"ReceiptOperationChoices":null}`,
		`{"ReceiptOperationChoices":[]}`,
		`{"ReceiptOperationChoices":[{}]}`,
		`{"ReceiptOperationChoices":[{"Kind":"file/write","Unknown":"field"},{}]}`,
		`{"Unknown":"field"}`,
	} {
		var rule RuntimeLineageAssertionRule
		if json.Unmarshal([]byte(raw), &rule) == nil {
			t.Fatalf("lossy/unknown rule accepted: %s", raw)
		}
	}
	// Existing exact-selector rule encoding remains byte-for-byte unchanged.
	legacy := []byte(`{"Ref":"legacy","Kind":"terminal_owner_operation_receipt","ConsumptionPoint":"post_terminal_delivery","OwnerModuleID":"file","OperationKind":"file/write","RequiredEffectScope":"workspace_durable","TerminalContractRef":"runtime/terminal-receipt-proof","ReceiptSourceScope":"deliverable","ReceiptSourceID":"source_change"}`)
	var rule RuntimeLineageAssertionRule
	if err := json.Unmarshal(legacy, &rule); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(rule)
	if err != nil || string(encoded) != string(legacy) {
		t.Fatalf("legacy serialization changed: %s %v", encoded, err)
	}
}
