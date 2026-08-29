package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// bindCurrentFocusTargetSelection makes a fixture model the production commit
// invariant: a target-selection resource is authority only after its
// focus-scoped prerequisite was committed. Tests must not use the raw resource
// pointer as a legacy authority shortcut.
func bindCurrentFocusTargetSelection(facts *runtimecontract.RuntimeFacts) {
	if facts == nil || facts.Resources.TargetSelection == nil {
		return
	}
	selection := facts.Resources.TargetSelection.Normalized()
	facts.Resources.TargetSelection = &selection
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID:            selection.PrerequisiteID,
		Kind:          runtimecontract.PrerequisiteKindTargetSelection,
		Scope:         runtimecontract.RuntimePrerequisiteScopeFocus,
		FocusID:       facts.Focus.ID,
		OwnerModuleID: selection.OwnerModuleID,
		SubjectRef:    selection.TargetRole,
	}
	keptPending := facts.Prerequisites.Pending[:0]
	for _, value := range facts.Prerequisites.Pending {
		if value.ID != prerequisite.ID {
			keptPending = append(keptPending, value)
		}
	}
	facts.Prerequisites.Pending = keptPending
	keptRetired := facts.Prerequisites.Retired[:0]
	for _, value := range facts.Prerequisites.Retired {
		if value.ID != prerequisite.ID {
			keptRetired = append(keptRetired, value)
		}
	}
	facts.Prerequisites.Retired = keptRetired
	for index := range facts.Prerequisites.Satisfied {
		if facts.Prerequisites.Satisfied[index].ID == prerequisite.ID {
			facts.Prerequisites.Satisfied[index] = prerequisite
			return
		}
	}
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, prerequisite)
}

type runtimeTestBindingTargetSurface struct {
	TargetRefs            []string
	SelectorRef           string
	SelectorOwnerRef      string
	SelectorSubjectRef    string
	SelectorCandidateRefs []string
	ProspectiveTargetRef  string
}

// runtimeTestSealCurrentFocusBindingTargetSurface explicitly constructs the
// post-SM-2 authority surface used by a fixture. The binding is written first;
// Focus is then projected from it. Owner observations remain consumers and are
// never allowed to rewrite this surface implicitly.
func runtimeTestSealCurrentFocusBindingTargetSurface(facts *runtimecontract.RuntimeFacts, surface runtimeTestBindingTargetSurface) {
	if facts == nil || len(facts.Focus.CoverageEvidenceBindings) == 0 {
		return
	}
	focus := facts.Focus.Normalized()
	for index := range focus.CoverageEvidenceBindings {
		binding := focus.CoverageEvidenceBindings[index].Clone()
		binding.TargetRefs = append([]string(nil), surface.TargetRefs...)
		binding.TargetSelectorRef = surface.SelectorRef
		binding.TargetSelectorOwnerRef = surface.SelectorOwnerRef
		binding.TargetSelectorSubjectRef = surface.SelectorSubjectRef
		binding.TargetSelectorCandidateRefs = append([]string(nil), surface.SelectorCandidateRefs...)
		if surface.ProspectiveTargetRef != "" {
			binding.ProspectiveTargetRefs = []string{surface.ProspectiveTargetRef}
		}
		binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
		focus.CoverageEvidenceBindings[index] = binding
	}
	focus = runtimeTestAlignFocusTargetSurfaceFromBindings(focus)
	facts.Focus = focus
	allFocuses := append([]runtimecontract.CoverageFocus{focus}, facts.RemainingFocuses...)
	allFocuses = append(allFocuses, facts.CompletedFocuses...)
	if contract, err := runtimecontract.CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, allFocuses); err == nil {
		facts.ReconciliationContract = contract
	}
	for coverageIndex := range facts.Coverage {
		row := &facts.Coverage[coverageIndex]
		for _, binding := range focus.CoverageEvidenceBindings {
			if binding.CoverageWorkItemID != row.ID || row.CurrentBindingRef == nil {
				continue
			}
			ref := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(binding, row.CurrentBindingRef.SemanticManifestHash)
			row.CurrentBindingRef = &ref
			if len(row.BindingLineage) != 0 {
				row.BindingLineage[len(row.BindingLineage)-1] = ref
			}
		}
	}
}
