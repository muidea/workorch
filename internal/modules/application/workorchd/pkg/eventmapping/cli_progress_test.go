package eventmapping

import (
	"reflect"
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestCLITaskProgressRoundTripDetachesRepairDiagnostics(t *testing.T) {
	input := projectioncontract.TaskProgress{
		WorkspaceID: "workspace-1", RunID: "run-1",
		Milestones: []projectioncontract.ProgressMilestone{{ID: "milestone-1", EvidenceRefs: []string{"milestone-evidence"}}},
		Items:      []projectioncontract.ProgressItem{{ID: "item-1", EvidenceRefs: []string{"item-evidence"}}},
		RepairLoop: projectioncontract.RepairLoop{
			StructuredFailureSignals: map[string]any{"nested": map[string]any{"code": "contract"}},
			RepairInputCompaction:    map[string]any{"kept": true},
			MissingDoDItems:          []map[string]any{{"id": "dod-1"}},
			RecentAttempts:           []map[string]any{{"attempt": float64(1)}}, EvidenceRefs: []string{"repair-evidence"},
		},
		EvidenceRefs: []string{"progress-evidence"},
		Runtime:      &projectioncontract.RuntimeProgress{MissingEvidenceKinds: []string{"validation"}},
	}
	dto, err := taskProgressDTO(input)
	if err != nil {
		t.Fatalf("task progress DTO: %v", err)
	}
	input.Milestones[0].EvidenceRefs[0] = "mutated"
	input.RepairLoop.StructuredFailureSignals["nested"].(map[string]any)["code"] = "mutated"
	input.RepairLoop.MissingDoDItems[0]["id"] = "mutated"
	input.Runtime.MissingEvidenceKinds[0] = "mutated"

	got := taskProgressProjection(dto)
	want := projectioncontract.CloneTaskProgress(projectioncontract.TaskProgress{
		WorkspaceID: "workspace-1", RunID: "run-1",
		Milestones: []projectioncontract.ProgressMilestone{{ID: "milestone-1", EvidenceRefs: []string{"milestone-evidence"}}},
		Items:      []projectioncontract.ProgressItem{{ID: "item-1", EvidenceRefs: []string{"item-evidence"}}},
		RepairLoop: projectioncontract.RepairLoop{
			StructuredFailureSignals: map[string]any{"nested": map[string]any{"code": "contract"}},
			RepairInputCompaction:    map[string]any{"kept": true},
			MissingDoDItems:          []map[string]any{{"id": "dod-1"}},
			RecentAttempts:           []map[string]any{{"attempt": float64(1)}}, EvidenceRefs: []string{"repair-evidence"},
		},
		EvidenceRefs: []string{"progress-evidence"},
		Runtime:      &projectioncontract.RuntimeProgress{MissingEvidenceKinds: []string{"validation"}},
	})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("task progress round trip = %#v, want %#v", got, want)
	}

	dto.Milestones[0].EvidenceRefs[0] = "dto-mutated"
	dto.RepairLoop.EvidenceRefs[0] = "dto-mutated"
	dto.Runtime.MissingEvidenceKinds[0] = "dto-mutated"
	if got.Milestones[0].EvidenceRefs[0] != "milestone-evidence" || got.RepairLoop.EvidenceRefs[0] != "repair-evidence" || got.Runtime.MissingEvidenceKinds[0] != "validation" {
		t.Fatalf("task progress projection shares DTO storage: %#v", got)
	}
}
