package eventmapping

import (
	"reflect"
	"testing"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCLITaskProjectionRoundTripDetachesMutableFields(t *testing.T) {
	input := map[string]any{
		"nested": map[string]any{"name": "original"},
		"items":  []any{map[string]any{"value": "original"}},
	}
	target := &runtimecontract.ProviderModelTarget{ProviderID: "provider", ModelID: "model"}
	activation := &projectioncontract.RuntimeActivationStatus{Disposition: "ready"}
	plans := []projectioncontract.TaskPlan{{
		ID: "plan-1", WorkspaceID: "workspace-1", RunID: "run-1", AnchorStepID: "step-1",
		Nodes: []projectioncontract.TaskDecompositionNode{{
			ID: "node-1", DependsOn: []string{"node-0"}, EvidenceRefs: []string{"evidence-1"},
			CoverageTargets: []string{"target-1"}, Input: input,
		}},
		EvidenceRefs: []string{"plan-evidence-1"}, SealedLLMTarget: target, RuntimeActivation: activation,
	}}

	dto, err := taskPlanDTOs(plans)
	if err != nil {
		t.Fatalf("task plan DTOs: %v", err)
	}
	plans[0].Nodes[0].DependsOn[0] = "mutated"
	plans[0].Nodes[0].EvidenceRefs[0] = "mutated"
	plans[0].Nodes[0].CoverageTargets[0] = "mutated"
	input["nested"].(map[string]any)["name"] = "mutated"
	input["items"].([]any)[0].(map[string]any)["value"] = "mutated"
	target.ModelID = "mutated"
	activation.Disposition = "mutated"

	roundTrip := taskPlanProjections(dto)
	want := []projectioncontract.TaskPlan{{
		ID: "plan-1", WorkspaceID: "workspace-1", RunID: "run-1", AnchorStepID: "step-1",
		Nodes: []projectioncontract.TaskDecompositionNode{{
			ID: "node-1", DependsOn: []string{"node-0"}, EvidenceRefs: []string{"evidence-1"},
			CoverageTargets: []string{"target-1"}, Input: map[string]any{
				"nested": map[string]any{"name": "original"},
				"items":  []any{map[string]any{"value": "original"}},
			},
		}},
		EvidenceRefs:      []string{"plan-evidence-1"},
		SealedLLMTarget:   &runtimecontract.ProviderModelTarget{ProviderID: "provider", ModelID: "model"},
		RuntimeActivation: &projectioncontract.RuntimeActivationStatus{Disposition: "ready"},
	}}
	if !reflect.DeepEqual(roundTrip, want) {
		t.Fatalf("task plan round trip = %#v, want %#v", roundTrip, want)
	}

	dto[0].Nodes[0].DependsOn[0] = "dto-mutated"
	dto[0].Nodes[0].Input = dto[0].Nodes[0].Input.Clone()
	if roundTrip[0].Nodes[0].DependsOn[0] != "node-0" || roundTrip[0].Nodes[0].Input["nested"].(map[string]any)["name"] != "original" {
		t.Fatalf("task projection shares DTO storage: %#v", roundTrip[0])
	}
}

func TestCLITaskProjectionPreservesNilAndEmptyCollections(t *testing.T) {
	if got, err := taskDecompositionDTOs(nil); err != nil || got != nil {
		t.Fatalf("nil decomposition DTOs = %#v, err=%v", got, err)
	}
	if got := taskDecompositionProjections(nil); got != nil {
		t.Fatalf("nil decomposition projections = %#v", got)
	}
	if got, err := taskPlanDTOs([]projectioncontract.TaskPlan{}); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty task plan DTOs = %#v, err=%v", got, err)
	}
	if got := taskPlanProjections([]workorchdevents.CLITaskPlanInfo{}); got == nil {
		t.Fatal("empty task plan projections must remain non-nil")
	}
}
