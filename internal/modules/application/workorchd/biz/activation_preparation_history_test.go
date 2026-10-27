package biz

import (
	"context"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestPreparationHistoryApplicationRequiresExactCompleteOwnerFact(t *testing.T) {
	for _, scenario := range []string{"empty", "historical", "missing", "wrong_type", "scope", "request", "malformed", "cancelled", "parent_scope"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			defer hub.Terminate(ctx)
			observer := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
			calls := 0
			if err := observer.Subscribe(runevents.TopicObserveWorkspacePreparationHistory, func(ev magicevent.Event, result magicevent.Result) {
				calls++
				c, ok := ev.Data().(runevents.ObserveWorkspacePreparationHistoryCommand)
				deadline, bounded := ev.Context().Deadline()
				if !ok || c.RequestID != "check" || c.WorkspaceID != "ws" || ev.Source() != ModuleID || common.RuntimeWorkspaceIDFromContext(ev.Context()) != "ws" || !bounded || time.Until(deadline) > 2*time.Second {
					t.Error("wrong typed query/source/scope/budget")
				}
				if scenario == "missing" {
					result.Set(nil, cd.NewError(cd.NotFound, "fixture missing result"))
					return
				}
				if scenario == "wrong_type" {
					result.Set(runevents.ListRunsByWorkspaceResult{}, nil)
					return
				}
				r := runevents.ObserveWorkspacePreparationHistoryResult{RequestID: "check", ObservedAt: time.Now().UTC(), History: runmodel.WorkspacePreparationHistory{WorkspaceID: "ws"}}
				if scenario == "historical" {
					r.History.GenerationHead = 4
				}
				if scenario == "scope" {
					r.History.WorkspaceID = "other"
				}
				if scenario == "request" {
					r.RequestID = "other"
				}
				if scenario == "malformed" {
					r.History.RunCount = 1
				}
				result.Set(r, nil)
			}); err != nil {
				t.Fatal(err)
			}
			defer observer.Unsubscribe(runevents.TopicObserveWorkspacePreparationHistory)
			b := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			queryCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			if scenario == "cancelled" {
				cancel()
			}
			if scenario == "parent_scope" {
				queryCtx, _ = common.BindRuntimeWorkspaceContext(queryCtx, "other")
			}
			r, err := b.ObservePreparationRunHistory(queryCtx, "ws", "check")
			wantSuccess := scenario == "empty" || scenario == "historical"
			if (err == nil) != wantSuccess {
				t.Fatalf("result=%+v err=%v", r, err)
			}
			if scenario == "historical" && r.History.GenerationHead != 4 {
				t.Fatal("empty visible inventory lost historical watermark")
			}
			wantCalls := 1
			if scenario == "cancelled" || scenario == "parent_scope" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatal("unexpected retry or scope/cancellation bypass")
			}
		})
	}
}
