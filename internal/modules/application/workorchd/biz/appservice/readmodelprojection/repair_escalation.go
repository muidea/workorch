package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func repairEscalationProjection(snapshot readmodelcore.RunSnapshot, recovery readmodelcore.RecoveryPlan) (ticketID string, reason string, context workorcheventpayload.Payload, ok bool) {
	if recovery.BlockingTicketID == "" {
		return "", "", workorcheventpayload.Payload{}, false
	}
	for _, item := range snapshot.Decisions {
		if item.ID != recovery.BlockingTicketID {
			continue
		}
		attempts, _ := item.Context.Int64("attempts")
		failureClass, _ := item.Context.String("failureClass")
		failureSignature, _ := item.Context.String("failureSignature")
		if attempts > 0 || strings.TrimSpace(failureClass) != "" || strings.TrimSpace(failureSignature) != "" {
			return item.ID, firstNonEmptyTimelineString(item.Reason, item.Question), item.Context.Clone(), true
		}
		return "", "", workorcheventpayload.Payload{}, false
	}
	return "", "", workorcheventpayload.Payload{}, false
}
