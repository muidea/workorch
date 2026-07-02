package mcp

import (
	"context"
	"errors"
	"testing"

	mcpcontracts "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/contracts"
	mcpevents "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.Service() != nil {
		t.Fatal("new module service is non-nil before Configure")
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(mcpModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.Service() == nil {
		t.Fatal("service is nil after Configure")
	}
	if module.bizPtr == nil || module.servicePtr == nil || module.eventHub == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.unsub)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe mcp commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.unsub) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.unsub), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.Service() != nil ||
		module.bizPtr != nil ||
		module.servicePtr != nil ||
		module.eventHub != nil ||
		len(module.unsub) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresCoreDependencies(t *testing.T) {
	base := mcpModuleContractDependencies()
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing servers", deps: func() Dependencies { d := base; d.Servers = nil; return d }()},
		{name: "missing capabilities", deps: func() Dependencies { d := base; d.Capabilities = nil; return d }()},
		{name: "missing event hub", deps: func() Dependencies { d := base; d.EventHub = nil; return d }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

func TestModuleMCPServerCommandsUseRegistryContract(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	module := New()
	if err := module.Configure(Dependencies{
		Servers:      memory.NewMCPServerRepository(),
		Capabilities: memory.NewMCPCapabilityRepository(),
		EventHub:     hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	err := hub.Publish(ctx, events.Envelope{
		Topic:       mcpevents.TopicMCPCommandCreateServer,
		SubjectType: "mcp_server",
		SubjectID:   "mcp-server-contract",
		Payload: map[string]any{
			events.PayloadKeyCommand: mcpcontracts.CreateServerCommand{
				ServerID:              "mcp-server-contract",
				Name:                  "contract server",
				Transport:             "stdio",
				Command:               "mcp-server",
				Enabled:               true,
				AllowedTools:          []string{"contract_tool"},
				DefaultApprovalPolicy: "read_only",
			},
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("create server err = %v, want response", err)
	}
	server, ok := response.Payload[events.PayloadKeyServer].(*mcpmodel.Server)
	if !ok || server == nil || server.ID != "mcp-server-contract" || server.Transport != "stdio" {
		t.Fatalf("server = %+v, want created server", response.Payload[events.PayloadKeyServer])
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic: mcpevents.TopicMCPCommandListServers,
	})
	response, ok = events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("list servers err = %v, want response", err)
	}
	servers, ok := response.Payload[events.PayloadKeyServers].([]*mcpmodel.Server)
	if !ok || len(servers) != 1 || servers[0].ID != "mcp-server-contract" {
		t.Fatalf("servers = %+v, want created server", response.Payload[events.PayloadKeyServers])
	}
}

func mcpModuleContractDependencies() Dependencies {
	return Dependencies{
		Servers:      memory.NewMCPServerRepository(),
		Capabilities: memory.NewMCPCapabilityRepository(),
		EventHub:     events.NewMemoryHub(),
	}
}
