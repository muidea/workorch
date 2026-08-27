package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// RuntimeOperationCorrectionRole is executable-operation-contract metadata for
// the finite semantic dimensions that SM-3 may use to prove a legal attempt-1
// change. It does not authorize retry; the Orchestrator still derives
// correction only from a persisted rejection and the sealed candidate manifest.
type RuntimeOperationCorrectionRole string

const (
	RuntimeOperationCorrectionRoleOperationKind    RuntimeOperationCorrectionRole = "operation_kind"
	RuntimeOperationCorrectionRoleTarget           RuntimeOperationCorrectionRole = "target"
	RuntimeOperationCorrectionRoleVariant          RuntimeOperationCorrectionRole = "variant"
	RuntimeOperationCorrectionRoleValidationRecipe RuntimeOperationCorrectionRole = "validation_recipe"
	RuntimeOperationCorrectionRoleClosedParameter  RuntimeOperationCorrectionRole = "closed_semantic_parameter"
)

// RuntimeOperationCorrectionFieldSource distinguishes a Runtime operation
// field from a capability-owner parameter. This prevents generic correction
// code from inferring semantic roles from owner-specific parameter names.
type RuntimeOperationCorrectionFieldSource string

const (
	RuntimeOperationCorrectionFieldOperation RuntimeOperationCorrectionFieldSource = "operation"
	RuntimeOperationCorrectionFieldParameter RuntimeOperationCorrectionFieldSource = "parameter"
)

// RuntimeOperationCorrectionField is the manifest-sealed view consumed by the
// candidate correction frontier. Name is wire metadata, never a semantic role.
type RuntimeOperationCorrectionField struct {
	Role   RuntimeOperationCorrectionRole
	Source RuntimeOperationCorrectionFieldSource
	Name   string
}

const (
	runtimeOperationCorrectionKindFieldName    = "kind"
	runtimeOperationCorrectionTargetFieldName  = "targetRefs"
	runtimeOperationCorrectionVariantFieldName = "parameters"
)

func RuntimeOperationKindCorrectionField() RuntimeOperationCorrectionField {
	return RuntimeOperationCorrectionField{
		Role: RuntimeOperationCorrectionRoleOperationKind, Source: RuntimeOperationCorrectionFieldOperation,
		Name: runtimeOperationCorrectionKindFieldName,
	}
}

func RuntimeOperationVariantCorrectionField() RuntimeOperationCorrectionField {
	return RuntimeOperationCorrectionField{
		Role: RuntimeOperationCorrectionRoleVariant, Source: RuntimeOperationCorrectionFieldOperation,
		Name: runtimeOperationCorrectionVariantFieldName,
	}
}

// RuntimeOperationCorrectionFields returns the correction metadata sealed in
// one operation manifest. Outer target correction comes from descriptor-owned
// target cardinality; capability parameter roles come only from the operation
// contract. No owner, operation kind, or parameter spelling is interpreted.
func RuntimeOperationCorrectionFields(manifest RuntimeCandidateOperationManifest) ([]RuntimeOperationCorrectionField, error) {
	manifest = manifest.Normalized()
	ret := make([]RuntimeOperationCorrectionField, 0, len(manifest.Parameters)+1)
	switch manifest.TargetCardinality {
	case RuntimeOperationTargetCardinalityOne, RuntimeOperationTargetCardinalityMany:
		ret = append(ret, RuntimeOperationCorrectionField{
			Role: RuntimeOperationCorrectionRoleTarget, Source: RuntimeOperationCorrectionFieldOperation,
			Name: runtimeOperationCorrectionTargetFieldName,
		})
	case "", RuntimeOperationTargetCardinalityZero:
	default:
		return nil, fmt.Errorf("operation correction metadata has invalid target cardinality %q", manifest.TargetCardinality)
	}

	seenSingleton := make(map[RuntimeOperationCorrectionRole]struct{}, 2)
	for _, parameter := range manifest.Parameters {
		role := RuntimeOperationCorrectionRole(strings.TrimSpace(string(parameter.CorrectionRole)))
		if role == "" {
			continue
		}
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			return nil, fmt.Errorf("operation correction metadata has an empty parameter name")
		}
		switch role {
		case RuntimeOperationCorrectionRoleValidationRecipe:
			if parameter.Type != RuntimeOperationParameterTypeString {
				return nil, fmt.Errorf("operation correction role %q requires a string parameter", role)
			}
		case RuntimeOperationCorrectionRoleClosedParameter:
			if parameter.Type != RuntimeOperationParameterTypeBoolean {
				return nil, fmt.Errorf("operation correction role %q requires a boolean parameter", role)
			}
		default:
			return nil, fmt.Errorf("operation correction metadata has unknown parameter role %q", role)
		}
		if role == RuntimeOperationCorrectionRoleValidationRecipe {
			if _, duplicate := seenSingleton[role]; duplicate {
				return nil, fmt.Errorf("operation correction role %q must identify exactly one parameter", role)
			}
			seenSingleton[role] = struct{}{}
		}
		ret = append(ret, RuntimeOperationCorrectionField{Role: role, Source: RuntimeOperationCorrectionFieldParameter, Name: name})
	}
	return ret, nil
}

// ValidateRuntimeCandidateOperationCorrectionMetadata fail-closes a sealed
// candidate manifest whose descriptor/parameter correction declarations are
// unknown, ill-typed, or ambiguous. Callers must run this before using a field
// lookup so malformed metadata cannot look like an exhausted frontier.
func ValidateRuntimeCandidateOperationCorrectionMetadata(manifest RuntimeCandidateContractManifest) error {
	manifest = manifest.Normalized()
	for _, operation := range manifest.Operations {
		if _, err := RuntimeOperationCorrectionFields(operation); err != nil {
			return fmt.Errorf("operation %q correction metadata: %w", operation.Kind, err)
		}
	}
	return nil
}

func RuntimeOperationCorrectionFieldsForRole(manifest RuntimeCandidateOperationManifest, role RuntimeOperationCorrectionRole) ([]RuntimeOperationCorrectionField, error) {
	fields, err := RuntimeOperationCorrectionFields(manifest)
	if err != nil {
		return nil, err
	}
	role = RuntimeOperationCorrectionRole(strings.TrimSpace(string(role)))
	ret := make([]RuntimeOperationCorrectionField, 0, len(fields))
	for _, field := range fields {
		if field.Role == role {
			ret = append(ret, field)
		}
	}
	return ret, nil
}

func RuntimeOperationCorrectionFieldForRole(manifest RuntimeCandidateOperationManifest, role RuntimeOperationCorrectionRole) (RuntimeOperationCorrectionField, bool, error) {
	fields, err := RuntimeOperationCorrectionFieldsForRole(manifest, role)
	if err != nil {
		return RuntimeOperationCorrectionField{}, false, err
	}
	if len(fields) == 0 {
		return RuntimeOperationCorrectionField{}, false, nil
	}
	if len(fields) != 1 {
		return RuntimeOperationCorrectionField{}, false, fmt.Errorf("operation correction role %q is ambiguous", role)
	}
	return fields[0], true, nil
}

// CandidatePath is the canonical candidate diagnostic path for this typed
// field. It centralizes wire spelling at the contract boundary.
func (f RuntimeOperationCorrectionField) CandidatePath(operationIndex int) string {
	name := strings.TrimSpace(f.Name)
	if operationIndex < 0 || name == "" {
		return ""
	}
	switch f.Source {
	case RuntimeOperationCorrectionFieldOperation:
		return fmt.Sprintf("$.operations[%d].%s", operationIndex, name)
	case RuntimeOperationCorrectionFieldParameter:
		return fmt.Sprintf("$.operations[%d].parameters[name=%s]", operationIndex, name)
	default:
		return ""
	}
}

func (f RuntimeOperationCorrectionField) WitnessDimension() RuntimeCandidateCorrectionWitnessDimension {
	switch f.Role {
	case RuntimeOperationCorrectionRoleOperationKind:
		return RuntimeCandidateCorrectionWitnessOperationKind
	case RuntimeOperationCorrectionRoleTarget:
		return RuntimeCandidateCorrectionWitnessTarget
	case RuntimeOperationCorrectionRoleVariant:
		return RuntimeCandidateCorrectionWitnessVariant
	case RuntimeOperationCorrectionRoleValidationRecipe:
		return RuntimeCandidateCorrectionWitnessValidationRecipe
	case RuntimeOperationCorrectionRoleClosedParameter:
		return RuntimeCandidateCorrectionWitnessClosedParameter
	default:
		return ""
	}
}
