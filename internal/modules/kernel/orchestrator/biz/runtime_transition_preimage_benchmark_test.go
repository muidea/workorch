package orchestrator

import (
	"context"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Explicit cold-audit oracle; production consumes committed owner reads.
type runtimeArchivePreimageRepository struct {
	record runtimecontract.RuntimeStateRecord
	loads  int
	saves  int
}

func (r *runtimeArchivePreimageRepository) Load(_ context.Context, _ common.WorkspaceID, _ common.RunID) (runtime.RuntimeAggregate, error) {
	r.loads++
	if err := r.record.Validate(); err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	aggregate := runtimeAggregateFromRecord(r.record)
	return aggregate, aggregate.Validate()
}

func (r *runtimeArchivePreimageRepository) Save(_ context.Context, _ runtime.RuntimeAggregate, _ uint64) error {
	r.saves++
	return nil
}

type runtimeArchiveTransactionRepository struct {
	*runtimeArchivePreimageRepository
}

func (r runtimeArchiveTransactionRepository) LoadOwnerStateOriginal(_ context.Context, _ common.WorkspaceID, _ common.RunID) (runtimecontract.RuntimeStateRecord, runtime.RuntimeAggregate, error) {
	r.loads++
	return r.record, runtimeAggregateFromRecord(r.record), nil
}
