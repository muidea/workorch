package common

import (
	approvalservice "github.com/muidea/workorch/internal/modules/blocks/approval/service"
	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const InitiatorID = "workorch/runtime"

type RepositorySet struct {
	Workspaces  repository.WorkspaceRepository
	Runs        repository.RunRepository
	Steps       repository.StepRepository
	Tickets     repository.DecisionTicketRepository
	Resolutions repository.DecisionResolutionRepository
	Approvals   repository.ApprovalRepository
}

type Helper interface {
	Repositories() RepositorySet
	EventHub() events.Hub
	LegacyRuntimeAdapter() orchestratorbiz.RuntimeAdapter
	SetLegacyRuntimeAdapter(orchestratorbiz.RuntimeAdapter)
	WorkspaceService() workspaceservice.Service
	SetWorkspaceService(workspaceservice.Service)
	DecisionService() decisionservice.Service
	SetDecisionService(decisionservice.Service)
	ApprovalService() approvalservice.Service
	SetApprovalService(approvalservice.Service)
	OrchestratorService() orchestratorservice.Service
	SetOrchestratorService(orchestratorservice.Service)
	RunService() runservice.Service
	SetRunService(runservice.Service)
}
