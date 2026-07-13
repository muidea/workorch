package orchestrator

import (
	"context"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestActivateRuntimeV2RunOnlyProjectsOwnerState(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	steps := memory.NewStepRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateBlockedOnDecision, BlockingTicketID: "ticket-v2",
	}); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, steps, workspaces, nil, nil)
	if err := orchestrator.ActivateRuntimeV2Run(ctx, "ws-v2", "run-v2"); err != nil {
		t.Fatal(err)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != common.RunStateActive || run.BlockingTicketID != "" {
		t.Fatalf("activated run = %+v", run)
	}
	items, err := steps.ListByRun(ctx, "run-v2")
	if err != nil || len(items) != 0 {
		t.Fatalf("Runtime V2 activation created legacy steps: %+v err=%v", items, err)
	}
}

func TestProjectRuntimeV2CompleteBypassesLegacyResultGate(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive}); err != nil {
		t.Fatal(err)
	}
	orchestrator := NewRuntimeOrchestrator(runs, memory.NewStepRepository(), workspaces, nil, nil)
	terminal := &runtimecontract.TerminalOutcome{Code: "coverage_satisfied", Summary: "全部 coverage focus 已满足"}
	if err := orchestrator.ProjectRuntimeV2Terminal(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeActionComplete, terminal); err != nil {
		t.Fatal(err)
	}
	if err := orchestrator.ProjectRuntimeV2Terminal(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeActionComplete, terminal); err != nil {
		t.Fatalf("terminal replay: %v", err)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != common.RunStateCompleted || run.EndedAt == "" || workspace.ActiveRunID != "" {
		t.Fatalf("run=%+v workspace=%+v", run, workspace)
	}
}
