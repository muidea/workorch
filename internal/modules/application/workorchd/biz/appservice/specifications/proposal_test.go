package specifications

import (
	"encoding/json"
	"strings"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestProposalCannotApproveItselfOrChangeGoal(t *testing.T) {
	goal := workspace.DefinitionCandidateItem{CandidateItemRef: "goal:1", Text: "生成文档并保留来源", SourceIdentity: "goal:1@1"}
	dod := []workspace.DefinitionCandidateItem{{CandidateItemRef: "dod:1", Text: "来源可追溯", SourceIdentity: "dod:1@1"}}
	spec := workspace.GoalSpecification{Version: 1, GoalItemRef: goal.CandidateItemRef, GoalTextHash: workspace.StableDefinitionTextHash(goal.Text), Resources: []workspace.GoalSpecificationResource{}, Dimensions: []workspace.GoalSpecificationDimension{}, Requirements: []workspace.GoalSpecificationRequirement{}, Obligations: []workspace.GoalSpecificationObligation{}, Verifications: []workspace.GoalSpecificationVerification{}, Reviews: []workspace.GoalSpecificationCompletenessReview{}}
	encode := func(value workspace.GoalSpecification) []byte {
		t.Helper()
		data, err := json.Marshal(struct {
			Specification workspace.GoalSpecification `json:"specification"`
		}{value})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if _, err := ParseProposal(encode(spec), goal, dod); err != nil {
		t.Fatalf("proposal shape: %v", err)
	}
	changed := spec.Clone()
	changed.GoalTextHash = workspace.StableDefinitionTextHash("输出任意文档")
	if _, err := ParseProposal(encode(changed), goal, dod); err == nil {
		t.Fatal("changed Goal accepted")
	}
	changed = spec.Clone()
	changed.Reviews = []workspace.GoalSpecificationCompletenessReview{{Kind: workspace.GoalReviewTraceability, Status: workspace.GoalReviewPass}}
	if _, err := ParseProposal(encode(changed), goal, dod); err == nil {
		t.Fatal("self-approved proposal accepted")
	}
	data := strings.TrimSuffix(string(encode(spec)), "}") + `,"unknown":true}`
	if _, err := ParseProposal([]byte(data), goal, dod); err == nil {
		t.Fatal("unknown provider field accepted")
	}
}

func TestReviewRejectsUnknownCriticEvenWhenEchoed(t *testing.T) {
	spec := workspace.GoalSpecification{Version: 1, GoalItemRef: "goal:1", GoalTextHash: "hash"}
	payload, err := json.Marshal(struct {
		Review workspace.GoalSpecificationCompletenessReview `json:"review"`
	}{workspace.GoalSpecificationCompletenessReview{Kind: "custom", Status: workspace.GoalReviewPass, SpecificationHash: spec.ContentHash()}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseReview(payload, "custom", spec); err == nil {
		t.Fatal("unknown critic accepted by matching echo")
	}
}

func TestReviewRequiresExactKindHashAndTriState(t *testing.T) {
	spec := workspace.GoalSpecification{Version: 1, GoalItemRef: "goal:1", GoalTextHash: "hash"}
	kind := workspace.GoalReviewRegression
	for _, status := range []string{workspace.GoalReviewPass, workspace.GoalReviewFail, workspace.GoalReviewUnknown} {
		review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: status, SpecificationHash: spec.ContentHash(), EvidenceRefs: []string{}, RequirementIDs: []string{}}
		payload, err := json.Marshal(struct {
			Review workspace.GoalSpecificationCompletenessReview `json:"review"`
		}{review})
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseReview(payload, kind, spec)
		if err != nil || got.Status != status {
			t.Fatalf("review status changed: %#v, %v", got, err)
		}
		if _, err := ParseReview(payload, workspace.GoalReviewNegativePath, spec); err == nil {
			t.Fatal("different critic accepted")
		}
		other := spec.Clone()
		other.GoalTextHash = "different"
		if _, err := ParseReview(payload, kind, other); err == nil {
			t.Fatal("stale review accepted")
		}
	}
}
