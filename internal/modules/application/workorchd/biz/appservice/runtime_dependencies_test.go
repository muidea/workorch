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

	if deps.Snapshot.Workspaces != exports.Repositories.Workspaces {
		t.Fatal("snapshot workspaces repository mismatch")
	}
	if deps.Snapshot.Runs != exports.Repositories.Runs {
		t.Fatal("snapshot runs repository mismatch")
	}
	if deps.Snapshot.Steps != exports.Repositories.Steps {
		t.Fatal("snapshot steps repository mismatch")
	}
	if deps.Snapshot.Resolutions != exports.Repositories.Resolutions {
		t.Fatal("snapshot resolutions repository mismatch")
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

	if _, err := exports.Repositories.Workspaces.Get(ctx, workspaceID); err != nil {
		t.Fatalf("get workspace from repository: %v", err)
	}
	if _, err := exports.Repositories.Runs.Get(ctx, runID); err != nil {
		t.Fatalf("get run from repository: %v", err)
	}
	if _, err := exports.Repositories.Tickets.Get(ctx, ticketID); err != nil {
		t.Fatalf("get ticket from repository: %v", err)
	}
	if err := runtime.EnsureResolvedTicket(ctx, common.ResolutionID("resolution-runtime-deps-port"), ticketID, "approve_path", map[string]any{"operator": "runtime-deps"}); err != nil {
		t.Fatalf("ensure resolved ticket: %v", err)
	}
	if _, err := runtime.GetResolutionByTicketID(ctx, ticketID); err != nil {
		t.Fatalf("get resolution from runtime snapshot: %v", err)
	}
	if _, err := exports.Repositories.Resolutions.GetByTicketID(ctx, ticketID); err != nil {
		t.Fatalf("get resolution from repository: %v", err)
	}
	if _, err := exports.Repositories.Approvals.Get(ctx, approvalID); err != nil {
		t.Fatalf("get approval from repository: %v", err)
	}
}
