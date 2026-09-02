package events

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

type AcquireRuntimeOperationAdmissibilityCommand runtimecontract.RuntimeOperationAdmissibilityRequest

type AcquireRuntimeOperationAdmissibilityResult struct {
	Acquisition runtimecontract.RuntimeOperationAdmissibilityAcquisition
}

type ProbeRuntimeOperationFeasibilityCommand runtimecontract.RuntimeOperationFeasibilityRequest

type ProbeRuntimeOperationFeasibilityResult runtimecontract.RuntimeOperationFeasibilityObservation

type ValidateRuntimeOperationCandidateConformanceCommand runtimecontract.RuntimeOperationCandidateConformanceRequest

type ValidateRuntimeOperationCandidateConformanceResult runtimecontract.RuntimeOperationCandidateConformanceObservation

type ProbeRuntimeSemanticProofFeasibilityCommand runtimecontract.RuntimeSemanticProofFeasibilityRequest

type ProbeRuntimeSemanticProofFeasibilityResult runtimecontract.RuntimeSemanticProofFeasibilityObservation

func ValidateRuntimeSemanticProofFeasibilityCommand(c ProbeRuntimeSemanticProofFeasibilityCommand, owner string) error {
	return runtimecontract.ValidateRuntimeSemanticProofFeasibilityRequest(runtimecontract.RuntimeSemanticProofFeasibilityRequest(c), owner)
}

func NewRuntimeSemanticProofFeasibilityResult(c ProbeRuntimeSemanticProofFeasibilityCommand, status runtimecontract.RuntimeSemanticFeasibilityStatus, observedRevision string, targetRefs []string, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) (ProbeRuntimeSemanticProofFeasibilityResult, error) {
	v, err := runtimecontract.NewRuntimeSemanticProofFeasibilityObservation(runtimecontract.RuntimeSemanticProofFeasibilityRequest(c), status, observedRevision, targetRefs, diagnostic)
	if err != nil {
		return ProbeRuntimeSemanticProofFeasibilityResult{}, err
	}
	return ProbeRuntimeSemanticProofFeasibilityResult(v), nil
}

// NewRuntimeSemanticProofFeasibilityOwnerResult keeps File's prospective
// manifest delta inside its concrete owner observation. The required-fact
// owner's prospective-satisfaction judgment is attached later by Orchestrator
// and is never synthesized at the File boundary.
func NewRuntimeSemanticProofFeasibilityOwnerResult(c ProbeRuntimeSemanticProofFeasibilityCommand, status runtimecontract.RuntimeSemanticFeasibilityStatus, observedRevision string, targetRefs []string, prospectiveDelta *runtimecontract.RuntimeProspectiveWorkspaceManifestDelta, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) (ProbeRuntimeSemanticProofFeasibilityResult, error) {
	v, err := runtimecontract.NewRuntimeSemanticProofFeasibilityOwnerObservation(runtimecontract.RuntimeSemanticProofFeasibilityRequest(c), status, observedRevision, targetRefs, prospectiveDelta, diagnostic)
	if err != nil {
		return ProbeRuntimeSemanticProofFeasibilityResult{}, err
	}
	return ProbeRuntimeSemanticProofFeasibilityResult(v), nil
}

func ValidateRuntimeOperationFeasibilityCommand(c ProbeRuntimeOperationFeasibilityCommand, owner string) error {
	return runtimecontract.ValidateRuntimeOperationFeasibilityRequest(runtimecontract.RuntimeOperationFeasibilityRequest(c), owner)
}

func ValidateRuntimeOperationCandidateConformanceCommandValue(c ValidateRuntimeOperationCandidateConformanceCommand, owner string) error {
	return runtimecontract.ValidateRuntimeOperationCandidateConformanceRequest(runtimecontract.RuntimeOperationCandidateConformanceRequest(c), owner)
}

func NewValidateRuntimeOperationCandidateConformanceResult(c ValidateRuntimeOperationCandidateConformanceCommand, outcome runtimecontract.RuntimeOperationCandidateConformanceOutcome, violation *runtimecontract.RuntimeOperationCandidateConformanceViolation) (ValidateRuntimeOperationCandidateConformanceResult, error) {
	request := runtimecontract.RuntimeOperationCandidateConformanceRequest(c).Normalized()
	variantID, _, err := runtimecontract.RuntimeOperationCandidateConstraintsForOperation(request.Operation, request.OperationAuthorization)
	if err != nil {
		return ValidateRuntimeOperationCandidateConformanceResult{}, err
	}
	observation := runtimecontract.RuntimeOperationCandidateConformanceObservation{
		OperationID: request.Operation.ID, OperationKind: request.Operation.Kind, VariantID: variantID,
		OwnerModuleID: request.OperationAuthorization.OwnerModuleID, CandidateManifestHash: request.CandidateManifestHash,
		Outcome: outcome, Violation: violation,
	}.Normalized()
	if err := observation.ValidateForRequest(request, request.OperationAuthorization.OwnerModuleID); err != nil {
		return ValidateRuntimeOperationCandidateConformanceResult{}, err
	}
	return ValidateRuntimeOperationCandidateConformanceResult(observation), nil
}

func NewProbeRuntimeOperationFeasibilityResult(c ProbeRuntimeOperationFeasibilityCommand, category runtimecontract.RuntimeOwnerFeasibilityCategory, failure *runtimecontract.FailureFact, stale *runtimecontract.RuntimePrerequisiteStale, prerequisiteRequest *runtimecontract.RuntimeOperationPrerequisiteRequest, replayProof *runtimecontract.RuntimeOperationReplayProof) (ProbeRuntimeOperationFeasibilityResult, error) {
	v, err := runtimecontract.NewRuntimeOperationFeasibilityObservation(runtimecontract.RuntimeOperationFeasibilityRequest(c), category, failure, stale, prerequisiteRequest, replayProof)
	if err != nil {
		return ProbeRuntimeOperationFeasibilityResult{}, err
	}
	return ProbeRuntimeOperationFeasibilityResult(v), nil
}

func ValidateRuntimeOperationAdmissibilityCommand(c AcquireRuntimeOperationAdmissibilityCommand, owner string) error {
	return runtimecontract.ValidateRuntimeOperationAdmissibilityRequest(runtimecontract.RuntimeOperationAdmissibilityRequest(c), owner)
}
