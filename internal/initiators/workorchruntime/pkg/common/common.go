package common

import (
	approvalservice "github.com/muidea/workorch/internal/modules/blocks/approval/service"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	mcpservice "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
	memoryservice "github.com/muidea/workorch/internal/modules/kernel/memory/service"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratorservice "github.com/muidea/workorch/internal/modules/kernel/orchestrator/service"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	skillservice "github.com/muidea/workorch/internal/modules/kernel/skill/service"
	soulservice "github.com/muidea/workorch/internal/modules/kernel/soul/service"
	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const InitiatorID = "workorch/runtime"

type RepositorySet struct {
	Souls              repository.SoulRepository
	MemoryItems        repository.MemoryItemRepository
	Skills             repository.SkillRepository
	MCPServers         repository.MCPServerRepository
	Workspaces         repository.WorkspaceRepository
	Goals              repository.GoalRepository
	DoDs               repository.DoDRepository
	Runs               repository.RunRepository
	Steps              repository.StepRepository
	ToolCalls          repository.ToolCallRepository
	Patches            repository.PatchRepository
	ValidationResults  repository.ValidationResultRepository
	RuntimeCheckpoints repository.RuntimeCheckpointRepository
	Tickets            repository.DecisionTicketRepository
	Resolutions        repository.DecisionResolutionRepository
	Approvals          repository.ApprovalRepository
}

type Helper interface {
	Repositories() RepositorySet
	EventHub() events.Hub
	LegacyRuntimeAdapter() orchestratorbiz.RuntimeAdapter
	SetLegacyRuntimeAdapter(orchestratorbiz.RuntimeAdapter)
	ShellRuntimeAdapter() orchestratorbiz.RuntimeAdapter
	SetShellRuntimeAdapter(orchestratorbiz.RuntimeAdapter)
	SSHRuntimeAdapter() orchestratorbiz.RuntimeAdapter
	SetSSHRuntimeAdapter(orchestratorbiz.RuntimeAdapter)
	MCPRuntimeAdapter() orchestratorbiz.RuntimeAdapter
	SetMCPRuntimeAdapter(orchestratorbiz.RuntimeAdapter)
	LLMRuntimeAdapter() *llmbiz.RuntimeAdapter
	SetLLMRuntimeAdapter(*llmbiz.RuntimeAdapter)
	SoulService() soulservice.Service
	SetSoulService(soulservice.Service)
	MemoryService() memoryservice.Service
	SetMemoryService(memoryservice.Service)
	SkillService() skillservice.Service
	SetSkillService(skillservice.Service)
	MCPService() mcpservice.Service
	SetMCPService(mcpservice.Service)
	WorkspaceService() workspaceservice.Service
	SetWorkspaceService(workspaceservice.Service)
	DecisionService() decisionservice.Service
	SetDecisionService(decisionservice.Service)
	ApprovalService() approvalservice.Service
	SetApprovalService(approvalservice.Service)
	OrchestratorService() orchestratorservice.Service
	SetOrchestratorService(orchestratorservice.Service)
	RunService() runservice.Service
	SetRunService(runservice.Service)
}
