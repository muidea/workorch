package run

import (
	"context"
	"sort"
	"strings"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/run/pkg/common"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID

type CreateRunCommand = runcontracts.CreateRunCommand
type StartRunCommand = runcontracts.StartRunCommand
type ResumeRunCommand = runcontracts.ResumeRunCommand
type UpdateRunCommand = runcontracts.UpdateRunCommand
type BlockOnDecisionCommand = runcontracts.BlockOnDecisionCommand
type BlockOnApprovalCommand = runcontracts.BlockOnApprovalCommand
type CancelRunCommand = runcontracts.CancelRunCommand
type SupersedeRunCommand = runcontracts.SupersedeRunCommand
type FailRunCommand = runcontracts.FailRunCommand

type Dependencies struct {
	Runs              repository.RunRepository
	Steps             repository.StepRepository
	Checkpoints       repository.RuntimeCheckpointRepository
	EventHub          magicevent.Hub
	BackgroundRoutine magictask.BackgroundRoutine
}

type Biz struct {
	// commandMu only coordinates in-flight EventHub owner commands with Stop.
	// Framework serializes Configure/Start/Stop lifecycle calls.
	commandMu sync.RWMutex
	stopping  bool
	*Service
	runs          repository.RunRepository
	steps         repository.StepRepository
	checkpoints   repository.RuntimeCheckpointRepository
	topics        []string
	recoveryReady bool
	started       bool
}

func New() *Biz { return &Biz{} }

func (m *Biz) ID() string { return ModuleID }

func (m *Biz) Configure(deps Dependencies) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	if deps.Runs == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.stopping = false
	m.runs = deps.Runs
	m.steps = deps.Steps
	m.checkpoints = deps.Checkpoints
	m.Service = NewServiceWithEventHub(deps.Runs, deps.EventHub, deps.BackgroundRoutine)
	// Configure is invoked exclusively from Setup (or explicit test assembly).
	// Establish the SM-4 downstream read contract before any later module Run.
	return m.prepareRuntimeV2RecoveryInventory()
}

func (m *Biz) Start() error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	m.commandMu.Lock()
	defer m.commandMu.Unlock()
	if m.stopping || m.Service == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	if err := m.ensureRuntimeV2RecoveryInventoryLocked(); err != nil {
		return err
	}
	regularTopics := []string{
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
		runevents.TopicRunCommandPurgeRuntimeResources,
		runevents.TopicRunCommandUpdate,
		runevents.TopicRunCommandLinkSuccessor,
		runevents.TopicRunCommandDelete,
	}
	for _, topic := range regularTopics {
		m.SubscribeFunc(topic, m.handleCommand)
	}
	m.topics = append(m.topics, regularTopics...)
	m.started = true
	return nil
}

// prepareRuntimeV2RecoveryInventory establishes the narrow SM-4 recovery read
// contract during Setup. Regular Run owner commands remain Run-phase concerns.
func (m *Biz) prepareRuntimeV2RecoveryInventory() error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	m.commandMu.Lock()
	defer m.commandMu.Unlock()
	if m.stopping || m.Service == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureRuntimeV2RecoveryInventoryLocked()
}

// PrepareRuntimeV2RecoveryInventory establishes the SM-4 read subscription
// during framework Setup, before Orchestrator starts startup recovery.
func (m *Biz) PrepareRuntimeV2RecoveryInventory() error {
	return m.prepareRuntimeV2RecoveryInventory()
}

func (m *Biz) ensureRuntimeV2RecoveryInventoryLocked() error {
	if m.recoveryReady {
		return nil
	}
	m.SubscribeFunc(runevents.TopicRunCommandRuntimeV2RecoveryInventory, m.handleRuntimeV2RecoveryInventory)
	m.topics = append(m.topics, runevents.TopicRunCommandRuntimeV2RecoveryInventory)
	m.recoveryReady = true
	return nil
}

func (m *Biz) handleRuntimeV2RecoveryInventory(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	cmd, ok := ev.Data().(runevents.QueryRuntimeV2RecoveryInventoryCommand)
	if !ok || m == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	runs, err := m.runs.List(ev.Context())
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(buildRuntimeV2RecoveryInventory(runs, cmd), nil)
}

func buildRuntimeV2RecoveryInventory(runs []*runmodel.Run, cmd runevents.QueryRuntimeV2RecoveryInventoryCommand) runevents.QueryRuntimeV2RecoveryInventoryResult {
	limit := cmd.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	cursor := strings.TrimSpace(cmd.Cursor)
	items := make([]runevents.RuntimeV2RecoveryInventoryItem, 0)
	for _, run := range runs {
		if run == nil || run.RuntimeContractVersion != runtimemodel.RuntimeContractVersionV2 || run.RuntimeState == nil {
			continue
		}
		key := string(run.WorkspaceID) + "\x00" + string(run.ID)
		if cursor != "" && key <= cursor {
			continue
		}
		state := run.RuntimeState
		item := runevents.RuntimeV2RecoveryInventoryItem{
			WorkspaceID: run.WorkspaceID, RunID: run.ID, StateRevision: state.StateRevision,
			BlockingTicketID: run.BlockingTicketID, BlockingApprovalID: run.BlockingApprovalID,
			TerminalDeliveryAttempts: state.Facts.Recovery.TerminalDeliveryAttempts,
		}
		if state.ActiveDispatch != nil {
			item.ActiveDispatchID = state.ActiveDispatch.ID
		}
		switch {
		case state.Decision.Action == runtimemodel.RuntimeActionComplete || state.Decision.Action == runtimemodel.RuntimeActionFail:
			if !runStateIsTerminal(run.State) {
				item.Class = runevents.RuntimeV2RecoveryTerminalDeliveryPending
			}
		case run.State == common.RunStateBlockedOnDecision || run.State == common.RunStateBlockedOnApproval:
			item.Class = runevents.RuntimeV2RecoveryBlockedGovernance
		case run.State == common.RunStateActive && state.ActiveDispatch != nil:
			item.Class = runevents.RuntimeV2RecoveryActiveSealedDispatch
		case run.State == common.RunStateActive:
			item.Class = runevents.RuntimeV2RecoveryActiveNoDispatch
		}
		if item.Class != "" {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		left := string(items[i].WorkspaceID) + "\x00" + string(items[i].RunID)
		right := string(items[j].WorkspaceID) + "\x00" + string(items[j].RunID)
		return left < right
	})
	ret := runevents.QueryRuntimeV2RecoveryInventoryResult{}
	if len(items) <= limit {
		ret.Items = items
		return ret
	}
	ret.Items = items[:limit]
	last := ret.Items[len(ret.Items)-1]
	ret.NextCursor = string(last.WorkspaceID) + "\x00" + string(last.RunID)
	return ret
}

// BuildRuntimeV2RecoveryInventory is the Run owner's typed SM-4 inventory
// projection. It does not select actions or drive Runtime V2.
func BuildRuntimeV2RecoveryInventory(runs []*runmodel.Run, cmd runevents.QueryRuntimeV2RecoveryInventoryCommand) runevents.QueryRuntimeV2RecoveryInventoryResult {
	return buildRuntimeV2RecoveryInventory(runs, cmd)
}

func runStateIsTerminal(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func (m *Biz) Stop(ctx context.Context) error {
	_ = ctx
	if m == nil {
		return nil
	}
	m.commandMu.Lock()
	owner := m.Service
	topics := append([]string(nil), m.topics...)
	m.stopping = true
	m.topics = nil
	m.recoveryReady = false
	m.started = false
	m.runs = nil
	m.steps = nil
	m.checkpoints = nil
	m.commandMu.Unlock()
	if owner != nil {
		for idx := len(topics) - 1; idx >= 0; idx-- {
			owner.UnsubscribeFunc(topics[idx])
		}
	}
	m.commandMu.Lock()
	m.Service = nil
	m.commandMu.Unlock()
	return nil
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setRunMagicResult(result, response, err)
}

func (m *Biz) executeCommand(ev magicevent.Event) (runevents.RunCommandResult, error) {
	if m == nil {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	ctx := ev.Context()
	topic := ev.ID()
	data := ev.Data()
	if m.stopping || m.runs == nil || m.Service == nil {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	workspaceID := common.WorkspaceID("")
	switch cmd := data.(type) {
	case runevents.CreateRunCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.GetRunCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.ListRunsByWorkspaceCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.GetStepCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.ListStepsByRunCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.GetRuntimeCheckpointCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.ListRuntimeCheckpointsByRunCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.PurgeRuntimeResourcesCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.UpdateRunCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.LinkRunSuccessorCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.DeleteRunCommand:
		workspaceID = cmd.WorkspaceID
	}
	if workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	switch topic {
	case runevents.TopicRunCommandEnsure:
		cmd, ok := data.(runevents.EnsureRunCommand)
		if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.EnsureRun(ctx, cmd); err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{}, nil
	case runevents.TopicRunCommandCreate:
		cmd, ok := data.(runevents.CreateRunCommand)
		if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.CreateRun(ctx, cmd); err != nil {
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
	case runevents.TopicRunCommandPurgeRuntimeResources:
		cmd, ok := data.(runevents.PurgeRuntimeResourcesCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.RunID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if _, err := m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID); err != nil {
			return runevents.RunCommandResult{}, err
		}
		deleted, err := purgeRunRuntimeResources(ctx, m.steps, m.checkpoints, cmd.RunID)
		return runevents.RunCommandResult{DeletedCount: deleted}, err
	case runevents.TopicRunCommandLinkSuccessor:
		cmd, ok := data.(runevents.LinkRunSuccessorCommand)
		if !ok || cmd.WorkspaceID == "" || cmd.PredecessorRunID == "" || cmd.SuccessorRunID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.LinkRunSuccessor(ctx, cmd); err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{}, nil
	case runevents.TopicRunCommandUpdate:
		cmd, ok := data.(runevents.UpdateRunCommand)
		if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		current, err := m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID)
		if err != nil {
			return runevents.RunCommandResult{}, err
		}
		if cmd.RuntimeContractVersion == runtimemodel.RuntimeContractVersionV2 {
			if cmd.RuntimeState == nil || cmd.ExpectedRuntimeStateRevision == nil || cmd.RuntimeState.Facts.WorkspaceID != cmd.WorkspaceID || cmd.RuntimeState.Facts.RunID != cmd.RunID {
				return runevents.RunCommandResult{}, common.ErrInvalidArgument
			}
			if err := cmd.RuntimeState.Validate(); err != nil {
				return runevents.RunCommandResult{}, err
			}
		} else if cmd.RuntimeState != nil {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if err := validateRunUpdate(current, cmd); err != nil {
			return runevents.RunCommandResult{}, err
		}
		if cmd.ExpectedRuntimeStateRevision != nil {
			currentRevision := uint64(0)
			if current.RuntimeState != nil {
				currentRevision = current.RuntimeState.StateRevision
			}
			if currentRevision != *cmd.ExpectedRuntimeStateRevision {
				return runevents.RunCommandResult{}, common.ErrConflict
			}
		}
		item := runFromUpdateCommand(cmd)
		if err := m.runs.Update(ctx, item); err != nil {
			return runevents.RunCommandResult{}, err
		}
		m.publishRunUpdated(ctx, item)
		return runevents.RunCommandResult{Run: item}, nil
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

func purgeRunRuntimeResources(ctx context.Context, steps repository.StepRepository, checkpoints repository.RuntimeCheckpointRepository, runID common.RunID) (int, error) {
	if runID == "" {
		return 0, common.ErrInvalidArgument
	}
	deleted := 0
	if steps != nil {
		items, err := steps.ListByRun(ctx, runID)
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := steps.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	if checkpoints != nil {
		items, err := checkpoints.ListByRun(ctx, runID)
		if err != nil {
			return deleted, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if err := checkpoints.Delete(ctx, item.ID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}

func (m *Biz) publishRunUpdated(ctx context.Context, item *runmodel.Run) {
	if m == nil || m.Service == nil || m.EventHub() == nil || item == nil {
		return
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunStateChanged, modulecommon.ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, runevents.RunStateChangedDataFromModel(item))
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(event.ID(), workorchevents.EventHubLaneScope{SubjectType: "run", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.ID)}))
	m.PostEvent(event)
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

func setRunMagicResult(result magicevent.Result, data runevents.RunCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func (m *Biz) validateRunReadScope(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID) error {
	if runID == "" || workspaceID == "" {
		return nil
	}
	_, err := m.runs.GetByWorkspaceID(ctx, workspaceID, runID)
	return err
}

func runFromUpdateCommand(cmd runcontracts.UpdateRunCommand) *runmodel.Run {
	return &runmodel.Run{
		ID:                     cmd.RunID,
		WorkspaceID:            cmd.WorkspaceID,
		GoalID:                 cmd.GoalID,
		DoDID:                  cmd.DoDID,
		Lineage:                cmd.Lineage,
		DefinitionRevision:     cmd.DefinitionRevision,
		InitialStepKind:        cmd.InitialStepKind,
		RuntimeBootstrap:       cloneRuntimeBootstrap(cmd.RuntimeBootstrap),
		RuntimeContractVersion: cmd.RuntimeContractVersion,
		RuntimeState:           cloneRuntimeStateRecord(cmd.RuntimeState),
		State:                  cmd.State,
		Priority:               cmd.Priority,
		BlockingTicketID:       cmd.BlockingTicketID,
		BlockingApprovalID:     cmd.BlockingApprovalID,
		FailureReason:          cmd.FailureReason,
		CreatedAt:              cmd.CreatedAt,
		StartedAt:              cmd.StartedAt,
		EndedAt:                cmd.EndedAt,
	}
}

func validateRunUpdate(current *runmodel.Run, cmd runcontracts.UpdateRunCommand) error {
	if current == nil || current.ID != cmd.RunID || current.WorkspaceID != cmd.WorkspaceID {
		return common.ErrConflict
	}
	if current.GoalID != cmd.GoalID || current.DoDID != cmd.DoDID ||
		current.DefinitionRevision != cmd.DefinitionRevision || current.InitialStepKind != cmd.InitialStepKind ||
		current.CreatedAt != cmd.CreatedAt {
		return common.ErrConflict
	}
	if !sameRunRuntimeBootstrap(current.RuntimeBootstrap, cmd.RuntimeBootstrap) {
		return common.ErrConflict
	}
	if current.RuntimeContractVersion == runtimemodel.RuntimeContractVersionV2 && cmd.RuntimeContractVersion != runtimemodel.RuntimeContractVersionV2 {
		return common.ErrConflict
	}
	if current.RuntimeState != nil && cmd.RuntimeState == nil {
		return common.ErrConflict
	}
	if cmd.State != current.State && !common.RunStateCanTransition(current.State, cmd.State) {
		return common.ErrConflict
	}
	if err := validateRunBlockerState(cmd); err != nil {
		return err
	}
	if cmd.RuntimeContractVersion != runtimemodel.RuntimeContractVersionV2 {
		return nil
	}
	if cmd.RuntimeBootstrap == nil || cmd.RuntimeState == nil {
		return common.ErrConflict
	}
	bootstrap := cmd.RuntimeBootstrap.Normalized()
	state := cmd.RuntimeState.Clone()
	if bootstrap.Definition.GoalID != cmd.GoalID || bootstrap.Definition.DoDID != cmd.DoDID || bootstrap.Definition.Revision != cmd.DefinitionRevision ||
		state.Definition.GoalID != cmd.GoalID || state.Definition.DoDID != cmd.DoDID || state.Definition.Revision != cmd.DefinitionRevision ||
		!runtimemodel.SameRuntimeDefinitionContext(bootstrap.Definition, state.Definition) ||
		!runtimemodel.SameRuntimeActivationContext(bootstrap.Activation, state.Facts.Activation) {
		return common.ErrConflict
	}
	return nil
}

// ValidateRunUpdate exposes the Run owner's SM-1 persistence invariant for
// focused contract tests; it does not alter owner state.
func ValidateRunUpdate(current *runmodel.Run, cmd runcontracts.UpdateRunCommand) error {
	return validateRunUpdate(current, cmd)
}

func validateRunBlockerState(cmd runcontracts.UpdateRunCommand) error {
	switch cmd.State {
	case common.RunStateBlockedOnDecision:
		if cmd.BlockingTicketID == "" || cmd.BlockingApprovalID != "" {
			return common.ErrConflict
		}
	case common.RunStateBlockedOnApproval:
		if cmd.BlockingApprovalID == "" || cmd.BlockingTicketID != "" {
			return common.ErrConflict
		}
	default:
		if cmd.BlockingTicketID != "" || cmd.BlockingApprovalID != "" {
			return common.ErrConflict
		}
	}
	return nil
}

func sameRunRuntimeBootstrap(left *runtimemodel.RuntimeBootstrap, right *runtimemodel.RuntimeBootstrap) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return runtimemodel.SameRuntimeBootstrap(*left, *right)
}

func cloneRuntimeStateRecord(value *runtimemodel.RuntimeStateRecord) *runtimemodel.RuntimeStateRecord {
	if value == nil {
		return nil
	}
	cloned := value.Clone()
	return &cloned
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
