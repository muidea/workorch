package orchestrator

import "testing"

func TestCoverageApplyWorkItemEvidence_RejectionUsesRequiredEvidenceCounts(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-decision-count",
		"status":                "satisfied",
		"requiredEvidenceKinds": []string{"decision"},
		"requiredEvidenceCounts": map[string]any{
			"decision": 2,
		},
		"acceptedEvidenceByKind": map[string]any{
			"decision": []string{"ticket-1"},
		},
		"acceptedEvidenceRefs": []string{"ticket-1"},
		"missingEvidenceKinds": []string{},
		"currentGap":           "",
	}

	changed := coverageApplyWorkItemEvidence(
		item,
		[]string{"decision"},
		[]string{"ticket-2"},
		false,
		false,
		"run-1/repair-001",
		"need second decision",
	)
	if !changed {
		t.Fatalf("changed = false, want true")
	}
	if got := stringFromMap(item, "status"); got != "in_progress" {
		t.Fatalf("status = %q, want in_progress", got)
	}
	missing := stringSliceFromValue(item["missingEvidenceKinds"])
	if len(missing) != 1 || missing[0] != "decision" {
		t.Fatalf("missingEvidenceKinds = %+v, want [decision]", missing)
	}
	if got := stringFromMap(item, "currentGap"); got != "need second decision" {
		t.Fatalf("currentGap = %q, want rejection reason", got)
	}
}
