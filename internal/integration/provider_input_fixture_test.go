package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

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
func fixtureFrozenWorkSource(input fixtureResponsesInput, context, outputContract json.RawMessage) (*rc.RuntimeCandidateWorkContent, int, error) {
	var tail struct {
		Attempt *int                            `json:"candidateAttempt"`
		Source  *rc.RuntimeCandidateWorkContent `json:"implementationWorkContent"`
		Rule    string                          `json:"referenceRule"`
		Refs    []struct {
			Snapshot int    `json:"snapshot"`
			Source   string `json:"source"`
		} `json:"workFileRefs"`
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
	sources := map[string]rc.RuntimeValidationInputReviewFile{}
	for gi, g := range authority.Contexts {
		if g.Context != nil {
			for fi, f := range g.Context.Files {
				sources[fmt.Sprintf("C3.context.validationGenerationContexts[%d].context.Files[%d].content", gi, fi)] = f
			}
		}
	}
	var output struct {
		Operations []struct {
			InputRevision *struct {
				Binding     rc.RuntimeValidationProofRequirementBinding
				Observation *struct {
					Files []rc.RuntimeValidationInputReviewFile
				} `json:"ownerObservation"`
			} `json:"inputRevision"`
		} `json:"operationContracts"`
	}
	if err := json.Unmarshal(outputContract, &output); err != nil {
		return nil, -1, err
	}
	for oi, op := range output.Operations {
		if op.InputRevision == nil {
			continue
		}
		base := fmt.Sprintf("C3.outputContract.operationContracts[%d].inputRevision", oi)
		var files []rc.RuntimeValidationInputReviewFile
		if op.InputRevision.Observation != nil {
			base += ".ownerObservation.Files"
			files = op.InputRevision.Observation.Files
		} else if op.InputRevision.Binding.InputReview != nil {
			base += ".Binding.InputReview.Files"
			files = op.InputRevision.Binding.InputReview.Files
		}
		for fi, f := range files {
			sources[fmt.Sprintf("%s[%d].content", base, fi)] = f
		}
	}
	seen := map[int]bool{}
	for _, ref := range tail.Refs {
		if ref.Snapshot < 0 || ref.Snapshot >= len(tail.Source.Snapshots) || seen[ref.Snapshot] {
			return nil, -1, fmt.Errorf("fixture reference duplicate or outside request")
		}
		seen[ref.Snapshot] = true
		f, found := sources[ref.Source]
		if !found {
			return nil, -1, fmt.Errorf("fixture corpus missing: %s", ref.Source)
		}
		s := &tail.Source.Snapshots[ref.Snapshot]
		if s.Content != "" || f.Content == "" || s.TargetRef != f.TargetRef || s.ContentHash != f.ContentHash || rc.RuntimeValidationInputContentHash(f.Content) != f.ContentHash {
			return nil, -1, fmt.Errorf("fixture alias changed exact bytes/identity")
		}
		s.Content = f.Content
	}
	return tail.Source, *tail.Attempt, nil
}

func TestFixtureFrozenWorkSourceCurrentReferences(t *testing.T) {
	f := rc.RuntimeValidationInputReviewFile{TargetRef: "exact-target", Content: "完整原文\r\n"}
	f.ContentHash = rc.RuntimeValidationInputContentHash(f.Content)
	for _, source := range []string{"C3.context.validationGenerationContexts[0].context.Files[0].content", "C3.outputContract.operationContracts[0].inputRevision.Binding.InputReview.Files[0].content", "C3.outputContract.operationContracts[0].inputRevision.ownerObservation.Files[0].content"} {
		t.Run(source, func(t *testing.T) {
			context, _ := json.Marshal(map[string]any{"validationGenerationContexts": []any{map[string]any{"context": map[string]any{"Files": []any{f}}}}})
			revision := map[string]any{"Binding": map[string]any{"InputReview": map[string]any{"Files": []any{f}}}}
			if strings.Contains(source, "ownerObservation") {
				revision["ownerObservation"] = map[string]any{"Files": []any{f}}
			}
			output, _ := json.Marshal(map[string]any{"operationContracts": []any{map[string]any{"inputRevision": revision}}})
			for _, invalid := range []string{"", "unknown-source", "wrong-hash", "duplicate"} {
				t.Run(invalid, func(t *testing.T) {
					hash := f.ContentHash
					path := source
					if invalid == "wrong-hash" {
						hash = "wrong"
					}
					if invalid == "unknown-source" {
						path = "outside.request"
					}
					refs := []any{map[string]any{"snapshot": 0, "source": path}}
					if invalid == "duplicate" {
						refs = append(refs, refs[0])
					}
					tail, _ := json.Marshal(map[string]any{"candidateAttempt": 0, "implementationWorkContent": map[string]any{"snapshots": []any{map[string]any{"TargetRef": f.TargetRef, "ContentHash": hash, "Content": ""}}}, "referenceRule": "closed same-request source", "workFileRefs": refs})
					got, attempt, err := fixtureFrozenWorkSource(fixtureResponsesInput(tail), context, output)
					if invalid != "" {
						if err == nil {
							t.Fatal("broken source accepted")
						}
						return
					}
					if err != nil || attempt != 0 || got == nil || len(got.Snapshots) != 1 || got.Snapshots[0].Content != f.Content {
						t.Fatal("exact source did not expand", err)
					}
				})
			}
		})
	}
}
