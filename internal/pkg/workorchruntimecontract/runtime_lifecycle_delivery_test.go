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
		Action:              RuntimeActionComplete,
		TerminalCode:        "completed",
		TerminalSummary:     "done",
	}
	if err := id.Validate(); err != nil {
		t.Fatalf("valid identity: %v", err)
	}
	id.Action = RuntimeAction("unknown")
	if err := id.Validate(); err == nil {
		t.Fatal("expected invalid action")
	}
}

func TestRuntimeDeliveryAcknowledgementValidate(t *testing.T) {
	ack := RuntimeDeliveryAcknowledgement{
		WorkspaceID:        "ws-1",
		RunID:              "run-1",
		TerminalDeliveryID: "td-1",
		AuthoritySnapshotID: "auth-1",
		CausationID:        "terminal-delivery/td-1",
		Action:             RuntimeActionComplete,
		TerminalCode:       "completed",
		Owner:              RuntimeDeliveryOwnerResult,
		AcknowledgedState:  RuntimeDeliveryAcknowledgedResultRecorded,
	}
	if err := ack.Validate(); err != nil {
		t.Fatalf("valid ack: %v", err)
	}
	ack.Owner = RuntimeDeliveryOwner("other")
	if err := ack.Validate(); err == nil {
		t.Fatal("expected invalid owner")
	}
}

func TestStableTerminalDeliveryID(t *testing.T) {
	got := StableTerminalDeliveryID(common.WorkspaceID("ws"), common.RunID("run"), "auth", "complete", "ok")
	want := "terminal-delivery:ws/run/auth/complete/ok"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
