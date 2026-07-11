package events

import (
	"fmt"
	"sort"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type EventValueKind string

const (
	EventValueNull   EventValueKind = "null"
	EventValueString EventValueKind = "string"
	EventValueBool   EventValueKind = "bool"
	EventValueInt    EventValueKind = "int"
	EventValueUint   EventValueKind = "uint"
	EventValueFloat  EventValueKind = "float"
	EventValueObject EventValueKind = "object"
	EventValueArray  EventValueKind = "array"
)

type EventField struct {
	Name  string
	Value EventValue
}

type EventValue struct {
	Kind   EventValueKind
	String string
	Bool   bool
	Int    int64
	Uint   uint64
	Float  float64
	Object []EventField
	Array  []EventValue
}

type EventPayload struct {
	Fields []EventField
}

type EventRecordData struct {
	ID             common.EventRecordID
	Version        string
	Topic          string
	OccurredAt     string
	SourceModule   string
	SubjectType    string
	SubjectID      string
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	StepID         common.StepID
	Revision       int64
	Payload        EventPayload
	PayloadSummary EventPayload
	PayloadRef     string
}

func EventRecordDataFromModel(item *eventstreammodel.EventRecord) (EventRecordData, error) {
	if item == nil {
		return EventRecordData{}, common.ErrInvalidArgument
	}
	payload, err := EventPayloadFromMap(item.Payload)
	if err != nil {
		return EventRecordData{}, fmt.Errorf("event record %s payload: %w", item.ID, err)
	}
	summary, err := EventPayloadFromMap(item.PayloadSummary)
	if err != nil {
		return EventRecordData{}, fmt.Errorf("event record %s payload summary: %w", item.ID, err)
	}
	return EventRecordData{
		ID:             item.ID,
		Version:        item.Version,
		Topic:          item.Topic,
		OccurredAt:     item.OccurredAt,
		SourceModule:   item.SourceModule,
		SubjectType:    item.SubjectType,
		SubjectID:      item.SubjectID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Revision:       item.Revision,
		Payload:        payload,
		PayloadSummary: summary,
		PayloadRef:     item.PayloadRef,
	}, nil
}

func (item EventRecordData) Model() *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		ID:             item.ID,
		Version:        item.Version,
		Topic:          item.Topic,
		OccurredAt:     item.OccurredAt,
		SourceModule:   item.SourceModule,
		SubjectType:    item.SubjectType,
		SubjectID:      item.SubjectID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Revision:       item.Revision,
		Payload:        item.Payload.Map(),
		PayloadSummary: item.PayloadSummary.Map(),
		PayloadRef:     item.PayloadRef,
	}
}

func EventPayloadFromMap(input map[string]any) (EventPayload, error) {
	if len(input) == 0 {
		return EventPayload{}, nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]EventField, 0, len(keys))
	for _, key := range keys {
		value, err := eventValueFromAny(input[key])
		if err != nil {
			return EventPayload{}, fmt.Errorf("field %s: %w", key, err)
		}
		fields = append(fields, EventField{Name: key, Value: value})
	}
	return EventPayload{Fields: fields}, nil
}

func (payload EventPayload) Map() map[string]any {
	if len(payload.Fields) == 0 {
		return nil
	}
	ret := make(map[string]any, len(payload.Fields))
	for _, field := range payload.Fields {
		if field.Name == "" {
			continue
		}
		ret[field.Name] = field.Value.Any()
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func (value EventValue) Any() any {
	switch value.Kind {
	case EventValueNull:
		return nil
	case EventValueString:
		return value.String
	case EventValueBool:
		return value.Bool
	case EventValueInt:
		return value.Int
	case EventValueUint:
		return value.Uint
	case EventValueFloat:
		return value.Float
	case EventValueObject:
		ret := make(map[string]any, len(value.Object))
		for _, field := range value.Object {
			if field.Name != "" {
				ret[field.Name] = field.Value.Any()
			}
		}
		return ret
	case EventValueArray:
		ret := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			ret = append(ret, item.Any())
		}
		return ret
	default:
		return nil
	}
}

func eventValueFromAny(input any) (EventValue, error) {
	switch value := input.(type) {
	case nil:
		return EventValue{Kind: EventValueNull}, nil
	case string:
		return EventValue{Kind: EventValueString, String: value}, nil
	case bool:
		return EventValue{Kind: EventValueBool, Bool: value}, nil
	case int:
		return EventValue{Kind: EventValueInt, Int: int64(value)}, nil
	case int8:
		return EventValue{Kind: EventValueInt, Int: int64(value)}, nil
	case int16:
		return EventValue{Kind: EventValueInt, Int: int64(value)}, nil
	case int32:
		return EventValue{Kind: EventValueInt, Int: int64(value)}, nil
	case int64:
		return EventValue{Kind: EventValueInt, Int: value}, nil
	case uint:
		return EventValue{Kind: EventValueUint, Uint: uint64(value)}, nil
	case uint8:
		return EventValue{Kind: EventValueUint, Uint: uint64(value)}, nil
	case uint16:
		return EventValue{Kind: EventValueUint, Uint: uint64(value)}, nil
	case uint32:
		return EventValue{Kind: EventValueUint, Uint: uint64(value)}, nil
	case uint64:
		return EventValue{Kind: EventValueUint, Uint: value}, nil
	case float32:
		return EventValue{Kind: EventValueFloat, Float: float64(value)}, nil
	case float64:
		return EventValue{Kind: EventValueFloat, Float: value}, nil
	case map[string]any:
		payload, err := EventPayloadFromMap(value)
		return EventValue{Kind: EventValueObject, Object: payload.Fields}, err
	case []any:
		return eventValueArray(value)
	case []string:
		items := make([]EventValue, 0, len(value))
		for _, item := range value {
			items = append(items, EventValue{Kind: EventValueString, String: item})
		}
		return EventValue{Kind: EventValueArray, Array: items}, nil
	case []map[string]any:
		items := make([]any, 0, len(value))
		for _, item := range value {
			items = append(items, item)
		}
		return eventValueArray(items)
	default:
		return EventValue{}, fmt.Errorf("%w: unsupported event payload value %T", common.ErrInvalidArgument, input)
	}
}

func eventValueArray(input []any) (EventValue, error) {
	items := make([]EventValue, 0, len(input))
	for _, item := range input {
		converted, err := eventValueFromAny(item)
		if err != nil {
			return EventValue{}, err
		}
		items = append(items, converted)
	}
	return EventValue{Kind: EventValueArray, Array: items}, nil
}
