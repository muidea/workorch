package runtime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/grounding"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type PrerequisiteResult struct {
	PrerequisiteID         string
	Manifest               *runtimecontract.WorkspaceManifest
	TargetSnapshot         *runtimecontract.TargetSnapshot
	TargetSelection        *runtimecontract.RuntimeTargetSelection
	Capabilities           *runtimecontract.RuntimeCapabilities
	ValidationScratch      *runtimecontract.ValidationScratch
	ProviderCeiling        *runtimecontract.ProviderCeiling
	CapabilityState        *runtimecontract.RuntimeCapabilityStateSnapshot
	OperationAdmissibility *runtimecontract.RuntimeOperationAdmissibility
	Observation            *runtimecontract.RuntimePrerequisiteObservation
}

func PreflightRuntimeFacts(facts runtimecontract.RuntimeFacts, requirements []runtimecontract.RuntimeRequirement) runtimecontract.RuntimeFacts {
	facts = facts.Normalized()
	normalizedRequirements := normalizeRuntimeRequirements(requirements)
	normalizedRequirements = runtimeRequirementsWithCapabilityPrerequisiteFacts(facts, normalizedRequirements)
	pending := make([]runtimecontract.RuntimePrerequisite, 0, len(normalizedRequirements))
	reacquiring := make(map[string]struct{})
	for _, requirement := range normalizedRequirements {
		if runtimeRequirementSatisfied(facts, requirement) {
			continue
		}
		prerequisiteID := runtimePrerequisiteID(requirement)
		reacquiring[prerequisiteID] = struct{}{}
		pending = append(pending, runtimecontract.RuntimePrerequisite{
			ID:                                   prerequisiteID,
			Kind:                                 requirement.Kind,
			Scope:                                requirement.Scope,
			FocusID:                              requirement.FocusID,
			OwnerModuleID:                        requirement.OwnerModuleID,
			SubjectRef:                           requirement.SubjectRef,
			TargetSelectorRef:                    requirement.TargetSelectorRef,
			TargetSelectorOwnerRef:               requirement.TargetSelectorOwnerRef,
			TargetSelectorSubjectRef:             requirement.TargetSelectorSubjectRef,
			TargetSelectorCandidateRefs:          requirement.TargetSelectorCandidateRefs,
			ProspectiveTargetAllOfConstraintRefs: requirement.ProspectiveTargetAllOfConstraintRefs,
			ProspectiveTargetRef:                 requirement.ProspectiveTargetRef,
			CapabilityClass:                      requirement.CapabilityClass,
			RequiredCapabilities:                 requirement.RequiredCapabilities,
			CapabilityStateKind:                  requirement.CapabilityStateKind,
			MinimumBytes:                         requirement.MinimumBytes,
			Reason:                               requirement.Reason,
		})
	}
	if len(reacquiring) != 0 {
		// A resource may have been invalidated or cleared while its old
		// prerequisite identity is still persisted (for example, authority
		// reconciliation after a delayed Focus result). A prerequisite cannot
		// assert both "satisfied" and "must be acquired" in the same SM-2
		// authority. Remove the stale satisfaction and its active observation
		// before scheduling the replacement.
		satisfied := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Satisfied))
		for _, prerequisite := range facts.Prerequisites.Satisfied {
			if _, stale := reacquiring[prerequisite.ID]; stale {
				continue
			}
			satisfied = append(satisfied, prerequisite)
		}
		facts.Prerequisites.Satisfied = satisfied
		observations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(facts.Resources.PrerequisiteObservations))
		for _, observation := range facts.Resources.PrerequisiteObservations {
			if _, stale := reacquiring[observation.PrerequisiteID]; stale {
				continue
			}
			observations = append(observations, observation)
		}
		facts.Resources.PrerequisiteObservations = observations
		facts = reactivateReopenedPendingFocusPrerequisites(facts, reacquiring)
	}
	facts.Prerequisites.Pending = pending
	return facts.Normalized()
}

// preflightRuntimeFactsPreservingPending is the only additive SM-2
// prerequisite compiler. PreflightRuntimeFacts intentionally treats its input
// as the complete desired set and replaces Pending; local freshness/grounding
// callers must therefore preserve the current pending graph before adding one
// requirement family. Satisfied prerequisites remain committed authority and
// must not be recompiled: doing so can manufacture a second canonical ID for
// an already accepted owner fact. This prevents a target-snapshot refresh from
// silently dropping unrelated manifest, capability, admissibility, or provider
// debt without reopening settled acquisitions.
func preflightRuntimeFactsPreservingPending(facts runtimecontract.RuntimeFacts, additions []runtimecontract.RuntimeRequirement) runtimecontract.RuntimeFacts {
	requirements := runtimePrerequisiteRequirements(facts.Prerequisites.Pending)
	requirements = append(requirements, additions...)
	return PreflightRuntimeFacts(facts, requirements)
}

// reactivateReopenedPendingFocusPrerequisites handles the one bounded case in
// which a retired prerequisite identity can become current again. A reopened
// CWI deliberately returns to the stable semantic-pending/<cwi> focus identity;
// therefore a focus-scoped prerequisite acquired by an earlier semantic
// generation can have the exact ID needed by the new generation. SM-2 removes
// that ID and its old owner observation from the retired current collection
// before scheduling acquisition. Immutable audit remains in the CWI reopen and
// semantic-generation histories; the old observation must not become authority
// for the reopened generation.
func reactivateReopenedPendingFocusPrerequisites(facts runtimecontract.RuntimeFacts, reacquiring map[string]struct{}) runtimecontract.RuntimeFacts {
	if len(reacquiring) == 0 || len(facts.Focus.CoverageWorkItemIDs) != 1 ||
		facts.Focus.ID != "semantic-pending/"+string(facts.Focus.CoverageWorkItemIDs[0]) {
		return facts
	}
	coverageID := facts.Focus.CoverageWorkItemIDs[0]
	reopened := false
	for _, item := range facts.Coverage {
		if item.ID == coverageID && item.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateReopened && len(item.ReopenLineage) != 0 {
			reopened = true
			break
		}
	}
	if !reopened {
		return facts
	}

	reactivated := make(map[string]struct{})
	retired := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Retired))
	for _, prerequisite := range facts.Prerequisites.Retired {
		_, currentRequirement := reacquiring[prerequisite.ID]
		if currentRequirement && prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus && prerequisite.FocusID == facts.Focus.ID &&
			prerequisite.ID == runtimePrerequisiteCanonicalID(prerequisite) {
			reactivated[prerequisite.ID] = struct{}{}
			continue
		}
		retired = append(retired, prerequisite)
	}
	if len(reactivated) == 0 {
		return facts
	}
	facts.Prerequisites.Retired = retired
	retiredObservations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(facts.Resources.RetiredPrerequisiteObservations))
	for _, observation := range facts.Resources.RetiredPrerequisiteObservations {
		if _, currentRequirement := reactivated[observation.PrerequisiteID]; currentRequirement {
			continue
		}
		retiredObservations = append(retiredObservations, observation)
	}
	facts.Resources.RetiredPrerequisiteObservations = retiredObservations
	return facts
}

func runtimePrerequisiteCanonicalID(prerequisite runtimecontract.RuntimePrerequisite) string {
	return runtimePrerequisiteID(runtimecontract.RuntimeRequirement{
		Kind: prerequisite.Kind, Scope: prerequisite.Scope, FocusID: prerequisite.FocusID,
		OwnerModuleID: prerequisite.OwnerModuleID, SubjectRef: prerequisite.SubjectRef,
		TargetSelectorRef: prerequisite.TargetSelectorRef, TargetSelectorOwnerRef: prerequisite.TargetSelectorOwnerRef,
		TargetSelectorSubjectRef: prerequisite.TargetSelectorSubjectRef, TargetSelectorCandidateRefs: prerequisite.TargetSelectorCandidateRefs,
		ProspectiveTargetAllOfConstraintRefs: prerequisite.ProspectiveTargetAllOfConstraintRefs,
		ProspectiveTargetRef:                 prerequisite.ProspectiveTargetRef, CapabilityClass: prerequisite.CapabilityClass,
		RequiredCapabilities: prerequisite.RequiredCapabilities, CapabilityStateKind: prerequisite.CapabilityStateKind,
		MinimumBytes: prerequisite.MinimumBytes, Reason: prerequisite.Reason,
	})
}

// runtimeRequirementsWithCapabilityPrerequisiteFacts compiles immutable
// resource dependencies published by capability owners. Runtime core never
// branches on an owner or operation kind: a fact applies only while its
// operation is part of the current requirement set.
func runtimeRequirementsWithCapabilityPrerequisiteFacts(facts runtimecontract.RuntimeFacts, requirements []runtimecontract.RuntimeRequirement) []runtimecontract.RuntimeRequirement {
	requiredKinds := make(map[string]struct{})
	for _, requirement := range requirements {
		// Capability acquisition proves that an owner supports a kind; it does
		// not yet consume the operation resource. Owner-published resource facts
		// become active when SM-2 requests that kind's admissibility inventory.
		if requirement.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility {
			continue
		}
		for _, kind := range requirement.RequiredCapabilities {
			if kind = runtimecontract.CanonicalRuntimeBlockKind(kind); kind != "" {
				requiredKinds[kind] = struct{}{}
			}
		}
	}
	ret := append([]runtimecontract.RuntimeRequirement(nil), requirements...)
	for _, fact := range runtimecontract.NormalizeRuntimeCapabilityPrerequisiteFacts(facts.Capabilities.PrerequisiteFacts) {
		if _, applies := requiredKinds[fact.OperationKind]; applies {
			ret = append(ret, fact.Requirement)
		}
	}
	return normalizeRuntimeRequirements(ret)
}

// runtimeTargetGroundingRequirements translates a sealed focus declaration
// into owner acquisitions. It never chooses a target or resource owner from
// workspace content. Existing refs require snapshots from the unique owner
// already sealed by a prerequisite/observation, target selection, semantic
// operation descriptor, or current capability contract. A binding-sealed
// prospective target already has owner feasibility authority and must not be
// read before creation. Discovery source IDs only assert the owner facts that
// the profile declared as a bounded basis.
func runtimeTargetGroundingRequirements(facts runtimecontract.RuntimeFacts, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) []runtimecontract.RuntimeRequirement {
	facts = facts.Normalized()
	focus, required := runtimeCurrentTargetGroundingFocus(facts)
	if !required {
		return nil
	}
	prospectiveAcquisitionConsumed := prospectiveOwnerFactAcquisitionReceiptCommitted(
		facts,
		runtimeCoverageProofLineageArgument(proofLineages),
	)
	requirements := make([]runtimecontract.RuntimeRequirement, 0, len(focus.TargetRefs)+len(focus.TargetDiscoverySources)+1)
	// Use the same owner-private target classification as SM-2 grounding and
	// SM-3 candidate admission. Only targets classified as currently existing
	// may produce a snapshot prerequisite.
	targetAuthority := runtimeTargetAuthorityPlanForFacts(facts)
	groundingOwner := runtimeTargetGroundingOwner(facts, focus)
	snapshotRequirements := make([]runtimecontract.RuntimeRequirement, 0, len(targetAuthority.existingTargets()))
	for _, target := range targetAuthority.existingTargets() {
		owner := runtimeTargetSnapshotRequirementOwner(facts, focus, target)
		if owner == "" {
			// An incomplete or ambiguous owner lineage cannot be repaired by
			// choosing a conventional capability owner. Keep the whole target
			// set out of the acquisition graph so the authority boundary fails
			// closed instead of scheduling a partial all-of snapshot set.
			return nil
		}
		snapshotRequirements = append(snapshotRequirements, focusScopedRuntimeRequirement(facts, runtimecontract.RuntimeRequirement{
			Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: owner,
			SubjectRef: target, Reason: "grounded repair target requires owner snapshot",
		}))
	}
	requirements = append(requirements, snapshotRequirements...)
	// A failure target is already an owner-selected repair surface. Otherwise a
	// resource owner may only make the neutral cardinality observation below;
	// it must not infer an implementation role from paths, suffixes, or content.
	if len(targetAuthority.targetRefs) == 0 && !targetAuthority.hasProspectiveTargets() {
		selection := currentFocusTargetSelection(facts)
		if selection == nil {
			if prospectiveAcquisitionConsumed {
				// Re-querying the create-only selector after its accepted
				// acquisition receipt can only observe the now-existing target as
				// unsupported. Keep compiling the ordinary manifest/catalog
				// freshness requirements below so the required owner is reobserved.
				goto selectionRequirementDone
			}
			// A prospective selector is itself a sealed target
			// acquisition requirement.  Semantic planning may provide the
			// selector/prospective declaration without a discovery-source hint;
			// that must not let SM-2 skip the owner observation and reach the
			// operations provider with an ungrounded target. This branch only
			// schedules an owner already established by typed Runtime facts; the
			// declaration selector owner is deliberately not a module identity.
			if groundingOwner != "" && focus.TargetSelectorRef != "" && len(focus.ProspectiveTargetRefs) == 1 {
				requirements = append(requirements, focusScopedRuntimeRequirement(facts, runtimecontract.RuntimeRequirement{
					Kind: runtimecontract.PrerequisiteKindTargetSelection, OwnerModuleID: groundingOwner,
					SubjectRef: focus.TargetRole, TargetSelectorRef: focus.TargetSelectorRef,
					TargetSelectorOwnerRef: focus.TargetSelectorOwnerRef, TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef,
					TargetSelectorCandidateRefs: focus.TargetSelectorCandidateRefs, ProspectiveTargetAllOfConstraintRefs: focus.ProspectiveTargetAllOfConstraintRefs, ProspectiveTargetRef: focus.ProspectiveTargetRefs[0],
					Reason: "prospective target requires owner target selection",
				}))
				goto selectionRequirementDone
			}
			for _, source := range focus.TargetDiscoverySources {
				switch source {
				case runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest, runtimecontract.RuntimeValidationDiscoverySourceProjectFiles:
					if groundingOwner == "" {
						return nil
					}
					requirements = append(requirements, focusScopedRuntimeRequirement(facts, runtimecontract.RuntimeRequirement{
						Kind: runtimecontract.PrerequisiteKindTargetSelection, OwnerModuleID: groundingOwner,
						SubjectRef: focus.TargetRole, TargetSelectorRef: focus.TargetSelectorRef, TargetSelectorOwnerRef: focus.TargetSelectorOwnerRef, TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef, TargetSelectorCandidateRefs: focus.TargetSelectorCandidateRefs, ProspectiveTargetAllOfConstraintRefs: focus.ProspectiveTargetAllOfConstraintRefs, Reason: "focus target grounding requires owner target selection",
					}))
					// One owner observation covers both resource-backed source IDs.
					goto selectionRequirementDone
				}
			}
		} else if targetSelectionHasConcreteTarget(*selection) {
			selectionOwner := strings.TrimSpace(selection.OwnerModuleID)
			if selectionOwner == "" {
				return nil
			}
			selectedSnapshots := make([]runtimecontract.RuntimeRequirement, 0, len(selection.TargetRefs))
			for _, target := range selection.TargetRefs {
				selectedSnapshots = append(selectedSnapshots, focusScopedRuntimeRequirement(facts, runtimecontract.RuntimeRequirement{
					Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: selectionOwner,
					SubjectRef: target, Reason: "selected focus target requires owner snapshot",
				}))
			}
			requirements = append(requirements, selectedSnapshots...)
		}
	}
selectionRequirementDone:
	for _, source := range focus.TargetDiscoverySources {
		switch source {
		case runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest, runtimecontract.RuntimeValidationDiscoverySourceProjectFiles:
			manifestRequirements := runtimeActiveRequirementsForKind(facts, runtimecontract.PrerequisiteKindManifest)
			if owner, ok := runtimeUniqueRequirementOwner(manifestRequirements); ok {
				requirements = append(requirements, manifestRequirements...)
				groundingOwner = owner
			} else if len(manifestRequirements) != 0 || groundingOwner == "" {
				return nil
			} else {
				requirements = append(requirements, runtimecontract.RuntimeRequirement{
					Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: groundingOwner,
					Reason: "focus target grounding requires workspace owner discovery",
				})
			}
		case runtimecontract.RuntimeValidationDiscoverySourceDeclaredCommands:
			requirements = append(requirements, runtimecontract.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
				Reason: "focus target grounding requires declared command discovery",
			})
		case runtimecontract.RuntimeValidationDiscoverySourceAvailableCapabilities:
			// Capability and admissibility requirements are activation-sealed for
			// every executable operation. The core must not invent an owner here.
		}
	}
	return normalizeRuntimeRequirements(requirements)
}

// runtimeTargetGroundingOwner returns one already-declared resource owner. It
// never treats TargetSelectorOwnerRef as a component ID: that field belongs to
// Profile/Definition declaration lineage. Conflicting typed sources are an
// authority defect and therefore return no owner.
func runtimeTargetGroundingOwner(facts runtimecontract.RuntimeFacts, focus runtimecontract.CoverageFocus) string {
	if selection := currentFocusTargetSelection(facts); selection != nil {
		return strings.TrimSpace(selection.OwnerModuleID)
	}
	selectionRequirements := runtimeMatchingTargetSelectionRequirements(facts, focus)
	if len(selectionRequirements) != 0 {
		owner, ok := runtimeUniqueRequirementOwner(selectionRequirements)
		if !ok {
			return ""
		}
		return owner
	}
	manifestRequirements := runtimeActiveRequirementsForKind(facts, runtimecontract.PrerequisiteKindManifest)
	if len(manifestRequirements) != 0 {
		owner, ok := runtimeUniqueRequirementOwner(manifestRequirements)
		if !ok {
			return ""
		}
		return owner
	}
	if owner, found := runtimeStateChangeDescriptorOwner(runtimeFocusBindingStateChangeKinds(focus)); found {
		return owner
	}
	if owner, found := runtimeStateChangeDescriptorOwner(focus.AcceptedStepKinds); found {
		return owner
	}
	owner, _ := runtimeStateChangeDescriptorOwner(facts.Capabilities.StateChangeKinds)
	return owner
}

func runtimeTargetSnapshotRequirementOwner(facts runtimecontract.RuntimeFacts, focus runtimecontract.CoverageFocus, targetRef string) string {
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return ""
	}
	if owner := runtimeTargetSnapshotOwner(facts, targetRef); owner != "" {
		return owner
	}
	if selection := currentFocusTargetSelection(facts); selection != nil &&
		(runtimeStringIncluded(selection.TargetRefs, targetRef) || runtimeStringIncluded(selection.CandidateTargetRefs, targetRef)) {
		return strings.TrimSpace(selection.OwnerModuleID)
	}
	if failureTarget := facts.Resources.FailureTarget; failureTarget != nil && failureTarget.TargetObservation != nil &&
		runtimeStringIncluded(failureTarget.TargetRefs, targetRef) {
		observation := failureTarget.TargetObservation.Normalized()
		// A receipt-evidence observation names the diagnostic owner, not the
		// repair resource owner. Its resource owner must instead come from an
		// already observed snapshot or another sealed target contract below.
		if observation.CatalogDependency == nil {
			return observation.OwnerModuleID
		}
	}
	return runtimeTargetGroundingOwner(facts, focus)
}

func runtimeMatchingTargetSelectionRequirements(facts runtimecontract.RuntimeFacts, focus runtimecontract.CoverageFocus) []runtimecontract.RuntimeRequirement {
	prospectiveTargetRef := ""
	if len(focus.ProspectiveTargetRefs) == 1 {
		prospectiveTargetRef = focus.ProspectiveTargetRefs[0]
	} else if len(focus.ProspectiveTargetRefs) > 1 {
		return nil
	}
	ret := make([]runtimecontract.RuntimeRequirement, 0, 1)
	for _, requirement := range runtimeActiveRequirementsForKind(facts, runtimecontract.PrerequisiteKindTargetSelection) {
		if requirement.Scope != runtimecontract.RuntimePrerequisiteScopeFocus || requirement.FocusID != facts.Focus.ID ||
			requirement.SubjectRef != focus.TargetRole || requirement.TargetSelectorRef != focus.TargetSelectorRef ||
			requirement.TargetSelectorOwnerRef != focus.TargetSelectorOwnerRef || requirement.TargetSelectorSubjectRef != focus.TargetSelectorSubjectRef ||
			!runtimecontract.SameRuntimeTargetSelectorCandidateRefs(requirement.TargetSelectorCandidateRefs, focus.TargetSelectorCandidateRefs) ||
			!runtimecontract.SameRuntimeProspectiveTargetAllOfConstraintRefs(requirement.ProspectiveTargetAllOfConstraintRefs, focus.ProspectiveTargetAllOfConstraintRefs) ||
			requirement.ProspectiveTargetRef != prospectiveTargetRef {
			continue
		}
		ret = append(ret, requirement)
	}
	return normalizeRuntimeRequirements(ret)
}

func runtimeActiveRequirementsForKind(facts runtimecontract.RuntimeFacts, kind runtimecontract.PrerequisiteKind) []runtimecontract.RuntimeRequirement {
	ret := make([]runtimecontract.RuntimeRequirement, 0)
	for _, requirement := range runtimeActivePrerequisiteRequirements(facts) {
		if requirement.Kind == kind {
			ret = append(ret, requirement)
		}
	}
	return normalizeRuntimeRequirements(ret)
}

func runtimeUniqueRequirementOwner(requirements []runtimecontract.RuntimeRequirement) (string, bool) {
	owner := ""
	for _, requirement := range requirements {
		candidate := strings.TrimSpace(requirement.OwnerModuleID)
		if candidate == "" || owner != "" && owner != candidate {
			return "", false
		}
		owner = candidate
	}
	return owner, owner != ""
}

func runtimeFocusBindingStateChangeKinds(focus runtimecontract.CoverageFocus) []string {
	ret := make([]string, 0)
	for _, binding := range focus.CoverageEvidenceBindings {
		for _, proof := range binding.Proofs {
			if proof.EvidenceKind == runtimecontract.EvidenceKindStateChange {
				ret = append(ret, proof.AllowedOperationKinds...)
			}
		}
	}
	return normalizeRequirementStrings(ret)
}

// runtimeStateChangeDescriptorOwner resolves only a unique owner from the
// shared typed operation descriptors. Unknown state-change kinds or mixed
// owners are ambiguous and deliberately yield no owner.
func runtimeStateChangeDescriptorOwner(kinds []string) (string, bool) {
	owner := ""
	found := false
	for _, kind := range normalizeRequirementStrings(kinds) {
		descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(kind)
		if !known {
			return "", true
		}
		allowsStateChange := false
		for _, state := range descriptor.AllowedStates {
			if state == runtimecontract.ConvergenceStateNeedStateChange {
				allowsStateChange = true
				break
			}
		}
		if !allowsStateChange {
			continue
		}
		candidate := strings.TrimSpace(descriptor.OwnerModuleID)
		if candidate == "" || owner != "" && owner != candidate {
			return "", true
		}
		owner = candidate
		found = true
	}
	return owner, found
}

// currentFocusTargetSelection returns only an owner selection committed as a
// satisfied Focus-scoped prerequisite for the current Focus. A raw resource
// pointer is not authority by itself: it can be historical audit lineage or a
// malformed aggregate and must never skip a current acquisition.
func currentFocusTargetSelection(facts runtimecontract.RuntimeFacts) *runtimecontract.RuntimeTargetSelection {
	return grounding.CurrentFocusTargetSelection(facts)
}

func focusScopedRuntimeRequirement(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) runtimecontract.RuntimeRequirement {
	requirement.Scope = runtimecontract.RuntimePrerequisiteScopeFocus
	requirement.FocusID = facts.Focus.ID
	return requirement
}

func CommitPrerequisiteResult(facts runtimecontract.RuntimeFacts, result PrerequisiteResult) (runtimecontract.RuntimeFacts, error) {
	facts = facts.Normalized()
	result.PrerequisiteID = strings.TrimSpace(result.PrerequisiteID)
	if result.PrerequisiteID == "" {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("prerequisite result requires an id")
	}
	var completed *runtimecontract.RuntimePrerequisite
	for idx := range facts.Prerequisites.Pending {
		if facts.Prerequisites.Pending[idx].ID == result.PrerequisiteID {
			item := facts.Prerequisites.Pending[idx]
			completed = &item
			break
		}
	}
	if completed == nil {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("unknown or already committed prerequisite %q", result.PrerequisiteID)
	}
	if err := applyPrerequisiteResult(&facts, *completed, result); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	if result.Observation != nil {
		observation := result.Observation.Normalized()
		if observation.PrerequisiteID != completed.ID || observation.Kind != completed.Kind ||
			(completed.OwnerModuleID != "" && observation.OwnerModuleID != completed.OwnerModuleID) {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("prerequisite observation does not match completed prerequisite %q", completed.ID)
		}
		if err := observation.Validate(); err != nil {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("invalid prerequisite observation: %w", err)
		}
		facts.Resources.PrerequisiteObservations = append(facts.Resources.PrerequisiteObservations, observation)
	}
	next := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Pending)-1)
	for _, item := range facts.Prerequisites.Pending {
		if item.ID != result.PrerequisiteID {
			next = append(next, item)
		}
	}
	facts.Prerequisites.Pending = next
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, *completed)
	facts = facts.Normalized()
	if completed.Kind == runtimecontract.PrerequisiteKindCapability && result.Capabilities != nil && len(result.Capabilities.PrerequisiteFacts) != 0 {
		// Capability owners may reveal typed resource dependencies only when
		// their sealed catalog is acquired. Recompile the still-active
		// prerequisite graph immediately so a dependent owner inventory cannot
		// run ahead of the newly published fact in the same SM-2 drive.
		facts = preflightRuntimeFactsPreservingPending(facts, nil)
	}
	return facts, nil
}

func runtimeActivePrerequisiteRequirements(facts runtimecontract.RuntimeFacts) []runtimecontract.RuntimeRequirement {
	values := append(append([]runtimecontract.RuntimePrerequisite(nil), facts.Prerequisites.Pending...), facts.Prerequisites.Satisfied...)
	return runtimePrerequisiteRequirements(values)
}

func runtimePrerequisiteRequirements(values []runtimecontract.RuntimePrerequisite) []runtimecontract.RuntimeRequirement {
	requirements := make([]runtimecontract.RuntimeRequirement, 0, len(values))
	for _, prerequisite := range values {
		requirements = append(requirements, runtimecontract.RuntimeRequirement{
			Kind: prerequisite.Kind, Scope: prerequisite.Scope, FocusID: prerequisite.FocusID,
			OwnerModuleID: prerequisite.OwnerModuleID, SubjectRef: prerequisite.SubjectRef,
			TargetSelectorRef: prerequisite.TargetSelectorRef, TargetSelectorOwnerRef: prerequisite.TargetSelectorOwnerRef,
			TargetSelectorSubjectRef: prerequisite.TargetSelectorSubjectRef, TargetSelectorCandidateRefs: prerequisite.TargetSelectorCandidateRefs,
			ProspectiveTargetAllOfConstraintRefs: prerequisite.ProspectiveTargetAllOfConstraintRefs,
			ProspectiveTargetRef:                 prerequisite.ProspectiveTargetRef, CapabilityClass: prerequisite.CapabilityClass,
			RequiredCapabilities: prerequisite.RequiredCapabilities, CapabilityStateKind: prerequisite.CapabilityStateKind,
			MinimumBytes: prerequisite.MinimumBytes, Reason: prerequisite.Reason,
		})
	}
	return normalizeRuntimeRequirements(requirements)
}

func applyPrerequisiteResult(facts *runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite, result PrerequisiteResult) error {
	switch prerequisite.Kind {
	case runtimecontract.PrerequisiteKindManifest:
		return applyManifestPrerequisiteResult(facts, result)
	case runtimecontract.PrerequisiteKindTargetSnapshot:
		return applyTargetSnapshotPrerequisiteResult(facts, prerequisite, result)
	case runtimecontract.PrerequisiteKindTargetSelection:
		return applyTargetSelectionPrerequisiteResult(facts, prerequisite, result)
	case runtimecontract.PrerequisiteKindCapability:
		return applyCapabilityPrerequisiteResult(facts, prerequisite, result)
	case runtimecontract.PrerequisiteKindCapabilityState:
		return applyCapabilityStatePrerequisiteResult(facts, prerequisite, result)
	case runtimecontract.PrerequisiteKindValidationScratch:
		return applyValidationScratchPrerequisiteResult(facts, result)
	case runtimecontract.PrerequisiteKindProviderCeiling:
		return applyProviderCeilingPrerequisiteResult(facts, prerequisite, result)
	case runtimecontract.PrerequisiteKindOperationAdmissibility:
		return applyOperationAdmissibilityPrerequisiteResult(facts, prerequisite, result)
	default:
		return fmt.Errorf("unsupported prerequisite kind %q", prerequisite.Kind)
	}
}

func runtimeCapabilityStateBindingExists(values []runtimecontract.RuntimeCapabilityStateSnapshot, owner, binding string) bool {
	for _, snapshot := range values {
		if snapshot.OwnerModuleID != owner {
			continue
		}
		if snapshot.AcquisitionBinding == binding || (snapshot.AcquisitionBinding == "" && snapshot.AcquisitionKey == binding) {
			return true
		}
	}
	return false
}

func runtimeRequirementSatisfied(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) bool {
	switch requirement.Kind {
	case runtimecontract.PrerequisiteKindManifest:
		return facts.Resources.Manifest != nil && facts.Resources.Manifest.Revision != ""
	case runtimecontract.PrerequisiteKindTargetSnapshot:
		return runtimeTargetSnapshotRequirementSatisfied(facts, requirement)
	case runtimecontract.PrerequisiteKindTargetSelection:
		return runtimeTargetSelectionRequirementSatisfied(facts, requirement)
	case runtimecontract.PrerequisiteKindCapability:
		return capabilitiesSatisfy(facts.Capabilities, requirement.CapabilityClass, requirement.RequiredCapabilities)
	case runtimecontract.PrerequisiteKindCapabilityState:
		return runtimeCapabilityStateRequirementSatisfied(facts, requirement)
	case runtimecontract.PrerequisiteKindValidationScratch:
		return facts.Resources.ValidationScratch != nil && facts.Resources.ValidationScratch.Reference != "" && facts.Resources.ValidationScratch.Revision != ""
	case runtimecontract.PrerequisiteKindProviderCeiling:
		return facts.Resources.ProviderCeiling != nil && facts.Resources.ProviderCeiling.MaxBytes >= requirement.MinimumBytes
	case runtimecontract.PrerequisiteKindOperationAdmissibility:
		return runtimeOperationAdmissibilityRequirementSatisfied(facts, requirement)
	default:
		return false
	}
}

func runtimeTargetSnapshotRequirementSatisfied(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) bool {
	for _, snapshot := range facts.Resources.TargetSnapshots {
		if snapshot.TargetRef == requirement.SubjectRef && snapshot.Complete() {
			return true
		}
	}
	return false
}

func runtimeTargetSelectionRequirementSatisfied(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) bool {
	selection := facts.Resources.TargetSelection
	if selection == nil || selection.PrerequisiteID != runtimePrerequisiteID(requirement) {
		return false
	}
	if requirement.Scope != runtimecontract.RuntimePrerequisiteScopeFocus {
		return true
	}
	return currentFocusTargetSelection(facts) != nil
}

func runtimeCapabilityStateRequirementSatisfied(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) bool {
	for _, snapshot := range facts.Resources.CapabilityStates {
		if snapshot.OwnerModuleID == requirement.OwnerModuleID && snapshot.CapabilityStateKind == requirement.CapabilityStateKind && snapshot.SubjectRef == requirement.SubjectRef {
			return true
		}
	}
	return false
}

func runtimeOperationAdmissibilityRequirementSatisfied(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) bool {
	prerequisiteID := runtimePrerequisiteID(requirement)
	for _, inventory := range facts.Resources.OperationAdmissibilities {
		if inventory.PrerequisiteID == prerequisiteID && inventory.OwnerModuleID == requirement.OwnerModuleID {
			return true
		}
	}
	return false
}

func mergeCapabilityKinds(existing []string, incoming []string) []string {
	ret := make([]string, 0, len(existing)+len(incoming))
	seen := map[string]struct{}{}
	for _, values := range [][]string{existing, incoming} {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			ret = append(ret, value)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func capabilitiesSatisfy(capabilities runtimecontract.RuntimeCapabilities, class runtimecontract.RuntimeCapabilityClass, required []string) bool {
	var values []string
	switch class {
	case runtimecontract.RuntimeCapabilityClassStateChange:
		values = capabilities.StateChangeKinds
	case runtimecontract.RuntimeCapabilityClassValidation:
		values = capabilities.ValidationKinds
	case runtimecontract.RuntimeCapabilityClassGovernance:
		values = capabilities.GovernanceKinds
	default:
		return false
	}
	available := make(map[string]struct{}, len(values))
	for _, value := range values {
		available[value] = struct{}{}
	}
	for _, value := range required {
		if _, ok := available[value]; !ok {
			return false
		}
	}
	return len(required) > 0
}

func normalizeRuntimeRequirements(values []runtimecontract.RuntimeRequirement) []runtimecontract.RuntimeRequirement {
	byID := make(map[string]runtimecontract.RuntimeRequirement, len(values))
	for _, value := range values {
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		value.Scope = runtimecontract.RuntimePrerequisiteScope(strings.TrimSpace(string(value.Scope)))
		if value.Scope == "" {
			value.Scope = runtimecontract.RuntimePrerequisiteScopeGlobal
		}
		value.FocusID = strings.TrimSpace(value.FocusID)
		if value.Scope != runtimecontract.RuntimePrerequisiteScopeFocus {
			value.Scope = runtimecontract.RuntimePrerequisiteScopeGlobal
			value.FocusID = ""
		}
		value.TargetSelectorRef = strings.TrimSpace(value.TargetSelectorRef)
		value.TargetSelectorOwnerRef = strings.TrimSpace(value.TargetSelectorOwnerRef)
		value.TargetSelectorSubjectRef = strings.TrimSpace(value.TargetSelectorSubjectRef)
		value.TargetSelectorCandidateRefs = normalizeRequirementStrings(value.TargetSelectorCandidateRefs)
		value.ProspectiveTargetAllOfConstraintRefs = runtimecontract.NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(value.ProspectiveTargetAllOfConstraintRefs)
		value.ProspectiveTargetRef = strings.TrimSpace(value.ProspectiveTargetRef)
		value.OwnerModuleID = strings.TrimSpace(value.OwnerModuleID)
		if value.Kind == runtimecontract.PrerequisiteKindValidationScratch && value.OwnerModuleID == "" {
			// Scratch is an Orchestrator-local prerequisite. Recording its owner
			// in the normalized requirement makes its observation/invalidation
			// binding explicit rather than treating a local resource as ownerless.
			value.OwnerModuleID = runtimecontract.RuntimeModuleOrchestrator
		}
		value.CapabilityClass = runtimecontract.RuntimeCapabilityClass(strings.TrimSpace(string(value.CapabilityClass)))
		value.RequiredCapabilities = normalizeRequirementStrings(value.RequiredCapabilities)
		value.CapabilityStateKind = strings.TrimSpace(value.CapabilityStateKind)
		value.Reason = strings.TrimSpace(value.Reason)
		if value.Kind == "" {
			continue
		}
		byID[runtimePrerequisiteID(value)] = value
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ret := make([]runtimecontract.RuntimeRequirement, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, byID[id])
	}
	return ret
}

func runtimePrerequisiteID(requirement runtimecontract.RuntimeRequirement) string {
	parts := []string{string(requirement.Kind), string(requirement.Scope), requirement.FocusID, requirement.OwnerModuleID, requirement.SubjectRef, requirement.TargetSelectorRef, requirement.TargetSelectorOwnerRef, requirement.TargetSelectorSubjectRef, strings.Join(requirement.TargetSelectorCandidateRefs, ","), strings.Join(requirement.ProspectiveTargetAllOfConstraintRefs, ","), requirement.ProspectiveTargetRef, string(requirement.CapabilityClass), strings.Join(requirement.RequiredCapabilities, ","), requirement.CapabilityStateKind, fmt.Sprintf("%d", requirement.MinimumBytes)}
	return strings.Join(parts, ":")
}

func normalizeRequirementStrings(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}
