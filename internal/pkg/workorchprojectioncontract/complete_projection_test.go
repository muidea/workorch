package workorchprojectioncontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestCompleteProjectionRoundTripPreservesDiagnosticAndResolutionFields(t *testing.T) {
	payload, err := workorcheventpayload.FromMap(map[string]any{"answer": "continue"})
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	want := struct {
		Decision Decision             `json:"decision"`
		Progress TaskProgress         `json:"progress"`
		Memory   MemoryRecallTrace    `json:"memory"`
		Runtime  WorkspaceRuntimeView `json:"runtime"`
	}{
		Decision: Decision{
			ID: "decision-1", WorkspaceID: "workspace-1", RunID: "run-1", StepID: "step-1",
			DecisionKind: "recovery", FailureClass: "contract", FailureSignature: "sig-1",
			ResolutionID: "resolution-1", ResolutionType: "provide_missing_info", ResolutionPayload: payload,
			ResolvedAt: "2026-08-04T10:00:00Z", ResolvedBy: "user", CreatedAt: "2026-08-04T09:00:00Z", UpdatedAt: "2026-08-04T10:00:00Z",
		},
		Progress: TaskProgress{
			WorkspaceID: "workspace-1", RunID: "run-1", BlockingTicketID: "decision-1",
			Items:        []ProgressItem{{ID: "cwi-1", Kind: "coverage", State: "active"}},
			Blockers:     []ProgressBlocker{{ID: "decision-1", Kind: "decision", Reason: "missing input"}},
			EvidenceRefs: []string{"evidence-1"},
			RepairLoop: RepairLoop{
				MissingDoDItems:      []map[string]any{{"id": "dod-1"}},
				RecentAttempts:       []map[string]any{{"attempt": float64(2)}},
				RecommendedNextSteps: []string{"change operation shape"},
			},
		},
		Memory: MemoryRecallTrace{
			ID: "trace-1", RefreshSource: "runtime", SelectionPolicyVersion: "v2",
			CandidateMemoryIDs: []string{"memory-1"}, SelectedMemoryIDs: []string{"memory-1"},
			TypeQuota: map[string]int{"semantic": 1}, SelectionScores: []MemoryRecallSelectionScore{{ID: "memory-1", Score: 10}},
		},
		Runtime: WorkspaceRuntimeView{
			Projection: WorkspaceViewModel{StreamGeneration: "generation-1", FailedWorkspaceIds: []string{"workspace-2"}, DegradedSince: "2026-08-04T08:00:00Z"},
			RunCommand: &RunCommandStatus{Disposition: "accepted", Diagnostic: "scheduled", QueuedAt: "2026-08-04T09:30:00Z"},
			Activity:   WorkspaceActivityFeed{StreamGeneration: "generation-1"},
		},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got typeofCompleteProjectionFixture
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wantComparable := typeofCompleteProjectionFixture(want)
	if !reflect.DeepEqual(got, wantComparable) {
		t.Fatalf("round trip changed projection\nwant=%+v\n got=%+v", wantComparable, got)
	}
}

type typeofCompleteProjectionFixture struct {
	Decision Decision             `json:"decision"`
	Progress TaskProgress         `json:"progress"`
	Memory   MemoryRecallTrace    `json:"memory"`
	Runtime  WorkspaceRuntimeView `json:"runtime"`
}
