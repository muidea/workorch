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
	EventHub          magicevent.Hub
	BackgroundRoutine magictask.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	// commandMu only coordinates in-flight EventHub owner commands with Stop.
	// Framework serializes Configure/Start/Stop lifecycle calls.
	commandMu     sync.RWMutex
	stopping      bool
	notifier      runStateNotifier
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
	m.notifier = eventRunStateNotifier{post: m.PostEvent}
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
		m.SubscribeFunc(topic, m.handleCommand)
	}
	m.topics = append(m.topics, regularTopics...)
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
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(runevents.Contracts(), event.ID(), eventcontract.LaneScope{
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
	m.notifier = nil
	m.commandMu.Unlock()
	for idx := len(topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(topics[idx])
	}
	return nil
}
