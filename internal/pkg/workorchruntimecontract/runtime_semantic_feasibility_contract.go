package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeSemanticProofFeasibilityRequest is the owner-neutral value used by
// Orchestrator to construct one concrete receiver-owned EventHub command. It
// is not itself an EventHub command.
type RuntimeSemanticProofFeasibilityRequest struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	FeasibilityBatchID            string
	WorkspaceDeliverableDirectory string
	Manifest                      RuntimeSemanticPlanManifest
	Proposal                      RuntimeSemanticProofProposal
	OwnerCatalog                  RuntimeOperationAdmissibility
	CatalogObservation            *RuntimePrerequisiteObservation
}

type RuntimeSemanticProofFeasibilityObservation struct {
	FeasibilityBatchID string
	Observation        RuntimeSemanticProofFeasibility
}

// ValidateRuntimeSemanticProofFeasibilityCommand validates only immutable
// scope and catalog membership. Mutable-resource freshness and target facts
// remain the addressed capability owner's responsibility.
func ValidateRuntimeSemanticProofFeasibilityRequest(command RuntimeSemanticProofFeasibilityRequest, ownerModuleID string) error {
	manifest := command.Manifest.Normalized()
	proposal := command.Proposal.Normalized()
	catalog := command.OwnerCatalog.Normalized()
	if command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.FeasibilityBatchID) == "" ||
		strings.TrimSpace(command.WorkspaceDeliverableDirectory) == "" || strings.TrimSpace(ownerModuleID) == "" {
		return common.ErrInvalidArgument
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if manifest.WorkspaceID != command.WorkspaceID || manifest.RunID != command.RunID {
		return fmt.Errorf("runtime semantic feasibility manifest scope does not match command")
	}
	if err := proposal.Validate(); err != nil {
		return err
	}
	if proposal.OwnerModuleID != ownerModuleID {
		return fmt.Errorf("runtime semantic feasibility proposal owner %q does not match receiver %q", proposal.OwnerModuleID, ownerModuleID)
	}
	if acquisition, found := runtimeSemanticOwnerFactAcquisitionForCoverage(manifest, proposal.CoverageWorkItemID); found {
		if err := validateRuntimeSemanticOwnerFactAcquisition(RuntimeSemanticCWIProofSet{
			CoverageWorkItemID: proposal.CoverageWorkItemID,
			Proposals:          []RuntimeSemanticProofProposal{proposal},
		}, &acquisition, 0); err != nil {
			return err
		}
	}
	if !runtimeSemanticManifestContainsProposal(manifest, proposal) {
		return fmt.Errorf("runtime semantic feasibility proposal is outside sealed manifest")
	}
	if err := catalog.Validate(); err != nil {
		return err
	}
	if catalog.OwnerModuleID != ownerModuleID || !containsCanonicalOperationKind(catalog.OperationKinds, proposal.OperationKind) {
		return fmt.Errorf("runtime semantic feasibility proposal is absent from owner catalog")
	}
	if command.CatalogObservation != nil {
		observation := command.CatalogObservation.Normalized()
		if err := observation.Validate(); err != nil {
			return err
		}
		if observation.PrerequisiteID != catalog.PrerequisiteID || observation.OwnerModuleID != ownerModuleID || observation.ContentHash != catalog.SnapshotID {
			return fmt.Errorf("runtime semantic feasibility catalog observation identity does not match catalog")
		}
	}
	if err := validateSemanticProposalAgainstOwnerCatalog(proposal, catalog); err != nil {
		return err
	}
	return nil
}

func runtimeSemanticManifestContainsProposal(manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal) bool {
	coverageFound := false
	for _, item := range manifest.CoverageItems {
		if item.ID == proposal.CoverageWorkItemID {
			coverageFound = true
			break
		}
	}
	return coverageFound && semanticContainsString(manifest.AllowedOwnerModuleIDs, proposal.OwnerModuleID) && containsCanonicalOperationKind(manifest.AllowedOperationKinds, proposal.OperationKind)
}

func validateSemanticProposalAgainstOwnerCatalog(proposal RuntimeSemanticProofProposal, catalog RuntimeOperationAdmissibility) error {
	proposal = proposal.Normalized()
	catalog = catalog.Normalized()
	variant, variantFound, variantErr := RuntimeOperationAdmissibilityVariantForProposal(catalog, proposal.OperationKind, proposal.OperationVariantID)
	if variantErr != nil {
		return variantErr
	}
	contract, contractFound := RuntimeOperationContractForKind(proposal.OperationKind)
	if contractFound && len(contract.Variants) != 0 && !variantFound {
		return runtimeSemanticProposalConstraint("operationVariantId", RuntimeSemanticConstraintAllowedValue, proposal.OperationVariantID, []string{"owner-admissible static operation variant"}, runtimeSemanticCatalogVariantIDs(catalog, proposal.OperationKind))
	}
	allowed, _, _, _, policyErr := RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, proposal.OperationKind, proposal.OperationVariantID, proposal.Target.Policy)
	if policyErr != nil {
		return policyErr
	}
	if !allowed {
		// Prefer an owner-catalog operation/variant delta when the selected
		// target policy is valid for another replacement surface. Returning only
		// "change target.policy" would let a create-only kind escape an exact
		// existing target by inventing a prospective target on correction. The
		// owner still supplies every allowed value; this pure evaluator only
		// projects the sealed intersection and does not choose a Runtime action.
		if variants := runtimeSemanticCatalogVariantIDsForPolicy(catalog, proposal.OperationKind, proposal.Target.Policy); len(variants) != 0 {
			return runtimeSemanticProposalConstraint("operationVariantId", RuntimeSemanticConstraintAllowedValue, proposal.OperationVariantID, nil, variants)
		}
		if kinds := runtimeSemanticCatalogOperationKindsForProposalPolicy(catalog, proposal); len(kinds) != 0 && !containsCanonicalOperationKind(kinds, proposal.OperationKind) {
			return runtimeSemanticProposalConstraint("operationKind", RuntimeSemanticConstraintAllowedValue, proposal.OperationKind, nil, kinds)
		}
		allowedPolicies := runtimeSemanticOwnerTargetPolicies(catalog, proposal.OperationKind, proposal.OperationVariantID)
		return runtimeSemanticProposalConstraint("target.policy", RuntimeSemanticConstraintAllowedValue, string(proposal.Target.Policy), nil, runtimeSemanticTargetPoliciesAsStrings(allowedPolicies))
	}
	selectorRefs := catalog.SelectorRefs
	constraintRefs := catalog.ConstraintRefs
	if variantFound {
		selectorRefs = variant.SelectorRefs
		constraintRefs = variant.ConstraintRefs
	}
	target := proposal.Target.Normalized()
	switch target.Policy {
	case RuntimeOperationTargetPolicyExactTarget:
		if !allStringsContained(target.ExactTargetRefs, catalog.TargetRefs) {
			return runtimeSemanticProposalConstraint("target.exactTargetRefs", RuntimeSemanticConstraintAllowedValue, strings.Join(target.ExactTargetRefs, ","), nil, catalog.TargetRefs)
		}
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		if len(target.OwnerScopeTargetRefs) != 0 && !allStringsContained(target.OwnerScopeTargetRefs, catalog.TargetRefs) {
			return runtimeSemanticProposalConstraint("target.ownerScopeTargetRefs", RuntimeSemanticConstraintAllowedValue, strings.Join(target.OwnerScopeTargetRefs, ","), nil, catalog.TargetRefs)
		}
		if len(target.OwnerSelectorRefs) != 0 && !allStringsContained(target.OwnerSelectorRefs, selectorRefs) {
			return runtimeSemanticProposalConstraint("target.ownerSelectorRefs", RuntimeSemanticConstraintAllowedValue, strings.Join(target.OwnerSelectorRefs, ","), nil, selectorRefs)
		}
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if !allStringsContained(target.OwnerSelectorRefs, selectorRefs) {
			return runtimeSemanticProposalConstraint("target.ownerSelectorRefs", RuntimeSemanticConstraintAllowedValue, strings.Join(target.OwnerSelectorRefs, ","), nil, selectorRefs)
		}
		if !allStringsContained(target.ProspectiveConstraintRefs, constraintRefs) {
			return runtimeSemanticProposalConstraint("target.prospectiveConstraintRefs", RuntimeSemanticConstraintAllowedValue, strings.Join(target.ProspectiveConstraintRefs, ","), nil, constraintRefs)
		}
		if semanticContainsString(catalog.TargetRefs, target.ProspectiveTargetRef) {
			return runtimeSemanticProposalConstraint("target.prospectiveTargetRef", RuntimeSemanticConstraintRelational, target.ProspectiveTargetRef, []string{"target absent from sealed existing target set"}, nil)
		}
	}
	if proposal.ValidationRecipeID != "" {
		if !allStringsContained([]string{proposal.ValidationRecipeID}, catalog.ValidationRecipeRefs) {
			return runtimeSemanticProposalConstraint("validationRecipeId", RuntimeSemanticConstraintAllowedValue, proposal.ValidationRecipeID, nil, catalog.ValidationRecipeRefs)
		}
		// Every validation recipe is bound to its exact owner-published typed
		// surface. Do not derive protocol-specific semantics from its identity.
		if surface, found := RuntimeValidationSurfaceForID(catalog.ValidationSurfaces, proposal.ValidationRecipeID); found {
			actual := surface.Normalized().EffectScope
			if actual == RuntimeEffectUnknown || !allStringsContained([]string{string(actual)}, RuntimeEffectScopesAsStrings(proposal.RequiredEffectScopes)) {
				return runtimeSemanticProposalConstraint("requiredEffectScopes", RuntimeSemanticConstraintAllowedValue,
					strings.Join(RuntimeEffectScopesAsStrings(proposal.RequiredEffectScopes), ","), nil, []string{string(actual)})
			}
		}
	}
	return nil
}

// runtimeSemanticCatalogVariantIDsForPolicy returns only variants published by
// one owner catalog that can use the requested target policy. It is a pure
// projection over the persisted owner fact, not a process-global operation
// registry and not a selection decision.
func runtimeSemanticCatalogVariantIDsForPolicy(catalog RuntimeOperationAdmissibility, kind string, policy RuntimeOperationTargetPolicy) []string {
	catalog = catalog.Normalized()
	kind = CanonicalRuntimeBlockKind(kind)
	contract, found := RuntimeOperationContractForKind(kind)
	if !found || len(contract.Variants) == 0 {
		return nil
	}
	ret := make([]string, 0)
	for _, variant := range catalog.OperationVariants {
		variant = variant.Normalized()
		if variant.Kind != kind || variant.VariantID == "" || !runtimeOperationAdmissibilityVariantAllowsTargetPolicy(variant, policy) {
			continue
		}
		ret = append(ret, variant.VariantID)
	}
	return normalizeStrings(ret)
}

// runtimeSemanticCatalogOperationKindsForProposalPolicy returns the bounded
// operation-kind alternatives that the same owner catalog authorizes for the
// proposal's evidence kind and target policy. This lets a correction replace a
// create-only kind with an owner-admissible replacement kind without parsing
// File-specific diagnostics or widening the sealed manifest.
func runtimeSemanticCatalogOperationKindsForProposalPolicy(catalog RuntimeOperationAdmissibility, proposal RuntimeSemanticProofProposal) []string {
	catalog = catalog.Normalized()
	proposal = proposal.Normalized()
	evidenceKind := ""
	if len(proposal.RequiredEvidenceKinds) == 1 {
		evidenceKind = proposal.RequiredEvidenceKinds[0]
	}
	ret := make([]string, 0, len(catalog.OperationKinds))
	for _, kind := range catalog.OperationKinds {
		kind = CanonicalRuntimeBlockKind(kind)
		if kind == "" || !RuntimeOperationKindSupportsCoverageEvidence(kind, evidenceKind) {
			continue
		}
		contract, found := RuntimeOperationContractForKind(kind)
		if found && len(contract.Variants) != 0 {
			if len(runtimeSemanticCatalogVariantIDsForPolicy(catalog, kind, proposal.Target.Policy)) != 0 {
				ret = append(ret, kind)
			}
			continue
		}
		allowed, _, _, _, err := RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, kind, "", proposal.Target.Policy)
		if err == nil && allowed {
			ret = append(ret, kind)
		}
	}
	return canonicalizeOperationKinds(ret)
}

func runtimeSemanticOwnerTargetPolicies(catalog RuntimeOperationAdmissibility, kind, variantID string) []RuntimeOperationTargetPolicy {
	ret := make([]RuntimeOperationTargetPolicy, 0, 3)
	for _, policy := range []RuntimeOperationTargetPolicy{
		RuntimeOperationTargetPolicyExactTarget,
		RuntimeOperationTargetPolicyExistingOwnerScope,
		RuntimeOperationTargetPolicyProspectiveOwnerTarget,
	} {
		allowed, _, _, _, err := RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, kind, variantID, policy)
		if err == nil && allowed {
			ret = append(ret, policy)
		}
	}
	return ret
}

func runtimeSemanticVariantExists(contract RuntimeOperationContract, variantID string) bool {
	variantID = strings.TrimSpace(variantID)
	for _, variant := range contract.Variants {
		if variant.ID == variantID {
			return true
		}
	}
	return false
}

func containsCanonicalOperationKind(values []string, wanted string) bool {
	wanted = CanonicalRuntimeBlockKind(wanted)
	for _, value := range values {
		if CanonicalRuntimeBlockKind(value) == wanted {
			return true
		}
	}
	return false
}

func semanticContainsString(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

func allStringsContained(values, allowed []string) bool {
	for _, value := range values {
		if !semanticContainsString(allowed, value) {
			return false
		}
	}
	return true
}

// NewRuntimeSemanticProofFeasibilityResult builds the exact owner observation.
// Non-feasible statuses require an original, audit-safe diagnostic; this
// constructor never summarizes or rewrites that diagnostic.
func NewRuntimeSemanticProofFeasibilityObservation(command RuntimeSemanticProofFeasibilityRequest, status RuntimeSemanticFeasibilityStatus, observedRevision string, targetRefs []string, diagnostic *RuntimeOriginalDiagnostic) (RuntimeSemanticProofFeasibilityObservation, error) {
	return NewRuntimeSemanticProofFeasibilityOwnerObservation(command, status, observedRevision, targetRefs, nil, diagnostic)
}

// NewRuntimeSemanticProofFeasibilityOwnerObservation permits the addressed
// File owner to attach its typed prospective workspace delta before owner-
// stage validation. Required-fact-owner satisfaction is deliberately absent
// at this stage and is validated only after Orchestrator attaches it.
func NewRuntimeSemanticProofFeasibilityOwnerObservation(command RuntimeSemanticProofFeasibilityRequest, status RuntimeSemanticFeasibilityStatus, observedRevision string, targetRefs []string, prospectiveDelta *RuntimeProspectiveWorkspaceManifestDelta, diagnostic *RuntimeOriginalDiagnostic) (RuntimeSemanticProofFeasibilityObservation, error) {
	proposal := command.Proposal.Normalized()
	confirmedChecks := runtimeSemanticOwnerConfirmedCheckRefs(command.OwnerCatalog, proposal.ValidationRecipeID)
	observation := RuntimeSemanticProofFeasibility{
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent,
		ManifestHash:  command.Manifest.Normalized().Hash, DefinitionSeedHash: command.Manifest.Normalized().DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal),
		OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: command.OwnerCatalog.SnapshotID, Status: status, ObservedRevision: strings.TrimSpace(observedRevision),
		TargetRefs: append([]string(nil), targetRefs...), ValidationRecipeID: proposal.ValidationRecipeID,
		RequiredCheckRefs: confirmedChecks, OriginalDiagnostic: diagnostic.CloneOriginal(),
	}
	if prospectiveDelta != nil {
		value := prospectiveDelta.Clone()
		observation.ProspectiveWorkspaceManifestDelta = &value
	}
	if command.CatalogObservation != nil {
		observation.CatalogObservationID = command.CatalogObservation.ObservationID
	}
	if err := observation.ValidateOwnerForProposal(command.Manifest, proposal); err != nil {
		return RuntimeSemanticProofFeasibilityObservation{}, err
	}
	return RuntimeSemanticProofFeasibilityObservation{FeasibilityBatchID: strings.TrimSpace(command.FeasibilityBatchID), Observation: observation}, nil
}

// runtimeSemanticOwnerConfirmedCheckRefs derives the canonical validation
// identities from the addressed owner's exact ready/direct surface. Recipe
// identity and check identity are deliberately separate: a recipe may execute
// several tool-native checks, and the provider cannot invent either set.
func runtimeSemanticOwnerConfirmedCheckRefs(catalog RuntimeOperationAdmissibility, recipeID string) []string {
	recipeID = strings.TrimSpace(recipeID)
	if recipeID == "" {
		return nil
	}
	surface, found := RuntimeValidationSurfaceForID(catalog.ValidationSurfaces, recipeID)
	if !found {
		return nil
	}
	surface = surface.Normalized()
	if surface.Readiness != RuntimeValidationSurfaceReady || surface.ProofApplicability != RuntimeValidationProofDirect ||
		!containsString(catalog.ValidationRecipeRefs, recipeID) {
		return nil
	}
	return append([]string(nil), surface.CheckRefs...)
}

func RuntimeSemanticFeasibilityDiagnostic(ownerModuleID, code, message, expectedRevision, observedRevision string, targetRefs []string) *RuntimeOriginalDiagnostic {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" {
		code = "semantic_proof_infeasible"
	}
	if message == "" {
		message = "runtime semantic proof is not currently feasible"
	}
	return &RuntimeOriginalDiagnostic{
		SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: ownerModuleID,
		OriginalClass: "semantic_feasibility", OriginalCode: code, OriginalMessage: message,
		Signature: "semantic-feasibility/" + ownerModuleID + "/" + code, TargetRefs: append([]string(nil), targetRefs...),
		ExpectedRevision: strings.TrimSpace(expectedRevision), ObservedRevision: strings.TrimSpace(observedRevision),
	}
}

// RuntimeSemanticProposalTargetRefs returns the exact target surface the owner
// observed without interpreting Goal or DoD text.
func RuntimeSemanticProposalTargetRefs(proposal RuntimeSemanticProofProposal) []string {
	target := proposal.Normalized().Target
	switch target.Policy {
	case RuntimeOperationTargetPolicyExactTarget:
		return append([]string(nil), target.ExactTargetRefs...)
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		return append([]string(nil), target.OwnerScopeTargetRefs...)
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		// File owner feasibility must inspect the concrete prospective path for
		// traversal, existence and regular-file conflicts. It is not a current
		// catalog target, but it is still the exact owner-observed target fact.
		if target.ProspectiveTargetRef == "" {
			return nil
		}
		return []string{target.ProspectiveTargetRef}
	}
	return nil
}
