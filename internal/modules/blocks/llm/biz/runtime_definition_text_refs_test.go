package llm

import (
	"encoding/json"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
	"strings"
	"testing"
)

func TestDefinitionTextReferencesAreExactClosedAndIsolated(t *testing.T) {
	text := strings.Repeat("原始 Goal `x`\n", 100)
	p := runtimeCandidateDefinitionProjection{GoalDefinition: text, DoDDefinition: "DoD", SourceObligations: &rc.RuntimeSourceObligationInventory{Sources: []rc.RuntimeDefinitionSource{{Text: text}, {Text: text + " "}}, Requirements: []rc.RuntimeSourceRequirement{{Text: text}}}}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), `原始 Goal`) != 200 {
		t.Fatal("exact copies not shared or whitespace-distinct text merged")
	}
	var expanded runtimeCandidateDefinitionProjection
	if err = json.Unmarshal(raw, &expanded); err != nil || !reflect.DeepEqual(p, expanded) {
		t.Fatal("lossless expansion", err)
	}
	if p.SourceObligations.Sources[0].Text != text {
		t.Fatal("source mutated")
	}
	for _, kind := range []string{"unknown", "cycle", "mixed", "duplicate", "rule"} {
		t.Run(kind, func(t *testing.T) {
			var w runtimeDefinitionWire
			json.Unmarshal(raw, &w)
			switch kind {
			case "unknown":
				w.TextRefs[0].Ref = "other-workspace"
			case "cycle":
				w.TextRefs[0].Ref = w.TextRefs[0].Field
			case "mixed":
				w.SourceObligations.Sources[0].Text = "new"
			case "duplicate":
				w.TextRefs = append(w.TextRefs, w.TextRefs[0])
			case "rule":
				w.TextRefRule = ""
			}
			bad, _ := json.Marshal(w)
			if json.Unmarshal(bad, &expanded) == nil {
				t.Fatal("invalid alias accepted")
			}
		})
	}
}
