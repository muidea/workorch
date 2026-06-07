package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

var eventPayloadGovernanceAllowedKeys = map[string]struct{}{
	events.PayloadKeyAction:             {},
	events.PayloadKeyApprovalID:         {},
	events.PayloadKeyBlockingApprovalID: {},
	events.PayloadKeyBlockingTicketID:   {},
	events.PayloadKeyConfidenceScore:    {},
	events.PayloadKeyLLMDecisionPreview: {},
	events.PayloadKeyPolicy:             {},
	events.PayloadKeyReason:             {},
	events.PayloadKeyRisk:               {},
}
