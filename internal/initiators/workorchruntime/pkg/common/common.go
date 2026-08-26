package common

import (
	"github.com/muidea/workorch/internal/pkg/repository"
)

const InitiatorID = "workorch/repositories"

type WorkspaceRepositories struct {
	Workspaces              repository.WorkspaceRepository
	DeletionMaterialCleaner repository.WorkspaceDeletionMaterialCleaner
	Goals                   repository.GoalRepository
	DoDs                    repository.DoDRepository
	Templates               repository.WorkspaceTemplateRepository
}

type DecisionRepositories struct {
	Tickets            repository.DecisionTicketRepository
	Resolutions        repository.DecisionResolutionRepository
	WorkspaceDeletions repository.DecisionWorkspaceDeletionRepository
}

type ApprovalRepositories struct {
	Approvals          repository.ApprovalRepository
	WorkspaceDeletions repository.ApprovalWorkspaceDeletionRepository
}

type EventStreamRepositories struct {
	EventRecords       repository.EventRecordRepository
	WorkspaceDeletions repository.EventStreamWorkspaceDeletionRepository
	WorkspacePurger    repository.EventStreamWorkspacePurger
}

type GovernanceRepositories struct {
	AutoGovernanceTraces repository.AutoGovernanceTraceRepository
}

type MCPRepositories struct {
	Registry repository.MCPRegistryRepository
}

type MemoryRepositories struct {
	MemoryItems        repository.MemoryItemRepository
	MemoryRecallTraces repository.MemoryRecallTraceRepository
	WorkspaceDeletions repository.MemoryWorkspaceDeletionRepository
}

type ResultRepositories struct {
	Results            repository.ResultRepository
	Artifacts          repository.ArtifactRepository
	WorkspaceDeletions repository.ResultWorkspaceDeletionRepository
}

type RunRepositories struct {
	Runs        repository.RunRepository
	Steps       repository.StepRepository
	Checkpoints repository.RuntimeCheckpointRepository
	Owner       repository.RunOwnerRepositories
}

type SkillRepositories struct {
	Skills repository.SkillRepository
}

type SoulRepositories struct {
	Souls repository.SoulRepository
}

type RepositoryProvider interface {
	Workspace() WorkspaceRepositories
	Decision() DecisionRepositories
	Approval() ApprovalRepositories
	EventStream() EventStreamRepositories
	Governance() GovernanceRepositories
	MCP() MCPRepositories
	Memory() MemoryRepositories
	Orchestrator() repository.OrchestratorEvidenceOwnerRepositories
	Result() ResultRepositories
	Run() RunRepositories
	Skill() SkillRepositories
	Soul() SoulRepositories
}

type Provider interface {
	RepositoryProvider() RepositoryProvider
}
