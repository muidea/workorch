package mcp

import (
	"context"
	"errors"
	"testing"

	mcpservice "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
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
		ServerID:      common.MCPServerID("mcp-capability"),
		Kind:          "resource",
		Name:          "workspace://acceptance",
		DisplayName:   "工作空间验收资料",
		Category:      "验收",
		StageHints:    []string{"validation", "acceptance"},
		RiskLevel:     "read_only",
		Enabled:       true,
		OutputSummary: "返回 DoD、Result、Artifact 和证据引用。",
	})
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	if created.ID == "" || created.URI != "" {
		t.Fatalf("created capability = %+v, want generated id and explicit uri only when provided", created)
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
