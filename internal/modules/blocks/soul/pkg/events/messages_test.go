package events

import (
	"testing"

	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
)

func TestSoulCommandResultCloneDetachesOwnerState(t *testing.T) {
	item := &soulmodel.Soul{BusinessDomains: []string{"owner"}}
	cloned := (SoulCommandResult{Soul: item}).Clone()
	item.BusinessDomains[0] = "mutated"
	if cloned.Soul.BusinessDomains[0] != "owner" {
		t.Fatal("cloned soul shares business domains")
	}
}
