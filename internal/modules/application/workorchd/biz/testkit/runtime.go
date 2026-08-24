package testkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	eventstreambiz "github.com/muidea/workorch/internal/modules/blocks/eventstream/biz"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	mcpblockbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	mcpregistrybiz "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/biz"
	memorybiz "github.com/muidea/workorch/internal/modules/blocks/memory/biz"
	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	skillbiz "github.com/muidea/workorch/internal/modules/blocks/skill/biz"
	soulbiz "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	sshbiz "github.com/muidea/workorch/internal/modules/blocks/ssh/biz"
	webbiz "github.com/muidea/workorch/internal/modules/blocks/web/biz"
	workspacebiz "github.com/muidea/workorch/internal/modules/blocks/workspace/biz"
	governancebiz "github.com/muidea/workorch/internal/modules/kernel/governance/biz"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	systembiz "github.com/muidea/workorch/internal/modules/kernel/system/biz"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	eventstreammodule "github.com/muidea/workorch/internal/modules/blocks/eventstream"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	gitmodule "github.com/muidea/workorch/internal/modules/blocks/git"
	gitbiz "github.com/muidea/workorch/internal/modules/blocks/git/biz"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	mcpblockmodule "github.com/muidea/workorch/internal/modules/blocks/mcp"
	mcpmodule "github.com/muidea/workorch/internal/modules/blocks/mcpregistry"
	memorymodule "github.com/muidea/workorch/internal/modules/blocks/memory"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	skillmodule "github.com/muidea/workorch/internal/modules/blocks/skill"
	soulmodule "github.com/muidea/workorch/internal/modules/blocks/soul"
	sshmodule "github.com/muidea/workorch/internal/modules/blocks/ssh"
	webmodule "github.com/muidea/workorch/internal/modules/blocks/web"
	workspacemodule "github.com/muidea/workorch/internal/modules/blocks/workspace"
	governancemodule "github.com/muidea/workorch/internal/modules/kernel/governance"
	orchestratormodule "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	systemmodule "github.com/muidea/workorch/internal/modules/kernel/system"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
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
	dataRoot = testRuntimeDataRoot(backend, dataRoot)
	hub := newRuntimeHub(commandHub)
	hub.repos = buildRepositories(backend, dataRoot)
	runtime := bootstrap.NewWithBackendAndCommandHub(backend, dataRoot, hub)
	modules, err := startOwnerModules(RuntimeExports(runtime), hub, dataRoot)
	if err != nil {
		panic(fmt.Errorf("start application test owner modules: %w", err))
	}
	hub.modules = modules
	return runtime
}

// testRuntimeDataRoot prevents test-only owner state from falling back to a
// package working directory. Callers that need persistence across sessions pass
// an explicit root (normally t.TempDir); isolated fixtures receive a /tmp root.
func testRuntimeDataRoot(backend string, dataRoot string) string {
	if dataRoot != "" {
		return dataRoot
	}
	if workorchrepo.NormalizeRepositoryBackend(backend) == "file" {
		return workorchrepo.ResolveRepositoryDataRoot(backend, "")
	}
	root, err := os.MkdirTemp("", "workorch-testkit-runtime-")
	if err != nil {
		panic(fmt.Errorf("create testkit runtime root: %w", err))
	}
	return root
}

func startOwnerModules(exports Exports, rawHub magicevent.Hub, dataRoot string) ([]lifecycle, error) {
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
	if err := configure(fileModule, fileModule.Configure(filebiz.Dependencies{EventHub: rawHub})); err != nil {
		return nil, err
	}
	gitModule := gitmodule.New()
	// Test runtimes must not depend on a process-global repository.dataDir.
	// Capability-state query receipts are owner-private durable state, so give
	// the Git owner an explicit local store rather than projecting a global
	// configuration value into the test session.
	gitReceipts, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(dataRoot, "runtime", "git-receipts"))
	if err != nil {
		return nil, err
	}
	if err := configure(gitModule, gitModule.Configure(gitbiz.Dependencies{
		EventHub: rawHub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: gitReceipts,
	})); err != nil {
		return nil, err
	}
	shellModule := shellmodule.New()
	if err := configure(shellModule, shellModule.Configure(shellbiz.Dependencies{EventHub: rawHub})); err != nil {
		return nil, err
	}
	sshModule := sshmodule.New()
	if err := configure(sshModule, sshModule.Configure(sshbiz.Dependencies{EventHub: rawHub})); err != nil {
		return nil, err
	}
	webModule := webmodule.New()
	if err := configure(webModule, webModule.Configure(webbiz.Dependencies{EventHub: rawHub})); err != nil {
		return nil, err
	}
	llmModule := llmmodule.New()
	if err := configure(llmModule, llmModule.Configure(llmbiz.Dependencies{EventHub: rawHub})); err != nil {
		return nil, err
	}
	mcpBlockModule := mcpblockmodule.New()
	if err := configure(mcpBlockModule, mcpBlockModule.Configure(mcpblockbiz.Dependencies{EventHub: rawHub})); err != nil {
		return nil, err
	}

	soulModule := soulmodule.New()
	if err := configure(soulModule, soulModule.Configure(soulbiz.Dependencies{Souls: repositories.Souls, EventHub: rawHub})); err != nil {
		return nil, err
	}
	memoryModule := memorymodule.New()
	if err := configure(memoryModule, memoryModule.Configure(memorybiz.Dependencies{MemoryItems: repositories.MemoryItems, MemoryRecallTraces: repositories.MemoryRecallTraces, EventHub: rawHub})); err != nil {
		return nil, err
	}
	skillModule := skillmodule.New()
	if err := configure(skillModule, skillModule.Configure(skillbiz.Dependencies{Skills: repositories.Skills, EventHub: rawHub})); err != nil {
		return nil, err
	}
	mcpModule := mcpmodule.New()
	if err := configure(mcpModule, mcpModule.Configure(mcpregistrybiz.Dependencies{Registry: repositories.MCPRegistry, EventHub: rawHub})); err != nil {
		return nil, err
	}
	eventStreamModule := eventstreammodule.New()
	if err := configure(eventStreamModule, eventStreamModule.Configure(eventstreambiz.Dependencies{EventRecords: repositories.EventRecords, EventHub: rawHub})); err != nil {
		return nil, err
	}
	workspaceModule := workspacemodule.New()
	if err := configure(workspaceModule, workspaceModule.Configure(workspacebiz.Dependencies{Workspaces: repositories.Workspaces, Goals: repositories.Goals, DoDs: repositories.DoDs, Templates: repositories.WorkspaceTemplates, EventHub: rawHub})); err != nil {
		return nil, err
	}
	decisionModule := decisionmodule.New()
	if err := configure(decisionModule, decisionModule.Configure(decisionbiz.Dependencies{Tickets: repositories.Tickets, Resolutions: repositories.Resolutions, EventHub: rawHub})); err != nil {
		return nil, err
	}
	approvalModule := approvalmodule.New()
	if err := configure(approvalModule, approvalModule.Configure(approvalbiz.Dependencies{Approvals: repositories.Approvals, EventHub: rawHub})); err != nil {
		return nil, err
	}
	resultModule := resultmodule.New()
	if err := configure(resultModule, resultModule.Configure(resultbiz.Dependencies{Results: repositories.Results, Artifacts: repositories.Artifacts, EventHub: rawHub})); err != nil {
		return nil, err
	}

	orchestratorModule := orchestratormodule.New()
	if err := configure(orchestratorModule, orchestratorModule.Configure(orchestratorbiz.Dependencies{
		ToolCalls: repositories.ToolCalls,
		Patches:   repositories.Patches, ValidationResults: repositories.ValidationResults,
		EvidenceRecords:   repositories.EvidenceRecords,
		CoverageWorkItems: repositories.CoverageWorkItems, ArtifactContinuations: repositories.ArtifactContinuations,
		CommandHub: rawHub, BackgroundRoutine: background,
	})); err != nil {
		return nil, err
	}
	runModule := runmodule.New()
	if err := configure(runModule, runModule.Configure(runbiz.Dependencies{Runs: repositories.Runs, Steps: repositories.Steps, Checkpoints: repositories.RuntimeCheckpoints, EventHub: rawHub})); err != nil {
		return nil, err
	}
	governanceModule := governancemodule.New()
	if err := configure(governanceModule, governanceModule.Configure(governancebiz.EventDependencies{AutoGovernanceTraces: repositories.AutoGovernanceTraces, EventHub: rawHub, LoadConfig: exports.Infrastructure.LoadConfig, BackgroundRoutine: background})); err != nil {
		return nil, err
	}
	systemModule := systemmodule.New()
	if err := systemModule.Configure(systembiz.Dependencies{EventHub: rawHub}); err != nil {
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
	if cfg, err := workorchconfig.Current(); err == nil && cfg.LLM.AIProxy.BaseURL != "" && cfg.LLM.DefaultChat.ProviderID == "ai-proxy" && cfg.LLM.DefaultChat.ModelID != "" {
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
providerID = "ai-proxy"
modelID = "model-1"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

[llm.aiProxy.modelOverrides.model-1]
contextWindowTokens = 128000
maxOutputTokens = 8192
operations = ["responses"]
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
