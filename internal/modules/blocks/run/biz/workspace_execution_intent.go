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
	runcontracts "github.com/muidea/workorch/internal/modules/blocks/run/pkg/contracts"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
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
	case runevents.WorkspaceExecutionIntentResume:
		return m.resumeWorkspaceExecutionIntent(ctx, cmd)
	case runevents.WorkspaceExecutionIntentRestart:
		return m.restartWorkspaceExecutionIntent(ctx, cmd)
	default:
		return runevents.WorkspaceExecutionIntentResult{}, common.ErrInvalidArgument
	}
}

func runExecutionIntentResult(cmd runevents.WorkspaceExecutionIntentCommand, runID common.RunID, disposition string, diagnostic string) runevents.WorkspaceExecutionIntentResult {
	return runevents.WorkspaceExecutionIntentResult{
		WorkspaceID: cmd.WorkspaceID,
		RunID:       runID,
		RequestID:   cmd.RequestID,
		Kind:        cmd.Kind,
		Disposition: disposition,
		Diagnostic:  diagnostic,
	}
}

func (m *Biz) resumeWorkspaceExecutionIntent(ctx context.Context, cmd runevents.WorkspaceExecutionIntentCommand) (runevents.WorkspaceExecutionIntentResult, error) {
	current, err := m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID)
	if err != nil {
		if err == common.ErrNotFound {
			return runExecutionIntentResult(cmd, cmd.RunID, "restart_required", "Workspace selected Run is absent"), nil
		}
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	if runStateIsTerminal(current.State) {
		return runExecutionIntentResult(cmd, current.ID, "restart_required", "Workspace selected Run is terminal"), nil
	}
	if runtimeActivationDispositionForRun(current) == runmodel.RuntimeActivationDispositionFreshActivationRequired {
		return runExecutionIntentResult(cmd, current.ID, string(runmodel.RuntimeActivationDispositionFreshActivationRequired), "Runtime aggregate requires fresh activation"), nil
	}
	if current.GoalID != cmd.GoalID || current.DoDID != cmd.DoDID || strings.TrimSpace(current.DefinitionRevision) != cmd.DefinitionRevision {
		return runExecutionIntentResult(cmd, current.ID, "definition_changed_requires_restart", "Workspace Goal/DoD definition differs from sealed Run"), nil
	}
	if current.RuntimeBootstrap == nil || current.RuntimeBootstrap.Definition.Normalized().Revision != cmd.DefinitionRevision {
		return runExecutionIntentResult(cmd, current.ID, "fresh_activation_required", "Run lacks a matching sealed Runtime V2 bootstrap"), nil
	}
	switch current.State {
	case common.RunStateActive:
		m.publishContinuationRequested(ctx, current, "workspace-execution/resume/"+cmd.RequestID)
		return runExecutionIntentResult(cmd, current.ID, "reconciliation_requested", "current Run continuation accepted"), nil
	case common.RunStatePending:
		transition, err := m.applyLifecycleLocked(ctx, cmd.WorkspaceID, current.ID, func(run *runmodel.Run) (lifecycleTransition, error) {
			return reduceActivateRun(run, runevents.ActivateRunCommand{WorkspaceID: cmd.WorkspaceID, RunID: current.ID, CausationID: "workspace-execution/resume/" + cmd.RequestID, ReconciliationRequestID: cmd.RequestID})
		}, nil)
		if err != nil {
			return runevents.WorkspaceExecutionIntentResult{}, err
		}
		return runExecutionIntentResult(cmd, transition.Run.ID, "reconciliation_requested", "pending Run activated for continuation"), nil
	case common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
		return runExecutionIntentResult(cmd, current.ID, "governance_required", "Run remains blocked until matching governance resolution"), nil
	default:
		return runExecutionIntentResult(cmd, current.ID, "restart_required", "Run is not resumable"), nil
	}
}

func (m *Biz) restartWorkspaceExecutionIntent(ctx context.Context, cmd runevents.WorkspaceExecutionIntentCommand) (runevents.WorkspaceExecutionIntentResult, error) {
	items, err := m.runs.ListByWorkspaceID(ctx, cmd.WorkspaceID)
	if err != nil {
		return runevents.WorkspaceExecutionIntentResult{}, err
	}
	for _, item := range items {
		if item != nil && strings.TrimSpace(item.ActivationRequestID) == cmd.RequestID {
			return runExecutionIntentResult(cmd, item.ID, "replacement_pending", "duplicate restart request"), nil
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
		return runExecutionIntentResult(cmd, created.ID, "reconciliation_requested", "new Workspace Run activated"), nil
	}
	if runtimeActivationDispositionForRun(predecessor) == runmodel.RuntimeActivationDispositionFreshActivationRequired {
		m.postFreshActivationSlotRelease(ctx, predecessor)
		if err := m.activateWorkspaceExecutionReplacement(ctx, created, cmd.RequestID); err != nil {
			return runevents.WorkspaceExecutionIntentResult{}, err
		}
		return runExecutionIntentResult(cmd, created.ID, "reconciliation_requested", "fresh activation replacement created"), nil
	}
	// SM-4 owns terminal delivery. Run owns the choice to supersede its own
	// predecessor, so it posts a bounded immutable termination intent only
	// after persisting the successor. The terminal Run fact later activates the
	// pending successor through a Run-private internal command.
	m.postSupersedeTermination(ctx, predecessor, cmd.RequestID)
	return runExecutionIntentResult(cmd, created.ID, "replacement_pending", "predecessor supersede requested"), nil
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
			ID:                     runID,
			WorkspaceID:            cmd.WorkspaceID,
			GoalID:                 cmd.GoalID,
			DoDID:                  cmd.DoDID,
			DefinitionRevision:     cmd.DefinitionRevision,
			InitialStepKind:        llmcommon.StepKindWorkspaceExecution,
			RuntimeBootstrap:       &bootstrap,
			RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2,
			ActivationRequestID:    cmd.RequestID,
			ReconciliationScope:    runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
			Lineage:                lineage,
			State:                  common.RunStatePending,
			Priority:               cmd.Priority,
			CreatedAt:              now,
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
		m.publishRuntimeV2ActivationMemoryRecall(ctx, runcontracts.CreateRunCommand{
			RunID: item.ID, WorkspaceID: item.WorkspaceID, GoalID: item.GoalID, DoDID: item.DoDID,
			DefinitionRevision: item.DefinitionRevision, InitialStepKind: item.InitialStepKind,
			RuntimeBootstrap: item.RuntimeBootstrap, Lineage: item.Lineage, Priority: item.Priority,
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
	_, err := m.applyLifecycleLocked(ctx, run.WorkspaceID, run.ID, func(current *runmodel.Run) (lifecycleTransition, error) {
		return reduceActivateRun(current, runevents.ActivateRunCommand{WorkspaceID: run.WorkspaceID, RunID: run.ID, CausationID: "workspace-execution/restart/" + requestID})
	}, nil)
	return err
}

func (m *Biz) postSupersedeTermination(ctx context.Context, predecessor *runmodel.Run, requestID string) {
	if m == nil || m.EventHub() == nil || predecessor == nil {
		return
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, ModuleID, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.RequestRuntimeTerminationCommand{
		WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID, Kind: runtimecontract.RuntimeTerminationSuperseded,
		RequestedBy: ModuleID, Reason: "workspace execution restart/" + requestID,
	})
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(event.ID(), workorchevents.EventHubLaneScope{SubjectType: "run", SubjectID: string(predecessor.ID), WorkspaceID: string(predecessor.WorkspaceID), RunID: string(predecessor.ID)}))
	m.PostEvent(event)
}

func (m *Biz) postFreshActivationSlotRelease(ctx context.Context, predecessor *runmodel.Run) {
	if m == nil || m.EventHub() == nil || predecessor == nil || predecessor.RuntimeState == nil || predecessor.RuntimeActivationStatus == nil {
		return
	}
	status := predecessor.RuntimeActivationStatus
	if status.Disposition != runmodel.RuntimeActivationDispositionFreshActivationRequired {
		return
	}
	command := workspaceevents.ReleaseFreshActivationRunSlotFactCommand{
		WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID, SourceOwner: ModuleID,
		CausationID:          runevents.RunStateChangedDataFromModel(predecessor).CausationID,
		RuntimeStateRevision: predecessor.RuntimeState.StateRevision, RuntimeStateRef: status.RuntimeStateRef,
		RuntimeActivationDisposition: status.Disposition, ActualRuntimeStateSchemaVersion: status.ActualSchemaVersion, SupportedRuntimeStateSchemaVersion: status.SupportedSchemaVersion,
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandReleaseFreshActivationRunSlot, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(event.ID(), workorchevents.EventHubLaneScope{SubjectType: "workspace", SubjectID: string(predecessor.WorkspaceID), WorkspaceID: string(predecessor.WorkspaceID), RunID: string(predecessor.ID)}))
	m.PostEvent(event)
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
	byID := make(map[common.RunID]*runmodel.Run, len(items))
	for _, item := range items {
		if item != nil {
			byID[item.ID] = item
		}
	}
	for _, item := range items {
		if item == nil || item.State != common.RunStatePending || strings.TrimSpace(item.ActivationRequestID) == "" {
			continue
		}
		predecessorID := item.Lineage.PredecessorRunID
		if predecessorID != "" {
			predecessor := byID[predecessorID]
			if predecessor == nil || predecessor.WorkspaceID != item.WorkspaceID || !runStateIsTerminal(predecessor.State) {
				continue
			}
		}
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), item.WorkspaceID)
		m.postWorkspaceExecutionReplacementActivation(ctx, item, "startup-recovery")
	}
}

func (m *Biz) postWorkspaceExecutionReplacementActivation(ctx context.Context, item *runmodel.Run, cause string) {
	if m == nil || m.EventHub() == nil || item == nil || strings.TrimSpace(item.ActivationRequestID) == "" {
		return
	}
	activation := magicevent.NewEventWithContext(runevents.TopicRunCommandActivate, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.ActivateRunCommand{
		WorkspaceID: item.WorkspaceID, RunID: item.ID, CausationID: "workspace-execution/" + cause + "/" + item.ActivationRequestID,
	})
	activation.BindLaneKey(workorchevents.EventHubLaneKeyFor(activation.ID(), workorchevents.EventHubLaneScope{SubjectType: "run", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.ID)}))
	m.PostEvent(activation)
}
