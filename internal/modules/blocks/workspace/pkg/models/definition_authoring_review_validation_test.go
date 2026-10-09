package workspace

import (
	"reflect"
	"strings"
	"testing"
)

func TestAuthoringReviewValidationRetainsFullSpecificationChecks(t *testing.T) {
	candidate := testGoalSpecificationCandidate()
	proposal := candidate.Specification.Clone()
	proposal.Reviews = nil
	context := authoringSpecificationContext{goal: candidate.GoalInput, items: candidate.DoDInputs}
	hash := proposal.ContentHash()
	valid := GoalSpecificationCompletenessReview{Kind: GoalReviewTraceability, Status: GoalReviewPass, SpecificationHash: hash, Reason: "独立审查", EvidenceRefs: []string{"goal"}, RequirementIDs: []string{"req"}}
	for name, mutate := range map[string]func(*GoalSpecificationCompletenessReview){
		"pass": func(*GoalSpecificationCompletenessReview) {},
		"fail": func(r *GoalSpecificationCompletenessReview) {
			r.Status = GoalReviewFail
			r.Reason = ""
			r.EvidenceRefs = nil
			r.RequirementIDs = nil
		},
		"unknown": func(r *GoalSpecificationCompletenessReview) {
			r.Status = GoalReviewUnknown
			r.Reason = ""
			r.EvidenceRefs = nil
			r.RequirementIDs = nil
		},
		"stale_hash":            func(r *GoalSpecificationCompletenessReview) { r.SpecificationHash = "stale" },
		"empty_status":          func(r *GoalSpecificationCompletenessReview) { r.Status = "" },
		"unknown_status":        func(r *GoalSpecificationCompletenessReview) { r.Status = "OTHER" },
		"missing_reason":        func(r *GoalSpecificationCompletenessReview) { r.Reason = "" },
		"reason_bound":          func(r *GoalSpecificationCompletenessReview) { r.Reason = strings.Repeat("r", 4097) },
		"missing_evidence":      func(r *GoalSpecificationCompletenessReview) { r.EvidenceRefs = nil },
		"unknown_evidence":      func(r *GoalSpecificationCompletenessReview) { r.EvidenceRefs = []string{"absent"} },
		"duplicate_evidence":    func(r *GoalSpecificationCompletenessReview) { r.EvidenceRefs = []string{"goal", "goal"} },
		"evidence_bound":        func(r *GoalSpecificationCompletenessReview) { r.EvidenceRefs = make([]string, 65) },
		"missing_requirement":   func(r *GoalSpecificationCompletenessReview) { r.RequirementIDs = nil },
		"unknown_requirement":   func(r *GoalSpecificationCompletenessReview) { r.RequirementIDs = []string{"absent"} },
		"duplicate_requirement": func(r *GoalSpecificationCompletenessReview) { r.RequirementIDs = []string{"req", "req"} },
		"requirement_bound":     func(r *GoalSpecificationCompletenessReview) { r.RequirementIDs = make([]string, 257) },
		"normalized_refs": func(r *GoalSpecificationCompletenessReview) {
			r.EvidenceRefs = []string{" goal "}
			r.RequirementIDs = []string{" req "}
		},
	} {
		t.Run(name, func(t *testing.T) {
			review := valid
			mutate(&review)
			before := proposal.Clone()
			seed := proposal.Clone()
			for _, kind := range goalReviewKinds {
				entry := GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewUnknown, SpecificationHash: hash}
				if kind == review.Kind {
					entry = review
				}
				seed.Reviews = append(seed.Reviews, entry)
			}
			want := seed.Validate(candidate.GoalInput, candidate.DoDInputs)
			validCase := name == "pass" || name == "fail" || name == "unknown" || name == "normalized_refs"
			if (want == nil) != validCase {
				t.Fatalf("完整规格校验未保持预期合同：%v", want)
			}
			got := authoringProposalReviewContext(context, proposal, hash).validate(review)
			if (got == nil) != (want == nil) || !reflect.DeepEqual(before, proposal) {
				t.Fatalf("独立 review 校验与完整规格合同不一致或改写提案：got=%v want=%v", got, want)
			}
		})
	}
}
