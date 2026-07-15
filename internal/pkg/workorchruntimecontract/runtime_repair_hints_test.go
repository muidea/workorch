package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDeriveRuntimeRepairHintsPreferValidation(t *testing.T) {
	facts := RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "ev-1",
		Focus: CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindValidation},
		},
		Evidence: EvidenceLedger{
			Accepted: []EvidenceFact{{
				ID: "e1", Kind: EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
				Revision: "evidence-v2-00000000000000000001",
			}},
			Rejected: []EvidenceFact{{
				ID: "e2", Kind: EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
				FailureSignature: "sig-1", FailureCode: "conflict", FailureMessage: "conflict: target mismatch",
				Revision: "evidence-v2-00000000000000000002",
			}},
		},
		Failure: &FailureFact{Class: FailureClassExecutionFailure, Code: "conflict", Signature: "sig-1", Message: "conflict: target mismatch"},
	}.Normalized()
	hints := DeriveRuntimeRepairHints(facts)
	codes := map[string]bool{}
	for _, hint := range hints {
		codes[hint.Code] = true
	}
	for _, code := range []string{"produce_missing_evidence", "prefer_validation_over_state_change", "reread_before_retry", "avoid_same_failed_shape"} {
		if !codes[code] {
			t.Fatalf("missing hint code %s in %#v", code, hints)
		}
	}
	// Domain-specific strings must not appear in core hints.
	for _, hint := range hints {
		for _, banned := range []string{"*_test.go", "text_replace", "file_write", "mcp/"} {
			if strings.Contains(hint.Message, banned) || strings.Contains(hint.Code, banned) {
				t.Fatalf("domain-specific hint leaked into core: %#v", hint)
			}
		}
	}
}
