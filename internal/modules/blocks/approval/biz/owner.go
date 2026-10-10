package approval

import (
	"context"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	OwnerRepositories repository.ApprovalOwnerRepositories
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service            *mutationService
	approvals          repository.ApprovalRepository
	topics             []string
	started            bool
	lifecycle          basebiz.Lifecycle
	shutdownMu         sync.Mutex
	startAttempted     bool
	starting           bool
	closing            bool
	releasing          bool
	released           bool
	lifecycleMu        sync.Mutex
	reconcileMu        sync.Mutex
	reconcileCancel    context.CancelFunc
	reconcileDone      chan struct{}
	workspaceDeletions repository.ApprovalWorkspaceDeletionRepository
	workspaceLocksMu   sync.Mutex
	workspaceLocks     map[common.WorkspaceID]*sync.Mutex
}

func New(h magicevent.Hub, b task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.EventHub == nil || h == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := deps.OwnerRepositories.Validate(); err != nil {
		return nil, err
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, h, b), service: newMutationService(deps.OwnerRepositories.Approvals()), approvals: deps.OwnerRepositories.Approvals(), workspaceDeletions: deps.OwnerRepositories.WorkspaceDeletions(), workspaceLocks: map[common.WorkspaceID]*sync.Mutex{}}, nil
}

func (m *Biz) Start() error {
	if m == nil {
		return common.ErrInvalidArgument
	}
	m.shutdownMu.Lock()
	if m.closing || m.released {
		m.shutdownMu.Unlock()
		return basebiz.ErrLifecycleClosing
	}
	if m.startAttempted {
		started := m.started
		m.shutdownMu.Unlock()
		if started {
			return nil
		}
		return common.ErrConflict
	}
	if m.service == nil || m.approvals == nil || m.EventHub() == nil {
		m.shutdownMu.Unlock()
		return common.ErrInvalidArgument
	}
	if err := m.lifecycle.Reset(); err != nil {
		m.shutdownMu.Unlock()
		return err
	}
	leave, err := m.lifecycle.Enter()
	if err != nil {
		m.shutdownMu.Unlock()
		return err
	}
	m.startAttempted, m.starting = true, true
	m.shutdownMu.Unlock()
	defer leave()
	defer func() { m.shutdownMu.Lock(); m.starting = false; m.shutdownMu.Unlock() }()
	for _, topic := range approvalCommandTopics() {
		if err := m.subscribeOwner(topic, m.handleCommand); err != nil {
			return err
		}
	}
	if err := m.subscribeOwner(approvalevents.TopicOpenRuntimeApprovalRequest, m.handleRuntimeApprovalRequest); err != nil {
		return err
	}
	if err := m.subscribeOwner(approvalevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities); err != nil {
		return err
	}
	if err := m.startApprovalReconciliation(); err != nil {
		return err
	}
	m.shutdownMu.Lock()
	defer m.shutdownMu.Unlock()
	if m.closing {
		return basebiz.ErrLifecycleClosing
	}
	m.started = true
	return nil
}

func (m *Biz) subscribeOwner(topic string, handler magicevent.ObserverFunc) error {
	if err := m.SubscribeFuncChecked(topic, func(ev magicevent.Event, result magicevent.Result) {
		leave, err := m.enterOwner(approvalDrainTopic(topic))
		if err != nil {
			setApprovalMagicResult(result, struct{}{}, err)
			return
		}
		defer leave()
		handler(ev, result)
	}); err != nil {
		return err
	}
	m.shutdownMu.Lock()
	m.topics = append(m.topics, topic)
	m.shutdownMu.Unlock()
	return nil
}

func approvalDrainTopic(topic string) bool {
	family, ok := approvalCommandFamilyForTopic(topic)
	return ok && (family == approvalCommandFamilyQuery || family == approvalCommandFamilyResolution)
}

func (m *Biz) enterOwner(drain bool) (func(), error) {
	m.shutdownMu.Lock()
	defer m.shutdownMu.Unlock()
	if m.releasing || m.released {
		return nil, basebiz.ErrLifecycleClosing
	}
	if drain {
		return m.lifecycle.EnterDrain()
	}
	return m.lifecycle.Enter()
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator {
		setApprovalRuntimeCapabilityResult(result, approvalevents.AcquireRuntimeCapabilitiesResult{}, common.ErrInvalidArgument)
		return
	}
	command, ok := event.Data().(approvalevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setApprovalRuntimeCapabilityResult(result, approvalevents.AcquireRuntimeCapabilitiesResult{}, common.ErrInvalidArgument)
		return
	}
	request := runtimecontract.RuntimeCapabilityAcquisitionRequest(command)
	if _, err := common.BindRuntimeWorkspaceContext(event.Context(), request.WorkspaceID); err != nil {
		setApprovalRuntimeCapabilityResult(result, approvalevents.AcquireRuntimeCapabilitiesResult{}, err)
		return
	}
	response, err := approvalevents.ResolveRuntimeCapabilities(command, ModuleID, modulecommon.RuntimeCapabilityKinds()...)
	setApprovalRuntimeCapabilityResult(result, response, err)
}

func setApprovalRuntimeCapabilityResult(result magicevent.Result, response approvalevents.AcquireRuntimeCapabilitiesResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) BeginShutdown() {
	if m == nil {
		return
	}
	m.shutdownMu.Lock()
	m.closing = true
	m.lifecycle.BeginShutdown()
	m.shutdownMu.Unlock()
	m.stopApprovalReconciliation()
}

func (m *Biz) Quiesce(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		return basebiz.ErrNilLifecycleContext
	}
	if err := m.lifecycle.Quiesce(ctx); err != nil {
		return err
	}
	m.reconcileMu.Lock()
	done := m.reconcileDone
	m.reconcileMu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (m *Biz) Teardown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.BeginShutdown()
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	m.shutdownMu.Lock()
	if m.released {
		m.shutdownMu.Unlock()
		return nil
	}
	if m.releasing {
		m.shutdownMu.Unlock()
		return common.ErrConflict
	}
	m.releasing = true
	m.shutdownMu.Unlock()
	defer func() { m.shutdownMu.Lock(); m.releasing = false; m.shutdownMu.Unlock() }()
	// 最终释放先封闭 drain 准入，再复核真实在途计数。
	if err := m.lifecycle.Quiesce(ctx); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		m.shutdownMu.Lock()
		if len(m.topics) == 0 {
			m.shutdownMu.Unlock()
			break
		}
		topic := m.topics[len(m.topics)-1]
		m.shutdownMu.Unlock()
		if err := m.UnsubscribeFuncChecked(topic); err != nil {
			return err
		}
		m.shutdownMu.Lock()
		m.topics = m.topics[:len(m.topics)-1]
		m.shutdownMu.Unlock()
	}
	m.reconcileMu.Lock()
	m.reconcileCancel, m.reconcileDone = nil, nil
	m.reconcileMu.Unlock()
	m.shutdownMu.Lock()
	m.service, m.approvals, m.workspaceDeletions = nil, nil, nil
	m.started, m.released = false, true
	m.shutdownMu.Unlock()
	return nil
}
