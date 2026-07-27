package events

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRunStateChangedDataFromModelIncludesRuntimeV2Progress(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
		RuntimeState: &runtimecontract.RuntimeStateRecord{
			StateRevision: 7,
			Facts: runtimecontract.RuntimeFacts{
				WorkspaceID: "ws-1", RunID: "run-1", Focus: runtimecontract.CoverageFocus{ID: "focus-003"},
				Repair: runtimecontract.RuntimeRepairLedger{RepairAttempts: 3},
			},
			Decision: runtimecontract.RuntimeDecision{
				State: runtimecontract.ConvergenceStateNeedStateChange, Action: runtimecontract.RuntimeActionProduceStateChange,
				AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-003"},
			},
		},
	}
	data := RunStateChangedDataFromModel(run)
	if data.RuntimeStateRevision != 7 || data.RuntimeConvergenceState != runtimecontract.ConvergenceStateNeedStateChange ||
		data.RuntimeAction != runtimecontract.RuntimeActionProduceStateChange || data.RuntimeFocusID != "focus-003" ||
		data.AuthoritySnapshotID != "authority-003" || data.RepairAttempts != 3 {
		t.Fatalf("runtime progress data = %+v", data)
	}
}
