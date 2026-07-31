package run

import (
	"context"
	"errors"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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
		runevents.TopicRunCommandActivate,
		runevents.TopicRunCommandContinue,
		runevents.TopicRunCommandApplyGovernanceWait,
		runevents.TopicRunCommandApplyContinuationEligible,
		runevents.TopicRunCommandApplyTerminalDelivery,
		runevents.TopicRunCommandApplyRuntimeStateIncompatible,
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
	result.Set(data, nil)
}

func (m *Biz) executeLifecycleCommand(ev magicevent.Event) (runevents.RunLifecycleCommandResult, error) {
	if m == nil {
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil || m.Service == nil {
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	topic := ev.ID()
	data := ev.Data()

	var (
		workspaceID common.WorkspaceID
		runID       common.RunID
		reduce      func(*runmodel.Run) (lifecycleTransition, error)
		deliveryCmd *runevents.ApplyRuntimeTerminalDeliveryCommand
	)
	switch topic {
	case runevents.TopicRunCommandActivate:
		cmd, ok := data.(runevents.ActivateRunCommand)
		if !ok {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		workspaceID, runID = cmd.WorkspaceID, cmd.RunID
		reduce = func(current *runmodel.Run) (lifecycleTransition, error) {
			return reduceActivateRun(current, cmd)
		}
	case runevents.TopicRunCommandContinue:
		cmd, ok := data.(runevents.ContinueRunCommand)
		if !ok {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		workspaceID, runID = cmd.WorkspaceID, cmd.RunID
		reduce = func(current *runmodel.Run) (lifecycleTransition, error) {
			return reduceContinueRun(current, cmd)
		}
	case runevents.TopicRunCommandApplyGovernanceWait:
		cmd, ok := data.(runevents.ApplyRuntimeGovernanceWaitCommand)
		if !ok || ev.Source() != runtimeLifecycleFactSource {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		workspaceID, runID = cmd.WorkspaceID, cmd.RunID
		reduce = func(current *runmodel.Run) (lifecycleTransition, error) {
			return reduceGovernanceWait(current, cmd)
		}
	case runevents.TopicRunCommandApplyContinuationEligible:
		cmd, ok := data.(runevents.ApplyRuntimeContinuationEligibleCommand)
		if !ok || ev.Source() != runtimeLifecycleFactSource {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		workspaceID, runID = cmd.WorkspaceID, cmd.RunID
		reduce = func(current *runmodel.Run) (lifecycleTransition, error) {
			return reduceContinuationEligible(current, cmd)
		}
	case runevents.TopicRunCommandApplyTerminalDelivery:
		cmd, ok := data.(runevents.ApplyRuntimeTerminalDeliveryCommand)
		if !ok || ev.Source() != runtimeLifecycleFactSource {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		workspaceID, runID = cmd.WorkspaceID, cmd.RunID
		cmdCopy := cmd
		deliveryCmd = &cmdCopy
		reduce = func(current *runmodel.Run) (lifecycleTransition, error) {
			return reduceTerminalDelivery(current, cmd)
		}
	case runevents.TopicRunCommandApplyRuntimeStateIncompatible:
		cmd, ok := data.(runevents.ApplyRuntimeStateIncompatibleFactCommand)
		if !ok || ev.Source() != runtimeLifecycleFactSource {
			return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		workspaceID, runID = cmd.WorkspaceID, cmd.RunID
		reduce = func(current *runmodel.Run) (lifecycleTransition, error) {
			return reduceRuntimeStateIncompatible(current, cmd)
		}
	default:
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	if workspaceID == "" || runID == "" || reduce == nil {
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	var scopeErr error
	ctx, scopeErr = common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if scopeErr != nil {
		return runevents.RunLifecycleCommandResult{}, scopeErr
	}
	response, err := m.applyLifecycleLocked(ctx, workspaceID, runID, reduce, deliveryCmd)
	if err != nil {
		return response, err
	}
	if topic == runevents.TopicRunCommandContinue && !response.Transitioned && response.Run != nil && response.Run.State == common.RunStateActive && response.RuntimeActivationDisposition != runmodel.RuntimeActivationDispositionFreshActivationRequired {
		m.publishContinuationRequested(ctx, response.Run, data.(runevents.ContinueRunCommand).CausationID)
	}
	return response, nil
}

func (m *Biz) applyLifecycleLocked(
	ctx context.Context,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	reduce func(*runmodel.Run) (lifecycleTransition, error),
	deliveryCmd *runevents.ApplyRuntimeTerminalDeliveryCommand,
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
				Diagnostic:                   transition.Diagnostic,
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

func runtimeActivationDispositionForRun(run *runmodel.Run) runmodel.RuntimeActivationDisposition {
	if run == nil || run.RuntimeActivationStatus == nil {
		return ""
	}
	return run.RuntimeActivationStatus.Disposition
}

func runTerminalDeliveryAck(
	run *runmodel.Run,
	cmd runevents.ApplyRuntimeTerminalDeliveryCommand,
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
	if m == nil || m.Service == nil || m.EventHub() == nil || item == nil {
		return
	}
	data := runevents.RunStateChangedDataFromModel(item)
	// Specific topic for the transition, then the canonical state-changed fact.
	specific := notificationTopicForRunState(item.State)
	if specific != "" && specific != runevents.TopicRunStateChanged {
		event := magicevent.NewEventWithContext(specific, ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
		event.BindLaneKey(workorchevents.EventHubLaneKeyFor(event.ID(), workorchevents.EventHubLaneScope{
			SubjectType: "run", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.ID),
		}))
		m.PostEvent(event)
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunStateChanged, ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(event.ID(), workorchevents.EventHubLaneScope{
		SubjectType: "run", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.ID),
	}))
	m.PostEvent(event)
}

func (m *Biz) publishContinuationRequested(ctx context.Context, item *runmodel.Run, causationID string) {
	if m == nil || m.Service == nil || m.EventHub() == nil || item == nil {
		return
	}
	data := runevents.RunContinuationRequestedData{WorkspaceID: item.WorkspaceID, RunID: item.ID, CausationID: causationID, RequestID: "continuation/" + causationID}.Normalized()
	event := magicevent.NewEventWithContext(runevents.TopicRunContinuationRequested, ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(event.ID(), workorchevents.EventHubLaneScope{SubjectType: "run", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.ID)}))
	m.PostEvent(event)
}
