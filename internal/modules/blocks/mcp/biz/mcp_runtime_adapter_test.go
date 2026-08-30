package mcp

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	registryevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorcheventpayload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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

	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-local-context"),
		Name:          "local context",
		Transport:     "http",
		Endpoint:      server.URL,
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		WorkspaceID: common.WorkspaceID("ws-mcp"),
		RunID:       common.RunID("run-mcp"),
		StepID:      common.StepID("run-mcp/mcp-1"),
		Kind:        "mcp/tool_call",
		Data:        mcpevents.OperationData{ServerID: "mcp-local-context", Tool: "repo.summary", Input: []runtimecontract.OperationParameter{mcpStringArgument("path", ".")}, Summary: "summarize repository"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if result.Status != "completed" || result.Stage != "mcp_tool_call_completed" {
		t.Fatalf("output = %+v, want completed mcp output", result)
	}
	if captured["workspaceId"] != "ws-mcp" || captured["tool"] != "repo.summary" {
		t.Fatalf("captured = %+v, want workorch trace refs", captured)
	}
	resultPayload := decodedMCPResponse(t, result.Response)
	if resultPayload["summary"] != "repo summary ready" {
		t.Fatalf("result payload = %#v, want decoded response", decodedMCPResponse(t, result.Response))
	}
}

func TestRuntimeAdapterProbeChecksServerPolicyWithoutCallingTool(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	adapter := NewRuntimeAdapter(newMCPService(t, mcpmodel.Server{
		ID: common.MCPServerID("mcp-probe"), Transport: "http", Endpoint: server.URL, Enabled: true, AllowedTools: []string{"repo.summary"},
	}))
	ids, recipeRefs, revision, err := adapter.OperationAdmissibility(context.Background())
	if err != nil || len(ids) != 1 || ids[0] != "mcp-probe" || revision == "" {
		t.Fatalf("mcp admissibility = ids=%v revision=%q err=%v", ids, revision, err)
	}
	if len(recipeRefs) != 1 || recipeRefs[0] != "mcp/tool_call/mcp-probe/repo.summary" {
		t.Fatalf("MCP validation recipe refs = %v", recipeRefs)
	}
	if err := adapter.ProbeOperation(context.Background(), RuntimeOperationRequest{
		Kind: "mcp/tool_call", Data: mcpevents.OperationData{ServerID: "mcp-probe", Tool: "repo.summary"},
	}); err != nil {
		t.Fatalf("probe configured MCP tool: %v", err)
	}
	if calls != 0 {
		t.Fatalf("probe called MCP endpoint %d times", calls)
	}
	if err := adapter.ProbeOperation(context.Background(), RuntimeOperationRequest{
		Kind: "mcp/tool_call", Data: mcpevents.OperationData{ServerID: "mcp-probe", Tool: "repo.delete"},
	}); err == nil {
		t.Fatal("probe accepted tool outside server policy")
	}
	if calls != 0 {
		t.Fatalf("rejected probe called MCP endpoint %d times", calls)
	}
}

func TestRuntimeAdapterAdmissibilityKeepsInputRequiringToolCallableButNotValidationReady(t *testing.T) {
	server := &mcpmodel.Server{
		ID: common.MCPServerID("mcp-lsp-bridge"), Transport: "http", Endpoint: "http://mcp.invalid",
		Enabled: true, AllowedTools: []string{"lsp_definition", "lsp_status"},
	}
	capability := func(name string, schema workorcheventpayload.Payload, applicability runtimecontract.RuntimeValidationProofApplicability) *mcpmodel.Capability {
		return &mcpmodel.Capability{
			ID: common.MCPCapabilityID("mcp-lsp-bridge:tool:" + name), ServerID: server.ID,
			Kind: mcpmodel.CapabilityKindTool, Name: name, Enabled: true,
			EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
			ValidationProofApplicability: string(applicability),
			Schema:                       schema,
		}
	}
	statusSchema := workorcheventpayload.New(workorcheventpayload.StringField("type", "object"))
	definitionSchema := workorcheventpayload.New(
		workorcheventpayload.StringField("type", "object"),
		workorcheventpayload.StringArrayField("required", []string{"line", "path", "col"}),
	)
	adapter := NewRuntimeAdapter(staticMCPServerGetter{server: server, capabilities: []*mcpmodel.Capability{
		capability("lsp_definition", definitionSchema, runtimecontract.RuntimeValidationProofDirect),
		capability("lsp_status", statusSchema, runtimecontract.RuntimeValidationProofAuxiliary),
	}})

	snapshot, err := adapter.runtimeOperationAdmissibilitySnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Capabilities) != 2 {
		t.Fatalf("callable MCP capability count = %d, want 2", len(snapshot.Capabilities))
	}
	if len(snapshot.ValidationRecipes) != 0 {
		t.Fatalf("auxiliary/input-incomplete surfaces leaked provider-visible validation recipes: %v", snapshot.ValidationRecipes)
	}
	definition, found := runtimecontract.RuntimeValidationSurfaceForID(snapshot.ValidationSurfaces, "mcp/tool_call/mcp-lsp-bridge/lsp_definition")
	if !found || definition.Readiness != runtimecontract.RuntimeValidationSurfacePrerequisiteRequired || definition.PrerequisiteRef == "" || definition.InputBindingRef != "" {
		t.Fatalf("lsp_definition validation surface = %+v, want input-binding prerequisite", definition)
	}
	status, found := runtimecontract.RuntimeValidationSurfaceForID(snapshot.ValidationSurfaces, "mcp/tool_call/mcp-lsp-bridge/lsp_status")
	if !found || status.Readiness != runtimecontract.RuntimeValidationSurfaceReady || status.ProofApplicability != runtimecontract.RuntimeValidationProofAuxiliary || status.InputBindingRef == "" || status.PrerequisiteRef != "" {
		t.Fatalf("lsp_status validation surface = %+v, want ready auxiliary empty-input binding", status)
	}
}

func TestRuntimeAdapterAdmissibilityRejectsServerOnlyCatalog(t *testing.T) {
	adapter := NewRuntimeAdapter(serverOnlyMCPGetter{server: &mcpmodel.Server{
		ID: common.MCPServerID("mcp-server-only"), Enabled: true,
		Transport: "http", AllowedTools: []string{"repo.summary"},
	}})
	_, _, _, err := adapter.OperationAdmissibility(context.Background())
	if err == nil {
		t.Fatal("server-only MCP policy unexpectedly produced admissibility")
	}
	var unavailable *runtimecontract.RuntimeCapabilityCatalogUnavailableError
	if !errors.As(err, &unavailable) || unavailable.OwnerModuleID != ModuleID || unavailable.SubjectRef != "mcp_capability_catalog" || unavailable.Reason != "owner_unavailable" {
		t.Fatalf("catalog error = %#v, want typed owner-unavailable capability catalog error", err)
	}
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("catalog error = %v, want invalid-argument compatibility classification", err)
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

	service := newMCPService(t, mcpmodel.Server{
		ID:               common.MCPServerID("mcp-http-context"),
		Name:             "http context",
		Transport:        "http",
		Endpoint:         server.URL,
		Enabled:          true,
		AllowedResources: []string{"file://README.md"},
		AllowedPrompts:   []string{"delivery.plan"},
		TimeoutMillis:    1000,
	})
	adapter := NewRuntimeAdapter(service)

	resourceResult, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Kind: "mcp/resource_read",
		Data: mcpevents.OperationData{ServerID: "mcp-http-context", URI: "file://README.md"},
	})
	if err != nil {
		t.Fatalf("resource ExecuteOperation: %v", err)
	}
	if resourceResult.Stage != "mcp_resource_read_completed" || resourceResult.URI != "file://README.md" {
		t.Fatalf("resource output = %+v, want completed resource read", resourceResult)
	}

	promptResult, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Kind: "mcp/prompt_get",
		Data: mcpevents.OperationData{ServerID: "mcp-http-context", Prompt: "delivery.plan", Arguments: []runtimecontract.OperationParameter{mcpStringArgument("goal", "ship")}},
	})
	if err != nil {
		t.Fatalf("prompt ExecuteOperation: %v", err)
	}
	if promptResult.Stage != "mcp_prompt_get_completed" || promptResult.Prompt != "delivery.plan" {
		t.Fatalf("prompt output = %+v, want completed prompt get", promptResult)
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

	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-http-stream"),
		Name:          "http stream",
		Transport:     "http",
		Endpoint:      server.URL,
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)
	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Kind: "mcp/tool_call",
		Data: mcpevents.OperationData{ServerID: "mcp-http-stream", Tool: "repo.summary", Stream: true},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	events := result.StreamEvents
	if len(events) != 2 {
		t.Fatalf("streamEvents = %#v, want two HTTP stream events", result.StreamEvents)
	}
}

func TestRuntimeAdapterRejectsOversizedAndUnboundedHTTPResponses(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        func() string
		wantCode    string
	}{
		{
			name: "oversized response",
			body: func() string {
				return `{"content":"` + strings.Repeat("x", maxMCPResponseBytes) + `"}`
			},
			wantCode: "mcp_output_too_large",
		},
		{
			name: "too many stream events", contentType: "text/event-stream",
			body: func() string {
				return strings.Repeat("data: {\"delta\":\"x\"}\n\n", maxMCPStreamEvents+1)
			},
			wantCode: "mcp_protocol_invalid",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
				}
				_, _ = w.Write([]byte(tc.body()))
			}))
			defer server.Close()
			adapter := NewRuntimeAdapter(newMCPService(t, mcpmodel.Server{
				ID: "mcp-bounded-response", Transport: "http", Endpoint: server.URL,
				Enabled: true, AllowedTools: []string{"inspect"}, TimeoutMillis: 1000,
			}))
			result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
				Kind: runtimecontract.StepKindMCPToolCall,
				Data: mcpevents.OperationData{ServerID: "mcp-bounded-response", Tool: "inspect", Stream: tc.contentType != ""},
			})
			if err == nil || result.FailureCode != tc.wantCode {
				t.Fatalf("bounded MCP response result=%+v err=%v, want code %q", result, err, tc.wantCode)
			}
		})
	}
}

func TestRuntimeAdapterRejectsCrossOriginRedirectBeforeForwardingPayload(t *testing.T) {
	var destinationCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		destinationCalls.Add(1)
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", destination.URL+"/capture")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	adapter := NewRuntimeAdapter(newMCPService(t, mcpmodel.Server{
		ID: "mcp-redirect", Transport: "http", Endpoint: origin.URL,
		Enabled: true, AllowedTools: []string{"inspect"}, TimeoutMillis: 1000,
	}))
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind: runtimecontract.StepKindMCPToolCall,
		Data: mcpevents.OperationData{ServerID: "mcp-redirect", Tool: "inspect", Input: []runtimecontract.OperationParameter{mcpStringArgument("secret", "owner-private-input")}},
	})
	if err == nil || destinationCalls.Load() != 0 || result.Status != "failed" {
		t.Fatalf("cross-origin MCP redirect result=%+v err=%v destinationCalls=%d", result, err, destinationCalls.Load())
	}
}

func TestReadJSONRPCResponseRejectsOversizedLineAndNotificationCount(t *testing.T) {
	t.Run("oversized line", func(t *testing.T) {
		reader := bufio.NewReader(strings.NewReader(strings.Repeat("x", maxMCPResponseBytes+1) + "\n"))
		if _, _, err := readJSONRPCResponse(reader, 1); err == nil || !strings.Contains(err.Error(), "output limit") {
			t.Fatalf("oversized stdio response error = %v", err)
		}
	})
	t.Run("notification count", func(t *testing.T) {
		var input strings.Builder
		for index := 0; index <= maxMCPStreamEvents; index++ {
			input.WriteString(`{"jsonrpc":"2.0","method":"progress","params":{"value":1}}`)
			input.WriteByte('\n')
		}
		reader := bufio.NewReader(strings.NewReader(input.String()))
		if _, _, err := readJSONRPCResponse(reader, 1); err == nil || !strings.Contains(err.Error(), "notification count") {
			t.Fatalf("unbounded stdio notifications error = %v", err)
		}
	})
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

	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-http-tool-error"),
		Name:          "http tool error",
		Transport:     "http",
		Endpoint:      server.URL,
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)
	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Kind: "mcp/tool_call",
		Data: mcpevents.OperationData{ServerID: "mcp-http-tool-error", Tool: "repo.summary"},
	})
	if err == nil || err.Error() != "mcp tool returned an error result" || strings.Contains(err.Error(), "missing argument path") {
		t.Fatalf("ExecuteOperation err = %v, want MCP tool result error", err)
	}
	if result.Status != "failed" || result.Stage != "mcp_tool_call_failed" {
		t.Fatalf("output = %+v, want failed tool call", result)
	}
	diagnostics := result.Diagnostics
	if diagnostics.TargetRef != "" {
		t.Fatalf("runtimeDiagnostics = %+v, did not expect target without input target", diagnostics)
	}
}

func TestRuntimeAdapterFailureDiagnosticsKeepToolInputAsContextOnly(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{{"type": "text", "text": "column is beyond end of line"}},
			"isError": true,
		})
	}))
	defer server.Close()

	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-http-tool-target"),
		Name:          "http tool target",
		Transport:     "http",
		Endpoint:      server.URL,
		Enabled:       true,
		AllowedTools:  []string{"lookup.references"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)
	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Kind: "mcp/tool_call",
		Data: mcpevents.OperationData{ServerID: "mcp-http-tool-target", Tool: "lookup.references", Input: []runtimecontract.OperationParameter{mcpStringArgument("path", "src/symbol.go"), mcpIntegerArgument("line", 1), mcpIntegerArgument("col", 13)}},
	})
	if err == nil || err.Error() != "mcp tool returned an error result" || strings.Contains(err.Error(), "column is beyond end of line") {
		t.Fatalf("ExecuteOperation err = %v, want MCP tool result error", err)
	}
	diagnostics := result.Diagnostics
	if diagnostics.TargetRef != "src/symbol.go" {
		t.Fatalf("runtimeDiagnostics = %+v, want tool input target", diagnostics)
	}
	if len(diagnostics.FailureTargets) != 0 {
		t.Fatalf("failureTargets = %#v, want no unobserved target", diagnostics.FailureTargets)
	}
}

func TestRuntimeAdapterFailureDiagnosticsUseExplicitResponseFailureTargets(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":        []map[string]any{{"type": "text", "text": "implementation failure"}},
			"failureTargets": []string{" ./implementation.go ", "implementation.go", ""},
			"isError":        true,
		})
	}))
	defer server.Close()

	adapter := NewRuntimeAdapter(newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-http-observed-target"),
		Name:          "http observed target",
		Transport:     "http",
		Endpoint:      server.URL,
		Enabled:       true,
		AllowedTools:  []string{"lookup.references"},
		TimeoutMillis: 1000,
	}))
	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Kind:       "mcp/tool_call",
		TargetRefs: []string{"validation-entry.go"}, TargetRole: "implementation",
		Data: mcpevents.OperationData{
			ServerID: "mcp-http-observed-target", Tool: "lookup.references",
			Input: []runtimecontract.OperationParameter{mcpStringArgument("path", "validation-entry.go")},
		},
	})
	if err == nil || err.Error() != "mcp tool returned an error result" || strings.Contains(err.Error(), "implementation failure") {
		t.Fatalf("ExecuteOperation err = %v, want MCP tool result error", err)
	}
	if result.Diagnostics.TargetRef != "validation-entry.go" {
		t.Fatalf("diagnostic target = %q, want operation context", result.Diagnostics.TargetRef)
	}
	if got, want := result.Diagnostics.FailureTargets, []string{"implementation.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("observed failure targets = %#v, want %#v", got, want)
	}
	if got, want := result.TargetRefs, []string{"validation-entry.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("operation target refs = %#v, want %#v", got, want)
	}
	if result.Diagnostics.TargetRole != "implementation" || result.OwnerRevision == "" {
		t.Fatalf("owner observation binding = %+v ownerRevision=%q", result.Diagnostics, result.OwnerRevision)
	}
	_, _, admissibilityRevision, admissibilityErr := adapter.OperationAdmissibility(ctx)
	if admissibilityErr != nil || result.OwnerRevision != admissibilityRevision {
		t.Fatalf("owner revision = %q, admissibility revision = %q, err=%v", result.OwnerRevision, admissibilityRevision, admissibilityErr)
	}
}

func TestRuntimeAdapterSuccessDoesNotObserveFailureTargets(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"failureTargets": []string{"implementation.go"}, "isError": false})
	}))
	defer server.Close()

	adapter := NewRuntimeAdapter(newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-http-success-target"),
		Name:          "http success target",
		Transport:     "http",
		Endpoint:      server.URL,
		Enabled:       true,
		AllowedTools:  []string{"lookup.references"},
		TimeoutMillis: 1000,
	}))
	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Kind: "mcp/tool_call",
		Data: mcpevents.OperationData{ServerID: "mcp-http-success-target", Tool: "lookup.references"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v", err)
	}
	if len(result.Diagnostics.FailureTargets) != 0 {
		t.Fatalf("failure targets = %#v, want none for successful result", result.Diagnostics.FailureTargets)
	}
}

func TestRuntimeAdapterRejectsDisallowedTool(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, mcpmodel.Server{
		ID:           common.MCPServerID("mcp-policy"),
		Name:         "policy",
		Transport:    "http",
		Endpoint:     "http://127.0.0.1:1",
		Enabled:      true,
		AllowedTools: []string{"repo.summary"},
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Data: mcpevents.OperationData{ServerID: "mcp-policy", Tool: "repo.write"},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ExecuteOperation err = %v, want invalid argument", err)
	}
	if result.Stage != "mcp_tool_call_rejected" {
		t.Fatalf("output = %+v, want rejected stage", result)
	}
}

func TestRuntimeAdapterExecutesAllowedStdioToolCall(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-stdio"),
		Name:          "stdio",
		Transport:     "stdio",
		Endpoint:      "env WORKORCH_MCP_STDIO_HELPER=1 " + os.Args[0] + " -test.run=TestMCPStdioHelperProcess",
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		WorkspaceID: common.WorkspaceID("ws-mcp"),
		RunID:       common.RunID("run-mcp"),
		StepID:      common.StepID("run-mcp/mcp-stdio-1"),
		Kind:        "mcp/tool_call",
		Data:        mcpevents.OperationData{ServerID: "mcp-stdio", Tool: "repo.summary", Input: []runtimecontract.OperationParameter{mcpStringArgument("path", ".")}, Summary: "summarize repository through stdio"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	if result.Status != "completed" || result.Stage != "mcp_tool_call_completed" {
		t.Fatalf("output = %+v, want completed mcp output", result)
	}
	if result.Transport != "stdio" {
		t.Fatalf("transport = %v, want stdio", result.Transport)
	}
	events := result.StreamEvents
	if len(events) == 0 {
		t.Fatalf("streamEvents = %#v, want stdio notification evidence", result.StreamEvents)
	}
	resultPayload := decodedMCPResponse(t, result.Response)
	if resultPayload == nil {
		t.Fatalf("result payload = %#v, want decoded response", decodedMCPResponse(t, result.Response))
	}
	structured, ok := resultPayload["structuredContent"].(map[string]any)
	if !ok || structured["tool"] != "repo.summary" {
		t.Fatalf("structured content = %#v, want tool result", resultPayload["structuredContent"])
	}
}

func TestRuntimeAdapterStdioToolCallHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-stdio-canceled"),
		Name:          "stdio canceled",
		Transport:     "stdio",
		Endpoint:      "env WORKORCH_MCP_STDIO_HELPER=1 " + os.Args[0] + " -test.run=TestMCPStdioHelperProcess",
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 5000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		WorkspaceID: common.WorkspaceID("ws-mcp-canceled"),
		RunID:       common.RunID("run-mcp-canceled"),
		StepID:      common.StepID("run-mcp-canceled/mcp-stdio-1"),
		Kind:        "mcp/tool_call",
		Data:        mcpevents.OperationData{ServerID: "mcp-stdio-canceled", Tool: "repo.summary", Input: []runtimecontract.OperationParameter{mcpStringArgument("path", ".")}},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecuteOperation err = %v output=%+v, want context canceled", err, result)
	}
}

func TestRuntimeAdapterExecutesStdioCommandArgsEnvWhenEndpointMissing(t *testing.T) {
	ctx := context.Background()
	internalDir := filepath.Join(t.TempDir(), "internal")
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-stdio-command"),
		Name:          "stdio command",
		Transport:     "stdio",
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestMCPStdioHelperProcess"},
		Env:           map[string]string{"WORKORCH_MCP_STDIO_HELPER": "1", "WORKORCH_MCP_STDIO_ENV_ECHO": "{workspaceInternalDirectory}"},
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		WorkspaceID:                common.WorkspaceID("ws-mcp-command"),
		WorkspaceInternalDirectory: internalDir,
		RunID:                      common.RunID("run-mcp-command"),
		StepID:                     common.StepID("run-mcp-command/mcp-stdio-1"),
		Kind:                       "mcp/tool_call",
		Data:                       mcpevents.OperationData{ServerID: "mcp-stdio-command", Tool: "repo.summary"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	resultPayload := decodedMCPResponse(t, result.Response)
	if resultPayload == nil {
		t.Fatalf("result payload = %#v, want decoded response", decodedMCPResponse(t, result.Response))
	}
	structured, ok := resultPayload["structuredContent"].(map[string]any)
	if !ok || structured["envEcho"] != internalDir {
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
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-managed-config"),
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

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		WorkspaceID:                common.WorkspaceID("ws-mcp-managed-config"),
		WorkspaceInternalDirectory: internalDir,
		RunID:                      common.RunID("run-mcp-managed-config"),
		StepID:                     common.StepID("run-mcp-managed-config/mcp-stdio-1"),
		Kind:                       "mcp/tool_call",
		Data:                       mcpevents.OperationData{ServerID: "mcp-managed-config", Tool: "repo.summary"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	resultPayload := decodedMCPResponse(t, result.Response)
	if resultPayload == nil {
		t.Fatalf("result payload = %#v, want decoded response", decodedMCPResponse(t, result.Response))
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
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-system-config"),
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

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		WorkspaceID:                common.WorkspaceID("ws-mcp-system-config"),
		WorkspaceInternalDirectory: internalDir,
		RunID:                      common.RunID("run-mcp-system-config"),
		StepID:                     common.StepID("run-mcp-system-config/mcp-stdio-1"),
		Kind:                       "mcp/tool_call",
		Data:                       mcpevents.OperationData{ServerID: "mcp-system-config", Tool: "repo.summary"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	resultPayload := decodedMCPResponse(t, result.Response)
	if resultPayload == nil {
		t.Fatalf("result payload = %#v, want decoded response", decodedMCPResponse(t, result.Response))
	}
	structured, ok := resultPayload["structuredContent"].(map[string]any)
	if !ok || structured["envEcho"] != systemConfigPath {
		t.Fatalf("structured content = %#v, want system managed config env", resultPayload["structuredContent"])
	}
}

func TestRuntimeAdapterStartsStdioInWorkspaceDeliverableDirectory(t *testing.T) {
	ctx := context.Background()
	deliverableDir := t.TempDir()
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-stdio-workdir"),
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

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		WorkspaceID:                   common.WorkspaceID("ws-mcp-workdir"),
		WorkspaceDeliverableDirectory: deliverableDir,
		Kind:                          "mcp/tool_call",
		Data:                          mcpevents.OperationData{ServerID: "mcp-stdio-workdir", Tool: "repo.summary"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	resultPayload := decodedMCPResponse(t, result.Response)
	if resultPayload == nil {
		t.Fatalf("result payload = %#v, want decoded response", decodedMCPResponse(t, result.Response))
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
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-stdio-relative-endpoint"),
		Name:          "stdio relative endpoint",
		Transport:     "stdio",
		Endpoint:      "." + string(os.PathSeparator) + helperName,
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		WorkspaceID:                   common.WorkspaceID("ws-mcp-relative-endpoint"),
		WorkspaceDeliverableDirectory: deliverableDir,
		Kind:                          "mcp/tool_call",
		Data:                          mcpevents.OperationData{ServerID: "mcp-stdio-relative-endpoint", Tool: "repo.summary"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	if result.Transport != "stdio" || result.Status != "completed" {
		t.Fatalf("output = %+v, want completed stdio call from relative endpoint", result)
	}
}

func TestRuntimeAdapterPrefersEndpointOverStdioCommandArgsEnv(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-stdio-endpoint-priority"),
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

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Kind: "mcp/tool_call",
		Data: mcpevents.OperationData{ServerID: "mcp-stdio-endpoint-priority", Tool: "repo.summary"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	resultPayload := decodedMCPResponse(t, result.Response)
	structured, _ := resultPayload["structuredContent"].(map[string]any)
	if structured["envEcho"] != "from-endpoint" {
		t.Fatalf("structured content = %#v, want endpoint env to take priority", structured)
	}
}

func TestRuntimeAdapterFailsStdioToolResultError(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-stdio-tool-error"),
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

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Kind: "mcp/tool_call",
		Data: mcpevents.OperationData{ServerID: "mcp-stdio-tool-error", Tool: "repo.summary"},
	})
	if err == nil || err.Error() != "mcp tool returned an error result" || strings.Contains(err.Error(), "stdio tool failed") {
		t.Fatalf("ExecuteOperation err = %v, want stdio MCP tool result error", err)
	}
	if result.Status != "failed" || result.Stage != "mcp_tool_call_failed" {
		t.Fatalf("output = %+v, want failed tool call", result)
	}
}

func TestRuntimeAdapterRejectsStdioWithoutEndpointOrCommand(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-stdio-missing-command"),
		Name:          "stdio missing command",
		Transport:     "stdio",
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Kind: "mcp/tool_call",
		Data: mcpevents.OperationData{ServerID: "mcp-stdio-missing-command", Tool: "repo.summary"},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "endpoint or command") {
		t.Fatalf("ExecuteOperation err = %v, want explicit missing endpoint or command", err)
	}
	if result.Stage != "mcp_tool_call_failed" {
		t.Fatalf("output = %+v, want failed stage", result)
	}
}

func TestRuntimeAdapterExecutesAllowedStdioResourceRead(t *testing.T) {
	ctx := context.Background()
	service := newMCPService(t, mcpmodel.Server{
		ID:               common.MCPServerID("mcp-stdio-resource"),
		Name:             "stdio resource",
		Transport:        "stdio",
		Endpoint:         "env WORKORCH_MCP_STDIO_HELPER=1 " + os.Args[0] + " -test.run=TestMCPStdioHelperProcess",
		Enabled:          true,
		AllowedResources: []string{"file://README.md"},
		TimeoutMillis:    1000,
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Kind: "mcp/resource_read",
		Data: mcpevents.OperationData{ServerID: "mcp-stdio-resource", URI: "file://README.md"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	if result.Stage != "mcp_resource_read_completed" {
		t.Fatalf("output = %+v, want completed resource read", result)
	}
}

func TestRuntimeAdapterReusesLongLivedStdioSession(t *testing.T) {
	ctx := context.Background()
	initFile := filepath.Join(t.TempDir(), "mcp-init-count.txt")
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-stdio-session"),
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
		result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
			Kind: "mcp/tool_call",
			Data: mcpevents.OperationData{ServerID: "mcp-stdio-session", Tool: "repo.summary"},
		})
		if err != nil {
			t.Fatalf("ExecuteOperation #%d: %v output=%+v", idx+1, err, result)
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

func TestMCPOwnerTeardownTerminatesLongLivedStdioSession(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-stdio-close-session"),
		Name:          "stdio close session",
		Transport:     "stdio",
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestMCPStdioHelperProcess"},
		Env:           map[string]string{"WORKORCH_MCP_STDIO_HELPER": "1"},
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		SessionPolicy: "long_lived",
		TimeoutMillis: 1000,
	})
	adapter := NewRuntimeAdapter(service)
	owner, err := NewWithDependencies(Dependencies{EventHub: hub, RuntimeAdapter: adapter})
	if err != nil {
		t.Fatalf("NewWithDependencies: %v", err)
	}
	if err := owner.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		WorkspaceID: "ws-close", Kind: "mcp/tool_call",
		Data: mcpevents.OperationData{ServerID: "mcp-stdio-close-session", Tool: "repo.summary"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	adapter.sessionMu.Lock()
	sessionCount := len(adapter.sessions)
	if sessionCount != 1 {
		adapter.sessionMu.Unlock()
		t.Fatalf("stdio sessions = %d, want 1 before close", sessionCount)
	}
	var session *stdioSession
	for _, value := range adapter.sessions {
		session = value
	}
	adapter.sessionMu.Unlock()
	if session == nil || session.cmd == nil || session.cmd.Process == nil {
		t.Fatalf("long-lived stdio session is not active before close: %+v", session)
	}

	owner.Teardown(ctx)
	adapter.sessionMu.Lock()
	remaining := len(adapter.sessions)
	adapter.sessionMu.Unlock()
	if remaining != 0 || owner.runtimeAdapter != nil || session.cmd.ProcessState == nil || !session.cmd.ProcessState.Exited() {
		t.Fatalf("owner teardown left stdio session active: remaining=%d adapter=%v state=%v", remaining, owner.runtimeAdapter, session.cmd.ProcessState)
	}
	// Teardown is an idempotent lifecycle fence.
	owner.Teardown(ctx)
}

func TestRuntimeAdapterEvictsIdleLongLivedStdioSession(t *testing.T) {
	ctx := context.Background()
	initFile := filepath.Join(t.TempDir(), "mcp-init-count.txt")
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-stdio-idle-session"),
		Name:          "stdio idle session",
		Transport:     "stdio",
		Endpoint:      "env WORKORCH_MCP_STDIO_HELPER=1 WORKORCH_MCP_STDIO_INIT_FILE=" + initFile + " " + os.Args[0] + " -test.run=TestMCPStdioHelperProcess",
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		SessionPolicy: "long_lived",
		TimeoutMillis: 1000,
	})
	adapter := newRuntimeAdapterWithIdleSessionTimeout(service, http.DefaultClient, 20*time.Millisecond)
	defer adapter.Close()

	for idx := 0; idx < 2; idx++ {
		result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
			WorkspaceID: common.WorkspaceID("ws-idle"),
			Kind:        "mcp/tool_call",
			Data:        mcpevents.OperationData{ServerID: "mcp-stdio-idle-session", Tool: "repo.summary"},
		})
		if err != nil {
			t.Fatalf("ExecuteOperation #%d: %v output=%+v", idx+1, err, result)
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
	service := newMCPService(t, mcpmodel.Server{
		ID:            common.MCPServerID("mcp-stdio-workspace-session"),
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
		result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
			WorkspaceID: workspaceID,
			Kind:        "mcp/tool_call",
			Data:        mcpevents.OperationData{ServerID: "mcp-stdio-workspace-session", Tool: "repo.summary"},
		})
		if err != nil {
			t.Fatalf("ExecuteOperation workspace=%s: %v output=%+v", workspaceID, err, result)
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
	service := newMCPService(t, mcpmodel.Server{
		ID:           common.MCPServerID("mcp-websocket"),
		Name:         "websocket",
		Transport:    "websocket",
		Endpoint:     "tool-server",
		Enabled:      true,
		AllowedTools: []string{"repo.summary"},
	})
	adapter := NewRuntimeAdapter(service)

	result, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{
		Data: mcpevents.OperationData{ServerID: "mcp-websocket", Tool: "repo.summary"},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ExecuteOperation err = %v, want invalid argument", err)
	}
	if result.Stage != "mcp_tool_call_unsupported_transport" {
		t.Fatalf("output = %+v, want unsupported transport stage", result)
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
			if os.Getenv("WORKORCH_MCP_STDIO_TOOL_DELAY") == "1" {
				time.Sleep(5 * time.Second)
			}
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

func decodedMCPResponse(t *testing.T, response MCPResponsePayload) map[string]any {
	t.Helper()
	if len(response.JSON) == 0 {
		return nil
	}
	var ret map[string]any
	if err := json.Unmarshal(response.JSON, &ret); err != nil {
		t.Fatalf("decode MCP response: %v", err)
	}
	return ret
}

func mcpStringArgument(name string, value string) runtimecontract.OperationParameter {
	return runtimecontract.OperationParameter{Name: name, String: &value}
}

func mcpIntegerArgument(name string, value int64) runtimecontract.OperationParameter {
	return runtimecontract.OperationParameter{Name: name, Integer: &value}
}

func mcpBoolArgument(name string, value bool) runtimecontract.OperationParameter {
	return runtimecontract.OperationParameter{Name: name, Boolean: &value}
}

func mcpStringListArgument(name string, value []string) runtimecontract.OperationParameter {
	return runtimecontract.OperationParameter{Name: name, StringList: append([]string(nil), value...)}
}

type staticMCPServerGetter struct {
	server       *mcpmodel.Server
	capabilities []*mcpmodel.Capability
}

type serverOnlyMCPGetter struct {
	server *mcpmodel.Server
}

func (g serverOnlyMCPGetter) GetServer(_ context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if g.server == nil || g.server.ID != id {
		return nil, common.ErrNotFound
	}
	return g.server, nil
}

func (g serverOnlyMCPGetter) ListServers(_ context.Context) ([]*mcpmodel.Server, error) {
	if g.server == nil {
		return nil, common.ErrNotFound
	}
	return []*mcpmodel.Server{g.server}, nil
}

func (g staticMCPServerGetter) GetServer(_ context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if g.server == nil || g.server.ID != id {
		return nil, common.ErrNotFound
	}
	return g.server, nil
}

func (g staticMCPServerGetter) RegistrySnapshot(context.Context) (*mcpmodel.RegistrySnapshot, string, error) {
	if g.server == nil {
		return nil, "", common.ErrNotFound
	}
	return testMCPRegistrySnapshot([]*mcpmodel.Server{g.server}, g.capabilities)
}

func testMCPRegistrySnapshot(servers []*mcpmodel.Server, capabilities []*mcpmodel.Capability) (*mcpmodel.RegistrySnapshot, string, error) {
	snapshot := &mcpmodel.RegistrySnapshot{Servers: mcpmodel.CloneServers(servers), Capabilities: mcpmodel.CloneCapabilities(capabilities)}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, "", err
	}
	return snapshot, fmt.Sprintf("test-mcp-registry-%x", sha256.Sum256(raw))[:42], nil
}

func newMCPService(t *testing.T, server mcpmodel.Server) serverGetter {
	t.Helper()
	if server.ID == "" {
		t.Fatal("mcp server id is required")
	}
	for key, value := range server.Env {
		if registryevents.ValidEnvironmentReference(value) {
			continue
		}
		sum := sha256.Sum256([]byte(string(server.ID) + "\x00" + key))
		name := fmt.Sprintf("WORKORCH_TEST_MCP_ENV_%X", sum[:8])
		t.Setenv(name, value)
		server.Env[key] = "env://" + name
	}
	// Tests that exercise admissibility provide an explicit owner catalog. The
	// fixture intentionally materializes capability facts here rather than
	// relying on RuntimeAdapter's former server-only fallback.
	capabilities := make([]*mcpmodel.Capability, 0, len(server.AllowedTools))
	for _, tool := range server.AllowedTools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		capabilities = append(capabilities, &mcpmodel.Capability{
			ID: common.MCPCapabilityID(string(server.ID) + ":tool:" + tool), ServerID: server.ID,
			Kind: mcpmodel.CapabilityKindTool, Name: tool, Enabled: true,
			EffectScope:                  string(runtimecontract.RuntimeEffectNone),
			IdempotencyClass:             string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
			ValidationProofApplicability: string(runtimecontract.RuntimeValidationProofDirect),
			Schema:                       workorcheventpayload.New(workorcheventpayload.StringField("type", "object")),
		})
	}
	return staticMCPServerGetter{server: &server, capabilities: capabilities}
}

func TestRuntimeSemanticRecipeParser(t *testing.T) {
	if server, tool, err := parseMCPValidationRecipe("mcp/tool_call/mcp-lsp-bridge/lsp_initialize"); err != nil || server != "mcp-lsp-bridge" || tool != "lsp_initialize" {
		t.Fatalf("parse MCP recipe = %q/%q/%v", server, tool, err)
	}
}
