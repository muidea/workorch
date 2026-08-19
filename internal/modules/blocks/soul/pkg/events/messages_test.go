package events

import (
	"testing"

	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
)

func TestSoulConcreteResultsCloneDetachesOwnerState(t *testing.T) {
	item := &soulmodel.Soul{BusinessDomains: []string{"owner"}}
	created := (CreateSoulResult{Soul: item}).Clone()
	got := (GetSoulResult{Soul: item}).Clone()
	listed := (ListSoulsResult{Souls: []*soulmodel.Soul{item}, NextCursor: "soul-1"}).Clone()
	updated := (UpdateSoulResult{Soul: item}).Clone()
	item.BusinessDomains[0] = "mutated"
	if created.Soul.BusinessDomains[0] != "owner" || got.Soul.BusinessDomains[0] != "owner" || listed.Souls[0].BusinessDomains[0] != "owner" || updated.Soul.BusinessDomains[0] != "owner" || listed.NextCursor != "soul-1" {
		t.Fatal("concrete Soul result shares owner state")
	}
}
