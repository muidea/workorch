package runtimecore

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMapRunProjectsLineageAndLifecycleTimestamps(t *testing.T) {
	item := &runmodel.Run{
		ID:          "run-successor",
		WorkspaceID: "ws-run-lineage",
		GoalID:      "goal-run-lineage",
		DoDID:       "dod-run-lineage",
		Lineage: runmodel.RunLineage{
			PredecessorRunID:    "run-predecessor",
			PredecessorResultID: "result-predecessor",
			SuccessorRunID:      "run-next",
			LineageState:        "active",
		},
		State:     common.RunStateActive,
		CreatedAt: "2026-07-06T03:00:00Z",
		StartedAt: "2026-07-06T03:01:00Z",
		EndedAt:   "2026-07-06T03:02:00Z",
	}

	got := MapRun(item)
	if got.Lineage.PredecessorRunID != "run-predecessor" ||
		got.Lineage.PredecessorResultID != "result-predecessor" ||
		got.Lineage.SuccessorRunID != "run-next" ||
		got.Lineage.LineageState != "active" ||
		got.CreatedAt != item.CreatedAt ||
		got.StartedAt != item.StartedAt ||
		got.EndedAt != item.EndedAt {
		t.Fatalf("mapped run = %+v, want lineage and timestamps projected", got)
	}
}

func TestMapRunProjectsSealedProviderSelectionWithoutAliasingBootstrap(t *testing.T) {
	sealed := runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "provider-sealed", ModelID: "model-sealed"}, ReasoningEffort: "high"}
	item := &runmodel.Run{
		ID:          "run-sealed-target",
		WorkspaceID: "ws-sealed-target",
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{
			Activation: runtimecontract.RuntimeActivationContext{ProviderSelection: sealed},
		},
	}

	got := MapRun(item)
	if got.SealedLLMSelection == nil || !got.SealedLLMSelection.Same(sealed) {
		t.Fatalf("sealed target = %+v, want %+v", got.SealedLLMSelection, sealed)
	}
	got.SealedLLMSelection.Target.ModelID = "mutated-read-model"
	if item.RuntimeBootstrap.Activation.ProviderSelection.Target.ModelID != "model-sealed" {
		t.Fatalf("read model target mutation changed bootstrap = %+v", item.RuntimeBootstrap.Activation.ProviderSelection.Target)
	}
}

func TestMapRunPreservesPrivateOwnerFencesWithoutJSONExposure(t *testing.T) {
	item := &runmodel.Run{
		ID: "run-owner", WorkspaceID: "workspace-owner", StartupInputHash: " owner startup hash ",
		TerminalResultID: " owner terminal result ", WorkspaceRunGeneration: 41,
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{Definition: runtimecontract.RuntimeDefinitionContext{StartupInputHash: "must-not-rebuild-from-bootstrap"}},
	}
	got := MapRun(item)
	if got.StartupInputHash != item.StartupInputHash || got.TerminalResultID != string(item.TerminalResultID) || got.WorkspaceRunGeneration != item.WorkspaceRunGeneration {
		t.Fatalf("owner fence mapping lost original fields: %+v", got)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"startupInputHash", "terminalResultId", "terminalResultID", "workspaceRunGeneration", "owner startup hash", "owner terminal result"} {
		if strings.Contains(string(data), name) {
			t.Fatalf("private owner fence exposed in JSON: %s", data)
		}
	}
}

func TestMapRunDoesNotRetainRuntimeAuthorityOrCompatibilityState(t *testing.T) {
	got := MapRun(&runmodel.Run{ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive})
	for _, field := range []string{"RuntimeState", "RuntimeActivation", "RuntimeActivationStatus"} {
		if _, ok := reflect.TypeOf(got).FieldByName(field); ok {
			t.Fatalf("application run read model retains %s", field)
		}
	}
}
