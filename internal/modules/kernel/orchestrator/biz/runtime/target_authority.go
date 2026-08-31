package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/grounding"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeTargetAuthorityPlan is the sole owner-private projection that
// classifies the current Focus target authority. It is not persisted and does
// not choose a Runtime transition: SM-2 and SM-3 consume the same derived fact
// so a target cannot be treated as both prospective and already existing.
type runtimeTargetAuthorityPlan struct {
	targetRefs               []string
	existingTargetRefs       []string
	prospectiveTargetRefs    []string
	prospectiveScopeGrounded bool
	semanticScopeGrounded    bool
	ownerScopeGrounded       bool
}

// runtimeCurrentTargetGroundingFocus projects the current target declaration
// without copying it into the pre-semantic queue cursor. Before the first
// semantic seal, Focus intentionally contains presentation/CWI identity only;
// the immutable DefinitionFocus remains the target-selection authority. After
// seal, the generation-specific Focus is authoritative and is returned as-is.
//
// This is a pure SM-2 projection. It neither selects a target nor persists a
// second focus/reducer state.
func runtimeCurrentTargetGroundingFocus(facts runtimecontract.RuntimeFacts) (runtimecontract.CoverageFocus, bool) {
	facts = facts.Normalized()
	focus := facts.Focus.Normalized()
	if focus.RequireTargetGrounding {
		return focus, true
	}
	if len(focus.CoverageWorkItemIDs) != 1 {
		return runtimecontract.CoverageFocus{}, false
	}
	declaration, found, err := runtimeDefinitionFocusForCoverage(facts.DefinitionFocuses, focus.CoverageWorkItemIDs[0])
	if err != nil || !found || !declaration.RequireTargetGrounding {
		return runtimecontract.CoverageFocus{}, false
	}
	projected := declaration.CoverageFocusDeclaration()
	// Prerequisites are scoped to the current queue/generation focus identity,
	// never to a copied DefinitionFocus identity from an earlier phase.
	projected.ID = focus.ID
	projected.SliceID = focus.SliceID
	projected.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), focus.CoverageWorkItemIDs...)
	return projected.Normalized(), true
}

// CurrentFocusTargetSelectionConfirmsUnresolved recognizes only an exact,
// current-focus, owner-observed TargetSelection whose immutable selection
// result proves that target grounding cannot proceed. A raw selection pointer,
// an unobserved/satisfied-only fixture, or a selection for another declaration
// remains transient and cannot make target_grounding_unresolved deliverable.
func CurrentFocusTargetSelectionConfirmsUnresolved(facts runtimecontract.RuntimeFacts) bool {
	facts = facts.Normalized()
	selection := grounding.CurrentFocusTargetSelection(facts)
	declaration, declared := runtimeCurrentTargetGroundingFocus(facts)
	if selection == nil || !declared || selection.Validate() != nil {
		return false
	}
	value := selection.Normalized()
	prospectiveTargetRef := ""
	if len(declaration.ProspectiveTargetRefs) == 1 {
		prospectiveTargetRef = declaration.ProspectiveTargetRefs[0]
	} else if len(declaration.ProspectiveTargetRefs) > 1 {
		return false
	}
	if value.TargetRole != declaration.TargetRole ||
		value.SelectorRef != declaration.TargetSelectorRef ||
		value.SelectorOwnerRef != declaration.TargetSelectorOwnerRef ||
		value.SelectorSubjectRef != declaration.TargetSelectorSubjectRef ||
		!runtimecontract.SameRuntimeTargetSelectorCandidateRefs(value.SelectorCandidateRefs, declaration.TargetSelectorCandidateRefs) ||
		!runtimecontract.SameRuntimeProspectiveTargetAllOfConstraintRefs(value.ProspectiveTargetAllOfConstraintRefs, declaration.ProspectiveTargetAllOfConstraintRefs) ||
		value.ProspectiveTargetRef != prospectiveTargetRef {
		return false
	}

	prerequisiteMatched := false
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if prerequisite.ID == value.PrerequisiteID &&
			prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSelection &&
			prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus &&
			prerequisite.FocusID == facts.Focus.ID &&
			prerequisite.OwnerModuleID == value.OwnerModuleID &&
			prerequisite.SubjectRef == value.TargetRole &&
			prerequisite.TargetSelectorRef == value.SelectorRef &&
			prerequisite.TargetSelectorOwnerRef == value.SelectorOwnerRef &&
			prerequisite.TargetSelectorSubjectRef == value.SelectorSubjectRef &&
			runtimecontract.SameRuntimeTargetSelectorCandidateRefs(prerequisite.TargetSelectorCandidateRefs, value.SelectorCandidateRefs) &&
			runtimecontract.SameRuntimeProspectiveTargetAllOfConstraintRefs(prerequisite.ProspectiveTargetAllOfConstraintRefs, value.ProspectiveTargetAllOfConstraintRefs) &&
			prerequisite.ProspectiveTargetRef == value.ProspectiveTargetRef {
			prerequisiteMatched = true
			break
		}
	}
	if !prerequisiteMatched {
		return false
	}

	observationMatched := false
	for _, observation := range facts.Resources.PrerequisiteObservations {
		observation = observation.Normalized()
		if observation.Validate() != nil || observation.PrerequisiteID != value.PrerequisiteID ||
			observation.Kind != runtimecontract.PrerequisiteKindTargetSelection || observation.OwnerModuleID != value.OwnerModuleID ||
			observation.ContentHash != value.SnapshotID || len(observation.Dependencies) != 1 {
			continue
		}
		dependency := observation.Dependencies[0].Normalized()
		if dependency.Domain == runtimecontract.RuntimeResourceWorkspaceTree && dependency.OwnerModuleID == value.OwnerModuleID &&
			dependency.Revision == value.ObservedRevision {
			observationMatched = true
			break
		}
	}
	if !observationMatched {
		return false
	}

	switch value.Disposition {
	case runtimecontract.RuntimeTargetSelectionAbsent,
		runtimecontract.RuntimeTargetSelectionAmbiguous,
		runtimecontract.RuntimeTargetSelectionUnsupported:
		return true
	case runtimecontract.RuntimeTargetSelectionScope:
		// Empty current owner scope is terminal only when the immutable
		// declaration has neither a finite exact candidate surface nor an
		// owner-accepted all-of prospective creation contract. With either
		// declaration it means "nothing exists yet", not "no legal target can
		// be created".
		return value.CandidateCount == 0 && len(value.TargetRefs) == 0 && len(value.CandidateTargetRefs) == 0 &&
			len(value.SelectorCandidateRefs) == 0 && len(value.ProspectiveTargetAllOfConstraintRefs) == 0
	default:
		return false
	}
}

func runtimeTargetAuthorityPlanForFacts(facts runtimecontract.RuntimeFacts) runtimeTargetAuthorityPlan {
	facts = facts.Normalized()
	focus := facts.Focus.Normalized()
	targetRefs := grounding.RepairTargets(facts)

	prospective := make(map[string]struct{}, len(focus.ProspectiveTargetRefs)+1)
	semanticScopeGrounded := hasSemanticOwnerGroundedTargetScope(facts)
	semanticProspectiveGrounded := len(focus.ProspectiveTargetRefs) != 0 && semanticScopeGrounded
	if semanticProspectiveGrounded {
		// A semantic binding is itself the first authoritative source for this
		// prospective all-of set. The older repair-target projection only knows
		// explicit/failure/selection/directive refs, so use the sealed Focus set
		// as its lowest-precedence fallback instead of returning a grounded flag
		// with no targets.
		if len(targetRefs) == 0 {
			targetRefs = append([]string(nil), focus.ProspectiveTargetRefs...)
		}
		for _, targetRef := range focus.ProspectiveTargetRefs {
			if targetRef = strings.TrimSpace(targetRef); targetRef != "" {
				prospective[targetRef] = struct{}{}
			}
		}
	}
	if grounding.HasOwnerGroundedCreationScope(facts) {
		if selection := grounding.CurrentFocusTargetSelection(facts); selection != nil {
			if targetRef := strings.TrimSpace(selection.ProspectiveTargetRef); targetRef != "" {
				prospective[targetRef] = struct{}{}
			}
		}
	}

	plan := runtimeTargetAuthorityPlan{
		targetRefs:               append([]string(nil), targetRefs...),
		prospectiveScopeGrounded: semanticProspectiveGrounded || grounding.HasOwnerGroundedCreationScope(facts),
		semanticScopeGrounded:    semanticScopeGrounded,
		ownerScopeGrounded:       grounding.HasOwnerGroundedTargetScope(facts),
	}
	for _, targetRef := range targetRefs {
		targetRef = strings.TrimSpace(targetRef)
		if targetRef == "" {
			continue
		}
		if _, found := prospective[targetRef]; found {
			plan.prospectiveTargetRefs = append(plan.prospectiveTargetRefs, targetRef)
			continue
		}
		plan.existingTargetRefs = append(plan.existingTargetRefs, targetRef)
	}
	return plan
}

func (p runtimeTargetAuthorityPlan) targets() []string {
	return append([]string(nil), p.targetRefs...)
}

func (p runtimeTargetAuthorityPlan) existingTargets() []string {
	return append([]string(nil), p.existingTargetRefs...)
}

func (p runtimeTargetAuthorityPlan) prospectiveTargets() []string {
	return append([]string(nil), p.prospectiveTargetRefs...)
}

func (p runtimeTargetAuthorityPlan) hasProspectiveTargets() bool {
	return p.prospectiveScopeGrounded
}

func (p runtimeTargetAuthorityPlan) grounded() bool {
	return len(p.targetRefs) != 0 || p.prospectiveScopeGrounded || p.semanticScopeGrounded || p.ownerScopeGrounded
}

// runtimeValidationScopeTargetRefs projects the diagnostic scope from the
// exact SM-3 dispatch seal. A selector-only authorization deliberately has no
// finite diagnostic scope: execution remains legal, but the validation owner cannot
// publish a structured repair observation for it. Neither shape may substitute
// the current workspace manifest, focus, or owner inventory after seal/restart.
func runtimeValidationScopeTargetRefs(dispatch SealedDispatch, operation runtimecontract.RuntimeOperation) ([]string, error) {
	if runtimecontract.CanonicalRuntimeBlockKind(operation.Kind) != runtimecontract.RuntimeBlockKindManagedValidationSession {
		return nil, nil
	}
	authorization, err := runtimeValidationTargetAuthorization(dispatch, operation)
	if err != nil {
		return nil, err
	}
	return authorization.Target.AllowedTargetRefs(), nil
}

func runtimeValidationTargetAuthorization(dispatch SealedDispatch, operation runtimecontract.RuntimeOperation) (runtimecontract.RuntimeDispatchOperationTargetAuthorization, error) {
	if runtimecontract.CanonicalRuntimeBlockKind(operation.Kind) != runtimecontract.RuntimeBlockKindManagedValidationSession {
		return runtimecontract.RuntimeDispatchOperationTargetAuthorization{}, fmt.Errorf("operation %q is not managed validation", operation.ID)
	}
	authorization, err := runtimecontract.RuntimeDispatchOperationTargetAuthorizationForOperation(
		dispatch.Candidate,
		dispatch.OperationTargetAuthorizations,
		operation,
	)
	if err != nil {
		return runtimecontract.RuntimeDispatchOperationTargetAuthorization{}, fmt.Errorf("sealed managed-validation target authorization: %w", err)
	}
	return authorization, nil
}
