package soul

import (
	"context"

	soulbiz "github.com/muidea/workorch/internal/modules/kernel/soul/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/common"
	soulservice "github.com/muidea/workorch/internal/modules/kernel/soul/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = soulservice.Service
type CreateSoulCommand = soulservice.CreateSoulCommand
type UpdateSoulCommand = soulservice.UpdateSoulCommand
type ValidateCapabilitySelectionCommand = soulservice.ValidateCapabilitySelectionCommand

type Dependencies struct {
	Souls repository.SoulRepository
}

type Module struct {
	bizPtr     *soulbiz.Service
	servicePtr *soulservice.Module
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	if deps.Souls == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = soulbiz.NewService(deps.Souls)
	m.servicePtr = soulservice.New(soulservice.Dependencies{
		CreateSoul:                  m.bizPtr.CreateSoul,
		UpdateSoul:                  m.bizPtr.UpdateSoul,
		GetSoul:                     m.bizPtr.GetSoul,
		ListSouls:                   m.bizPtr.ListSouls,
		ValidateCapabilitySelection: m.bizPtr.ValidateCapabilitySelection,
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
