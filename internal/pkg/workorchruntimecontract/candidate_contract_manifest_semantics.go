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
	OperationID           string
	Path                  string
	Actual                string
	Allowed               []string
	Required              []string
	Forbidden             []string
	ConstraintRef         string
	RequiredShape         string
	OperationSetViolation *RuntimeCandidateOperationSetViolation
	Cause                 error
}

// RuntimeCandidateOperationSetViolation identifies the exact candidate-wide
// owner/target cardinality conflict. It is bounded correction evidence, not a
// request to mutate Runtime state or an instruction to a capability owner.
type RuntimeCandidateOperationSetViolation struct {
	OwnerModuleID    string   `json:"ownerModuleId"`
	TargetRef        string   `json:"targetRef"`
	OperationKinds   []string `json:"operationKinds"`
	OperationIndexes []int    `json:"operationIndexes"`
	Maximum          int      `json:"maximum"`
}

func (v RuntimeCandidateOperationSetViolation) Normalized() RuntimeCandidateOperationSetViolation {
	v.OwnerModuleID = strings.TrimSpace(v.OwnerModuleID)
	v.TargetRef = strings.TrimSpace(v.TargetRef)
	operationKinds := append([]string(nil), v.OperationKinds...)
	v.OperationKinds = make([]string, len(operationKinds))
	for index, kind := range operationKinds {
		v.OperationKinds[index] = CanonicalRuntimeBlockKind(kind)
	}
	v.OperationIndexes = append([]int(nil), v.OperationIndexes...)
	return v
}

func (v RuntimeCandidateOperationSetViolation) Validate() error {
	v = v.Normalized()
	if v.OwnerModuleID == "" || v.TargetRef == "" || v.Maximum != 1 || len(v.OperationKinds) < 2 || len(v.OperationKinds) != len(v.OperationIndexes) {
		return fmt.Errorf("runtime candidate operation-set violation is incomplete")
	}
	seenIndexes := make(map[int]struct{}, len(v.OperationIndexes))
	previousIndex := -1
	for index, kind := range v.OperationKinds {
		if kind == "" || v.OperationIndexes[index] < 0 {
			return fmt.Errorf("runtime candidate operation-set violation has invalid operation identity")
		}
		if v.OperationIndexes[index] <= previousIndex {
			return fmt.Errorf("runtime candidate operation-set violation indexes are not strictly increasing")
		}
		if _, duplicate := seenIndexes[v.OperationIndexes[index]]; duplicate {
			return fmt.Errorf("runtime candidate operation-set violation repeats operation index %d", v.OperationIndexes[index])
		}
		seenIndexes[v.OperationIndexes[index]] = struct{}{}
		previousIndex = v.OperationIndexes[index]
	}
	return nil
}

func (v *RuntimeCandidateOperationSetViolation) Clone() *RuntimeCandidateOperationSetViolation {
	if v == nil {
		return nil
	}
	value := v.Normalized()
	return &value
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
	diagnostic := &RuntimeCandidateFailureDiagnostic{
		OperationID:           value.OperationID,
		Path:                  value.Path,
		Actual:                value.Actual,
		Allowed:               append([]string(nil), value.Allowed...),
		Required:              append([]string(nil), value.Required...),
		Forbidden:             append([]string(nil), value.Forbidden...),
		ConstraintRef:         value.ConstraintRef,
		RequiredShape:         value.RequiredShape,
		OperationSetViolation: value.OperationSetViolation.Clone(),
	}
	if value.OperationSetViolation != nil {
		diagnostic.Objectives = []RuntimeCandidateCorrectionObjective{RuntimeCandidateCorrectionConsolidateOperationSet}
	}
	return diagnostic.Clone()
}

// ValidateRuntimeCandidateAgainstManifest is the complete static admission for
// one decoded candidate. It consumes only the sealed manifest; mutable Runtime
// facts and global operation descriptors cannot add a second interpretation.
// Dynamic freshness, owner feasibility and aggregate-local replay remain with
// their respective owner transactions.
func ValidateRuntimeCandidateAgainstManifest(manifest RuntimeCandidateContractManifest, candidate RuntimeCandidate) error {
	manifest = manifest.Normalized()
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" || manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) {
		return &RuntimeCandidateContractDiagnostic{Path: "$", Actual: "unsealed_manifest", Cause: fmt.Errorf("runtime candidate contract manifest is invalid")}
	}
	return validateRuntimeCandidateAgainstNormalizedManifest(manifest, candidate)
}

func validateRuntimeCandidateAgainstValidatedManifest(validated runtimeValidatedCandidateManifest, candidate RuntimeCandidate) error {
	manifest := validated.manifest
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" {
		return &RuntimeCandidateContractDiagnostic{Path: "$", Actual: "unsealed_manifest", Cause: fmt.Errorf("runtime candidate contract manifest is invalid")}
	}
	return validateRuntimeCandidateAgainstNormalizedManifest(manifest, candidate)
}

func validateRuntimeCandidateAgainstNormalizedManifest(manifest RuntimeCandidateContractManifest, candidate RuntimeCandidate) error {
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
	candidateKinds := 0
	for _, present := range []bool{candidate.StateChange != nil, candidate.Validation != nil, candidate.Assessment != nil, candidate.Governance != nil} {
		if present {
			candidateKinds++
		}
	}
	if candidateKinds != 1 {
		return &RuntimeCandidateContractDiagnostic{Path: "$", Actual: fmt.Sprintf("candidate_kind_count_%d", candidateKinds), Required: []string{"exactly_one_candidate_kind"}, Cause: fmt.Errorf("runtime candidate requires exactly one candidate kind")}
	}
	switch manifest.State {
	case ConvergenceStateNeedStateChange:
		if candidate.StateChange == nil {
			return &RuntimeCandidateContractDiagnostic{Path: "$.operations", Actual: "non_state_change_candidate", Required: []string{"state_change_candidate"}}
		}
	case ConvergenceStateNeedValidation:
		if candidate.Validation == nil {
			return &RuntimeCandidateContractDiagnostic{Path: "$.operations", Actual: "non_validation_candidate", Required: []string{"validation_candidate"}}
		}
	case ConvergenceStateNeedAssessment:
		return validateRuntimeAssessmentCandidateAgainstManifest(manifest, candidate.Assessment)
	case ConvergenceStateNeedGovernance:
		if candidate.Governance == nil {
			return &RuntimeCandidateContractDiagnostic{Path: "$.externalDependency", Actual: "non_governance_candidate", Required: []string{"governance_candidate"}}
		}
		if err := candidate.Governance.ExternalDependency.Validate(); err != nil {
			return &RuntimeCandidateContractDiagnostic{Path: "$.externalDependency", Actual: "invalid_external_dependency", Cause: err}
		}
		return nil
	default:
		return &RuntimeCandidateContractDiagnostic{Path: "$", Actual: string(manifest.State), Cause: fmt.Errorf("convergence state cannot admit an LLM candidate")}
	}
	operations, err := runtimeCandidatePayloadOperations(candidate)
	if err != nil {
		return &RuntimeCandidateContractDiagnostic{Path: "$.operations", Actual: "candidate_shape", Cause: err}
	}
	if len(operations) > manifest.MaxOperationCount {
		return &RuntimeCandidateContractDiagnostic{
			Path: "$.operations", Actual: fmt.Sprintf("operation_count_%d", len(operations)),
			Allowed: []string{fmt.Sprintf("1..%d operations", manifest.MaxOperationCount)},
			Cause:   fmt.Errorf("candidate operation count exceeds sealed maxOperationCount"),
		}
	}
	if len(operations) == 0 {
		return &RuntimeCandidateContractDiagnostic{Path: "$.operations", Actual: "operation_count_0", Required: []string{"at_least_one_operation"}, Cause: fmt.Errorf("candidate requires at least one operation")}
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
	if violation := RuntimeCandidateOperationSetViolationForManifest(manifest, operations); violation != nil {
		operationID := ""
		if last := len(violation.OperationIndexes) - 1; last >= 0 && violation.OperationIndexes[last] < len(operations) {
			operationID = operations[violation.OperationIndexes[last]].Normalized().ID
		}
		return &RuntimeCandidateContractDiagnostic{
			OperationID:           operationID,
			Path:                  "$.operations",
			Actual:                violation.OwnerModuleID + "|" + violation.TargetRef,
			Required:              []string{"one_operation_per_owner_target"},
			OperationSetViolation: violation,
			Cause:                 fmt.Errorf("candidate operations exceed the sealed owner/target cardinality"),
		}
	}
	return nil
}

func validateRuntimeAssessmentCandidateAgainstManifest(manifest RuntimeCandidateContractManifest, candidate *AssessmentCandidate) error {
	if candidate == nil {
		return &RuntimeCandidateContractDiagnostic{Path: "$.focusId", Actual: "non_assessment_candidate", Required: []string{"assessment_candidate"}}
	}
	assessment := candidate.Normalized()
	if assessment.FocusID == "" || assessment.Summary == "" {
		return &RuntimeCandidateContractDiagnostic{Path: "$", Actual: "incomplete_assessment", Required: []string{"focusId", "summary"}}
	}
	focusIDs := make(map[string]struct{})
	requiredCoverage := make(map[common.CoverageWorkItemID]struct{})
	for _, binding := range manifest.Bindings {
		binding = binding.Normalized()
		focusIDs[binding.FocusID] = struct{}{}
		requiredCoverage[binding.CoverageWorkItemID] = struct{}{}
	}
	if _, found := focusIDs[assessment.FocusID]; !found {
		allowed := make([]string, 0, len(focusIDs))
		for focusID := range focusIDs {
			allowed = append(allowed, focusID)
		}
		return &RuntimeCandidateContractDiagnostic{Path: "$.focusId", Actual: assessment.FocusID, Allowed: allowed, Cause: fmt.Errorf("assessment candidate is not bound to the manifest focus")}
	}
	if len(requiredCoverage) != 0 && !coverageIDsIntersect(requiredCoverage, assessment.CoverageWorkItemIDs) {
		required := make([]string, 0, len(requiredCoverage))
		for id := range requiredCoverage {
			required = append(required, string(id))
		}
		return &RuntimeCandidateContractDiagnostic{Path: "$.coverageWorkItemIds", Actual: "outside_manifest_bindings", Required: required, Cause: fmt.Errorf("assessment candidate is not bound to manifest coverage")}
	}
	if _, err := ValidateAssessmentDisposition(assessment.Disposition, assessment.Items, assessment.Gap); err != nil {
		return &RuntimeCandidateContractDiagnostic{Path: "$.disposition", Actual: string(assessment.Disposition), Cause: err}
	}
	return nil
}

// RuntimeCandidateOperationSetViolationForManifest evaluates only the
// candidate-wide policy sealed in the manifest. It must not consult Runtime
// facts, owner catalogs, operation descriptors, or activation policies.
func RuntimeCandidateOperationSetViolationForManifest(manifest RuntimeCandidateContractManifest, operations []RuntimeOperation) *RuntimeCandidateOperationSetViolation {
	manifest = manifest.Normalized()
	policy := manifest.OperationSetPolicy
	if policy.MaxOperationsPerOwnerTarget <= 0 {
		return nil
	}
	requiredRefs := make(map[string]struct{}, len(manifest.RequiredOperationSequence))
	for _, requirement := range manifest.RequiredOperationSequence {
		if requirement.RequirementID != "" {
			requiredRefs[requirement.RequirementID] = struct{}{}
		}
	}
	type ownerTarget struct {
		owner  string
		target string
	}
	type observed struct {
		kind     string
		index    int
		required bool
	}
	seen := make(map[ownerTarget]observed)
	for index, raw := range operations {
		operation := raw.Normalized()
		authorization, found := runtimeCandidatePayloadOperationManifest(manifest, operation.Kind)
		if !found {
			continue
		}
		_, required := requiredRefs[operation.RequiredOperationRef]
		for _, target := range operation.TargetRefs {
			key := ownerTarget{owner: authorization.OwnerModuleID, target: strings.TrimSpace(target)}
			if key.owner == "" || key.target == "" {
				continue
			}
			if previous, duplicate := seen[key]; duplicate {
				if policy.AllowRequiredOperationSequenceOverlap && previous.required && required {
					continue
				}
				return (&RuntimeCandidateOperationSetViolation{
					OwnerModuleID: key.owner, TargetRef: key.target,
					OperationKinds: []string{previous.kind, operation.Kind}, OperationIndexes: []int{previous.index, index},
					Maximum: policy.MaxOperationsPerOwnerTarget,
				}).Clone()
			}
			seen[key] = observed{kind: operation.Kind, index: index, required: required}
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
	// concern. The decoded graph is checked above against the exact dependency
	// relation derived from the manifest-required operation sequence.
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
	if err := validateRuntimeOperationTargetParameterPreimage(operation, operationManifest); err != nil {
		return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters", "target_parameter_preimage_mismatch", nil, nil, nil, err)
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
	violation, err := RuntimeOperationValueRuleViolationForOperation(operation, operationManifest)
	if err != nil {
		return runtimeCandidateContractDiagnostic(operation.ID, path+".parameters", "invalid_value_rule_catalog", nil, nil, nil, err)
	}
	if violation != nil {
		return &RuntimeCandidateContractDiagnostic{
			OperationID: operation.ID, Path: path + ".parameters[name=" + violation.ParameterName + "]",
			Actual: string(violation.ActualShape), Required: []string{string(violation.RequiredShape)},
			ConstraintRef: violation.RuleRef, RequiredShape: string(violation.RequiredShape), Cause: violation,
		}
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
