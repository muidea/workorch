package readmodelports

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func mustPortEventPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatalf("event payload: %v", err)
	}
	return payload
}

func TestPortWorkspaceSnapshotInfoPreservesTypedProjectionFields(t *testing.T) {
	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID:               "ws-map",
			State:            string(common.WorkspaceStateActive),
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
		ConvergenceStatus: &readmodelcore.ConvergenceStatus{
			WorkspaceID:          "ws-map",
			RunID:                "run-map",
			RunState:             string(common.RunStateActive),
			NextActionKind:       "produce_evidence",
			MissingEvidenceKinds: []string{"artifact"},
			IneffectiveRetry:     true,
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
			ID:          "run-map",
			WorkspaceID: "ws-map",
			State:       string(common.RunStateActive),
		}},
		Decisions: []readmodelcore.Decision{{
			ID:          "ticket-map",
			WorkspaceID: "ws-map",
			RunID:       "run-map",
			State:       "open",
		}},
		EventRecords: []readmodelcore.EventRecord{{
			ID:          "event-map",
			SubjectType: "run",
			WorkspaceID: "ws-map",
			RunID:       "run-map",
			Payload: mustPortEventPayload(t, map[string]any{
				"state": "active",
			}),
			PayloadRef: "event://payload/event-map",
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
	if got.ConvergenceStatus == nil || got.ConvergenceStatus.NextActionKind != "produce_evidence" || !got.ConvergenceStatus.IneffectiveRetry {
		t.Fatalf("convergence = %+v, want convergence fields", got.ConvergenceStatus)
	}
	if got.RunCommand == nil || got.RunCommand.DecisionID != "ticket-map" || got.RunCommand.Revision != 9 {
		t.Fatalf("runCommand = %+v, want decision command metadata", got.RunCommand)
	}
	if got.GapBaseline == nil || got.GapBaseline.AcceptedEvidenceRefs[0] != "evidence-a" {
		t.Fatalf("gapBaseline = %+v, want evidence refs", got.GapBaseline)
	}
	if got.Decisions[0].ID != "ticket-map" || len(got.EventRecords[0].Payload.Fields) != 1 {
		t.Fatalf("governance/event = %+v/%+v, want mapped values", got.Decisions, got.EventRecords)
	}
	if got.EventRecords[0].Summary != "active" || got.EventRecords[0].PayloadRef != "event://payload/event-map" {
		t.Fatalf("event record = %+v, want typed payload summary and payload ref", got.EventRecords[0])
	}
}

func TestPortWorkspaceSnapshotInfoDoesNotFallbackToPayloadSummaryMap(t *testing.T) {
	got := PortWorkspaceSnapshotInfo(readmodelcore.WorkspaceSnapshot{
		EventRecords: []readmodelcore.EventRecord{{
			ID:             "event-summary-only",
			SubjectType:    "run",
			PayloadSummary: mustPortEventPayload(t, map[string]any{"state": "active"}),
		}},
	})
	if len(got.EventRecords) != 0 {
		t.Fatalf("event records = %+v, want summary-only map record rejected", got.EventRecords)
	}
}
