package readmodelprojection

import "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

var eventPayloadRuntimeAllowedKeys = map[string]struct{}{
	recordpayload.PayloadKeyAddedLines:        {},
	recordpayload.PayloadKeyAPISurface:        {},
	recordpayload.PayloadKeyCacheMarkersSent:  {},
	recordpayload.PayloadKeyCacheUsage:        {},
	recordpayload.PayloadKeyChunkCount:        {},
	recordpayload.PayloadKeyDeletedLines:      {},
	recordpayload.PayloadKeyDiffHash:          {},
	recordpayload.PayloadKeyDurationMs:        {},
	recordpayload.PayloadKeyElapsedMs:         {},
	recordpayload.PayloadKeyEstimated:         {},
	recordpayload.PayloadKeyOutputTokens:      {},
	recordpayload.PayloadKeyPolicyBlocked:     {},
	recordpayload.PayloadKeyPolicyMatched:     {},
	recordpayload.PayloadKeyPromptFingerprint: {},
	recordpayload.PayloadKeyReceivedBytes:     {},
	recordpayload.PayloadKeyRequestBytes:      {},
	recordpayload.PayloadKeyResponseBytes:     {},
	recordpayload.PayloadKeyStreaming:         {},
	recordpayload.PayloadKeyTotalTokens:       {},
}
