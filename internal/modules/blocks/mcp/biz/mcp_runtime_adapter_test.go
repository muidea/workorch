package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kernelmcp "github.com/muidea/workorch/internal/modules/kernel/mcp"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAdapterExecutesAllowedHTTPToolCall(t *testing.T) {
	ctx := context.Background()
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"summary": "repo summary ready",
			"ok":      true,
		})
	}))
	defer server.Close()

	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-local-context"),
		Name:          "local context",
		Transport:     "http",
		Endpoint:      server.URL,
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		WorkspaceID: common.WorkspaceID("ws-mcp"),
		RunID:       common.RunID("run-mcp"),
		StepID:      common.StepID("run-mcp/mcp-1"),
		Kind:        "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-local-context",
			"tool":     "repo.summary",
			"input":    map[string]any{"path": "."},
			"summary":  "summarize repository",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	if result.Output["status"] != "completed" || result.Output["stage"] != "mcp_tool_call_completed" {
		t.Fatalf("output = %+v, want completed mcp output", result.Output)
	}
	if captured["workspaceId"] != "ws-mcp" || captured["tool"] != "repo.summary" {
		t.Fatalf("captured = %+v, want workorch trace refs", captured)
	}
	resultPayload, ok := result.Output["result"].(map[string]any)
	if !ok || resultPayload["summary"] != "repo summary ready" {
		t.Fatalf("result payload = %#v, want decoded response", result.Output["result"])
	}
}

func TestRuntimeAdapterExecutesHTTPResourceReadAndPromptGet(t *testing.T) {
	ctx := context.Background()
	var captured []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		captured = append(captured, payload)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"operation": payload["operation"],
			"ok":        true,
		})
	}))
	defer server.Close()

	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:         common.MCPServerID("mcp-http-context"),
		Name:             "http context",
		Transport:        "http",
		Endpoint:         server.URL,
		Enabled:          true,
		AllowedResources: []string{"file://README.md"},
		AllowedPrompts:   []string{"delivery.plan"},
		TimeoutMillis:    1000,
	})
	adapter := NewRuntimeAdapter(service)

	resourceResult, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		Kind: "mcp/resource_read",
		Input: map[string]any{
			"serverId": "mcp-http-context",
			"uri":      "file://README.md",
		},
	})
	if err != nil {
		t.Fatalf("resource ExecuteRuntimeStep: %v", err)
	}
	if resourceResult.Output["stage"] != "mcp_resource_read_completed" || resourceResult.Output["uri"] != "file://README.md" {
		t.Fatalf("resource output = %+v, want completed resource read", resourceResult.Output)
	}

	promptResult, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		Kind: "mcp/prompt_get",
		Input: map[string]any{
			"serverId":  "mcp-http-context",
			"prompt":    "delivery.plan",
			"arguments": map[string]any{"goal": "ship"},
		},
	})
	if err != nil {
		t.Fatalf("prompt ExecuteRuntimeStep: %v", err)
	}
	if promptResult.Output["stage"] != "mcp_prompt_get_completed" || promptResult.Output["prompt"] != "delivery.plan" {
		t.Fatalf("prompt output = %+v, want completed prompt get", promptResult.Output)
	}
	if len(captured) != 2 || captured[0]["operation"] != "mcp/resource_read" || captured[1]["operation"] != "mcp/prompt_get" {
		t.Fatalf("captured = %+v, want resource then prompt operations", captured)
	}
}

func TestRuntimeAdapterCollectsHTTPStreamEvents(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"delta\":\"planning\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"summary\":\"done\"}\n\n"))
	}))
	defer server.Close()

	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-http-stream"),
		Name:          "http stream",
		Transport:     "http",
		Endpoint:      server.URL,
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)
	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		Kind: "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-http-stream",
			"tool":     "repo.summary",
			"stream":   true,
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v", err)
	}
	events, ok := result.Output["streamEvents"].([]any)
	if !ok || len(events) != 2 {
		t.Fatalf("streamEvents = %#v, want two HTTP stream events", result.Output["streamEvents"])
	}
}

func TestRuntimeAdapterFailsHTTPToolResultError(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "missing argument path"},
			},
			"isError": true,
		})
	}))
	defer server.Close()

	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-http-tool-error"),
		Name:          "http tool error",
		Transport:     "http",
		Endpoint:      server.URL,
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)
	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		Kind: "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-http-tool-error",
			"tool":     "repo.summary",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "missing argument path") {
		t.Fatalf("ExecuteRuntimeStep err = %v, want MCP tool result error", err)
	}
	if result.Output["status"] != "failed" || result.Output["stage"] != "mcp_tool_call_failed" {
		t.Fatalf("output = %+v, want failed tool call", result.Output)
	}
	diagnostics := result.Output["runtimeDiagnostics"].(map[string]any)
	if _, ok := diagnostics["targetRef"]; ok {
		t.Fatalf("runtimeDiagnostics = %+v, did not expect target without input target", diagnostics)
	}
}

func TestRuntimeAdapterFailureDiagnosticsIncludeToolInputTarget(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "column is beyond end of line"}},
			"isError": true,
		})
	}))
	defer server.Close()

	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-http-tool-target"),
		Name:          "http tool target",
		Transport:     "http",
		Endpoint:      server.URL,
		Enabled:       true,
		AllowedTools:  []string{"lookup.references"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)
	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		Kind: "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-http-tool-target",
			"tool":     "lookup.references",
			"input": map[string]any{
				"path": "src/symbol.go",
				"line": 1,
				"col":  13,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "column is beyond end of line") {
		t.Fatalf("ExecuteRuntimeStep err = %v, want MCP tool result error", err)
	}
	diagnostics := result.Output["runtimeDiagnostics"].(map[string]any)
	if diagnostics["targetRef"] != "src/symbol.go" {
		t.Fatalf("runtimeDiagnostics = %+v, want tool input target", diagnostics)
	}
	targets := diagnostics["failureTargets"].([]string)
	if len(targets) != 1 || targets[0] != "src/symbol.go" {
		t.Fatalf("failureTargets = %#v, want tool input target", diagnostics["failureTargets"])
	}
}

func TestRuntimeAdapterRejectsDisallowedTool(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:     common.MCPServerID("mcp-policy"),
		Name:         "policy",
		Transport:    "http",
		Endpoint:     "http://127.0.0.1:1",
		Enabled:      true,
		AllowedTools: []string{"repo.summary"},
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		Input: map[string]any{
			"serverId": "mcp-policy",
			"tool":     "repo.write",
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ExecuteRuntimeStep err = %v, want invalid argument", err)
	}
	if result.Output["stage"] != "mcp_tool_call_rejected" {
		t.Fatalf("output = %+v, want rejected stage", result.Output)
	}
}

func TestRuntimeAdapterExecutesAllowedStdioToolCall(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-stdio"),
		Name:          "stdio",
		Transport:     "stdio",
		Endpoint:      "env WORKORCH_MCP_STDIO_HELPER=1 " + os.Args[0] + " -test.run=TestMCPStdioHelperProcess",
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		WorkspaceID: common.WorkspaceID("ws-mcp"),
		RunID:       common.RunID("run-mcp"),
		StepID:      common.StepID("run-mcp/mcp-stdio-1"),
		Kind:        "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-stdio",
			"tool":     "repo.summary",
			"input":    map[string]any{"path": "."},
			"summary":  "summarize repository through stdio",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v output=%+v", err, result.Output)
	}
	if result.Output["status"] != "completed" || result.Output["stage"] != "mcp_tool_call_completed" {
		t.Fatalf("output = %+v, want completed mcp output", result.Output)
	}
	if result.Output["transport"] != "stdio" {
		t.Fatalf("transport = %v, want stdio", result.Output["transport"])
	}
	events, ok := result.Output["streamEvents"].([]any)
	if !ok || len(events) == 0 {
		t.Fatalf("streamEvents = %#v, want stdio notification evidence", result.Output["streamEvents"])
	}
	resultPayload, ok := result.Output["result"].(map[string]any)
	if !ok {
		t.Fatalf("result payload = %#v, want decoded response", result.Output["result"])
	}
	structured, ok := resultPayload["structuredContent"].(map[string]any)
	if !ok || structured["tool"] != "repo.summary" {
		t.Fatalf("structured content = %#v, want tool result", resultPayload["structuredContent"])
	}
}

func TestRuntimeAdapterExecutesStdioCommandArgsEnvWhenEndpointMissing(t *testing.T) {
	ctx := context.Background()
	internalDir := filepath.Join(t.TempDir(), "internal")
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-stdio-command"),
		Name:          "stdio command",
		Transport:     "stdio",
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestMCPStdioHelperProcess"},
		Env:           map[string]string{"WORKORCH_MCP_STDIO_HELPER": "1", "WORKORCH_MCP_STDIO_ENV_ECHO": "{workspaceInternalDirectory}/config.json"},
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		WorkspaceID:                common.WorkspaceID("ws-mcp-command"),
		WorkspaceInternalDirectory: internalDir,
		RunID:                      common.RunID("run-mcp-command"),
		StepID:                     common.StepID("run-mcp-command/mcp-stdio-1"),
		Kind:                       "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-stdio-command",
			"tool":     "repo.summary",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v output=%+v", err, result.Output)
	}
	resultPayload, ok := result.Output["result"].(map[string]any)
	if !ok {
		t.Fatalf("result payload = %#v, want decoded response", result.Output["result"])
	}
	structured, ok := resultPayload["structuredContent"].(map[string]any)
	if !ok || structured["envEcho"] != filepath.Join(internalDir, "config.json") {
		t.Fatalf("structured content = %#v, want env echo from command env", resultPayload["structuredContent"])
	}
}

func TestRuntimeAdapterExpandsManagedConfigPlaceholderWithWorkspaceOverride(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	internalDir := filepath.Join(dataRoot, "internal", "workspaces", "ws-mcp-managed-config")
	workspaceConfigPath := filepath.Join(internalDir, "lsp-bridge", "mcp-config.json")
	if err := os.MkdirAll(filepath.Dir(workspaceConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir workspace config: %v", err)
	}
	if err := os.WriteFile(workspaceConfigPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-managed-config"),
		Name:          "managed config",
		Transport:     "stdio",
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestMCPStdioHelperProcess"},
		Env:           map[string]string{"WORKORCH_MCP_STDIO_HELPER": "1", "WORKORCH_MCP_STDIO_ENV_ECHO": "{workspaceManagedConfig:lsp-bridge/mcp-config.json}"},
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		WorkspaceID:                common.WorkspaceID("ws-mcp-managed-config"),
		WorkspaceInternalDirectory: internalDir,
		RunID:                      common.RunID("run-mcp-managed-config"),
		StepID:                     common.StepID("run-mcp-managed-config/mcp-stdio-1"),
		Kind:                       "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-managed-config",
			"tool":     "repo.summary",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v output=%+v", err, result.Output)
	}
	resultPayload, ok := result.Output["result"].(map[string]any)
	if !ok {
		t.Fatalf("result payload = %#v, want decoded response", result.Output["result"])
	}
	structured, ok := resultPayload["structuredContent"].(map[string]any)
	if !ok || structured["envEcho"] != workspaceConfigPath {
		t.Fatalf("structured content = %#v, want workspace managed config env", resultPayload["structuredContent"])
	}
}

func TestRuntimeAdapterExpandsManagedConfigPlaceholderWithSystemDefault(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	internalDir := filepath.Join(dataRoot, "internal", "workspaces", "ws-mcp-system-config")
	systemConfigPath := filepath.Join(dataRoot, "system", "managed-configs", "lsp-bridge", "mcp-config.json")
	if err := os.MkdirAll(filepath.Dir(systemConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir system config: %v", err)
	}
	if err := os.WriteFile(systemConfigPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write system config: %v", err)
	}
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-system-config"),
		Name:          "system config",
		Transport:     "stdio",
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestMCPStdioHelperProcess"},
		Env:           map[string]string{"WORKORCH_MCP_STDIO_HELPER": "1", "WORKORCH_MCP_STDIO_ENV_ECHO": "{workspaceManagedConfig:lsp-bridge/mcp-config.json}"},
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		WorkspaceID:                common.WorkspaceID("ws-mcp-system-config"),
		WorkspaceInternalDirectory: internalDir,
		RunID:                      common.RunID("run-mcp-system-config"),
		StepID:                     common.StepID("run-mcp-system-config/mcp-stdio-1"),
		Kind:                       "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-system-config",
			"tool":     "repo.summary",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v output=%+v", err, result.Output)
	}
	resultPayload, ok := result.Output["result"].(map[string]any)
	if !ok {
		t.Fatalf("result payload = %#v, want decoded response", result.Output["result"])
	}
	structured, ok := resultPayload["structuredContent"].(map[string]any)
	if !ok || structured["envEcho"] != systemConfigPath {
		t.Fatalf("structured content = %#v, want system managed config env", resultPayload["structuredContent"])
	}
}

func TestRuntimeAdapterStartsStdioInWorkspaceDeliverableDirectory(t *testing.T) {
	ctx := context.Background()
	deliverableDir := t.TempDir()
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-stdio-workdir"),
		Name:          "stdio workdir",
		Transport:     "stdio",
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestMCPStdioHelperProcess"},
		Env:           map[string]string{"WORKORCH_MCP_STDIO_HELPER": "1"},
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		WorkspaceID:                   common.WorkspaceID("ws-mcp-workdir"),
		WorkspaceDeliverableDirectory: deliverableDir,
		Kind:                          "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-stdio-workdir",
			"tool":     "repo.summary",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v output=%+v", err, result.Output)
	}
	resultPayload, ok := result.Output["result"].(map[string]any)
	if !ok {
		t.Fatalf("result payload = %#v, want decoded response", result.Output["result"])
	}
	structured, ok := resultPayload["structuredContent"].(map[string]any)
	if !ok || structured["cwd"] != deliverableDir {
		t.Fatalf("structured content = %#v, want cwd %q", resultPayload["structuredContent"], deliverableDir)
	}
}

func TestRuntimeAdapterResolvesRelativeStdioEndpointBeforeWorkspaceCWD(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	deliverableDir := filepath.Join(root, "workspace")
	endpointDir := filepath.Join(root, "system", "mcp")
	if err := os.MkdirAll(deliverableDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(endpointDir, 0o755); err != nil {
		t.Fatal(err)
	}
	helperName := "mcp_helper.sh"
	helperPath := filepath.Join(endpointDir, helperName)
	if err := os.WriteFile(helperPath, []byte("#!/bin/sh\nWORKORCH_MCP_STDIO_HELPER=1 exec "+os.Args[0]+" -test.run=TestMCPStdioHelperProcess\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(endpointDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previousWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-stdio-relative-endpoint"),
		Name:          "stdio relative endpoint",
		Transport:     "stdio",
		Endpoint:      "." + string(os.PathSeparator) + helperName,
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		WorkspaceID:                   common.WorkspaceID("ws-mcp-relative-endpoint"),
		WorkspaceDeliverableDirectory: deliverableDir,
		Kind:                          "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-stdio-relative-endpoint",
			"tool":     "repo.summary",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v output=%+v", err, result.Output)
	}
	if result.Output["transport"] != "stdio" || result.Output["status"] != "completed" {
		t.Fatalf("output = %+v, want completed stdio call from relative endpoint", result.Output)
	}
}

func TestRuntimeAdapterPrefersEndpointOverStdioCommandArgsEnv(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-stdio-endpoint-priority"),
		Name:          "stdio endpoint priority",
		Transport:     "stdio",
		Endpoint:      "env WORKORCH_MCP_STDIO_HELPER=1 WORKORCH_MCP_STDIO_ENV_ECHO=from-endpoint " + os.Args[0] + " -test.run=TestMCPStdioHelperProcess",
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestMCPStdioHelperProcess"},
		Env:           map[string]string{"WORKORCH_MCP_STDIO_HELPER": "1", "WORKORCH_MCP_STDIO_ENV_ECHO": "from-command"},
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		Kind: "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-stdio-endpoint-priority",
			"tool":     "repo.summary",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v output=%+v", err, result.Output)
	}
	resultPayload, _ := result.Output["result"].(map[string]any)
	structured, _ := resultPayload["structuredContent"].(map[string]any)
	if structured["envEcho"] != "from-endpoint" {
		t.Fatalf("structured content = %#v, want endpoint env to take priority", structured)
	}
}

func TestRuntimeAdapterFailsStdioToolResultError(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-stdio-tool-error"),
		Name:          "stdio tool error",
		Transport:     "stdio",
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestMCPStdioHelperProcess"},
		Env:           map[string]string{"WORKORCH_MCP_STDIO_HELPER": "1", "WORKORCH_MCP_STDIO_TOOL_ERROR": "1"},
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		Kind: "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-stdio-tool-error",
			"tool":     "repo.summary",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "stdio tool failed") {
		t.Fatalf("ExecuteRuntimeStep err = %v, want stdio MCP tool result error", err)
	}
	if result.Output["status"] != "failed" || result.Output["stage"] != "mcp_tool_call_failed" {
		t.Fatalf("output = %+v, want failed tool call", result.Output)
	}
}

func TestRuntimeAdapterRejectsStdioWithoutEndpointOrCommand(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-stdio-missing-command"),
		Name:          "stdio missing command",
		Transport:     "stdio",
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		Kind: "mcp/tool_call",
		Input: map[string]any{
			"serverId": "mcp-stdio-missing-command",
			"tool":     "repo.summary",
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "endpoint or command") {
		t.Fatalf("ExecuteRuntimeStep err = %v, want explicit missing endpoint or command", err)
	}
	if result.Output["stage"] != "mcp_tool_call_failed" {
		t.Fatalf("output = %+v, want failed stage", result.Output)
	}
}

func TestRuntimeAdapterExecutesAllowedStdioResourceRead(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:         common.MCPServerID("mcp-stdio-resource"),
		Name:             "stdio resource",
		Transport:        "stdio",
		Endpoint:         "env WORKORCH_MCP_STDIO_HELPER=1 " + os.Args[0] + " -test.run=TestMCPStdioHelperProcess",
		Enabled:          true,
		AllowedResources: []string{"file://README.md"},
		TimeoutMillis:    1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		Kind: "mcp/resource_read",
		Input: map[string]any{
			"serverId": "mcp-stdio-resource",
			"uri":      "file://README.md",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeStep: %v output=%+v", err, result.Output)
	}
	if result.Output["stage"] != "mcp_resource_read_completed" {
		t.Fatalf("output = %+v, want completed resource read", result.Output)
	}
}

func TestRuntimeAdapterReusesLongLivedStdioSession(t *testing.T) {
	ctx := context.Background()
	initFile := filepath.Join(t.TempDir(), "mcp-init-count.txt")
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-stdio-session"),
		Name:          "stdio session",
		Transport:     "stdio",
		Endpoint:      "env WORKORCH_MCP_STDIO_HELPER=1 WORKORCH_MCP_STDIO_INIT_FILE=" + initFile + " " + os.Args[0] + " -test.run=TestMCPStdioHelperProcess",
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		SessionPolicy: "long_lived",
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)
	defer adapter.Close()

	for idx := 0; idx < 2; idx++ {
		result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
			Kind: "mcp/tool_call",
			Input: map[string]any{
				"serverId": "mcp-stdio-session",
				"tool":     "repo.summary",
			},
		})
		if err != nil {
			t.Fatalf("ExecuteRuntimeStep #%d: %v output=%+v", idx+1, err, result.Output)
		}
	}
	raw, err := os.ReadFile(initFile)
	if err != nil {
		t.Fatalf("read init file: %v", err)
	}
	if got := strings.Count(string(raw), "initialized\n"); got != 1 {
		t.Fatalf("initialize count = %d, want one long-lived process initialization; raw=%q", got, string(raw))
	}
}

func TestRuntimeAdapterEvictsIdleLongLivedStdioSession(t *testing.T) {
	oldIdleTimeout := longLivedStdioSessionIdleTimeout
	longLivedStdioSessionIdleTimeout = 20 * time.Millisecond
	defer func() { longLivedStdioSessionIdleTimeout = oldIdleTimeout }()

	ctx := context.Background()
	initFile := filepath.Join(t.TempDir(), "mcp-init-count.txt")
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-stdio-idle-session"),
		Name:          "stdio idle session",
		Transport:     "stdio",
		Endpoint:      "env WORKORCH_MCP_STDIO_HELPER=1 WORKORCH_MCP_STDIO_INIT_FILE=" + initFile + " " + os.Args[0] + " -test.run=TestMCPStdioHelperProcess",
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		SessionPolicy: "long_lived",
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)
	defer adapter.Close()

	for idx := 0; idx < 2; idx++ {
		result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
			WorkspaceID: common.WorkspaceID("ws-idle"),
			Kind:        "mcp/tool_call",
			Input: map[string]any{
				"serverId": "mcp-stdio-idle-session",
				"tool":     "repo.summary",
			},
		})
		if err != nil {
			t.Fatalf("ExecuteRuntimeStep #%d: %v output=%+v", idx+1, err, result.Output)
		}
		if idx == 0 {
			waitForNoStdioSessions(t, adapter, time.Second)
		}
	}
	raw, err := os.ReadFile(initFile)
	if err != nil {
		t.Fatalf("read init file: %v", err)
	}
	if got := strings.Count(string(raw), "initialized\n"); got != 2 {
		t.Fatalf("initialize count = %d, want session reinitialized after idle eviction; raw=%q", got, string(raw))
	}
}

func waitForNoStdioSessions(t *testing.T, adapter *RuntimeAdapter, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		adapter.sessionMu.Lock()
		count := len(adapter.sessions)
		adapter.sessionMu.Unlock()
		if count == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	adapter.sessionMu.Lock()
	count := len(adapter.sessions)
	adapter.sessionMu.Unlock()
	t.Fatalf("stdio session count = %d after %s, want 0", count, timeout)
}

func TestRuntimeAdapterLongLivedStdioSessionIsWorkspaceScoped(t *testing.T) {
	ctx := context.Background()
	initFile := filepath.Join(t.TempDir(), "mcp-init-count.txt")
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-stdio-workspace-session"),
		Name:          "stdio workspace session",
		Transport:     "stdio",
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestMCPStdioHelperProcess"},
		Env:           map[string]string{"WORKORCH_MCP_STDIO_HELPER": "1", "WORKORCH_MCP_STDIO_INIT_FILE": initFile},
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		SessionPolicy: "long_lived",
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)
	defer adapter.Close()

	for _, workspaceID := range []common.WorkspaceID{"ws-one", "ws-one", "ws-two"} {
		result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
			WorkspaceID: workspaceID,
			Kind:        "mcp/tool_call",
			Input: map[string]any{
				"serverId": "mcp-stdio-workspace-session",
				"tool":     "repo.summary",
			},
		})
		if err != nil {
			t.Fatalf("ExecuteRuntimeStep workspace=%s: %v output=%+v", workspaceID, err, result.Output)
		}
	}
	raw, err := os.ReadFile(initFile)
	if err != nil {
		t.Fatalf("read init file: %v", err)
	}
	if got := strings.Count(string(raw), "initialized\n"); got != 2 {
		t.Fatalf("initialize count = %d, want one session per workspace; raw=%q", got, string(raw))
	}
}

func TestRuntimeAdapterFailsUnsupportedTransport(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, kernelmcp.CreateServerCommand{
		ServerID:     common.MCPServerID("mcp-websocket"),
		Name:         "websocket",
		Transport:    "websocket",
		Endpoint:     "tool-server",
		Enabled:      true,
		AllowedTools: []string{"repo.summary"},
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteRuntimeStep(ctx, runtimemodel.StepExecutionRequest{
		Input: map[string]any{
			"serverId": "mcp-websocket",
			"tool":     "repo.summary",
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ExecuteRuntimeStep err = %v, want invalid argument", err)
	}
	if result.Output["stage"] != "mcp_tool_call_unsupported_transport" {
		t.Fatalf("output = %+v, want unsupported transport stage", result.Output)
	}
}

func TestMCPStdioHelperProcess(t *testing.T) {
	if os.Getenv("WORKORCH_MCP_STDIO_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     any            `json:"id,omitempty"`
			Method string         `json:"method"`
			Params map[string]any `json:"params,omitempty"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      nil,
				"error": map[string]any{
					"code":    -32700,
					"message": err.Error(),
				},
			})
			continue
		}
		switch request.Method {
		case "initialize":
			if initFile := os.Getenv("WORKORCH_MCP_STDIO_INIT_FILE"); initFile != "" {
				f, err := os.OpenFile(initFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
				if err == nil {
					_, _ = f.WriteString("initialized\n")
					_ = f.Close()
				}
			}
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{
					"protocolVersion": mcpProtocolVersion,
					"capabilities": map[string]any{
						"tools": map[string]any{},
					},
					"serverInfo": map[string]any{
						"name":    "workorch-test-mcp",
						"version": "test",
					},
				},
			})
		case "notifications/initialized":
		case "tools/call":
			if os.Getenv("WORKORCH_MCP_STDIO_TOOL_ERROR") == "1" {
				_ = encoder.Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      request.ID,
					"result": map[string]any{
						"content": []map[string]any{
							{
								"type": "text",
								"text": "stdio tool failed",
							},
						},
						"isError": true,
					},
				})
				continue
			}
			cwd, _ := os.Getwd()
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"method":  "notifications/progress",
				"params": map[string]any{
					"message": "tool call started",
				},
			})
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{
					"content": []map[string]any{
						{
							"type": "text",
							"text": "repo summary ready",
						},
					},
					"structuredContent": map[string]any{
						"tool":      request.Params["name"],
						"arguments": request.Params["arguments"],
						"envEcho":   os.Getenv("WORKORCH_MCP_STDIO_ENV_ECHO"),
						"cwd":       cwd,
						"ok":        true,
					},
					"isError": false,
				},
			})
		case "resources/read":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{
					"contents": []map[string]any{
						{
							"uri":  request.Params["uri"],
							"text": "resource content",
						},
					},
				},
			})
		case "prompts/get":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{
					"description": "prompt ready",
					"messages": []map[string]any{
						{
							"role": "user",
							"content": map[string]any{
								"type": "text",
								"text": "plan delivery",
							},
						},
					},
				},
			})
		default:
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"error": map[string]any{
					"code":    -32601,
					"message": "method not found",
				},
			})
		}
	}
	os.Exit(0)
}

func newMCPService(t *testing.T, cmd kernelmcp.CreateServerCommand) kernelmcp.Service {
	t.Helper()
	module := kernelmcp.New()
	if err := module.Configure(kernelmcp.Dependencies{
		Servers:      memory.NewMCPServerRepository(),
		Capabilities: memory.NewMCPCapabilityRepository(),
		EventHub:     events.NewMemoryHub(),
	}); err != nil {
		t.Fatalf("setup mcp module: %v", err)
	}
	if err := module.Service().CreateServer(context.Background(), cmd); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}
	return module.Service()
}
