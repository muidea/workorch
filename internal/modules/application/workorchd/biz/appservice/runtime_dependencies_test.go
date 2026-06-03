package appservice

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeDependenciesFromExportsMapsGroupedCapabilities(t *testing.T) {
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := session.Bootstrap.Exports()
	deps := RuntimeDependenciesFromExports(exports)

	if deps.Snapshot == nil {
		t.Fatal("snapshot read port is nil")
	}
	if deps.Actions.Workspace == nil {
		t.Fatal("workspace action port is nil")
	}
	if deps.Actions.Run == nil {
		t.Fatal("run action port is nil")
	}
	if deps.Actions.Decision == nil {
		t.Fatal("decision action port is nil")
	}
	if deps.Actions.Approval == nil {
		t.Fatal("approval action port is nil")
	}
}

func TestRuntimeDependenciesFromExportsActionPortsDriveModuleServices(t *testing.T) {
	session, _ := newTestRuntimeSession(t, "memory", "")
	exports := session.Bootstrap.Exports()
	deps := RuntimeDependenciesFromExports(exports)
	runtime := NewRuntimeAdapter(deps)
	ctx := context.Background()

	workspaceID := common.WorkspaceID("ws-runtime-deps-port")
	runID := common.RunID("run-runtime-deps-port")
	ticketID := common.TicketID("ticket-runtime-deps-port")
	approvalID := common.ApprovalID("approval-runtime-deps-port")
	stepID := common.StepID("run-runtime-deps-port/manual-gate")

	if err := runtime.EnsureWorkspace(ctx, workspaceID, "runtime deps"); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}
	if err := runtime.EnsureRun(ctx, runID, workspaceID); err != nil {
		t.Fatalf("ensure run: %v", err)
	}
	if err := runtime.EnsureTicket(ctx, ticketID, workspaceID, runID, stepID, "decision gate"); err != nil {
		t.Fatalf("ensure ticket: %v", err)
	}
	if err := runtime.EnsureApproval(ctx, approvalID, workspaceID, runID, stepID, "approval gate"); err != nil {
		t.Fatalf("ensure approval: %v", err)
	}

	if _, err := runtime.GetWorkspaceByID(ctx, workspaceID); err != nil {
		t.Fatalf("get workspace from runtime snapshot: %v", err)
	}
	if _, err := runtime.GetRun(ctx, runID); err != nil {
		t.Fatalf("get run from runtime snapshot: %v", err)
	}
	if _, err := runtime.GetTicket(ctx, ticketID); err != nil {
		t.Fatalf("get ticket from runtime snapshot: %v", err)
	}
	if err := runtime.EnsureResolvedTicket(ctx, common.ResolutionID("resolution-runtime-deps-port"), ticketID, "provide_missing_info", map[string]any{"operator": "runtime-deps"}); err != nil {
		t.Fatalf("ensure resolved ticket: %v", err)
	}
	if _, err := runtime.GetResolutionByTicketID(ctx, ticketID); err != nil {
		t.Fatalf("get resolution from runtime snapshot: %v", err)
	}
	if _, err := runtime.GetApproval(ctx, approvalID); err != nil {
		t.Fatalf("get approval from runtime snapshot: %v", err)
	}
}
