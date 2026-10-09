package runtime

import (
	"context"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestTerminalDeliveryCompletionRejectsMalformedLedger(t *testing.T) {
	aggregate := materialSealTerminalAggregate(t)
	repository := &sealTransitionRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind: RuntimeInternalEventTerminalMaterialSealRequired,
		Scope: RuntimeInternalEventScope{
			WorkspaceID: aggregate.Facts.WorkspaceID,
			RunID:       aggregate.Facts.RunID,
		},
		Delivery: &RuntimeInternalDeliveryPayload{},
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate = sealed.Aggregate
	seal := aggregate.TerminalMaterialSeal
	if seal == nil {
		t.Fatal("SM-4 seal event did not persist terminal material")
	}
	deliveryID := seal.Identity.TerminalDeliveryID
	owners := []runtimecontract.RuntimeDeliveryOwner{runtimecontract.RuntimeDeliveryOwnerResult, runtimecontract.RuntimeDeliveryOwnerWorkspace, runtimecontract.RuntimeDeliveryOwnerRun}
	for _, owner := range owners {
		ack := runtimecontract.RuntimeDeliveryAcknowledgement{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			TerminalDeliveryID: deliveryID, Owner: owner,
			AuthoritySnapshotID: seal.Identity.AuthoritySnapshotID, CausationID: "terminal-delivery/" + deliveryID,
			Outcome: seal.Identity.Outcome,
		}
		switch owner {
		case runtimecontract.RuntimeDeliveryOwnerResult:
			ack.ResultID = runtimecontract.RuntimeTerminalResultID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
			closeout := seal.Material.RuntimeCloseout.CloneRaw()
			ack.ResultCloseout = &closeout
			ack.AcknowledgedState = runtimecontract.RuntimeDeliveryAcknowledgedResultRecorded
		case runtimecontract.RuntimeDeliveryOwnerWorkspace:
			ack.AcknowledgedState = runtimecontract.RuntimeDeliveryAcknowledgedWorkspaceReleased
		case runtimecontract.RuntimeDeliveryOwnerRun:
			ack.AcknowledgedState = runtimecontract.RuntimeDeliveryAcknowledgedRunTerminal
		}
		acknowledged, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
			Kind: RuntimeInternalEventTerminalDeliveryAcknowledged,
			Scope: RuntimeInternalEventScope{
				WorkspaceID: aggregate.Facts.WorkspaceID,
				RunID:       aggregate.Facts.RunID,
			},
			Delivery: &RuntimeInternalDeliveryPayload{DeliveryAcknowledgement: &ack},
		})
		if err != nil {
			t.Fatalf("record %s acknowledgement through SM-4 event: %v", owner, err)
		}
		aggregate = acknowledged.Aggregate
	}
	if !TerminalDeliveryFullyAcknowledged(aggregate) {
		t.Fatal("合法有序三 owner 回执未完成交付")
	}
	for _, mutate := range []func(*RuntimeAggregate){
		func(value *RuntimeAggregate) {
			value.DeliveryAcknowledgements[0], value.DeliveryAcknowledgements[1] = value.DeliveryAcknowledgements[1], value.DeliveryAcknowledgements[0]
		},
		func(value *RuntimeAggregate) { value.DeliveryAcknowledgements[0].ResultID = "other-result" },
		func(value *RuntimeAggregate) { value.DeliveryAcknowledgements[2].Outcome.Code = "other-terminal" },
	} {
		invalid := aggregate
		invalid.DeliveryAcknowledgements = append([]runtimecontract.RuntimeDeliveryAcknowledgement(nil), aggregate.DeliveryAcknowledgements...)
		mutate(&invalid)
		if TerminalDeliveryFullyAcknowledged(invalid) {
			t.Fatal("完成谓词接受错序、错 ResultID 或错终态的回执账本")
		}
	}
}

func TestDeliveryAcknowledgementDuplicateMatchesExactMaterial(t *testing.T) {
	ack := runtimecontract.RuntimeDeliveryAcknowledgement{
		WorkspaceID: "workspace", RunID: "run", ResultID: "result", Owner: runtimecontract.RuntimeDeliveryOwnerResult,
		TerminalDeliveryID: "delivery", AuthoritySnapshotID: "authority", CausationID: "cause",
		AcknowledgedState: runtimecontract.RuntimeDeliveryAcknowledgedResultRecorded,
		Outcome:           runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "sealed"},
		ResultCloseout: &runtimecontract.RuntimeResultCloseout{
			CloseoutHash: "original", RequirementOutcomes: []runtimecontract.RuntimeRequirementOutcome{{AcceptedEvidenceRefs: []string{"evidence"}}},
		},
	}
	before := ack.CloneRaw()
	noop := ack.CloneRaw()
	noop.AcknowledgedState = runtimecontract.RuntimeDeliveryAcknowledgedNoop
	if !sameDeliveryAcknowledgementIdentity(ack, noop) {
		t.Fatal("同一已保存材料的 noop 重放不应冲突")
	}
	for _, mutate := range []func(*runtimecontract.RuntimeDeliveryAcknowledgement){
		func(value *runtimecontract.RuntimeDeliveryAcknowledgement) {
			value.ResultCloseout.CloseoutHash = "other"
		},
		func(value *runtimecontract.RuntimeDeliveryAcknowledgement) {
			value.ResultCloseout.RequirementOutcomes[0].AcceptedEvidenceRefs[0] = "other"
		},
		func(value *runtimecontract.RuntimeDeliveryAcknowledgement) { value.ResultCloseout = nil },
	} {
		conflict := noop.CloneRaw()
		mutate(&conflict)
		if sameDeliveryAcknowledgementIdentity(ack, conflict) {
			t.Fatal("同身份回执的材料变化未被识别为冲突")
		}
	}
	if !reflect.DeepEqual(ack, before) {
		t.Fatal("重复回执比较修改原材料")
	}
}
