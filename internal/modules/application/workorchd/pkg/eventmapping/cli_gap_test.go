package eventmapping

import (
	"reflect"
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestCLIGapAndStepRoundTripDetachStructuredFields(t *testing.T) {
	step := &projectioncontract.StepView{
		ID: "step-1", Input: map[string]any{"nested": map[string]any{"value": "input"}},
		Output:        map[string]any{"items": []any{map[string]any{"value": "output"}}},
		ContractAudit: map[string]any{"code": "kept"},
	}
	stepDTO, err := stepViewDTO(step)
	if err != nil {
		t.Fatalf("step DTO: %v", err)
	}
	step.Input["nested"].(map[string]any)["value"] = "mutated"
	step.Output["items"].([]any)[0].(map[string]any)["value"] = "mutated"
	stepRoundTrip := stepViewProjection(stepDTO)
	if stepRoundTrip.Input["nested"].(map[string]any)["value"] != "input" || stepRoundTrip.Output["items"].([]any)[0].(map[string]any)["value"] != "output" {
		t.Fatalf("step round trip shares source storage: %#v", stepRoundTrip)
	}

	gap := &projectioncontract.GapBaseline{
		WorkspaceID: "workspace-1", CurrentFocus: map[string]any{"nested": map[string]any{"id": "focus-1"}},
		OpenCoverageWorkItems: []map[string]any{{"id": "cwi-1"}},
		TodoItems: []projectioncontract.GapTodoItem{{
			ID: "todo-1", CoverageWorkItemIDs: []string{"cwi-1"}, RequiredEvidenceKinds: []string{"validation"},
			Metadata: map[string]any{"nested": map[string]any{"reason": "missing"}},
		}},
		AcceptedEvidenceRefs: []string{"accepted-1"}, Diagnostics: []projectioncontract.GapDiagnostic{{Code: "gap"}},
	}
	gapDTO, err := gapBaselineDTO(gap)
	if err != nil {
		t.Fatalf("gap DTO: %v", err)
	}
	gap.CurrentFocus["nested"].(map[string]any)["id"] = "mutated"
	gap.OpenCoverageWorkItems[0]["id"] = "mutated"
	gap.TodoItems[0].CoverageWorkItemIDs[0] = "mutated"
	gap.TodoItems[0].Metadata["nested"].(map[string]any)["reason"] = "mutated"
	got := gapBaselineProjection(gapDTO)
	want := &projectioncontract.GapBaseline{
		WorkspaceID: "workspace-1", CurrentFocus: map[string]any{"nested": map[string]any{"id": "focus-1"}},
		OpenCoverageWorkItems: []map[string]any{{"id": "cwi-1"}},
		TodoItems: []projectioncontract.GapTodoItem{{
			ID: "todo-1", CoverageWorkItemIDs: []string{"cwi-1"}, RequiredEvidenceKinds: []string{"validation"},
			Metadata: map[string]any{"nested": map[string]any{"reason": "missing"}},
		}},
		AcceptedEvidenceRefs: []string{"accepted-1"}, Diagnostics: []projectioncontract.GapDiagnostic{{Code: "gap"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gap round trip = %#v, want %#v", got, want)
	}

	gapDTO.TodoItems[0].CoverageWorkItemIDs[0] = "dto-mutated"
	gapDTO.Diagnostics[0].Code = "dto-mutated"
	if got.TodoItems[0].CoverageWorkItemIDs[0] != "cwi-1" || got.Diagnostics[0].Code != "gap" {
		t.Fatalf("gap projection shares DTO storage: %#v", got)
	}
}
