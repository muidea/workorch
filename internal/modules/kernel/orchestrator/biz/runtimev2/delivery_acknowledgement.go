package runtimev2

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// recordDeliveryAcknowledgement persists an SM-4 receipt without changing the
// sealed authority/terminal decision. It is reachable only through the
// controller's private internal-event dispatcher, so no adapter or sibling
// owner can invoke a delivery-ledger transition directly. Duplicate identical
// acknowledgements are idempotent; a conflicting owner receipt is rejected.
func (c *RuntimeController) recordDeliveryAcknowledgement(ctx context.Context, acknowledgement runtimecontract.RuntimeDeliveryAcknowledgement) (RuntimeAggregate, bool, error) {
	if c == nil || c.repository == nil {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	acknowledgement = acknowledgement.Normalized()
	if err := acknowledgement.Validate(); err != nil {
		return RuntimeAggregate{}, false, err
	}
	aggregate, err := c.repository.Load(ctx, acknowledgement.WorkspaceID, acknowledgement.RunID)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	if err := validateDeliveryAcknowledgementAgainstAggregate(aggregate, acknowledgement); err != nil {
		return RuntimeAggregate{}, false, err
	}
	for _, existing := range aggregate.DeliveryAcknowledgements {
		if existing.TerminalDeliveryID == acknowledgement.TerminalDeliveryID && existing.Owner == acknowledgement.Owner {
			// A resource owner may durably apply a delivery request and lose the
			// response before SM-4 records its acknowledgement.  On replay that
			// owner correctly reports its current local state as noop instead of
			// repeating the original transition.  The delivery identity is the
			// idempotency key; the owner-local reporting state is not.
			if sameDeliveryAcknowledgementIdentity(existing, acknowledgement) {
				return aggregate, false, nil
			}
			return RuntimeAggregate{}, false, fmt.Errorf("%w: conflicting terminal delivery acknowledgement", common.ErrConflict)
		}
	}
	expected := aggregate.StateRevision
	aggregate.DeliveryAcknowledgements = append(aggregate.DeliveryAcknowledgements, acknowledgement)
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, false, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expected); err != nil {
		return RuntimeAggregate{}, false, err
	}
	return aggregate, true, nil
}

func validateDeliveryAcknowledgementAgainstAggregate(aggregate RuntimeAggregate, acknowledgement runtimecontract.RuntimeDeliveryAcknowledgement) error {
	if aggregate.Decision.Terminal == nil && aggregate.Termination == nil {
		return fmt.Errorf("%w: terminal delivery acknowledgement has no sealed terminal", common.ErrConflict)
	}
	acknowledgement = acknowledgement.Normalized()
	outcome := runtimecontract.TerminalOutcome{}
	if aggregate.Termination != nil {
		switch aggregate.Termination.Kind {
		case runtimecontract.RuntimeTerminationCancelled:
			outcome.Kind = runtimecontract.TerminalOutcomeKindCancelled
		case runtimecontract.RuntimeTerminationSuperseded:
			outcome.Kind = runtimecontract.TerminalOutcomeKindSuperseded
		default:
			return fmt.Errorf("%w: terminal delivery acknowledgement has invalid administrative kind", common.ErrConflict)
		}
		outcome.Code = "administrative_" + string(aggregate.Termination.Kind)
		outcome.Summary = aggregate.Termination.Reason
	} else {
		switch aggregate.Decision.State {
		case runtimecontract.ConvergenceStateComplete:
			outcome.Kind = runtimecontract.TerminalOutcomeKindCompleted
		case runtimecontract.ConvergenceStateFailed:
			outcome.Kind = runtimecontract.TerminalOutcomeKindFailed
		default:
			return fmt.Errorf("%w: terminal delivery acknowledgement has non-terminal convergence state", common.ErrConflict)
		}
		outcome.Code = aggregate.Decision.Terminal.Code
		outcome.Summary = aggregate.Decision.Terminal.Summary
	}
	expectedID := runtimecontract.StableTerminalDeliveryID(
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		aggregate.Decision.AuthoritySnapshot.ID,
		outcome,
	)
	if acknowledgement.TerminalDeliveryID != expectedID ||
		acknowledgement.AuthoritySnapshotID != aggregate.Decision.AuthoritySnapshot.ID ||
		acknowledgement.CausationID != "terminal-delivery/"+expectedID ||
		acknowledgement.Outcome.Normalized() != outcome.Normalized() {
		return fmt.Errorf("%w: terminal delivery acknowledgement does not match sealed terminal", common.ErrConflict)
	}
	if !deliveryOwnerPredecessorsAcknowledged(aggregate, acknowledgement.TerminalDeliveryID, acknowledgement.Owner) {
		return fmt.Errorf("%w: terminal delivery acknowledgement is out of order", common.ErrConflict)
	}
	return nil
}

func deliveryOwnerPredecessorsAcknowledged(aggregate RuntimeAggregate, deliveryID string, owner runtimecontract.RuntimeDeliveryOwner) bool {
	has := func(required runtimecontract.RuntimeDeliveryOwner) bool {
		for _, acknowledgement := range aggregate.DeliveryAcknowledgements {
			if acknowledgement.TerminalDeliveryID == deliveryID && acknowledgement.Owner == required {
				return true
			}
		}
		return false
	}
	switch owner {
	case runtimecontract.RuntimeDeliveryOwnerResult:
		return true
	case runtimecontract.RuntimeDeliveryOwnerWorkspace:
		return has(runtimecontract.RuntimeDeliveryOwnerResult)
	case runtimecontract.RuntimeDeliveryOwnerRun:
		return has(runtimecontract.RuntimeDeliveryOwnerResult) && has(runtimecontract.RuntimeDeliveryOwnerWorkspace)
	default:
		return false
	}
}

// sameDeliveryAcknowledgementIdentity deliberately excludes
// AcknowledgedState. Result-recorded/workspace-released/run-terminal and noop
// are equivalent receipts for the same already-applied owner delivery. Every
// sealed cross-owner identity field remains part of the conflict check.
func sameDeliveryAcknowledgementIdentity(left, right runtimecontract.RuntimeDeliveryAcknowledgement) bool {
	left, right = left.Normalized(), right.Normalized()
	return left.WorkspaceID == right.WorkspaceID &&
		left.RunID == right.RunID &&
		left.TerminalDeliveryID == right.TerminalDeliveryID &&
		left.Owner == right.Owner &&
		left.AuthoritySnapshotID == right.AuthoritySnapshotID &&
		left.CausationID == right.CausationID &&
		left.Outcome == right.Outcome
}
