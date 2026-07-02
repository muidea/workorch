package readmodelports

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestPortWorkspaceSnapshotInfoPreservesTypedProjectionFields(t *testing.T) {
	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID:               "ws-map",
			State:            string(common.WorkspaceStateRunning),
			ActiveRunID:      "run-map",
			BlockingTicketID: "ticket-map",
		},
		ViewModel: readmodelcore.WorkspaceViewModel{
			WorkspaceID: "ws-map",
			State:       "running",
			PromptMode:  "decision",
			Revision:    7,
			Sections: map[string]readmodelcore.WorkspaceViewModelSection{
				"prompt": {Name: "prompt", Revision: 7, State: "decision"},
			},
		},
		RunHeartbeat: readmodelcore.RunHeartbeat{
			WorkspaceID: "ws-map",
			RunID:       "run-map",
			State:       "running",
			Stalled:     true,
		},
		RuntimeExecution: &readmodelcore.RuntimeExecutionState{
			StepID:    "step-map",
			StepKind:  "llm/workspace_execution",
			StepState: "running",
		},
		ConvergenceStatus: &readmodelcore.ConvergenceStatus{
			WorkspaceID:                "ws-map",
			RunID:                      "run-map",
			RunState:                   string(common.RunStateRunning),
			NextActionKind:             "produce_evidence",
			RetryBudgetScope:           map[string]any{"focus": map[string]any{"id": "focus-1"}},
			MissingEvidenceKinds:       []string{"artifact"},
			IneffectiveRetry:           true,
			ManagedValidationAvailable: true,
		},
		RunCommand: &readmodelcore.RunCommandStatus{
			WorkspaceID: "ws-map",
			RunID:       "run-map",
			State:       "accepted",
			Intent:      "replan",
			DecisionID:  "ticket-map",
			Revision:    9,
		},
		GapBaseline: &readmodelcore.GapBaseline{
			WorkspaceID:          "ws-map",
			RunID:                "run-map",
			BaselineRevision:     "gap-1",
			AcceptedEvidenceRefs: []string{"evidence-a"},
			CurrentFocus:         map[string]any{"id": "focus-1"},
		},
		Runs: []readmodelcore.Run{{
			ID:               "run-map",
			WorkspaceID:      "ws-map",
			State:            string(common.RunStateRunning),
			ExecutionContext: map[string]any{"goal": map[string]any{"text": "goal"}},
		}},
		Decisions: []readmodelcore.Decision{{
			ID:          "ticket-map",
			WorkspaceID: "ws-map",
			RunID:       "run-map",
			State:       "open",
		}},
		EventRecords: []readmodelcore.EventRecord{{
			ID:             "event-map",
			WorkspaceID:    "ws-map",
			RunID:          "run-map",
			Payload:        map[string]any{"raw": "large event payload"},
			PayloadSummary: map[string]any{"state": "running"},
			PayloadRef:     "event://payload/event-map",
		}},
	}

	got := PortWorkspaceSnapshotInfo(snapshot)
	if got.Workspace.ID != "ws-map" || got.Workspace.ActiveRunID != "run-map" {
		t.Fatalf("workspace = %+v, want typed workspace IDs", got.Workspace)
	}
	if got.ViewModel.PromptMode != "decision" || got.ViewModel.Sections["prompt"].Revision != 7 {
		t.Fatalf("viewModel = %+v, want prompt projection", got.ViewModel)
	}
	if got.RunHeartbeat.RunID != "run-map" || !got.RunHeartbeat.Stalled {
		t.Fatalf("runHeartbeat = %+v, want heartbeat fields", got.RunHeartbeat)
	}
	if got.RuntimeExecution == nil || got.RuntimeExecution.StepID != "step-map" {
		t.Fatalf("runtimeExecution = %+v, want step-map", got.RuntimeExecution)
	}
	if got.ConvergenceStatus == nil || got.ConvergenceStatus.NextActionKind != "produce_evidence" || !got.ConvergenceStatus.ManagedValidationAvailable {
		t.Fatalf("convergence = %+v, want convergence fields", got.ConvergenceStatus)
	}
	if got.RunCommand == nil || got.RunCommand.DecisionID != "ticket-map" || got.RunCommand.Revision != 9 {
		t.Fatalf("runCommand = %+v, want decision command metadata", got.RunCommand)
	}
	if got.GapBaseline == nil || got.GapBaseline.AcceptedEvidenceRefs[0] != "evidence-a" {
		t.Fatalf("gapBaseline = %+v, want evidence refs", got.GapBaseline)
	}
	if got.Runs[0].ExecutionContext["goal"].(map[string]any)["text"] != "goal" {
		t.Fatalf("run context = %+v, want goal context", got.Runs[0].ExecutionContext)
	}
	if got.Decisions[0].ID != "ticket-map" || got.EventRecords[0].PayloadSummary["state"] != "running" {
		t.Fatalf("governance/event = %+v/%+v, want mapped values", got.Decisions, got.EventRecords)
	}
	if got.EventRecords[0].Payload != nil || got.EventRecords[0].PayloadRef != "event://payload/event-map" {
		t.Fatalf("event record = %+v, want snapshot projection with payload ref and no raw payload", got.EventRecords[0])
	}

	snapshot.Runs[0].ExecutionContext["goal"].(map[string]any)["text"] = "mutated"
	if got.Runs[0].ExecutionContext["goal"].(map[string]any)["text"] != "goal" {
		t.Fatalf("run context was aliased after source mutation: %+v", got.Runs[0].ExecutionContext)
	}
	snapshot.ConvergenceStatus.RetryBudgetScope["focus"].(map[string]any)["id"] = "mutated"
	if got.ConvergenceStatus.RetryBudgetScope["focus"].(map[string]any)["id"] != "focus-1" {
		t.Fatalf("retry budget scope was aliased after source mutation: %+v", got.ConvergenceStatus.RetryBudgetScope)
	}
}
