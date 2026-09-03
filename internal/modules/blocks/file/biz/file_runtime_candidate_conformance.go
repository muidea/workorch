package file

import (
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ProbeRuntimeOperationCandidateConformance validates only owner-private
// parameter representation. It does not read the workspace, apply a patch,
// choose a retry, or create execution evidence.
func (a *RuntimeAdapter) ProbeRuntimeOperationCandidateConformance(operation runtimecontract.RuntimeOperation, authorization runtimecontract.RuntimeCandidateOperationManifest) runtimecontract.RuntimeOperationCandidateConformanceObservation {
	operation = operation.Normalized()
	variantID, constraints, err := runtimecontract.RuntimeOperationCandidateConformanceConstraintsForOperation(operation, authorization)
	observation := runtimecontract.RuntimeOperationCandidateConformanceObservation{
		OperationID: operation.ID, OperationKind: operation.Kind, VariantID: variantID,
		OwnerModuleID: ModuleID, Outcome: runtimecontract.RuntimeOperationCandidateConformant,
	}
	if err != nil || a == nil {
		observation.Outcome = runtimecontract.RuntimeOperationCandidateContractDefect
		return observation.Normalized()
	}
	for _, constraint := range constraints {
		switch constraint.RequiredShape {
		case runtimecontract.RuntimeOperationCandidateRequiredShapeUnifiedDiffSingleFile:
			diff := runtimeOperationParameterString(operation, constraint.ParameterName)
			patch, parseErr := parsePatchUnifiedDiff(diff)
			if parseErr != nil {
				observation.Outcome = runtimecontract.RuntimeOperationCandidateNonconformant
				observation.Violation = &runtimecontract.RuntimeOperationCandidateConstraintViolation{
					ConstraintRef: constraint.Ref, ParameterName: constraint.ParameterName,
					RequiredShape: constraint.RequiredShape, ActualShape: runtimecontract.RuntimeOperationCandidateActualShapeInvalidSyntax,
				}
				return observation.Normalized()
			}
			targetRef := ""
			if len(operation.TargetRefs) == 1 {
				targetRef = strings.TrimSpace(operation.TargetRefs[0])
			}
			if targetRef == "" || !patchDiffTargetMatches(targetRef, patch) {
				observation.Outcome = runtimecontract.RuntimeOperationCandidateNonconformant
				observation.Violation = &runtimecontract.RuntimeOperationCandidateConstraintViolation{
					ConstraintRef: constraint.Ref, ParameterName: constraint.ParameterName,
					RequiredShape: constraint.RequiredShape, ActualShape: runtimecontract.RuntimeOperationCandidateActualShapeTargetMismatch,
				}
				return observation.Normalized()
			}
		case runtimecontract.RuntimeOperationCandidateRequiredShapeUnifiedDiffAuthorizedTargetMode:
			diff := runtimeOperationParameterString(operation, constraint.ParameterName)
			patch, parseErr := parsePatchUnifiedDiff(diff)
			if parseErr != nil {
				observation.Outcome = runtimecontract.RuntimeOperationCandidateNonconformant
				observation.Violation = &runtimecontract.RuntimeOperationCandidateConstraintViolation{
					ConstraintRef: constraint.Ref, ParameterName: constraint.ParameterName,
					RequiredShape: constraint.RequiredShape, ActualShape: runtimecontract.RuntimeOperationCandidateActualShapeInvalidSyntax,
				}
				return observation.Normalized()
			}
			policy := authorization.Normalized().Target.Policy
			modeMatches := policy == runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget && patch.OldRef == "/dev/null"
			if policy == runtimecontract.RuntimeOperationTargetPolicyExactTarget || policy == runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope {
				modeMatches = patch.OldRef != "/dev/null"
			}
			if !modeMatches {
				observation.Outcome = runtimecontract.RuntimeOperationCandidateNonconformant
				observation.Violation = &runtimecontract.RuntimeOperationCandidateConstraintViolation{
					ConstraintRef: constraint.Ref, ParameterName: constraint.ParameterName,
					RequiredShape: constraint.RequiredShape, ActualShape: runtimecontract.RuntimeOperationCandidateActualShapeTargetModeMismatch,
				}
				return observation.Normalized()
			}
		default:
			observation.Outcome = runtimecontract.RuntimeOperationCandidateContractDefect
			return observation.Normalized()
		}
	}
	return observation.Normalized()
}
