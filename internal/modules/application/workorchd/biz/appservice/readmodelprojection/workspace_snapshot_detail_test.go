package readmodelprojection

import (
	"strconv"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func TestProjectWorkspaceSnapshotDetailCompactsHeavyFields(t *testing.T) {
	t.Parallel()

	runs := make([]readmodelcore.Run, 0, 12)
	for idx := 0; idx < 12; idx++ {
		runs = append(runs, readmodelcore.Run{
			ID:               "run-" + strconv.Itoa(idx),
			WorkspaceID:      "ws-compact",
			State:            "running",
			ExecutionContext: map[string]any{"large": "context"},
		})
	}
	traces := make([]readmodelcore.LLMUsageTrace, 0, workspaceSnapshotDetailLLMTraceLimit+3)
	for idx := 0; idx < workspaceSnapshotDetailLLMTraceLimit+3; idx++ {
		traces = append(traces, readmodelcore.LLMUsageTrace{ID: "usage-" + strconv.Itoa(idx), RunID: "run-11"})
	}

	got := ProjectWorkspaceSnapshotDetail(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID:               "ws-compact",
			ActiveRunID:      "run-0",
			ContextHints:     map[string]any{"large": "profile"},
			EvidencePolicies: []map[string]any{{"large": "policy"}},
		},
		Run:             &runs[11],
		Runs:            runs,
		LLMUsageTraces:  traces,
		LLMUsageSummary: readmodelcore.LLMUsageSummary{EntryTotal: len(traces)},
	})

	if got.Workspace.ContextHints != nil || got.Workspace.EvidencePolicies != nil {
		t.Fatalf("workspace heavy fields were not compacted: %+v", got.Workspace)
	}
	if got.Run == nil || got.Run.ExecutionContext != nil {
		t.Fatalf("active run execution context = %+v, want compact run", got.Run)
	}
	if len(got.Runs) != workspaceSnapshotDetailRunLimit+1 {
		t.Fatalf("runs = %d, want active plus latest %d", len(got.Runs), workspaceSnapshotDetailRunLimit)
	}
	if got.Runs[0].ID != "run-0" {
		t.Fatalf("first run = %q, want active run preserved", got.Runs[0].ID)
	}
	for _, item := range got.Runs {
		if item.ExecutionContext != nil {
			t.Fatalf("run %s execution context = %+v, want compact run", item.ID, item.ExecutionContext)
		}
	}
	if len(got.LLMUsageTraces) != workspaceSnapshotDetailLLMTraceLimit {
		t.Fatalf("llm usage traces = %d, want %d", len(got.LLMUsageTraces), workspaceSnapshotDetailLLMTraceLimit)
	}
	if got.LLMUsageTraces[0].ID != "usage-3" {
		t.Fatalf("first trace = %q, want latest traces retained", got.LLMUsageTraces[0].ID)
	}
	if got.LLMUsageSummary.EntryTotal != len(traces) {
		t.Fatalf("summary entry total = %d, want full total %d", got.LLMUsageSummary.EntryTotal, len(traces))
	}
}
