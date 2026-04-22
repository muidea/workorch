package demo

import (
	"github.com/muidea/workorch/application/workorchd/presenter"
	"testing"
)

func TestWorkspaceIntegrationAcrossPhase1Slices(t *testing.T) {
	for _, tc := range integrationBackends(t) {
		t.Run(tc.name, func(t *testing.T) {
			assertWorkspaceIntegrationAcrossPhase1Slices(t, tc.slices)
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

func TestDecisionIntegrationAcrossDecisionSlice(t *testing.T) {
	for _, tc := range integrationBackends(t) {
		t.Run(tc.name, func(t *testing.T) {
			assertDecisionIntegrationAcrossDecisionSlice(t, tc.slices)
		})
	}
}

func TestRunIntegrationTerminalStatesAcrossPhase1Slices(t *testing.T) {
	for _, tc := range integrationBackends(t) {
		t.Run(tc.name, func(t *testing.T) {
			assertRunIntegrationTerminalStatesAcrossPhase1Slices(t, tc.slices)
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

	assertPhase1SlicesRemainStableAcrossRuns(t, first, second)
	assertWorkspaceIntegrationAcrossPhase1Slices(t, second)
	assertApprovalIntegrationAcrossApprovalSlices(t, second)
	assertDecisionIntegrationAcrossDecisionSlice(t, second)
	assertRunIntegrationTerminalStatesAcrossPhase1Slices(t, second)
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

func assertWorkspaceIntegrationAcrossPhase1Slices(t *testing.T, slices []Slice) {
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

	approved := sliceByTitle(t, slices, "phase1 approval-blocked-resume slice")
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

	rejected := sliceByTitle(t, slices, "phase1 approval-rejected-terminal slice")
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

func assertDecisionIntegrationAcrossDecisionSlice(t *testing.T, slices []Slice) {
	t.Helper()

	decision := sliceByTitle(t, slices, "phase1 decision-blocked-resume slice")
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
}

func assertRunIntegrationTerminalStatesAcrossPhase1Slices(t *testing.T, slices []Slice) {
	t.Helper()

	wantStates := map[string]string{
		"phase1 runnable slice":                   "completed",
		"phase1 decision-blocked-resume slice":    "completed",
		"phase1 approval-blocked-resume slice":    "completed",
		"phase1 approval-rejected-terminal slice": "failed",
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
		"phase1 runnable slice",
		"phase1 decision-blocked-resume slice",
		"phase1 approval-blocked-resume slice",
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

	rejected := sliceByTitle(t, slices, "phase1 approval-rejected-terminal slice")
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

func assertPhase1SlicesRemainStableAcrossRuns(t *testing.T, first []Slice, second []Slice) {
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
		if first.Observed[idx].Topic != second.Observed[idx].Topic {
			t.Fatalf("%s observed topic[%d] mismatch after reload: first=%s second=%s", title, idx, first.Observed[idx].Topic, second.Observed[idx].Topic)
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
