package bootstrap

import (
	"os"
	"path/filepath"

	"github.com/muidea/workorch/internal/pkg/repository"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

type repositorySet struct {
	soul              repository.SoulRepository
	workspaceTemplate repository.WorkspaceTemplateRepository
	memoryItem        repository.MemoryItemRepository
	skill             repository.SkillRepository
	mcpServer         repository.MCPServerRepository
	result            repository.ResultRepository
	artifact          repository.ArtifactRepository
	eventRecord       repository.EventRecordRepository
	workspace         repository.WorkspaceRepository
	goal              repository.GoalRepository
	dod               repository.DoDRepository
	run               repository.RunRepository
	step              repository.StepRepository
	toolCall          repository.ToolCallRepository
	patch             repository.PatchRepository
	validationResult  repository.ValidationResultRepository
	runtimeCheckpoint repository.RuntimeCheckpointRepository
	ticket            repository.DecisionTicketRepository
	resolution        repository.DecisionResolutionRepository
	approval          repository.ApprovalRepository
}

func buildRepositories(backend string, dataRoot string) repositorySet {
	switch backend {
	case "", "memory":
		return repositorySet{
			soul:              memory.NewSoulRepository(),
			workspaceTemplate: memory.NewWorkspaceTemplateRepository(),
			memoryItem:        memory.NewMemoryItemRepository(),
			skill:             memory.NewSkillRepository(),
			mcpServer:         memory.NewMCPServerRepository(),
			result:            memory.NewResultRepository(),
			artifact:          memory.NewArtifactRepository(),
			eventRecord:       memory.NewEventRecordRepository(),
			workspace:         memory.NewWorkspaceRepository(),
			goal:              memory.NewGoalRepository(),
			dod:               memory.NewDoDRepository(),
			run:               memory.NewRunRepository(),
			step:              memory.NewStepRepository(),
			toolCall:          memory.NewToolCallRepository(),
			patch:             memory.NewPatchRepository(),
			validationResult:  memory.NewValidationResultRepository(),
			runtimeCheckpoint: memory.NewRuntimeCheckpointRepository(),
			ticket:            memory.NewDecisionTicketRepository(),
			resolution:        memory.NewDecisionResolutionRepository(),
			approval:          memory.NewApprovalRepository(),
		}
	case "file":
		dataRoot = ResolveRepositoryDataRoot(backend, dataRoot)
		workspaceScopedRepos := NewWorkspaceScopedFileRepositorySet(dataRoot)
		return repositorySet{
			soul:              fileRepo.NewSoulRepository(filepath.Join(dataRoot, "system", "souls")),
			workspaceTemplate: fileRepo.NewWorkspaceTemplateRepository(filepath.Join(dataRoot, "system", "workspace_templates.json")),
			memoryItem:        workspaceScopedRepos.MemoryItems,
			skill:             fileRepo.NewSkillRepository(filepath.Join(dataRoot, "system", "skill.json")),
			mcpServer:         fileRepo.NewMCPServerRepository(filepath.Join(dataRoot, "system", "mcp_server.json")),
			result:            workspaceScopedRepos.Results,
			artifact:          workspaceScopedRepos.Artifacts,
			eventRecord:       workspaceScopedRepos.EventRecords,
			workspace:         workspaceScopedRepos.Workspaces,
			goal:              workspaceScopedRepos.Goals,
			dod:               workspaceScopedRepos.DoDs,
			run:               workspaceScopedRepos.Runs,
			step:              workspaceScopedRepos.Steps,
			toolCall:          workspaceScopedRepos.ToolCalls,
			patch:             workspaceScopedRepos.Patches,
			validationResult:  workspaceScopedRepos.ValidationResults,
			runtimeCheckpoint: workspaceScopedRepos.RuntimeCheckpoints,
			ticket:            workspaceScopedRepos.Tickets,
			resolution:        workspaceScopedRepos.Resolutions,
			approval:          workspaceScopedRepos.Approvals,
		}
	default:
		panic("unsupported repository backend: " + backend)
	}
}

func ResolveRepositoryDataRoot(backend string, dataRoot string) string {
	if backend != "file" {
		return ""
	}
	if dataRoot == "" {
		return filepath.Join(os.TempDir(), "workorchd-data")
	}
	return dataRoot
}
