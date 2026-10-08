package runtime

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type activeDispatchCountingRepository struct {
	delegate *controllerRepository
	loads    int
	saves    int
}

func (r *activeDispatchCountingRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	r.loads++
	return r.delegate.Load(ctx, workspaceID, runID)
}

func (r *activeDispatchCountingRepository) Save(ctx context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	r.saves++
	return r.delegate.Save(ctx, aggregate, expectedRevision)
}

func (r *activeDispatchCountingRepository) resetCounts() {
	r.loads = 0
	r.saves = 0
}

func TestExecuteActiveDispatchKeepsOwnerCallAndCASCounts(t *testing.T) {
	ctx := context.Background()
	repository := &activeDispatchCountingRepository{delegate: &controllerRepository{}}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := controller.initialize(ctx, controllerFacts(), controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, aggregate, runtimecontract.RuntimeCandidate{
		StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
			Operation: runtimecontract.RuntimeOperation{
				ID: "write-count", Kind: runtimecontract.StepKindFileWrite,
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
				TargetRefs:          []string{"target.txt"}, Parameters: runtimeFileWriteParameters("counted"),
			},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	repository.resetCounts()
	executor := &acceptingOperationExecutor{owner: controller}
	completed, err := controller.executeActiveDispatch(ctx, sealed.Facts.WorkspaceID, sealed.Facts.RunID, executor)
	if err != nil {
		t.Fatal(err)
	}
	if completed.ActiveDispatch != nil || executor.called != 1 {
		t.Fatalf("dispatch result active=%v owner calls=%d, want nil/1", completed.ActiveDispatch != nil, executor.called)
	}
	// 原有五次 load 仍围住 preflight、claim、owner receipt 与 evidence 投影；
	// 三次 transition CAS 各独立重载持久前像，不增加 owner effect 或 CAS 次数。
	if repository.loads != 8 || repository.saves != 3 {
		t.Fatalf("dispatch repository calls: loads=%d saves=%d, want 8/3", repository.loads, repository.saves)
	}
}
