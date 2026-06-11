package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/events"

var eventPayloadRuntimeAllowedKeys = map[string]struct{}{
	events.PayloadKeyAddedLines:        {},
	events.PayloadKeyAPISurface:        {},
	events.PayloadKeyCacheMarkersSent:  {},
	events.PayloadKeyCacheUsage:        {},
	events.PayloadKeyChunkCount:        {},
	events.PayloadKeyDeletedLines:      {},
	events.PayloadKeyDiffHash:          {},
	events.PayloadKeyDurationMs:        {},
	events.PayloadKeyElapsedMs:         {},
	events.PayloadKeyEstimated:         {},
	events.PayloadKeyOutputTokens:      {},
	events.PayloadKeyPolicyBlocked:     {},
	events.PayloadKeyPolicyMatched:     {},
	events.PayloadKeyPromptFingerprint: {},
	events.PayloadKeyReceivedBytes:     {},
	events.PayloadKeyRequestBytes:      {},
	events.PayloadKeyResponseBytes:     {},
	events.PayloadKeyStreaming:         {},
	events.PayloadKeyTotalTokens:       {},
}
