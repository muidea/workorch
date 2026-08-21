package contextregistryports

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

var soulManagementBenchmarkSink []workorchmanagementcontract.Soul

func TestListSoulsCollectsMultipleOwnerPages(t *testing.T) {
	hub, calls := pagedSoulBenchmarkHub(t, 250)
	items, err := ListSouls(context.Background(), hub)
	if err != nil || len(items) != 250 || *calls != 3 {
		t.Fatalf("items=%d calls=%d err=%v", len(items), *calls, err)
	}
	if items[0].ID != "soul-0000" || items[len(items)-1].ID != "soul-0249" {
		t.Fatalf("Soul ordering first=%q last=%q", items[0].ID, items[len(items)-1].ID)
	}
}

func BenchmarkListSoulsMultiPageManagementView(b *testing.B) {
	hub, _ := pagedSoulBenchmarkHub(b, 450)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var err error
		soulManagementBenchmarkSink, err = ListSouls(context.Background(), hub)
		if err != nil || len(soulManagementBenchmarkSink) != 450 {
			b.Fatalf("souls=%d err=%v", len(soulManagementBenchmarkSink), err)
		}
	}
}

type soulBenchmarkTB interface {
	Helper()
	Cleanup(func())
}

func pagedSoulBenchmarkHub(tb soulBenchmarkTB, count int) (magicevent.Hub, *int) {
	tb.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	tb.Cleanup(func() { hub.Terminate(ctx) })
	items := make([]*soulmodel.Soul, count)
	for idx := range items {
		item := &soulmodel.Soul{
			ID: common.SoulID(fmt.Sprintf("soul-%04d", idx)), Revision: 1, SourceProvenance: soulmodel.SoulSourceUserManaged,
			Name: fmt.Sprintf("Soul %04d", idx), SoulMD: "# Soul\n",
			AllowedSkillRefs: []string{fmt.Sprintf("skill-%04d", idx)},
		}
		if idx == 0 {
			item.SoulMD = strings.Repeat("s", 64*1024)
		}
		item.ContentHash = soulmodel.StableSoulContentHash(item)
		items[idx] = item
	}
	calls := 0
	observer := magicevent.NewSimpleObserver(soulevents.CommandDestination, hub)
	observer.Subscribe(soulevents.TopicSoulCommandList, func(event magicevent.Event, result magicevent.Result) {
		calls++
		command := event.Data().(soulevents.ListSoulsCommand)
		start := sort.Search(len(items), func(idx int) bool { return items[idx].ID > command.CursorAfterID })
		end := min(start+command.Limit, len(items))
		page := soulmodel.CloneSouls(items[start:end])
		response := soulevents.ListSoulsResult{Souls: page}
		if end < len(items) && len(page) > 0 {
			response.NextCursor = page[len(page)-1].ID
		}
		result.Set(response, nil)
	})
	tb.Cleanup(func() { observer.Unsubscribe(soulevents.TopicSoulCommandList) })
	return hub, &calls
}
