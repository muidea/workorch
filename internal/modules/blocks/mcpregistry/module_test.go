package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/biz"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if ModuleID != "blocks/mcp-registry" || module.ID() != ModuleID {
		t.Fatalf("module identity = %q/%q", ModuleID, module.ID())
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(mcpModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(context.Background()); err != nil || module.bizPtr != nil {
		t.Fatalf("Stop: %v biz=%v", err, module.bizPtr)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresAggregateAndEventHub(t *testing.T) {
	base := mcpModuleContractDependencies()
	tests := []struct {
		name string
		deps mcpbiz.Dependencies
	}{
		{name: "missing aggregate", deps: func() mcpbiz.Dependencies { d := base; d.Registry = nil; return d }()},
		{name: "missing event hub", deps: func() mcpbiz.Dependencies { d := base; d.EventHub = nil; return d }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

func TestModuleUsesConcreteCommandResultsAndConsistentSnapshot(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	registry := memory.NewMCPRegistryRepository()
	module := New()
	if err := module.Configure(mcpbiz.Dependencies{Registry: registry, EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	create := sendMCPCommand[mcpevents.CreateServerCommand, mcpevents.CreateServerResult](t, ctx, hub, mcpevents.TopicMCPCommandCreateServer, mcpevents.CreateServerCommand{
		ServerID: "mcp-contract", Name: "contract", Transport: "stdio", Command: "mcp-contract", Enabled: true,
		EnvironmentRefs: []mcpevents.EnvironmentReference{{Name: "TOKEN", Reference: "env://MCP_TOKEN"}},
		AllowedTools:    []string{"repo.summary"},
	})
	if create.Server == nil || create.Server.ID != "mcp-contract" || create.RegistryRevision == "" {
		t.Fatalf("CreateServerResult = %+v", create)
	}
	snapshot := sendMCPCommand[mcpevents.ReadRegistrySnapshotCommand, mcpevents.RegistrySnapshotResult](t, ctx, hub, mcpevents.TopicMCPCommandReadRegistrySnapshot, mcpevents.ReadRegistrySnapshotCommand{})
	if snapshot.RegistryRevision != create.RegistryRevision || len(snapshot.Servers) != 1 || len(snapshot.Capabilities) != 1 {
		t.Fatalf("RegistrySnapshotResult = %+v", snapshot)
	}

	createdCapability := sendMCPCommand[mcpevents.CreateCapabilityCommand, mcpevents.CreateCapabilityResult](t, ctx, hub, mcpevents.TopicMCPCommandCreateCapability, mcpevents.CreateCapabilityCommand{
		CapabilityID: "cap-repo-summary", ServerID: "mcp-contract", Kind: "tool", Name: "repo.summary",
		DisplayName: "Repository Summary", Category: "repository", Tags: []string{"read"}, StageHints: []string{"planning"},
		EffectScope: "none", IdempotencyClass: "safe_to_replay", ValidationProofApplicability: "direct",
		Schema: mustMCPPayload(t, map[string]any{"type": "object"}), Enabled: true,
	})
	if createdCapability.Capability == nil || createdCapability.Capability.Source != "user" {
		t.Fatalf("CreateCapabilityResult = %+v", createdCapability)
	}
	selection := sendMCPCommand[mcpevents.ReadSelectionSnapshotCommand, mcpevents.SelectionSnapshotResult](t, ctx, hub, mcpevents.TopicMCPCommandReadSelectionSnapshot, mcpevents.ReadSelectionSnapshotCommand{
		ServerIDs: []common.MCPServerID{"mcp-contract"}, CapabilityIDs: []common.MCPCapabilityID{"cap-repo-summary"},
	})
	if len(selection.Servers) != 1 || len(selection.Capabilities) != 1 || selection.RegistryRevision != createdCapability.RegistryRevision {
		t.Fatalf("SelectionSnapshotResult = %+v", selection)
	}
}

func TestModulePostCannotMutateRegistry(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	registry := memory.NewMCPRegistryRepository()
	module := New()
	if err := module.Configure(mcpbiz.Dependencies{Registry: registry, EventHub: hub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	hub.Post(magicevent.NewEventWithContext(mcpevents.TopicMCPCommandCreateServer, "test", mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.CreateServerCommand{
		ServerID: "mcp-post", Name: "post", Transport: "stdio", Command: "mcp-post",
	}))
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		snapshot, err := registry.Load(ctx)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if len(snapshot.Servers) != 0 || len(snapshot.Capabilities) != 0 {
			t.Fatalf("Post mutated registry: %+v", snapshot)
		}
		time.Sleep(time.Millisecond)
	}
}

func mustMCPPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatalf("workorcheventpayload.FromMap: %v", err)
	}
	return payload
}

func mcpModuleContractDependencies() mcpbiz.Dependencies {
	return mcpbiz.Dependencies{Registry: memory.NewMCPRegistryRepository(), EventHub: magicevent.NewHub(16)}
}

func sendMCPCommand[Command any, Result any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) Result {
	t.Helper()
	var zero Result
	event := magicevent.NewEventWithContext(topic, "test", mcpevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := hub.Send(event)
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s error: %v", topic, result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("result %s: %v", topic, err)
	}
	typed, ok := data.(Result)
	if !ok {
		t.Fatalf("result %s type = %T", topic, data)
		return zero
	}
	return typed
}
