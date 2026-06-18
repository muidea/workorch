package llm

import (
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

	if got.ContextBudget == nil || got.ContextBudget.ContextGateAction != providerContextGateFailFast {
		t.Fatalf("contextBudget = %+v, want fail_fast", got.ContextBudget)
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
	if got.ContextBudget.ContextGateAction != providerContextGateFailFast {
		t.Fatalf("contextGateAction = %q, want fail_fast", got.ContextBudget.ContextGateAction)
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
	if got.ContextBudget.ContextGateAction != providerContextGateFailFast {
		t.Fatalf("contextGateAction = %q, want fail_fast", got.ContextBudget.ContextGateAction)
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
	if got.ContextBudget.ContextGateAction != providerContextGateFailFast {
		t.Fatalf("contextGateAction = %q, want fail_fast", got.ContextBudget.ContextGateAction)
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
