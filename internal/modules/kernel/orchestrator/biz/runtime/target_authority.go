package runtime

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/grounding"
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
