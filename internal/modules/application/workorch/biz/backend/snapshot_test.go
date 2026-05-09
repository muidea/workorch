package backend

import "testing"

func TestDecodeSnapshotInfoPreservesRunExecutionContext(t *testing.T) {
	snapshot, err := decodeSnapshotInfo(map[string]any{
		"workspace": map[string]any{"id": "ws-a", "state": "running"},
		"runs": []any{
			map[string]any{
				"id":          "run-a",
				"workspaceId": "ws-a",
				"goalId":      "goal-latest",
				"doDId":       "dod-latest",
				"state":       "running",
				"executionContext": map[string]any{
					"goal": map[string]any{"text": "first goal\nsecond goal"},
					"dod":  map[string]any{"items": []any{"first dod", "second dod"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("decodeSnapshotInfo: %v", err)
	}
	if snapshot == nil || len(snapshot.Runs) != 1 {
		t.Fatalf("snapshot = %+v, want one run", snapshot)
	}
	goal, _ := snapshot.Runs[0].ExecutionContext["goal"].(map[string]any)
	if goal["text"] != "first goal\nsecond goal" {
		t.Fatalf("goal context = %+v, want cumulative goal text", goal)
	}
}

func TestDecodeSnapshotInfoPreservesLLMContextBudget(t *testing.T) {
	snapshot, err := decodeSnapshotInfo(map[string]any{
		"workspace": map[string]any{"id": "ws-budget", "state": "running"},
		"llmUsageTraces": []any{
			map[string]any{
				"id": "trace-1",
				"contextBudget": map[string]any{
					"maxTokens":            4096,
					"estimatedInputTokens": 1800,
					"includedSections":     []any{"p0", "turn-01"},
					"goalDoDCompressed":    false,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("decodeSnapshotInfo: %v", err)
	}
	if snapshot == nil || len(snapshot.LLMUsageTraces) != 1 {
		t.Fatalf("snapshot = %+v, want one llm usage trace", snapshot)
	}
	budget := snapshot.LLMUsageTraces[0].ContextBudget
	if budget == nil {
		t.Fatalf("contextBudget = nil, want decoded budget")
	}
	if budget["maxTokens"] != float64(4096) {
		t.Fatalf("contextBudget = %+v, want maxTokens preserved", budget)
	}
}
