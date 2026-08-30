package common

import (
	"github.com/muidea/workorch/internal/pkg/repository"
)

const InitiatorID = "workorch/repositories"

type WorkspaceRepositories struct {
	Owner repository.WorkspaceOwnerRepositories
}

type DecisionRepositories struct {
	Owner repository.DecisionOwnerRepositories
}

type ApprovalRepositories struct {
	Owner repository.ApprovalOwnerRepositories
}

type EventStreamRepositories struct {
	Owner repository.EventStreamOwnerRepositories
}

type GovernanceRepositories struct {
	AutoGovernanceTraces repository.AutoGovernanceTraceRepository
}

type MCPRepositories struct {
	Registry repository.MCPRegistryRepository
}

type MemoryRepositories struct {
	Owner repository.MemoryOwnerRepositories
}

type ResultRepositories struct {
	Owner repository.ResultOwnerRepositories
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
