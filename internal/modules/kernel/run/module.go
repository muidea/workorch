package run

import (
	"context"

	runbiz "github.com/muidea/workorch/internal/modules/kernel/run/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/run/pkg/common"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
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
	EventHub     events.Hub
}

type Module struct {
	bizPtr      *runbiz.Service
	servicePtr  *runservice.Module
	runs        repository.RunRepository
	steps       repository.StepRepository
	checkpoints repository.RuntimeCheckpointRepository
	eventHub    events.Hub
	unsub       []func()
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
	workspaces := deps.Workspaces
	if workspaces == nil {
		workspaces = workspaceEventReader{hub: deps.EventHub}
	}
	orchestrator := deps.Orchestrator
	if orchestrator == nil {
		orchestrator = NewEventOrchestrator(deps.EventHub)
	}
	m.bizPtr = runbiz.NewServiceWithWorkspaceReader(deps.Runs, workspaces, orchestrator, deps.EventHub)
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
	if len(m.unsub) > 0 {
		return nil
	}
	m.unsub = append(m.unsub,
		m.eventHub.Subscribe(events.TopicRunCommandEnsureResumed, m.handleEnsureResumedCommand),
		m.eventHub.Subscribe(events.TopicRunCommandEnsure, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandCreate, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandGet, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandList, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandListByWorkspace, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandGetStep, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandListStepsByRun, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandGetRuntimeCheckpoint, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandListCheckpointsByRun, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandUpdate, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandDelete, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandStart, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandSupersede, m.handleRunQueryCommand),
		m.eventHub.Subscribe(events.TopicRunCommandFail, m.handleRunQueryCommand),
	)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	for idx := len(m.unsub) - 1; idx >= 0; idx-- {
		if m.unsub[idx] != nil {
			m.unsub[idx]()
		}
	}
	m.unsub = nil
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

func (m *Module) handleEnsureResumedCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	cmd, ok := envelope.Payload[events.PayloadKeyCommand].(runcontracts.ResumeRunCommand)
	if !ok {
		cmd = runcontracts.ResumeRunCommand{
			RunID:       common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID)),
			WorkspaceID: common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)),
		}
	}
	if cmd.RunID == "" {
		cmd.RunID = common.RunID(envelope.RunID)
	}
	if cmd.WorkspaceID == "" {
		cmd.WorkspaceID = common.WorkspaceID(envelope.WorkspaceID)
	}
	if err := m.servicePtr.EnsureResumed(ctx, cmd); err != nil {
		return err
	}
	return events.Response(events.Envelope{Topic: envelope.Topic + "/response", SourceModule: ModuleID, SubjectType: envelope.SubjectType, SubjectID: envelope.SubjectID, WorkspaceID: envelope.WorkspaceID, RunID: string(cmd.RunID), StepID: envelope.StepID})
}

func (m *Module) handleRunQueryCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.runs == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	switch envelope.Topic {
	case events.TopicRunCommandEnsure:
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
		if err := m.servicePtr.EnsureRun(ctx, cmd); err != nil {
			return err
		}
		return runCommandResponse(envelope, nil)
	case events.TopicRunCommandCreate:
		if cmd, ok := envelope.Payload[events.PayloadKeyCommand].(runcontracts.CreateRunCommand); ok {
			if err := m.servicePtr.CreateRun(ctx, cmd); err != nil {
				return err
			}
			return runCommandResponse(envelope, nil)
		}
		cmd, err := createRunCommandFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.servicePtr.CreateRun(ctx, cmd); err != nil {
			return err
		}
		return runCommandResponse(envelope, nil)
	case events.TopicRunCommandGet:
		runID := common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))
		if runID == "" {
			runID = common.RunID(envelope.RunID)
		}
		workspaceID := common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID))
		if workspaceID == "" {
			workspaceID = common.WorkspaceID(envelope.WorkspaceID)
		}
		item, err := m.runs.Get(ctx, runID)
		if err != nil {
			return err
		}
		if workspaceID != "" && item.WorkspaceID != workspaceID {
			return common.ErrNotFound
		}
		return runCommandResponse(envelope, map[string]any{events.PayloadKeyRun: item})
	case events.TopicRunCommandList:
		items, err := m.runs.List(ctx)
		if err != nil {
			return err
		}
		return runCommandResponse(envelope, map[string]any{events.PayloadKeyRuns: items})
	case events.TopicRunCommandListByWorkspace:
		items, err := m.runs.ListByWorkspaceID(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return runCommandResponse(envelope, map[string]any{events.PayloadKeyRuns: items})
	case events.TopicRunCommandGetStep:
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
		return runCommandResponse(envelope, map[string]any{events.PayloadKeyStep: item})
	case events.TopicRunCommandListStepsByRun:
		if m.steps == nil {
			return common.ErrInvalidArgument
		}
		runID := common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))
		if runID == "" {
			runID = common.RunID(envelope.RunID)
		}
		items, err := m.steps.ListByRun(ctx, runID)
		if err != nil {
			return err
		}
		return runCommandResponse(envelope, map[string]any{events.PayloadKeySteps: items})
	case events.TopicRunCommandGetRuntimeCheckpoint:
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
		return runCommandResponse(envelope, map[string]any{events.PayloadKeyCheckpoint: item})
	case events.TopicRunCommandListCheckpointsByRun:
		if m.checkpoints == nil {
			return common.ErrInvalidArgument
		}
		runID := common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))
		if runID == "" {
			runID = common.RunID(envelope.RunID)
		}
		items, err := m.checkpoints.ListByRun(ctx, runID)
		if err != nil {
			return err
		}
		return runCommandResponse(envelope, map[string]any{events.PayloadKeyCheckpoints: items})
	case events.TopicRunCommandUpdate:
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
		if err := m.runs.Update(ctx, item); err != nil {
			return err
		}
		return runCommandResponse(envelope, nil)
	case events.TopicRunCommandDelete:
		runID := deleteRunIDFromPayload(envelope.Payload, envelope.RunID)
		if err := m.runs.Delete(ctx, runID); err != nil {
			return err
		}
		return runCommandResponse(envelope, nil)
	case events.TopicRunCommandStart:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(runcontracts.StartRunCommand)
		if !ok {
			cmd = runcontracts.StartRunCommand{RunID: common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))}
		}
		if err := m.servicePtr.EnsureStarted(ctx, cmd); err != nil {
			return err
		}
		return runCommandResponse(envelope, nil)
	case events.TopicRunCommandSupersede:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(runcontracts.SupersedeRunCommand)
		if !ok {
			cmd = runcontracts.SupersedeRunCommand{RunID: common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID))}
		}
		if err := m.servicePtr.SupersedeRun(ctx, cmd); err != nil {
			return err
		}
		return runCommandResponse(envelope, nil)
	case events.TopicRunCommandFail:
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(runcontracts.FailRunCommand)
		if !ok {
			cmd = runcontracts.FailRunCommand{
				RunID:  common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID)),
				Reason: stringPayload(envelope.Payload, events.PayloadKeyFailureReason),
			}
		}
		if err := m.servicePtr.FailRun(ctx, cmd); err != nil {
			return err
		}
		return runCommandResponse(envelope, nil)
	default:
		return common.ErrInvalidArgument
	}
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
