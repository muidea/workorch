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
	executionContext := recordFromAny(got.Input["executionContext"])
	if len(recordFromAny(executionContext["goal"])) == 0 || len(recordFromAny(executionContext["dod"])) == 0 {
		t.Fatalf("executionContext = %+v, want goal and DoD preserved", executionContext)
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
