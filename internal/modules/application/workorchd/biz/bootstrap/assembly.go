package bootstrap

import (
	"context"

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
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	eventstreammodule "github.com/muidea/workorch/internal/modules/kernel/eventstream"
	governancemodule "github.com/muidea/workorch/internal/modules/kernel/governance"
	mcpmodule "github.com/muidea/workorch/internal/modules/kernel/mcp"
	memorymodule "github.com/muidea/workorch/internal/modules/kernel/memory"
	orchestratormodule "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	skillmodule "github.com/muidea/workorch/internal/modules/kernel/skill"
	soulmodule "github.com/muidea/workorch/internal/modules/kernel/soul"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func NewFromEnv() *Runtime {
	cfg, err := workorchconfig.Current()
	if err != nil {
		panic("load workorch config: " + err.Error())
	}
	return newWithBackendRuntimePolicyAndEventHub(
		cfg.Repository.Backend,
		cfg.Repository.DataDir,
		RuntimePolicyFromConfig(cfg),
		nil,
	)
}

func RuntimePolicyFromConfig(cfg workorchconfig.Config) orchestratorbiz.RuntimePolicy {
	return orchestratorbiz.RuntimePolicy{
		MaxRuntimeRepairAttempts: cfg.Runtime.MaxRuntimeRepairAttempts,
		MaxProviderRetryAttempts: cfg.Runtime.MaxProviderRetryAttempts,
		LLMOutput: orchestratorbiz.LLMOutputRuntimePolicy{
			MaxPatchContentBytes:               cfg.Runtime.LLMOutput.MaxPatchContentBytes,
			MaxGraphBytes:                      cfg.Runtime.LLMOutput.MaxGraphBytes,
			MaxGraphFileWrites:                 cfg.Runtime.LLMOutput.MaxGraphFileWrites,
			SchemaInvalidLargePreviewThreshold: cfg.Runtime.LLMOutput.SchemaInvalidLargePreviewThreshold,
			MaxArtifactChunkBytes:              cfg.Runtime.LLMOutput.MaxArtifactChunkBytes,
			PreviousChunkTailBytes:             cfg.Runtime.LLMOutput.PreviousChunkTailBytes,
			MaxChunkRetryAttempts:              cfg.Runtime.LLMOutput.MaxChunkRetryAttempts,
			MaxArtifactPlanAttempts:            cfg.Runtime.LLMOutput.MaxArtifactPlanAttempts,
			MaxArtifactReplanAttempts:          cfg.Runtime.LLMOutput.MaxArtifactReplanAttempts,
		},
	}
}

func NewWithBackend(backend string, dataRoot string) *Runtime {
	return NewWithBackendAndEventHub(backend, dataRoot, nil)
}

func NewWithBackendAndRuntimePolicy(backend string, dataRoot string, runtimePolicy orchestratorbiz.RuntimePolicy) *Runtime {
	return newWithBackendRuntimePolicyAndEventHub(backend, dataRoot, runtimePolicy, nil)
}

func NewWithBackendAndEventHub(backend string, dataRoot string, eventHub events.Hub) *Runtime {
	return newWithBackendRuntimePolicyAndEventHub(backend, dataRoot, orchestratorbiz.RuntimePolicy{}, eventHub)
}

func NewWithBackendRuntimePolicyAndEventHub(backend string, dataRoot string, runtimePolicy orchestratorbiz.RuntimePolicy, eventHub events.Hub) *Runtime {
	return newWithBackendRuntimePolicyAndEventHub(backend, dataRoot, runtimePolicy, eventHub)
}

func newWithBackendRuntimePolicyAndEventHub(backend string, dataRoot string, runtimePolicy orchestratorbiz.RuntimePolicy, eventHub events.Hub) *Runtime {
	repos := buildRepositories(backend, dataRoot)

	if eventHub == nil {
		eventHub = events.NewMagicHubWithOwner("workorch/bootstrap")
	}
	fileModule := filemodule.New()
	mustSetup("file", fileModule.Setup())
	gitModule := gitmodule.New()
	mustSetup("git", gitModule.Setup())
	shellModule := shellmodule.New()
	mustSetup("shell", shellModule.Setup())
	sshModule := sshmodule.New()
	mustSetup("ssh", sshModule.Setup())
	webModule := webmodule.New()
	mustSetup("web", webModule.Setup(webmodule.Dependencies{
		Approvals: repos.approval,
	}))
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
		Servers:      repos.mcpServer,
		Capabilities: repos.mcpCapability,
	}))
	mcpBlockModule := mcpblockmodule.New()
	mustSetup("mcp-block", mcpBlockModule.Setup(mcpblockmodule.Dependencies{
		MCPService: mcpModule.Service(),
	}))

	eventStreamModule := eventstreammodule.New()
	mustSetup("eventstream", eventStreamModule.Setup(eventstreammodule.Dependencies{
		EventRecords: repos.eventRecord,
		EventHub:     eventHub,
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
	runtimeRegistry, err := buildRuntimeAdapterRegistry(
		fileModule.RuntimeAdapter(),
		gitModule.RuntimeAdapter(),
		shellModule.RuntimeAdapter(),
		sshModule.RuntimeAdapter(),
		webModule.RuntimeAdapter(),
		mcpBlockModule.RuntimeAdapter(),
		llmModule.RuntimeAdapter(),
	)
	mustSetup("runtime-registry", err)
	mustSetup("orchestrator", orchestratorModule.Setup(orchestratormodule.Dependencies{
		Runtime:               runtimeRegistry,
		Runs:                  repos.run,
		Steps:                 repos.step,
		Workspaces:            repos.workspace,
		Tickets:               repos.ticket,
		Resolutions:           repos.resolution,
		Approvals:             repos.approval,
		Results:               repos.result,
		Artifacts:             repos.artifact,
		ToolCalls:             repos.toolCall,
		Patches:               repos.patch,
		ValidationResults:     repos.validationResult,
		RuntimeCheckpoints:    repos.runtimeCheckpoint,
		ArtifactContinuations: repos.artifactContinuation,
		MemoryItems:           repos.memoryItem,
		MemoryRecallTraces:    repos.memoryRecallTrace,
		AutoGovernanceTraces:  repos.autoGovernanceTrace,
		RuntimePolicy:         runtimePolicy,
		EventHub:              eventHub,
	}))

	runModule := runmodule.New()
	mustSetup("run", runModule.Setup(runmodule.Dependencies{
		Runs:         repos.run,
		Workspaces:   repos.workspace,
		Orchestrator: orchestratorModule.Service(),
		EventHub:     eventHub,
	}))
	governanceModule := governancemodule.New()
	mustSetup("governance", governanceModule.Setup(governancemodule.Dependencies{
		Tickets:              repos.ticket,
		Resolutions:          repos.resolution,
		Approvals:            repos.approval,
		Runs:                 repos.run,
		Goals:                repos.goal,
		DoDs:                 repos.dod,
		MemoryItems:          repos.memoryItem,
		AutoGovernanceTraces: repos.autoGovernanceTrace,
		EventHub:             eventHub,
		ResolveDecision: func(ctx context.Context, ticketID common.TicketID, resolutionType string, payload map[string]any) error {
			if !decisioncommon.IsSupportedResolutionType(resolutionType) {
				resolutionType = decisioncommon.ResolutionTypeProvideMissingInfo
			}
			return decisionModule.Service().EnsureResolvedTicket(ctx, decisionservice.ResolveTicketCommand{
				ResolutionID: common.ResolutionID("resolution-" + string(ticketID) + "-auto-governance"),
				TicketID:     ticketID,
				Type:         resolutionType,
				Payload:      payload,
			})
		},
		ResolveApproval: func(ctx context.Context, approvalID common.ApprovalID) error {
			return approvalModule.Service().EnsureApproved(ctx, approvalmodule.ResolveApprovalCommand{ApprovalID: approvalID})
		},
		ResumeRun: func(ctx context.Context, runID common.RunID) error {
			return runModule.Service().EnsureResumed(ctx, runservice.ResumeRunCommand{RunID: runID})
		},
		LoadConfig: workorchconfig.Current,
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
		mcpBlockModule,
		eventStreamModule,
		workspaceModule,
		decisionModule,
		approvalModule,
		orchestratorModule,
		runModule,
		governanceModule,
	}
	moduleLifecycle, err := newRuntimeModuleLifecycle(modules)
	mustSetup("module-lifecycle", err)
	mustSetup("module-lifecycle", moduleLifecycle.Run())

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
				ArtifactContinuations: repos.artifactContinuation,
				Tickets:               repos.ticket,
				Resolutions:           repos.resolution,
				Approvals:             repos.approval,
			},
			Services: ServiceExports{
				WorkspaceService:   workspaceModule.Service(),
				SoulService:        soulModule.Service(),
				MemoryService:      memoryModule.Service(),
				SkillService:       skillModule.Service(),
				MCPService:         mcpModule.Service(),
				EventStreamService: eventStreamModule.Service(),
				DecisionService:    decisionModule.Service(),
				ApprovalService:    approvalModule.Service(),
				RunService:         runModule.Service(),
			},
			Infrastructure: InfrastructureExports{
				EventHub:   eventHub,
				LoadConfig: workorchconfig.Current,
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

func buildRuntimeAdapterRegistry(
	fileAdapter orchestratorbiz.RuntimeAdapter,
	gitAdapter orchestratorbiz.RuntimeAdapter,
	shellAdapter orchestratorbiz.RuntimeAdapter,
	sshAdapter orchestratorbiz.RuntimeAdapter,
	webAdapter orchestratorbiz.RuntimeAdapter,
	mcpAdapter orchestratorbiz.RuntimeAdapter,
	llmAdapter orchestratorbiz.RuntimeAdapter,
) (*orchestratorbiz.RuntimeAdapterRegistry, error) {
	runtimeRegistry := orchestratorbiz.NewRuntimeAdapterRegistry()
	for _, stepKind := range []string{
		filemodule.StepKindList,
		filemodule.StepKindRead,
		filemodule.StepKindEdit,
	} {
		if err := runtimeRegistry.Register(stepKind, fileAdapter); err != nil {
			return nil, err
		}
	}
	if gitAdapter != nil {
		for _, stepKind := range []string{
			gitmodule.StepKindRead,
			gitmodule.StepKindWrite,
		} {
			if err := runtimeRegistry.Register(stepKind, gitAdapter); err != nil {
				return nil, err
			}
		}
	}
	if err := runtimeRegistry.Register(shellmodule.StepKindCommand, shellAdapter); err != nil {
		return nil, err
	}
	if err := runtimeRegistry.Register(sshmodule.StepKindCommand, sshAdapter); err != nil {
		return nil, err
	}
	for _, stepKind := range []string{
		webmodule.StepKindSearch,
		webmodule.StepKindFetch,
	} {
		if err := runtimeRegistry.Register(stepKind, webAdapter); err != nil {
			return nil, err
		}
	}
	for _, stepKind := range []string{
		mcpblockmodule.StepKindToolCall,
		mcpblockmodule.StepKindResourceRead,
		mcpblockmodule.StepKindPromptGet,
	} {
		if err := runtimeRegistry.Register(stepKind, mcpAdapter); err != nil {
			return nil, err
		}
	}
	if err := runtimeRegistry.Register(llmcommon.RuntimeBlockKindPatchApply, orchestratorbiz.NewPatchRuntimeAdapter()); err != nil {
		return nil, err
	}
	if err := runtimeRegistry.Register(llmcommon.RuntimeBlockKindPatchPreview, orchestratorbiz.NewPatchRuntimeAdapter()); err != nil {
		return nil, err
	}
	if err := runtimeRegistry.Register(llmcommon.RuntimeBlockKindPatchRevert, orchestratorbiz.NewPatchRuntimeAdapter()); err != nil {
		return nil, err
	}
	if err := runtimeRegistry.Register(llmmodule.StepKindWorkspaceExecution, llmAdapter); err != nil {
		return nil, err
	}
	return runtimeRegistry, nil
}
