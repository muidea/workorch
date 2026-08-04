package readmodelprojection

import "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

var eventPayloadGovernanceAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyAction:             {},
	recordpayload.PayloadKeyApprovalID:         {},
	recordpayload.PayloadKeyBlockingApprovalID: {},
	recordpayload.PayloadKeyBlockingTicketID:   {},
	recordpayload.PayloadKeyConfidenceScore:    {},
	recordpayload.PayloadKeyLLMDecisionPreview: {},
	recordpayload.PayloadKeyPolicy:             {},
	recordpayload.PayloadKeyReason:             {},
	recordpayload.PayloadKeyRisk:               {},
}
