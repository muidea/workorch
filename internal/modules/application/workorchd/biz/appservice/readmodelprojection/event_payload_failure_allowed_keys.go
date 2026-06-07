package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

var eventPayloadFailureAllowedKeys = map[string]struct{}{
	events.PayloadKeyError:         {},
	events.PayloadKeyFailureClass:  {},
	events.PayloadKeyFailureReason: {},
}
