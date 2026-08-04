package readmodelprojection

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mustSnapshotDetailPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	return payload
}

func TestProjectWorkspaceSnapshotDetailCompactsHeavyFields(t *testing.T) {
	t.Parallel()

	runs := make([]appmodelcore.Run, 0, 12)
	for idx := 0; idx < 12; idx++ {
		runs = append(runs, appmodelcore.Run{
			ID:          "run-" + strconv.Itoa(idx),
			WorkspaceID: "ws-compact",
			State:       "running",
		})
	}
	traces := make([]projectioncontract.LLMUsageTrace, 0, workspaceSnapshotDetailLLMTraceLimit+3)
	for idx := 0; idx < workspaceSnapshotDetailLLMTraceLimit+3; idx++ {
		traces = append(traces, projectioncontract.LLMUsageTrace{
			ID:    "usage-" + strconv.Itoa(idx),
			RunID: "run-11",
			ContextBudget: mustSnapshotDetailPayload(t, map[string]any{
				"failureSignature": "prefix-" + strconv.Itoa(idx) + "-" + strings.Repeat("x", workspaceSnapshotTraceStringLimit+200),
				"recentFailures":   []any{"one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve", "thirteen"},
			}),
		})
	}

	got := ProjectWorkspaceSnapshotDetail(readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{
			ID:               "ws-compact",
			ActiveRunID:      "run-0",
			ContextHints:     map[string]any{"large": "profile"},
			EvidencePolicies: []map[string]any{{"large": "policy"}},
		},
		Run:             &runs[11],
		Runs:            runs,
		LLMUsageTraces:  traces,
		LLMUsageSummary: projectioncontract.LLMUsageSummary{EntryTotal: len(traces)},
	})

	if got.Workspace.ContextHints != nil || got.Workspace.EvidencePolicies != nil {
		t.Fatalf("workspace heavy fields were not compacted: %+v", got.Workspace)
	}
	if got.Run == nil {
		t.Fatalf("active run = %+v, want compact run", got.Run)
	}
	if len(got.Runs) != workspaceSnapshotDetailRunLimit+1 {
		t.Fatalf("runs = %d, want active plus latest %d", len(got.Runs), workspaceSnapshotDetailRunLimit)
	}
	if got.Runs[0].ID != "run-0" {
		t.Fatalf("first run = %q, want active run preserved", got.Runs[0].ID)
	}
	if len(got.LLMUsageTraces) != workspaceSnapshotDetailLLMTraceLimit {
		t.Fatalf("llm usage traces = %d, want %d", len(got.LLMUsageTraces), workspaceSnapshotDetailLLMTraceLimit)
	}
	if got.LLMUsageTraces[0].ID != "usage-3" {
		t.Fatalf("first trace = %q, want latest traces retained", got.LLMUsageTraces[0].ID)
	}
	if signature, ok := got.LLMUsageTraces[0].ContextBudget.String("failureSignature"); !ok || len(signature) > workspaceSnapshotTraceStringLimit+len("...[truncated]") {
		t.Fatalf("context budget failureSignature length = %d, want bounded", len(signature))
	}
	if recent, ok := got.LLMUsageTraces[0].ContextBudget.Field("recentFailures"); !ok || len(recent.Array) != workspaceSnapshotTraceSliceLimit {
		t.Fatalf("context budget recentFailures = %d, want %d", len(recent.Array), workspaceSnapshotTraceSliceLimit)
	}
	if got.LLMUsageSummary.EntryTotal != len(traces) {
		t.Fatalf("summary entry total = %d, want full total %d", got.LLMUsageSummary.EntryTotal, len(traces))
	}
}

func TestProjectWorkspaceSnapshotSummaryOmitsPollingHeavyRuntimeFields(t *testing.T) {
	t.Parallel()

	got := ProjectWorkspaceSnapshotSummary(readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-summary-compact", ActiveRunID: "run-summary-compact"},
		Run: &appmodelcore.Run{
			ID:          "run-summary-compact",
			WorkspaceID: "ws-summary-compact",
			State:       "running",
		},
		Step: &readmodelcore.StepView{
			ID:    "run-summary-compact/step-1",
			Kind:  "llm/workspace_execution",
			State: "running",
			Input: map[string]any{
				"repairActionPackage": map[string]any{
					"targetSnapshots": []map[string]any{{
						"targetRef": "target.txt",
						"content":   strings.Repeat("snapshot", 2000),
					}},
				},
			},
		},
		ContextBundle: &readmodelcore.ContextBundle{
			WorkspaceID: "ws-summary-compact",
			Sections:    repeatedContextSections(3, 3),
		},
		ExecutionSections: repeatedExecutionSectionsForSummary(3),
		LLMUsageTraces: []projectioncontract.LLMUsageTrace{{
			ID: "usage-summary-compact",
			ContextBudget: mustSnapshotDetailPayload(t, map[string]any{
				"large": strings.Repeat("budget", 2000),
			}),
		}},
		EventRecords: []projectioncontract.EventRecord{{
			ID: "evt-summary-compact",
		}},
	})

	if got.Run != nil || got.Step != nil || got.ContextBundle != nil {
		t.Fatalf("summary retained heavy runtime fields: run=%+v step=%+v context=%+v", got.Run, got.Step, got.ContextBundle)
	}
	if len(got.ExecutionSections) != 0 || len(got.LLMUsageTraces) != 0 || len(got.EventRecords) != 0 {
		t.Fatalf("summary retained polling-heavy slices: execution=%d usage=%d events=%d", len(got.ExecutionSections), len(got.LLMUsageTraces), len(got.EventRecords))
	}
	if got.Workspace.ActiveRunID != "run-summary-compact" {
		t.Fatalf("summary workspace = %+v, want lightweight active run identity retained", got.Workspace)
	}
}

func TestProjectWorkspaceSnapshotDetailCompactsRuntimeAndTaskGraphFields(t *testing.T) {
	t.Parallel()

	largeInput := map[string]any{
		"repairActionPackage": map[string]any{
			"targetSnapshot": strings.Repeat("s", workspaceSnapshotTraceStringLimit*4),
			"sectionHashes":  repeatedStrings("section-hash", workspaceSnapshotTraceSliceLimit+4),
		},
		"contextBudget": map[string]any{
			"actualBytes": 731429,
			"cachePlan":   strings.Repeat("c", workspaceSnapshotTraceStringLimit*4),
		},
	}
	nodes := make([]readmodelcore.TaskDecompositionNode, 0, workspaceSnapshotTraceSliceLimit+3)
	for idx := 0; idx < workspaceSnapshotTraceSliceLimit+3; idx++ {
		nodes = append(nodes, readmodelcore.TaskDecompositionNode{
			ID:               "node-" + strconv.Itoa(idx),
			Title:            strings.Repeat("n", workspaceSnapshotTraceStringLimit+200),
			State:            "pending",
			DependsOn:        repeatedStrings("dependency", workspaceSnapshotTraceSliceLimit+3),
			EvidenceRefs:     repeatedStrings("evidence", workspaceSnapshotTraceSliceLimit+3),
			CoverageTargets:  repeatedStrings("coverage", workspaceSnapshotTraceSliceLimit+3),
			FailureSignature: strings.Repeat("f", workspaceSnapshotTraceStringLimit+200),
			Input:            largeInput,
		})
	}
	decompositions := make([]readmodelcore.TaskDecomposition, 0, workspaceSnapshotTraceSliceLimit+3)
	plans := make([]readmodelcore.TaskPlan, 0, workspaceSnapshotTraceSliceLimit+3)
	for idx := 0; idx < workspaceSnapshotTraceSliceLimit+3; idx++ {
		decompositions = append(decompositions, readmodelcore.TaskDecomposition{
			ID:           "decomposition-" + strconv.Itoa(idx),
			Summary:      strings.Repeat("d", workspaceSnapshotTraceStringLimit+200),
			Nodes:        nodes,
			EvidenceRefs: repeatedStrings("decomposition-evidence", workspaceSnapshotTraceSliceLimit+3),
		})
		plans = append(plans, readmodelcore.TaskPlan{
			ID:           "plan-" + strconv.Itoa(idx),
			Summary:      strings.Repeat("p", workspaceSnapshotTraceStringLimit+200),
			Nodes:        nodes,
			EvidenceRefs: repeatedStrings("plan-evidence", workspaceSnapshotTraceSliceLimit+3),
		})
	}

	got := ProjectWorkspaceSnapshotDetail(readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-runtime-compact"},
		Step: &readmodelcore.StepView{
			ID:    "current-step",
			Kind:  "llm/workspace_execution",
			State: "running",
			Output: map[string]any{
				"summary":       "current summary",
				"contextBudget": map[string]any{"actualBytes": 731429},
			},
		},
		TaskProgress:       readmodelcore.TaskProgress{CurrentStepID: "step-1"},
		TaskDecompositions: decompositions,
		TaskPlans:          plans,
	})

	if got.Step == nil || got.Step.Output["summary"] != "current summary" {
		t.Fatalf("current step = %+v, want projected summary", got.Step)
	}
	if budget, ok := got.Step.Output["contextBudget"].(map[string]any); !ok || budget["actualBytes"] == nil {
		t.Fatalf("current step output = %+v, want bounded contextBudget summary", got.Step.Output)
	} else if cachePlan, ok := budget["cachePlan"].(string); ok && len(cachePlan) > workspaceSnapshotTraceStringLimit+len("...[truncated]") {
		t.Fatalf("current step contextBudget cachePlan length = %d, want bounded", len(cachePlan))
	}
	if len(got.TaskDecompositions) != workspaceSnapshotTraceSliceLimit {
		t.Fatalf("task decompositions = %d, want %d", len(got.TaskDecompositions), workspaceSnapshotTraceSliceLimit)
	}
	if got.TaskDecompositions[0].ID != "decomposition-3" {
		t.Fatalf("first decomposition = %q, want latest retained", got.TaskDecompositions[0].ID)
	}
	if len(got.TaskDecompositions[0].Nodes) != workspaceSnapshotTraceSliceLimit {
		t.Fatalf("decomposition nodes = %d, want %d", len(got.TaskDecompositions[0].Nodes), workspaceSnapshotTraceSliceLimit)
	}
	if len(got.TaskPlans) != workspaceSnapshotTraceSliceLimit {
		t.Fatalf("task plans = %d, want %d", len(got.TaskPlans), workspaceSnapshotTraceSliceLimit)
	}
	node := got.TaskPlans[0].Nodes[0]
	if len(node.DependsOn) != workspaceSnapshotTraceSliceLimit || len(node.EvidenceRefs) != workspaceSnapshotTraceSliceLimit || len(node.CoverageTargets) != workspaceSnapshotTraceSliceLimit {
		t.Fatalf("projected node refs = %+v, want bounded refs", node)
	}
	if len(node.Title) > workspaceSnapshotTraceStringLimit+len("...[truncated]") || len(node.FailureSignature) > workspaceSnapshotTraceStringLimit+len("...[truncated]") {
		t.Fatalf("projected node text not bounded: title=%d signature=%d", len(node.Title), len(node.FailureSignature))
	}
	if node.Input == nil || node.Input["repairActionPackage"] == nil {
		t.Fatalf("projected node input = %+v, want bounded diagnostic input retained", node.Input)
	}
	if text, ok := node.Input["contextBudget"].(map[string]any)["cachePlan"].(string); ok && len(text) > workspaceSnapshotTraceStringLimit+len("...[truncated]") {
		t.Fatalf("projected node input cachePlan length = %d, want bounded", len(text))
	}
}

func TestProjectWorkspaceSnapshotDetailCompactsTypedContextSectionData(t *testing.T) {
	t.Parallel()

	nodes := []readmodelcore.TaskDecompositionNode{{
		ID:    "node-heavy",
		Title: strings.Repeat("node-title", 200),
		Input: map[string]any{
			"repairActionPackage": map[string]any{
				"targetSnapshot": strings.Repeat("snapshot", 2000),
				"sectionHashes":  repeatedStrings("hash", workspaceSnapshotTraceSliceLimit+5),
			},
		},
		EvidenceRefs: repeatedStrings("node-evidence", workspaceSnapshotTraceSliceLimit+5),
	}}
	result := readmodelcore.Result{
		ID:      "result-heavy",
		State:   "rejected",
		Summary: strings.Repeat("result-summary", 200),
		SummaryDetail: &readmodelcore.ResultSummary{
			GoalOutcome:           strings.Repeat("goal-outcome", 200),
			ImplementationSummary: strings.Repeat("implementation", 200),
			Diagnostics: map[string]any{
				"runtimeDiagnostics": map[string]any{
					"candidateFailure": map[string]any{
						"actualOutputSummary": map[string]any{"error": strings.Repeat("diagnostic", 2000)},
					},
				},
			},
		},
	}
	decision := appmodelcore.Decision{
		ID:    "decision-heavy",
		State: "open",
		Context: mustProjectionPayload(t, map[string]any{
			"failedStepInput": map[string]any{
				"failureTargetRepairContext": []any{
					map[string]any{"contentPreview": strings.Repeat("preview", 2000)},
				},
			},
			"failedStepOutput": map[string]any{
				"error": strings.Repeat("decision-error", 2000),
			},
		}),
	}

	got := ProjectWorkspaceSnapshotDetail(readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-context-typed"},
		ContextBundle: &readmodelcore.ContextBundle{
			WorkspaceID: "ws-context-typed",
			Sections: []readmodelcore.ContextSection{
				{
					ID:      "formal_outputs",
					Data:    map[string]any{"results": []readmodelcore.Result{result}},
					Summary: strings.Repeat("formal", 200),
				},
				{
					ID:   "task_decomposition",
					Data: map[string]any{"taskPlans": []readmodelcore.TaskPlan{{ID: "plan-heavy", Nodes: nodes}}},
				},
				{
					ID:   "governance",
					Data: map[string]any{"decisions": []appmodelcore.Decision{decision}},
				},
				{
					ID: "memory_recall",
					Data: map[string]any{"memoryRecallTraces": []readmodelcore.MemoryRecallTrace{{
						ID:                  "memory-heavy",
						QueryTextPreview:    strings.Repeat("query", 2000),
						CandidateMemoryIDs:  repeatedStrings("candidate", workspaceSnapshotTraceSliceLimit+5),
						RetrievedMemoryHits: repeatedMemoryHits(workspaceSnapshotTraceSliceLimit + 5),
					}}},
				},
			},
		},
	})

	if got.ContextBundle == nil || len(got.ContextBundle.Sections) != 4 {
		t.Fatalf("sections = %+v, want projected typed sections", got.ContextBundle)
	}
	for _, section := range got.ContextBundle.Sections {
		dataBytes := jsonSize(t, section.Data)
		if dataBytes > 20000 {
			t.Fatalf("section %s data bytes = %d, want bounded projection data", section.ID, dataBytes)
		}
		assertNoLongStringInJSONValue(t, section.Data, workspaceSnapshotTraceStringLimit+len("...[truncated]"))
	}
	formalOutputs := contextSectionByID(got.ContextBundle.Sections, "formal_outputs")
	if results, ok := formalOutputs.Data["results"].([]any); !ok || len(results) != 1 || results[0].(map[string]any)["id"] != "result-heavy" {
		t.Fatalf("formal output data = %+v, want projected result id retained", formalOutputs.Data)
	}
	taskSection := contextSectionByID(got.ContextBundle.Sections, "task_decomposition")
	if plans, ok := taskSection.Data["taskPlans"].([]any); !ok || len(plans) != 1 || plans[0].(map[string]any)["id"] != "plan-heavy" {
		t.Fatalf("task data = %+v, want projected task plan id retained", taskSection.Data)
	}
}

func TestProjectWorkspaceSnapshotDetailPreservesActiveBlockingGovernance(t *testing.T) {
	t.Parallel()

	decisions := make([]appmodelcore.Decision, 0, decisionPageLimit+5)
	approvals := make([]appmodelcore.Approval, 0, approvalPageLimit+5)
	for idx := 0; idx < decisionPageLimit+4; idx++ {
		decisions = append(decisions, appmodelcore.Decision{
			ID:          "old-decision-" + strconv.Itoa(idx),
			WorkspaceID: "ws-governance-compact",
			RunID:       "run-old",
			State:       "open",
		})
	}
	for idx := 0; idx < approvalPageLimit+4; idx++ {
		approvals = append(approvals, appmodelcore.Approval{
			ID:          "old-approval-" + strconv.Itoa(idx),
			WorkspaceID: "ws-governance-compact",
			RunID:       "run-old",
			State:       "pending",
		})
	}
	decisions = append(decisions, appmodelcore.Decision{
		ID:          "active-blocking-decision",
		WorkspaceID: "ws-governance-compact",
		RunID:       "run-active",
		State:       "open",
		Question:    "current decision",
	})
	approvals = append(approvals, appmodelcore.Approval{
		ID:          "active-blocking-approval",
		WorkspaceID: "ws-governance-compact",
		RunID:       "run-active",
		State:       "pending",
		Reason:      "current approval",
	})

	got := ProjectWorkspaceSnapshotDetail(readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-governance-compact", ActiveRunID: "run-active"},
		TaskProgress: readmodelcore.TaskProgress{
			BlockingTicketID:   "active-blocking-decision",
			BlockingApprovalID: "active-blocking-approval",
			Recovery: projectioncontract.RecoveryPlan{
				BlockingTicketID:   "active-blocking-decision",
				BlockingApprovalID: "active-blocking-approval",
			},
		},
		Decisions: decisions,
		Approvals: approvals,
	})

	if len(got.Decisions) != decisionPageLimit {
		t.Fatalf("decisions = %d, want %d", len(got.Decisions), decisionPageLimit)
	}
	if got.Decisions[0].ID != "active-blocking-decision" {
		t.Fatalf("first decision = %q, want active blocking decision preserved before historical items", got.Decisions[0].ID)
	}
	if len(got.Approvals) != approvalPageLimit {
		t.Fatalf("approvals = %d, want %d", len(got.Approvals), approvalPageLimit)
	}
	if got.Approvals[0].ID != "active-blocking-approval" {
		t.Fatalf("first approval = %q, want active blocking approval preserved before historical items", got.Approvals[0].ID)
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
	budgets := make([]projectioncontract.LLMContextBudgetTrace, 0, workspaceSnapshotDetailLLMContextBudgetLimit+3)
	for idx := 0; idx < workspaceSnapshotDetailLLMContextBudgetLimit+3; idx++ {
		budgets = append(budgets, projectioncontract.LLMContextBudgetTrace{
			StepID:     "step-" + strconv.Itoa(idx),
			Rules:      repeatedStrings("rule", workspaceSnapshotTraceSliceLimit+3),
			SourceRefs: repeatedStrings("source", workspaceSnapshotTraceSliceLimit+3),
			Profile: mustSnapshotDetailPayload(t, map[string]any{
				"long": strings.Repeat("p", workspaceSnapshotTraceStringLimit+200),
			}),
		})
	}

	got := ProjectWorkspaceSnapshotDetail(readmodelcore.WorkspaceSnapshot{
		Workspace:          appmodelcore.Workspace{ID: "ws-compact-derived"},
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

func repeatedMemoryHits(count int) []projectioncontract.MemoryRecallHit {
	ret := make([]projectioncontract.MemoryRecallHit, 0, count)
	for idx := 0; idx < count; idx++ {
		ret = append(ret, projectioncontract.MemoryRecallHit{
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

func repeatedExecutionSectionsForSummary(count int) []readmodelcore.ExecutionSection {
	ret := make([]readmodelcore.ExecutionSection, 0, count)
	for idx := 0; idx < count; idx++ {
		ret = append(ret, readmodelcore.ExecutionSection{
			ID:      "execution-summary-" + strconv.Itoa(idx),
			Summary: strings.Repeat("execution", 2000),
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

func contextSectionByID(items []readmodelcore.ContextSection, id string) readmodelcore.ContextSection {
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	return readmodelcore.ContextSection{}
}

func jsonSize(t *testing.T, value any) int {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return len(raw)
}

func assertNoLongStringInJSONValue(t *testing.T, value any, limit int) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	assertNoLongString(t, decoded, limit)
}

func assertNoLongString(t *testing.T, value any, limit int) {
	t.Helper()
	switch typed := value.(type) {
	case string:
		if len(typed) > limit {
			t.Fatalf("string length = %d, want <= %d", len(typed), limit)
		}
	case []any:
		for _, item := range typed {
			assertNoLongString(t, item, limit)
		}
	case map[string]any:
		for _, item := range typed {
			assertNoLongString(t, item, limit)
		}
	}
}
