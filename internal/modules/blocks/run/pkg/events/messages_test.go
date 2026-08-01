package events

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRunStateChangedDataFromModelIncludesOnlyOpaqueRuntimeReference(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
		RuntimeState: &runtimecontract.RuntimeStateRecord{
			StateRevision: 7,
			Facts:         runtimecontract.RuntimeFacts{WorkspaceID: "ws-1", RunID: "run-1"},
			Decision: runtimecontract.RuntimeDecision{
				State:             runtimecontract.ConvergenceStateNeedStateChange,
				AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority-003"},
			},
		},
	}
	data := RunStateChangedDataFromModel(run)
	if data.RuntimeStateRevision != 7 || data.RuntimeStateRef != "runtime-state/ws-1/run-1/7" {
		t.Fatalf("runtime reference data = %+v", data)
	}
}

func TestRunStateChangedDataCarriesOnlyRunOwnedFreshActivationDisposition(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
		RuntimeState: &runtimecontract.RuntimeStateRecord{StateRevision: 7},
		RuntimeActivationStatus: &runmodel.RuntimeActivationStatus{
			Disposition:            runmodel.RuntimeActivationDispositionFreshActivationRequired,
			ActualSchemaVersion:    2,
			SupportedSchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		},
	}
	data := RunStateChangedDataFromModel(run)
	if !data.RequiresFreshActivation() || data.RuntimeActivationDisposition != runmodel.RuntimeActivationDispositionFreshActivationRequired || data.CausationID != "run-state/run-1/7/fresh_activation_required" {
		t.Fatalf("run state fact=%+v", data)
	}
	if err := data.Validate(); err != nil {
		t.Fatalf("validate run state fact: %v", err)
	}
	if data.ActualRuntimeStateSchemaVersion != 2 || data.SupportedRuntimeStateSchemaVersion != runtimecontract.RuntimeStateSchemaVersionCurrent {
		t.Fatalf("fresh activation schema identity=%+v", data)
	}
}

func TestRunContinuationRequestedDataValidate(t *testing.T) {
	if err := (RunContinuationRequestedData{WorkspaceID: "ws-1", RunID: "run-1", CausationID: "resume-1"}).Validate(); err != nil {
		t.Fatalf("valid continuation fact: %v", err)
	}
	if err := (RunContinuationRequestedData{WorkspaceID: "ws-1", RunID: "run-1"}).Validate(); err == nil {
		t.Fatal("continuation without causation must be rejected")
	}
}
