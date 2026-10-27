package memory

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type preparationInventoryRepository struct {
	repository.MemoryItemRepository
	items  []*memorymodel.MemoryItem
	err    error
	calls  int
	cancel context.CancelFunc
}

func (r *preparationInventoryRepository) ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	r.calls++
	if r.cancel != nil {
		r.cancel()
	}
	return r.items, r.err
}
func preparationMemoryItem() *memorymodel.MemoryItem {
	return &memorymodel.MemoryItem{ID: "memory", WorkspaceID: "ws", SourceType: "user", MemoryType: "project_context", Summary: "原摘要", Content: "原文\r\n第二行\x00", Tags: []string{"原标签"}, Vector: []float64{0.125, 0.25}, EmbeddingProviderID: "original", EmbeddingModel: "original", EmbeddingText: "原嵌入文本"}
}

func TestPreparationMemoryInventoryCompleteOrRejectWithoutFiltering(t *testing.T) {
	for _, scenario := range []string{"valid", "empty", "nil", "scope", "duplicate", "invalid_utf8", "too_many", "too_large", "vector", "nan", "repository_error", "cancelled_before", "cancelled_during", "bound_scope"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repo := &preparationInventoryRepository{items: []*memorymodel.MemoryItem{preparationMemoryItem()}}
			switch scenario {
			case "empty":
				repo.items = nil
			case "nil":
				repo.items[0] = nil
			case "scope":
				repo.items[0].WorkspaceID = "other"
			case "duplicate":
				repo.items = append(repo.items, preparationMemoryItem())
			case "invalid_utf8":
				repo.items[0].Content = string([]byte{255})
			case "too_many":
				repo.items = make([]*memorymodel.MemoryItem, memorymodel.PreparationInventoryMaxItems+1)
			case "too_large":
				repo.items[0].Content = strings.Repeat("x", memorymodel.PreparationInventoryMaxBytes)
			case "vector":
				repo.items[0].Vector = make([]float64, 16385)
			case "nan":
				repo.items[0].VectorScore = math.NaN()
			case "repository_error":
				repo.err = errors.New("original diagnostic")
			case "cancelled_before":
				cancel()
			case "cancelled_during":
				repo.cancel = cancel
			case "bound_scope":
				ctx, _ = common.BindRuntimeWorkspaceContext(ctx, "other")
			}
			h := magicevent.NewHub(8)
			defer h.Terminate(context.Background())
			m := lifecycleOwner(t, h)
			m.service.items = repo
			v, err := m.observePreparationInventory(ctx, memoryevents.ObservePreparationInventoryCommand{WorkspaceID: "ws"})
			if scenario == "valid" || scenario == "empty" {
				if err != nil || v.ValidateFor(memoryevents.ObservePreparationInventoryCommand{WorkspaceID: "ws"}) != nil {
					t.Fatal(err)
				}
				if scenario == "valid" {
					if !reflect.DeepEqual(v.Inventory.Items[0], repo.items[0]) {
						t.Fatal("raw value changed")
					}
					v.Inventory.Items[0].Content = "caller"
					v.Inventory.Items[0].Tags[0] = "caller"
					v.Inventory.Items[0].Vector[0] = 99
					if !reflect.DeepEqual(repo.items[0], preparationMemoryItem()) {
						t.Fatal("clone aliases repository")
					}
				}
			} else if err == nil || v.Inventory.Items != nil {
				t.Fatal("invalid observation became complete", err)
			}
			if scenario == "repository_error" && !errors.Is(err, repo.err) {
				t.Fatal("raw error replaced")
			}
			want := 1
			if scenario == "cancelled_before" || scenario == "bound_scope" {
				want = 0
			}
			if repo.calls != want {
				t.Fatal("unexpected query count", repo.calls)
			}
		})
	}
}

func TestPreparationMemoryInventorySourceDrainAndOwnerEpoch(t *testing.T) {
	ctx := context.Background()
	h := magicevent.NewHub(16)
	defer h.Terminate(ctx)
	m := lifecycleOwner(t, h)
	if err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Teardown(ctx)
	query := func(source string, scope context.Context) magicevent.Result {
		return h.Send(magicevent.NewEventWithContext(memoryevents.TopicObservePreparationInventory, source, ModuleID, magicevent.NewHeader(), scope, memoryevents.ObservePreparationInventoryCommand{WorkspaceID: "ws"}))
	}
	for _, src := range []string{"test", "application/workorchd/appservice", "kernel/workspace"} {
		r := query(src, ctx)
		if r == nil || r.Error() == nil {
			t.Fatal("wrong source allowed", src)
		}
	}
	bound, _ := common.BindRuntimeWorkspaceContext(ctx, "other")
	if r := query("application/workorchd", bound); r == nil || r.Error() == nil {
		t.Fatal("scope mismatch allowed")
	}
	m.BeginShutdown(ctx)
	r := query("application/workorchd", ctx)
	if r == nil || r.Error() != nil {
		t.Fatal("read-only drain rejected", r)
	}
	value, err := r.Get()
	v, ok := value.(memoryevents.ObservePreparationInventoryResult)
	if err != nil || !ok || v.ValidateFor(memoryevents.ObservePreparationInventoryCommand{WorkspaceID: "ws"}) != nil || len(v.Inventory.Items) != 0 {
		t.Fatal("valid empty inventory rejected", err)
	}
	other := lifecycleOwner(t, h)
	if other.preparationEpoch == m.preparationEpoch {
		t.Fatal("owner epoch reused")
	}
	scope := deletioncontract.Scope{WorkspaceID: "ws", DeletionID: "delete-inventory", DeletionGeneration: 1, Mode: deletioncontract.ModeRetain, QuiescenceID: "quiescence-inventory"}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	tombstone, reduceErr := reduceWorkspaceDeletionInternalEvent(nil, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventFenceObserved, Scope: scope}, time.Now().UTC().Format(time.RFC3339Nano))
	if reduceErr != nil {
		t.Fatal(reduceErr)
	}
	if err := m.workspaceDeletions.Create(ctx, tombstone); err != nil {
		t.Fatal(err)
	}
	if r := query("application/workorchd", ctx); r == nil || r.Error() == nil {
		t.Fatal("retired Workspace became an empty inventory")
	}
	// Post has no result and cannot turn a query into a write or panic.
	m.handleCommand(magicevent.NewEventWithContext(memoryevents.TopicObservePreparationInventory, "application/workorchd", ModuleID, nil, ctx, memoryevents.ObservePreparationInventoryCommand{WorkspaceID: "ws"}), nil)
}
