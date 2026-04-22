package presenter

import (
	"testing"

	"github.com/muidea/workorch/application/workorchd/observed"
)

func TestMapObservedPreservesEnvelopeShape(t *testing.T) {
	items := []observed.SnapshotItem{
		{
			ID:           "/run/snapshot:run-1",
			Version:      "phase1-demo.v1",
			Topic:        "/run/snapshot",
			OccurredAt:   "2026-04-22T00:00:00Z",
			SourceModule: "application/workorchd",
			SubjectType:  "run",
			SubjectID:    "run-1",
			WorkspaceID:  "ws-1",
			RunID:        "run-1",
			Payload: observed.RunPayload{
				State:              "completed",
				BlockingTicketID:   "",
				BlockingApprovalID: "",
			},
		},
	}

	mapped := MapObserved(items)
	if len(mapped) != 1 {
		t.Fatalf("len(mapped) = %d, want 1", len(mapped))
	}
	if mapped[0].Topic != "/run/snapshot" {
		t.Fatalf("topic = %s, want /run/snapshot", mapped[0].Topic)
	}
	if mapped[0].Payload["state"] != "completed" {
		t.Fatalf("payload state = %v, want completed", mapped[0].Payload["state"])
	}
}
