package events

import (
	"testing"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
)

func TestGovernanceCommandResultCloneDetachesOwnerState(t *testing.T) {
	item := &governancemodel.AutoGovernanceTrace{EvidenceRefs: []string{"owner"}}
	cloned := (GovernanceCommandResult{Traces: []*governancemodel.AutoGovernanceTrace{item}}).Clone()
	item.EvidenceRefs[0] = "mutated"
	if cloned.Traces[0].EvidenceRefs[0] != "owner" {
		t.Fatal("cloned governance trace shares evidence refs")
	}
}
