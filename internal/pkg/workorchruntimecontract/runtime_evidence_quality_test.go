package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeOperationReceiptHashIncludesValidationQuality(t *testing.T) {
	exitCode := int64(0)
	base := RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/validate", Fence: 1, OperationID: "validate",
		Outcome: RuntimeOperationOutcomeAccepted, ObservedEffectScope: RuntimeEffectNone,
		Evidence: RuntimeOperationEvidence{Summary: "completed", ExitCode: &exitCode},
	}
	withQuality := base
	withQuality.Evidence.Quality = common.EvidenceQualityInconclusive
	withQuality.Evidence.ZeroWork = true
	withQuality.Evidence.EvidenceReason = "no checks matched"
	withQuality.Evidence.EvidenceTags = []string{"zero_work"}
	withQuality.Evidence.Validation = &RuntimeValidationObservation{OutputPreview: "no tests to run"}
	if StableRuntimeOperationReceiptHash(base) == StableRuntimeOperationReceiptHash(withQuality) {
		t.Fatal("receipt quality and validation observation must participate in the stable hash")
	}
}

func TestRuntimeOperationReceiptNormalizedDeepCopiesValidationObservation(t *testing.T) {
	receipt := RuntimeOperationReceipt{
		ExecutionKey: "exec/ws/run/dispatch/validate", Fence: 1, OperationID: "validate",
		Outcome: RuntimeOperationOutcomeAccepted, ObservedEffectScope: RuntimeEffectNone,
		Evidence: RuntimeOperationEvidence{
			EvidenceTags: []string{" validation ", "validation"},
			Validation: &RuntimeValidationObservation{
				ExecutedChecks: []string{" TestBehavior ", "TestBehavior"},
				PassedChecks:   []string{" TestBehavior "},
				FailedChecks:   []string{" TestFailure "},
				SkippedChecks:  []string{" TestSkipped "},
				OutputPreview:  " output ",
			},
		},
	}
	normalized := receipt.Normalized()
	if normalized.Evidence.Validation == nil || len(normalized.Evidence.Validation.ExecutedChecks) != 1 ||
		normalized.Evidence.Validation.ExecutedChecks[0] != "TestBehavior" || normalized.Evidence.Validation.OutputPreview != "output" {
		t.Fatalf("normalized validation observation = %+v", normalized.Evidence.Validation)
	}
	receipt.Evidence.EvidenceTags[0] = "mutated"
	receipt.Evidence.Validation.ExecutedChecks[0] = "mutated"
	receipt.Evidence.Validation.PassedChecks[0] = "mutated"
	if normalized.Evidence.EvidenceTags[0] != "validation" || normalized.Evidence.Validation.ExecutedChecks[0] != "TestBehavior" ||
		normalized.Evidence.Validation.PassedChecks[0] != "TestBehavior" {
		t.Fatalf("normalized receipt aliases source slices: %+v", normalized.Evidence)
	}
}

func TestInitialRuntimeCoverageFactsPreserveClaimIdentity(t *testing.T) {
	facts := InitialRuntimeCoverageFacts([]RuntimeCoverageItem{{
		ID: "cwi-1", SliceID: "slice-1", SourceScope: "dod_item", SourceID: "dod-1",
		TargetText: "request latency stays under 50ms",
	}})
	if len(facts) != 1 || facts[0].SourceScope != "dod_item" || facts[0].SourceID != "dod-1" ||
		facts[0].TargetText != "request latency stays under 50ms" {
		t.Fatalf("coverage claim identity was not preserved: %+v", facts)
	}
}

func TestRuntimeCandidateContextPreservesValidationQualityObservation(t *testing.T) {
	facts := RuntimeFacts{
		WorkspaceID: "ws", RunID: "run", DefinitionRevision: "definition", EvidenceRevision: "runtime-evidence-00000000000000000001",
		Focus:    CoverageFocus{ID: "focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}},
		Coverage: []RuntimeCoverageItemFact{{ID: "cwi", State: RuntimeCoverageItemStateOpen}},
		Evidence: EvidenceLedger{Accepted: []EvidenceFact{{
			ID: "evidence", Kind: EvidenceKindValidation, Quality: common.EvidenceQualityValid,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"},
			Validation:          &RuntimeValidationObservation{PassedChecks: []string{"TestBehavior"}, OutputPreview: "PASS"},
		}}},
	}
	context := RuntimeCandidateContextFromFacts(facts)
	if len(context.Evidence.Accepted) != 1 || context.Evidence.Accepted[0].Quality != common.EvidenceQualityValid ||
		context.Evidence.Accepted[0].Validation == nil || context.Evidence.Accepted[0].Validation.PassedChecks[0] != "TestBehavior" {
		t.Fatalf("candidate context lost validation quality: %+v", context.Evidence)
	}
	facts.Evidence.Accepted[0].Validation.PassedChecks[0] = "mutated"
	if context.Evidence.Accepted[0].Validation.PassedChecks[0] != "TestBehavior" {
		t.Fatalf("candidate context aliases source validation observation: %+v", context.Evidence.Accepted[0].Validation)
	}
}
