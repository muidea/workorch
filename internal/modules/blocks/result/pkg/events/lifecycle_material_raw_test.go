package events

import (
	"reflect"
	"testing"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCloneResultPreservesRawCloseoutDiagnosticAndEmptyCollections(t *testing.T) {
	item := &resultmodel.Result{RuntimeCloseout: &runtimecontract.RuntimeResultCloseout{
		OriginalDiagnostic:   &runtimecontract.RuntimeOriginalDiagnostic{TargetRefs: []string{" 原始目标 "}},
		AcceptedEvidenceRefs: []string{},
		RequirementOutcomes:  []runtimecontract.RuntimeRequirementOutcome{{AcceptedEvidenceRefs: []string{}}},
	}}
	before := item.RuntimeCloseout.CloneRaw()
	cloned := CloneResult(item)
	if !reflect.DeepEqual(*cloned.RuntimeCloseout, before) {
		t.Fatal("Result 边界克隆改变材料 nil/empty 或原诊断")
	}
	cloned.RuntimeCloseout.OriginalDiagnostic.TargetRefs[0] = "changed"
	if !reflect.DeepEqual(*item.RuntimeCloseout, before) {
		t.Fatal("Result 边界共享原诊断嵌套指针")
	}
}

func TestTerminalDeliveryRejectsRawCloseoutBeforeNormalization(t *testing.T) {
	closeout := runtimecontract.RuntimeResultCloseout{
		SchemaVersion:  runtimecontract.RuntimeResultCloseoutSchemaVersionCurrent,
		RecoveryPolicy: runtimecontract.DefaultRuntimeRecoveryPolicy(),
		EvidenceRefs:   []string{"evidence"},
	}
	cmd := RuntimeTerminalDeliveryFact{
		RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{
			RuntimeTerminalDeliveryIdentity: runtimecontract.RuntimeTerminalDeliveryIdentity{
				WorkspaceID: "workspace", RunID: "run", TerminalDeliveryID: "delivery", AuthoritySnapshotID: "authority",
				DefinitionRevision: "definition", EvidenceRevision: "evidence", DispatchID: "dispatch",
				Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "sealed"},
			}, SourceOwner: "kernel/orchestrator", CausationID: "cause",
		},
		Material: ResultMaterial{WorkspaceID: "workspace", RunID: "run", GoalID: "goal", DoDID: "dod", Summary: "sealed", RuntimeCloseout: &closeout},
	}
	if err := cmd.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*runtimecontract.RuntimeResultCloseout){
		func(value *runtimecontract.RuntimeResultCloseout) { value.EvidenceRefs[0] += " " },
		func(value *runtimecontract.RuntimeResultCloseout) {
			value.EvidenceRefs = append(value.EvidenceRefs, value.EvidenceRefs[0])
		},
	} {
		invalid := cmd
		material := closeout.CloneRaw()
		mutate(&material)
		invalid.Material.RuntimeCloseout = &material
		before := material.CloneRaw()
		if err := invalid.Material.Validate(); err == nil {
			t.Fatal("材料接收边界用规范化隐藏原值损坏")
		}
		if err := invalid.Validate(); err == nil {
			t.Fatal("终态消息接收边界用规范化隐藏原值损坏")
		}
		if !reflect.DeepEqual(material, before) {
			t.Fatal("接收校验改写原材料")
		}
	}
}
