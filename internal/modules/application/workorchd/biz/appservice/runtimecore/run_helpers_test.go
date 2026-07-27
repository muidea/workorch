package runtimecore

import (
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

func TestMapRunProjectsSealedProviderTargetWithoutAliasingBootstrap(t *testing.T) {
	sealed := runtimecontract.ProviderModelTarget{ProviderID: "provider-sealed", ModelID: "model-sealed"}
	item := &runmodel.Run{
		ID:          "run-sealed-target",
		WorkspaceID: "ws-sealed-target",
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{
			Activation: runtimecontract.RuntimeActivationContext{ProviderTarget: sealed},
		},
	}

	got := MapRun(item)
	if got.SealedLLMTarget == nil || !got.SealedLLMTarget.Same(sealed) {
		t.Fatalf("sealed target = %+v, want %+v", got.SealedLLMTarget, sealed)
	}
	got.SealedLLMTarget.ModelID = "mutated-read-model"
	if item.RuntimeBootstrap.Activation.ProviderTarget.ModelID != "model-sealed" {
		t.Fatalf("read model target mutation changed bootstrap = %+v", item.RuntimeBootstrap.Activation.ProviderTarget)
	}
}
