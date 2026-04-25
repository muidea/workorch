package demo

import (
	"reflect"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	"testing"
)

func TestWorkspaceIntegrationAcrossWorkorchSlices(t *testing.T) {
	for _, tc := range integrationBackends(t) {
		t.Run(tc.name, func(t *testing.T) {
			assertWorkspaceIntegrationAcrossWorkorchSlices(t, tc.slices)
		})
	}
}

func TestApprovalIntegrationAcrossApprovalSlices(t *testing.T) {
	for _, tc := range integrationBackends(t) {
		t.Run(tc.name, func(t *testing.T) {
			assertApprovalIntegrationAcrossApprovalSlices(t, tc.slices)
		})
	}
}

func TestDecisionIntegrationAcrossDecisionSlices(t *testing.T) {
	for _, tc := range integrationBackends(t) {
		t.Run(tc.name, func(t *testing.T) {
			assertDecisionIntegrationAcrossDecisionSlices(t, tc.slices)
		})
	}
}

func TestRunIntegrationTerminalStatesAcrossWorkorchSlices(t *testing.T) {
	for _, tc := range integrationBackends(t) {
		t.Run(tc.name, func(t *testing.T) {
			assertRunIntegrationTerminalStatesAcrossWorkorchSlices(t, tc.slices)
		})
	}
}

func TestOrchestratorAndLegacyIntegrationStepDispatchAcrossRunnableSlices(t *testing.T) {
	for _, tc := range integrationBackends(t) {
		t.Run(tc.name, func(t *testing.T) {
			assertOrchestratorAndLegacyIntegrationStepDispatchAcrossRunnableSlices(t, tc.slices)
		})
	}
}

func TestFileBackendIntegrationRemainsStableAcrossReloads(t *testing.T) {
	dataRoot := t.TempDir()

	first := mustRunAllWithBackend(t, "file", dataRoot)
	second := mustRunAllWithBackend(t, "file", dataRoot)

	assertWorkorchSlicesRemainStableAcrossRuns(t, first, second)
	assertWorkspaceIntegrationAcrossWorkorchSlices(t, second)
	assertApprovalIntegrationAcrossApprovalSlices(t, second)
	assertDecisionIntegrationAcrossDecisionSlices(t, second)
	assertRunIntegrationTerminalStatesAcrossWorkorchSlices(t, second)
	assertOrchestratorAndLegacyIntegrationStepDispatchAcrossRunnableSlices(t, second)
}

type backendSlices struct {
	name   string
	slices []Slice
}

func integrationBackends(t *testing.T) []backendSlices {
	t.Helper()

	return []backendSlices{
		{name: "memory", slices: mustRunAllWithBackend(t, "memory", "")},
		{name: "file", slices: mustRunAllWithBackend(t, "file", t.TempDir())},
	}
}

func assertWorkspaceIntegrationAcrossWorkorchSlices(t *testing.T, slices []Slice) {
	t.Helper()

	for _, item := range slices {
		if item.Snapshot == nil || item.Snapshot.Workspace == nil {
			t.Fatalf("%s missing workspace snapshot", item.Title)
		}
		if item.Snapshot.Workspace.State != "active" {
			t.Fatalf("%s workspace state = %s, want active", item.Title, item.Snapshot.Workspace.State)
		}
		if item.Snapshot.Workspace.ActiveRunID != "" {
			t.Fatalf("%s workspace activeRunId = %q, want empty", item.Title, item.Snapshot.Workspace.ActiveRunID)
		}
		if item.Snapshot.Run == nil {
			t.Fatalf("%s missing run snapshot", item.Title)
		}
		if item.Snapshot.Workspace.ID != item.Snapshot.Run.WorkspaceID {
			t.Fatalf("%s workspace/run mismatch: workspace=%s run.workspaceId=%s", item.Title, item.Snapshot.Workspace.ID, item.Snapshot.Run.WorkspaceID)
		}
	}
}

func assertApprovalIntegrationAcrossApprovalSlices(t *testing.T, slices []Slice) {
	t.Helper()

	runtimeApproval := sliceByTitle(t, slices, "workorch runtime-approval-blocked-resume slice")
	if runtimeApproval.Snapshot.Blocking == nil || runtimeApproval.Snapshot.Blocking.Approval == nil {
		t.Fatal("runtime approval blocked/resume slice missing approval snapshot")
	}
	if runtimeApproval.Snapshot.Blocking.Approval.ID != "approval-run-workorch-runtime-approval-block-runtime-approval" {
		t.Fatalf("runtime approval id = %s, want approval-run-workorch-runtime-approval-block-runtime-approval", runtimeApproval.Snapshot.Blocking.Approval.ID)
	}
	if runtimeApproval.Snapshot.Blocking.Approval.State != "approved" {
		t.Fatalf("runtime approval state = %s, want approved", runtimeApproval.Snapshot.Blocking.Approval.State)
	}
	if runtimeApproval.Snapshot.Run.State != "completed" {
		t.Fatalf("runtime approval blocked/resume run state = %s, want completed", runtimeApproval.Snapshot.Run.State)
	}
	if runtimeApproval.Snapshot.Step == nil || runtimeApproval.Snapshot.Step.ID != "run-workorch-runtime-approval-block/step-2" || runtimeApproval.Snapshot.Step.State != "completed" {
		t.Fatalf("runtime approval blocked/resume step = %+v, want completed step-2", runtimeApproval.Snapshot.Step)
	}
	assertObservedTopic(t, runtimeApproval.Snapshot, "/approval/snapshot")

	runtimeRejected := sliceByTitle(t, slices, "workorch runtime-approval-rejected-terminal slice")
	if runtimeRejected.Snapshot.Blocking == nil || runtimeRejected.Snapshot.Blocking.Approval == nil {
		t.Fatal("runtime approval rejected terminal slice missing approval snapshot")
	}
	if runtimeRejected.Snapshot.Blocking.Approval.ID != "approval-run-workorch-runtime-approval-block-rejected-runtime-approval" {
		t.Fatalf("runtime approval rejected id = %s, want approval-run-workorch-runtime-approval-block-rejected-runtime-approval", runtimeRejected.Snapshot.Blocking.Approval.ID)
	}
	if runtimeRejected.Snapshot.Blocking.Approval.State != "rejected" {
		t.Fatalf("runtime approval rejected state = %s, want rejected", runtimeRejected.Snapshot.Blocking.Approval.State)
	}
	if runtimeRejected.Snapshot.Run.State != "failed" {
		t.Fatalf("runtime approval rejected run state = %s, want failed", runtimeRejected.Snapshot.Run.State)
	}
	if runtimeRejected.Snapshot.Step != nil {
		t.Fatalf("runtime approval rejected terminal step = %+v, want nil", runtimeRejected.Snapshot.Step)
	}
	assertObservedTopic(t, runtimeRejected.Snapshot, "/approval/snapshot")

	mixed := sliceByTitle(t, slices, "workorch runtime-mixed-decision-approval-completed slice")
	if mixed.Snapshot.Blocking == nil || mixed.Snapshot.Blocking.Approval == nil {
		t.Fatal("runtime mixed decision approval slice missing approval snapshot")
	}
	if mixed.Snapshot.Blocking.Approval.ID != "approval-run-workorch-mixed-decision-approval-mixed-approval" {
		t.Fatalf("runtime mixed approval id = %s, want approval-run-workorch-mixed-decision-approval-mixed-approval", mixed.Snapshot.Blocking.Approval.ID)
	}
	if mixed.Snapshot.Blocking.Approval.State != "approved" {
		t.Fatalf("runtime mixed approval state = %s, want approved", mixed.Snapshot.Blocking.Approval.State)
	}
	if mixed.Snapshot.Run.State != "completed" {
		t.Fatalf("runtime mixed decision approval run state = %s, want completed", mixed.Snapshot.Run.State)
	}
	if mixed.Snapshot.Step == nil || mixed.Snapshot.Step.ID != "run-workorch-mixed-decision-approval/step-3" || mixed.Snapshot.Step.State != "completed" {
		t.Fatalf("runtime mixed decision approval step = %+v, want completed step-3", mixed.Snapshot.Step)
	}
	assertObservedTopic(t, mixed.Snapshot, "/approval/snapshot")

	mixedRejected := sliceByTitle(t, slices, "workorch runtime-mixed-decision-approval-rejected-terminal slice")
	if mixedRejected.Snapshot.Blocking == nil || mixedRejected.Snapshot.Blocking.Approval == nil {
		t.Fatal("runtime mixed decision approval rejected slice missing approval snapshot")
	}
	if mixedRejected.Snapshot.Blocking.Approval.ID != "approval-run-workorch-mixed-decision-approval-rejected-mixed-approval" {
		t.Fatalf("runtime mixed rejected approval id = %s, want approval-run-workorch-mixed-decision-approval-rejected-mixed-approval", mixedRejected.Snapshot.Blocking.Approval.ID)
	}
	if mixedRejected.Snapshot.Blocking.Approval.State != "rejected" {
		t.Fatalf("runtime mixed rejected approval state = %s, want rejected", mixedRejected.Snapshot.Blocking.Approval.State)
	}
	if mixedRejected.Snapshot.Run.State != "failed" {
		t.Fatalf("runtime mixed decision approval rejected run state = %s, want failed", mixedRejected.Snapshot.Run.State)
	}
	if mixedRejected.Snapshot.Step != nil {
		t.Fatalf("runtime mixed decision approval rejected step = %+v, want nil", mixedRejected.Snapshot.Step)
	}
	assertObservedTopic(t, mixedRejected.Snapshot, "/approval/snapshot")

	approved := sliceByTitle(t, slices, "workorch approval-blocked-resume slice")
	if approved.Snapshot.Blocking == nil || approved.Snapshot.Blocking.Approval == nil {
		t.Fatal("approval blocked/resume slice missing approval snapshot")
	}
	if approved.Snapshot.Blocking.Approval.State != "approved" {
		t.Fatalf("approval blocked/resume approval state = %s, want approved", approved.Snapshot.Blocking.Approval.State)
	}
	if approved.Snapshot.Run.State != "completed" {
		t.Fatalf("approval blocked/resume run state = %s, want completed", approved.Snapshot.Run.State)
	}
	if approved.Snapshot.Step == nil || approved.Snapshot.Step.State != "completed" {
		t.Fatalf("approval blocked/resume step = %+v, want completed step", approved.Snapshot.Step)
	}
	assertObservedTopic(t, approved.Snapshot, "/approval/snapshot")

	rejected := sliceByTitle(t, slices, "workorch approval-rejected-terminal slice")
	if rejected.Snapshot.Blocking == nil || rejected.Snapshot.Blocking.Approval == nil {
		t.Fatal("approval rejected terminal slice missing approval snapshot")
	}
	if rejected.Snapshot.Blocking.Approval.State != "rejected" {
		t.Fatalf("approval rejected terminal approval state = %s, want rejected", rejected.Snapshot.Blocking.Approval.State)
	}
	if rejected.Snapshot.Run.State != "failed" {
		t.Fatalf("approval rejected terminal run state = %s, want failed", rejected.Snapshot.Run.State)
	}
	if rejected.Snapshot.Step != nil {
		t.Fatalf("approval rejected terminal step = %+v, want nil", rejected.Snapshot.Step)
	}
	assertObservedTopic(t, rejected.Snapshot, "/approval/snapshot")
}

func assertDecisionIntegrationAcrossDecisionSlices(t *testing.T, slices []Slice) {
	t.Helper()

	multiGate := sliceByTitle(t, slices, "workorch runtime-multi-decision-gate-completed slice")
	if multiGate.Snapshot.Blocking == nil || multiGate.Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("runtime multi decision gate slice missing decision ticket snapshot")
	}
	if multiGate.Snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-multi-decision-gate-gate-2" {
		t.Fatalf("runtime multi decision gate ticket id = %s, want ticket-run-workorch-multi-decision-gate-gate-2", multiGate.Snapshot.Blocking.DecisionTicket.ID)
	}
	if multiGate.Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("runtime multi decision gate ticket state = %s, want resolved", multiGate.Snapshot.Blocking.DecisionTicket.State)
	}
	if multiGate.Snapshot.Run.State != "completed" {
		t.Fatalf("runtime multi decision gate run state = %s, want completed", multiGate.Snapshot.Run.State)
	}
	if multiGate.Snapshot.Step == nil || multiGate.Snapshot.Step.ID != "run-workorch-multi-decision-gate/step-3" || multiGate.Snapshot.Step.State != "completed" {
		t.Fatalf("runtime multi decision gate step = %+v, want completed step-3", multiGate.Snapshot.Step)
	}
	assertObservedTopic(t, multiGate.Snapshot, "/decision/ticket/snapshot")

	multiGateRejected := sliceByTitle(t, slices, "workorch runtime-multi-decision-gate-second-gate-rejected-terminal slice")
	if multiGateRejected.Snapshot.Blocking == nil || multiGateRejected.Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("runtime multi decision gate second gate rejected slice missing decision ticket snapshot")
	}
	if multiGateRejected.Snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-multi-decision-gate-rejected-gate-2" {
		t.Fatalf("runtime multi decision gate second gate rejected ticket id = %s, want ticket-run-workorch-multi-decision-gate-rejected-gate-2", multiGateRejected.Snapshot.Blocking.DecisionTicket.ID)
	}
	if multiGateRejected.Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("runtime multi decision gate second gate rejected ticket state = %s, want resolved", multiGateRejected.Snapshot.Blocking.DecisionTicket.State)
	}
	if multiGateRejected.Snapshot.Run.State != "failed" {
		t.Fatalf("runtime multi decision gate second gate rejected run state = %s, want failed", multiGateRejected.Snapshot.Run.State)
	}
	if multiGateRejected.Snapshot.Step != nil {
		t.Fatalf("runtime multi decision gate second gate rejected step = %+v, want nil", multiGateRejected.Snapshot.Step)
	}
	assertObservedTopic(t, multiGateRejected.Snapshot, "/decision/ticket/snapshot")

	runtimeDecision := sliceByTitle(t, slices, "workorch runtime-decision-blocked-resume slice")
	if runtimeDecision.Snapshot.Blocking == nil || runtimeDecision.Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("runtime decision blocked/resume slice missing decision ticket snapshot")
	}
	if runtimeDecision.Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("runtime decision ticket state = %s, want resolved", runtimeDecision.Snapshot.Blocking.DecisionTicket.State)
	}
	if runtimeDecision.Snapshot.Run.State != "completed" {
		t.Fatalf("runtime decision blocked/resume run state = %s, want completed", runtimeDecision.Snapshot.Run.State)
	}
	if runtimeDecision.Snapshot.Step == nil || runtimeDecision.Snapshot.Step.ID != "run-workorch-runtime-decision-block/step-2" || runtimeDecision.Snapshot.Step.State != "completed" {
		t.Fatalf("runtime decision blocked/resume step = %+v, want completed step-2", runtimeDecision.Snapshot.Step)
	}
	assertObservedTopic(t, runtimeDecision.Snapshot, "/decision/ticket/snapshot")

	runtimeRejected := sliceByTitle(t, slices, "workorch runtime-decision-rejected-terminal slice")
	if runtimeRejected.Snapshot.Blocking == nil || runtimeRejected.Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("runtime decision rejected terminal slice missing decision ticket snapshot")
	}
	if runtimeRejected.Snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-runtime-decision-block-rejected-runtime-decision" {
		t.Fatalf("runtime decision rejected ticket id = %s, want ticket-run-workorch-runtime-decision-block-rejected-runtime-decision", runtimeRejected.Snapshot.Blocking.DecisionTicket.ID)
	}
	if runtimeRejected.Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("runtime decision rejected ticket state = %s, want resolved", runtimeRejected.Snapshot.Blocking.DecisionTicket.State)
	}
	if runtimeRejected.Snapshot.Run.State != "failed" {
		t.Fatalf("runtime decision rejected run state = %s, want failed", runtimeRejected.Snapshot.Run.State)
	}
	if runtimeRejected.Snapshot.Step != nil {
		t.Fatalf("runtime decision rejected step = %+v, want nil", runtimeRejected.Snapshot.Step)
	}
	assertObservedTopic(t, runtimeRejected.Snapshot, "/decision/ticket/snapshot")

	mixed := sliceByTitle(t, slices, "workorch runtime-mixed-decision-approval-completed slice")
	if mixed.Snapshot.Blocking == nil || mixed.Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("runtime mixed decision approval slice missing decision ticket snapshot")
	}
	if mixed.Snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-mixed-decision-approval-mixed-decision" {
		t.Fatalf("runtime mixed decision ticket id = %s, want ticket-run-workorch-mixed-decision-approval-mixed-decision", mixed.Snapshot.Blocking.DecisionTicket.ID)
	}
	if mixed.Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("runtime mixed decision ticket state = %s, want resolved", mixed.Snapshot.Blocking.DecisionTicket.State)
	}
	if mixed.Snapshot.Run.State != "completed" {
		t.Fatalf("runtime mixed decision approval run state = %s, want completed", mixed.Snapshot.Run.State)
	}
	if mixed.Snapshot.Step == nil || mixed.Snapshot.Step.ID != "run-workorch-mixed-decision-approval/step-3" || mixed.Snapshot.Step.State != "completed" {
		t.Fatalf("runtime mixed decision approval step = %+v, want completed step-3", mixed.Snapshot.Step)
	}
	assertObservedTopic(t, mixed.Snapshot, "/decision/ticket/snapshot")

	mixedRejected := sliceByTitle(t, slices, "workorch runtime-mixed-decision-approval-rejected-terminal slice")
	if mixedRejected.Snapshot.Blocking == nil || mixedRejected.Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("runtime mixed decision approval rejected slice missing decision ticket snapshot")
	}
	if mixedRejected.Snapshot.Blocking.DecisionTicket.ID != "ticket-run-workorch-mixed-decision-approval-rejected-mixed-decision" {
		t.Fatalf("runtime mixed rejected decision ticket id = %s, want ticket-run-workorch-mixed-decision-approval-rejected-mixed-decision", mixedRejected.Snapshot.Blocking.DecisionTicket.ID)
	}
	if mixedRejected.Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("runtime mixed rejected decision ticket state = %s, want resolved", mixedRejected.Snapshot.Blocking.DecisionTicket.State)
	}
	if mixedRejected.Snapshot.Run.State != "failed" {
		t.Fatalf("runtime mixed decision approval rejected run state = %s, want failed", mixedRejected.Snapshot.Run.State)
	}
	if mixedRejected.Snapshot.Step != nil {
		t.Fatalf("runtime mixed decision approval rejected step = %+v, want nil", mixedRejected.Snapshot.Step)
	}
	assertObservedTopic(t, mixedRejected.Snapshot, "/decision/ticket/snapshot")

	decision := sliceByTitle(t, slices, "workorch decision-blocked-resume slice")
	if decision.Snapshot.Blocking == nil || decision.Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("decision blocked/resume slice missing decision ticket snapshot")
	}
	if decision.Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("decision ticket state = %s, want resolved", decision.Snapshot.Blocking.DecisionTicket.State)
	}
	if decision.Snapshot.Run.State != "completed" {
		t.Fatalf("decision blocked/resume run state = %s, want completed", decision.Snapshot.Run.State)
	}
	if decision.Snapshot.Step == nil || decision.Snapshot.Step.State != "completed" {
		t.Fatalf("decision blocked/resume step = %+v, want completed step", decision.Snapshot.Step)
	}
	assertObservedTopic(t, decision.Snapshot, "/decision/ticket/snapshot")

	rejected := sliceByTitle(t, slices, "workorch decision-rejected-terminal slice")
	if rejected.Snapshot.Blocking == nil || rejected.Snapshot.Blocking.DecisionTicket == nil {
		t.Fatal("decision rejected terminal slice missing decision ticket snapshot")
	}
	if rejected.Snapshot.Blocking.DecisionTicket.State != "resolved" {
		t.Fatalf("decision rejected terminal ticket state = %s, want resolved", rejected.Snapshot.Blocking.DecisionTicket.State)
	}
	if rejected.Snapshot.Run.State != "failed" {
		t.Fatalf("decision rejected terminal run state = %s, want failed", rejected.Snapshot.Run.State)
	}
	if rejected.Snapshot.Step != nil {
		t.Fatalf("decision rejected terminal step = %+v, want nil", rejected.Snapshot.Step)
	}
	assertObservedTopic(t, rejected.Snapshot, "/decision/ticket/snapshot")
}

func assertRunIntegrationTerminalStatesAcrossWorkorchSlices(t *testing.T, slices []Slice) {
	t.Helper()

	wantStates := map[string]string{
		"workorch runnable slice":                                                  "completed",
		"workorch runtime-failed-terminal slice":                                   "failed",
		"workorch multi-step-completed slice":                                      "completed",
		"workorch conditional-branch-completed slice":                              "completed",
		"workorch conditional-branch-review-completed slice":                       "completed",
		"workorch runtime-multi-decision-gate-completed slice":                     "completed",
		"workorch runtime-multi-decision-gate-second-gate-rejected-terminal slice": "failed",
		"workorch runtime-decision-blocked-resume slice":                           "completed",
		"workorch runtime-decision-rejected-terminal slice":                        "failed",
		"workorch runtime-approval-blocked-resume slice":                           "completed",
		"workorch runtime-approval-rejected-terminal slice":                        "failed",
		"workorch runtime-mixed-decision-approval-completed slice":                 "completed",
		"workorch runtime-mixed-decision-approval-rejected-terminal slice":         "failed",
		"workorch decision-blocked-resume slice":                                   "completed",
		"workorch decision-rejected-terminal slice":                                "failed",
		"workorch approval-blocked-resume slice":                                   "completed",
		"workorch approval-rejected-terminal slice":                                "failed",
	}

	for _, item := range slices {
		if item.Snapshot == nil || item.Snapshot.Run == nil {
			t.Fatalf("%s missing run snapshot", item.Title)
		}
		if item.Snapshot.Run.State != wantStates[item.Title] {
			t.Fatalf("%s run state = %s, want %s", item.Title, item.Snapshot.Run.State, wantStates[item.Title])
		}
		if item.Snapshot.Run.BlockingTicketID != "" || item.Snapshot.Run.BlockingApprovalID != "" {
			t.Fatalf("%s run blocking refs not cleared: ticket=%q approval=%q", item.Title, item.Snapshot.Run.BlockingTicketID, item.Snapshot.Run.BlockingApprovalID)
		}
	}
}

func assertOrchestratorAndLegacyIntegrationStepDispatchAcrossRunnableSlices(t *testing.T, slices []Slice) {
	t.Helper()

	for _, title := range []string{
		"workorch runnable slice",
		"workorch multi-step-completed slice",
		"workorch conditional-branch-completed slice",
		"workorch conditional-branch-review-completed slice",
		"workorch runtime-multi-decision-gate-completed slice",
		"workorch runtime-decision-blocked-resume slice",
		"workorch runtime-approval-blocked-resume slice",
		"workorch runtime-mixed-decision-approval-completed slice",
		"workorch decision-blocked-resume slice",
		"workorch approval-blocked-resume slice",
	} {
		item := sliceByTitle(t, slices, title)
		if item.Snapshot.Step == nil {
			t.Fatalf("%s missing step snapshot", title)
		}
		if item.Snapshot.Step.Kind != "legacy/minimal" {
			t.Fatalf("%s step kind = %s, want legacy/minimal", title, item.Snapshot.Step.Kind)
		}
		if item.Snapshot.Step.State != "completed" {
			t.Fatalf("%s step state = %s, want completed", title, item.Snapshot.Step.State)
		}
		if item.Snapshot.Step.Output["status"] != "completed" {
			t.Fatalf("%s step output status = %v, want completed", title, item.Snapshot.Step.Output["status"])
		}
		if item.Snapshot.Step.Output["runId"] != item.Snapshot.Run.ID {
			t.Fatalf("%s step output runId = %v, want %s", title, item.Snapshot.Step.Output["runId"], item.Snapshot.Run.ID)
		}
		if item.Snapshot.Step.Output["stepId"] != item.Snapshot.Step.ID {
			t.Fatalf("%s step output stepId = %v, want %s", title, item.Snapshot.Step.Output["stepId"], item.Snapshot.Step.ID)
		}
		assertObservedTopic(t, item.Snapshot, "/step/snapshot")
	}

	runtimeFailed := sliceByTitle(t, slices, "workorch runtime-failed-terminal slice")
	if runtimeFailed.Snapshot.Step == nil {
		t.Fatal("runtime failed terminal slice missing step snapshot")
	}
	if runtimeFailed.Snapshot.Step.Kind != "legacy/minimal" {
		t.Fatalf("runtime failed terminal step kind = %s, want legacy/minimal", runtimeFailed.Snapshot.Step.Kind)
	}
	if runtimeFailed.Snapshot.Step.State != "failed" {
		t.Fatalf("runtime failed terminal step state = %s, want failed", runtimeFailed.Snapshot.Step.State)
	}
	if runtimeFailed.Snapshot.Step.Output["reason"] == nil {
		t.Fatalf("runtime failed terminal step output missing reason: %+v", runtimeFailed.Snapshot.Step.Output)
	}
	if runtimeFailed.Snapshot.Run.State != "failed" {
		t.Fatalf("runtime failed terminal run state = %s, want failed", runtimeFailed.Snapshot.Run.State)
	}
	assertObservedTopic(t, runtimeFailed.Snapshot, "/step/snapshot")

	multiStep := sliceByTitle(t, slices, "workorch multi-step-completed slice")
	if multiStep.Snapshot.Step == nil || multiStep.Snapshot.Step.ID != "run-workorch-multi-step/step-2" {
		t.Fatalf("multi-step completed final step = %+v, want step-2", multiStep.Snapshot.Step)
	}
	if len(multiStep.Snapshot.Steps) != 2 {
		t.Fatalf("multi-step completed steps = %+v, want two steps", multiStep.Snapshot.Steps)
	}
	if multiStep.Snapshot.Steps[0].ID != "run-workorch-multi-step/step-1" || multiStep.Snapshot.Steps[1].ID != "run-workorch-multi-step/step-2" {
		t.Fatalf("multi-step completed step order = %+v, want step-1 then step-2", multiStep.Snapshot.Steps)
	}
	assertObservedTopic(t, multiStep.Snapshot, "/step/snapshot")

	conditionalBranch := sliceByTitle(t, slices, "workorch conditional-branch-completed slice")
	if conditionalBranch.Snapshot.Step == nil || conditionalBranch.Snapshot.Step.ID != "run-workorch-conditional-branch/step-fast" {
		t.Fatalf("conditional branch final step = %+v, want step-fast", conditionalBranch.Snapshot.Step)
	}
	if len(conditionalBranch.Snapshot.Steps) != 2 {
		t.Fatalf("conditional branch steps = %+v, want two steps", conditionalBranch.Snapshot.Steps)
	}
	if conditionalBranch.Snapshot.Steps[0].ID != "run-workorch-conditional-branch/step-1" || conditionalBranch.Snapshot.Steps[1].ID != "run-workorch-conditional-branch/step-fast" {
		t.Fatalf("conditional branch step order = %+v, want step-1 then step-fast", conditionalBranch.Snapshot.Steps)
	}
	if conditionalBranch.Snapshot.Steps[0].Output["branch"] != "fast-track" {
		t.Fatalf("conditional branch output = %v, want fast-track", conditionalBranch.Snapshot.Steps[0].Output["branch"])
	}
	assertObservedTopic(t, conditionalBranch.Snapshot, "/step/snapshot")

	conditionalReview := sliceByTitle(t, slices, "workorch conditional-branch-review-completed slice")
	if conditionalReview.Snapshot.Step == nil || conditionalReview.Snapshot.Step.ID != "run-workorch-conditional-branch-branch-slow/step-review" {
		t.Fatalf("conditional review final step = %+v, want step-review", conditionalReview.Snapshot.Step)
	}
	if len(conditionalReview.Snapshot.Steps) != 2 {
		t.Fatalf("conditional review steps = %+v, want two steps", conditionalReview.Snapshot.Steps)
	}
	if conditionalReview.Snapshot.Steps[0].ID != "run-workorch-conditional-branch-branch-slow/step-1" || conditionalReview.Snapshot.Steps[1].ID != "run-workorch-conditional-branch-branch-slow/step-review" {
		t.Fatalf("conditional review step order = %+v, want step-1 then step-review", conditionalReview.Snapshot.Steps)
	}
	if conditionalReview.Snapshot.Steps[0].Output["branch"] != "review-path" {
		t.Fatalf("conditional review output = %v, want review-path", conditionalReview.Snapshot.Steps[0].Output["branch"])
	}
	assertObservedTopic(t, conditionalReview.Snapshot, "/step/snapshot")

	multiGate := sliceByTitle(t, slices, "workorch runtime-multi-decision-gate-completed slice")
	if multiGate.Snapshot.Step == nil || multiGate.Snapshot.Step.ID != "run-workorch-multi-decision-gate/step-3" {
		t.Fatalf("runtime multi decision gate final step = %+v, want step-3", multiGate.Snapshot.Step)
	}
	if len(multiGate.Snapshot.Steps) != 3 {
		t.Fatalf("runtime multi decision gate steps = %+v, want three steps", multiGate.Snapshot.Steps)
	}
	if multiGate.Snapshot.Steps[0].ID != "run-workorch-multi-decision-gate/step-1" || multiGate.Snapshot.Steps[1].ID != "run-workorch-multi-decision-gate/step-2" || multiGate.Snapshot.Steps[2].ID != "run-workorch-multi-decision-gate/step-3" {
		t.Fatalf("runtime multi decision gate step order = %+v, want step-1 then step-2 then step-3", multiGate.Snapshot.Steps)
	}
	if multiGate.Snapshot.Steps[0].Output["blockDecisionTicketId"] != "ticket-run-workorch-multi-decision-gate-gate-1" {
		t.Fatalf("runtime multi decision gate step-1 output = %v, want ticket-run-workorch-multi-decision-gate-gate-1", multiGate.Snapshot.Steps[0].Output["blockDecisionTicketId"])
	}
	if multiGate.Snapshot.Steps[1].Output["blockDecisionTicketId"] != "ticket-run-workorch-multi-decision-gate-gate-2" {
		t.Fatalf("runtime multi decision gate step-2 output = %v, want ticket-run-workorch-multi-decision-gate-gate-2", multiGate.Snapshot.Steps[1].Output["blockDecisionTicketId"])
	}
	assertObservedTopic(t, multiGate.Snapshot, "/step/snapshot")

	multiGateRejected := sliceByTitle(t, slices, "workorch runtime-multi-decision-gate-second-gate-rejected-terminal slice")
	if multiGateRejected.Snapshot.Step != nil {
		t.Fatalf("runtime multi decision gate second gate rejected step = %+v, want nil", multiGateRejected.Snapshot.Step)
	}
	if len(multiGateRejected.Snapshot.Steps) != 2 {
		t.Fatalf("runtime multi decision gate second gate rejected steps = %+v, want two steps", multiGateRejected.Snapshot.Steps)
	}
	if multiGateRejected.Snapshot.Steps[0].ID != "run-workorch-multi-decision-gate-rejected/step-1" || multiGateRejected.Snapshot.Steps[1].ID != "run-workorch-multi-decision-gate-rejected/step-2" {
		t.Fatalf("runtime multi decision gate second gate rejected step order = %+v, want step-1 then step-2", multiGateRejected.Snapshot.Steps)
	}
	if multiGateRejected.Snapshot.Steps[0].Output["blockDecisionTicketId"] != "ticket-run-workorch-multi-decision-gate-rejected-gate-1" {
		t.Fatalf("runtime multi decision gate second gate rejected step-1 output = %v, want ticket-run-workorch-multi-decision-gate-rejected-gate-1", multiGateRejected.Snapshot.Steps[0].Output["blockDecisionTicketId"])
	}
	if multiGateRejected.Snapshot.Steps[1].Output["blockDecisionTicketId"] != "ticket-run-workorch-multi-decision-gate-rejected-gate-2" {
		t.Fatalf("runtime multi decision gate second gate rejected step-2 output = %v, want ticket-run-workorch-multi-decision-gate-rejected-gate-2", multiGateRejected.Snapshot.Steps[1].Output["blockDecisionTicketId"])
	}
	assertObservedTopic(t, multiGateRejected.Snapshot, "/step/snapshot")

	runtimeDecision := sliceByTitle(t, slices, "workorch runtime-decision-blocked-resume slice")
	if len(runtimeDecision.Snapshot.Steps) != 2 {
		t.Fatalf("runtime decision blocked/resume steps = %+v, want two steps", runtimeDecision.Snapshot.Steps)
	}
	if runtimeDecision.Snapshot.Steps[0].ID != "run-workorch-runtime-decision-block/step-1" || runtimeDecision.Snapshot.Steps[1].ID != "run-workorch-runtime-decision-block/step-2" {
		t.Fatalf("runtime decision blocked/resume step order = %+v, want step-1 then step-2", runtimeDecision.Snapshot.Steps)
	}
	assertObservedTopic(t, runtimeDecision.Snapshot, "/step/snapshot")

	runtimeDecisionRejected := sliceByTitle(t, slices, "workorch runtime-decision-rejected-terminal slice")
	if runtimeDecisionRejected.Snapshot.Step != nil {
		t.Fatalf("runtime decision rejected terminal step = %+v, want nil", runtimeDecisionRejected.Snapshot.Step)
	}
	if len(runtimeDecisionRejected.Snapshot.Steps) != 1 {
		t.Fatalf("runtime decision rejected terminal steps = %+v, want one step", runtimeDecisionRejected.Snapshot.Steps)
	}
	if runtimeDecisionRejected.Snapshot.Steps[0].ID != "run-workorch-runtime-decision-block-rejected/step-1" {
		t.Fatalf("runtime decision rejected terminal step order = %+v, want step-1", runtimeDecisionRejected.Snapshot.Steps)
	}
	if runtimeDecisionRejected.Snapshot.Steps[0].Output["blockDecisionTicketId"] != "ticket-run-workorch-runtime-decision-block-rejected-runtime-decision" {
		t.Fatalf("runtime decision rejected terminal step-1 output = %v, want ticket-run-workorch-runtime-decision-block-rejected-runtime-decision", runtimeDecisionRejected.Snapshot.Steps[0].Output["blockDecisionTicketId"])
	}
	assertObservedTopic(t, runtimeDecisionRejected.Snapshot, "/step/snapshot")

	runtimeApproval := sliceByTitle(t, slices, "workorch runtime-approval-blocked-resume slice")
	if len(runtimeApproval.Snapshot.Steps) != 2 {
		t.Fatalf("runtime approval blocked/resume steps = %+v, want two steps", runtimeApproval.Snapshot.Steps)
	}
	if runtimeApproval.Snapshot.Steps[0].ID != "run-workorch-runtime-approval-block/step-1" || runtimeApproval.Snapshot.Steps[1].ID != "run-workorch-runtime-approval-block/step-2" {
		t.Fatalf("runtime approval blocked/resume step order = %+v, want step-1 then step-2", runtimeApproval.Snapshot.Steps)
	}
	if runtimeApproval.Snapshot.Steps[0].Output["blockApprovalId"] != "approval-run-workorch-runtime-approval-block-runtime-approval" {
		t.Fatalf("runtime approval blocked/resume step-1 output = %v, want approval-run-workorch-runtime-approval-block-runtime-approval", runtimeApproval.Snapshot.Steps[0].Output["blockApprovalId"])
	}
	assertObservedTopic(t, runtimeApproval.Snapshot, "/step/snapshot")

	runtimeRejected := sliceByTitle(t, slices, "workorch runtime-approval-rejected-terminal slice")
	if runtimeRejected.Snapshot.Step != nil {
		t.Fatalf("runtime approval rejected terminal step = %+v, want nil", runtimeRejected.Snapshot.Step)
	}
	if len(runtimeRejected.Snapshot.Steps) != 1 {
		t.Fatalf("runtime approval rejected terminal steps = %+v, want one step", runtimeRejected.Snapshot.Steps)
	}
	if runtimeRejected.Snapshot.Steps[0].ID != "run-workorch-runtime-approval-block-rejected/step-1" {
		t.Fatalf("runtime approval rejected terminal step order = %+v, want step-1", runtimeRejected.Snapshot.Steps)
	}
	if runtimeRejected.Snapshot.Steps[0].Output["blockApprovalId"] != "approval-run-workorch-runtime-approval-block-rejected-runtime-approval" {
		t.Fatalf("runtime approval rejected terminal step-1 output = %v, want approval-run-workorch-runtime-approval-block-rejected-runtime-approval", runtimeRejected.Snapshot.Steps[0].Output["blockApprovalId"])
	}
	assertObservedTopic(t, runtimeRejected.Snapshot, "/step/snapshot")

	mixed := sliceByTitle(t, slices, "workorch runtime-mixed-decision-approval-completed slice")
	if mixed.Snapshot.Step == nil || mixed.Snapshot.Step.ID != "run-workorch-mixed-decision-approval/step-3" {
		t.Fatalf("runtime mixed decision approval final step = %+v, want step-3", mixed.Snapshot.Step)
	}
	if len(mixed.Snapshot.Steps) != 3 {
		t.Fatalf("runtime mixed decision approval steps = %+v, want three steps", mixed.Snapshot.Steps)
	}
	if mixed.Snapshot.Steps[0].ID != "run-workorch-mixed-decision-approval/step-1" || mixed.Snapshot.Steps[1].ID != "run-workorch-mixed-decision-approval/step-2" || mixed.Snapshot.Steps[2].ID != "run-workorch-mixed-decision-approval/step-3" {
		t.Fatalf("runtime mixed decision approval step order = %+v, want step-1 then step-2 then step-3", mixed.Snapshot.Steps)
	}
	if mixed.Snapshot.Steps[0].Output["blockDecisionTicketId"] != "ticket-run-workorch-mixed-decision-approval-mixed-decision" {
		t.Fatalf("runtime mixed decision approval step-1 output = %v, want ticket-run-workorch-mixed-decision-approval-mixed-decision", mixed.Snapshot.Steps[0].Output["blockDecisionTicketId"])
	}
	if mixed.Snapshot.Steps[1].Output["blockApprovalId"] != "approval-run-workorch-mixed-decision-approval-mixed-approval" {
		t.Fatalf("runtime mixed decision approval step-2 output = %v, want approval-run-workorch-mixed-decision-approval-mixed-approval", mixed.Snapshot.Steps[1].Output["blockApprovalId"])
	}
	assertObservedTopic(t, mixed.Snapshot, "/step/snapshot")

	mixedRejected := sliceByTitle(t, slices, "workorch runtime-mixed-decision-approval-rejected-terminal slice")
	if mixedRejected.Snapshot.Step != nil {
		t.Fatalf("runtime mixed decision approval rejected terminal step = %+v, want nil", mixedRejected.Snapshot.Step)
	}
	if len(mixedRejected.Snapshot.Steps) != 2 {
		t.Fatalf("runtime mixed decision approval rejected terminal steps = %+v, want two steps", mixedRejected.Snapshot.Steps)
	}
	if mixedRejected.Snapshot.Steps[0].ID != "run-workorch-mixed-decision-approval-rejected/step-1" || mixedRejected.Snapshot.Steps[1].ID != "run-workorch-mixed-decision-approval-rejected/step-2" {
		t.Fatalf("runtime mixed decision approval rejected terminal step order = %+v, want step-1 then step-2", mixedRejected.Snapshot.Steps)
	}
	if mixedRejected.Snapshot.Steps[0].Output["blockDecisionTicketId"] != "ticket-run-workorch-mixed-decision-approval-rejected-mixed-decision" {
		t.Fatalf("runtime mixed decision approval rejected terminal step-1 output = %v, want ticket-run-workorch-mixed-decision-approval-rejected-mixed-decision", mixedRejected.Snapshot.Steps[0].Output["blockDecisionTicketId"])
	}
	if mixedRejected.Snapshot.Steps[1].Output["blockApprovalId"] != "approval-run-workorch-mixed-decision-approval-rejected-mixed-approval" {
		t.Fatalf("runtime mixed decision approval rejected terminal step-2 output = %v, want approval-run-workorch-mixed-decision-approval-rejected-mixed-approval", mixedRejected.Snapshot.Steps[1].Output["blockApprovalId"])
	}
	assertObservedTopic(t, mixedRejected.Snapshot, "/step/snapshot")

	decisionRejected := sliceByTitle(t, slices, "workorch decision-rejected-terminal slice")
	if decisionRejected.Snapshot.Step != nil {
		t.Fatalf("decision rejected terminal step = %+v, want nil", decisionRejected.Snapshot.Step)
	}
	assertObservedTopicAbsent(t, decisionRejected.Snapshot, "/step/snapshot")

	rejected := sliceByTitle(t, slices, "workorch approval-rejected-terminal slice")
	if rejected.Snapshot.Step != nil {
		t.Fatalf("approval rejected terminal step = %+v, want nil", rejected.Snapshot.Step)
	}
	assertObservedTopicAbsent(t, rejected.Snapshot, "/step/snapshot")
}

func sliceByTitle(t *testing.T, slices []Slice, title string) Slice {
	t.Helper()

	for _, item := range slices {
		if item.Title == title {
			return item
		}
	}
	t.Fatalf("slice %q not found", title)
	return Slice{}
}

func assertObservedTopic(t *testing.T, snapshot *presenter.Snapshot, topic string) {
	t.Helper()

	for _, item := range snapshot.Observed {
		if item.Topic == topic {
			return
		}
	}
	t.Fatalf("snapshot %s missing observed topic %s", snapshot.Slice, topic)
}

func assertObservedTopicAbsent(t *testing.T, snapshot *presenter.Snapshot, topic string) {
	t.Helper()

	for _, item := range snapshot.Observed {
		if item.Topic == topic {
			t.Fatalf("snapshot %s unexpectedly contains observed topic %s", snapshot.Slice, topic)
		}
	}
}

func assertWorkorchSlicesRemainStableAcrossRuns(t *testing.T, first []Slice, second []Slice) {
	t.Helper()

	if len(first) != len(second) {
		t.Fatalf("slice count mismatch after reload: first=%d second=%d", len(first), len(second))
	}

	for idx := range first {
		assertSliceRemainsStableAcrossRuns(t, idx, first[idx], second[idx])
	}
}

func assertSliceRemainsStableAcrossRuns(t *testing.T, index int, first Slice, second Slice) {
	t.Helper()

	if first.Title != second.Title {
		t.Fatalf("slice %d title mismatch after reload: first=%q second=%q", index, first.Title, second.Title)
	}
	if first.Snapshot == nil || second.Snapshot == nil {
		t.Fatalf("slice %q snapshot nil after reload: first=%v second=%v", first.Title, first.Snapshot == nil, second.Snapshot == nil)
	}

	assertObservedTopicsMatch(t, first.Title, first.Snapshot, second.Snapshot)
	assertWorkspaceViewMatches(t, first.Title, first.Snapshot, second.Snapshot)
	assertRunViewMatches(t, first.Title, first.Snapshot, second.Snapshot)
	assertStepViewMatches(t, first.Title, first.Snapshot, second.Snapshot)
	assertBlockingViewMatches(t, first.Title, first.Snapshot, second.Snapshot)
}

func assertObservedTopicsMatch(t *testing.T, title string, first *presenter.Snapshot, second *presenter.Snapshot) {
	t.Helper()

	if len(first.Observed) != len(second.Observed) {
		t.Fatalf("%s observed count mismatch after reload: first=%d second=%d", title, len(first.Observed), len(second.Observed))
	}
	for idx := range first.Observed {
		left := first.Observed[idx]
		right := second.Observed[idx]
		if left.ID != right.ID ||
			left.Version != right.Version ||
			left.Topic != right.Topic ||
			left.SourceModule != right.SourceModule ||
			left.SubjectType != right.SubjectType ||
			left.SubjectID != right.SubjectID ||
			left.WorkspaceID != right.WorkspaceID ||
			left.RunID != right.RunID ||
			left.StepID != right.StepID {
			t.Fatalf("%s observed[%d] mismatch after reload: first=%+v second=%+v", title, idx, left, right)
		}
		if !reflect.DeepEqual(left.Payload, right.Payload) {
			t.Fatalf("%s observed payload[%d] mismatch after reload: first=%+v second=%+v", title, idx, left.Payload, right.Payload)
		}
	}
}

func assertWorkspaceViewMatches(t *testing.T, title string, first *presenter.Snapshot, second *presenter.Snapshot) {
	t.Helper()

	if first.Workspace == nil || second.Workspace == nil {
		t.Fatalf("%s workspace snapshot nil after reload: first=%v second=%v", title, first.Workspace == nil, second.Workspace == nil)
	}
	if *first.Workspace != *second.Workspace {
		t.Fatalf("%s workspace mismatch after reload: first=%+v second=%+v", title, *first.Workspace, *second.Workspace)
	}
}

func assertRunViewMatches(t *testing.T, title string, first *presenter.Snapshot, second *presenter.Snapshot) {
	t.Helper()

	if first.Run == nil || second.Run == nil {
		t.Fatalf("%s run snapshot nil after reload: first=%v second=%v", title, first.Run == nil, second.Run == nil)
	}
	if *first.Run != *second.Run {
		t.Fatalf("%s run mismatch after reload: first=%+v second=%+v", title, *first.Run, *second.Run)
	}
}

func assertStepViewMatches(t *testing.T, title string, first *presenter.Snapshot, second *presenter.Snapshot) {
	t.Helper()

	if (first.Step == nil) != (second.Step == nil) {
		t.Fatalf("%s step presence mismatch after reload: first=%v second=%v", title, first.Step == nil, second.Step == nil)
	}
	if first.Step == nil {
		return
	}
	if first.Step.ID != second.Step.ID || first.Step.Kind != second.Step.Kind || first.Step.State != second.Step.State {
		t.Fatalf("%s step mismatch after reload: first=%+v second=%+v", title, *first.Step, *second.Step)
	}
	if !reflect.DeepEqual(first.Step.Input, second.Step.Input) {
		t.Fatalf("%s step input mismatch after reload: first=%+v second=%+v", title, first.Step.Input, second.Step.Input)
	}
	if !reflect.DeepEqual(first.Step.Output, second.Step.Output) {
		t.Fatalf("%s step output mismatch after reload: first=%+v second=%+v", title, first.Step.Output, second.Step.Output)
	}
	if len(first.Steps) != len(second.Steps) {
		t.Fatalf("%s steps count mismatch after reload: first=%d second=%d", title, len(first.Steps), len(second.Steps))
	}
	for idx := range first.Steps {
		if first.Steps[idx].ID != second.Steps[idx].ID ||
			first.Steps[idx].Kind != second.Steps[idx].Kind ||
			first.Steps[idx].State != second.Steps[idx].State {
			t.Fatalf("%s steps[%d] mismatch after reload: first=%+v second=%+v", title, idx, first.Steps[idx], second.Steps[idx])
		}
		if !reflect.DeepEqual(first.Steps[idx].Input, second.Steps[idx].Input) {
			t.Fatalf("%s steps[%d] input mismatch after reload: first=%+v second=%+v", title, idx, first.Steps[idx].Input, second.Steps[idx].Input)
		}
		if !reflect.DeepEqual(first.Steps[idx].Output, second.Steps[idx].Output) {
			t.Fatalf("%s steps[%d] output mismatch after reload: first=%+v second=%+v", title, idx, first.Steps[idx].Output, second.Steps[idx].Output)
		}
	}
}

func assertBlockingViewMatches(t *testing.T, title string, first *presenter.Snapshot, second *presenter.Snapshot) {
	t.Helper()

	if (first.Blocking == nil) != (second.Blocking == nil) {
		t.Fatalf("%s blocking presence mismatch after reload: first=%v second=%v", title, first.Blocking == nil, second.Blocking == nil)
	}
	if first.Blocking == nil {
		return
	}

	if (first.Blocking.DecisionTicket == nil) != (second.Blocking.DecisionTicket == nil) {
		t.Fatalf("%s decision ticket presence mismatch after reload: first=%v second=%v", title, first.Blocking.DecisionTicket == nil, second.Blocking.DecisionTicket == nil)
	}
	if first.Blocking.DecisionTicket != nil && *first.Blocking.DecisionTicket != *second.Blocking.DecisionTicket {
		t.Fatalf("%s decision ticket mismatch after reload: first=%+v second=%+v", title, *first.Blocking.DecisionTicket, *second.Blocking.DecisionTicket)
	}

	if (first.Blocking.Approval == nil) != (second.Blocking.Approval == nil) {
		t.Fatalf("%s approval presence mismatch after reload: first=%v second=%v", title, first.Blocking.Approval == nil, second.Blocking.Approval == nil)
	}
	if first.Blocking.Approval != nil && *first.Blocking.Approval != *second.Blocking.Approval {
		t.Fatalf("%s approval mismatch after reload: first=%+v second=%+v", title, *first.Blocking.Approval, *second.Blocking.Approval)
	}
}
