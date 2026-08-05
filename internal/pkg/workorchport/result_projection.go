package workorchport

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

func projectionPayload(input map[string]any) workorcheventpayload.Payload {
	payload, _ := workorcheventpayload.FromMap(input)
	return payload
}

func projectionPayloads(input []map[string]any) []workorcheventpayload.Payload {
	payloads, _ := workorcheventpayload.FromMapSlice(input)
	return payloads
}

func cloneProjectionStrings(input []string) []string {
	return append([]string(nil), input...)
}
