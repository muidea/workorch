package readmodelprojection

import "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

var eventPayloadArtifactAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyArtifactRefs: {},
	recordpayload.PayloadKeyArtifactType: {},
	recordpayload.PayloadKeyContentHash:  {},
	recordpayload.PayloadKeyContentType:  {},
	recordpayload.PayloadKeyPatchID:      {},
	recordpayload.PayloadKeyPatchType:    {},
	recordpayload.PayloadKeyResultID:     {},
	recordpayload.PayloadKeyTargetRef:    {},
	recordpayload.PayloadKeyURI:          {},
}
