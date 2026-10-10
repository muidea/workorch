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
	governancebiz "github.com/muidea/workorch/internal/modules/kernel/governance/biz"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	systembiz "github.com/muidea/workorch/internal/modules/kernel/system/biz"
	workspacebiz "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"

	cd "github.com/muidea/magicCommon/def"
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
	governancemodule "github.com/muidea/workorch/internal/modules/kernel/governance"
	orchestratormodule "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	systemmodule "github.com/muidea/workorch/internal/modules/kernel/system"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

type lifecycle interface {
	Start() error
	Stop(context.Context) error
	BeginShutdown(context.Context)
	Quiesce(context.Context) *cd.Error
}

type contextLifecycle struct {
	start   func(context.Context) error
	stop    func(context.Context) error
	begin   func(context.Context)
	quiesce func(context.Context) *cd.Error
}

func (l contextLifecycle) Start() error                          { return l.start(context.Background()) }
func (l contextLifecycle) Stop(ctx context.Context) error        { return l.stop(ctx) }
func (l contextLifecycle) BeginShutdown(ctx context.Context)     { l.begin(ctx) }
func (l contextLifecycle) Quiesce(ctx context.Context) *cd.Error { return l.quiesce(ctx) }

type runtimeHub struct {
	magicevent.Hub
	repos      RepositoryExports
	modules    []lifecycle
	shutdownMu sync.Mutex
	quiesced   bool
	hubDrained bool
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

func (h *runtimeHub) Quiesce(ctx context.Context) error {
	h.shutdownMu.Lock()
	defer h.shutdownMu.Unlock()
	if ctx == nil {
		return fmt.Errorf("shutdown context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, module := range h.modules {
		module.BeginShutdown(ctx)
	}
	for idx := len(h.modules) - 1; idx >= 0; idx-- {
		if err := h.modules[idx].Quiesce(ctx); err != nil {
			return err
		}
	}
	h.quiesced = true
	return nil
}

func (h *runtimeHub) Release(ctx context.Context) error {
	h.shutdownMu.Lock()
	defer h.shutdownMu.Unlock()
	if ctx == nil {
		return fmt.Errorf("shutdown context is required")
	}
	if !h.quiesced || !h.hubDrained {
		return fmt.Errorf("local owners require quiesce and Hub drain before release")
	}
	for len(h.modules) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(h.modules) - 1
		if err := h.modules[last].Stop(ctx); err != nil {
			return err
		}
		h.modules = h.modules[:last]
	}
	h.repos = RepositoryExports{}
	return nil
}

func (h *runtimeHub) Drain(ctx context.Context) *cd.Error {
	h.shutdownMu.Lock()
	defer h.shutdownMu.Unlock()
	if !h.quiesced {
		return cd.NewError(cd.ServiceUnavailable, "local owners have not quiesced")
	}
	if drain, ok := h.Hub.(magicevent.DrainingHub); ok {
		if err := drain.Drain(ctx); err != nil {
			return err
		}
		h.hubDrained = true
		return nil
	}
	return cd.NewError(cd.ServiceUnavailable, "local runtime requires a draining Hub")
}

func (h *runtimeHub) TerminateChecked(ctx context.Context) *cd.Error {
	// owner 释放只能发生在 bootstrap 完成共享任务和 Hub 屏障之后。
	h.shutdownMu.Lock()
	remaining := len(h.modules)
	h.shutdownMu.Unlock()
	if remaining != 0 {
		return cd.NewError(cd.ServiceUnavailable, "local owners have not been released")
	}
	if drain, ok := h.Hub.(magicevent.DrainingHub); ok {
		return drain.TerminateChecked(ctx)
	}
	return cd.NewError(cd.ServiceUnavailable, "local runtime requires a draining Hub")
}

func (h *runtimeHub) Terminate(ctx context.Context) {
	if err := h.TerminateChecked(ctx); err != nil {
		panic(err)
	}
}

type runtimeStartupError struct {
	Runtime *bootstrap.Runtime
	Cause   error
}

func (e *runtimeStartupError) Error() string {
	return fmt.Sprintf("start application test owner modules: %v", e.Cause)
}
func (e *runtimeStartupError) Unwrap() error { return e.Cause }

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
			if failure, ok := recovered.(*runtimeStartupError); ok {
				runtime, err = failure.Runtime, failure.Cause
			} else {
				err = fmt.Errorf("build application test runtime: %v", recovered)
				runtime = nil
			}
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
	defer func() {
		if recovered := recover(); recovered != nil {
			cause, ok := recovered.(error)
			if !ok {
				cause = fmt.Errorf("start application test owners panic: %v", recovered)
			}
			panic(&runtimeStartupError{Runtime: runtime, Cause: cause})
		}
	}()
	modules, err := startOwnerModules(RuntimeExports(runtime), hub, dataRoot)
	hub.modules = modules
	if err != nil {
		panic(err)
	}
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
	configured := make([]lifecycle, 0, 20)
	// 在 Start 返回错误或 panic 前发布已配置账本，保留同一实例用于检查式停止。
	defer func() {
		if hub, ok := rawHub.(*runtimeHub); ok {
			hub.modules = configured
		}
	}()
	configure := func(module lifecycle, err error) error {
		if err != nil {
			return err
		}
		configured = append(configured, module)
		return nil
	}

	fileModule := filemodule.New()
	if err := configure(fileModule, fileModule.Configure(filebiz.Dependencies{EventHub: rawHub})); err != nil {
		return configured, err
	}
	gitModule := gitmodule.New()
	// Test runtimes must not depend on a process-global repository.dataDir.
	// Capability-state query receipts are owner-private durable state, so give
	// the Git owner an explicit local store rather than projecting a global
	// configuration value into the test session.
	gitReceipts, err := gitbiz.NewCapabilityStateReceiptStore(filepath.Join(dataRoot, "runtime", "git-receipts"))
	if err != nil {
		return configured, err
	}
	if err := configure(gitModule, gitModule.Configure(gitbiz.Dependencies{
		EventHub: rawHub, RuntimeAdapter: gitbiz.NewRuntimeAdapter(), CapabilityStateReceipts: gitReceipts,
	})); err != nil {
		return configured, err
	}
	shellModule := shellmodule.New()
	if err := configure(shellModule, shellModule.Configure(shellbiz.Dependencies{EventHub: rawHub})); err != nil {
		return configured, err
	}
	sshModule := sshmodule.New()
	if err := configure(sshModule, sshModule.Configure(sshbiz.Dependencies{EventHub: rawHub})); err != nil {
		return configured, err
	}
	webModule := webmodule.New()
	if err := configure(webModule, webModule.Configure(webbiz.Dependencies{EventHub: rawHub})); err != nil {
		return configured, err
	}
	llmModule := llmmodule.New()
	if err := configure(llmModule, llmModule.Configure(llmbiz.Dependencies{EventHub: rawHub})); err != nil {
		return configured, err
	}
	mcpBlockModule := mcpblockmodule.New()
	if err := configure(mcpBlockModule, mcpBlockModule.Configure(mcpblockbiz.Dependencies{EventHub: rawHub})); err != nil {
		return configured, err
	}

	soulModule := soulmodule.New()
	if err := configure(soulModule, soulModule.Configure(soulbiz.Dependencies{Souls: repositories.Souls, EventHub: rawHub})); err != nil {
		return configured, err
	}
	memoryModule := memorymodule.New()
	if err := configure(memoryModule, memoryModule.Configure(memorybiz.Dependencies{OwnerRepositories: repositories.MemoryOwner, EventHub: rawHub})); err != nil {
		return configured, err
	}
	skillModule := skillmodule.New()
	if err := configure(skillModule, skillModule.Configure(skillbiz.Dependencies{Skills: repositories.Skills, EventHub: rawHub})); err != nil {
		return configured, err
	}
	mcpModule := mcpmodule.New()
	if err := configure(mcpModule, mcpModule.Configure(mcpregistrybiz.Dependencies{Registry: repositories.MCPRegistry, EventHub: rawHub})); err != nil {
		return configured, err
	}
	eventStreamModule := eventstreammodule.New()
	if err := configure(eventStreamModule, eventStreamModule.Configure(eventstreambiz.Dependencies{
		OwnerRepositories: repositories.EventStreamOwner, EventHub: rawHub,
	})); err != nil {
		return configured, err
	}
	workspaceModule := workspacemodule.New()
	if err := configure(workspaceModule, workspaceModule.Configure(workspacebiz.Dependencies{OwnerRepositories: repositories.WorkspaceOwner, EventHub: rawHub})); err != nil {
		return configured, err
	}
	decisionModule := decisionmodule.New()
	if err := configure(decisionModule, decisionModule.Configure(decisionbiz.Dependencies{OwnerRepositories: repositories.DecisionOwner, EventHub: rawHub})); err != nil {
		return configured, err
	}
	approvalModule := approvalmodule.New()
	if err := configure(approvalModule, approvalModule.Configure(approvalbiz.Dependencies{OwnerRepositories: repositories.ApprovalOwner, EventHub: rawHub})); err != nil {
		return configured, err
	}
	resultModule := resultmodule.New()
	if err := configure(resultModule, resultModule.Configure(resultbiz.Dependencies{OwnerRepositories: repositories.ResultOwner, EventHub: rawHub})); err != nil {
		return configured, err
	}

	// Run starts before Orchestrator so Run-owner pending activation and
	// retirement facts are queryable when Orchestrator performs its startup
	// inventory. Orchestrator then observes the already-recovered current facts;
	// it must not rely on a best-effort Run notification sent before subscription.
	runModule := runmodule.New()
	// Run.Configure 已可能登记订阅；调用前纳入账本，错误和 panic 都不能丢失该 owner。
	configured = append(configured, runModule)
	if err := runModule.Configure(runbiz.Dependencies{OwnerRepositories: repositories.RunOwner, EventHub: rawHub}); err != nil {
		return configured, err
	}
	orchestratorModule := orchestratormodule.New()
	if err := configure(orchestratorModule, orchestratorModule.Configure(orchestratorbiz.Dependencies{
		Evidence:   repositories.OrchestratorEvidence,
		CommandHub: rawHub, BackgroundRoutine: background,
	})); err != nil {
		return configured, err
	}
	governanceModule := governancemodule.New()
	if err := configure(governanceModule, governanceModule.Configure(governancebiz.EventDependencies{AutoGovernanceTraces: repositories.AutoGovernanceTraces, EventHub: rawHub, LoadConfig: exports.Infrastructure.LoadConfig, BackgroundRoutine: background})); err != nil {
		return configured, err
	}
	systemModule := systemmodule.New()
	if err := systemModule.Configure(systembiz.Dependencies{EventHub: rawHub}); err != nil {
		return configured, err
	}
	configured = append(configured, contextLifecycle{start: systemModule.Start, stop: systemModule.Stop, begin: systemModule.BeginShutdown, quiesce: systemModule.Quiesce})

	for _, module := range configured {
		if err := module.Start(); err != nil {
			return configured, err
		}
	}
	return configured, nil
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
