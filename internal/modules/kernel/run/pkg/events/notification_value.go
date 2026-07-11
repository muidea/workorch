package events

import (
	"fmt"
	"sort"

	"github.com/muidea/workorch/internal/pkg/common"
)

type NotificationValueKind string

const (
	NotificationValueNull   NotificationValueKind = "null"
	NotificationValueString NotificationValueKind = "string"
	NotificationValueBool   NotificationValueKind = "bool"
	NotificationValueInt    NotificationValueKind = "int"
	NotificationValueUint   NotificationValueKind = "uint"
	NotificationValueFloat  NotificationValueKind = "float"
	NotificationValueObject NotificationValueKind = "object"
	NotificationValueArray  NotificationValueKind = "array"
)

type NotificationField struct {
	Name  string
	Value NotificationValue
}

type NotificationValue struct {
	Kind   NotificationValueKind
	String string
	Bool   bool
	Int    int64
	Uint   uint64
	Float  float64
	Object []NotificationField
	Array  []NotificationValue
}

type NotificationPayload struct {
	Fields []NotificationField
}

func NotificationPayloadFromMap(input map[string]any) (NotificationPayload, error) {
	if len(input) == 0 {
		return NotificationPayload{}, nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]NotificationField, 0, len(keys))
	for _, key := range keys {
		value, err := notificationValueFromAny(input[key])
		if err != nil {
			return NotificationPayload{}, fmt.Errorf("field %s: %w", key, err)
		}
		fields = append(fields, NotificationField{Name: key, Value: value})
	}
	return NotificationPayload{Fields: fields}, nil
}

func (payload NotificationPayload) Map() map[string]any {
	if len(payload.Fields) == 0 {
		return nil
	}
	ret := make(map[string]any, len(payload.Fields))
	for _, field := range payload.Fields {
		if field.Name != "" {
			ret[field.Name] = field.Value.Any()
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func (value NotificationValue) Any() any {
	switch value.Kind {
	case NotificationValueNull:
		return nil
	case NotificationValueString:
		return value.String
	case NotificationValueBool:
		return value.Bool
	case NotificationValueInt:
		return value.Int
	case NotificationValueUint:
		return value.Uint
	case NotificationValueFloat:
		return value.Float
	case NotificationValueObject:
		return NotificationPayload{Fields: value.Object}.Map()
	case NotificationValueArray:
		ret := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			ret = append(ret, item.Any())
		}
		return ret
	default:
		return nil
	}
}

func notificationValueFromAny(input any) (NotificationValue, error) {
	switch value := input.(type) {
	case nil:
		return NotificationValue{Kind: NotificationValueNull}, nil
	case string:
		return NotificationValue{Kind: NotificationValueString, String: value}, nil
	case bool:
		return NotificationValue{Kind: NotificationValueBool, Bool: value}, nil
	case int:
		return NotificationValue{Kind: NotificationValueInt, Int: int64(value)}, nil
	case int8:
		return NotificationValue{Kind: NotificationValueInt, Int: int64(value)}, nil
	case int16:
		return NotificationValue{Kind: NotificationValueInt, Int: int64(value)}, nil
	case int32:
		return NotificationValue{Kind: NotificationValueInt, Int: int64(value)}, nil
	case int64:
		return NotificationValue{Kind: NotificationValueInt, Int: value}, nil
	case uint:
		return NotificationValue{Kind: NotificationValueUint, Uint: uint64(value)}, nil
	case uint8:
		return NotificationValue{Kind: NotificationValueUint, Uint: uint64(value)}, nil
	case uint16:
		return NotificationValue{Kind: NotificationValueUint, Uint: uint64(value)}, nil
	case uint32:
		return NotificationValue{Kind: NotificationValueUint, Uint: uint64(value)}, nil
	case uint64:
		return NotificationValue{Kind: NotificationValueUint, Uint: value}, nil
	case float32:
		return NotificationValue{Kind: NotificationValueFloat, Float: float64(value)}, nil
	case float64:
		return NotificationValue{Kind: NotificationValueFloat, Float: value}, nil
	case map[string]any:
		payload, err := NotificationPayloadFromMap(value)
		return NotificationValue{Kind: NotificationValueObject, Object: payload.Fields}, err
	case map[string]string:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := NotificationPayloadFromMap(mapped)
		return NotificationValue{Kind: NotificationValueObject, Object: payload.Fields}, err
	case map[string][]string:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := NotificationPayloadFromMap(mapped)
		return NotificationValue{Kind: NotificationValueObject, Object: payload.Fields}, err
	case map[string]bool:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := NotificationPayloadFromMap(mapped)
		return NotificationValue{Kind: NotificationValueObject, Object: payload.Fields}, err
	case map[string]int:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := NotificationPayloadFromMap(mapped)
		return NotificationValue{Kind: NotificationValueObject, Object: payload.Fields}, err
	case map[string]int64:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := NotificationPayloadFromMap(mapped)
		return NotificationValue{Kind: NotificationValueObject, Object: payload.Fields}, err
	case map[string]float64:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := NotificationPayloadFromMap(mapped)
		return NotificationValue{Kind: NotificationValueObject, Object: payload.Fields}, err
	case []any:
		return notificationValueArray(value)
	case []string:
		items := make([]NotificationValue, 0, len(value))
		for _, item := range value {
			items = append(items, NotificationValue{Kind: NotificationValueString, String: item})
		}
		return NotificationValue{Kind: NotificationValueArray, Array: items}, nil
	case []bool:
		items := make([]NotificationValue, 0, len(value))
		for _, item := range value {
			items = append(items, NotificationValue{Kind: NotificationValueBool, Bool: item})
		}
		return NotificationValue{Kind: NotificationValueArray, Array: items}, nil
	case []int:
		items := make([]NotificationValue, 0, len(value))
		for _, item := range value {
			items = append(items, NotificationValue{Kind: NotificationValueInt, Int: int64(item)})
		}
		return NotificationValue{Kind: NotificationValueArray, Array: items}, nil
	case []int64:
		items := make([]NotificationValue, 0, len(value))
		for _, item := range value {
			items = append(items, NotificationValue{Kind: NotificationValueInt, Int: item})
		}
		return NotificationValue{Kind: NotificationValueArray, Array: items}, nil
	case []float64:
		items := make([]NotificationValue, 0, len(value))
		for _, item := range value {
			items = append(items, NotificationValue{Kind: NotificationValueFloat, Float: item})
		}
		return NotificationValue{Kind: NotificationValueArray, Array: items}, nil
	case []map[string]any:
		items := make([]any, 0, len(value))
		for _, item := range value {
			items = append(items, item)
		}
		return notificationValueArray(items)
	default:
		return NotificationValue{}, fmt.Errorf("%w: unsupported run notification value %T", common.ErrInvalidArgument, input)
	}
}

func notificationValueArray(input []any) (NotificationValue, error) {
	items := make([]NotificationValue, 0, len(input))
	for _, item := range input {
		converted, err := notificationValueFromAny(item)
		if err != nil {
			return NotificationValue{}, err
		}
		items = append(items, converted)
	}
	return NotificationValue{Kind: NotificationValueArray, Array: items}, nil
}
