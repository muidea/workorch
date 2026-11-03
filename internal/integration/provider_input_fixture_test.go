package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func fixtureExpandPromptSchema(raw json.RawMessage) (json.RawMessage, error) {
	var root map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&root); err != nil {
		return nil, err
	}
	defs, _ := root["$defs"].(map[string]any)
	var expand func(any, map[string]bool) (any, error)
	expand = func(value any, active map[string]bool) (any, error) {
		switch v := value.(type) {
		case map[string]any:
			if ref, found := v["$ref"]; found {
				s, ok := ref.(string)
				if !ok || !strings.HasPrefix(s, "#/$defs/") || len(v) != 1 || active[s] {
					return nil, fmt.Errorf("fixture schema reference invalid or cyclic")
				}
				body, found := defs[strings.TrimPrefix(s, "#/$defs/")]
				if !found {
					return nil, fmt.Errorf("fixture schema reference unresolved")
				}
				active[s] = true
				result, err := expand(body, active)
				delete(active, s)
				return result, err
			}
			result := make(map[string]any, len(v))
			for k, child := range v {
				if k == "$defs" {
					continue
				}
				if k == "const" || k == "enum" || k == "default" || k == "examples" {
					result[k] = child
					continue
				}
				item, err := expand(child, active)
				if err != nil {
					return nil, err
				}
				result[k] = item
			}
			return result, nil
		case []any:
			result := make([]any, len(v))
			for i, child := range v {
				item, err := expand(child, active)
				if err != nil {
					return nil, err
				}
				result[i] = item
			}
			return result, nil
		}
		return value, nil
	}
	value, err := expand(root, map[string]bool{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

// The synthetic provider consumes the same frozen-request aliases as a model.
// Resolve only the declared local corpus, then retain the existing complete
// content/intent assertions. An empty aliased body is not a missing owner body.
func fixtureFrozenWorkSource(input fixtureResponsesInput, context json.RawMessage) (*rc.RuntimeCandidateWorkContent, int, error) {
	var tail struct {
		Attempt *int                                       `json:"candidateAttempt"`
		Source  *rc.RuntimeCandidateWorkContent            `json:"implementationWorkContent"`
		Rule    string                                     `json:"referenceRule"`
		Refs    []struct{ Snapshot, Generation, File int } `json:"workFileRefs"`
	}
	count := 0
	for _, line := range strings.Split(string(input), "\n") {
		var value json.RawMessage = []byte(line)
		var candidate struct {
			Attempt *int `json:"candidateAttempt"`
		}
		if json.Unmarshal(value, &candidate) == nil && candidate.Attempt != nil {
			if err := json.Unmarshal(value, &tail); err != nil {
				return nil, -1, err
			}
			count++
		}
	}
	if count != 1 || tail.Source == nil {
		return nil, -1, fmt.Errorf("fixture requires one exact execution tail")
	}
	var authority struct {
		Contexts []struct {
			Context *rc.RuntimeValidationGenerationContext `json:"context"`
		} `json:"validationGenerationContexts"`
	}
	if err := json.Unmarshal(context, &authority); err != nil {
		return nil, -1, err
	}
	if len(tail.Refs) > 0 && tail.Rule == "" {
		return nil, -1, fmt.Errorf("fixture reference rule missing")
	}
	for _, ref := range tail.Refs {
		if ref.Snapshot < 0 || ref.Snapshot >= len(tail.Source.Snapshots) || ref.Generation < 0 || ref.Generation >= len(authority.Contexts) {
			return nil, -1, fmt.Errorf("fixture reference outside request")
		}
		g := authority.Contexts[ref.Generation].Context
		if g == nil || ref.File < 0 || ref.File >= len(g.Files) {
			return nil, -1, fmt.Errorf("fixture corpus missing")
		}
		s, f := &tail.Source.Snapshots[ref.Snapshot], g.Files[ref.File]
		if s.Content != "" || f.Content == "" || s.TargetRef != f.TargetRef || s.ContentHash != f.ContentHash || rc.RuntimeValidationInputContentHash(f.Content) != f.ContentHash {
			return nil, -1, fmt.Errorf("fixture alias changed exact bytes/identity")
		}
		s.Content = f.Content
	}
	return tail.Source, *tail.Attempt, nil
}
