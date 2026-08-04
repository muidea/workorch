package shell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"

	magicevent "github.com/muidea/magicCommon/event"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
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

func TestShellModuleIdentityConfigureStartStopContract(t *testing.T) {
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
	if err := module.Configure(shellbiz.Dependencies{EventHub: rawHub}); err != nil {
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

func TestShellModuleExecutesConcreteEventHubContract(t *testing.T) {
	useModuleTestConfig(t)
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	registerShellModuleReceiptOwner(t, hub)
	module := New()
	if err := module.Configure(shellbiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	result := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
		shellevents.ExecuteOperationCommand{
			RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
				Execution:   runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-1/run-1/dispatch-1/shell-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectNone},
				WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1",
				Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", RequiredOutputHash: "output-1"},
				OperationID: "shell-1", EvidenceKind: runtimecontract.EvidenceKindValidation,
				WorkspaceDeliverableDirectory: t.TempDir(),
			},
			Data: shellevents.OperationData{Command: "sh", Args: []string{"-c", "printf typed-shell"}, TimeoutMillis: 5000},
		},
	))
	data, magicErr := result.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	response := data.(shellevents.ExecuteOperationResult)
	if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || response.Receipt.Evidence.ExitCode == nil {
		t.Fatalf("shell response = %+v", response)
	}
}

func TestShellModuleStartRequiresEventHub(t *testing.T) {
	useModuleTestConfig(t)
	module := New()
	if err := module.Configure(shellbiz.Dependencies{}); err != nil {
		t.Fatalf("Configure with empty dependencies: %v", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub err = %v, want invalid argument", err)
	}
}

func TestShellModuleResolvesManagedValidationCapability(t *testing.T) {
	result, err := shellevents.ResolveRuntimeCapabilities(shellevents.AcquireRuntimeCapabilitiesCommand{
		WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "snap-1", PrerequisiteID: "cap-1",
		OwnerModuleID: ModuleID, CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation,
		RequiredKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession, StepKindCommand},
	}, ModuleID, StepKindCommand, runtimecontract.RuntimeBlockKindManagedValidationSession)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Acquisition.Capabilities.ValidationKinds) != 2 {
		t.Fatalf("validation kinds = %v", result.Acquisition.Capabilities.ValidationKinds)
	}
	foundManaged, foundShell := false, false
	for _, kind := range result.Acquisition.Capabilities.ValidationKinds {
		if kind == runtimecontract.RuntimeBlockKindManagedValidationSession {
			foundManaged = true
		}
		if kind == StepKindCommand {
			foundShell = true
		}
	}
	if !foundManaged || !foundShell {
		t.Fatalf("validation kinds = %v", result.Acquisition.Capabilities.ValidationKinds)
	}
}

func registerShellModuleReceiptOwner(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	receipts := map[string]runtimecontract.RuntimeOperationReceipt{}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found := receipts[command.ExecutionKey]
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Receipt: &receipt, Found: found}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		receipts[command.Receipt.ExecutionKey] = command.Receipt
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})
}
