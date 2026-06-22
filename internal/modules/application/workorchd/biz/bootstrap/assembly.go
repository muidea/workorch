package bootstrap

import (
	"context"

	magictask "github.com/muidea/magicCommon/task"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	gitmodule "github.com/muidea/workorch/internal/modules/blocks/git"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpblockmodule "github.com/muidea/workorch/internal/modules/blocks/mcp"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	sshmodule "github.com/muidea/workorch/internal/modules/blocks/ssh"
	webmodule "github.com/muidea/workorch/internal/modules/blocks/web"
	approvalmodule "github.com/muidea/workorch/internal/modules/kernel/approval"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	eventstreammodule "github.com/muidea/workorch/internal/modules/kernel/eventstream"
	governancemodule "github.com/muidea/workorch/internal/modules/kernel/governance"
	mcpmodule "github.com/muidea/workorch/internal/modules/kernel/mcp"
	memorymodule "github.com/muidea/workorch/internal/modules/kernel/memory"
	orchestratormodule "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	resultmodule "github.com/muidea/workorch/internal/modules/kernel/result"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	skillmodule "github.com/muidea/workorch/internal/modules/kernel/skill"
	soulmodule "github.com/muidea/workorch/internal/modules/kernel/soul"
	systemmodule "github.com/muidea/workorch/internal/modules/kernel/system"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
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

func newWithBackendRuntimePolicyAndEventHub(backend string, dataRoot string, runtimePolicy workorchruntimepolicy.RuntimePolicy, eventHub events.Hub) *Runtime {
	repos := buildRepositories(backend, dataRoot)

	if eventHub == nil {
		eventHub = events.NewMagicHubWithOwner("workorch/bootstrap")
	}
	backgroundRoutine := magictask.NewBackgroundRoutine(1024)
	fileModule := filemodule.New()
	mustSetup("file", fileModule.Configure(filemodule.Dependencies{EventHub: eventHub, Background: backgroundRoutine}))
	gitModule := gitmodule.New()
	mustSetup("git", gitModule.Configure(gitmodule.Dependencies{EventHub: eventHub, Background: backgroundRoutine}))
	shellModule := shellmodule.New()
	mustSetup("shell", shellModule.Configure(shellmodule.Dependencies{EventHub: eventHub, Background: backgroundRoutine}))
	sshModule := sshmodule.New()
	mustSetup("ssh", sshModule.Configure(sshmodule.Dependencies{EventHub: eventHub, Background: backgroundRoutine}))
	webModule := webmodule.New()
	mustSetup("web", webModule.Configure(webmodule.Dependencies{
		EventHub:   eventHub,
		Background: backgroundRoutine,
	}))
	llmModule := llmmodule.New()
	mustSetup("llm", llmModule.Configure(llmmodule.Dependencies{EventHub: eventHub, Background: backgroundRoutine}))

	soulModule := soulmodule.New()
	mustSetup("soul", soulModule.Configure(soulmodule.Dependencies{
		Souls:    repos.soul,
		EventHub: eventHub,
	}))

	memoryModule := memorymodule.New()
	mustSetup("memory", memoryModule.Configure(memorymodule.Dependencies{
		MemoryItems:        repos.memoryItem,
		MemoryRecallTraces: repos.memoryRecallTrace,
		EventHub:           eventHub,
	}))

	skillModule := skillmodule.New()
	mustSetup("skill", skillModule.Configure(skillmodule.Dependencies{
		Skills:   repos.skill,
		EventHub: eventHub,
	}))

	mcpModule := mcpmodule.New()
	mustSetup("mcp", mcpModule.Configure(mcpmodule.Dependencies{
		Servers:      repos.mcpServer,
		Capabilities: repos.mcpCapability,
		EventHub:     eventHub,
	}))
	systemModule := systemmodule.New()
	mustSetup("system", systemModule.Configure(systemmodule.Dependencies{
		Souls:           repos.soul,
		Templates:       repos.workspaceTemplate,
		MCPServers:      repos.mcpServer,
		MCPCapabilities: repos.mcpCapability,
	}))
	mcpBlockModule := mcpblockmodule.New()
	mustSetup("mcp-block", mcpBlockModule.Configure(mcpblockmodule.Dependencies{
		EventHub:   eventHub,
		Background: backgroundRoutine,
	}))

	eventStreamModule := eventstreammodule.New()
	mustSetup("eventstream", eventStreamModule.Configure(eventstreammodule.Dependencies{
		EventRecords: repos.eventRecord,
		EventHub:     eventHub,
	}))

	workspaceModule := workspacemodule.New()
	mustSetup("workspace", workspaceModule.Configure(workspacemodule.Dependencies{
		Workspaces: repos.workspace,
		Goals:      repos.goal,
		DoDs:       repos.dod,
		Templates:  repos.workspaceTemplate,
		EventHub:   eventHub,
	}))

	decisionModule := decisionmodule.New()
	mustSetup("decision", decisionModule.Configure(decisionmodule.Dependencies{
		Tickets:     repos.ticket,
		Resolutions: repos.resolution,
		EventHub:    eventHub,
	}))

	approvalModule := approvalmodule.New()
	mustSetup("approval", approvalModule.Configure(approvalmodule.Dependencies{
		Approvals: repos.approval,
		EventHub:  eventHub,
	}))

	resultModule := resultmodule.New()
	mustSetup("result", resultModule.Configure(resultmodule.Dependencies{
		Results:   repos.result,
		Artifacts: repos.artifact,
		EventHub:  eventHub,
	}))

	orchestratorModule := orchestratormodule.New()
	runtimeRegistry, err := buildRuntimeAdapterRegistry(
		eventHub,
	)
	mustSetup("runtime-registry", err)
	mustSetup("orchestrator", orchestratorModule.Configure(orchestratormodule.Dependencies{
		Runtime:               runtimeRegistry,
		Steps:                 repos.step,
		ToolCalls:             repos.toolCall,
		Patches:               repos.patch,
		ValidationResults:     repos.validationResult,
		RuntimeCheckpoints:    repos.runtimeCheckpoint,
		CoverageWorkItems:     repos.coverageWorkItem,
		ArtifactContinuations: repos.artifactContinuation,
		RuntimePolicy:         runtimePolicy,
		EventHub:              eventHub,
		BackgroundRoutine:     backgroundRoutine,
	}))

	runModule := runmodule.New()
	mustSetup("run", runModule.Configure(runmodule.Dependencies{
		Runs:        repos.run,
		Steps:       repos.step,
		Checkpoints: repos.runtimeCheckpoint,
		EventHub:    eventHub,
	}))
	governanceModule := governancemodule.New()
	mustSetup("governance", governanceModule.Configure(governancemodule.Dependencies{
		AutoGovernanceTraces: repos.autoGovernanceTrace,
		EventHub:             eventHub,
		LoadConfig:           workorchconfig.Current,
	}))

	modules := []lifecycle{
		fileModule,
		gitModule,
		shellModule,
		sshModule,
		webModule,
		llmModule,
		soulModule,
		memoryModule,
		skillModule,
		mcpModule,
		contextStartLifecycle{
			id:    systemModule.ID(),
			start: systemModule.Start,
			stop:  systemModule.Stop,
		},
		mcpBlockModule,
		eventStreamModule,
		workspaceModule,
		decisionModule,
		approvalModule,
		resultModule,
		orchestratorModule,
		runModule,
		governanceModule,
	}
	moduleLifecycle, err := newRuntimeModuleLifecycle(modules)
	mustSetup("module-lifecycle", err)
	mustSetup("module-lifecycle", moduleLifecycle.Start())

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
		modules:         modules,
		moduleLifecycle: moduleLifecycle,
		background:      backgroundRoutine,
	}
}

func mustSetup(name string, err error) {
	if err != nil {
		panic("setup module " + name + ": " + err.Error())
	}
}

type contextStartLifecycle struct {
	id    string
	start func(context.Context) error
	stop  func(context.Context) error
}

func (l contextStartLifecycle) ID() string { return l.id }

func (l contextStartLifecycle) Start() error {
	if l.start == nil {
		return nil
	}
	return l.start(context.Background())
}

func (l contextStartLifecycle) Stop(ctx context.Context) error {
	if l.stop == nil {
		return nil
	}
	return l.stop(ctx)
}

func buildRuntimeAdapterRegistry(eventHub events.Hub) (*orchestratormodule.RuntimeAdapterRegistry, error) {
	runtimeRegistry := orchestratormodule.NewRuntimeAdapterRegistry()
	eventAdapter := orchestratormodule.NewEventRuntimeAdapter(eventHub)
	for _, stepKind := range []string{
		filemodule.StepKindList,
		filemodule.StepKindRead,
		filemodule.StepKindEdit,
	} {
		if err := runtimeRegistry.Register(stepKind, eventAdapter); err != nil {
			return nil, err
		}
	}
	for _, stepKind := range []string{
		gitmodule.StepKindRead,
		gitmodule.StepKindWrite,
	} {
		if err := runtimeRegistry.Register(stepKind, eventAdapter); err != nil {
			return nil, err
		}
	}
	if err := runtimeRegistry.Register(shellmodule.StepKindCommand, eventAdapter); err != nil {
		return nil, err
	}
	if err := runtimeRegistry.Register(sshmodule.StepKindCommand, eventAdapter); err != nil {
		return nil, err
	}
	for _, stepKind := range []string{
		webmodule.StepKindSearch,
		webmodule.StepKindFetch,
	} {
		if err := runtimeRegistry.Register(stepKind, eventAdapter); err != nil {
			return nil, err
		}
	}
	for _, stepKind := range []string{
		mcpblockmodule.StepKindToolCall,
		mcpblockmodule.StepKindResourceRead,
		mcpblockmodule.StepKindPromptGet,
	} {
		if err := runtimeRegistry.Register(stepKind, eventAdapter); err != nil {
			return nil, err
		}
	}
	if err := runtimeRegistry.Register(llmcommon.RuntimeBlockKindPatchApply, eventAdapter); err != nil {
		return nil, err
	}
	if err := runtimeRegistry.Register(llmcommon.RuntimeBlockKindPatchPreview, eventAdapter); err != nil {
		return nil, err
	}
	if err := runtimeRegistry.Register(llmcommon.RuntimeBlockKindPatchRevert, eventAdapter); err != nil {
		return nil, err
	}
	if err := runtimeRegistry.Register(llmmodule.StepKindWorkspaceExecution, eventAdapter); err != nil {
		return nil, err
	}
	return runtimeRegistry, nil
}
