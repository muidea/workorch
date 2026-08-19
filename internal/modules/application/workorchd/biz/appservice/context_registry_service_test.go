package appservice

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestUpdateMCPServerPreservesEnvWhenEnvOmitted(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := CreateMCPServerWithBackend(ctx, "file", dataRoot, workorchmanagementcontract.MCPServerMutation{
		ID:              "mcp-env-preserve",
		Name:            "env preserve",
		Transport:       "stdio",
		Command:         "tool",
		EnvironmentRefs: map[string]string{"SECRET": "env://MCP_SECRET"},
		Enabled:         true,
		AllowedTools:    []string{"lookup"},
	}); err != nil {
		t.Fatalf("CreateMCPServerWithBackend: %v", err)
	}
	if _, err := UpdateMCPServerWithBackend(ctx, "file", dataRoot, "mcp-env-preserve", workorchmanagementcontract.MCPServerMutation{
		ID:           "mcp-env-preserve",
		Name:         "env preserve updated",
		Transport:    "stdio",
		Command:      "tool",
		Enabled:      true,
		AllowedTools: []string{"lookup"},
	}); err != nil {
		t.Fatalf("UpdateMCPServerWithBackend: %v", err)
	}
	session, err := newRuntimeSessionWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("newRuntimeSessionWithBackend: %v", err)
	}
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	item, err := exports.Repositories.MCPServers.Get(ctx, "mcp-env-preserve")
	if err != nil {
		t.Fatalf("get server: %v", err)
	}
	if item.Env["SECRET"] != "env://MCP_SECRET" {
		t.Fatalf("environment refs = %+v, want preserved reference", item.Env)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close session: %v", err)
	}
}

func TestUpdateMCPServerAllowsExplicitEmptyEnvClear(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := CreateMCPServerWithBackend(ctx, "file", dataRoot, workorchmanagementcontract.MCPServerMutation{
		ID:              "mcp-env-clear",
		Name:            "env clear",
		Transport:       "stdio",
		Command:         "tool",
		EnvironmentRefs: map[string]string{"SECRET": "env://MCP_SECRET"},
		Enabled:         true,
		AllowedTools:    []string{"lookup"},
	}); err != nil {
		t.Fatalf("CreateMCPServerWithBackend: %v", err)
	}
	if _, err := UpdateMCPServerWithBackend(ctx, "file", dataRoot, "mcp-env-clear", workorchmanagementcontract.MCPServerMutation{
		ID:              "mcp-env-clear",
		Name:            "env clear updated",
		Transport:       "stdio",
		Command:         "tool",
		EnvironmentRefs: map[string]string{},
		Enabled:         true,
		AllowedTools:    []string{"lookup"},
	}); err != nil {
		t.Fatalf("UpdateMCPServerWithBackend: %v", err)
	}
	session, err := newRuntimeSessionWithBackend(ctx, "file", dataRoot)
	if err != nil {
		t.Fatalf("newRuntimeSessionWithBackend: %v", err)
	}
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	item, err := exports.Repositories.MCPServers.Get(ctx, "mcp-env-clear")
	if err != nil {
		t.Fatalf("get server: %v", err)
	}
	if len(item.Env) != 0 {
		t.Fatalf("env = %+v, want explicit clear", item.Env)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close session: %v", err)
	}
}
