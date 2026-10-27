package memory

import (
	"context"
	"crypto/rand"
	"sync"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	OwnerRepositories repository.MemoryOwnerRepositories
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service            *service
	workspaceDeletions repository.MemoryWorkspaceDeletionRepository
	topics             []string
	started            bool
	preparationEpoch   string
	lifecycle          basebiz.Lifecycle
	shutdownMu         sync.Mutex
	releasing          bool
	released           bool
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
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, h, b), service: newServiceWithRecallTraces(deps.OwnerRepositories.Items(), deps.OwnerRepositories.RecallTraces()), workspaceDeletions: deps.OwnerRepositories.WorkspaceDeletions(), workspaceLocks: map[common.WorkspaceID]*sync.Mutex{}, preparationEpoch: "memory-preparation-epoch:" + rand.Text()}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	leave, err := m.enterOwner(false)
	if err != nil {
		return err
	}
	defer leave()
	if m.started {
		return nil
	}
	if len(m.topics) != 0 {
		return common.ErrConflict
	}
	for _, topic := range memoryCommandTopics() {
		if err := m.subscribeOwner(topic, m.handleCommand); err != nil {
			return err
		}
	}
	m.started = true
	return nil
}

func (m *Biz) Teardown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.BeginShutdown(ctx)
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	m.shutdownMu.Lock()
	if m.releasing {
		m.shutdownMu.Unlock()
		return common.ErrConflict
	}
	m.releasing = true
	m.shutdownMu.Unlock()
	defer func() { m.shutdownMu.Lock(); m.releasing = false; m.shutdownMu.Unlock() }()
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	for len(m.topics) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(m.topics) - 1
		if err := m.UnsubscribeFuncChecked(m.topics[last]); err != nil {
			return err
		}
		m.topics = m.topics[:last]
	}
	m.service = nil
	m.preparationEpoch = ""
	m.workspaceDeletions = nil
	m.workspaceLocks = nil
	m.started = false
	m.shutdownMu.Lock()
	m.released = true
	m.shutdownMu.Unlock()
	return nil
}
