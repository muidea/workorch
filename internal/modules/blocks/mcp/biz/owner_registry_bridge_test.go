package mcp

import (
	"context"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	registryevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorcheventpayload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMCPOwnerReadsTypedRegistrySnapshotThroughProductionEventBridge(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })

	server := &mcpmodel.Server{
		ID: "registry-bridge", Name: "registry bridge", Transport: "http", Endpoint: "https://mcp.invalid",
		Enabled: true, AllowedTools: []string{"inspect"}, Env: map[string]string{"TOKEN": "env://MCP_BRIDGE_TOKEN"}, TimeoutMillis: 1000,
	}
	wireServer, err := registryevents.MCPServerFromModel(server)
	if err != nil {
		t.Fatal(err)
	}
	capability := &mcpmodel.Capability{
		ID: "registry-bridge:tool:inspect", ServerID: server.ID, Kind: mcpmodel.CapabilityKindTool, Name: "inspect", Enabled: true,
		EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
		ValidationProofApplicability: string(runtimecontract.RuntimeValidationProofDirect),
		Schema: workorcheventpayload.New(
			workorcheventpayload.StringField("type", "object"),
			workorcheventpayload.BoolField("additionalProperties", false),
		),
	}

	var getCalls, snapshotCalls atomic.Int32
	registry := magicevent.NewSimpleObserver(registryevents.CommandDestination, hub)
	registry.Subscribe(registryevents.TopicMCPCommandGetServer, func(event magicevent.Event, result magicevent.Result) {
		getCalls.Add(1)
		command, ok := event.Data().(registryevents.GetServerCommand)
		if !ok || event.Source() != ModuleID || command.ServerID != server.ID {
			result.Set(nil, nil)
			return
		}
		result.Set(registryevents.GetServerResult{Server: wireServer, RegistryRevision: "registry-v1"}, nil)
	})
	registry.Subscribe(registryevents.TopicMCPCommandReadRegistrySnapshot, func(event magicevent.Event, result magicevent.Result) {
		snapshotCalls.Add(1)
		if _, ok := event.Data().(registryevents.ReadRegistrySnapshotCommand); !ok || event.Source() != ModuleID {
			result.Set(nil, nil)
			return
		}
		result.Set(registryevents.RegistrySnapshotResult{
			Servers: []*registryevents.MCPServer{wireServer}, Capabilities: []*mcpmodel.Capability{capability}, RegistryRevision: "registry-v1",
		}, nil)
	})
	t.Cleanup(func() {
		registry.Unsubscribe(registryevents.TopicMCPCommandGetServer)
		registry.Unsubscribe(registryevents.TopicMCPCommandReadRegistrySnapshot)
	})

	getter := mcpEventServerGetter{hub: hub}
	gotServer, err := getter.GetServer(ctx, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotServer.ID != server.ID || gotServer.Env["TOKEN"] != "env://MCP_BRIDGE_TOKEN" || getCalls.Load() != 1 {
		t.Fatalf("typed GetServer bridge returned %+v after %d calls", gotServer, getCalls.Load())
	}

	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	request := runtimecontract.RuntimeOperationAdmissibilityRequest{
		WorkspaceID: "ws-mcp-registry-bridge", RunID: "run-mcp-registry-bridge", AuthoritySnapshotID: "authority-mcp-registry-bridge",
		PrerequisiteID: "admissibility-mcp-registry-bridge", OwnerModuleID: ModuleID,
		WorkspaceDeliverableDirectory: t.TempDir(), RequiredKinds: []string{runtimecontract.StepKindMCPToolCall},
		MaxItems: runtimecontract.RuntimeOwnerInventoryItemsMaxCount,
	}
	result := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, mcpevents.AcquireRuntimeOperationAdmissibilityCommand(request),
	))
	data, getErr := result.Get()
	if getErr != nil {
		t.Fatal(getErr)
	}
	response, ok := data.(mcpevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok {
		t.Fatalf("admissibility response type = %T", data)
	}
	if err := response.Acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("validate MCP admissibility acquired through Registry bridge: %v", err)
	}
	if snapshotCalls.Load() != 1 || len(response.Acquisition.Admissibility.MCPCapabilities) != 1 || response.Acquisition.Admissibility.ObservedRevision != "registry-v1" {
		t.Fatalf("MCP Registry bridge acquisition = %+v snapshotCalls=%d", response.Acquisition, snapshotCalls.Load())
	}
	if response.Acquisition.Admissibility.MCPCapabilities[0].ServerID != common.MCPServerID("registry-bridge") {
		t.Fatalf("MCP capability lost Registry owner identity: %+v", response.Acquisition.Admissibility.MCPCapabilities)
	}
}
