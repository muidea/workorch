package testkit

import (
	"context"
	"fmt"
	"sync"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	gitmodule "github.com/muidea/workorch/internal/modules/blocks/git"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
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
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

type lifecycle interface {
	Start() error
	Stop(context.Context) error
}

type contextLifecycle struct {
	start func(context.Context) error
	stop  func(context.Context) error
}

func (l contextLifecycle) Start() error                   { return l.start(context.Background()) }
func (l contextLifecycle) Stop(ctx context.Context) error { return l.stop(ctx) }

type runtimeHub struct {
	primary events.Hub
	command magicevent.Hub
	modules []lifecycle
	unsub   func()
	once    sync.Once
}

func newRuntimeHub(existing events.Hub, commandHub magicevent.Hub) *runtimeHub {
	if commandHub == nil {
		commandHub = magicevent.NewHub(1024)
	}
	if existing == nil {
		existing = events.NewMagicHubWithHubOwner(commandHub, "application/workorchd/testkit")
	}
	return &runtimeHub{primary: existing, command: commandHub}
}

func (h *runtimeHub) Publish(ctx context.Context, envelope events.Envelope) error {
	return h.primary.Publish(ctx, envelope)
}

func (h *runtimeHub) PublishAsync(ctx context.Context, envelope events.Envelope) error {
	if asyncHub, ok := h.primary.(interface {
		PublishAsync(context.Context, events.Envelope) error
	}); ok {
		return asyncHub.PublishAsync(ctx, envelope)
	}
	return h.primary.Publish(ctx, envelope)
}

func (h *runtimeHub) Subscribe(topic string, handler events.Handler) func() {
	return h.primary.Subscribe(topic, handler)
}

func (h *runtimeHub) Published() []events.Envelope {
	if provider, ok := h.primary.(interface{ Published() []events.Envelope }); ok {
		return provider.Published()
	}
	return nil
}

func (h *runtimeHub) Terminate(ctx context.Context) {
	h.once.Do(func() {
		for idx := len(h.modules) - 1; idx >= 0; idx-- {
			_ = h.modules[idx].Stop(ctx)
		}
		h.modules = nil
		if h.unsub != nil {
			h.unsub()
			h.unsub = nil
		}
		if terminator, ok := h.primary.(interface{ Terminate(context.Context) }); ok {
			terminator.Terminate(ctx)
		} else if h.command != nil {
			h.command.Terminate(ctx)
		}
	})
}

// NewRuntime creates a repository runtime and explicitly starts all owner modules
// required by application integration tests. Production bootstrap intentionally
// remains repository-only and does not call this helper.
func NewRuntime(backend string, dataRoot string, policy workorchruntimepolicy.RuntimePolicy, existing events.Hub, commandHub magicevent.Hub) *bootstrap.Runtime {
	return newRuntime(backend, dataRoot, policy, existing, commandHub)
}

// TryNewRuntime is the error-returning form used by tests that exercise invalid
// bootstrap configuration.
func TryNewRuntime(backend string, dataRoot string, policy workorchruntimepolicy.RuntimePolicy, existing events.Hub, commandHub magicevent.Hub) (runtime *bootstrap.Runtime, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("build application test runtime: %v", recovered)
			runtime = nil
		}
	}()
	return newRuntime(backend, dataRoot, policy, existing, commandHub), nil
}

func newRuntime(backend string, dataRoot string, policy workorchruntimepolicy.RuntimePolicy, existing events.Hub, commandHub magicevent.Hub) *bootstrap.Runtime {
	hub := newRuntimeHub(existing, commandHub)
	runtime := bootstrap.NewWithBackendRuntimePolicyAndEventHubs(backend, dataRoot, policy, hub, hub.command)
	modules, err := startOwnerModules(runtime.Exports(), policy, hub.command)
	if err != nil {
		panic(fmt.Errorf("start application test owner modules: %w", err))
	}
	hub.modules = modules
	return runtime
}

func startOwnerModules(exports bootstrap.Exports, policy workorchruntimepolicy.RuntimePolicy, rawHub magicevent.Hub) ([]lifecycle, error) {
	background := exports.Infrastructure.BackgroundRoutine
	repositories := exports.Repositories
	configured := make([]lifecycle, 0, 21)
	configure := func(module lifecycle, err error) error {
		if err != nil {
			return err
		}
		configured = append(configured, module)
		return nil
	}

	fileModule := filemodule.New()
	if err := configure(fileModule, fileModule.Configure(filemodule.Dependencies{EventHub: rawHub, Background: background})); err != nil {
		return nil, err
	}
	gitModule := gitmodule.New()
	if err := configure(gitModule, gitModule.Configure(gitmodule.Dependencies{EventHub: rawHub, Background: background})); err != nil {
		return nil, err
	}
	shellModule := shellmodule.New()
	if err := configure(shellModule, shellModule.Configure(shellmodule.Dependencies{EventHub: rawHub, Background: background})); err != nil {
		return nil, err
	}
	sshModule := sshmodule.New()
	if err := configure(sshModule, sshModule.Configure(sshmodule.Dependencies{EventHub: rawHub, Background: background})); err != nil {
		return nil, err
	}
	webModule := webmodule.New()
	if err := configure(webModule, webModule.Configure(webmodule.Dependencies{EventHub: rawHub, Background: background})); err != nil {
		return nil, err
	}
	llmModule := llmmodule.New()
	if err := configure(llmModule, llmModule.Configure(llmmodule.Dependencies{EventHub: rawHub, Background: background})); err != nil {
		return nil, err
	}
	mcpBlockModule := mcpblockmodule.New()
	if err := configure(mcpBlockModule, mcpBlockModule.Configure(mcpblockmodule.Dependencies{EventHub: rawHub, Background: background})); err != nil {
		return nil, err
	}

	soulModule := soulmodule.New()
	if err := configure(soulModule, soulModule.Configure(soulmodule.Dependencies{Souls: repositories.Souls, EventHub: rawHub})); err != nil {
		return nil, err
	}
	memoryModule := memorymodule.New()
	if err := configure(memoryModule, memoryModule.Configure(memorymodule.Dependencies{MemoryItems: repositories.MemoryItems, MemoryRecallTraces: repositories.MemoryRecallTraces, EventHub: rawHub})); err != nil {
		return nil, err
	}
	skillModule := skillmodule.New()
	if err := configure(skillModule, skillModule.Configure(skillmodule.Dependencies{Skills: repositories.Skills, EventHub: rawHub})); err != nil {
		return nil, err
	}
	mcpModule := mcpmodule.New()
	if err := configure(mcpModule, mcpModule.Configure(mcpmodule.Dependencies{Servers: repositories.MCPServers, Capabilities: repositories.MCPCapabilities, EventHub: rawHub})); err != nil {
		return nil, err
	}
	eventStreamModule := eventstreammodule.New()
	if err := configure(eventStreamModule, eventStreamModule.Configure(eventstreammodule.Dependencies{EventRecords: repositories.EventRecords, EventHub: rawHub})); err != nil {
		return nil, err
	}
	workspaceModule := workspacemodule.New()
	if err := configure(workspaceModule, workspaceModule.Configure(workspacemodule.Dependencies{Workspaces: repositories.Workspaces, Goals: repositories.Goals, DoDs: repositories.DoDs, Templates: repositories.WorkspaceTemplates, EventHub: rawHub})); err != nil {
		return nil, err
	}
	decisionModule := decisionmodule.New()
	if err := configure(decisionModule, decisionModule.Configure(decisionmodule.Dependencies{Tickets: repositories.Tickets, Resolutions: repositories.Resolutions, EvidenceRecords: repositories.EvidenceRecords, EventHub: rawHub})); err != nil {
		return nil, err
	}
	approvalModule := approvalmodule.New()
	if err := configure(approvalModule, approvalModule.Configure(approvalmodule.Dependencies{Approvals: repositories.Approvals, EvidenceRecords: repositories.EvidenceRecords, EventHub: rawHub})); err != nil {
		return nil, err
	}
	resultModule := resultmodule.New()
	if err := configure(resultModule, resultModule.Configure(resultmodule.Dependencies{Results: repositories.Results, Artifacts: repositories.Artifacts, EventHub: rawHub})); err != nil {
		return nil, err
	}

	runtimeRegistry := orchestratormodule.NewRuntimeAdapterRegistry()
	eventAdapter := orchestratormodule.NewEventRuntimeAdapter(rawHub)
	for _, kind := range []string{
		filemodule.StepKindList, filemodule.StepKindRead, filemodule.StepKindEdit,
		filemodule.StepKindPatchApply, filemodule.StepKindPatchPreview, filemodule.StepKindPatchRevert,
		gitmodule.StepKindRead, gitmodule.StepKindWrite, shellmodule.StepKindCommand, sshmodule.StepKindCommand,
		webmodule.StepKindSearch, webmodule.StepKindFetch,
		mcpblockmodule.StepKindToolCall, mcpblockmodule.StepKindResourceRead, mcpblockmodule.StepKindPromptGet,
		llmmodule.StepKindWorkspaceExecution,
	} {
		if err := runtimeRegistry.Register(kind, eventAdapter); err != nil {
			return nil, err
		}
	}
	orchestratorModule := orchestratormodule.New()
	if err := configure(orchestratorModule, orchestratorModule.Configure(orchestratormodule.Dependencies{
		Runtime: runtimeRegistry, Steps: repositories.Steps, ToolCalls: repositories.ToolCalls,
		Patches: repositories.Patches, ValidationResults: repositories.ValidationResults,
		RuntimeCheckpoints: repositories.RuntimeCheckpoints, EvidenceRecords: repositories.EvidenceRecords,
		CoverageWorkItems: repositories.CoverageWorkItems, ArtifactContinuations: repositories.ArtifactContinuations,
		RuntimePolicy: policy, EventHub: exports.Infrastructure.EventHub, CommandHub: rawHub, BackgroundRoutine: background,
	})); err != nil {
		return nil, err
	}
	runModule := runmodule.New()
	if err := configure(runModule, runModule.Configure(runmodule.Dependencies{Runs: repositories.Runs, Steps: repositories.Steps, Checkpoints: repositories.RuntimeCheckpoints, EventHub: rawHub})); err != nil {
		return nil, err
	}
	governanceModule := governancemodule.New()
	if err := configure(governanceModule, governanceModule.Configure(governancemodule.Dependencies{AutoGovernanceTraces: repositories.AutoGovernanceTraces, EventHub: rawHub, LoadConfig: exports.Infrastructure.LoadConfig, BackgroundRoutine: background})); err != nil {
		return nil, err
	}
	systemModule := systemmodule.New()
	if err := systemModule.Configure(systemmodule.Dependencies{Souls: repositories.Souls, Templates: repositories.WorkspaceTemplates, MCPServers: repositories.MCPServers, MCPCapabilities: repositories.MCPCapabilities}); err != nil {
		return nil, err
	}
	configured = append(configured, contextLifecycle{start: systemModule.Start, stop: systemModule.Stop})

	started := make([]lifecycle, 0, len(configured))
	for _, module := range configured {
		if err := module.Start(); err != nil {
			for idx := len(started) - 1; idx >= 0; idx-- {
				_ = started[idx].Stop(context.Background())
			}
			return nil, err
		}
		started = append(started, module)
	}
	return started, nil
}
