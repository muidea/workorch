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

func (m *Module) validateRunReadScope(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	if runID == "" || workspaceID == "" {
		return nil
	}
	_, err := m.runs.GetByWorkspaceID(ctx, workspaceID, runID)
	return err
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
