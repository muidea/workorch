package events

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestApplyRunProjectionFactCommandValidate(t *testing.T) {
	cmd := ApplyRunProjectionFactCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "run-state/run-1/1",
		RunState: common.RunStateActive, RuntimeStateRevision: 1, RuntimeStateRef: "runtime-state/ws-1/run-1/1",
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid projection: %v", err)
	}
	preRuntime := ApplyRunProjectionFactCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "run-state/run-1/0",
		RunState: common.RunStateActive,
	}
	if err := preRuntime.Validate(); err != nil {
		t.Fatalf("valid pre-runtime activation projection: %v", err)
	}
	preRuntime.RunState = common.RunStateBlockedOnDecision
	if err := preRuntime.Validate(); err == nil {
		t.Fatal("pre-runtime governance projection must be rejected")
	}
	cmd.RunState = common.RunStateCompleted
	if err := cmd.Validate(); err == nil {
		t.Fatal("expected unsupported terminal run state")
	}
}

func TestWorkspaceExecutionIntentResultUsesTypedAdmissionWithoutInventedRun(t *testing.T) {
	result := WorkspaceExecutionIntentResult{
		WorkspaceID: "ws-1", RequestID: "request-1", Kind: runtimecontract.RuntimeExecutionIntentResume,
		Outcome: runtimecontract.RuntimeExecutionIntentRestartRequired,
		Reason:  runtimecontract.RuntimeExecutionIntentReasonWorkspaceActiveRunAbsent,
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.Outcome = runtimecontract.RuntimeExecutionIntentReconciliationRequested
	if err := result.Validate(); err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("mismatched admission err = %v", err)
	}
}

func TestReleaseFreshActivationRunSlotFactCommandValidate(t *testing.T) {
	cmd := ReleaseFreshActivationRunSlotFactCommand{
		WorkspaceID:                        "ws-1",
		RunID:                              "run-1",
		SourceOwner:                        "blocks/run",
		CausationID:                        "run-state/run-1/7/fresh_activation_required",
		RuntimeStateRevision:               7,
		RuntimeStateRef:                    "runtime-state/ws-1/run-1/7",
		RuntimeActivationDisposition:       runtimecontract.RuntimeActivationDispositionFreshActivationRequired,
		ActualRuntimeStateSchemaVersion:    2,
		SupportedRuntimeStateSchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid fresh activation release: %v", err)
	}
	cmd.RuntimeActivationDisposition = ""
	if err := cmd.Validate(); err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("missing disposition err=%v", err)
	}
}

func TestWorkspaceApplyRuntimeTerminalDeliveryCommandValidate(t *testing.T) {
	cmd := ApplyRuntimeTerminalDeliveryCommand{
		RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryIdentity: runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, SourceOwner: "kernel/orchestrator", CausationID: "terminal-delivery/td-1"},
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid terminal: %v", err)
	}
	if err := (ApplyRuntimeTerminalDeliveryCommand{}).Validate(); err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty terminal err = %v", err)
	}
}
