package decision

import (
	"context"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	OwnerRepositories repository.DecisionOwnerRepositories
	EventHub          magicevent.Hub
	BackgroundRoutine magictask.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service            *mutationService
	tickets            repository.DecisionTicketRepository
	resolutions        repository.DecisionResolutionRepository
	topics             []string
	started            bool
	lifecycle          basebiz.Lifecycle
	shutdownMu         sync.Mutex
	startAttempted     bool
	starting           bool
	closing            bool
	releasing          bool
	released           bool
	reconcileMu        sync.Mutex
	reconcileCancel    context.CancelFunc
	reconcileDone      chan struct{}
	workspaceDeletions repository.DecisionWorkspaceDeletionRepository
	workspaceLocksMu   sync.Mutex
	workspaceLocks     map[common.WorkspaceID]*sync.Mutex
}

func New(h magicevent.Hub, background magictask.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.EventHub == nil || h == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := deps.OwnerRepositories.Validate(); err != nil {
		return nil, err
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, h, background), service: newMutationService(deps.OwnerRepositories.Tickets(), deps.OwnerRepositories.Resolutions()), tickets: deps.OwnerRepositories.Tickets(), resolutions: deps.OwnerRepositories.Resolutions(), workspaceDeletions: deps.OwnerRepositories.WorkspaceDeletions(), workspaceLocks: map[common.WorkspaceID]*sync.Mutex{}}, nil
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
	if m.service == nil || m.EventHub() == nil {
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
	for _, topic := range decisionCommandTopics() {
		if err := m.subscribeOwner(topic, m.handleCommand); err != nil {
			return err
		}
	}
	if err := m.subscribeOwner(decisionevents.TopicOpenRuntimeGovernanceRequest, m.handleRuntimeGovernanceRequest); err != nil {
		return err
	}
	if err := m.subscribeOwner(decisionevents.TopicRegisterContractChangeProposal, m.handleRegisterContractChangeProposal); err != nil {
		return err
	}
	for _, topic := range definitionClarificationTopics() {
		var handler magicevent.ObserverFunc
		switch topic {
		case decisionevents.TopicOpenWorkspaceDefinitionClarification:
			handler = m.handleOpenWorkspaceDefinitionClarification
		case decisionevents.TopicReconcileWorkspaceDefinitionClarificationCandidate:
			handler = m.handleReconcileWorkspaceDefinitionClarification
		case decisionevents.TopicResolveWorkspaceDefinitionClarificationByAdmission:
			handler = m.handleResolveWorkspaceDefinitionClarification
		case decisionevents.TopicQueryWorkspaceDefinitionClarificationFact:
			handler = m.handleQueryWorkspaceDefinitionClarificationFact
		case decisionevents.TopicOpenRuntimeDefinitionClarification:
			handler = m.handleOpenRuntimeDefinitionClarification

		case decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement:
			handler = m.handleResolveRuntimeDefinitionClarification
		}
		if err := m.subscribeOwner(topic, handler); err != nil {
			return err
		}
	}
	if err := m.subscribeOwner(decisionevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities); err != nil {
		return err
	}
	if err := m.startRuntimeGovernanceProposalReconciliation(); err != nil {
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
		leave, err := m.enterOwner(decisionDrainTopic(topic))
		if err != nil {
			setDecisionMagicResult(result, struct{}{}, err)
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

func decisionDrainTopic(topic string) bool {
	family, ok := decisionCommandFamilyForTopic(topic)
	if ok {
		return family == decisionCommandFamilyTicketQuery || family == decisionCommandFamilyResolution
	}
	switch topic {
	case decisionevents.TopicQueryWorkspaceDefinitionClarificationFact,
		decisionevents.TopicResolveWorkspaceDefinitionClarificationByAdmission,
		decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement:
		return true
	default:
		return false
	}
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
		setDecisionRuntimeCapabilityResult(result, decisionevents.AcquireRuntimeCapabilitiesResult{}, common.ErrInvalidArgument)
		return
	}
	command, ok := event.Data().(decisionevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setDecisionRuntimeCapabilityResult(result, decisionevents.AcquireRuntimeCapabilitiesResult{}, common.ErrInvalidArgument)
		return
	}
	request := runtimecontract.RuntimeCapabilityAcquisitionRequest(command)
	if _, err := common.BindRuntimeWorkspaceContext(event.Context(), request.WorkspaceID); err != nil {
		setDecisionRuntimeCapabilityResult(result, decisionevents.AcquireRuntimeCapabilitiesResult{}, err)
		return
	}
	response, err := decisionevents.ResolveRuntimeCapabilities(command, ModuleID, modulecommon.RuntimeCapabilityKinds()...)
	setDecisionRuntimeCapabilityResult(result, response, err)
}

func setDecisionRuntimeCapabilityResult(result magicevent.Result, response decisionevents.AcquireRuntimeCapabilitiesResult, err error) {
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
	m.stopRuntimeGovernanceProposalReconciliation()
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
	m.service, m.tickets, m.resolutions, m.workspaceDeletions = nil, nil, nil, nil
	m.started, m.released = false, true
	m.shutdownMu.Unlock()
	return nil
}
