package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func catalogServiceTestConfig(baseURL string) workorchconfig.LLMConfig {
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = baseURL
	cfg.AIProxy.CatalogRequestTimeout = "2s"
	cfg.AIProxy.CatalogTTL = "1m"
	cfg.AIProxy.CatalogMaxStaleAge = "1h"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "model-a"}
	return cfg
}

func writeCatalogTestResponse(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data": []map[string]any{{
			"id": "model-a", "supported_endpoints": []string{"/v1/responses"},
			"contextWindowTokens": 8192, "maxOutputTokens": 1024,
		}},
	})
}

func TestCatalogReadinessIsDerivedAndNeverStoredInCacheValue(t *testing.T) {
	base := llmmodels.ProviderModelCatalog{
		ProviderID: "ai-proxy", Revision: "revision-1", State: llmmodels.ProviderCatalogStateStale,
		Stale: true, ErrorCode: "old_error", ErrorMessage: "old message",
	}
	stored := catalogCacheValue(base)
	if stored.State != "" || stored.Stale || stored.ErrorCode != "" || stored.ErrorMessage != "" {
		t.Fatalf("catalog cache retained derived readiness: %+v", stored)
	}
	fresh := freshCatalogObservation(stored)
	if fresh.State != llmmodels.ProviderCatalogStateFresh || fresh.Stale || fresh.ErrorCode != "" {
		t.Fatalf("fresh observation = %+v", fresh)
	}
	stale := staleCatalogObservation(stored, context.DeadlineExceeded)
	if stale.State != llmmodels.ProviderCatalogStateStale || !stale.Stale || stale.ErrorCode == "" || stale.ErrorMessage == "" {
		t.Fatalf("stale observation = %+v", stale)
	}
	if stored.State != "" || stored.Stale || stored.ErrorCode != "" {
		t.Fatalf("derived observations mutated cached catalog: %+v", stored)
	}
}

func TestCatalogServiceMergesConcurrentRefreshPerKey(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{{
				"id": "gpt-4o", "object": "model", "owned_by": "openai",
				"supported_endpoints": []string{"/v1/responses"},
				"contextWindowTokens": 128000, "maxOutputTokens": 16384,
			}},
		})
	}))
	defer server.Close()

	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = server.URL
	cfg.AIProxy.CatalogRequestTimeout = "2s"
	cfg.AIProxy.CatalogTTL = "5m"
	cfg.AIProxy.CatalogMaxStaleAge = "24h"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "gpt-4o"}

	svc := NewCatalogService()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.GetCatalog(context.Background(), cfg, true); err != nil {
				t.Errorf("GetCatalog: %v", err)
			}
		}()
	}
	wg.Wait()
	if hits.Load() != 1 {
		t.Fatalf("upstream hits = %d, want 1 concurrent merge", hits.Load())
	}
}

func TestCatalogServiceDoesNotMergeAcrossDifferentKeys(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(30 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{{
				"id": "gpt-4o", "object": "model", "owned_by": "openai",
				"supported_endpoints": []string{"/v1/responses"},
				"contextWindowTokens": 128000, "maxOutputTokens": 16384,
			}},
		})
	}))
	defer server.Close()

	cfgA := workorchconfig.DefaultLLMConfig()
	cfgA.AIProxy.BaseURL = server.URL
	cfgA.AIProxy.APIKey = "key-AAAA"
	cfgA.AIProxy.CatalogRequestTimeout = "2s"
	cfgA.AIProxy.CatalogTTL = "5m"
	cfgA.AIProxy.CatalogMaxStaleAge = "24h"
	cfgA.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "gpt-4o"}

	cfgB := cfgA
	cfgB.AIProxy.APIKey = "key-BBBB"

	svc := NewCatalogService()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := svc.GetCatalog(context.Background(), cfgA, true); err != nil {
			t.Errorf("GetCatalog A: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := svc.GetCatalog(context.Background(), cfgB, true); err != nil {
			t.Errorf("GetCatalog B: %v", err)
		}
	}()
	wg.Wait()
	if hits.Load() != 2 {
		t.Fatalf("upstream hits = %d, want 2 for different cache keys", hits.Load())
	}
}

func TestCatalogServiceAPIKeyRotationUsesNewAuthorization(t *testing.T) {
	var authorizations []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authorizations = append(authorizations, r.Header.Get("Authorization"))
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{{
				"id": "gpt-5.5", "object": "model",
				"supported_endpoints": []string{"/v1/responses"},
				"contextWindowTokens": 400000, "maxOutputTokens": 128000,
			}},
		})
	}))
	defer server.Close()

	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = server.URL
	cfg.AIProxy.APIKey = "old-key"
	cfg.AIProxy.CatalogRequestTimeout = "2s"
	cfg.AIProxy.CatalogTTL = "5m"
	cfg.AIProxy.CatalogMaxStaleAge = "24h"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "gpt-5.5"}

	svc := NewCatalogService()
	if _, err := svc.GetCatalog(context.Background(), cfg, true); err != nil {
		t.Fatalf("catalog with old key: %v", err)
	}
	cfg.AIProxy.APIKey = "new-key"
	if _, err := svc.GetCatalog(context.Background(), cfg, true); err != nil {
		t.Fatalf("catalog after key rotation: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(authorizations) != 2 || authorizations[0] != "Bearer old-key" || authorizations[1] != "Bearer new-key" {
		t.Fatalf("authorization sequence = %q, want old then new key", authorizations)
	}
}

func TestCatalogServiceTransportStatusAndBodyFailureMatrix(t *testing.T) {
	t.Run("network error retries once", func(t *testing.T) {
		var calls atomic.Int32
		svc := NewCatalogService()
		svc.client = &http.Client{Transport: llmRoundTripFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("network unavailable")
		})}
		catalog, err := svc.GetCatalog(context.Background(), catalogServiceTestConfig("http://provider.invalid"), true)
		if err == nil || calls.Load() != 2 || catalog.State != llmmodels.ProviderCatalogStateUnavailable {
			t.Fatalf("network catalog=%+v err=%v calls=%d, want unavailable after two attempts", catalog, err, calls.Load())
		}
	})

	for _, testCase := range []struct {
		name      string
		status    int
		body      string
		maxBytes  int
		wantCalls int32
		fragment  string
	}{
		{name: "5xx retries once", status: http.StatusServiceUnavailable, body: "temporarily unavailable", wantCalls: 2, fragment: "status 503"},
		{name: "4xx does not retry", status: http.StatusTooManyRequests, body: "rate limited", wantCalls: 1, fragment: "status 429"},
		{name: "malformed JSON", status: http.StatusOK, body: `{"object":`, wantCalls: 1, fragment: "decode failed"},
		{name: "oversize body", status: http.StatusOK, body: strings.Repeat("x", 65), maxBytes: 64, wantCalls: 1, fragment: "exceeds maxCatalogResponseBytes"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(testCase.status)
				_, _ = io.WriteString(w, testCase.body)
			}))
			defer server.Close()
			cfg := catalogServiceTestConfig(server.URL)
			if testCase.maxBytes > 0 {
				cfg.AIProxy.MaxCatalogResponseBytes = testCase.maxBytes
			}
			catalog, err := NewCatalogService().GetCatalog(context.Background(), cfg, true)
			if err == nil || !strings.Contains(err.Error(), testCase.fragment) || calls.Load() != testCase.wantCalls || catalog.State != llmmodels.ProviderCatalogStateUnavailable {
				t.Fatalf("catalog=%+v err=%v calls=%d, want unavailable/%q/%d", catalog, err, calls.Load(), testCase.fragment, testCase.wantCalls)
			}
		})
	}
}

func TestCatalogServiceRedirectPolicyAndStaleFallback(t *testing.T) {
	t.Run("same-origin redirect strips authorization", func(t *testing.T) {
		var redirectedAuthorization string
		server := httptest.NewServer(nil)
		server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/models" {
				http.Redirect(w, r, "/redirected-models", http.StatusTemporaryRedirect)
				return
			}
			redirectedAuthorization = r.Header.Get("Authorization")
			writeCatalogTestResponse(w)
		})
		defer server.Close()
		cfg := catalogServiceTestConfig(server.URL)
		cfg.AIProxy.APIKey = "catalog-secret"
		catalog, err := NewCatalogService().GetCatalog(context.Background(), cfg, true)
		if err != nil || catalog.State != llmmodels.ProviderCatalogStateFresh || redirectedAuthorization != "" {
			t.Fatalf("redirect catalog=%+v err=%v authorization=%q", catalog, err, redirectedAuthorization)
		}
	})

	t.Run("cross-origin redirect never reaches target", func(t *testing.T) {
		var targetCalls atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			targetCalls.Add(1)
			writeCatalogTestResponse(w)
		}))
		defer target.Close()
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+"/v1/models", http.StatusTemporaryRedirect)
		}))
		defer origin.Close()
		catalog, err := NewCatalogService().GetCatalog(context.Background(), catalogServiceTestConfig(origin.URL), true)
		if err == nil || targetCalls.Load() != 0 || catalog.State != llmmodels.ProviderCatalogStateUnavailable {
			t.Fatalf("cross-origin catalog=%+v err=%v targetCalls=%d", catalog, err, targetCalls.Load())
		}
	})

	t.Run("bounded LKG becomes stale then expires", func(t *testing.T) {
		var fail atomic.Bool
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if fail.Load() {
				http.Error(w, "private catalog outage", http.StatusServiceUnavailable)
				return
			}
			writeCatalogTestResponse(w)
		}))
		defer server.Close()
		cfg := catalogServiceTestConfig(server.URL)
		cfg.AIProxy.APIKey = "catalog-secret"
		now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
		svc := NewCatalogService()
		svc.now = func() time.Time { return now }
		fresh, err := svc.GetCatalog(context.Background(), cfg, true)
		if err != nil || fresh.State != llmmodels.ProviderCatalogStateFresh {
			t.Fatalf("initial catalog=%+v err=%v", fresh, err)
		}
		fail.Store(true)
		now = now.Add(2 * time.Minute)
		stale, err := svc.GetCatalog(context.Background(), cfg, true)
		if err != nil || stale.State != llmmodels.ProviderCatalogStateStale || !stale.Stale || strings.Contains(stale.ErrorMessage, cfg.AIProxy.APIKey) {
			t.Fatalf("stale catalog=%+v err=%v", stale, err)
		}
		now = now.Add(2 * time.Hour)
		unavailable, err := svc.GetCatalog(context.Background(), cfg, true)
		if err == nil || unavailable.State != llmmodels.ProviderCatalogStateUnavailable {
			t.Fatalf("expired catalog=%+v err=%v", unavailable, err)
		}
	})
}

func TestBuildCatalogFromWirePreservesCaseDistinctModelIDs(t *testing.T) {
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "DeepSeek-V4-Flash"}

	catalog, err := buildCatalogFromWire(aiProxyModelsResponse{Object: "list", Data: []aiProxyModelRecord{
		{ID: "DeepSeek-V4-Flash", OwnedBy: "proxy", SupportedEndpoints: []string{"/v1/responses"}, ContextWindowTokens: 1000000, MaxOutputTokens: 65536},
		{ID: "deepseek-v4-flash", OwnedBy: "proxy", SupportedEndpoints: []string{"/v1/responses"}, ContextWindowTokens: 1000000, MaxOutputTokens: 65536},
	}}, cfg, time.Now())
	if err != nil {
		t.Fatalf("buildCatalogFromWire: %v", err)
	}
	if len(catalog.Models) != 2 || catalog.Models[0].Target.ModelID == catalog.Models[1].Target.ModelID {
		t.Fatalf("case-distinct catalog models were collapsed: %+v", catalog.Models)
	}
	if catalog.Models[0].CapabilityRevision == catalog.Models[1].CapabilityRevision {
		t.Fatalf("case-distinct catalog models share capability revision: %+v", catalog.Models)
	}
}

func TestBuildCatalogFromWireDerivesResponsesAndEmbeddingOperations(t *testing.T) {
	cfg := workorchconfig.DefaultLLMConfig()
	catalog, err := buildCatalogFromWire(aiProxyModelsResponse{Object: "list", Data: []aiProxyModelRecord{
		{ID: "gpt-5.5", SupportedEndpoints: []string{"/v1/responses"}, ContextWindowTokens: 400000, MaxOutputTokens: 128000},
		{ID: "intfloat/multilingual-e5-small", SupportedEndpoints: []string{"/v1/embeddings"}, ContextWindowTokens: 512},
	}}, cfg, time.Now())
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}
	if len(catalog.Models) != 2 || len(catalog.Models[0].SupportedEndpoints) == 0 {
		t.Fatalf("catalog = %+v", catalog.Models)
	}
	var responsesReady, embeddingsReady bool
	for _, model := range catalog.Models {
		if model.Target.ModelID == "gpt-5.5" {
			responsesReady = model.CapabilityReady && containsOperation(model.Operations, workorchconfig.LLMOperationResponses)
		}
		if model.Target.ModelID == "intfloat/multilingual-e5-small" {
			embeddingsReady = model.CapabilityReady && model.MaxOutputTokens == 0 && containsOperation(model.Operations, workorchconfig.LLMOperationEmbeddings)
		}
	}
	if !responsesReady {
		t.Fatalf("responses model not ready: %+v", catalog.Models)
	}
	if !embeddingsReady {
		t.Fatalf("embedding-only model must be ready without output capacity: %+v", catalog.Models)
	}
}

func TestBuildCatalogKeepsEmbeddingDiscoveryWithoutInputCapacity(t *testing.T) {
	// Endpoint support permits bounded embeddings without inventing token capacity.
	var wire aiProxyModelsResponse
	if err := json.Unmarshal([]byte(`{"object":"list","data":[
		{"id":"intfloat/multilingual-e5-small","object":"model","supported_endpoints":["/v1/embeddings"]},
		{"id":"jina-embeddings-v2-base-code","object":"model","supported_endpoints":["/v1/embeddings"]}
	]}`), &wire); err != nil {
		t.Fatal(err)
	}
	cfg := testCatalogOutputCeilingConfig("chat-model")
	catalog, err := buildCatalogFromWire(wire, cfg, time.Now())
	if err != nil || len(catalog.Models) != 2 {
		t.Fatalf("catalog = %+v, err = %v", catalog, err)
	}
	for _, model := range catalog.Models {
		if !containsOperation(model.Operations, workorchconfig.LLMOperationEmbeddings) || !model.CapabilityReady || !model.EmbeddingReady || model.ContextWindowTokens != 0 || model.MaxOutputTokens != 0 || model.UnavailableReason != "" {
			t.Fatalf("endpoint-only model discovery/readiness = %+v", model)
		}
		profile, err := ResolveProviderModelTarget(cfg, &catalog, model.Target, workorchconfig.LLMOperationEmbeddings)
		if err != nil || profile.ContextWindowTokens != 0 || profile.MaxInputTokens != 0 || profile.MaxRequestBytes != embeddingMaxInputBytes {
			t.Fatalf("endpoint-only model bounded profile: %+v, err=%v", profile, err)
		}
	}
}

func TestCatalogSeparatesMixedModelEmbeddingReadinessFromResponses(t *testing.T) {
	var wire aiProxyModelsResponse
	if err := json.Unmarshal([]byte(`{"data":[
		{"id":"mixed","supported_endpoints":["/v1/responses","/v1/embeddings"]},
		{"id":"negative-context","supported_endpoints":["/v1/embeddings"],"contextWindowTokens":-1},
		{"id":"negative-output","supported_endpoints":["/v1/embeddings"],"maxOutputTokens":-1}
	]}`), &wire); err != nil {
		t.Fatal(err)
	}
	cfg := testCatalogOutputCeilingConfig("chat-model")
	catalog, err := buildCatalogFromWire(wire, cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range catalog.Models {
		if model.CapabilityReady {
			t.Fatalf("invalid Responses/capacity marked globally ready: %+v", model)
		}
		_, embeddingErr := ResolveProviderModelTarget(cfg, &catalog, model.Target, workorchconfig.LLMOperationEmbeddings)
		if model.Target.ModelID == "mixed" {
			if !model.EmbeddingReady || embeddingErr != nil {
				t.Fatalf("mixed embedding: %+v %v", model, embeddingErr)
			}
			if _, err := ResolveProviderModelTarget(cfg, &catalog, model.Target, workorchconfig.LLMOperationResponses); err == nil {
				t.Fatal("embedding readiness leaked to Responses")
			}
		} else if model.EmbeddingReady || embeddingErr == nil {
			t.Fatalf("negative capacity accepted: %+v %v", model, embeddingErr)
		}
	}
}

func TestBuildCatalogFromWireNormalizesReasoningCapability(t *testing.T) {
	var wire aiProxyModelsResponse
	if err := json.Unmarshal([]byte(`{
		"object":"list",
		"data":[{
			"id":"DeepSeek-V4-Flash",
			"supported_endpoints":["/v1/responses"],
			"contextWindowTokens":1000000,
			"maxOutputTokens":65536,
			"capabilities":{"reasoning":{"supported":true,"default_effort":"low","efforts":["none","low","high","high"]}},
			"future_field":{"ignored":true}
		}]
	}`), &wire); err != nil {
		t.Fatalf("decode wire: %v", err)
	}
	catalog, err := buildCatalogFromWire(wire, workorchconfig.DefaultLLMConfig(), time.Now())
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}
	if len(catalog.Models) != 1 {
		t.Fatalf("models = %+v", catalog.Models)
	}
	reasoning := catalog.Models[0].Reasoning
	if !reasoning.Supported || !reasoning.Valid || reasoning.DefaultEffort != "low" || len(reasoning.Efforts) != 3 || !reasoning.Allows("high") || reasoning.Allows("HIGH") {
		t.Fatalf("reasoning capability = %+v", reasoning)
	}
}

func TestBuildCatalogFromWireKeepsBaseReadinessWhenReasoningIsInvalid(t *testing.T) {
	var wire aiProxyModelsResponse
	if err := json.Unmarshal([]byte(`{
		"object":"list",
		"data":[{
			"id":"model-a",
			"supported_endpoints":["/v1/responses"],
			"contextWindowTokens":8192,
			"maxOutputTokens":2048,
			"capabilities":{"reasoning":{"supported":true,"default_effort":"max","efforts":["low","high"]}}
		},{
			"id":"model-b",
			"supported_endpoints":["/v1/responses"],
			"contextWindowTokens":8192,
			"maxOutputTokens":2048
		}]
	}`), &wire); err != nil {
		t.Fatalf("decode wire: %v", err)
	}
	catalog, err := buildCatalogFromWire(wire, workorchconfig.DefaultLLMConfig(), time.Now())
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}
	for _, model := range catalog.Models {
		if !model.CapabilityReady {
			t.Fatalf("base model should remain ready: %+v", model)
		}
		if model.Target.ModelID == "model-a" && (model.Reasoning.Valid || model.Reasoning.Allows("high")) {
			t.Fatalf("invalid reasoning capability = %+v", model.Reasoning)
		}
		if model.Target.ModelID == "model-b" && (model.Reasoning.Supported || !model.Reasoning.Valid || !model.Reasoning.Allows("")) {
			t.Fatalf("missing reasoning capability = %+v", model.Reasoning)
		}
	}
}
