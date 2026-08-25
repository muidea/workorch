package events

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

type AcquireRuntimeOperationAdmissibilityCommand runtimecontract.RuntimeOperationAdmissibilityRequest

type AcquireRuntimeOperationAdmissibilityResult struct {
	Acquisition runtimecontract.RuntimeOperationAdmissibilityAcquisition
}

type ProbeRuntimeOperationFeasibilityCommand runtimecontract.RuntimeOperationFeasibilityRequest

type ProbeRuntimeOperationFeasibilityResult runtimecontract.RuntimeOperationFeasibilityObservation

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
