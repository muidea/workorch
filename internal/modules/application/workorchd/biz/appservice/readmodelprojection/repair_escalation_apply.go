package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func applyRepairEscalationProjection(ret *readmodelcore.RepairLoop, ticketID string, reason string, escalationContext map[string]any) {
	if ret == nil {
		return
	}
	ret.Escalated = true
	ret.EscalationTicketID = ticketID
	ret.EscalationReason = reason
	ret.EvidenceRefs = append(ret.EvidenceRefs, ticketID)
	ret.State = "escalated_decision"
	if items := readModelMapSliceValue(escalationContext["missingDoDItems"]); len(items) > 0 {
		ret.MissingDoDItems = items
	}
	if attempts := readModelMapSliceValue(escalationContext["recentRepairAttempts"]); len(attempts) > 0 {
		ret.RecentAttempts = attempts
	}
	if suggestions := readModelStringSliceValue(escalationContext["recommendedNextActions"]); len(suggestions) > 0 {
		ret.RecommendedNextSteps = suggestions
	}
}
