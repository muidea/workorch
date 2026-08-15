package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *driveDeliveryComponent) requestTermination(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeTerminationKind, requestedBy, reason string) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || reason == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Termination != nil {
		if aggregate.Termination.Kind == kind {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: termination already recorded as %s", common.ErrConflict, aggregate.Termination.Kind)
	}
	if isTerminalConvergenceState(aggregate.Decision.State) {
		return RuntimeAggregate{}, fmt.Errorf("%w: terminal runtime cannot accept administrative termination", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	term := runtimecontract.RuntimeTerminationRecord{
		Kind: kind, RequestedBy: strings.TrimSpace(requestedBy), Reason: strings.TrimSpace(reason),
		Revision: expectedRevision + 1, RequestedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := term.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Termination = &term
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}
