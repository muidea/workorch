package events

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeGovernanceWaitRecordedDataValidate(t *testing.T) {
	data := RuntimeGovernanceWaitRecordedData{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "runtime-governance-wait/auth-1/dispatch-1/ticket-1/", AuthoritySnapshotID: "auth-1",
		DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", BlockingTicketID: "ticket-1",
	}
	if err := data.Validate(); err != nil {
		t.Fatalf("valid wait fact: %v", err)
	}
	data.BlockingApprovalID = "approval-1"
	if err := data.Validate(); err == nil {
		t.Fatal("expected exactly-one blocker")
	}
}

func TestRuntimeContinuationEligibleDataValidate(t *testing.T) {
	data := RuntimeContinuationEligibleData{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "runtime-governance-continuation/auth-1/dispatch-1/res-1", AuthoritySnapshotID: "auth-1",
		DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", ResolutionID: "res-1", TicketID: "ticket-1",
	}
	if err := data.Validate(); err != nil {
		t.Fatalf("valid continuation fact: %v", err)
	}
}

func TestRuntimeTerminalDeliveryRequestDataValidate(t *testing.T) {
	data := RuntimeTerminalDeliveryRequestData{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1",
		Action: runtimecontract.RuntimeActionComplete, TerminalCode: "completed", TerminalSummary: "done",
	}
	if err := data.Validate(); err != nil {
		t.Fatalf("valid terminal request: %v", err)
	}
	if err := (RuntimeTerminalDeliveryRequestData{}).Validate(); err == nil {
		t.Fatal("expected empty terminal request to fail")
	}
}

func TestRecordRuntimeDeliveryAcknowledgementCommandValidate(t *testing.T) {
	cmd := RecordRuntimeDeliveryAcknowledgementCommand{
		Acknowledgement: runtimecontract.RuntimeDeliveryAcknowledgement{
			WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1",
			AuthoritySnapshotID: "auth-1", CausationID: "terminal-delivery/td-1",
			Action: runtimecontract.RuntimeActionComplete, TerminalCode: "completed",
			Owner:             runtimecontract.RuntimeDeliveryOwnerRun,
			AcknowledgedState: runtimecontract.RuntimeDeliveryAcknowledgedRunTerminal,
		},
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid ack command: %v", err)
	}
}
