package testkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	gitmodule "github.com/muidea/workorch/internal/modules/blocks/git"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	mcpblockmodule "github.com/muidea/workorch/internal/modules/blocks/mcp"
	mcpmodule "github.com/muidea/workorch/internal/modules/blocks/mcpregistry"
	memorymodule "github.com/muidea/workorch/internal/modules/blocks/memory"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	skillmodule "github.com/muidea/workorch/internal/modules/blocks/skill"
	soulmodule "github.com/muidea/workorch/internal/modules/blocks/soul"
	sshmodule "github.com/muidea/workorch/internal/modules/blocks/ssh"
	webmodule "github.com/muidea/workorch/internal/modules/blocks/web"
	workspacemodule "github.com/muidea/workorch/internal/modules/blocks/workspace"
	eventstreammodule "github.com/muidea/workorch/internal/modules/kernel/eventstream"
	governancemodule "github.com/muidea/workorch/internal/modules/kernel/governance"
	orchestratormodule "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	systemmodule "github.com/muidea/workorch/internal/modules/kernel/system"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
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
	magicevent.Hub
	repos   RepositoryExports
	modules []lifecycle
	once    sync.Once
}

func (h *runtimeHub) testRepositories() RepositoryExports {
	if h == nil {
		return RepositoryExports{}
	}
	return h.repos
}

func newRuntimeHub(commandHub magicevent.Hub) *runtimeHub {
	if commandHub == nil {
		commandHub = magicevent.NewHub(1024)
	}
	return &runtimeHub{Hub: commandHub}
}

func (h *runtimeHub) Terminate(ctx context.Context) {
	h.once.Do(func() {
		// Module Stop unsubscribes observers. That must happen while the hub is
		// still accepting control-plane actions, but after background work has
		// been drained by bootstrap.Runtime.Shutdown (background-first).
		for idx := len(h.modules) - 1; idx >= 0; idx-- {
			_ = h.modules[idx].Stop(ctx)
		}
		h.modules = nil
		h.repos = RepositoryExports{}
		if h.Hub != nil {
			h.Hub.Terminate(ctx)
		}
	})
}

// NewRuntime creates a repository runtime and explicitly starts all owner modules
// required by application integration tests. Production bootstrap intentionally
// remains repository-only and does not call this helper.
func NewRuntime(backend string, dataRoot string, commandHub magicevent.Hub) *bootstrap.Runtime {
	return newRuntime(backend, dataRoot, commandHub)
}

// TryNewRuntime is the error-returning form used by tests that exercise invalid
// bootstrap configuration.
func TryNewRuntime(backend string, dataRoot string, commandHub magicevent.Hub) (runtime *bootstrap.Runtime, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("build application test runtime: %v", recovered)
			runtime = nil
		}
	}()
	return newRuntime(backend, dataRoot, commandHub), nil
}

func newRuntime(backend string, dataRoot string, commandHub magicevent.Hub) *bootstrap.Runtime {
	ensureTestLLMConfig()
	hub := newRuntimeHub(commandHub)
	hub.repos = buildRepositories(backend, dataRoot)
	runtime := bootstrap.NewWithBackendAndCommandHub(backend, dataRoot, hub)
	modules, err := startOwnerModules(RuntimeExports(runtime), hub)
	if err != nil {
		panic(fmt.Errorf("start application test owner modules: %w", err))
	}
	hub.modules = modules
	return runtime
}

func startOwnerModules(exports Exports, rawHub magicevent.Hub) ([]lifecycle, error) {
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
	if err := configure(fileModule, fileModule.Configure(filemodule.Dependencies{EventHub: rawHub})); err != nil {
		return nil, err
	}
	gitModule := gitmodule.New()
	if err := configure(gitModule, gitModule.Configure(gitmodule.Dependencies{EventHub: rawHub})); err != nil {
		return nil, err
	}
	shellModule := shellmodule.New()
	if err := configure(shellModule, shellModule.Configure(shellmodule.Dependencies{EventHub: rawHub})); err != nil {
		return nil, err
	}
	sshModule := sshmodule.New()
	if err := configure(sshModule, sshModule.Configure(sshmodule.Dependencies{EventHub: rawHub})); err != nil {
		return nil, err
	}
	webModule := webmodule.New()
	if err := configure(webModule, webModule.Configure(webmodule.Dependencies{EventHub: rawHub})); err != nil {
		return nil, err
	}
	llmModule := llmmodule.New()
	if err := configure(llmModule, llmModule.Configure(llmmodule.Dependencies{EventHub: rawHub})); err != nil {
		return nil, err
	}
	mcpBlockModule := mcpblockmodule.New()
	if err := configure(mcpBlockModule, mcpBlockModule.Configure(mcpblockmodule.Dependencies{EventHub: rawHub})); err != nil {
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
	if err := configure(decisionModule, decisionModule.Configure(decisionmodule.Dependencies{Tickets: repositories.Tickets, Resolutions: repositories.Resolutions, EventHub: rawHub})); err != nil {
		return nil, err
	}
	approvalModule := approvalmodule.New()
	if err := configure(approvalModule, approvalModule.Configure(approvalmodule.Dependencies{Approvals: repositories.Approvals, EventHub: rawHub})); err != nil {
		return nil, err
	}
	resultModule := resultmodule.New()
	if err := configure(resultModule, resultModule.Configure(resultmodule.Dependencies{Results: repositories.Results, Artifacts: repositories.Artifacts, EventHub: rawHub})); err != nil {
		return nil, err
	}

	orchestratorModule := orchestratormodule.New()
	if err := configure(orchestratorModule, orchestratorModule.Configure(orchestratormodule.Dependencies{
		ToolCalls: repositories.ToolCalls,
		Patches:   repositories.Patches, ValidationResults: repositories.ValidationResults,
		EvidenceRecords:   repositories.EvidenceRecords,
		CoverageWorkItems: repositories.CoverageWorkItems, ArtifactContinuations: repositories.ArtifactContinuations,
		CommandHub: rawHub, BackgroundRoutine: background,
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
	if err := systemModule.Configure(systemmodule.Dependencies{EventHub: rawHub}); err != nil {
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

func ensureTestLLMConfig() {
	if cfg, err := workorchconfig.Current(); err == nil && cfg.LLM.AIProxy.BaseURL != "" && cfg.LLM.DefaultChat.ModelID != "" && (cfg.LLM.DefaultChat.ProviderID != "ai-proxy" || len(cfg.LLM.CustomProviders) > 0 || true) {
		return
	}
	configDir, err := os.MkdirTemp("", "workorch-testkit-config-*")
	if err != nil {
		panic(fmt.Errorf("create testkit config dir: %w", err))
	}
	content := `endpointName = "workorch"

[server]
addr = ":8080"

[repository]
backend = "file"
dataDir = ".workorch-data"

[llm.aiProxy]
baseURL = "http://127.0.0.1:9"
timeout = "180s"
catalogRequestTimeout = "1s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "custom-test"
modelID = "model-1"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
defaultMaxOutputTokens = 4096
maxRequestBytes = 4194304

[llm.customProviders.custom-test]
provider = "compatible"
apiSurface = "openai_compatible_http"
baseURL = "http://127.0.0.1:9"
timeout = "30s"

[llm.customProviders.custom-test.models.model-1]
contextWindowTokens = 128000
maxOutputTokens = 8192
operations = ["chat_completions"]
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		panic(fmt.Errorf("write testkit config: %w", err))
	}
	_ = workorchconfig.Close()
	workorchconfig.ConfigureDirectory(configDir)
	if _, err := workorchconfig.Current(); err != nil {
		panic(fmt.Errorf("load testkit config: %w", err))
	}
}
