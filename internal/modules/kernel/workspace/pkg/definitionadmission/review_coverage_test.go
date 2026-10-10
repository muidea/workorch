package definitionadmission

import (
	"strings"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestCompileRejectsEachCriticMissingRequiredGoal(t *testing.T) {
	candidate := independentGoalSpecificationCandidate()
	for _, kind := range []string{workspace.GoalReviewDimensionCoverage, workspace.GoalReviewNegativePath, workspace.GoalReviewRegression, workspace.GoalReviewResourceEnvironment, workspace.GoalReviewTraceability, workspace.GoalReviewVerifierFeasibility} {
		t.Run(kind, func(t *testing.T) {
			incomplete := candidate.Clone()
			for index := range incomplete.Specification.Reviews {
				if incomplete.Specification.Reviews[index].Kind == kind {
					incomplete.Specification.Reviews[index].RequirementIDs = []string{"requirement"}
				}
			}
			incomplete = incomplete.WithHash()
			result, err := Compile(incomplete)
			if err == nil || !strings.Contains(err.Error(), kind) || !strings.Contains(err.Error(), "independent-goal") || result.Disposition == workspace.DefinitionAdmissionAdmittedAllOf {
				t.Fatalf("六 PASS 漏审获得准入: %+v %v", result, err)
			}
		})
	}
}
