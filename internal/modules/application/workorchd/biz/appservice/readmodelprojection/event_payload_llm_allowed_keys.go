package readmodelprojection

import "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

var eventPayloadLLMAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyLLMModel:      {},
	recordpayload.PayloadKeyLLMProviderID: {},
	recordpayload.PayloadKeyUsageSource:   {},
}
