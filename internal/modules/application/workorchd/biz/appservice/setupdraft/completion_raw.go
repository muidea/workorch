package setupdraft

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func parseCompletionAliases(payload []byte) (Draft, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return Draft{}, fmt.Errorf("%w: invalid workspace setup draft response: %v", common.ErrInvalidArgument, err)
	}
	for _, key := range []string{"draft", "workspaceSetupDraft", "result", "data"} {
		if nestedPayload, ok := raw[key]; ok {
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(nestedPayload, &nested); err == nil {
				draft := fromRawObject(nested)
				if HasContent(draft) {
					return NormalizeDraft(draft), nil
				}
			}
		}
	}
	draft := fromRawObject(raw)
	return NormalizeDraft(draft), nil
}

func firstRawString(raw map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		payload, ok := raw[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(payload, &value); err == nil && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstRawStringSlice(raw map[string]json.RawMessage, keys ...string) []string {
	for _, key := range keys {
		payload, ok := raw[key]
		if !ok {
			continue
		}
		values := rawStringSlice(payload)
		if len(values) > 0 {
			return values
		}
	}
	return nil
}

func rawStringSlice(payload json.RawMessage) []string {
	var values []string
	if err := json.Unmarshal(payload, &values); err == nil {
		return CompactStrings(values)
	}
	var value string
	if err := json.Unmarshal(payload, &value); err == nil {
		return CompactStrings(strings.Split(value, "\n"))
	}
	var objects []map[string]any
	if err := json.Unmarshal(payload, &objects); err == nil {
		ret := make([]string, 0, len(objects))
		for _, object := range objects {
			for _, key := range []string{"text", "content", "item", "description", "title"} {
				if text, ok := object[key].(string); ok && strings.TrimSpace(text) != "" {
					ret = append(ret, text)
					break
				}
			}
		}
		return CompactStrings(ret)
	}
	return nil
}
