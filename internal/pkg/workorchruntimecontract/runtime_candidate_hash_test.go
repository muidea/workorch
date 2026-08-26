package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestStableRuntimeCandidateAndOperationHashesIncludeLegacyCoverageWorkItemIDs(t *testing.T) {
	content := "sealed content"
	baseOperation := RuntimeOperation{
		ID:         "write-1",
		Kind:       StepKindFileWrite,
		TargetRefs: []string{"target.txt"},
		Parameters: []OperationParameter{{Name: "content", String: &content}},
	}
	withLegacyCoverage := baseOperation
	withLegacyCoverage.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-legacy"}
	if StableRuntimeOperationHash(baseOperation) == StableRuntimeOperationHash(withLegacyCoverage) {
		t.Fatal("operation hash omitted legacy coverage work item ids")
	}
	baseCandidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: baseOperation}}}}
	legacyCandidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: withLegacyCoverage}}}}
	if StableRuntimeCandidateHash(baseCandidate) == StableRuntimeCandidateHash(legacyCandidate) {
		t.Fatal("candidate hash omitted operation coverage work item ids")
	}
}

func TestStableRuntimeCandidateHashIncludesCompleteAssessment(t *testing.T) {
	base := RuntimeCandidate{Assessment: &AssessmentCandidate{
		FocusID:             "focus-1",
		SliceID:             "slice-1",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition:         AssessmentDispositionRepairableGap,
		Items: []RuntimeAssessmentItemFact{{
			CoverageWorkItemID: "cwi-1",
			Satisfied:          false,
			Reason:             "item reason",
			EvidenceRefs:       []string{"evidence://item"},
			ArtifactRefs:       []common.ArtifactID{"artifact-item"},
		}},
		Gap: &RuntimeAssessmentGap{
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			MissingEvidenceKinds: []string{EvidenceKindValidation},
			TargetRefs:           []string{"target.txt"},
			EvidenceRefs:         []string{"evidence://gap"},
			Reason:               "gap reason",
			FailureClass:         FailureClassExecutionFailure,
			FailureCode:          "gap-code",
			FailureSignature:     "gap-signature",
		},
		Summary:      "assessment summary",
		EvidenceRefs: []string{"evidence://assessment"},
		ArtifactRefs: []string{"artifact-assessment"},
	}}
	baseHash := StableRuntimeCandidateHash(base)
	tests := []struct {
		name   string
		mutate func(*AssessmentCandidate)
	}{
		{name: "slice id", mutate: func(value *AssessmentCandidate) { value.SliceID = "slice-2" }},
		{name: "item coverage", mutate: func(value *AssessmentCandidate) { value.Items[0].CoverageWorkItemID = "cwi-2" }},
		{name: "item satisfied", mutate: func(value *AssessmentCandidate) { value.Items[0].Satisfied = true }},
		{name: "item reason", mutate: func(value *AssessmentCandidate) { value.Items[0].Reason = "other item reason" }},
		{name: "item evidence refs", mutate: func(value *AssessmentCandidate) { value.Items[0].EvidenceRefs = []string{"evidence://other-item"} }},
		{name: "item artifact refs", mutate: func(value *AssessmentCandidate) {
			value.Items[0].ArtifactRefs = []common.ArtifactID{"artifact-other-item"}
		}},
		{name: "gap coverage", mutate: func(value *AssessmentCandidate) { value.Gap.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-2"} }},
		{name: "gap missing evidence", mutate: func(value *AssessmentCandidate) { value.Gap.MissingEvidenceKinds = []string{EvidenceKindStateChange} }},
		{name: "gap targets", mutate: func(value *AssessmentCandidate) { value.Gap.TargetRefs = []string{"other.txt"} }},
		{name: "gap evidence refs", mutate: func(value *AssessmentCandidate) { value.Gap.EvidenceRefs = []string{"evidence://other-gap"} }},
		{name: "gap reason", mutate: func(value *AssessmentCandidate) { value.Gap.Reason = "other gap reason" }},
		{name: "gap failure class", mutate: func(value *AssessmentCandidate) { value.Gap.FailureClass = FailureClassBuildFailure }},
		{name: "gap failure code", mutate: func(value *AssessmentCandidate) { value.Gap.FailureCode = "other-gap-code" }},
		{name: "gap failure signature", mutate: func(value *AssessmentCandidate) { value.Gap.FailureSignature = "other-gap-signature" }},
		{name: "gap presence", mutate: func(value *AssessmentCandidate) { value.Gap = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneRuntimeCandidate(base)
			test.mutate(candidate.Assessment)
			if got := StableRuntimeCandidateHash(candidate); got == baseHash {
				t.Fatalf("assessment mutation %q did not change candidate hash", test.name)
			}
		})
	}
}
