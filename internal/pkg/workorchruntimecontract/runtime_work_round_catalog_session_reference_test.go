package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
)

// Complete pre-session round and working-binding gates.
func referenceWorkRoundCatalogSession(r RuntimeImplementationWorkRound) error {
	for _, ref := range []string{string(r.WorkspaceID), string(r.RunID), r.StartupInputHash, r.DefinitionSeedHash, string(r.CoverageWorkItemID), r.PolicyHash} {
		if !implementationWorkExactRef(ref) {
			return fmt.Errorf("implementation work round requires exact bounded startup/CWI identity")
		}
	}
	request := r.ScopeRequest
	if err := request.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(request.Policy, request.Policy.Normalized()) || !reflect.DeepEqual(request.ManifestBinding, request.ManifestBinding.Normalized()) {
		return fmt.Errorf("implementation work round policy/manifest must be raw-canonical")
	}
	if r.WorkspaceID != request.WorkspaceID || r.RunID != request.RunID || r.DefinitionSeedHash != request.DefinitionSeedHash || r.CoverageWorkItemID != request.CoverageWorkItemID ||
		r.PolicyHash != StableRuntimeImplementationWorkPolicyHash(request.Policy) {
		return fmt.Errorf("implementation work round scope/policy source mismatch")
	}
	if r.Ordinal < 1 || r.Ordinal > request.Policy.MaxRoundsPerCWI || (r.Ordinal == 1) != (r.PrecedingReceiptSetHash == "") {
		return fmt.Errorf("implementation work round ordinal/preceding receipt lineage is invalid")
	}
	if r.PrecedingReceiptSetHash != "" && !implementationWorkExactRef(r.PrecedingReceiptSetHash) {
		return fmt.Errorf("implementation work preceding receipt set must be exact")
	}
	if r.ScopeObservation != nil {
		if err := r.ScopeObservation.ValidateForRequest(request); err != nil {
			return err
		}
		if !reflect.DeepEqual(r.ScopeObservation.Selection, r.ScopeObservation.Selection.Normalized()) ||
			!reflect.DeepEqual(r.ScopeObservation.Observation, r.ScopeObservation.Observation.Normalized()) {
			return fmt.Errorf("implementation work observation must retain canonical complete owner facts")
		}
	}
	if r.WorkingBinding != nil {
		if r.ScopeObservation == nil {
			return fmt.Errorf("implementation working binding has no scope observation")
		}
		if err := referenceWorkingBindingCatalogSession(*r.WorkingBinding, r); err != nil {
			return err
		}
	}
	if (r.DispatchID == "") != (r.DispatchAuthorityHash == "") || (r.DispatchID != "" && r.WorkingBinding == nil) {
		return fmt.Errorf("implementation work dispatch requires exact sealed working binding")
	}
	for _, ref := range []string{r.DispatchID, r.DispatchAuthorityHash, r.CompletedReceiptSetHash, r.FailedReceiptHash} {
		if ref != "" && !implementationWorkExactRef(ref) {
			return fmt.Errorf("implementation work dispatch/receipt identity must be exact")
		}
	}
	if (r.OutcomeUnknownExecutionKey == "") != (r.OutcomeUnknownFence == 0) || r.OutcomeUnknownExecutionKey != "" && (r.DispatchID == "" || !implementationWorkExactRef(r.OutcomeUnknownExecutionKey) || r.CompletedReceiptSetHash != "" || r.FailedReceiptHash != "" || r.GovernanceReceiptHash != "") {
		return fmt.Errorf("implementation unknown outcome requires original unsettled physical identity")
	}
	if r.CompletedReceiptSetHash != "" && r.FailedReceiptHash != "" {
		return fmt.Errorf("implementation work cannot be both collectively complete and failed")
	}
	if r.FailedReceiptHash != "" && r.DispatchID == "" {
		return fmt.Errorf("implementation work failure has no original dispatch")
	}
	if (r.FailedReceiptHash == "") != (r.ReceiptDisposition == "") || (r.ReceiptDisposition != "" && !validImplementationWorkReceiptDisposition(r.ReceiptDisposition)) {
		return fmt.Errorf("implementation work closed receipt requires exact typed disposition")
	}
	if r.CompletedReceiptSetHash != "" && r.DispatchID == "" {
		return fmt.Errorf("implementation work completion has no dispatch commitment")
	}
	if len(r.GovernanceDispatchIDs) == 0 && r.GovernanceDispatchIDs != nil {
		return fmt.Errorf("implementation governance history must preserve canonical absence")
	}
	if r.GovernanceReceiptHash != "" && (r.DispatchID == "" || !implementationWorkExactRef(r.GovernanceReceiptHash) || r.CompletedReceiptSetHash != "" || r.FailedReceiptHash != "") || len(r.GovernanceDispatchIDs) > MaxRuntimeImplementationWorkRounds {
		return fmt.Errorf("implementation governance suspension must retain one outstanding bounded round")
	}
	seenGovernanceDispatches := map[string]bool{}
	for _, id := range r.GovernanceDispatchIDs {
		if !implementationWorkExactRef(id) || id == r.DispatchID || seenGovernanceDispatches[id] {
			return fmt.Errorf("implementation governance dispatch history is invalid")
		}
		seenGovernanceDispatches[id] = true
	}
	if r.Hash == "" || r.Hash != StableRuntimeImplementationWorkRoundHash(r) {
		return fmt.Errorf("implementation work round hash mismatch")
	}
	return nil
}

func referenceWorkingBindingCatalogSession(b RuntimeImplementationWorkingBinding, r RuntimeImplementationWorkRound) error {
	for _, ref := range []string{b.SemanticGenerationID, b.SemanticManifestHash, b.LogicalRequestID} {
		if !implementationWorkExactRef(ref) {
			return fmt.Errorf("implementation working binding requires exact semantic request identity")
		}
	}
	if b.BindingGeneration == 0 || r.ScopeObservation == nil || b.SemanticGenerationID != StableRuntimeSemanticGenerationID(b.SemanticManifestHash, b.LogicalRequestID) {
		return fmt.Errorf("implementation working binding requires generation and complete scope observation")
	}
	selection := r.ScopeObservation.Selection
	if selection.Disposition != RuntimeTargetSelectionSelected && selection.Disposition != RuntimeTargetSelectionScope {
		return fmt.Errorf("implementation working binding cannot use unresolved/unsupported scope")
	}
	catalog := b.OwnerCatalog
	if err := catalog.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(catalog, catalog.Normalized()) || catalog.OwnerModuleID != r.ScopeRequest.Policy.OwnerModuleID || catalog.ObservedRevision != selection.ObservedRevision {
		return fmt.Errorf("implementation working binding requires exact current resource owner catalog")
	}
	if err := b.CatalogObservation.Validate(); err != nil {
		return err
	}
	observation := b.CatalogObservation
	if !reflect.DeepEqual(observation, observation.Normalized()) || observation.Kind != PrerequisiteKindOperationAdmissibility || observation.OwnerModuleID != catalog.OwnerModuleID ||
		observation.PrerequisiteID != catalog.PrerequisiteID || observation.ContentHash != catalog.SnapshotID ||
		!runtimeObservationDependsOnWorkspaceTreeRevision(observation, catalog.OwnerModuleID, selection.ObservedRevision) {
		return fmt.Errorf("implementation working binding catalog observation has no exact scope revision preimage")
	}
	if len(b.Proposals) == 0 || len(b.Proposals) > r.ScopeRequest.Policy.MaxTargetsPerRound || len(b.Proposals) != len(b.Feasibilities) {
		return fmt.Errorf("implementation working binding requires bounded complete proposal/feasibility set")
	}
	if err := ValidateRuntimeOperationAdmissibilities(b.MutationInputCatalogs); err != nil {
		return err
	}
	for _, inputCatalog := range b.MutationInputCatalogs {
		if !reflect.DeepEqual(inputCatalog, inputCatalog.Normalized()) {
			return fmt.Errorf("implementation work input protection must retain raw-canonical catalog")
		}
	}
	excluded := runtimeValidationInputMutationExclusions(catalog.OwnerModuleID, RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: b.MutationInputCatalogs}, RequiredOutputContract{}, nil)
	if !reflect.DeepEqual(excluded, b.ExcludedTargetRefs) {
		return fmt.Errorf("implementation work input exclusions do not match complete owner preimages")
	}
	seen := map[string]bool{}
	for index, proposal := range b.Proposals {
		if err := proposal.Validate(); err != nil {
			return err
		}
		if !reflect.DeepEqual(proposal, proposal.Normalized()) || proposal.CoverageWorkItemID != r.CoverageWorkItemID || proposal.OwnerModuleID != catalog.OwnerModuleID ||
			!slices.Equal(proposal.RequiredEvidenceKinds, []string{EvidenceKindStateChange}) || !slices.Equal(proposal.RequiredEffectScopes, []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}) ||
			proposal.ValidationRecipeID != "" || len(proposal.RequiredCheckRefs) != 0 || proposal.ValidationInputReview != nil {
			return fmt.Errorf("implementation work proposal cannot carry terminal validation/input-maintenance proof")
		}
		if !slices.Contains(r.ScopeRequest.Policy.AllowedOperations, RuntimeOperationVariantRef{Kind: proposal.OperationKind, VariantID: proposal.OperationVariantID}) ||
			!slices.Contains(r.ScopeRequest.Policy.AllowedTargetPolicies, proposal.Target.Policy) {
			return fmt.Errorf("implementation work proposal exceeds sealed operation/variant/policy declaration")
		}
		if err := validateSemanticProposalAgainstOwnerCatalog(proposal, catalog); err != nil {
			return err
		}
		targets := RuntimeSemanticProposalTargetRefs(proposal)
		// The first value contract requires exact, non-overlapping targets; an
		// owner selector is observation vocabulary, never effect authorization.
		if len(targets) == 0 || len(targets) > r.ScopeRequest.Policy.MaxTargetsPerRound {
			return fmt.Errorf("implementation work proposal requires bounded exact targets")
		}
		for _, target := range targets {
			if !implementationWorkExactRef(target) || seen[target] || slices.Contains(excluded, target) {
				return fmt.Errorf("implementation work target duplicates or mutates protected validation input")
			}
			seen[target] = true
			policy := r.ScopeRequest.Policy
			if len(policy.TargetSelectorCandidateRefs) != 0 && !slices.Contains(policy.TargetSelectorCandidateRefs, target) {
				return fmt.Errorf("implementation work target is outside declared work selector candidates")
			}
			if proposal.Target.Policy != RuntimeOperationTargetPolicyProspectiveOwnerTarget && !slices.Contains(selection.TargetRefs, target) {
				return fmt.Errorf("implementation work target is absent from complete owner work scope")
			}
		}
		// The existing hypothetical delta is specific to prospective OWNER
		// FACT satisfaction. Ordinary implementation work must not borrow that
		// closure or its second owner's judgment as work authorization.
		if b.Feasibilities[index].ProspectiveWorkspaceManifestDelta != nil {
			return fmt.Errorf("implementation work cannot borrow owner-fact prospective satisfaction delta")
		}
		if proposal.Target.Policy == RuntimeOperationTargetPolicyProspectiveOwnerTarget && !SameRuntimeProspectiveTargetAllOfConstraintRefs(proposal.Target.ProspectiveConstraintRefs, r.ScopeRequest.Policy.ProspectiveTargetAllOfConstraintRefs) {
			return fmt.Errorf("implementation work prospective constraints differ from sealed all-of declaration")
		}
		fact := b.Feasibilities[index]
		if !reflect.DeepEqual(fact, fact.Normalized()) || fact.SchemaVersion != RuntimeSemanticFeasibilitySchemaCurrent || fact.Status != RuntimeSemanticFeasibilityFeasible ||
			fact.ManifestHash != b.SemanticManifestHash || fact.DefinitionSeedHash != r.DefinitionSeedHash || fact.CoverageWorkItemID != r.CoverageWorkItemID ||
			fact.ProposalHash != StableRuntimeSemanticProofProposalHash(proposal) || fact.OwnerModuleID != catalog.OwnerModuleID || fact.CatalogSnapshotID != catalog.SnapshotID ||
			fact.CatalogObservationID != observation.ObservationID || fact.ObservedRevision != catalog.ObservedRevision || !slices.Equal(fact.TargetRefs, targets) ||
			fact.ValidationRecipeID != "" || len(fact.RequiredCheckRefs) != 0 || fact.OwnerFactProspectiveSatisfactionObservation != nil || fact.OriginalDiagnostic != nil {
			return fmt.Errorf("implementation work feasibility differs from exact proposal/catalog/source")
		}
	}
	if len(seen) > r.ScopeRequest.Policy.MaxTargetsPerRound {
		return fmt.Errorf("implementation work collective target set exceeds sealed round limit")
	}
	if b.Hash == "" || b.Hash != StableRuntimeImplementationWorkingBindingHash(b) {
		return fmt.Errorf("implementation working binding hash mismatch")
	}
	return nil
}
