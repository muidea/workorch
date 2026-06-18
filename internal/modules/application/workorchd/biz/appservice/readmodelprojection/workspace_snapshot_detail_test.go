package readmodelprojection

import (
	"strconv"
	"strings"
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
		traces = append(traces, readmodelcore.LLMUsageTrace{
			ID:    "usage-" + strconv.Itoa(idx),
			RunID: "run-11",
			ContextBudget: map[string]any{
				"failureSignature": "prefix-" + strconv.Itoa(idx) + "-" + strings.Repeat("x", workspaceSnapshotTraceStringLimit+200),
				"recentFailures":   []any{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve", "thirteen"},
			},
		})
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
	if signature := got.LLMUsageTraces[0].ContextBudget["failureSignature"].(string); len(signature) > workspaceSnapshotTraceStringLimit+len("...[truncated]") {
		t.Fatalf("context budget failureSignature length = %d, want bounded", len(signature))
	}
	if recent := got.LLMUsageTraces[0].ContextBudget["recentFailures"].([]any); len(recent) != workspaceSnapshotTraceSliceLimit {
		t.Fatalf("context budget recentFailures = %d, want %d", len(recent), workspaceSnapshotTraceSliceLimit)
	}
	if got.LLMUsageSummary.EntryTotal != len(traces) {
		t.Fatalf("summary entry total = %d, want full total %d", got.LLMUsageSummary.EntryTotal, len(traces))
	}
}

func TestProjectWorkspaceSnapshotDetailCompactsContextAndRunDerivedCollections(t *testing.T) {
	t.Parallel()

	sections := make([]readmodelcore.ExecutionSection, 0, workspaceSnapshotDetailExecutionSectionLimit+3)
	for idx := 0; idx < workspaceSnapshotDetailExecutionSectionLimit+3; idx++ {
		sections = append(sections, readmodelcore.ExecutionSection{
			ID:                 "section-" + strconv.Itoa(idx),
			Summary:            strings.Repeat("s", workspaceSnapshotTraceStringLimit+200),
			ActualEvidenceRefs: repeatedStrings("evidence", workspaceSnapshotTraceSliceLimit+3),
		})
	}
	recalls := make([]readmodelcore.MemoryRecallTrace, 0, workspaceSnapshotDetailMemoryRecallLimit+3)
	for idx := 0; idx < workspaceSnapshotDetailMemoryRecallLimit+3; idx++ {
		recalls = append(recalls, readmodelcore.MemoryRecallTrace{
			ID:                  "memory-" + strconv.Itoa(idx),
			QueryTextPreview:    strings.Repeat("q", workspaceSnapshotTraceStringLimit+200),
			CandidateMemoryIDs:  repeatedStrings("candidate", workspaceSnapshotTraceSliceLimit+3),
			RetrievedMemoryHits: repeatedMemoryHits(workspaceSnapshotTraceSliceLimit + 3),
		})
	}
	budgets := make([]readmodelcore.LLMContextBudgetTrace, 0, workspaceSnapshotDetailLLMContextBudgetLimit+3)
	for idx := 0; idx < workspaceSnapshotDetailLLMContextBudgetLimit+3; idx++ {
		budgets = append(budgets, readmodelcore.LLMContextBudgetTrace{
			StepID:     "step-" + strconv.Itoa(idx),
			Rules:      repeatedStrings("rule", workspaceSnapshotTraceSliceLimit+3),
			SourceRefs: repeatedStrings("source", workspaceSnapshotTraceSliceLimit+3),
			Profile: map[string]any{
				"long": strings.Repeat("p", workspaceSnapshotTraceStringLimit+200),
			},
		})
	}

	got := ProjectWorkspaceSnapshotDetail(readmodelcore.WorkspaceSnapshot{
		Workspace:          readmodelcore.Workspace{ID: "ws-compact-derived"},
		ExecutionSections:  sections,
		MemoryRecallTraces: recalls,
		LLMContextBudgets:  budgets,
		ContextBundle: &readmodelcore.ContextBundle{
			WorkspaceID: "ws-compact-derived",
			Sections: repeatedContextSections(
				workspaceSnapshotDetailContextSectionLimit+3,
				workspaceSnapshotTraceSliceLimit+3,
			),
			RecentEvidence: repeatedContextEvidence(
				workspaceSnapshotDetailRecentEvidenceLimit+3,
				workspaceSnapshotTraceSliceLimit+3,
			),
			CompressionCandidates: repeatedCompressionCandidates(
				workspaceSnapshotDetailCompressionCandidateMax+3,
				workspaceSnapshotTraceSliceLimit+3,
			),
			CompressedContext: readmodelcore.CompressedContext{
				ResultRefs:         repeatedStrings("result", workspaceSnapshotTraceSliceLimit+3),
				SourceEvidenceRefs: repeatedStrings("source-evidence", workspaceSnapshotTraceSliceLimit+3),
			},
		},
	})

	if len(got.ExecutionSections) != workspaceSnapshotDetailExecutionSectionLimit {
		t.Fatalf("execution sections = %d, want %d", len(got.ExecutionSections), workspaceSnapshotDetailExecutionSectionLimit)
	}
	if got.ExecutionSections[0].ID != "section-3" {
		t.Fatalf("first execution section = %q, want latest retained", got.ExecutionSections[0].ID)
	}
	if len(got.ExecutionSections[0].ActualEvidenceRefs) != workspaceSnapshotTraceSliceLimit {
		t.Fatalf("execution evidence refs = %d, want %d", len(got.ExecutionSections[0].ActualEvidenceRefs), workspaceSnapshotTraceSliceLimit)
	}
	if len(got.MemoryRecallTraces) != workspaceSnapshotDetailMemoryRecallLimit {
		t.Fatalf("memory recall traces = %d, want %d", len(got.MemoryRecallTraces), workspaceSnapshotDetailMemoryRecallLimit)
	}
	if got.MemoryRecallTraces[0].ID != "memory-3" {
		t.Fatalf("first memory trace = %q, want latest retained", got.MemoryRecallTraces[0].ID)
	}
	if len(got.MemoryRecallTraces[0].CandidateMemoryIDs) != workspaceSnapshotTraceSliceLimit {
		t.Fatalf("candidate memory ids = %d, want %d", len(got.MemoryRecallTraces[0].CandidateMemoryIDs), workspaceSnapshotTraceSliceLimit)
	}
	if len(got.LLMContextBudgets) != workspaceSnapshotDetailLLMContextBudgetLimit {
		t.Fatalf("llm context budgets = %d, want %d", len(got.LLMContextBudgets), workspaceSnapshotDetailLLMContextBudgetLimit)
	}
	if got.LLMContextBudgets[0].StepID != "step-3" {
		t.Fatalf("first budget = %q, want latest retained", got.LLMContextBudgets[0].StepID)
	}
	if len(got.LLMContextBudgets[0].Rules) != workspaceSnapshotTraceSliceLimit {
		t.Fatalf("budget rules = %d, want %d", len(got.LLMContextBudgets[0].Rules), workspaceSnapshotTraceSliceLimit)
	}
	if got.ContextBundle == nil {
		t.Fatal("context bundle is nil")
	}
	if len(got.ContextBundle.Sections) != workspaceSnapshotDetailContextSectionLimit {
		t.Fatalf("context sections = %d, want %d", len(got.ContextBundle.Sections), workspaceSnapshotDetailContextSectionLimit)
	}
	if len(got.ContextBundle.Sections[0].EvidenceRefs) != workspaceSnapshotTraceSliceLimit {
		t.Fatalf("context section evidence refs = %d, want %d", len(got.ContextBundle.Sections[0].EvidenceRefs), workspaceSnapshotTraceSliceLimit)
	}
	if len(got.ContextBundle.RecentEvidence) != workspaceSnapshotDetailRecentEvidenceLimit {
		t.Fatalf("recent evidence = %d, want %d", len(got.ContextBundle.RecentEvidence), workspaceSnapshotDetailRecentEvidenceLimit)
	}
	if len(got.ContextBundle.CompressionCandidates) != workspaceSnapshotDetailCompressionCandidateMax {
		t.Fatalf("compression candidates = %d, want %d", len(got.ContextBundle.CompressionCandidates), workspaceSnapshotDetailCompressionCandidateMax)
	}
	if len(got.ContextBundle.CompressedContext.ResultRefs) != workspaceSnapshotTraceSliceLimit {
		t.Fatalf("compressed context result refs = %d, want %d", len(got.ContextBundle.CompressedContext.ResultRefs), workspaceSnapshotTraceSliceLimit)
	}
}

func repeatedStrings(prefix string, count int) []string {
	ret := make([]string, 0, count)
	for idx := 0; idx < count; idx++ {
		ret = append(ret, prefix+"-"+strconv.Itoa(idx))
	}
	return ret
}

func repeatedMemoryHits(count int) []readmodelcore.MemoryRecallHit {
	ret := make([]readmodelcore.MemoryRecallHit, 0, count)
	for idx := 0; idx < count; idx++ {
		ret = append(ret, readmodelcore.MemoryRecallHit{
			ID:      "memory-hit-" + strconv.Itoa(idx),
			Summary: strings.Repeat("h", workspaceSnapshotTraceStringLimit+200),
		})
	}
	return ret
}

func repeatedContextSections(count int, refs int) []readmodelcore.ContextSection {
	ret := make([]readmodelcore.ContextSection, 0, count)
	for idx := 0; idx < count; idx++ {
		ret = append(ret, readmodelcore.ContextSection{
			ID:           "context-section-" + strconv.Itoa(idx),
			Summary:      strings.Repeat("c", workspaceSnapshotTraceStringLimit+200),
			EvidenceRefs: repeatedStrings("context-evidence", refs),
			Data: map[string]any{
				"long": strings.Repeat("d", workspaceSnapshotTraceStringLimit+200),
			},
		})
	}
	return ret
}

func repeatedContextEvidence(count int, refs int) []readmodelcore.ContextEvidence {
	ret := make([]readmodelcore.ContextEvidence, 0, count)
	for idx := 0; idx < count; idx++ {
		ret = append(ret, readmodelcore.ContextEvidence{
			ID:           "context-evidence-" + strconv.Itoa(idx),
			Summary:      strings.Repeat("e", workspaceSnapshotTraceStringLimit+200),
			EvidenceRefs: repeatedStrings("evidence-ref", refs),
			Data: map[string]any{
				"long": strings.Repeat("v", workspaceSnapshotTraceStringLimit+200),
			},
		})
	}
	return ret
}

func repeatedCompressionCandidates(count int, refs int) []readmodelcore.ContextCompressionCandidate {
	ret := make([]readmodelcore.ContextCompressionCandidate, 0, count)
	for idx := 0; idx < count; idx++ {
		ret = append(ret, readmodelcore.ContextCompressionCandidate{
			SectionID:  "candidate-" + strconv.Itoa(idx),
			SourceRefs: repeatedStrings("candidate-source", refs),
		})
	}
	return ret
}
