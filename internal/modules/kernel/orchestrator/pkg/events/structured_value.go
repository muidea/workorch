package events

import (
	"fmt"
	"sort"

	"github.com/muidea/workorch/internal/pkg/common"
)

type StructuredValueKind string

const (
	StructuredNull   StructuredValueKind = "null"
	StructuredString StructuredValueKind = "string"
	StructuredBool   StructuredValueKind = "bool"
	StructuredInt    StructuredValueKind = "int"
	StructuredUint   StructuredValueKind = "uint"
	StructuredFloat  StructuredValueKind = "float"
	StructuredObject StructuredValueKind = "object"
	StructuredArray  StructuredValueKind = "array"
)

type StructuredField struct {
	Name  string
	Value StructuredValue
}

type StructuredValue struct {
	Kind   StructuredValueKind
	String string
	Bool   bool
	Int    int64
	Uint   uint64
	Float  float64
	Object []StructuredField
	Array  []StructuredValue
}

type StructuredPayload struct {
	Fields []StructuredField
}

func StructuredPayloadFromMap(input map[string]any) (StructuredPayload, error) {
	if len(input) == 0 {
		return StructuredPayload{}, nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]StructuredField, 0, len(keys))
	for _, key := range keys {
		value, err := structuredValueFromAny(input[key])
		if err != nil {
			return StructuredPayload{}, fmt.Errorf("field %s: %w", key, err)
		}
		fields = append(fields, StructuredField{Name: key, Value: value})
	}
	return StructuredPayload{Fields: fields}, nil
}

func (payload StructuredPayload) Map() map[string]any {
	if len(payload.Fields) == 0 {
		return nil
	}
	ret := make(map[string]any, len(payload.Fields))
	for _, field := range payload.Fields {
		if field.Name != "" {
			ret[field.Name] = field.Value.Any()
		}
	}
	return ret
}

func (value StructuredValue) Any() any {
	switch value.Kind {
	case StructuredNull:
		return nil
	case StructuredString:
		return value.String
	case StructuredBool:
		return value.Bool
	case StructuredInt:
		return value.Int
	case StructuredUint:
		return value.Uint
	case StructuredFloat:
		return value.Float
	case StructuredObject:
		return StructuredPayload{Fields: value.Object}.Map()
	case StructuredArray:
		ret := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			ret = append(ret, item.Any())
		}
		return ret
	default:
		return nil
	}
}

func structuredValueFromAny(input any) (StructuredValue, error) {
	switch value := input.(type) {
	case nil:
		return StructuredValue{Kind: StructuredNull}, nil
	case string:
		return StructuredValue{Kind: StructuredString, String: value}, nil
	case bool:
		return StructuredValue{Kind: StructuredBool, Bool: value}, nil
	case int:
		return StructuredValue{Kind: StructuredInt, Int: int64(value)}, nil
	case int8:
		return StructuredValue{Kind: StructuredInt, Int: int64(value)}, nil
	case int16:
		return StructuredValue{Kind: StructuredInt, Int: int64(value)}, nil
	case int32:
		return StructuredValue{Kind: StructuredInt, Int: int64(value)}, nil
	case int64:
		return StructuredValue{Kind: StructuredInt, Int: value}, nil
	case uint:
		return StructuredValue{Kind: StructuredUint, Uint: uint64(value)}, nil
	case uint8:
		return StructuredValue{Kind: StructuredUint, Uint: uint64(value)}, nil
	case uint16:
		return StructuredValue{Kind: StructuredUint, Uint: uint64(value)}, nil
	case uint32:
		return StructuredValue{Kind: StructuredUint, Uint: uint64(value)}, nil
	case uint64:
		return StructuredValue{Kind: StructuredUint, Uint: value}, nil
	case float32:
		return StructuredValue{Kind: StructuredFloat, Float: float64(value)}, nil
	case float64:
		return StructuredValue{Kind: StructuredFloat, Float: value}, nil
	case map[string]any:
		payload, err := StructuredPayloadFromMap(value)
		return StructuredValue{Kind: StructuredObject, Object: payload.Fields}, err
	case map[string]string:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := StructuredPayloadFromMap(mapped)
		return StructuredValue{Kind: StructuredObject, Object: payload.Fields}, err
	case map[string]int:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := StructuredPayloadFromMap(mapped)
		return StructuredValue{Kind: StructuredObject, Object: payload.Fields}, err
	case map[string][]string:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := StructuredPayloadFromMap(mapped)
		return StructuredValue{Kind: StructuredObject, Object: payload.Fields}, err
	case []any:
		return structuredArray(value)
	case []map[string]any:
		items := make([]any, 0, len(value))
		for _, item := range value {
			items = append(items, item)
		}
		return structuredArray(items)
	case []string:
		items := make([]StructuredValue, 0, len(value))
		for _, item := range value {
			items = append(items, StructuredValue{Kind: StructuredString, String: item})
		}
		return StructuredValue{Kind: StructuredArray, Array: items}, nil
	case []int:
		items := make([]StructuredValue, 0, len(value))
		for _, item := range value {
			items = append(items, StructuredValue{Kind: StructuredInt, Int: int64(item)})
		}
		return StructuredValue{Kind: StructuredArray, Array: items}, nil
	default:
		return StructuredValue{}, fmt.Errorf("%w: unsupported orchestrator structured value %T", common.ErrInvalidArgument, input)
	}
}

func structuredArray(input []any) (StructuredValue, error) {
	items := make([]StructuredValue, 0, len(input))
	for _, item := range input {
		converted, err := structuredValueFromAny(item)
		if err != nil {
			return StructuredValue{}, err
		}
		items = append(items, converted)
	}
	return StructuredValue{Kind: StructuredArray, Array: items}, nil
}
