package workspace

import (
	"fmt"
	"slices"
	"strings"
)

const DefinitionAuthoringGoalItemRef = "authoring-goal"

// AuthoringSpecificationResources derives resource identities from a snapshot
// already admitted to the Workspace derivation ledger. This pure function only
// checks internal consistency; File-origin provenance is checked at reserve.
// A provider may reference these identities, but cannot invent availability or
// choose a different snapshot hash.
func AuthoringSpecificationResources(input DefinitionAuthoringInput, snapshots []DefinitionAuthoringResourceSnapshot) ([]GoalSpecificationResource, error) {
	if err := ValidateDefinitionAuthoringResources(input, snapshots); err != nil {
		return nil, err
	}
	resources := make([]GoalSpecificationResource, 0, len(snapshots))
	for index, snapshot := range snapshots {
		resources = append(resources, GoalSpecificationResource{ID: fmt.Sprintf("authoring-resource:%06d", index+1), Ref: snapshot.Ref, SnapshotHash: snapshot.ContentHash, Required: true, Available: true})
	}
	return resources, nil
}

type authoringSpecificationContext struct {
	goal      DefinitionCandidateItem
	items     []DefinitionCandidateItem
	resources []GoalSpecificationResource
}

func newAuthoringSpecificationContext(input DefinitionAuthoringInput, template *WorkspaceTemplate, derived DefinitionDerivedAcceptance, snapshots []DefinitionAuthoringResourceSnapshot) (authoringSpecificationContext, error) {
	items, err := derived.CandidateItems(input, template)
	if err != nil {
		return authoringSpecificationContext{}, err
	}
	resources, err := AuthoringSpecificationResources(input, snapshots)
	if err != nil {
		return authoringSpecificationContext{}, err
	}
	return authoringSpecificationContext{
		goal:      DefinitionCandidateItem{CandidateItemRef: DefinitionAuthoringGoalItemRef, Text: input.Normalized().GoalText, SourceIdentity: input.InputHash()},
		items:     items,
		resources: resources,
	}, nil
}

// ValidateAuthoringSpecification checks that a proposed specification retains
// every sealed authoring item and the exact persisted resource identity. It
// does not establish File-origin provenance or current availability. Textual
// equivalence beyond exact preservation still requires the six independent
// reviews and owner admission.
func ValidateAuthoringSpecification(input DefinitionAuthoringInput, template *WorkspaceTemplate, derived DefinitionDerivedAcceptance, snapshots []DefinitionAuthoringResourceSnapshot, spec GoalSpecification) error {
	context, err := newAuthoringSpecificationContext(input, template, derived, snapshots)
	if err != nil {
		return err
	}
	return context.validate(spec)
}

func (c authoringSpecificationContext) validate(spec GoalSpecification) error {
	goal, items, resources := c.goal, c.items, c.resources
	if len(spec.Resources) != len(resources) {
		return fmt.Errorf("authoring specification changed sealed resource facts")
	}
	expectedByID := make(map[string]GoalSpecificationResource, len(resources))
	for _, expected := range resources {
		expectedByID[expected.ID] = expected
	}
	seen := make(map[string]bool, len(spec.Resources))
	for _, actual := range spec.Resources {
		if expected, ok := expectedByID[actual.ID]; !ok || seen[actual.ID] || actual != expected {
			return fmt.Errorf("authoring specification changed sealed resource facts")
		}
		seen[actual.ID] = true
	}
	withReviews := spec.Clone()
	if len(withReviews.Reviews) == 0 {
		proposalHash := spec.ContentHash()
		for _, kind := range goalReviewKinds {
			withReviews.Reviews = append(withReviews.Reviews, GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewUnknown, SpecificationHash: proposalHash})
		}
	}
	if err := withReviews.Validate(goal, items); err != nil {
		return err
	}
	if len(spec.Dimensions) == 0 || len(spec.Requirements) == 0 {
		return fmt.Errorf("authoring specification requires dimensions and requirements")
	}
	for _, dimension := range spec.Dimensions {
		if dimension.Required && !dimension.Activated {
			return fmt.Errorf("authoring specification dimension %q is not activated", dimension.ID)
		}
	}
	for _, requirement := range spec.Requirements {
		if !requirement.Required || requirement.Text == "" || len(requirement.SourceItemRefs) == 0 {
			return fmt.Errorf("authoring specification requirement %q is not required or sourced", requirement.ID)
		}
		if !authoringRequirementVerified(requirement, spec.Obligations, spec.Verifications) {
			return fmt.Errorf("authoring specification requirement %q has no traceable obligation and verification", requirement.ID)
		}
	}
	if !authoringGoalRequirementPresent(goal.Text, spec.Requirements) {
		return fmt.Errorf("authoring specification omitted original Goal")
	}
	for _, item := range items {
		if !authoringItemObligationPresent(item, spec.Requirements, spec.Obligations) {
			return fmt.Errorf("authoring specification omitted or changed item %q", item.CandidateItemRef)
		}
	}
	for _, obligation := range spec.Obligations {
		if obligation.Text == "" || len(obligation.DoDItemRefs) == 0 {
			return fmt.Errorf("authoring specification obligation %q lacks candidate mapping", obligation.ID)
		}
	}
	for _, requirement := range spec.Requirements {
		for _, ref := range requirement.SourceItemRefs {
			if ref == DefinitionAuthoringGoalItemRef {
				if requirement.SourceKind != GoalRequirementUserExplicit {
					return fmt.Errorf("authoring specification Goal source was relabeled")
				}
				continue
			}
			for _, item := range items {
				if item.CandidateItemRef == ref && requirement.SourceKind != authoringItemKind(item) {
					return fmt.Errorf("authoring specification item source was relabeled")
				}
			}
		}
	}
	return nil
}

func authoringRequirementVerified(requirement GoalSpecificationRequirement, obligations []GoalSpecificationObligation, verifications []GoalSpecificationVerification) bool {
	for _, obligation := range obligations {
		if obligation.RequirementID != requirement.ID || obligation.Text == "" || len(obligation.DoDItemRefs) == 0 {
			continue
		}
		for _, verification := range verifications {
			if verification.ObligationID == obligation.ID && verification.Declaration != "" {
				return true
			}
		}
	}
	return false
}

func authoringGoalRequirementPresent(goal string, requirements []GoalSpecificationRequirement) bool {
	for _, requirement := range requirements {
		if requirement.Required && requirement.SourceKind == GoalRequirementUserExplicit &&
			requirement.Text == goal && slices.Contains(requirement.SourceItemRefs, DefinitionAuthoringGoalItemRef) {
			return true
		}
	}
	return false
}

func authoringItemKind(item DefinitionCandidateItem) string {
	switch {
	case strings.HasPrefix(item.CandidateItemRef, "derived-dod:user:"):
		return GoalRequirementUserExplicit
	case strings.HasPrefix(item.CandidateItemRef, "derived-dod:template:"):
		return GoalRequirementTemplateRequired
	default:
		return GoalRequirementDerived
	}
}

func authoringItemObligationPresent(item DefinitionCandidateItem, requirements []GoalSpecificationRequirement, obligations []GoalSpecificationObligation) bool {
	kind := authoringItemKind(item)
	for _, requirement := range requirements {
		if !requirement.Required || requirement.SourceKind != kind || !slices.Contains(requirement.SourceItemRefs, item.CandidateItemRef) {
			continue
		}
		for _, obligation := range obligations {
			if obligation.RequirementID == requirement.ID && obligation.Text == item.Text && slices.Contains(obligation.DoDItemRefs, item.CandidateItemRef) {
				return true
			}
		}
	}
	return false
}
