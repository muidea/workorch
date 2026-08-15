package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

var runtimeMCPBenchmarkResult RuntimeOperationResult

func BenchmarkRuntimeAdapterHTTPToolCall(b *testing.B) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"summary":"ready"}`))
	}))
	b.Cleanup(server.Close)
	serverID := common.MCPServerID("mcp-benchmark")
	adapter := NewRuntimeAdapter(staticMCPServerGetter{server: &mcpmodel.Server{
		ID: serverID, Transport: "http", Endpoint: server.URL, Enabled: true,
		AllowedTools: []string{"repo.summary"}, TimeoutMillis: 1000,
	}})
	request := RuntimeOperationRequest{
		WorkspaceID: "ws-benchmark", RunID: "run-benchmark", StepID: "step-benchmark",
		Kind: "mcp/tool_call", Data: mcpevents.OperationData{ServerID: string(serverID), Tool: "repo.summary"},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		result, err := adapter.ExecuteOperation(context.Background(), request)
		if err != nil || result.Status != "completed" {
			b.Fatalf("HTTP MCP result = %+v err=%v", result, err)
		}
		runtimeMCPBenchmarkResult = result
	}
	b.StopTimer()
	if calls.Load() != int64(b.N) {
		b.Fatalf("HTTP transport calls = %d, want one per operation (%d)", calls.Load(), b.N)
	}
}
