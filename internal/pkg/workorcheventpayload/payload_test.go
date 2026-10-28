package workorcheventpayload

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPayloadJSONBoundaryUsesObjectShape(t *testing.T) {
	payload := New(
		StringField("operator", "tester"),
		BoolField("retry", true),
		ObjectField("semantic", New(StringField("intent", "approve_execution"))),
	)

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if string(raw) != `{"operator":"tester","retry":true,"semantic":{"intent":"approve_execution"}}` {
		t.Fatalf("payload JSON = %s, want object contract", raw)
	}

	var decoded Payload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if !decoded.Equal(payload) {
		t.Fatalf("decoded payload = %+v, want %+v", decoded, payload)
	}
}

func TestPayloadJSONBoundaryPreservesNestedEmptyObject(t *testing.T) {
	payload, err := FromMap(map[string]any{
		"schema": map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("FromMap: %v", err)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if string(raw) != `{"schema":{"properties":{},"type":"object"}}` {
		t.Fatalf("payload JSON = %s, want nested empty object", raw)
	}

	var decoded Payload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if !decoded.Equal(payload) {
		t.Fatalf("decoded payload = %+v, want %+v", decoded, payload)
	}
}

func TestPayloadJSONBoundaryRejectsNonObject(t *testing.T) {
	for _, raw := range []string{"null", `[]`, `"text"`} {
		var payload Payload
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			t.Fatalf("json.Unmarshal(%s) succeeded, want invalid payload", raw)
		}
	}
}

func TestPayloadRoundTripWithoutSerialization(t *testing.T) {
	input := map[string]any{
		"mode":  "resume",
		"count": 2,
		"flags": []any{true, "bounded"},
		"scope": map[string]any{"workspaceId": "ws-1"},
		"items": []map[string]any{{"id": "item-1"}},
	}
	payload, err := FromMap(input)
	if err != nil {
		t.Fatalf("FromMap: %v", err)
	}
	output := payload.Map()
	if output["mode"] != "resume" || output["count"] != int64(2) {
		t.Fatalf("output = %#v", output)
	}
	scope, ok := output["scope"].(map[string]any)
	if !ok || scope["workspaceId"] != "ws-1" {
		t.Fatalf("scope = %#v", output["scope"])
	}
	items, ok := output["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v", output["items"])
	}
}

func TestCloneSliceKeepsPayloadsIsolated(t *testing.T) {
	original := []Payload{New(
		StringField("id", "policy-1"),
		ObjectField("nested", New(StringField("state", "ready"))),
	)}

	cloned := CloneSlice(original)
	cloned[0].Fields[0].Value.String = "mutated"
	cloned[0].Fields[1].Value.Object[0].Value.String = "changed"

	id, _ := original[0].String("id")
	nested, _ := original[0].Object("nested")
	state, _ := nested.String("state")
	if id != "policy-1" || state != "ready" {
		t.Fatalf("original payload mutated through clone: id=%q state=%q", id, state)
	}
}

func TestClonePreservesExactEmptyCollectionsAndNestedOwnership(t *testing.T) {
	var decoded Payload
	if err := json.Unmarshal([]byte(`{"schema":{"properties":{},"required":[],"branches":[{"enum":[]},{"items":{}}]},"values":[[],{},null,"原文"]}`), &decoded); err != nil {
		t.Fatal(err)
	}
	for name, original := range map[string]Payload{
		"nil":         {},
		"empty":       {Fields: make([]Field, 0, 1)},
		"nested_json": decoded,
		"typed": {Fields: []Field{
			{Name: "nil_object", Value: Value{Kind: ValueObject}},
			{Name: "empty_object", Value: Value{Kind: ValueObject, Object: make([]Field, 0, 1)}},
			{Name: "nil_array", Value: Value{Kind: ValueArray}},
			{Name: "empty_array", Value: Value{Kind: ValueArray, Array: make([]Value, 0, 1)}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			cloned := original.Clone()
			if !reflect.DeepEqual(original, cloned) {
				t.Fatal("Clone changed the exact typed value, including nil/empty collections")
			}
			beforeJSON, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			beforeCanonical := original.CanonicalString()
			// Mutate every reachable collection in both the object and array
			// branches; a shallow top-level copy must not pass this check.
			var mutate func(*Value)
			mutate = func(v *Value) {
				for i := range v.Object {
					v.Object[i].Name = "changed"
					mutate(&v.Object[i].Value)
				}
				for i := range v.Array {
					mutate(&v.Array[i])
				}
				v.Object = append(v.Object, StringField("added", "changed"))
				v.Array = append(v.Array, Value{Kind: ValueString, String: "changed"})
			}
			for i := range cloned.Fields {
				cloned.Fields[i].Name = "changed"
				mutate(&cloned.Fields[i].Value)
			}
			cloned.Fields = append(cloned.Fields, StringField("added", "changed"))
			afterJSON, err := json.Marshal(original)
			if err != nil || string(beforeJSON) != string(afterJSON) || beforeCanonical != original.CanonicalString() {
				t.Fatal("source changed through nested clone")
			}
		})
	}
	for _, original := range [][]Payload{nil, make([]Payload, 0, 1), {{}, decoded}} {
		if !reflect.DeepEqual(original, CloneSlice(original)) {
			t.Fatal("CloneSlice changed nil/empty or nested typed values")
		}
	}
}

func TestPayloadMapSliceBoundaryRoundTrip(t *testing.T) {
	payloads, err := FromMapSlice([]map[string]any{{"state": "active"}, {"state": "completed"}})
	if err != nil {
		t.Fatalf("FromMapSlice: %v", err)
	}
	mapped := MapSlice(payloads)
	if len(mapped) != 2 || mapped[0]["state"] != "active" || mapped[1]["state"] != "completed" {
		t.Fatalf("mapped = %#v", mapped)
	}
}

func TestPayloadRejectsUnsupportedValues(t *testing.T) {
	if _, err := FromMap(map[string]any{"channel": make(chan struct{})}); err == nil {
		t.Fatal("expected unsupported value error")
	}
}
