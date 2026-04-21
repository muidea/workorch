package orchestrator

import (
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = orchestratorservice.Service
type RuntimeAdapter = orchestratorbiz.RuntimeAdapter
type RuntimeOrchestrator = orchestratorbiz.RuntimeOrchestrator

func NewRuntimeOrchestrator(
	runtime orchestratorbiz.RuntimeAdapter,
	runRepo repository.RunRepository,
	stepRepo repository.StepRepository,
	workspaceRepo repository.WorkspaceRepository,
	ticketRepo repository.DecisionTicketRepository,
	approvalRepo repository.ApprovalRepository,
	eventHub events.Hub,
) *orchestratorbiz.RuntimeOrchestrator {
	return orchestratorbiz.NewRuntimeOrchestrator(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		approvalRepo,
		eventHub,
	)
}
