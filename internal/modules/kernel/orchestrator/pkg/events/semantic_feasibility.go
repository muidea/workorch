package events

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ValidateRuntimeSemanticProofFeasibilityCommand validates only immutable
// scope and catalog membership. Mutable-resource freshness and target facts
// remain the addressed capability owner's responsibility.
func ValidateRuntimeSemanticProofFeasibilityCommand(command ProbeRuntimeSemanticProofFeasibilityCommand, ownerModuleID string) error {
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

func runtimeSemanticManifestContainsProposal(manifest runtimecontract.RuntimeSemanticPlanManifest, proposal runtimecontract.RuntimeSemanticProofProposal) bool {
	coverageFound := false
	for _, item := range manifest.CoverageItems {
		if item.ID == proposal.CoverageWorkItemID {
			coverageFound = true
			break
		}
	}
	return coverageFound && containsString(manifest.AllowedOwnerModuleIDs, proposal.OwnerModuleID) && containsCanonicalOperationKind(manifest.AllowedOperationKinds, proposal.OperationKind)
}

func validateSemanticProposalAgainstOwnerCatalog(proposal runtimecontract.RuntimeSemanticProofProposal, catalog runtimecontract.RuntimeOperationAdmissibility) error {
	target := proposal.Target.Normalized()
	switch target.Policy {
	case runtimecontract.RuntimeOperationTargetPolicyExactTarget:
		if len(catalog.TargetRefs) != 0 && !allStringsContained(target.ExactTargetRefs, catalog.TargetRefs) {
			return fmt.Errorf("runtime semantic exact target is outside owner catalog")
		}
	case runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope:
		if len(target.OwnerScopeTargetRefs) != 0 && !allStringsContained(target.OwnerScopeTargetRefs, catalog.TargetRefs) {
			return fmt.Errorf("runtime semantic owner scope is outside owner catalog")
		}
		if len(target.OwnerSelectorRefs) != 0 && !allStringsContained(target.OwnerSelectorRefs, catalog.SelectorRefs) {
			return fmt.Errorf("runtime semantic owner selector is outside owner catalog")
		}
	case runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if !allStringsContained(target.OwnerSelectorRefs, catalog.SelectorRefs) || !allStringsContained(target.ProspectiveConstraintRefs, catalog.ConstraintRefs) {
			return fmt.Errorf("runtime semantic prospective target policy is outside owner catalog")
		}
	}
	if proposal.OperationVariantID != "" {
		contract, found := runtimecontract.RuntimeOperationContractForKind(proposal.OperationKind)
		if !found || !runtimeSemanticVariantExists(contract, proposal.OperationVariantID) {
			return fmt.Errorf("runtime semantic operation variant %q is outside owner contract", proposal.OperationVariantID)
		}
	}
	if proposal.ValidationRecipeID != "" {
		if !allStringsContained([]string{proposal.ValidationRecipeID}, catalog.ValidationRecipeRefs) {
			return fmt.Errorf("runtime semantic validation recipe %q is outside owner catalog", proposal.ValidationRecipeID)
		}
	}
	return nil
}

func runtimeSemanticVariantExists(contract runtimecontract.RuntimeOperationContract, variantID string) bool {
	variantID = strings.TrimSpace(variantID)
	for _, variant := range contract.Variants {
		if variant.ID == variantID {
			return true
		}
	}
	return false
}

func containsCanonicalOperationKind(values []string, wanted string) bool {
	wanted = runtimecontract.CanonicalRuntimeBlockKind(wanted)
	for _, value := range values {
		if runtimecontract.CanonicalRuntimeBlockKind(value) == wanted {
			return true
		}
	}
	return false
}

func containsString(values []string, wanted string) bool {
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
		if !containsString(allowed, value) {
			return false
		}
	}
	return true
}

// NewRuntimeSemanticProofFeasibilityResult builds the exact owner observation.
// Non-feasible statuses require an original, audit-safe diagnostic; this
// constructor never summarizes or rewrites that diagnostic.
func NewRuntimeSemanticProofFeasibilityResult(command ProbeRuntimeSemanticProofFeasibilityCommand, status runtimecontract.RuntimeSemanticFeasibilityStatus, observedRevision string, targetRefs []string, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) (ProbeRuntimeSemanticProofFeasibilityResult, error) {
	proposal := command.Proposal.Normalized()
	observation := runtimecontract.RuntimeSemanticProofFeasibility{
		SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
		ManifestHash:  command.Manifest.Normalized().Hash, DefinitionSeedHash: command.Manifest.Normalized().DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal),
		OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: command.OwnerCatalog.SnapshotID, Status: status, ObservedRevision: strings.TrimSpace(observedRevision),
		TargetRefs: append([]string(nil), targetRefs...), ValidationRecipeID: proposal.ValidationRecipeID,
		RequiredCheckRefs: append([]string(nil), proposal.RequiredCheckRefs...), OriginalDiagnostic: diagnostic.CloneOriginal(),
	}
	if command.CatalogObservation != nil {
		observation.CatalogObservationID = command.CatalogObservation.ObservationID
	}
	if err := observation.ValidateForProposal(command.Manifest, proposal); err != nil {
		return ProbeRuntimeSemanticProofFeasibilityResult{}, err
	}
	return ProbeRuntimeSemanticProofFeasibilityResult{FeasibilityBatchID: strings.TrimSpace(command.FeasibilityBatchID), Observation: observation}, nil
}

func RuntimeSemanticFeasibilityDiagnostic(ownerModuleID, code, message, expectedRevision, observedRevision string, targetRefs []string) *runtimecontract.RuntimeOriginalDiagnostic {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" {
		code = "semantic_proof_infeasible"
	}
	if message == "" {
		message = "runtime semantic proof is not currently feasible"
	}
	return &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: ownerModuleID,
		OriginalClass: "semantic_feasibility", OriginalCode: code, OriginalMessage: message,
		Signature: "semantic-feasibility/" + ownerModuleID + "/" + code, TargetRefs: append([]string(nil), targetRefs...),
		ExpectedRevision: strings.TrimSpace(expectedRevision), ObservedRevision: strings.TrimSpace(observedRevision),
	}
}

// RuntimeSemanticProposalTargetRefs returns the exact target surface the owner
// observed without interpreting Goal or DoD text.
func RuntimeSemanticProposalTargetRefs(proposal runtimecontract.RuntimeSemanticProofProposal) []string {
	target := proposal.Normalized().Target
	switch target.Policy {
	case runtimecontract.RuntimeOperationTargetPolicyExactTarget:
		return append([]string(nil), target.ExactTargetRefs...)
	case runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope:
		return append([]string(nil), target.OwnerScopeTargetRefs...)
	case runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if target.ProspectiveTargetRef != "" {
			return []string{target.ProspectiveTargetRef}
		}
	}
	return nil
}
