package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

// runtimeOperationRecoveryBudgetRequiredError is a read-only handoff from
// SM-3 execution to the SM-4 recovery-budget event. It deliberately carries no
// aggregate or mutation callback: the next event must reload the aggregate and
// own its independent CAS.
type runtimeOperationRecoveryBudgetRequiredError struct {
	OperationID string
}

type runtimeOperationOutcomeUnknownTransitionRequiredError struct {
	OperationID string
}

func (e *runtimeOperationOutcomeUnknownTransitionRequiredError) Error() string {
	if e == nil {
		return "runtime operation outcome-unknown transition is required"
	}
	return fmt.Sprintf("runtime operation %q requires an outcome-unknown transition", e.OperationID)
}

// runtimeOperationRecoveryAuthorizationClaimRequiredError is the read-only
// SM-3 handoff emitted before a recovery physical call. SM-4 must atomically
// change the exact issued authorization to attempted; only the caller that
// observes changed=true may pass the resulting one-shot token back to the next
// dispatch execution step.
type runtimeOperationRecoveryAuthorizationClaimRequiredError struct {
	OperationID     string
	AuthorizationID string
}

// runtimeOperationRecoveryInFlightError prevents another authorization while
// the exact durable replay claim may still be executing. The outer drive owner
// schedules a persisted retry for the lease deadline without consuming a
// second recovery budget.
type runtimeOperationRecoveryInFlightError struct {
	OperationID   string
	ClaimID       string
	LeaseDeadline string
}

func (e *runtimeOperationRecoveryInFlightError) Error() string {
	if e == nil {
		return "runtime operation recovery call is in flight"
	}
	return fmt.Sprintf("runtime operation %q recovery claim %q is in flight until %s", e.OperationID, e.ClaimID, e.LeaseDeadline)
}

func (e *runtimeOperationRecoveryAuthorizationClaimRequiredError) Error() string {
	if e == nil {
		return "runtime operation recovery authorization claim is required"
	}
	return fmt.Sprintf("runtime operation %q recovery authorization %q requires a pre-call claim", e.OperationID, e.AuthorizationID)
}

func (e *runtimeOperationRecoveryBudgetRequiredError) Error() string {
	if e == nil || e.OperationID == "" {
		return "runtime operation recovery budget is required"
	}
	return fmt.Sprintf("runtime operation %q requires recovery budget", e.OperationID)
}

// RuntimeOperationRecoveryBudgetRequired classifies the owner-local handoff
// without exposing a mutation entry or the concrete error type to Biz.
func RuntimeOperationRecoveryBudgetRequired(err error) (string, bool) {
	var required *runtimeOperationRecoveryBudgetRequiredError
	if !errors.As(err, &required) || required == nil || required.OperationID == "" {
		return "", false
	}
	return required.OperationID, true
}

func RuntimeOperationOutcomeUnknownTransitionRequired(err error) (string, bool) {
	var required *runtimeOperationOutcomeUnknownTransitionRequiredError
	if !errors.As(err, &required) || required == nil || required.OperationID == "" {
		return "", false
	}
	return required.OperationID, true
}

func RuntimeOperationRecoveryAuthorizationClaimRequired(err error) (string, string, bool) {
	var required *runtimeOperationRecoveryAuthorizationClaimRequiredError
	if !errors.As(err, &required) || required == nil || required.OperationID == "" || required.AuthorizationID == "" {
		return "", "", false
	}
	return required.OperationID, required.AuthorizationID, true
}

func RuntimeOperationRecoveryInFlight(err error) (string, string, string, bool) {
	var active *runtimeOperationRecoveryInFlightError
	if !errors.As(err, &active) || active == nil || active.OperationID == "" || active.ClaimID == "" || active.LeaseDeadline == "" {
		return "", "", "", false
	}
	return active.OperationID, active.ClaimID, active.LeaseDeadline, true
}

func runtimeOperationRecoveryBudgetAvailable(aggregate RuntimeAggregate) bool {
	policy := aggregate.Facts.RecoveryPolicy.Normalized()
	return aggregate.Facts.Recovery.OperationRecoveryAttempts < policy.MaxOperationRecoveryAttempts
}

func (c *candidateDispatchComponent) executeActiveDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, recoveryClaimID, recoveryNow string) (RuntimeAggregate, *RuntimeOperationExecutionCallHandoff, *RuntimeDispatchExecutionHandoff, error) {
	execution, err := prepareActiveDispatchExecution(ctx, c, workspaceID, runID, recoveryClaimID, recoveryNow)
	if err != nil {
		return RuntimeAggregate{}, nil, nil, err
	}
	return execution.run()
}
