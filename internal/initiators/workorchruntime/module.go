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
	MemoryDeletions       repository.MemoryWorkspaceDeletionRepository
	AutoGovernanceTraces  repository.AutoGovernanceTraceRepository
	Skills                repository.SkillRepository
	MCPRegistry           repository.MCPRegistryRepository
	Results               repository.ResultRepository
	Artifacts             repository.ArtifactRepository
	ResultDeletions       repository.ResultWorkspaceDeletionRepository
	EventRecords          repository.EventRecordRepository
	EventStreamDeletions  repository.EventStreamWorkspaceDeletionRepository
	EventStreamPurger     repository.EventStreamWorkspacePurger
	Workspaces            repository.WorkspaceRepository
	WorkspaceCleaner      repository.WorkspaceDeletionMaterialCleaner
	Goals                 repository.GoalRepository
	DoDs                  repository.DoDRepository
	Runs                  repository.RunRepository
	Steps                 repository.StepRepository
	RunOwner              repository.RunOwnerRepositories
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	RuntimeCheckpoints    repository.RuntimeCheckpointRepository
	EvidenceRecords       repository.EvidenceRecordRepository
	CoverageWorkItems     repository.CoverageWorkItemRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
	OrchestratorDeletions repository.OrchestratorWorkspaceDeletionRepository
	OrchestratorPurger    repository.OrchestratorWorkspaceEvidencePurger
	Tickets               repository.DecisionTicketRepository
	Resolutions           repository.DecisionResolutionRepository
	Approvals             repository.ApprovalRepository
	DecisionDeletions     repository.DecisionWorkspaceDeletionRepository
	ApprovalDeletions     repository.ApprovalWorkspaceDeletionRepository
	OrchestratorOwner     repository.OrchestratorEvidenceOwnerRepositories
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
		Workspaces:              r.Workspaces,
		DeletionMaterialCleaner: r.WorkspaceCleaner,
		Goals:                   r.Goals,
		DoDs:                    r.DoDs,
		Templates:               r.WorkspaceTemplates,
	}
}

func (r repositorySet) Decision() common.DecisionRepositories {
	return common.DecisionRepositories{
		Tickets:            r.Tickets,
		Resolutions:        r.Resolutions,
		WorkspaceDeletions: r.DecisionDeletions,
	}
}

func (r repositorySet) Approval() common.ApprovalRepositories {
	return common.ApprovalRepositories{
		Approvals: r.Approvals, WorkspaceDeletions: r.ApprovalDeletions,
	}
}

func (r repositorySet) EventStream() common.EventStreamRepositories {
	return common.EventStreamRepositories{
		EventRecords: r.EventRecords, WorkspaceDeletions: r.EventStreamDeletions, WorkspacePurger: r.EventStreamPurger,
	}
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
		WorkspaceDeletions: r.MemoryDeletions,
	}
}

func (r repositorySet) Orchestrator() repository.OrchestratorEvidenceOwnerRepositories {
	return r.OrchestratorOwner
}

func (r repositorySet) Result() common.ResultRepositories {
	return common.ResultRepositories{
		Results:            r.Results,
		Artifacts:          r.Artifacts,
		WorkspaceDeletions: r.ResultDeletions,
	}
}

func (r repositorySet) Run() common.RunRepositories {
	return common.RunRepositories{
		Runs:        r.Runs,
		Steps:       r.Steps,
		Checkpoints: r.RuntimeCheckpoints,
		Owner:       r.RunOwner,
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
		workspaceRepository := memory.NewWorkspaceRepository()
		memoryOwner := memory.NewMemoryOwnerRepositorySet()
		orchestratorOwner := memory.NewOrchestratorOwnerRepositorySet()
		resultOwner := memory.NewResultOwnerRepositorySet()
		decisionOwner := memory.NewDecisionOwnerRepositorySet()
		approvalOwner := memory.NewApprovalOwnerRepositorySet()
		eventStreamOwner := memory.NewEventStreamOwnerRepositorySet()
		runOwner := memory.NewRunOwnerRepositorySet()
		return repositorySet{
			Souls:                 memory.NewSoulRepository(),
			WorkspaceTemplates:    memory.NewWorkspaceTemplateRepository(),
			MemoryItems:           memoryOwner.Items,
			MemoryRecallTraces:    memoryOwner.RecallTraces,
			MemoryDeletions:       memoryOwner.WorkspaceDeletions,
			AutoGovernanceTraces:  memory.NewAutoGovernanceTraceRepository(),
			Skills:                memory.NewSkillRepository(),
			MCPRegistry:           memory.NewMCPRegistryRepository(),
			Results:               resultOwner.Results,
			Artifacts:             resultOwner.Artifacts,
			ResultDeletions:       resultOwner.WorkspaceDeletions,
			EventRecords:          eventStreamOwner.EventRecords,
			EventStreamDeletions:  eventStreamOwner.WorkspaceDeletions,
			EventStreamPurger:     eventStreamOwner.EventRecords,
			Workspaces:            workspaceRepository,
			WorkspaceCleaner:      workspaceRepository,
			Goals:                 memory.NewGoalRepository(),
			DoDs:                  memory.NewDoDRepository(),
			Runs:                  runOwner.Runs(),
			Steps:                 runOwner.Steps(),
			RunOwner:              runOwner,
			ToolCalls:             orchestratorOwner.ToolCalls(),
			Patches:               orchestratorOwner.Patches(),
			ValidationResults:     orchestratorOwner.ValidationResults(),
			RuntimeCheckpoints:    runOwner.Checkpoints(),
			EvidenceRecords:       orchestratorOwner.EvidenceRecords(),
			CoverageWorkItems:     orchestratorOwner.CoverageWorkItems(),
			ArtifactContinuations: orchestratorOwner.ArtifactContinuations(),
			OrchestratorDeletions: orchestratorOwner.WorkspaceDeletions(),
			OrchestratorPurger:    orchestratorOwner.WorkspacePurger(),
			Tickets:               decisionOwner.Tickets,
			Resolutions:           decisionOwner.Resolutions,
			Approvals:             approvalOwner.Approvals,
			DecisionDeletions:     decisionOwner.WorkspaceDeletions,
			ApprovalDeletions:     approvalOwner.WorkspaceDeletions,
			OrchestratorOwner:     orchestratorOwner,
		}, nil
	case "file":
		dataRoot = workorchrepo.ResolveRepositoryDataRoot(backend, dataRoot)
		workspaceScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
		return repositorySet{
			Souls:                 fileRepo.NewSoulRepository(filepath.Join(dataRoot, "system", "souls")),
			WorkspaceTemplates:    fileRepo.NewWorkspaceTemplateRepository(filepath.Join(dataRoot, "system", "workspace_templates.json")),
			MemoryItems:           workspaceScopedRepos.MemoryItems,
			MemoryRecallTraces:    workspaceScopedRepos.MemoryRecallTraces,
			MemoryDeletions:       workspaceScopedRepos.MemoryDeletions,
			AutoGovernanceTraces:  fileRepo.NewAutoGovernanceTraceRepository(filepath.Join(dataRoot, "system", "auto_governance_trace.json")),
			Skills:                fileRepo.NewSkillRepository(filepath.Join(dataRoot, "system", "skill.json")),
			MCPRegistry:           fileRepo.NewMCPRegistryRepository(filepath.Join(dataRoot, "system", "mcp_registry.json")),
			Results:               workspaceScopedRepos.Results,
			Artifacts:             workspaceScopedRepos.Artifacts,
			ResultDeletions:       workspaceScopedRepos.ResultDeletions,
			EventRecords:          workspaceScopedRepos.EventRecords,
			EventStreamDeletions:  workspaceScopedRepos.EventStreamDeletions,
			EventStreamPurger:     workspaceScopedRepos.EventStreamPurger,
			Workspaces:            workspaceScopedRepos.Workspaces,
			WorkspaceCleaner:      workspaceScopedRepos.WorkspaceCleaner,
			Goals:                 workspaceScopedRepos.Goals,
			DoDs:                  workspaceScopedRepos.DoDs,
			Runs:                  workspaceScopedRepos.Runs,
			Steps:                 workspaceScopedRepos.Steps,
			RunOwner:              workspaceScopedRepos.RunOwner,
			ToolCalls:             workspaceScopedRepos.ToolCalls,
			Patches:               workspaceScopedRepos.Patches,
			ValidationResults:     workspaceScopedRepos.ValidationResults,
			RuntimeCheckpoints:    workspaceScopedRepos.RuntimeCheckpoints,
			EvidenceRecords:       workspaceScopedRepos.EvidenceRecords,
			CoverageWorkItems:     workspaceScopedRepos.CoverageWorkItems,
			ArtifactContinuations: workspaceScopedRepos.ArtifactContinuations,
			OrchestratorDeletions: workspaceScopedRepos.OrchestratorDeletions,
			OrchestratorPurger:    workspaceScopedRepos.OrchestratorPurger,
			Tickets:               workspaceScopedRepos.Tickets,
			Resolutions:           workspaceScopedRepos.Resolutions,
			Approvals:             workspaceScopedRepos.Approvals,
			DecisionDeletions:     workspaceScopedRepos.DecisionDeletions,
			ApprovalDeletions:     workspaceScopedRepos.ApprovalDeletions,
			OrchestratorOwner:     workspaceScopedRepos.OrchestratorEvidence,
		}, nil
	default:
		return repositorySet{}, fmt.Errorf("unsupported repository backend: %s", backend)
	}
}
