package orchestrator

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// A committed Run notification outlives its initiating HTTP/command request.
// Exercise the real Hub: Send must still reject canceled synchronous requests,
// while SM-4 notification admission uses the Orchestrator lifecycle instead.
func TestRunNotificationAdmissionUsesOwnerLifecycle(t *testing.T) {
	for _, ingress := range []string{"active", "resume", "continuation"} {
		for _, guard := range []string{"live", "owner_canceled", "retired", "wrong_source"} {
			t.Run(ingress+"/"+guard, func(t *testing.T) {
				hub := magicevent.NewHub(8)
				t.Cleanup(func() { hub.Terminate(context.Background()) })
				background := &recordingBackgroundRoutine{}
				module := newTestBiz(newRuntimeTestOwner(), hub, background)
				if err := module.ConfigureRuntimeRepository(&inMemoryRuntimeStateRepository{}); err != nil {
					t.Fatal(err)
				}
				ownerCtx, cancelOwner := context.WithCancel(context.Background())
				t.Cleanup(cancelOwner)
				if err := module.openRuntimeLifecycle(ownerCtx); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(module.lifecycleCancel)
				const workspaceID common.WorkspaceID = "ws-notification"
				const runID common.RunID = "run-notification"
				if guard == "owner_canceled" {
					cancelOwner()
				}
				if guard == "retired" {
					module.retiredWorkspaces = map[common.WorkspaceID]runevents.RunWorkspaceRetirementFence{
						workspaceID: {Phase: runmodel.WorkspaceRetirementPhaseRetired},
					}
				}
				source := runevents.CommandDestination
				if guard == "wrong_source" {
					source = "not-run-owner"
				}
				requestCtx, cancelRequest := context.WithCancel(context.Background())
				cancelRequest()
				if err := module.scheduleRuntimeDrive(requestCtx, workspaceID, runID, "synchronous"); guard == "live" && err == nil {
					t.Fatal("synchronous scheduling accepted a canceled request")
				}
				topic := runevents.TopicRunStateChanged
				state := runevents.RunStateChangedData{
					WorkspaceID: workspaceID, RunID: runID, WorkspaceRunGeneration: 1, RunRevision: 1,
					State: common.RunStateActive,
				}
				state.CausationID = runevents.RunStateChangedCausationID(runID, 1, 1, 0)
				if ingress == "resume" {
					state.ReconciliationRequestID = "resume-request"
				}
				var data any = state
				handle := module.handleRunStateChanged
				if ingress == "continuation" {
					topic = runevents.TopicRunContinuationRequested
					data = runevents.RunContinuationRequestedData{
						WorkspaceID: workspaceID, RunID: runID, CausationID: "continue-fact", RequestID: "continue-request",
					}
					handle = module.handleRunContinuationRequested
				}
				ev := magicevent.NewEventWithContext(topic, source, runevents.NotificationDestination, magicevent.NewHeader(), requestCtx, data)
				handle(ev, nil)
				handle(ev, nil)
				want := 0
				if guard == "live" {
					want = 1
				}
				if background.asyncFunctionCalls != want {
					t.Fatalf("scheduled %d workers, want %d (duplicate must coalesce)", background.asyncFunctionCalls, want)
				}
			})
		}
	}
}
