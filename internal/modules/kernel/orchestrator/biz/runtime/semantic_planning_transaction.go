package runtime

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// semanticPlanningMutationTransaction is a short-lived value owned by one
// private event handler. It is not retained by a component and cannot execute
// arbitrary mutation callbacks; each handler must apply its concrete typed
// mutation before explicitly committing this transaction.
type semanticPlanningMutationTransaction struct {
	aggregate        RuntimeAggregate
	ledger           runtimecontract.RuntimeSemanticPlanningLedger
	expectedRevision uint64
}

func (c *planningTransactionComponent) beginSemanticPlanningMutation(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (semanticPlanningMutationTransaction, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return semanticPlanningMutationTransaction{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return semanticPlanningMutationTransaction{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return semanticPlanningMutationTransaction{}, err
	}
	if aggregate.Facts.WorkspaceID != workspaceID || aggregate.Facts.RunID != runID {
		return semanticPlanningMutationTransaction{}, fmt.Errorf("%w: runtime semantic planning scope mismatch", common.ErrConflict)
	}
	return semanticPlanningMutationTransaction{
		aggregate: aggregate, ledger: aggregate.SemanticPlanning.Clone(), expectedRevision: aggregate.StateRevision,
	}, nil
}

func (c *planningTransactionComponent) commitSemanticPlanningMutation(ctx context.Context, transaction semanticPlanningMutationTransaction, changed bool) (RuntimeAggregate, error) {
	if c == nil || c.transitions == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if !changed {
		return transaction.aggregate, nil
	}
	if err := transaction.ledger.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	transaction.aggregate.SemanticPlanning = transaction.ledger
	transaction.aggregate.StateRevision++
	// 同一未变 after-image 交由唯一 CAS coordinator 完整校验；不保留或传递校验成功凭据。
	if err := c.transitions.Commit(ctx, transaction.aggregate, transaction.expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return transaction.aggregate, nil
}
