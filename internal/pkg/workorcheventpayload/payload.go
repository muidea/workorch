package workorcheventpayload

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var ErrInvalidPayload = errors.New("invalid event payload")

type ValueKind string

const (
	ValueNull   ValueKind = "null"
	ValueString ValueKind = "string"
	ValueBool   ValueKind = "bool"
	ValueInt    ValueKind = "int"
	ValueUint   ValueKind = "uint"
	ValueFloat  ValueKind = "float"
	ValueObject ValueKind = "object"
	ValueArray  ValueKind = "array"
)

type Field struct {
	Name  string `json:"name"`
	Value Value  `json:"value"`
}

type Value struct {
	Kind   ValueKind `json:"kind"`
	String string    `json:"string,omitempty"`
	Bool   bool      `json:"bool,omitempty"`
	Int    int64     `json:"int,omitempty"`
	Uint   uint64    `json:"uint,omitempty"`
	Float  float64   `json:"float,omitempty"`
	Object []Field   `json:"object,omitempty"`
	Array  []Value   `json:"array,omitempty"`
}

type Payload struct {
	Fields []Field `json:"fields,omitempty"`
}

func (payload Payload) MarshalJSON() ([]byte, error) {
	value := payload.Map()
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
}

func (payload *Payload) UnmarshalJSON(data []byte) error {
	if payload == nil {
		return fmt.Errorf("%w: nil payload target", ErrInvalidPayload)
	}
	if trimmed := strings.TrimSpace(string(data)); trimmed == "" || trimmed == "null" {
		return fmt.Errorf("%w: payload must be a JSON object", ErrInvalidPayload)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}
	decoded, err := FromMap(value)
	if err != nil {
		return err
	}
	*payload = decoded
	return nil
}

func New(fields ...Field) Payload {
	return Payload{Fields: cloneFields(fields)}
}

func StringField(name string, value string) Field {
	return Field{Name: name, Value: Value{Kind: ValueString, String: value}}
}

func BoolField(name string, value bool) Field {
	return Field{Name: name, Value: Value{Kind: ValueBool, Bool: value}}
}

func IntField(name string, value int64) Field {
	return Field{Name: name, Value: Value{Kind: ValueInt, Int: value}}
}

func FloatField(name string, value float64) Field {
	return Field{Name: name, Value: Value{Kind: ValueFloat, Float: value}}
}

func ObjectField(name string, value Payload) Field {
	return Field{Name: name, Value: Value{Kind: ValueObject, Object: cloneFields(value.Fields)}}
}

func StringArrayField(name string, values []string) Field {
	items := make([]Value, 0, len(values))
	for _, value := range values {
		items = append(items, Value{Kind: ValueString, String: value})
	}
	return Field{Name: name, Value: Value{Kind: ValueArray, Array: items}}
}

func (payload Payload) Field(name string) (Value, bool) {
	for _, field := range payload.Fields {
		if field.Name == name {
			return field.Value, true
		}
	}
	return Value{}, false
}

func (payload Payload) String(name string) (string, bool) {
	value, ok := payload.Field(name)
	if !ok || value.Kind != ValueString {
		return "", false
	}
	return value.String, true
}

func (payload Payload) Bool(name string) (bool, bool) {
	value, ok := payload.Field(name)
	if !ok || value.Kind != ValueBool {
		return false, false
	}
	return value.Bool, true
}

func (payload Payload) Float64(name string) (float64, bool) {
	value, ok := payload.Field(name)
	if !ok {
		return 0, false
	}
	switch value.Kind {
	case ValueFloat:
		return value.Float, true
	case ValueInt:
		return float64(value.Int), true
	case ValueUint:
		return float64(value.Uint), true
	}
	return 0, false
}

func (payload Payload) StringSlice(name string) ([]string, bool) {
	value, ok := payload.Field(name)
	if !ok || value.Kind != ValueArray {
		return nil, false
	}
	ret := make([]string, 0, len(value.Array))
	for _, item := range value.Array {
		if item.Kind != ValueString {
			return nil, false
		}
		ret = append(ret, item.String)
	}
	return ret, true
}

func (payload Payload) Int64(name string) (int64, bool) {
	value, ok := payload.Field(name)
	if !ok {
		return 0, false
	}
	switch value.Kind {
	case ValueInt:
		return value.Int, true
	case ValueUint:
		if value.Uint <= uint64(^uint64(0)>>1) {
			return int64(value.Uint), true
		}
	case ValueFloat:
		return int64(value.Float), true
	}
	return 0, false
}

func (payload Payload) Object(name string) (Payload, bool) {
	value, ok := payload.Field(name)
	if !ok || value.Kind != ValueObject {
		return Payload{}, false
	}
	return Payload{Fields: cloneFields(value.Object)}, true
}

func (payload Payload) Clone() Payload {
	return Payload{Fields: cloneFields(payload.Fields)}
}

func CloneSlice(input []Payload) []Payload {
	if len(input) == 0 {
		return nil
	}
	ret := make([]Payload, 0, len(input))
	for _, item := range input {
		ret = append(ret, item.Clone())
	}
	return ret
}

func (payload Payload) Merge(overlay Payload) Payload {
	ret := payload.Clone()
	for _, field := range overlay.Fields {
		updated := false
		for idx := range ret.Fields {
			if ret.Fields[idx].Name == field.Name {
				ret.Fields[idx] = cloneField(field)
				updated = true
				break
			}
		}
		if !updated {
			ret.Fields = append(ret.Fields, cloneField(field))
		}
	}
	return ret
}

func (payload Payload) Equal(other Payload) bool {
	if len(payload.Fields) != len(other.Fields) {
		return false
	}
	left := payload.sortedFields()
	right := other.sortedFields()
	for idx := range left {
		if left[idx].Name != right[idx].Name || !left[idx].Value.Equal(right[idx].Value) {
			return false
		}
	}
	return true
}

func (payload Payload) CanonicalString() string {
	var builder strings.Builder
	appendCanonicalFields(&builder, payload.sortedFields())
	return builder.String()
}

func (payload Payload) Map() map[string]any {
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

func (value Value) Any() any {
	switch value.Kind {
	case ValueNull:
		return nil
	case ValueString:
		return value.String
	case ValueBool:
		return value.Bool
	case ValueInt:
		return value.Int
	case ValueUint:
		return value.Uint
	case ValueFloat:
		return value.Float
	case ValueObject:
		mapped := Payload{Fields: value.Object}.Map()
		if mapped == nil {
			return map[string]any{}
		}
		return mapped
	case ValueArray:
		ret := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			ret = append(ret, item.Any())
		}
		return ret
	default:
		return nil
	}
}

func (value Value) Equal(other Value) bool {
	if value.Kind != other.Kind || value.String != other.String || value.Bool != other.Bool || value.Int != other.Int || value.Uint != other.Uint || value.Float != other.Float {
		return false
	}
	if value.Kind == ValueObject {
		return Payload{Fields: value.Object}.Equal(Payload{Fields: other.Object})
	}
	if value.Kind == ValueArray {
		if len(value.Array) != len(other.Array) {
			return false
		}
		for idx := range value.Array {
			if !value.Array[idx].Equal(other.Array[idx]) {
				return false
			}
		}
	}
	return true
}

// FromMap is reserved for explicit JSON/persistence boundaries. Internal callers
// should construct Payload values directly instead of round-tripping through maps.
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
			return Payload{}, fmt.Errorf("payload field %s: %w", key, err)
		}
		fields = append(fields, Field{Name: key, Value: value})
	}
	return Payload{Fields: fields}, nil
}

// FromMapSlice is reserved for explicit product/persistence boundaries that
// still own map-shaped data. EventHub contracts should carry the returned
// Payload values directly.
func FromMapSlice(input []map[string]any) ([]Payload, error) {
	if len(input) == 0 {
		return nil, nil
	}
	ret := make([]Payload, 0, len(input))
	for idx, item := range input {
		payload, err := FromMap(item)
		if err != nil {
			return nil, fmt.Errorf("payload item %d: %w", idx, err)
		}
		ret = append(ret, payload)
	}
	return ret, nil
}

// MapSlice materializes payloads only for an explicit map-shaped boundary.
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

func fromAny(input any) (Value, error) {
	switch value := input.(type) {
	case nil:
		return Value{Kind: ValueNull}, nil
	case string:
		return Value{Kind: ValueString, String: value}, nil
	case bool:
		return Value{Kind: ValueBool, Bool: value}, nil
	case int:
		return Value{Kind: ValueInt, Int: int64(value)}, nil
	case int8:
		return Value{Kind: ValueInt, Int: int64(value)}, nil
	case int16:
		return Value{Kind: ValueInt, Int: int64(value)}, nil
	case int32:
		return Value{Kind: ValueInt, Int: int64(value)}, nil
	case int64:
		return Value{Kind: ValueInt, Int: value}, nil
	case uint:
		return Value{Kind: ValueUint, Uint: uint64(value)}, nil
	case uint8:
		return Value{Kind: ValueUint, Uint: uint64(value)}, nil
	case uint16:
		return Value{Kind: ValueUint, Uint: uint64(value)}, nil
	case uint32:
		return Value{Kind: ValueUint, Uint: uint64(value)}, nil
	case uint64:
		return Value{Kind: ValueUint, Uint: value}, nil
	case float32:
		return Value{Kind: ValueFloat, Float: float64(value)}, nil
	case float64:
		return Value{Kind: ValueFloat, Float: value}, nil
	case map[string]any:
		payload, err := FromMap(value)
		return Value{Kind: ValueObject, Object: payload.Fields}, err
	case map[string]string:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := FromMap(mapped)
		return Value{Kind: ValueObject, Object: payload.Fields}, err
	case map[string]bool:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := FromMap(mapped)
		return Value{Kind: ValueObject, Object: payload.Fields}, err
	case map[string]int:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := FromMap(mapped)
		return Value{Kind: ValueObject, Object: payload.Fields}, err
	case map[string]int64:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := FromMap(mapped)
		return Value{Kind: ValueObject, Object: payload.Fields}, err
	case map[string]float64:
		mapped := make(map[string]any, len(value))
		for key, item := range value {
			mapped[key] = item
		}
		payload, err := FromMap(mapped)
		return Value{Kind: ValueObject, Object: payload.Fields}, err
	case []string:
		items := make([]Value, 0, len(value))
		for _, item := range value {
			items = append(items, Value{Kind: ValueString, String: item})
		}
		return Value{Kind: ValueArray, Array: items}, nil
	case []bool:
		items := make([]Value, 0, len(value))
		for _, item := range value {
			items = append(items, Value{Kind: ValueBool, Bool: item})
		}
		return Value{Kind: ValueArray, Array: items}, nil
	case []int:
		items := make([]Value, 0, len(value))
		for _, item := range value {
			items = append(items, Value{Kind: ValueInt, Int: int64(item)})
		}
		return Value{Kind: ValueArray, Array: items}, nil
	case []int64:
		items := make([]Value, 0, len(value))
		for _, item := range value {
			items = append(items, Value{Kind: ValueInt, Int: item})
		}
		return Value{Kind: ValueArray, Array: items}, nil
	case []float64:
		items := make([]Value, 0, len(value))
		for _, item := range value {
			items = append(items, Value{Kind: ValueFloat, Float: item})
		}
		return Value{Kind: ValueArray, Array: items}, nil
	case []map[string]any:
		items := make([]Value, 0, len(value))
		for _, item := range value {
			converted, err := fromAny(item)
			if err != nil {
				return Value{}, err
			}
			items = append(items, converted)
		}
		return Value{Kind: ValueArray, Array: items}, nil
	case []any:
		items := make([]Value, 0, len(value))
		for _, item := range value {
			converted, err := fromAny(item)
			if err != nil {
				return Value{}, err
			}
			items = append(items, converted)
		}
		return Value{Kind: ValueArray, Array: items}, nil
	default:
		return Value{}, fmt.Errorf("unsupported value type %T", input)
	}
}

func cloneFields(fields []Field) []Field {
	if len(fields) == 0 {
		return nil
	}
	ret := make([]Field, 0, len(fields))
	for _, field := range fields {
		ret = append(ret, cloneField(field))
	}
	return ret
}

func cloneField(field Field) Field {
	value := field.Value
	value.Object = cloneFields(value.Object)
	value.Array = cloneValues(value.Array)
	return Field{Name: field.Name, Value: value}
}

func cloneValues(values []Value) []Value {
	if len(values) == 0 {
		return nil
	}
	ret := make([]Value, 0, len(values))
	for _, value := range values {
		value.Object = cloneFields(value.Object)
		value.Array = cloneValues(value.Array)
		ret = append(ret, value)
	}
	return ret
}

func (payload Payload) sortedFields() []Field {
	fields := cloneFields(payload.Fields)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	return fields
}

func appendCanonicalFields(builder *strings.Builder, fields []Field) {
	for _, field := range fields {
		builder.WriteString(strconv.Quote(field.Name))
		builder.WriteByte(':')
		appendCanonicalValue(builder, field.Value)
		builder.WriteByte(';')
	}
}

func appendCanonicalValue(builder *strings.Builder, value Value) {
	builder.WriteString(string(value.Kind))
	builder.WriteByte('=')
	switch value.Kind {
	case ValueString:
		builder.WriteString(strconv.Quote(value.String))
	case ValueBool:
		builder.WriteString(strconv.FormatBool(value.Bool))
	case ValueInt:
		builder.WriteString(strconv.FormatInt(value.Int, 10))
	case ValueUint:
		builder.WriteString(strconv.FormatUint(value.Uint, 10))
	case ValueFloat:
		builder.WriteString(strconv.FormatFloat(value.Float, 'g', -1, 64))
	case ValueObject:
		builder.WriteByte('{')
		appendCanonicalFields(builder, Payload{Fields: value.Object}.sortedFields())
		builder.WriteByte('}')
	case ValueArray:
		builder.WriteByte('[')
		for _, item := range value.Array {
			appendCanonicalValue(builder, item)
			builder.WriteByte(',')
		}
		builder.WriteByte(']')
	}
}
