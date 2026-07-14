package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

func projectEventPayloadValue(value workorcheventpayload.Value) (workorcheventpayload.Value, bool) {
	switch value.Kind {
	case workorcheventpayload.ValueNull:
		return workorcheventpayload.Value{}, false
	case workorcheventpayload.ValueString:
		if len(value.String) > EventPayloadStringLimit {
			value.String = value.String[:EventPayloadStringLimit] + "..."
		}
		return value, true
	case workorcheventpayload.ValueBool, workorcheventpayload.ValueInt, workorcheventpayload.ValueUint, workorcheventpayload.ValueFloat:
		return value, true
	case workorcheventpayload.ValueArray:
		value.Array = projectEventPayloadArray(value.Array)
		return value, len(value.Array) > 0
	case workorcheventpayload.ValueObject:
		value.Object = projectEventPayloadObject(value.Object)
		return value, len(value.Object) > 0
	default:
		return workorcheventpayload.Value{}, false
	}
}
