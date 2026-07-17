package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMapAcceptanceProjectionReadsPersistedRuntimeFacts(t *testing.T) {
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		Focus:  runtimecontract.CoverageFocus{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		Slices: []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"}}},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{
			{ID: "cwi-1", SliceID: "slice-1", State: runtimecontract.RuntimeCoverageItemStateSatisfied},
			{ID: "cwi-2", SliceID: "slice-1", State: runtimecontract.RuntimeCoverageItemStateOpen},
		},
		ConvergencePolicy: runtimecontract.DefaultRuntimeConvergencePolicy(),
		Repair:            runtimecontract.RuntimeRepairLedger{FocusID: "focus-1", SliceID: "slice-1", RepairAttempts: 2},
	}, Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedValidation, Action: runtimecontract.RuntimeActionProduceValidation}}
	projection := MapAcceptanceProjection(readmodelcore.Run{ID: "run-1", RuntimeState: &state})
	if projection.ActiveSliceID != "slice-1" || projection.CurrentFocus == nil || projection.CurrentFocus.ID != "focus-1" || projection.Budget == nil || projection.Budget.RepairAttempts != 2 {
		t.Fatalf("acceptance projection = %+v", projection)
	}
	if len(projection.Slices) != 1 || projection.Slices[0].State != "active" {
		t.Fatalf("slice projection = %+v", projection.Slices)
	}
}
