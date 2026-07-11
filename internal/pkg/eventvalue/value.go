package eventvalue

import (
	"fmt"
	"sort"
)

type Kind string

const (
	KindNull   Kind = "null"
	KindString Kind = "string"
	KindBool   Kind = "bool"
	KindInt    Kind = "int"
	KindUint   Kind = "uint"
	KindFloat  Kind = "float"
	KindObject Kind = "object"
	KindArray  Kind = "array"
)

type Field struct {
	Name  string
	Value Value
}

type Value struct {
	Kind   Kind
	String string
	Bool   bool
	Int    int64
	Uint   uint64
	Float  float64
	Object []Field
	Array  []Value
}

type Payload struct {
	Fields []Field
}

func FromMap(input map[string]any) (Payload, error) {
	if len(input) == 0 {
		return Payload{}, nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]Field, 0, len(keys))
	for _, key := range keys {
		value, err := fromAny(input[key])
		if err != nil {
			return Payload{}, fmt.Errorf("field %s: %w", key, err)
		}
		fields = append(fields, Field{Name: key, Value: value})
	}
	return Payload{Fields: fields}, nil
}

func FromMapSlice(input []map[string]any) ([]Payload, error) {
	if len(input) == 0 {
		return nil, nil
	}
	ret := make([]Payload, 0, len(input))
	for idx, item := range input {
		payload, err := FromMap(item)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", idx, err)
		}
		ret = append(ret, payload)
	}
	return ret, nil
}

func (p Payload) Map() map[string]any {
	if len(p.Fields) == 0 {
		return nil
	}
	ret := make(map[string]any, len(p.Fields))
	for _, field := range p.Fields {
		if field.Name != "" {
			ret[field.Name] = field.Value.Any()
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func MapSlice(input []Payload) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, item.Map())
	}
	return ret
}

func (v Value) Any() any {
	switch v.Kind {
	case KindNull:
		return nil
	case KindString:
		return v.String
	case KindBool:
		return v.Bool
	case KindInt:
		return v.Int
	case KindUint:
		return v.Uint
	case KindFloat:
		return v.Float
	case KindObject:
		return Payload{Fields: v.Object}.Map()
	case KindArray:
		ret := make([]any, 0, len(v.Array))
		for _, item := range v.Array {
			ret = append(ret, item.Any())
		}
		return ret
	default:
		return nil
	}
}

func fromAny(input any) (Value, error) {
	switch value := input.(type) {
	case nil:
		return Value{Kind: KindNull}, nil
	case string:
		return Value{Kind: KindString, String: value}, nil
	case bool:
		return Value{Kind: KindBool, Bool: value}, nil
	case int:
		return Value{Kind: KindInt, Int: int64(value)}, nil
	case int8:
		return Value{Kind: KindInt, Int: int64(value)}, nil
	case int16:
		return Value{Kind: KindInt, Int: int64(value)}, nil
	case int32:
		return Value{Kind: KindInt, Int: int64(value)}, nil
	case int64:
		return Value{Kind: KindInt, Int: value}, nil
	case uint:
		return Value{Kind: KindUint, Uint: uint64(value)}, nil
	case uint8:
		return Value{Kind: KindUint, Uint: uint64(value)}, nil
	case uint16:
		return Value{Kind: KindUint, Uint: uint64(value)}, nil
	case uint32:
		return Value{Kind: KindUint, Uint: uint64(value)}, nil
	case uint64:
		return Value{Kind: KindUint, Uint: value}, nil
	case float32:
		return Value{Kind: KindFloat, Float: float64(value)}, nil
	case float64:
		return Value{Kind: KindFloat, Float: value}, nil
	case map[string]any:
		payload, err := FromMap(value)
		return Value{Kind: KindObject, Object: payload.Fields}, err
	case map[string]string:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := FromMap(mapped)
		return Value{Kind: KindObject, Object: payload.Fields}, err
	case map[string]bool:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := FromMap(mapped)
		return Value{Kind: KindObject, Object: payload.Fields}, err
	case map[string]int:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := FromMap(mapped)
		return Value{Kind: KindObject, Object: payload.Fields}, err
	case map[string]int64:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := FromMap(mapped)
		return Value{Kind: KindObject, Object: payload.Fields}, err
	case map[string]float64:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := FromMap(mapped)
		return Value{Kind: KindObject, Object: payload.Fields}, err
	case []any:
		return fromArray(value)
	case []string:
		items := make([]Value, 0, len(value))
		for _, item := range value {
			items = append(items, Value{Kind: KindString, String: item})
		}
		return Value{Kind: KindArray, Array: items}, nil
	case []bool:
		items := make([]Value, 0, len(value))
		for _, item := range value {
			items = append(items, Value{Kind: KindBool, Bool: item})
		}
		return Value{Kind: KindArray, Array: items}, nil
	case []int:
		items := make([]Value, 0, len(value))
		for _, item := range value {
			items = append(items, Value{Kind: KindInt, Int: int64(item)})
		}
		return Value{Kind: KindArray, Array: items}, nil
	case []int64:
		items := make([]Value, 0, len(value))
		for _, item := range value {
			items = append(items, Value{Kind: KindInt, Int: item})
		}
		return Value{Kind: KindArray, Array: items}, nil
	case []float64:
		items := make([]Value, 0, len(value))
		for _, item := range value {
			items = append(items, Value{Kind: KindFloat, Float: item})
		}
		return Value{Kind: KindArray, Array: items}, nil
	case []map[string]any:
		items := make([]any, 0, len(value))
		for _, item := range value {
			items = append(items, item)
		}
		return fromArray(items)
	default:
		return Value{}, fmt.Errorf("unsupported event value %T", input)
	}
}

func fromArray(input []any) (Value, error) {
	items := make([]Value, 0, len(input))
	for _, item := range input {
		converted, err := fromAny(item)
		if err != nil {
			return Value{}, err
		}
		items = append(items, converted)
	}
	return Value{Kind: KindArray, Array: items}, nil
}
