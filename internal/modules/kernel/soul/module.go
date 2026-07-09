package soul

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	soulbiz "github.com/muidea/workorch/internal/modules/kernel/soul/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/common"
	soulcontracts "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/contracts"
	soulevents "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/events"
	soulservice "github.com/muidea/workorch/internal/modules/kernel/soul/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = soulservice.Service
type CreateSoulCommand = soulcontracts.CreateSoulCommand
type UpdateSoulCommand = soulcontracts.UpdateSoulCommand
type ValidateCapabilitySelectionCommand = soulcontracts.ValidateCapabilitySelectionCommand

type Dependencies struct {
	Souls    repository.SoulRepository
	EventHub magicevent.Hub
}

type Module struct {
	bizPtr     *soulbiz.Service
	servicePtr *soulservice.Module
	eventHub   magicevent.Hub
	observer   magicevent.SimpleObserver
	topics     []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Souls == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.bizPtr = soulbiz.NewService(deps.Souls)
	m.servicePtr = soulservice.New(soulservice.Dependencies{
		CreateSoul:                  m.bizPtr.CreateSoul,
		UpdateSoul:                  m.bizPtr.UpdateSoul,
		DeleteSoul:                  m.bizPtr.DeleteSoul,
		GetSoul:                     m.bizPtr.GetSoul,
		ListSouls:                   m.bizPtr.ListSouls,
		ValidateCapabilitySelection: m.bizPtr.ValidateCapabilitySelection,
	})
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	if m.eventHub == nil {
		return nil
	}
	if m.observer != nil {
		return nil
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.topics = []string{
		soulevents.TopicSoulCommandCreate,
		soulevents.TopicSoulCommandGet,
		soulevents.TopicSoulCommandList,
		soulevents.TopicSoulCommandUpdate,
		soulevents.TopicSoulCommandDelete,
		soulevents.TopicSoulCommandValidateCapability,
	}
	for _, topic := range m.topics {
		m.observer.Subscribe(topic, m.handleSoulCommand)
	}
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.observer != nil {
		for idx := len(m.topics) - 1; idx >= 0; idx-- {
			m.observer.Unsubscribe(m.topics[idx])
		}
	}
	m.topics = nil
	m.observer = nil
	m.servicePtr = nil
	m.bizPtr = nil
	m.eventHub = nil
	return nil
}

func (m *Module) Service() Service {
	if m == nil || m.servicePtr == nil {
		return nil
	}
	return m.servicePtr
}

func (m *Module) handleSoulCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeSoulCommand(ev.Context(), ev.ID(), ev.Data())
	setMagicResult(result, response, err)
}

func (m *Module) executeSoulCommand(ctx context.Context, topic string, data any) (soulevents.SoulCommandResult, error) {
	if m == nil || m.servicePtr == nil {
		return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
	}
	switch topic {
	case soulevents.TopicSoulCommandCreate:
		cmd, ok := data.(soulevents.CreateSoulCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.CreateSoul(ctx, cmd); err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		item, err := m.servicePtr.GetSoul(ctx, cmd.SoulID)
		if err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{Soul: item}, nil
	case soulevents.TopicSoulCommandGet:
		cmd, ok := data.(soulevents.GetSoulCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.servicePtr.GetSoul(ctx, cmd.SoulID)
		if err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{Soul: item}, nil
	case soulevents.TopicSoulCommandList:
		if _, ok := data.(soulevents.ListSoulsCommand); !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.servicePtr.ListSouls(ctx)
		if err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{Souls: items}, nil
	case soulevents.TopicSoulCommandUpdate:
		cmd, ok := data.(soulevents.UpdateSoulCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.UpdateSoul(ctx, cmd); err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		item, err := m.servicePtr.GetSoul(ctx, cmd.SoulID)
		if err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{Soul: item}, nil
	case soulevents.TopicSoulCommandDelete:
		cmd, ok := data.(soulevents.DeleteSoulCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.DeleteSoul(ctx, cmd.SoulID); err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{}, nil
	case soulevents.TopicSoulCommandValidateCapability:
		cmd, ok := data.(soulevents.ValidateCapabilitySelectionCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.ValidateCapabilitySelection(ctx, cmd); err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{}, nil
	default:
		return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
	}
}

func setMagicResult(result magicevent.Result, data any, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}
