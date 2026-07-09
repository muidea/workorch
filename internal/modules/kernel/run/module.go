package run

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	runbiz "github.com/muidea/workorch/internal/modules/kernel/run/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/run/pkg/common"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = runservice.Service
type Orchestrator = runbiz.Orchestrator
type CreateRunCommand = runcontracts.CreateRunCommand
type StartRunCommand = runcontracts.StartRunCommand
type ResumeRunCommand = runcontracts.ResumeRunCommand
type UpdateRunCommand = runcontracts.UpdateRunCommand
type BlockOnDecisionCommand = runcontracts.BlockOnDecisionCommand
type BlockOnApprovalCommand = runcontracts.BlockOnApprovalCommand
type ValidateRunCommand = runcontracts.ValidateRunCommand
type CompleteRunCommand = runcontracts.CompleteRunCommand
type CancelRunCommand = runcontracts.CancelRunCommand
type SupersedeRunCommand = runcontracts.SupersedeRunCommand
type FailRunCommand = runcontracts.FailRunCommand

type Dependencies struct {
	Runs         repository.RunRepository
	Steps        repository.StepRepository
	Checkpoints  repository.RuntimeCheckpointRepository
	Workspaces   runbiz.WorkspaceReader
	Orchestrator runbiz.Orchestrator
	EventHub     magicevent.Hub
}

type Module struct {
	bizPtr      *runbiz.Service
	servicePtr  *runservice.Module
	runs        repository.RunRepository
	steps       repository.StepRepository
	checkpoints repository.RuntimeCheckpointRepository
	eventHub    magicevent.Hub
	observer    magicevent.SimpleObserver
	topics      []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Runs == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.runs = deps.Runs
	m.steps = deps.Steps
	m.checkpoints = deps.Checkpoints
	eventHubBridge := events.NewMagicHubWithHubOwner(deps.EventHub, ModuleID)
	workspaces := deps.Workspaces
	if workspaces == nil {
		workspaces = workspaceEventReader{hub: eventHubBridge}
	}
	orchestrator := deps.Orchestrator
	if orchestrator == nil {
		orchestrator = NewEventOrchestrator(eventHubBridge)
	}
	m.bizPtr = runbiz.NewServiceWithWorkspaceReader(deps.Runs, workspaces, orchestrator, eventHubBridge)
	m.servicePtr = runservice.New(runservice.Dependencies{
		CreateRun:       m.bizPtr.CreateRun,
		EnsureRun:       m.bizPtr.EnsureRun,
		StartRun:        m.bizPtr.StartRun,
		EnsureStarted:   m.bizPtr.EnsureStarted,
		ResumeRun:       m.bizPtr.ResumeRun,
		EnsureResumed:   m.bizPtr.EnsureResumed,
		BlockOnDecision: m.bizPtr.BlockOnDecision,
		EnsureDecision:  m.bizPtr.EnsureBlockedOnDecision,
		BlockOnApproval: m.bizPtr.BlockOnApproval,
		EnsureApproval:  m.bizPtr.EnsureBlockedOnApproval,
		ValidateRun:     m.bizPtr.ValidateRun,
		CompleteRun:     m.bizPtr.CompleteRun,
		CancelRun:       m.bizPtr.CancelRun,
		SupersedeRun:    m.bizPtr.SupersedeRun,
		FailRun:         m.bizPtr.FailRun,
	})
	m.eventHub = deps.EventHub
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.servicePtr == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.observer != nil {
		return nil
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.topics = []string{
		runevents.TopicRunCommandEnsure,
		runevents.TopicRunCommandCreate,
		runevents.TopicRunCommandGet,
		runevents.TopicRunCommandList,
		runevents.TopicRunCommandListByWorkspace,
		runevents.TopicRunCommandGetStep,
		runevents.TopicRunCommandListStepsByRun,
		runevents.TopicRunCommandListStepSummariesByRun,
		runevents.TopicRunCommandGetRuntimeCheckpoint,
		runevents.TopicRunCommandListCheckpointsByRun,
		runevents.TopicRunCommandUpdate,
		runevents.TopicRunCommandDelete,
	}
	for _, topic := range m.topics {
		m.observer.Subscribe(topic, m.handleCommand)
	}
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.observer != nil {
		for idx := len(m.topics) - 1; idx >= 0; idx-- {
			m.observer.Unsubscribe(m.topics[idx])
		}
	}
	m.topics = nil
	m.observer = nil
	m.servicePtr = nil
	m.bizPtr = nil
	m.runs = nil
	m.steps = nil
	m.checkpoints = nil
	m.eventHub = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}

func (m *Module) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev.Context(), ev.ID(), ev.Data())
	setRunMagicResult(result, response, err)
}

func (m *Module) executeCommand(ctx context.Context, topic string, data any) (runevents.RunCommandResult, error) {
	if m == nil || m.runs == nil || m.servicePtr == nil {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	if workspaceID := runCommandWorkspaceID(data); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	switch topic {
	case runevents.TopicRunCommandEnsure:
		cmd, ok := data.(runevents.EnsureRunCommand)
		if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.EnsureRun(ctx, cmd); err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{}, nil
	case runevents.TopicRunCommandCreate:
		cmd, ok := data.(runevents.CreateRunCommand)
		if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.CreateRun(ctx, cmd); err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{}, nil
	case runevents.TopicRunCommandGet:
		cmd, ok := data.(runevents.GetRunCommand)
		if !ok || cmd.RunID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		var item *runmodel.Run
		var err error
		if cmd.WorkspaceID != "" {
			item, err = m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID)
		} else {
			item, err = m.runs.Get(ctx, cmd.RunID)
		}
		if err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{Run: item}, nil
	case runevents.TopicRunCommandList:
		cmd, ok := data.(runevents.ListRunsCommand)
		if !ok {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.runs.List(ctx)
		if err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{Runs: projectRunCommandListItems(items, runStateFilter(cmd.States), cmd.Compact)}, nil
	case runevents.TopicRunCommandListByWorkspace:
		cmd, ok := data.(runevents.ListRunsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.runs.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{Runs: projectRunCommandListItems(items, runStateFilter(cmd.States), cmd.Compact)}, nil
	case runevents.TopicRunCommandGetStep:
		cmd, ok := data.(runevents.GetStepCommand)
		if !ok || cmd.StepID == "" || m.steps == nil {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.steps.Get(ctx, cmd.StepID)
		if err != nil {
			return runevents.RunCommandResult{}, err
		}
		if cmd.RunID != "" && cmd.RunID != item.RunID {
			return runevents.RunCommandResult{}, common.ErrConflict
		}
		if err := m.validateRunReadScope(ctx, item.RunID, cmd.WorkspaceID); err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{Step: item}, nil
	case runevents.TopicRunCommandListStepsByRun, runevents.TopicRunCommandListStepSummariesByRun:
		cmd, ok := data.(runevents.ListStepsByRunCommand)
		if !ok || cmd.RunID == "" || m.steps == nil {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.validateRunReadScope(ctx, cmd.RunID, cmd.WorkspaceID); err != nil {
			return runevents.RunCommandResult{}, err
		}
		var items []*runmodel.Step
		var err error
		if topic == runevents.TopicRunCommandListStepSummariesByRun {
			items, err = m.steps.ListByRunSummary(ctx, cmd.RunID)
		} else {
			items, err = m.steps.ListByRun(ctx, cmd.RunID)
		}
		if err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{Steps: items}, nil
	case runevents.TopicRunCommandGetRuntimeCheckpoint:
		cmd, ok := data.(runevents.GetRuntimeCheckpointCommand)
		if !ok || cmd.CheckpointID == "" || m.checkpoints == nil {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.checkpoints.Get(ctx, cmd.CheckpointID)
		if err != nil {
			return runevents.RunCommandResult{}, err
		}
		if cmd.RunID != "" && cmd.RunID != item.RunID {
			return runevents.RunCommandResult{}, common.ErrConflict
		}
		if cmd.WorkspaceID != "" && item.WorkspaceID != cmd.WorkspaceID {
			return runevents.RunCommandResult{}, common.ErrNotFound
		}
		if err := m.validateRunReadScope(ctx, item.RunID, cmd.WorkspaceID); err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{Checkpoint: item}, nil
	case runevents.TopicRunCommandListCheckpointsByRun:
		cmd, ok := data.(runevents.ListRuntimeCheckpointsByRunCommand)
		if !ok || cmd.RunID == "" || m.checkpoints == nil {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.validateRunReadScope(ctx, cmd.RunID, cmd.WorkspaceID); err != nil {
			return runevents.RunCommandResult{}, err
		}
		items, err := m.checkpoints.ListByRun(ctx, cmd.RunID)
		if err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{Checkpoints: items}, nil
	case runevents.TopicRunCommandUpdate:
		cmd, ok := data.(runevents.UpdateRunCommand)
		if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.runs.Update(ctx, runFromUpdateCommand(cmd)); err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{}, nil
	case runevents.TopicRunCommandDelete:
		cmd, ok := data.(runevents.DeleteRunCommand)
		if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if _, err := m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID); err != nil {
			return runevents.RunCommandResult{}, err
		}
		if err := m.runs.Delete(ctx, cmd.RunID); err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{}, nil
	default:
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
}

func runCommandWorkspaceID(data any) common.WorkspaceID {
	switch cmd := data.(type) {
	case runevents.CreateRunCommand:
		return cmd.WorkspaceID
	case runevents.GetRunCommand:
		return cmd.WorkspaceID
	case runevents.ListRunsByWorkspaceCommand:
		return cmd.WorkspaceID
	case runevents.GetStepCommand:
		return cmd.WorkspaceID
	case runevents.ListStepsByRunCommand:
		return cmd.WorkspaceID
	case runevents.GetRuntimeCheckpointCommand:
		return cmd.WorkspaceID
	case runevents.ListRuntimeCheckpointsByRunCommand:
		return cmd.WorkspaceID
	case runevents.UpdateRunCommand:
		return cmd.WorkspaceID
	case runevents.DeleteRunCommand:
		return cmd.WorkspaceID
	default:
		return ""
	}
}

func runStateFilter(states []common.RunState) map[common.RunState]struct{} {
	if len(states) == 0 {
		return nil
	}
	ret := map[common.RunState]struct{}{}
	for _, state := range states {
		if state != "" {
			ret[state] = struct{}{}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func setRunMagicResult(result magicevent.Result, data any, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func (m *Module) handleRunQueryCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.runs == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	if workspaceID := runScopedWorkspaceID(envelope); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	switch envelope.Topic {
	case runevents.TopicRunCommandEnsure:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(runcontracts.CreateRunCommand)
		if !ok {
			cmd = runcontracts.CreateRunCommand{
				RunID:       common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID)),
				WorkspaceID: common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)),
			}
			if cmd.RunID == "" {
				cmd.RunID = common.RunID(envelope.RunID)
			}
			if cmd.WorkspaceID == "" {
				cmd.WorkspaceID = common.WorkspaceID(envelope.WorkspaceID)
			}
		}
		normalized, err := normalizeRunCreateCommandScope(envelope, cmd)
		if err != nil {
			return err
		}
		cmd = normalized
		if err := m.servicePtr.EnsureRun(ctx, cmd); err != nil {
			return err
		}
		return runCommandResponse(envelope, nil)
	case runevents.TopicRunCommandCreate:
		if cmd, ok := envelope.Payload[events.PayloadKeyCommand].(runcontracts.CreateRunCommand); ok {
			normalized, err := normalizeRunCreateCommandScope(envelope, cmd)
			if err != nil {
				return err
			}
			cmd = normalized
			if err := m.servicePtr.CreateRun(ctx, cmd); err != nil {
				return err
			}
			return runCommandResponse(envelope, nil)
		}
		cmd, err := createRunCommandFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		normalized, err := normalizeRunCreateCommandScope(envelope, cmd)
		if err != nil {
			return err
		}
		cmd = normalized
		if err := m.servicePtr.CreateRun(ctx, cmd); err != nil {
			return err
		}
		return runCommandResponse(envelope, nil)
	case runevents.TopicRunCommandGet:
		runID := common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))
		if runID == "" {
			runID = common.RunID(envelope.RunID)
		}
		workspaceID := runScopedWorkspaceID(envelope)
		var item *runmodel.Run
		var err error
		if workspaceID != "" {
			item, err = m.runs.GetByWorkspaceID(ctx, workspaceID, runID)
		} else {
			item, err = m.runs.Get(ctx, runID)
		}
		if err != nil {
			return err
		}
		return runCommandResponse(envelope, map[string]any{events.PayloadKeyRun: item})
	case runevents.TopicRunCommandList:
		items, err := m.runs.List(ctx)
		if err != nil {
			return err
		}
		items = projectRunCommandListItems(items, runStateFilterFromPayload(envelope.Payload), boolPayload(envelope.Payload, events.PayloadKeyCompact))
		return runCommandResponse(envelope, map[string]any{events.PayloadKeyRuns: items})
	case runevents.TopicRunCommandListByWorkspace:
		items, err := m.runs.ListByWorkspaceID(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		items = projectRunCommandListItems(items, runStateFilterFromPayload(envelope.Payload), boolPayload(envelope.Payload, events.PayloadKeyCompact))
		return runCommandResponse(envelope, map[string]any{events.PayloadKeyRuns: items})
	case runevents.TopicRunCommandGetStep:
		if m.steps == nil {
			return common.ErrInvalidArgument
		}
		stepID := common.StepID(stringPayload(envelope.Payload, events.PayloadKeyStepID))
		if stepID == "" {
			stepID = common.StepID(envelope.StepID)
		}
		if stepID == "" {
			return common.ErrInvalidArgument
		}
		item, err := m.steps.Get(ctx, stepID)
		if err != nil {
			return err
		}
		if envelope.RunID != "" && common.RunID(envelope.RunID) != item.RunID {
			return common.ErrConflict
		}
		if err := m.validateRunReadScope(ctx, item.RunID, runScopedWorkspaceID(envelope)); err != nil {
			return err
		}
		return runCommandResponse(envelope, map[string]any{events.PayloadKeyStep: item})
	case runevents.TopicRunCommandListStepsByRun:
		if m.steps == nil {
			return common.ErrInvalidArgument
		}
		runID := common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))
		if runID == "" {
			runID = common.RunID(envelope.RunID)
		}
		if runID == "" {
			return common.ErrInvalidArgument
		}
		if err := m.validateRunReadScope(ctx, runID, runScopedWorkspaceID(envelope)); err != nil {
			return err
		}
		items, err := m.steps.ListByRun(ctx, runID)
		if err != nil {
			return err
		}
		return runCommandResponse(envelope, map[string]any{events.PayloadKeySteps: items})
	case runevents.TopicRunCommandListStepSummariesByRun:
		if m.steps == nil {
			return common.ErrInvalidArgument
		}
		runID := common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))
		if runID == "" {
			runID = common.RunID(envelope.RunID)
		}
		if runID == "" {
			return common.ErrInvalidArgument
		}
		if err := m.validateRunReadScope(ctx, runID, runScopedWorkspaceID(envelope)); err != nil {
			return err
		}
		items, err := m.steps.ListByRunSummary(ctx, runID)
		if err != nil {
			return err
		}
		return runCommandResponse(envelope, map[string]any{events.PayloadKeySteps: items})
	case runevents.TopicRunCommandGetRuntimeCheckpoint:
		if m.checkpoints == nil {
			return common.ErrInvalidArgument
		}
		checkpointID := common.CheckpointID(stringPayload(envelope.Payload, events.PayloadKeyCheckpointID))
		if checkpointID == "" {
			return common.ErrInvalidArgument
		}
		item, err := m.checkpoints.Get(ctx, checkpointID)
		if err != nil {
			return err
		}
		if envelope.RunID != "" && common.RunID(envelope.RunID) != item.RunID {
			return common.ErrConflict
		}
		workspaceID := runScopedWorkspaceID(envelope)
		if workspaceID != "" && item.WorkspaceID != workspaceID {
			return common.ErrNotFound
		}
		if err := m.validateRunReadScope(ctx, item.RunID, workspaceID); err != nil {
			return err
		}
		return runCommandResponse(envelope, map[string]any{events.PayloadKeyCheckpoint: item})
	case runevents.TopicRunCommandListCheckpointsByRun:
		if m.checkpoints == nil {
			return common.ErrInvalidArgument
		}
		runID := common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))
		if runID == "" {
			runID = common.RunID(envelope.RunID)
		}
		if runID == "" {
			return common.ErrInvalidArgument
		}
		if err := m.validateRunReadScope(ctx, runID, runScopedWorkspaceID(envelope)); err != nil {
			return err
		}
		items, err := m.checkpoints.ListByRun(ctx, runID)
		if err != nil {
			return err
		}
		return runCommandResponse(envelope, map[string]any{events.PayloadKeyCheckpoints: items})
	case runevents.TopicRunCommandUpdate:
		var item *runmodel.Run
		if cmd, ok := envelope.Payload[events.PayloadKeyCommand].(runcontracts.UpdateRunCommand); ok {
			item = runFromUpdateCommand(cmd)
		} else {
			var err error
			item, err = runFromPayload(envelope.Payload)
			if err != nil {
				return err
			}
		}
		if err := validateRunWriteScope(envelope, item.ID, item.WorkspaceID); err != nil {
			return err
		}
		if err := m.runs.Update(ctx, item); err != nil {
			return err
		}
		return runCommandResponse(envelope, nil)
	case runevents.TopicRunCommandDelete:
		runID := deleteRunIDFromPayload(envelope.Payload, envelope.RunID)
		workspaceID := runScopedWorkspaceID(envelope)
		if workspaceID == "" || runID == "" {
			return common.ErrInvalidArgument
		}
		if _, err := m.runs.GetByWorkspaceID(ctx, workspaceID, runID); err != nil {
			return err
		}
		if err := m.runs.Delete(ctx, runID); err != nil {
			return err
		}
		return runCommandResponse(envelope, nil)
	default:
		return common.ErrInvalidArgument
	}
}

func normalizeRunCreateCommandScope(envelope events.Envelope, cmd runcontracts.CreateRunCommand) (runcontracts.CreateRunCommand, error) {
	if cmd.RunID == "" {
		cmd.RunID = common.RunID(envelope.RunID)
	}
	scopedWorkspaceID := runScopedWorkspaceID(envelope)
	if cmd.WorkspaceID == "" {
		cmd.WorkspaceID = scopedWorkspaceID
	}
	if cmd.RunID == "" || cmd.WorkspaceID == "" {
		return runcontracts.CreateRunCommand{}, common.ErrInvalidArgument
	}
	if scopedWorkspaceID != "" && cmd.WorkspaceID != scopedWorkspaceID {
		return runcontracts.CreateRunCommand{}, common.ErrConflict
	}
	return cmd, nil
}

func validateRunWriteScope(envelope events.Envelope, runID common.RunID, workspaceID common.WorkspaceID) error {
	scopedWorkspaceID := runScopedWorkspaceID(envelope)
	if runID == "" || workspaceID == "" || scopedWorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	if envelope.RunID != "" && common.RunID(envelope.RunID) != runID {
		return common.ErrConflict
	}
	if scopedWorkspaceID != workspaceID {
		return common.ErrConflict
	}
	return nil
}

func (m *Module) validateRunReadScope(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	if runID == "" || workspaceID == "" {
		return nil
	}
	_, err := m.runs.GetByWorkspaceID(ctx, workspaceID, runID)
	return err
}

func runFromPayload(payload map[string]any) (*runmodel.Run, error) {
	item, ok := payload[events.PayloadKeyRun].(*runmodel.Run)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func runFromUpdateCommand(cmd runcontracts.UpdateRunCommand) *runmodel.Run {
	return &runmodel.Run{
		ID:                 cmd.RunID,
		WorkspaceID:        cmd.WorkspaceID,
		GoalID:             cmd.GoalID,
		DoDID:              cmd.DoDID,
		DefinitionRevision: cmd.DefinitionRevision,
		InitialStepKind:    cmd.InitialStepKind,
		ExecutionContext:   cloneMap(cmd.ExecutionContext),
		State:              cmd.State,
		Priority:           cmd.Priority,
		BlockingTicketID:   cmd.BlockingTicketID,
		BlockingApprovalID: cmd.BlockingApprovalID,
		FailureReason:      cmd.FailureReason,
	}
}

func deleteRunIDFromPayload(payload map[string]any, envelopeRunID string) common.RunID {
	if cmd, ok := payload[events.PayloadKeyCommand].(runcontracts.DeleteRunCommand); ok && cmd.RunID != "" {
		return cmd.RunID
	}
	id := common.RunID(stringPayload(payload, events.PayloadKeyRunID))
	if id != "" {
		return id
	}
	return common.RunID(envelopeRunID)
}

func createRunCommandFromPayload(payload map[string]any) (runcontracts.CreateRunCommand, error) {
	if item, err := runFromPayload(payload); err == nil {
		return runcontracts.CreateRunCommand{
			RunID:              item.ID,
			WorkspaceID:        item.WorkspaceID,
			GoalID:             item.GoalID,
			DoDID:              item.DoDID,
			DefinitionRevision: item.DefinitionRevision,
			InitialStepKind:    item.InitialStepKind,
			ExecutionContext:   cloneMap(item.ExecutionContext),
			Priority:           item.Priority,
		}, nil
	}
	cmd := runcontracts.CreateRunCommand{
		RunID:              common.RunID(stringPayload(payload, events.PayloadKeyRunID)),
		WorkspaceID:        common.WorkspaceID(stringPayload(payload, events.PayloadKeyWorkspaceID)),
		GoalID:             common.GoalID(stringPayload(payload, events.PayloadKeyGoalID)),
		DoDID:              common.DoDID(stringPayload(payload, events.PayloadKeyDoDID)),
		InitialStepKind:    stringPayload(payload, events.PayloadKeyKind),
		ExecutionContext:   mapPayload(payload, events.PayloadKeyExecutionContext),
		DefinitionRevision: stringPayload(payload, events.PayloadKeyDefinitionRevision),
	}
	if priority, ok := payload[events.PayloadKeyPriority].(int); ok {
		cmd.Priority = priority
	}
	if cmd.RunID == "" || cmd.WorkspaceID == "" {
		return runcontracts.CreateRunCommand{}, common.ErrInvalidArgument
	}
	return cmd, nil
}

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func boolPayload(payload map[string]any, key string) bool {
	value, _ := payload[key].(bool)
	return value
}

func runStateFilterFromPayload(payload map[string]any) map[common.RunState]struct{} {
	raw, ok := payload[events.PayloadKeyStates]
	if !ok {
		return nil
	}
	values := make([]string, 0)
	switch typed := raw.(type) {
	case []string:
		values = append(values, typed...)
	case []common.RunState:
		for _, item := range typed {
			values = append(values, string(item))
		}
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
	}
	if len(values) == 0 {
		return nil
	}
	ret := map[common.RunState]struct{}{}
	for _, value := range values {
		if value == "" {
			continue
		}
		ret[common.RunState(value)] = struct{}{}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func projectRunCommandListItems(items []*runmodel.Run, states map[common.RunState]struct{}, compact bool) []*runmodel.Run {
	if len(items) == 0 {
		return items
	}
	ret := make([]*runmodel.Run, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if len(states) > 0 {
			if _, ok := states[item.State]; !ok {
				continue
			}
		}
		if compact {
			ret = append(ret, compactRunListItem(item))
			continue
		}
		ret = append(ret, item)
	}
	return ret
}

func compactRunListItem(item *runmodel.Run) *runmodel.Run {
	if item == nil {
		return nil
	}
	return &runmodel.Run{
		ID:                 item.ID,
		WorkspaceID:        item.WorkspaceID,
		GoalID:             item.GoalID,
		DoDID:              item.DoDID,
		Lineage:            item.Lineage,
		DefinitionRevision: item.DefinitionRevision,
		InitialStepKind:    item.InitialStepKind,
		State:              item.State,
		Priority:           item.Priority,
		BlockingTicketID:   item.BlockingTicketID,
		BlockingApprovalID: item.BlockingApprovalID,
		FailureReason:      item.FailureReason,
		CreatedAt:          item.CreatedAt,
		StartedAt:          item.StartedAt,
		EndedAt:            item.EndedAt,
	}
}

func runScopedWorkspaceID(envelope events.Envelope) common.WorkspaceID {
	workspaceID := common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID))
	if workspaceID == "" {
		workspaceID = common.WorkspaceID(envelope.WorkspaceID)
	}
	return workspaceID
}

func mapPayload(payload map[string]any, key string) map[string]any {
	value, _ := payload[key].(map[string]any)
	return cloneMap(value)
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func runCommandResponse(request events.Envelope, payload map[string]any) error {
	return events.Response(events.Envelope{
		Topic:        request.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  request.SubjectType,
		SubjectID:    request.SubjectID,
		WorkspaceID:  request.WorkspaceID,
		RunID:        request.RunID,
		StepID:       request.StepID,
		Payload:      payload,
	})
}
