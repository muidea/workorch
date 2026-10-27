package workspace

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestFreshIntentReceiptSenderUsesExactTypedReadWithoutForwardOrSettlement(t *testing.T) {
	_, original := reservationTestInput(t)
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	repos := memory.NewRunOwnerRepositorySet()
	runOwner := runbiz.New()
	if err := runOwner.Configure(runbiz.Dependencies{OwnerRepositories: repos, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	defer runOwner.Stop(ctx)
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	d := original.Bootstrap.Definition.Normalized()
	forward := runevents.WorkspaceExecutionIntentCommand{WorkspaceID: original.WorkspaceID, RequestID: original.RequestID, Kind: original.Kind,
		GoalID: d.GoalID, DoDID: d.DoDID, DefinitionRevision: d.Revision, StartupInputHash: d.StartupInputHash, Bootstrap: original.Bootstrap,
	}
	hash, err := runevents.WorkspaceExecutionIntentPayloadHash(forward)
	if err != nil {
		t.Fatal(err)
	}
	c := runevents.QueryWorkspaceExecutionIntentReceiptCommand{WorkspaceID: original.WorkspaceID, RequestID: original.RequestID, PayloadHash: hash}
	unknown, err := owner.queryRunExecutionIntentReceipt(ctx, c)
	if err != nil || unknown.Receipt != nil {
		t.Fatal("missing original was not explicitly unknown")
	}
	items, err := repos.Runs().List(ctx)
	if err != nil || len(items) != 0 {
		t.Fatal("query created a Run")
	}
	// Explicit test-only acceptance prepares the original owner receipt; the
	// query sender itself has no execution or Workspace mutation path.
	response := hub.Send(magicevent.NewEventWithContext(runevents.TopicRunCommandWorkspaceExecutionIntent, ModuleID, runevents.CommandDestination, nil, ctx, forward))
	if response == nil || response.Error() != nil {
		t.Fatalf("fixture acceptance: %v", response)
	}
	data, resultErr := response.Get()
	admission, ok := data.(runevents.WorkspaceExecutionIntentResult)
	if resultErr != nil || !ok || admission.Validate() != nil {
		t.Fatalf("fixture lost exact original admission: %T %+v %v", data, data, resultErr)
	}
	before, _ := repos.Runs().List(ctx)
	for i := 0; i < 2; i++ {
		value, err := owner.queryRunExecutionIntentReceipt(ctx, c)
		if err != nil || value.Receipt == nil || value.Receipt.Admission != admission.RuntimeExecutionIntentAdmission {
			t.Fatalf("original sender result: %+v %v", value, err)
		}
		value.Receipt.PayloadHash = "caller mutation"
	}
	after, _ := repos.Runs().List(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("query sender mutated Run state")
	}
}

func TestFreshIntentReceiptSenderRejectsUnboundResultsWithoutRetry(t *testing.T) {
	for _, scenario := range []string{"unknown", "owner_error", "wrong_type", "scope", "request", "hash", "time", "cancelled", "parent_scope"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(32)
			defer hub.Terminate(ctx)
			observer := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
			calls := 0
			fixtureErr := errors.New("fixture original lookup failed")
			if err := observer.Subscribe(runevents.TopicQueryWorkspaceExecutionIntentReceipt, func(ev magicevent.Event, result magicevent.Result) {
				calls++
				c, ok := ev.Data().(runevents.QueryWorkspaceExecutionIntentReceiptCommand)
				deadline, bounded := ev.Context().Deadline()
				if !ok || ev.Source() != ModuleID || common.RuntimeWorkspaceIDFromContext(ev.Context()) != "ws" || !bounded || time.Until(deadline) > 2*time.Second {
					t.Error("wrong query contract/source/scope/budget")
				}
				if scenario == "owner_error" {
					result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "fixture", Cause: fixtureErr})
					return
				}
				if scenario == "wrong_type" {
					result.Set(runevents.ListRunsByWorkspaceResult{}, nil)
					return
				}
				r := runevents.QueryWorkspaceExecutionIntentReceiptResult{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, PayloadHash: c.PayloadHash, ObservedAt: time.Now().UTC()}
				switch scenario {
				case "scope":
					r.WorkspaceID = "other"
				case "request":
					r.RequestID = "other"
				case "hash":
					r.PayloadHash = strings.Repeat("b", 64)
				case "time":
					r.ObservedAt = time.Time{}
				}
				result.Set(r, nil)
			}); err != nil {
				t.Fatal(err)
			}
			defer observer.Unsubscribe(runevents.TopicQueryWorkspaceExecutionIntentReceipt)
			owner := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
			queryCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			if scenario == "cancelled" {
				cancel()
			}
			if scenario == "parent_scope" {
				queryCtx = common.ContextWithRuntimeWorkspaceID(queryCtx, "other")
			}
			r, err := owner.queryRunExecutionIntentReceipt(queryCtx, runevents.QueryWorkspaceExecutionIntentReceiptCommand{WorkspaceID: "ws", RequestID: "original", PayloadHash: strings.Repeat("a", 64)})
			if (err == nil) != (scenario == "unknown") || r.Receipt != nil {
				t.Fatalf("result=%+v err=%v", r, err)
			}
			if scenario == "owner_error" && !errors.Is(err, fixtureErr) {
				t.Fatal("owner failure was turned into unknown")
			}
			wantCalls := 1
			if scenario == "cancelled" || scenario == "parent_scope" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatal("unexpected retry or scope bypass")
			}
		})
	}
}
