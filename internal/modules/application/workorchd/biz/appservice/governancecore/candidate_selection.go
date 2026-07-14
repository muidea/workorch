package governancecore

import (
	"strings"

	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func NormalizeCandidateSelectionResolutionPayloadForTicket(ticket *decisionmodel.Ticket, payload workorcheventpayload.Payload) workorcheventpayload.Payload {
	if ticket == nil || len(ticket.Context.Fields) == 0 || len(payload.Fields) == 0 {
		return payload
	}
	semantic := DecisionPayloadObject(payload, "governanceSemantic")
	if strings.TrimSpace(StringFromDecisionPayload(semantic, "intent")) != governancecommon.GovernanceIntentApproveExecution {
		return payload
	}
	candidates := DecisionPayloadObject(ticket.Context, "candidateValues")
	if len(candidates.Fields) == 0 {
		return payload
	}
	ret := CloneDecisionPayload(payload)
	selected := make([]string, 0, len(candidates.Fields))
	selectedFields := make([]workorcheventpayload.Field, 0, len(candidates.Fields))
	for _, candidateField := range candidates.Fields {
		field := strings.TrimSpace(candidateField.Name)
		if field == "" || strings.TrimSpace(StringFromDecisionPayload(ret, field)) != "" {
			continue
		}
		if candidate := PreferredContinuationCandidate(candidateField.Value); candidate != "" {
			selectedFields = append(selectedFields, workorcheventpayload.StringField(field, candidate))
			selected = append(selected, field)
		}
	}
	if len(selected) == 0 {
		return payload
	}
	return ret.Merge(workorcheventpayload.New(
		append(selectedFields,
			workorcheventpayload.StringArrayField("selectedCandidateFields", selected),
			workorcheventpayload.StringField("candidateSelectionMode", "semantic_resume"),
		)...,
	))
}
