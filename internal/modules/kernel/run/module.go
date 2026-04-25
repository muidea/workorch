package run

import (
	"context"

	runbiz "github.com/muidea/workorch/internal/modules/kernel/run/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/run/pkg/common"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = runservice.Service
type Orchestrator = runbiz.Orchestrator
type CreateRunCommand = runservice.CreateRunCommand
type StartRunCommand = runservice.StartRunCommand
type ResumeRunCommand = runservice.ResumeRunCommand
type BlockOnDecisionCommand = runservice.BlockOnDecisionCommand
type BlockOnApprovalCommand = runservice.BlockOnApprovalCommand
type ValidateRunCommand = runservice.ValidateRunCommand
type CompleteRunCommand = runservice.CompleteRunCommand
type CancelRunCommand = runservice.CancelRunCommand
type SupersedeRunCommand = runservice.SupersedeRunCommand
type FailRunCommand = runservice.FailRunCommand

type Dependencies struct {
	Runs         repository.RunRepository
	Workspaces   repository.WorkspaceRepository
	Orchestrator runbiz.Orchestrator
	EventHub     events.Hub
}

type Module struct {
	bizPtr     *runbiz.Service
	servicePtr *runservice.Module
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	if deps.Runs == nil || deps.Workspaces == nil || deps.Orchestrator == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = runbiz.NewService(deps.Runs, deps.Workspaces, deps.Orchestrator, deps.EventHub)
	m.servicePtr = runservice.New(runservice.Dependencies{
		CreateRun:       m.bizPtr.CreateRun,
		EnsureRun:       m.bizPtr.EnsureRun,
		StartRun:        m.bizPtr.StartRun,
		EnsureStarted:   m.bizPtr.EnsureStarted,
		ResumeRun:       m.bizPtr.ResumeRun,
		EnsureResumed:   m.bizPtr.EnsureResumed,
		BlockOnDecision: m.bizPtr.BlockOnDecision,
		EnsureDecision:  m.bizPtr.EnsureBlockedOnDecision,
		BlockOnApproval: m.bizPtr.BlockOnApproval,
		EnsureApproval:  m.bizPtr.EnsureBlockedOnApproval,
		ValidateRun:     m.bizPtr.ValidateRun,
		CompleteRun:     m.bizPtr.CompleteRun,
		CancelRun:       m.bizPtr.CancelRun,
		SupersedeRun:    m.bizPtr.SupersedeRun,
		FailRun:         m.bizPtr.FailRun,
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
