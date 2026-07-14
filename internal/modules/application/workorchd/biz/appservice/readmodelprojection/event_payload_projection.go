package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

const (
	EventPayloadStringLimit = 512
	EventPayloadListLimit   = 16
)

func projectEventPayload(payload workorcheventpayload.Payload) workorcheventpayload.Payload {
	if len(payload.Fields) == 0 {
		return workorcheventpayload.Payload{}
	}
	fields := make([]workorcheventpayload.Field, 0, len(payload.Fields))
	for _, field := range payload.Fields {
		if !isEventPayloadAllowedKey(field.Name) {
			continue
		}
		if projected, ok := projectEventPayloadValue(field.Value); ok {
			fields = append(fields, workorcheventpayload.Field{Name: field.Name, Value: projected})
		}
	}
	return workorcheventpayload.New(fields...)
}
