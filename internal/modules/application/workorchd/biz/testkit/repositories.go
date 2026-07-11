package testkit

import (
	"path/filepath"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/repository"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

// RepositoryExports is a test-only repository fixture surface. Production
// application exports intentionally do not expose owner repositories.
type RepositoryExports struct {
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

type Exports struct {
	Repositories   RepositoryExports
	Infrastructure bootstrap.InfrastructureExports
}

type sessionExports interface {
	Exports() (bootstrap.Exports, error)
}

func RuntimeExports(runtime *bootstrap.Runtime) Exports {
	if runtime == nil {
		return Exports{}
	}
	return exportsWithRepositories(runtime.Exports())
}

// OpenRepositories returns test-only repository fixtures for direct seed and
// verification without starting owner module lifecycles.
func OpenRepositories(backend string, dataRoot string) RepositoryExports {
	return buildRepositories(backend, dataRoot)
}

func SessionExports(session sessionExports) (Exports, error) {
	if session == nil {
		return Exports{}, nil
	}
	exports, err := session.Exports()
	if err != nil {
		return Exports{}, err
	}
	return exportsWithRepositories(exports), nil
}

func exportsWithRepositories(exports bootstrap.Exports) Exports {
	repositories := RepositoryExports{}
	if provider, ok := exports.Infrastructure.CommandHub.(interface{ testRepositories() RepositoryExports }); ok {
		repositories = provider.testRepositories()
	}
	return Exports{
		Repositories:   repositories,
		Infrastructure: exports.Infrastructure,
	}
}

func buildRepositories(backend string, dataRoot string) RepositoryExports {
	backend = workorchrepo.NormalizeRepositoryBackend(backend)
	switch backend {
	case "memory":
		return RepositoryExports{
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
		}
	case "file":
		dataRoot = workorchrepo.ResolveRepositoryDataRoot(backend, dataRoot)
		workspaceScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
		return RepositoryExports{
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
		}
	default:
		panic("unsupported repository backend: " + backend)
	}
}
