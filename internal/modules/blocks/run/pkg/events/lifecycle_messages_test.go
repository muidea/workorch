package events

import (
	"errors"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWorkspaceExecutionIntentResultUsesTypedAdmission(t *testing.T) {
	result := WorkspaceExecutionIntentResult{
		WorkspaceID: "ws-1", RunID: "run-1", RequestID: "request-1", Kind: runtimecontract.RuntimeExecutionIntentResume,
		Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		Reason:  runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.Reason = runtimecontract.RuntimeExecutionIntentReasonRunNotResumable
	if err := result.Validate(); err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("mismatched admission err = %v", err)
	}
}

func TestWorkspaceExecutionIntentResultRequiresCompleteFreshActivationReleaseFact(t *testing.T) {
	result := WorkspaceExecutionIntentResult{
		WorkspaceID: "ws-1", RunID: "run-2", RequestID: "request-1", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		Reason:  runtimecontract.RuntimeExecutionIntentReasonFreshActivationReplacement,
	}
	if err := result.Validate(); err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("missing release fact err = %v", err)
	}
	result.FreshActivationRelease = &FreshActivationReleaseFact{
		WorkspaceID: "ws-1", RunID: "run-1", CausationID: "run-state/run-1/7/fresh_activation_required",
		RuntimeStateRevision: 7, RuntimeStateRef: "runtime-state/ws-1/run-1/7",
		RuntimeActivationDisposition:    runmodel.RuntimeActivationDispositionFreshActivationRequired,
		ActualRuntimeStateSchemaVersion: 1, SupportedRuntimeStateSchemaVersion: 2,
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("valid release fact: %v", err)
	}
}

func TestApplyRuntimeGovernanceWaitCommandValidate(t *testing.T) {
	cmd := ApplyRuntimeGovernanceWaitCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "runtime-governance-wait/auth-1/dispatch-1/ticket-1/", AuthoritySnapshotID: "auth-1",
		DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", BlockingTicketID: "ticket-1",
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid wait: %v", err)
	}
	cmd.BlockingApprovalID = "approval-1"
	if err := cmd.Validate(); err == nil {
		t.Fatal("expected exactly-one blocker")
	}
}

func TestApplyRuntimeContinuationEligibleCommandValidate(t *testing.T) {
	cmd := ApplyRuntimeContinuationEligibleCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "runtime-governance-continuation/auth-1/dispatch-1/res-1", AuthoritySnapshotID: "auth-1",
		DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", ResolutionID: "res-1", TicketID: "ticket-1",
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid continuation: %v", err)
	}
	cmd.TicketID = ""
	if err := cmd.Validate(); err == nil {
		t.Fatal("expected binding required")
	}
}

func TestApplyRuntimeTerminalDeliveryCommandValidate(t *testing.T) {
	cmd := ApplyRuntimeTerminalDeliveryCommand{
		RuntimeTerminalDeliveryIdentity: runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}},
		SourceOwner:                     "kernel/orchestrator", CausationID: "terminal-delivery/td-1",
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid terminal: %v", err)
	}
	admin := ApplyRuntimeTerminalDeliveryCommand{
		RuntimeTerminalDeliveryIdentity: runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-2", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCancelled, Code: "administrative_cancelled", Summary: "operator cancel"}},
		SourceOwner:                     "kernel/orchestrator", CausationID: "terminal-delivery/td-2",
	}
	if err := admin.Validate(); err != nil {
		t.Fatalf("valid administrative terminal: %v", err)
	}
	id := cmd.Identity()
	if err := id.Validate(); err != nil {
		t.Fatalf("identity: %v", err)
	}
}
