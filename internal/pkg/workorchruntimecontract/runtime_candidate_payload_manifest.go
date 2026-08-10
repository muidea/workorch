package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// RuntimeCandidatePayloadDiagnostic is a pure, machine-actionable description
// of a provider payload rejected at the strict DTO boundary. It carries no
// aggregate state and does not choose retry/action; SM-3 classifies it after
// the LLM owner returns the error.
type RuntimeCandidatePayloadDiagnostic struct {
	Path               string
	Actual             string
	Allowed            []string
	Required           []string
	Forbidden          []string
	SemanticViolations []RuntimeSemanticContractViolation
	Cause              error
}

func (d *RuntimeCandidatePayloadDiagnostic) Error() string {
	if d == nil {
		return "runtime candidate payload is invalid"
	}
	parts := []string{"runtime candidate payload diagnostic"}
	if d.Path != "" {
		parts = append(parts, "path="+d.Path)
	}
	if d.Actual != "" {
		parts = append(parts, "actual="+d.Actual)
	}
	if len(d.Allowed) > 0 {
		parts = append(parts, "allowed="+strings.Join(d.Allowed, ","))
	}
	if len(d.Required) > 0 {
		parts = append(parts, "required="+strings.Join(d.Required, ","))
	}
	if len(d.Forbidden) > 0 {
		parts = append(parts, "forbidden="+strings.Join(d.Forbidden, ","))
	}
	if d.Cause != nil {
		parts = append(parts, "cause="+d.Cause.Error())
	}
	return strings.Join(parts, "; ")
}

func (d *RuntimeCandidatePayloadDiagnostic) Unwrap() error {
	if d == nil {
		return nil
	}
	return d.Cause
}

// RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic deliberately drops
// Cause. Cause may contain provider/private decoder text; the remaining fields
// are the sealed-manifest facts a correction renderer may safely expose.
func RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic(value *RuntimeCandidatePayloadDiagnostic) *RuntimeCandidateFailureDiagnostic {
	if value == nil {
		return nil
	}
	return (&RuntimeCandidateFailureDiagnostic{
		Path:      value.Path,
		Actual:    value.Actual,
		Allowed:   append([]string(nil), value.Allowed...),
		Required:  append([]string(nil), value.Required...),
		Forbidden: append([]string(nil), value.Forbidden...),
	}).Clone()
}

// DecodeRuntimeCandidatePayloadWithManifest keeps strict JSON decoding while
// deriving dynamic operation diagnostics from one sealed manifest. The helper
// is deliberately pure: it neither reads owner facts nor mutates attempts.
func DecodeRuntimeCandidatePayloadWithManifest(manifest RuntimeCandidateContractManifest, payload []byte) (RuntimeCandidate, error) {
	manifest = manifest.Normalized()
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" ||
		manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$", Actual: "unsealed_manifest", Cause: fmt.Errorf("runtime candidate contract manifest is invalid")}
	}
	if manifest.EffectPolicy.State != manifest.State {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.effectPolicy.state", Actual: string(manifest.EffectPolicy.State), Allowed: []string{string(manifest.State)}, Cause: fmt.Errorf("runtime candidate effect policy state does not match manifest")}
	}
	if err := manifest.EffectPolicy.Validate(); err != nil {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.effectPolicy", Actual: "invalid", Cause: err}
	}
	candidate, err := DecodeRuntimeCandidatePayload(manifest.State, payload)
	if err != nil {
		return RuntimeCandidate{}, runtimeCandidatePayloadDecodeDiagnostic(manifest, payload, err)
	}
	if err := validateRuntimeCandidatePayloadManifest(manifest, payload, candidate); err != nil {
		return RuntimeCandidate{}, err
	}
	return candidate, nil
}

func runtimeCandidatePayloadDecodeDiagnostic(manifest RuntimeCandidateContractManifest, payload []byte, cause error) error {
	path, allowed := "$", manifest.AllowedTopLevelFields
	field := runtimeCandidatePayloadUnknownField(cause)
	if field != "" {
		path, allowed = runtimeCandidatePayloadUnknownFieldPath(manifest, payload, field)
	}
	actual := firstRuntimeCandidatePayloadActual(field, cause)
	var typeError *json.UnmarshalTypeError
	if field == "" && errors.As(cause, &typeError) {
		path = runtimeCandidatePayloadTypeErrorPath(payload, typeError.Field)
		actual = strings.TrimSpace(typeError.Value)
		if expected := runtimeCandidatePayloadExpectedJSONType(typeError.Type); expected != "" {
			allowed = []string{expected}
		}
	}
	return &RuntimeCandidatePayloadDiagnostic{Path: path, Actual: actual, Allowed: allowed, Forbidden: manifest.ForbiddenTopLevelFields, Cause: cause}
}

// runtimeCandidatePayloadTypeErrorPath converts encoding/json's Go-struct
// field path to a stable JSON path without attempting to infer any runtime
// authority. For repeated operations we locate the first raw element that
// contains the rejected field; this is only diagnostic metadata and cannot
// affect candidate admission.
func runtimeCandidatePayloadTypeErrorPath(payload []byte, field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return "$"
	}
	parts := strings.Split(field, ".")
	if parts[0] != "operations" {
		return "$." + strings.Join(parts, ".")
	}
	path := "$.operations"
	if len(parts) == 1 {
		return path
	}
	operationIndex, parameterIndex := runtimeCandidatePayloadTypeErrorIndices(payload, parts[1:])
	if operationIndex >= 0 {
		path += fmt.Sprintf("[%d]", operationIndex)
	} else {
		path += "[]"
	}
	if len(parts) == 1 {
		return path
	}
	if parts[1] == "parameters" && len(parts) > 2 {
		if parameterIndex >= 0 {
			path += fmt.Sprintf(".parameters[%d]", parameterIndex)
		} else {
			path += ".parameters[]"
		}
		if len(parts) > 2 {
			path += "." + strings.Join(parts[2:], ".")
		}
		return path
	}
	return path + "." + strings.Join(parts[1:], ".")
}

func runtimeCandidatePayloadTypeErrorIndices(payload []byte, parts []string) (int, int) {
	if len(parts) == 0 {
		return -1, -1
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(payload, &envelope) != nil {
		return -1, -1
	}
	var operations []json.RawMessage
	if json.Unmarshal(envelope["operations"], &operations) != nil {
		return -1, -1
	}
	for operationIndex, rawOperation := range operations {
		var operation map[string]json.RawMessage
		if json.Unmarshal(rawOperation, &operation) != nil {
			continue
		}
		if parts[0] != "parameters" {
			if _, found := operation[parts[0]]; found {
				return operationIndex, -1
			}
			continue
		}
		var parameters []json.RawMessage
		if json.Unmarshal(operation["parameters"], &parameters) != nil {
			if _, found := operation["parameters"]; found {
				return operationIndex, -1
			}
			continue
		}
		if len(parts) == 1 {
			return operationIndex, -1
		}
		for parameterIndex, rawParameter := range parameters {
			var parameter map[string]json.RawMessage
			if json.Unmarshal(rawParameter, &parameter) != nil {
				continue
			}
			if _, found := parameter[parts[1]]; found {
				return operationIndex, parameterIndex
			}
		}
	}
	return -1, -1
}

func runtimeCandidatePayloadExpectedJSONType(value reflect.Type) string {
	if value == nil {
		return ""
	}
	for value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Struct, reflect.Map:
		return "object"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	default:
		return ""
	}
}

func runtimeCandidatePayloadUnknownField(cause error) string {
	if cause == nil {
		return ""
	}
	message := cause.Error()
	const prefix = "json: unknown field \""
	start := strings.Index(message, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := strings.Index(message[start:], "\"")
	if end < 0 {
		return ""
	}
	return message[start : start+end]
}

func firstRuntimeCandidatePayloadActual(field string, cause error) string {
	if field != "" {
		return field
	}
	if cause == nil {
		return ""
	}
	return cause.Error()
}

func runtimeCandidatePayloadUnknownFieldPath(manifest RuntimeCandidateContractManifest, payload []byte, field string) (string, []string) {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(payload, &envelope) != nil {
		return "$." + field, manifest.AllowedTopLevelFields
	}
	if _, ok := envelope[field]; ok {
		return "$." + field, manifest.AllowedTopLevelFields
	}
	operationsRaw, ok := envelope["operations"]
	if !ok {
		return "$." + field, manifest.AllowedTopLevelFields
	}
	var operations []map[string]json.RawMessage
	if json.Unmarshal(operationsRaw, &operations) != nil {
		return "$.operations", nil
	}
	for index, operation := range operations {
		kind := runtimeCandidatePayloadRawString(operation["kind"])
		if operationManifest, found := runtimeCandidatePayloadOperationManifest(manifest, kind); found {
			if _, exists := operation[field]; exists {
				return fmt.Sprintf("$.operations[%d].%s", index, field), operationManifest.AllowedOperationFields
			}
		}
		parametersRaw, exists := operation["parameters"]
		if !exists {
			continue
		}
		var parameters []map[string]json.RawMessage
		if json.Unmarshal(parametersRaw, &parameters) != nil {
			continue
		}
		for parameterIndex, parameter := range parameters {
			if _, exists := parameter[field]; exists {
				return fmt.Sprintf("$.operations[%d].parameters[%d].%s", index, parameterIndex, field), []string{"name", "string", "integer", "boolean", "stringList"}
			}
		}
	}
	return "$." + field, manifest.AllowedTopLevelFields
}

func validateRuntimeCandidatePayloadManifest(manifest RuntimeCandidateContractManifest, payload []byte, candidate RuntimeCandidate) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return &RuntimeCandidatePayloadDiagnostic{Path: "$", Actual: "invalid_json", Cause: err}
	}
	for _, field := range manifest.RequiredTopLevelFields {
		if _, found := envelope[field]; !found {
			return &RuntimeCandidatePayloadDiagnostic{Path: "$", Actual: "missing_" + field, Required: manifest.RequiredTopLevelFields, Allowed: manifest.AllowedTopLevelFields}
		}
	}
	for field := range envelope {
		if !runtimeCandidatePayloadContains(manifest.AllowedTopLevelFields, field) {
			return &RuntimeCandidatePayloadDiagnostic{Path: "$." + field, Actual: field, Allowed: manifest.AllowedTopLevelFields, Forbidden: manifest.ForbiddenTopLevelFields}
		}
	}
	if manifest.State != ConvergenceStateNeedStateChange && manifest.State != ConvergenceStateNeedValidation {
		return nil
	}
	operations, err := runtimeCandidatePayloadOperations(candidate)
	if err != nil {
		return err
	}
	var rawOperations []map[string]json.RawMessage
	if err := json.Unmarshal(envelope["operations"], &rawOperations); err != nil {
		return &RuntimeCandidatePayloadDiagnostic{Path: "$.operations", Actual: "not_array", Cause: err}
	}
	if len(rawOperations) != len(operations) {
		return &RuntimeCandidatePayloadDiagnostic{Path: "$.operations", Actual: "decode_length_mismatch", Cause: fmt.Errorf("decoded operations do not match payload")}
	}
	for index, operation := range operations {
		if err := validateRuntimeCandidatePayloadOperation(manifest, rawOperations[index], operation, index); err != nil {
			return err
		}
	}
	return nil
}

func runtimeCandidatePayloadOperations(candidate RuntimeCandidate) ([]RuntimeOperation, error) {
	if candidate.StateChange != nil {
		ret := make([]RuntimeOperation, 0, len(candidate.StateChange.Operations))
		for _, operation := range candidate.StateChange.Operations {
			ret = append(ret, operation.Operation)
		}
		return ret, nil
	}
	if candidate.Validation != nil {
		ret := make([]RuntimeOperation, 0, len(candidate.Validation.Operations))
		for _, operation := range candidate.Validation.Operations {
			ret = append(ret, operation.Operation)
		}
		return ret, nil
	}
	return nil, &RuntimeCandidatePayloadDiagnostic{Path: "$.operations", Actual: "missing_operations", Required: []string{"operations"}}
}

func validateRuntimeCandidatePayloadOperation(manifest RuntimeCandidateContractManifest, raw map[string]json.RawMessage, operation RuntimeOperation, index int) error {
	path := fmt.Sprintf("$.operations[%d]", index)
	operationManifest, found := runtimeCandidatePayloadOperationManifest(manifest, operation.Kind)
	if !found {
		return &RuntimeCandidatePayloadDiagnostic{Path: path + ".kind", Actual: operation.Kind, Allowed: runtimeCandidatePayloadOperationKinds(manifest)}
	}
	for field := range raw {
		if !runtimeCandidatePayloadContains(operationManifest.AllowedOperationFields, field) {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + "." + field, Actual: field, Allowed: operationManifest.AllowedOperationFields}
		}
	}
	for _, required := range []string{"id", "kind"} {
		if strings.TrimSpace(runtimeCandidatePayloadRawString(raw[required])) == "" {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + "." + required, Actual: "missing_" + required, Required: []string{required}, Allowed: operationManifest.AllowedOperationFields}
		}
	}
	for _, bindingRef := range operation.BindingRefs {
		if !runtimeCandidatePayloadContains(operationManifest.AllowedBindingRefs, bindingRef) {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".bindingRefs", Actual: bindingRef, Allowed: operationManifest.AllowedBindingRefs}
		}
	}
	if operationManifest.RequireTargetRefs && len(operation.TargetRefs) == 0 {
		return &RuntimeCandidatePayloadDiagnostic{Path: path + ".targetRefs", Actual: "missing_targetRefs", Required: []string{"targetRefs"}, Allowed: operationManifest.AllowedOperationFields}
	}
	switch operationManifest.Target.Normalized().Policy {
	case RuntimeOperationTargetPolicyExactTarget, RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if allowedTargets := operationManifest.Target.AllowedTargetRefs(); len(allowedTargets) != 0 {
			for _, targetRef := range operation.TargetRefs {
				if !runtimeCandidatePayloadContains(allowedTargets, targetRef) {
					return &RuntimeCandidatePayloadDiagnostic{Path: path + ".targetRefs", Actual: targetRef, Allowed: allowedTargets}
				}
			}
		}
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		// A finite owner scope is still a membership contract. Selector-only
		// scopes intentionally leave the final target resolution to the owner
		// feasibility probe, but a bounded scope must reject arbitrary targets at
		// the candidate boundary.
		if allowedTargets := operationManifest.Target.AllowedTargetRefs(); len(allowedTargets) != 0 {
			for _, targetRef := range operation.TargetRefs {
				if !runtimeCandidatePayloadContains(allowedTargets, targetRef) {
					return &RuntimeCandidatePayloadDiagnostic{Path: path + ".targetRefs", Actual: targetRef, Allowed: allowedTargets}
				}
			}
		}
	}
	if !runtimeCandidatePayloadContains(operationManifest.AllowedOperationFields, "dependsOn") {
		if _, found := raw["dependsOn"]; found {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".dependsOn", Actual: "dependsOn", Allowed: operationManifest.AllowedOperationFields}
		}
	}
	if _, present := raw["requiredOperationRef"]; present {
		ref := strings.TrimSpace(operation.RequiredOperationRef)
		if ref == "" || !runtimeCandidatePayloadContains(operationManifest.RequiredOperationReferences, ref) {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".requiredOperationRef", Actual: ref, Allowed: operationManifest.RequiredOperationReferences}
		}
	}
	if err := validateRuntimeCandidatePayloadParameters(operationManifest, operation.Parameters, path); err != nil {
		return err
	}
	return validateRuntimeCandidatePayloadVariant(operationManifest, operation, path)
}

func validateRuntimeCandidatePayloadParameters(operationManifest RuntimeCandidateOperationManifest, parameters []OperationParameter, path string) error {
	known := make(map[string]RuntimeOperationParameterContract, len(operationManifest.Parameters))
	for _, parameter := range operationManifest.Parameters {
		known[parameter.Name] = parameter
	}
	allowed := make([]string, 0, len(known))
	for name := range known {
		allowed = append(allowed, name)
	}
	allowed = normalizeStrings(allowed)
	for index, parameter := range parameters {
		name := strings.TrimSpace(parameter.Name)
		expected, found := known[name]
		if !found && !runtimeCandidatePayloadDynamicParameterAllowed(name, operationManifest.DynamicParameterPrefixes) {
			return &RuntimeCandidatePayloadDiagnostic{Path: fmt.Sprintf("%s.parameters[%d].name", path, index), Actual: name, Allowed: allowed, Forbidden: operationManifest.ForbiddenParameters}
		}
		if !found {
			if parameter.String == nil && parameter.Integer == nil && parameter.Boolean == nil && parameter.StringList == nil {
				return &RuntimeCandidatePayloadDiagnostic{Path: fmt.Sprintf("%s.parameters[%d]", path, index), Actual: "missing_scalar_value", Allowed: []string{"string", "integer", "boolean"}}
			}
			continue
		}
		actual, count := runtimeCandidatePayloadParameterType(parameter)
		if count != 1 || actual != expected.Type {
			return &RuntimeCandidatePayloadDiagnostic{Path: fmt.Sprintf("%s.parameters[%d]", path, index), Actual: string(actual), Allowed: []string{string(expected.Type)}, Cause: fmt.Errorf("parameter %q requires exactly one typed value", name)}
		}
		if runtimeCandidatePayloadContains(operationManifest.ForbiddenParameters, name) {
			return &RuntimeCandidatePayloadDiagnostic{Path: fmt.Sprintf("%s.parameters[%d].name", path, index), Actual: name, Allowed: allowed, Forbidden: operationManifest.ForbiddenParameters}
		}
	}
	return nil
}

func validateRuntimeCandidatePayloadVariant(operationManifest RuntimeCandidateOperationManifest, operation RuntimeOperation, path string) error {
	if len(operationManifest.Variants) == 0 {
		return nil
	}
	values := make(map[string]OperationParameter, len(operation.Parameters))
	for _, parameter := range operation.Parameters {
		values[strings.TrimSpace(parameter.Name)] = parameter
	}
	matches := make([]RuntimeOperationVariantContract, 0, 1)
	for _, variant := range operationManifest.Variants {
		if runtimeCandidatePayloadVariantMatches(values, variant) {
			matches = append(matches, variant)
		}
	}
	if len(matches) != 1 {
		return &RuntimeCandidatePayloadDiagnostic{Path: path + ".parameters", Actual: "unmatched_variant", Allowed: operationManifest.AllowedVariantIDs}
	}
	variant := matches[0]
	for _, name := range variant.RequiredParameters {
		if _, found := values[name]; !found {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".parameters", Actual: "missing_" + name, Required: []string{name}, Allowed: operationManifest.AllowedVariantIDs}
		}
	}
	for _, name := range variant.RequiredNonEmpty {
		if value, found := values[name]; !found || !runtimeCandidatePayloadParameterHasValue(value) {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".parameters", Actual: "empty_" + name, Required: []string{name}, Allowed: operationManifest.AllowedVariantIDs}
		}
	}
	for _, name := range variant.ForbiddenParameters {
		if _, found := values[name]; found {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".parameters", Actual: name, Forbidden: []string{name}, Allowed: operationManifest.AllowedVariantIDs}
		}
	}
	return nil
}

func runtimeCandidatePayloadVariantMatches(values map[string]OperationParameter, variant RuntimeOperationVariantContract) bool {
	if len(variant.Discriminators) == 0 {
		return false
	}
	for _, predicate := range variant.Discriminators {
		value, found := values[strings.TrimSpace(predicate.Name)]
		if !found || value.String == nil || !strings.EqualFold(strings.TrimSpace(*value.String), strings.TrimSpace(predicate.EqualsString)) {
			return false
		}
	}
	return true
}

func runtimeCandidatePayloadParameterHasValue(value OperationParameter) bool {
	switch {
	case value.String != nil:
		return strings.TrimSpace(*value.String) != ""
	case value.Integer != nil, value.Boolean != nil:
		return true
	default:
		return len(value.StringList) > 0
	}
}

func runtimeCandidatePayloadParameterType(value OperationParameter) (RuntimeOperationParameterType, int) {
	count := 0
	typeValue := RuntimeOperationParameterType("")
	if value.String != nil {
		count++
		typeValue = RuntimeOperationParameterTypeString
	}
	if value.Integer != nil {
		count++
		typeValue = RuntimeOperationParameterTypeInteger
	}
	if value.Boolean != nil {
		count++
		typeValue = RuntimeOperationParameterTypeBoolean
	}
	if value.StringList != nil {
		count++
		typeValue = RuntimeOperationParameterTypeStringList
	}
	return typeValue, count
}

func runtimeCandidatePayloadDynamicParameterAllowed(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) && strings.TrimSpace(strings.TrimPrefix(name, prefix)) != "" {
			return true
		}
	}
	return false
}

func runtimeCandidatePayloadOperationManifest(manifest RuntimeCandidateContractManifest, kind string) (RuntimeCandidateOperationManifest, bool) {
	kind = CanonicalRuntimeBlockKind(kind)
	for _, operation := range manifest.Operations {
		operation = operation.Normalized()
		if operation.Kind == kind {
			return operation, true
		}
	}
	return RuntimeCandidateOperationManifest{}, false
}

func runtimeCandidatePayloadOperationKinds(manifest RuntimeCandidateContractManifest) []string {
	kinds := make([]string, 0, len(manifest.Operations))
	for _, operation := range manifest.Operations {
		kinds = append(kinds, operation.Kind)
	}
	return normalizeStrings(kinds)
}

func runtimeCandidatePayloadContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func runtimeCandidatePayloadRawString(value json.RawMessage) string {
	var decoded string
	_ = json.Unmarshal(value, &decoded)
	return decoded
}
