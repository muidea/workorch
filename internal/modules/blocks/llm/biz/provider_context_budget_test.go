package llm

import (
	"fmt"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestEnforceProviderRequestBudgetCompactsLargeExecutionContext(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-budget",
		RunID:       "run-budget",
		StepID:      "run-budget/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"executionContext": map[string]any{
				"goal": map[string]any{"text": "finish the workspace goal"},
				"dod":  map[string]any{"items": []any{"evidence is available"}},
				"soulRouting": map[string]any{
					"bindings": map[string]any{
						"execution": map[string]any{
							"soul": map[string]any{
								"id":          "soul-large",
								"name":        "large soul",
								"description": "large execution soul",
								"soulMd":      strings.Repeat("long guidance ", 5000),
							},
						},
					},
				},
			},
			"workspaceFileManifest": repeatedBudgetItems(200),
			"graphEvidence":         repeatedBudgetItems(200),
		},
		RuntimeCapabilities: declaredRuntimeCapabilities(),
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         1024,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         12 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudgetLimit(req, 12*1024)
	if got.ContextBudget == nil || !got.ContextBudget.Truncated {
		t.Fatalf("contextBudget = %+v, want truncated metadata", got.ContextBudget)
	}
	if got.ContextBudget.ActualBytes > got.ContextBudget.MaxBytes {
		t.Fatalf("actual bytes = %d, max = %d", got.ContextBudget.ActualBytes, got.ContextBudget.MaxBytes)
	}
	if got.ContextBudget.Estimator != "bytes_ratio" ||
		got.ContextBudget.MaxTokens != 1024 ||
		got.ContextBudget.EstimatedInputTokens <= 0 ||
		len(got.ContextBudget.SourceRefs) == 0 {
		t.Fatalf("contextBudget = %+v, want token metadata", got.ContextBudget)
	}
	if got.ContextBudget.GoalDoDCompressed {
		t.Fatalf("contextBudget = %+v, want goal/dod protected from compression", got.ContextBudget)
	}
	executionContext := recordFromAny(got.Input["executionContext"])
	if len(recordFromAny(executionContext["goal"])) == 0 || len(recordFromAny(executionContext["dod"])) == 0 {
		t.Fatalf("executionContext = %+v input=%+v, want goal and DoD preserved", executionContext, got.Input)
	}
	payload := userPrompt(got)
	if strings.Contains(payload, strings.Repeat("long guidance ", 100)) {
		t.Fatal("prompt still contains unbounded soul markdown")
	}
	if !strings.Contains(payload, "contextBudget") {
		t.Fatal("prompt missing contextBudget metadata")
	}
}

func TestEnforceProviderRequestBudgetPreservesEvidenceLedgerRefs(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-evidence-budget",
		RunID:       "run-evidence-budget",
		StepID:      "run-evidence-budget/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"goal": map[string]any{"text": "finish goal"},
			"dod":  map[string]any{"items": []any{"validated evidence"}},
			"evidenceLedger": map[string]any{
				"schemaVersion": "workorch.evidence_ledger_context.v1",
				"items": []any{
					map[string]any{
						"evidenceRef":     "evidence://validation-001",
						"kind":            "validation",
						"status":          "accepted",
						"targetRefs":      []any{"target://primary"},
						"observedClaims":  []any{"primary behavior observed"},
						"evidenceQuality": "valid",
						"payloadRef":      "workorch://payload/validation-001",
					},
				},
			},
			"graphEvidence": repeatedBudgetItems(400),
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         128,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         4 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudgetLimit(req, 4*1024)

	ledger := recordFromAny(got.Input["evidenceLedger"])
	if len(ledger) == 0 {
		t.Fatalf("input = %+v, want evidenceLedger preserved under budget", got.Input)
	}
	encoded := userPrompt(got)
	if !strings.Contains(encoded, "evidence://validation-001") ||
		!strings.Contains(encoded, "target://primary") ||
		!strings.Contains(encoded, "primary behavior observed") {
		t.Fatalf("prompt missing compact evidence ledger refs: %s", encoded)
	}
	if got.ContextBudget == nil || !stringSliceContains(got.ContextBudget.ProtectedFields, "input.evidenceLedger") {
		t.Fatalf("contextBudget = %+v, want evidenceLedger protected field", got.ContextBudget)
	}
}

func TestEnforceProviderRequestBudgetFailsWhenCoverageWorkItemFocusMissing(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-cwi-budget",
		RunID:       "run-cwi-budget",
		StepID:      "run-cwi-budget/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"goal": map[string]any{"text": "goal"},
			"dod":  map[string]any{"items": []any{"dod"}},
			"coverageWorkItemContext": map[string]any{
				"summary": map[string]any{
					"total": 1,
					"open":  1,
				},
				"unresolvedItems": []map[string]any{{
					"id":                   "cwi-001",
					"targetText":           "goal",
					"missingEvidenceKinds": []string{"validation"},
				}},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         4096,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         64 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)

	if got.ContextBudget == nil || got.ContextBudget.ContextGateAction != providerContextGateDegradeToAssessment {
		t.Fatalf("contextBudget = %+v, want degrade_to_assessment", got.ContextBudget)
	}
	if got.ContextBudget.ContextGateReason != "coverage_work_item_unresolved_without_current_focus" {
		t.Fatalf("contextGateReason = %q, want coverage focus warning", got.ContextBudget.ContextGateReason)
	}
}

func TestProviderContextBudgetAllowsAssessmentOnlyRepairPackageWithoutRequiredOutput(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-assessment-budget",
		RunID:       "run-assessment-budget",
		StepID:      "run-assessment-budget/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"goal":                  map[string]any{"text": "goal"},
			"dod":                   map[string]any{"items": []any{"dod"}},
			"runtimeRepairFollowUp": true,
			"runtimeStage":          "verification",
			"assessmentOnly":        true,
			"repairObjective":       "complete_dod_goal_coverage_assessment",
			"convergenceContract": map[string]any{
				"state":           "need_assessment",
				"runtimeStage":    "verification",
				"repairObjective": "complete_dod_goal_coverage_assessment",
			},
			"coverageWorkItemContext": map[string]any{
				"summary": map[string]any{"total": 1, "open": 1},
				"currentFocus": map[string]any{
					"id":                           "cwi-001",
					"definitionRevision":           "rev-current",
					"sourceScope":                  "goal_leaf",
					"sourceRequirementId":          "goal-leaf-001",
					"targetText":                   "goal leaf",
					"status":                       "evidence_ready",
					"currentGap":                   "required evidence kinds present; coverage assessment required",
					"requiredEvidenceKinds":        []string{"validation"},
					"acceptedEvidenceRefs":         []string{"run-001/verify"},
					"rejectedEvidenceRefs":         []string{},
					"nextAllowedConvergenceStates": []string{"need_assessment"},
				},
				"currentFocusItems": []map[string]any{{
					"id":                           "cwi-001",
					"definitionRevision":           "rev-current",
					"sourceScope":                  "goal_leaf",
					"sourceRequirementId":          "goal-leaf-001",
					"targetText":                   "goal leaf",
					"status":                       "evidence_ready",
					"currentGap":                   "required evidence kinds present; coverage assessment required",
					"requiredEvidenceKinds":        []string{"validation"},
					"acceptedEvidenceRefs":         []string{"run-001/verify"},
					"rejectedEvidenceRefs":         []string{},
					"nextAllowedConvergenceStates": []string{"need_assessment"},
				}},
			},
			"repairActionPackage": map[string]any{
				"currentFailure": map[string]any{
					"failedStepId": "run-assessment-budget/graph-001/validate",
					"reason":       "validation action was rejected by runtime policy",
				},
				"coverageTargetSet": []map[string]any{{
					"sourceRequirementId": "goal-leaf-001",
					"targetText":          "goal leaf",
				}},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         4096,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         64 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)

	if got.ContextBudget == nil {
		t.Fatalf("contextBudget = nil, want metadata")
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "repair_action_package_required_output_missing") {
		t.Fatalf("integrityWarnings = %+v, want assessment-only package without requiredOutput accepted", got.ContextBudget.IntegrityWarnings)
	}
	if got.ContextBudget.ContextGateReason == "repair_action_package_required_output_missing" {
		t.Fatalf("contextGateReason = %q, want no requiredOutput gate for assessment-only package", got.ContextBudget.ContextGateReason)
	}
}

func TestProviderContextBudgetRequiresExecutableRepairPackageRequiredOutput(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-executable-budget",
		RunID:       "run-executable-budget",
		StepID:      "run-executable-budget/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"goal":                        map[string]any{"text": "goal"},
			"dod":                         map[string]any{"items": []any{"dod"}},
			"runtimeRepairFollowUp":       true,
			"runtimeStage":                "repair",
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"currentFailure": map[string]any{
					"failedStepId": "run-executable-budget/graph-001/action",
					"reason":       "state-changing output is missing",
				},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         4096,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         64 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)

	if got.ContextBudget == nil || got.ContextBudget.ContextGateAction != providerContextGateFailFast {
		t.Fatalf("contextBudget = %+v, want fail_fast", got.ContextBudget)
	}
	if got.ContextBudget.ContextGateReason != "repair_action_package_required_output_missing" {
		t.Fatalf("contextGateReason = %q, want repair_action_package_required_output_missing", got.ContextBudget.ContextGateReason)
	}
}

func TestProviderContextBudgetAllowsGovernancePackageWithoutRequiredOutput(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-governance-budget",
		RunID:       "run-governance-budget",
		StepID:      "run-governance-budget/decision-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"goal":                  map[string]any{"text": "goal"},
			"dod":                   map[string]any{"items": []any{"dod"}},
			"runtimeRepairFollowUp": true,
			"runtimeStage":          "decision",
			"runtimeStageMode":      "structured_governance",
			"convergenceContract": map[string]any{
				"state":            "need_governance",
				"runtimeStage":     "decision",
				"runtimeStageMode": "structured_governance",
			},
			"repairActionPackage": map[string]any{
				"currentFailure": map[string]any{
					"failedStepId": "run-governance-budget/graph-001/action",
					"reason":       "operator decision required",
				},
				"coverageTargetSet": []map[string]any{{
					"sourceRequirementId": "goal-leaf-001",
					"targetText":          "goal leaf",
				}},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         4096,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         64 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)

	if got.ContextBudget == nil {
		t.Fatalf("contextBudget = nil, want metadata")
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "repair_action_package_required_output_missing") {
		t.Fatalf("integrityWarnings = %+v, want no requiredOutput warning for governance", got.ContextBudget.IntegrityWarnings)
	}
	if got.ContextBudget.ContextGateReason == "repair_action_package_required_output_missing" {
		t.Fatalf("contextGateReason = %q, want governance package without executable output allowed", got.ContextBudget.ContextGateReason)
	}
}

func TestProviderCoverageWorkItemWarningsAcceptCurrentFocusItems(t *testing.T) {
	input := map[string]any{
		"coverageWorkItemContext": map[string]any{
			"summary": map[string]any{
				"total": 1,
				"open":  1,
			},
			"currentFocusItems": []map[string]any{{
				"id":                           "cwi-001",
				"definitionRevision":           "rev-current",
				"sourceScope":                  "goal_leaf",
				"sourceRequirementId":          "goal-leaf-001",
				"targetText":                   "goal leaf",
				"status":                       "open",
				"currentGap":                   "missing accepted evidence binding for required evidence kind: validation",
				"requiredEvidenceKinds":        []string{"validation"},
				"missingEvidenceKinds":         []string{"validation"},
				"acceptedEvidenceRefs":         []string{},
				"rejectedEvidenceRefs":         []string{"run-001/001-verify"},
				"nextAllowedConvergenceStates": []string{"need_validation"},
			}},
		},
	}

	if warnings := providerCoverageWorkItemWarnings(input); len(warnings) != 0 {
		t.Fatalf("warnings = %+v, want none for complete currentFocusItems projection", warnings)
	}
}

func TestProtectedCoverageWorkItemContextUsesTopLevelAuthority(t *testing.T) {
	topLevel := providerBudgetCoverageWorkItemContextForTest("cwi-top", "top-level focus")
	protected := captureProtectedProviderCoverageWorkItemContext(map[string]any{
		"coverageWorkItemContext": topLevel,
		"executionContextPackage": map[string]any{
			"coverageWorkItemContext": providerBudgetCoverageWorkItemContextForTest("cwi-package", "package mirror"),
		},
		"repairActionPackage": map[string]any{
			"coverageWorkItemContext": providerBudgetCoverageWorkItemContextForTest("cwi-repair", "repair mirror"),
		},
	}, 0)
	input := map[string]any{
		"coverageWorkItemContext": compactProviderCoverageWorkItemContext(topLevel),
		"executionContextPackage": map[string]any{
			"coverageWorkItemContext": providerBudgetCoverageWorkItemContextForTest("cwi-package-compacted", "compacted package mirror"),
		},
		"repairActionPackage": map[string]any{
			"coverageWorkItemContext": providerBudgetCoverageWorkItemContextForTest("cwi-repair-compacted", "compacted repair mirror"),
		},
	}

	if !isProtectedProviderCoverageWorkItemContextEqual(input, protected) {
		t.Fatalf("protected coverage context should use preserved top-level authority despite nested mirror drift")
	}
}

func TestProtectedCoverageWorkItemContextFallsBackToNestedMirrorsWithoutTopLevel(t *testing.T) {
	protected := captureProtectedProviderCoverageWorkItemContext(map[string]any{
		"executionContextPackage": map[string]any{
			"coverageWorkItemContext": providerBudgetCoverageWorkItemContextForTest("cwi-package", "package mirror"),
		},
	}, 0)
	input := map[string]any{
		"executionContextPackage": map[string]any{
			"coverageWorkItemContext": providerBudgetCoverageWorkItemContextForTest("cwi-package-drift", "changed package mirror"),
		},
	}

	if isProtectedProviderCoverageWorkItemContextEqual(input, protected) {
		t.Fatalf("protected coverage context should compare nested mirrors when no top-level authority exists")
	}
}

func TestProviderContextGateDecisionClassifiesRecoveryActions(t *testing.T) {
	tests := []struct {
		name    string
		warning string
		action  string
	}{
		{
			name:    "read required for missing protected snapshot",
			warning: "protected_repair_action_target_snapshot_missing_after_context_budget",
			action:  providerContextGateReadRequired,
		},
		{
			name:    "degrade to assessment for missing coverage focus",
			warning: "coverage_work_item_unresolved_without_current_focus",
			action:  providerContextGateDegradeToAssessment,
		},
		{
			name:    "blocker required for missing current failure",
			warning: "runtime_repair_current_failure_missing",
			action:  providerContextGateBlockerRequired,
		},
		{
			name:    "replan required for missing coverage target set",
			warning: "coverage_repair_required_without_coverage_target_set",
			action:  providerContextGateReplanRequired,
		},
		{
			name:    "fail fast for inadmissible repair target",
			warning: "inadmissible_repair_target_set",
			action:  providerContextGateFailFast,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, reason := providerContextGateDecision(false, false, false, []string{tt.warning})
			if action != tt.action || reason != tt.warning {
				t.Fatalf("providerContextGateDecision() = %q/%q, want %q/%q", action, reason, tt.action, tt.warning)
			}
		})
	}
}

func TestProviderBudgetCompactionPreservesCoverageWorkItemFocusFields(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-cwi-budget-compact",
		RunID:       "run-cwi-budget-compact",
		StepID:      "run-cwi-budget-compact/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"goal": map[string]any{"text": "goal"},
			"dod":  map[string]any{"items": []any{"dod"}},
			"coverageWorkItemContext": map[string]any{
				"summary": map[string]any{"total": 1, "open": 1},
				"currentFocus": map[string]any{
					"acceptedEvidenceByKind":       map[string]any{},
					"acceptedEvidenceRefs":         []string{},
					"coverageBoundary":             "must stay within source targetText",
					"currentGap":                   "missing accepted evidence binding for required evidence kind: validation",
					"definitionRevision":           "rev-current",
					"id":                           "cwi-001",
					"missingEvidenceKinds":         []string{"validation"},
					"nextAllowedConvergenceStates": []string{"need_validation"},
					"normalizedSourceKey":          "goal:goal_leaf:goal-leaf-001",
					"rejectedEvidenceByKind":       map[string]any{},
					"rejectedEvidenceRefs":         []string{"run-001/001-verify"},
					"requiredEvidenceKinds":        []string{"validation"},
					"source":                       "goal",
					"sourcePath":                   "goal.leaves[0]",
					"sourceRequirementId":          "goal-leaf-001",
					"sourceScope":                  "goal_leaf",
					"status":                       "open",
					"targetText":                   "goal leaf",
				},
				"currentFocusItems": []map[string]any{{
					"acceptedEvidenceByKind":       map[string]any{},
					"acceptedEvidenceRefs":         []string{},
					"coverageBoundary":             "must stay within source targetText",
					"currentGap":                   "missing accepted evidence binding for required evidence kind: validation",
					"definitionRevision":           "rev-current",
					"id":                           "cwi-001",
					"missingEvidenceKinds":         []string{"validation"},
					"nextAllowedConvergenceStates": []string{"need_validation"},
					"normalizedSourceKey":          "goal:goal_leaf:goal-leaf-001",
					"rejectedEvidenceByKind":       map[string]any{},
					"rejectedEvidenceRefs":         []string{"run-001/001-verify"},
					"requiredEvidenceKinds":        []string{"validation"},
					"source":                       "goal",
					"sourcePath":                   "goal.leaves[0]",
					"sourceRequirementId":          "goal-leaf-001",
					"sourceScope":                  "goal_leaf",
					"status":                       "open",
					"targetText":                   "goal leaf",
				}},
				"unresolvedItems": []map[string]any{{
					"id":                    "cwi-001",
					"definitionRevision":    "rev-current",
					"sourceScope":           "goal_leaf",
					"sourceRequirementId":   "goal-leaf-001",
					"targetText":            strings.Repeat("goal leaf ", 200),
					"status":                "open",
					"currentGap":            "missing accepted evidence binding for required evidence kind: validation",
					"requiredEvidenceKinds": []string{"validation"},
					"missingEvidenceKinds":  []string{"validation"},
				}},
			},
			"workspaceFileManifest": strings.Repeat("large manifest ", 2000),
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         4096,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         8 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)

	if got.ContextBudget == nil {
		t.Fatal("contextBudget missing")
	}
	if got.ContextBudget.ContextGateAction != providerContextGatePass {
		t.Fatalf("contextBudget = %+v, want pass after preserving CWI focus fields", got.ContextBudget)
	}
	focus := recordFromAny(recordFromAny(got.Input["coverageWorkItemContext"])["currentFocus"])
	if len(stringSliceFromAny(focus["missingEvidenceKinds"])) != 1 ||
		len(stringSliceFromAny(focus["requiredEvidenceKinds"])) != 1 ||
		len(stringSliceFromAny(focus["nextAllowedConvergenceStates"])) != 1 {
		t.Fatalf("currentFocus = %+v, want preserved minimum array fields", focus)
	}
	for _, key := range []string{"sourceScope", "status", "targetText"} {
		if strings.TrimSpace(stringValue(focus[key])) == "" {
			t.Fatalf("currentFocus = %+v, want preserved minimum scalar field %q", focus, key)
		}
	}
}

func TestProviderBudgetPriorityCompactionRestoresCoverageWorkItemFocusFromOriginal(t *testing.T) {
	cwiContext := map[string]any{
		"summary": map[string]any{"total": 1, "open": 1},
		"currentFocus": map[string]any{
			"acceptedEvidenceByKind":       map[string]any{},
			"acceptedEvidenceRefs":         []string{},
			"coverageBoundary":             "must stay within source targetText",
			"currentGap":                   "missing accepted evidence binding for required evidence kind: validation",
			"definitionRevision":           "rev-current",
			"id":                           "cwi-001",
			"missingEvidenceKinds":         []string{"validation"},
			"nextAllowedConvergenceStates": []string{"need_validation"},
			"normalizedSourceKey":          "goal:goal_leaf:goal-leaf-001",
			"rejectedEvidenceByKind":       map[string]any{},
			"rejectedEvidenceRefs":         []string{},
			"requiredEvidenceKinds":        []string{"validation"},
			"source":                       "goal",
			"sourcePath":                   "goal.leaves[0]",
			"sourceRequirementId":          "goal-leaf-001",
			"sourceScope":                  "goal_leaf",
			"status":                       "open",
			"targetText":                   "goal leaf",
		},
	}
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-cwi-budget-priority",
		RunID:       "run-cwi-budget-priority",
		StepID:      "run-cwi-budget-priority/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"goal":                    map[string]any{"text": "goal"},
			"dod":                     map[string]any{"items": []any{"dod"}},
			"runtimeStage":            "decision",
			"coverageWorkItemContext": cwiContext,
			"executionContext": map[string]any{
				"coverageWorkItemContext": cwiContext,
				"governanceHistory":       []map[string]any{{"ticketId": "decision-1"}},
			},
			"executionContextPackage": map[string]any{
				"coverageWorkItemContext": cwiContext,
				"selectedContext":         strings.Repeat("large selected context ", 2000),
			},
			"workspaceFileManifest": strings.Repeat("large manifest ", 2000),
			"previousResults":       strings.Repeat("large results ", 2000),
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    32768,
				MaxInputTokens:         4096,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         8 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)

	if got.ContextBudget == nil || got.ContextBudget.ShrinkLevel < 3 {
		t.Fatalf("contextBudget = %+v, want priority compaction path", got.ContextBudget)
	}
	if got.ContextBudget.ContextGateAction != providerContextGatePass {
		t.Fatalf("contextBudget = %+v, want pass after protected CWI restore", got.ContextBudget)
	}
	focus := recordFromAny(recordFromAny(got.Input["coverageWorkItemContext"])["currentFocus"])
	for _, key := range []string{"sourceScope", "status", "targetText"} {
		if strings.TrimSpace(stringValue(focus[key])) == "" {
			t.Fatalf("currentFocus = %+v, want preserved minimum scalar field %q", focus, key)
		}
	}
}

func TestPrioritizedProviderInputKeepsProcessEvidenceTimelineAsP2(t *testing.T) {
	input := map[string]any{
		"workspaceId": "ws-process-budget",
		"goalId":      "goal-process-budget",
		"processEvidenceTimeline": []map[string]any{
			{
				"order":         1,
				"sourceRef":     "step://run-process/graph-001/001-action",
				"stepId":        "run-process/graph-001/001-action",
				"actionKind":    "tool_call",
				"state":         "failed",
				"summary":       strings.Repeat("actionable failure ", 100),
				"failureClass":  "runtime_failed",
				"failureReason": strings.Repeat("failure detail ", 100),
				"payloadRef":    "workorch://workspaces/ws-process-budget/runs/run-process/steps/run-process/graph-001/001-action/output.json",
			},
		},
		"currentProgressDigest": map[string]any{
			"currentGap":         "latest failure still open",
			"nextRequiredAction": "address_last_failure_under_current_stage_contract",
		},
	}

	got := prioritizedProviderInput(input, true)

	if items, ok := got["processEvidenceTimeline"].([]any); !ok || len(items) == 0 {
		t.Fatalf("prioritized input = %+v, want processEvidenceTimeline retained as P2", got)
	}
	digest := recordFromAny(got["currentProgressDigest"])
	if digest["nextRequiredAction"] != "address_last_failure_under_current_stage_contract" {
		t.Fatalf("currentProgressDigest = %+v, want retained next action", digest)
	}
	refs := providerInputSourceRefs(input)
	if !stringSliceContains(refs, "input.processEvidenceTimeline") ||
		!stringSliceContains(refs, "input.currentProgressDigest") {
		t.Fatalf("sourceRefs = %+v, want process evidence refs", refs)
	}
}

func TestEnforceProviderRequestBudgetDoesNotMarkSmallRequestTruncated(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-small",
		RunID:       "run-small",
		StepID:      "run-small/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input:       map[string]any{"executionContext": map[string]any{"goal": map[string]any{"text": "small"}}},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         4096,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         12 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudgetLimit(req, 12*1024)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget metadata missing")
	}
	if got.ContextBudget.Truncated {
		t.Fatalf("contextBudget = %+v, want non-truncated small request", got.ContextBudget)
	}
}

func TestEnforceProviderRequestBudgetAppliesStageAwareBudget(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-stage-budget",
		RunID:       "run-stage-budget",
		StepID:      "run-stage-budget/validation-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"currentSoulStage": "validation",
			"executionContext": map[string]any{
				"goal": map[string]any{"text": "small"},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ProfileMode:             contextProfileModeAuto,
				CapabilitySource:        contextCapabilitySourceModelRegistry,
				ContextWindowTokens:     1_000_000,
				MaxInputTokens:          800_000,
				ReservedOutputTokens:    65_536,
				SafetyRatio:             0.8,
				ContextUtilizationRatio: 0.75,
				BytesHardLimit:          4_000_000,
				MaxRequestBytes:         4_194_304,
				EstimatedTokensPerByte:  0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget metadata missing")
	}
	if got.ContextBudget.MaxTokens != 440_000 ||
		got.ContextBudget.MaxBytes != 2_200_000 ||
		got.ContextBudget.StageBudgetRatio != 0.55 ||
		got.ContextBudget.StageBudgetSource != "stage_validation" {
		t.Fatalf("contextBudget = %+v, want validation stage budget ratio applied", got.ContextBudget)
	}
}

func TestEnforceProviderRequestBudgetPreservesAssessmentOnlyRuntimeContract(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-assessment-budget",
		RunID:       "run-assessment-budget",
		StepID:      "run-assessment-budget/assessment-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"workspaceId":                   "ws-assessment-budget",
			"goalId":                        "goal-assessment-budget",
			"doDId":                         "dod-assessment-budget",
			"previousStepId":                "run-assessment-budget/follow-up-graph-3",
			"assessmentOnly":                true,
			"assessmentOnlyReason":          "graph_follow_up_expansion_limit_exceeded",
			"graphExpansionLimitFollowUp":   true,
			"graphFollowUpExpansionCount":   3,
			"graphFollowUpExpansionLimit":   2,
			"runtimeStage":                  "verification",
			"runtimeStageMode":              "strict_convergence",
			"currentSoulStage":              "validation",
			"processEvidenceTimeline":       repeatedBudgetItems(200),
			"workspaceFileManifest":         repeatedBudgetItems(200),
			"runEvidenceSummary":            repeatedBudgetItems(200),
			"currentProgressDigest":         map[string]any{"currentGap": "none_observed_in_recent_process_evidence"},
			"executionContextPackage":       map[string]any{"goal": map[string]any{"text": "goal"}, "dod": map[string]any{"items": []any{"dod"}}},
			"executionContext":              map[string]any{"goal": map[string]any{"text": "goal"}, "dod": map[string]any{"items": []any{"dod"}}},
			"largeTransportOnlyNoiseMarker": strings.Repeat("noise ", 4000),
		},
		RuntimeCapabilities: declaredRuntimeCapabilities(),
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         1024,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         10 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudgetLimit(req, 10*1024)

	if got.ContextBudget == nil || got.ContextBudget.ShrinkLevel < 3 {
		t.Fatalf("contextBudget = %+v, want aggressive compaction", got.ContextBudget)
	}
	if got.Input["assessmentOnly"] != true ||
		got.Input["assessmentOnlyReason"] != "graph_follow_up_expansion_limit_exceeded" ||
		got.Input["graphExpansionLimitFollowUp"] != true ||
		intValue(got.Input["graphFollowUpExpansionCount"]) != 3 ||
		intValue(got.Input["graphFollowUpExpansionLimit"]) != 2 ||
		got.Input["runtimeStage"] != "verification" ||
		got.Input["runtimeStageMode"] != "strict_convergence" ||
		got.Input["currentSoulStage"] != "validation" {
		t.Fatalf("input = %+v, want assessment-only runtime contract preserved after compaction", got.Input)
	}
	if !stringSliceContains(got.ContextBudget.ProtectedFields, "input.assessmentOnly") ||
		!stringSliceContains(got.ContextBudget.ProtectedFields, "input.graphExpansionLimitFollowUp") ||
		!stringSliceContains(got.ContextBudget.ProtectedFields, "input.runtimeStage") {
		t.Fatalf("protectedFields = %+v, want runtime contract controls recorded", got.ContextBudget.ProtectedFields)
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "protected_runtime_contract_changed_after_context_budget") ||
		got.ContextBudget.ContextGateReason == "protected_runtime_contract_changed_after_context_budget" {
		t.Fatalf("contextBudget = %+v, want protected runtime contract preserved without drift warning", got.ContextBudget)
	}
}

func TestEnforceProviderRequestBudgetCompactsWhenTokenBudgetExceeded(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-token",
		RunID:       "run-token",
		StepID:      "run-token/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"executionContext": map[string]any{
				"goal": map[string]any{"text": strings.Repeat("goal context ", 500)},
				"dod":  map[string]any{"items": []any{strings.Repeat("validation requirement ", 300)}},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    4096,
				MaxInputTokens:         120,
				ReservedOutputTokens:   512,
				SafetyRatio:            0.8,
				BytesHardLimit:         512 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}
	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget metadata missing")
	}
	if !got.ContextBudget.Truncated {
		t.Fatalf("contextBudget = %+v, want truncated by token budget", got.ContextBudget)
	}
	if got.ContextBudget.MaxTokens != 120 || got.ContextBudget.ShrinkLevel != 4 || len(got.ContextBudget.Rules) < 2 {
		t.Fatalf("contextBudget = %+v, want token-aware aggressive compaction metadata", got.ContextBudget)
	}
	if got.ContextBudget.GoalDoDCompressed {
		t.Fatalf("contextBudget = %+v, want goal/dod not compressed under token pressure", got.ContextBudget)
	}
}

func TestMinimalProviderInputUsesStageAwareExecutionContext(t *testing.T) {
	input := map[string]any{
		"currentSoulStage": "validation",
		"executionContext": map[string]any{
			"goal":              map[string]any{"text": "goal"},
			"dod":               map[string]any{"items": []any{"must pass"}},
			"dods":              []any{"d1", "d2"},
			"governanceHistory": []any{map[string]any{"id": "g1"}, map[string]any{"id": "g2"}},
			"workspaceManifest": map[string]any{"id": "manifest"},
			"skillSelection":    map[string]any{"reason": "planner"},
		},
	}
	out := minimalProviderInput(input)
	exec := recordFromAny(out["executionContext"])
	if len(recordFromAny(exec["goal"])) == 0 || len(recordFromAny(exec["dod"])) == 0 {
		t.Fatalf("executionContext = %+v, want goal/dod preserved", exec)
	}
	if _, ok := exec["workspaceManifest"]; ok {
		t.Fatalf("executionContext = %+v, workspaceManifest should be omitted for validation stage", exec)
	}
	if _, ok := exec["skillSelection"]; ok {
		t.Fatalf("executionContext = %+v, skillSelection should be omitted for validation stage", exec)
	}
}

func TestEnforceProviderRequestBudgetAdaptiveShrinkForProviderRetry(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-retry",
		RunID:       "run-retry",
		StepID:      "run-retry/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"currentSoulStage":      "execution",
			"providerRetryFollowUp": true,
			"providerRetryAttempt":  2,
			"failureClass":          "provider_timeout",
			"failedExecutionGraph":  repeatedBudgetItems(80),
			"workspaceFileManifest": repeatedBudgetItems(80),
			"executionContext": map[string]any{
				"goal": strings.Repeat("goal ", 200),
				"dod":  strings.Repeat("dod ", 200),
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         512,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         256 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}
	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget metadata missing")
	}
	if got.ContextBudget.ShrinkLevel < 4 {
		t.Fatalf("contextBudget = %+v, want adaptive shrink level >=4 on retry attempt 2", got.ContextBudget)
	}
	if len(got.ContextBudget.Rules) < 2 || got.ContextBudget.Rules[1] != "adaptive shrink based on provider retry" {
		t.Fatalf("contextBudget rules = %+v, want adaptive shrink rule", got.ContextBudget.Rules)
	}
}

func TestEnforceProviderRequestBudgetAdaptiveShrinkForStreamIncompleteRetry(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-stream-retry",
		RunID:       "run-stream-retry",
		StepID:      "run-stream-retry/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"currentSoulStage":      "execution",
			"providerRetryFollowUp": true,
			"providerRetryAttempt":  2,
			"failureClass":          "provider_stream_incomplete",
			"failedExecutionGraph":  repeatedBudgetItems(80),
			"workspaceFileManifest": repeatedBudgetItems(80),
			"executionContext": map[string]any{
				"goal": strings.Repeat("goal ", 200),
				"dod":  strings.Repeat("dod ", 200),
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         512,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         256 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}
	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget metadata missing")
	}
	if got.ContextBudget.ShrinkLevel < 4 {
		t.Fatalf("contextBudget = %+v, want adaptive shrink level >=4 on stream incomplete retry attempt 2", got.ContextBudget)
	}
}

func TestEnforceProviderRequestBudgetAdaptiveShrinkForContractViolationRetry(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-contract-retry",
		RunID:       "run-contract-retry",
		StepID:      "run-contract-retry/contract-retry-003",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"currentSoulStage":       "validation",
			"runtimeRepairFollowUp":  true,
			"runtimeRepairAttempt":   3,
			"contractViolationRetry": true,
			"contractViolationRecovery": map[string]any{
				"correctionMode": "forced_narrow",
			},
			"failedExecutionGraph":  repeatedBudgetItems(120),
			"workspaceFileManifest": repeatedBudgetItems(120),
			"repairActionPackage": map[string]any{
				"recentRepairAttempts": repeatedBudgetItems(40),
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence": true,
					"acceptedKinds":                         []string{"validation/managed_session"},
				},
			},
			"executionContext": map[string]any{
				"goal": strings.Repeat("goal ", 200),
				"dod":  strings.Repeat("dod ", 200),
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    32768,
				MaxInputTokens:         21504,
				ReservedOutputTokens:   4096,
				BytesHardLimit:         256 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil || got.ContextBudget.ShrinkLevel < 4 {
		t.Fatalf("contextBudget = %+v, want aggressive shrink on repeated contract retry", got.ContextBudget)
	}
	if !stringSliceContains(got.ContextBudget.Rules, "adaptive shrink based on runtime recovery lineage") {
		t.Fatalf("contextBudget rules = %+v, want runtime recovery adaptive shrink rule", got.ContextBudget.Rules)
	}
	if got.ContextBudget.ActualBytes >= defaultProviderRequestBudgetBytes {
		t.Fatalf("contextBudget actualBytes = %d, want repeated contract retry below default dynamic request budget", got.ContextBudget.ActualBytes)
	}
	if _, ok := got.Input["failedExecutionGraph"]; ok {
		t.Fatalf("input retained failedExecutionGraph after repeated contract retry shrink")
	}
}

func TestEnforceProviderRequestBudgetCurrentActionKeepsOnlyStagePacketSnapshotTransport(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-current-action",
		RunID:       "run-current-action",
		StepID:      "run-current-action/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"currentActionContract": map[string]any{
				"nextAction": map[string]any{"kind": "validate"},
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence": true,
					"acceptedKinds":                         []string{"shell/command"},
				},
			},
			"stagePacket": map[string]any{
				"schemaVersion": "workorch.stage_packet.v1",
				"workspaceId":   "ws-current-action",
				"runId":         "run-current-action",
				"stepId":        "run-current-action/old-step",
				"nextAction":    map[string]any{"kind": "state_change"},
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
				},
				"targetSnapshots": []map[string]any{{
					"targetRef":             "target-a",
					"content":               "complete target content",
					"contentComplete":       true,
					"safeForFileWrite":      true,
					"safeForExactFileWrite": true,
					"integrityState":        "full",
				}},
			},
		},
		Config: llmmodels.RedactedProviderConfig{ContextProfile: llmmodels.ProviderContextProfile{
			ContextWindowTokens:    8192,
			MaxInputTokens:         4096,
			ReservedOutputTokens:   1024,
			BytesHardLimit:         256 * 1024,
			EstimatedTokensPerByte: 0.25,
		}},
	}

	got := enforceProviderRequestBudget(req)
	packet := recordFromAny(got.Input["stagePacket"])
	if len(mapSliceFromAny(packet["targetSnapshots"])) != 1 {
		t.Fatalf("stagePacket = %+v, want target snapshot transport preserved", packet)
	}
	for _, key := range []string{"nextAction", "requiredOutput", "activeFocus", "currentFailure", "currentGap"} {
		if _, ok := packet[key]; ok {
			t.Fatalf("stagePacket = %+v, want legacy current-action field %q removed", packet, key)
		}
	}
	if got.ContextBudget == nil || len(got.ContextBudget.IntegrityWarnings) != 0 {
		t.Fatalf("contextBudget = %+v, want current-action stage transport without integrity warning", got.ContextBudget)
	}
}

func TestEnforceProviderRequestBudgetDoesNotHalveContextRetryBudget(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-context-retry",
		RunID:       "run-context-retry",
		StepID:      "run-context-retry/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"currentSoulStage":      "execution",
			"providerRetryFollowUp": true,
			"providerRetryAttempt":  1,
			"failureClass":          "context_too_large",
			"executionContextPackage": map[string]any{
				"goal": map[string]any{"text": strings.Repeat("goal contract ", 600)},
				"dod":  map[string]any{"items": []any{strings.Repeat("acceptance contract ", 400)}},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    32768,
				MaxInputTokens:         21504,
				ReservedOutputTokens:   4096,
				SafetyRatio:            0.8,
				BytesHardLimit:         86016,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)

	if got.ContextBudget == nil {
		t.Fatal("contextBudget metadata missing")
	}
	if got.ContextBudget.StageBudgetRatio != 1 || got.ContextBudget.StageBudgetSource != "provider_retry_context_contract" {
		t.Fatalf("contextBudget = %+v, want full budget for context retry", got.ContextBudget)
	}
	if got.ContextBudget.P0Overflow {
		t.Fatalf("contextBudget = %+v, context retry must not create artificial P0 overflow", got.ContextBudget)
	}
}

func TestEnforceProviderRequestBudgetPreservesDirectStateChangingRecoveryContract(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-direct-recovery",
		RunID:       "run-direct-recovery",
		StepID:      "run-direct-recovery/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"currentSoulStage":                  "execution",
			"providerRetryFollowUp":             true,
			"providerRetryAttempt":              1,
			"failureClass":                      "provider_timeout",
			"repairObjective":                   "produce_state_changing_output",
			"stateChangingOutputRequired":       true,
			"stateChangingRecoveryMode":         "direct_state_change",
			"providerTimeoutStateChangingRetry": true,
			"failedStepOutput":                  map[string]any{"partialOutputPreview": strings.Repeat("patch ", 200)},
			"failedExecutionGraph":              repeatedBudgetItems(80),
			"graphEvidence":                     repeatedBudgetItems(80),
			"executionContextPackage": map[string]any{
				"goal":              map[string]any{"text": "packaged goal"},
				"dod":               map[string]any{"items": []any{"packaged dod"}},
				"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         512,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         256 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)

	if got.ContextBudget == nil || got.ContextBudget.ShrinkLevel < 4 {
		t.Fatalf("contextBudget = %+v, want aggressive direct recovery shrink", got.ContextBudget)
	}
	if got.Input["stateChangingRecoveryMode"] != "direct_state_change" ||
		got.Input["stateChangingOutputRequired"] != true {
		t.Fatalf("input = %+v, want direct state-changing recovery keys preserved", got.Input)
	}
	pkg := recordFromAny(got.Input["executionContextPackage"])
	if recordFromAny(pkg["goal"])["text"] != "packaged goal" ||
		len(recordFromAny(pkg["dod"])) == 0 ||
		recordFromAny(pkg["executionContract"])["stateChangePolicy"] != "must_produce_state_change" {
		t.Fatalf("executionContextPackage = %+v, want protected package Goal/DoD/contract preserved", pkg)
	}
	if _, ok := got.Input["failedExecutionGraph"]; ok {
		t.Fatalf("input retained failedExecutionGraph after direct recovery shrink: %+v", got.Input["failedExecutionGraph"])
	}
}

func TestEnforceProviderRequestBudgetPreservesSafeRepairTargetSnapshots(t *testing.T) {
	safeContent := "package main\n\nconst APIKey = \"test-key\"\n"
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-repair-budget",
		RunID:       "run-repair-budget",
		StepID:      "run-repair-budget/repair-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"currentSoulStage":      "execution",
			"providerRetryFollowUp": true,
			"providerRetryAttempt":  2,
			"failureClass":          "provider_timeout",
			"failedExecutionGraph":  repeatedBudgetItems(200),
			"graphEvidence":         repeatedBudgetItems(200),
			"failureTargetRepairContext": []map[string]any{{
				"targetRef":        "main_test.go",
				"content":          safeContent,
				"contentHash":      "sha256-safe",
				"contentComplete":  true,
				"safeForFileWrite": true,
				"redacted":         false,
				"truncated":        false,
				"isBinary":         false,
			}},
			"repairActionPackage": map[string]any{
				"targetSnapshots": []map[string]any{{
					"targetRef":        "main_test.go",
					"content":          safeContent,
					"contentHash":      "sha256-safe",
					"contentComplete":  true,
					"safeForFileWrite": true,
					"redacted":         false,
					"truncated":        false,
					"isBinary":         false,
				}},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         512,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         16 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil || got.ContextBudget.ShrinkLevel < 4 {
		t.Fatalf("contextBudget = %+v, want aggressive shrink under retry budget pressure", got.ContextBudget)
	}
	failureTargets := providerRepairSnapshotItemsFromValue(got.Input["failureTargetRepairContext"])
	if len(failureTargets) != 1 || failureTargets[0]["content"] != safeContent {
		t.Fatalf("failureTargetRepairContext = %#v, want safe content preserved", got.Input["failureTargetRepairContext"])
	}
	pkg := recordFromAny(got.Input["repairActionPackage"])
	targets := providerRepairSnapshotItemsFromValue(pkg["targetSnapshots"])
	if len(targets) != 1 || targets[0]["content"] != safeContent {
		t.Fatalf("repairActionPackage.targetSnapshots = %#v, want safe content preserved", pkg["targetSnapshots"])
	}
}

func TestRedactProviderRequestPreservesOnlySafeRepairSnapshotContent(t *testing.T) {
	safeContent := "package main\n\nconst APIKey = \"test-key\"\n"
	unsafeContent := "package main\n\nconst APIKey = \"unsafe-key\"\n"
	req := llmmodels.ProviderRequest{
		Input: map[string]any{
			"apiKey": "top-secret",
			"failureTargetRepairContext": []map[string]any{{
				"targetRef":        "safe.go",
				"content":          safeContent,
				"contentHash":      "sha256-safe",
				"contentComplete":  true,
				"safeForFileWrite": true,
				"redacted":         false,
				"truncated":        false,
				"isBinary":         false,
			}},
			"patchRepairContext": []map[string]any{{
				"targetRef":        "unsafe.go",
				"content":          unsafeContent,
				"contentHash":      "sha256-unsafe",
				"contentComplete":  false,
				"safeForFileWrite": false,
				"redacted":         false,
				"truncated":        true,
				"isBinary":         false,
			}},
			"repairActionPackage": map[string]any{
				"targetSnapshots": []map[string]any{{
					"targetRef":        "safe.go",
					"content":          safeContent,
					"contentHash":      "sha256-safe",
					"contentComplete":  true,
					"safeForFileWrite": true,
					"redacted":         false,
					"truncated":        false,
					"isBinary":         false,
				}},
			},
		},
	}

	got := redactProviderRequestForLLM(req)
	if got.Input["apiKey"] != "[REDACTED]" {
		t.Fatalf("input.apiKey = %#v, want redacted", got.Input["apiKey"])
	}
	failureTargets := providerRepairSnapshotItemsFromValue(got.Input["failureTargetRepairContext"])
	if len(failureTargets) != 1 {
		t.Fatalf("failureTargetRepairContext = %#v, want one safe target", got.Input["failureTargetRepairContext"])
	}
	if failureTargets[0]["content"] != safeContent {
		t.Fatalf("safe target content = %#v, want preserved", failureTargets[0]["content"])
	}
	patchTargets := providerRepairSnapshotItemsFromValue(got.Input["patchRepairContext"])
	if len(patchTargets) != 1 {
		t.Fatalf("patchRepairContext = %#v, want one unsafe target", got.Input["patchRepairContext"])
	}
	if patchTargets[0]["content"] == unsafeContent || !strings.Contains(stringValue(patchTargets[0]["content"]), "[REDACTED]") {
		t.Fatalf("unsafe patch target content = %#v, want redacted", patchTargets[0]["content"])
	}
	pkg := recordFromAny(got.Input["repairActionPackage"])
	targets := providerRepairSnapshotItemsFromValue(pkg["targetSnapshots"])
	if len(targets) != 1 || targets[0]["content"] != safeContent {
		t.Fatalf("repairActionPackage.targetSnapshots = %#v, want safe content preserved", pkg["targetSnapshots"])
	}
	if got.ContextBudget == nil || !got.ContextBudget.RedactionApplied {
		t.Fatalf("contextBudget = %+v, want redaction manifest", got.ContextBudget)
	}
	for _, path := range got.ContextBudget.RedactedPaths {
		if path == "input.failureTargetRepairContext[].content" || path == "input.repairActionPackage.targetSnapshots[].content" {
			t.Fatalf("redactedPaths = %+v, safe repair content should not be reported as redacted", got.ContextBudget.RedactedPaths)
		}
	}
}

func TestRedactProviderRequestPreservesCoverageTargetTokenContractIdentifiers(t *testing.T) {
	req := llmmodels.ProviderRequest{
		Input: map[string]any{
			"accessToken": "secret-access-token",
			"coverageTargetSet": []map[string]any{{
				"coverageTargetToken": "goal-req-001",
				"sourceRequirementId": "goal-req-001",
				"targetText":          "visible goal target",
			}},
			"repairActionPackage": map[string]any{
				"coverageTargetSet": []any{
					map[string]any{
						"coverageTargetToken": "version_control_snapshot",
						"sourceRequirementId": "version_control_snapshot",
						"targetText":          "version_control_snapshot",
					},
				},
			},
			"nested": map[string]any{
				"token":               "plain-secret-token",
				"coverageTargetToken": "dod-item-001",
			},
		},
	}

	got := redactProviderRequestForLLM(req)
	if got.Input["accessToken"] != "[REDACTED]" {
		t.Fatalf("accessToken = %#v, want redacted", got.Input["accessToken"])
	}
	targets := mapSliceFromAny(got.Input["coverageTargetSet"])
	if len(targets) != 1 || targets[0]["coverageTargetToken"] != "goal-req-001" {
		t.Fatalf("coverageTargetSet = %#v, want coverageTargetToken preserved", got.Input["coverageTargetSet"])
	}
	pkg := recordFromAny(got.Input["repairActionPackage"])
	pkgTargets := mapSliceFromAny(pkg["coverageTargetSet"])
	if len(pkgTargets) != 1 || pkgTargets[0]["coverageTargetToken"] != "version_control_snapshot" {
		t.Fatalf("repairActionPackage.coverageTargetSet = %#v, want coverageTargetToken preserved", pkg["coverageTargetSet"])
	}
	nested := recordFromAny(got.Input["nested"])
	if nested["token"] != "[REDACTED]" {
		t.Fatalf("nested token = %#v, want redacted", nested["token"])
	}
	if nested["coverageTargetToken"] != "dod-item-001" {
		t.Fatalf("nested coverageTargetToken = %#v, want preserved", nested["coverageTargetToken"])
	}
}

func TestRedactProviderRequestDoesNotReportSafeSnapshotRedactedForJSONDecodedSlices(t *testing.T) {
	safeContent := "package main\n\nconst APIKey = \"test-key\"\n"
	req := llmmodels.ProviderRequest{
		Input: map[string]any{
			"apiKey": "top-secret",
			"failureTargetRepairContext": []any{map[string]any{
				"targetRef":        "gateway/main_test.go",
				"content":          safeContent,
				"contentHash":      "sha256-safe",
				"contentComplete":  true,
				"safeForFileWrite": true,
				"redacted":         false,
			}},
			"repairActionPackage": map[string]any{
				"targetSnapshots": []any{map[string]any{
					"targetRef":        "gateway/main_test.go",
					"content":          safeContent,
					"contentHash":      "sha256-safe",
					"contentComplete":  true,
					"safeForFileWrite": true,
					"redacted":         false,
				}},
			},
		},
	}

	got := redactProviderRequestForLLM(req)
	if got.ContextBudget == nil || !got.ContextBudget.RedactionApplied {
		t.Fatalf("contextBudget = %+v, want redaction manifest for sensitive top-level content", got.ContextBudget)
	}
	for _, path := range got.ContextBudget.RedactedPaths {
		if strings.HasPrefix(path, "input.failureTargetRepairContext[].") ||
			strings.HasPrefix(path, "input.repairActionPackage.targetSnapshots[].") {
			t.Fatalf("redactedPaths = %+v, safe JSON-decoded repair snapshot should not be reported as redacted", got.ContextBudget.RedactedPaths)
		}
	}
}

func TestEnforceProviderRequestBudgetRecordsContextIntegrityAudit(t *testing.T) {
	safeContent := "package main\n\nfunc main() {}\n"
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-integrity",
		RunID:       "run-integrity",
		StepID:      "run-integrity/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"goal": "preserve goal",
			"dod":  "preserve dod",
			"repairActionPackage": map[string]any{
				"coverageRepairRequired": true,
				"coverageTargetSet": []map[string]any{{
					"kind":                "dod",
					"coverageTargetToken": "target-1",
					"targetText":          "target one",
				}},
				"currentFailure": map[string]any{
					"failureClass":     "shell_command_failed",
					"failureSignature": "class=shell_command_failed|kind=shell/command|failureTargets=main.go|exitCode=1",
					"stderrPreview":    "main.go:12:2: undefined: missingThing",
				},
				"lastFailureDiagnostics": map[string]any{
					"stderrPreview": "main.go:12:2: undefined: missingThing",
				},
				"currentExecutionProgress": map[string]any{
					"coverageGapCount": 1,
				},
				"requiredOutput": map[string]any{
					"mustDeclareCoverageTargets": true,
				},
				"targetSnapshots": []map[string]any{{
					"targetRef":        "main.go",
					"content":          safeContent,
					"contentHash":      "hash-main",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{BytesHardLimit: 4096, EstimatedTokensPerByte: 0.25},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget = nil, want integrity audit")
	}
	for _, expected := range []string{
		"input.goal",
		"input.dod",
		"input.repairActionPackage.coverageTargetSet",
		"input.repairActionPackage.currentFailure",
		"input.repairActionPackage.currentExecutionProgress",
		"input.repairActionPackage.lastFailureDiagnostics",
		"input.repairActionPackage.requiredOutput",
		"input.repairActionPackage.targetSnapshots",
	} {
		if !stringSliceContains(got.ContextBudget.ProtectedFields, expected) {
			t.Fatalf("protectedFields = %+v, missing %s", got.ContextBudget.ProtectedFields, expected)
		}
	}
	if len(got.ContextBudget.IntegrityWarnings) != 0 {
		t.Fatalf("integrityWarnings = %+v, want none", got.ContextBudget.IntegrityWarnings)
	}
	if got.ContextBudget.ContextGateAction != providerContextGatePass {
		t.Fatalf("contextGateAction = %q, want pass", got.ContextBudget.ContextGateAction)
	}
}

func TestEnforceProviderRequestBudgetWarnsCoverageRepairWithoutTargets(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-integrity-warning",
		RunID:       "run-integrity-warning",
		StepID:      "run-integrity-warning/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"repairActionPackage": map[string]any{
				"coverageRepairRequired": true,
				"requiredOutput": map[string]any{
					"mustDeclareCoverageTargets": true,
				},
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget = nil, want integrity audit")
	}
	for _, expected := range []string{
		"coverage_repair_required_without_coverage_target_set",
		"must_declare_coverage_targets_without_target_set",
	} {
		if !stringSliceContains(got.ContextBudget.IntegrityWarnings, expected) {
			t.Fatalf("integrityWarnings = %+v, missing %s", got.ContextBudget.IntegrityWarnings, expected)
		}
	}
	if got.ContextBudget.ContextGateAction != providerContextGateReplanRequired {
		t.Fatalf("contextGateAction = %q, want replan_required", got.ContextBudget.ContextGateAction)
	}
}

func TestEnforceProviderRequestBudgetFailsGateWhenRepairCurrentFailureMissing(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-missing-failure",
		RunID:       "run-missing-failure",
		StepID:      "run-missing-failure/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"runtimeRepairFollowUp": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{"mustReturnValidJSON": true},
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget = nil, want integrity audit")
	}
	if !stringSliceContains(got.ContextBudget.IntegrityWarnings, "runtime_repair_current_failure_missing") {
		t.Fatalf("integrityWarnings = %+v, want missing current failure warning", got.ContextBudget.IntegrityWarnings)
	}
	if got.ContextBudget.ContextGateAction != providerContextGateBlockerRequired {
		t.Fatalf("contextGateAction = %q, want blocker_required", got.ContextBudget.ContextGateAction)
	}
}

func TestEnforceProviderRequestBudgetAcceptsRepairPackageCurrentFailure(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-package-current-failure",
		RunID:       "run-package-current-failure",
		StepID:      "run-package-current-failure/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"runtimeRepairFollowUp": true,
			"repairActionPackage": map[string]any{
				"currentFailure": map[string]any{
					"failedStepId":     "run-package-current-failure/graph-001/002-run-tests",
					"failedStepKind":   "shell/command",
					"failureClass":     "shell_command_failed",
					"failureSignature": "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=0",
					"reason":           "test output contains failure markers",
				},
				"currentExecutionProgress": map[string]any{
					"currentGap":         "main_test.go:28 expected /v1/chat/completions actual /chat/completions",
					"nextRequiredAction": "address_last_failure_under_current_stage_contract",
				},
				"requiredOutput": map[string]any{"mustReturnValidJSON": true},
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget = nil, want integrity audit")
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "runtime_repair_current_failure_missing") {
		t.Fatalf("integrityWarnings = %+v, did not expect missing current failure", got.ContextBudget.IntegrityWarnings)
	}
	if got.ContextBudget.ContextGateAction == providerContextGateFailFast {
		t.Fatalf("contextGateAction = %q, did not expect fail_fast", got.ContextBudget.ContextGateAction)
	}
}

func TestEnforceProviderRequestBudgetAcceptsTopLevelCurrentFailure(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-top-level-current-failure",
		RunID:       "run-top-level-current-failure",
		StepID:      "run-top-level-current-failure/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"runtimeRepairFollowUp": true,
			"currentFailure": map[string]any{
				"failureClass":       "coverage_actionable_gap",
				"coverageWorkItemId": "cwi-top-level",
				"reason":             "coverage work item requires additional evidence",
			},
			"currentProgressDigest": map[string]any{
				"currentGap":         "coverage item still lacks accepted validation evidence",
				"nextRequiredAction": "continue_current_stage_contract",
			},
			"processEvidenceTimeline": []any{
				map[string]any{"stepId": "run-top-level-current-failure/step-1", "state": "completed"},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{"mustReturnValidJSON": true},
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget = nil, want integrity audit")
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "runtime_repair_current_failure_missing") {
		t.Fatalf("integrityWarnings = %+v, did not expect missing current failure", got.ContextBudget.IntegrityWarnings)
	}
}

func TestEnforceProviderRequestBudgetAcceptsStagePacketCurrentFailure(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-stage-packet-current-failure",
		RunID:       "run-stage-packet-current-failure",
		StepID:      "run-stage-packet-current-failure/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"runtimeRepairFollowUp": true,
			"currentProgressDigest": map[string]any{
				"currentGap":         "coverage item still lacks accepted validation and state_change evidence",
				"nextRequiredAction": "continue_current_stage_contract",
			},
			"processEvidenceTimeline": []any{
				map[string]any{"stepId": "run-stage-packet-current-failure/step-1", "state": "failed"},
			},
			"stagePacket": map[string]any{
				"currentFailure": map[string]any{
					"failedStepId":     "run-stage-packet-current-failure/step-1",
					"failedStepKind":   "llm/workspace_execution",
					"failureClass":     "runtime_failed",
					"failureSignature": "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan",
					"reason":           "previous execution graph output was rejected by contract validation",
				},
			},
			"repairActionPackage": map[string]any{
				"currentExecutionProgress": map[string]any{
					"currentGap":         "coverage item still lacks accepted validation and state_change evidence",
					"nextRequiredAction": "continue_current_stage_contract",
				},
				"requiredOutput": map[string]any{"mustReturnValidJSON": true},
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget = nil, want integrity audit")
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "runtime_repair_current_failure_missing") {
		t.Fatalf("integrityWarnings = %+v, did not expect missing current failure", got.ContextBudget.IntegrityWarnings)
	}
	if got.ContextBudget.ContextGateAction == providerContextGateBlockerRequired {
		t.Fatalf("contextGateAction = %q, did not expect blocker_required", got.ContextBudget.ContextGateAction)
	}
}

func TestEnforceProviderRequestBudgetAcceptsProtectedStagePacketRepairContext(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-protected-stage-packet-repair-context",
		RunID:       "run-protected-stage-packet-repair-context",
		StepID:      "run-protected-stage-packet-repair-context/follow-up",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"runtimeRepairFollowUp": true,
			"graphEvidence":         strings.Repeat("large evidence ", 20000),
			"stagePacket": map[string]any{
				"schemaVersion": "workorch.stage_packet.v1",
				"activeStage":   "repair",
				"nextAction":    map[string]any{"type": "acquire_evidence"},
				"currentGap": map[string]any{
					"summary": "missing accepted evidence binding for required evidence kind: validation, state_change",
				},
				"currentFailure": map[string]any{
					"failureClass":       "coverage_actionable_gap",
					"coverageWorkItemId": "cwi-001",
					"evidenceValidity":   "actionable_gap",
					"reason":             "missing accepted evidence binding for required evidence kind: validation, state_change",
				},
				"processEvidenceTimeline": []any{
					map[string]any{"stepId": "run-protected-stage-packet-repair-context/graph-001/001-list", "kind": "file/list", "state": "completed"},
				},
				"requiredOutput": map[string]any{
					"mustProduceConcreteFailureEvidence": true,
					"acceptedKinds":                      []string{"file/list"},
				},
			},
		},
	}

	got := enforceProviderRequestBudgetLimit(req, 12*1024)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget = nil, want integrity audit")
	}
	if !got.ContextBudget.Truncated {
		t.Fatalf("contextBudget.Truncated = false, want compaction path")
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "runtime_repair_current_failure_missing") ||
		stringSliceContains(got.ContextBudget.IntegrityWarnings, "runtime_repair_process_evidence_missing") {
		t.Fatalf("integrityWarnings = %+v, did not expect stage packet repair context warnings", got.ContextBudget.IntegrityWarnings)
	}
	packet := recordFromAny(got.Input["stagePacket"])
	if len(recordFromAny(packet["currentFailure"])) == 0 || len(mapSliceFromAny(packet["processEvidenceTimeline"])) == 0 {
		t.Fatalf("stagePacket after budget = %+v, want protected repair context", packet)
	}
	if got.ContextBudget.ContextGateAction == providerContextGateBlockerRequired {
		t.Fatalf("contextGateAction = %q, did not expect blocker_required", got.ContextBudget.ContextGateAction)
	}
}

func TestEnforceProviderRequestBudgetAcceptsInitialStateChangingRunWithoutProcessEvidence(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-state-changing-initial",
		RunID:       "run-state-changing-initial",
		StepID:      "run-state-changing-initial/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"runtimeStage":                "repair",
			"stateChangingOutputRequired": true,
			"goal":                        map[string]any{"text": "goal"},
			"dod":                         map[string]any{"items": []any{"dod"}},
			"convergenceContract": map[string]any{
				"state":                       "need_state_change",
				"repairObjective":             "produce_state_changing_output",
				"stateChangingOutputRequired": true,
			},
			"currentFailure": map[string]any{
				"coverageWorkItemId":    "cwi-001",
				"evidenceValidity":      "actionable_gap",
				"failureClass":          "coverage_actionable_gap",
				"failureReason":         "missing accepted evidence binding for required evidence kind: validation, state_change",
				"missingEvidenceKinds":  []string{"validation", "state_change"},
				"requiredEvidenceKinds": []string{"validation", "state_change"},
				"source":                "coverageWorkItemContext",
				"sourceRequirementId":   "source_change",
				"sourceScope":           "deliverable",
				"targetRef":             "source_change",
			},
			"coverageWorkItemContext": map[string]any{
				"summary": map[string]any{
					"total": 1,
					"open":  1,
				},
				"currentFocus": map[string]any{
					"id":                           "cwi-001",
					"definitionRevision":           "rev-1",
					"sourceScope":                  "deliverable",
					"sourceRequirementId":          "source_change",
					"targetText":                   "source_change",
					"status":                       "open",
					"currentGap":                   "missing accepted evidence binding for required evidence kind: validation, state_change",
					"requiredEvidenceKinds":        []string{"validation", "state_change"},
					"missingEvidenceKinds":         []string{"validation", "state_change"},
					"acceptedEvidenceRefs":         []string{},
					"rejectedEvidenceRefs":         []string{},
					"nextAllowedConvergenceStates": []string{"need_evidence", "need_state_change"},
				},
			},
			"repairActionPackage": map[string]any{
				"coverageTargetSet": []map[string]any{{
					"kind":                "deliverable",
					"sourceRequirementId": "source_change",
					"targetText":          "source_change",
					"state":               "open",
				}},
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"mustDeclareCoverageTargets":   true,
				},
			},
			"stagePacket": map[string]any{
				"activeStage": "repair",
				"nextAction": map[string]any{
					"type": "state_change",
				},
				"currentFailure": map[string]any{
					"coverageWorkItemId":    "cwi-001",
					"evidenceValidity":      "actionable_gap",
					"failureClass":          "coverage_actionable_gap",
					"failureReason":         "missing accepted evidence binding for required evidence kind: validation, state_change",
					"missingEvidenceKinds":  []string{"validation", "state_change"},
					"requiredEvidenceKinds": []string{"validation", "state_change"},
					"source":                "coverageWorkItemContext",
					"sourceRequirementId":   "source_change",
					"sourceScope":           "deliverable",
					"targetRef":             "source_change",
				},
				"requiredOutput": map[string]any{
					"acceptedKinds":                  []string{"patch/apply", "git/write", "shell/command", "mcp/tool_call"},
					"mustBindCoverageFocus":          true,
					"mustDeclareCoverageTargets":     true,
					"mustIncludeStateChangingNode":   true,
					"coverageWorkItemIds":            []string{"cwi-001"},
					"forbiddenKinds":                 []string{"read_only_graph", "llm_gated_graph"},
					"patchSubstantiveChangeRule":     "patch/apply text_replace oldText must be exact",
					"coverageFocusBindingRule":       "outputs must bind current focus",
					"evidenceKindRequirementRule":    "state-changing outputs must satisfy current focus",
					"toolStateChangingRule":          "tool state changing output must declare target",
					"coverageTargetMatchRule":        "coverage targets must match current focus",
					"patchApplyInputContract":        map[string]any{"text_replace": []string{"targetRef", "oldText", "newText"}},
					"toolStateChangingInputContract": map[string]any{"requiredFields": []string{"stateChanging", "targetRef"}},
				},
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget = nil, want integrity audit")
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "runtime_repair_process_evidence_missing") {
		t.Fatalf("integrityWarnings = %+v, did not expect missing process evidence for initial state-changing run", got.ContextBudget.IntegrityWarnings)
	}
	if got.ContextBudget.ContextGateAction == providerContextGateFailFast {
		t.Fatalf("contextGateAction = %q, did not expect fail_fast", got.ContextBudget.ContextGateAction)
	}
}

func TestEnforceProviderRequestBudgetFailsGateWhenRepairProcessEvidenceMissing(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-missing-process-evidence",
		RunID:       "run-missing-process-evidence",
		StepID:      "run-missing-process-evidence/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"runtimeRepairFollowUp": true,
			"failedStepId":          "run-missing-process-evidence/graph-001/001-patch",
			"failureClass":          "runtime_failed",
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{"mustReturnValidJSON": true},
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget = nil, want integrity audit")
	}
	if !stringSliceContains(got.ContextBudget.IntegrityWarnings, "runtime_repair_process_evidence_missing") {
		t.Fatalf("integrityWarnings = %+v, want missing process evidence warning", got.ContextBudget.IntegrityWarnings)
	}
	if got.ContextBudget.ContextGateAction != providerContextGateBlockerRequired {
		t.Fatalf("contextGateAction = %q, want blocker_required", got.ContextBudget.ContextGateAction)
	}
}

func TestEnforceProviderRequestBudgetAcceptsRepairActionProgressAsProcessEvidence(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-repair-progress",
		RunID:       "run-repair-progress",
		StepID:      "run-repair-progress/provider-retry-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"runtimeRepairFollowUp":       true,
			"stateChangingOutputRequired": true,
			"failedStepId":                "run-repair-progress/repair-001",
			"failureClass":                "dod_coverage_missing",
			"repairActionPackage": map[string]any{
				"coverageTargetSet": []map[string]any{{
					"kind":       "dod",
					"targetText": "required outcome",
				}},
				"currentExecutionProgress": map[string]any{
					"failedStepId":     "run-repair-progress/repair-001",
					"failureClass":     "dod_coverage_missing",
					"failureReason":    "completionState failed has no actionable continuation",
					"coverageGapCount": 1,
				},
				"requiredOutput": map[string]any{
					"mustDeclareCoverageTargets": true,
				},
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget = nil, want integrity audit")
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "runtime_repair_process_evidence_missing") {
		t.Fatalf("integrityWarnings = %+v, did not expect missing process evidence", got.ContextBudget.IntegrityWarnings)
	}
	if got.ContextBudget.ContextGateAction == providerContextGateFailFast {
		t.Fatalf("contextGateAction = %q, did not expect fail_fast", got.ContextBudget.ContextGateAction)
	}
}

func TestEnforceProviderRequestBudgetAcceptsFailedStepContextAsProcessEvidence(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-repair-failed-step-context",
		RunID:       "run-repair-failed-step-context",
		StepID:      "run-repair-failed-step-context/repair-002",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"runtimeStage": "repair",
			"currentProgressDigest": map[string]any{
				"currentGap":         "completionState continue has no actionable continuation",
				"nextRequiredAction": "address_last_failure_under_current_stage_contract",
			},
			"failedStepInput": map[string]any{
				"continuationFollowUp": true,
				"previousStepId":       "run-repair-failed-step-context/repair-001",
			},
			"failedStepOutput": map[string]any{
				"error":        "completionState continue has no actionable continuation",
				"failureClass": "runtime_failed",
				"stage":        "llm_plan",
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget = nil, want integrity audit")
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "runtime_repair_process_evidence_missing") {
		t.Fatalf("integrityWarnings = %+v, did not expect missing process evidence", got.ContextBudget.IntegrityWarnings)
	}
	if got.ContextBudget.ContextGateAction == providerContextGateFailFast {
		t.Fatalf("contextGateAction = %q, did not expect fail_fast", got.ContextBudget.ContextGateAction)
	}
}

func TestEnforceProviderRequestBudgetAcceptsReplanContinuationAsProcessEvidence(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-replan-continuation",
		RunID:       "run-replan-continuation",
		StepID:      "run-replan-continuation/follow-up-graph-planning-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"graphFollowUp": true,
			"executionContextPackage": map[string]any{
				"replanContinuation": map[string]any{
					"currentProgressDigest": map[string]any{
						"currentGap":         "completionState continue has no actionable continuation",
						"nextRequiredAction": "address_last_failure_under_current_stage_contract",
					},
					"failureContext": map[string]any{
						"failedStepId":     "run-replan-continuation/repair-010",
						"failureClass":     "runtime_failed",
						"failureSignature": "class=runtime_failed|kind=llm/workspace_execution",
					},
					"processEvidenceTimeline": []map[string]any{{
						"stepId":       "run-replan-continuation/graph-001/001-run-tests",
						"kind":         "shell/command",
						"state":        "failed",
						"failureClass": "shell_command_failed",
					}},
				},
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget = nil, want integrity audit")
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "runtime_repair_process_evidence_missing") {
		t.Fatalf("integrityWarnings = %+v, did not expect missing process evidence", got.ContextBudget.IntegrityWarnings)
	}
	if got.ContextBudget.ContextGateAction == providerContextGateFailFast {
		t.Fatalf("contextGateAction = %q, did not expect fail_fast", got.ContextBudget.ContextGateAction)
	}
}

func TestEnforceProviderRequestBudgetPreservesRepairContractWithoutSnapshots(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-repair-contract",
		RunID:       "run-repair-contract",
		StepID:      "run-repair-contract/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"currentSoulStage":     "execution",
			"failedExecutionGraph": repeatedBudgetItems(200),
			"graphEvidence":        repeatedBudgetItems(200),
			"repairActionPackage": map[string]any{
				"coverageRepairRequired": true,
				"coverageTargetSet": []map[string]any{{
					"kind":                "dod",
					"coverageTargetToken": "target-1",
					"targetText":          "target one",
				}},
				"currentExecutionProgress": map[string]any{
					"coverageGapCount": 1,
				},
				"requiredOutput": map[string]any{
					"mustDeclareCoverageTargets": true,
					"mustReturnValidJSON":        true,
				},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    4096,
				MaxInputTokens:         256,
				ReservedOutputTokens:   128,
				BytesHardLimit:         8 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil || got.ContextBudget.ShrinkLevel == 0 {
		t.Fatalf("contextBudget = %+v, want compacted budget", got.ContextBudget)
	}
	pkg := recordFromAny(got.Input["repairActionPackage"])
	if len(mapSliceFromAny(pkg["coverageTargetSet"])) != 1 {
		t.Fatalf("repairActionPackage = %+v, want coverageTargetSet preserved without targetSnapshots", pkg)
	}
	required := recordFromAny(pkg["requiredOutput"])
	if !boolValue(required["mustDeclareCoverageTargets"]) || !boolValue(required["mustReturnValidJSON"]) {
		t.Fatalf("requiredOutput = %+v, want protected repair output contract", required)
	}
	progress := recordFromAny(pkg["currentExecutionProgress"])
	if intValue(progress["coverageGapCount"]) != 1 {
		t.Fatalf("currentExecutionProgress = %+v, want preserved progress", progress)
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "protected_repair_action_target_snapshot_missing_after_context_budget") {
		t.Fatalf("integrityWarnings = %+v, want no target snapshot warning when original repair package had no targetSnapshots", got.ContextBudget.IntegrityWarnings)
	}
	if got.ContextBudget.ContextGateReason == "protected_repair_action_target_snapshot_missing_after_context_budget" {
		t.Fatalf("contextGateReason = %q warnings=%+v, want no target snapshot gate when original repair package had no targetSnapshots", got.ContextBudget.ContextGateReason, got.ContextBudget.IntegrityWarnings)
	}
}

func TestEnforceProviderRequestBudgetPreservesCurrentActionContract(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-current-action-budget",
		RunID:       "run-current-action-budget",
		StepID:      "run-current-action-budget/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"currentSoulStage":            "execution",
			"runtimeRepairFollowUp":       true,
			"stateChangingOutputRequired": true,
			"repairObjective":             "produce_state_changing_output",
			"currentActionContract": map[string]any{
				"schemaVersion": "workorch.current_action_contract.v1",
				"workspaceId":   "ws-current-action-budget",
				"runId":         "run-current-action-budget",
				"stepId":        "run-current-action-budget/repair-001",
				"nextAction": map[string]any{
					"kind": "state_change",
				},
				"currentFocus": map[string]any{
					"id":                   "cwi-current-action",
					"sourceRequirementId":  "deliverable-current-action",
					"targetText":           "deliverable current action must change",
					"missingEvidenceKinds": []string{"state_change"},
				},
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
					"coverageWorkItemIds":          []string{"cwi-current-action"},
					"structuredTargetRefs":         []string{"deliverable-current-action"},
				},
				"latestRejectedDiagnostic": map[string]any{
					"violationType": "repair_no_state_changing_output",
					"violatedRule":  "state_changing_required_missing",
				},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
				},
				"currentFailure": map[string]any{
					"failureClass":  "runtime_failed",
					"failureReason": "previous output returned read-only graph",
				},
				"currentExecutionProgress": map[string]any{
					"currentGap": "needs direct state change",
				},
			},
			"failedExecutionGraph":    repeatedBudgetItems(400),
			"graphEvidence":           repeatedBudgetItems(400),
			"processEvidenceTimeline": repeatedBudgetItems(400),
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    4096,
				MaxInputTokens:         256,
				ReservedOutputTokens:   128,
				BytesHardLimit:         8 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil || got.ContextBudget.ShrinkLevel == 0 {
		t.Fatalf("contextBudget = %+v, want compacted budget", got.ContextBudget)
	}
	contract := recordFromAny(got.Input["currentActionContract"])
	if len(contract) == 0 {
		t.Fatalf("input = %+v, want currentActionContract preserved", got.Input)
	}
	required := recordFromAny(contract["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) || !stringSliceContains(stringSliceValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("currentActionContract.requiredOutput = %+v, want state-changing patch contract", required)
	}
	focus := recordFromAny(contract["currentFocus"])
	if stringValue(focus["id"]) != "cwi-current-action" {
		t.Fatalf("currentActionContract.currentFocus = %+v, want cwi focus preserved", focus)
	}
	if !stringSliceContains(got.ContextBudget.ProtectedFields, "input.currentActionContract") {
		t.Fatalf("protectedFields = %+v, want currentActionContract protected", got.ContextBudget.ProtectedFields)
	}
	if stringSliceContains(got.ContextBudget.IntegrityWarnings, "protected_runtime_contract_changed_after_context_budget") {
		t.Fatalf("integrityWarnings = %+v, want current action contract preserved without runtime contract warning", got.ContextBudget.IntegrityWarnings)
	}
}

func TestEnforceProviderRequestBudgetNarrowsCurrentActionInputBeforeHardLimit(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-current-action-narrow",
		RunID:       "run-current-action-narrow",
		StepID:      "run-current-action-narrow/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"currentSoulStage":      "execution",
			"runtimeRepairFollowUp": true,
			"currentActionContract": map[string]any{
				"schemaVersion": "workorch.current_action_contract.v1",
				"nextAction":    map[string]any{"kind": "state_change"},
				"currentFocus":  map[string]any{"id": "cwi-current-action-narrow", "targetText": "narrow focus"},
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
				},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
				},
				"currentFailure": map[string]any{
					"failureClass": "runtime_failed",
				},
				"currentExecutionProgress": map[string]any{
					"currentGap": "needs current action retry",
				},
			},
			"failedExecutionGraph":    repeatedBudgetItems(500),
			"graphEvidence":           repeatedBudgetItems(500),
			"runEvidenceSummary":      repeatedBudgetItems(500),
			"processEvidenceTimeline": repeatedBudgetItems(500),
			"workspaceFileManifest":   repeatedBudgetItems(500),
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    128000,
				MaxInputTokens:         64000,
				ReservedOutputTokens:   1024,
				BytesHardLimit:         512 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}
	before := providerRequestJSONBytes(req)

	got := enforceProviderRequestBudget(req)
	after := providerRequestJSONBytes(got)
	if got.ContextBudget == nil || got.ContextBudget.ShrinkLevel == 0 {
		t.Fatalf("contextBudget = %+v, want current-action narrowing even before hard limit", got.ContextBudget)
	}
	if !stringSliceContains(got.ContextBudget.CompressedSections, "current_action") {
		t.Fatalf("compressedSections = %+v, want current_action", got.ContextBudget.CompressedSections)
	}
	if after >= before/2 {
		t.Fatalf("request bytes before=%d after=%d, want material current-action narrowing", before, after)
	}
	if len(recordFromAny(got.Input["currentActionContract"])) == 0 {
		t.Fatalf("input = %+v, want currentActionContract preserved", got.Input)
	}
	if _, ok := got.Input["graphEvidence"]; ok {
		t.Fatalf("input retained graphEvidence under current-action narrowing: %+v", got.Input["graphEvidence"])
	}
	if _, ok := got.Input["processEvidenceTimeline"]; ok {
		t.Fatalf("input retained processEvidenceTimeline under current-action narrowing: %+v", got.Input["processEvidenceTimeline"])
	}
}

func TestEnforceProviderRequestBudgetContractRetryTailIsBoundedAndMonotonic(t *testing.T) {
	for _, itemCount := range []int{40, 89} {
		t.Run(fmt.Sprintf("coverage-%d", itemCount), func(t *testing.T) {
			actualBytes := make([]int, 0, 3)
			for attempt := 1; attempt <= 3; attempt++ {
				req := providerContractRetryBudgetFixture(itemCount, attempt)
				got := enforceProviderRequestBudget(req)
				if got.ContextBudget == nil {
					t.Fatal("contextBudget = nil")
				}
				wantShrink := attempt + 1
				if got.ContextBudget.ShrinkLevel < wantShrink {
					t.Fatalf("attempt %d shrinkLevel = %d, want >= %d", attempt, got.ContextBudget.ShrinkLevel, wantShrink)
				}
				if got.ContextBudget.ActualBytes > defaultProviderRequestBudgetBytes || got.ContextBudget.OverBudget {
					t.Fatalf("attempt %d budget = %+v, want <= %d bytes", attempt, got.ContextBudget, defaultProviderRequestBudgetBytes)
				}
				if got.ContextBudget.ContextGateAction != providerContextGatePass || len(got.ContextBudget.IntegrityWarnings) > 0 {
					t.Fatalf("attempt %d context gate = %s warnings=%+v", attempt, got.ContextBudget.ContextGateAction, got.ContextBudget.IntegrityWarnings)
				}
				contract := recordFromAny(got.Input["currentActionContract"])
				if stringValue(recordFromAny(contract["currentFocus"])["id"]) != "cwi-001" ||
					!boolValue(recordFromAny(contract["requiredOutput"])["mustProduceConcreteValidationEvidence"]) {
					t.Fatalf("attempt %d currentActionContract = %+v, want protected focus/requiredOutput", attempt, contract)
				}
				if stringValue(recordFromAny(got.Input["goal"])["id"]) != "goal-live" ||
					stringValue(recordFromAny(got.Input["dod"])["id"]) != "dod-live" {
					t.Fatalf("attempt %d goal/dod changed: goal=%+v dod=%+v", attempt, got.Input["goal"], got.Input["dod"])
				}
				actualBytes = append(actualBytes, got.ContextBudget.ActualBytes)
			}
			if actualBytes[1] > actualBytes[0] || actualBytes[2] > actualBytes[1] {
				t.Fatalf("retry bytes = %+v, want monotonic non-increasing", actualBytes)
			}
		})
	}
}

func providerContractRetryBudgetFixture(itemCount int, attempt int) llmmodels.ProviderRequest {
	focus := map[string]any{
		"id":                           "cwi-001",
		"definitionRevision":           "definition-live",
		"sourceScope":                  "dod",
		"sourceRequirementId":          "dod-001",
		"targetText":                   "validate current behavior",
		"currentGap":                   "concrete validation evidence is missing",
		"status":                       "in_progress",
		"requiredEvidenceKinds":        []string{"validation"},
		"missingEvidenceKinds":         []string{"validation"},
		"acceptedEvidenceRefs":         []string{},
		"rejectedEvidenceRefs":         []string{"rejected-validation-001"},
		"nextAllowedConvergenceStates": []string{"need_validation", "need_assessment"},
	}
	items := make([]map[string]any, 0, itemCount)
	for idx := 0; idx < itemCount; idx++ {
		items = append(items, map[string]any{
			"id":                   fmt.Sprintf("cwi-%03d", idx+1),
			"definitionRevision":   "definition-live",
			"sourceScope":          "dod",
			"sourceRequirementId":  fmt.Sprintf("dod-%03d", idx+1),
			"targetText":           strings.Repeat(fmt.Sprintf("coverage item %03d ", idx+1), 40),
			"currentGap":           strings.Repeat("missing concrete evidence ", 30),
			"status":               "open",
			"missingEvidenceKinds": []string{"validation"},
		})
	}
	coverageContext := map[string]any{
		"definitionRevision": "definition-live",
		"currentFocus":       focus,
		"currentFocusItems":  items,
		"unresolvedItems":    items,
		"ledgerSummary":      items,
		"summary":            map[string]any{"open": itemCount, "in_progress": 1},
		"fullLedgerRef":      "evidence://coverage/full-ledger",
	}
	required := map[string]any{
		"mustProduceConcreteValidationEvidence": true,
		"mustDeclareValidationEvidenceMode":     true,
		"acceptedKinds":                         []string{"validation/managed_session"},
		"forbiddenKinds":                        []string{"patch/apply", "file/write", "git/write"},
	}
	progress := map[string]any{
		"failureReason":    "validation_static_only",
		"failureClass":     "runtime_failed",
		"failureSignature": "class=runtime_failed|rule=validation_static_only",
		"currentFailure": map[string]any{
			"stage":               "llm_execution_graph_plan",
			"violatedRule":        "validation_static_only",
			"actualOutputSummary": strings.Repeat("static validation output ", 200),
		},
		"lastObservedFailure":    map[string]any{"reason": strings.Repeat("same rejected validation shape ", 200)},
		"latestActionEvidence":   repeatedBudgetItems(itemCount),
		"completedGraphEvidence": repeatedBudgetItems(itemCount),
	}
	return llmmodels.ProviderRequest{
		WorkspaceID: "ws-live",
		RunID:       "run-live",
		StepID:      fmt.Sprintf("run-live/contract-retry-%03d", attempt),
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"workspaceId":            "ws-live",
			"runtimeRepairFollowUp":  true,
			"runtimeRepairAttempt":   attempt,
			"contractViolationRetry": true,
			"runtimeStage":           "verification",
			"repairObjective":        "produce_concrete_validation_evidence",
			"goal":                   map[string]any{"id": "goal-live", "content": strings.Repeat("stable goal ", 100)},
			"dod":                    map[string]any{"id": "dod-live", "content": strings.Repeat("stable dod ", 100)},
			"currentActionContract": map[string]any{
				"schemaVersion":  "workorch.current_action_contract.v1",
				"nextAction":     map[string]any{"kind": "validate"},
				"currentFocus":   focus,
				"requiredOutput": required,
			},
			"coverageWorkItemContext": coverageContext,
			"contractViolationRecovery": map[string]any{
				"violatedRule":        "validation_static_only",
				"attempt":             attempt,
				"maxAttempts":         10,
				"correctionProfileId": "managed_validation_evidence",
				"correctionMode":      "forced_narrow",
				"expectedContract":    map[string]any{"historical": strings.Repeat("old expected contract ", 2000)},
				"actualOutputSummary": map[string]any{"summary": strings.Repeat("rejected output ", 2000)},
				"correctionContract":  map[string]any{"requiredOutputShape": "managed_validation_evidence"},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput":                required,
				"coverageRepairRequired":        true,
				"coverageTargetSet":             items,
				"coverageWorkItemContext":       coverageContext,
				"currentFailure":                progress["currentFailure"],
				"currentExecutionProgress":      progress,
				"lastFailureDiagnostics":        map[string]any{"details": strings.Repeat("historical diagnostics ", 2000)},
				"previousRejectedOutputSummary": map[string]any{"content": strings.Repeat("old rejected output ", 2000)},
			},
			"stagePacket": map[string]any{
				"schemaVersion":           "workorch.stage_packet.v1",
				"workspaceId":             "ws-live",
				"runId":                   "run-live",
				"stepId":                  fmt.Sprintf("run-live/contract-retry-%03d", attempt),
				"activeStage":             "verification",
				"activeFocus":             focus,
				"requiredOutput":          required,
				"processEvidenceTimeline": repeatedBudgetItems(itemCount),
			},
			"processEvidenceTimeline": repeatedBudgetItems(itemCount),
			"evidenceLedger":          repeatedBudgetItems(itemCount),
			"failedExecutionGraph":    repeatedBudgetItems(itemCount),
			"graphEvidence":           repeatedBudgetItems(itemCount),
		},
		Config: llmmodels.RedactedProviderConfig{ContextProfile: llmmodels.ProviderContextProfile{
			ContextWindowTokens:    128000,
			MaxInputTokens:         24576,
			ReservedOutputTokens:   4096,
			BytesHardLimit:         defaultProviderRequestBudgetBytes,
			EstimatedTokensPerByte: 0.25,
		}},
	}
}

func TestEnforceProviderRequestBudgetPreservesStagePacketCriticalFields(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-stage-packet-budget",
		RunID:       "run-stage-packet-budget",
		StepID:      "run-stage-packet-budget/step-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"currentSoulStage":     "execution",
			"failedExecutionGraph": repeatedBudgetItems(300),
			"graphEvidence":        repeatedBudgetItems(300),
			"previousResults":      repeatedBudgetItems(300),
			"stagePacket": map[string]any{
				"schemaVersion": "workorch.stage_packet.v1",
				"workspaceId":   "ws-stage-packet-budget",
				"runId":         "run-stage-packet-budget",
				"stepId":        "run-stage-packet-budget/step-001",
				"definition": map[string]any{
					"revision": "definition-stage-budget",
					"goalId":   "goal-stage-budget",
					"doDId":    "dod-stage-budget",
				},
				"activeStage": "state_change",
				"activeFocus": map[string]any{
					"coverageItemId": "coverage-stage-budget",
					"currentGap":     "produce accepted evidence",
				},
				"requiredOutput": map[string]any{
					"mustUseCurrentTargetSnapshot": true,
					"mustBindAcceptedEvidenceRefs": true,
					"targetRef":                    "target-stage-budget",
				},
				"allowedActions":        []string{"state_change"},
				"forbiddenActions":      []string{"complete_without_evidence"},
				"requiredEvidenceKinds": []string{"state_change", "validation"},
				"acceptedEvidenceRefs":  []string{"accepted-stage-1", "accepted-stage-2"},
				"rejectedEvidenceRefs":  []string{"rejected-stage-1"},
				"idempotencyKey":        "stage-packet-budget-key",
				"targetSnapshots": []map[string]any{{
					"targetRef":             "target-stage-budget",
					"content":               "snapshot body must not be protected through stagePacket",
					"contentPreview":        "snapshot preview must not be protected through stagePacket",
					"contentHash":           "sha256-stage-budget",
					"contentComplete":       false,
					"safeForFileWrite":      false,
					"safeForExactFileWrite": false,
					"truncated":             true,
					"integrityState":        "truncated",
					"source":                "stagePacket",
					"contextSource":         "runtime",
				}},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    4096,
				MaxInputTokens:         256,
				ReservedOutputTokens:   128,
				BytesHardLimit:         8 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil || got.ContextBudget.ShrinkLevel < 4 {
		t.Fatalf("contextBudget = %+v, want aggressive compaction", got.ContextBudget)
	}
	for _, expected := range []string{
		"input.stagePacket",
		"input.stagePacket.requiredOutput",
		"input.stagePacket.requiredEvidenceKinds",
		"input.stagePacket.acceptedEvidenceRefs",
		"input.stagePacket.rejectedEvidenceRefs",
		"input.stagePacket.targetSnapshots",
	} {
		if !stringSliceContains(got.ContextBudget.ProtectedFields, expected) {
			t.Fatalf("protectedFields = %+v, missing %s", got.ContextBudget.ProtectedFields, expected)
		}
	}
	if len(got.ContextBudget.IntegrityWarnings) != 0 {
		t.Fatalf("integrityWarnings = %+v, want none", got.ContextBudget.IntegrityWarnings)
	}
	packet := recordFromAny(got.Input["stagePacket"])
	required := recordFromAny(packet["requiredOutput"])
	if !boolValue(required["mustUseCurrentTargetSnapshot"]) ||
		!boolValue(required["mustBindAcceptedEvidenceRefs"]) ||
		stringValue(required["targetRef"]) != "target-stage-budget" {
		t.Fatalf("stagePacket.requiredOutput = %+v, want critical output contract preserved", required)
	}
	if gotRefs := stringSliceFromAny(packet["acceptedEvidenceRefs"]); !stringSliceContains(gotRefs, "accepted-stage-1") || !stringSliceContains(gotRefs, "accepted-stage-2") {
		t.Fatalf("stagePacket.acceptedEvidenceRefs = %+v, want refs preserved", packet["acceptedEvidenceRefs"])
	}
	if gotRefs := stringSliceFromAny(packet["rejectedEvidenceRefs"]); !stringSliceContains(gotRefs, "rejected-stage-1") {
		t.Fatalf("stagePacket.rejectedEvidenceRefs = %+v, want refs preserved", packet["rejectedEvidenceRefs"])
	}
	if gotKinds := stringSliceFromAny(packet["requiredEvidenceKinds"]); !stringSliceContains(gotKinds, "state_change") || !stringSliceContains(gotKinds, "validation") {
		t.Fatalf("stagePacket.requiredEvidenceKinds = %+v, want evidence kinds preserved", packet["requiredEvidenceKinds"])
	}
	snapshots := mapSliceFromAny(packet["targetSnapshots"])
	if len(snapshots) != 1 {
		t.Fatalf("stagePacket.targetSnapshots = %+v, want one metadata snapshot", packet["targetSnapshots"])
	}
	snapshot := snapshots[0]
	if stringValue(snapshot["targetRef"]) != "target-stage-budget" ||
		stringValue(snapshot["contentHash"]) != "sha256-stage-budget" ||
		stringValue(snapshot["integrityState"]) != "truncated" ||
		boolValue(snapshot["safeForExactFileWrite"]) {
		t.Fatalf("stagePacket.targetSnapshots[0] = %+v, want integrity metadata preserved", snapshot)
	}
	if _, ok := snapshot["content"]; ok {
		t.Fatalf("stagePacket target snapshot leaked content: %+v", snapshot)
	}
	if _, ok := snapshot["contentPreview"]; ok {
		t.Fatalf("stagePacket target snapshot leaked contentPreview: %+v", snapshot)
	}
}

func TestMissingProtectedRepairSnapshotWarningsRequireOriginalRepairSnapshots(t *testing.T) {
	withoutSnapshots := missingProtectedRepairSnapshotWarnings(
		map[string]any{"repairActionPackage": map[string]any{"requiredOutput": map[string]any{"mustReturnValidJSON": true}}},
		protectedProviderRepairTargets{RepairActionPackage: map[string]any{"requiredOutput": map[string]any{"mustReturnValidJSON": true}}},
	)
	if stringSliceContains(withoutSnapshots, "protected_repair_action_target_snapshot_missing_after_context_budget") {
		t.Fatalf("warnings = %+v, want no target snapshot warning for package-only protected fields", withoutSnapshots)
	}

	withSnapshots := missingProtectedRepairSnapshotWarnings(
		map[string]any{"repairActionPackage": map[string]any{"requiredOutput": map[string]any{"mustReturnValidJSON": true}}},
		protectedProviderRepairTargets{
			RepairActionPackage:         map[string]any{"targetSnapshots": []any{map[string]any{"targetRef": "resource-1.txt", "content": "current resource\n"}}},
			RepairActionTargetSnapshots: []any{map[string]any{"targetRef": "resource-1.txt", "content": "current resource\n"}},
		},
	)
	if !stringSliceContains(withSnapshots, "protected_repair_action_target_snapshot_missing_after_context_budget") {
		t.Fatalf("warnings = %+v, want missing target snapshot warning when original snapshots were protected", withSnapshots)
	}
}

func TestEnforceProviderRequestBudgetMarksP0OverflowWhenGoalDoDContractDominates(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-p0",
		RunID:       "run-p0",
		StepID:      "run-p0/step-1",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"executionContext": map[string]any{
				"goal": strings.Repeat("goal ", 1200),
				"dod":  strings.Repeat("dod ", 1200),
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    2048,
				MaxInputTokens:         200,
				ReservedOutputTokens:   128,
				SafetyRatio:            0.8,
				BytesHardLimit:         512 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}
	got := enforceProviderRequestBudget(req)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget metadata missing")
	}
	if !got.ContextBudget.P0Overflow {
		t.Fatalf("contextBudget = %+v, want p0 overflow on protected goal/dod contract", got.ContextBudget)
	}
	if got.ContextBudget.GoalDoDCompressed {
		t.Fatalf("contextBudget = %+v, want goal/dod preserved", got.ContextBudget)
	}
}

func TestEnforceProviderRequestBudgetDoesNotTreatReplanContinuationAsProtectedGoalDoD(t *testing.T) {
	req := llmmodels.ProviderRequest{
		WorkspaceID: "ws-replan-budget",
		RunID:       "run-replan-budget",
		StepID:      "run-replan-budget/repair-001",
		Kind:        "llm/workspace_execution",
		Model:       "model",
		Schema:      decisionSchemaName,
		Input: map[string]any{
			"runtimeRepairFollowUp": true,
			"runtimeStage":          "repair",
			"executionContextPackage": map[string]any{
				"goal": map[string]any{"text": "finish the current goal"},
				"dod":  map[string]any{"items": []any{"validated"}},
				"replanContinuation": map[string]any{
					"predecessorRunId":        "run-001",
					"reason":                  "decision_replan",
					"processEvidenceTimeline": repeatedBudgetItems(300),
					"currentProgressDigest": map[string]any{
						"currentGap": strings.Repeat("historical gap ", 400),
					},
					"failureContext": map[string]any{
						"failedStepId":  "run-001/repair-010",
						"failureReason": strings.Repeat("historical failure ", 400),
					},
				},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustReturnCorrectedExecutionGraph": true,
				},
				"currentFailure": map[string]any{
					"failureClass":  "runtime_failed",
					"failureReason": "previous repair attempt failed",
				},
				"currentExecutionProgress": map[string]any{
					"currentGap": "need a bounded repair graph",
				},
			},
		},
		Config: llmmodels.RedactedProviderConfig{
			ContextProfile: llmmodels.ProviderContextProfile{
				ContextWindowTokens:    8192,
				MaxInputTokens:         2048,
				ReservedOutputTokens:   1024,
				SafetyRatio:            0.8,
				BytesHardLimit:         14 * 1024,
				EstimatedTokensPerByte: 0.25,
			},
		},
	}

	got := enforceProviderRequestBudgetLimit(req, 14*1024)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget metadata missing")
	}
	if got.ContextBudget.P0Overflow || got.ContextBudget.ContextGateAction == providerContextGateFailFast {
		t.Fatalf("contextBudget = %+v, want replan continuation compacted without P0 overflow gate", got.ContextBudget)
	}
	pkg := recordFromAny(got.Input["executionContextPackage"])
	if len(recordFromAny(pkg["goal"])) == 0 || len(recordFromAny(pkg["dod"])) == 0 {
		t.Fatalf("executionContextPackage = %+v, want goal/dod preserved", pkg)
	}
	replan := recordFromAny(pkg["replanContinuation"])
	if len(replan) != 0 {
		t.Fatalf("executionContextPackage.replanContinuation = %+v, want omitted from protected Goal/DoD budget floor", replan)
	}
}

func TestProviderSemanticContextWarningsRejectMemoryMismatchAndRuntimeTargets(t *testing.T) {
	warnings := providerSemanticContextWarnings(map[string]any{
		"executionContextPackage": map[string]any{
			"selectedContext": map[string]any{
				"memories": []map[string]any{{"id": "memory-old"}},
				"memoryRetrieval": map[string]any{
					"selectedMemoryIds": []string{},
					"excludedMemoryIds": []string{"memory-old"},
				},
			},
		},
		"repairActionPackage": map[string]any{
			"repairTargetSet": []string{"home/user/project/.workorch-workspaces/ws-1"},
			"targetSnapshots": []map[string]any{{
				"targetRef": "llmexecutiondecision.plan",
				"exists":    true,
			}},
		},
	})
	for _, want := range []string{
		"selected_context_memories_without_selected_memory_ids",
		"selected_context_memories_include_unselected_or_excluded_memory",
		"inadmissible_repair_target_set",
		"inadmissible_repair_target_snapshot",
	} {
		if !stringSliceContains(warnings, want) {
			t.Fatalf("warnings = %+v, want %s", warnings, want)
		}
	}
}

func TestCaptureAndRestoreProtectedGoalDoD(t *testing.T) {
	input := map[string]any{
		"executionContext": map[string]any{
			"goal": map[string]any{"text": "goal"},
			"dod":  map[string]any{"items": []any{"d1"}},
		},
	}
	protected := captureProtectedGoalDoD(input)
	if len(protected.ExecutionContext) == 0 {
		t.Fatalf("protected = %+v, want executionContext goal/dod captured", protected)
	}
	restored := restoreProtectedGoalDoD(map[string]any{"executionContext": map[string]any{}}, protected)
	exec := recordFromAny(restored["executionContext"])
	if len(recordFromAny(exec["goal"])) == 0 || len(recordFromAny(exec["dod"])) == 0 {
		t.Fatalf("restored executionContext = %+v, want goal/dod restored", exec)
	}
}

func repeatedBudgetItems(count int) []any {
	out := make([]any, 0, count)
	for idx := 0; idx < count; idx++ {
		out = append(out, map[string]any{
			"id":      idx,
			"summary": strings.Repeat("evidence ", 200),
		})
	}
	return out
}

func providerBudgetCoverageWorkItemContextForTest(id string, targetText string) map[string]any {
	return map[string]any{
		"schemaVersion":        "workorch.coverage_work_item_context.v1",
		"definitionRevision":   "rev-test",
		"coverageBaselineHash": "sha256:test",
		"summary": map[string]any{
			"total": 1,
			"open":  1,
		},
		"currentFocus": map[string]any{
			"id":                           id,
			"definitionRevision":           "rev-test",
			"sourceScope":                  "goal_leaf",
			"sourceRequirementId":          id + "-requirement",
			"normalizedSourceKey":          "goal:goal_leaf:" + id,
			"targetText":                   targetText,
			"status":                       "open",
			"currentGap":                   "missing accepted evidence binding for required evidence kind: validation",
			"requiredEvidenceKinds":        []string{"validation"},
			"missingEvidenceKinds":         []string{"validation"},
			"acceptedEvidenceRefs":         []string{},
			"rejectedEvidenceRefs":         []string{},
			"nextAllowedConvergenceStates": []string{"need_validation"},
		},
	}
}
