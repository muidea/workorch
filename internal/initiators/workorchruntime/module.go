package workorchruntime

import (
	"context"
	"fmt"
	"path/filepath"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/framework/plugin/initiator"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

func init() {
	initiator.Register(New())
}

type Runtime struct {
	repositories  common.RepositorySet
	eventHub      workorchevents.Hub
	runtimePolicy orchestratormodel.RuntimePolicy
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

func (r *Runtime) Setup(_ context.Context, eventHub event.Hub, _ task.BackgroundRoutine) *cd.Error {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return cd.NewError(cd.IllegalParam, "invalid argument: "+err.Error())
	}
	repositories, err := buildRepositories(cfg.Repository.Backend, cfg.Repository.DataDir)
	if err != nil {
		return cd.NewError(cd.IllegalParam, "invalid argument: "+err.Error())
	}
	r.repositories = repositories
	r.eventHub = workorchevents.NewMagicHubWithHubOwner(eventHub, common.InitiatorID)
	r.runtimePolicy = workorchruntimepolicy.FromConfig(cfg)
	return nil
}

func (r *Runtime) Run(ctx context.Context) *cd.Error {
	if err := workorchdefaults.EnsureBuiltinDefaults(ctx, r.repositories.Souls, r.repositories.WorkspaceTemplates, r.repositories.MCPServers, r.repositories.MCPCapabilities); err != nil {
		return cd.NewError(cd.IllegalParam, "ensure builtin defaults: "+err.Error())
	}
	return nil
}

func (r *Runtime) Teardown(_ context.Context) {
	r.repositories = common.RepositorySet{}
	r.eventHub = nil
	r.runtimePolicy = orchestratormodel.RuntimePolicy{}
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

func (r *Runtime) RuntimePolicy() orchestratormodel.RuntimePolicy {
	if r == nil {
		return orchestratormodel.RuntimePolicy{}
	}
	return r.runtimePolicy
}

func buildRepositories(backend string, dataRoot string) (common.RepositorySet, error) {
	backend = workorchrepo.NormalizeRepositoryBackend(backend)
	switch backend {
	case "memory":
		return common.RepositorySet{
			Souls:                 memory.NewSoulRepository(),
			WorkspaceTemplates:    memory.NewWorkspaceTemplateRepository(),
			MemoryItems:           memory.NewMemoryItemRepository(),
			MemoryRecallTraces:    memory.NewMemoryRecallTraceRepository(),
			AutoGovernanceTraces:  memory.NewAutoGovernanceTraceRepository(),
			Skills:                memory.NewSkillRepository(),
			MCPServers:            memory.NewMCPServerRepository(),
			MCPCapabilities:       memory.NewMCPCapabilityRepository(),
			Results:               memory.NewResultRepository(),
			Artifacts:             memory.NewArtifactRepository(),
			EventRecords:          memory.NewEventRecordRepository(),
			Workspaces:            memory.NewWorkspaceRepository(),
			Goals:                 memory.NewGoalRepository(),
			DoDs:                  memory.NewDoDRepository(),
			Runs:                  memory.NewRunRepository(),
			Steps:                 memory.NewStepRepository(),
			ToolCalls:             memory.NewToolCallRepository(),
			Patches:               memory.NewPatchRepository(),
			ValidationResults:     memory.NewValidationResultRepository(),
			RuntimeCheckpoints:    memory.NewRuntimeCheckpointRepository(),
			ArtifactContinuations: memory.NewArtifactContinuationRepository(),
			Tickets:               memory.NewDecisionTicketRepository(),
			Resolutions:           memory.NewDecisionResolutionRepository(),
			Approvals:             memory.NewApprovalRepository(),
		}, nil
	case "file":
		dataRoot = workorchrepo.ResolveRepositoryDataRoot(backend, dataRoot)
		workspaceScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
		return common.RepositorySet{
			Souls:                 fileRepo.NewSoulRepository(filepath.Join(dataRoot, "system", "souls")),
			WorkspaceTemplates:    fileRepo.NewWorkspaceTemplateRepository(filepath.Join(dataRoot, "system", "workspace_templates.json")),
			MemoryItems:           workspaceScopedRepos.MemoryItems,
			MemoryRecallTraces:    workspaceScopedRepos.MemoryRecallTraces,
			AutoGovernanceTraces:  fileRepo.NewAutoGovernanceTraceRepository(filepath.Join(dataRoot, "system", "auto_governance_trace.json")),
			Skills:                fileRepo.NewSkillRepository(filepath.Join(dataRoot, "system", "skill.json")),
			MCPServers:            fileRepo.NewMCPServerRepository(filepath.Join(dataRoot, "system", "mcp_server.json")),
			MCPCapabilities:       fileRepo.NewMCPCapabilityRepository(filepath.Join(dataRoot, "system", "mcp_capability.json")),
			Results:               workspaceScopedRepos.Results,
			Artifacts:             workspaceScopedRepos.Artifacts,
			EventRecords:          workspaceScopedRepos.EventRecords,
			Workspaces:            workspaceScopedRepos.Workspaces,
			Goals:                 workspaceScopedRepos.Goals,
			DoDs:                  workspaceScopedRepos.DoDs,
			Runs:                  workspaceScopedRepos.Runs,
			Steps:                 workspaceScopedRepos.Steps,
			ToolCalls:             workspaceScopedRepos.ToolCalls,
			Patches:               workspaceScopedRepos.Patches,
			ValidationResults:     workspaceScopedRepos.ValidationResults,
			RuntimeCheckpoints:    workspaceScopedRepos.RuntimeCheckpoints,
			ArtifactContinuations: workspaceScopedRepos.ArtifactContinuations,
			Tickets:               workspaceScopedRepos.Tickets,
			Resolutions:           workspaceScopedRepos.Resolutions,
			Approvals:             workspaceScopedRepos.Approvals,
		}, nil
	default:
		return common.RepositorySet{}, fmt.Errorf("unsupported repository backend: %s", backend)
	}
}
