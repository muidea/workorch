package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

// cacheLayoutJSON only reorders compiler-generated JSON objects. Scalars are
// copied as encoded bytes, arrays retain their semantic order, and file bodies
// are never parsed as JSON. No mutable state or cross-request cache is involved.
func cacheLayoutJSON(raw []byte, path string) ([]byte, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return nil, fmt.Errorf("invalid compiled cache-layout JSON")
	}
	switch raw[0] {
	case '{':
		decoder := json.NewDecoder(bytes.NewReader(raw))
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		type field struct {
			key   string
			value json.RawMessage
		}
		var fields []field
		seen := map[string]bool{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := token.(string)
			if !ok || seen[key] {
				return nil, fmt.Errorf("duplicate compiled cache-layout field")
			}
			seen[key] = true
			var value json.RawMessage
			if err = decoder.Decode(&value); err != nil {
				return nil, err
			}
			fields = append(fields, field{key, value})
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		if _, err := decoder.Token(); err != io.EOF {
			return nil, fmt.Errorf("trailing compiled cache-layout value")
		}
		first, last := cacheLayoutOrder(path)
		rank := func(key string) int {
			if i := slices.Index(first, key); i >= 0 {
				return i
			}
			if i := slices.Index(last, key); i >= 0 {
				return len(first) + 1 + i
			}
			return len(first)
		}
		slices.SortStableFunc(fields, func(a, b field) int { return rank(a.key) - rank(b.key) })
		var out bytes.Buffer
		out.WriteByte('{')
		for i, f := range fields {
			if i > 0 {
				out.WriteByte(',')
			}
			key, _ := json.Marshal(f.key)
			out.Write(key)
			out.WriteByte(':')
			value, err := cacheLayoutJSON(f.value, path+"/"+f.key)
			if err != nil {
				return nil, err
			}
			out.Write(value)
		}
		out.WriteByte('}')
		return out.Bytes(), nil
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		var out bytes.Buffer
		out.WriteByte('[')
		for i, item := range items {
			if i > 0 {
				out.WriteByte(',')
			}
			value, err := cacheLayoutJSON(item, path+"/[]")
			if err != nil {
				return nil, err
			}
			out.Write(value)
		}
		out.WriteByte(']')
		return out.Bytes(), nil
	default:
		return append([]byte(nil), raw...), nil
	}
}

func cacheLayoutOrder(path string) (first, last []string) {
	switch path {
	case "semantic":
		return []string{"workspaceId", "runId", "soul", "definitionSeed", "planStep", "outputInstruction", "schema"}, []string{"manifest", "batchId", "batchSequence", "batchCoverageWorkItemIds"}
	case "operations":
		return []string{"context", "outputContract"}, []string{"decision", "transaction"}
	case "operations/context":
		return []string{"workspaceId", "runId", "definitionRevision", "requiredState", "requiredEffectScopes", "requiredEvidenceKinds", "activation"}, []string{"evidenceRevision", "focus"}
	case "operations/context/validationGenerationContexts/[]":
		return []string{"ownerModuleId", "validationRecipeId", "context"}, []string{"catalogSnapshotId", "requirementHashes"}
	case "operations/context/validationGenerationContexts/[]/context":
		return nil, []string{"ObservedRevision"}
	case "operations/context/validationGenerationContexts/[]/context/Files/[]":
		return []string{"targetRef", "content"}, []string{"contentHash"}
	}
	if strings.Contains(path, "/schema/") && strings.HasSuffix(path, "/properties") {
		return nil, []string{"manifestHash", "wireContractHash", "requiredOperationRef", "batchId", "batchSequence"}
	}
	return nil, nil
}

// The sole duplicate removed by v4 is decision.Focus, after exact byte equality
// with context.focus has been proved. The full current focus remains in C3.
// A closed local reference states where it lives; this is not an authority ref.
func compactRuntimeDecisionFocus(raw []byte) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	var decision, context map[string]json.RawMessage
	if err := json.Unmarshal(root["decision"], &decision); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(root["context"], &context); err != nil {
		return nil, err
	}
	if len(decision["Focus"]) == 0 || !bytes.Equal(decision["Focus"], context["focus"]) {
		return raw, nil
	}
	delete(decision, "Focus")
	decision["FocusRef"] = json.RawMessage(`"context.focus"`)
	body, err := json.Marshal(decision)
	if err != nil {
		return nil, err
	}
	root["decision"] = body
	return json.Marshal(root)
}
