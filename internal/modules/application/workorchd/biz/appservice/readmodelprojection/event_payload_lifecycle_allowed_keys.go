package readmodelprojection

import "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

var eventPayloadLifecycleAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyCompletedAt: {},
	recordpayload.PayloadKeyStartedAt:   {},
	recordpayload.PayloadKeyState:       {},
	recordpayload.PayloadKeyStatus:      {},
}
