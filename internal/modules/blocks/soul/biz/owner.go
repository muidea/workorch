package soul

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/common"
	soulcontracts "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/contracts"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type CreateSoulCommand = soulcontracts.CreateSoulCommand
type UpdateSoulCommand = soulcontracts.UpdateSoulCommand
type ValidateCapabilitySelectionCommand = soulcontracts.ValidateCapabilitySelectionCommand

type Dependencies struct {
	Souls             repository.SoulRepository
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service *Service
	souls   repository.SoulRepository
	topics  []string
	started bool
}

func New(eventHub magicevent.Hub, background task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.Souls == nil {
		return nil, common.ErrInvalidArgument
	}
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, background), service: NewService(deps.Souls), souls: deps.Souls}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	m.topics = []string{
		soulevents.TopicSoulCommandCreate,
		soulevents.TopicSoulCommandGet,
		soulevents.TopicSoulCommandList,
		soulevents.TopicSoulCommandUpdate,
		soulevents.TopicSoulCommandDelete,
		soulevents.TopicSoulCommandValidateCapability,
	}
	for _, topic := range m.topics {
		m.SubscribeFunc(topic, m.handleSoulCommand)
	}
	m.topics = append(m.topics, soulevents.TopicSoulCommandEnsureDefaults)
	m.SubscribeFunc(soulevents.TopicSoulCommandEnsureDefaults, m.handleSystemDefaultsCommand)
	m.started = true
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	for idx := len(m.topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.topics[idx])
	}
	m.topics = nil
	m.service = nil
	m.souls = nil
	m.started = false
}

func (m *Biz) handleSystemDefaultsCommand(ev magicevent.Event, result magicevent.Result) {
	if _, ok := ev.Data().(soulevents.EnsureDefaultsCommand); !ok || m == nil || m.souls == nil {
		setSystemSoulDefaultsResult(result, common.ErrInvalidArgument)
		return
	}
	setSystemSoulDefaultsResult(result, ensureBuiltinSoulDefaults(ev.Context(), m.souls))
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
	response, err := m.executeSoulCommand(ev)
	setMagicResult(result, response, err)
}

func (m *Biz) executeSoulCommand(ev magicevent.Event) (soulevents.SoulCommandResult, error) {
	if m == nil || m.service == nil {
		return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case soulevents.TopicSoulCommandCreate:
		cmd, ok := ev.Data().(soulevents.CreateSoulCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.service.CreateSoul(ctx, createSoulContract(cmd)); err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		item, err := m.service.GetSoul(ctx, cmd.SoulID)
		if err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{Soul: item}, nil
	case soulevents.TopicSoulCommandGet:
		cmd, ok := ev.Data().(soulevents.GetSoulCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.service.GetSoul(ctx, cmd.SoulID)
		if err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{Soul: item}, nil
	case soulevents.TopicSoulCommandList:
		if _, ok := ev.Data().(soulevents.ListSoulsCommand); !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.service.ListSouls(ctx)
		if err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{Souls: items}, nil
	case soulevents.TopicSoulCommandUpdate:
		cmd, ok := ev.Data().(soulevents.UpdateSoulCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.service.UpdateSoul(ctx, updateSoulContract(cmd)); err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		item, err := m.service.GetSoul(ctx, cmd.SoulID)
		if err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{Soul: item}, nil
	case soulevents.TopicSoulCommandDelete:
		cmd, ok := ev.Data().(soulevents.DeleteSoulCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.service.DeleteSoul(ctx, cmd.SoulID); err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{}, nil
	case soulevents.TopicSoulCommandValidateCapability:
		cmd, ok := ev.Data().(soulevents.ValidateCapabilitySelectionCommand)
		if !ok {
			return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.service.ValidateCapabilitySelection(ctx, validateCapabilitySelectionContract(cmd)); err != nil {
			return soulevents.SoulCommandResult{}, err
		}
		return soulevents.SoulCommandResult{}, nil
	default:
		return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
	}
}

func createSoulContract(cmd soulevents.CreateSoulCommand) soulcontracts.CreateSoulCommand {
	return soulcontracts.CreateSoulCommand{
		SoulID: cmd.SoulID, Name: cmd.Name, Description: cmd.Description, SoulMD: cmd.SoulMD,
		BusinessDomains:             append([]string(nil), cmd.BusinessDomains...),
		AllowedSkillRefs:            append([]string(nil), cmd.AllowedSkillRefs...),
		AllowedMCPServerRefs:        append([]string(nil), cmd.AllowedMCPServerRefs...),
		AllowedCapabilityRefs:       append([]string(nil), cmd.AllowedCapabilityRefs...),
		AllowedCapabilityCategories: append([]string(nil), cmd.AllowedCapabilityCategories...),
		AllowedToolKinds:            append([]string(nil), cmd.AllowedToolKinds...),
	}
}

func updateSoulContract(cmd soulevents.UpdateSoulCommand) soulcontracts.UpdateSoulCommand {
	return soulcontracts.UpdateSoulCommand(createSoulContract(soulevents.CreateSoulCommand(cmd)))
}

func validateCapabilitySelectionContract(cmd soulevents.ValidateCapabilitySelectionCommand) soulcontracts.ValidateCapabilitySelectionCommand {
	return soulcontracts.ValidateCapabilitySelectionCommand{
		SoulID:               cmd.SoulID,
		SkillRefs:            append([]string(nil), cmd.SkillRefs...),
		MCPServerRefs:        append([]string(nil), cmd.MCPServerRefs...),
		CapabilityRefs:       append([]string(nil), cmd.CapabilityRefs...),
		CapabilityCategories: append([]string(nil), cmd.CapabilityCategories...),
		ToolKinds:            append([]string(nil), cmd.ToolKinds...),
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
