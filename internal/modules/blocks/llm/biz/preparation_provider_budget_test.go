package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPreparationProviderBudgetReusesExactResolverWithoutHTTP(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	cfg := catalogServiceTestConfig("http://private-budget.invalid")
	cfg.AIProxy.APIKey = "budget-credential-canary"
	cfg.AIProxy.ModelOverrides = map[string]workorchconfig.AIProxyModelOverrideConfig{"override": {ContextWindowTokens: 64000, MaxOutputTokens: 8000, Operations: []string{"responses"}}}
	service := NewCatalogService()
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	key := seedCachedPreparationCatalog(t, service, cfg, now)
	var calls atomic.Int32
	service.client = &http.Client{Transport: lifecycleRoundTripper(func(*http.Request) (*http.Response, error) { calls.Add(1); return nil, errors.New("HTTP forbidden") })}
	owner, err := New(hub, nil, Dependencies{CatalogService: service, LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return cfg, nil }})
	if err != nil || owner.Start() != nil {
		t.Fatal("owner start", err)
	}
	defer owner.Teardown(ctx)
	for _, model := range []string{"model-a", "override", "missing"} {
		target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: model}
		c := llmevents.ObserveProviderCatalogCacheCommand{WorkspaceID: "ws", Target: &target}
		ev := magicevent.NewEventWithContext(llmevents.TopicObserveProviderCatalogCache, llmApplicationOwnerSource, ModuleID, nil, ctx, c)
		result := hub.Send(ev)
		if result == nil || result.Error() != nil {
			t.Fatal("observation unavailable", result)
		}
		value, resultErr := result.Get()
		r, ok := value.(llmevents.ObserveProviderCatalogCacheResult)
		if resultErr != nil || !ok || r.ValidateFor(c) != nil || r.Budget == nil || r.Budget.Target != target {
			t.Fatal("missing exact budget", resultErr)
		}
		if model == "missing" {
			if r.Budget.FailureCode != "capacity_unavailable" {
				t.Fatal("missing model borrowed default capacity")
			}
			continue
		}
		p, err := ResolveProviderModelTarget(cfg, &r.Catalog, target, workorchconfig.LLMOperationResponses)
		b := r.Budget
		if err != nil || b.FailureCode != "" || b.ContextWindowTokens != p.ContextWindowTokens || b.ProviderMaxOutputTokens != p.ProviderMaxOutputTokens || b.EffectiveMaxOutputTokens != p.EffectiveMaxOutputTokens || b.MaxInputTokens != p.MaxInputTokens || b.MaxRequestBytes != p.MaxRequestBytes || b.EstimatedTokensPerByte != p.EstimatedTokensPerByte || b.CapabilityRevision != p.CapabilityRevision {
			t.Fatal("capacity diverged from production resolver", err)
		}
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{cfg.AIProxy.APIKey, cfg.AIProxy.BaseURL, p.ConfigurationRevision, key.ConnectionRevision} {
			if strings.Contains(string(raw), private) {
				t.Fatal("private configuration identity escaped")
			}
		}
		copy := r.Clone()
		copy.Budget.MaxInputTokens++
		if copy.Budget.MaxInputTokens == r.Budget.MaxInputTokens {
			t.Fatal("budget result aliases original")
		}
		other := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "foreign"}
		if r.ValidateFor(llmevents.ObserveProviderCatalogCacheCommand{WorkspaceID: "ws", Target: &other}) == nil {
			t.Fatal("foreign request borrowed exact budget")
		}
	}
	now = now.Add(2 * time.Hour)
	r, err := owner.observeProviderCatalogCache(ctx, llmevents.ObserveProviderCatalogCacheCommand{WorkspaceID: "ws"})
	if err != nil || r.Budget != nil || calls.Load() != 0 {
		t.Fatal("expired cache retained capacity or performed HTTP", err)
	}
}
