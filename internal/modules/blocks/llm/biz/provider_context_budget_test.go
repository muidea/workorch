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
	}

	got := enforceProviderRequestBudgetLimit(req, 12*1024)
	if got.ContextBudget == nil || !got.ContextBudget.Truncated {
		t.Fatalf("contextBudget = %+v, want truncated metadata", got.ContextBudget)
	}
	if got.ContextBudget.ActualBytes > got.ContextBudget.MaxBytes {
		t.Fatalf("actual bytes = %d, max = %d", got.ContextBudget.ActualBytes, got.ContextBudget.MaxBytes)
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
	}

	got := enforceProviderRequestBudgetLimit(req, 12*1024)
	if got.ContextBudget == nil {
		t.Fatal("contextBudget metadata missing")
	}
	if got.ContextBudget.Truncated {
		t.Fatalf("contextBudget = %+v, want non-truncated small request", got.ContextBudget)
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
