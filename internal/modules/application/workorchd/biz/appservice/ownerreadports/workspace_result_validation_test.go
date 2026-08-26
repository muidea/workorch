package ownerreadports

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceOwnerReadPortsRejectMismatchedResultIdentityAndScope(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-owner-read-contract")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	tests := []struct {
		name     string
		topic    string
		response any
		call     func(context.Context, magicevent.Hub) error
	}{
		{
			name:     "workspace identity",
			topic:    workspaceevents.TopicWorkspaceCommandGet,
			response: workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-foreign"}},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := ReadWorkspaceByOwnerCommand(ctx, hub, workspaceID)
				return err
			},
		},
		{
			name:  "workspace list duplicate",
			topic: workspaceevents.TopicWorkspaceCommandList,
			response: workspaceevents.WorkspaceListResult{Workspaces: []*workspaceevents.Workspace{
				{ID: workspaceID}, {ID: workspaceID},
			}},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := ListWorkspacesByOwnerCommand(ctx, hub)
				return err
			},
		},
		{
			name:     "goal scope",
			topic:    workspaceevents.TopicWorkspaceCommandGetGoal,
			response: workspaceevents.GoalResult{Goal: &workspacemodel.Goal{ID: "goal-contract", WorkspaceID: "ws-foreign"}},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := ReadGoalByOwnerCommand(ctx, hub, "goal-contract")
				return err
			},
		},
		{
			name:     "goal list scope",
			topic:    workspaceevents.TopicWorkspaceCommandListGoalsByWS,
			response: workspaceevents.GoalListResult{Goals: []*workspacemodel.Goal{{ID: "goal-contract", WorkspaceID: "ws-foreign"}}},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := ListGoalsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
				return err
			},
		},
		{
			name:     "dod identity",
			topic:    workspaceevents.TopicWorkspaceCommandGetDoD,
			response: workspaceevents.DoDResult{DoD: &workspacemodel.DoD{ID: "dod-foreign", WorkspaceID: workspaceID}},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := ReadDoDByOwnerCommand(ctx, hub, "dod-contract")
				return err
			},
		},
		{
			name:  "dod list duplicate",
			topic: workspaceevents.TopicWorkspaceCommandListDoDsByWS,
			response: workspaceevents.DoDListResult{DoDs: []*workspacemodel.DoD{
				{ID: "dod-contract", WorkspaceID: workspaceID}, {ID: "dod-contract", WorkspaceID: workspaceID},
			}},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := ListDoDsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
				return err
			},
		},
		{
			name:     "template identity",
			topic:    workspaceevents.TopicWorkspaceTemplateCommandGet,
			response: workspaceevents.WorkspaceTemplateResult{Template: &workspaceevents.WorkspaceTemplate{ID: "template-foreign"}},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := ReadWorkspaceTemplateByOwnerCommand(ctx, hub, "template-contract")
				return err
			},
		},
		{
			name:  "template list duplicate",
			topic: workspaceevents.TopicWorkspaceTemplateCommandList,
			response: workspaceevents.WorkspaceTemplateListResult{Templates: []*workspaceevents.WorkspaceTemplate{
				{ID: "template-contract"}, {ID: "template-contract"},
			}},
			call: func(ctx context.Context, hub magicevent.Hub) error {
				_, err := ListWorkspaceTemplatesByOwnerCommand(ctx, hub)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hub := magicevent.NewHub(4)
			observer := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
			observer.Subscribe(test.topic, func(_ magicevent.Event, result magicevent.Result) {
				result.Set(test.response, nil)
			})
			t.Cleanup(func() { observer.Unsubscribe(test.topic) })

			if err := test.call(ctx, hub); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("error = %v, want owner result conflict", err)
			}
		})
	}
}
