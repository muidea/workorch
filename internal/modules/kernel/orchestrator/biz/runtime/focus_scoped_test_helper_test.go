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
	for index := range facts.Prerequisites.Satisfied {
		if facts.Prerequisites.Satisfied[index].ID == prerequisite.ID {
			facts.Prerequisites.Satisfied[index] = prerequisite
			return
		}
	}
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, prerequisite)
}
