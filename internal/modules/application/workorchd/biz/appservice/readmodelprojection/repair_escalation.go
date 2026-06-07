package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func repairEscalationProjection(snapshot readmodelcore.RunSnapshot, recovery readmodelcore.RecoveryPlan) (ticketID string, reason string, context map[string]any, ok bool) {
	if recovery.BlockingTicketID == "" {
		return "", "", nil, false
	}
	for _, item := range snapshot.Decisions {
		if item.ID != recovery.BlockingTicketID {
			continue
		}
		context := readModelMapValue(item.Context)
		if readModelIntValue(context["attempts"]) > 0 ||
			strings.TrimSpace(readModelStringValue(context["failureClass"])) != "" ||
			strings.TrimSpace(readModelStringValue(context["failureSignature"])) != "" {
			return item.ID, firstNonEmptyTimelineString(item.Reason, item.Question), context, true
		}
		return "", "", nil, false
	}
	return "", "", nil, false
}
