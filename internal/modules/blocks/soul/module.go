package soul

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	soulbiz "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/common"
	soulcontracts "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/contracts"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	systemevents "github.com/muidea/workorch/internal/modules/kernel/system/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
)

const ModuleID = modulecommon.ModuleID

type CreateSoulCommand = soulcontracts.CreateSoulCommand
type UpdateSoulCommand = soulcontracts.UpdateSoulCommand
type ValidateCapabilitySelectionCommand = soulcontracts.ValidateCapabilitySelectionCommand

type Dependencies struct {
	Souls    repository.SoulRepository
	EventHub magicevent.Hub
}

type Module struct {
	bizPtr   *soulbiz.Service
	souls    repository.SoulRepository
	eventHub magicevent.Hub
	observer magicevent.SimpleObserver
	topics   []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Souls == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.souls = deps.Souls
	m.bizPtr = soulbiz.NewService(deps.Souls)
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.bizPtr == nil {
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
	m.topics = append(m.topics, systemevents.TopicSystemCommandEnsureSoulDefaults)
	m.observer.Subscribe(systemevents.TopicSystemCommandEnsureSoulDefaults, m.handleSystemDefaultsCommand)
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
	m.bizPtr = nil
	m.souls = nil
	m.eventHub = nil
	return nil
}

func (m *Module) handleSystemDefaultsCommand(ev magicevent.Event, result magicevent.Result) {
	if _, ok := ev.Data().(systemevents.EnsureSoulDefaultsCommand); !ok || m == nil || m.souls == nil {
		setSystemSoulDefaultsResult(result, common.ErrInvalidArgument)
		return
	}
	setSystemSoulDefaultsResult(result, workorchdefaults.EnsureBuiltinSoulDefaults(ev.Context(), m.souls))
}

func setSystemSoulDefaultsResult(result magicevent.Result, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(systemevents.EnsureSoulDefaultsResult{Ensured: true}, nil)
}

func (m *Module) handleSoulCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeSoulCommand(ev)
	setMagicResult(result, response, err)
}

func (m *Module) executeSoulCommand(ev magicevent.Event) (soulevents.SoulCommandResult, error) {
	if m == nil || m.bizPtr == nil {
		return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case soulevents.TopicSoulCommandCreate:
		cmd, ok := ev.Data().(soulevents.CreateSoulCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.bizPtr.CreateSoul(ctx, cmd); err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		item, err := m.bizPtr.GetSoul(ctx, cmd.SoulID)
		if err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{Soul: item}, nil
	case soulevents.TopicSoulCommandGet:
		cmd, ok := ev.Data().(soulevents.GetSoulCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.bizPtr.GetSoul(ctx, cmd.SoulID)
		if err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{Soul: item}, nil
	case soulevents.TopicSoulCommandList:
		if _, ok := ev.Data().(soulevents.ListSoulsCommand); !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.bizPtr.ListSouls(ctx)
		if err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{Souls: items}, nil
	case soulevents.TopicSoulCommandUpdate:
		cmd, ok := ev.Data().(soulevents.UpdateSoulCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.bizPtr.UpdateSoul(ctx, cmd); err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		item, err := m.bizPtr.GetSoul(ctx, cmd.SoulID)
		if err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{Soul: item}, nil
	case soulevents.TopicSoulCommandDelete:
		cmd, ok := ev.Data().(soulevents.DeleteSoulCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.bizPtr.DeleteSoul(ctx, cmd.SoulID); err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{}, nil
	case soulevents.TopicSoulCommandValidateCapability:
		cmd, ok := ev.Data().(soulevents.ValidateCapabilitySelectionCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.bizPtr.ValidateCapabilitySelection(ctx, cmd); err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{}, nil
	default:
		return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
	}
}

func setMagicResult(result magicevent.Result, data soulevents.SoulCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}
