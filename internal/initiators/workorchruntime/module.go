package workorchruntime

import (
	"fmt"
	"path/filepath"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/framework/plugin/initiator"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
	appbootstrap "github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	approvalservice "github.com/muidea/workorch/internal/modules/blocks/approval/service"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	eventstreamservice "github.com/muidea/workorch/internal/modules/kernel/eventstream/service"
	mcpservice "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
	memoryservice "github.com/muidea/workorch/internal/modules/kernel/memory/service"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	skillservice "github.com/muidea/workorch/internal/modules/kernel/skill/service"
	soulservice "github.com/muidea/workorch/internal/modules/kernel/soul/service"
	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func init() {
	initiator.Register(New())
}

type Runtime struct {
	repositories        common.RepositorySet
	eventHub            workorchevents.Hub
	shellRuntimeAdapter orchestratorbiz.RuntimeAdapter
	sshRuntimeAdapter   orchestratorbiz.RuntimeAdapter
	mcpRuntimeAdapter   orchestratorbiz.RuntimeAdapter
	llmRuntimeAdapter   *llmbiz.RuntimeAdapter
	soulService         soulservice.Service
	memoryService       memoryservice.Service
	skillService        skillservice.Service
	mcpService          mcpservice.Service
	eventStreamService  eventstreamservice.Service
	workspaceService    workspaceservice.Service
	decisionService     decisionservice.Service
	approvalService     approvalservice.Service
	orchestratorService orchestratorservice.Service
	runService          runservice.Service
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
	cfg, err := workorchconfig.Current()
	if err != nil {
		return cd.NewError(cd.IllegalParam, "invalid argument: "+err.Error())
	}
	repositories, err := buildRepositories(cfg.Repository.Backend, cfg.Repository.DataDir)
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
	r.shellRuntimeAdapter = nil
	r.sshRuntimeAdapter = nil
	r.mcpRuntimeAdapter = nil
	r.llmRuntimeAdapter = nil
	r.soulService = nil
	r.memoryService = nil
	r.skillService = nil
	r.mcpService = nil
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

func (r *Runtime) ShellRuntimeAdapter() orchestratorbiz.RuntimeAdapter {
	if r == nil {
		return nil
	}
	return r.shellRuntimeAdapter
}

func (r *Runtime) SetShellRuntimeAdapter(adapter orchestratorbiz.RuntimeAdapter) {
	if r != nil {
		r.shellRuntimeAdapter = adapter
	}
}

func (r *Runtime) SSHRuntimeAdapter() orchestratorbiz.RuntimeAdapter {
	if r == nil {
		return nil
	}
	return r.sshRuntimeAdapter
}

func (r *Runtime) SetSSHRuntimeAdapter(adapter orchestratorbiz.RuntimeAdapter) {
	if r != nil {
		r.sshRuntimeAdapter = adapter
	}
}

func (r *Runtime) MCPRuntimeAdapter() orchestratorbiz.RuntimeAdapter {
	if r == nil {
		return nil
	}
	return r.mcpRuntimeAdapter
}

func (r *Runtime) SetMCPRuntimeAdapter(adapter orchestratorbiz.RuntimeAdapter) {
	if r != nil {
		r.mcpRuntimeAdapter = adapter
	}
}

func (r *Runtime) LLMRuntimeAdapter() *llmbiz.RuntimeAdapter {
	if r == nil {
		return nil
	}
	return r.llmRuntimeAdapter
}

func (r *Runtime) SetLLMRuntimeAdapter(adapter *llmbiz.RuntimeAdapter) {
	if r != nil {
		r.llmRuntimeAdapter = adapter
	}
}

func (r *Runtime) SoulService() soulservice.Service {
	if r == nil {
		return nil
	}
	return r.soulService
}

func (r *Runtime) SetSoulService(service soulservice.Service) {
	if r != nil {
		r.soulService = service
	}
}

func (r *Runtime) MemoryService() memoryservice.Service {
	if r == nil {
		return nil
	}
	return r.memoryService
}

func (r *Runtime) SetMemoryService(service memoryservice.Service) {
	if r != nil {
		r.memoryService = service
	}
}

func (r *Runtime) SkillService() skillservice.Service {
	if r == nil {
		return nil
	}
	return r.skillService
}

func (r *Runtime) SetSkillService(service skillservice.Service) {
	if r != nil {
		r.skillService = service
	}
}

func (r *Runtime) MCPService() mcpservice.Service {
	if r == nil {
		return nil
	}
	return r.mcpService
}

func (r *Runtime) SetMCPService(service mcpservice.Service) {
	if r != nil {
		r.mcpService = service
	}
}

func (r *Runtime) EventStreamService() eventstreamservice.Service {
	if r == nil {
		return nil
	}
	return r.eventStreamService
}

func (r *Runtime) SetEventStreamService(service eventstreamservice.Service) {
	if r != nil {
		r.eventStreamService = service
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
			Souls:              memory.NewSoulRepository(),
			WorkspaceTemplates: memory.NewWorkspaceTemplateRepository(),
			MemoryItems:        memory.NewMemoryItemRepository(),
			Skills:             memory.NewSkillRepository(),
			MCPServers:         memory.NewMCPServerRepository(),
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			EventRecords:       memory.NewEventRecordRepository(),
			Workspaces:         memory.NewWorkspaceRepository(),
			Goals:              memory.NewGoalRepository(),
			DoDs:               memory.NewDoDRepository(),
			Runs:               memory.NewRunRepository(),
			Steps:              memory.NewStepRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			Tickets:            memory.NewDecisionTicketRepository(),
			Resolutions:        memory.NewDecisionResolutionRepository(),
			Approvals:          memory.NewApprovalRepository(),
		}, nil
	case "file":
		dataRoot = appbootstrap.ResolveRepositoryDataRoot(backend, dataRoot)
		workspaceScopedRepos := appbootstrap.NewWorkspaceScopedFileRepositorySet(dataRoot)
		return common.RepositorySet{
			Souls:              fileRepo.NewSoulRepository(filepath.Join(dataRoot, "system", "souls")),
			WorkspaceTemplates: fileRepo.NewWorkspaceTemplateRepository(filepath.Join(dataRoot, "system", "workspace_templates.json")),
			MemoryItems:        workspaceScopedRepos.MemoryItems,
			Skills:             fileRepo.NewSkillRepository(filepath.Join(dataRoot, "system", "skill.json")),
			MCPServers:         fileRepo.NewMCPServerRepository(filepath.Join(dataRoot, "system", "mcp_server.json")),
			Results:            workspaceScopedRepos.Results,
			Artifacts:          workspaceScopedRepos.Artifacts,
			EventRecords:       workspaceScopedRepos.EventRecords,
			Workspaces:         workspaceScopedRepos.Workspaces,
			Goals:              workspaceScopedRepos.Goals,
			DoDs:               workspaceScopedRepos.DoDs,
			Runs:               workspaceScopedRepos.Runs,
			Steps:              workspaceScopedRepos.Steps,
			ToolCalls:          workspaceScopedRepos.ToolCalls,
			Patches:            workspaceScopedRepos.Patches,
			ValidationResults:  workspaceScopedRepos.ValidationResults,
			RuntimeCheckpoints: workspaceScopedRepos.RuntimeCheckpoints,
			Tickets:            workspaceScopedRepos.Tickets,
			Resolutions:        workspaceScopedRepos.Resolutions,
			Approvals:          workspaceScopedRepos.Approvals,
		}, nil
	default:
		return common.RepositorySet{}, fmt.Errorf("unsupported repository backend: %s", backend)
	}
}
