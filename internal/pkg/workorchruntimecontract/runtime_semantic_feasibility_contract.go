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
		// Dynamic MCP recipes are bound to the exact effect fact published by
		// the MCP owner.  Do not let a provider choose a broader or different
		// effect and defer the mismatch until dispatch/evidence commit.
		if proposal.OperationKind == StepKindMCPToolCall {
			if capability, found := runtimeMCPCapabilityForValidationRecipe(catalog, proposal.ValidationRecipeID); found {
				actual := NormalizeRuntimeEffectScope(RuntimeEffectScope(capability.EffectScope))
				if actual == RuntimeEffectUnknown || !allStringsContained([]string{string(actual)}, RuntimeEffectScopesAsStrings(proposal.RequiredEffectScopes)) {
					return runtimeSemanticProposalConstraint("requiredEffectScopes", RuntimeSemanticConstraintAllowedValue,
						strings.Join(RuntimeEffectScopesAsStrings(proposal.RequiredEffectScopes), ","), nil, []string{string(actual)})
				}
			}
		}
	}
	return nil
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

func runtimeMCPCapabilityForValidationRecipe(catalog RuntimeOperationAdmissibility, recipeID string) (RuntimeMCPCapabilityContext, bool) {
	parts := strings.SplitN(strings.TrimSpace(recipeID), "/", 4)
	if len(parts) != 4 || parts[0] != "mcp" || parts[1] != "tool_call" {
		return RuntimeMCPCapabilityContext{}, false
	}
	serverID, tool := parts[2], parts[3]
	for _, capability := range catalog.MCPCapabilities {
		capability = SealMCPCapabilityEffect(capability)
		if capability.Enabled && string(capability.ServerID) == serverID && capability.Name == tool &&
			CanonicalRuntimeBlockKind(capability.OperationKind) == StepKindMCPToolCall {
			return capability, true
		}
	}
	return RuntimeMCPCapabilityContext{}, false
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
	if command.CatalogObservation != nil {
		observation.CatalogObservationID = command.CatalogObservation.ObservationID
	}
	if err := observation.ValidateForProposal(command.Manifest, proposal); err != nil {
		return RuntimeSemanticProofFeasibilityObservation{}, err
	}
	return RuntimeSemanticProofFeasibilityObservation{FeasibilityBatchID: strings.TrimSpace(command.FeasibilityBatchID), Observation: observation}, nil
}

// runtimeSemanticOwnerConfirmedCheckRefs derives the canonical validation
// identity from the addressed owner's sealed catalog. Provider proposals do
// not carry check refs and therefore cannot influence this value.
func runtimeSemanticOwnerConfirmedCheckRefs(catalog RuntimeOperationAdmissibility, recipeID string) []string {
	recipeID = strings.TrimSpace(recipeID)
	if recipeID == "" {
		return nil
	}
	for _, recipe := range catalog.ValidationRecipes {
		if strings.TrimSpace(recipe.ID) == recipeID {
			return []string{recipeID}
		}
	}
	for _, value := range catalog.ValidationRecipeRefs {
		if strings.TrimSpace(value) == recipeID {
			return []string{recipeID}
		}
	}
	return nil
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
