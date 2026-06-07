package skill

import (
	"context"

	skillcontracts "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/contracts"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateSkillCommand = skillcontracts.CreateSkillCommand
type UpdateSkillCommand = skillcontracts.UpdateSkillCommand

type Service interface {
	CreateSkill(context.Context, CreateSkillCommand) error
	UpdateSkill(context.Context, UpdateSkillCommand) error
	GetSkill(context.Context, common.SkillID) (*skillmodel.Skill, error)
	ListSkills(context.Context) ([]*skillmodel.Skill, error)
}

type Dependencies struct {
	CreateSkill func(context.Context, CreateSkillCommand) error
	UpdateSkill func(context.Context, UpdateSkillCommand) error
	GetSkill    func(context.Context, common.SkillID) (*skillmodel.Skill, error)
	ListSkills  func(context.Context) ([]*skillmodel.Skill, error)
}

type Module struct {
	createSkill func(context.Context, CreateSkillCommand) error
	updateSkill func(context.Context, UpdateSkillCommand) error
	getSkill    func(context.Context, common.SkillID) (*skillmodel.Skill, error)
	listSkills  func(context.Context) ([]*skillmodel.Skill, error)
}

func New(deps Dependencies) *Module {
	return &Module{
		createSkill: deps.CreateSkill,
		updateSkill: deps.UpdateSkill,
		getSkill:    deps.GetSkill,
		listSkills:  deps.ListSkills,
	}
}

func (m *Module) CreateSkill(ctx context.Context, cmd CreateSkillCommand) error {
	if m == nil || m.createSkill == nil {
		return common.ErrInvalidArgument
	}
	return m.createSkill(ctx, cmd)
}

func (m *Module) UpdateSkill(ctx context.Context, cmd UpdateSkillCommand) error {
	if m == nil || m.updateSkill == nil {
		return common.ErrInvalidArgument
	}
	return m.updateSkill(ctx, cmd)
}

func (m *Module) GetSkill(ctx context.Context, id common.SkillID) (*skillmodel.Skill, error) {
	if m == nil || m.getSkill == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.getSkill(ctx, id)
}

func (m *Module) ListSkills(ctx context.Context) ([]*skillmodel.Skill, error) {
	if m == nil || m.listSkills == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.listSkills(ctx)
}
