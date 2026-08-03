package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type AcquireRuntimeOperationAdmissibilityCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	AuthoritySnapshotID           string
	PrerequisiteID                string
	OwnerModuleID                 string
	WorkspaceDeliverableDirectory string
	RequiredKinds                 []string
	RequiredTargetRefs            []string
	ValidationRecipes             []runtimecontract.RuntimeValidationRecipe
	CapabilityStateBinding        string
	MaxItems                      int
}
type AcquireRuntimeOperationAdmissibilityResult struct {
	Acquisition runtimecontract.RuntimeOperationAdmissibilityAcquisition
}

type ProbeRuntimeOperationFeasibilityCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	Transaction                   runtimecontract.AuthorityTransaction
	WorkspaceDeliverableDirectory string
	Operation                     runtimecontract.RuntimeOperation
	Admissibility                 runtimecontract.RuntimeOperationAdmissibility
	CapabilityStateBinding        string
	CandidateManifestHash         string
	OperationAuthorization        runtimecontract.RuntimeCandidateOperationManifest
	Observation                   *runtimecontract.RuntimePrerequisiteObservation
	CurrentTargetSnapshot         *runtimecontract.TargetSnapshot
}

type ProbeRuntimeOperationFeasibilityResult struct {
	OperationID             string
	AdmissibilitySnapshotID string
	CapabilityStateBinding  string
	CandidateManifestHash   string
	Outcome                 runtimecontract.RuntimeOwnerFeasibilityOutcome
	Feedback                *runtimecontract.RuntimeFeedback
	ReplayProof             *runtimecontract.RuntimeOperationReplayProof
	Stale                   *runtimecontract.RuntimePrerequisiteStale
	PrerequisiteRequest     *runtimecontract.RuntimeOperationPrerequisiteRequest
}

type ProbeRuntimeSemanticProofFeasibilityCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	FeasibilityBatchID            string
	WorkspaceDeliverableDirectory string
	Manifest                      runtimecontract.RuntimeSemanticPlanManifest
	Proposal                      runtimecontract.RuntimeSemanticProofProposal
	OwnerCatalog                  runtimecontract.RuntimeOperationAdmissibility
	CatalogObservation            *runtimecontract.RuntimePrerequisiteObservation
}

type ProbeRuntimeSemanticProofFeasibilityResult struct {
	FeasibilityBatchID string
	Observation        runtimecontract.RuntimeSemanticProofFeasibility
}

func ValidateRuntimeSemanticProofFeasibilityCommand(c ProbeRuntimeSemanticProofFeasibilityCommand, owner string) error {
	return runtimecontract.ValidateRuntimeSemanticProofFeasibilityRequest(runtimeSemanticProofFeasibilityRequest(c), owner)
}

func NewRuntimeSemanticProofFeasibilityResult(c ProbeRuntimeSemanticProofFeasibilityCommand, status runtimecontract.RuntimeSemanticFeasibilityStatus, observedRevision string, targetRefs []string, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) (ProbeRuntimeSemanticProofFeasibilityResult, error) {
	v, err := runtimecontract.NewRuntimeSemanticProofFeasibilityObservation(runtimeSemanticProofFeasibilityRequest(c), status, observedRevision, targetRefs, diagnostic)
	if err != nil {
		return ProbeRuntimeSemanticProofFeasibilityResult{}, err
	}
	return ProbeRuntimeSemanticProofFeasibilityResult{FeasibilityBatchID: v.FeasibilityBatchID, Observation: v.Observation}, nil
}

func runtimeSemanticProofFeasibilityRequest(c ProbeRuntimeSemanticProofFeasibilityCommand) runtimecontract.RuntimeSemanticProofFeasibilityRequest {
	return runtimecontract.RuntimeSemanticProofFeasibilityRequest{WorkspaceID: c.WorkspaceID, RunID: c.RunID, FeasibilityBatchID: c.FeasibilityBatchID, WorkspaceDeliverableDirectory: c.WorkspaceDeliverableDirectory, Manifest: c.Manifest, Proposal: c.Proposal, OwnerCatalog: c.OwnerCatalog, CatalogObservation: c.CatalogObservation}
}

func ValidateRuntimeOperationFeasibilityCommand(c ProbeRuntimeOperationFeasibilityCommand, owner string) error {
	return runtimecontract.ValidateRuntimeOperationFeasibilityRequest(runtimeOperationFeasibilityRequest(c), owner)
}

func NewProbeRuntimeOperationFeasibilityResult(c ProbeRuntimeOperationFeasibilityCommand, category runtimecontract.RuntimeOwnerFeasibilityCategory, failure *runtimecontract.FailureFact, stale *runtimecontract.RuntimePrerequisiteStale, prerequisiteRequest *runtimecontract.RuntimeOperationPrerequisiteRequest, replayProof *runtimecontract.RuntimeOperationReplayProof) (ProbeRuntimeOperationFeasibilityResult, error) {
	value, err := runtimecontract.NewRuntimeOperationFeasibilityObservation(runtimeOperationFeasibilityRequest(c), category, failure, stale, prerequisiteRequest, replayProof)
	if err != nil {
		return ProbeRuntimeOperationFeasibilityResult{}, err
	}
	return probeRuntimeOperationFeasibilityResult(value), nil
}

func runtimeOperationFeasibilityRequest(c ProbeRuntimeOperationFeasibilityCommand) runtimecontract.RuntimeOperationFeasibilityRequest {
	return runtimecontract.RuntimeOperationFeasibilityRequest{WorkspaceID: c.WorkspaceID, RunID: c.RunID, Transaction: c.Transaction, WorkspaceDeliverableDirectory: c.WorkspaceDeliverableDirectory, Operation: c.Operation, Admissibility: c.Admissibility, CapabilityStateBinding: c.CapabilityStateBinding, CandidateManifestHash: c.CandidateManifestHash, OperationAuthorization: c.OperationAuthorization, Observation: c.Observation, CurrentTargetSnapshot: c.CurrentTargetSnapshot}
}

func probeRuntimeOperationFeasibilityResult(v runtimecontract.RuntimeOperationFeasibilityObservation) ProbeRuntimeOperationFeasibilityResult {
	return ProbeRuntimeOperationFeasibilityResult{OperationID: v.OperationID, AdmissibilitySnapshotID: v.AdmissibilitySnapshotID, CapabilityStateBinding: v.CapabilityStateBinding, CandidateManifestHash: v.CandidateManifestHash, Outcome: v.Outcome, Feedback: v.Feedback, ReplayProof: v.ReplayProof, Stale: v.Stale, PrerequisiteRequest: v.PrerequisiteRequest}
}

func ValidateRuntimeOperationAdmissibilityCommand(c AcquireRuntimeOperationAdmissibilityCommand, owner string) error {
	return runtimecontract.ValidateRuntimeOperationAdmissibilityRequest(runtimeOperationAdmissibilityRequest(c), owner)
}
func runtimeOperationAdmissibilityRequest(c AcquireRuntimeOperationAdmissibilityCommand) runtimecontract.RuntimeOperationAdmissibilityRequest {
	return runtimecontract.RuntimeOperationAdmissibilityRequest{WorkspaceID: c.WorkspaceID, RunID: c.RunID, AuthoritySnapshotID: c.AuthoritySnapshotID, PrerequisiteID: c.PrerequisiteID, OwnerModuleID: c.OwnerModuleID, WorkspaceDeliverableDirectory: c.WorkspaceDeliverableDirectory, RequiredKinds: c.RequiredKinds, RequiredTargetRefs: c.RequiredTargetRefs, ValidationRecipes: c.ValidationRecipes, CapabilityStateBinding: c.CapabilityStateBinding, MaxItems: c.MaxItems}
}
