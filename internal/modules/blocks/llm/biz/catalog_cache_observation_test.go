package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func seedCachedPreparationCatalog(t *testing.T, service *CatalogService, cfg workorchconfig.LLMConfig, fetchedAt time.Time) catalogCacheKey {
	t.Helper()
	cfg, err := workorchconfig.NormalizeLLMConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var wire aiProxyModelsResponse
	if err := json.Unmarshal([]byte(`{"object":"list","data":[{"id":"model-a","supported_endpoints":["/v1/responses"],"contextWindowTokens":8192,"maxOutputTokens":1024}]}`), &wire); err != nil {
		t.Fatal(err)
	}
	catalog, err := buildCatalogFromWire(wire, cfg, fetchedAt)
	if err != nil {
		t.Fatal(err)
	}
	key := catalogCacheKeyFromConfig(cfg)
	service.entries[key] = &catalogCacheEntry{key: key, connectionRev: key.ConnectionRevision, catalog: catalogCacheValue(catalog)}
	return key
}

func TestPreparationCatalogCacheObservationNeverRefreshesWaitsOrEvicts(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	cfg := catalogServiceTestConfig("http://provider.invalid")
	cfg.AIProxy.APIKey = "fixture-credential-do-not-export"
	service := NewCatalogService()
	service.now = func() time.Time { return now }
	var calls atomic.Int32
	service.client = &http.Client{Transport: lifecycleRoundTripper(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("network must not be used")
	})}
	missing, observedAt, err := service.observeCache(ctx, cfg)
	if err != nil || missing.State != llmmodels.ProviderCatalogStateUnavailable || missing.ErrorCode != "cache_miss" || observedAt != now || len(service.entries) != 0 || len(service.inflight) != 0 {
		t.Fatalf("cache miss observation: %+v %v", missing, err)
	}
	key := seedCachedPreparationCatalog(t, service, cfg, now)
	before := *service.entries[key]
	// An unfinished refresh must neither be joined nor count as a fresh fact.
	service.inflight[key] = &catalogRefreshCall{key: key, done: make(chan struct{})}
	for _, delta := range []time.Duration{0, time.Minute, 2 * time.Hour} {
		now = before.catalog.FetchedAt.Add(delta)
		catalog, at, err := service.observeCache(ctx, cfg)
		if err != nil || at != now || !reflect.DeepEqual(before, *service.entries[key]) || len(service.inflight) != 1 {
			t.Fatalf("cache observation changed cache/inflight: %v", err)
		}
		want := llmmodels.ProviderCatalogStateFresh
		if delta >= time.Minute {
			want = llmmodels.ProviderCatalogStateStale
		}
		if catalog.State != want || catalog.ErrorMessage != "" || catalog.Revision != before.catalog.Revision || len(catalog.Models) != len(before.catalog.Models) {
			t.Fatalf("wrong freshness or incomplete cache: %+v", catalog)
		}
		catalog.Models[0].Operations[0] = "caller mutation"
		if !reflect.DeepEqual(before, *service.entries[key]) {
			t.Fatal("observation aliases stored capability declarations")
		}
	}
	rotated := workorchconfig.CloneLLMConfig(cfg)
	rotated.AIProxy.APIKey = "same-length-rotated-credential"
	if catalog, _, err := service.observeCache(ctx, rotated); err != nil || catalog.ErrorCode != "cache_miss" {
		t.Fatal("rotation reused a different credential's cache")
	}
	if calls.Load() != 0 {
		t.Fatalf("cache-only observation made %d HTTP calls", calls.Load())
	}
}

func TestPreparationCatalogCacheOwnerTypedLifecycleAndNoSecretIdentity(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	cfg := catalogServiceTestConfig("http://private-endpoint.invalid")
	cfg.AIProxy.APIKey = "fixture-secret-canary-do-not-export"
	cfg.AIProxy.ModelOverrides = map[string]workorchconfig.AIProxyModelOverrideConfig{
		"override-model": {ContextWindowTokens: 32000, MaxOutputTokens: 4000, Operations: []string{"responses"}},
	}
	service := NewCatalogService()
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	key := seedCachedPreparationCatalog(t, service, cfg, now)
	var calls atomic.Int32
	service.client = &http.Client{Transport: lifecycleRoundTripper(func(*http.Request) (*http.Response, error) { calls.Add(1); return nil, errors.New("forbidden") })}
	owner, err := New(hub, nil, Dependencies{CatalogService: service, LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return cfg, nil }})
	if err != nil || owner.Start() != nil {
		t.Fatalf("start: %v", err)
	}
	defer owner.Teardown(ctx)
	command := llmevents.ObserveProviderCatalogCacheCommand{WorkspaceID: "ws-cache"}
	query := func() llmevents.ObserveProviderCatalogCacheResult {
		t.Helper()
		event := magicevent.NewEventWithContext(llmevents.TopicObserveProviderCatalogCache, llmApplicationOwnerSource, ModuleID, nil, ctx, command)
		result := hub.Send(event)
		if result == nil || result.Error() != nil {
			t.Fatalf("cache query: %+v", result)
		}
		value, err := result.Get()
		response, ok := value.(llmevents.ObserveProviderCatalogCacheResult)
		if err != nil || !ok || response.ValidateFor(command) != nil {
			t.Fatalf("invalid cache query result: %v", err)
		}
		return response
	}
	first := query()
	if first.Configuration == nil || first.Configuration.DefaultChat != cfg.DefaultChat.ToRuntimeTarget() || len(first.Configuration.Overrides) != 1 || first.Configuration.Overrides[0].ModelID != "override-model" {
		t.Fatal("frozen selection declarations missing")
	}
	// Neither the loader's mutable configuration nor a returned nested slice
	// may alter the owner lifecycle's original selection source.
	cfg.DefaultChat.ModelID = "loader-mutation"
	cfg.AIProxy.ModelOverrides["override-model"].Operations[0] = "embeddings"
	detached := first.Clone()
	detached.Configuration.Overrides[0].Operations[0] = "caller-mutation"
	next := query()
	if next.Configuration.DefaultChat != first.Configuration.DefaultChat || next.Configuration.Overrides[0].Operations[0] != "responses" {
		t.Fatal("selection source was not frozen/detached")
	}
	encoded, _ := json.Marshal(first)
	for _, forbidden := range []string{cfg.AIProxy.APIKey, cfg.AIProxy.BaseURL, key.ConnectionRevision, owner.activeLLMConfigurationRevision, "apiKeyFingerprint", "target-config-sha256:"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("preparation observation exported private configuration identity")
		}
	}
	first.Catalog.Models[0].SupportedEndpoints[0] = "caller mutation"
	if query().Catalog.Models[0].SupportedEndpoints[0] != "/v1/responses" {
		t.Fatal("owner result aliases cached endpoint facts")
	}
	owner.BeginShutdown(ctx)
	if query().OwnerEpoch != first.OwnerEpoch {
		t.Fatal("drain changed configuration epoch")
	}
	if err := owner.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	if owner.preparationConfigurationEpoch != "" || calls.Load() != 0 {
		t.Fatal("teardown retained epoch or observation performed HTTP")
	}
	// A new owner has a new epoch even with identical frozen configuration.
	second, err := New(hub, nil, Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return cfg, nil }})
	if err != nil || second.Start() != nil {
		t.Fatal("replacement owner failed to start")
	}
	defer second.Teardown(ctx)
	if second.preparationConfigurationEpoch == first.OwnerEpoch {
		t.Fatal("restart reused a prior observation epoch")
	}
	missing, err := second.observeProviderCatalogCache(ctx, command)
	if err != nil || missing.Catalog.ErrorCode != "cache_miss" {
		t.Fatal("restart recreated cache facts from old observation")
	}
}

func TestPreparationCatalogCacheRejectsCorruptionCancellationAndConcurrentReadAliasing(t *testing.T) {
	ctx := context.Background()
	cfg := catalogServiceTestConfig("http://provider.invalid")
	service := NewCatalogService()
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	key := seedCachedPreparationCatalog(t, service, cfg, now)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := service.observeCache(cancelled, cfg); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled cache read was accepted")
	}
	var wg sync.WaitGroup
	for index := 0; index < 16; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, _, err := service.observeCache(ctx, cfg)
			if err != nil {
				t.Error(err)
				return
			}
			value.Models[0].Operations[0] = "detached"
		}()
	}
	wg.Wait()
	service.entries[key].catalog.Revision += "corrupt"
	if _, _, err := service.observeCache(ctx, cfg); err == nil {
		t.Fatal("corrupt complete catalog identity was accepted")
	}
}

func TestPreparationCatalogRejectsStoredDiagnosticsWithoutRewritingThem(t *testing.T) {
	for _, diagnostic := range []string{"state", "stale", "code", "message"} {
		t.Run(diagnostic, func(t *testing.T) {
			now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
			cfg := catalogServiceTestConfig("http://provider.invalid")
			service := NewCatalogService()
			service.now = func() time.Time { return now }
			key := seedCachedPreparationCatalog(t, service, cfg, now)
			catalog := &service.entries[key].catalog
			switch diagnostic {
			case "state":
				catalog.State = llmmodels.ProviderCatalogStateUnavailable
			case "stale":
				catalog.Stale = true
			case "code":
				catalog.ErrorCode = "original_failure"
			case "message":
				catalog.ErrorMessage = "original diagnostic\n原始内容"
			}
			before := (llmevents.ObserveProviderCatalogCacheResult{Catalog: *catalog}).Clone().Catalog
			value, _, err := service.observeCache(context.Background(), cfg)
			if err == nil || !reflect.DeepEqual(before, *catalog) || len(value.Models) != 0 {
				t.Fatal("unexpected diagnostic was erased/promoted or leaked into a success")
			}
		})
	}
}
