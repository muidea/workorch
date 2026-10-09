package runtime

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeDeliveryLineageUsesSharedSealAndRejectsDeclarationDrift(t *testing.T) {
	// 完成谓词和 lease 仅使用同一共享 seal validator；这里检验 owner adapter 不丢来源或 seal。
	rule := runtimecontract.RuntimeLineageAssertionRule{Ref: "delivery", Kind: runtimecontract.RuntimeLineageDeliveryAcknowledgements, ConsumptionPoint: runtimecontract.RuntimeProofPhasePostTerminalDelivery}
	aggregate := stateChangeAggregateWithPreparation(t, func(initial *RuntimeAggregate) {
		item := runtimecontract.RuntimeCoverageItem{ID: "cwi-1", SourceScope: "dodItem", SourceID: "requirement-1", TargetText: "完整封存交付要求", SliceID: "slice-1"}
		initial.Facts.DefinitionSeed = runtimecontract.RuntimeDefinitionSeed{SchemaVersion: runtimecontract.RuntimeDefinitionSeedSchemaCurrent,
			DefinitionRevision: initial.Facts.DefinitionRevision, PolicyHash: "policy-test", CoverageItems: []runtimecontract.RuntimeCoverageItem{item},
			Slices: []runtimecontract.RuntimeAcceptanceSlice{{ID: item.SliceID, CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}}}}
		initial.Facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(initial.Facts.DefinitionSeed)
		initial.Facts.Coverage[0].SourceScope, initial.Facts.Coverage[0].SourceID, initial.Facts.Coverage[0].TargetText = item.SourceScope, item.SourceID, item.TargetText
		initial.Facts.Coverage[0].SliceID = item.SliceID
		initial.Facts.Slices = initial.Facts.DefinitionSeed.Slices
		initial.Facts.Focus.SliceID = item.SliceID
		initial.Facts.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
			ID: item.SourceID, Scope: item.SourceScope, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite}, LineageAssertions: []runtimecontract.RuntimeLineageAssertionRule{rule},
		}}}}
	})
	aggregate.Termination = &runtimecontract.RuntimeTerminationRecord{Kind: runtimecontract.RuntimeTerminationCancelled, RequestedBy: "blocks/run", Reason: "取消", Revision: aggregate.StateRevision}
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	bindings, err := RuntimeDeliveryLineageBindings(aggregate)
	if err != nil || len(bindings) != 1 {
		t.Fatalf("adapter 丢失 exact source: %v %+v", err, bindings)
	}
	state, err := EvaluateRuntimeDeliveryLineageAssertion(aggregate, bindings[0])
	if err != nil || state != runtimecontract.RuntimeDeliveryLineagePending {
		t.Fatalf("未封存材料应 pending: %s %v", state, err)
	}
	seal, err := terminalMaterialSealFromAggregate(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.TerminalMaterialSeal = &seal
	aggregate.StateRevision++
	before := aggregate.CloneRaw()
	state, err = EvaluateRuntimeDeliveryLineageAssertion(aggregate, bindings[0])
	if err != nil || state != runtimecontract.RuntimeDeliveryLineagePending || !reflect.DeepEqual(before, aggregate) {
		t.Fatalf("adapter 未原样使用 seal: %s %v", state, err)
	}
	changed := bindings[0]
	changed.Assertions = append([]runtimecontract.RuntimeLineageAssertion(nil), changed.Assertions...)
	changed.Assertions[0].SourceTextHash = runtimecontract.RuntimeLineageSourceTextHash("改写来源")
	if _, err := EvaluateRuntimeDeliveryLineageAssertion(aggregate, changed); err == nil {
		t.Fatal("adapter 接受了改写来源")
	}
}
