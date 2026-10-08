package memory

import (
	"reflect"
	"testing"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResultCloneDetachesOriginalTerminalMaterial(t *testing.T) {
	item := &resultmodel.Result{
		RuntimeCloseout: &runtimecontract.RuntimeResultCloseout{
			OriginalDiagnostic:   &runtimecontract.RuntimeOriginalDiagnostic{TargetRefs: []string{" 原始目标 "}},
			AcceptedEvidenceRefs: []string{},
			RequirementOutcomes:  []runtimecontract.RuntimeRequirementOutcome{{AcceptedEvidenceRefs: []string{"evidence"}}},
		},
		TerminalDelivery: &runtimecontract.RuntimeTerminalDeliveryIdentity{TerminalDeliveryID: "delivery"},
	}
	before := item.RuntimeCloseout.CloneRaw()
	copy := cloneResult(item)
	if !reflect.DeepEqual(*copy.RuntimeCloseout, before) {
		t.Fatal("仓储复制改变原值或 nil/empty 身份")
	}
	copy.RuntimeCloseout.OriginalDiagnostic.TargetRefs[0] = "changed"
	copy.RuntimeCloseout.RequirementOutcomes[0].AcceptedEvidenceRefs[0] = "changed"
	copy.TerminalDelivery.TerminalDeliveryID = "changed"
	if !reflect.DeepEqual(*item.RuntimeCloseout, before) || item.TerminalDelivery.TerminalDeliveryID != "delivery" {
		t.Fatal("仓储复制共享终态材料或 delivery identity")
	}
}
