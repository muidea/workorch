package bootstrap

import (
	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

func NewFromEnv() *Runtime {
	cfg, err := workorchconfig.Current()
	if err != nil {
		panic("load workorch config: " + err.Error())
	}
	return newWithBackendRuntimePolicyAndEventHub(
		cfg.Repository.Backend,
		cfg.Repository.DataDir,
		workorchruntimepolicy.FromConfig(cfg),
		nil,
	)
}

func NewWithBackend(backend string, dataRoot string) *Runtime {
	return NewWithBackendAndEventHub(backend, dataRoot, nil)
}

func NewWithBackendAndRuntimePolicy(backend string, dataRoot string, runtimePolicy workorchruntimepolicy.RuntimePolicy) *Runtime {
	return newWithBackendRuntimePolicyAndEventHub(backend, dataRoot, runtimePolicy, nil)
}

func NewWithBackendAndEventHub(backend string, dataRoot string, eventHub events.Hub) *Runtime {
	return newWithBackendRuntimePolicyAndEventHub(backend, dataRoot, workorchruntimepolicy.RuntimePolicy{}, eventHub)
}

func NewWithBackendRuntimePolicyAndEventHub(backend string, dataRoot string, runtimePolicy workorchruntimepolicy.RuntimePolicy, eventHub events.Hub) *Runtime {
	return newWithBackendRuntimePolicyAndEventHub(backend, dataRoot, runtimePolicy, eventHub)
}

// newWithBackendRuntimePolicyAndEventHub builds repository-only exports.
// Module, block, and initiator lifecycles are owned exclusively by the
// magicCommon framework plugin runtime.
func newWithBackendRuntimePolicyAndEventHub(backend string, dataRoot string, runtimePolicy workorchruntimepolicy.RuntimePolicy, eventHub events.Hub) *Runtime {
	_ = runtimePolicy
	repos := buildRepositories(backend, dataRoot)

	if eventHub == nil {
		eventHub = events.NewMagicHubWithOwner("workorch/bootstrap")
	}
	backgroundRoutine := magictask.NewBackgroundRoutine(1024)

	return &Runtime{
		exports: Exports{
			Repositories: RepositoryExports{
				Souls:                 repos.soul,
				WorkspaceTemplates:    repos.workspaceTemplate,
				MemoryItems:           repos.memoryItem,
				MemoryRecallTraces:    repos.memoryRecallTrace,
				AutoGovernanceTraces:  repos.autoGovernanceTrace,
				Skills:                repos.skill,
				MCPServers:            repos.mcpServer,
				MCPCapabilities:       repos.mcpCapability,
				Results:               repos.result,
				Artifacts:             repos.artifact,
				EventRecords:          repos.eventRecord,
				Workspaces:            repos.workspace,
				Goals:                 repos.goal,
				DoDs:                  repos.dod,
				Runs:                  repos.run,
				Steps:                 repos.step,
				ToolCalls:             repos.toolCall,
				Patches:               repos.patch,
				ValidationResults:     repos.validationResult,
				RuntimeCheckpoints:    repos.runtimeCheckpoint,
				EvidenceRecords:       repos.evidenceRecord,
				CoverageWorkItems:     repos.coverageWorkItem,
				ArtifactContinuations: repos.artifactContinuation,
				Tickets:               repos.ticket,
				Resolutions:           repos.resolution,
				Approvals:             repos.approval,
			},
			Infrastructure: InfrastructureExports{
				EventHub:          eventHub,
				BackgroundRoutine: backgroundRoutine,
				LoadConfig:        workorchconfig.Current,
			},
		},
		background: backgroundRoutine,
	}
}
