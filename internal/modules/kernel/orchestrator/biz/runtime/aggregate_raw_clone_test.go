package runtime

import (
	"context"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAggregateCloneRawPreservesShapeAndDetachesPreimage(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.ProcessedEventIDs = []string{}
	aggregate.CompletedDispatches = []SealedDispatch{}
	aggregate.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{State: runtimecontract.RuntimeCandidateAttemptStatePending}}
	aggregate.ActiveDispatch = &SealedDispatch{
		CompletedOperationIDs: []string{},
		OperationExecutions:   []runtimecontract.RuntimeOperationExecutionRecord{{OperationID: "original", State: runtimecontract.OperationExecutionSealed}},
	}
	aggregate.TerminalMaterialSeal = &runtimecontract.RuntimeTerminalMaterialSeal{
		Material: runtimecontract.RuntimeTerminalResultMaterial{
			EvidenceRefs: []string{},
			RuntimeCloseout: &runtimecontract.RuntimeResultCloseout{
				OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{OriginalMessage: "  原始诊断\n  ", TargetRefs: []string{"target"}},
			},
		},
	}
	before := aggregate.CloneRaw()
	copy := aggregate.CloneRaw()
	if !reflect.DeepEqual(copy, aggregate) || copy.CompletedDispatches == nil || copy.ProcessedEventIDs == nil || copy.ActiveDispatch.CompletedOperationIDs == nil || copy.TerminalMaterialSeal.Material.EvidenceRefs == nil {
		t.Fatal("aggregate raw clone 改写 shape 或 nil/empty 身份")
	}
	copy.CandidateAttempts[0].State = runtimecontract.RuntimeCandidateAttemptStateAccepted
	copy.ActiveDispatch.OperationExecutions[0].OperationID = "changed"
	copy.Facts.Coverage[0].FocusID = "changed"
	copy.TerminalMaterialSeal.Material.RuntimeCloseout.OriginalDiagnostic.TargetRefs[0] = "changed"
	if !reflect.DeepEqual(aggregate, before) {
		t.Fatal("修改 raw clone 污染了独立前像")
	}
}

func TestDurableRuntimeRepositoryLoadAndSaveDetachMutationFromPreimage(t *testing.T) {
	original := stateChangeAggregate(t)
	repository := &durableRuntimeStateRepository{aggregate: original.CloneRaw()}
	loaded, err := repository.Load(context.Background(), original.Facts.WorkspaceID, original.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Facts.Coverage[0].FocusID = "caller-load-mutation"
	if !reflect.DeepEqual(repository.aggregate, original) {
		t.Fatal("Load 返回值通过共享 slice 改写了持久前像")
	}
	next := original.CloneRaw()
	next.StateRevision++
	if err := repository.Save(context.Background(), next, original.StateRevision); err != nil {
		t.Fatal(err)
	}
	persisted := repository.aggregate.CloneRaw()
	next.Facts.Coverage[0].FocusID = "caller-save-mutation"
	if !reflect.DeepEqual(repository.aggregate, persisted) {
		t.Fatal("Save 后的调用方 mutation 改写了持久前像")
	}
}
