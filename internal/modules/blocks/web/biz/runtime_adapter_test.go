package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAdapterSearchUsesHTTPProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["query"] != "golang latest" || payload["maxResults"].(float64) != 2 {
			t.Fatalf("payload = %+v, want query and maxResults", payload)
		}
		_, _ = w.Write([]byte(`{"results":[
			{"title":"Go Downloads","url":"https://go.dev/dl/","snippet":"Download Go","source":"go.dev"},
			{"title":"Release notes","url":"https://go.dev/doc/devel/release","snippet":"Release history","source":"go.dev"}
		]}`))
	}))
	defer server.Close()

	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]ProviderConfig{
		"http": {
			ID:             "http",
			Provider:       webcommon.ProviderHTTP,
			SearchEndpoint: server.URL,
			Timeout:        time.Second,
			MaxResults:     4,
			MaxFetchBytes:  4096,
		},
	}, nil, server.Client())

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind: webcommon.StepKindSearch,
		Input: map[string]any{
			"query":      "golang latest",
			"maxResults": 2,
		},
	})
	if err != nil {
		t.Fatalf("execute search: %v", err)
	}
	if result.Output["status"] != "completed" || result.Output["stage"] != "web_search" || result.Output["resultCount"] != 2 {
		t.Fatalf("output = %+v, want completed search", result.Output)
	}
	results, ok := result.Output["results"].([]map[string]any)
	if !ok || len(results) != 2 || results[0]["url"] != "https://go.dev/dl/" || results[0]["contentHash"] == "" {
		t.Fatalf("results = %+v, want normalized search results", result.Output["results"])
	}
}

func TestRuntimeAdapterSearchLargeResultSetRequiresApproval(t *testing.T) {
	approvals := memory.NewApprovalRepository()
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["maxResults"].(float64) != 12 {
			t.Fatalf("payload = %+v, want approved maxResults", payload)
		}
		_, _ = w.Write([]byte(`{"results":[
			{"title":"One","url":"https://example.com/1","snippet":"one"},
			{"title":"Two","url":"https://example.com/2","snippet":"two"}
		]}`))
	}))
	defer server.Close()

	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]ProviderConfig{
		"http": {
			ID:             "http",
			Provider:       webcommon.ProviderHTTP,
			SearchEndpoint: server.URL,
			Timeout:        time.Second,
			MaxResults:     4,
			MaxFetchBytes:  4096,
		},
	}, approvals, server.Client())
	request := runtimemodel.StepExecutionRequest{
		WorkspaceID: common.WorkspaceID("ws-search-approval"),
		RunID:       common.RunID("run-search-approval"),
		StepID:      common.StepID("run-search-approval/step-1"),
		Kind:        webcommon.StepKindSearch,
		Input: map[string]any{
			"query":      "wide search",
			"maxResults": 12,
		},
	}
	blocked, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if err != nil {
		t.Fatalf("blocked search returned err: %v", err)
	}
	if blocked.Output["status"] != "blocked" ||
		blocked.Output["errorCode"] != webcommon.ErrorApprovalRequired ||
		blocked.Output["approvalAction"] != webcommon.StepKindSearch ||
		calls != 0 {
		t.Fatalf("blocked output/calls = %+v/%d, want approval without provider call", blocked.Output, calls)
	}
	contextMap, ok := blocked.Output["approvalContext"].(map[string]any)
	if !ok || contextMap["query"] != "wide search" {
		t.Fatalf("approvalContext = %+v, want query context", blocked.Output["approvalContext"])
	}
	approvalID := common.ApprovalID("approval-web-search")
	if err := approvals.Create(context.Background(), &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: request.WorkspaceID,
		RunID:       request.RunID,
		StepID:      request.StepID,
		Reason:      "approve wide search",
		Action:      webcommon.StepKindSearch,
		Context:     contextMap,
		State:       common.ApprovalStateApproved,
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	request.Input["approvalId"] = string(approvalID)
	completed, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if err != nil {
		t.Fatalf("approved search: %v", err)
	}
	if completed.Output["status"] != "completed" ||
		completed.Output["approvalId"] != string(approvalID) ||
		completed.Output["resultCount"] != 2 ||
		calls != 1 {
		t.Fatalf("completed output/calls = %+v/%d, want approved search", completed.Output, calls)
	}
}

func TestRuntimeAdapterFetchBlocksLoopbackUntilApproved(t *testing.T) {
	approvals := memory.NewApprovalRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"http://127.0.0.1/private","title":"Private","content":"ok","contentType":"text/plain","source":"local"}`))
	}))
	defer server.Close()
	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]ProviderConfig{
		"http": {
			ID:             "http",
			Provider:       webcommon.ProviderHTTP,
			SearchEndpoint: server.URL,
			FetchEndpoint:  server.URL,
			Timeout:        time.Second,
			MaxFetchBytes:  4096,
		},
	}, approvals, server.Client())

	request := runtimemodel.StepExecutionRequest{
		WorkspaceID: common.WorkspaceID("ws-1"),
		RunID:       common.RunID("run-1"),
		StepID:      common.StepID("run-1/step-1"),
		Kind:        webcommon.StepKindFetch,
		Input: map[string]any{
			"url": "http://127.0.0.1/private",
		},
	}
	blocked, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if err != nil {
		t.Fatalf("blocked fetch returned err: %v", err)
	}
	if blocked.Output["status"] != "blocked" || blocked.Output["errorCode"] != webcommon.ErrorApprovalRequired || blocked.Output["approvalRequired"] != true {
		t.Fatalf("blocked output = %+v, want approval required", blocked.Output)
	}
	contextMap, ok := blocked.Output["approvalContext"].(map[string]any)
	if !ok {
		t.Fatalf("approvalContext = %+v, want map", blocked.Output["approvalContext"])
	}
	approvalID := common.ApprovalID("approval-web-fetch")
	if err := approvals.Create(context.Background(), &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: request.WorkspaceID,
		RunID:       request.RunID,
		StepID:      request.StepID,
		Reason:      "approve loopback fetch",
		Action:      webcommon.StepKindFetch,
		Context:     contextMap,
		State:       common.ApprovalStateApproved,
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	request.Input["approvalId"] = string(approvalID)
	completed, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if err != nil {
		t.Fatalf("approved fetch: %v", err)
	}
	if completed.Output["status"] != "completed" || completed.Output["stage"] != "web_fetch" || completed.Output["approvalId"] != string(approvalID) {
		t.Fatalf("completed output = %+v, want approved fetch", completed.Output)
	}
}

func TestRuntimeAdapterFetchRechecksFinalURLPolicy(t *testing.T) {
	approvals := memory.NewApprovalRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://93.184.216.34/source","finalUrl":"http://127.0.0.1/private","title":"Private","content":"ok","contentType":"text/plain","source":"local"}`))
	}))
	defer server.Close()
	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]ProviderConfig{
		"http": {
			ID:             "http",
			Provider:       webcommon.ProviderHTTP,
			SearchEndpoint: server.URL,
			FetchEndpoint:  server.URL,
			Timeout:        time.Second,
			MaxFetchBytes:  4096,
		},
	}, approvals, server.Client())

	request := runtimemodel.StepExecutionRequest{
		WorkspaceID: common.WorkspaceID("ws-final-url"),
		RunID:       common.RunID("run-final-url"),
		StepID:      common.StepID("run-final-url/step-1"),
		Kind:        webcommon.StepKindFetch,
		Input: map[string]any{
			"url": "https://93.184.216.34/source",
		},
	}
	blocked, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if err != nil {
		t.Fatalf("blocked final url fetch returned err: %v", err)
	}
	if blocked.Output["status"] != "blocked" ||
		blocked.Output["errorCode"] != webcommon.ErrorApprovalRequired ||
		blocked.Output["finalUrl"] != "http://127.0.0.1/private" {
		t.Fatalf("blocked output = %+v, want final URL approval requirement", blocked.Output)
	}
	contextMap, ok := blocked.Output["approvalContext"].(map[string]any)
	if !ok || contextMap["url"] != "http://127.0.0.1/private" {
		t.Fatalf("approvalContext = %+v, want final URL context", blocked.Output["approvalContext"])
	}
	approvalID := common.ApprovalID("approval-final-url")
	if err := approvals.Create(context.Background(), &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: request.WorkspaceID,
		RunID:       request.RunID,
		StepID:      request.StepID,
		Reason:      "approve final URL",
		Action:      webcommon.StepKindFetch,
		Context:     contextMap,
		State:       common.ApprovalStateApproved,
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	request.Input["approvalId"] = string(approvalID)
	completed, err := adapter.ExecuteRuntimeStep(context.Background(), request)
	if err != nil {
		t.Fatalf("approved final url fetch: %v", err)
	}
	if completed.Output["status"] != "completed" ||
		completed.Output["finalUrl"] != "http://127.0.0.1/private" ||
		completed.Output["approvalId"] != string(approvalID) {
		t.Fatalf("completed output = %+v, want approved final URL fetch", completed.Output)
	}
}

func TestRuntimeAdapterURLPolicyCoversSSRFBoundaries(t *testing.T) {
	adapter := NewRuntimeAdapter("http", nil, nil)
	provider := ProviderConfig{
		ID:           "http",
		Provider:     webcommon.ProviderHTTP,
		AllowDomains: []string{"example.com"},
		DenyDomains:  []string{"blocked.example.com"},
	}
	tests := []struct {
		name             string
		rawURL           string
		fetch            bool
		useDomainPolicy  bool
		expectedDecision string
	}{
		{name: "allowed domain", rawURL: "https://docs.example.com/page", fetch: false, useDomainPolicy: true, expectedDecision: policyAllowed},
		{name: "deny domain", rawURL: "https://blocked.example.com/page", fetch: false, useDomainPolicy: true, expectedDecision: policyDenied},
		{name: "outside allow list", rawURL: "https://other.test/page", fetch: false, useDomainPolicy: true, expectedDecision: policyDenied},
		{name: "metadata hostname", rawURL: "https://metadata.google.internal/compute", fetch: true, expectedDecision: policyDenied},
		{name: "metadata ip", rawURL: "http://169.254.169.254/latest/meta-data", fetch: true, expectedDecision: policyDenied},
		{name: "loopback ipv4", rawURL: "http://127.0.0.1/private", fetch: true, expectedDecision: policyApproval},
		{name: "loopback ipv6", rawURL: "http://[::1]/private", fetch: true, expectedDecision: policyApproval},
		{name: "private ipv4", rawURL: "https://10.0.0.1/private", fetch: true, expectedDecision: policyApproval},
		{name: "userinfo", rawURL: "https://user:pass@example.com/private", fetch: true, expectedDecision: policyDenied},
		{name: "unsupported scheme", rawURL: "file:///etc/passwd", fetch: true, expectedDecision: policyDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProvider := provider
			if !tt.useDomainPolicy {
				testProvider.AllowDomains = nil
				testProvider.DenyDomains = nil
			}
			decision := adapter.classifyURL(context.Background(), tt.rawURL, testProvider, tt.fetch)
			if decision.Decision != tt.expectedDecision {
				t.Fatalf("decision = %+v, want %s", decision, tt.expectedDecision)
			}
		})
	}
}

func TestRuntimeAdapterProviderResponseSizeLimitIsStableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"too-large","url":"https://example.com/large","snippet":"0123456789"}]}`))
	}))
	defer server.Close()
	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]ProviderConfig{
		"http": {
			ID:             "http",
			Provider:       webcommon.ProviderHTTP,
			SearchEndpoint: server.URL,
			Timeout:        time.Second,
			MaxResults:     2,
			MaxFetchBytes:  16,
		},
	}, nil, server.Client())

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:  webcommon.StepKindSearch,
		Input: map[string]any{"query": "too large"},
	})
	if err == nil {
		t.Fatalf("execute search err = nil, want content too large")
	}
	if result.Output["status"] != "failed" || result.Output["errorCode"] != webcommon.ErrorContentTooLarge {
		t.Fatalf("output = %+v, want content_too_large", result.Output)
	}
}

func TestRuntimeAdapterProviderTimeoutIsStableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()
	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]ProviderConfig{
		"http": {
			ID:             "http",
			Provider:       webcommon.ProviderHTTP,
			SearchEndpoint: server.URL,
			Timeout:        time.Millisecond,
			MaxResults:     2,
			MaxFetchBytes:  4096,
		},
	}, nil, server.Client())

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:  webcommon.StepKindSearch,
		Input: map[string]any{"query": "timeout"},
	})
	if err == nil {
		t.Fatalf("execute search err = nil, want timeout")
	}
	if result.Output["status"] != "failed" || result.Output["errorCode"] != webcommon.ErrorTimeout {
		t.Fatalf("output = %+v, want timeout", result.Output)
	}
}

func TestRuntimeAdapterSearchRejectsBadGatewayShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"missing url"}]}`))
	}))
	defer server.Close()
	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]ProviderConfig{
		"http": {
			ID:             "http",
			Provider:       webcommon.ProviderHTTP,
			SearchEndpoint: server.URL,
			Timeout:        time.Second,
			MaxFetchBytes:  4096,
		},
	}, nil, server.Client())

	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:  webcommon.StepKindSearch,
		Input: map[string]any{"query": "bad shape"},
	})
	if err == nil {
		t.Fatalf("execute search err = nil, want bad gateway response")
	}
	if result.Output["status"] != "failed" || result.Output["errorCode"] != webcommon.ErrorBadGatewayResponse {
		t.Fatalf("output = %+v, want bad gateway response", result.Output)
	}
}

func TestRuntimeAdapterReloadsProviderConfigByRevision(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"First","url":"https://example.com/first","snippet":"first"}]}`))
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"Second","url":"https://example.com/second","snippet":"second"}]}`))
	}))
	defer second.Close()

	revision := uint64(1)
	loadCount := 0
	adapter := NewRuntimeAdapterFromConfigLoaderWithRevision(func() (string, map[string]ProviderConfig, error) {
		loadCount++
		endpoint := first.URL
		if revision > 1 {
			endpoint = second.URL
		}
		return "http", map[string]ProviderConfig{
			"http": {
				ID:             "http",
				Provider:       webcommon.ProviderHTTP,
				SearchEndpoint: endpoint,
				Timeout:        time.Second,
				MaxResults:     2,
				MaxFetchBytes:  4096,
			},
		}, nil
	}, func() uint64 {
		return revision
	}, nil)
	adapter.client = first.Client()

	for i := 0; i < 2; i++ {
		result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
			Kind:  webcommon.StepKindSearch,
			Input: map[string]any{"query": "reload"},
		})
		if err != nil {
			t.Fatalf("execute search before reload: %v", err)
		}
		results := result.Output["results"].([]map[string]any)
		if results[0]["title"] != "First" {
			t.Fatalf("results = %+v, want cached first provider", results)
		}
	}
	if loadCount != 1 {
		t.Fatalf("loadCount = %d, want cached config for same revision", loadCount)
	}

	revision = 2
	adapter.client = second.Client()
	result, err := adapter.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:  webcommon.StepKindSearch,
		Input: map[string]any{"query": "reload"},
	})
	if err != nil {
		t.Fatalf("execute search after reload: %v", err)
	}
	results := result.Output["results"].([]map[string]any)
	if results[0]["title"] != "Second" || loadCount != 2 {
		t.Fatalf("results/loadCount = %+v/%d, want reloaded second provider", results, loadCount)
	}
}
