package specifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

// ParseProposal decodes a complete specification proposal, not an admitted
// definition. Reviews must be supplied by separate review transactions.
func ParseProposal(payload []byte, goal workspace.DefinitionCandidateItem, dod []workspace.DefinitionCandidateItem) (workspace.GoalSpecification, error) {
	if err := validateDocumentShape(payload); err != nil {
		return workspace.GoalSpecification{}, err
	}
	var document struct {
		Specification *workspace.GoalSpecification `json:"specification"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return workspace.GoalSpecification{}, fmt.Errorf("decode specification proposal: %w", err)
	}
	if document.Specification == nil || document.Specification.Version != 1 {
		return workspace.GoalSpecification{}, fmt.Errorf("specification proposal requires current version")
	}
	spec := document.Specification.Clone()
	if len(spec.Reviews) != 0 {
		return workspace.GoalSpecification{}, fmt.Errorf("specification proposal cannot supply its own completeness reviews")
	}
	if spec.GoalItemRef != goal.CandidateItemRef || spec.GoalTextHash != workspace.StableDefinitionTextHash(goal.Text) {
		return workspace.GoalSpecification{}, fmt.Errorf("specification proposal does not match the original Goal")
	}
	if len(dod) == 0 {
		return workspace.GoalSpecification{}, fmt.Errorf("specification proposal requires explicit candidate item identities")
	}
	return spec, nil
}

// ParseReview binds a separately obtained critique to the exact proposal. A
// syntactically valid PASS remains a proposal for Workspace admission checks.
func ParseReview(payload []byte, kind string, specification workspace.GoalSpecification) (workspace.GoalSpecificationCompletenessReview, error) {
	if !slices.Contains(ReviewKinds(), kind) {
		return workspace.GoalSpecificationCompletenessReview{}, fmt.Errorf("unknown specification review kind")
	}
	if err := validateDocumentShape(payload); err != nil {
		return workspace.GoalSpecificationCompletenessReview{}, err
	}
	var document struct {
		Review *workspace.GoalSpecificationCompletenessReview `json:"review"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return workspace.GoalSpecificationCompletenessReview{}, fmt.Errorf("decode specification review: %w", err)
	}
	if document.Review == nil || document.Review.Kind != kind || document.Review.SpecificationHash != specification.ContentHash() {
		return workspace.GoalSpecificationCompletenessReview{}, fmt.Errorf("specification review identity mismatch")
	}
	switch document.Review.Status {
	case workspace.GoalReviewPass, workspace.GoalReviewFail, workspace.GoalReviewUnknown:
	default:
		return workspace.GoalSpecificationCompletenessReview{}, fmt.Errorf("specification review status is invalid")
	}
	return *document.Review, nil
}

func ReviewKinds() []string {
	return []string{
		workspace.GoalReviewDimensionCoverage,
		workspace.GoalReviewNegativePath,
		workspace.GoalReviewRegression,
		workspace.GoalReviewResourceEnvironment,
		workspace.GoalReviewTraceability,
		workspace.GoalReviewVerifierFeasibility,
	}
}
