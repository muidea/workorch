package workorchruntime

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

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
	mu           sync.Mutex
	closing      bool
}

var _ common.Provider = (*RepositoryInitiator)(nil)

type repositorySet struct {
	Souls                 repository.SoulRepository
	WorkspaceOwner        repository.WorkspaceOwnerRepositories
	MemoryOwner           repository.MemoryOwnerRepositories
	AutoGovernanceTraces  repository.AutoGovernanceTraceRepository
	Skills                repository.SkillRepository
	MCPRegistry           repository.MCPRegistryRepository
	ResultOwner           repository.ResultOwnerRepositories
	EventStreamOwner      repository.EventStreamOwnerRepositories
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
	r.mu.Lock()
	defer r.mu.Unlock()
	// 已发布 provider 在消费者释放前不得被重复 Setup 撤销。
	if r.repositories != nil {
		return cd.WrapError(cd.Unexpected, fmt.Errorf("repository provider is already configured"), "repository setup rejected")
	}
	if r.closing {
		return cd.NewError(cd.ServiceUnavailable, "repository provider is shutting down")
	}
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

func (r *RepositoryInitiator) BeginShutdown(context.Context) {
	r.mu.Lock()
	r.closing = true
	r.mu.Unlock()
}

func (r *RepositoryInitiator) Quiesce(ctx context.Context) *cd.Error {
	if ctx == nil {
		return cd.NewError(cd.IllegalParam, "shutdown context is required")
	}
	if err := ctx.Err(); err != nil {
		return cd.WrapError(cd.Unexpected, err, "repository quiesce failed")
	}
	// 无自有任务；全体 owner 和共享队列屏障由进程协调者完成。
	return nil
}

func (r *RepositoryInitiator) Teardown(ctx context.Context) {
	if err := r.Quiesce(ctx); err != nil {
		panic(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.repositories = nil
	r.closing = false
}

func (r *RepositoryInitiator) RepositoryProvider() common.RepositoryProvider {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.repositories == nil {
		return nil
	}
	return r.repositories
}

func (r repositorySet) Workspace() common.WorkspaceRepositories {
	return common.WorkspaceRepositories{Owner: r.WorkspaceOwner}
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
		workspaceOwner := memory.NewWorkspaceOwnerRepositorySet()
		memoryOwner := memory.NewMemoryOwnerRepositorySet()
		orchestratorOwner := memory.NewOrchestratorOwnerRepositorySet()
		resultOwner := memory.NewResultOwnerRepositorySet()
		decisionOwner := memory.NewDecisionOwnerRepositorySet()
		approvalOwner := memory.NewApprovalOwnerRepositorySet()
		eventStreamOwner := memory.NewEventStreamOwnerRepositorySet()
		runOwner := memory.NewRunOwnerRepositorySet()
		return repositorySet{
			Souls:                 memory.NewSoulRepository(),
			WorkspaceOwner:        workspaceOwner.OwnerRepositories(),
			MemoryOwner:           memoryOwner.OwnerRepositories(),
			AutoGovernanceTraces:  memory.NewAutoGovernanceTraceRepository(),
			Skills:                memory.NewSkillRepository(),
			MCPRegistry:           memory.NewMCPRegistryRepository(),
			ResultOwner:           resultOwner.OwnerRepositories(),
			EventStreamOwner:      eventStreamOwner.OwnerRepositories(),
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
			WorkspaceOwner:        workspaceScopedRepos.WorkspaceOwner,
			MemoryOwner:           workspaceScopedRepos.MemoryOwner,
			AutoGovernanceTraces:  fileRepo.NewAutoGovernanceTraceRepository(filepath.Join(dataRoot, "system", "auto_governance_trace.json")),
			Skills:                fileRepo.NewSkillRepository(filepath.Join(dataRoot, "system", "skill.json")),
			MCPRegistry:           fileRepo.NewMCPRegistryRepository(filepath.Join(dataRoot, "system", "mcp_registry.json")),
			ResultOwner:           workspaceScopedRepos.ResultOwner,
			EventStreamOwner:      workspaceScopedRepos.EventStreamOwner,
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
