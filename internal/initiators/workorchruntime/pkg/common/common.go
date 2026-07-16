package common

import (
	"github.com/muidea/workorch/internal/pkg/repository"
)

const InitiatorID = "workorch/repositories"

type WorkspaceRepositories struct {
	Workspaces repository.WorkspaceRepository
	Goals      repository.GoalRepository
	DoDs       repository.DoDRepository
	Templates  repository.WorkspaceTemplateRepository
}

type DecisionRepositories struct {
	Tickets     repository.DecisionTicketRepository
	Resolutions repository.DecisionResolutionRepository
}

type ApprovalRepositories struct {
	Approvals repository.ApprovalRepository
}

type EventStreamRepositories struct {
	EventRecords repository.EventRecordRepository
}

type GovernanceRepositories struct {
	AutoGovernanceTraces repository.AutoGovernanceTraceRepository
}

type MCPRepositories struct {
	Servers      repository.MCPServerRepository
	Capabilities repository.MCPCapabilityRepository
}

type MemoryRepositories struct {
	MemoryItems        repository.MemoryItemRepository
	MemoryRecallTraces repository.MemoryRecallTraceRepository
}

type OrchestratorRepositories struct {
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	EvidenceRecords       repository.EvidenceRecordRepository
	CoverageWorkItems     repository.CoverageWorkItemRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
}

type ResultRepositories struct {
	Results   repository.ResultRepository
	Artifacts repository.ArtifactRepository
}

type RunRepositories struct {
	Runs        repository.RunRepository
	Steps       repository.StepRepository
	Checkpoints repository.RuntimeCheckpointRepository
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
	Orchestrator() OrchestratorRepositories
	Result() ResultRepositories
	Run() RunRepositories
	Skill() SkillRepositories
	Soul() SoulRepositories
}

type Provider interface {
	RepositoryProvider() RepositoryProvider
}
