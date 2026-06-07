package readmodelprojection

import (
	"strconv"

	"github.com/muidea/workorch/internal/pkg/events"
)

func streamProgressSummaryFromPayload(payload map[string]any) string {
	chunkCount := readModelIntValue(payload[events.PayloadKeyChunkCount])
	receivedBytes := readModelIntValue(payload[events.PayloadKeyReceivedBytes])
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
