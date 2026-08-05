package run

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const workspaceExecutionIntentRunSource = "blocks/workspace"

func (m *Biz) handleWorkspaceExecutionIntent(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeWorkspaceExecutionIntent(ev)
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	if err := response.Validate(); err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

// executeWorkspaceExecutionIntent converts a Workspace-owner selection fact
// to a Run-private SM-1 action. No ingress source supplies a target state;
// all activation, replacement, and continuation choices are made by Run
// against its own persisted records.
func (m *Biz) executeWorkspaceExecutionIntent(ev magicevent.Event) (runevents.WorkspaceExecutionIntentResult, error) {
	if m == nil {
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrInvalidArgument
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.runs == nil || m.Service == nil || m.EventHub() == nil {
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrInvalidArgument
	}
	cmd, ok := ev.Data().(runevents.WorkspaceExecutionIntentCommand)
	if !ok || ev.Source() != workspaceExecutionIntentRunSource {
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrInvalidArgument
	}
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	if m.stopping {
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrConflict
	}
	switch cmd.Kind {
	case runtimecontract.RuntimeExecutionIntentResume:
		return m.resumeWorkspaceExecutionIntent(ctx, cmd)
	case runtimecontract.RuntimeExecutionIntentRestart:
		return m.restartWorkspaceExecutionIntent(ctx, cmd)
	default:
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrInvalidArgument
	}
}

func runExecutionIntentResult(cmd runevents.WorkspaceExecutionIntentCommand, runID common.RunID, outcome runtimecontract.RuntimeExecutionIntentOutcome, reason runtimecontract.RuntimeExecutionIntentReason) runevents.WorkspaceExecutionIntentResult {
	return runevents.WorkspaceExecutionIntentResult{
		WorkspaceID: cmd.WorkspaceID,
		RunID:       runID,
		RequestID:   cmd.RequestID,
		Kind:        cmd.Kind,
		Outcome:     outcome,
		Reason:      reason,
	}
}

func (m *Biz) resumeWorkspaceExecutionIntent(ctx context.Context, cmd runevents.WorkspaceExecutionIntentCommand) (runevents.WorkspaceExecutionIntentResult, error) {
	current, err := m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID)
	if err != nil {
		if err == common.ErrNotFound {
			return runExecutionIntentResult(cmd, cmd.RunID, runtimecontract.RuntimeExecutionIntentRestartRequired, runtimecontract.RuntimeExecutionIntentReasonSelectedRunAbsent), nil
		}
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	if runStateIsTerminal(current.State) {
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentRestartRequired, runtimecontract.RuntimeExecutionIntentReasonSelectedRunTerminal), nil
	}
	if runtimeActivationDispositionForRun(current) == runtimecontract.RuntimeActivationDispositionFreshActivationRequired {
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentFreshActivationRequired, runtimecontract.RuntimeExecutionIntentReasonRuntimeSchemaIncompatible), nil
	}
	if current.GoalID != cmd.GoalID || current.DoDID != cmd.DoDID || strings.TrimSpace(current.DefinitionRevision) != cmd.DefinitionRevision {
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentDefinitionChanged, runtimecontract.RuntimeExecutionIntentReasonDefinitionChanged), nil
	}
	if current.RuntimeBootstrap == nil || current.RuntimeBootstrap.Definition.Normalized().Revision != cmd.DefinitionRevision {
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentFreshActivationRequired, runtimecontract.RuntimeExecutionIntentReasonRuntimeBootstrapMissing), nil
	}
	switch current.State {
	case common.RunStateActive:
		m.publishContinuationRequested(ctx, current, "workspace-execution/resume/"+cmd.RequestID)
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted), nil
	case common.RunStatePending:
		transition, err := m.applyLifecycleLocked(ctx, cmd.WorkspaceID, current.ID, func(run *runmodel.Run) (lifecycleTransition, error) {
			return reduceActivateRun(run, activateRunFact{WorkspaceID: cmd.WorkspaceID, RunID: current.ID, CausationID: "workspace-execution/resume/" + cmd.RequestID, ReconciliationRequestID: cmd.RequestID})
		}, nil)
		if err != nil {
			return runevents.WorkspaceExecutionIntentResult{}, err
		}
		return runExecutionIntentResult(cmd, transition.Run.ID, runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonPendingRunActivated), nil
	case common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentGovernanceRequired, runtimecontract.RuntimeExecutionIntentReasonGovernancePending), nil
	default:
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentRestartRequired, runtimecontract.RuntimeExecutionIntentReasonRunNotResumable), nil
	}
}

func (m *Biz) restartWorkspaceExecutionIntent(ctx context.Context, cmd runevents.WorkspaceExecutionIntentCommand) (runevents.WorkspaceExecutionIntentResult, error) {
	items, err := m.runs.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	for _, item := range items {
		if item != nil && strings.TrimSpace(item.ActivationRequestID) == cmd.RequestID {
			m.scheduleReplacementTerminationDelivery(item.WorkspaceID, item.ID)
			return runExecutionIntentResult(cmd, item.ID, runtimecontract.RuntimeExecutionIntentReplacementPending, runtimecontract.RuntimeExecutionIntentReasonDuplicateRestart), nil
		}
	}
	var predecessor *runmodel.Run
	for _, item := range items {
		if item == nil || runStateIsTerminal(item.State) {
			continue
		}
		if predecessor != nil && predecessor.ID != item.ID {
			return runevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: more than one non-terminal Run belongs to Workspace", common.ErrConflict)
		}
		predecessor = item
	}
	created, err := m.createWorkspaceExecutionReplacement(ctx, cmd, items, predecessor)
	if err != nil {
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	if predecessor == nil {
		if err := m.activateWorkspaceExecutionReplacement(ctx, created, cmd.RequestID); err != nil {
			return runevents.WorkspaceExecutionIntentResult{}, err
		}
		return runExecutionIntentResult(cmd, created.ID, runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonNewRunActivated), nil
	}
	if runtimeActivationDispositionForRun(predecessor) == runtimecontract.RuntimeActivationDispositionFreshActivationRequired {
		if err := m.activateWorkspaceExecutionReplacement(ctx, created, cmd.RequestID); err != nil {
			return runevents.WorkspaceExecutionIntentResult{}, err
		}
		result := runExecutionIntentResult(cmd, created.ID, runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonFreshActivationReplacement)
		result.FreshActivationRelease = freshActivationReleaseFact(predecessor)
		return result, nil
	}
	// SM-4 owns terminal delivery. The Run successor already contains the
	// durable pending intent, so delivery leaves this synchronous
	// Workspace -> Run lane before entering Orchestrator. The terminal Run fact
	// later activates the pending successor through a Run-private reducer.
	m.scheduleReplacementTerminationDelivery(created.WorkspaceID, created.ID)
	return runExecutionIntentResult(cmd, created.ID, runtimecontract.RuntimeExecutionIntentReplacementPending, runtimecontract.RuntimeExecutionIntentReasonPredecessorSupersedeRequested), nil
}

func (m *Biz) createWorkspaceExecutionReplacement(ctx context.Context, cmd runevents.WorkspaceExecutionIntentCommand, items []*runmodel.Run, predecessor *runmodel.Run) (*runmodel.Run, error) {
	bootstrap := cmd.Bootstrap.Clone()
	if err := bootstrap.Validate(); err != nil {
		return nil, err
	}
	maxSequence := 0
	for _, item := range items {
		if item == nil {
			continue
		}
		if sequence, ok := numericRunSequence(item.ID); ok && sequence > maxSequence {
			maxSequence = sequence
		}
	}
	lineage := runmodel.RunLineage{LineageState: "active"}
	if predecessor != nil {
		lineage.PredecessorRunID = predecessor.ID
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for attempt := 1; attempt <= 8; attempt++ {
		runID := common.RunID(fmt.Sprintf("run-%03d", maxSequence+attempt))
		item := &runmodel.Run{
			ID:                  runID,
			WorkspaceID:         cmd.WorkspaceID,
			GoalID:              cmd.GoalID,
			DoDID:               cmd.DoDID,
			DefinitionRevision:  cmd.DefinitionRevision,
			InitialStepKind:     llmcommon.StepKindWorkspaceExecution,
			RuntimeBootstrap:    &bootstrap,
			ActivationRequestID: cmd.RequestID,
			ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
			Lineage:             lineage,
			State:               common.RunStatePending,
			Priority:            cmd.Priority,
			CreatedAt:           now,
		}
		if predecessor != nil && runtimeActivationDispositionForRun(predecessor) != runtimecontract.RuntimeActivationDispositionFreshActivationRequired {
			item.ReplacementTerminationIntent = &runmodel.ReplacementTerminationIntent{
				RequestID:        "workspace-execution/restart/" + cmd.RequestID,
				PredecessorRunID: predecessor.ID,
				Kind:             runtimecontract.RuntimeTerminationSuperseded,
				Reason:           "workspace execution restart/" + cmd.RequestID,
				State:            runmodel.ReplacementTerminationIntentPending,
			}
			if !item.ReplacementTerminationIntent.ValidFor(item) {
				return nil, common.ErrInvalidArgument
			}
		}
		if err := m.runs.Create(ctx, item); err != nil {
			if err == common.ErrConflict {
				continue
			}
			return nil, err
		}
		if err := m.publishRunEvent(ctx, runevents.TopicRunCreated, item); err != nil {
			return nil, err
		}
		// Restart replacement creation bypasses the generic CreateRun command so
		// Run can persist the successor and its lineage atomically within the
		// owner. Preserve the same typed activation-memory handoff used by the
		// regular create/ensure paths after the Run record is durable.
		m.publishRuntimeActivationMemoryRecall(ctx, runevents.CreateRunCommand{
			RunID: item.ID, WorkspaceID: item.WorkspaceID, GoalID: item.GoalID, DoDID: item.DoDID,
			DefinitionRevision: item.DefinitionRevision, InitialStepKind: item.InitialStepKind,
			RuntimeBootstrap: item.RuntimeBootstrap, PredecessorRunID: item.Lineage.PredecessorRunID,
			PredecessorResultID: item.Lineage.PredecessorResultID, Priority: item.Priority,
		})
		return item, nil
	}
	return nil, common.ErrConflict
}

func numericRunSequence(runID common.RunID) (int, bool) {
	raw := strings.TrimSpace(string(runID))
	if !strings.HasPrefix(raw, "run-") {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimPrefix(raw, "run-"))
	return value, err == nil && value > 0
}

func (m *Biz) activateWorkspaceExecutionReplacement(ctx context.Context, run *runmodel.Run, requestID string) error {
	if run == nil {
		return common.ErrInvalidArgument
	}
	_, err := m.applyRunActivation(ctx, activateRunFact{WorkspaceID: run.WorkspaceID, RunID: run.ID, CausationID: "workspace-execution/restart/" + requestID})
	return err
}

const replacementTerminationDeliveryAttemptsPerTask = 3

func (m *Biz) scheduleReplacementTerminationDelivery(workspaceID common.WorkspaceID, successorRunID common.RunID) {
	if m == nil || workspaceID == "" || successorRunID == "" || m.BackgroundRoutine() == nil {
		return
	}
	m.AsyncTask(func() {
		for attempt := 0; attempt < replacementTerminationDeliveryAttemptsPerTask; attempt++ {
			if err := m.deliverReplacementTermination(workspaceID, successorRunID); err == nil {
				return
			}
		}
	})
}

// deliverReplacementTermination sends one persisted Run-owner delivery
// intent and records the concrete SM-4 acknowledgement. It is intentionally
// called only by a background task or startup recovery, never from the
// Workspace -> Run synchronous command stack.
func (m *Biz) deliverReplacementTermination(workspaceID common.WorkspaceID, successorRunID common.RunID) error {
	intent, deliver, err := m.claimReplacementTerminationAttempt(workspaceID, successorRunID)
	if err != nil || !deliver {
		return err
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, ModuleID, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.RequestRuntimeTerminationCommand{
		WorkspaceID: workspaceID, RunID: intent.PredecessorRunID, Kind: intent.Kind,
		RequestedBy: ModuleID, Reason: intent.Reason,
	})
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(orchestratorevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "run", SubjectID: string(intent.PredecessorRunID), WorkspaceID: string(workspaceID), RunID: string(intent.PredecessorRunID)}))
	response := m.SendEvent(event)
	if response == nil {
		err = fmt.Errorf("%w: Runtime termination returned no result", common.ErrConflict)
		m.recordReplacementTerminationFailure(workspaceID, successorRunID, intent.RequestID, err)
		return err
	}
	if responseErr := response.Error(); responseErr != nil {
		err = responseErr
		m.recordReplacementTerminationFailure(workspaceID, successorRunID, intent.RequestID, err)
		return err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		err = resultErr
		m.recordReplacementTerminationFailure(workspaceID, successorRunID, intent.RequestID, err)
		return err
	}
	ack, ok := data.(orchestratorevents.RequestRuntimeTerminationResult)
	if !ok || !ack.WorkspaceReleased || strings.TrimSpace(ack.TerminalDeliveryID) == "" || ack.StateRevision == 0 {
		err = fmt.Errorf("%w: invalid Runtime termination acknowledgement", common.ErrConflict)
		m.recordReplacementTerminationFailure(workspaceID, successorRunID, intent.RequestID, err)
		return err
	}
	return m.acknowledgeReplacementTermination(workspaceID, successorRunID, intent.RequestID, ack)
}

func (m *Biz) claimReplacementTerminationAttempt(workspaceID common.WorkspaceID, successorRunID common.RunID) (runmodel.ReplacementTerminationIntent, bool, error) {
	for attempt := 0; attempt < 8; attempt++ {
		runs, err := m.activeRunRepository()
		if err != nil {
			return runmodel.ReplacementTerminationIntent{}, false, err
		}
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
		item, err := runs.GetByWorkspaceID(ctx, workspaceID, successorRunID)
		if err != nil {
			return runmodel.ReplacementTerminationIntent{}, false, err
		}
		if item.ReplacementTerminationIntent == nil {
			return runmodel.ReplacementTerminationIntent{}, false, nil
		}
		intent := *item.ReplacementTerminationIntent
		if !intent.ValidFor(item) {
			return runmodel.ReplacementTerminationIntent{}, false, common.ErrConflict
		}
		if intent.State == runmodel.ReplacementTerminationIntentAcknowledged {
			return intent, false, nil
		}
		intent.AttemptCount++
		intent.LastAttemptAt = time.Now().UTC().Format(time.RFC3339Nano)
		item.ReplacementTerminationIntent = &intent
		if err := runs.CompareAndSwap(ctx, item, item.Revision); err != nil {
			if err == common.ErrConflict {
				continue
			}
			return runmodel.ReplacementTerminationIntent{}, false, err
		}
		return intent, true, nil
	}
	return runmodel.ReplacementTerminationIntent{}, false, common.ErrConflict
}

func (m *Biz) acknowledgeReplacementTermination(workspaceID common.WorkspaceID, successorRunID common.RunID, requestID string, ack orchestratorevents.RequestRuntimeTerminationResult) error {
	return m.updateReplacementTerminationIntent(workspaceID, successorRunID, requestID, func(intent *runmodel.ReplacementTerminationIntent) {
		intent.State = runmodel.ReplacementTerminationIntentAcknowledged
		intent.LastError = ""
		intent.TerminalDeliveryID = strings.TrimSpace(ack.TerminalDeliveryID)
		intent.RuntimeStateRevision = ack.StateRevision
		intent.AcknowledgedAt = time.Now().UTC().Format(time.RFC3339Nano)
	})
}

func (m *Biz) recordReplacementTerminationFailure(workspaceID common.WorkspaceID, successorRunID common.RunID, requestID string, cause error) {
	_ = m.updateReplacementTerminationIntent(workspaceID, successorRunID, requestID, func(intent *runmodel.ReplacementTerminationIntent) {
		intent.LastError = cause.Error()
	})
}

func (m *Biz) updateReplacementTerminationIntent(workspaceID common.WorkspaceID, successorRunID common.RunID, requestID string, mutate func(*runmodel.ReplacementTerminationIntent)) error {
	for attempt := 0; attempt < 8; attempt++ {
		runs, err := m.activeRunRepository()
		if err != nil {
			return err
		}
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
		item, err := runs.GetByWorkspaceID(ctx, workspaceID, successorRunID)
		if err != nil {
			return err
		}
		if item.ReplacementTerminationIntent == nil || item.ReplacementTerminationIntent.RequestID != requestID {
			return common.ErrConflict
		}
		if item.ReplacementTerminationIntent.State == runmodel.ReplacementTerminationIntentAcknowledged {
			return nil
		}
		intent := *item.ReplacementTerminationIntent
		mutate(&intent)
		item.ReplacementTerminationIntent = &intent
		if !intent.ValidFor(item) {
			return common.ErrConflict
		}
		if err := runs.CompareAndSwap(ctx, item, item.Revision); err != nil {
			if err == common.ErrConflict {
				continue
			}
			return err
		}
		return nil
	}
	return common.ErrConflict
}

func (m *Biz) activeRunRepository() (repository.RunRepository, error) {
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil || m.EventHub() == nil {
		return nil, common.ErrConflict
	}
	return m.runs, nil
}

func freshActivationReleaseFact(predecessor *runmodel.Run) *runevents.FreshActivationReleaseFact {
	if predecessor == nil || predecessor.RuntimeState == nil || predecessor.RuntimeActivationStatus == nil {
		return nil
	}
	status := predecessor.RuntimeActivationStatus
	if status.Disposition != runtimecontract.RuntimeActivationDispositionFreshActivationRequired {
		return nil
	}
	return &runevents.FreshActivationReleaseFact{
		WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID,
		CausationID:          runevents.RunStateChangedDataFromModel(predecessor).CausationID,
		RuntimeStateRevision: predecessor.RuntimeState.StateRevision, RuntimeStateRef: status.RuntimeStateRef,
		RuntimeActivationDisposition: status.Disposition, ActualRuntimeStateSchemaVersion: status.ActualSchemaVersion, SupportedRuntimeStateSchemaVersion: status.SupportedSchemaVersion,
	}
}

// handleReplacementPredecessorState converts a persisted terminal Run fact
// into a Run-owner internal activation command for its already-persisted
// successor. It never reads Workspace state or infers a definition.
func (m *Biz) handleReplacementPredecessorState(ev magicevent.Event, _ magicevent.Result) {
	if m == nil {
		return
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	data, ok := ev.Data().(runevents.RunStateChangedData)
	if !ok || ev.Source() != ModuleID || data.Validate() != nil || !runStateIsTerminal(data.State) || m.stopping || m.runs == nil || m.EventHub() == nil {
		return
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), data.WorkspaceID)
	if err != nil {
		return
	}
	items, err := m.runs.ListByWorkspaceID(ctx, data.WorkspaceID)
	if err != nil {
		return
	}
	for _, item := range items {
		if item == nil || item.State != common.RunStatePending || item.Lineage.PredecessorRunID != data.RunID || strings.TrimSpace(item.ActivationRequestID) == "" {
			continue
		}
		m.postWorkspaceExecutionReplacementActivation(ctx, item, "predecessor-terminal")
	}
}

func (m *Biz) recoverPendingWorkspaceExecutionSuccessors() {
	if m == nil {
		return
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil || m.EventHub() == nil {
		return
	}
	items, err := m.runs.List(context.Background())
	if err != nil {
		return
	}
	byID := make(map[string]*runmodel.Run, len(items))
	for _, item := range items {
		if item != nil {
			byID[replacementRunScopeKey(item.WorkspaceID, item.ID)] = item
		}
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.ReplacementTerminationIntent != nil && item.ReplacementTerminationIntent.State == runmodel.ReplacementTerminationIntentPending {
			m.scheduleReplacementTerminationDelivery(item.WorkspaceID, item.ID)
		}
		if item.State != common.RunStatePending || strings.TrimSpace(item.ActivationRequestID) == "" {
			continue
		}
		predecessorID := item.Lineage.PredecessorRunID
		if predecessorID != "" {
			predecessor := byID[replacementRunScopeKey(item.WorkspaceID, predecessorID)]
			if predecessor == nil || predecessor.WorkspaceID != item.WorkspaceID || !runStateIsTerminal(predecessor.State) {
				continue
			}
		}
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), item.WorkspaceID)
		m.postWorkspaceExecutionReplacementActivation(ctx, item, "startup-recovery")
	}
}

func replacementRunScopeKey(workspaceID common.WorkspaceID, runID common.RunID) string {
	return string(workspaceID) + "\x00" + string(runID)
}

func (m *Biz) postWorkspaceExecutionReplacementActivation(ctx context.Context, item *runmodel.Run, cause string) {
	if m == nil || item == nil || strings.TrimSpace(item.ActivationRequestID) == "" {
		return
	}
	_, _ = m.applyRunActivation(ctx, activateRunFact{
		WorkspaceID: item.WorkspaceID, RunID: item.ID, CausationID: "workspace-execution/" + cause + "/" + item.ActivationRequestID,
	})
}
