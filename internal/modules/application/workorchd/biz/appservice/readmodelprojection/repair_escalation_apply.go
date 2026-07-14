package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func applyRepairEscalationProjection(ret *readmodelcore.RepairLoop, ticketID string, reason string, escalationContext workorcheventpayload.Payload) {
	if ret == nil {
		return
	}
	ret.Escalated = true
	ret.EscalationTicketID = ticketID
	ret.EscalationReason = reason
	ret.EvidenceRefs = append(ret.EvidenceRefs, ticketID)
	ret.State = "escalated_decision"
	if items := payloadObjectMapSlice(escalationContext, "missingDoDItems"); len(items) > 0 {
		ret.MissingDoDItems = items
	}
	if attempts := payloadObjectMapSlice(escalationContext, "recentRepairAttempts"); len(attempts) > 0 {
		ret.RecentAttempts = attempts
	}
	if suggestions, ok := escalationContext.StringSlice("recommendedNextActions"); ok && len(suggestions) > 0 {
		ret.RecommendedNextSteps = suggestions
	}
}

func payloadObjectMapSlice(payload workorcheventpayload.Payload, name string) []map[string]any {
	value, ok := payload.Field(name)
	if !ok || value.Kind != workorcheventpayload.ValueArray {
		return nil
	}
	ret := make([]map[string]any, 0, len(value.Array))
	for _, item := range value.Array {
		if item.Kind != workorcheventpayload.ValueObject {
			continue
		}
		mapped := workorcheventpayload.New(item.Object...).Map()
		if len(mapped) > 0 {
			ret = append(ret, mapped)
		}
	}
	return ret
}
