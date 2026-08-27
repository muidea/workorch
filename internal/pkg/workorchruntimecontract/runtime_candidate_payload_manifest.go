package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
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
	if err := ValidateRuntimeCandidateManifestOperationCardinality(manifest); err != nil {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.operations", Actual: "invalid_max_operation_count", Cause: err}
	}
	if manifest.EffectPolicy.State != manifest.State {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.effectPolicy.state", Actual: string(manifest.EffectPolicy.State), Allowed: []string{string(manifest.State)}, Cause: fmt.Errorf("runtime candidate effect policy state does not match manifest")}
	}
	if err := manifest.EffectPolicy.Validate(); err != nil {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.effectPolicy", Actual: "invalid", Cause: err}
	}
	if err := validateRuntimeCandidateManifestOperationWireFields(manifest); err != nil {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.operations", Actual: "invalid_operation_fields", Cause: err}
	}
	candidate, err := DecodeRuntimeCandidatePayload(manifest.State, payload)
	if err != nil {
		return RuntimeCandidate{}, runtimeCandidatePayloadDecodeDiagnostic(manifest, payload, err)
	}
	if err := validateRuntimeCandidatePayloadManifest(manifest, payload, candidate); err != nil {
		return RuntimeCandidate{}, err
	}
	return DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
}

// DeriveRuntimeCandidateOperationAuthority deterministically assigns the
// operation identity and dependency graph owned by the sealed candidate
// transaction. Provider documents contribute only operation content and
// requiredOperationRef. Required operations use their normalized manifest
// sequence slot; auxiliary operations use their canonical execution-semantic
// key. Provider array order is never identity authority.
// Existing ID and DependsOn values on an in-process candidate are ignored.
func DeriveRuntimeCandidateOperationAuthority(manifest RuntimeCandidateContractManifest, candidate RuntimeCandidate) (RuntimeCandidate, error) {
	manifest = manifest.Normalized()
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" ||
		manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$", Actual: "unsealed_manifest", Cause: fmt.Errorf("runtime candidate contract manifest is invalid")}
	}
	if err := ValidateRuntimeCandidateManifestOperationCardinality(manifest); err != nil {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.operations", Actual: "invalid_max_operation_count", Cause: err}
	}
	if err := validateRuntimeCandidateManifestOperationWireFields(manifest); err != nil {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.operations", Actual: "invalid_operation_fields", Cause: err}
	}
	if err := validateRequiredOperationVariants(manifest.RequiredOperationSequence); err != nil {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.operations", Actual: "invalid_required_operation_sequence", Cause: err}
	}
	if manifest.State != ConvergenceStateNeedStateChange && manifest.State != ConvergenceStateNeedValidation {
		return cloneRuntimeCandidate(candidate), nil
	}
	if manifest.State == ConvergenceStateNeedStateChange && candidate.StateChange == nil {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.operations", Actual: "missing_state_change_operations", Required: []string{"operations"}}
	}
	if manifest.State == ConvergenceStateNeedValidation && candidate.Validation == nil {
		return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{Path: "$.operations", Actual: "missing_validation_operations", Required: []string{"operations"}}
	}

	ret := cloneRuntimeCandidate(candidate)
	operations, err := runtimeCandidatePayloadOperations(ret)
	if err != nil {
		return RuntimeCandidate{}, err
	}
	if err := validateRuntimeCandidateOperationCount(manifest, len(operations)); err != nil {
		return RuntimeCandidate{}, err
	}
	type requiredSlot struct {
		requirement RuntimeRequiredOperationVariant
		operationID string
	}
	requiredByRef := make(map[string]requiredSlot, len(manifest.RequiredOperationSequence))
	for ordinal, requirement := range manifest.RequiredOperationSequence {
		requiredByRef[requirement.RequirementID] = requiredSlot{
			requirement: requirement,
			operationID: stableRuntimeCandidateOperationID(manifest.Hash, ordinal, "required", requirement.RequirementID),
		}
	}
	seenRequired := make(map[string]int, len(requiredByRef))
	for index := range operations {
		operation := operations[index].Normalized()
		operation.ID = ""
		operation.DependsOn = nil
		if operation.RequiredOperationRef == "" {
			operations[index] = operation
			continue
		}
		slot, found := requiredByRef[operation.RequiredOperationRef]
		if !found {
			return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{
				Path: fmt.Sprintf("$.operations[%d].requiredOperationRef", index), Actual: operation.RequiredOperationRef,
				Allowed: runtimeCandidateRequiredOperationRefs(manifest.RequiredOperationSequence),
			}
		}
		if previous, duplicate := seenRequired[operation.RequiredOperationRef]; duplicate {
			return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{
				Path: fmt.Sprintf("$.operations[%d].requiredOperationRef", index), Actual: operation.RequiredOperationRef,
				Cause: fmt.Errorf("required operation reference duplicates operations[%d]", previous),
			}
		}
		seenRequired[operation.RequiredOperationRef] = index
		operation.ID = slot.operationID
		operations[index] = operation
	}
	for _, requirement := range manifest.RequiredOperationSequence {
		if _, found := seenRequired[requirement.RequirementID]; !found {
			return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{
				Path: "$.operations", Actual: "missing_required_operation_" + requirement.RequirementID,
				Required: runtimeCandidateRequiredOperationRefs(manifest.RequiredOperationSequence),
			}
		}
	}
	semanticSlots := make(map[string]int, len(operations))
	// Required sequence slots are manifest authority and may intentionally carry
	// the same execution shape. They reserve their semantic keys so a provider
	// cannot smuggle the same execution a second time as an auxiliary operation.
	for index, operation := range operations {
		if operation.RequiredOperationRef == "" {
			continue
		}
		key := StableRuntimeOperationShape(operation)
		if _, found := semanticSlots[key]; !found {
			semanticSlots[key] = index
		}
	}
	for index := range operations {
		operation := operations[index]
		if operation.RequiredOperationRef != "" {
			continue
		}
		semanticKey := StableRuntimeOperationShape(operation)
		if previous, duplicate := semanticSlots[semanticKey]; duplicate {
			return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{
				Path: fmt.Sprintf("$.operations[%d]", index), Actual: "duplicate_auxiliary_semantic_slot",
				Allowed: []string{"unique execution-semantic operation"},
				Cause:   fmt.Errorf("auxiliary operation duplicates operations[%d]", previous),
			}
		}
		semanticSlots[semanticKey] = index
		operation.ID = stableRuntimeCandidateAuxiliaryOperationID(manifest.Hash, semanticKey)
		operations[index] = operation
	}
	seenOperationIDs := make(map[string]int, len(operations))
	for index, operation := range operations {
		if previous, collision := seenOperationIDs[operation.ID]; collision {
			return RuntimeCandidate{}, &RuntimeCandidatePayloadDiagnostic{
				Path: fmt.Sprintf("$.operations[%d]", index), Actual: "operation_identity_collision_" + operation.ID,
				Allowed: []string{"unique manifest-derived operation identity"},
				Cause:   fmt.Errorf("derived operation identity collides with operations[%d]", previous),
			}
		}
		seenOperationIDs[operation.ID] = index
	}
	for index := range operations {
		operation := operations[index].Normalized()
		if operation.RequiredOperationRef != "" {
			slot := requiredByRef[operation.RequiredOperationRef]
			operation.DependsOn = make([]string, 0, len(slot.requirement.DependsOnRequirementIDs))
			for _, dependencyRef := range slot.requirement.DependsOnRequirementIDs {
				operation.DependsOn = append(operation.DependsOn, requiredByRef[dependencyRef].operationID)
			}
			operation = operation.Normalized()
		}
		operations[index] = operation
	}
	return runtimeCandidateWithOperations(ret, operations), nil
}

func stableRuntimeCandidateAuxiliaryOperationID(manifestHash, semanticKey string) string {
	hash := sha256.New()
	writeHashString(hash, "runtime-candidate-auxiliary-operation-id/v2")
	writeHashString(hash, manifestHash)
	writeHashString(hash, semanticKey)
	digest := hash.Sum(nil)
	return "op-aux-" + hex.EncodeToString(digest[:10])
}

func stableRuntimeCandidateOperationID(manifestHash string, ordinal int, slotClass, slotKey string) string {
	hash := sha256.New()
	writeHashString(hash, "runtime-candidate-operation-id/v1")
	writeHashString(hash, manifestHash)
	writeHashString(hash, slotClass)
	writeHashString(hash, fmt.Sprintf("%d", ordinal))
	writeHashString(hash, slotKey)
	digest := hash.Sum(nil)
	return fmt.Sprintf("op-%04d-%s", ordinal+1, hex.EncodeToString(digest[:10]))
}

func validateRuntimeCandidateOperationCount(manifest RuntimeCandidateContractManifest, count int) error {
	if count <= manifest.MaxOperationCount {
		return nil
	}
	return &RuntimeCandidatePayloadDiagnostic{
		Path: "$.operations", Actual: fmt.Sprintf("operation_count_%d", count),
		Allowed: []string{fmt.Sprintf("0..%d operations", manifest.MaxOperationCount)},
		Cause:   fmt.Errorf("candidate operation count exceeds sealed maxOperationCount"),
	}
}

func runtimeCandidateRequiredOperationRefs(values []RuntimeRequiredOperationVariant) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		ret = append(ret, value.RequirementID)
	}
	return normalizeStrings(ret)
}

func runtimeCandidateWithOperations(candidate RuntimeCandidate, operations []RuntimeOperation) RuntimeCandidate {
	if candidate.StateChange != nil {
		candidate.StateChange.Operations = make([]StateChangingOperation, len(operations))
		for index, operation := range operations {
			candidate.StateChange.Operations[index] = StateChangingOperation{Operation: cloneRuntimeOperation(operation)}
		}
		return candidate
	}
	if candidate.Validation != nil {
		candidate.Validation.Operations = make([]ValidationOperation, len(operations))
		for index, operation := range operations {
			candidate.Validation.Operations[index] = ValidationOperation{Operation: cloneRuntimeOperation(operation)}
		}
	}
	return candidate
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
			return &RuntimeCandidatePayloadDiagnostic{Path: "$." + field, Actual: "missing_" + field, Required: manifest.RequiredTopLevelFields, Allowed: manifest.AllowedTopLevelFields}
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
	if err := validateRuntimeCandidateOperationCount(manifest, len(rawOperations)); err != nil {
		return err
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
	for _, required := range operationManifest.RequiredOperationFields {
		value, present := raw[required]
		if !present || (required == "kind" && strings.TrimSpace(runtimeCandidatePayloadRawString(value)) == "") {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + "." + required, Actual: "missing_" + required, Required: operationManifest.RequiredOperationFields, Allowed: operationManifest.AllowedOperationFields}
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
	if _, present := raw["requiredOperationRef"]; present {
		ref := strings.TrimSpace(operation.RequiredOperationRef)
		if ref == "" || !runtimeCandidatePayloadContains(operationManifest.RequiredOperationReferences, ref) {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".requiredOperationRef", Actual: ref, Allowed: operationManifest.RequiredOperationReferences}
		}
	}
	if err := validateRuntimeCandidatePayloadParameters(operationManifest, operation.Parameters, path); err != nil {
		return err
	}
	if err := validateRuntimeCandidatePayloadVariant(operationManifest, operation, path); err != nil {
		return err
	}
	authorization, err := RuntimeCandidateOperationBindingAuthorizationFor(operationManifest, operation)
	if err != nil {
		return &RuntimeCandidatePayloadDiagnostic{Path: path + ".bindingRefs", Actual: "unsealed_operation_authorization", Cause: err}
	}
	for _, bindingRef := range operation.BindingRefs {
		if !runtimeCandidatePayloadContains(authorization.AllowedBindingRefs, bindingRef) {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".bindingRefs", Actual: bindingRef, Allowed: authorization.AllowedBindingRefs}
		}
	}
	return nil
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
	present := make(map[string]OperationParameter, len(parameters))
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
		present[name] = parameter
	}
	for _, parameter := range operationManifest.Parameters {
		if !parameter.Required {
			continue
		}
		_, found := present[parameter.Name]
		if !found {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".parameters[name=" + parameter.Name + "]", Actual: "missing_" + parameter.Name, Required: []string{parameter.Name}, Allowed: allowed}
		}
	}
	if len(operationManifest.RequireOneOfParameters) != 0 {
		found := false
		for _, name := range operationManifest.RequireOneOfParameters {
			if value, exists := present[name]; exists && runtimeCandidatePayloadParameterHasValue(value) {
				found = true
				break
			}
		}
		if !found {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".parameters", Actual: "missing_one_of_parameters", Required: operationManifest.RequireOneOfParameters, Allowed: allowed}
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
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".parameters[name=" + name + "]", Actual: "missing_" + name, Required: []string{name}, Allowed: operationManifest.AllowedVariantIDs}
		}
	}
	for _, name := range variant.RequiredNonEmpty {
		if value, found := values[name]; !found || !runtimeCandidatePayloadParameterHasValue(value) {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".parameters[name=" + name + "]", Actual: "empty_" + name, Required: []string{name}, Allowed: operationManifest.AllowedVariantIDs}
		}
	}
	for _, name := range variant.ForbiddenParameters {
		if _, found := values[name]; found {
			return &RuntimeCandidatePayloadDiagnostic{Path: path + ".parameters[name=" + name + "]", Actual: name, Forbidden: []string{name}, Allowed: operationManifest.AllowedVariantIDs}
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
