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
