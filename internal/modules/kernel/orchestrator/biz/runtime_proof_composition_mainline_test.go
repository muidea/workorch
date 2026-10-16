package orchestrator

import (
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Real File/Shell/Run/Result owners; scripted reviewer and private startup.
// This proves the opt-in composition transaction, not public work availability.
func TestDriveRuntimeProofCompositionV3OwnerMainlineAndRestart(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectConjunction, ValidationCount: 1, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV3, SourceObligations: true, DeliveryGuard: true}
	baseline := runValidationInputReviewMainline(t, "consistent", "", options)
	for _, cut := range []string{"after_review_document", "after_review_binding", "after_project_receipt", "after_result_persistence", "after_workspace_persistence", "after_run_persistence"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "consistent", cut, options); calls != baseline {
				t.Fatalf("v3 recovery repeated provider consumption: %d != %d", calls, baseline)
			}
		})
	}
	for _, mode := range []string{"conflict", "omission", "clarify"} {
		t.Run(mode, func(t *testing.T) { runValidationInputReviewMainline(t, mode, "", options) })
	}
}
