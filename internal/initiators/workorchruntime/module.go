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
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

func init() {
	initiator.Register(New())
}

type Runtime struct {
	repositories  repositorySet
	eventHub      workorchevents.Hub
	commandHub    event.Hub
	runtimePolicy workorchruntimepolicy.RuntimePolicy
}

type repositorySet struct {
	Souls                 repository.SoulRepository
	WorkspaceTemplates    repository.WorkspaceTemplateRepository
	MemoryItems           repository.MemoryItemRepository
	MemoryRecallTraces    repository.MemoryRecallTraceRepository
	AutoGovernanceTraces  repository.AutoGovernanceTraceRepository
	Skills                repository.SkillRepository
	MCPServers            repository.MCPServerRepository
	MCPCapabilities       repository.MCPCapabilityRepository
	Results               repository.ResultRepository
	Artifacts             repository.ArtifactRepository
	EventRecords          repository.EventRecordRepository
	Workspaces            repository.WorkspaceRepository
	Goals                 repository.GoalRepository
	DoDs                  repository.DoDRepository
	Runs                  repository.RunRepository
	Steps                 repository.StepRepository
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	RuntimeCheckpoints    repository.RuntimeCheckpointRepository
	EvidenceRecords       repository.EvidenceRecordRepository
	CoverageWorkItems     repository.CoverageWorkItemRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
	Tickets               repository.DecisionTicketRepository
	Resolutions           repository.DecisionResolutionRepository
	Approvals             repository.ApprovalRepository
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
	r.commandHub = eventHub
	r.eventHub = workorchevents.NewMagicHubWithHubOwner(eventHub, common.InitiatorID)
	r.runtimePolicy = workorchruntimepolicy.FromConfig(cfg)
	return nil
}

func (r *Runtime) Run(_ context.Context) *cd.Error {
	return nil
}

func (r *Runtime) Teardown(_ context.Context) {
	r.repositories = repositorySet{}
	r.eventHub = nil
	r.commandHub = nil
	r.runtimePolicy = workorchruntimepolicy.RuntimePolicy{}
}

func (r *Runtime) RepositoryProvider() common.RepositoryProvider {
	if r == nil {
		return nil
	}
	return r.repositories
}

func (r *Runtime) EventHub() workorchevents.Hub {
	if r == nil {
		return nil
	}
	return r.eventHub
}

func (r *Runtime) CommandHub() event.Hub {
	if r == nil {
		return nil
	}
	return r.commandHub
}

func (r *Runtime) RuntimePolicy() workorchruntimepolicy.RuntimePolicy {
	if r == nil {
		return workorchruntimepolicy.RuntimePolicy{}
	}
	return r.runtimePolicy
}

func (r repositorySet) System() common.SystemRepositories {
	return common.SystemRepositories{
		Souls:           r.Souls,
		Templates:       r.WorkspaceTemplates,
		MCPServers:      r.MCPServers,
		MCPCapabilities: r.MCPCapabilities,
	}
}

func (r repositorySet) Workspace() common.WorkspaceRepositories {
	return common.WorkspaceRepositories{
		Workspaces: r.Workspaces,
		Goals:      r.Goals,
		DoDs:       r.DoDs,
		Templates:  r.WorkspaceTemplates,
	}
}

func (r repositorySet) Decision() common.DecisionRepositories {
	return common.DecisionRepositories{
		Tickets:         r.Tickets,
		Resolutions:     r.Resolutions,
		EvidenceRecords: r.EvidenceRecords,
	}
}

func (r repositorySet) Approval() common.ApprovalRepositories {
	return common.ApprovalRepositories{
		Approvals:       r.Approvals,
		EvidenceRecords: r.EvidenceRecords,
	}
}

func (r repositorySet) EventStream() common.EventStreamRepositories {
	return common.EventStreamRepositories{EventRecords: r.EventRecords}
}

func (r repositorySet) Governance() common.GovernanceRepositories {
	return common.GovernanceRepositories{AutoGovernanceTraces: r.AutoGovernanceTraces}
}

func (r repositorySet) MCP() common.MCPRepositories {
	return common.MCPRepositories{
		Servers:      r.MCPServers,
		Capabilities: r.MCPCapabilities,
	}
}

func (r repositorySet) Memory() common.MemoryRepositories {
	return common.MemoryRepositories{
		MemoryItems:        r.MemoryItems,
		MemoryRecallTraces: r.MemoryRecallTraces,
	}
}

func (r repositorySet) Orchestrator() common.OrchestratorRepositories {
	return common.OrchestratorRepositories{
		Steps:                 r.Steps,
		ToolCalls:             r.ToolCalls,
		Patches:               r.Patches,
		ValidationResults:     r.ValidationResults,
		RuntimeCheckpoints:    r.RuntimeCheckpoints,
		EvidenceRecords:       r.EvidenceRecords,
		CoverageWorkItems:     r.CoverageWorkItems,
		ArtifactContinuations: r.ArtifactContinuations,
	}
}

func (r repositorySet) Result() common.ResultRepositories {
	return common.ResultRepositories{
		Results:   r.Results,
		Artifacts: r.Artifacts,
	}
}

func (r repositorySet) Run() common.RunRepositories {
	return common.RunRepositories{
		Runs:        r.Runs,
		Steps:       r.Steps,
		Checkpoints: r.RuntimeCheckpoints,
	}
}

func (r repositorySet) Skill() common.SkillRepositories {
	return common.SkillRepositories{Skills: r.Skills}
}

func (r repositorySet) Soul() common.SoulRepositories {
	return common.SoulRepositories{Souls: r.Souls}
}

func buildRepositories(backend string, dataRoot string) (repositorySet, error) {
	backend = workorchrepo.NormalizeRepositoryBackend(backend)
	switch backend {
	case "memory":
		return repositorySet{
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
			EvidenceRecords:       memory.NewEvidenceRecordRepository(),
			CoverageWorkItems:     memory.NewCoverageWorkItemRepository(),
			ArtifactContinuations: memory.NewArtifactContinuationRepository(),
			Tickets:               memory.NewDecisionTicketRepository(),
			Resolutions:           memory.NewDecisionResolutionRepository(),
			Approvals:             memory.NewApprovalRepository(),
		}, nil
	case "file":
		dataRoot = workorchrepo.ResolveRepositoryDataRoot(backend, dataRoot)
		workspaceScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
		return repositorySet{
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
			EvidenceRecords:       workspaceScopedRepos.EvidenceRecords,
			CoverageWorkItems:     workspaceScopedRepos.CoverageWorkItems,
			ArtifactContinuations: workspaceScopedRepos.ArtifactContinuations,
			Tickets:               workspaceScopedRepos.Tickets,
			Resolutions:           workspaceScopedRepos.Resolutions,
			Approvals:             workspaceScopedRepos.Approvals,
		}, nil
	default:
		return repositorySet{}, fmt.Errorf("unsupported repository backend: %s", backend)
	}
}
