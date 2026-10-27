package biz

import (
	"context"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestPreparationCatalogApplicationNeverUsesRefreshingCatalogFallback(t *testing.T) {
	for _, scenario := range []string{"missing", "scope", "wrong_type", "cancelled", "diagnostic", "parent_scope"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			defer hub.Terminate(ctx)
			owner := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
			calls := 0
			if err := owner.Subscribe(llmevents.TopicObserveProviderCatalogCache, func(ev magicevent.Event, result magicevent.Result) {
				calls++
				command, ok := ev.Data().(llmevents.ObserveProviderCatalogCacheCommand)
				deadline, bounded := ev.Context().Deadline()
				if !ok || command.WorkspaceID != "ws" || ev.Source() != ModuleID || !bounded || time.Until(deadline) > 2*time.Second {
					t.Error("wrong cache-only query or missing bounded context")
				}
				if scenario == "wrong_type" {
					result.Set(llmevents.GetProviderModelCatalogResult{}, nil)
					return
				}
				response := llmevents.ObserveProviderCatalogCacheResult{WorkspaceID: "ws", OwnerEpoch: "llm-preparation-epoch:ABCDEFGHIJKLMNOPQRSTUVWXYZ", ObservedAt: time.Now().UTC(), Catalog: llmmodels.ProviderModelCatalog{ProviderID: "ai-proxy", Source: "ai_proxy", State: llmmodels.ProviderCatalogStateUnavailable, ErrorCode: "cache_miss"}}
				if scenario == "scope" {
					response.WorkspaceID = "other"
				}
				if scenario == "diagnostic" {
					response.Catalog.ErrorMessage = "private diagnostic"
				}
				if scenario == "missing" && response.ValidateFor(command) != nil {
					t.Fatal("invalid successful cache-miss fixture")
				}
				result.Set(response, nil)
			}); err != nil {
				t.Fatal(err)
			}
			defer owner.Unsubscribe(llmevents.TopicObserveProviderCatalogCache)
			b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			queryCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			if scenario == "cancelled" {
				cancel()
			}
			if scenario == "parent_scope" {
				queryCtx, _ = common.BindRuntimeWorkspaceContext(queryCtx, "other")
			}
			value, err := b.ObservePreparationProviderCatalog(queryCtx, "ws")
			if (err == nil) != (scenario == "missing") {
				t.Fatalf("calls=%d result=%+v err=%v", calls, value, err)
			}
			if scenario == "cancelled" || scenario == "parent_scope" {
				if calls != 0 {
					t.Fatal("cancelled query sent")
				}
				return
			}
			if calls != 1 {
				t.Fatal("retried cache-only query")
			}
			// Only the new query observer exists; invoking old catalog/selection
			// fallbacks would fail. A miss is data, never a reason to refresh.
			if scenario == "missing" && value.Catalog.ErrorCode != "cache_miss" {
				t.Fatal("cache miss not preserved")
			}
		})
	}
}
