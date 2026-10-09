package definitionadmission

import (
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func independentGoalSpecificationCandidate() workspace.DefinitionAdmissionCandidate {
	candidate := candidateWithSpecification()
	spec := candidate.Specification
	spec.Requirements[0].SourceItemRefs = []string{candidate.DoDInputs[0].CandidateItemRef}
	goal := workspace.GoalSpecificationRequirement{
		ID: "independent-goal", DimensionID: spec.Dimensions[0].ID, Required: true,
		SourceKind:     workspace.GoalRequirementUserExplicit,
		SourceItemRefs: []string{spec.GoalItemRef}, Text: candidate.GoalInput.Text,
	}
	spec.Requirements = append(spec.Requirements, goal)
	return resealIndependentGoalSpecification(candidate)
}

func resealIndependentGoalSpecification(candidate workspace.DefinitionAdmissionCandidate) workspace.DefinitionAdmissionCandidate {
	hash := candidate.Specification.ContentHash()
	for index := range candidate.Specification.Reviews {
		candidate.Specification.Reviews[index].SpecificationHash = hash
		ids, err := candidate.Specification.RequiredReviewRequirementIDs(candidate.Specification.Reviews[index].Kind)
		if err != nil {
			panic(err)
		}
		candidate.Specification.Reviews[index].RequirementIDs = ids
	}
	return candidate.WithHash()
}

func independentGoalRequirement(candidate *workspace.DefinitionAdmissionCandidate) *workspace.GoalSpecificationRequirement {
	for index := range candidate.Specification.Requirements {
		if candidate.Specification.Requirements[index].ID == "independent-goal" {
			return &candidate.Specification.Requirements[index]
		}
	}
	panic("测试 fixture 缺独立 Goal requirement")
}

func TestCompileIndependentExactGoalDoesNotInventDoDObligation(t *testing.T) {
	candidate := independentGoalSpecificationCandidate()
	before := candidate.Clone()
	result, err := Compile(candidate)
	if err != nil || result.Disposition != workspace.DefinitionAdmissionAdmittedAllOf {
		t.Fatalf("独立 exact Goal 不应借用 DoD obligation: disposition=%s refs=%v err=%v", result.Disposition, result.OffendingRefs, err)
	}
	if !reflect.DeepEqual(candidate, before) || len(candidate.Specification.Obligations) != 1 || candidate.Specification.Obligations[0].RequirementID == "independent-goal" {
		t.Fatal("准入修改原规格或补造 Goal DoD obligation")
	}
	again, err := Compile(candidate)
	if err != nil || !reflect.DeepEqual(result, again) {
		t.Fatalf("exact Goal 重放不一致: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*workspace.DefinitionAdmissionCandidate)
	}{
		{"different_goal_text", func(c *workspace.DefinitionAdmissionCandidate) {
			independentGoalRequirement(c).Text = "改写后的目标"
		}},
		{"different_source_kind", func(c *workspace.DefinitionAdmissionCandidate) {
			independentGoalRequirement(c).SourceKind = workspace.GoalRequirementDerived
		}},
		{"mixed_goal_and_dod", func(c *workspace.DefinitionAdmissionCandidate) {
			independentGoalRequirement(c).SourceItemRefs = append(independentGoalRequirement(c).SourceItemRefs, c.DoDInputs[0].CandidateItemRef)
		}},
		{"duplicate_goal_source", func(c *workspace.DefinitionAdmissionCandidate) {
			extra := *independentGoalRequirement(c)
			extra.ID = "duplicate-goal"
			c.Specification.Requirements = append(c.Specification.Requirements, extra)
		}},
		{"missing_dod_obligation", func(c *workspace.DefinitionAdmissionCandidate) {
			c.Specification.Obligations = nil
			c.Specification.Verifications = nil
		}},
		{"unverified_dod", func(c *workspace.DefinitionAdmissionCandidate) {
			v := &c.Specification.Verifications[0]
			v.Status = workspace.GoalVerificationUnverified
			v.ProfileID, v.EvidenceRequirementID, v.Scope = "", "", ""
		}},
		{"unknown_review", func(c *workspace.DefinitionAdmissionCandidate) {
			c.Specification.Reviews[0].Status = workspace.GoalReviewUnknown
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := candidate.Clone()
			test.mutate(&invalid)
			invalid = resealIndependentGoalSpecification(invalid)
			result, err := Compile(invalid)
			if err != nil {
				t.Fatal(err)
			}
			if result.Disposition != workspace.DefinitionAdmissionClarificationRequired || result.ViolationCode != workspace.DefinitionAdmissionSpecificationIncomplete {
				t.Fatalf("非 exact Goal 或普通 DoD/review 缺口绕过准入: %s %v", result.Disposition, result.OffendingRefs)
			}
		})
	}
}
