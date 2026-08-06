package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestCatalogServiceMergesConcurrentRefreshPerKey(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{{
				"id": "gpt-4o", "object": "model", "owned_by": "openai",
				"operations":          []string{"chat_completions"},
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
				"operations":          []string{"chat_completions"},
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

func TestBuildCatalogFromWirePreservesCaseDistinctModelIDs(t *testing.T) {
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "DeepSeek-V4-Flash"}

	catalog, err := buildCatalogFromWire(aiProxyModelsResponse{Object: "list", Data: []aiProxyModelRecord{
		{ID: "DeepSeek-V4-Flash", OwnedBy: "proxy", Operations: []string{"chat_completions"}, ContextWindowTokens: 1000000, MaxOutputTokens: 65536},
		{ID: "deepseek-v4-flash", OwnedBy: "proxy", Operations: []string{"chat_completions"}, ContextWindowTokens: 1000000, MaxOutputTokens: 65536},
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
		{ID: "intfloat/multilingual-e5-small", SupportedEndpoints: []string{"/v1/embeddings"}},
	}}, cfg, time.Now())
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}
	if len(catalog.Models) != 2 || len(catalog.Models[0].SupportedEndpoints) == 0 {
		t.Fatalf("catalog = %+v", catalog.Models)
	}
	var responsesReady bool
	for _, model := range catalog.Models {
		if model.Target.ModelID == "gpt-5.5" {
			responsesReady = model.CapabilityReady && containsOperation(model.Operations, workorchconfig.LLMOperationResponses)
		}
	}
	if !responsesReady {
		t.Fatalf("responses model not ready: %+v", catalog.Models)
	}
}
