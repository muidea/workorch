package events

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestActivateRunCommandValidate(t *testing.T) {
	cmd := ActivateRunCommand{WorkspaceID: "ws-1", RunID: "run-1"}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid activate: %v", err)
	}
	if err := (ActivateRunCommand{}).Validate(); err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty activate err = %v", err)
	}
}

func TestContinueRunCommandRequiresCausation(t *testing.T) {
	valid := ContinueRunCommand{WorkspaceID: "ws-1", RunID: "run-1", CausationID: "workspace-run/resume/request-1"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid continue: %v", err)
	}
	if err := (ContinueRunCommand{WorkspaceID: "ws-1", RunID: "run-1"}).Validate(); err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("continue without causation err = %v, want invalid argument", err)
	}
}

func TestWorkspaceExecutionIntentResultUsesTypedAdmission(t *testing.T) {
	result := WorkspaceExecutionIntentResult{
		WorkspaceID: "ws-1", RunID: "run-1", RequestID: "request-1", Kind: WorkspaceExecutionIntentResume,
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
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "terminal-delivery/td-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
		Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid terminal: %v", err)
	}
	admin := ApplyRuntimeTerminalDeliveryCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "terminal-delivery/td-2", TerminalDeliveryID: "td-2", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
		Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCancelled, Code: "administrative_cancelled", Summary: "operator cancel"},
	}
	if err := admin.Validate(); err != nil {
		t.Fatalf("valid administrative terminal: %v", err)
	}
	id := cmd.Identity()
	if err := id.Validate(); err != nil {
		t.Fatalf("identity: %v", err)
	}
}
