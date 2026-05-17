package llm

import (
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

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
