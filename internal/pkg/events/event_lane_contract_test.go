package events

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

func TestEventHubLaneKeyUsesOnlySuppliedOwnerContracts(t *testing.T) {
	contracts := eventcontract.Build("workspace", eventcontract.CommandClassWrite, eventcontract.LanePolicyState, "/workspace/command/update")
	key := EventHubLaneKeyFor(contracts, "/workspace/command/update", EventHubLaneScope{WorkspaceID: "ws-1"})
	if key != "state/workspace/ws-1" {
		t.Fatalf("lane key = %q", key)
	}
}

func TestEventHubLaneKeyRejectsMissingOwnerContract(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("missing owner contract did not fail fast")
		}
	}()
	_ = EventHubLaneKeyFor(nil, "/workspace/command/update", EventHubLaneScope{WorkspaceID: "ws-1"})
}
