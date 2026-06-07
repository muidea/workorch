package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

var eventPayloadProviderAllowedKeys = map[string]struct{}{
	events.PayloadKeyModel:      {},
	events.PayloadKeyProvider:   {},
	events.PayloadKeyProviderID: {},
}
