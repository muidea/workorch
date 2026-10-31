package soul

import (
	"context"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/common"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

const systemCommandSource = "kernel/system"

const (
	applicationContextRegistrySource = "application/workorchd/context-registry"
	applicationOwnerReadSource       = "application/workorchd/appservice"
)

type Dependencies struct {
	Souls             repository.SoulRepository
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service    *service
	souls      repository.SoulRepository
	topics     []string
	started    bool
	lifecycle  basebiz.Lifecycle
	shutdownMu sync.Mutex
	releasing  bool
	released   bool
}

func New(eventHub magicevent.Hub, background task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.Souls == nil {
		return nil, common.ErrInvalidArgument
	}
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, background), service: newService(deps.Souls), souls: deps.Souls}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil {
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
	topics := []string{
		soulevents.TopicSoulCommandCreate,
		soulevents.TopicSoulCommandGet,
		soulevents.TopicSoulCommandList,
		soulevents.TopicSoulCommandUpdate,
		soulevents.TopicSoulCommandDelete,
		soulevents.TopicSoulCommandValidateCapability,
	}
	for _, topic := range topics {
		if err := m.subscribeOwner(topic, m.handleSoulCommand); err != nil {
			return err
		}
	}
	if err := m.subscribeOwner(soulevents.TopicSoulCommandEnsureDefaults, m.handleSystemDefaultsCommand); err != nil {
		return err
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
	m.souls = nil
	m.started = false
	m.shutdownMu.Lock()
	m.released = true
	m.shutdownMu.Unlock()
	return nil
}

func (m *Biz) handleSystemDefaultsCommand(ev magicevent.Event, result magicevent.Result) {
	// All Soul topics are synchronous commands. A fire-and-forget Post has no
	// result channel and must never mutate owner state.
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != systemCommandSource || m == nil || m.service == nil {
		setSystemSoulDefaultsResult(result, common.ErrInvalidArgument)
		return
	}
	if _, ok := ev.Data().(soulevents.EnsureDefaultsCommand); !ok {
		setSystemSoulDefaultsResult(result, common.ErrInvalidArgument)
		return
	}
	setSystemSoulDefaultsResult(result, m.service.ensureCurrentBuiltinSoulDefaults(ev.Context()))
}

func setSystemSoulDefaultsResult(result magicevent.Result, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(soulevents.EnsureDefaultsResult{Ensured: true}, nil)
}

func (m *Biz) handleSoulCommand(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if m == nil || m.service == nil {
		setMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return
	}
	if ev == nil || !soulCommandSourceAllowed(ev.ID(), ev.Source()) {
		setSoulCommandInvalidResult(ev, result)
		return
	}
	ctx := ev.Context()
	switch ev.ID() {
	case soulevents.TopicSoulCommandCreate:
		cmd, ok := ev.Data().(soulevents.CreateSoulCommand)
		if !ok {
			setMagicResult(result, soulevents.CreateSoulResult{}, common.ErrInvalidArgument)
			return
		}
		item, err := m.service.createSoulCommand(ctx, cmd)
		if err != nil {
			setMagicResult(result, soulevents.CreateSoulResult{}, err)
			return
		}
		setMagicResult(result, (soulevents.CreateSoulResult{Soul: item}).Clone(), nil)
	case soulevents.TopicSoulCommandGet:
		cmd, ok := ev.Data().(soulevents.GetSoulCommand)
		if !ok {
			setMagicResult(result, soulevents.GetSoulResult{}, common.ErrInvalidArgument)
			return
		}
		item, err := m.service.getSoul(ctx, cmd.SoulID)
		if err != nil {
			setMagicResult(result, soulevents.GetSoulResult{}, err)
			return
		}
		setMagicResult(result, (soulevents.GetSoulResult{Soul: item}).Clone(), nil)
	case soulevents.TopicSoulCommandList:
		cmd, ok := ev.Data().(soulevents.ListSoulsCommand)
		if !ok {
			setMagicResult(result, soulevents.ListSoulsResult{}, common.ErrInvalidArgument)
			return
		}
		items, nextCursor, err := m.service.listSouls(ctx, cmd.CursorAfterID, cmd.Limit)
		if err != nil {
			setMagicResult(result, soulevents.ListSoulsResult{}, err)
			return
		}
		setMagicResult(result, (soulevents.ListSoulsResult{Souls: items, NextCursor: nextCursor}).Clone(), nil)
	case soulevents.TopicSoulCommandUpdate:
		cmd, ok := ev.Data().(soulevents.UpdateSoulCommand)
		if !ok {
			setMagicResult(result, soulevents.UpdateSoulResult{}, common.ErrInvalidArgument)
			return
		}
		item, err := m.service.updateSoul(ctx, soulevents.CreateSoulCommand(cmd))
		if err != nil {
			setMagicResult(result, soulevents.UpdateSoulResult{}, err)
			return
		}
		setMagicResult(result, (soulevents.UpdateSoulResult{Soul: item}).Clone(), nil)
	case soulevents.TopicSoulCommandDelete:
		cmd, ok := ev.Data().(soulevents.DeleteSoulCommand)
		if !ok {
			setMagicResult(result, soulevents.DeleteSoulResult{}, common.ErrInvalidArgument)
			return
		}
		if err := m.service.deleteSoul(ctx, cmd.SoulID); err != nil {
			setMagicResult(result, soulevents.DeleteSoulResult{}, err)
			return
		}
		setMagicResult(result, soulevents.DeleteSoulResult{SoulID: cmd.SoulID, Deleted: true}, nil)
	case soulevents.TopicSoulCommandValidateCapability:
		cmd, ok := ev.Data().(soulevents.ValidateCapabilitySelectionCommand)
		if !ok {
			setMagicResult(result, soulevents.ValidateCapabilitySelectionResult{}, common.ErrInvalidArgument)
			return
		}
		if err := m.service.validateCapabilitySelection(ctx, cmd); err != nil {
			setMagicResult(result, soulevents.ValidateCapabilitySelectionResult{}, err)
			return
		}
		setMagicResult(result, soulevents.ValidateCapabilitySelectionResult{SoulID: cmd.SoulID, Validated: true}, nil)
	default:
		setMagicResult(result, struct{}{}, common.ErrInvalidArgument)
	}
}

func soulCommandSourceAllowed(topic, source string) bool {
	switch topic {
	case soulevents.TopicSoulCommandCreate,
		soulevents.TopicSoulCommandList,
		soulevents.TopicSoulCommandUpdate,
		soulevents.TopicSoulCommandDelete,
		soulevents.TopicSoulCommandValidateCapability:
		return source == applicationContextRegistrySource
	case soulevents.TopicSoulCommandGet:
		return source == applicationContextRegistrySource || source == applicationOwnerReadSource
	default:
		return false
	}
}

func setSoulCommandInvalidResult(ev magicevent.Event, result magicevent.Result) {
	topic := ""
	if ev != nil {
		topic = ev.ID()
	}
	switch topic {
	case soulevents.TopicSoulCommandCreate:
		setMagicResult(result, soulevents.CreateSoulResult{}, common.ErrInvalidArgument)
	case soulevents.TopicSoulCommandGet:
		setMagicResult(result, soulevents.GetSoulResult{}, common.ErrInvalidArgument)
	case soulevents.TopicSoulCommandList:
		setMagicResult(result, soulevents.ListSoulsResult{}, common.ErrInvalidArgument)
	case soulevents.TopicSoulCommandUpdate:
		setMagicResult(result, soulevents.UpdateSoulResult{}, common.ErrInvalidArgument)
	case soulevents.TopicSoulCommandDelete:
		setMagicResult(result, soulevents.DeleteSoulResult{}, common.ErrInvalidArgument)
	case soulevents.TopicSoulCommandValidateCapability:
		setMagicResult(result, soulevents.ValidateCapabilitySelectionResult{}, common.ErrInvalidArgument)
	default:
		setMagicResult(result, struct{}{}, common.ErrInvalidArgument)
	}
}

func setMagicResult[Response any](result magicevent.Result, data Response, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}
