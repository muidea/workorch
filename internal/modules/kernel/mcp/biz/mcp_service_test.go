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
	service := NewService(backend.NewMCPServerRepository())

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
}

func TestServiceRejectsInvalidServer(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMCPServerRepository())

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
