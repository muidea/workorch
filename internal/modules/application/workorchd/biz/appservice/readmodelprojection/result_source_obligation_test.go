package readmodelprojection

import (
	"reflect"
	"testing"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResultSourceObligationProjectionPreservesOwnerCloseout(t *testing.T) {
	closeout := &runtimecontract.RuntimeResultCloseout{CloseoutHash: "exact-owner-hash", SourceObligations: &runtimecontract.RuntimeSourceObligationInventory{Obligations: []runtimecontract.RuntimeSourceObligation{{ID: "B", DoDItemRefs: []string{"dod"}, Text: " B 原文\n "}}},
		RequirementOutcomes: []runtimecontract.RuntimeRequirementOutcome{{CoverageWorkItemID: "shared", SourceObligations: []runtimecontract.RuntimeSourceObligationOutcome{{
			Check: runtimecontract.RuntimeSourceObligationCheck{ObligationID: "B", Criterion: " B 原文\n ", Declaration: " 验证 B\t "}, AcceptedEvidenceRefs: []string{"evidence"}, CurrentBindingRef: &runtimecontract.RuntimeCWIBindingRef{BindingHash: "current"},
		}}}}}
	item := &resultmodel.Result{RuntimeCloseout: closeout}
	for _, projected := range []struct {
		name     string
		closeout *runtimecontract.RuntimeResultCloseout
	}{{"map", mapResult(item).RuntimeCloseout}, {"single", projectResult(item).RuntimeCloseout}} {
		if !reflect.DeepEqual(projected.closeout, closeout) {
			t.Fatalf("%s rewrote or omitted sealed source closeout", projected.name)
		}
		projected.closeout.RequirementOutcomes[0].SourceObligations[0].AcceptedEvidenceRefs[0] = "changed"
		projected.closeout.RequirementOutcomes[0].SourceObligations[0].CurrentBindingRef.BindingHash = "changed"
		projected.closeout.SourceObligations.Obligations[0].DoDItemRefs[0] = "changed"
	}
	if closeout.RequirementOutcomes[0].SourceObligations[0].AcceptedEvidenceRefs[0] != "evidence" || closeout.RequirementOutcomes[0].SourceObligations[0].CurrentBindingRef.BindingHash != "current" || closeout.SourceObligations.Obligations[0].DoDItemRefs[0] != "dod" {
		t.Fatal("Result projection aliases persisted owner closeout")
	}
}
