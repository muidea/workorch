package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

var eventPayloadLLMAllowedKeys = map[string]struct{}{
	events.PayloadKeyLLMModel:      {},
	events.PayloadKeyLLMProviderID: {},
	events.PayloadKeyUsageSource:   {},
}
