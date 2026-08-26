package run

import (
	"context"
	"sort"
	"strings"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	Runs              repository.RunRepository
	Steps             repository.StepRepository
	Checkpoints       repository.RuntimeCheckpointRepository
	OwnerRepositories repository.RunOwnerRepositories
	EventHub          magicevent.Hub
	BackgroundRoutine magictask.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	// commandMu only coordinates in-flight EventHub owner commands with Stop.
	// Framework serializes Configure/Start/Stop lifecycle calls.
	commandMu sync.RWMutex
	// workspaceExecutionMu serializes Workspace-scoped restart admission with
	// successor materialization. Durable repository guards remain the final
	// fence; this lock prevents two owner-private workflows from racing toward
	// different live Runs in one process.
	workspaceExecutionMu sync.Mutex
	stopping             bool
	notifier             runStateNotifier
	runs                 repository.RunRepository
	steps                repository.StepRepository
	checkpoints          repository.RuntimeCheckpointRepository
	workspacePurger      repository.RunWorkspaceResourcePurger
	topics               []string
	recoveryReady        bool
	started              bool
	// replacementRecoveryTasks is process-local wakeup deduplication only. The
	// exact stage/generation/due time authority remains persisted inside the
	// Run-owned ReplacementIntent recovery lease.
	replacementRecoveryMu     sync.Mutex
	replacementRecoveryTasks  map[string]uint64
	replacementRecoveryCtx    context.Context
	replacementRecoveryCancel context.CancelFunc
	replacementRecoveryEpoch  uint64
	replacementRecoveryWG     sync.WaitGroup
}

func New() *Biz { return &Biz{} }

func (m *Biz) ID() string { return ModuleID }

func (m *Biz) Configure(deps Dependencies) error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	if deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	if deps.OwnerRepositories.Present() {
		if deps.OwnerRepositories.Validate() != nil || deps.Runs != nil || deps.Steps != nil || deps.Checkpoints != nil {
			return common.ErrInvalidArgument
		}
		m.runs = deps.OwnerRepositories.Runs()
		m.steps = deps.OwnerRepositories.Steps()
		m.checkpoints = deps.OwnerRepositories.Checkpoints()
		m.workspacePurger = deps.OwnerRepositories.WorkspacePurger()
	} else {
		if deps.Runs == nil {
			return common.ErrInvalidArgument
		}
		m.runs = deps.Runs
		m.steps = deps.Steps
		m.checkpoints = deps.Checkpoints
		m.workspacePurger = nil
	}
	m.stopping = false
	m.Base = basebiz.New(modulecommon.ModuleID, deps.EventHub, deps.BackgroundRoutine)
	m.notifier = eventRunStateNotifier{post: m.PostEvent}
	m.replacementRecoveryMu.Lock()
	if m.replacementRecoveryCancel != nil {
		m.replacementRecoveryCancel()
	}
	m.replacementRecoveryEpoch++
	m.replacementRecoveryCtx, m.replacementRecoveryCancel = context.WithCancel(context.Background())
	m.replacementRecoveryTasks = make(map[string]uint64)
	m.replacementRecoveryMu.Unlock()
	// A restart creates the Run and activates it in two owner-private writes.
	// Recover that durable cut before exposing the SM-4 inventory subscription:
	// Orchestrator setup must either observe the Run as active or fail startup,
	// never inventory a pending Run whose activation notification was lost.
	if err := m.recoverPendingWorkspaceActivations(); err != nil {
		return err
	}
	// Configure is invoked exclusively from Setup (or explicit test assembly).
	// Establish the SM-4 downstream read contract before any later module Run.
	return m.prepareRuntimeRecoveryInventory()
}

func (m *Biz) Start() error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	m.commandMu.Lock()
	if m.stopping || m.runs == nil || m.EventHub() == nil {
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
	regularTopics := runCommandTopics()
	for _, topic := range regularTopics {
		if topic == runevents.TopicRunCommandWorkspaceRetirementFact {
			continue
		}
		m.SubscribeFunc(topic, m.handleCommand)
		m.topics = append(m.topics, topic)
	}
	for _, topic := range m.lifecycleTopics() {
		m.SubscribeFunc(topic, m.handleLifecycleCommand)
		m.topics = append(m.topics, topic)
	}
	m.SubscribeFunc(runevents.TopicRunCommandWorkspaceExecutionIntent, m.handleWorkspaceExecutionIntent)
	m.topics = append(m.topics, runevents.TopicRunCommandWorkspaceExecutionIntent)
	m.started = true
	m.commandMu.Unlock()
	// Re-scan Run-owned records for two durable handoffs that may have been
	// interrupted by a crash: replacement termination delivery and the
	// create-once activation-memory audit. Neither path reconstructs Runtime
	// authority or advances an SM-2/SM-3 decision.
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
	if m.stopping || m.runs == nil || m.EventHub() == nil {
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
	m.SubscribeFunc(runevents.TopicRunCommandWorkspaceRetirementInventory, m.handleWorkspaceRetirementInventory)
	// Orchestrator starts before Run by framework weight. A persisted draining
	// or retired head therefore needs both the inventory and its exact physical
	// Run fact during Setup, before ordinary Run commands are exposed.
	m.SubscribeFunc(runevents.TopicRunCommandWorkspaceRetirementFact, m.handleCommand)
	m.topics = append(m.topics,
		runevents.TopicRunCommandRuntimeRecoveryInventory,
		runevents.TopicRunCommandRuntimeStateRecordReadiness,
		runevents.TopicRunCommandRuntimeStateRecord,
		runevents.TopicRunCommandWorkspaceRetirementInventory,
		runevents.TopicRunCommandWorkspaceRetirementFact,
	)
	m.recoveryReady = true
	return nil
}

func (m *Biz) handleRuntimeRecoveryInventory(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	cmd, ok := ev.Data().(runevents.QueryRuntimeRecoveryInventoryCommand)
	if !ok || m == nil || ev.Source() != workspaceRetirementFactQuerySource {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	// Runtime recovery inventory and retirement finalization share the Run
	// owner's workflow lock. A successful inventory response therefore cannot
	// cross the retired visibility linearization point in this process.
	m.workspaceExecutionMu.Lock()
	defer m.workspaceExecutionMu.Unlock()
	runs, err := m.runs.List(ev.Context())
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	retirements, err := m.runs.ListWorkspaceRetirements(ev.Context())
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	retired := make(map[common.WorkspaceID]struct{}, len(retirements))
	draining := make(map[common.WorkspaceID]struct{}, len(retirements))
	for _, fence := range retirements {
		if fence.IsRetired() {
			retired[fence.WorkspaceID] = struct{}{}
		} else if fence.IsDraining() {
			draining[fence.WorkspaceID] = struct{}{}
		}
	}
	filtered := make([]*runmodel.Run, 0, len(runs))
	for _, item := range runs {
		if item == nil {
			continue
		}
		if _, fenced := retired[item.WorkspaceID]; !fenced {
			filtered = append(filtered, item)
		}
	}
	result.Set(buildRuntimeRecoveryInventoryWithDraining(filtered, cmd, draining), nil)
}

func (m *Biz) handleWorkspaceRetirementInventory(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	cmd, ok := ev.Data().(runevents.QueryWorkspaceRetirementInventoryCommand)
	if !ok || m == nil || ev.Source() != workspaceRetirementFactQuerySource {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	items, err := m.runs.ListWorkspaceRetirements(ev.Context())
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	response, err := buildWorkspaceRetirementInventory(items, cmd)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) handleRuntimeStateRecordReadiness(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	_, ok := ev.Data().(runevents.QueryRuntimeStateRecordReadinessCommand)
	if !ok || m == nil || ev.Source() != workspaceRetirementFactQuerySource {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil || m.EventHub() == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	result.Set(runevents.QueryRuntimeStateRecordReadinessResult{Ready: m.started}, nil)
}

func (m *Biz) handleRuntimeStateRecord(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	cmd, ok := ev.Data().(runevents.QueryRuntimeStateRecordCommand)
	if !ok || cmd.Validate() != nil || m == nil || ev.Source() != workspaceRetirementFactQuerySource {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: common.ErrInvalidArgument.Error(), Cause: common.ErrInvalidArgument})
		return
	}
	m.workspaceExecutionMu.Lock()
	defer m.workspaceExecutionMu.Unlock()
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), cmd.WorkspaceID)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: err.Error(), Cause: err})
		return
	}
	retired, err := m.workspaceRunsRetired(ctx, cmd.WorkspaceID)
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	if retired {
		result.Set(nil, &cd.Error{Code: cd.NotFound, Message: common.ErrNotFound.Error(), Cause: common.ErrNotFound})
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
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(runevents.Contracts(), event.ID(), eventcontract.LaneScope{
		SubjectType: "run_owner", SubjectID: ModuleID,
	}))
	m.PostEvent(event)
}

func buildRuntimeRecoveryInventory(runs []*runmodel.Run, cmd runevents.QueryRuntimeRecoveryInventoryCommand) runevents.QueryRuntimeRecoveryInventoryResult {
	return buildRuntimeRecoveryInventoryWithDraining(runs, cmd, nil)
}

func buildRuntimeRecoveryInventoryWithDraining(runs []*runmodel.Run, cmd runevents.QueryRuntimeRecoveryInventoryCommand, draining map[common.WorkspaceID]struct{}) runevents.QueryRuntimeRecoveryInventoryResult {
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
		_, retirementDraining := draining[run.WorkspaceID]
		if run.RuntimeBootstrap == nil || (runStateIsTerminal(run.State) && (!retirementDraining || run.RuntimeState == nil)) {
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
			item.RuntimeStateRef = runevents.RuntimeStateRef(run.WorkspaceID, run.ID, run.RuntimeState.StateRevision)
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
	m.commandMu.Unlock()

	m.replacementRecoveryMu.Lock()
	cancelReplacementRecovery := m.replacementRecoveryCancel
	m.replacementRecoveryCancel = nil
	m.replacementRecoveryCtx = nil
	m.replacementRecoveryEpoch++
	m.replacementRecoveryTasks = nil
	m.replacementRecoveryMu.Unlock()
	if cancelReplacementRecovery != nil {
		cancelReplacementRecovery()
	}
	// A process-local wakeup may already be reducing a persisted Run-owned
	// recovery generation. Drain that owner-local work before dependencies are
	// cleared; canceled timers never enter the reducer, while an already claimed
	// generation completes or leaves its durable running lease for startup
	// recovery.
	m.replacementRecoveryWG.Wait()
	for idx := len(topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(topics[idx])
	}
	m.commandMu.Lock()
	m.runs = nil
	m.steps = nil
	m.checkpoints = nil
	m.workspacePurger = nil
	m.notifier = nil
	m.commandMu.Unlock()
	return nil
}
