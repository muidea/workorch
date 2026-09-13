package workorchruntimecontract

import (
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDurableDispatchBudgetCountsOriginalSealsNotOperationsOrResumes(t *testing.T) {
	dispatch := RuntimeDispatchRecord{ID: "original", CoverageBindings: []CoverageEvidenceBinding{
		{CoverageWorkItemID: "a"}, {CoverageWorkItemID: "a"}, {CoverageWorkItemID: "b"},
	}, OperationExecutions: []RuntimeOperationExecutionRecord{
		{ExpectedEffectScope: RuntimeEffectWorkspaceDurable}, {ExpectedEffectScope: RuntimeEffectRemoteDurable},
	}}
	attempts := []RuntimeCandidateAttemptRecord{{State: RuntimeCandidateAttemptStateAccepted, DispatchID: dispatch.ID}}
	resume := dispatch
	resume.ID = "governance-resume"
	for _, state := range []RuntimeDispatchState{RuntimeDispatchStateSealed, RuntimeDispatchStateCommitted, RuntimeDispatchStateInvalidated} {
		dispatch.State = state
		usage, err := RuntimeDurableDispatchUsage(&resume, []RuntimeDispatchRecord{dispatch}, attempts)
		if err != nil || !reflect.DeepEqual(usage, map[common.CoverageWorkItemID]int{"a": 1, "b": 1}) {
			t.Fatalf("state %s refunded/duplicated original seal: %v %v", state, usage, err)
		}
	}
	limits := &RuntimeExecutionLimits{ProtocolRef: RuntimeExecutionLimitsV1, MaxProviderCalls: 8, MaxCapabilityOperations: 8, MaxDurableDispatchesPerCWI: 1}
	next := dispatch
	next.ID = "new-generation-repair"
	var exhausted *RuntimeDurableDispatchBudgetExhaustedError
	if err := CheckRuntimeDurableDispatchBudget(limits, nil, []RuntimeDispatchRecord{dispatch}, attempts, next); !errors.As(err, &exhausted) || exhausted.CoverageWorkItemID != "a" {
		t.Fatalf("same-CWI new work did not exhaust deterministically: %v", err)
	}
	next.CoverageBindings = []CoverageEvidenceBinding{{CoverageWorkItemID: "c"}}
	if err := CheckRuntimeDurableDispatchBudget(limits, nil, []RuntimeDispatchRecord{dispatch}, attempts, next); err != nil {
		t.Fatal(err)
	}
	next.OperationExecutions = []RuntimeOperationExecutionRecord{{ExpectedEffectScope: RuntimeEffectNone}}
	next.CoverageBindings = dispatch.CoverageBindings
	if err := CheckRuntimeDurableDispatchBudget(limits, nil, []RuntimeDispatchRecord{dispatch}, attempts, next); err != nil {
		t.Fatal("read-only work spent durable budget", err)
	}
	if err := ValidateRuntimeDurableDispatchLimits(limits, &resume, []RuntimeDispatchRecord{dispatch}, attempts); err != nil {
		t.Fatal(err)
	}
	// A second original accepted seal is corruption at fresh load, not a refund.
	second := dispatch
	second.ID = "second"
	if err := ValidateRuntimeDurableDispatchLimits(limits, &second, []RuntimeDispatchRecord{dispatch}, append(attempts, RuntimeCandidateAttemptRecord{State: RuntimeCandidateAttemptStateAccepted, DispatchID: second.ID})); err == nil {
		t.Fatal("persisted over-consumption accepted")
	}
	if _, err := RuntimeDurableDispatchUsage(&dispatch, []RuntimeDispatchRecord{dispatch}, attempts); err == nil {
		t.Fatal("duplicate dispatch accepted")
	}
	if _, err := RuntimeDurableDispatchUsage(nil, nil, attempts); err == nil {
		t.Fatal("missing accepted dispatch accepted")
	}
	if _, err := RuntimeDurableDispatchUsage(&dispatch, nil, append(attempts, attempts[0])); err == nil {
		t.Fatal("duplicate accepted origin accepted")
	}
	dispatch.CoverageBindings = nil
	if _, err := RuntimeDurableDispatchUsage(&dispatch, nil, attempts); err == nil {
		t.Fatal("ungrounded durable work accepted")
	}
}
