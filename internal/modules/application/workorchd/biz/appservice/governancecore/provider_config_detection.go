package governancecore

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

func ContainsInlineLLMProviderConfig(payload workorcheventpayload.Payload) bool {
	if len(payload.Fields) == 0 {
		return false
	}
	hits := map[string]bool{}
	VisitProviderConfigPayload(payload, hits)
	if hits["apiKey"] || hits["secret"] {
		return true
	}
	configHits := 0
	for _, key := range []string{"provider", "baseURL", "model"} {
		if hits[key] {
			configHits++
		}
	}
	return configHits >= 2
}

func VisitProviderConfigPayload(payload workorcheventpayload.Payload, hits map[string]bool) {
	for _, field := range payload.Fields {
		RecordProviderConfigKey(field.Name, hits)
		VisitProviderConfigValue(field.Value, hits)
	}
}

func VisitProviderConfigValue(value workorcheventpayload.Value, hits map[string]bool) {
	switch value.Kind {
	case workorcheventpayload.ValueObject:
		VisitProviderConfigPayload(workorcheventpayload.Payload{Fields: value.Object}, hits)
	case workorcheventpayload.ValueArray:
		for _, nested := range value.Array {
			VisitProviderConfigValue(nested, hits)
		}
	case workorcheventpayload.ValueString:
		RecordProviderConfigText(value.String, hits)
	}
}
