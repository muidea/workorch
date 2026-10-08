package orchestrator

import (
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeDeliveryCloseoutRepositoryCopiesAreDetached(t *testing.T) {
	closeout := &runtimecontract.RuntimeResultCloseout{
		CloseoutHash: "original-hash",
		RequirementOutcomes: []runtimecontract.RuntimeRequirementOutcome{{
			AcceptedEvidenceRefs: []string{"original-evidence"},
		}},
	}
	record := runtimecontract.RuntimeStateRecord{
		DeliveryAcknowledgements: []runtimecontract.RuntimeDeliveryAcknowledgement{{
			Owner: runtimecontract.RuntimeDeliveryOwnerResult, ResultCloseout: closeout,
		}},
	}
	aggregate := runtimeAggregateFromRecord(record)
	aggregate.DeliveryAcknowledgements[0].ResultCloseout.RequirementOutcomes[0].AcceptedEvidenceRefs[0] = "aggregate-change"
	if closeout.RequirementOutcomes[0].AcceptedEvidenceRefs[0] != "original-evidence" {
		t.Fatal("读取 opaque record 后修改 aggregate 污染原始回执")
	}
	before := aggregate.DeliveryAcknowledgements[0].CloneRaw()
	saved := runtimeStateRecordFromAggregate(aggregate)
	saved.DeliveryAcknowledgements[0].ResultCloseout.CloseoutHash = "record-change"
	if !reflect.DeepEqual(aggregate.DeliveryAcknowledgements[0], before) {
		t.Fatal("写入 opaque record 的副本与 aggregate 原材料共享指针")
	}
	for _, values := range [][]runtimecontract.RuntimeDeliveryAcknowledgement{nil, {}} {
		cloned := cloneRuntimeDeliveryAcknowledgements(values)
		if (values == nil) != (cloned == nil) || len(cloned) != 0 {
			t.Fatal("空回执集合改变 nil/empty 身份")
		}
	}
	_ = runtime.RuntimeAggregate{}
}
