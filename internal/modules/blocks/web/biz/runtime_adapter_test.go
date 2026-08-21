package web

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeWebGrant(requirement runtimecontract.RuntimeGovernanceRequirement) *runtimecontract.RuntimeOperationGovernanceGrant {
	requirement = requirement.Normalized()
	return &runtimecontract.RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-web", ApprovalID: "approval-web", ExecutionKey: "execution-web",
		GovernanceFence: 1, ResumeFence: 2, DependencyKind: string(requirement.Kind),
		SubjectRef: requirement.SubjectRef, RequestedAction: requirement.RequestedAction, RiskLevel: requirement.RiskLevel,
		AuthorizationHash:     runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext),
		GovernanceReceiptHash: "receipt-web",
	}
}

func testWebProvider(endpoint string) webmodels.ProviderConfig {
	return webmodels.ProviderConfig{
		ID: "http", Provider: webcommon.ProviderHTTP, SearchEndpoint: endpoint,
		Timeout: time.Second, MaxResults: 4, MaxFetchBytes: 4096, MaxRedirects: 2,
	}
}

func TestRuntimeAdapterSearchReturnsRecoverableBoundedEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["query"] != "golang latest" || payload["maxResults"].(float64) != 2 {
			t.Fatalf("payload = %+v", payload)
		}
		_, _ = w.Write([]byte(`{"results":[
			{"title":"Go Downloads","url":"https://go.dev/dl/","snippet":"Download Go","source":"go.dev"},
			{"title":"Release notes","url":"https://go.dev/doc/devel/release","snippet":"Release history","source":"go.dev"}
		]}`))
	}))
	defer server.Close()
	internalDirectory := t.TempDir()
	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]webmodels.ProviderConfig{"http": testWebProvider(server.URL)}, server.Client())

	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind: webcommon.StepKindSearch, Query: "golang latest", MaxResults: 2,
		WorkspaceInternalDirectory: internalDirectory,
	})
	if err != nil {
		t.Fatalf("execute search: %v", err)
	}
	if result.Status != "completed" || result.ResultCount != 2 || !strings.Contains(result.Evidence.Summary, "Download Go") || result.Evidence.ContentHash == "" {
		t.Fatalf("result = %+v", result)
	}
	if result.Evidence.Quality != common.EvidenceQualityValid || len(result.Evidence.EvidenceRefs) != 3 {
		t.Fatalf("evidence = %+v", result.Evidence)
	}
	assertPersistedWebEvidence(t, internalDirectory, result.Evidence.EvidenceRefs)
}

func TestRuntimeAdapterSearchLargeResultSetRequiresExactGrantBeforeProviderCall(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"results":[{"title":"One","url":"https://example.com/1","snippet":"one"}]}`))
	}))
	defer server.Close()
	provider := testWebProvider(server.URL)
	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]webmodels.ProviderConfig{"http": provider}, server.Client())
	request := RuntimeOperationRequest{
		WorkspaceID: "ws-search", RunID: "run-search", StepID: "step-search",
		Kind: webcommon.StepKindSearch, Query: "wide search", MaxResults: 12,
		WorkspaceInternalDirectory: t.TempDir(),
	}
	blocked, err := adapter.ExecuteOperation(context.Background(), request)
	if err == nil || blocked.Governance == nil || blocked.FailureCode != webcommon.ErrorApprovalRequired || calls.Load() != 0 {
		t.Fatalf("blocked=%+v err=%v calls=%d", blocked, err, calls.Load())
	}
	request.GovernanceGrant = runtimeWebGrant(*blocked.Governance)
	completed, err := adapter.ExecuteOperation(context.Background(), request)
	if err != nil || completed.Status != "completed" || calls.Load() != 1 {
		t.Fatalf("completed=%+v err=%v calls=%d", completed, err, calls.Load())
	}

	request.MaxResults = 4
	if _, err := adapter.ExecuteOperation(context.Background(), request); err == nil || calls.Load() != 1 {
		t.Fatalf("stale grant reached provider: err=%v calls=%d", err, calls.Load())
	}
}

func TestRuntimeAdapterFetchBlocksLoopbackUntilExactGrant(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Private</title></head><body>bounded private content</body></html>`))
	}))
	defer server.Close()
	internalDirectory := t.TempDir()
	provider := testWebProvider(server.URL)
	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]webmodels.ProviderConfig{"http": provider}, server.Client())
	request := RuntimeOperationRequest{
		WorkspaceID: "ws-fetch", RunID: "run-fetch", StepID: "step-fetch",
		Kind: webcommon.StepKindFetch, URL: server.URL + "/private",
		WorkspaceInternalDirectory: internalDirectory,
	}
	blocked, err := adapter.ExecuteOperation(context.Background(), request)
	if err == nil || blocked.Governance == nil || calls.Load() != 0 {
		t.Fatalf("blocked=%+v err=%v calls=%d", blocked, err, calls.Load())
	}
	request.GovernanceGrant = runtimeWebGrant(*blocked.Governance)
	completed, err := adapter.ExecuteOperation(context.Background(), request)
	if err != nil || completed.Status != "completed" || calls.Load() != 1 {
		t.Fatalf("completed=%+v err=%v calls=%d", completed, err, calls.Load())
	}
	if !strings.Contains(completed.Evidence.Summary, "bounded private content") || len(completed.Evidence.EvidenceRefs) != 2 {
		t.Fatalf("evidence=%+v", completed.Evidence)
	}
	assertPersistedWebEvidence(t, internalDirectory, completed.Evidence.EvidenceRefs)
}

func TestRuntimeAdapterFetchChecksRedirectTargetBeforeAccess(t *testing.T) {
	var initialCalls atomic.Int64
	var redirectedCalls atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedCalls.Add(1)
		_, _ = w.Write([]byte("must not be reached"))
	}))
	defer target.Close()
	initial := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		initialCalls.Add(1)
		http.Redirect(w, r, target.URL+"/private", http.StatusTemporaryRedirect)
	}))
	defer initial.Close()
	provider := testWebProvider(initial.URL)
	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]webmodels.ProviderConfig{"http": provider}, initial.Client())
	request := RuntimeOperationRequest{
		Kind: webcommon.StepKindFetch, URL: initial.URL + "/start", WorkspaceInternalDirectory: t.TempDir(),
	}
	initialPolicy := adapter.classifyURL(context.Background(), request.URL, provider, true)
	initialRequirement := webFetchApprovalRequirement(provider, request.URL, provider.MaxFetchBytes, "", initialPolicy)
	request.GovernanceGrant = runtimeWebGrant(initialRequirement)

	blocked, err := adapter.ExecuteOperation(context.Background(), request)
	if err == nil || blocked.Governance == nil || blocked.FinalURL != target.URL+"/private" {
		t.Fatalf("blocked=%+v err=%v", blocked, err)
	}
	if initialCalls.Load() != 1 || redirectedCalls.Load() != 0 {
		t.Fatalf("initial/redirected calls = %d/%d", initialCalls.Load(), redirectedCalls.Load())
	}
}

type countingResolver struct {
	calls atomic.Int64
	first net.IP
	later net.IP
}

func (r *countingResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	if r.calls.Add(1) == 1 {
		return []net.IPAddr{{IP: append(net.IP(nil), r.first...)}}, nil
	}
	return []net.IPAddr{{IP: append(net.IP(nil), r.later...)}}, nil
}

func TestRuntimeAdapterFetchPinsOneDNSResolutionToTransport(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("pinned response"))
	}))
	defer server.Close()
	resolver := &countingResolver{first: net.ParseIP("127.0.0.1"), later: net.ParseIP("169.254.169.254")}
	provider := testWebProvider(server.URL)
	provider.AllowDomains = []string{"fetch.test"}
	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]webmodels.ProviderConfig{"http": provider}, server.Client())
	adapter.resolver = resolver
	targetURL := strings.Replace(server.URL, "127.0.0.1", "fetch.test", 1) + "/page"
	policy := policyDecision{Decision: policyApproval, Reasons: []string{"loopback", "plain_http"}}
	requirement := webFetchApprovalRequirement(provider, targetURL, provider.MaxFetchBytes, "", policy)
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
		Kind: webcommon.StepKindFetch, URL: targetURL, GovernanceGrant: runtimeWebGrant(requirement),
		WorkspaceInternalDirectory: t.TempDir(),
	})
	if err != nil || result.Status != "completed" || calls.Load() != 1 || resolver.calls.Load() != 1 {
		t.Fatalf("result=%+v err=%v calls=%d resolutions=%d", result, err, calls.Load(), resolver.calls.Load())
	}
}

func TestSafeFetchRedirectLimitFailsBeforeFollowingNextRequest(t *testing.T) {
	provider := testWebProvider("https://provider.invalid/search")
	provider.MaxRedirects = 1
	request := RuntimeOperationRequest{Kind: webcommon.StepKindFetch, URL: "http://127.0.0.1/page", MaxBytes: 128}
	requirement := webFetchApprovalRequirement(provider, request.URL, request.MaxBytes, "", policyDecision{Decision: policyApproval, Reasons: []string{"loopback", "plain_http"}})
	request.GovernanceGrant = runtimeWebGrant(requirement)
	plan := newSafeFetchPlan(NewRuntimeAdapter("http", map[string]webmodels.ProviderConfig{"http": provider}), provider, request)
	next, err := http.NewRequestWithContext(context.Background(), http.MethodGet, request.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	via := []*http.Request{{}, {}}
	if err := plan.checkRedirect(next, via); err == nil || !strings.Contains(err.Error(), "redirect limit") {
		t.Fatalf("redirect limit err=%v", err)
	}
}

func TestRuntimeAdapterURLPolicyCoversSSRFBoundaries(t *testing.T) {
	adapter := NewRuntimeAdapter("http", nil)
	provider := webmodels.ProviderConfig{ID: "http", Provider: webcommon.ProviderHTTP, AllowDomains: []string{"example.com"}, DenyDomains: []string{"blocked.example.com"}}
	tests := []struct {
		name, rawURL, expected string
		fetch, domainPolicy    bool
	}{
		{name: "allowed domain", rawURL: "https://docs.example.com/page", domainPolicy: true, expected: policyAllowed},
		{name: "deny domain", rawURL: "https://blocked.example.com/page", domainPolicy: true, expected: policyDenied},
		{name: "outside allow list", rawURL: "https://other.test/page", domainPolicy: true, expected: policyDenied},
		{name: "metadata hostname", rawURL: "https://metadata.google.internal/compute", fetch: true, expected: policyDenied},
		{name: "metadata ip", rawURL: "http://169.254.169.254/latest/meta-data", fetch: true, expected: policyDenied},
		{name: "loopback ipv4", rawURL: "http://127.0.0.1/private", fetch: true, expected: policyApproval},
		{name: "loopback ipv6", rawURL: "http://[::1]/private", fetch: true, expected: policyApproval},
		{name: "private ipv4", rawURL: "https://10.0.0.1/private", fetch: true, expected: policyApproval},
		{name: "userinfo", rawURL: "https://user:pass@example.com/private", fetch: true, expected: policyDenied},
		{name: "unsupported scheme", rawURL: "file:///etc/passwd", fetch: true, expected: policyDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testProvider := provider
			if !tt.domainPolicy {
				testProvider.AllowDomains, testProvider.DenyDomains = nil, nil
			}
			if decision := adapter.classifyURL(context.Background(), tt.rawURL, testProvider, tt.fetch); decision.Decision != tt.expected {
				t.Fatalf("decision=%+v want=%s", decision, tt.expected)
			}
		})
	}
}

func TestRuntimeAdapterProviderFailuresAreBoundedAndTransportSafe(t *testing.T) {
	t.Run("response size", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"results":[{"title":"too-large","url":"https://example.com/large","snippet":"0123456789"}]}`))
		}))
		defer server.Close()
		provider := testWebProvider(server.URL)
		provider.MaxFetchBytes = 16
		adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]webmodels.ProviderConfig{"http": provider}, server.Client())
		result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{Kind: webcommon.StepKindSearch, Query: "too large", WorkspaceInternalDirectory: t.TempDir()})
		if err == nil || result.FailureCode != webcommon.ErrorContentTooLarge {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	})

	t.Run("timeout hides endpoint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(50 * time.Millisecond)
			_, _ = w.Write([]byte(`{"results":[]}`))
		}))
		defer server.Close()
		provider := testWebProvider(server.URL)
		provider.Timeout = time.Millisecond
		adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]webmodels.ProviderConfig{"http": provider}, server.Client())
		result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{Kind: webcommon.StepKindSearch, Query: "timeout", WorkspaceInternalDirectory: t.TempDir()})
		if err == nil || result.FailureCode != webcommon.ErrorTimeout || strings.Contains(err.Error(), server.URL) {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	})

	t.Run("cross origin redirect", func(t *testing.T) {
		var targetCalls atomic.Int64
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1) }))
		defer target.Close()
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", target.URL)
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		defer origin.Close()
		adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]webmodels.ProviderConfig{"http": testWebProvider(origin.URL)}, origin.Client())
		result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{Kind: webcommon.StepKindSearch, Query: "redirect", WorkspaceInternalDirectory: t.TempDir()})
		if err == nil || targetCalls.Load() != 0 || result.FailureCode != webcommon.ErrorBadGatewayResponse || strings.Contains(err.Error(), target.URL) {
			t.Fatalf("result=%+v err=%v targetCalls=%d", result, err, targetCalls.Load())
		}
	})
}

func TestRuntimeAdapterSearchRejectsBadGatewayShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"title":"missing url"}]}`))
	}))
	defer server.Close()
	adapter := NewRuntimeAdapterWithHTTPClient("http", map[string]webmodels.ProviderConfig{"http": testWebProvider(server.URL)}, server.Client())
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{Kind: webcommon.StepKindSearch, Query: "bad shape", WorkspaceInternalDirectory: t.TempDir()})
	if err == nil || result.FailureCode != webcommon.ErrorBadGatewayResponse {
		t.Fatalf("result=%+v err=%v", result, err)
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
	adapter := NewRuntimeAdapterFromConfigLoaderWithRevision(func() (string, map[string]webmodels.ProviderConfig, error) {
		loadCount++
		endpoint := first.URL
		if revision > 1 {
			endpoint = second.URL
		}
		return "http", map[string]webmodels.ProviderConfig{"http": testWebProvider(endpoint)}, nil
	}, func() uint64 { return revision })
	adapter.client = first.Client()
	internalDirectory := t.TempDir()
	for range 2 {
		result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{Kind: webcommon.StepKindSearch, Query: "reload", WorkspaceInternalDirectory: internalDirectory})
		if err != nil || result.Results[0].Title != "First" {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
	if loadCount != 1 {
		t.Fatalf("loadCount=%d", loadCount)
	}
	revision = 2
	adapter.client = second.Client()
	result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{Kind: webcommon.StepKindSearch, Query: "reload", WorkspaceInternalDirectory: internalDirectory})
	if err != nil || result.Results[0].Title != "Second" || loadCount != 2 {
		t.Fatalf("result=%+v err=%v loadCount=%d", result, err, loadCount)
	}
}

func TestRuntimeAdapterOperationAdmissibilityTracksConfigAndProbeAvoidsHTTP(t *testing.T) {
	revision := uint64(1)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer server.Close()
	adapter := NewRuntimeAdapterFromConfigLoaderWithRevision(func() (string, map[string]webmodels.ProviderConfig, error) {
		provider := testWebProvider(server.URL)
		provider.MaxResults = int(revision) + 1
		return "http", map[string]webmodels.ProviderConfig{"http": provider}, nil
	}, func() uint64 { return revision })
	adapter.client = server.Client()
	ids, firstRevision, err := adapter.OperationAdmissibility()
	if err != nil || len(ids) != 1 || firstRevision == "" {
		t.Fatalf("ids=%v revision=%q err=%v", ids, firstRevision, err)
	}
	if err := adapter.ProbeOperation(context.Background(), RuntimeOperationRequest{Kind: webcommon.StepKindSearch, Query: "runtime", ProviderID: "http"}); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatalf("probe requests=%d", requests.Load())
	}
	if err := adapter.ProbeOperation(context.Background(), RuntimeOperationRequest{Kind: webcommon.StepKindFetch, URL: "http://169.254.169.254/latest", ProviderID: "http"}); err == nil {
		t.Fatal("metadata URL passed probe")
	}
	revision++
	_, secondRevision, err := adapter.OperationAdmissibility()
	if err != nil || secondRevision == firstRevision {
		t.Fatalf("first=%q second=%q err=%v", firstRevision, secondRevision, err)
	}
}

func assertPersistedWebEvidence(t *testing.T, internalDirectory string, refs []string) {
	t.Helper()
	for _, ref := range refs {
		const prefix = "workorch://web/evidence/"
		if !strings.HasPrefix(ref, prefix) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(internalDirectory, "web", "evidence", strings.TrimPrefix(ref, prefix)+".json"))
		if err != nil || !strings.Contains(string(raw), webEvidenceSchemaVersion) {
			t.Fatalf("persisted evidence ref=%q err=%v raw=%q", ref, err, raw)
		}
		return
	}
	t.Fatalf("no persisted Web evidence ref in %v", refs)
}
