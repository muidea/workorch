package orchestrator

import (
	"strings"
	"testing"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestAttachCurrentActionContractFromAuthority(t *testing.T) {
	run := &runModel.Run{ID: "run-001", WorkspaceID: "ws-current-action"}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction":   map[string]any{"kind": "state_change", "reason": "need state change"},
			"currentFocus": map[string]any{"id": "cwi-001", "coverageWorkItemIds": []any{"cwi-001"}},
		},
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"stateChangingOutputRequired": true,
		stepInputKeyStagePacket: map[string]any{
			"schemaVersion": "workorch.stage_packet.v1",
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"coverageWorkItemIds":          []any{"cwi-001"},
				"acceptedKinds":                []any{"patch/apply"},
			},
		},
	}

	got := attachCurrentActionContractToInput(run, common.StepID("run-001/step-001"), input)
	contract := mapFromValue(got[currentActionContractKey])
	if contract["schemaVersion"] != currentActionContractSchema ||
		contract["workspaceId"] != string(run.WorkspaceID) ||
		contract["runId"] != string(run.ID) ||
		contract["stepId"] != "run-001/step-001" {
		t.Fatalf("currentActionContract identity = %+v", contract)
	}
	if next := mapFromValue(contract["nextAction"]); next["kind"] != "state_change" {
		t.Fatalf("nextAction = %+v, want state_change", next)
	}
	if required := mapFromValue(contract["requiredOutput"]); required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["coverageWorkItemIds"]), "cwi-001") {
		t.Fatalf("requiredOutput = %+v, want state-changing cwi-001 contract", required)
	}
	if err := validateConvergenceDispatchInput(got); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v", err)
	}
}

func TestValidateConvergenceDispatchInputRejectsCurrentActionContractDrift(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": "state_change"},
		},
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"stateChangingOutputRequired": true,
		stepInputKeyStagePacket:       map[string]any{"schemaVersion": "workorch.stage_packet.v1"},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []any{"patch/apply"},
			},
		},
		currentActionContractKey: map[string]any{
			"nextAction":     map[string]any{"kind": "validate"},
			"requiredOutput": map[string]any{"mustIncludeStateChangingNode": true, "acceptedKinds": []any{"patch/apply"}},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil || !strings.Contains(err.Error(), "currentActionContract nextAction=validate expected=state_change") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want currentActionContract drift", err)
	}
}
