package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

var eventPayloadArtifactAllowedKeys = map[string]struct{}{
	events.PayloadKeyArtifactRefs: {},
	events.PayloadKeyArtifactType: {},
	events.PayloadKeyContentHash:  {},
	events.PayloadKeyContentType:  {},
	events.PayloadKeyPatchID:      {},
	events.PayloadKeyPatchType:    {},
	events.PayloadKeyResultID:     {},
	events.PayloadKeyTargetRef:    {},
	events.PayloadKeyURI:          {},
}
