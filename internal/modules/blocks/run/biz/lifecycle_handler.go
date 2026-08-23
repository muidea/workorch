package run

import (
	"context"
	"errors"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RUNTIME-SMO owner-local lifecycle command handling.
//
// External inputs never carry a target RunState. Only this owner reduces and
// persists SM-1 transitions, then publishes RunStateChangedData after CAS
// success.

const runtimeLifecycleFactSource = "kernel/orchestrator"

func (m *Biz) lifecycleTopics() []string {
	return []string{
		runevents.TopicRunFactGovernanceWait,
		runevents.TopicRunFactContinuationEligible,
		runevents.TopicRunFactTerminalDelivery,
		runevents.TopicRunFactRuntimeStateIncompatible,
	}
}

func (m *Biz) handleLifecycleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeLifecycleCommand(ev)
	setRunLifecycleResult(result, response, err)
}

func setRunLifecycleResult(result magicevent.Result, data runevents.RunLifecycleCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data.Clone(), nil)
}

func (m *Biz) executeLifecycleCommand(ev magicevent.Event) (runevents.RunLifecycleCommandResult, error) {
	if m == nil {
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil {
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	topic := ev.ID()
	data := ev.Data()

	var (
		workspaceID common.WorkspaceID
		runID       common.RunID
		internal    lifecycleInternalEvent
		boundCtx    = ctx
	)
	switch topic {
	case runevents.TopicRunFactGovernanceWait:
		cmd, ok := data.(runevents.RuntimeGovernanceWaitFact)
		if !ok || ev.Source() != runtimeLifecycleFactSource {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		var err error
		boundCtx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return runevents.RunLifecycleCommandResult{}, err
		}
		workspaceID, runID = cmd.WorkspaceID, cmd.RunID
		cmdCopy := cmd
		internal = lifecycleInternalEvent{Kind: lifecycleInternalEventGovernanceWaitObserved, WorkspaceID: workspaceID, RunID: runID, GovernanceWait: &cmdCopy}
	case runevents.TopicRunFactContinuationEligible:
		cmd, ok := data.(runevents.RuntimeContinuationEligibleFact)
		if !ok || ev.Source() != runtimeLifecycleFactSource {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		var err error
		boundCtx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return runevents.RunLifecycleCommandResult{}, err
		}
		workspaceID, runID = cmd.WorkspaceID, cmd.RunID
		cmdCopy := cmd
		internal = lifecycleInternalEvent{Kind: lifecycleInternalEventContinuationEligibleObserved, WorkspaceID: workspaceID, RunID: runID, ContinuationEligible: &cmdCopy}
	case runevents.TopicRunFactTerminalDelivery:
		cmd, ok := data.(runevents.RuntimeTerminalDeliveryFact)
		if !ok || ev.Source() != runtimeLifecycleFactSource {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		var err error
		boundCtx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return runevents.RunLifecycleCommandResult{}, err
		}
		workspaceID, runID = cmd.WorkspaceID, cmd.RunID
		cmdCopy := cmd
		internal = lifecycleInternalEvent{Kind: lifecycleInternalEventTerminalDeliveryObserved, WorkspaceID: workspaceID, RunID: runID, TerminalDelivery: &cmdCopy}
	case runevents.TopicRunFactRuntimeStateIncompatible:
		cmd, ok := data.(runevents.RuntimeStateIncompatibleFact)
		if !ok || ev.Source() != runtimeLifecycleFactSource {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		var err error
		boundCtx, err = common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
		if err != nil {
			return runevents.RunLifecycleCommandResult{}, err
		}
		workspaceID, runID = cmd.WorkspaceID, cmd.RunID
		cmdCopy := cmd
		internal = lifecycleInternalEvent{Kind: lifecycleInternalEventStateIncompatibleObserved, WorkspaceID: workspaceID, RunID: runID, StateIncompatible: &cmdCopy}
	default:
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	if workspaceID == "" || runID == "" || internal.Kind == "" {
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	response, err := m.submitLifecycleInternalEvent(boundCtx, internal)
	if err != nil {
		return runevents.RunLifecycleCommandResult{}, err
	}
	return response, nil
}

func (m *Biz) applyRunActivation(ctx context.Context, fact activateRunFact) (runevents.RunLifecycleCommandResult, error) {
	if m == nil || fact.validate() != nil {
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	factCopy := fact
	return m.submitLifecycleInternalEvent(ctx, lifecycleInternalEvent{
		Kind: lifecycleInternalEventActivationObserved, WorkspaceID: fact.WorkspaceID, RunID: fact.RunID, Activation: &factCopy,
	})
}

func (m *Biz) applyLifecycleLocked(
	ctx context.Context,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	reduce func(*runmodel.Run) (lifecycleTransition, error),
	deliveryCmd *runevents.RuntimeTerminalDeliveryFact,
) (runevents.RunLifecycleCommandResult, error) {
	// A CAS conflict is never permission to derive a new authority. Re-read the
	// same owner record and reduce the same immutable input; a duplicate
	// terminal delivery then becomes the fenced no-op acknowledgement.
	for attempt := 0; attempt < 3; attempt++ {
		current, err := m.runs.GetByWorkspaceID(ctx, workspaceID, runID)
		if err != nil {
			return runevents.RunLifecycleCommandResult{}, err
		}
		previous := current.State
		transition, err := reduce(current)
		if err != nil {
			return runevents.RunLifecycleCommandResult{}, err
		}
		if transition.Noop {
			result := runevents.RunLifecycleCommandResult{
				Run:                          current,
				Transitioned:                 false,
				PreviousState:                previous,
				RuntimeActivationDisposition: runtimeActivationDispositionForRun(current),
			}
			if deliveryCmd != nil && runStateIsTerminal(current.State) {
				result.DeliveryAck = runTerminalDeliveryAck(current, *deliveryCmd, runtimecontract.RuntimeDeliveryAcknowledgedNoop)
			}
			return result, nil
		}
		next := applyLifecycleTransition(current, transition)
		if err := m.runs.CompareAndSwap(ctx, next, current.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return runevents.RunLifecycleCommandResult{}, err
		}
		m.publishLifecycleTransition(ctx, next)
		result := runevents.RunLifecycleCommandResult{
			Run:                          next,
			Transitioned:                 true,
			PreviousState:                previous,
			RuntimeActivationDisposition: runtimeActivationDispositionForRun(next),
		}
		if deliveryCmd != nil {
			result.DeliveryAck = runTerminalDeliveryAck(next, *deliveryCmd, runtimecontract.RuntimeDeliveryAcknowledgedRunTerminal)
		}
		return result, nil
	}
	return runevents.RunLifecycleCommandResult{}, common.ErrConflict
}

func runtimeActivationDispositionForRun(run *runmodel.Run) runtimecontract.RuntimeActivationDisposition {
	if run == nil || run.RuntimeActivationStatus == nil {
		return ""
	}
	return run.RuntimeActivationStatus.Disposition
}

func runTerminalDeliveryAck(
	run *runmodel.Run,
	cmd runevents.RuntimeTerminalDeliveryFact,
	state runtimecontract.RuntimeDeliveryAcknowledgedState,
) *runtimecontract.RuntimeDeliveryAcknowledgement {
	if run == nil {
		return nil
	}
	cmd = cmd.Normalized()
	ack := runtimecontract.RuntimeDeliveryAcknowledgement{
		WorkspaceID:         run.WorkspaceID,
		RunID:               run.ID,
		TerminalDeliveryID:  cmd.TerminalDeliveryID,
		Owner:               runtimecontract.RuntimeDeliveryOwnerRun,
		AcknowledgedState:   state,
		AuthoritySnapshotID: cmd.AuthoritySnapshotID,
		CausationID:         cmd.CausationID,
		Outcome:             cmd.Outcome,
	}.Normalized()
	return &ack
}

func (m *Biz) publishLifecycleTransition(ctx context.Context, item *runmodel.Run) {
	if m == nil || m.runs == nil || m.EventHub() == nil || item == nil {
		return
	}
	data := runevents.RunStateChangedDataFromModel(item)
	// Specific topic for the transition, then the canonical state-changed fact.
	specific := notificationTopicForRunState(item.State)
	if specific != "" && specific != runevents.TopicRunStateChanged {
		event := magicevent.NewEventWithContext(specific, ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(runevents.Contracts(), event.ID(), eventcontract.LaneScope{
			SubjectType: "run", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.ID),
		}))
		m.PostEvent(event)
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunStateChanged, ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(runevents.Contracts(), event.ID(), eventcontract.LaneScope{
		SubjectType: "run", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.ID),
	}))
	m.PostEvent(event)
}

func (m *Biz) publishContinuationRequested(ctx context.Context, item *runmodel.Run, causationID string) {
	if m == nil || m.runs == nil || m.EventHub() == nil || item == nil {
		return
	}
	data := runevents.RunContinuationRequestedData{WorkspaceID: item.WorkspaceID, RunID: item.ID, CausationID: causationID, RequestID: "continuation/" + causationID}.Normalized()
	event := magicevent.NewEventWithContext(runevents.TopicRunContinuationRequested, ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(runevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "run", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.ID)}))
	m.PostEvent(event)
}
