package run

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	runcontracts "github.com/muidea/workorch/internal/modules/blocks/run/pkg/contracts"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID

type CreateRunCommand = runcontracts.CreateRunCommand
type UpdateRunCommand = runcontracts.UpdateRunCommand

type Dependencies struct {
	Runs              repository.RunRepository
	Steps             repository.StepRepository
	Checkpoints       repository.RuntimeCheckpointRepository
	EventHub          magicevent.Hub
	BackgroundRoutine magictask.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
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
	m.Base = basebiz.New(modulecommon.ModuleID, deps.EventHub, deps.BackgroundRoutine)
	m.Service = newServiceWithPorts(
		deps.Runs,
		workspaceEventReader{send: m.SendEvent},
		eventRunStateNotifier{post: m.PostEvent},
	)
	// Configure is invoked exclusively from Setup (or explicit test assembly).
	// Establish the SM-4 downstream read contract before any later module Run.
	return m.prepareRuntimeRecoveryInventory()
}

func (m *Biz) Start() error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	m.commandMu.Lock()
	if m.stopping || m.Service == nil || m.EventHub() == nil {
		m.commandMu.Unlock()
		return common.ErrInvalidArgument
	}
	if m.started {
		m.commandMu.Unlock()
		return nil
	}
	if err := m.ensureRuntimeRecoveryInventoryLocked(); err != nil {
		m.commandMu.Unlock()
		return err
	}
	regularTopics := []string{
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
		runevents.TopicRunCommandDelete,
	}
	for _, topic := range regularTopics {
		m.SubscribeFunc(topic, m.handleCommand)
	}
	m.topics = append(m.topics, regularTopics...)
	for _, topic := range m.lifecycleTopics() {
		m.SubscribeFunc(topic, m.handleLifecycleCommand)
		m.topics = append(m.topics, topic)
	}
	m.SubscribeFunc(runevents.TopicRunCommandWorkspaceExecutionIntent, m.handleWorkspaceExecutionIntent)
	m.topics = append(m.topics, runevents.TopicRunCommandWorkspaceExecutionIntent)
	m.SubscribeMatchedFunc(ModuleID+"/replacement-activation", runevents.NotificationDestination, runevents.TopicRunStateChanged, m.handleReplacementPredecessorState)
	m.started = true
	m.commandMu.Unlock()
	// A crash may occur after the replacement successor is persisted but before
	// the predecessor terminal notification reaches this process. Re-scan only
	// Run-owned pending activation records; the helper posts owner-local
	// activation commands and never reconstructs Runtime authority.
	m.recoverPendingWorkspaceExecutionSuccessors()
	// The readiness fact is published only after all regular Run commands are
	// subscribed. Startup recovery may have loaded opaque state during Setup,
	// but it must not drive (and therefore save aggregate state) before this
	// owner-local readiness boundary is true.
	m.publishRuntimeCommandsReady()
	return nil
}

// prepareRuntimeRecoveryInventory establishes the narrow SM-4 recovery read
// contract during Setup. Regular Run owner commands remain Run-phase concerns.
func (m *Biz) prepareRuntimeRecoveryInventory() error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	m.commandMu.Lock()
	defer m.commandMu.Unlock()
	if m.stopping || m.Service == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureRuntimeRecoveryInventoryLocked()
}

// PrepareRuntimeRecoveryInventory establishes the SM-4 read subscription
// during framework Setup, before Orchestrator starts startup recovery.
func (m *Biz) PrepareRuntimeRecoveryInventory() error {
	return m.prepareRuntimeRecoveryInventory()
}

func (m *Biz) ensureRuntimeRecoveryInventoryLocked() error {
	if m.recoveryReady {
		return nil
	}
	m.SubscribeFunc(runevents.TopicRunCommandRuntimeRecoveryInventory, m.handleRuntimeRecoveryInventory)
	m.SubscribeFunc(runevents.TopicRunCommandRuntimeStateRecordReadiness, m.handleRuntimeStateRecordReadiness)
	m.SubscribeFunc(runevents.TopicRunCommandRuntimeStateRecord, m.handleRuntimeStateRecord)
	m.topics = append(m.topics,
		runevents.TopicRunCommandRuntimeRecoveryInventory,
		runevents.TopicRunCommandRuntimeStateRecordReadiness,
		runevents.TopicRunCommandRuntimeStateRecord,
	)
	m.recoveryReady = true
	return nil
}

func (m *Biz) handleRuntimeRecoveryInventory(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	cmd, ok := ev.Data().(runevents.QueryRuntimeRecoveryInventoryCommand)
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
	result.Set(buildRuntimeRecoveryInventory(runs, cmd), nil)
}

func (m *Biz) handleRuntimeStateRecordReadiness(_ magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if m == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.Service == nil || m.EventHub() == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	result.Set(runevents.QueryRuntimeStateRecordReadinessResult{Ready: m.started}, nil)
}

func (m *Biz) handleRuntimeStateRecord(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	cmd, ok := ev.Data().(runevents.QueryRuntimeStateRecordCommand)
	if !ok || cmd.Validate() != nil || m == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: err.Error(), Cause: err})
		return
	}
	run, err := m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	response := runevents.QueryRuntimeStateRecordResult{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID}
	if run.RuntimeState != nil {
		response.StatePresent = true
		record := run.RuntimeState.Clone()
		response.RuntimeState = &record
		response.RuntimeStateRevision = response.RuntimeState.StateRevision
		response.RuntimeStateRef = runevents.RuntimeStateRef(cmd.WorkspaceID, cmd.RunID, response.RuntimeStateRevision)
		if cmd.ExpectedRuntimeStateRevision != 0 &&
			(cmd.ExpectedRuntimeStateRevision != response.RuntimeStateRevision || cmd.ExpectedRuntimeStateRef != response.RuntimeStateRef) {
			result.Set(nil, &cd.Error{Code: cd.VersionConflict, Message: common.ErrConflict.Error(), Cause: common.ErrConflict})
			return
		}
	}
	if err := response.Validate(); err != nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) publishRuntimeCommandsReady() {
	if m == nil || m.EventHub() == nil {
		return
	}
	event := magicevent.NewEventWithContext(
		runevents.TopicRunRuntimeCommandsReady,
		ModuleID,
		runevents.NotificationDestination,
		magicevent.NewHeader(),
		context.Background(),
		runevents.RuntimeCommandsReadyData{CausationID: "run-owner-commands-ready"},
	)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(runevents.Contracts(), event.ID(), workorchevents.EventHubLaneScope{
		SubjectType: "run_owner", SubjectID: ModuleID,
	}))
	m.PostEvent(event)
}

func buildRuntimeRecoveryInventory(runs []*runmodel.Run, cmd runevents.QueryRuntimeRecoveryInventoryCommand) runevents.QueryRuntimeRecoveryInventoryResult {
	limit := cmd.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	cursor := strings.TrimSpace(cmd.Cursor)
	items := make([]runevents.RuntimeRecoveryInventoryItem, 0)
	for _, run := range runs {
		if run == nil {
			continue
		}
		key := string(run.WorkspaceID) + "\x00" + string(run.ID)
		if cursor != "" && key <= cursor {
			continue
		}
		// This inventory is deliberately raw: Run owns only its lifecycle and
		// opaque persistence record. It must not inspect Decision, dispatch,
		// governance, delivery, or recovery semantics owned by Orchestrator.
		if runStateIsTerminal(run.State) || run.RuntimeBootstrap == nil {
			continue
		}
		item := runevents.RuntimeRecoveryInventoryItem{
			WorkspaceID:         run.WorkspaceID,
			RunID:               run.ID,
			State:               run.State,
			HasRuntimeBootstrap: run.RuntimeBootstrap != nil,
			HasRuntimeState:     run.RuntimeState != nil,
		}
		if run.RuntimeState != nil {
			item.RuntimeStateRevision = run.RuntimeState.StateRevision
			item.RuntimeStateSchemaVersion = run.RuntimeState.SchemaVersion
			item.RuntimeStateRef = runevents.RuntimeStateRef(run.WorkspaceID, run.ID, run.RuntimeState.StateRevision)
		}
		if run.RuntimeActivationStatus != nil {
			item.RuntimeActivationDisposition = run.RuntimeActivationStatus.Disposition
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		left := string(items[i].WorkspaceID) + "\x00" + string(items[i].RunID)
		right := string(items[j].WorkspaceID) + "\x00" + string(items[j].RunID)
		return left < right
	})
	ret := runevents.QueryRuntimeRecoveryInventoryResult{}
	if len(items) <= limit {
		ret.Items = items
		return ret
	}
	ret.Items = items[:limit]
	last := ret.Items[len(ret.Items)-1]
	ret.NextCursor = string(last.WorkspaceID) + "\x00" + string(last.RunID)
	return ret
}

// BuildRuntimeRecoveryInventory is the Run owner's typed SM-4 inventory
// projection. It does not select actions or drive Runtime.
func BuildRuntimeRecoveryInventory(runs []*runmodel.Run, cmd runevents.QueryRuntimeRecoveryInventoryCommand) runevents.QueryRuntimeRecoveryInventoryResult {
	return buildRuntimeRecoveryInventory(runs, cmd)
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
	topics := append([]string(nil), m.topics...)
	m.stopping = true
	m.topics = nil
	m.recoveryReady = false
	m.started = false
	m.runs = nil
	m.steps = nil
	m.checkpoints = nil
	m.commandMu.Unlock()
	for idx := len(topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(topics[idx])
	}
	m.UnsubscribeMatchedFunc(ModuleID+"/replacement-activation", runevents.TopicRunStateChanged)
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
	case runevents.ListStepSummariesByRunCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.GetRuntimeCheckpointCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.ListRuntimeCheckpointsByRunCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.PurgeRuntimeResourcesCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.UpdateRunCommand:
		workspaceID = cmd.WorkspaceID
	case runevents.DeleteRunCommand:
		workspaceID = cmd.WorkspaceID
	}
	if workspaceID != "" {
		var scopeErr error
		ctx, scopeErr = common.BindRuntimeWorkspaceContext(ctx, workspaceID)
		if scopeErr != nil {
			return runevents.RunCommandResult{}, scopeErr
		}
	}
	switch topic {
	case runevents.TopicRunCommandCreate:
		cmd, ok := data.(runevents.CreateRunCommand)
		if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.CreateRun(ctx, createRunContract(cmd)); err != nil {
			return runevents.RunCommandResult{}, err
		}
		// The Run owner emits the immutable activation-memory fact after the Run
		// record is durable. Memory remains the sole owner of RecallTrace
		// persistence; this notification is only the typed cross-owner handoff.
		// It is deliberately best-effort so an observability write cannot turn a
		// successful Run creation into an ambiguous side-effect error. Upsert is
		// keyed by the stable run activation trace ID, so duplicate create/replay
		// notifications remain idempotent.
		m.publishRuntimeActivationMemoryRecall(ctx, cmd)
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
		var cmd runevents.ListStepsByRunCommand
		var ok bool
		if topic == runevents.TopicRunCommandListStepSummariesByRun {
			var summaryCommand runevents.ListStepSummariesByRunCommand
			summaryCommand, ok = data.(runevents.ListStepSummariesByRunCommand)
			cmd = runevents.ListStepsByRunCommand(summaryCommand)
		} else {
			cmd, ok = data.(runevents.ListStepsByRunCommand)
		}
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
	case runevents.TopicRunCommandUpdate:
		cmd, ok := data.(runevents.UpdateRunCommand)
		if !ok || cmd.RunID == "" || cmd.WorkspaceID == "" {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		current, err := m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID)
		if err != nil {
			return runevents.RunCommandResult{}, err
		}
		privateCmd := updateRunContract(cmd)
		if runUsesRuntime(current) {
			return m.updateRuntimeState(ctx, current, privateCmd)
		} else if cmd.RuntimeState != nil || cmd.ExpectedRuntimeStateRevision != nil {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		if err := validateRunUpdate(current, privateCmd); err != nil {
			return runevents.RunCommandResult{}, err
		}
		item := runFromUpdateCommand(current, privateCmd)
		if err := m.runs.CompareAndSwap(ctx, item, current.Revision); err != nil {
			return runevents.RunCommandResult{}, err
		}
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

func createRunContract(cmd runevents.CreateRunCommand) runcontracts.CreateRunCommand {
	ret := runcontracts.CreateRunCommand{
		RunID: cmd.RunID, WorkspaceID: cmd.WorkspaceID, GoalID: cmd.GoalID, DoDID: cmd.DoDID,
		DefinitionRevision: cmd.DefinitionRevision, InitialStepKind: cmd.InitialStepKind,
		Lineage: cmd.Lineage, Priority: cmd.Priority,
	}
	if cmd.RuntimeBootstrap != nil {
		bootstrap := cmd.RuntimeBootstrap.Clone()
		ret.RuntimeBootstrap = &bootstrap
	}
	return ret
}

func updateRunContract(cmd runevents.UpdateRunCommand) runcontracts.UpdateRunCommand {
	ret := runcontracts.UpdateRunCommand{RunID: cmd.RunID, WorkspaceID: cmd.WorkspaceID}
	if cmd.RuntimeState != nil {
		state := cmd.RuntimeState.Clone()
		ret.RuntimeState = &state
	}
	if cmd.ExpectedRuntimeStateRevision != nil {
		revision := *cmd.ExpectedRuntimeStateRevision
		ret.ExpectedRuntimeStateRevision = &revision
	}
	return ret
}

// updateRuntimeState persists an Orchestrator-owned aggregate through the
// Run owner's physical record. SM-1 mutations can legitimately change the
// record revision while the aggregate revision remains unchanged. In that
// case rebase the opaque RuntimeState write on the latest Run record; only an
// aggregate revision mismatch is a semantic conflict that returns to the
// Orchestrator SM-4 recovery owner.
func (m *Biz) updateRuntimeState(ctx context.Context, current *runmodel.Run, cmd runcontracts.UpdateRunCommand) (runevents.RunCommandResult, error) {
	const maxPhysicalCASRebases = 4
	for attempt := 0; attempt < maxPhysicalCASRebases; attempt++ {
		if current == nil {
			return runtimeStateUpdateOutcome(nil, cmd, runtimemodel.RuntimeFeedbackCategoryOwnerTransient, 0, common.ErrNotFound), nil
		}
		if err := validateRunUpdate(current, cmd); err != nil {
			observedRevision := uint64(0)
			if current.RuntimeState != nil {
				observedRevision = current.RuntimeState.StateRevision
			}
			return runtimeStateUpdateOutcome(current, cmd, runtimemodel.RuntimeFeedbackCategorySemanticRejection, observedRevision, err), nil
		}
		currentRevision := uint64(0)
		if current.RuntimeState != nil {
			currentRevision = current.RuntimeState.StateRevision
		}
		if currentRevision != *cmd.ExpectedRuntimeStateRevision {
			return runtimeStateUpdateOutcome(current, cmd, runtimemodel.RuntimeFeedbackCategoryAggregateRevisionMismatch, currentRevision, nil), nil
		}
		item := runFromUpdateCommand(current, cmd)
		err := m.runs.CompareAndSwap(ctx, item, current.Revision)
		if err == nil {
			return runtimeStateUpdateOutcome(item, cmd, runtimemodel.RuntimeFeedbackCategoryAccepted, item.RuntimeState.StateRevision, nil), nil
		}
		if !errors.Is(err, common.ErrConflict) {
			return runtimeStateUpdateOutcome(current, cmd, runtimemodel.RuntimeFeedbackCategoryOwnerTransient, currentRevision, err), nil
		}
		// A physical CAS conflict may be a concurrent SM-1/model write. Reload
		// and retry only while the semantic aggregate fence is still current.
		current, err = m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID)
		if err != nil {
			return runtimeStateUpdateOutcome(nil, cmd, runtimemodel.RuntimeFeedbackCategoryOwnerTransient, currentRevision, err), nil
		}
	}
	observedRevision := uint64(0)
	if current != nil && current.RuntimeState != nil {
		observedRevision = current.RuntimeState.StateRevision
	}
	return runtimeStateUpdateOutcome(current, cmd, runtimemodel.RuntimeFeedbackCategoryOwnerTransient, observedRevision, common.ErrConflict), nil
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

func runFromUpdateCommand(current *runmodel.Run, cmd runcontracts.UpdateRunCommand) *runmodel.Run {
	if current == nil {
		return nil
	}
	item := *current
	item.RuntimeState = cloneRuntimeStateRecord(cmd.RuntimeState)
	item.RuntimeActivationStatus = current.RuntimeActivationStatus.Clone()
	return &item
}

func validateRunUpdate(current *runmodel.Run, cmd runcontracts.UpdateRunCommand) error {
	if current == nil || current.ID != cmd.RunID || current.WorkspaceID != cmd.WorkspaceID {
		return common.ErrConflict
	}
	if !runUsesRuntime(current) {
		return nil
	}
	if cmd.RuntimeState == nil || cmd.ExpectedRuntimeStateRevision == nil ||
		cmd.RuntimeState.Facts.WorkspaceID != cmd.WorkspaceID || cmd.RuntimeState.Facts.RunID != cmd.RunID {
		return common.ErrConflict
	}
	if current.RuntimeBootstrap == nil {
		// The sealed bootstrap is required to validate an aggregate write. Reject a
		// malformed record instead of letting a generic owner command panic.
		return common.ErrConflict
	}
	state := cmd.RuntimeState.Clone()
	bootstrap := current.RuntimeBootstrap.Normalized()
	if state.Definition.GoalID != current.GoalID || state.Definition.DoDID != current.DoDID || state.Definition.Revision != current.DefinitionRevision ||
		!runtimemodel.SameRuntimeDefinitionContext(bootstrap.Definition, state.Definition) ||
		!runtimemodel.SameRuntimeActivationContext(bootstrap.Activation, state.Facts.Activation) {
		return common.ErrConflict
	}
	return nil
}

func runUsesRuntime(run *runmodel.Run) bool {
	return run != nil && run.RuntimeBootstrap != nil
}

// ValidateRunUpdate exposes the Run owner's SM-1 persistence invariant for
// focused contract tests; it does not alter owner state.
func ValidateRunUpdate(current *runmodel.Run, cmd runevents.UpdateRunCommand) error {
	return validateRunUpdate(current, updateRunContract(cmd))
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
		ID:                      item.ID,
		WorkspaceID:             item.WorkspaceID,
		GoalID:                  item.GoalID,
		DoDID:                   item.DoDID,
		Lineage:                 item.Lineage,
		DefinitionRevision:      item.DefinitionRevision,
		InitialStepKind:         item.InitialStepKind,
		RuntimeActivationStatus: item.RuntimeActivationStatus.Clone(),
		State:                   item.State,
		Priority:                item.Priority,
		BlockingTicketID:        item.BlockingTicketID,
		BlockingApprovalID:      item.BlockingApprovalID,
		FailureReason:           item.FailureReason,
		CreatedAt:               item.CreatedAt,
		StartedAt:               item.StartedAt,
		EndedAt:                 item.EndedAt,
	}
}
