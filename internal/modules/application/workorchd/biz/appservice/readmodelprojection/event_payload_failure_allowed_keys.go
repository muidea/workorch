package readmodelprojection

import "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

var eventPayloadFailureAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyError:         {},
	recordpayload.PayloadKeyFailureClass:  {},
	recordpayload.PayloadKeyFailureReason: {},
}
