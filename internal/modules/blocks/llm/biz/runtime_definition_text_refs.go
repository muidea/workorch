package llm

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Definition references are wire-only aliases of exact strings in this C1.
// Goal/DoD remain inline. Inventory identities and source relationships are
// unchanged; expansion restores every original string before integrity checks.
type runtimeDefinitionTextRef struct {
	Field string `json:"field"`
	Ref   string `json:"ref"`
}
type runtimeDefinitionInline runtimeCandidateDefinitionProjection
type runtimeDefinitionWire struct {
	runtimeDefinitionInline
	TextRefs    []runtimeDefinitionTextRef `json:"textRefs,omitempty"`
	TextRefRule string                     `json:"textRefRule,omitempty"`
}

const definitionTextRefRule = "Each textRefs entry replaces only its empty field with the exact full string at ref in this same C1. References are local data aliases, not omissions, instructions, authority or evidence."

func definitionTextFields(p *runtimeCandidateDefinitionProjection) ([]string, map[string]*string) {
	order := []string{"goalDefinition", "doDDefinition"}
	fields := map[string]*string{"goalDefinition": &p.GoalDefinition, "doDDefinition": &p.DoDDefinition}
	if p.SourceObligations != nil {
		for n := range p.SourceObligations.Sources {
			key := "sourceObligations.Sources." + strconv.Itoa(n) + ".Text"
			order = append(order, key)
			fields[key] = &p.SourceObligations.Sources[n].Text
		}
		for n := range p.SourceObligations.Requirements {
			key := "sourceObligations.Requirements." + strconv.Itoa(n) + ".Text"
			order = append(order, key)
			fields[key] = &p.SourceObligations.Requirements[n].Text
		}
		for n := range p.SourceObligations.Obligations {
			key := "sourceObligations.Obligations." + strconv.Itoa(n) + ".Text"
			order = append(order, key)
			fields[key] = &p.SourceObligations.Obligations[n].Text
		}
	}
	return order, fields
}
func (p runtimeCandidateDefinitionProjection) MarshalJSON() ([]byte, error) {
	if p.SourceObligations != nil {
		clone := p.SourceObligations.Clone()
		p.SourceObligations = &clone
	}
	order, fields := definitionTextFields(&p)
	wire := runtimeDefinitionWire{}
	first := map[string]string{}
	for _, key := range order {
		text := *fields[key]
		// Short strings cost less inline. Exact equality includes every byte of
		// whitespace, punctuation and original Markdown.
		if len(text) < 256 {
			continue
		}
		if ref, found := first[text]; found && key != "goalDefinition" && key != "doDDefinition" {
			*fields[key] = ""
			wire.TextRefs = append(wire.TextRefs, runtimeDefinitionTextRef{Field: key, Ref: ref})
		} else {
			first[text] = key
		}
	}
	wire.runtimeDefinitionInline = runtimeDefinitionInline(p)
	if len(wire.TextRefs) > 0 {
		wire.TextRefRule = definitionTextRefRule
	}
	return json.Marshal(wire)
}
func (p *runtimeCandidateDefinitionProjection) UnmarshalJSON(raw []byte) error {
	var wire runtimeDefinitionWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	value := runtimeCandidateDefinitionProjection(wire.runtimeDefinitionInline)
	_, fields := definitionTextFields(&value)
	destinations := map[string]bool{}
	for _, ref := range wire.TextRefs {
		if destinations[ref.Field] {
			return fmt.Errorf("duplicate definition text destination")
		}
		destinations[ref.Field] = true
	}
	if len(wire.TextRefs) > 0 && wire.TextRefRule != definitionTextRefRule {
		return fmt.Errorf("definition text reference rule missing")
	}
	for _, ref := range wire.TextRefs {
		target, ok := fields[ref.Field]
		source, found := fields[ref.Ref]
		if !ok || !found || destinations[ref.Ref] || ref.Field == "goalDefinition" || ref.Field == "doDDefinition" || *target != "" || *source == "" {
			return fmt.Errorf("definition text reference is missing, external, cyclic or mixed")
		}
		*target = *source
	}
	*p = value
	return nil
}
