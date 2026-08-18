package events

import (
	"errors"
	"testing"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

func TestCloneResultAndArtifactIsolateEventHubBoundary(t *testing.T) {
	result := &resultmodel.Result{
		EvidenceRefs: []string{"evidence-1"},
		SummaryDetail: workorchresultcontract.ResultSummary{
			KnownLimitations: []string{"limitation-1"},
			Diagnostics:      workorcheventpayload.New(workorcheventpayload.StringField("source", "owner")),
		},
		DoDAssessment: workorchresultcontract.DoDAssessment{Items: []workorchresultcontract.DoDAssessmentItem{{EvidenceRefs: []string{"dod-evidence"}}}},
	}
	clonedResult := CloneResult(result)
	result.EvidenceRefs[0] = "mutated"
	result.SummaryDetail.KnownLimitations[0] = "mutated"
	result.DoDAssessment.Items[0].EvidenceRefs[0] = "mutated"
	if clonedResult.EvidenceRefs[0] != "evidence-1" || clonedResult.SummaryDetail.KnownLimitations[0] != "limitation-1" || clonedResult.DoDAssessment.Items[0].EvidenceRefs[0] != "dod-evidence" {
		t.Fatalf("result clone shares mutable values: %+v", clonedResult)
	}

	artifact := &resultmodel.Artifact{EvidenceRefs: []string{"artifact-evidence"}, Metadata: workorcheventpayload.New(workorcheventpayload.StringField("source", "owner"))}
	clonedArtifact := CloneArtifact(artifact)
	artifact.EvidenceRefs[0] = "mutated"
	if clonedArtifact.EvidenceRefs[0] != "artifact-evidence" {
		t.Fatalf("artifact clone shares evidence refs: %+v", clonedArtifact)
	}
}

func TestNormalizeListWindowIsBounded(t *testing.T) {
	cursor, limit, err := NormalizeListWindow(" cursor ", 0)
	if err != nil || cursor != "cursor" || limit != DefaultListLimit {
		t.Fatalf("normalized window = %q/%d err=%v", cursor, limit, err)
	}
	if _, _, err := NormalizeListWindow("", MaxListLimit+1); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("oversized limit err = %v", err)
	}
}
