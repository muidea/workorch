package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// compactRuntimeParameterSchema factors only identical, self-contained parameter
// schemas. The strict transport schema and local manifest decoder stay original;
// this equivalent grammar is only the versioned provider prompt projection.
func compactRuntimeParameterSchema(raw []byte) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	// The compiler currently has no definitions. Do not capture or reinterpret a
	// future schema's reference namespace without its own rendering contract.
	if _, found := root["$defs"]; found {
		return raw, nil
	}
	var properties, operations, items map[string]json.RawMessage
	if json.Unmarshal(root["properties"], &properties) != nil || json.Unmarshal(properties["operations"], &operations) != nil || json.Unmarshal(operations["items"], &items) != nil {
		return raw, nil
	}
	var branches []map[string]json.RawMessage
	if json.Unmarshal(items["oneOf"], &branches) != nil {
		return raw, nil
	}
	type group struct {
		schema json.RawMessage
		slots  []int
	}
	var groups []group
	for i, branch := range branches {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(branch["properties"], &fields); err != nil {
			return nil, err
		}
		parameter := fields["parameters"]
		if len(parameter) == 0 || !selfContainedParameterSchema(parameter) {
			continue
		}
		found := false
		for j := range groups {
			if bytes.Equal(groups[j].schema, parameter) {
				groups[j].slots = append(groups[j].slots, i)
				found = true
				break
			}
		}
		if !found {
			groups = append(groups, group{parameter, []int{i}})
		}
	}
	definitions := map[string]json.RawMessage{}
	for _, g := range groups {
		if len(g.slots) < 2 {
			continue
		}
		name := fmt.Sprintf("parameters%d", len(definitions))
		definitions[name] = g.schema
		ref, _ := json.Marshal(struct {
			Ref string `json:"$ref"`
		}{"#/$defs/" + name})
		for _, i := range g.slots {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(branches[i]["properties"], &fields); err != nil {
				return nil, err
			}
			fields["parameters"] = ref
			body, err := json.Marshal(fields)
			if err != nil {
				return nil, err
			}
			branches[i]["properties"] = body
		}
	}
	if len(definitions) == 0 {
		return raw, nil
	}
	encode := func(value any) (json.RawMessage, error) { body, err := json.Marshal(value); return body, err }
	var err error
	if items["oneOf"], err = encode(branches); err != nil {
		return nil, err
	}
	if operations["items"], err = encode(items); err != nil {
		return nil, err
	}
	if properties["operations"], err = encode(operations); err != nil {
		return nil, err
	}
	if root["properties"], err = encode(properties); err != nil {
		return nil, err
	}
	if root["$defs"], err = encode(definitions); err != nil {
		return nil, err
	}
	compact, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	// Replacing a small schema with long reference paths must never inflate it.
	if len(compact) >= len(raw) {
		return raw, nil
	}
	return compact, nil
}

func selfContainedParameterSchema(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) == nil && object != nil {
		for key, value := range object {
			switch key {
			case "$ref", "$id", "id", "$anchor", "$dynamicRef", "$dynamicAnchor", "$recursiveRef", "$recursiveAnchor", "$schema":
				return false
			}
			if !selfContainedParameterSchema(value) {
				return false
			}
		}
		return true
	}
	var array []json.RawMessage
	if json.Unmarshal(raw, &array) == nil {
		for _, v := range array {
			if !selfContainedParameterSchema(v) {
				return false
			}
		}
	}
	return true
}
