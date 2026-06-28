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

func TestCoverageApplyWorkItemEvidence_TargetUnalignedValidationIsSupportingOnly(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-target-proof",
		"status":                "open",
		"sourceRequirementId":   "requirement-alpha",
		"targetText":            "requirement alpha must be proven",
		"requiredEvidenceKinds": []string{"validation"},
		"requiredEvidenceCounts": map[string]any{
			"validation": 1,
		},
		"missingEvidenceKinds": []string{"validation"},
	}

	changed := coverageApplyWorkItemEvidenceWithOptions(
		item,
		[]string{"validation"},
		[]string{"run-1/validation-generic"},
		true,
		false,
		"run-1/step-001",
		"",
		coverageEvidenceAcceptanceOptions{
			EnforceProofStrength: true,
			Quality:              runtimeEvidenceQuality{Quality: "valid"},
			TargetRefs:           []string{"unrelated"},
			ObservedClaims:       []string{"generic command passed"},
			Tags:                 []string{"generic_test_fallback"},
		},
	)
	if !changed {
		t.Fatalf("changed = false, want supporting evidence recorded")
	}
	if got := stringFromMap(item, "status"); got != "open" {
		t.Fatalf("status = %q, want open", got)
	}
	if missing := stringSliceFromValue(item["missingEvidenceKinds"]); len(missing) != 1 || missing[0] != "validation" {
		t.Fatalf("missingEvidenceKinds = %+v, want [validation]", missing)
	}
	if accepted := mapStringSliceValue(item["acceptedEvidenceByKind"]); len(accepted["validation"]) != 0 {
		t.Fatalf("acceptedEvidenceByKind = %+v, want no accepted validation", accepted)
	}
	supporting := mapStringSliceValue(item["supportingEvidenceByKind"])
	if got := supporting["validation"]; len(got) != 1 || got[0] != "run-1/validation-generic" {
		t.Fatalf("supportingEvidenceByKind = %+v, want supporting validation ref", supporting)
	}
}

func TestCoverageApplyWorkItemEvidence_TextOnlyTargetRefIsSupportingOnly(t *testing.T) {
	textTarget := "deliverable must expose runtime health behavior"
	item := map[string]any{
		"id":                    "cwi-text-only",
		"status":                "open",
		"targetText":            textTarget,
		"requiredEvidenceKinds": []string{"validation"},
		"requiredEvidenceCounts": map[string]any{
			"validation": 1,
		},
		"missingEvidenceKinds": []string{"validation"},
	}

	changed := coverageApplyWorkItemEvidenceWithOptions(
		item,
		[]string{"validation"},
		[]string{"run-1/validation-text-only"},
		true,
		false,
		"run-1/step-001",
		"",
		coverageEvidenceAcceptanceOptions{
			EnforceProofStrength: true,
			Quality:              runtimeEvidenceQuality{Quality: "valid"},
			TargetRefs:           []string{textTarget},
			ObservedClaims:       []string{"behavior was observed"},
			ProofKind:            "runtime_behavior",
		},
	)
	if !changed {
		t.Fatalf("changed = false, want supporting evidence recorded")
	}
	if got := stringFromMap(item, "status"); got != "open" {
		t.Fatalf("status = %q, want open", got)
	}
	if accepted := mapStringSliceValue(item["acceptedEvidenceByKind"]); len(accepted["validation"]) != 0 {
		t.Fatalf("accepted validation refs = %+v, want none for text-only targetRef", accepted["validation"])
	}
	if supporting := mapStringSliceValue(item["supportingEvidenceByKind"]); !stringSliceContains(supporting["validation"], "run-1/validation-text-only") {
		t.Fatalf("supporting validation refs = %+v, want text-only evidence demoted to supporting", supporting["validation"])
	}
}

func TestCoverageApplyWorkItemEvidence_TargetAlignedValidationCanSatisfyRequiredKind(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-target-proof",
		"status":                "open",
		"sourceRequirementId":   "requirement-alpha",
		"targetText":            "requirement alpha must be proven",
		"requiredEvidenceKinds": []string{"validation"},
		"requiredEvidenceCounts": map[string]any{
			"validation": 1,
		},
		"missingEvidenceKinds": []string{"validation"},
	}

	changed := coverageApplyWorkItemEvidenceWithOptions(
		item,
		[]string{"validation"},
		[]string{"run-1/validation-targeted"},
		true,
		false,
		"run-1/step-001",
		"",
		coverageEvidenceAcceptanceOptions{
			EnforceProofStrength: true,
			Quality:              runtimeEvidenceQuality{Quality: "valid"},
			TargetRefs:           []string{"requirement-alpha"},
			ObservedClaims:       []string{"requirement alpha observed at runtime"},
			ProofKind:            "runtime_behavior",
		},
	)
	if !changed {
		t.Fatalf("changed = false, want accepted validation")
	}
	if got := stringFromMap(item, "status"); got != "evidence_ready" {
		t.Fatalf("status = %q, want evidence_ready", got)
	}
	if missing := stringSliceFromValue(item["missingEvidenceKinds"]); len(missing) != 0 {
		t.Fatalf("missingEvidenceKinds = %+v, want none", missing)
	}
	accepted := mapStringSliceValue(item["acceptedEvidenceByKind"])
	if got := accepted["validation"]; len(got) != 1 || got[0] != "run-1/validation-targeted" {
		t.Fatalf("acceptedEvidenceByKind = %+v, want accepted validation ref", accepted)
	}
}

func TestEvidenceKindRequirementEvidenceKindsMixedAcceptedKindsDoNotRequireStateChange(t *testing.T) {
	requirement := profileEvidenceKindRequirement{
		Scope:         "dod_item",
		ID:            "*",
		RequireDirect: true,
		AcceptedStepKinds: []string{
			"shell/command",
			"patch/apply",
			"mcp/tool_call",
		},
	}

	if got := evidenceKindRequirementEvidenceKinds(requirement); len(got) != 1 || got[0] != "validation" {
		t.Fatalf("evidence kinds = %+v, want validation only for mixed accepted step kinds", got)
	}

	requirement.AcceptedStepKinds = []string{"patch/apply"}
	if got := evidenceKindRequirementEvidenceKinds(requirement); !stringSliceContains(got, "validation") || !stringSliceContains(got, "state_change") {
		t.Fatalf("evidence kinds = %+v, want validation and state_change for state-changing-only requirement", got)
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
