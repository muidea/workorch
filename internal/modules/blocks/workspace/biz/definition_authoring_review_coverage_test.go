package workspace

import (
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func authoringRequiredRequirementIDs(t *testing.T, proposal workspace.GoalSpecification, kind string) []string {
	t.Helper()
	ids, err := proposal.RequiredReviewRequirementIDs(kind)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}
