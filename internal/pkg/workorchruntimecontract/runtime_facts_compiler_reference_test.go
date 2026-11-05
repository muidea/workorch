package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	"strings"
)

// Frozen pre-compilation algorithm: independent cold acceptance and first-error oracle.
func referenceRuntimeFactsBeforeOwnerCompilation(f RuntimeFacts) error {
	raw := f
	// Facts normalization must not repair a coordinated persisted rewrite of
	// activation evidence authority. Check this nested raw collection before
	// f.Normalized() can canonicalize nested authority fields.
	if err := ValidateRuntimeFactsRaw(raw); err != nil {
		return err
	}
	rawFocuses := make([]CoverageFocus, 0, 1+len(raw.RemainingFocuses)+len(raw.CompletedFocuses))
	rawFocuses = append(rawFocuses, raw.Focus)
	rawFocuses = append(rawFocuses, raw.RemainingFocuses...)
	rawFocuses = append(rawFocuses, raw.CompletedFocuses...)
	for _, focus := range rawFocuses {
		if len(focus.CoverageEvidenceBindings) == 0 {
			continue
		}
		if err := ValidateCoverageEvidenceBindingsForFocus(focus, raw.DefinitionRevision); err != nil {
			return fmt.Errorf("runtime facts focus %s: %w", strings.TrimSpace(focus.ID), err)
		}
	}
	for _, snapshot := range raw.Resources.CapabilityStates {
		if err := validateRequiredOperationVariants(snapshot.RequiredOperationSequence); err != nil {
			return fmt.Errorf("runtime capability-state snapshot %q operation sequence: %w", strings.TrimSpace(snapshot.SnapshotID), err)
		}
	}
	if err := referenceRuntimeFactsCatalogCollectionBeforeCompilation(raw.Resources.OperationAdmissibilities); err != nil {
		return fmt.Errorf("runtime operation admissibility: %w", err)
	}
	for _, fact := range append(append([]EvidenceFact(nil), raw.Evidence.Accepted...), raw.Evidence.Rejected...) {
		if fact.Validation != nil {
			if err := fact.Validation.ValidateFailureDetails(); err != nil {
				return fmt.Errorf("runtime evidence validation observation %q: %w", fact.ID, err)
			}
		}
	}
	f = f.Normalized()
	if f.WorkspaceID == "" || f.RunID == "" {
		return fmt.Errorf("runtime facts require workspace and run scope")
	}
	if f.DefinitionRevision == "" || f.EvidenceRevision == "" {
		return fmt.Errorf("runtime facts require definition and evidence revisions")
	}
	if raw.CWILedgerSchemaVersion != "" {
		if err := ValidateRuntimeCWILedger(raw); err != nil {
			return err
		}
		if err := ValidateRuntimeCWIFocusBindings(f); err != nil {
			return err
		}
	}
	if f.DefinitionSeed.SchemaVersion != "" {
		if err := f.DefinitionSeed.Validate(); err != nil {
			return fmt.Errorf("runtime facts definition seed: %w", err)
		}
		if err := ValidateRuntimeSourceObligationApplicability(f.DefinitionSeed.SourceObligations, f.Activation.ExecutionContract.ProfileContracts); err != nil {
			return fmt.Errorf("runtime facts source applicability: %w", err)
		}
		if err := ValidateRuntimeGoalExecutionAssertionSeed(f.DefinitionSeed, f.Activation.ExecutionContract.ProfileContracts); err != nil {
			return err
		}
		if f.DefinitionSeed.DefinitionRevision != f.DefinitionRevision {
			return fmt.Errorf("runtime facts definition seed revision mismatch")
		}
		if len(f.DefinitionFocuses) != 0 {
			if err := ValidateRuntimeDefinitionFocuses(f.DefinitionSeed, f.DefinitionFocuses); err != nil {
				return fmt.Errorf("runtime facts definition focuses: %w", err)
			}
			if err := ValidateRuntimeDefinitionImplementationWorkPolicies(f.DefinitionFocuses, f.Activation.ExecutionContract.ProfileContracts); err != nil {
				return fmt.Errorf("runtime facts definition work policies: %w", err)
			}
			if err := ValidateRuntimeDefinitionLineageAssertions(f.DefinitionFocuses, f.Activation.ExecutionContract.ProfileContracts); err != nil {
				return err
			}
			if err := ValidateRuntimeDefinitionVerificationPolicies(f.DefinitionFocuses, f.Activation.ExecutionContract.ProfileContracts); err != nil {
				return fmt.Errorf("runtime facts definition verification policies: %w", err)
			}
		}
	}
	if err := ValidateRuntimeTerminalProofDeclarationSources(f.DefinitionSeed.CoverageItems, f.Activation.ExecutionContract.ProfileContracts); err != nil {
		return err
	}
	if f.Focus.ID == "" {
		return fmt.Errorf("runtime facts require a coverage focus")
	}
	if err := ValidateRuntimeCapabilityPrerequisiteFacts(raw.Capabilities.PrerequisiteFacts); err != nil {
		return fmt.Errorf("runtime capability prerequisite facts: %w", err)
	}
	for _, focus := range append(append([]CoverageFocus{f.Focus}, f.RemainingFocuses...), f.CompletedFocuses...) {
		if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(focus.ProspectiveTargetAllOfConstraintRefs); err != nil {
			return fmt.Errorf("runtime focus %q prospective target all-of declaration: %w", focus.ID, err)
		}
		if len(focus.ProspectiveTargetAllOfConstraintRefs) != 0 &&
			(!focus.RequireTargetGrounding || focus.TargetSelectorRef == "" || focus.TargetSelectorOwnerRef == "" || focus.TargetSelectorSubjectRef == "") {
			return fmt.Errorf("runtime focus %q prospective target all-of declaration requires grounding and a complete selector", focus.ID)
		}
		if err := ValidateRuntimeOwnerFactRequirements(focus.RequiredOwnerFacts); err != nil {
			return fmt.Errorf("runtime focus %q owner facts: %w", focus.ID, err)
		}
		if err := ValidateRuntimeOwnerFactConvergences(focus.RequiredOwnerFacts, focus.OwnerFactConvergences); err != nil {
			return fmt.Errorf("runtime focus %q owner-fact convergence: %w", focus.ID, err)
		}
	}
	if err := f.Activation.Validate(); err != nil {
		return err
	}
	if err := f.ConvergencePolicy.Validate(); err != nil {
		return fmt.Errorf("runtime facts convergence policy: %w", err)
	}
	if err := f.RecoveryPolicy.Validate(); err != nil {
		return fmt.Errorf("runtime facts recovery policy: %w", err)
	}
	if err := f.ExecutionBudgetPolicy.Validate(); err != nil {
		return fmt.Errorf("runtime facts execution budget policy: %w", err)
	}
	if err := ValidateRuntimeExecutionBudgetPolicySources(f.ExecutionBudgetPolicy, f.ConvergencePolicy, f.RecoveryPolicy); err != nil {
		return fmt.Errorf("runtime facts execution budget policy: %w", err)
	}
	if err := ValidateRuntimeExecutionLimitsBinding(f.ExecutionBudgetPolicy, f.Activation.ExecutionContract.ExecutionLimits); err != nil {
		return fmt.Errorf("runtime facts execution budget limits: %w", err)
	}
	if err := f.Recovery.Validate(f.RecoveryPolicy); err != nil {
		return fmt.Errorf("runtime facts recovery ledger: %w", err)
	}
	if f.Resources.TargetSelection != nil {
		if err := f.Resources.TargetSelection.Validate(); err != nil {
			return fmt.Errorf("runtime facts target selection: %w", err)
		}
		selection := f.Resources.TargetSelection.Normalized()
		bound := false
		for _, prerequisite := range f.Prerequisites.Satisfied {
			if prerequisite.ID == selection.PrerequisiteID &&
				prerequisite.Kind == PrerequisiteKindTargetSelection &&
				prerequisite.Scope == RuntimePrerequisiteScopeFocus &&
				prerequisite.FocusID == f.Focus.ID &&
				prerequisite.OwnerModuleID == selection.OwnerModuleID {
				bound = true
				break
			}
		}
		if !bound {
			return fmt.Errorf("runtime facts target selection %q is not bound to an active current-focus prerequisite", selection.PrerequisiteID)
		}
	}
	if f.Failure != nil {
		if f.Failure.OriginalDiagnostic != nil {
			if err := f.Failure.OriginalDiagnostic.Validate(); err != nil {
				return fmt.Errorf("runtime facts failure original diagnostic: %w", err)
			}
		}
		if err := ValidateRuntimeFailureTargetObservationBinding(*f.Failure); err != nil {
			return fmt.Errorf("runtime facts failure target observation: %w", err)
		}
		if err := ValidateRuntimeFailureGovernanceDependency(*f.Failure); err != nil {
			return fmt.Errorf("runtime facts governance dependency: %w", err)
		}
		if f.Failure.ValidationFailureObservation != nil {
			if err := f.Failure.ValidationFailureObservation.Validate(); err != nil {
				return fmt.Errorf("runtime facts validation failure observation: %w", err)
			}
		}
	}
	if f.Resources.FailureTarget != nil && f.Resources.FailureTarget.TargetObservation != nil {
		bound := FailureFact{TargetRefs: f.Resources.FailureTarget.TargetRefs, TargetObservation: f.Resources.FailureTarget.TargetObservation}
		if err := ValidateRuntimeFailureTargetObservationBinding(bound); err != nil {
			return fmt.Errorf("runtime facts failure target resource observation: %w", err)
		}
	}
	for _, snapshot := range f.Resources.CapabilityStates {
		if snapshot.RequirementID == "" || snapshot.SnapshotID == "" || snapshot.AcquisitionKey == "" || snapshot.QueryReceiptRef == "" || snapshot.OwnerModuleID == "" || snapshot.CapabilityStateKind == "" || snapshot.SubjectRef == "" || snapshot.ObservedRevision == "" || snapshot.ContentHash == "" {
			return fmt.Errorf("runtime capability-state snapshot requires identity, owner, subject and revision")
		}
		switch snapshot.Disposition {
		case RuntimeCapabilityStateReady:
			if len(snapshot.RequiredOperationSequence) != 0 {
				return fmt.Errorf("ready capability-state snapshot cannot require operations")
			}
		case RuntimeCapabilityStateSequenceRequired:
			if len(snapshot.RequiredOperationSequence) == 0 {
				return fmt.Errorf("capability-state sequence disposition requires operations")
			}
		case RuntimeCapabilityStateGovernance, RuntimeCapabilityStateUnavailable, RuntimeCapabilityStateUnknown:
			if len(snapshot.RequiredOperationSequence) != 0 {
				return fmt.Errorf("non-ready capability-state snapshot cannot carry required operations")
			}
		default:
			return fmt.Errorf("unknown capability-state disposition %q", snapshot.Disposition)
		}
	}
	for _, admissibility := range f.Resources.OperationAdmissibilities {
		if err := admissibility.Validate(); err != nil {
			return fmt.Errorf("runtime operation admissibility: %w", err)
		}
	}
	for _, observation := range f.Resources.PrerequisiteObservations {
		if err := observation.Validate(); err != nil {
			return fmt.Errorf("runtime prerequisite observation: %w", err)
		}
		matched := false
		for _, prerequisite := range f.Prerequisites.Satisfied {
			if prerequisite.ID == observation.PrerequisiteID && prerequisite.Kind == observation.Kind && prerequisite.OwnerModuleID == observation.OwnerModuleID {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("runtime prerequisite observation %q has no matching satisfied prerequisite", observation.PrerequisiteID)
		}
	}
	activePrerequisites := append(append([]RuntimePrerequisite(nil), f.Prerequisites.Pending...), f.Prerequisites.Satisfied...)
	activeIDs := make(map[string]struct{}, len(activePrerequisites))
	for _, prerequisite := range activePrerequisites {
		if !RuntimePrerequisiteKindSupported(prerequisite.Kind) {
			return fmt.Errorf("runtime prerequisite %q has unsupported kind %q", prerequisite.ID, prerequisite.Kind)
		}
		if err := validateRuntimePrerequisiteProspectiveTargetAllOfDeclaration(prerequisite); err != nil {
			return err
		}
		if prerequisite.Scope == RuntimePrerequisiteScopeFocus && prerequisite.FocusID != f.Focus.ID {
			return fmt.Errorf("runtime active focus-scoped prerequisite %q is bound to focus %q instead of current focus %q", prerequisite.ID, prerequisite.FocusID, f.Focus.ID)
		}
		activeIDs[prerequisite.ID] = struct{}{}
	}
	retiredIDs := make(map[string]RuntimePrerequisite, len(f.Prerequisites.Retired))
	for _, prerequisite := range f.Prerequisites.Retired {
		if !RuntimePrerequisiteKindSupported(prerequisite.Kind) {
			return fmt.Errorf("runtime retired prerequisite %q has unsupported kind %q", prerequisite.ID, prerequisite.Kind)
		}
		if err := validateRuntimePrerequisiteProspectiveTargetAllOfDeclaration(prerequisite); err != nil {
			return err
		}
		if _, active := activeIDs[prerequisite.ID]; active {
			return fmt.Errorf("runtime prerequisite %q cannot be both active and retired", prerequisite.ID)
		}
		if prerequisite.Scope != RuntimePrerequisiteScopeFocus || prerequisite.FocusID == "" {
			return fmt.Errorf("runtime retired prerequisite %q must be focus-scoped", prerequisite.ID)
		}
		retiredIDs[prerequisite.ID] = prerequisite
	}
	for _, observation := range f.Resources.RetiredPrerequisiteObservations {
		if err := observation.Validate(); err != nil {
			return fmt.Errorf("runtime retired prerequisite observation: %w", err)
		}
		prerequisite, found := retiredIDs[observation.PrerequisiteID]
		if !found || prerequisite.Kind != observation.Kind || prerequisite.OwnerModuleID != observation.OwnerModuleID {
			return fmt.Errorf("runtime retired prerequisite observation %q has no matching retired prerequisite", observation.PrerequisiteID)
		}
	}
	seenFocus := map[string]struct{}{f.Focus.ID: {}}
	for _, focus := range append(append([]CoverageFocus(nil), f.RemainingFocuses...), f.CompletedFocuses...) {
		if focus.ID == "" {
			return fmt.Errorf("runtime facts queued focus requires identity")
		}
		if _, ok := seenFocus[focus.ID]; ok {
			return fmt.Errorf("runtime facts coverage focus %s is duplicated", focus.ID)
		}
		seenFocus[focus.ID] = struct{}{}
	}
	coverage := map[common.CoverageWorkItemID]struct{}{}
	for _, item := range f.Coverage {
		coverage[item.ID] = struct{}{}
	}
	for _, focus := range append(append([]CoverageFocus{f.Focus}, f.RemainingFocuses...), f.CompletedFocuses...) {
		for _, id := range focus.CoverageWorkItemIDs {
			if _, ok := coverage[id]; !ok {
				return fmt.Errorf("runtime facts focus coverage item %s has no coverage ledger entry", id)
			}
		}
		if len(focus.CoverageEvidenceBindings) != 0 {
			if err := ValidateCoverageEvidenceBindingsForFocus(focus, f.DefinitionRevision); err != nil {
				return fmt.Errorf("runtime facts focus %s: %w", focus.ID, err)
			}
		}
	}
	if f.ReconciliationContract.SchemaVersion != "" {
		if err := f.ReconciliationContract.Validate(); err != nil {
			return fmt.Errorf("runtime facts reconciliation contract: %w", err)
		}
	}
	coverageItems := make([]RuntimeCoverageItem, 0, len(f.Coverage))
	for _, item := range f.Coverage {
		coverageItems = append(coverageItems, RuntimeCoverageItem{
			ID: item.ID, SliceID: item.SliceID, SourceScope: item.SourceScope, SourceID: item.SourceID,
			TargetText: item.TargetText, PrerequisiteCoverageWorkItemIDs: item.PrerequisiteCoverageWorkItemIDs,
			ReopenDependencies: item.ReopenDependencies,
		})
	}
	if err := ValidateRuntimeCoverageItemDependencies(coverageItems); err != nil {
		return fmt.Errorf("runtime facts coverage dependencies: %w", err)
	}
	if f.RepairDirective != nil {
		if err := f.RepairDirective.Validate(); err != nil {
			return fmt.Errorf("runtime facts repair directive: %w", err)
		}
		if f.RepairDirective.FocusID != f.Focus.ID {
			return fmt.Errorf("runtime facts repair directive focus %s does not match current focus %s", f.RepairDirective.FocusID, f.Focus.ID)
		}
	}
	for _, assessment := range f.Assessments {
		if assessment.ID == "" || assessment.Revision == "" || assessment.FocusID == "" {
			return fmt.Errorf("runtime assessment fact requires identity, revision and focus")
		}
		if err := assessment.Disposition.Validate(); err != nil {
			return fmt.Errorf("runtime assessment fact %s canonical disposition: %w", assessment.ID, err)
		}
		if assessment.ClaimedDisposition != "" {
			if err := assessment.ClaimedDisposition.Validate(); err != nil {
				return fmt.Errorf("runtime assessment fact %s claimed disposition: %w", assessment.ID, err)
			}
		}
	}
	return nil
}

func referenceRuntimeFactsCatalogCollectionBeforeCompilation(values []RuntimeOperationAdmissibility) error {
	seenPrerequisites := make(map[string]struct{}, len(values))
	for _, raw := range values {
		if err := raw.Validate(); err != nil {
			return err
		}
		// The full catalog gate above already validated every raw member.
		// This collection check reads only Normalized's scalar prerequisite ID,
		// not another copy of all recipes, proof bindings and input bytes.
		prerequisiteID := strings.TrimSpace(raw.PrerequisiteID)
		if _, duplicate := seenPrerequisites[prerequisiteID]; duplicate {
			return fmt.Errorf("runtime operation admissibility duplicates prerequisite %q", prerequisiteID)
		}
		seenPrerequisites[prerequisiteID] = struct{}{}
	}
	return nil
}
