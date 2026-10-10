package workorchprojectioncontract

import (
	"encoding/json"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCloneResultPreservesDetachedSourceCloseout(t *testing.T) {
	original := Result{RuntimeCloseout: &runtimecontract.RuntimeResultCloseout{
		CloseoutHash: "owner-closeout",
		SourceObligations: &runtimecontract.RuntimeSourceObligationInventory{
			Obligations: []runtimecontract.RuntimeSourceObligation{{ID: "B", Text: " B 原判据\r\n ", DoDItemRefs: []string{"dod"}}},
		},
		RequirementOutcomes: []runtimecontract.RuntimeRequirementOutcome{{SourceObligations: []runtimecontract.RuntimeSourceObligationOutcome{{
			Check:                runtimecontract.RuntimeSourceObligationCheck{Criterion: " B 原判据\r\n ", Declaration: " 验证 B\t "},
			AcceptedEvidenceRefs: []string{"evidence"},
			CurrentBindingRef:    &runtimecontract.RuntimeCWIBindingRef{BindingHash: "current"},
		}}}},
	}}
	before, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded Result
	if err := json.Unmarshal(before, &reloaded); err != nil {
		t.Fatal(err)
	}
	for _, source := range []Result{original, reloaded} {
		for _, cloned := range []Result{CloneResult(source), CloneResults([]Result{source})[0]} {
			if !reflect.DeepEqual(cloned, source) {
				t.Fatal("projection clone 改写 owner 已封存的 closeout")
			}
			cloned.RuntimeCloseout.SourceObligations.Obligations[0].DoDItemRefs[0] = "changed"
			cloned.RuntimeCloseout.RequirementOutcomes[0].SourceObligations[0].AcceptedEvidenceRefs[0] = "changed"
			cloned.RuntimeCloseout.RequirementOutcomes[0].SourceObligations[0].CurrentBindingRef.BindingHash = "changed"
		}
		after, err := json.Marshal(source)
		if err != nil || string(before) != string(after) {
			t.Fatal("projection clone 共享原始材料的嵌套存储")
		}
	}
	if CloneResult(Result{}).RuntimeCloseout != nil || CloneResults(nil) != nil {
		t.Fatal("projection clone 虚构缺失的服务端材料")
	}
}
