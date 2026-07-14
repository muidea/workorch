package shell

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellModuleIdentityConfigureStartStopContract(t *testing.T) {
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

func TestShellModuleExecutesConcreteEventHubContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	if err := module.Configure(Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteShellOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx,
		orchestratorevents.ExecuteShellOperationCommand{
			WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1", RequiredOutputHash: "output-1"},
			OperationID: "shell-1", EvidenceKind: runtimecontract.EvidenceKindValidation,
			WorkspaceDeliverableDirectory: t.TempDir(),
			Data:                          orchestratorevents.ShellOperationData{Command: "sh", Args: []string{"-c", "printf typed-shell"}, TimeoutMillis: 5000},
		},
	))
	data, magicErr := result.Get()
	if magicErr != nil {
		t.Fatal(magicErr)
	}
	response := data.(orchestratorevents.ExecuteShellOperationResult)
	if response.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || response.Data.ExitCode != 0 || response.Data.Stdout != "typed-shell" || response.Evidence.ExitCode == nil {
		t.Fatalf("shell response = %+v", response)
	}
}

func TestShellModuleStartRequiresEventHub(t *testing.T) {
	module := New()
	if err := module.Configure(Dependencies{}); err != nil {
		t.Fatalf("Configure with empty dependencies: %v", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub err = %v, want invalid argument", err)
	}
}
