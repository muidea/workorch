package specifications

import (
	"encoding/json"
	"strings"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestReviewPASSRequiresAllRequiredRequirements(t *testing.T) {
	spec := workspace.GoalSpecification{Version: 1, Requirements: []workspace.GoalSpecificationRequirement{
		{ID: "goal", Required: true}, {ID: "required-a", Required: true}, {ID: "required-b", Required: true}, {ID: "optional"},
	}}
	for _, kind := range ReviewKinds() {
		t.Run(kind, func(t *testing.T) {
			for _, ids := range [][]string{{"goal"}, {"goal", "required-a", "optional"}, {}} {
				review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, SpecificationHash: spec.ContentHash(), Reason: "声称全部通过", EvidenceRefs: []string{"goal"}, RequirementIDs: ids}
				payload, err := json.Marshal(struct {
					Review workspace.GoalSpecificationCompletenessReview `json:"review"`
				}{review})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := ParseReview(payload, kind, spec); err == nil || !strings.Contains(err.Error(), "required-b") {
					t.Fatalf("漏审未被明确拒绝: ids=%v err=%v", ids, err)
				}
			}
			for _, status := range []string{workspace.GoalReviewPass, workspace.GoalReviewFail, workspace.GoalReviewUnknown} {
				ids := []string{"goal", "required-a", "required-b", "optional"}
				if status != workspace.GoalReviewPass {
					ids = []string{"goal"}
				}
				review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: status, SpecificationHash: spec.ContentHash(), Reason: "独立审查", EvidenceRefs: []string{"goal"}, RequirementIDs: ids}
				payload, err := json.Marshal(struct {
					Review workspace.GoalSpecificationCompletenessReview `json:"review"`
				}{review})
				if err != nil {
					t.Fatal(err)
				}
				got, err := ParseReview(payload, kind, spec)
				if err != nil || got.Status != status || len(got.RequirementIDs) != len(ids) {
					t.Fatalf("有效审查事实被改写: status=%s got=%+v err=%v", status, got, err)
				}
			}
		})
	}
}
