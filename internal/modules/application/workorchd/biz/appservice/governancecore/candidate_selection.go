package governancecore

import (
	"strings"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
)

func NormalizeCandidateSelectionResolutionPayloadForTicket(ticket *decisionmodel.Ticket, payload map[string]any) map[string]any {
	if ticket == nil || len(ticket.Context) == 0 || len(payload) == 0 {
		return payload
	}
	semantic := DecisionPayloadMap(payload["governanceSemantic"])
	if strings.TrimSpace(StringFromDecisionPayload(semantic, "intent")) != governancecommon.GovernanceIntentApproveExecution {
		return payload
	}
	candidates := DecisionPayloadMap(ticket.Context["candidateValues"])
	if len(candidates) == 0 {
		return payload
	}
	ret := CloneDecisionPayload(payload)
	if ret == nil {
		ret = map[string]any{}
	}
	selected := make([]string, 0, len(candidates))
	for field, raw := range candidates {
		field = strings.TrimSpace(field)
		if field == "" || strings.TrimSpace(StringFromDecisionPayload(ret, field)) != "" {
			continue
		}
		if candidate := PreferredContinuationCandidate(raw); candidate != "" {
			ret[field] = candidate
			selected = append(selected, field)
		}
	}
	if len(selected) == 0 {
		return payload
	}
	ret["selectedCandidateFields"] = selected
	ret["candidateSelectionMode"] = "semantic_resume"
	return ret
}
