package run

import (
	"context"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// lifecycleInternalEvent is the only owner-local SM-1 transition input. The
// EventHub adapter may validate and copy an immutable external fact, but it
// cannot select a reducer or target state directly.
type lifecycleInternalEvent struct {
	Kind        lifecycleInternalEventKind
	WorkspaceID common.WorkspaceID
	RunID       common.RunID

	Activation           *activateRunFact
	GovernanceWait       *runevents.RuntimeGovernanceWaitFact
	ContinuationEligible *runevents.RuntimeContinuationEligibleFact
	TerminalDelivery     *runevents.RuntimeTerminalDeliveryFact
	StateIncompatible    *runevents.RuntimeStateIncompatibleFact
}

type lifecycleInternalEventKind string

const (
	lifecycleInternalEventActivationObserved           lifecycleInternalEventKind = "activation_observed"
	lifecycleInternalEventGovernanceWaitObserved       lifecycleInternalEventKind = "governance_wait_observed"
	lifecycleInternalEventContinuationEligibleObserved lifecycleInternalEventKind = "continuation_eligible_observed"
	lifecycleInternalEventTerminalDeliveryObserved     lifecycleInternalEventKind = "terminal_delivery_observed"
	lifecycleInternalEventStateIncompatibleObserved    lifecycleInternalEventKind = "state_incompatible_observed"
)

func (m *Biz) submitLifecycleInternalEvent(ctx context.Context, event lifecycleInternalEvent) (runevents.RunLifecycleCommandResult, error) {
	if m == nil || event.WorkspaceID == "" || event.RunID == "" {
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	payloadCount := 0
	for _, present := range []bool{
		event.Activation != nil,
		event.GovernanceWait != nil,
		event.ContinuationEligible != nil,
		event.TerminalDelivery != nil,
		event.StateIncompatible != nil,
	} {
		if present {
			payloadCount++
		}
	}
	if payloadCount != 1 {
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, event.WorkspaceID)
	if err != nil {
		return runevents.RunLifecycleCommandResult{}, err
	}

	var (
		reduce      func(*runmodel.Run) (lifecycleTransition, error)
		deliveryCmd *runevents.RuntimeTerminalDeliveryFact
	)
	switch event.Kind {
	case lifecycleInternalEventActivationObserved:
		if event.Activation == nil {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		fact := *event.Activation
		reduce = func(current *runmodel.Run) (lifecycleTransition, error) {
			return reduceActivateRun(current, fact)
		}
	case lifecycleInternalEventGovernanceWaitObserved:
		if event.GovernanceWait == nil {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		fact := *event.GovernanceWait
		reduce = func(current *runmodel.Run) (lifecycleTransition, error) {
			return reduceGovernanceWait(current, fact)
		}
	case lifecycleInternalEventContinuationEligibleObserved:
		if event.ContinuationEligible == nil {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		fact := *event.ContinuationEligible
		reduce = func(current *runmodel.Run) (lifecycleTransition, error) {
			return reduceContinuationEligible(current, fact)
		}
	case lifecycleInternalEventTerminalDeliveryObserved:
		if event.TerminalDelivery == nil {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		fact := *event.TerminalDelivery
		deliveryCmd = &fact
		reduce = func(current *runmodel.Run) (lifecycleTransition, error) {
			return reduceTerminalDelivery(current, fact)
		}
	case lifecycleInternalEventStateIncompatibleObserved:
		if event.StateIncompatible == nil {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		fact := *event.StateIncompatible
		reduce = func(current *runmodel.Run) (lifecycleTransition, error) {
			return reduceRuntimeStateIncompatible(current, fact)
		}
	default:
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}

	return m.applyLifecycleLocked(bound, event.WorkspaceID, event.RunID, reduce, deliveryCmd)
}
