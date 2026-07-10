package backend

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestEventHubRunBackendSendsTypedSchedulerCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	hub := events.NewMagicHubWithHubOwner(rawHub, "workorch/backend/test")
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	var captured workorchdevents.WorkspaceRunRequestedCommand
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, rawHub)
	observer.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(ev magicevent.Event, result magicevent.Result) {
		captured, _ = ev.Data().(workorchdevents.WorkspaceRunRequestedCommand)
		result.Set(workorchdevents.WorkspaceRunRequestedResult{Accepted: true}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested) })

	backend := NewEventHub(hub)
	_, err := backend.Run().Command(context.Background(), WorkspaceRunCommand{
		WorkspaceID:        "ws-typed-backend",
		ExpectedRunID:      "run-typed-backend",
		Intent:             RunCommandIntentResume,
		Reason:             "typed contract",
		RequestID:          "request-typed-backend",
		Source:             "test",
		Priority:           9,
		DefinitionRevision: "revision-typed-backend",
		DecisionID:         "ticket-typed-backend",
		Activation: ActivateRunRequest{
			GoalID:        "goal-typed-backend",
			DoDID:         "dod-typed-backend",
			SoulID:        "soul-typed-backend",
			SkillIDs:      []common.SkillID{"skill-typed-backend"},
			MCPServerIDs:  []common.MCPServerID{"mcp-typed-backend"},
			CapabilityIDs: []common.MCPCapabilityID{"capability-typed-backend"},
			MemoryIDs:     []common.MemoryID{"memory-typed-backend"},
		},
	})
	if err != nil {
		t.Fatalf("run command: type=%T value=%#v text=%v", err, err, err)
	}
	if captured.WorkspaceID != "ws-typed-backend" || captured.RunID != "run-typed-backend" || captured.Intent != "resume" || captured.Priority != 9 {
		t.Fatalf("captured command = %+v", captured)
	}
	if captured.GoalID != "goal-typed-backend" || len(captured.SkillIDs) != 1 || captured.SkillIDs[0] != "skill-typed-backend" {
		t.Fatalf("captured activation contract = %+v", captured)
	}
}
