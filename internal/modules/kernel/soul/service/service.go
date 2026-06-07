package soul

import (
	"context"

	soulcontracts "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/contracts"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateSoulCommand = soulcontracts.CreateSoulCommand
type UpdateSoulCommand = soulcontracts.UpdateSoulCommand
type ValidateCapabilitySelectionCommand = soulcontracts.ValidateCapabilitySelectionCommand

type Service interface {
	CreateSoul(context.Context, CreateSoulCommand) error
	UpdateSoul(context.Context, UpdateSoulCommand) error
	DeleteSoul(context.Context, common.SoulID) error
	GetSoul(context.Context, common.SoulID) (*soulmodel.Soul, error)
	ListSouls(context.Context) ([]*soulmodel.Soul, error)
	ValidateCapabilitySelection(context.Context, ValidateCapabilitySelectionCommand) error
}

type Dependencies struct {
	CreateSoul                  func(context.Context, CreateSoulCommand) error
	UpdateSoul                  func(context.Context, UpdateSoulCommand) error
	DeleteSoul                  func(context.Context, common.SoulID) error
	GetSoul                     func(context.Context, common.SoulID) (*soulmodel.Soul, error)
	ListSouls                   func(context.Context) ([]*soulmodel.Soul, error)
	ValidateCapabilitySelection func(context.Context, ValidateCapabilitySelectionCommand) error
}

type Module struct {
	createSoul                  func(context.Context, CreateSoulCommand) error
	updateSoul                  func(context.Context, UpdateSoulCommand) error
	deleteSoul                  func(context.Context, common.SoulID) error
	getSoul                     func(context.Context, common.SoulID) (*soulmodel.Soul, error)
	listSouls                   func(context.Context) ([]*soulmodel.Soul, error)
	validateCapabilitySelection func(context.Context, ValidateCapabilitySelectionCommand) error
}

func New(deps Dependencies) *Module {
	return &Module{
		createSoul:                  deps.CreateSoul,
		updateSoul:                  deps.UpdateSoul,
		deleteSoul:                  deps.DeleteSoul,
		getSoul:                     deps.GetSoul,
		listSouls:                   deps.ListSouls,
		validateCapabilitySelection: deps.ValidateCapabilitySelection,
	}
}

func (m *Module) CreateSoul(ctx context.Context, cmd CreateSoulCommand) error {
	if m == nil || m.createSoul == nil {
		return common.ErrInvalidArgument
	}
	return m.createSoul(ctx, cmd)
}

func (m *Module) UpdateSoul(ctx context.Context, cmd UpdateSoulCommand) error {
	if m == nil || m.updateSoul == nil {
		return common.ErrInvalidArgument
	}
	return m.updateSoul(ctx, cmd)
}

func (m *Module) DeleteSoul(ctx context.Context, id common.SoulID) error {
	if m == nil || m.deleteSoul == nil {
		return common.ErrInvalidArgument
	}
	return m.deleteSoul(ctx, id)
}

func (m *Module) GetSoul(ctx context.Context, id common.SoulID) (*soulmodel.Soul, error) {
	if m == nil || m.getSoul == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.getSoul(ctx, id)
}

func (m *Module) ListSouls(ctx context.Context) ([]*soulmodel.Soul, error) {
	if m == nil || m.listSouls == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.listSouls(ctx)
}

func (m *Module) ValidateCapabilitySelection(ctx context.Context, cmd ValidateCapabilitySelectionCommand) error {
	if m == nil || m.validateCapabilitySelection == nil {
		return common.ErrInvalidArgument
	}
	return m.validateCapabilitySelection(ctx, cmd)
}
