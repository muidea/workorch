package eventstream

import (
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestContractChangeRegistrationRejectsOrdinaryNotifications(t *testing.T) {
	for _, runID := range []common.RunID{"", "run"} {
		data := decisionevents.TicketChangedData{NotificationID: "notification", TicketID: "ticket", WorkspaceID: "workspace", RunID: runID, DecisionKind: decisionmodel.ContractChangeDecisionKind, State: common.DecisionTicketStateOpen}
		if validTicketCreatedData(data) {
			t.Fatal("登记被接受为普通created通知")
		}
		data.State = common.DecisionTicketStateResolved
		if validTicketResolvedData(decisionevents.TicketResolvedData{NotificationID: "resolved", Ticket: data, ResolutionID: "resolution", ResolutionType: "replan"}) {
			t.Fatal("登记被接受为普通resolved通知")
		}
	}
}
