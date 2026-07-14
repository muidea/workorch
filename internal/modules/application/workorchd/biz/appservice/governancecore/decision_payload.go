package governancecore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func DecisionPayloadObject(payload workorcheventpayload.Payload, key string) workorcheventpayload.Payload {
	value, _ := payload.Object(key)
	return value
}

func StringFromDecisionPayload(payload workorcheventpayload.Payload, key string) string {
	value, _ := payload.String(key)
	return strings.TrimSpace(value)
}

func CloneDecisionPayload(input workorcheventpayload.Payload) workorcheventpayload.Payload {
	return input.Clone()
}
