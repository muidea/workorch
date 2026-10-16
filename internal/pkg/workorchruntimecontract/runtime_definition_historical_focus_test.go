package workorchruntimecontract

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"testing"
)

func TestDefinitionHistoricalFocusRetainsSourceChecksWithoutInventedLocalValidation(t *testing.T) {
	_, _, seed := purposeEndpointFixture(t)
	focuses := make([]RuntimeDefinitionFocus, 0, len(seed.CoverageItems))
	for _, item := range seed.CoverageItems {
		focus := RuntimeDefinitionFocus{
			ID: "focus/" + string(item.ID), SliceID: item.SliceID,
			AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
			CoverageWorkItemIDs:    []common.CoverageWorkItemID{item.ID},
			EvidenceDeclarationRef: CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID),
			RequiredEvidenceKinds:  []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession},
		}
		if item.HistoricalAcceptance != nil {
			focus.RequiredEvidenceKinds, focus.AcceptedStepKinds = []string{EvidenceKindStateChange}, []string{StepKindFileWrite}
		}
		focuses = append(focuses, focus)
	}
	if err := ValidateRuntimeDefinitionFocuses(seed, focuses); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"unmapped", "consumer-no-validation", "consumer-no-owner-operation", "changed-source", "missing-inventory"} {
		t.Run(name, func(t *testing.T) {
			changed, declarations := seed.Clone(), CloneRuntimeDefinitionFocuses(focuses)
			switch name {
			case "unmapped":
				changed.CoverageItems[0].HistoricalAcceptance = nil
			case "consumer-no-validation":
				declarations[1].RequiredEvidenceKinds = []string{EvidenceKindStateChange}
			case "consumer-no-owner-operation":
				declarations[1].AcceptedStepKinds = []string{StepKindFileWrite}
			case "changed-source":
				changed.CoverageItems[0].HistoricalAcceptance.AcceptanceChecksHash = "self-reported"
			case "missing-inventory":
				changed.SourceObligations = nil
			}
			changed.Hash = StableRuntimeDefinitionSeedHash(changed)
			if ValidateRuntimeDefinitionFocuses(changed, declarations) == nil {
				t.Fatal("source-only exemption bypassed original mapping or ordinary consumer")
			}
		})
	}
}
