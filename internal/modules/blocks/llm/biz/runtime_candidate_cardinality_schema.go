package llm

import (
	"encoding/json"
	"regexp"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Parameter names have the exact cardinality enforced by the current wire contract.
func runtimeCandidateCardinalitySchema(manifest runtimecontract.RuntimeCandidateContractManifest, wire runtimecontract.RuntimeCandidateWireContract) string {
	base := runtimeCandidateSchemaForManifest(manifest)
	if manifest.State != runtimecontract.ConvergenceStateNeedStateChange && manifest.State != runtimecontract.ConvergenceStateNeedValidation {
		return base
	}
	var schema map[string]any
	if json.Unmarshal([]byte(base), &schema) != nil {
		return ""
	}
	schema["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	operations := schema["properties"].(map[string]any)["operations"].(map[string]any)
	branches := operations["items"].(map[string]any)["oneOf"].([]any)
	if len(branches) != len(wire.OperationRules) {
		return ""
	}
	for i, rule := range wire.OperationRules {
		properties := branches[i].(map[string]any)["properties"].(map[string]any)
		parameters, found := properties["parameters"].(map[string]any)
		if !found {
			continue
		}
		requirements, _ := parameters["allOf"].([]any)
		for _, cardinality := range rule.ParameterCardinalities() {
			constraint := runtimeParameterContainsSchema(cardinality.Name)
			constraint["minContains"], constraint["maxContains"] = cardinality.Minimum, cardinality.Maximum
			requirements = append(requirements, constraint)
		}
		parameters["allOf"] = requirements
		operation, found := runtimeCandidateOperationManifestForKind(manifest.Operations, rule.Kind)
		if !found {
			return ""
		}
		if !operation.AllowsDynamicParameters {
			parameters["maxItems"] = len(rule.ProviderParameterNames)
		} else {
			// Render the one current pattern directly from sealed prefixes.
			prefixes := make([]string, 0, len(rule.DynamicParameterPrefixes))
			for _, prefix := range rule.DynamicParameterPrefixes {
				prefixes = append(prefixes, regexp.QuoteMeta(prefix))
			}
			if len(prefixes) == 0 {
				return ""
			}
			items := parameters["items"].(map[string]any)["oneOf"].([]any)
			for _, item := range items {
				name := item.(map[string]any)["properties"].(map[string]any)["name"].(map[string]any)
				if _, dynamic := name["pattern"]; dynamic {
					name["pattern"] = "^(?:" + strings.Join(prefixes, "|") + `)[\s\S]*\S[\s\S]*$`
				}
			}
		}
	}
	body, err := json.Marshal(schema)
	if err != nil {
		return ""
	}
	return string(body)
}

func runtimeCandidateContractCatalogCurrent() string {
	return runtimeCandidateCompactContractCatalog() + "\n" +
		"Within each operation parameters array every name must occur at most once, and every required name exactly once. The same name in different operations is allowed. An array of entries is not an edits list. maxContains counts entries by name, not by value; uniqueItems alone cannot establish this rule. Dynamic parameter names also must be unique and authorized by their exact prefixes.\n" +
		"File adapter: file/edit represents exactly one oldText/newText replacement (and optional occurrence), not several replacement pairs. For multiple changes to one file, use one exact contiguous oldText from the current owner content and one newText preserving all intervening unchanged text. Never repeat oldText/newText, drop an intended change, split a required slot, or switch to an unsealed operation. If the current owner input is incomplete, do not invent missing source text."
}
