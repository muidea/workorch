package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	kernelmcp "github.com/muidea/workorch/internal/modules/kernel/mcp"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
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
	resultPayload, ok := result.Output["result"].(map[string]any)
	if !ok {
		t.Fatalf("result payload = %#v, want decoded response", result.Output["result"])
	}
	structured, ok := resultPayload["structuredContent"].(map[string]any)
	if !ok || structured["tool"] != "repo.summary" {
		t.Fatalf("structured content = %#v, want tool result", resultPayload["structuredContent"])
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
						"ok":        true,
					},
					"isError": false,
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
	if err := module.Setup(kernelmcp.Dependencies{Servers: memory.NewMCPServerRepository()}); err != nil {
		t.Fatalf("setup mcp module: %v", err)
	}
	if err := module.Service().CreateServer(context.Background(), cmd); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}
	return module.Service()
}
