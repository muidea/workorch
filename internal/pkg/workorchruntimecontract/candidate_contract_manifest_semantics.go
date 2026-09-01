package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeCandidateContractDiagnostic is the pure semantic counterpart of the
// strict JSON diagnostic. It describes a decoded candidate that is outside the
// sealed manifest; it owns no aggregate state and cannot select a retry.
type RuntimeCandidateContractDiagnostic struct {
	OperationID string
	Path        string
	Actual      string
	Allowed     []string
	Required    []string
	Forbidden   []string
	Cause       error
}

func (d *RuntimeCandidateContractDiagnostic) Error() string {
	if d == nil {
		return "runtime candidate contract is invalid"
	}
	parts := []string{"runtime candidate contract diagnostic"}
	if d.OperationID != "" {
		parts = append(parts, "operation="+d.OperationID)
	}
	if d.Path != "" {
		parts = append(parts, "path="+d.Path)
	}
	if d.Actual != "" {
		parts = append(parts, "actual="+d.Actual)
	}
	if len(d.Allowed) != 0 {
		parts = append(parts, "allowed="+strings.Join(normalizeStrings(d.Allowed), ","))
	}
	if len(d.Required) != 0 {
		parts = append(parts, "required="+strings.Join(normalizeStrings(d.Required), ","))
	}
	if len(d.Forbidden) != 0 {
		parts = append(parts, "forbidden="+strings.Join(normalizeStrings(d.Forbidden), ","))
	}
	if d.Cause != nil {
		parts = append(parts, "cause="+d.Cause.Error())
	}
	return strings.Join(parts, "; ")
}

func (d *RuntimeCandidateContractDiagnostic) Unwrap() error {
	if d == nil {
		return nil
	}
	return d.Cause
}

// RuntimeCandidateFailureDiagnosticFromContractDiagnostic converts a pure
// manifest diagnostic into the bounded correction DTO. Its Cause intentionally
// never crosses this boundary.
func RuntimeCandidateFailureDiagnosticFromContractDiagnostic(value *RuntimeCandidateContractDiagnostic) *RuntimeCandidateFailureDiagnostic {
	if value == nil {
		return nil
	}
	return (&RuntimeCandidateFailureDiagnostic{
		OperationID: value.OperationID,
		Path:        value.Path,
		Actual:      value.Actual,
		Allowed:     append([]string(nil), value.Allowed...),
		Required:    append([]string(nil), value.Required...),
		Forbidden:   append([]string(nil), value.Forbidden...),
	}).Clone()
}

// ValidateRuntimeCandidateAgainstManifest checks only decoded candidate fields
// that are directly projected by the sealed manifest. State/effect policy and
// aggregate-local target grounding remain with their existing authorities.
func ValidateRuntimeCandidateAgainstManifest(manifest RuntimeCandidateContractManifest, candidate RuntimeCandidate) error {
	manifest = manifest.Normalized()
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" || manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) {
		return &RuntimeCandidateContractDiagnostic{Path: "$", Actual: "unsealed_manifest", Cause: fmt.Errorf("runtime candidate contract manifest is invalid")}
	}
	if err := ValidateRuntimeCandidateManifestOperationCardinality(manifest); err != nil {
		return &RuntimeCandidateContractDiagnostic{Path: "$.operations", Actual: "invalid_max_operation_count", Cause: err}
	}
	if manifest.EffectPolicy.State != manifest.State {
		return &RuntimeCandidateContractDiagnostic{Path: "$.effectPolicy.state", Actual: string(manifest.EffectPolicy.State), Allowed: []string{string(manifest.State)}, Cause: fmt.Errorf("runtime candidate effect policy state does not match manifest")}
	}
	if err := validateRuntimeCandidateManifestOperationWireFields(manifest); err != nil {
		return &RuntimeCandidateContractDiagnostic{Path: "$.operations", Actual: "invalid_operation_fields", Cause: err}
	}
	if err := ValidateRuntimeCandidateOperationCorrectionMetadata(manifest); err != nil {
		return &RuntimeCandidateContractDiagnostic{Path: "$.operations", Actual: "invalid_correction_metadata", Cause: err}
	}
	if err := manifest.EffectPolicy.Validate(); err != nil {
		return &RuntimeCandidateContractDiagnostic{Path: "$.effectPolicy", Actual: "invalid", Cause: err}
	}
	if manifest.State != ConvergenceStateNeedStateChange && manifest.State != ConvergenceStateNeedValidation {
		return nil
	}
	operations, err := runtimeCandidatePayloadOperations(candidate)
	if err != nil {
		return &RuntimeCandidateContractDiagnostic{Path: "$.operations", Actual: "candidate_shape", Cause: err}
	}
	if len(operations) > manifest.MaxOperationCount {
		return &RuntimeCandidateContractDiagnostic{
			Path: "$.operations", Actual: fmt.Sprintf("operation_count_%d", len(operations)),
			Allowed: []string{fmt.Sprintf("0..%d operations", manifest.MaxOperationCount)},
			Cause:   fmt.Errorf("candidate operation count exceeds sealed maxOperationCount"),
		}
	}
	expectedCandidate, err := DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		return &RuntimeCandidateContractDiagnostic{Path: "$.operations", Actual: "operation_authority_derivation_failed", Cause: err}
	}
	expectedOperations, err := runtimeCandidatePayloadOperations(expectedCandidate)
	if err != nil || len(expectedOperations) != len(operations) {
		return &RuntimeCandidateContractDiagnostic{Path: "$.operations", Actual: "operation_authority_shape_mismatch", Cause: err}
	}
	for index, operation := range operations {
		operation = operation.Normalized()
		expected := expectedOperations[index].Normalized()
		if operation.ID != expected.ID {
			return runtimeCandidateContractDiagnostic(operation.ID, fmt.Sprintf("$.operations[%d].id", index), operation.ID, []string{expected.ID}, []string{"owner-derived id"}, nil, fmt.Errorf("operation id is not the manifest-derived identity"))
		}
		if !runtimeCandidateStringSlicesEqual(operation.DependsOn, expected.DependsOn) {
			return runtimeCandidateContractDiagnostic(operation.ID, fmt.Sprintf("$.operations[%d].dependsOn", index), strings.Join(operation.DependsOn, ","), expected.DependsOn, []string{"owner-derived dependency graph"}, nil, fmt.Errorf("operation dependencies do not match the required operation graph"))
		}
	}
	operationIDs := make(map[string]struct{}, len(operations))
	for index, operation := range operations {
		operation = operation.Normalized()
		if operation.ID == "" {
			return runtimeCandidateContractDiagnostic("", fmt.Sprintf("$.operations[%d].id", index), "missing_id", []string{"id"}, []string{"id"}, nil, nil)
		}
		if _, duplicate := operationIDs[operation.ID]; duplicate {
			return runtimeCandidateContractDiagnostic(operation.ID, fmt.Sprintf("$.operations[%d].id", index), "duplicate_"+operation.ID, []string{"unique operation id"}, nil, nil, nil)
		}
		operationIDs[operation.ID] = struct{}{}
	}
	for index, operation := range operations {
		if err := validateRuntimeCandidateOperationAgainstManifest(manifest, operation, index); err != nil {
			return err
		}
		operation = operation.Normalized()
		for _, dependency := range operation.DependsOn {
			if _, found := operationIDs[dependency]; !found {
				allowed := make([]string, 0, len(operationIDs))
				for id := range operationIDs {
					allowed = append(allowed, id)
				}
				return runtimeCandidateContractDiagnostic(operation.ID, fmt.Sprintf("$.operations[%d].dependsOn", index), dependency, allowed, nil, nil, fmt.Errorf("operation dependency is absent from candidate"))
			}
		}
	}
	return nil
}

func validateRuntimeCandidateManifestOperationWireFields(manifest RuntimeCandidateContractManifest) error {
	for _, operation := range manifest.Operations {
		operation = operation.Normalized()
		for _, field := range []string{"id", "dependsOn", "coverageWorkItemIds"} {
			if runtimeCandidatePayloadContains(operation.AllowedOperationFields, field) {
				return fmt.Errorf("candidate manifest operation kind %q exposes owner-derived field %q", operation.Kind, field)
			}
		}
		if !runtimeCandidatePayloadContains(operation.RequiredOperationFields, "kind") {
			return fmt.Errorf("candidate manifest operation kind %q must require kind", operation.Kind)
		}
		for _, field := range operation.RequiredOperationFields {
			if !runtimeCandidatePayloadContains(operation.AllowedOperationFields, field) {
				return fmt.Errorf("candidate manifest operation kind %q requires non-allowed field %q", operation.Kind, field)
			}
		}
	}
	return nil
}

func runtimeCandidateStringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validateRuntimeCandidateOperationAgainstManifest(manifest RuntimeCandidateContractManifest, operation RuntimeOperation, index int) error {
	operation = operation.Normalized()
	path := fmt.Sprintf("$.operations[%d]", index)
	operationManifest, found := runtimeCandidatePayloadOperationManifest(manifest, operation.Kind)
	if !found {
		return runtimeCandidateContractDiagnostic(operation.ID, path+".kind", operation.Kind, runtimeCandidatePayloadOperationKinds(manifest), nil, nil, nil)
	}
	if len(operation.CoverageWorkItemIDs) != 0 {
		return runtimeCandidateContractDiagnostic(operation.ID, path+".coverageWorkItemIds", strings.Join(coverageIDStrings(operation.CoverageWorkItemIDs), ","), nil, nil, []string{"coverageWorkItemIds"}, nil)
	}
	if err := validateRuntimeCandidateManifestTargetCardinality(operation, operationManifest.TargetCardinality); err != nil {
		return runtimeCandidateContractDiagnostic(operation.ID, path+".targetRefs", runtimeCandidateManifestTargetCardinalityActual(operation.TargetRefs), operationManifest.AllowedOperationFields, []string{string(operationManifest.TargetCardinality)}, nil, err)
	}
	if err := ValidateRuntimeOperationTargetsAgainstAuthorization(operationManifest.Target, operation.TargetRefs, operationManifest.RequireTargetRefs); err != nil {
		actual, allowed, required := runtimeOperationTargetAuthorizationDiagnostic(err)
		return runtimeCandidateContractDiagnostic(operation.ID, path+".targetRefs", actual, allowed, required, nil, err)
	}
	// Whether the renderer emits the optional dependsOn field is a wire-schema
	// concern. The decoded graph is instead checked by ValidateWithActivation,
	// which owns the candidate-wide operation-ID/dependency relation.
	if operation.RequiredOperationRef != "" && !runtimeCandidatePayloadContains(operationManifest.RequiredOperationReferences, operation.RequiredOperationRef) {
		return runtimeCandidateContractDiagnostic(operation.ID, path+".requiredOperationRef", operation.RequiredOperationRef, operationManifest.RequiredOperationReferences, nil, nil, nil)
	}
	values := make(map[string]OperationParameter, len(operation.Parameters))
	allowedParameters := make([]string, 0, len(operationManifest.Parameters))
	contracts := make(map[string]RuntimeOperationParameterContract, len(operationManifest.Parameters))
	for _, parameter := range operationManifest.Parameters {
		contracts[parameter.Name] = parameter
		allowedParameters = append(allowedParameters, parameter.Name)
	}
	for _, parameter := range operation.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if _, duplicate := values[name]; duplicate {
			return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters[name="+name+"]", "duplicate_"+name, allowedParameters, nil, nil, nil)
		}
		contract, known := contracts[name]
		if !known && !(operationManifest.AllowsDynamicParameters && runtimeCandidatePayloadDynamicParameterAllowed(name, operationManifest.DynamicParameterPrefixes)) {
			return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters[name="+name+"].name", name, allowedParameters, nil, operationManifest.ForbiddenParameters, nil)
		}
		if known {
			actual, count := runtimeCandidatePayloadParameterType(parameter)
			if count != 1 || actual != contract.Type {
				return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters[name="+name+"]", string(actual), []string{string(contract.Type)}, nil, nil, fmt.Errorf("parameter %q requires exactly one typed value", name))
			}
			if runtimeCandidatePayloadContains(operationManifest.ForbiddenParameters, name) {
				return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters[name="+name+"].name", name, allowedParameters, nil, operationManifest.ForbiddenParameters, nil)
			}
		}
		values[name] = parameter
	}
	for _, parameter := range operationManifest.Parameters {
		if parameter.Required {
			if _, found := values[parameter.Name]; !found {
				return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters", "missing_"+parameter.Name, allowedParameters, []string{parameter.Name}, nil, nil)
			}
		}
	}
	if len(operationManifest.RequireOneOfParameters) != 0 {
		found := false
		for _, name := range operationManifest.RequireOneOfParameters {
			if value, exists := values[name]; exists && runtimeCandidatePayloadParameterHasValue(value) {
				found = true
				break
			}
		}
		if !found {
			return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters", "missing_one_of_parameters", operationManifest.RequireOneOfParameters, operationManifest.RequireOneOfParameters, nil, nil)
		}
	}
	if err := validateRuntimeCandidateManagedValidationRecipe(operationManifest, operation, values); err != nil {
		allowed := make([]string, 0, len(operationManifest.ValidationRecipes))
		for _, recipe := range operationManifest.ValidationRecipes {
			allowed = append(allowed, recipe.ID)
		}
		actual := "missing_recipeId"
		if value, found := values["recipeId"]; found && value.String != nil {
			actual = strings.TrimSpace(*value.String)
		}
		return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters[name=recipeId]", actual, allowed, []string{"recipeId"}, nil, err)
	}
	if err := validateRuntimeCandidateVariantAgainstManifest(operationManifest, operation, values, path); err != nil {
		return err
	}
	bindingAuthorization, err := RuntimeCandidateOperationBindingAuthorizationFor(operationManifest, operation)
	if err != nil {
		return runtimeCandidateContractDiagnostic(operation.ID, path+".requiredOperationRef", operation.RequiredOperationRef, operationManifest.RequiredOperationReferences, nil, nil, err)
	}
	allowedBindingRefs := bindingAuthorization.AllowedBindingRefs
	if len(operation.BindingRefs) > 1 {
		return runtimeCandidateContractDiagnostic(operation.ID, path+".bindingRefs", strings.Join(operation.BindingRefs, ","), allowedBindingRefs, []string{"bindingRefs"}, nil, fmt.Errorf("an operation may select only one coverage evidence binding"))
	}
	if len(allowedBindingRefs) != 0 && len(operation.BindingRefs) != 1 {
		return runtimeCandidateContractDiagnostic(operation.ID, path+".bindingRefs", "missing_bindingRefs", allowedBindingRefs, []string{"bindingRefs"}, nil, fmt.Errorf("a binding-authorized operation must select exactly one sealed coverage evidence binding"))
	}
	if len(operation.BindingRefs) != 0 && !runtimeCandidateManifestBindingRefsAllowed(allowedBindingRefs, operation.BindingRefs) {
		return runtimeCandidateContractDiagnostic(operation.ID, path+".bindingRefs", strings.Join(operation.BindingRefs, ","), allowedBindingRefs, nil, []string{"bindingRefs"}, nil)
	}
	return nil
}

// RuntimeCandidateOperationBindingAuthorizationFor resolves the only binding
// policy applicable to an exact decoded operation. It is pure manifest
// membership; it neither grants evidence nor mutates Runtime state.
func RuntimeCandidateOperationBindingAuthorizationFor(manifest RuntimeCandidateOperationManifest, operation RuntimeOperation) (RuntimeOperationBindingAuthorization, error) {
	manifest = manifest.Normalized()
	operation = operation.Normalized()
	contract, found := RuntimeOperationContractForKind(operation.Kind)
	if !found {
		return RuntimeOperationBindingAuthorization{}, fmt.Errorf("operation kind %q has no variant contract", operation.Kind)
	}
	variant, err := MatchRuntimeOperationVariant(operation, contract)
	if err != nil {
		return RuntimeOperationBindingAuthorization{}, err
	}
	for _, authorization := range manifest.BindingAuthorizations {
		authorization = authorization.Normalized()
		if authorization.RequiredOperationRef == operation.RequiredOperationRef && authorization.VariantID == variant.ID {
			return authorization, nil
		}
	}
	return RuntimeOperationBindingAuthorization{}, fmt.Errorf("operation variant %q with required operation ref %q has no sealed binding authorization", variant.ID, operation.RequiredOperationRef)
}

// validateRuntimeCandidateManagedValidationRecipe is a pure manifest
// membership check.  The list is already narrowed to the selected binding's
// Shell-owner-confirmed recipes; this function neither executes a recipe nor
// converts its result into coverage credit.
func validateRuntimeCandidateManagedValidationRecipe(manifest RuntimeCandidateOperationManifest, operation RuntimeOperation, values map[string]OperationParameter) error {
	if CanonicalRuntimeBlockKind(operation.Kind) != RuntimeBlockKindManagedValidationSession {
		return nil
	}
	value, found := values["recipeId"]
	if !found || value.String == nil || strings.TrimSpace(*value.String) == "" {
		return fmt.Errorf("managed validation operation requires recipeId")
	}
	recipeID := strings.TrimSpace(*value.String)
	if _, found := RuntimeValidationRecipeForID(manifest.ValidationRecipes, recipeID); !found {
		return fmt.Errorf("managed validation operation selected unavailable recipe %q", recipeID)
	}
	return nil
}

func runtimeCandidateManifestBindingRefsMatch(allowed, actual []string) bool {
	actual = normalizeStrings(actual)
	if len(actual) == 0 {
		return false
	}
	for _, ref := range actual {
		if !runtimeCandidatePayloadContains(allowed, ref) {
			return false
		}
	}
	return true
}

func runtimeCandidateManifestBindingRefsAllowed(allowed, actual []string) bool {
	actual = normalizeStrings(actual)
	for _, ref := range actual {
		if !runtimeCandidatePayloadContains(allowed, ref) {
			return false
		}
	}
	return true
}

func runtimeCandidateManifestTargetCardinalityActual(targetRefs []string) string {
	if len(targetRefs) == 0 {
		return "missing_targetRefs"
	}
	return strings.Join(targetRefs, ",")
}

func validateRuntimeCandidateManifestTargetCardinality(operation RuntimeOperation, cardinality RuntimeOperationTargetCardinality) error {
	switch cardinality {
	case "", RuntimeOperationTargetCardinalityZero:
		if cardinality == RuntimeOperationTargetCardinalityZero && len(operation.TargetRefs) != 0 {
			return fmt.Errorf("kind %q does not accept targetRefs", operation.Kind)
		}
	case RuntimeOperationTargetCardinalityOne:
		if len(operation.TargetRefs) != 1 || strings.TrimSpace(operation.TargetRefs[0]) == "" {
			return fmt.Errorf("kind %q requires exactly one targetRefs entry", operation.Kind)
		}
	case RuntimeOperationTargetCardinalityMany:
		// Many permits an empty target set. Operation-specific state-changing
		// requirements remain in the executable contract.
	default:
		return fmt.Errorf("kind %q has invalid target cardinality %q", operation.Kind, cardinality)
	}
	return nil
}

func validateRuntimeCandidateVariantAgainstManifest(manifest RuntimeCandidateOperationManifest, operation RuntimeOperation, values map[string]OperationParameter, path string) error {
	if len(manifest.Variants) == 0 {
		return nil
	}
	matches := make([]RuntimeOperationVariantContract, 0, 1)
	for _, variant := range manifest.Variants {
		if runtimeCandidatePayloadVariantMatches(values, variant) {
			matches = append(matches, variant)
		}
	}
	if len(matches) != 1 {
		return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters", "unmatched_variant", manifest.AllowedVariantIDs, nil, nil, nil)
	}
	variant := matches[0]
	for _, name := range variant.RequiredParameters {
		if _, found := values[name]; !found {
			return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters", "missing_"+name, manifest.AllowedVariantIDs, []string{name}, nil, nil)
		}
	}
	for _, name := range variant.RequiredNonEmpty {
		if value, found := values[name]; !found || !runtimeCandidatePayloadParameterHasValue(value) {
			return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters", "empty_"+name, manifest.AllowedVariantIDs, []string{name}, nil, nil)
		}
	}
	for _, name := range variant.ForbiddenParameters {
		if _, found := values[name]; found {
			return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters", name, manifest.AllowedVariantIDs, nil, []string{name}, nil)
		}
	}
	return nil
}

// coverageIDStrings is diagnostic-only. Executable candidate authorization is
// exclusively binding-based; this conversion only preserves the original raw
// field values when a direct in-process candidate is rejected.
func coverageIDStrings(values []common.CoverageWorkItemID) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		ret = append(ret, string(value))
	}
	return normalizeStrings(ret)
}

func runtimeCandidateContractDiagnostic(operationID, path, actual string, allowed, required, forbidden []string, cause error) error {
	return &RuntimeCandidateContractDiagnostic{
		OperationID: strings.TrimSpace(operationID), Path: strings.TrimSpace(path), Actual: strings.TrimSpace(actual),
		Allowed: append([]string(nil), allowed...), Required: append([]string(nil), required...), Forbidden: append([]string(nil), forbidden...), Cause: cause,
	}
}
