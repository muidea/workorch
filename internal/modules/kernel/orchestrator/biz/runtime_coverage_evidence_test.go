package orchestrator

import "testing"

func TestCoverageApplyWorkItemEvidence_AcceptedEvidenceSatisfiesRequiredKind(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-accepted-evidence",
		"status":                "open",
		"requiredEvidenceKinds": []string{"validation"},
		"requiredEvidenceCounts": map[string]any{
			"validation": 1,
		},
		"missingEvidenceKinds": []string{"validation"},
		"currentGap":           "missing validation evidence",
	}

	changed := coverageApplyWorkItemEvidence(
		item,
		[]string{"validation"},
		[]string{"run-1/validation-001"},
		true,
		false,
		"run-1/step-001",
		"",
	)
	if !changed {
		t.Fatalf("changed = false, want true")
	}
	if got := stringFromMap(item, "status"); got != "evidence_ready" {
		t.Fatalf("status = %q, want evidence_ready", got)
	}
	if missing := stringSliceFromValue(item["missingEvidenceKinds"]); len(missing) != 0 {
		t.Fatalf("missingEvidenceKinds = %+v, want none", missing)
	}
	accepted := mapStringSliceValue(item["acceptedEvidenceByKind"])
	if got := accepted["validation"]; len(got) != 1 || got[0] != "run-1/validation-001" {
		t.Fatalf("acceptedEvidenceByKind = %+v, want accepted validation ref", accepted)
	}
}

func TestCoverageApplyWorkItemEvidence_RejectedEvidenceDoesNotSatisfyRequiredKind(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-rejected-evidence",
		"status":                "open",
		"requiredEvidenceKinds": []string{"validation"},
		"requiredEvidenceCounts": map[string]any{
			"validation": 1,
		},
		"missingEvidenceKinds": []string{"validation"},
		"currentGap":           "missing validation evidence",
	}

	changed := coverageApplyWorkItemEvidence(
		item,
		[]string{"validation"},
		[]string{"run-1/validation-rejected"},
		false,
		false,
		"run-1/step-001",
		"validation evidence rejected",
	)
	if !changed {
		t.Fatalf("changed = false, want true")
	}
	if got := stringFromMap(item, "status"); got != "failed" {
		t.Fatalf("status = %q, want failed", got)
	}
	missing := stringSliceFromValue(item["missingEvidenceKinds"])
	if len(missing) != 1 || missing[0] != "validation" {
		t.Fatalf("missingEvidenceKinds = %+v, want [validation]", missing)
	}
	if accepted := mapStringSliceValue(item["acceptedEvidenceByKind"]); len(accepted["validation"]) != 0 {
		t.Fatalf("acceptedEvidenceByKind = %+v, want no accepted validation refs", accepted)
	}
	rejected := mapStringSliceValue(item["rejectedEvidenceByKind"])
	if got := rejected["validation"]; len(got) != 1 || got[0] != "run-1/validation-rejected" {
		t.Fatalf("rejectedEvidenceByKind = %+v, want rejected validation ref", rejected)
	}
	if got := stringFromMap(item, "currentGap"); got != "validation evidence rejected" {
		t.Fatalf("currentGap = %q, want rejection reason", got)
	}
}

func TestCoverageMissingEvidenceKindsAreIndependentByEvidenceKind(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-independent-kinds",
		"status":                "open",
		"requiredEvidenceKinds": []string{"state_change", "validation", "assessment"},
		"requiredEvidenceCounts": map[string]any{
			"state_change": 1,
			"validation":   1,
			"assessment":   1,
		},
		"missingEvidenceKinds": []string{"state_change", "validation", "assessment"},
	}

	if !coverageApplyWorkItemEvidence(item, []string{"state_change"}, []string{"run-1/state-change-001"}, true, false, "run-1/step-001", "") {
		t.Fatalf("state_change update changed = false, want true")
	}
	if missing := stringSet(stringSliceFromValue(item["missingEvidenceKinds"])); missing["state_change"] || !missing["validation"] || !missing["assessment"] {
		t.Fatalf("missing after state_change = %+v, want validation and assessment only", item["missingEvidenceKinds"])
	}

	if !coverageApplyWorkItemEvidence(item, []string{"validation"}, []string{"run-1/validation-001"}, true, false, "run-1/step-002", "") {
		t.Fatalf("validation update changed = false, want true")
	}
	if missing := stringSet(stringSliceFromValue(item["missingEvidenceKinds"])); missing["state_change"] || missing["validation"] || !missing["assessment"] {
		t.Fatalf("missing after validation = %+v, want assessment only", item["missingEvidenceKinds"])
	}

	if !coverageApplyWorkItemEvidence(item, []string{"assessment"}, []string{"run-1/assessment-001"}, true, false, "run-1/step-003", "") {
		t.Fatalf("assessment update changed = false, want true")
	}
	if missing := stringSliceFromValue(item["missingEvidenceKinds"]); len(missing) != 0 {
		t.Fatalf("missing after assessment = %+v, want none", missing)
	}
	if got := stringFromMap(item, "status"); got != "evidence_ready" {
		t.Fatalf("status = %q, want evidence_ready", got)
	}
}

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
