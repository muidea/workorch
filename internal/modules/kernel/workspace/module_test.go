package workspace

import (
	"context"
	"errors"
	"testing"

	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestModuleWorkspaceCommandsRejectEnvelopePayloadScopeMismatch(t *testing.T) {
	ctx := context.Background()
	module := &Module{}
	tests := []struct {
		name     string
		envelope events.Envelope
	}{
		{
			name: "get goal",
			envelope: events.Envelope{
				Topic:       workspaceevents.TopicWorkspaceCommandGetGoal,
				WorkspaceID: "ws-envelope",
				Payload: map[string]any{
					events.PayloadKeyWorkspaceID: "ws-payload",
					events.PayloadKeyGoalID:      "goal-scope",
				},
			},
		},
		{
			name: "get dod",
			envelope: events.Envelope{
				Topic:       workspaceevents.TopicWorkspaceCommandGetDoD,
				WorkspaceID: "ws-envelope",
				Payload: map[string]any{
					events.PayloadKeyWorkspaceID: "ws-payload",
					events.PayloadKeyDoDID:       "dod-scope",
				},
			},
		},
		{
			name: "define goal command",
			envelope: events.Envelope{
				Topic:       workspaceevents.TopicWorkspaceCommandDefineGoal,
				WorkspaceID: "ws-envelope",
				Payload: map[string]any{
					events.PayloadKeyCommand: workspacecontracts.DefineGoalCommand{
						WorkspaceID: common.WorkspaceID("ws-command"),
						GoalID:      common.GoalID("goal-scope"),
						Text:        "goal",
					},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := module.handleWorkspaceQueryCommand(ctx, tc.envelope); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("handleWorkspaceQueryCommand err = %v, want conflict", err)
			}
		})
	}
}
