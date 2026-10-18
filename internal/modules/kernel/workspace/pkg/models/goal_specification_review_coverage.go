package workspace

import (
	"fmt"
	"sort"
)

// ReviewEvidenceRefs projects the exact review-provenance namespace from an
// unreviewed, structurally valid proposal. Hashes and Profile declarations are
// request context, not additional evidence IDs or execution evidence.
func (s GoalSpecification) ReviewEvidenceRefs(goal DefinitionCandidateItem, dod []DefinitionCandidateItem) ([]string, error) {
	if len(s.Reviews) != 0 {
		return nil, fmt.Errorf("review evidence refs require an unreviewed proposal")
	}
	checked := s.Clone()
	for _, kind := range goalReviewKinds {
		checked.Reviews = append(checked.Reviews, GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewUnknown, SpecificationHash: s.ContentHash()})
	}
	if err := checked.Validate(goal, dod); err != nil {
		return nil, err
	}
	refs := []string{goal.CandidateItemRef}
	for _, item := range dod {
		refs = append(refs, item.CandidateItemRef)
	}
	for _, resource := range s.Resources {
		refs = append(refs, resource.ID)
	}
	for _, dimension := range s.Dimensions {
		refs = append(refs, dimension.ID)
	}
	for _, requirement := range s.Requirements {
		refs = append(refs, requirement.ID)
	}
	for _, obligation := range s.Obligations {
		refs = append(refs, obligation.ID)
	}
	for _, verification := range s.Verifications {
		refs = append(refs, verification.ID)
	}
	sort.Strings(refs)
	return refs, nil
}

// 应审范围只从 owner 编译的要求派生；缺少资源或风险声明不证明不适用。
func (s GoalSpecification) RequiredReviewRequirementIDs(kind string) ([]string, error) {
	if !GoalSpecificationReviewKind(kind) {
		return nil, fmt.Errorf("unknown goal specification review kind %q", kind)
	}
	ids := make([]string, 0)
	for _, requirement := range s.Requirements {
		if requirement.Required {
			ids = append(ids, requirement.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// 仅检查审查记录的覆盖范围，不代替语义充分性判断。
func (s GoalSpecification) MissingReviewRequirementIDs(review GoalSpecificationCompletenessReview) ([]string, error) {
	required, err := s.RequiredReviewRequirementIDs(review.Kind)
	if err != nil {
		return nil, err
	}
	covered := make(map[string]bool, len(review.RequirementIDs))
	for _, id := range review.RequirementIDs {
		covered[id] = true
	}
	missing := make([]string, 0)
	for _, id := range required {
		if !covered[id] {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

func (s GoalSpecification) ValidateReviewCoverage(review GoalSpecificationCompletenessReview) error {
	missing, err := s.MissingReviewRequirementIDs(review)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("goal specification review %q lacks required requirement coverage: %v", review.Kind, missing)
	}
	return nil
}
