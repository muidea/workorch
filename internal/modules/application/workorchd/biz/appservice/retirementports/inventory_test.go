package retirementports

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func TestLoadRuntimeV1InventoryUsesTypedOwnerCommands(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	registerRetirementInventoryOwner(t, hub, runevents.CommandDestination, runevents.TopicRunCommandList, runevents.RunCommandResult{Runs: []*runmodel.Run{{ID: "run-v1", WorkspaceID: "ws-v1"}}})
	registerRetirementInventoryOwner(t, hub, workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandList, workspaceevents.WorkspaceCommandResult{Workspaces: []*workspacemodel.Workspace{{ID: "ws-v1", ActiveRunID: "run-v1"}}})
	registerRetirementInventoryOwner(t, hub, decisionevents.CommandDestination, decisionevents.TopicDecisionCommandListTickets, decisionevents.DecisionCommandResult{Tickets: []*decisionmodel.Ticket{{ID: "ticket-v1", RunID: "run-v1"}}})
	registerRetirementInventoryOwner(t, hub, approvalevents.CommandDestination, approvalevents.TopicApprovalCommandList, approvalevents.ApprovalCommandResult{Approvals: []*approvalmodel.Approval{{ID: "approval-v1", RunID: "run-v1"}}})

	inventory, err := LoadRuntimeV1Inventory(context.Background(), hub)
	if err != nil {
		t.Fatalf("LoadRuntimeV1Inventory: %v", err)
	}
	if len(inventory.Runs) != 1 || inventory.Runs[0].ID != "run-v1" || len(inventory.Workspaces) != 1 || len(inventory.Tickets) != 1 || len(inventory.Approvals) != 1 {
		t.Fatalf("inventory = %+v", inventory)
	}
}

func TestLoadRuntimeV1InventoryRejectsMissingOwnerResponse(t *testing.T) {
	hub := magicevent.NewHub(4)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	if _, err := LoadRuntimeV1Inventory(context.Background(), hub); err == nil {
		t.Fatal("LoadRuntimeV1Inventory returned nil error")
	}
}

func registerRetirementInventoryOwner[Result any](t *testing.T, hub magicevent.Hub, destination string, topic string, response Result) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(destination, hub)
	observer.Subscribe(topic, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(response, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(topic) })
}
