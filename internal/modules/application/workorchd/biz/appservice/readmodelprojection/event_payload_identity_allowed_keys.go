package readmodelprojection

import "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

var eventPayloadIdentityAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyActiveRunID:  {},
	recordpayload.PayloadKeyDoDID:        {},
	recordpayload.PayloadKeyEvidenceRefs: {},
	recordpayload.PayloadKeyGoalID:       {},
	recordpayload.PayloadKeyItemID:       {},
	recordpayload.PayloadKeyRunID:        {},
	recordpayload.PayloadKeyStepID:       {},
	recordpayload.PayloadKeyToolCallID:   {},
	recordpayload.PayloadKeyTraceID:      {},
	recordpayload.PayloadKeyWorkspaceID:  {},
}
