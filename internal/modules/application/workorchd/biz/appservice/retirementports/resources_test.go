package retirementports

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/retirementcore"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
)

func TestReleaseRuntimeV1ResourcesUsesTypedOrchestratorCommand(t *testing.T) {
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommandReleaseRuntimeV1Resources, func(event magicevent.Event, result magicevent.Result) {
		cmd := event.Data().(orchestratorevents.ReleaseRuntimeV1ResourcesCommand)
		if cmd.WorkspaceID == "" || cmd.RunID == "" {
			t.Fatalf("release command = %+v", cmd)
		}
		result.Set(orchestratorevents.ReleaseRuntimeV1ResourcesResult{ReleasedTotal: 3}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommandReleaseRuntimeV1Resources) })
	total, err := ReleaseRuntimeV1Resources(context.Background(), hub, retirementcore.RuntimeV1RetirementPlan{Runs: []retirementcore.RuntimeV1RunAction{{WorkspaceID: "ws-v1", RunID: "run-v1"}}})
	if err != nil || total != 3 {
		t.Fatalf("release total/err = %d/%v", total, err)
	}
}
