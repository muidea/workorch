package governance

import (
	"context"
	"errors"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type staticGovernanceTraceInventory struct {
	items     []*governancemodel.AutoGovernanceTrace
	lastQuery workorchquerycontract.AutoGovernanceTraceQuery
}

func (r *staticGovernanceTraceInventory) Create(context.Context, *governancemodel.AutoGovernanceTrace) error {
	return common.ErrInvalidArgument
}

func (r *staticGovernanceTraceInventory) Upsert(context.Context, *governancemodel.AutoGovernanceTrace) error {
	return common.ErrInvalidArgument
}

func (r *staticGovernanceTraceInventory) Get(context.Context, common.AutoGovernanceTraceID) (*governancemodel.AutoGovernanceTrace, error) {
	return nil, common.ErrNotFound
}

func (r *staticGovernanceTraceInventory) List(_ context.Context, query workorchquerycontract.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	r.lastQuery = query
	return r.items, nil
}

func (r *staticGovernanceTraceInventory) Delete(context.Context, common.AutoGovernanceTraceID) error {
	return common.ErrInvalidArgument
}

func TestGovernanceRecoveryInventoriesHaveExplicitTotalLimit(t *testing.T) {
	traceInventory := &staticGovernanceTraceInventory{items: make([]*governancemodel.AutoGovernanceTrace, governanceRecoveryInventoryMaxItems+1)}
	traceService := &Service{traces: traceInventory}
	if err := traceService.reconcileIncompleteTraces(context.Background()); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("trace recovery error = %v, want inventory limit conflict", err)
	}
	if traceInventory.lastQuery.Limit != governanceRecoveryInventoryMaxItems+1 {
		t.Fatalf("trace inventory query limit = %d, want %d", traceInventory.lastQuery.Limit, governanceRecoveryInventoryMaxItems+1)
	}

	runService := &Service{runs: replayGovernanceRunReader{items: make([]*runmodel.Run, governanceRecoveryInventoryMaxItems+1)}}
	if err := runService.reconcileBlockedRuns(context.Background()); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Run recovery error = %v, want inventory limit conflict", err)
	}
}

func TestGovernanceRecoveryInventoriesRejectInvalidOwnerFacts(t *testing.T) {
	traceService := &Service{traces: &staticGovernanceTraceInventory{items: []*governancemodel.AutoGovernanceTrace{nil}}}
	if err := traceService.reconcileIncompleteTraces(context.Background()); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("trace recovery error = %v, want invalid fact conflict", err)
	}

	runService := &Service{runs: replayGovernanceRunReader{items: []*runmodel.Run{nil}}}
	if err := runService.reconcileBlockedRuns(context.Background()); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Run recovery error = %v, want invalid fact conflict", err)
	}
}
