package bootstrap

import (
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	legacymodule "github.com/muidea/workorch/internal/modules/blocks/legacy"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	mcpblockmodule "github.com/muidea/workorch/internal/modules/blocks/mcp"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	sshmodule "github.com/muidea/workorch/internal/modules/blocks/ssh"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	mcpmodule "github.com/muidea/workorch/internal/modules/kernel/mcp"
	memorymodule "github.com/muidea/workorch/internal/modules/kernel/memory"
	orchestratormodule "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	skillmodule "github.com/muidea/workorch/internal/modules/kernel/skill"
	soulmodule "github.com/muidea/workorch/internal/modules/kernel/soul"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func NewFromEnv() *Runtime {
	cfg, err := workorchconfig.Current()
	if err != nil {
		panic("load workorch config: " + err.Error())
	}
	return NewWithBackend(cfg.Repository.Backend, cfg.Repository.DataDir)
}

func NewWithBackend(backend string, dataRoot string) *Runtime {
	return NewWithBackendAndEventHub(backend, dataRoot, nil)
}

func NewWithBackendAndEventHub(backend string, dataRoot string, eventHub events.Hub) *Runtime {
	repos := buildRepositories(backend, dataRoot)

	if eventHub == nil {
		eventHub = events.NewMagicHub()
	}
	legacyModule := legacymodule.New()
	mustSetup("legacy", legacyModule.Setup())
	shellModule := shellmodule.New()
	mustSetup("shell", shellModule.Setup())
	sshModule := sshmodule.New()
	mustSetup("ssh", sshModule.Setup())
	llmModule := llmmodule.New()
	mustSetup("llm", llmModule.Setup())

	soulModule := soulmodule.New()
	mustSetup("soul", soulModule.Setup(soulmodule.Dependencies{
		Souls: repos.soul,
	}))

	memoryModule := memorymodule.New()
	mustSetup("memory", memoryModule.Setup(memorymodule.Dependencies{
		MemoryItems: repos.memoryItem,
	}))

	skillModule := skillmodule.New()
	mustSetup("skill", skillModule.Setup(skillmodule.Dependencies{
		Skills: repos.skill,
	}))

	mcpModule := mcpmodule.New()
	mustSetup("mcp", mcpModule.Setup(mcpmodule.Dependencies{
		Servers: repos.mcpServer,
	}))
	mcpBlockModule := mcpblockmodule.New()
	mustSetup("mcp-block", mcpBlockModule.Setup(mcpblockmodule.Dependencies{
		MCPService: mcpModule.Service(),
	}))

	workspaceModule := workspacemodule.New()
	mustSetup("workspace", workspaceModule.Setup(workspacemodule.Dependencies{
		Workspaces: repos.workspace,
		Goals:      repos.goal,
		DoDs:       repos.dod,
		EventHub:   eventHub,
	}))

	decisionModule := decisionmodule.New()
	mustSetup("decision", decisionModule.Setup(decisionmodule.Dependencies{
		Tickets:     repos.ticket,
		Resolutions: repos.resolution,
		EventHub:    eventHub,
	}))

	approvalModule := approvalmodule.New()
	mustSetup("approval", approvalModule.Setup(approvalmodule.Dependencies{
		Approvals: repos.approval,
		EventHub:  eventHub,
	}))

	orchestratorModule := orchestratormodule.New()
	runtimeRegistry := orchestratorbiz.NewRuntimeAdapterRegistry(legacyModule.RuntimeAdapter())
	mustSetup("runtime-registry", runtimeRegistry.Register(shellmodule.StepKindCommand, shellModule.RuntimeAdapter()))
	mustSetup("runtime-registry", runtimeRegistry.Register(sshmodule.StepKindCommand, sshModule.RuntimeAdapter()))
	mustSetup("runtime-registry", runtimeRegistry.Register(mcpblockmodule.StepKindToolCall, mcpBlockModule.RuntimeAdapter()))
	mustSetup("runtime-registry", runtimeRegistry.Register(llmmodule.StepKindWorkspaceExecution, llmModule.RuntimeAdapter()))
	mustSetup("orchestrator", orchestratorModule.Setup(orchestratormodule.Dependencies{
		Runtime:            runtimeRegistry,
		Runs:               repos.run,
		Steps:              repos.step,
		Workspaces:         repos.workspace,
		Tickets:            repos.ticket,
		Resolutions:        repos.resolution,
		Approvals:          repos.approval,
		ToolCalls:          repos.toolCall,
		Patches:            repos.patch,
		ValidationResults:  repos.validationResult,
		RuntimeCheckpoints: repos.runtimeCheckpoint,
		MemoryItems:        repos.memoryItem,
		EventHub:           eventHub,
	}))

	runModule := runmodule.New()
	mustSetup("run", runModule.Setup(runmodule.Dependencies{
		Runs:         repos.run,
		Workspaces:   repos.workspace,
		Orchestrator: orchestratorModule.Service(),
		EventHub:     eventHub,
	}))

	modules := []lifecycle{
		legacyModule,
		shellModule,
		sshModule,
		llmModule,
		soulModule,
		memoryModule,
		skillModule,
		mcpModule,
		mcpBlockModule,
		workspaceModule,
		decisionModule,
		approvalModule,
		orchestratorModule,
		runModule,
	}
	moduleLifecycle, err := newModulePluginLifecycle(modules)
	mustSetup("module-lifecycle", err)
	mustSetup("module-lifecycle", moduleLifecycle.Run())

	return &Runtime{
		exports: Exports{
			Repositories: RepositoryExports{
				Souls:              repos.soul,
				MemoryItems:        repos.memoryItem,
				Skills:             repos.skill,
				MCPServers:         repos.mcpServer,
				Workspaces:         repos.workspace,
				Goals:              repos.goal,
				DoDs:               repos.dod,
				Runs:               repos.run,
				Steps:              repos.step,
				ToolCalls:          repos.toolCall,
				Patches:            repos.patch,
				ValidationResults:  repos.validationResult,
				RuntimeCheckpoints: repos.runtimeCheckpoint,
				Tickets:            repos.ticket,
				Resolutions:        repos.resolution,
				Approvals:          repos.approval,
			},
			Services: ServiceExports{
				WorkspaceService: workspaceModule.Service(),
				SoulService:      soulModule.Service(),
				MemoryService:    memoryModule.Service(),
				SkillService:     skillModule.Service(),
				MCPService:       mcpModule.Service(),
				DecisionService:  decisionModule.Service(),
				ApprovalService:  approvalModule.Service(),
				RunService:       runModule.Service(),
			},
			Infrastructure: InfrastructureExports{
				EventHub: eventHub,
			},
		},
		modules:         modules,
		moduleLifecycle: moduleLifecycle,
	}
}

func mustSetup(name string, err error) {
	if err != nil {
		panic("setup module " + name + ": " + err.Error())
	}
}
