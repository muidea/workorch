package readmodelprojection

import "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

var eventPayloadProviderAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyModel:      {},
	recordpayload.PayloadKeyProvider:   {},
	recordpayload.PayloadKeyProviderID: {},
}
