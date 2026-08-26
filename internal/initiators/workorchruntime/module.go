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
	MemoryOwner           repository.MemoryOwnerRepositories
	AutoGovernanceTraces  repository.AutoGovernanceTraceRepository
	Skills                repository.SkillRepository
	MCPRegistry           repository.MCPRegistryRepository
	ResultOwner           repository.ResultOwnerRepositories
	EventStreamOwner      repository.EventStreamOwnerRepositories
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
	DecisionOwner         repository.DecisionOwnerRepositories
	ApprovalOwner         repository.ApprovalOwnerRepositories
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
	return common.DecisionRepositories{Owner: r.DecisionOwner}
}

func (r repositorySet) Approval() common.ApprovalRepositories {
	return common.ApprovalRepositories{Owner: r.ApprovalOwner}
}

func (r repositorySet) EventStream() common.EventStreamRepositories {
	return common.EventStreamRepositories{Owner: r.EventStreamOwner}
}

func (r repositorySet) Governance() common.GovernanceRepositories {
	return common.GovernanceRepositories{AutoGovernanceTraces: r.AutoGovernanceTraces}
}

func (r repositorySet) MCP() common.MCPRepositories {
	return common.MCPRepositories{Registry: r.MCPRegistry}
}

func (r repositorySet) Memory() common.MemoryRepositories {
	return common.MemoryRepositories{Owner: r.MemoryOwner}
}

func (r repositorySet) Orchestrator() repository.OrchestratorEvidenceOwnerRepositories {
	return r.OrchestratorOwner
}

func (r repositorySet) Result() common.ResultRepositories {
	return common.ResultRepositories{Owner: r.ResultOwner}
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
			MemoryOwner:           memoryOwner.OwnerRepositories(),
			AutoGovernanceTraces:  memory.NewAutoGovernanceTraceRepository(),
			Skills:                memory.NewSkillRepository(),
			MCPRegistry:           memory.NewMCPRegistryRepository(),
			ResultOwner:           resultOwner.OwnerRepositories(),
			EventStreamOwner:      eventStreamOwner.OwnerRepositories(),
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
			DecisionOwner:         decisionOwner.OwnerRepositories(),
			ApprovalOwner:         approvalOwner.OwnerRepositories(),
			OrchestratorOwner:     orchestratorOwner,
		}, nil
	case "file":
		dataRoot = workorchrepo.ResolveRepositoryDataRoot(backend, dataRoot)
		workspaceScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
		return repositorySet{
			Souls:                 fileRepo.NewSoulRepository(filepath.Join(dataRoot, "system", "souls")),
			WorkspaceTemplates:    fileRepo.NewWorkspaceTemplateRepository(filepath.Join(dataRoot, "system", "workspace_templates.json")),
			MemoryOwner:           workspaceScopedRepos.MemoryOwner,
			AutoGovernanceTraces:  fileRepo.NewAutoGovernanceTraceRepository(filepath.Join(dataRoot, "system", "auto_governance_trace.json")),
			Skills:                fileRepo.NewSkillRepository(filepath.Join(dataRoot, "system", "skill.json")),
			MCPRegistry:           fileRepo.NewMCPRegistryRepository(filepath.Join(dataRoot, "system", "mcp_registry.json")),
			ResultOwner:           workspaceScopedRepos.ResultOwner,
			EventStreamOwner:      workspaceScopedRepos.EventStreamOwner,
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
			DecisionOwner:         workspaceScopedRepos.DecisionOwner,
			ApprovalOwner:         workspaceScopedRepos.ApprovalOwner,
			OrchestratorOwner:     workspaceScopedRepos.OrchestratorEvidence,
		}, nil
	default:
		return repositorySet{}, fmt.Errorf("unsupported repository backend: %s", backend)
	}
}
