package events

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeTerminalDeliveryRequestDataValidate(t *testing.T) {
	data := RuntimeTerminalDeliveryRequestData{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
		Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
	}
	if err := data.Validate(); err != nil {
		t.Fatalf("valid terminal request: %v", err)
	}
	if err := (RuntimeTerminalDeliveryRequestData{}).Validate(); err == nil {
		t.Fatal("expected empty terminal request to fail")
	}
}

func TestTerminalDeliveryAcknowledgementIsNotAPublicEventHubContract(t *testing.T) {
	// SM-4 acknowledgements are private Orchestrator internal events produced
	// only from the Result -> Workspace -> Run synchronous command responses.
	// Keep a literal guard here so a future package cannot restore the former
	// public ack endpoint without deliberately changing this boundary.
	const retiredTopic = "/orchestrator/runtime-lifecycle/record-delivery-acknowledgement"
	for _, contract := range Contracts() {
		if contract.Topic == retiredTopic {
			t.Fatalf("retired delivery acknowledgement topic remains registered: %+v", contract)
		}
	}
	for _, contract := range MessageContracts() {
		if contract.Topic == retiredTopic {
			t.Fatalf("retired delivery acknowledgement message remains registered: %+v", contract)
		}
	}
}
