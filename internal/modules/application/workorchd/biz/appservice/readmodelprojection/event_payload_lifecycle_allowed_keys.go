package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

var eventPayloadLifecycleAllowedKeys = map[string]struct{}{
	events.PayloadKeyCompletedAt: {},
	events.PayloadKeyStartedAt:   {},
	events.PayloadKeyState:       {},
	events.PayloadKeyStatus:      {},
}
