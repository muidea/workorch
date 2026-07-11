package appservice

import (
	"context"
	"testing"
)

func TestUpdateMCPServerPreservesEnvWhenEnvOmitted(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := CreateMCPServerWithBackend(ctx, "file", dataRoot, CreateMCPServerCommand{
		ID:           "mcp-env-preserve",
		Name:         "env preserve",
		Transport:    "stdio",
		Command:      "tool",
		Env:          map[string]string{"SECRET": "value"},
		Enabled:      true,
		AllowedTools: []string{"lookup"},
	}); err != nil {
		t.Fatalf("CreateMCPServerWithBackend: %v", err)
	}
	if _, err := UpdateMCPServerWithBackend(ctx, "file", dataRoot, "mcp-env-preserve", UpdateMCPServerCommand{
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
	if item.Env["SECRET"] != "value" {
		t.Fatalf("env = %+v, want preserved secret", item.Env)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("close session: %v", err)
	}
}

func TestUpdateMCPServerAllowsExplicitEmptyEnvClear(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	if _, err := CreateMCPServerWithBackend(ctx, "file", dataRoot, CreateMCPServerCommand{
		ID:           "mcp-env-clear",
		Name:         "env clear",
		Transport:    "stdio",
		Command:      "tool",
		Env:          map[string]string{"SECRET": "value"},
		Enabled:      true,
		AllowedTools: []string{"lookup"},
	}); err != nil {
		t.Fatalf("CreateMCPServerWithBackend: %v", err)
	}
	if _, err := UpdateMCPServerWithBackend(ctx, "file", dataRoot, "mcp-env-clear", UpdateMCPServerCommand{
		ID:           "mcp-env-clear",
		Name:         "env clear updated",
		Transport:    "stdio",
		Command:      "tool",
		Env:          map[string]string{},
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
