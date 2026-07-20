package mcp

import (
	"context"
	"errors"
	"testing"

	mcpservice "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/contracts"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestServiceCreateAndListServer(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMCPServerRepository(), backend.NewMCPCapabilityRepository())

	cmd := mcpservice.CreateServerCommand{
		ServerID:              common.MCPServerID("mcp-local-context"),
		Name:                  "local context",
		Transport:             "stdio",
		Endpoint:              "repo-context",
		Enabled:               true,
		AllowedTools:          []string{"repo.summary"},
		AllowedResources:      []string{"repo://summary"},
		AllowedPrompts:        []string{"delivery.plan"},
		DefaultApprovalPolicy: "manual_for_write",
		SessionPolicy:         "long_lived",
		TimeoutMillis:         30000,
	}
	if err := service.CreateServer(ctx, cmd); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}
	got, err := service.GetServer(ctx, cmd.ServerID)
	if err != nil {
		t.Fatalf("get mcp server: %v", err)
	}
	if got.Name != cmd.Name ||
		got.AllowedTools[0] != "repo.summary" ||
		got.AllowedPrompts[0] != "delivery.plan" ||
		got.SessionPolicy != "long_lived" {
		t.Fatalf("mcp server = %+v, want command values", got)
	}
	list, err := service.ListServers(ctx)
	if err != nil {
		t.Fatalf("list mcp servers: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("mcp servers len = %d, want 1", len(list))
	}
	capabilities, err := service.ListCapabilities(ctx, mcpservice.CapabilityQuery{ServerID: cmd.ServerID})
	if err != nil {
		t.Fatalf("list mcp capabilities: %v", err)
	}
	if len(capabilities) != 3 {
		t.Fatalf("mcp capabilities len = %d, want 3", len(capabilities))
	}
	if capabilities[0].DisplayName == "" || !capabilities[0].Enabled {
		t.Fatalf("default capability = %+v, want display name and enabled", capabilities[0])
	}
	var toolCapabilityDisplayName string
	for _, capability := range capabilities {
		if capability.Kind == mcpmodel.CapabilityKindTool && capability.Name == "repo.summary" {
			toolCapabilityDisplayName = capability.DisplayName
			break
		}
	}
	if toolCapabilityDisplayName != "工具 repo.summary" {
		t.Fatalf("tool capability display name = %q, want generic display name", toolCapabilityDisplayName)
	}
}

func TestServiceDerivesBuiltinAcceptanceCapabilityMetadata(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMCPServerRepository(), backend.NewMCPCapabilityRepository())

	serverID := common.MCPServerID("mcp-acceptance")
	if err := service.CreateServer(ctx, mcpservice.CreateServerCommand{
		ServerID:         serverID,
		Name:             "acceptance server",
		Transport:        "stdio",
		Enabled:          true,
		AllowedResources: []string{"workspace://acceptance"},
		AllowedPrompts:   []string{"workorch.acceptance"},
	}); err != nil {
		t.Fatalf("create server: %v", err)
	}

	resources, err := service.ListCapabilities(ctx, mcpservice.CapabilityQuery{ServerID: serverID, Kind: mcpmodel.CapabilityKindResource, Query: "验收资料"})
	if err != nil {
		t.Fatalf("list resource capabilities: %v", err)
	}
	if len(resources) != 1 || resources[0].DisplayName != "工作空间验收资料" || resources[0].URI != "workspace://acceptance" {
		t.Fatalf("resource capabilities = %+v, want acceptance resource metadata", resources)
	}

	prompts, err := service.ListCapabilities(ctx, mcpservice.CapabilityQuery{ServerID: serverID, Kind: mcpmodel.CapabilityKindPrompt, Query: "验收提示词"})
	if err != nil {
		t.Fatalf("list prompt capabilities: %v", err)
	}
	if len(prompts) != 1 || prompts[0].DisplayName != "验收提示词" {
		t.Fatalf("prompt capabilities = %+v, want acceptance prompt metadata", prompts)
	}
}

func TestServiceRejectsInvalidServer(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMCPServerRepository(), backend.NewMCPCapabilityRepository())

	err := service.CreateServer(ctx, mcpservice.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-invalid"),
		Name:          "invalid",
		Transport:     "stdio",
		AllowedTools:  []string{""},
		TimeoutMillis: -1,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create invalid mcp server err = %v, want invalid argument", err)
	}

	err = service.CreateServer(ctx, mcpservice.CreateServerCommand{
		ServerID:  common.MCPServerID("mcp-blank-name"),
		Name:      "   ",
		Transport: "stdio",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create blank mcp server name err = %v, want invalid argument", err)
	}

	err = service.CreateServer(ctx, mcpservice.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-invalid-session"),
		Name:          "invalid session",
		Transport:     "stdio",
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		SessionPolicy: "sticky",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create invalid mcp server session err = %v, want invalid argument", err)
	}
}

func TestServiceCreateAndFilterCapability(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMCPServerRepository(), backend.NewMCPCapabilityRepository())
	if err := service.CreateServer(ctx, mcpservice.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-capability"),
		Name:          "capability server",
		Transport:     "stdio",
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		SessionPolicy: "transient",
	}); err != nil {
		t.Fatalf("create server: %v", err)
	}
	created, err := service.CreateCapability(ctx, mcpservice.CreateCapabilityCommand{
		ServerID:         common.MCPServerID("mcp-capability"),
		Kind:             "resource",
		Name:             "workspace://acceptance",
		DisplayName:      "工作空间验收资料",
		Category:         "验收",
		StageHints:       []string{"validation", "acceptance"},
		RiskLevel:        "read_only",
		EffectScope:      "none",
		IdempotencyClass: "safe_to_replay",
		Enabled:          true,
		OutputSummary:    "返回 DoD、Result、Artifact 和证据引用。",
	})
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	if created.ID == "" || created.URI != "" || created.EffectScope != "none" || created.IdempotencyClass != "safe_to_replay" {
		t.Fatalf("created capability = %+v, want generated id, explicit runtime metadata, and uri only when provided", created)
	}
	list, err := service.ListCapabilities(ctx, mcpservice.CapabilityQuery{
		ServerID: common.MCPServerID("mcp-capability"),
		Stage:    "acceptance",
		Query:    "验收",
	})
	if err != nil {
		t.Fatalf("list capabilities: %v", err)
	}
	if len(list) != 1 || list[0].DisplayName != "工作空间验收资料" {
		t.Fatalf("filtered capabilities = %+v, want acceptance capability", list)
	}
}
