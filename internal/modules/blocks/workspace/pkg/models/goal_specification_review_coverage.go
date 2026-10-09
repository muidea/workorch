package workspace

import (
	"fmt"
	"sort"
)

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
