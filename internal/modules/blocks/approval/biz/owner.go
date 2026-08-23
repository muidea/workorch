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
	Approvals         repository.ApprovalRepository
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service         *mutationService
	approvals       repository.ApprovalRepository
	topics          []string
	started         bool
	lifecycleMu     sync.Mutex
	reconcileMu     sync.Mutex
	reconcileCancel context.CancelFunc
	reconcileDone   chan struct{}
}

func New(h magicevent.Hub, b task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.Approvals == nil || deps.EventHub == nil || h == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, h, b), service: newMutationService(deps.Approvals), approvals: deps.Approvals}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil || m.approvals == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	m.topics = approvalCommandTopics()
	for _, topic := range m.topics {
		m.SubscribeFunc(topic, m.handleCommand)
	}
	m.topics = append(m.topics, approvalevents.TopicOpenRuntimeApprovalRequest)
	m.SubscribeFunc(approvalevents.TopicOpenRuntimeApprovalRequest, m.handleRuntimeApprovalRequest)
	m.topics = append(m.topics, approvalevents.TopicAcquireRuntimeCapabilities)
	m.SubscribeFunc(approvalevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	if err := m.startApprovalReconciliation(); err != nil {
		for idx := len(m.topics) - 1; idx >= 0; idx-- {
			m.UnsubscribeFunc(m.topics[idx])
		}
		m.topics = nil
		return err
	}
	m.started = true
	return nil
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

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.stopApprovalReconciliation()
	for idx := len(m.topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.topics[idx])
	}
	m.topics = nil
	m.service = nil
	m.approvals = nil
	m.started = false
}
