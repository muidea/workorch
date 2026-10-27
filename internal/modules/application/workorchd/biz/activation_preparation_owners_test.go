package biz

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

// Real sender + actual owner receivers on one Hub: a permissive test observer
// alone cannot establish agreement on source, destination or concrete result.
func TestPreparationObservationApplicationUsesActualLLMAndRunOwners(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	repos := memory.NewRunOwnerRepositorySet()
	runs := runbiz.New()
	if err := runs.Configure(runbiz.Dependencies{OwnerRepositories: repos, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Start(); err != nil {
		t.Fatal(err)
	}
	defer runs.Stop(ctx)
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://provider.invalid"
	cfg.AIProxy.APIKey = "non-sensitive-fixture"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "fixture-chat"}
	llm, err := llmbiz.New(hub, nil, llmbiz.Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return cfg, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Teardown(ctx)
	application := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	catalog, err := application.ObservePreparationProviderCatalog(ctx, "ws")
	if err != nil || catalog.Catalog.ErrorCode != "cache_miss" {
		t.Fatalf("real LLM receiver: %+v %v", catalog, err)
	}
	history, err := application.ObservePreparationRunHistory(ctx, "ws", "first-check")
	if err != nil || history.History.GenerationHead != 0 {
		t.Fatalf("real Run receiver: %+v %v", history, err)
	}
	item := &runmodel.Run{WorkspaceID: "ws", State: common.RunStateCompleted}
	if err := repos.Runs().CreateNextWorkspaceRun(ctx, item); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		history, err = application.ObservePreparationRunHistory(ctx, "ws", "repeat-check")
		if err != nil || history.History.GenerationHead != 1 || history.History.RunCount != 1 {
			t.Fatalf("history hidden/replayed: %+v %v", history, err)
		}
	}
	items, err := repos.Runs().ListByWorkspaceID(ctx, "ws")
	if err != nil || len(items) != 1 || items[0].ID != item.ID || items[0].Revision != item.Revision || items[0].State != common.RunStateCompleted {
		t.Fatal("preparation observation mutated Run inventory")
	}
}
