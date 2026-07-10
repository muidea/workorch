package bootstrap

import (
	magicevent "github.com/muidea/magicCommon/event"
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
	return newWithBackendRuntimePolicyAndEventHubs(
		cfg.Repository.Backend,
		cfg.Repository.DataDir,
		workorchruntimepolicy.FromConfig(cfg),
		nil,
		nil,
	)
}

func NewWithBackend(backend string, dataRoot string) *Runtime {
	return newWithBackendRuntimePolicyAndEventHubs(backend, dataRoot, workorchruntimepolicy.RuntimePolicy{}, nil, nil)
}

func NewWithBackendAndRuntimePolicy(backend string, dataRoot string, runtimePolicy workorchruntimepolicy.RuntimePolicy) *Runtime {
	return newWithBackendRuntimePolicyAndEventHubs(backend, dataRoot, runtimePolicy, nil, nil)
}

func NewWithBackendAndEventHubs(backend string, dataRoot string, eventHub events.Hub, commandHub magicevent.Hub) *Runtime {
	return newWithBackendRuntimePolicyAndEventHubs(backend, dataRoot, workorchruntimepolicy.RuntimePolicy{}, eventHub, commandHub)
}

func NewWithBackendRuntimePolicyAndEventHubs(backend string, dataRoot string, runtimePolicy workorchruntimepolicy.RuntimePolicy, eventHub events.Hub, commandHub magicevent.Hub) *Runtime {
	return newWithBackendRuntimePolicyAndEventHubs(backend, dataRoot, runtimePolicy, eventHub, commandHub)
}

// newWithBackendRuntimePolicyAndEventHubs builds repository-only exports.
// Module, block, and initiator lifecycles are owned exclusively by the
// magicCommon framework plugin runtime.
func newWithBackendRuntimePolicyAndEventHubs(backend string, dataRoot string, runtimePolicy workorchruntimepolicy.RuntimePolicy, eventHub events.Hub, commandHub magicevent.Hub) *Runtime {
	_ = runtimePolicy
	repos := buildRepositories(backend, dataRoot)

	if commandHub == nil {
		commandHub = magicevent.NewHub(1024)
	}
	if eventHub == nil {
		eventHub = events.NewMagicHubWithHubOwner(commandHub, "workorch/bootstrap")
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
				CommandHub:        commandHub,
				BackgroundRoutine: backgroundRoutine,
				LoadConfig:        workorchconfig.Current,
			},
		},
		background: backgroundRoutine,
	}
}
