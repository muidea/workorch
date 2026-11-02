package llm

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

// Byte accounting is an observation of the existing request, never a token
// estimate, budget allocation, input selection or raw-content log.
func runtimeProviderSectionMetrics(s models.PromptSection) models.ProviderCacheSectionHash {
	m := models.ProviderCacheSectionHash{BodyBytes: len(s.Content), ExpandedBytes: len(s.Content)}
	switch promptSectionGroup(s.ID) {
	case "C0":
		m.Applicability = "runtime_contract"
	case "C1":
		m.Applicability = "sealed_definition_and_original_sources"
	case "C2":
		m.Applicability = "declarative_capability_contract"
	case "C3":
		m.Applicability = "current_request_authority"
	case "C4":
		m.Applicability = "execution_and_correction_facts"
	case "C5":
		m.Applicability = "acceptance_references"
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(s.Content), &fields) != nil {
		return m
	}
	keys := make([]string, 0, len(fields))
	allowed := runtimeMetricComponentNames(s.ID)
	for key := range fields {
		if allowed[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		m.Components = append(m.Components, models.ProviderContextComponent{Path: key, Bytes: len(fields[key])})
	}
	switch s.ID {
	case "C1.goal-dod":
		var wire runtimeDefinitionWire
		var p runtimeCandidateDefinitionProjection
		if json.Unmarshal([]byte(s.Content), &wire) == nil && json.Unmarshal([]byte(s.Content), &p) == nil {
			raw, _ := json.Marshal(runtimeDefinitionInline(p))
			m.ExpandedBytes = len(raw)
			m.LocalReferences = len(wire.TextRefs)
		}
	case "C3.semantic-authority":
		var wire runtimeSemanticAuthorityWire
		var p runtimeSemanticAuthorityProjection
		if json.Unmarshal([]byte(s.Content), &wire) == nil && json.Unmarshal([]byte(s.Content), &p) == nil {
			raw, _ := json.Marshal(runtimeSemanticAuthorityInline(p))
			m.ExpandedBytes = len(raw)
			m.LocalReferences = len(wire.FileRefs) + len(wire.IntentRefs) + len(wire.GenerationRefs)
		}
	}
	return m
}

// Audit field names come only from the owner's closed projection, never from
// arbitrary JSON keys in business text or another prompt family.
func runtimeMetricComponentNames(section string) map[string]bool {
	ret := map[string]bool{}
	var t reflect.Type
	switch section {
	case "C1.goal-dod":
		t = reflect.TypeFor[runtimeDefinitionWire]()
	case "C3.semantic-authority":
		t = reflect.TypeFor[runtimeSemanticAuthorityWire]()
	case "C4.semantic-tail":
		t = reflect.TypeFor[runtimeSemanticTailProjection]()
	case "C4.execution-tail":
		t = reflect.TypeFor[runtimeCandidateExecutionTailProjection]()
	case "C3.runtime-authority":
		for _, name := range []string{"decision", "transaction", "outputContract", "context"} {
			ret[name] = true
		}
	case "C2.capability-contract":
		for _, name := range []string{"kinds", "capabilityKinds", "executionContract"} {
			ret[name] = true
		}
	}
	if t != nil {
		for _, field := range reflect.VisibleFields(t) {
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" && !field.Anonymous {
				ret[name] = true
			}
		}
	}
	return ret
}
