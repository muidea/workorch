package workspaceports

import (
	"context"
	"fmt"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	memorymodule "github.com/muidea/workorch/internal/modules/blocks/memory"
	memorybiz "github.com/muidea/workorch/internal/modules/blocks/memory/biz"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestPurgeWorkspaceMemoryItemsConsumesAllBoundedPages(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(64)
	items := memoryrepo.NewMemoryItemRepository()
	module := memorymodule.New()
	if err := module.Configure(memorybiz.Dependencies{
		MemoryItems: items, MemoryRecallTraces: memoryrepo.NewMemoryRecallTraceRepository(), EventHub: hub,
	}); err != nil {
		t.Fatalf("configure memory owner: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("start memory owner: %v", err)
	}
	t.Cleanup(func() {
		_ = module.Stop(ctx)
		hub.Terminate(ctx)
	})

	workspaceID := common.WorkspaceID("ws-purge-pages")
	for idx := 0; idx < 205; idx++ {
		item := &memorymodel.MemoryItem{ID: common.MemoryID(fmt.Sprintf("memory-%03d", idx)), WorkspaceID: workspaceID}
		if err := items.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), item); err != nil {
			t.Fatalf("create %s: %v", item.ID, err)
		}
	}
	keep := &memorymodel.MemoryItem{ID: "memory-keep", WorkspaceID: "ws-keep"}
	if err := items.Create(common.ContextWithRuntimeWorkspaceID(ctx, keep.WorkspaceID), keep); err != nil {
		t.Fatalf("create keep memory: %v", err)
	}

	if err := purgeWorkspaceMemoryItems(ctx, hub, workspaceID); err != nil {
		t.Fatalf("purge paged workspace memory: %v", err)
	}
	remaining, err := items.ListByWorkspaceID(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), workspaceID)
	if err != nil {
		t.Fatalf("list purged workspace: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining workspace memories = %d", len(remaining))
	}
	kept, err := items.Get(common.ContextWithRuntimeWorkspaceID(ctx, keep.WorkspaceID), keep.ID)
	if err != nil || kept.WorkspaceID != keep.WorkspaceID {
		t.Fatalf("other workspace memory = %+v, err=%v", kept, err)
	}
}
