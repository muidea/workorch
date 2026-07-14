package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

func projectEventPayloadArray(values []workorcheventpayload.Value) []workorcheventpayload.Value {
	limit := min(len(values), EventPayloadListLimit)
	ret := make([]workorcheventpayload.Value, 0, limit)
	for _, item := range values[:limit] {
		if projected, ok := projectEventPayloadValue(item); ok {
			ret = append(ret, projected)
		}
	}
	return ret
}

func projectEventPayloadObject(fields []workorcheventpayload.Field) []workorcheventpayload.Field {
	ret := make([]workorcheventpayload.Field, 0, len(fields))
	for _, field := range fields {
		if projected, ok := projectEventPayloadValue(field.Value); ok {
			ret = append(ret, workorcheventpayload.Field{Name: field.Name, Value: projected})
		}
	}
	return ret
}
