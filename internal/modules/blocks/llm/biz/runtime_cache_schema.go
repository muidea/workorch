package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Visit only JSON Schema positions. Data under const/enum/default/examples and
// property names themselves are not schemas and must remain byte-value exact.
func visitPromptSchema(value any, visit func(map[string]any) any) any {
	node, ok := value.(map[string]any)
	if !ok {
		return value
	}
	mapPromptSchemaChildren(node, func(child any) any { return visitPromptSchema(child, visit) })
	return visit(node)
}

func mapPromptSchemaChildren(node map[string]any, rewrite func(any) any) {
	for _, key := range []string{"properties", "patternProperties", "dependentSchemas", "dependencies"} {
		if children, ok := node[key].(map[string]any); ok {
			keys := make([]string, 0, len(children))
			for name := range children {
				keys = append(keys, name)
			}
			sort.Strings(keys)
			for _, name := range keys {
				children[name] = rewrite(children[name])
			}
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		if children, ok := node[key].([]any); ok {
			for i, child := range children {
				children[i] = rewrite(child)
			}
		}
	}
	for _, key := range []string{"items", "additionalItems", "additionalProperties", "unevaluatedProperties", "unevaluatedItems", "contains", "propertyNames", "not", "if", "then", "else", "contentSchema"} {
		if child, found := node[key]; found {
			if children, ok := child.([]any); ok {
				for i, c := range children {
					children[i] = rewrite(c)
				}
			} else {
				node[key] = rewrite(child)
			}
		}
	}
}

// Factor exact repeated, self-contained schema subtrees only in the prompt.
// The transport schema and owner decoder remain complete and unchanged. No
// existing reference namespace is captured, no literal data is rewritten.
func compactRuntimePromptSchema(raw []byte) ([]byte, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("trailing prompt schema")
	}
	root, ok := value.(map[string]any)
	if !ok {
		return raw, nil
	}
	if !selfContainedPromptSchema(root) {
		return raw, nil
	}
	counts := map[string]int{}
	visitPromptSchema(value, func(node map[string]any) any {
		body, _ := json.Marshal(node)
		if len(body) >= 256 {
			counts[string(body)]++
		}
		return node
	})
	defs := map[string]any{}
	names := map[string]string{}
	// Select the largest repeated subtree first. Child factoring remains local
	// to its single definition body, so sharing a parent never duplicates defs.
	// Original schemas contain no references; these edges are acyclic.
	var factor func(any) any
	factor = func(value any) any {
		node, ok := value.(map[string]any)
		if !ok {
			return value
		}
		body, _ := json.Marshal(node)
		key := string(body)
		if len(body) >= 256 && counts[key] >= 2 {
			name, exists := names[key]
			if !exists {
				name = fmt.Sprintf("schema%d", len(names))
				names[key] = name
				mapPromptSchemaChildren(node, factor)
				defs[name] = node
			}
			return map[string]any{"$ref": "#/$defs/" + name}
		}
		mapPromptSchemaChildren(node, factor)
		return node
	}
	factored := factor(value)
	result, ok := factored.(map[string]any)
	if !ok || len(defs) == 0 {
		return raw, nil
	}
	result["$defs"] = defs
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if len(encoded) >= len(raw) {
		return raw, nil
	}
	return encoded, nil
}

// Root identity and dialect declarations stay at the root. Existing reference namespaces
// are left untouched rather than captured by the prompt-local definitions.
func selfContainedPromptSchema(root map[string]any) bool {
	check := make(map[string]any, len(root))
	for key, value := range root {
		if key != "$schema" && key != "$id" && key != "id" {
			check[key] = value
		}
	}
	ok := true
	visitPromptSchema(check, func(node map[string]any) any {
		for _, key := range []string{"$defs", "definitions", "$ref", "$id", "id", "$anchor", "$dynamicRef", "$dynamicAnchor", "$recursiveRef", "$recursiveAnchor", "$schema"} {
			if _, exists := node[key]; exists {
				ok = false
			}
		}
		return node
	})
	return ok
}
