package testkit

import (
	"context"
	"path/filepath"
	"sort"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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
	MemoryDeletions       repository.MemoryWorkspaceDeletionRepository
	AutoGovernanceTraces  repository.AutoGovernanceTraceRepository
	Skills                repository.SkillRepository
	MCPRegistry           repository.MCPRegistryRepository
	MCPServers            repository.MCPServerRepository
	MCPCapabilities       repository.MCPCapabilityRepository
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
	RunOwner              repository.RunOwnerRepositories
	Runs                  repository.RunRepository
	Steps                 repository.StepRepository
	ToolCalls             repository.ToolCallRepository
	Patches               repository.PatchRepository
	ValidationResults     repository.ValidationResultRepository
	RuntimeCheckpoints    repository.RuntimeCheckpointRepository
	EvidenceRecords       repository.EvidenceRecordRepository
	CoverageWorkItems     repository.CoverageWorkItemRepository
	ArtifactContinuations repository.ArtifactContinuationRepository
	OrchestratorEvidence  repository.OrchestratorEvidenceOwnerRepositories
	Tickets               repository.DecisionTicketRepository
	Resolutions           repository.DecisionResolutionRepository
	DecisionDeletions     repository.DecisionWorkspaceDeletionRepository
	Approvals             repository.ApprovalRepository
	ApprovalDeletions     repository.ApprovalWorkspaceDeletionRepository
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
		workspaceRepository := memory.NewWorkspaceRepository()
		registry := memory.NewMCPRegistryRepository()
		memoryOwner := memory.NewMemoryOwnerRepositorySet()
		resultOwner := memory.NewResultOwnerRepositorySet()
		decisionOwner := memory.NewDecisionOwnerRepositorySet()
		approvalOwner := memory.NewApprovalOwnerRepositorySet()
		eventStreamOwner := memory.NewEventStreamOwnerRepositorySet()
		orchestratorOwner := memory.NewOrchestratorOwnerRepositorySet()
		runOwner := memory.NewRunOwnerRepositorySet()
		return RepositoryExports{
			Souls:                 memory.NewSoulRepository(),
			WorkspaceTemplates:    memory.NewWorkspaceTemplateRepository(),
			MemoryItems:           memoryOwner.Items,
			MemoryRecallTraces:    memoryOwner.RecallTraces,
			MemoryDeletions:       memoryOwner.WorkspaceDeletions,
			AutoGovernanceTraces:  memory.NewAutoGovernanceTraceRepository(),
			Skills:                memory.NewSkillRepository(),
			MCPRegistry:           registry,
			MCPServers:            aggregateServerRepository{registry: registry},
			MCPCapabilities:       aggregateCapabilityRepository{registry: registry},
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
			RunOwner:              runOwner,
			Runs:                  runOwner.Runs(),
			Steps:                 runOwner.Steps(),
			ToolCalls:             orchestratorOwner.ToolCalls(),
			Patches:               orchestratorOwner.Patches(),
			ValidationResults:     orchestratorOwner.ValidationResults(),
			RuntimeCheckpoints:    runOwner.Checkpoints(),
			EvidenceRecords:       orchestratorOwner.EvidenceRecords(),
			CoverageWorkItems:     orchestratorOwner.CoverageWorkItems(),
			ArtifactContinuations: orchestratorOwner.ArtifactContinuations(),
			OrchestratorEvidence:  orchestratorOwner,
			Tickets:               decisionOwner.Tickets,
			Resolutions:           decisionOwner.Resolutions,
			DecisionDeletions:     decisionOwner.WorkspaceDeletions,
			Approvals:             approvalOwner.Approvals,
			ApprovalDeletions:     approvalOwner.WorkspaceDeletions,
		}
	case "file":
		dataRoot = workorchrepo.ResolveRepositoryDataRoot(backend, dataRoot)
		workspaceScopedRepos := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot)
		registry := fileRepo.NewMCPRegistryRepository(filepath.Join(dataRoot, "system", "mcp_registry.json"))
		return RepositoryExports{
			Souls:                 fileRepo.NewSoulRepository(filepath.Join(dataRoot, "system", "souls")),
			WorkspaceTemplates:    fileRepo.NewWorkspaceTemplateRepository(filepath.Join(dataRoot, "system", "workspace_templates.json")),
			MemoryItems:           workspaceScopedRepos.MemoryItems,
			MemoryRecallTraces:    workspaceScopedRepos.MemoryRecallTraces,
			MemoryDeletions:       workspaceScopedRepos.MemoryDeletions,
			AutoGovernanceTraces:  fileRepo.NewAutoGovernanceTraceRepository(filepath.Join(dataRoot, "system", "auto_governance_trace.json")),
			Skills:                fileRepo.NewSkillRepository(filepath.Join(dataRoot, "system", "skill.json")),
			MCPRegistry:           registry,
			MCPServers:            aggregateServerRepository{registry: registry},
			MCPCapabilities:       aggregateCapabilityRepository{registry: registry},
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
			RunOwner:              workspaceScopedRepos.RunOwner,
			Runs:                  workspaceScopedRepos.RunOwner.Runs(),
			Steps:                 workspaceScopedRepos.RunOwner.Steps(),
			ToolCalls:             workspaceScopedRepos.ToolCalls,
			Patches:               workspaceScopedRepos.Patches,
			ValidationResults:     workspaceScopedRepos.ValidationResults,
			RuntimeCheckpoints:    workspaceScopedRepos.RunOwner.Checkpoints(),
			EvidenceRecords:       workspaceScopedRepos.EvidenceRecords,
			CoverageWorkItems:     workspaceScopedRepos.CoverageWorkItems,
			ArtifactContinuations: workspaceScopedRepos.ArtifactContinuations,
			OrchestratorEvidence:  workspaceScopedRepos.OrchestratorEvidence,
			Tickets:               workspaceScopedRepos.Tickets,
			Resolutions:           workspaceScopedRepos.Resolutions,
			DecisionDeletions:     workspaceScopedRepos.DecisionDeletions,
			Approvals:             workspaceScopedRepos.Approvals,
			ApprovalDeletions:     workspaceScopedRepos.ApprovalDeletions,
		}
	default:
		panic("unsupported repository backend: " + backend)
	}
}

type aggregateServerRepository struct {
	registry repository.MCPRegistryRepository
}

func (r aggregateServerRepository) Create(ctx context.Context, item *mcpmodel.Server) error {
	return mutateTestRegistry(ctx, r.registry, func(snapshot *mcpmodel.RegistrySnapshot) error {
		for _, existing := range snapshot.Servers {
			if existing != nil && item != nil && existing.ID == item.ID {
				return common.ErrConflict
			}
		}
		if item == nil {
			return common.ErrInvalidArgument
		}
		snapshot.Servers = append(snapshot.Servers, mcpmodel.CloneServer(item))
		return nil
	})
}

func (r aggregateServerRepository) Get(ctx context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	snapshot, err := r.registry.Load(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range snapshot.Servers {
		if item != nil && item.ID == id {
			return mcpmodel.CloneServer(item), nil
		}
	}
	return nil, common.ErrNotFound
}

func (r aggregateServerRepository) List(ctx context.Context) ([]*mcpmodel.Server, error) {
	snapshot, err := r.registry.Load(ctx)
	if err != nil {
		return nil, err
	}
	return mcpmodel.CloneServers(snapshot.Servers), nil
}

func (r aggregateServerRepository) Update(ctx context.Context, item *mcpmodel.Server) error {
	return mutateTestRegistry(ctx, r.registry, func(snapshot *mcpmodel.RegistrySnapshot) error {
		for idx, existing := range snapshot.Servers {
			if existing != nil && item != nil && existing.ID == item.ID {
				snapshot.Servers[idx] = mcpmodel.CloneServer(item)
				return nil
			}
		}
		if item == nil {
			return common.ErrInvalidArgument
		}
		return common.ErrNotFound
	})
}

type aggregateCapabilityRepository struct {
	registry repository.MCPRegistryRepository
}

func (r aggregateCapabilityRepository) Create(ctx context.Context, item *mcpmodel.Capability) error {
	return mutateTestRegistry(ctx, r.registry, func(snapshot *mcpmodel.RegistrySnapshot) error {
		for _, existing := range snapshot.Capabilities {
			if existing != nil && item != nil && existing.ID == item.ID {
				return common.ErrConflict
			}
		}
		if item == nil {
			return common.ErrInvalidArgument
		}
		snapshot.Capabilities = append(snapshot.Capabilities, mcpmodel.CloneCapability(item))
		return nil
	})
}

func (r aggregateCapabilityRepository) Get(ctx context.Context, id common.MCPCapabilityID) (*mcpmodel.Capability, error) {
	snapshot, err := r.registry.Load(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range snapshot.Capabilities {
		if item != nil && item.ID == id {
			return mcpmodel.CloneCapability(item), nil
		}
	}
	return nil, common.ErrNotFound
}

func (r aggregateCapabilityRepository) List(ctx context.Context) ([]*mcpmodel.Capability, error) {
	snapshot, err := r.registry.Load(ctx)
	if err != nil {
		return nil, err
	}
	return mcpmodel.CloneCapabilities(snapshot.Capabilities), nil
}

func (r aggregateCapabilityRepository) Update(ctx context.Context, item *mcpmodel.Capability) error {
	return mutateTestRegistry(ctx, r.registry, func(snapshot *mcpmodel.RegistrySnapshot) error {
		for idx, existing := range snapshot.Capabilities {
			if existing != nil && item != nil && existing.ID == item.ID {
				snapshot.Capabilities[idx] = mcpmodel.CloneCapability(item)
				return nil
			}
		}
		if item == nil {
			return common.ErrInvalidArgument
		}
		return common.ErrNotFound
	})
}

func mutateTestRegistry(ctx context.Context, registry repository.MCPRegistryRepository, apply func(*mcpmodel.RegistrySnapshot) error) error {
	if registry == nil || apply == nil {
		return common.ErrInvalidArgument
	}
	for attempt := 0; attempt < 8; attempt++ {
		current, err := registry.Load(ctx)
		if err != nil {
			return err
		}
		next := mcpmodel.CloneRegistrySnapshot(current)
		if err := apply(next); err != nil {
			return err
		}
		sort.Slice(next.Servers, func(i, j int) bool { return next.Servers[i].ID < next.Servers[j].ID })
		sort.Slice(next.Capabilities, func(i, j int) bool { return next.Capabilities[i].ID < next.Capabilities[j].ID })
		next.Revision = current.Revision + 1
		if err := registry.Commit(ctx, current.Revision, next); err == nil {
			return nil
		} else if err != common.ErrConflict {
			return err
		}
	}
	return common.ErrConflict
}
