package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeTerminalDeliveryIdentityValidate(t *testing.T) {
	id := RuntimeTerminalDeliveryIdentity{
		TerminalDeliveryID:  "terminal-delivery:ws/run/auth/complete/ok",
		WorkspaceID:         "ws-1",
		RunID:               "run-1",
		AuthoritySnapshotID: "auth-1",
		DefinitionRevision:  "def-1",
		EvidenceRevision:    "evidence-1",
		DispatchID:          "dispatch-1",
		Outcome:             TerminalOutcome{Kind: TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
	}
	if err := id.Validate(); err != nil {
		t.Fatalf("valid identity: %v", err)
	}
	id.Outcome.Kind = TerminalOutcomeKind("unknown")
	if err := id.Validate(); err == nil {
		t.Fatal("expected invalid outcome kind")
	}
}

func TestRuntimeTerminalDeliveryFactNormalizesAndValidatesEnvelope(t *testing.T) {
	fact := RuntimeTerminalDeliveryFact{
		RuntimeTerminalDeliveryIdentity: RuntimeTerminalDeliveryIdentity{
			TerminalDeliveryID: " td-1 ", WorkspaceID: "ws-1", RunID: "run-1",
			AuthoritySnapshotID: " auth-1 ", DefinitionRevision: " def-1 ",
			EvidenceRevision: " evidence-1 ", DispatchID: " dispatch-1 ",
			Outcome: TerminalOutcome{Kind: TerminalOutcomeKindCompleted, Code: " completed ", Summary: " done "},
		},
		SourceOwner: " kernel/orchestrator ", CausationID: " terminal-delivery/td-1 ",
	}.Normalized()
	if err := fact.Validate(); err != nil {
		t.Fatalf("valid fact: %v", err)
	}
	if fact.SourceOwner != "kernel/orchestrator" || fact.CausationID != "terminal-delivery/td-1" || fact.Identity().TerminalDeliveryID != "td-1" {
		t.Fatalf("normalized fact = %+v", fact)
	}
	fact.CausationID = ""
	if err := fact.Validate(); err == nil {
		t.Fatal("expected missing causation to fail")
	}
}

func TestRuntimeDeliveryAcknowledgementValidate(t *testing.T) {
	ack := RuntimeDeliveryAcknowledgement{
		WorkspaceID:         "ws-1",
		RunID:               "run-1",
		TerminalDeliveryID:  "td-1",
		AuthoritySnapshotID: "auth-1",
		CausationID:         "terminal-delivery/td-1",
		Outcome:             TerminalOutcome{Kind: TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
		Owner:               RuntimeDeliveryOwnerResult,
		AcknowledgedState:   RuntimeDeliveryAcknowledgedResultRecorded,
	}
	if err := ack.Validate(); err != nil {
		t.Fatalf("valid ack: %v", err)
	}
	ack.Owner = RuntimeDeliveryOwner("other")
	if err := ack.Validate(); err == nil {
		t.Fatal("expected invalid owner")
	}
}

func TestTerminalOutcomeValidatesAllSealedKinds(t *testing.T) {
	for _, kind := range []TerminalOutcomeKind{
		TerminalOutcomeKindCompleted,
		TerminalOutcomeKindFailed,
		TerminalOutcomeKindCancelled,
		TerminalOutcomeKindSuperseded,
	} {
		t.Run(string(kind), func(t *testing.T) {
			outcome := TerminalOutcome{Kind: kind, Code: "terminal", Summary: "sealed"}
			if err := outcome.Validate(); err != nil {
				t.Fatalf("outcome %+v: %v", outcome, err)
			}
			identity := RuntimeTerminalDeliveryIdentity{
				TerminalDeliveryID: "terminal-delivery:ws/run/auth/" + string(kind) + "/terminal",
				WorkspaceID:        "ws", RunID: "run", AuthoritySnapshotID: "auth",
				DefinitionRevision: "definition", EvidenceRevision: "evidence", DispatchID: "dispatch",
				Outcome: outcome,
			}
			if err := identity.Validate(); err != nil {
				t.Fatalf("identity %+v: %v", identity, err)
			}
		})
	}
}

func TestStableTerminalDeliveryID(t *testing.T) {
	got := StableTerminalDeliveryID(common.WorkspaceID("ws"), common.RunID("run"), "auth", TerminalOutcome{Kind: TerminalOutcomeKindCompleted, Code: "ok", Summary: "done"})
	want := "terminal-delivery:ws/run/auth/completed/ok"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
