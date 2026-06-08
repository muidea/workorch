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

func TestDecodeSnapshotInfoPreservesWorkspaceGoalsAndDoDs(t *testing.T) {
	snapshot, err := decodeSnapshotInfo(map[string]any{
		"workspace": map[string]any{"id": "ws-1", "state": "active"},
		"goals": []any{
			map[string]any{"id": "goal-1", "workspaceId": "ws-1", "text": "goal text"},
		},
		"dods": []any{
			map[string]any{"id": "dod-1", "workspaceId": "ws-1", "items": []any{"dod item"}, "state": "ready"},
		},
	})
	if err != nil {
		t.Fatalf("decodeSnapshotInfo: %v", err)
	}
	if snapshot == nil || len(snapshot.Goals) != 1 || snapshot.Goals[0].ID != "goal-1" || snapshot.Goals[0].Text != "goal text" {
		t.Fatalf("goals = %+v, want decoded workspace goal", snapshot)
	}
	if len(snapshot.DoDs) != 1 || snapshot.DoDs[0].ID != "dod-1" || len(snapshot.DoDs[0].Items) != 1 || snapshot.DoDs[0].Items[0] != "dod item" {
		t.Fatalf("dods = %+v, want decoded workspace DoD", snapshot.DoDs)
	}
}

func TestDecodeSnapshotInfoPreservesClientSnapshotWorkspaceGoalsAndDoDs(t *testing.T) {
	snapshot, err := decodeSnapshotInfo(map[string]any{
		"workspace": map[string]any{"id": "ws-1", "state": "active"},
		"goals": []any{
			map[string]any{"id": "goal-1", "workspaceId": "ws-1", "text": "client goal"},
		},
		"dods": []any{
			map[string]any{"id": "dod-1", "workspaceId": "ws-1", "items": []any{"client dod"}, "state": "ready"},
		},
	})
	if err != nil {
		t.Fatalf("decodeSnapshotInfo: %v", err)
	}
	if snapshot == nil || len(snapshot.Goals) != 1 || snapshot.Goals[0].Text != "client goal" {
		t.Fatalf("goals = %+v, want decoded client snapshot goal", snapshot)
	}
	if len(snapshot.DoDs) != 1 || len(snapshot.DoDs[0].Items) != 1 || snapshot.DoDs[0].Items[0] != "client dod" {
		t.Fatalf("dods = %+v, want decoded client snapshot DoD", snapshot.DoDs)
	}
}
