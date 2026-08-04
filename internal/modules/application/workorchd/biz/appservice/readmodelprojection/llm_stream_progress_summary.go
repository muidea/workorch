package readmodelprojection

import (
	"strconv"

	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func streamProgressSummaryFromPayload(payload map[string]any) string {
	chunkCount := readModelIntValue(payload[recordpayload.PayloadKeyChunkCount])
	receivedBytes := readModelIntValue(payload[recordpayload.PayloadKeyReceivedBytes])
	switch {
	case chunkCount > 0 && receivedBytes > 0:
		return "received " + strconv.Itoa(chunkCount) + " chunks (" + strconv.Itoa(receivedBytes) + " bytes)"
	case chunkCount > 0:
		return "received " + strconv.Itoa(chunkCount) + " chunks"
	case receivedBytes > 0:
		return "received " + strconv.Itoa(receivedBytes) + " bytes"
	default:
		return ""
	}
}

func streamProgressSummaryFromEventPayload(payload workorcheventpayload.Payload) string {
	chunkCount := eventPayloadInt(payload, recordpayload.PayloadKeyChunkCount)
	receivedBytes := eventPayloadInt(payload, recordpayload.PayloadKeyReceivedBytes)
	switch {
	case chunkCount > 0 && receivedBytes > 0:
		return "received " + strconv.Itoa(chunkCount) + " chunks (" + strconv.Itoa(receivedBytes) + " bytes)"
	case chunkCount > 0:
		return "received " + strconv.Itoa(chunkCount) + " chunks"
	case receivedBytes > 0:
		return "received " + strconv.Itoa(receivedBytes) + " bytes"
	default:
		return ""
	}
}
