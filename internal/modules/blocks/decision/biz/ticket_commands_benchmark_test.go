package decision

import (
	"fmt"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

var decisionTicketPageBenchmarkSink []*decisionmodel.Ticket

func BenchmarkDecisionTicketInventoryPage100K(b *testing.B) {
	items := make([]*decisionmodel.Ticket, 100_000)
	for idx := range items {
		items[idx] = &decisionmodel.Ticket{
			ID:          common.TicketID(fmt.Sprintf("ticket-%06d", idx)),
			WorkspaceID: common.WorkspaceID(fmt.Sprintf("workspace-%03d", idx%100)),
			RunID:       common.RunID(fmt.Sprintf("run-%06d", idx)),
			State:       common.DecisionTicketStateOpen,
		}
	}
	cursor := decisionevents.TicketCursor(items[len(items)/2])
	b.ReportAllocs()
	b.ResetTimer()
	for idx := 0; idx < b.N; idx++ {
		page, _, err := paginateDecisionTickets(items, cursor, decisionevents.MaxListLimit)
		if err != nil {
			b.Fatal(err)
		}
		decisionTicketPageBenchmarkSink = page
	}
}
