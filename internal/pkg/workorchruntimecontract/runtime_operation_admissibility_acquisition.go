package workorchruntimecontract

import (
	"fmt"
	"reflect"
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
	// ValidationProofRequirements are Definition-owned per-CWI identities.
	// The addressed owner may publish an exact applicability binding for a
	// matching recipe but cannot widen or rewrite these values.
	ValidationProofRequirements []RuntimeValidationProofRequirement
	// WorkspaceManifestBinding is the current workspace-manifest owner fact used by a
	// capability owner whose admissibility depends on workspace contents.  It
	// is immutable request context, not a second workspace observation: the
	// receiver must copy its exact workspace-tree revision into the returned
	// freshness dependencies instead of inventing another owner revision.
	WorkspaceManifestBinding RuntimeWorkspaceManifestBinding
	CapabilityStateBinding   string
	MaxItems                 int
}

// RuntimeWorkspaceManifestBinding couples the current resource-owner manifest to
// its persisted prerequisite observation.  The DTO is owner-neutral and pure;
// it grants no operation authority and performs no filesystem acquisition.
type RuntimeWorkspaceManifestBinding struct {
	Manifest    WorkspaceManifest
	Observation RuntimePrerequisiteObservation
}

func (b RuntimeWorkspaceManifestBinding) Empty() bool {
	return strings.TrimSpace(b.Manifest.Revision) == "" && strings.TrimSpace(b.Observation.ObservationID) == ""
}

func (b RuntimeWorkspaceManifestBinding) Normalized() RuntimeWorkspaceManifestBinding {
	b.Manifest.Revision = strings.TrimSpace(b.Manifest.Revision)
	b.Manifest.TargetRefs = normalizeStrings(b.Manifest.TargetRefs)
	b.Observation = b.Observation.Normalized()
	return b
}

func (b RuntimeWorkspaceManifestBinding) Clone() RuntimeWorkspaceManifestBinding {
	b = b.Normalized()
	b.Manifest.TargetRefs = append([]string(nil), b.Manifest.TargetRefs...)
	b.Observation.Dependencies = append([]RuntimeFreshnessDependency(nil), b.Observation.Dependencies...)
	return b
}

func (b RuntimeWorkspaceManifestBinding) Validate() error {
	rawOwnerModuleID := b.Observation.OwnerModuleID
	b = b.Normalized()
	if b.Manifest.Revision == "" || b.Observation.PrerequisiteID == "" || b.Observation.ObservationID == "" {
		return fmt.Errorf("runtime workspace manifest binding requires manifest and observation identity")
	}
	if b.Observation.Kind != PrerequisiteKindManifest || b.Observation.OwnerModuleID == "" || rawOwnerModuleID != b.Observation.OwnerModuleID ||
		b.Observation.ContentHash != b.Manifest.Revision {
		return fmt.Errorf("runtime workspace manifest binding does not match canonical owner observation")
	}
	if err := b.Observation.Validate(); err != nil {
		return err
	}
	if !runtimeObservationDependsOnWorkspaceTreeRevision(b.Observation, b.Observation.OwnerModuleID, b.Manifest.Revision) {
		return fmt.Errorf("runtime workspace manifest binding has no matching owner workspace-tree dependency")
	}
	return nil
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
	if err := a.Admissibility.Validate(); err != nil {
		return err
	}
	if err := ValidateRuntimeValidationProofRequirements(request.ValidationProofRequirements); err != nil {
		return fmt.Errorf("runtime operation admissibility proof requirements: %w", err)
	}
	inventory := a.Admissibility.Normalized()
	if inventory.PrerequisiteID != strings.TrimSpace(request.PrerequisiteID) || inventory.OwnerModuleID != strings.TrimSpace(request.OwnerModuleID) {
		return fmt.Errorf("runtime operation admissibility acquisition inventory does not match request")
	}
	observation := a.Observation.Normalized()
	if observation.PrerequisiteID != inventory.PrerequisiteID ||
		observation.Kind != PrerequisiteKindOperationAdmissibility ||
		observation.OwnerModuleID != inventory.OwnerModuleID {
		return fmt.Errorf("runtime operation admissibility acquisition observation does not match inventory")
	}
	if err := observation.Validate(); err != nil {
		return err
	}
	binding := request.WorkspaceManifestBinding.Normalized()
	if !binding.Empty() && !runtimeObservationDependsOnWorkspaceTreeRevision(observation, binding.Observation.OwnerModuleID, binding.Manifest.Revision) {
		return fmt.Errorf("runtime operation admissibility acquisition omitted workspace manifest freshness")
	}
	declarations := NormalizeRuntimeValidationRecipeDeclarations(request.ValidationRecipes)
	for _, recipe := range inventory.ValidationRecipes {
		declared, found := RuntimeValidationRecipeDeclarationForID(declarations, recipe.ID)
		if !found || !runtimeValidationRecipeConfirmsDeclaration(declared, recipe) {
			return fmt.Errorf("runtime operation admissibility acquisition published undeclared validation recipe %q", recipe.ID)
		}
	}
	if inventory.OwnerModuleID == RuntimeModuleShell && runtimeOperationAdmissibilityKindsContain(inventory.OperationKinds, RuntimeBlockKindManagedValidationSession) {
		for _, recipeRef := range inventory.ValidationRecipeRefs {
			declared, declaredFound := RuntimeValidationRecipeDeclarationForID(declarations, recipeRef)
			confirmed, confirmedFound := RuntimeValidationRecipeForID(inventory.ValidationRecipes, recipeRef)
			if !declaredFound || !confirmedFound || !runtimeValidationRecipeConfirmsDeclaration(declared, confirmed) {
				return fmt.Errorf("runtime Shell admissibility published unconfirmed validation recipe ref %q", recipeRef)
			}
		}
		for _, recipe := range inventory.ValidationRecipes {
			surface, found := RuntimeValidationSurfaceForID(inventory.ValidationSurfaces, recipe.ID)
			if !found || surface.Readiness != RuntimeValidationSurfaceReady {
				return fmt.Errorf("runtime Shell admissibility confirmed recipe %q without a ready surface", recipe.ID)
			}
		}
	}
	if err := ValidateRuntimeValidationProofRequirementCatalogApplicability(request.ValidationProofRequirements, []RuntimeOperationAdmissibility{inventory}); err != nil {
		return err
	}
	return nil
}

// ValidateRuntimeValidationProofRequirementCatalogApplicability verifies the
// exact Definition-requirement participation of owner catalogs without
// exposing capability-specific classification to callers. A ready managed
// validation surface compatible with a requested requirement must publish one
// exact binding; no catalog may invent, widen, or duplicate requirement
// authority. Other typed validation owners remain outside this Shell proof
// protocol.
func ValidateRuntimeValidationProofRequirementCatalogApplicability(requirements []RuntimeValidationProofRequirement, catalogs []RuntimeOperationAdmissibility) error {
	return validateRuntimeValidationProofRequirementCatalogApplicability(requirements, catalogs, nil)
}

func validateRuntimeValidationProofRequirementCatalogApplicability(requirements []RuntimeValidationProofRequirement, catalogs []RuntimeOperationAdmissibility, prior *runtimeValidationCatalogRequirementPreimage) error {
	matched := prior != nil && reflect.DeepEqual(prior.raw, requirements)
	if !matched {
		if err := ValidateRuntimeValidationProofRequirements(requirements); err != nil {
			return fmt.Errorf("runtime validation proof requirements: %w", err)
		}
	}
	if err := ValidateRuntimeOperationAdmissibilities(catalogs); err != nil {
		return fmt.Errorf("runtime validation proof owner catalogs: %w", err)
	}
	var requested []RuntimeValidationProofRequirement
	if matched {
		requested = prior.requested
	} else {
		requested = NormalizeRuntimeValidationProofRequirements(requirements)
	}
	for _, rawCatalog := range catalogs {
		catalog := rawCatalog.Normalized()
		if catalog.OwnerModuleID != RuntimeModuleShell || !runtimeOperationAdmissibilityKindsContain(catalog.OperationKinds, RuntimeBlockKindManagedValidationSession) {
			continue
		}
		for _, surface := range catalog.ValidationSurfaces {
			seenSurfaceRequirements := make(map[string]struct{}, len(surface.ProofRequirementBindings)+len(surface.ProofRequirementPreparations))
			for _, binding := range surface.ProofRequirementBindings {
				requirement, found := RuntimeValidationProofRequirementForCoverage(requested, binding.CoverageWorkItemID)
				if !found || requirement.Hash != binding.RequirementHash || requirement.DefinitionRevision != binding.DefinitionRevision ||
					requirement.DeclarationRef != binding.DeclarationRef || binding.ValidationRecipeID != surface.ID ||
					(requirement.ValidationRecipeID != "" && requirement.ValidationRecipeID != binding.ValidationRecipeID) ||
					!sameStrings(requirement.RequiredCheckRefs, binding.RequiredCheckRefs) {
					return fmt.Errorf("runtime validation owner catalog published undeclared proof requirement binding %q", binding.RequirementHash)
				}
				seenSurfaceRequirements[binding.RequirementHash] = struct{}{}
			}
			for _, preparation := range surface.ProofRequirementPreparations {
				requirement, found := RuntimeValidationProofRequirementForCoverage(requested, preparation.CoverageWorkItemID)
				attestedChecks := make([]string, 0, len(preparation.CheckAttestations))
				for _, attestation := range preparation.CheckAttestations {
					attestedChecks = append(attestedChecks, attestation.RequiredCheckRef)
				}
				if !found || requirement.Hash != preparation.RequirementHash || requirement.DefinitionRevision != preparation.DefinitionRevision ||
					requirement.DeclarationRef != preparation.DeclarationRef || preparation.ValidationRecipeID != surface.ID ||
					(requirement.ValidationRecipeID != "" && requirement.ValidationRecipeID != preparation.ValidationRecipeID) ||
					!sameStrings(requirement.RequiredCheckRefs, attestedChecks) {
					return fmt.Errorf("runtime validation owner catalog published undeclared proof requirement preparation %q", preparation.RequirementHash)
				}
				seenSurfaceRequirements[preparation.RequirementHash] = struct{}{}
			}
			for _, unavailable := range surface.ProofRequirementUnavailabilities {
				requirement, found := RuntimeValidationProofRequirementForCoverage(requested, unavailable.Requirement.CoverageWorkItemID)
				if unavailable.Validate() != nil || !found || requirement.Hash != unavailable.Requirement.Hash || unavailable.ValidationRecipeID != surface.ID {
					return fmt.Errorf("runtime validation owner catalog published undeclared proof unavailability %q", unavailable.Requirement.Hash)
				}
				seenSurfaceRequirements[requirement.Hash] = struct{}{}
			}
			for _, requirement := range requested {
				if requirement.ValidationRecipeID != "" && requirement.ValidationRecipeID != surface.ID {
					continue
				}
				if surface.Readiness == RuntimeValidationSurfaceUnavailable || surface.ProofApplicability == RuntimeValidationProofAuxiliary {
					continue
				}
				if surface.ProofApplicability != RuntimeValidationProofDirect {
					return fmt.Errorf("runtime validation owner catalog recipe %q omitted applicability for requirement %q", surface.ID, requirement.Hash)
				}
				if _, exact := seenSurfaceRequirements[requirement.Hash]; !exact {
					return fmt.Errorf("runtime validation owner catalog recipe %q omitted exact ready-or-prerequisite record %q", surface.ID, requirement.Hash)
				}
			}
		}
	}
	return nil
}

func runtimeObservationDependsOnWorkspaceTreeRevision(observation RuntimePrerequisiteObservation, ownerModuleID, revision string) bool {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	revision = strings.TrimSpace(revision)
	for _, dependency := range observation.Normalized().Dependencies {
		if dependency.Domain == RuntimeResourceWorkspaceTree && ownerModuleID != "" && dependency.OwnerModuleID == ownerModuleID &&
			dependency.SubjectRef == "workspace_deliverable" && dependency.Revision == revision {
			return true
		}
	}
	return false
}

func sameRuntimeValidationRecipe(left, right RuntimeValidationRecipe) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.ID == right.ID && left.AdapterRef == right.AdapterRef && left.InputReviewProtocolRef == right.InputReviewProtocolRef && left.Command == right.Command && left.TimeoutMillis == right.TimeoutMillis && sameStrings(left.Args, right.Args)
}

func runtimeValidationRecipeConfirmsDeclaration(declared, confirmed RuntimeValidationRecipe) bool {
	declared = declared.Normalized()
	confirmed = confirmed.Normalized()
	if declared.ValidateDeclaration() != nil || confirmed.Validate() != nil || declared.ID != confirmed.ID ||
		declared.AdapterRef != confirmed.AdapterRef || declared.InputReviewProtocolRef != confirmed.InputReviewProtocolRef || declared.TimeoutMillis != confirmed.TimeoutMillis {
		return false
	}
	if declared.Command == "" {
		return declared.AdapterRef != ""
	}
	return declared.Command == confirmed.Command && sameStrings(declared.Args, confirmed.Args)
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
	OperationID                  string
	AdmissibilitySnapshotID      string
	CapabilityStateBinding       string
	CandidateManifestHash        string
	OwnerPolicy                  RuntimeOwnerPolicyIdentity
	ConstraintImpact             RuntimeOwnerFeasibilityConstraintImpact
	Outcome                      RuntimeOwnerFeasibilityOutcome
	CandidateConstraintViolation *RuntimeOperationCandidateConstraintViolation
	Feedback                     *RuntimeFeedback
	ReplayProof                  *RuntimeOperationReplayProof
	Stale                        *RuntimePrerequisiteStale
	PrerequisiteRequest          *RuntimeOperationPrerequisiteRequest
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
	if err := ValidateRuntimeOperationTargetsAgainstAuthorization(authorization.Target, operation.TargetRefs, authorization.RequireTargetRefs); err != nil {
		return fmt.Errorf("runtime operation feasibility target authorization: %w", err)
	}
	expectedBindings := runtimeCandidateTargetParameterBindings(operation.Kind, authorization.Target, []RuntimeOperationAdmissibility{inventory}, ownerModuleID)
	if !reflect.DeepEqual(expectedBindings, authorization.TargetParameterBindings) {
		return fmt.Errorf("runtime operation feasibility target preimages do not match owner catalog")
	}
	if err := validateRuntimeOperationTargetParameterPreimage(operation, authorization); err != nil {
		return fmt.Errorf("runtime operation feasibility target precondition: %w", err)
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
	result.OwnerPolicy = result.OwnerPolicy.Normalized()
	result.ConstraintImpact = RuntimeOwnerFeasibilityConstraintImpact(strings.TrimSpace(string(result.ConstraintImpact)))
	result.Outcome = RuntimeOwnerFeasibilityOutcome(strings.TrimSpace(string(result.Outcome)))
	if result.CandidateConstraintViolation != nil {
		value := result.CandidateConstraintViolation.Normalized()
		result.CandidateConstraintViolation = &value
	}
	if result.OperationID == "" || result.AdmissibilitySnapshotID == "" {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility result requires operation and admissibility identity")
	}
	if !ValidRuntimeOwnerFeasibilityConstraintImpact(result.ConstraintImpact) {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility has unknown constraint impact %q", result.ConstraintImpact)
	}
	if result.OwnerPolicy.OwnerModuleID != "" || result.OwnerPolicy.PolicyRevision != "" || result.OwnerPolicy.CatalogSnapshotID != "" {
		if err := result.OwnerPolicy.Validate(); err != nil {
			return RuntimeOperationFeasibilityObservation{}, err
		}
	}
	if result.ConstraintImpact != "" && result.Outcome != RuntimeOwnerFeasibilityInfeasible {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility constraint impact requires infeasible outcome")
	}
	switch result.Outcome {
	case RuntimeOwnerFeasibilityFeasible:
		if result.Stale != nil || result.CandidateConstraintViolation != nil || result.Feedback != nil {
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
		if result.CandidateConstraintViolation != nil || result.Feedback != nil {
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
		if result.PrerequisiteRequest == nil || result.CandidateConstraintViolation != nil || result.Feedback == nil || result.Stale != nil || result.ReplayProof != nil {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility prerequisite outcome requires feedback and prerequisite detail")
		}
	case RuntimeOwnerFeasibilityCandidateConstraintViolated:
		if result.CandidateConstraintViolation == nil || result.Feedback != nil || result.Stale != nil || result.PrerequisiteRequest != nil || result.ReplayProof != nil {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility candidate-constraint outcome requires only a bounded violation")
		}
		if err := result.CandidateConstraintViolation.Validate(); err != nil {
			return RuntimeOperationFeasibilityObservation{}, err
		}
		return result, nil
	case RuntimeOwnerFeasibilityInfeasible, RuntimeOwnerFeasibilityContractDefect:
		if result.CandidateConstraintViolation != nil || result.Feedback == nil || result.Stale != nil || result.PrerequisiteRequest != nil || result.ReplayProof != nil {
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
	if result.ConstraintImpact != "" {
		if category != RuntimeOwnerFeasibilityCategoryPolicyDisallowed {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility constraint impact requires policy-disallowed category")
		}
		if err := result.OwnerPolicy.Validate(); err != nil {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility constraint impact requires owner policy identity: %w", err)
		}
		if feedback.OriginalDiagnostic.OwnerModuleID != result.OwnerPolicy.OwnerModuleID || strings.TrimSpace(feedback.OriginalDiagnostic.ObservedRevision) == "" {
			return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation feasibility constraint impact requires matching owner and observed resource revision")
		}
	}
	result.Feedback = &feedback
	return result, nil
}

// RuntimeOwnerFeasibilityObservationFromOperation converts the concrete
// capability-owner operation result into the persisted Orchestrator fact
// without interpreting owner diagnostic text or selecting a transition.
func RuntimeOwnerFeasibilityObservationFromOperation(result RuntimeOperationFeasibilityObservation) (RuntimeOwnerFeasibilityObservation, error) {
	result, err := NormalizeRuntimeOperationFeasibilityObservation(result)
	if err != nil {
		return RuntimeOwnerFeasibilityObservation{}, err
	}
	observation := RuntimeOwnerFeasibilityObservation{
		Outcome: result.Outcome, OwnerPolicy: result.OwnerPolicy,
		ConstraintImpact: result.ConstraintImpact, Feedback: result.Feedback,
		CandidateConstraintViolation: result.CandidateConstraintViolation,
		Stale:                        result.Stale, PrerequisiteRequest: result.PrerequisiteRequest,
	}
	return observation, observation.Validate()
}

// NewRuntimeOperationCandidateConstraintViolationObservation seals one
// owner-observed candidate applicability fact against the exact request
// manifest, operation target and admissibility revision. It carries no owner
// failure category because the resource itself remains feasible.
func NewRuntimeOperationCandidateConstraintViolationObservation(request RuntimeOperationFeasibilityRequest, violation RuntimeOperationCandidateConstraintViolation) (RuntimeOperationFeasibilityObservation, error) {
	operation := request.Operation.Normalized()
	violation = violation.Normalized()
	if err := ValidateRuntimeOperationFeasibilityRequest(request, request.OperationAuthorization.Normalized().OwnerModuleID); err != nil {
		return RuntimeOperationFeasibilityObservation{}, err
	}
	if err := violation.Validate(); err != nil {
		return RuntimeOperationFeasibilityObservation{}, err
	}
	_, constraints, err := RuntimeOperationCandidateConstraintsForOperation(operation, request.OperationAuthorization)
	if err != nil {
		return RuntimeOperationFeasibilityObservation{}, err
	}
	matched := false
	for _, constraint := range constraints {
		if constraint.Kind == RuntimeOperationCandidateConstraintTargetApplicability &&
			constraint.Ref == violation.ConstraintRef && constraint.ParameterName == violation.ParameterName && constraint.RequiredShape == violation.RequiredShape {
			matched = true
			break
		}
	}
	if !matched {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation candidate applicability violation is not sealed in the operation authorization")
	}
	if violation.ObservedRevision != request.Admissibility.Normalized().ObservedRevision || !containsRuntimeString(operation.TargetRefs, violation.TargetRef) {
		return RuntimeOperationFeasibilityObservation{}, fmt.Errorf("runtime operation candidate applicability violation does not match the owner observation")
	}
	result := RuntimeOperationFeasibilityObservation{
		OperationID: operation.ID, AdmissibilitySnapshotID: request.Admissibility.SnapshotID,
		CapabilityStateBinding: request.Admissibility.AcquisitionBinding, CandidateManifestHash: request.CandidateManifestHash,
		Outcome: RuntimeOwnerFeasibilityCandidateConstraintViolated, CandidateConstraintViolation: &violation,
	}
	return NormalizeRuntimeOperationFeasibilityObservation(result)
}

func containsRuntimeString(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
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
	binding := request.WorkspaceManifestBinding.Normalized()
	if !binding.Empty() {
		if err := binding.Validate(); err != nil {
			return err
		}
	}
	if len(request.ValidationRecipes) == 0 {
		if len(request.ValidationProofRequirements) != 0 {
			return fmt.Errorf("runtime validation proof requirements require declared recipes")
		}
		return nil
	}
	if ownerModuleID != RuntimeModuleShell || !runtimeOperationAdmissibilityKindsContain(request.RequiredKinds, RuntimeBlockKindManagedValidationSession) {
		return fmt.Errorf("runtime validation recipes require Shell managed-validation admissibility")
	}
	if binding.Empty() {
		return fmt.Errorf("runtime Shell validation recipes require a File-owner workspace manifest binding")
	}
	seen := make(map[string]struct{}, len(request.ValidationRecipes))
	for _, recipe := range request.ValidationRecipes {
		recipe = recipe.Normalized()
		if err := recipe.ValidateDeclaration(); err != nil {
			return fmt.Errorf("runtime validation recipe declaration: %w", err)
		}
		if _, duplicate := seen[recipe.ID]; duplicate {
			return fmt.Errorf("runtime validation recipe %q is duplicated", recipe.ID)
		}
		seen[recipe.ID] = struct{}{}
	}
	if err := ValidateRuntimeValidationProofRequirements(request.ValidationProofRequirements); err != nil {
		return err
	}
	for _, requirement := range request.ValidationProofRequirements {
		requirement = requirement.Normalized()
		if requirement.ValidationRecipeID != "" {
			if _, declared := seen[requirement.ValidationRecipeID]; declared {
				continue
			}
			return fmt.Errorf("runtime validation proof requirement %q references undeclared recipe %q", requirement.Hash, requirement.ValidationRecipeID)
		}
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
