package mcp

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if ModuleID != "blocks/mcp-registry" {
		t.Fatalf("ModuleID = %q, want relocated stateful block ID", ModuleID)
	}
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(mcpModuleContractDependencies()); err != nil {
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
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
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
	hub := magicevent.NewHub(16)
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

	createResult := sendMCPCommand(t, ctx, hub, mcpevents.TopicMCPCommandCreateServer,
		mcpevents.CreateServerCommand{
			ServerID:              "mcp-server-contract",
			Name:                  "contract server",
			Transport:             "stdio",
			Command:               "mcp-server",
			Enabled:               true,
			AllowedTools:          []string{"contract_tool"},
			DefaultApprovalPolicy: "read_only",
		},
	)
	if createResult.Server == nil || createResult.Server.ID != "mcp-server-contract" || createResult.Server.Transport != "stdio" {
		t.Fatalf("server = %+v, want created server", createResult.Server)
	}

	listResult := sendMCPCommand(t, ctx, hub, mcpevents.TopicMCPCommandListServers, mcpevents.ListServersCommand{})
	if len(listResult.Servers) != 1 || listResult.Servers[0].ID != "mcp-server-contract" {
		t.Fatalf("servers = %+v, want created server", listResult.Servers)
	}
}

func TestModuleMCPCapabilityCommandsUseStructuredRegistryContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
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

	_ = sendMCPCommand(t, ctx, hub, mcpevents.TopicMCPCommandCreateServer,
		mcpevents.CreateServerCommand{
			ServerID:  "mcp-server-contract",
			Name:      "contract server",
			Transport: "stdio",
			Command:   "mcp-server",
			Enabled:   true,
		},
	)

	createCapabilityResult := sendMCPCommand(t, ctx, hub, mcpevents.TopicMCPCommandCreateCapability,
		mcpevents.CreateCapabilityCommand{
			Capability: mcpmodel.Capability{
				ID:               "mcp-server-contract:tool:repo.summary",
				ServerID:         "mcp-server-contract",
				Kind:             "tool",
				Name:             "repo.summary",
				DisplayName:      "Repository Summary",
				Description:      "Structured repository summary capability",
				Category:         "repository",
				Tags:             []string{"read", "summary"},
				UsageHint:        "Use for bounded repository summary.",
				InputSummary:     "path/ref",
				OutputSummary:    "bounded summary",
				StageHints:       []string{"planning", "validation"},
				RiskLevel:        "read_only",
				EffectScope:      "none",
				IdempotencyClass: "safe_to_replay",
				Schema:           mustMCPPayload(t, map[string]any{"type": "object", "required": []string{"path"}}),
				Enabled:          true,
				ApprovalPolicy:   "read_only",
				Status:           "active",
			},
		},
	)
	capability := createCapabilityResult.Capability
	if capability == nil || capability.ID != "mcp-server-contract:tool:repo.summary" {
		t.Fatalf("capability = %+v, want created capability", capability)
	}
	schemaType, _ := capability.Schema.String("type")
	if capability.Kind != "tool" ||
		capability.Category != "repository" ||
		capability.RiskLevel != "read_only" ||
		capability.EffectScope != "none" ||
		capability.IdempotencyClass != "safe_to_replay" ||
		capability.ApprovalPolicy != "read_only" ||
		len(capability.StageHints) != 2 ||
		schemaType != "object" {
		t.Fatalf("capability = %+v, want structured policy/capability fields preserved", capability)
	}

	listCapabilityResult := sendMCPCommand(t, ctx, hub, mcpevents.TopicMCPCommandListCapabilities,
		mcpevents.ListCapabilitiesCommand{
			Query: mcpevents.CapabilityQuery{
				ServerID:  "mcp-server-contract",
				Kind:      "tool",
				Category:  "repository",
				RiskLevel: "read_only",
			},
		},
	)
	if len(listCapabilityResult.Capabilities) != 1 || listCapabilityResult.Capabilities[0].ID != capability.ID {
		t.Fatalf("capabilities = %+v, want structured filtered capability", listCapabilityResult.Capabilities)
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

func mcpModuleContractDependencies() Dependencies {
	return Dependencies{
		Servers:      memory.NewMCPServerRepository(),
		Capabilities: memory.NewMCPCapabilityRepository(),
		EventHub:     magicevent.NewHub(16),
	}
}

func sendMCPCommand[Command any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) mcpevents.MCPCommandResult {
	t.Helper()
	ev := magicevent.NewEventWithContext(topic, "test", mcpevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := hub.Send(ev)
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s error: %v", topic, result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("result %s: %v", topic, err)
	}
	typed, ok := data.(mcpevents.MCPCommandResult)
	if !ok {
		t.Fatalf("result %s type = %T", topic, data)
	}
	return typed
}
