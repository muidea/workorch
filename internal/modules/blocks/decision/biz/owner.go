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
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	Tickets           repository.DecisionTicketRepository
	Resolutions       repository.DecisionResolutionRepository
	EventHub          magicevent.Hub
	BackgroundRoutine magictask.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service         *Service
	tickets         repository.DecisionTicketRepository
	resolutions     repository.DecisionResolutionRepository
	topics          []string
	started         bool
	reconcileMu     sync.Mutex
	reconcileCancel context.CancelFunc
	reconcileDone   chan struct{}
}

func New(h magicevent.Hub, background magictask.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.Tickets == nil || deps.Resolutions == nil || deps.EventHub == nil || h == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, h, background), service: NewService(deps.Tickets, deps.Resolutions), tickets: deps.Tickets, resolutions: deps.Resolutions}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	m.topics = decisionCommandTopics()
	for _, topic := range m.topics {
		m.SubscribeFunc(topic, m.handleCommand)
	}
	m.topics = append(m.topics, decisionevents.TopicOpenRuntimeGovernanceRequest)
	m.SubscribeFunc(decisionevents.TopicOpenRuntimeGovernanceRequest, m.handleRuntimeGovernanceRequest)
	m.topics = append(m.topics, decisionevents.TopicAcquireRuntimeCapabilities)
	m.SubscribeFunc(decisionevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	if err := m.startRuntimeGovernanceProposalReconciliation(); err != nil {
		return err
	}
	m.started = true
	return nil
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(decisionevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setDecisionRuntimeCapabilityResult(result, decisionevents.AcquireRuntimeCapabilitiesResult{}, common.ErrInvalidArgument)
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

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.stopRuntimeGovernanceProposalReconciliation()
	for idx := len(m.topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.topics[idx])
	}
	m.topics = nil
	m.service = nil
	m.tickets = nil
	m.resolutions = nil
	m.started = false
}
