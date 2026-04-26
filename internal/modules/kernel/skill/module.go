package skill

import (
	"context"

	skillbiz "github.com/muidea/workorch/internal/modules/kernel/skill/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/common"
	skillservice "github.com/muidea/workorch/internal/modules/kernel/skill/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = skillservice.Service
type CreateSkillCommand = skillservice.CreateSkillCommand
type UpdateSkillCommand = skillservice.UpdateSkillCommand

type Dependencies struct {
	Skills repository.SkillRepository
}

type Module struct {
	bizPtr     *skillbiz.Service
	servicePtr *skillservice.Module
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	if deps.Skills == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = skillbiz.NewService(deps.Skills)
	m.servicePtr = skillservice.New(skillservice.Dependencies{
		CreateSkill: m.bizPtr.CreateSkill,
		UpdateSkill: m.bizPtr.UpdateSkill,
		GetSkill:    m.bizPtr.GetSkill,
		ListSkills:  m.bizPtr.ListSkills,
	})
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.servicePtr = nil
	m.bizPtr = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}
