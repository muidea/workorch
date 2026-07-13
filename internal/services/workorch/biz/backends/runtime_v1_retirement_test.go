package backend

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
)

func TestEventHubBackendRuntimeV1RetirementUsesTypedApplicationCommand(t *testing.T) {
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, hub)
	observer.Subscribe(workorchdevents.TopicWorkorchDCommandRuntimeV1Retirement, func(event magicevent.Event, result magicevent.Result) {
		cmd := event.Data().(workorchdevents.RuntimeV1RetirementCommand)
		if !cmd.DryRun {
			t.Fatal("retirement command is not dry-run")
		}
		result.Set(workorchdevents.RuntimeV1RetirementResult{DryRun: true, Report: workorchdevents.RuntimeV1RetirementReport{ScannedV1Runs: 2, SupersededRuns: 1}}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandRuntimeV1Retirement) })
	maintenance := NewEventHub(hub).(workorchport.RuntimeV1RetirementBackend)
	result, err := maintenance.RetireRuntimeV1(context.Background(), workorchport.RuntimeV1RetirementOptions{DryRun: true})
	if err != nil || !result.DryRun || result.Report.ScannedV1Runs != 2 || result.Report.SupersededRuns != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
