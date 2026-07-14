package workorchpayloadmapper

import (
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchport"
)

func FromEventPayload(payload workorcheventpayload.Payload) workorchport.EventRecordPayloadInfo {
	return workorchport.EventRecordPayloadInfo{Fields: eventPayloadFields(payload.Fields)}
}

func eventPayloadFields(fields []workorcheventpayload.Field) []workorchport.EventPayloadFieldInfo {
	if len(fields) == 0 {
		return nil
	}
	ret := make([]workorchport.EventPayloadFieldInfo, 0, len(fields))
	for _, field := range fields {
		ret = append(ret, workorchport.EventPayloadFieldInfo{Name: field.Name, Value: eventPayloadValue(field.Value)})
	}
	return ret
}

func eventPayloadValue(value workorcheventpayload.Value) workorchport.EventPayloadValueInfo {
	ret := workorchport.EventPayloadValueInfo{
		String: value.String, Bool: value.Bool, Int: value.Int, Uint: value.Uint, Float: value.Float,
		Object: eventPayloadFields(value.Object),
	}
	if len(value.Array) > 0 {
		ret.Array = make([]workorchport.EventPayloadValueInfo, 0, len(value.Array))
		for _, item := range value.Array {
			ret.Array = append(ret.Array, eventPayloadValue(item))
		}
	}
	switch value.Kind {
	case workorcheventpayload.ValueNull:
		ret.Kind = workorchport.EventPayloadValueNull
	case workorcheventpayload.ValueString:
		ret.Kind = workorchport.EventPayloadValueString
	case workorcheventpayload.ValueBool:
		ret.Kind = workorchport.EventPayloadValueBool
	case workorcheventpayload.ValueInt:
		ret.Kind = workorchport.EventPayloadValueInt
	case workorcheventpayload.ValueUint:
		ret.Kind = workorchport.EventPayloadValueUint
	case workorcheventpayload.ValueFloat:
		ret.Kind = workorchport.EventPayloadValueFloat
	case workorcheventpayload.ValueObject:
		ret.Kind = workorchport.EventPayloadValueObject
	case workorcheventpayload.ValueArray:
		ret.Kind = workorchport.EventPayloadValueArray
	}
	return ret
}
