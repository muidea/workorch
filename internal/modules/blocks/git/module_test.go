package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
maxConcurrentRuns = 1

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
apiKey = "test-key"
timeout = "30s"

[llm.customProviders.custom-test.models.model-1]
contextWindowTokens = 128000
maxOutputTokens = 8192
operations = ["chat_completions"]
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

func TestValidateGitCapabilityStatePredecessorReceiptsRequiresExactDurableBinding(t *testing.T) {
	snapshot := runtimecontract.RuntimeCapabilityStateSnapshot{RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
		{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"},
		{RequirementID: "stage", Kind: runtimecontract.StepKindGitWrite, VariantID: "add", DependsOnRequirementIDs: []string{"repo-init"}},
	}}
	required := snapshot.RequiredOperationSequence[1]
	binding := runtimecontract.RuntimeRequiredOperationReceiptBinding{
		RequirementID: "repo-init", OperationID: "git-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init",
		ExecutionKey: "exec/ws/run/dispatch/git-init", Fence: 1, ReceiptRef: "receipt/git-init", ReceiptHash: "receipt-hash",
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true,
	}
	if err := validateGitCapabilityStatePredecessorReceipts(snapshot, required, []runtimecontract.RuntimeRequiredOperationReceiptBinding{binding}); err != nil {
		t.Fatalf("accepted predecessor binding: %v", err)
	}
	if err := validateGitCapabilityStatePredecessorReceipts(snapshot, required, nil); err == nil {
		t.Fatal("missing predecessor binding was accepted")
	}
	binding.VariantID = "commit"
	if err := validateGitCapabilityStatePredecessorReceipts(snapshot, required, []runtimecontract.RuntimeRequiredOperationReceiptBinding{binding}); err == nil {
		t.Fatal("variant-mismatched predecessor binding was accepted")
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
	if err := module.Configure(Dependencies{EventHub: rawHub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.runtimeAdapter == nil || module.eventHub == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstObserver := module.capabilityObserver
	if firstObserver == nil {
		t.Fatal("capability observer is nil after Start")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if module.capabilityObserver != firstObserver {
		t.Fatal("second Start replaced capability observer")
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.runtimeAdapter != nil || module.eventHub != nil || module.capabilityObserver != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
}

func TestGitModuleStartRequiresEventHub(t *testing.T) {
	useModuleTestConfig(t)
	module := New()
	if err := module.Configure(Dependencies{}); err != nil {
		t.Fatalf("Configure with empty dependencies: %v", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub err = %v, want invalid argument", err)
	}
}
