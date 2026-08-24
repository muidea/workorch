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
	"github.com/muidea/workorch/internal/pkg/repository"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func init() {
	initiator.Register(New())
}

type RepositoryInitiator struct {
	repositories *repositorySet
}

var _ common.Provider = (*RepositoryInitiator)(nil)

type repositorySet struct {
	Souls                 repository.SoulRepository
	WorkspaceTemplates    repository.WorkspaceTemplateRepository
	MemoryItems           repository.MemoryItemRepository
	MemoryRecallTraces    repository.MemoryRecallTraceRepository
	AutoGovernanceTraces  repository.AutoGovernanceTraceRepository
	Skills                repository.SkillRepository
	MCPRegistry           repository.MCPRegistryRepository
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

func New() *RepositoryInitiator {
	return &RepositoryInitiator{}
}

func (r *RepositoryInitiator) ID() string {
	return common.InitiatorID
}

func (r *RepositoryInitiator) Weight() int {
	return 10
}

func (r *RepositoryInitiator) Setup(_ context.Context, _ event.Hub, _ task.BackgroundRoutine) *cd.Error {
	// Revoke any previously published provider before attempting a fresh
	// setup. A failed repeated Setup must not leave stale repositories visible.
	r.repositories = nil
	cfg, err := workorchconfig.Current()
	if err != nil {
		return cd.NewError(cd.IllegalParam, "invalid argument: "+err.Error())
	}
	repositories, err := buildRepositories(cfg.Repository.Backend, cfg.Repository.DataDir)
	if err != nil {
		return cd.NewError(cd.IllegalParam, "invalid argument: "+err.Error())
	}
	r.repositories = &repositories
	return nil
}

func (r *RepositoryInitiator) Run(_ context.Context) *cd.Error {
	return nil
}

func (r *RepositoryInitiator) Teardown(_ context.Context) {
	r.repositories = nil
}

func (r *RepositoryInitiator) RepositoryProvider() common.RepositoryProvider {
	if r == nil || r.repositories == nil {
		return nil
	}
	return r.repositories
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
		Tickets:     r.Tickets,
		Resolutions: r.Resolutions,
	}
}

func (r repositorySet) Approval() common.ApprovalRepositories {
	return common.ApprovalRepositories{
		Approvals: r.Approvals,
	}
}

func (r repositorySet) EventStream() common.EventStreamRepositories {
	return common.EventStreamRepositories{EventRecords: r.EventRecords}
}

func (r repositorySet) Governance() common.GovernanceRepositories {
	return common.GovernanceRepositories{AutoGovernanceTraces: r.AutoGovernanceTraces}
}

func (r repositorySet) MCP() common.MCPRepositories {
	return common.MCPRepositories{Registry: r.MCPRegistry}
}

func (r repositorySet) Memory() common.MemoryRepositories {
	return common.MemoryRepositories{
		MemoryItems:        r.MemoryItems,
		MemoryRecallTraces: r.MemoryRecallTraces,
	}
}

func (r repositorySet) Orchestrator() common.OrchestratorRepositories {
	return common.OrchestratorRepositories{
		ToolCalls:             r.ToolCalls,
		Patches:               r.Patches,
		ValidationResults:     r.ValidationResults,
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
			MCPRegistry:           memory.NewMCPRegistryRepository(),
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
			MCPRegistry:           fileRepo.NewMCPRegistryRepository(filepath.Join(dataRoot, "system", "mcp_registry.json")),
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
