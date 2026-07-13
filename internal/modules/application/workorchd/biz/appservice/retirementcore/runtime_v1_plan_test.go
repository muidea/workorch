package retirementcore

import (
	"reflect"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestBuildRuntimeV1RetirementPlanIsDeterministicAndExcludesV2(t *testing.T) {
	v2State := &runtimecontract.RuntimeStateRecord{}
	inventory := RuntimeV1Inventory{
		Runs: []*runmodel.Run{
			{ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive, RuntimeContractVersion: runtimecontract.RuntimeContractVersionV2, RuntimeState: v2State},
			{ID: "run-v1-b", WorkspaceID: "ws-b", State: common.RunStateBlockedOnApproval},
			{ID: "run-v1-a", WorkspaceID: "ws-a", State: common.RunStateActive},
			{ID: "run-v1-terminal", WorkspaceID: "ws-terminal", State: common.RunStateFailed},
		},
		Workspaces: []*workspacemodel.Workspace{
			{ID: "ws-terminal", ActiveRunID: "run-v1-terminal"},
			{ID: "ws-b", ActiveRunID: "run-v1-b"},
			{ID: "ws-a", ActiveRunID: "run-v1-a"},
			{ID: "ws-v2", ActiveRunID: "run-v2"},
		},
		Tickets: []*decisionmodel.Ticket{
			{ID: "ticket-b", WorkspaceID: "ws-b", RunID: "run-v1-b", State: common.DecisionTicketStateOpen},
			{ID: "ticket-a", WorkspaceID: "ws-a", RunID: "run-v1-a", State: common.DecisionTicketStateOpen},
			{ID: "ticket-v2", WorkspaceID: "ws-v2", RunID: "run-v2", State: common.DecisionTicketStateOpen},
		},
		Approvals: []*approvalmodel.Approval{
			{ID: "approval-b", WorkspaceID: "ws-b", RunID: "run-v1-b", State: common.ApprovalStatePending},
			{ID: "approval-a", WorkspaceID: "ws-a", RunID: "run-v1-a", State: common.ApprovalStatePending},
		},
	}

	first := BuildRuntimeV1RetirementPlan(inventory)
	second := BuildRuntimeV1RetirementPlan(inventory)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("plans differ:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if first.Report.ScannedV1Runs != 3 || first.Report.SupersededRuns != 2 || first.Report.ClearedActiveRunIDs != 3 || first.Report.SupersededTickets != 2 || first.Report.SupersededApprovals != 2 || first.Report.ResourceScopesToRelease != 2 || first.Report.ReleasedResourceScopes != 0 {
		t.Fatalf("report = %+v", first.Report)
	}
	if got := []common.RunID{first.Runs[0].RunID, first.Runs[1].RunID}; !reflect.DeepEqual(got, []common.RunID{"run-v1-a", "run-v1-b"}) {
		t.Fatalf("run actions = %+v", first.Runs)
	}
}

func TestBuildRuntimeV1RetirementPlanHasNoActionsAfterApply(t *testing.T) {
	inventory := RuntimeV1Inventory{
		Runs:       []*runmodel.Run{{ID: "run-v1", WorkspaceID: "ws-v1", State: common.RunStateSuperseded, FailureReason: RuntimeV1RetirementReason}},
		Workspaces: []*workspacemodel.Workspace{{ID: "ws-v1", State: common.WorkspaceStateActive}},
		Tickets:    []*decisionmodel.Ticket{{ID: "ticket-v1", WorkspaceID: "ws-v1", RunID: "run-v1", State: common.DecisionTicketStateSuperseded}},
		Approvals:  []*approvalmodel.Approval{{ID: "approval-v1", WorkspaceID: "ws-v1", RunID: "run-v1", State: common.ApprovalStateSuperseded}},
	}

	plan := BuildRuntimeV1RetirementPlan(inventory)
	if len(plan.Runs) != 0 || len(plan.Workspaces) != 0 || len(plan.Tickets) != 0 || len(plan.Approvals) != 0 {
		t.Fatalf("post-apply plan = %+v, want no actions", plan)
	}
	if plan.Report.ScannedV1Runs != 1 {
		t.Fatalf("scanned V1 runs = %d, want 1", plan.Report.ScannedV1Runs)
	}
}

func TestBuildRuntimeV1RetirementPlanReportsMalformedV1Run(t *testing.T) {
	plan := BuildRuntimeV1RetirementPlan(RuntimeV1Inventory{Runs: []*runmodel.Run{{State: common.RunStateActive}}})
	if len(plan.Report.Failures) != 1 || plan.Report.Failures[0].Reason != "missing_run_or_workspace_id" {
		t.Fatalf("failures = %+v", plan.Report.Failures)
	}
}
