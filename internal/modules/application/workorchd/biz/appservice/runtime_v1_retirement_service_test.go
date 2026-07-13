package appservice

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestExecuteRuntimeV1RetirementDryRunReportsWithoutMutation(t *testing.T) {
	hub := newRuntimeV1RetirementInventoryHub(t, []*runmodel.Run{{ID: "run-v1", WorkspaceID: "ws-v1", State: common.RunStateActive}}, []*workspacemodel.Workspace{{ID: "ws-v1", ActiveRunID: "run-v1"}})
	rebuildCalls := 0
	result, err := ExecuteRuntimeV1Retirement(context.Background(), hub, true, func(context.Context) (int64, error) {
		rebuildCalls++
		return 9, nil
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeV1Retirement: %v", err)
	}
	if !result.DryRun || result.Report.ScannedV1Runs != 1 || result.Report.SupersededRuns != 1 || result.Report.ResourceScopesToRelease != 1 || result.Report.ReleasedResourceScopes != 0 || result.ProjectionRebuildRevision != 0 || rebuildCalls != 0 {
		t.Fatalf("result = %+v rebuildCalls=%d", result, rebuildCalls)
	}
}

func TestExecuteRuntimeV1RetirementApplyRejectsUnreleasedResourceScopes(t *testing.T) {
	hub := newRuntimeV1RetirementInventoryHub(t, []*runmodel.Run{{ID: "run-v1", WorkspaceID: "ws-v1", State: common.RunStateActive}}, []*workspacemodel.Workspace{{ID: "ws-v1", ActiveRunID: "run-v1"}})
	rebuildCalls := 0
	if _, err := ExecuteRuntimeV1Retirement(context.Background(), hub, false, func(context.Context) (int64, error) {
		rebuildCalls++
		return 1, nil
	}); err == nil {
		t.Fatal("ExecuteRuntimeV1Retirement returned nil error")
	}
	if rebuildCalls != 0 {
		t.Fatalf("rebuild calls = %d, want 0", rebuildCalls)
	}
}

func TestExecuteRuntimeV1RetirementApplyReleasesAppliesAndRebuilds(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	run := &runmodel.Run{ID: "run-v1", WorkspaceID: "ws-v1", State: common.RunStateActive}
	workspace := &workspacemodel.Workspace{ID: "ws-v1", State: common.WorkspaceStateActive, ActiveRunID: "run-v1"}
	registerRuntimeV1RetirementDynamicResponse(t, hub, runevents.CommandDestination, runevents.TopicRunCommandList, func(magicevent.Event) any { return runevents.RunCommandResult{Runs: []*runmodel.Run{run}} })
	registerRuntimeV1RetirementDynamicResponse(t, hub, workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandList, func(magicevent.Event) any {
		return workspaceevents.WorkspaceCommandResult{Workspaces: []*workspacemodel.Workspace{workspace}}
	})
	registerRuntimeV1RetirementInventoryResponse(t, hub, decisionevents.CommandDestination, decisionevents.TopicDecisionCommandListTickets, decisionevents.DecisionCommandResult{})
	registerRuntimeV1RetirementInventoryResponse(t, hub, approvalevents.CommandDestination, approvalevents.TopicApprovalCommandList, approvalevents.ApprovalCommandResult{})
	registerRuntimeV1RetirementDynamicResponse(t, hub, orchestratorevents.CommandDestination, orchestratorevents.TopicOrchestratorCommandReleaseRuntimeV1Resources, func(magicevent.Event) any {
		return orchestratorevents.ReleaseRuntimeV1ResourcesResult{ReleasedTotal: 2}
	})
	registerRuntimeV1RetirementDynamicResponse(t, hub, runevents.CommandDestination, runevents.TopicRunCommandUpdate, func(event magicevent.Event) any {
		cmd := event.Data().(runevents.UpdateRunCommand)
		run.State = cmd.State
		run.FailureReason = cmd.FailureReason
		return runevents.RunCommandResult{Run: run}
	})
	registerRuntimeV1RetirementDynamicResponse(t, hub, workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandUpdate, func(event magicevent.Event) any {
		cmd := event.Data().(workspaceevents.UpdateWorkspaceCommand)
		workspace.State = cmd.State
		workspace.ActiveRunID = cmd.ActiveRunID
		return workspaceevents.WorkspaceCommandResult{Workspace: workspace}
	})
	rebuildCalls := 0
	result, err := ExecuteRuntimeV1Retirement(context.Background(), hub, false, func(context.Context) (int64, error) {
		rebuildCalls++
		return 42, nil
	})
	if err != nil {
		t.Fatalf("ExecuteRuntimeV1Retirement: %v", err)
	}
	if result.Report.ReleasedResourceScopes != 2 || result.ProjectionRebuildRevision != 42 || rebuildCalls != 1 || run.State != common.RunStateSuperseded || workspace.ActiveRunID != "" {
		t.Fatalf("result=%+v run=%+v workspace=%+v rebuildCalls=%d", result, run, workspace, rebuildCalls)
	}
}

func newRuntimeV1RetirementInventoryHub(t *testing.T, runs []*runmodel.Run, workspaces []*workspacemodel.Workspace) magicevent.Hub {
	t.Helper()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	registerRuntimeV1RetirementInventoryResponse(t, hub, runevents.CommandDestination, runevents.TopicRunCommandList, runevents.RunCommandResult{Runs: runs})
	registerRuntimeV1RetirementInventoryResponse(t, hub, workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandList, workspaceevents.WorkspaceCommandResult{Workspaces: workspaces})
	registerRuntimeV1RetirementInventoryResponse(t, hub, decisionevents.CommandDestination, decisionevents.TopicDecisionCommandListTickets, decisionevents.DecisionCommandResult{})
	registerRuntimeV1RetirementInventoryResponse(t, hub, approvalevents.CommandDestination, approvalevents.TopicApprovalCommandList, approvalevents.ApprovalCommandResult{})
	return hub
}

func registerRuntimeV1RetirementInventoryResponse[Result any](t *testing.T, hub magicevent.Hub, destination string, topic string, response Result) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(destination, hub)
	observer.Subscribe(topic, func(_ magicevent.Event, result magicevent.Result) { result.Set(response, nil) })
	t.Cleanup(func() { observer.Unsubscribe(topic) })
}

func registerRuntimeV1RetirementDynamicResponse(t *testing.T, hub magicevent.Hub, destination string, topic string, response func(magicevent.Event) any) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(destination, hub)
	observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) { result.Set(response(event), nil) })
	t.Cleanup(func() { observer.Unsubscribe(topic) })
}
