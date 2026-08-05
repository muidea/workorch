package workorchport

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// CompactEventRecordPayloadSummary derives one display line from the original
// immutable payload. It deliberately does not rebuild a parallel typed payload
// hierarchy or infer any owner state.
func CompactEventRecordPayloadSummary(topic string, payload workorcheventpayload.Payload) string {
	topic = strings.ToLower(strings.TrimSpace(topic))
	switch {
	case strings.Contains(topic, "/goal/"):
		return firstPayloadString(payload, "text", "summary", "reason", "state")
	case strings.Contains(topic, "/workspace/"):
		return firstPayloadString(payload, "title", "summary", "reason", "state")
	case strings.Contains(topic, "/run/"):
		return firstPayloadString(payload, "failureReason", "summary", "reason", "state")
	case strings.Contains(topic, "/decision/"), strings.Contains(topic, "/approval/"):
		return firstPayloadString(payload, "reason", "summary", "action", "state")
	case strings.Contains(topic, "/result/"), strings.Contains(topic, "/artifact/"):
		return firstPayloadString(payload, "summary", "title", "name", "state")
	case strings.Contains(topic, "/patch/"):
		return firstPayloadString(payload, "summary", "error", "state", "targetRef")
	case strings.Contains(topic, "/memory/"):
		return firstPayloadString(payload, "summary", "reason", "memoryType", "status")
	case strings.Contains(topic, "/governance/"):
		return firstPayloadString(payload, "reason", "summary", "status", "action")
	default:
		return firstPayloadString(payload, "summary", "title", "text", "reason", "failureReason", "state", "kind", "action", "error", "targetRef")
	}
}

func firstPayloadString(payload workorcheventpayload.Payload, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload.String(key); ok {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}
