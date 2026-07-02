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

func TestModuleMCPCapabilityCommandsUseStructuredRegistryContract(t *testing.T) {
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

	if err := hub.Publish(ctx, events.Envelope{
		Topic:       mcpevents.TopicMCPCommandCreateServer,
		SubjectType: "mcp_server",
		SubjectID:   "mcp-server-contract",
		Payload: map[string]any{
			events.PayloadKeyCommand: mcpcontracts.CreateServerCommand{
				ServerID:  "mcp-server-contract",
				Name:      "contract server",
				Transport: "stdio",
				Command:   "mcp-server",
				Enabled:   true,
			},
		},
	}); err == nil {
		t.Fatalf("create server err = nil, want response envelope")
	} else if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("create server err = %v, want response", err)
	}

	err := hub.Publish(ctx, events.Envelope{
		Topic:       mcpevents.TopicMCPCommandCreateCapability,
		SubjectType: "mcp_capability",
		SubjectID:   "mcp-server-contract:tool:repo.summary",
		Payload: map[string]any{
			events.PayloadKeyCommand: mcpcontracts.CreateCapabilityCommand{
				ID:             "mcp-server-contract:tool:repo.summary",
				ServerID:       "mcp-server-contract",
				Kind:           "tool",
				Name:           "repo.summary",
				DisplayName:    "Repository Summary",
				Description:    "Structured repository summary capability",
				Category:       "repository",
				Tags:           []string{"read", "summary"},
				UsageHint:      "Use for bounded repository summary.",
				InputSummary:   "path/ref",
				OutputSummary:  "bounded summary",
				StageHints:     []string{"planning", "validation"},
				RiskLevel:      "read_only",
				Schema:         map[string]any{"type": "object", "required": []string{"path"}},
				Enabled:        true,
				ApprovalPolicy: "read_only",
				Status:         "active",
			},
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("create capability err = %v, want response", err)
	}
	capability, ok := response.Payload[events.PayloadKeyCapability].(*mcpmodel.Capability)
	if !ok || capability == nil || capability.ID != "mcp-server-contract:tool:repo.summary" {
		t.Fatalf("capability = %+v, want created capability", response.Payload[events.PayloadKeyCapability])
	}
	if capability.Kind != "tool" ||
		capability.Category != "repository" ||
		capability.RiskLevel != "read_only" ||
		capability.ApprovalPolicy != "read_only" ||
		len(capability.StageHints) != 2 ||
		capability.Schema["type"] != "object" {
		t.Fatalf("capability = %+v, want structured policy/capability fields preserved", capability)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic: mcpevents.TopicMCPCommandListCapabilities,
		Payload: map[string]any{
			events.PayloadKeyCapabilityQuery: mcpcontracts.CapabilityQuery{
				ServerID:  "mcp-server-contract",
				Kind:      "tool",
				Category:  "repository",
				RiskLevel: "read_only",
			},
		},
	})
	response, ok = events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("list capabilities err = %v, want response", err)
	}
	capabilities, ok := response.Payload[events.PayloadKeyCapabilities].([]*mcpmodel.Capability)
	if !ok || len(capabilities) != 1 || capabilities[0].ID != capability.ID {
		t.Fatalf("capabilities = %+v, want structured filtered capability", response.Payload[events.PayloadKeyCapabilities])
	}
}

func mcpModuleContractDependencies() Dependencies {
	return Dependencies{
		Servers:      memory.NewMCPServerRepository(),
		Capabilities: memory.NewMCPCapabilityRepository(),
		EventHub:     events.NewMemoryHub(),
	}
}
