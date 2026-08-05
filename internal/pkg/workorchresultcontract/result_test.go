package workorchresultcontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestResultSummaryCloneDetachesNestedValues(t *testing.T) {
	original := ResultSummary{
		DoDItems:     []DoDAssessmentItem{{EvidenceRefs: []string{"evidence-1"}}},
		ArtifactRefs: []common.ArtifactID{"artifact-1"},
		Diagnostics:  workorcheventpayload.New(workorcheventpayload.StringArrayField("refs", []string{"ref-1"})),
	}
	cloned := original.Clone()
	original.DoDItems[0].EvidenceRefs[0] = "changed"
	original.ArtifactRefs[0] = "changed"
	original.Diagnostics.Fields[0].Value.Array[0].String = "changed"
	if cloned.DoDItems[0].EvidenceRefs[0] != "evidence-1" || cloned.ArtifactRefs[0] != "artifact-1" {
		t.Fatalf("clone shares result slices: %+v", cloned)
	}
	refs, ok := cloned.Diagnostics.StringSlice("refs")
	if !ok || len(refs) != 1 || refs[0] != "ref-1" {
		t.Fatalf("clone shares diagnostic payload: %+v", cloned.Diagnostics)
	}
}
