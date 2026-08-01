package runtime

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *RuntimeController) ConsumeRecoveryBudget(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeRecoveryBudgetKind) (RuntimeAggregate, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	expectedRevision := aggregate.StateRevision
	ledger, consumed, err := consumeRecoveryBudget(aggregate.Facts.Recovery, kind, aggregate.Facts.RecoveryPolicy)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	if !consumed {
		return aggregate, false, nil
	}
	aggregate.Facts.Recovery = ledger
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, false, fmt.Errorf("runtime recovery budget: %w", err)
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, false, err
	}
	return aggregate, true, nil
}
