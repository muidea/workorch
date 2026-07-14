package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

func eventPayloadString(payload workorcheventpayload.Payload, key string) string {
	value, _ := payload.String(key)
	return value
}

func eventPayloadInt(payload workorcheventpayload.Payload, key string) int {
	value, _ := payload.Int64(key)
	return int(value)
}

func eventPayloadBool(payload workorcheventpayload.Payload, key string) bool {
	value, _ := payload.Bool(key)
	return value
}

func eventPayloadMap(payload workorcheventpayload.Payload, key string) map[string]any {
	value, ok := payload.Object(key)
	if !ok {
		return nil
	}
	return value.Map()
}
