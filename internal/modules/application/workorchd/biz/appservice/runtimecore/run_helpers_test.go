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

func TestMapRunProjectsFreshActivationStatusWithoutRuntimeState(t *testing.T) {
	item := &runmodel.Run{
		ID:          "run-incompatible-schema",
		WorkspaceID: "ws-incompatible-schema",
		State:       common.RunStateActive,
		RuntimeActivationStatus: &runmodel.RuntimeActivationStatus{
			Disposition:     runtimecontract.RuntimeActivationDispositionFreshActivationRequired,
			RuntimeStateRef: "runtime-state/ws-incompatible-schema/run-incompatible-schema",
			Incompatibility: runtimecontract.RuntimeStateIncompatibility{
				Reason:                 runtimecontract.RuntimeStateIncompatibilityReasonSchemaUnsupported,
				ActualSchemaVersion:    2,
				SupportedSchemaVersion: 4,
			},
		},
	}

	got := MapRun(item)
	if got.RuntimeActivation == nil ||
		got.RuntimeActivation.Disposition != string(runtimecontract.RuntimeActivationDispositionFreshActivationRequired) ||
		got.RuntimeActivation.ActualSchemaVersion != 2 ||
		got.RuntimeActivation.SupportedSchemaVersion != 4 {
		t.Fatalf("runtime activation = %+v, want bounded fresh-activation projection", got.RuntimeActivation)
	}
	if got.RuntimeState != nil {
		t.Fatalf("runtime state = %+v, want opaque state excluded from read model", got.RuntimeState)
	}
}

func TestMapRunProjectsCWILedgerFreshActivationReasonWithoutRuntimeState(t *testing.T) {
	item := &runmodel.Run{
		ID:          "run-invalid-cwi-ledger",
		WorkspaceID: "ws-invalid-cwi-ledger",
		State:       common.RunStateActive,
		RuntimeActivationStatus: &runmodel.RuntimeActivationStatus{
			Disposition:     runtimecontract.RuntimeActivationDispositionFreshActivationRequired,
			RuntimeStateRef: "runtime-state/ws-invalid-cwi-ledger/run-invalid-cwi-ledger/4",
			Incompatibility: runtimecontract.RuntimeStateIncompatibility{
				Reason:                 runtimecontract.RuntimeStateIncompatibilityReasonCWILedgerInvalid,
				ActualSchemaVersion:    runtimecontract.RuntimeStateSchemaVersionCurrent,
				SupportedSchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
			},
		},
	}

	got := MapRun(item)
	if got.RuntimeActivation == nil ||
		got.RuntimeActivation.Disposition != string(runtimecontract.RuntimeActivationDispositionFreshActivationRequired) ||
		got.RuntimeActivation.Reason != string(runtimecontract.RuntimeStateIncompatibilityReasonCWILedgerInvalid) ||
		got.RuntimeActivation.ActualSchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent ||
		got.RuntimeActivation.SupportedSchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent {
		t.Fatalf("runtime activation = %+v, want CWI ledger fresh-activation projection", got.RuntimeActivation)
	}
	if got.RuntimeState != nil {
		t.Fatalf("runtime state = %+v, want opaque state excluded from read model", got.RuntimeState)
	}
}
