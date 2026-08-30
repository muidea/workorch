package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestReadWorkspaceProjectionHighWatermarksRejectsResultContractDrift(t *testing.T) {
	for name, response := range map[string]any{
		"wrong type": "wrong",
		"missing scope": eventstreamevents.HighWatermarksResult{HighWatermarks: []eventstreamevents.WorkspaceHighWatermark{
			{WorkspaceID: "ws-a", Revision: 1},
		}},
		"injected scope": eventstreamevents.HighWatermarksResult{HighWatermarks: []eventstreamevents.WorkspaceHighWatermark{
			{WorkspaceID: "ws-a", Revision: 1}, {WorkspaceID: "ws-c", Revision: 2},
		}},
		"negative revision": eventstreamevents.HighWatermarksResult{HighWatermarks: []eventstreamevents.WorkspaceHighWatermark{
			{WorkspaceID: "ws-a", Revision: -1}, {WorkspaceID: "ws-b", Revision: 2},
		}},
		"noncanonical order": eventstreamevents.HighWatermarksResult{HighWatermarks: []eventstreamevents.WorkspaceHighWatermark{
			{WorkspaceID: "ws-b", Revision: 2}, {WorkspaceID: "ws-a", Revision: 1},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(8)
			t.Cleanup(func() { hub.Terminate(ctx) })
			observer := magicevent.NewSimpleObserver(eventstreamevents.CommandDestination, hub)
			observer.Subscribe(eventstreamevents.TopicEventStreamCommandHighWatermarks, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(response, nil)
			})
			t.Cleanup(func() { observer.Unsubscribe(eventstreamevents.TopicEventStreamCommandHighWatermarks) })
			time.Sleep(10 * time.Millisecond)
			owner := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			_, err := owner.readWorkspaceProjectionHighWatermarks(ctx, []common.WorkspaceID{"ws-b", "ws-a"})
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("result drift error = %v, want invalid argument", err)
			}
		})
	}
}

func TestReadWorkspaceProjectionHighWatermarksReturnsDetachedCanonicalResult(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	shared := []eventstreamevents.WorkspaceHighWatermark{{WorkspaceID: "ws-a", Revision: 1}, {WorkspaceID: "ws-b", Revision: 2}}
	observer := magicevent.NewSimpleObserver(eventstreamevents.CommandDestination, hub)
	observer.Subscribe(eventstreamevents.TopicEventStreamCommandHighWatermarks, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(eventstreamevents.HighWatermarksResult{HighWatermarks: shared}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(eventstreamevents.TopicEventStreamCommandHighWatermarks) })
	time.Sleep(10 * time.Millisecond)
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	items, err := owner.readWorkspaceProjectionHighWatermarks(ctx, []common.WorkspaceID{"ws-b", "ws-a", "ws-b"})
	if err != nil {
		t.Fatal(err)
	}
	items[0].Revision = 99
	if shared[0].Revision != 1 {
		t.Fatalf("high-watermark result aliases owner response: %+v", shared)
	}
}
