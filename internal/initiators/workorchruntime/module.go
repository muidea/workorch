package workorchruntime

import (
	"fmt"
	"os"
	"path/filepath"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/framework/plugin/initiator"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
	approvalservice "github.com/muidea/workorch/internal/modules/blocks/approval/service"
	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

const (
	envBackend  = "WORKORCH_REPOSITORY_BACKEND"
	envDataRoot = "WORKORCH_DATA_DIR"
)

func init() {
	initiator.Register(New())
}

type Runtime struct {
	repositories         common.RepositorySet
	eventHub             workorchevents.Hub
	legacyRuntimeAdapter orchestratorbiz.RuntimeAdapter
	workspaceService     workspaceservice.Service
	decisionService      decisionservice.Service
	approvalService      approvalservice.Service
	orchestratorService  orchestratorservice.Service
	runService           runservice.Service
}

func New() *Runtime {
	return &Runtime{}
}

func (r *Runtime) ID() string {
	return common.InitiatorID
}

func (r *Runtime) Weight() int {
	return 10
}

func (r *Runtime) Setup(eventHub event.Hub, _ task.BackgroundRoutine) *cd.Error {
	repositories, err := buildRepositories(os.Getenv(envBackend), os.Getenv(envDataRoot))
	if err != nil {
		return cd.NewError(cd.IllegalParam, "invalid argument: "+err.Error())
	}
	r.repositories = repositories
	r.eventHub = workorchevents.NewMagicHubWithHub(eventHub)
	return nil
}

func (r *Runtime) Run() *cd.Error {
	return nil
}

func (r *Runtime) Teardown() {
	r.repositories = common.RepositorySet{}
	r.eventHub = nil
	r.legacyRuntimeAdapter = nil
	r.workspaceService = nil
	r.decisionService = nil
	r.approvalService = nil
	r.orchestratorService = nil
	r.runService = nil
}

func (r *Runtime) Repositories() common.RepositorySet {
	if r == nil {
		return common.RepositorySet{}
	}
	return r.repositories
}

func (r *Runtime) EventHub() workorchevents.Hub {
	if r == nil {
		return nil
	}
	return r.eventHub
}

func (r *Runtime) LegacyRuntimeAdapter() orchestratorbiz.RuntimeAdapter {
	if r == nil {
		return nil
	}
	return r.legacyRuntimeAdapter
}

func (r *Runtime) SetLegacyRuntimeAdapter(adapter orchestratorbiz.RuntimeAdapter) {
	if r != nil {
		r.legacyRuntimeAdapter = adapter
	}
}

func (r *Runtime) WorkspaceService() workspaceservice.Service {
	if r == nil {
		return nil
	}
	return r.workspaceService
}

func (r *Runtime) SetWorkspaceService(service workspaceservice.Service) {
	if r != nil {
		r.workspaceService = service
	}
}

func (r *Runtime) DecisionService() decisionservice.Service {
	if r == nil {
		return nil
	}
	return r.decisionService
}

func (r *Runtime) SetDecisionService(service decisionservice.Service) {
	if r != nil {
		r.decisionService = service
	}
}

func (r *Runtime) ApprovalService() approvalservice.Service {
	if r == nil {
		return nil
	}
	return r.approvalService
}

func (r *Runtime) SetApprovalService(service approvalservice.Service) {
	if r != nil {
		r.approvalService = service
	}
}

func (r *Runtime) OrchestratorService() orchestratorservice.Service {
	if r == nil {
		return nil
	}
	return r.orchestratorService
}

func (r *Runtime) SetOrchestratorService(service orchestratorservice.Service) {
	if r != nil {
		r.orchestratorService = service
	}
}

func (r *Runtime) RunService() runservice.Service {
	if r == nil {
		return nil
	}
	return r.runService
}

func (r *Runtime) SetRunService(service runservice.Service) {
	if r != nil {
		r.runService = service
	}
}

func buildRepositories(backend string, dataRoot string) (common.RepositorySet, error) {
	switch backend {
	case "", "memory":
		return common.RepositorySet{
			Workspaces:  memory.NewWorkspaceRepository(),
			Goals:       memory.NewGoalRepository(),
			DoDs:        memory.NewDoDRepository(),
			Runs:        memory.NewRunRepository(),
			Steps:       memory.NewStepRepository(),
			Tickets:     memory.NewDecisionTicketRepository(),
			Resolutions: memory.NewDecisionResolutionRepository(),
			Approvals:   memory.NewApprovalRepository(),
		}, nil
	case "file":
		if dataRoot == "" {
			dataRoot = filepath.Join(os.TempDir(), "workorchd-data")
		}
		return common.RepositorySet{
			Workspaces:  fileRepo.NewWorkspaceRepository(filepath.Join(dataRoot, "workspace.json")),
			Goals:       fileRepo.NewGoalRepository(filepath.Join(dataRoot, "goal.json")),
			DoDs:        fileRepo.NewDoDRepository(filepath.Join(dataRoot, "dod.json")),
			Runs:        fileRepo.NewRunRepository(filepath.Join(dataRoot, "run.json")),
			Steps:       fileRepo.NewStepRepository(filepath.Join(dataRoot, "step.json")),
			Tickets:     fileRepo.NewDecisionTicketRepository(filepath.Join(dataRoot, "ticket.json")),
			Resolutions: fileRepo.NewDecisionResolutionRepository(filepath.Join(dataRoot, "resolution.json")),
			Approvals:   fileRepo.NewApprovalRepository(filepath.Join(dataRoot, "approval.json")),
		}, nil
	default:
		return common.RepositorySet{}, fmt.Errorf("unsupported repository backend: %s", backend)
	}
}
