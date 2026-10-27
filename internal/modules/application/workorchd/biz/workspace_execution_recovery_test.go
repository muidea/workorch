package biz

import (
	"context"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceGatewayRecoversOriginalBeforeUnavailableCurrentCompiler(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "unknown"}[pending], func(t *testing.T) {
			hub := magicevent.NewHub(8)
			defer hub.Terminate(context.Background())
			observer := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			defer observer.Unsubscribe(workspaceevents.TopicWorkspaceCommandRecoverExecutionIntent)
			calls := 0
			observer.Subscribe(workspaceevents.TopicWorkspaceCommandRecoverExecutionIntent, func(ev magicevent.Event, result magicevent.Result) {
				calls++
				if ev.LaneKey() != "state/workspace/ws" || ev.Source() != ModuleID {
					t.Error("recovery lost owner lane/source", ev.LaneKey(), ev.Source())
				}
				c := ev.Data().(workspaceevents.RecoverWorkspaceExecutionIntentCommand)
				a := workspaceevents.WorkspaceExecutionIntentResult{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, Kind: c.Kind, RunID: "original-run", Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated}
				if pending {
					a.RunID = ""
					a.Outcome = runtimecontract.RuntimeExecutionIntentFreshUnresolved
					a.Reason = runtimecontract.RuntimeExecutionIntentReasonFreshResultUnknown
					a.PendingRequestID = "original"
					a.PendingStartupInputHash = "startup"
					a.PendingPayloadHash = strings.Repeat("a", 64)
				}
				result.Set(workspaceevents.RecoverWorkspaceExecutionIntentResult{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, Kind: c.Kind, Admission: &a}, nil)
			})
			// No runtime/compile session exists. Recovery must not borrow one.
			gateway := &workspaceCommandGateway{hub: hub}
			for range 2 {
				r, err := gateway.submit(context.Background(), workorchdevents.WorkspaceRunRequestedCommand{WorkspaceID: "ws", RequestID: "original", Intent: "restart"})
				if err != nil || r.RequestID != "original" || (pending && r.PendingRequestID != "original") || (!pending && r.RunID != "original-run") {
					t.Fatal("recompiled or lost original", r, err)
				}
			}
			if calls != 2 {
				t.Fatal("no fresh owner reads", calls)
			}
		})
	}
}

func TestWorkspaceGatewayRecoveryInvalidOrAbsentCannotGrantForward(t *testing.T) {
	for _, scenario := range []string{"wrong_type", "wrong_scope", "wrong_request", "wrong_kind", "invalid_admission", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			hub := magicevent.NewHub(8)
			defer hub.Terminate(context.Background())
			o := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			defer o.Unsubscribe(workspaceevents.TopicWorkspaceCommandRecoverExecutionIntent)
			o.Subscribe(workspaceevents.TopicWorkspaceCommandRecoverExecutionIntent, func(ev magicevent.Event, r magicevent.Result) {
				c := ev.Data().(workspaceevents.RecoverWorkspaceExecutionIntentCommand)
				v := workspaceevents.RecoverWorkspaceExecutionIntentResult{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, Kind: c.Kind}
				switch scenario {
				case "wrong_type":
					r.Set("not a receipt", nil)
					return
				case "wrong_scope":
					v.WorkspaceID = "other"
				case "wrong_request":
					v.RequestID = "other"
				case "wrong_kind":
					v.Kind = runtimecontract.RuntimeExecutionIntentResume
				case "invalid_admission":
					v.Admission = &workspaceevents.WorkspaceExecutionIntentResult{}
				}
				r.Set(v, nil)
			})
			g := &workspaceCommandGateway{hub: hub}
			if _, err := g.submit(context.Background(), workorchdevents.WorkspaceRunRequestedCommand{WorkspaceID: "ws", RequestID: "original", Intent: "restart"}); err == nil {
				t.Fatal("invalid/missing result granted acceptance")
			}
		})
	}
}
