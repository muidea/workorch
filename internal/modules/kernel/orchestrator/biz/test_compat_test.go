package orchestrator

import (
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	"github.com/muidea/workorch/internal/pkg/repository"
)

// orchestratorbiz keeps moved package-local tests readable without importing
// their own package through its old root path.
var orchestratorbiz orchestratorBizTestShim

type orchestratorBizTestShim struct{}

// Module keeps moved legacy test names readable. Production lifecycle wiring
// uses Biz exclusively.
type Module = Biz

func newTestBiz(owner *RuntimeOrchestrator, hub magicevent.Hub, background magictask.BackgroundRoutine) *Biz {
	return &Biz{Base: basebiz.New(ModuleID, hub, background), bizPtr: owner}
}

func (orchestratorBizTestShim) NewRuntimeOrchestrator(
	runs repository.RunRepository,
	workspaces repository.WorkspaceRepository,
	tickets repository.DecisionTicketRepository,
	approvals repository.ApprovalRepository,
) *RuntimeOrchestrator {
	return NewRuntimeOrchestrator(runs, workspaces, tickets, approvals)
}
