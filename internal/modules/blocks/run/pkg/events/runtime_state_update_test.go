package events

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeStateUpdateResultUsesFeedbackAsSingleOutcome(t *testing.T) {
	result := RuntimeStateUpdateResult{
		WorkspaceID: "ws-v2", RunID: "run-v2",
		ExpectedRuntimeRevision: 7, ObservedRuntimeRevision: 8,
		Feedback: workorchruntimecontract.RuntimeFeedback{
			Domain: workorchruntimecontract.RuntimeFeedbackDomainPersistence, StableCategory: workorchruntimecontract.RuntimeFeedbackCategoryAggregateRevisionMismatch,
			WorkspaceID: "ws-v2", RunID: "run-v2",
			OriginalDiagnostic: &workorchruntimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion: workorchruntimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: "blocks/run",
				OriginalClass: "aggregate_revision_mismatch", OriginalCode: "aggregate_revision_mismatch",
				OriginalMessage: "aggregate revision changed", Signature: "runtime-state-update/ws-v2/run-v2/7/8/aggregate_revision_mismatch",
				ExpectedRevision: "7", ObservedRevision: "8",
			},
		},
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("valid persistence feedback rejected: %v", err)
	}
	result.Feedback.StableCategory = "unmapped_persistence_outcome"
	if err := result.Validate(); err == nil {
		t.Fatal("unknown persistence feedback outcome accepted")
	}
}
