package llm

import (
	"encoding/json"

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
	items, ok := operations["items"].(map[string]any)
	if !ok {
		return base
	}
	wire, err := runtimecontract.BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		return ""
	}
	if len(manifest.RequiredOperationSequence) != 0 {
		branches := runtimeOperationSchemaBranchesForWire(manifest, wire)
		if len(branches) == 0 {
			return ""
		}
		operations["maxItems"] = manifest.MaxOperationCount
		operations["items"] = map[string]any{"oneOf": branches}
		requirements := make([]any, 0, len(manifest.RequiredOperationSequence))
		for _, requirement := range manifest.RequiredOperationSequence {
			requirements = append(requirements, map[string]any{"contains": map[string]any{
				"type": "object", "properties": map[string]any{
					"requiredOperationRef": map[string]any{"const": requirement.RequirementID},
				}, "required": []string{"requiredOperationRef"},
			}})
		}
		operations["allOf"] = requirements
		encoded, encodeErr := json.Marshal(schema)
		if encodeErr != nil {
			return base
		}
		return string(encoded)
	}
	allowedKinds := make([]string, 0, len(manifest.Operations))
	operations["maxItems"] = manifest.MaxOperationCount
	allowsRequiredOperationRef := false
	for _, operation := range manifest.Operations {
		allowedKinds = append(allowedKinds, operation.Kind)
		if runtimeStringIn(operation.AllowedOperationFields, "requiredOperationRef") {
			allowsRequiredOperationRef = true
		}
	}
	conditions := runtimeOperationSchemaConditionsForManifest(manifest.Operations)
	if itemProperties, ok := items["properties"].(map[string]any); ok {
		itemProperties["kind"] = map[string]any{"type": "string", "enum": allowedKinds}
		if !allowsRequiredOperationRef {
			delete(itemProperties, "requiredOperationRef")
		}
		delete(itemProperties, "coverageWorkItemIds")
		bindingRefs := make([]string, 0)
		for _, operation := range manifest.Operations {
			bindingRefs = append(bindingRefs, runtimeCandidateBindingAuthorizationRefs(operation.BindingAuthorizations)...)
		}
		bindingRefs = runtimeSortedUniqueStrings(bindingRefs)
		if len(bindingRefs) == 0 {
			delete(itemProperties, "bindingRefs")
		} else {
			itemProperties["bindingRefs"] = map[string]any{"type": "array", "minItems": 1, "maxItems": 1, "items": map[string]any{"type": "string", "enum": bindingRefs}}
		}
		items["required"] = runtimeCommonRequiredOperationFields(manifest.Operations)
	}
	if len(conditions) > 0 {
		items["allOf"] = conditions
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return base
	}
	return string(encoded)
}

// runtimeOperationSchemaBranchesForWire renders required-operation slots from
// the same pure wire projection consumed by the strict decoder. A required
// slot exposes only requiredOperationRef plus genuinely provider-selectable
// fields; kind, singleton binding/target and variant discriminators are absent
// from the branch and therefore rejected by additionalProperties=false.
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
	if !rule.RequiredSlot() && len(operation.Variants) != 0 {
		variantSchemas := make([]any, 0, len(operation.Variants))
		for _, variant := range operation.Variants {
			variantSchemas = append(variantSchemas, runtimeOperationVariantParameterSchema(operation.Kind, variant))
		}
		parameterSchema["oneOf"] = variantSchemas
	}
	return parameterSchema
}

func runtimeStringIn(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// runtimeOperationSchemaConditionsForManifest is the production schema
// projection. Its input is already descriptor-derived by
// BuildRuntimeCandidateContractManifest; it must not call the operation
// contract inventory again or maintain a parallel field whitelist.
func runtimeOperationSchemaConditionsForManifest(operations []runtimecontract.RuntimeCandidateOperationManifest) []any {
	ret := make([]any, 0, len(operations))
	for _, operation := range operations {
		operation = operation.Normalized()
		if operation.Kind == "" {
			continue
		}
		parameterTypes := make(map[string]runtimecontract.RuntimeOperationParameterType, len(operation.Parameters))
		parameterRequirements := make([]any, 0, len(operation.Parameters)+1)
		for _, parameter := range operation.Parameters {
			parameterTypes[parameter.Name] = parameter.Type
			if parameter.Required {
				parameterRequirements = append(parameterRequirements, runtimeParameterContainsSchema(parameter.Name))
			}
		}
		if len(operation.RequireOneOfParameters) > 0 {
			oneOf := make([]any, 0, len(operation.RequireOneOfParameters))
			for _, name := range operation.RequireOneOfParameters {
				oneOf = append(oneOf, runtimeNonEmptyParameterContainsSchema(name, parameterTypes[name]))
			}
			parameterRequirements = append(parameterRequirements, map[string]any{"anyOf": oneOf})
		}
		parameterSchema := map[string]any{
			"type": "array", "maxItems": len(operation.Parameters) + runtimeProviderStringListMaxItems,
			"items": runtimeOperationParameterItemSchemaForManifest(operation),
		}
		if len(parameterRequirements) > 0 {
			parameterSchema["allOf"] = parameterRequirements
		}
		if len(operation.Variants) > 0 {
			variantSchemas := make([]any, 0, len(operation.Variants))
			for _, variant := range operation.Variants {
				variantSchemas = append(variantSchemas, runtimeOperationVariantParameterSchema(operation.Kind, variant))
			}
			parameterSchema["oneOf"] = variantSchemas
		}
		thenProperties := map[string]any{"parameters": parameterSchema}
		then := map[string]any{"properties": thenProperties}
		requiredFields := append([]string(nil), operation.RequiredOperationFields...)
		bindingConditions := make([]any, 0, len(operation.BindingAuthorizations))
		for _, authorization := range operation.BindingAuthorizations {
			authorization = authorization.Normalized()
			if len(operation.BindingAuthorizations) == 1 && authorization.RequiredOperationRef == "" {
				if len(authorization.AllowedBindingRefs) != 0 {
					requiredFields = append(requiredFields, "bindingRefs")
				}
				continue
			}
			selectorProperties := make(map[string]any)
			selectorRequired := make([]string, 0, 1)
			if authorization.RequiredOperationRef != "" {
				selectorProperties["requiredOperationRef"] = map[string]any{"const": authorization.RequiredOperationRef}
				selectorRequired = append(selectorRequired, "requiredOperationRef")
			} else if variant, found := runtimeCandidateManifestVariantForID(operation.Variants, authorization.VariantID); found {
				selectorProperties["parameters"] = runtimeOperationVariantParameterSchema(operation.Kind, variant)
				selectorRequired = append(selectorRequired, "parameters")
			} else {
				continue
			}
			condition := map[string]any{
				"if": map[string]any{"properties": selectorProperties, "required": selectorRequired},
			}
			if len(authorization.AllowedBindingRefs) != 0 {
				condition["then"] = map[string]any{
					"properties": map[string]any{"bindingRefs": map[string]any{"type": "array", "minItems": 1, "maxItems": 1, "items": map[string]any{"type": "string", "enum": authorization.AllowedBindingRefs}}},
					"required":   []string{"bindingRefs"},
				}
			} else {
				condition["then"] = map[string]any{"not": map[string]any{"required": []string{"bindingRefs"}}}
			}
			bindingConditions = append(bindingConditions, condition)
		}
		allowedTargets := operation.Target.Normalized().AllowedTargetRefs()
		if operation.RequireTargetRefs || len(allowedTargets) > 0 {
			targetItems := map[string]any{"type": "string"}
			if len(allowedTargets) > 0 {
				targetItems["enum"] = append([]string(nil), allowedTargets...)
			}
			targetSchema := map[string]any{"type": "array", "items": targetItems}
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
			thenProperties["targetRefs"] = targetSchema
		}
		if len(requiredFields) != 0 {
			then["required"] = runtimeSortedUniqueStrings(requiredFields)
		}
		if len(bindingConditions) != 0 {
			then["allOf"] = bindingConditions
		}
		ret = append(ret, map[string]any{
			"if":   map[string]any{"properties": map[string]any{"kind": map[string]any{"const": operation.Kind}}, "required": []string{"kind"}},
			"then": then,
		})
	}
	return ret
}

func runtimeCommonRequiredOperationFields(operations []runtimecontract.RuntimeCandidateOperationManifest) []string {
	if len(operations) == 0 {
		return nil
	}
	common := append([]string(nil), operations[0].Normalized().RequiredOperationFields...)
	for _, operation := range operations[1:] {
		required := operation.Normalized().RequiredOperationFields
		filtered := common[:0]
		for _, field := range common {
			if runtimeStringIn(required, field) {
				filtered = append(filtered, field)
			}
		}
		common = filtered
	}
	return runtimeSortedUniqueStrings(common)
}

func runtimeOperationParameterItemSchemaForManifest(operation runtimecontract.RuntimeCandidateOperationManifest) map[string]any {
	branches := make([]any, 0, len(operation.Parameters)+3)
	for _, parameter := range operation.Parameters {
		if parameter.Name == "recipeId" && len(operation.ValidationRecipes) > 0 {
			branches = append(branches, runtimeValidationRecipeIDParameterSchema(operation.ValidationRecipes))
			continue
		}
		branches = append(branches, runtimeNamedOperationParameterSchema(parameter.Name, parameter.Type))
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

func runtimeNamedOperationParameterSchema(name string, parameterType runtimecontract.RuntimeOperationParameterType) map[string]any {
	valueField, valueSchema := runtimeOperationParameterValueSchema(parameterType)
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"name", valueField},
		"properties": map[string]any{
			"name":     map[string]any{"const": name},
			valueField: valueSchema,
		},
	}
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
