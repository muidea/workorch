package git

import (
	"context"
	"errors"
	frameworkdef "github.com/muidea/magicCommon/def"
	plugincommon "github.com/muidea/magicCommon/framework/plugin/common"
	"os"
	"path/filepath"
	"testing"

	gitbiz "github.com/muidea/workorch/internal/modules/blocks/git/biz"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func useModuleTestConfig(t *testing.T) {
	t.Helper()
	configDir := t.TempDir()
	content := `endpointName = "workorch"

[server]
addr = "127.0.0.1:0"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "30s"
idleTimeout = "60s"
shutdownTimeout = "5s"

[repository]
backend = "memory"
dataDir = ".workorch-data"

[runtime.git]
defaultTimeout = "30s"

[llm.aiProxy]
baseURL = "http://127.0.0.1:9"
apiKey = "test-key"
timeout = "60s"
catalogRequestTimeout = "5s"
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
		t.Fatalf("write module test config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
	if _, err := workorchconfig.Current(); err != nil {
		t.Fatalf("current config: %v", err)
	}
}

func TestGitModuleIdentityConfigureStartStopContract(t *testing.T) {
	useModuleTestConfig(t)
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	defer rawHub.Terminate(ctx)

	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(gitbiz.Dependencies{EventHub: rawHub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
}

func TestGitModuleStartRequiresEventHub(t *testing.T) {
	useModuleTestConfig(t)
	module := New()
	if err := module.Configure(gitbiz.Dependencies{}); err != nil {
		t.Fatalf("Configure with empty dependencies: %v", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub err = %v, want invalid argument", err)
	}
}

type moduleResourceLedgerHub struct {
	magicevent.Hub
	subscribeCalls  int
	failSubscribe   int
	failUnsubscribe bool
	failure         *frameworkdef.Error
}

func (h *moduleResourceLedgerHub) Subscribe(topic string, observer magicevent.Observer) *frameworkdef.Error {
	h.subscribeCalls++
	if h.subscribeCalls == h.failSubscribe {
		return h.failure
	}
	return h.Hub.Subscribe(topic, observer)
}
func (h *moduleResourceLedgerHub) Unsubscribe(topic string, observer magicevent.Observer) *frameworkdef.Error {
	if h.failUnsubscribe {
		return h.failure
	}
	return h.Hub.Unsubscribe(topic, observer)
}

func TestGitModuleResourceLedgerRetainsSameOwnerAfterPartialStartAndFailedStop(t *testing.T) {
	useModuleTestConfig(t)
	ctx := context.Background()
	hub := &moduleResourceLedgerHub{Hub: magicevent.NewHub(8), failSubscribe: 3, failure: frameworkdef.NewError(frameworkdef.ResourceExhausted, "subscription failure")}
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	if err := module.Configure(gitbiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	owner := module.bizPtr
	if err := module.Start(); err != hub.failure {
		t.Fatalf("Start error = %v", err)
	}
	if module.bizPtr != owner {
		t.Fatal("failed Start lost owner")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := module.Stop(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop error = %v", err)
	}
	if module.bizPtr != owner {
		t.Fatal("canceled Stop lost owner")
	}
	hub.failUnsubscribe = true
	if err := module.Stop(ctx); err != hub.failure {
		t.Fatalf("Stop error = %v", err)
	}
	if module.bizPtr != owner {
		t.Fatal("failed Stop lost owner")
	}
	hub.failUnsubscribe = false
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if module.bizPtr != nil {
		t.Fatal("successful Stop retained released owner")
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestGitModuleResourceLedgerPluginGuardReportsTeardownFailureAndRetries(t *testing.T) {
	useModuleTestConfig(t)
	ctx := context.Background()
	hub := &moduleResourceLedgerHub{Hub: magicevent.NewHub(8), failSubscribe: 3, failure: frameworkdef.NewError(frameworkdef.ResourceExhausted, "subscription failure")}
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	manager := plugincommon.NewPluginMgr("capability-resource-ledger-test")
	if err := manager.Register(module); err != nil {
		t.Fatal(err)
	}
	if err := manager.Setup(ctx, hub, nil); err != nil {
		t.Fatal(err)
	}
	owner := module.bizPtr
	if err := manager.Run(ctx); err == nil {
		t.Fatal("plugin Run hid subscription failure")
	}
	hub.failUnsubscribe = true
	if err := manager.TeardownChecked(ctx); err == nil {
		t.Fatal("plugin guard hid incomplete teardown")
	}
	if module.bizPtr != owner {
		t.Fatal("plugin guard lost retryable owner")
	}
	hub.failUnsubscribe = false
	if err := manager.TeardownChecked(ctx); err != nil {
		t.Fatal(err)
	}
	if module.bizPtr != nil {
		t.Fatal("plugin retry retained released owner")
	}
	if err := manager.TeardownChecked(ctx); err != nil {
		t.Fatal(err)
	}
}
