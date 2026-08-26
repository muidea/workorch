package events

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeStateStoreIsOpaquePersistenceNotSM1Transition(t *testing.T) {
	for _, contract := range Contracts() {
		if contract.Topic != TopicRunCommandStoreRuntimeStateRecord {
			continue
		}
		if contract.Metadata.StateMachine != eventcontract.StateMachineNone || contract.Metadata.ReducerOwner != "orchestrator" || contract.Metadata.MachineRole != eventcontract.StateMachineRoleOpaquePersistence || contract.Metadata.EffectClass != eventcontract.EffectClassOpaquePersistence || contract.Metadata.TransitionTarget != eventcontract.TransitionTargetOpaquePersistence {
			t.Fatalf("runtime store metadata = %+v", contract.Metadata)
		}
		return
	}
	t.Fatal("runtime state store contract missing")
}

func TestRuntimeCommandsReadyMetadataIsProcessScopedSM4Handoff(t *testing.T) {
	for _, contract := range Contracts() {
		if contract.Topic != TopicRunRuntimeCommandsReady {
			continue
		}
		if contract.Metadata.StateMachine != eventcontract.StateMachineSM4 ||
			contract.Metadata.ScopePolicy != eventcontract.ScopePolicyProcess ||
			contract.Metadata.IdentityPolicy != eventcontract.IdentityPolicyProcessLifecycle ||
			contract.Metadata.CanonicalFeedback {
			t.Fatalf("runtime commands-ready metadata = %+v", contract.Metadata)
		}
		return
	}
	t.Fatal("runtime commands-ready contract missing")
}

func TestRunStateChangedDataFromModelIncludesOnlyOpaqueRuntimeReference(t *testing.T) {
	run := &runmodel.Run{
		Revision: 3, WorkspaceRunGeneration: 1, ID: "run-1", WorkspaceID: "ws-1", State: common.RunStateActive,
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
	if data.RunRevision != 3 || data.WorkspaceRunGeneration != 1 || data.RuntimeStateRevision != 7 || data.RuntimeStateRef != "runtime-state/ws-1/run-1/7" || data.CausationID != "run-state/run-1/1/3/7" {
		t.Fatalf("runtime reference data = %+v", data)
	}
	if err := data.Validate(); err != nil {
		t.Fatalf("valid Run state fact: %v", err)
	}
}

func TestRunStateChangedDataRejectsOwnerRevisionIdentityDrift(t *testing.T) {
	valid := RunStateChangedData{
		WorkspaceID: "ws-1", RunID: "run-1", RunRevision: 4, State: common.RunStateActive,
		RuntimeStateRevision: 7, RuntimeStateRef: RuntimeStateRef("ws-1", "run-1", 7),
		WorkspaceRunGeneration: 1,
		CausationID:            RunStateChangedCausationID("run-1", 1, 4, 7),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid fact: %v", err)
	}
	invalid := valid
	invalid.RunRevision++
	if err := invalid.Validate(); err == nil {
		t.Fatal("Run revision drift must invalidate causation identity")
	}
	invalid = valid
	invalid.RuntimeStateRevision--
	if err := invalid.Validate(); err == nil {
		t.Fatal("Runtime revision drift must invalidate causation/ref identity")
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
