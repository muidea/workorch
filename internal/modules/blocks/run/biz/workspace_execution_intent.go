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
		RuntimeExecutionIntentAdmission: runtimecontract.RuntimeExecutionIntentAdmission{
			WorkspaceID: cmd.WorkspaceID,
			RunID:       runID,
			RequestID:   cmd.RequestID,
			Kind:        cmd.Kind,
			Outcome:     outcome,
			Reason:      reason,
		},
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
		return runExecutionIntentResult(cmd, current.ID, runtimecontract.RuntimeExecutionIntentFreshActivationRequired, runtimecontract.RuntimeExecutionIntentReasonRuntimeStateIncompatible), nil
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
		transition, err := m.applyRunActivation(ctx, activateRunFact{
			WorkspaceID: cmd.WorkspaceID, RunID: current.ID,
			CausationID: "workspace-execution/resume/" + cmd.RequestID, ReconciliationRequestID: cmd.RequestID,
		})
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
	requestedIntent := replacementIntentFromCommand(cmd)
	items, err := m.runs.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	for _, item := range items {
		if item != nil && strings.TrimSpace(item.ActivationRequestID) == cmd.RequestID {
			if !replacementRunMatchesIntent(item, requestedIntent) {
				return runevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: restart request identity was reused with different sealed input", common.ErrConflict)
			}
			return runExecutionIntentResult(cmd, item.ID, runtimecontract.RuntimeExecutionIntentReplacementPending, runtimecontract.RuntimeExecutionIntentReasonDuplicateRestart), nil
		}
		if item != nil && item.ReplacementIntent != nil && item.ReplacementIntent.RequestID == cmd.RequestID {
			if !replacementIntentMatches(item.ReplacementIntent, requestedIntent) {
				return runevents.WorkspaceExecutionIntentResult{}, fmt.Errorf("%w: restart request identity was reused with different sealed input", common.ErrConflict)
			}
			runID := item.ID
			if item.ReplacementIntent.SuccessorRunID != "" {
				runID = item.ReplacementIntent.SuccessorRunID
			}
			if item.ReplacementIntent.State != runmodel.ReplacementIntentCompleted {
				m.scheduleReplacementTerminationDelivery(item.WorkspaceID, item.ID)
			}
			return runExecutionIntentResult(cmd, runID, runtimecontract.RuntimeExecutionIntentReplacementPending, runtimecontract.RuntimeExecutionIntentReasonDuplicateRestart), nil
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
	if predecessor == nil {
		created, err := m.createWorkspaceExecutionReplacement(ctx, requestedIntent, "")
		if err != nil {
			return runevents.WorkspaceExecutionIntentResult{}, err
		}
		if err := m.activateWorkspaceExecutionReplacement(ctx, created, cmd.RequestID); err != nil {
			return runevents.WorkspaceExecutionIntentResult{}, err
		}
		return runExecutionIntentResult(cmd, created.ID, runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonNewRunActivated), nil
	}
	if predecessor.RuntimeActivationStatus != nil &&
		predecessor.RuntimeActivationStatus.Disposition == runtimecontract.RuntimeActivationDispositionFreshActivationRequired &&
		predecessor.RuntimeActivationStatus.Incompatibility.Reason == runtimecontract.RuntimeStateIncompatibilityReasonSchemaUnsupported {
		return runExecutionIntentResult(cmd, predecessor.ID, runtimecontract.RuntimeExecutionIntentDataResetRequired, runtimecontract.RuntimeExecutionIntentReasonDevelopmentDataIncompatible), nil
	}
	if err := m.persistReplacementIntent(ctx, predecessor, requestedIntent); err != nil {
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	// The predecessor owns the durable request. No successor record exists
	// until SM-4 has persisted Result, Workspace and final Run delivery.
	m.scheduleReplacementTerminationDelivery(predecessor.WorkspaceID, predecessor.ID)
	return runExecutionIntentResult(cmd, predecessor.ID, runtimecontract.RuntimeExecutionIntentReplacementPending, runtimecontract.RuntimeExecutionIntentReasonPredecessorSupersedeRequested), nil
}

func replacementIntentMatches(current *runmodel.ReplacementIntent, requested runmodel.ReplacementIntent) bool {
	if current == nil || current.Bootstrap == nil || requested.Bootstrap == nil {
		return false
	}
	return current.RequestID == requested.RequestID &&
		current.WorkspaceID == requested.WorkspaceID &&
		current.GoalID == requested.GoalID &&
		current.DoDID == requested.DoDID &&
		strings.TrimSpace(current.DefinitionRevision) == strings.TrimSpace(requested.DefinitionRevision) &&
		current.Priority == requested.Priority &&
		current.ReconciliationScope == requested.ReconciliationScope &&
		current.Kind == requested.Kind &&
		strings.TrimSpace(current.Reason) == strings.TrimSpace(requested.Reason) &&
		runtimecontract.StableRuntimeBootstrapHash(*current.Bootstrap) == runtimecontract.StableRuntimeBootstrapHash(*requested.Bootstrap)
}

func replacementRunMatchesIntent(current *runmodel.Run, requested runmodel.ReplacementIntent) bool {
	if current == nil || current.RuntimeBootstrap == nil || requested.Bootstrap == nil {
		return false
	}
	return strings.TrimSpace(current.ActivationRequestID) == requested.RequestID &&
		current.WorkspaceID == requested.WorkspaceID &&
		current.GoalID == requested.GoalID &&
		current.DoDID == requested.DoDID &&
		strings.TrimSpace(current.DefinitionRevision) == strings.TrimSpace(requested.DefinitionRevision) &&
		current.Priority == requested.Priority &&
		current.ReconciliationScope == requested.ReconciliationScope &&
		runtimecontract.StableRuntimeBootstrapHash(*current.RuntimeBootstrap) == runtimecontract.StableRuntimeBootstrapHash(*requested.Bootstrap)
}

func replacementIntentFromCommand(cmd runevents.WorkspaceExecutionIntentCommand) runmodel.ReplacementIntent {
	bootstrap := cmd.Bootstrap.Clone()
	return runmodel.ReplacementIntent{
		RequestID: cmd.RequestID, WorkspaceID: cmd.WorkspaceID, GoalID: cmd.GoalID, DoDID: cmd.DoDID,
		DefinitionRevision: cmd.DefinitionRevision, Bootstrap: &bootstrap, Priority: cmd.Priority,
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		Kind:                runtimecontract.RuntimeTerminationSuperseded, Reason: "workspace execution restart/" + cmd.RequestID,
		State: runmodel.ReplacementIntentPending,
	}
}

func (m *Biz) persistReplacementIntent(ctx context.Context, predecessor *runmodel.Run, intent runmodel.ReplacementIntent) error {
	if predecessor == nil || !intent.ValidFor(predecessor) {
		return common.ErrInvalidArgument
	}
	for attempt := 0; attempt < 8; attempt++ {
		current, err := m.runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
		if err != nil {
			return err
		}
		if current.ReplacementIntent != nil {
			if current.ReplacementIntent.RequestID == intent.RequestID {
				if replacementIntentMatches(current.ReplacementIntent, intent) {
					return nil
				}
				return common.ErrConflict
			}
			return common.ErrConflict
		}
		if runStateIsTerminal(current.State) {
			return common.ErrConflict
		}
		clone := intent
		bootstrap := intent.Bootstrap.Clone()
		clone.Bootstrap = &bootstrap
		current.ReplacementIntent = &clone
		if err := m.runs.CompareAndSwap(ctx, current, current.Revision); err != nil {
			if err == common.ErrConflict {
				continue
			}
			return err
		}
		return nil
	}
	return common.ErrConflict
}

func (m *Biz) createWorkspaceExecutionReplacement(ctx context.Context, intent runmodel.ReplacementIntent, predecessorID common.RunID) (*runmodel.Run, error) {
	if intent.Bootstrap == nil {
		return nil, common.ErrInvalidArgument
	}
	bootstrap := intent.Bootstrap.Clone()
	if err := bootstrap.Validate(); err != nil {
		return nil, err
	}
	items, err := m.runs.ListByWorkspaceID(ctx, intent.WorkspaceID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item != nil && strings.TrimSpace(item.ActivationRequestID) == intent.RequestID {
			if replacementRunMatchesIntent(item, intent) {
				return item, nil
			}
			return nil, common.ErrConflict
		}
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
	lineage.PredecessorRunID = predecessorID
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for attempt := 1; attempt <= 8; attempt++ {
		runID := common.RunID(fmt.Sprintf("run-%03d", maxSequence+attempt))
		item := &runmodel.Run{
			ID:                  runID,
			WorkspaceID:         intent.WorkspaceID,
			GoalID:              intent.GoalID,
			DoDID:               intent.DoDID,
			DefinitionRevision:  intent.DefinitionRevision,
			InitialStepKind:     llmcommon.StepKindWorkspaceExecution,
			RuntimeBootstrap:    &bootstrap,
			ActivationRequestID: intent.RequestID,
			ReconciliationScope: intent.ReconciliationScope,
			Lineage:             lineage,
			State:               common.RunStatePending,
			Priority:            intent.Priority,
			CreatedAt:           now,
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

func (m *Biz) scheduleReplacementTerminationDelivery(workspaceID common.WorkspaceID, predecessorRunID common.RunID) {
	if m == nil || workspaceID == "" || predecessorRunID == "" || m.BackgroundRoutine() == nil {
		return
	}
	m.AsyncTask(func() {
		for attempt := 0; attempt < replacementTerminationDeliveryAttemptsPerTask; attempt++ {
			if err := m.deliverReplacementTermination(workspaceID, predecessorRunID); err == nil {
				return
			}
		}
	})
}

// deliverReplacementTermination sends one persisted Run-owner delivery
// intent and records the concrete SM-4 acknowledgement. It is intentionally
// called only by a background task or startup recovery, never from the
// Workspace -> Run synchronous command stack.
func (m *Biz) deliverReplacementTermination(workspaceID common.WorkspaceID, predecessorRunID common.RunID) error {
	intent, deliver, err := m.claimReplacementTerminationAttempt(workspaceID, predecessorRunID)
	if err != nil || !deliver {
		return err
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, ModuleID, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.RequestRuntimeTerminationCommand{
		WorkspaceID: workspaceID, RunID: predecessorRunID, Kind: intent.Kind,
		RequestedBy: ModuleID, Reason: intent.Reason,
	})
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(orchestratorevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "run", SubjectID: string(predecessorRunID), WorkspaceID: string(workspaceID), RunID: string(predecessorRunID)}))
	response := m.SendEvent(event)
	if response == nil {
		err = fmt.Errorf("%w: Runtime termination returned no result", common.ErrConflict)
		m.recordReplacementTerminationFailure(workspaceID, predecessorRunID, intent.RequestID, err)
		return err
	}
	if responseErr := response.Error(); responseErr != nil {
		err = responseErr
		m.recordReplacementTerminationFailure(workspaceID, predecessorRunID, intent.RequestID, err)
		return err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		err = resultErr
		m.recordReplacementTerminationFailure(workspaceID, predecessorRunID, intent.RequestID, err)
		return err
	}
	ack, ok := data.(orchestratorevents.RequestRuntimeTerminationResult)
	if !ok || !ack.WorkspaceReleased || strings.TrimSpace(ack.TerminalDeliveryID) == "" || ack.StateRevision == 0 {
		err = fmt.Errorf("%w: invalid Runtime termination acknowledgement", common.ErrConflict)
		m.recordReplacementTerminationFailure(workspaceID, predecessorRunID, intent.RequestID, err)
		return err
	}
	return m.acknowledgeReplacementTermination(workspaceID, predecessorRunID, intent.RequestID, ack)
}

func (m *Biz) claimReplacementTerminationAttempt(workspaceID common.WorkspaceID, predecessorRunID common.RunID) (runmodel.ReplacementIntent, bool, error) {
	for attempt := 0; attempt < 8; attempt++ {
		runs, err := m.activeRunRepository()
		if err != nil {
			return runmodel.ReplacementIntent{}, false, err
		}
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
		item, err := runs.GetByWorkspaceID(ctx, workspaceID, predecessorRunID)
		if err != nil {
			return runmodel.ReplacementIntent{}, false, err
		}
		if item.ReplacementIntent == nil {
			return runmodel.ReplacementIntent{}, false, nil
		}
		intent := *item.ReplacementIntent
		if !intent.ValidFor(item) {
			return runmodel.ReplacementIntent{}, false, common.ErrConflict
		}
		if intent.State == runmodel.ReplacementIntentCompleted {
			return intent, false, nil
		}
		intent.AttemptCount++
		intent.LastAttemptAt = time.Now().UTC().Format(time.RFC3339Nano)
		item.ReplacementIntent = &intent
		if err := runs.CompareAndSwap(ctx, item, item.Revision); err != nil {
			if err == common.ErrConflict {
				continue
			}
			return runmodel.ReplacementIntent{}, false, err
		}
		return intent, true, nil
	}
	return runmodel.ReplacementIntent{}, false, common.ErrConflict
}

func (m *Biz) acknowledgeReplacementTermination(workspaceID common.WorkspaceID, predecessorRunID common.RunID, requestID string, ack orchestratorevents.RequestRuntimeTerminationResult) error {
	return m.updateReplacementIntent(workspaceID, predecessorRunID, requestID, func(intent *runmodel.ReplacementIntent) {
		intent.State = runmodel.ReplacementIntentCompleted
		intent.LastError = ""
		intent.TerminalDeliveryID = strings.TrimSpace(ack.TerminalDeliveryID)
		intent.RuntimeStateRevision = ack.StateRevision
		intent.AcknowledgedAt = time.Now().UTC().Format(time.RFC3339Nano)
	})
}

func (m *Biz) recordReplacementTerminationFailure(workspaceID common.WorkspaceID, predecessorRunID common.RunID, requestID string, cause error) {
	_ = m.updateReplacementIntent(workspaceID, predecessorRunID, requestID, func(intent *runmodel.ReplacementIntent) {
		intent.LastError = cause.Error()
	})
}

func (m *Biz) updateReplacementIntent(workspaceID common.WorkspaceID, predecessorRunID common.RunID, requestID string, mutate func(*runmodel.ReplacementIntent)) error {
	for attempt := 0; attempt < 8; attempt++ {
		runs, err := m.activeRunRepository()
		if err != nil {
			return err
		}
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
		item, err := runs.GetByWorkspaceID(ctx, workspaceID, predecessorRunID)
		if err != nil {
			return err
		}
		if item.ReplacementIntent == nil || item.ReplacementIntent.RequestID != requestID {
			return common.ErrConflict
		}
		if item.ReplacementIntent.State == runmodel.ReplacementIntentCompleted {
			return nil
		}
		intent := *item.ReplacementIntent
		mutate(&intent)
		item.ReplacementIntent = &intent
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

func (m *Biz) materializeReplacementAfterTerminalDelivery(ctx context.Context, predecessor *runmodel.Run, terminalDeliveryID string) (*runmodel.Run, error) {
	if predecessor == nil || predecessor.ReplacementIntent == nil || !runStateIsTerminal(predecessor.State) || strings.TrimSpace(terminalDeliveryID) == "" {
		return nil, common.ErrInvalidArgument
	}
	intent := *predecessor.ReplacementIntent
	if !intent.ValidFor(predecessor) {
		return nil, common.ErrConflict
	}
	created, err := m.createWorkspaceExecutionReplacement(ctx, intent, predecessor.ID)
	if err != nil {
		return nil, err
	}
	committed := false
	for attempt := 0; attempt < 8; attempt++ {
		current, loadErr := m.runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
		if loadErr != nil {
			return nil, loadErr
		}
		if current.ReplacementIntent == nil || current.ReplacementIntent.RequestID != intent.RequestID {
			return nil, common.ErrConflict
		}
		if current.ReplacementIntent.SuccessorRunID != "" && current.ReplacementIntent.SuccessorRunID != created.ID {
			return nil, common.ErrConflict
		}
		if current.ReplacementIntent.SuccessorRunID == created.ID && current.Lineage.SuccessorRunID == created.ID {
			committed = true
			break
		}
		nextIntent := *current.ReplacementIntent
		nextIntent.State = runmodel.ReplacementIntentSuccessorCreated
		nextIntent.SuccessorRunID = created.ID
		nextIntent.TerminalDeliveryID = strings.TrimSpace(terminalDeliveryID)
		current.ReplacementIntent = &nextIntent
		current.Lineage.SuccessorRunID = created.ID
		if !nextIntent.ValidFor(current) {
			return nil, common.ErrConflict
		}
		if swapErr := m.runs.CompareAndSwap(ctx, current, current.Revision); swapErr != nil {
			if swapErr == common.ErrConflict {
				continue
			}
			return nil, swapErr
		}
		committed = true
		break
	}
	if !committed {
		return nil, common.ErrConflict
	}
	if err := m.activateWorkspaceExecutionReplacement(ctx, created, intent.RequestID); err != nil {
		return nil, err
	}
	return created, nil
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
	for _, item := range items {
		if item == nil || item.ReplacementIntent == nil {
			continue
		}
		if item.ReplacementIntent.State != runmodel.ReplacementIntentCompleted {
			m.scheduleReplacementTerminationDelivery(item.WorkspaceID, item.ID)
		}
		if runStateIsTerminal(item.State) && item.TerminalDelivery != nil && item.ReplacementIntent.SuccessorRunID == "" {
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), item.WorkspaceID)
			_, _ = m.materializeReplacementAfterTerminalDelivery(ctx, item, item.TerminalDelivery.TerminalDeliveryID)
		}
	}
}
