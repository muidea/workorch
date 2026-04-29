package soul

import (
	"context"

	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateSoulCommand struct {
	SoulID               common.SoulID
	Name                 string
	Description          string
	BusinessDomains      []string
	SoulMD               string
	AllowedSkillRefs     []string
	AllowedMCPServerRefs []string
	AllowedToolKinds     []string
}

type UpdateSoulCommand = CreateSoulCommand

type ValidateCapabilitySelectionCommand struct {
	SoulID        common.SoulID
	SkillRefs     []string
	MCPServerRefs []string
	ToolKinds     []string
}

type Service interface {
	CreateSoul(context.Context, CreateSoulCommand) error
	UpdateSoul(context.Context, UpdateSoulCommand) error
	GetSoul(context.Context, common.SoulID) (*soulmodel.Soul, error)
	ListSouls(context.Context) ([]*soulmodel.Soul, error)
	ValidateCapabilitySelection(context.Context, ValidateCapabilitySelectionCommand) error
}

type Dependencies struct {
	CreateSoul                  func(context.Context, CreateSoulCommand) error
	UpdateSoul                  func(context.Context, UpdateSoulCommand) error
	GetSoul                     func(context.Context, common.SoulID) (*soulmodel.Soul, error)
	ListSouls                   func(context.Context) ([]*soulmodel.Soul, error)
	ValidateCapabilitySelection func(context.Context, ValidateCapabilitySelectionCommand) error
}

type Module struct {
	createSoul                  func(context.Context, CreateSoulCommand) error
	updateSoul                  func(context.Context, UpdateSoulCommand) error
	getSoul                     func(context.Context, common.SoulID) (*soulmodel.Soul, error)
	listSouls                   func(context.Context) ([]*soulmodel.Soul, error)
	validateCapabilitySelection func(context.Context, ValidateCapabilitySelectionCommand) error
}

func New(deps Dependencies) *Module {
	return &Module{
		createSoul:                  deps.CreateSoul,
		updateSoul:                  deps.UpdateSoul,
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
