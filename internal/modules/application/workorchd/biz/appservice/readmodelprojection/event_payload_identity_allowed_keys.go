package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

var eventPayloadIdentityAllowedKeys = map[string]struct{}{
	events.PayloadKeyActiveRunID:  {},
	events.PayloadKeyDoDID:        {},
	events.PayloadKeyEvidenceRefs: {},
	events.PayloadKeyGoalID:       {},
	events.PayloadKeyItemID:       {},
	events.PayloadKeyRunID:        {},
	events.PayloadKeyStepID:       {},
	events.PayloadKeyToolCallID:   {},
	events.PayloadKeyTraceID:      {},
	events.PayloadKeyWorkspaceID:  {},
}
