package llm

import (
	"encoding/json"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	runtimeProviderStringMaxLength    = 65536
	runtimeProviderStringListMaxItems = 256
	runtimeProviderAssessmentMaxItems = 256
)

func runtimeAssessmentCandidateSchema() string {
	stringSchema := func() map[string]any {
		return map[string]any{"type": "string", "maxLength": runtimeProviderStringMaxLength}
	}
	stringArray := func(minimum int) map[string]any {
		value := map[string]any{"type": "array", "maxItems": runtimeProviderAssessmentMaxItems, "items": stringSchema()}
		if minimum > 0 {
			value["minItems"] = minimum
		}
		return value
	}
	value := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"focusId", "coverageWorkItemIds", "disposition", "summary"},
		"properties": map[string]any{
			"focusId": stringSchema(), "sliceId": stringSchema(), "coverageWorkItemIds": stringArray(1),
			"disposition": map[string]any{"type": "string", "enum": []string{"satisfied", "repairable_gap", "governance_required", "terminal_rejection"}},
			"items": map[string]any{"type": "array", "maxItems": runtimeProviderAssessmentMaxItems, "items": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"coverageWorkItemId", "satisfied"},
				"properties": map[string]any{"coverageWorkItemId": stringSchema(), "satisfied": map[string]any{"type": "boolean"}, "reason": stringSchema(), "evidenceRefs": stringArray(0)},
			}},
			"gap": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
				"coverageWorkItemIds": stringArray(0), "missingEvidenceKinds": stringArray(0), "targetRefs": stringArray(0), "evidenceRefs": stringArray(0),
				"reason": stringSchema(), "failureClass": stringSchema(), "failureCode": stringSchema(), "failureSignature": stringSchema(),
			}},
			"summary": stringSchema(), "evidenceRefs": stringArray(0), "artifactRefs": stringArray(0),
		},
		"allOf": []any{map[string]any{
			"if":   map[string]any{"properties": map[string]any{"disposition": map[string]any{"const": "satisfied"}}, "required": []string{"disposition"}},
			"then": map[string]any{"required": []string{"evidenceRefs"}, "properties": map[string]any{"evidenceRefs": stringArray(1)}},
		}},
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func runtimeGovernanceCandidateSchema() string {
	stringSchema := map[string]any{"type": "string", "maxLength": runtimeProviderStringMaxLength}
	value := map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"externalDependency"},
		"properties": map[string]any{"externalDependency": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"kind", "reason"},
			"properties": map[string]any{"kind": stringSchema, "subjectRef": stringSchema, "reason": stringSchema},
		}},
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func runtimeCandidateSchema(state runtimecontract.ConvergenceState) string {
	switch state {
	case runtimecontract.ConvergenceStateNeedStateChange:
		return runtimeOperationCandidateSchema("workorch.runtime_candidate.state_change", "Runtime durable state-change candidate", runtimecontract.RuntimeConvergenceEffectInstruction(state))
	case runtimecontract.ConvergenceStateNeedValidation:
		return runtimeOperationCandidateSchema("workorch.runtime_candidate.validation", "Runtime non-durable validation candidate", runtimecontract.RuntimeConvergenceEffectInstruction(state))
	case runtimecontract.ConvergenceStateNeedAssessment:
		return runtimeAssessmentCandidateSchema()
	case runtimecontract.ConvergenceStateNeedGovernance:
		return runtimeGovernanceCandidateSchema()
	default:
		return ""
	}
}

// runtimeCandidateSchemaForManifest renders the volatile operation schema
// exclusively from the pure candidate manifest. It is the production path;
// descriptor/parameter/dependency rules must not be reconstructed here.
func runtimeCandidateSchemaForManifest(manifest runtimecontract.RuntimeCandidateContractManifest) string {
	manifest = manifest.Normalized()
	if manifest.EffectPolicy.State != manifest.State || manifest.EffectPolicy.Validate() != nil {
		return ""
	}
	if runtimecontract.ValidateRuntimeCandidateManifestOperationCardinality(manifest) != nil {
		return ""
	}
	base := runtimeCandidateSchema(manifest.State)
	if manifest.State != runtimecontract.ConvergenceStateNeedStateChange && manifest.State != runtimecontract.ConvergenceStateNeedValidation {
		return base
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(base), &schema); err != nil {
		return base
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return base
	}
	operations, ok := properties["operations"].(map[string]any)
	if !ok {
		return base
	}
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		return ""
	}
	branches := runtimeOperationSchemaBranchesForWire(manifest, wire)
	if len(branches) == 0 {
		return ""
	}
	operations["maxItems"] = manifest.MaxOperationCount
	operations["items"] = map[string]any{"oneOf": branches}
	if len(manifest.RequiredOperationSequence) != 0 {
		requirements := make([]any, 0, len(manifest.RequiredOperationSequence))
		for _, requirement := range manifest.RequiredOperationSequence {
			requirements = append(requirements, map[string]any{"contains": map[string]any{
				"type": "object", "properties": map[string]any{
					"requiredOperationRef": map[string]any{"const": requirement.RequirementID},
				}, "required": []string{"requiredOperationRef"},
			}})
		}
		operations["allOf"] = requirements
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return base
	}
	return string(encoded)
}

// runtimeOperationSchemaBranchesForWire renders every provider operation from
// the same pure wire projection consumed by the strict decoder. Required slots
// and auxiliary kinds both expose only genuinely provider-selectable fields;
// locally compiled bindings, targets and variant discriminators are rejected
// by additionalProperties=false.
func runtimeOperationSchemaBranchesForWire(manifest runtimecontract.RuntimeCandidateContractManifest, wire runtimecontract.RuntimeCandidateWireContract) []any {
	ret := make([]any, 0, len(wire.OperationRules))
	for _, rule := range wire.OperationRules {
		operation, found := runtimeCandidateOperationManifestForKind(manifest.Operations, rule.Kind)
		if !found {
			return nil
		}
		properties := make(map[string]any)
		if runtimeStringIn(rule.ProviderFields, "requiredOperationRef") {
			properties["requiredOperationRef"] = map[string]any{"const": rule.RequiredOperationRef}
		}
		if runtimeStringIn(rule.ProviderFields, "kind") {
			properties["kind"] = map[string]any{"const": rule.Kind}
		}
		if runtimeStringIn(rule.ProviderFields, "bindingRefs") {
			properties["bindingRefs"] = map[string]any{
				"type": "array", "minItems": 1, "maxItems": 1,
				"items": map[string]any{"type": "string", "enum": append([]string(nil), rule.AllowedBindingRefs...)},
			}
		}
		if runtimeStringIn(rule.ProviderFields, "targetRefs") {
			properties["targetRefs"] = runtimeOperationTargetRefsSchema(operation)
		}
		if runtimeStringIn(rule.ProviderFields, "parameters") {
			properties["parameters"] = runtimeOperationParameterSchemaForWireRule(operation, rule)
		}
		branch := map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           properties,
			"required":             append([]string(nil), rule.RequiredProviderFields...),
		}
		if conditions := runtimeOperationBindingSchemaConditionsForWireRule(operation, rule); len(conditions) != 0 {
			branch["allOf"] = conditions
		}
		ret = append(ret, branch)
	}
	return ret
}

func runtimeCandidateOperationManifestForKind(values []runtimecontract.RuntimeCandidateOperationManifest, kind string) (runtimecontract.RuntimeCandidateOperationManifest, bool) {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, operation := range values {
		operation = operation.Normalized()
		if operation.Kind == kind {
			return operation, true
		}
	}
	return runtimecontract.RuntimeCandidateOperationManifest{}, false
}

func runtimeOperationTargetRefsSchema(operation runtimecontract.RuntimeCandidateOperationManifest) map[string]any {
	allowedTargets := operation.Target.Normalized().AllowedTargetRefs()
	targetItems := map[string]any{"type": "string", "maxLength": runtimeProviderStringMaxLength}
	if len(allowedTargets) > 0 {
		targetItems["enum"] = append([]string(nil), allowedTargets...)
	}
	targetSchema := map[string]any{"type": "array", "items": targetItems, "maxItems": runtimecontract.RuntimeCandidateContextItemsMaxCount}
	if len(allowedTargets) > 0 {
		targetSchema["maxItems"] = len(allowedTargets)
	}
	switch operation.TargetCardinality {
	case runtimecontract.RuntimeOperationTargetCardinalityOne:
		targetSchema["minItems"] = 1
		targetSchema["maxItems"] = 1
	case runtimecontract.RuntimeOperationTargetCardinalityMany:
		if operation.RequireTargetRefs {
			targetSchema["minItems"] = 1
		}
	}
	return targetSchema
}

func runtimeOperationParameterSchemaForWireRule(operation runtimecontract.RuntimeCandidateOperationManifest, rule runtimecontract.RuntimeCandidateWireOperationRule) map[string]any {
	providerParameters := make([]runtimecontract.RuntimeOperationParameterContract, 0, len(operation.Parameters))
	parameterTypes := make(map[string]runtimecontract.RuntimeOperationParameterType, len(operation.Parameters))
	for _, parameter := range operation.Parameters {
		parameterTypes[parameter.Name] = parameter.Type
		if runtimeStringIn(rule.ProviderParameterNames, parameter.Name) {
			providerParameters = append(providerParameters, parameter)
		}
	}
	providerOperation := operation
	providerOperation.Parameters = providerParameters
	providerOperation.ValueRules = runtimeOperationValueRulesForSchema(operation, rule)
	parameterSchema := map[string]any{
		"type": "array", "maxItems": len(providerParameters) + runtimeProviderStringListMaxItems,
		"items": runtimeOperationParameterItemSchemaForManifest(providerOperation),
	}
	requirements := make([]any, 0, len(rule.RequiredProviderParameterNames)+len(rule.RequiredNonEmptyParameterNames)+1)
	for _, name := range rule.RequiredProviderParameterNames {
		requirements = append(requirements, runtimeParameterContainsSchema(name))
	}
	for _, name := range rule.RequiredNonEmptyParameterNames {
		requirements = append(requirements, runtimeNonEmptyParameterContainsSchema(name, parameterTypes[name]))
	}
	if len(rule.RequireOneOfProviderParameters) != 0 {
		anyOf := make([]any, 0, len(rule.RequireOneOfProviderParameters))
		for _, name := range rule.RequireOneOfProviderParameters {
			anyOf = append(anyOf, runtimeNonEmptyParameterContainsSchema(name, parameterTypes[name]))
		}
		requirements = append(requirements, map[string]any{"anyOf": anyOf})
	}
	if len(requirements) != 0 {
		parameterSchema["allOf"] = requirements
	}
	if !rule.RequiredSlot() && rule.VariantID == "" && len(operation.Variants) != 0 {
		variantSchemas := make([]any, 0, len(operation.Variants))
		for _, variant := range operation.Variants {
			variantSchemas = append(variantSchemas, runtimeOperationVariantParameterSchema(operation.Kind, variant))
		}
		parameterSchema["oneOf"] = variantSchemas
	}
	return parameterSchema
}

func runtimeOperationBindingSchemaConditionsForWireRule(operation runtimecontract.RuntimeCandidateOperationManifest, rule runtimecontract.RuntimeCandidateWireOperationRule) []any {
	if rule.RequiredSlot() || rule.VariantID != "" || !runtimeStringIn(rule.ProviderFields, "bindingRefs") || len(operation.Variants) < 2 {
		return nil
	}
	ret := make([]any, 0, len(operation.Variants))
	for _, variant := range operation.Variants {
		authorization, found := runtimeCandidateBindingAuthorizationForSchema(operation.BindingAuthorizations, variant.ID)
		if !found {
			return nil
		}
		condition := map[string]any{
			"if": map[string]any{
				"properties": map[string]any{"parameters": runtimeOperationVariantParameterSchema(operation.Kind, variant)},
				"required":   []string{"parameters"},
			},
		}
		if len(authorization.AllowedBindingRefs) == 0 {
			condition["then"] = map[string]any{"not": map[string]any{"required": []string{"bindingRefs"}}}
		} else {
			condition["then"] = map[string]any{
				"properties": map[string]any{"bindingRefs": map[string]any{
					"type": "array", "minItems": 1, "maxItems": 1,
					"items": map[string]any{"type": "string", "enum": append([]string(nil), authorization.AllowedBindingRefs...)},
				}},
				"required": []string{"bindingRefs"},
			}
		}
		ret = append(ret, condition)
	}
	return ret
}

func runtimeCandidateBindingAuthorizationForSchema(values []runtimecontract.RuntimeOperationBindingAuthorization, variantID string) (runtimecontract.RuntimeOperationBindingAuthorization, bool) {
	for _, authorization := range values {
		authorization = authorization.Normalized()
		if authorization.RequiredOperationRef == "" && authorization.VariantID == variantID {
			return authorization, true
		}
	}
	return runtimecontract.RuntimeOperationBindingAuthorization{}, false
}

func runtimeStringIn(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func runtimeOperationParameterItemSchemaForManifest(operation runtimecontract.RuntimeCandidateOperationManifest) map[string]any {
	branches := make([]any, 0, len(operation.Parameters)+3)
	for _, parameter := range operation.Parameters {
		if parameter.Name == "recipeId" && len(operation.ValidationRecipes) > 0 {
			branches = append(branches, runtimeValidationRecipeIDParameterSchema(operation.ValidationRecipes))
			continue
		}
		branches = append(branches, runtimeNamedOperationParameterSchema(parameter.Name, parameter.Type, operation.ValueRules))
	}
	if operation.AllowsDynamicParameters {
		branches = append(branches, runtimeMCPDynamicParameterSchemas(operation.Kind)...)
	}
	return map[string]any{"type": "object", "oneOf": branches}
}

func runtimeValidationRecipeIDParameterSchema(recipes []runtimecontract.RuntimeValidationRecipe) map[string]any {
	recipes = runtimecontract.NormalizeRuntimeValidationRecipes(recipes)
	ids := make([]string, 0, len(recipes))
	for _, recipe := range recipes {
		ids = append(ids, recipe.ID)
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"name", "string"},
		"properties": map[string]any{
			"name":   map[string]any{"const": "recipeId"},
			"string": map[string]any{"type": "string", "enum": ids},
		},
	}
}

func runtimeNamedOperationParameterSchema(name string, parameterType runtimecontract.RuntimeOperationParameterType, rules []runtimecontract.RuntimeOperationValueRule) map[string]any {
	valueField, valueSchema := runtimeOperationParameterValueSchema(parameterType)
	for _, rule := range runtimecontract.NormalizeRuntimeOperationValueRules(rules) {
		if len(rule.ParameterNames) != 1 || rule.ParameterNames[0] != name {
			continue
		}
		switch rule.Kind {
		case runtimecontract.RuntimeOperationValueRuleNonEmpty:
			if parameterType == runtimecontract.RuntimeOperationParameterTypeString {
				valueSchema["minLength"] = 1
			} else if parameterType == runtimecontract.RuntimeOperationParameterTypeStringList {
				valueSchema["minItems"] = 1
			}
		case runtimecontract.RuntimeOperationValueRuleIntegerRange:
			if rule.HasMinimum {
				valueSchema["minimum"] = rule.Minimum
			}
			if rule.HasMaximum {
				valueSchema["maximum"] = rule.Maximum
			}
		case runtimecontract.RuntimeOperationValueRuleStringByteRange:
			if rule.HasMinimum {
				valueSchema["minLength"] = rule.Minimum
			}
			if rule.HasMaximum {
				valueSchema["maxLength"] = rule.Maximum
			}
		case runtimecontract.RuntimeOperationValueRuleBooleanEquals:
			valueSchema["const"] = rule.ExpectedBoolean == "true"
		}
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"name", valueField},
		"properties": map[string]any{
			"name":     map[string]any{"const": name},
			valueField: valueSchema,
		},
	}
}

func runtimeOperationValueRulesForSchema(operation runtimecontract.RuntimeCandidateOperationManifest, rule runtimecontract.RuntimeCandidateWireOperationRule) []runtimecontract.RuntimeOperationValueRule {
	rules := append([]runtimecontract.RuntimeOperationValueRule(nil), operation.ValueRules...)
	for _, variant := range operation.Variants {
		if rule.VariantID == "" || strings.TrimSpace(rule.VariantID) == strings.TrimSpace(variant.ID) {
			rules = append(rules, variant.ValueRules...)
		}
	}
	return runtimecontract.NormalizeRuntimeOperationValueRules(rules)
}

func runtimeOperationParameterValueSchema(parameterType runtimecontract.RuntimeOperationParameterType) (string, map[string]any) {
	switch parameterType {
	case runtimecontract.RuntimeOperationParameterTypeInteger:
		return "integer", map[string]any{"type": "integer"}
	case runtimecontract.RuntimeOperationParameterTypeBoolean:
		return "boolean", map[string]any{"type": "boolean"}
	case runtimecontract.RuntimeOperationParameterTypeStringList:
		return "stringList", map[string]any{"type": "array", "minItems": 1, "maxItems": runtimeProviderStringListMaxItems, "items": map[string]any{"type": "string", "maxLength": runtimeProviderStringMaxLength}}
	default:
		return "string", map[string]any{"type": "string", "maxLength": runtimeProviderStringMaxLength}
	}
}

func runtimeMCPDynamicParameterSchemas(kind string) []any {
	prefix := ""
	switch runtimecontract.CanonicalRuntimeBlockKind(kind) {
	case runtimecontract.StepKindMCPToolCall:
		prefix = "input"
	case runtimecontract.StepKindMCPPromptGet:
		prefix = "arguments"
	default:
		return nil
	}
	// The shared operation contract permits only scalar dynamic MCP values. The
	// suffix needs at least one non-space rune, matching the contract helper's
	// trimmed non-empty suffix check without inventing tool-specific fields.
	nameSchema := map[string]any{"type": "string", "pattern": "^" + prefix + `\\..*\\S.*$`}
	ret := make([]any, 0, 3)
	for _, value := range []struct {
		field  string
		schema map[string]any
	}{
		{field: "string", schema: map[string]any{"type": "string", "maxLength": runtimeProviderStringMaxLength}},
		{field: "integer", schema: map[string]any{"type": "integer"}},
		{field: "boolean", schema: map[string]any{"type": "boolean"}},
	} {
		ret = append(ret, map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"name", value.field},
			"properties": map[string]any{
				"name": nameSchema, value.field: value.schema,
			},
		})
	}
	return ret
}

func runtimeParameterContainsSchema(name string) map[string]any {
	return map[string]any{
		"contains": map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"const": name}}, "required": []string{"name"}},
	}
}

// runtimeNonEmptyParameterContainsSchema keeps a descriptor's one-of
// parameter requirement aligned with the operation DTO: a selected canonical
// target cannot be an empty string. It only renders a provider schema and
// never discovers targets or changes Runtime state.
func runtimeNonEmptyParameterContainsSchema(name string, parameterType runtimecontract.RuntimeOperationParameterType) map[string]any {
	properties := map[string]any{"name": map[string]any{"const": name}}
	required := []string{"name"}
	switch parameterType {
	case runtimecontract.RuntimeOperationParameterTypeString:
		properties["string"] = map[string]any{"type": "string", "minLength": 1}
		required = append(required, "string")
	case runtimecontract.RuntimeOperationParameterTypeStringList:
		properties["stringList"] = map[string]any{"type": "array", "minItems": 1}
		required = append(required, "stringList")
	case runtimecontract.RuntimeOperationParameterTypeInteger:
		properties["integer"] = map[string]any{"type": "integer"}
		required = append(required, "integer")
	case runtimecontract.RuntimeOperationParameterTypeBoolean:
		properties["boolean"] = map[string]any{"type": "boolean"}
		required = append(required, "boolean")
	default:
		return runtimeParameterContainsSchema(name)
	}
	return map[string]any{"contains": map[string]any{"type": "object", "properties": properties, "required": required}}
}

func runtimeOperationVariantParameterSchema(kind string, variant runtimecontract.RuntimeOperationVariantContract) map[string]any {
	conditions := make([]any, 0, len(variant.Discriminators)+len(variant.RequiredParameters)+len(variant.RequiredNonEmpty)+len(variant.ForbiddenParameters))
	for _, discriminator := range variant.Discriminators {
		conditions = append(conditions, map[string]any{
			"contains": map[string]any{
				"type": "object", "properties": map[string]any{
					"name":   map[string]any{"const": discriminator.Name},
					"string": map[string]any{"const": discriminator.EqualsString},
				},
				"required": []string{"name", "string"},
			},
		})
	}
	for _, name := range variant.RequiredParameters {
		conditions = append(conditions, runtimeParameterContainsSchema(name))
	}
	for _, name := range variant.RequiredNonEmpty {
		conditions = append(conditions, map[string]any{
			"contains": map[string]any{
				"type": "object", "allOf": []any{
					map[string]any{"properties": map[string]any{"name": map[string]any{"const": name}}, "required": []string{"name"}},
					map[string]any{"anyOf": []any{
						map[string]any{"properties": map[string]any{"string": map[string]any{"type": "string", "minLength": 1}}, "required": []string{"string"}},
						map[string]any{"properties": map[string]any{"stringList": map[string]any{"type": "array", "minItems": 1}}, "required": []string{"stringList"}},
					}},
				},
			},
		})
	}
	for _, name := range variant.ForbiddenParameters {
		conditions = append(conditions, map[string]any{
			"not": map[string]any{"contains": map[string]any{
				"type": "object", "properties": map[string]any{"name": map[string]any{"const": name}}, "required": []string{"name"},
			}},
		})
	}
	return map[string]any{
		"title": "operation variant contract: " + runtimecontract.RuntimeOperationVariantRuleID(kind, variant.ID, "contract", ""),
		"allOf": conditions,
	}
}

func runtimeOperationCandidateSchema(id, title, description string) string {
	value := map[string]any{
		"$id": id, "title": title, "description": description,
		"type": "object", "additionalProperties": false, "required": []string{"operations"},
		"properties": map[string]any{
			"operations": map[string]any{
				"type": "array", "minItems": 1, "maxItems": runtimecontract.RuntimeCandidateContextItemsMaxCount,
				"items": map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{"kind"},
					"properties": map[string]any{
						"kind":                 map[string]any{"type": "string", "maxLength": runtimeProviderStringMaxLength},
						"requiredOperationRef": map[string]any{"type": "string", "maxLength": runtimeProviderStringMaxLength},
						"bindingRefs":          map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string"}},
						"targetRefs":           map[string]any{"type": "array", "maxItems": runtimecontract.RuntimeCandidateContextItemsMaxCount, "items": map[string]any{"type": "string", "maxLength": runtimeProviderStringMaxLength}},
						"parameters": map[string]any{"type": "array", "items": map[string]any{
							"type": "object", "additionalProperties": false, "required": []string{"name"},
							"properties": map[string]any{
								"name": map[string]any{"type": "string", "maxLength": runtimeProviderStringMaxLength}, "string": map[string]any{"type": "string", "maxLength": runtimeProviderStringMaxLength},
								"integer": map[string]any{"type": "integer"}, "boolean": map[string]any{"type": "boolean"},
								"stringList": map[string]any{"type": "array", "maxItems": runtimeProviderStringListMaxItems, "items": map[string]any{"type": "string", "maxLength": runtimeProviderStringMaxLength}},
							},
						}},
					},
				},
			},
		},
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
