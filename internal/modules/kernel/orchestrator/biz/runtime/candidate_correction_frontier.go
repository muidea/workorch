package runtime

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	candidateadmission "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/candidateadmission"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	candidateCorrectionFieldAbsent  = runtimecontract.RuntimeCandidateCorrectionFieldAbsent
	candidateCorrectionFieldPresent = runtimecontract.RuntimeCandidateCorrectionFieldPresent
	candidateCorrectionValueAbsent  = "value_absent:"
)

// candidateCorrectionFrontier is a detached, pure view of the semantic shapes
// already unavailable to the current SM-3 transaction. It owns no aggregate
// state and performs no provider or capability-owner call.
type candidateCorrectionFrontier struct {
	candidate               *runtimecontract.RuntimeCandidate
	rejectedOperationShapes []runtimecontract.RuntimeRejectedOperationShape
	noProgressShapes        []runtimecontract.RuntimeRejectedOperationShape
}

func candidateCorrectionFrontierForEvaluation(input candidateadmission.EvaluationInput) candidateCorrectionFrontier {
	candidate := input.Candidate
	return candidateCorrectionFrontier{
		candidate:               &candidate,
		rejectedOperationShapes: runtimecontract.NormalizeRuntimeRejectedOperationShapes(input.RejectedOperationShapes),
		noProgressShapes:        runtimecontract.NormalizeRuntimeRejectedOperationShapes(input.NoProgressOperationShapes),
	}
}

func (f candidateCorrectionFrontier) blockedOperationShapes() map[string]struct{} {
	ret := make(map[string]struct{}, len(f.rejectedOperationShapes)+len(f.noProgressShapes))
	for _, values := range [][]runtimecontract.RuntimeRejectedOperationShape{f.rejectedOperationShapes, f.noProgressShapes} {
		for _, value := range runtimecontract.NormalizeRuntimeRejectedOperationShapes(values) {
			ret[value.Hash] = struct{}{}
		}
	}
	return ret
}

// candidateCorrectionWitness derives one deterministic legal difference. A
// schema witness is checked directly against required/allowed manifest fields;
// an operation witness is accepted only after the mutated candidate passes the
// same manifest and its semantic shape differs from the rejection. IDs and the
// display-only summary parameter are never enumerated.
func candidateCorrectionWitness(
	failure *runtimecontract.RuntimeCandidateFailure,
	manifest runtimecontract.RuntimeCandidateContractManifest,
	frontier candidateCorrectionFrontier,
) *runtimecontract.RuntimeCandidateCorrectionWitness {
	if failure == nil {
		return nil
	}
	manifest = manifest.Normalized()
	if strings.TrimSpace(manifest.Hash) == "" {
		return nil
	}
	if witness := candidateDocumentShapeCorrectionWitness(failure); witness != nil {
		return witness
	}
	if witness := candidateParameterEntryEncodingCorrectionWitness(failure, manifest); witness != nil {
		return witness
	}
	if witness := candidateOwnerConstraintCorrectionWitness(failure, manifest, frontier.candidate); witness != nil {
		return witness
	}
	// A strict required-field absence or exact JSON-type mismatch describes the
	// rejected wire shape, not an operation-semantic alternative. Seal that
	// directly observable fact before consulting the decoded candidate frontier;
	// otherwise an unusable decoded value can be laundered into a semantic
	// witness. Other schema diagnostics still consult the typed semantic frontier
	// first because a concrete disallowed business value is not equivalent to a
	// required-field schema defect.
	if failure.Diagnostic != nil {
		if _, requiredFieldCorrection := runtimecontract.RuntimeCandidateRequiredFieldCorrectionRejectedValue(failure.Diagnostic.Normalized()); requiredFieldCorrection {
			return candidateSchemaCorrectionWitness(failure.Diagnostic, manifest, frontier.candidate)
		}
	}
	if frontier.candidate != nil {
		candidate := *frontier.candidate
		operations := candidateadmission.Operations(candidate)
		rejectedShape := runtimecontract.StableRuntimeCandidateSemanticShape(candidate)
		blocked := frontier.blockedOperationShapes()
		for index, operation := range operations {
			operation = operation.Normalized()
			kindField := runtimecontract.RuntimeOperationKindCorrectionField()
			if candidateCorrectionAllowsOperationWitness(failure, index, kindField) {
				if witness := candidateKindCorrectionWitness(manifest, candidate, index, operation, kindField, rejectedShape, blocked); witness != nil {
					return witness
				}
			}
			operationManifest, found := candidateCorrectionOperationManifest(manifest, operation.Kind)
			if !found {
				continue
			}
			bindingField, hasBindingField, fieldErr := runtimecontract.RuntimeOperationCorrectionFieldForRole(operationManifest, runtimecontract.RuntimeOperationCorrectionRoleEvidenceBinding)
			if fieldErr != nil {
				return nil
			}
			if hasBindingField && candidateCorrectionAllowsOperationWitness(failure, index, bindingField) {
				if witness := candidateEvidenceBindingCorrectionWitness(manifest, candidate, index, operation, operationManifest, bindingField, rejectedShape, blocked); witness != nil {
					return witness
				}
			}
			targetField, hasTargetField, fieldErr := runtimecontract.RuntimeOperationCorrectionFieldForRole(operationManifest, runtimecontract.RuntimeOperationCorrectionRoleTarget)
			if fieldErr != nil {
				return nil
			}
			if hasTargetField && candidateCorrectionAllowsOperationWitness(failure, index, targetField) {
				if witness := candidateTargetCorrectionWitness(manifest, candidate, index, operation, operationManifest, targetField, rejectedShape, blocked); witness != nil {
					return witness
				}
			}
			recipeField, hasRecipeField, fieldErr := runtimecontract.RuntimeOperationCorrectionFieldForRole(operationManifest, runtimecontract.RuntimeOperationCorrectionRoleValidationRecipe)
			if fieldErr != nil {
				return nil
			}
			if hasRecipeField && candidateCorrectionAllowsOperationWitness(failure, index, recipeField) {
				if witness := candidateRecipeCorrectionWitness(manifest, candidate, index, operation, operationManifest, recipeField, rejectedShape, blocked); witness != nil {
					return witness
				}
			}
			variantField := runtimecontract.RuntimeOperationVariantCorrectionField()
			if candidateCorrectionAllowsOperationWitness(failure, index, variantField) {
				if witness := candidateVariantCorrectionWitness(manifest, candidate, index, operation, operationManifest, variantField, rejectedShape, blocked); witness != nil {
					return witness
				}
			}
			closedFields, err := runtimecontract.RuntimeOperationCorrectionFieldsForRole(operationManifest, runtimecontract.RuntimeOperationCorrectionRoleClosedParameter)
			if err != nil {
				continue
			}
			for _, closedField := range closedFields {
				if !candidateCorrectionAllowsOperationWitness(failure, index, closedField) {
					continue
				}
				if witness := candidateClosedParameterCorrectionWitness(manifest, candidate, index, operation, closedField, rejectedShape, blocked); witness != nil {
					return witness
				}
			}
		}
	}
	return candidateSchemaCorrectionWitness(failure.Diagnostic, manifest, frontier.candidate)
}

func candidateOwnerConstraintCorrectionWitness(failure *runtimecontract.RuntimeCandidateFailure, manifest runtimecontract.RuntimeCandidateContractManifest, candidate *runtimecontract.RuntimeCandidate) *runtimecontract.RuntimeCandidateCorrectionWitness {
	if failure == nil || strings.TrimSpace(failure.Code) != "candidate_owner_constraint_nonconformant" || failure.Diagnostic == nil || candidate == nil {
		return nil
	}
	diagnostic := failure.Diagnostic.Normalized()
	operationIndex, found := candidateCorrectionOperationIndex(diagnostic.Path)
	operations := candidateadmission.Operations(*candidate)
	if !found || operationIndex < 0 || operationIndex >= len(operations) {
		return nil
	}
	operation := operations[operationIndex].Normalized()
	constraint, found := runtimecontract.RuntimeCandidateConstraintForOperationRef(manifest, operation.Kind, diagnostic.ConstraintRef)
	if !found || diagnostic.OperationID != operation.ID || constraint.ParameterName != candidateCorrectionPathField(diagnostic.Path) || string(constraint.RequiredShape) != diagnostic.RequiredShape || diagnostic.Actual == "" || diagnostic.RejectedShapeHash != "" {
		return nil
	}
	witness := &runtimecontract.RuntimeCandidateCorrectionWitness{
		Dimension:     runtimecontract.RuntimeCandidateCorrectionWitnessOwnerConstraint,
		OperationKind: operation.Kind, Path: diagnostic.Path, ConstraintRef: diagnostic.ConstraintRef,
		RejectedValue: diagnostic.Actual, AlternativeValue: diagnostic.RequiredShape,
	}
	if witness.Validate() != nil {
		return nil
	}
	return witness.Clone()
}

func candidateParameterEntryEncodingCorrectionWitness(failure *runtimecontract.RuntimeCandidateFailure, manifest runtimecontract.RuntimeCandidateContractManifest) *runtimecontract.RuntimeCandidateCorrectionWitness {
	if failure == nil || strings.TrimSpace(failure.Code) != "candidate_schema_invalid" || failure.Diagnostic == nil {
		return nil
	}
	diagnostic := failure.Diagnostic.Normalized()
	if diagnostic.ViolationKind != runtimecontract.RuntimeCandidatePayloadViolationParameterEntryEncoding ||
		len(diagnostic.ParameterEntryEncodings) == 0 ||
		runtimecontract.ValidateRuntimeCandidateParameterEntryEncodings(manifest, diagnostic.ParameterEntryEncodings) != nil {
		return nil
	}
	witness := &runtimecontract.RuntimeCandidateCorrectionWitness{
		Dimension:               runtimecontract.RuntimeCandidateCorrectionWitnessParameterEntryEncoding,
		Path:                    "$.operations",
		RejectedValue:           runtimecontract.RuntimeCandidateParameterEncodingInlineSiblings,
		AlternativeValue:        runtimecontract.RuntimeCandidateParameterEncodingNamedEntries,
		ParameterEntryEncodings: diagnostic.ParameterEntryEncodings,
	}
	if witness.Validate() != nil {
		return nil
	}
	return witness.Clone()
}

func candidateDocumentShapeCorrectionWitness(failure *runtimecontract.RuntimeCandidateFailure) *runtimecontract.RuntimeCandidateCorrectionWitness {
	if failure == nil || strings.TrimSpace(failure.Code) != "candidate_document_nonconformant" || failure.Diagnostic == nil {
		return nil
	}
	diagnostic := failure.Diagnostic.Normalized()
	if diagnostic.Path != "$" {
		return nil
	}
	witness := &runtimecontract.RuntimeCandidateCorrectionWitness{
		Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessDocumentShape,
		Path:      "$", RejectedValue: diagnostic.Actual,
		AlternativeValue: runtimecontract.RuntimeCandidateDocumentShapeConformant,
	}
	if witness.Validate() != nil {
		return nil
	}
	return witness
}

// candidateCorrectionAllowsOperationWitness keeps a legal manifest mutation
// tied to the rejection that authorized correction. Replay/no-progress
// failures may choose any deterministic unblocked semantic shape; every other
// failure must name the exact semantic dimension and operation index. This
// prevents an unrelated target/recipe mutation from laundering a schema
// rejection into attempt-1 authority.
func candidateCorrectionAllowsOperationWitness(failure *runtimecontract.RuntimeCandidateFailure, operationIndex int, field runtimecontract.RuntimeOperationCorrectionField) bool {
	if failure == nil {
		return false
	}
	switch strings.TrimSpace(failure.Code) {
	case "candidate_replays_rejected_shape", "candidate_partially_replays_rejected_shape", "candidate_replays_no_progress_shape":
		return true
	}
	if failure.Diagnostic == nil {
		return false
	}
	if candidateCorrectionDiagnosticReportsFieldAbsence(failure.Diagnostic.Normalized()) {
		return false
	}
	path := strings.TrimSpace(failure.Diagnostic.Path)
	if path == "" || path != field.CandidatePath(operationIndex) {
		return false
	}
	if field.WitnessDimension() == runtimecontract.RuntimeCandidateCorrectionWitnessVariant {
		return strings.TrimSpace(failure.Diagnostic.Actual) == "unmatched_variant"
	}
	return field.WitnessDimension() != ""
}

func candidateSchemaCorrectionWitness(diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic, manifest runtimecontract.RuntimeCandidateContractManifest, candidate *runtimecontract.RuntimeCandidate) *runtimecontract.RuntimeCandidateCorrectionWitness {
	if diagnostic == nil {
		return nil
	}
	diagnostic = diagnostic.Clone()
	path := strings.TrimSpace(diagnostic.Path)
	if path == "" {
		return nil
	}
	rejectedRequiredFieldValue, reportsRequiredFieldDefect := runtimecontract.RuntimeCandidateRequiredFieldCorrectionRejectedValue(diagnostic.Normalized())
	field := candidateCorrectionPathField(path)
	if field == "" || field == "summary" {
		return nil
	}
	// Operation identity/dependency fields are canonical manifest projections,
	// not provider-authored authority. A strict wire diagnostic may therefore
	// authorize only their removal; it must never ask attempt 1 to invent or
	// mutate the canonical values derived after decoding.
	if (field == "id" || field == "dependsOn") && strings.HasPrefix(path, "$.operations[") {
		if candidateCorrectionDiagnosticForbids(diagnostic, field) {
			return schemaCorrectionWitness(path, field, candidateCorrectionFieldAbsent)
		}
		return nil
	}
	if candidateCorrectionPathIsDisplayOnly(path) {
		return nil
	}
	if strings.HasPrefix(path, "$.") && !strings.HasPrefix(path, "$.operations[") {
		if containsCandidateCorrectionString(manifest.RequiredTopLevelFields, field) && reportsRequiredFieldDefect {
			return schemaCorrectionWitness(path, rejectedRequiredFieldValue, candidateCorrectionFieldPresent)
		}
		if !containsCandidateCorrectionString(manifest.AllowedTopLevelFields, field) && candidateCorrectionDiagnosticForbids(diagnostic, field) {
			return schemaCorrectionWitness(path, field, candidateCorrectionFieldAbsent)
		}
		return nil
	}
	if witness := candidateDecodedSchemaCorrectionWitness(diagnostic, manifest, candidate); witness != nil {
		return witness
	}
	operationManifest, found := candidateCorrectionDiagnosticOperationManifest(manifest, diagnostic)
	if !found {
		return nil
	}
	if !strings.Contains(path, ".parameters") {
		if reportsRequiredFieldDefect &&
			containsCandidateCorrectionString(operationManifest.AllowedOperationFields, field) &&
			candidateCorrectionDiagnosticRequires(diagnostic, field) {
			return schemaCorrectionWitness(path, rejectedRequiredFieldValue, candidateCorrectionFieldPresent)
		}
		return nil
	}
	if field == "summary" {
		return nil
	}
	if reportsRequiredFieldDefect && field == "parameters" &&
		containsCandidateCorrectionString(operationManifest.RequiredOperationFields, field) {
		return schemaCorrectionWitness(path, rejectedRequiredFieldValue, candidateCorrectionFieldPresent)
	}
	if reportsRequiredFieldDefect && candidateCorrectionParameterDeclared(operationManifest, field) {
		return schemaCorrectionWitness(path, rejectedRequiredFieldValue, candidateCorrectionFieldPresent)
	}
	if candidateCorrectionDiagnosticForbids(diagnostic, field) && candidateCorrectionParameterRemovalAllowed(operationManifest, field) {
		return schemaCorrectionWitness(path, field, candidateCorrectionFieldAbsent)
	}
	return nil
}

func schemaCorrectionWitness(path, rejected, alternative string) *runtimecontract.RuntimeCandidateCorrectionWitness {
	return &runtimecontract.RuntimeCandidateCorrectionWitness{
		Dimension: runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField,
		Path:      path, RejectedValue: rejected, AlternativeValue: alternative,
	}
}

// candidateDecodedSchemaCorrectionWitness handles decoded fields whose legal
// correction is a concrete removal. The resulting candidate must itself pass
// the sealed manifest, so a diagnostic alone cannot fabricate retry authority.
func candidateDecodedSchemaCorrectionWitness(diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic, manifest runtimecontract.RuntimeCandidateContractManifest, candidate *runtimecontract.RuntimeCandidate) *runtimecontract.RuntimeCandidateCorrectionWitness {
	if diagnostic == nil || candidate == nil {
		return nil
	}
	index, found := candidateCorrectionOperationIndex(diagnostic.Path)
	operations := candidateadmission.Operations(*candidate)
	if !found || index < 0 || index >= len(operations) {
		return nil
	}
	original := operations[index].Normalized()
	mutated := original
	alternative := ""
	switch {
	case strings.HasSuffix(diagnostic.Path, ".coverageWorkItemIds"):
		if len(mutated.CoverageWorkItemIDs) == 0 {
			return nil
		}
		mutated.CoverageWorkItemIDs = nil
		alternative = candidateCorrectionFieldAbsent
	case strings.HasSuffix(diagnostic.Path, ".dependsOn"):
		if len(mutated.DependsOn) == 0 {
			return nil
		}
		actual := strings.TrimSpace(diagnostic.Actual)
		if len(mutated.DependsOn) == 1 && (actual == "" || mutated.DependsOn[0] == actual) {
			mutated.DependsOn = nil
			alternative = candidateCorrectionFieldAbsent
		} else {
			ret := make([]string, 0, len(mutated.DependsOn))
			for _, dependency := range mutated.DependsOn {
				if dependency != actual {
					ret = append(ret, dependency)
				}
			}
			if len(ret) == len(mutated.DependsOn) {
				return nil
			}
			mutated.DependsOn = ret
			alternative = candidateCorrectionValueAbsent + actual
		}
	case strings.Contains(diagnostic.Path, ".parameters[name=") && candidateCorrectionDiagnosticForbids(diagnostic, candidateCorrectionPathField(diagnostic.Path)):
		name := candidateCorrectionPathField(diagnostic.Path)
		ret := make([]runtimecontract.OperationParameter, 0, len(mutated.Parameters))
		for _, parameter := range mutated.Parameters {
			if strings.TrimSpace(parameter.Name) != name {
				ret = append(ret, parameter)
			}
		}
		if len(ret) == len(mutated.Parameters) {
			return nil
		}
		mutated.Parameters = ret
		alternative = candidateCorrectionFieldAbsent
	default:
		return nil
	}
	updated, ok := candidateCorrectionReplaceOperation(*candidate, index, mutated)
	if !ok {
		return nil
	}
	updated, err := runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, updated)
	if err != nil || runtimecontract.ValidateRuntimeCandidateAgainstManifest(manifest, updated) != nil {
		return nil
	}
	witness := schemaCorrectionWitness(diagnostic.Path, diagnostic.Actual, alternative)
	rejectedShape := runtimecontract.StableRuntimeCandidateSemanticShape(*candidate)
	alternativeShape := runtimecontract.StableRuntimeCandidateSemanticShape(updated)
	if rejectedShape != alternativeShape {
		witness.RejectedShapeHash = rejectedShape
		witness.AlternativeShapeHash = alternativeShape
	}
	if witness.Validate() != nil {
		return nil
	}
	return witness
}

func candidateTargetCorrectionWitness(manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate, index int, operation runtimecontract.RuntimeOperation, authorization runtimecontract.RuntimeCandidateOperationManifest, field runtimecontract.RuntimeOperationCorrectionField, rejectedShape string, blocked map[string]struct{}) *runtimecontract.RuntimeCandidateCorrectionWitness {
	allowed := authorization.Target.AllowedTargetRefs()
	if len(allowed) < 2 && containsAllCandidateCorrectionStrings(allowed, operation.TargetRefs) {
		return nil
	}
	current := ""
	if len(operation.TargetRefs) > 0 {
		current = operation.TargetRefs[0]
	}
	for _, alternative := range allowed {
		if alternative == "" || containsCandidateCorrectionString(operation.TargetRefs, alternative) {
			continue
		}
		mutated := operation
		switch authorization.TargetCardinality {
		case runtimecontract.RuntimeOperationTargetCardinalityOne:
			mutated.TargetRefs = []string{alternative}
		case runtimecontract.RuntimeOperationTargetCardinalityMany:
			mutated.TargetRefs = append([]string(nil), operation.TargetRefs...)
			if len(mutated.TargetRefs) == 0 {
				mutated.TargetRefs = []string{alternative}
			} else {
				mutated.TargetRefs[0] = alternative
			}
		default:
			continue
		}
		if witness := candidateOperationMutationWitness(manifest, candidate, index, operation, mutated, field, current, alternative, rejectedShape, blocked); witness != nil {
			return witness
		}
	}
	return nil
}

func candidateEvidenceBindingCorrectionWitness(manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate, index int, operation runtimecontract.RuntimeOperation, operationManifest runtimecontract.RuntimeCandidateOperationManifest, field runtimecontract.RuntimeOperationCorrectionField, rejectedShape string, blocked map[string]struct{}) *runtimecontract.RuntimeCandidateCorrectionWitness {
	authorization, err := runtimecontract.RuntimeCandidateOperationBindingAuthorizationFor(operationManifest, operation)
	if err != nil || len(authorization.AllowedBindingRefs) == 0 {
		return nil
	}
	current := strings.Join(operation.BindingRefs, ",")
	for _, alternative := range authorization.AllowedBindingRefs {
		if alternative == "" || containsCandidateCorrectionString(operation.BindingRefs, alternative) {
			continue
		}
		mutated := operation
		mutated.BindingRefs = []string{alternative}
		if witness := candidateOperationMutationWitness(manifest, candidate, index, operation, mutated, field, current, alternative, rejectedShape, blocked); witness != nil {
			return witness
		}
	}
	return nil
}

func candidateRecipeCorrectionWitness(manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate, index int, operation runtimecontract.RuntimeOperation, authorization runtimecontract.RuntimeCandidateOperationManifest, field runtimecontract.RuntimeOperationCorrectionField, rejectedShape string, blocked map[string]struct{}) *runtimecontract.RuntimeCandidateCorrectionWitness {
	if len(authorization.ValidationRecipes) == 0 {
		return nil
	}
	current, _ := candidateCorrectionStringParameter(operation.Parameters, field.Name)
	for _, recipe := range authorization.ValidationRecipes {
		if recipe.ID == "" || recipe.ID == current {
			continue
		}
		mutated := operation
		mutated.Parameters = candidateCorrectionSetStringParameter(mutated.Parameters, field.Name, recipe.ID)
		if witness := candidateOperationMutationWitness(manifest, candidate, index, operation, mutated, field, current, recipe.ID, rejectedShape, blocked); witness != nil {
			return witness
		}
	}
	return nil
}

func candidateVariantCorrectionWitness(manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate, index int, operation runtimecontract.RuntimeOperation, authorization runtimecontract.RuntimeCandidateOperationManifest, field runtimecontract.RuntimeOperationCorrectionField, rejectedShape string, blocked map[string]struct{}) *runtimecontract.RuntimeCandidateCorrectionWitness {
	if len(authorization.Variants) == 0 {
		return nil
	}
	contract, found := runtimecontract.RuntimeOperationContractForKind(operation.Kind)
	if !found {
		return nil
	}
	current, _ := runtimecontract.MatchRuntimeOperationVariant(operation, contract)
	rejectedValue := current.ID
	if rejectedValue == "" {
		rejectedValue = "unmatched_variant"
	}
	for _, variant := range authorization.Variants {
		if variant.ID == "" || variant.ID == current.ID || len(variant.Discriminators) == 0 {
			continue
		}
		mutated := operation
		for _, discriminator := range variant.Discriminators {
			mutated.Parameters = candidateCorrectionSetStringParameter(mutated.Parameters, discriminator.Name, discriminator.EqualsString)
		}
		if witness := candidateOperationMutationWitness(manifest, candidate, index, operation, mutated, field, rejectedValue, variant.ID, rejectedShape, blocked); witness != nil {
			return witness
		}
	}
	return nil
}

func candidateClosedParameterCorrectionWitness(manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate, index int, operation runtimecontract.RuntimeOperation, field runtimecontract.RuntimeOperationCorrectionField, rejectedShape string, blocked map[string]struct{}) *runtimecontract.RuntimeCandidateCorrectionWitness {
	current, found := candidateCorrectionBooleanParameter(operation.Parameters, field.Name)
	if !found {
		return nil
	}
	mutated := operation
	mutated.Parameters = candidateCorrectionSetBooleanParameter(mutated.Parameters, field.Name, !current)
	if witness := candidateOperationMutationWitness(manifest, candidate, index, operation, mutated, field, fmt.Sprintf("%t", current), fmt.Sprintf("%t", !current), rejectedShape, blocked); witness != nil {
		return witness
	}
	return nil
}

func candidateKindCorrectionWitness(manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate, index int, operation runtimecontract.RuntimeOperation, field runtimecontract.RuntimeOperationCorrectionField, rejectedShape string, blocked map[string]struct{}) *runtimecontract.RuntimeCandidateCorrectionWitness {
	for _, authorization := range manifest.Operations {
		if authorization.Kind == "" || authorization.Kind == operation.Kind {
			continue
		}
		mutated := operation
		mutated.Kind = authorization.Kind
		if witness := candidateOperationMutationWitness(manifest, candidate, index, operation, mutated, field, operation.Kind, authorization.Kind, rejectedShape, blocked); witness != nil {
			return witness
		}
	}
	return nil
}

func candidateOperationMutationWitness(manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate, index int, original, mutated runtimecontract.RuntimeOperation, field runtimecontract.RuntimeOperationCorrectionField, rejected, alternative, rejectedShape string, blocked map[string]struct{}) *runtimecontract.RuntimeCandidateCorrectionWitness {
	updated, ok := candidateCorrectionReplaceOperation(candidate, index, mutated)
	if !ok {
		return nil
	}
	updated, err := runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, updated)
	if err != nil || runtimecontract.ValidateRuntimeCandidateAgainstManifest(manifest, updated) != nil {
		return nil
	}
	operationShape := runtimecontract.StableRuntimeOperationShape(mutated)
	if _, found := blocked[operationShape]; found {
		return nil
	}
	alternativeShape := runtimecontract.StableRuntimeCandidateSemanticShape(updated)
	if rejectedShape == "" || alternativeShape == "" || rejectedShape == alternativeShape {
		return nil
	}
	witness := &runtimecontract.RuntimeCandidateCorrectionWitness{
		Dimension: field.WitnessDimension(), OperationKind: original.Kind, Path: field.CandidatePath(index),
		RejectedValue: rejected, AlternativeValue: alternative,
		RejectedShapeHash: rejectedShape, AlternativeShapeHash: alternativeShape,
	}
	if witness.Validate() != nil {
		return nil
	}
	return witness
}

func candidateCorrectionReplaceOperation(candidate runtimecontract.RuntimeCandidate, index int, operation runtimecontract.RuntimeOperation) (runtimecontract.RuntimeCandidate, bool) {
	switch {
	case candidate.StateChange != nil && index >= 0 && index < len(candidate.StateChange.Operations):
		value := *candidate.StateChange
		value.Operations = append([]runtimecontract.StateChangingOperation(nil), value.Operations...)
		value.Operations[index].Operation = operation
		candidate.StateChange = &value
		return candidate, true
	case candidate.Validation != nil && index >= 0 && index < len(candidate.Validation.Operations):
		value := *candidate.Validation
		value.Operations = append([]runtimecontract.ValidationOperation(nil), value.Operations...)
		value.Operations[index].Operation = operation
		candidate.Validation = &value
		return candidate, true
	default:
		return runtimecontract.RuntimeCandidate{}, false
	}
}

func candidateCorrectionSetStringParameter(parameters []runtimecontract.OperationParameter, name, value string) []runtimecontract.OperationParameter {
	ret := append([]runtimecontract.OperationParameter(nil), parameters...)
	for index := range ret {
		if strings.TrimSpace(ret[index].Name) == name {
			valueCopy := value
			ret[index] = runtimecontract.OperationParameter{Name: name, String: &valueCopy}
			return ret
		}
	}
	valueCopy := value
	return append(ret, runtimecontract.OperationParameter{Name: name, String: &valueCopy})
}

func candidateCorrectionSetBooleanParameter(parameters []runtimecontract.OperationParameter, name string, value bool) []runtimecontract.OperationParameter {
	ret := append([]runtimecontract.OperationParameter(nil), parameters...)
	for index := range ret {
		if strings.TrimSpace(ret[index].Name) == name {
			valueCopy := value
			ret[index] = runtimecontract.OperationParameter{Name: name, Boolean: &valueCopy}
			return ret
		}
	}
	valueCopy := value
	return append(ret, runtimecontract.OperationParameter{Name: name, Boolean: &valueCopy})
}

func candidateCorrectionStringParameter(parameters []runtimecontract.OperationParameter, name string) (string, bool) {
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter.Name) == name && parameter.String != nil {
			return strings.TrimSpace(*parameter.String), true
		}
	}
	return "", false
}

func candidateCorrectionBooleanParameter(parameters []runtimecontract.OperationParameter, name string) (bool, bool) {
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter.Name) == name && parameter.Boolean != nil {
			return *parameter.Boolean, true
		}
	}
	return false, false
}

func candidateCorrectionOperationManifest(manifest runtimecontract.RuntimeCandidateContractManifest, kind string) (runtimecontract.RuntimeCandidateOperationManifest, bool) {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	for _, value := range manifest.Operations {
		value = value.Normalized()
		if value.Kind == kind {
			return value, true
		}
	}
	return runtimecontract.RuntimeCandidateOperationManifest{}, false
}

func candidateCorrectionDiagnosticOperationManifest(manifest runtimecontract.RuntimeCandidateContractManifest, diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic) (runtimecontract.RuntimeCandidateOperationManifest, bool) {
	if diagnostic != nil {
		actual := runtimecontract.CanonicalRuntimeBlockKind(diagnostic.Actual)
		if value, found := candidateCorrectionOperationManifest(manifest, actual); found {
			return value, true
		}
	}
	if len(manifest.Operations) == 1 {
		return manifest.Operations[0].Normalized(), true
	}
	return runtimecontract.RuntimeCandidateOperationManifest{}, false
}

func candidateCorrectionParameterDeclared(manifest runtimecontract.RuntimeCandidateOperationManifest, name string) bool {
	for _, parameter := range manifest.Parameters {
		if strings.TrimSpace(parameter.Name) == name {
			return true
		}
	}
	return false
}

func candidateCorrectionParameterRemovalAllowed(manifest runtimecontract.RuntimeCandidateOperationManifest, name string) bool {
	for _, parameter := range manifest.Parameters {
		if strings.TrimSpace(parameter.Name) == name {
			// Optional means the field may be absent; it does not mean the field is
			// forbidden when present. Only an explicit sealed prohibition can
			// authorize removing a declared parameter.
			return containsCandidateCorrectionString(manifest.ForbiddenParameters, name)
		}
	}
	return !manifest.AllowsDynamicParameters || !candidateCorrectionDynamicParameter(name, manifest.DynamicParameterPrefixes)
}

func candidateCorrectionDynamicParameter(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, strings.TrimSpace(prefix)) {
			return true
		}
	}
	return false
}

func candidateCorrectionDiagnosticRequires(diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic, field string) bool {
	if diagnostic == nil {
		return false
	}
	field = strings.TrimSpace(field)
	if field == "" {
		return false
	}
	if containsCandidateCorrectionString(diagnostic.Required, field) {
		return true
	}
	actual := strings.TrimSpace(diagnostic.Actual)
	for _, prefix := range []string{"missing_", "empty_"} {
		if strings.HasPrefix(actual, prefix) && candidateCorrectionFieldTokenEqual(strings.TrimPrefix(actual, prefix), field) {
			return true
		}
	}
	return false
}

func candidateCorrectionFieldTokenEqual(left, right string) bool {
	normalize := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		value = strings.ReplaceAll(value, "_", "")
		value = strings.ReplaceAll(value, "-", "")
		return value
	}
	return normalize(left) != "" && normalize(left) == normalize(right)
}

func candidateCorrectionDiagnosticForbids(diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic, field string) bool {
	if diagnostic == nil {
		return false
	}
	return containsCandidateCorrectionString(diagnostic.Forbidden, field) || (diagnostic.Actual == field && len(diagnostic.Allowed) != 0 && !containsCandidateCorrectionString(diagnostic.Allowed, field))
}

func candidateCorrectionPathField(path string) string {
	path = strings.TrimSpace(path)
	if marker := strings.LastIndex(path, "[name="); marker >= 0 {
		value := path[marker+len("[name="):]
		if end := strings.Index(value, "]"); end >= 0 {
			return strings.TrimSpace(value[:end])
		}
	}
	if dot := strings.LastIndex(path, "."); dot >= 0 && dot+1 < len(path) {
		return strings.TrimSpace(path[dot+1:])
	}
	return ""
}

func candidateCorrectionPathIsDisplayOnly(path string) bool {
	path = strings.ToLower(strings.TrimSpace(path))
	return strings.Contains(path, "[name=summary]") || strings.HasSuffix(path, ".summary") || strings.HasSuffix(path, ".id")
}

func candidateCorrectionRemovesRawProviderIdentity(witness runtimecontract.RuntimeCandidateCorrectionWitness) bool {
	witness = witness.Normalized()
	if witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField ||
		witness.AlternativeValue != candidateCorrectionFieldAbsent {
		return false
	}
	field := candidateCorrectionPathField(witness.Path)
	return strings.HasPrefix(witness.Path, "$.operations[") &&
		(field == "id" || field == "dependsOn") && witness.RejectedValue == field
}

func containsCandidateCorrectionString(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

func containsAllCandidateCorrectionStrings(allowed, actual []string) bool {
	if len(actual) == 0 {
		return false
	}
	for _, value := range actual {
		if !containsCandidateCorrectionString(allowed, value) {
			return false
		}
	}
	return true
}

func sameCandidateCorrectionDirective(left, right *runtimecontract.RuntimeCandidateCorrectionDirective) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return reflect.DeepEqual(left.Normalized(), right.Normalized())
}

// validatePersistedCandidateCorrection re-derives the exact witness from the
// rebuilt manifest and current finite frontier. This is the restart/attempt-1
// fence: a stale hash, removed allowed value, or now-blocked alternative cannot
// allocate a request session or reach the provider.
func validatePersistedCandidateCorrection(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest) error {
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	var first *runtimecontract.RuntimeCandidateAttemptRecord
	for index := range aggregate.CandidateAttempts {
		record := &aggregate.CandidateAttempts[index]
		if record.Attempt == 0 && runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, *record) == contractID {
			first = record
			break
		}
	}
	if first == nil || first.State != runtimecontract.RuntimeCandidateAttemptStateFailed || first.Failure == nil || first.Correction == nil {
		return fmt.Errorf("candidate correction witness is missing from failed attempt 0")
	}
	if first.ManifestHash != manifest.Hash || first.Correction.ManifestHash != manifest.Hash {
		return fmt.Errorf("candidate correction witness manifest identity changed")
	}
	input := candidateAdmissionEvaluationInput(aggregate, manifest, runtimecontract.RuntimeCandidate{})
	frontier := candidateCorrectionFrontierForEvaluation(input)
	if first.Proposal != nil {
		candidate := *first.Proposal
		frontier.candidate = &candidate
	} else {
		frontier.candidate = nil
	}
	_, expected, err := sealCandidateRejectionWithFrontier(first.Failure, manifest, frontier)
	if err != nil {
		return err
	}
	if expected == nil || expected.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce || !sameCandidateCorrectionDirective(first.Correction, expected) {
		return fmt.Errorf("candidate correction witness is no longer legal under the sealed manifest")
	}
	return nil
}

// ValidatePersistedCandidateCorrection is the production pre-provider SM-3
// guard. The exported surface remains a pure aggregate/manifest validation;
// only the Orchestrator mainline decides the owner-local terminal transition.
func ValidatePersistedCandidateCorrection(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest) error {
	return validatePersistedCandidateCorrection(aggregate, manifest)
}

// validateAttemptCandidateCrossesPersistedWitness proves that the actual
// attempt-1 candidate uses the exact alternative sealed on attempt 0. A model
// cannot satisfy this fence by changing operation IDs, summaries, or some
// unrelated manifest-valid field.
func validateAttemptCandidateCrossesPersistedWitness(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeCandidateContractManifest, candidate runtimecontract.RuntimeCandidate) error {
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Decision.AuthorityTransaction())
	var first *runtimecontract.RuntimeCandidateAttemptRecord
	correctionPending := false
	for index := range aggregate.CandidateAttempts {
		record := &aggregate.CandidateAttempts[index]
		if runtimecontract.CandidateAttemptAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, *record) != contractID {
			continue
		}
		if record.Attempt == 0 {
			first = record
		}
		if record.Attempt == 1 && record.State == runtimecontract.RuntimeCandidateAttemptStatePending {
			correctionPending = true
		}
	}
	if !correctionPending {
		return nil
	}
	if first == nil || first.Failure == nil || first.Correction == nil {
		return fmt.Errorf("attempt 1 has no persisted correction witness")
	}
	if err := validatePersistedCandidateCorrection(aggregate, manifest); err != nil {
		return err
	}
	directive := first.Correction.Normalized()
	if directive.Witness == nil {
		return fmt.Errorf("attempt 1 has no concrete correction witness")
	}
	shape := runtimecontract.StableRuntimeCandidateSemanticShape(candidate)
	if directive.Witness.RejectedShapeHash != "" && shape == directive.Witness.RejectedShapeHash {
		return fmt.Errorf("attempt 1 repeats the rejected semantic candidate shape")
	}
	if directive.Witness.AlternativeShapeHash != "" && shape != directive.Witness.AlternativeShapeHash {
		return fmt.Errorf("attempt 1 does not match the sealed alternative semantic shape")
	}
	if directive.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessOwnerConstraint {
		if first.Proposal == nil || !candidateCrossesOwnerConstraintWitness(*first.Proposal, candidate, manifest, *directive.Witness) {
			return fmt.Errorf("attempt 1 does not revise the sealed owner constraint %q at %s", directive.Witness.ConstraintRef, directive.Witness.Path)
		}
		// Only the capability owner can prove that the revised opaque parameter
		// has the required representation. The later conformance read lane is
		// therefore the exact alternative-value fence; this pre-owner gate proves
		// only same manifest/path plus a real change to the constrained value.
		return nil
	}
	if !candidateCrossesCorrectionWitness(candidate, manifest, *directive.Witness) {
		return fmt.Errorf("attempt 1 does not use sealed correction alternative %q at %s", directive.Witness.AlternativeValue, directive.Witness.Path)
	}
	return nil
}

func candidateCrossesOwnerConstraintWitness(previous runtimecontract.RuntimeCandidate, candidate runtimecontract.RuntimeCandidate, manifest runtimecontract.RuntimeCandidateContractManifest, witness runtimecontract.RuntimeCandidateCorrectionWitness) bool {
	witness = witness.Normalized()
	if witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessOwnerConstraint || witness.Validate() != nil {
		return false
	}
	operationIndex, found := candidateCorrectionOperationIndex(witness.Path)
	if !found {
		return false
	}
	previousOperations := candidateadmission.Operations(previous)
	operations := candidateadmission.Operations(candidate)
	if operationIndex < 0 || operationIndex >= len(previousOperations) || operationIndex >= len(operations) {
		return false
	}
	previousOperation := previousOperations[operationIndex].Normalized()
	operation := operations[operationIndex].Normalized()
	kind := runtimecontract.CanonicalRuntimeBlockKind(witness.OperationKind)
	constraint, found := runtimecontract.RuntimeCandidateConstraintForOperationRef(manifest, kind, witness.ConstraintRef)
	if !found || operation.Kind != kind || previousOperation.Kind != kind ||
		witness.Path != fmt.Sprintf("$.operations[%d].parameters[name=%s]", operationIndex, constraint.ParameterName) ||
		witness.AlternativeValue != string(constraint.RequiredShape) {
		return false
	}
	authorization, found := candidateCorrectionOperationManifest(manifest, operation.Kind)
	if !found {
		return false
	}
	_, constraints, err := runtimecontract.RuntimeOperationCandidateConstraintsForOperation(operation, authorization)
	if err != nil {
		return false
	}
	constraintSealed := false
	for _, selected := range constraints {
		if selected == constraint {
			constraintSealed = true
			break
		}
	}
	if !constraintSealed {
		return false
	}
	previousValue, previousFound := candidateCorrectionStringParameter(previousOperation.Parameters, constraint.ParameterName)
	value, valueFound := candidateCorrectionStringParameter(operation.Parameters, constraint.ParameterName)
	return previousFound && valueFound && previousValue != value
}

func candidateCrossesCorrectionWitness(candidate runtimecontract.RuntimeCandidate, manifest runtimecontract.RuntimeCandidateContractManifest, witness runtimecontract.RuntimeCandidateCorrectionWitness) bool {
	witness = witness.Normalized()
	if witness.Validate() != nil || (candidateCorrectionPathIsDisplayOnly(witness.Path) && !candidateCorrectionRemovesRawProviderIdentity(witness)) {
		return false
	}
	operations := candidateadmission.Operations(candidate)
	operationIndex, hasOperationIndex := candidateCorrectionOperationIndex(witness.Path)
	if hasOperationIndex && (operationIndex < 0 || operationIndex >= len(operations)) {
		return false
	}
	var operation runtimecontract.RuntimeOperation
	if hasOperationIndex {
		operation = operations[operationIndex].Normalized()
	}
	switch witness.Dimension {
	case runtimecontract.RuntimeCandidateCorrectionWitnessDocumentShape:
		// This function is reached only after strict decoding against the same
		// sealed manifest. That admission boundary is the proof that attempt 1
		// crossed from an empty/invalid/trailing body to one conformant document.
		return witness.Path == "$" && witness.AlternativeValue == runtimecontract.RuntimeCandidateDocumentShapeConformant
	case runtimecontract.RuntimeCandidateCorrectionWitnessParameterEntryEncoding:
		return candidateCrossesParameterEntryEncodingWitness(operations, manifest, witness)
	case runtimecontract.RuntimeCandidateCorrectionWitnessOperationKind:
		return hasOperationIndex && operation.Kind == runtimecontract.CanonicalRuntimeBlockKind(witness.AlternativeValue)
	case runtimecontract.RuntimeCandidateCorrectionWitnessTarget:
		if !hasOperationIndex || operation.Kind != runtimecontract.CanonicalRuntimeBlockKind(witness.OperationKind) {
			return false
		}
		authorization, found := candidateCorrectionOperationManifest(manifest, operation.Kind)
		if !found {
			return false
		}
		field, found, err := runtimecontract.RuntimeOperationCorrectionFieldForRole(authorization, runtimecontract.RuntimeOperationCorrectionRoleTarget)
		return err == nil && found && witness.Path == field.CandidatePath(operationIndex) && containsCandidateCorrectionString(operation.TargetRefs, witness.AlternativeValue)
	case runtimecontract.RuntimeCandidateCorrectionWitnessEvidenceBinding:
		if !hasOperationIndex || operation.Kind != runtimecontract.CanonicalRuntimeBlockKind(witness.OperationKind) {
			return false
		}
		operationManifest, found := candidateCorrectionOperationManifest(manifest, operation.Kind)
		if !found {
			return false
		}
		field, found, err := runtimecontract.RuntimeOperationCorrectionFieldForRole(operationManifest, runtimecontract.RuntimeOperationCorrectionRoleEvidenceBinding)
		if err != nil || !found || witness.Path != field.CandidatePath(operationIndex) {
			return false
		}
		authorization, err := runtimecontract.RuntimeCandidateOperationBindingAuthorizationFor(operationManifest, operation)
		return err == nil && containsCandidateCorrectionString(authorization.AllowedBindingRefs, witness.AlternativeValue) &&
			len(operation.BindingRefs) == 1 && operation.BindingRefs[0] == witness.AlternativeValue
	case runtimecontract.RuntimeCandidateCorrectionWitnessValidationRecipe:
		if !hasOperationIndex || operation.Kind != runtimecontract.CanonicalRuntimeBlockKind(witness.OperationKind) {
			return false
		}
		authorization, found := candidateCorrectionOperationManifest(manifest, operation.Kind)
		if !found {
			return false
		}
		field, found, err := runtimecontract.RuntimeOperationCorrectionFieldForRole(authorization, runtimecontract.RuntimeOperationCorrectionRoleValidationRecipe)
		if err != nil || !found || witness.Path != field.CandidatePath(operationIndex) {
			return false
		}
		value, found := candidateCorrectionStringParameter(operation.Parameters, field.Name)
		return found && value == witness.AlternativeValue
	case runtimecontract.RuntimeCandidateCorrectionWitnessVariant:
		if !hasOperationIndex {
			return false
		}
		contract, found := runtimecontract.RuntimeOperationContractForKind(operation.Kind)
		if !found {
			return false
		}
		variant, err := runtimecontract.MatchRuntimeOperationVariant(operation, contract)
		return err == nil && variant.ID == witness.AlternativeValue
	case runtimecontract.RuntimeCandidateCorrectionWitnessClosedParameter:
		if !hasOperationIndex || operation.Kind != runtimecontract.CanonicalRuntimeBlockKind(witness.OperationKind) {
			return false
		}
		authorization, found := candidateCorrectionOperationManifest(manifest, operation.Kind)
		if !found {
			return false
		}
		fields, err := runtimecontract.RuntimeOperationCorrectionFieldsForRole(authorization, runtimecontract.RuntimeOperationCorrectionRoleClosedParameter)
		if err != nil {
			return false
		}
		var field runtimecontract.RuntimeOperationCorrectionField
		for _, value := range fields {
			if witness.Path == value.CandidatePath(operationIndex) {
				field = value
				break
			}
		}
		if field.Name == "" {
			return false
		}
		if value, found := candidateCorrectionBooleanParameter(operation.Parameters, field.Name); found {
			return fmt.Sprintf("%t", value) == witness.AlternativeValue
		}
		return false
	case runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField:
		return candidateCrossesSchemaFieldWitness(candidate, manifest, operations, operation, operationIndex, hasOperationIndex, witness)
	default:
		return false
	}
}

func candidateCrossesParameterEntryEncodingWitness(operations []runtimecontract.RuntimeOperation, manifest runtimecontract.RuntimeCandidateContractManifest, witness runtimecontract.RuntimeCandidateCorrectionWitness) bool {
	if runtimecontract.ValidateRuntimeCandidateParameterEntryEncodings(manifest, witness.ParameterEntryEncodings) != nil {
		return false
	}
	for _, encoding := range witness.ParameterEntryEncodings {
		if encoding.OperationIndex < 0 || encoding.OperationIndex >= len(operations) {
			return false
		}
		operation := operations[encoding.OperationIndex].Normalized()
		if operation.Kind != runtimecontract.CanonicalRuntimeBlockKind(encoding.OperationKind) {
			return false
		}
		operationManifest, found := candidateCorrectionOperationManifest(manifest, operation.Kind)
		if !found {
			return false
		}
		containerContract, found := candidateCorrectionManifestParameter(operationManifest, encoding.ContainerParameterName)
		if !found || !candidateCorrectionOperationHasTypedParameter(operation, containerContract.Name, containerContract.Type) {
			return false
		}
		for _, misplaced := range encoding.MisplacedParameters {
			if !candidateCorrectionOperationHasTypedParameter(operation, misplaced.Name, misplaced.Type) {
				return false
			}
		}
	}
	return true
}

func candidateCorrectionManifestParameter(operation runtimecontract.RuntimeCandidateOperationManifest, name string) (runtimecontract.RuntimeOperationParameterContract, bool) {
	name = strings.TrimSpace(name)
	for _, parameter := range operation.Normalized().Parameters {
		if parameter.Name == name {
			return parameter, true
		}
	}
	return runtimecontract.RuntimeOperationParameterContract{}, false
}

func candidateCorrectionOperationHasTypedParameter(operation runtimecontract.RuntimeOperation, name string, parameterType runtimecontract.RuntimeOperationParameterType) bool {
	count := 0
	for _, parameter := range operation.Parameters {
		if strings.TrimSpace(parameter.Name) != strings.TrimSpace(name) {
			continue
		}
		count++
		if !runtimecontract.RuntimeOperationParameterMatchesContractType(parameter, parameterType) {
			return false
		}
	}
	return count == 1
}

func candidateCrossesSchemaFieldWitness(candidate runtimecontract.RuntimeCandidate, manifest runtimecontract.RuntimeCandidateContractManifest, operations []runtimecontract.RuntimeOperation, operation runtimecontract.RuntimeOperation, operationIndex int, hasOperationIndex bool, witness runtimecontract.RuntimeCandidateCorrectionWitness) bool {
	field := candidateCorrectionPathField(witness.Path)
	if field == "" || field == "summary" {
		return false
	}
	if !hasOperationIndex {
		switch field {
		case "operations":
			return witness.AlternativeValue == candidateCorrectionFieldPresent && len(operations) != 0
		case "externalDependency":
			return witness.AlternativeValue == candidateCorrectionFieldPresent && candidate.Governance != nil
		case "focusId":
			return candidate.Assessment != nil && candidate.Assessment.FocusID != ""
		case "coverageWorkItemIds":
			return candidate.Assessment != nil && len(candidate.Assessment.CoverageWorkItemIDs) != 0
		case "disposition":
			return candidate.Assessment != nil && candidate.Assessment.Disposition != ""
		default:
			// Unknown/forbidden top-level fields are unrepresentable after strict
			// decoding, which proves the sealed field_absent alternative.
			return witness.AlternativeValue == candidateCorrectionFieldAbsent
		}
	}
	if (field == "id" || field == "dependsOn") && witness.RejectedValue == field && witness.AlternativeValue == candidateCorrectionFieldAbsent {
		// Canonical IDs/edges are derived after the strict wire decoder. Their
		// presence on the decoded candidate cannot disprove removal of the raw
		// forbidden provider fields; successful strict decode already proves it.
		return true
	}
	switch {
	case strings.HasSuffix(witness.Path, ".kind"):
		if witness.AlternativeValue == candidateCorrectionFieldPresent {
			return operation.Kind != ""
		}
		if witness.AlternativeValue == candidateCorrectionFieldAbsent {
			return operation.Kind == ""
		}
		return operation.Kind == runtimecontract.CanonicalRuntimeBlockKind(witness.AlternativeValue)
	case strings.HasSuffix(witness.Path, ".coverageWorkItemIds"):
		if witness.AlternativeValue == candidateCorrectionFieldPresent {
			return len(operation.CoverageWorkItemIDs) != 0
		}
		return witness.AlternativeValue == candidateCorrectionFieldAbsent && len(operation.CoverageWorkItemIDs) == 0
	case strings.HasSuffix(witness.Path, ".dependsOn"):
		switch {
		case witness.AlternativeValue == candidateCorrectionFieldAbsent:
			return len(operation.DependsOn) == 0
		case witness.AlternativeValue == candidateCorrectionFieldPresent:
			return len(operation.DependsOn) != 0
		case strings.HasPrefix(witness.AlternativeValue, candidateCorrectionValueAbsent):
			return !containsCandidateCorrectionString(operation.DependsOn, strings.TrimPrefix(witness.AlternativeValue, candidateCorrectionValueAbsent))
		default:
			return containsCandidateCorrectionString(operation.DependsOn, witness.AlternativeValue)
		}
	case strings.HasSuffix(witness.Path, ".requiredOperationRef"):
		if witness.AlternativeValue == candidateCorrectionFieldAbsent {
			return operation.RequiredOperationRef == ""
		}
		if witness.AlternativeValue == candidateCorrectionFieldPresent {
			return operation.RequiredOperationRef != ""
		}
		return operation.RequiredOperationRef == witness.AlternativeValue
	case strings.HasSuffix(witness.Path, ".bindingRefs"):
		if witness.AlternativeValue == candidateCorrectionFieldAbsent {
			return len(operation.BindingRefs) == 0
		}
		if witness.AlternativeValue == candidateCorrectionFieldPresent {
			return len(operation.BindingRefs) != 0
		}
		return containsCandidateCorrectionString(operation.BindingRefs, witness.AlternativeValue)
	case strings.Contains(witness.Path, ".parameters"):
		name := candidateCorrectionPathField(witness.Path)
		if name == "parameters" {
			switch witness.AlternativeValue {
			case candidateCorrectionFieldPresent:
				return len(operation.Parameters) != 0
			case candidateCorrectionFieldAbsent:
				return len(operation.Parameters) == 0
			}
			contract, found := runtimecontract.RuntimeOperationContractForKind(operation.Kind)
			if !found {
				return false
			}
			variant, err := runtimecontract.MatchRuntimeOperationVariant(operation, contract)
			return err == nil && variant.ID == witness.AlternativeValue
		}
		_, stringPresent := candidateCorrectionStringParameter(operation.Parameters, name)
		_, boolPresent := candidateCorrectionBooleanParameter(operation.Parameters, name)
		present := stringPresent || boolPresent || candidateCorrectionParameterPresent(operation.Parameters, name)
		switch witness.AlternativeValue {
		case candidateCorrectionFieldAbsent:
			return !present
		case candidateCorrectionFieldPresent:
			return present
		default:
			value, found := candidateCorrectionStringParameter(operation.Parameters, name)
			return found && value == witness.AlternativeValue
		}
	default:
		authorization, found := candidateCorrectionOperationManifest(manifest, operation.Kind)
		if !found {
			return witness.AlternativeValue == candidateCorrectionFieldAbsent
		}
		targetField, found, err := runtimecontract.RuntimeOperationCorrectionFieldForRole(authorization, runtimecontract.RuntimeOperationCorrectionRoleTarget)
		if err == nil && found && witness.Path == targetField.CandidatePath(operationIndex) {
			if witness.AlternativeValue == candidateCorrectionFieldPresent {
				return len(operation.TargetRefs) != 0
			}
			if witness.AlternativeValue == candidateCorrectionFieldAbsent {
				return len(operation.TargetRefs) == 0
			}
			return containsCandidateCorrectionString(operation.TargetRefs, witness.AlternativeValue)
		}
		return witness.AlternativeValue == candidateCorrectionFieldAbsent
	}
}

func candidateCorrectionOperationIndex(path string) (int, bool) {
	const marker = "$.operations["
	start := strings.Index(path, marker)
	if start < 0 {
		return 0, false
	}
	value := path[start+len(marker):]
	end := strings.Index(value, "]")
	if end <= 0 {
		return 0, false
	}
	index, err := strconv.Atoi(value[:end])
	return index, err == nil
}

func candidateCorrectionParameterPresent(parameters []runtimecontract.OperationParameter, name string) bool {
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter.Name) == strings.TrimSpace(name) {
			return true
		}
	}
	return false
}
