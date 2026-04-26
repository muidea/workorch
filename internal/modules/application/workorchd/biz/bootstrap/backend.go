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
	memoryItem        repository.MemoryItemRepository
	skill             repository.SkillRepository
	mcpServer         repository.MCPServerRepository
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
			memoryItem:        memory.NewMemoryItemRepository(),
			skill:             memory.NewSkillRepository(),
			mcpServer:         memory.NewMCPServerRepository(),
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
		if dataRoot == "" {
			dataRoot = filepath.Join(os.TempDir(), "workorchd-data")
		}
		return repositorySet{
			soul:              fileRepo.NewSoulRepository(filepath.Join(dataRoot, "soul.json")),
			memoryItem:        fileRepo.NewMemoryItemRepository(filepath.Join(dataRoot, "memory_item.json")),
			skill:             fileRepo.NewSkillRepository(filepath.Join(dataRoot, "skill.json")),
			mcpServer:         fileRepo.NewMCPServerRepository(filepath.Join(dataRoot, "mcp_server.json")),
			workspace:         fileRepo.NewWorkspaceRepository(filepath.Join(dataRoot, "workspace.json")),
			goal:              fileRepo.NewGoalRepository(filepath.Join(dataRoot, "goal.json")),
			dod:               fileRepo.NewDoDRepository(filepath.Join(dataRoot, "dod.json")),
			run:               fileRepo.NewRunRepository(filepath.Join(dataRoot, "run.json")),
			step:              fileRepo.NewStepRepository(filepath.Join(dataRoot, "step.json")),
			toolCall:          fileRepo.NewToolCallRepository(filepath.Join(dataRoot, "tool_call.json")),
			patch:             fileRepo.NewPatchRepository(filepath.Join(dataRoot, "patch.json")),
			validationResult:  fileRepo.NewValidationResultRepository(filepath.Join(dataRoot, "validation_result.json")),
			runtimeCheckpoint: fileRepo.NewRuntimeCheckpointRepository(filepath.Join(dataRoot, "runtime_checkpoint.json")),
			ticket:            fileRepo.NewDecisionTicketRepository(filepath.Join(dataRoot, "ticket.json")),
			resolution:        fileRepo.NewDecisionResolutionRepository(filepath.Join(dataRoot, "resolution.json")),
			approval:          fileRepo.NewApprovalRepository(filepath.Join(dataRoot, "approval.json")),
		}
	default:
		panic("unsupported repository backend: " + backend)
	}
}
