package specifications

import (
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringReviewRequiresCompletedSealedDerivation(t *testing.T) {
	for _, request := range []workspace.DefinitionDerivationRequest{{}, {Proposal: &workspace.DefinitionDerivedAcceptance{}}} {
		if _, err := ParseAuthoringReview([]byte(`{}`), workspace.GoalReviewDimensionCoverage, workspace.GoalSpecification{}, request); err == nil {
			t.Fatal("review accepted missing sealed derivation")
		}
	}
}
