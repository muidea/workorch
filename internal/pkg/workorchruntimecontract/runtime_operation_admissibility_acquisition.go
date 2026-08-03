package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeOperationAdmissibilityRequest is the owner-neutral value used by
// Orchestrator to construct one concrete capability-owner EventHub command.
// It is not itself an EventHub command.
type RuntimeOperationAdmissibilityRequest struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	AuthoritySnapshotID           string
	PrerequisiteID                string
	OwnerModuleID                 string
	WorkspaceDeliverableDirectory string
	RequiredKinds                 []string
	RequiredTargetRefs            []string
	ValidationRecipes             []RuntimeValidationRecipe
	CapabilityStateBinding        string
	MaxItems                      int
}

type RuntimeOperationAdmissibilityAcquisition struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	Admissibility       RuntimeOperationAdmissibility
	Observation         RuntimePrerequisiteObservation
}

func (a RuntimeOperationAdmissibilityAcquisition) ValidateForRequest(request RuntimeOperationAdmissibilityRequest) error {
	if a.WorkspaceID != request.WorkspaceID || a.RunID != request.RunID ||
		strings.TrimSpace(a.AuthoritySnapshotID) != strings.TrimSpace(request.AuthoritySnapshotID) {
		return fmt.Errorf("runtime operation admissibility acquisition scope does not match request")
	}
	inventory := a.Admissibility.Normalized()
	if inventory.PrerequisiteID != strings.TrimSpace(request.PrerequisiteID) || inventory.OwnerModuleID != strings.TrimSpace(request.OwnerModuleID) {
		return fmt.Errorf("runtime operation admissibility acquisition inventory does not match request")
	}
	if err := inventory.Validate(); err != nil {
		return err
	}
	observation := a.Observation.Normalized()
	if observation.PrerequisiteID != inventory.PrerequisiteID ||
		observation.Kind != PrerequisiteKindOperationAdmissibility ||
		observation.OwnerModuleID != inventory.OwnerModuleID {
		return fmt.Errorf("runtime operation admissibility acquisition observation does not match inventory")
	}
	return observation.Validate()
}

// RuntimeOperationFeasibilityRequest is the owner-neutral value from which
// Orchestrator constructs one concrete receiver-owned feasibility command.
// It is not itself an EventHub command.
type RuntimeOperationFeasibilityRequest struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	Transaction                   AuthorityTransaction
	WorkspaceDeliverableDirectory string
	Operation                     RuntimeOperation
	Admissibility                 RuntimeOperationAdmissibility
	CapabilityStateBinding        string
	CandidateManifestHash         string
	OperationAuthorization        RuntimeCandidateOperationManifest
	Observation                   *RuntimePrerequisiteObservation
	CurrentTargetSnapshot         *TargetSnapshot
}

// RuntimeOperationFeasibilityObservation is the normalized outer observation
// returned by a capability owner. Outcome is the single factual authority;
// stale, prerequisite and failure values are outcome-specific detail only.
type RuntimeOperationFeasibilityObservation struct {
	OperationID             string
	AdmissibilitySnapshotID string
	CapabilityStateBinding  string
	CandidateManifestHash   string
	Outcome                 RuntimeOwnerFeasibilityOutcome
	Feedback                *RuntimeFeedback
	ReplayProof             *RuntimeOperationReplayProof
	Stale                   *RuntimePrerequisiteStale
	PrerequisiteRequest     *RuntimeOperationPrerequisiteRequest
}

// RuntimeOperationAuthorizationIdentity builds a minimal sealed authorization
// identity for tests and identity-only fixtures.
func RuntimeOperationAuthorizationIdentity(kind string) (RuntimeCandidateOperationManifest, error) {
	descriptor, ok := RuntimeOperationDescriptorForKind(kind)
	if !ok {
		return RuntimeCandidateOperationManifest{}, fmt.Errorf("unknown runtime operation kind %q", kind)
	}
	return RuntimeCandidateOperationManifest{Kind: descriptor.Kind, OwnerModuleID: descriptor.OwnerModuleID, OperationContractRevision: RuntimeOperationContractRevision()}.Normalized(), nil
}

// ValidateRuntimeOperationFeasibilityRequest performs only pure schema,
// identity, and sealed-authorization checks.
func ValidateRuntimeOperationFeasibilityRequest(request RuntimeOperationFeasibilityRequest, ownerModuleID string) error {
	operation := request.Operation.Normalized()
	inventory := request.Admissibility.Normalized()
	authorization := request.OperationAuthorization.Normalized()
	if request.WorkspaceID == "" || request.RunID == "" || strings.TrimSpace(request.Transaction.AuthoritySnapshotID) == "" ||
		strings.TrimSpace(request.WorkspaceDeliverableDirectory) == "" || operation.ID == "" || operation.Kind == "" || inventory.OwnerModuleID != ownerModuleID {
		return common.ErrInvalidArgument
	}
	descriptor, ok := RuntimeOperationDescriptorForKind(operation.Kind)
	if !ok || descriptor.OwnerModuleID != ownerModuleID {
		return fmt.Errorf("runtime operation kind %q does not belong to owner %q", operation.Kind, ownerModuleID)
	}
	if err := inventory.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(request.CandidateManifestHash) == "" {
		return fmt.Errorf("runtime operation feasibility requires sealed candidate manifest hash")
	}
	if authorization.Kind == "" {
		return fmt.Errorf("runtime operation feasibility requires sealed operation authorization")
	}
	if CanonicalRuntimeBlockKind(authorization.Kind) != operation.Kind {
		return fmt.Errorf("runtime operation feasibility authorization kind %q does not match operation %q", authorization.Kind, operation.Kind)
	}
	if authorization.OwnerModuleID != "" && authorization.OwnerModuleID != ownerModuleID {
		return fmt.Errorf("runtime operation feasibility authorization owner %q does not match receiver %q", authorization.OwnerModuleID, ownerModuleID)
	}
	if authorization.OperationContractRevision != "" {
		if err := ValidateRuntimeOperationContractRevision(authorization.OperationContractRevision); err != nil {
			return err
		}
	}
	if len(authorization.AdmissibilitySnapshotRefs) != 0 {
		found := false
		for _, ref := range authorization.AdmissibilitySnapshotRefs {
			if ref == inventory.SnapshotID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("runtime operation feasibility admissibility snapshot is absent from sealed authorization")
		}
	}
	for _, kind := range inventory.OperationKinds {
		if kind == operation.Kind {
			return nil
		}
	}
	return fmt.Errorf("runtime operation kind %q is absent from owner admissibility inventory", operation.Kind)
}

func NormalizeRuntimeOperationFeasibilityObservation(result RuntimeOperationFeasibilityObservation) (RuntimeOperationFeasibilityObservation, error) {
	result.OperationID = strings.TrimSpace(result.OperationID)
	result.AdmissibilitySnapshotID = strings.TrimSpace(result.AdmissibilitySnapshotID)
	result.CapabilityStateBinding = strings.TrimSpace(result.CapabilityStateBinding)
	result.CandidateManifestHash = strings.TrimSpace(result.CandidateManifestHash)
	result.Outcome = RuntimeOwnerFeasibilityOutcome(strings.TrimSpace(string(result.Outcome)))
	if result.OperationID == "" || result.AdmissibilitySnapshotID == "" {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility result requires operation and admissibility identity")
	}
	switch result.Outcome {
	case RuntimeOwnerFeasibilityFeasible:
		if result.Stale != nil || result.Feedback != nil {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility feasible result cannot carry stale or failure")
		}
		if result.PrerequisiteRequest != nil {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility feasible result cannot carry a prerequisite request")
		}
		return result, nil
	case RuntimeOwnerFeasibilityStale:
		if result.Stale == nil {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility stale outcome requires stale detail")
		}
		if result.Feedback != nil {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility stale result cannot also carry failure")
		}
		if result.PrerequisiteRequest != nil || result.ReplayProof != nil {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility stale result cannot carry prerequisite or replay detail")
		}
		if err := result.Stale.Validate(); err != nil {
			return RuntimeOperationFeasibilityObservation{}, err
		}
		return result, nil
	case RuntimeOwnerFeasibilityPrerequisiteRequired:
		if result.PrerequisiteRequest == nil || result.Feedback == nil || result.Stale != nil || result.ReplayProof != nil {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility prerequisite outcome requires feedback and prerequisite detail")
		}
	case RuntimeOwnerFeasibilityInfeasible, RuntimeOwnerFeasibilityContractDefect:
		if result.Feedback == nil || result.Stale != nil || result.PrerequisiteRequest != nil || result.ReplayProof != nil {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility infeasible outcome requires only feedback detail")
		}
	default:
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility has unknown outcome %q", result.Outcome)
	}
	category := RuntimeOwnerFeasibilityCategory(result.Feedback.Normalized().StableCategory)
	if !ValidRuntimeOwnerFeasibilityCategory(category) {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility failure requires owner category")
	}
	if result.Outcome == RuntimeOwnerFeasibilityContractDefect && category != RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility contract-defect outcome requires contract-defect category")
	}
	if result.Outcome != RuntimeOwnerFeasibilityContractDefect && category == RuntimeOwnerFeasibilityCategoryOwnerContractDefect {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility contract-defect category requires contract-defect outcome")
	}
	if result.Feedback == nil {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility failure requires original diagnostic feedback")
	}
	if err := result.Feedback.Validate(); err != nil {
		return RuntimeOperationFeasibilityObservation{}, err
	}
	feedback := result.Feedback.CloneOriginal()
	if feedback.Domain != RuntimeFeedbackDomainFeasibility || feedback.StableCategory != RuntimeFeedbackCategory(category) || feedback.OriginalDiagnostic == nil {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility feedback does not match owner outcome")
	}
	result.Feedback = &feedback
	return result, nil
}

func NewRuntimeOperationFeasibilityObservation(request RuntimeOperationFeasibilityRequest, category RuntimeOwnerFeasibilityCategory, failure *FailureFact, stale *RuntimePrerequisiteStale, prerequisiteRequest *RuntimeOperationPrerequisiteRequest, replayProof *RuntimeOperationReplayProof) (RuntimeOperationFeasibilityObservation, error) {
	operation := request.Operation.Normalized()
	if stale != nil && (failure != nil || prerequisiteRequest != nil) {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility cannot mix stale with failure or prerequisite request")
	}
	outcome := RuntimeOwnerFeasibilityFeasible
	switch {
	case stale != nil:
		outcome = RuntimeOwnerFeasibilityStale
	case prerequisiteRequest != nil:
		outcome = RuntimeOwnerFeasibilityPrerequisiteRequired
	case failure != nil && category == RuntimeOwnerFeasibilityCategoryOwnerContractDefect:
		outcome = RuntimeOwnerFeasibilityContractDefect
	case failure != nil:
		outcome = RuntimeOwnerFeasibilityInfeasible
	}
	if failure != nil && !ValidRuntimeOwnerFeasibilityCategory(category) {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility owner did not classify code %q", failure.Code)
	}
	if failure == nil && category != "" {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility category requires failure")
	}
	var feedback *RuntimeFeedback
	if failure != nil {
		descriptor, ok := RuntimeOperationDescriptorForKind(operation.Kind)
		if !ok {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility has unknown operation kind %q", operation.Kind)
		}
		value, err := RuntimeFeedbackFromFailureFact(RuntimeFeedbackScope{WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.Transaction.AuthoritySnapshotID}, RuntimeFeedbackDomainFeasibility, RuntimeFeedbackCategory(category), descriptor.OwnerModuleID, *failure)
		if err != nil {
			return RuntimeOperationFeasibilityObservation{}, err
		}
		value.OriginalDiagnostic.OperationID = operation.ID
		feedback = &value
	}
	result := RuntimeOperationFeasibilityObservation{OperationID: operation.ID, AdmissibilitySnapshotID: request.Admissibility.SnapshotID, CapabilityStateBinding: request.Admissibility.AcquisitionBinding, CandidateManifestHash: request.CandidateManifestHash, Outcome: outcome, Feedback: feedback, ReplayProof: replayProof, Stale: stale, PrerequisiteRequest: prerequisiteRequest}
	return NormalizeRuntimeOperationFeasibilityObservation(result)
}

// FailureCategory returns the stable owner classification from canonical
// feedback without storing a second category field.
func (o RuntimeOperationFeasibilityObservation) FailureCategory() RuntimeOwnerFeasibilityCategory {
	if o.Feedback == nil {
		return ""
	}
	return RuntimeOwnerFeasibilityCategory(o.Feedback.Normalized().StableCategory)
}

func RuntimeOperationInfeasibleFailure(operation RuntimeOperation, code, message string) *FailureFact {
	operation = operation.Normalized()
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" {
		code = "operation_infeasible"
	}
	if message == "" {
		message = "runtime operation is not currently feasible"
	}
	return &FailureFact{Class: FailureClassCandidateContractFailure, Code: code, Signature: "operation-admissibility/" + operation.Kind + "/" + code, Message: message, TargetRefs: append([]string(nil), operation.TargetRefs...)}
}

func NewRuntimeOperationAdmissibility(prerequisiteID, ownerModuleID string, operationKinds, targetRefs, selectorRefs, constraintRefs []string, observedRevision string) (RuntimeOperationAdmissibility, error) {
	return NewRuntimeOperationAdmissibilityWithBinding(prerequisiteID, ownerModuleID, operationKinds, targetRefs, selectorRefs, constraintRefs, observedRevision, "")
}

func NewRuntimeOperationAdmissibilityWithBinding(prerequisiteID, ownerModuleID string, operationKinds, targetRefs, selectorRefs, constraintRefs []string, observedRevision, acquisitionBinding string) (RuntimeOperationAdmissibility, error) {
	inventory := RuntimeOperationAdmissibility{
		PrerequisiteID: prerequisiteID, OwnerModuleID: ownerModuleID,
		OperationKinds: operationKinds, TargetRefs: targetRefs, SelectorRefs: selectorRefs,
		ConstraintRefs: constraintRefs, ObservedRevision: observedRevision, AcquisitionBinding: acquisitionBinding,
	}.Normalized()
	inventory.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		return RuntimeOperationAdmissibility{}, err
	}
	return inventory, nil
}

// ValidateRuntimeOperationAdmissibilityRequest is a pure schema and owner
// identity check. It does not inspect owner resources or choose Runtime flow.
func ValidateRuntimeOperationAdmissibilityRequest(request RuntimeOperationAdmissibilityRequest, ownerModuleID string) error {
	if request.WorkspaceID == "" || request.RunID == "" || strings.TrimSpace(request.AuthoritySnapshotID) == "" ||
		strings.TrimSpace(request.PrerequisiteID) == "" || strings.TrimSpace(request.WorkspaceDeliverableDirectory) == "" ||
		strings.TrimSpace(request.OwnerModuleID) == "" || request.OwnerModuleID != ownerModuleID || len(request.RequiredKinds) == 0 ||
		request.MaxItems < 1 || request.MaxItems > RuntimeOwnerInventoryItemsMaxCount {
		return common.ErrInvalidArgument
	}
	for _, kind := range request.RequiredKinds {
		descriptor, ok := RuntimeOperationDescriptorForKind(kind)
		if !ok || descriptor.OwnerModuleID != ownerModuleID {
			return fmt.Errorf("runtime operation kind %q does not belong to owner %q", kind, ownerModuleID)
		}
	}
	if len(request.ValidationRecipes) == 0 {
		return nil
	}
	if ownerModuleID != RuntimeModuleShell || !runtimeOperationAdmissibilityKindsContain(request.RequiredKinds, RuntimeBlockKindManagedValidationSession) {
		return fmt.Errorf("runtime validation recipes require Shell managed-validation admissibility")
	}
	seen := make(map[string]struct{}, len(request.ValidationRecipes))
	for _, recipe := range request.ValidationRecipes {
		recipe = recipe.Normalized()
		if err := recipe.Validate(); err != nil {
			return fmt.Errorf("runtime validation recipe: %w", err)
		}
		if _, duplicate := seen[recipe.ID]; duplicate {
			return fmt.Errorf("runtime validation recipe %q is duplicated", recipe.ID)
		}
		seen[recipe.ID] = struct{}{}
	}
	return nil
}

func runtimeOperationAdmissibilityKindsContain(values []string, wanted string) bool {
	wanted = CanonicalRuntimeBlockKind(wanted)
	for _, value := range values {
		if CanonicalRuntimeBlockKind(value) == wanted {
			return true
		}
	}
	return false
}
