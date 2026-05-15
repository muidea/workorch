package orchestrator

import (
	"context"
	"strings"
	"testing"

	memorycommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRunExecutionContextPackageIncludesCompactGoalDoDDefinitions(t *testing.T) {
	orch := &RuntimeOrchestrator{}
	run := &runmodel.Run{
		ID:          "run-context-definitions",
		WorkspaceID: "ws-context-definitions",
		GoalID:      "goal-context-definitions",
		DoDID:       "dod-context-definitions",
		ExecutionContext: map[string]any{
			"goalId": "goal-context-definitions",
			"doDId":  "dod-context-definitions",
			"goal": map[string]any{
				"id":          "goal-context-definitions",
				"workspaceId": "ws-context-definitions",
				"text":        "Build the requested gateway behavior.",
			},
			"dod": map[string]any{
				"id":          "dod-context-definitions",
				"workspaceId": "ws-context-definitions",
				"items":       []string{"proxy request", "record usage"},
				"state":       "ready",
			},
			"goals": []map[string]any{
				{"id": "goal-context-definitions", "text": "Build the requested gateway behavior."},
			},
			"dods": []map[string]any{
				{"id": "dod-context-definitions", "items": []string{"proxy request", "record usage"}},
			},
			"goalDefinitionPolicy": "workspace_goals_are_cumulative_latest_refines_previous",
			"doDDefinitionPolicy":  "workspace_dods_are_cumulative_latest_refines_previous",
			"soulRouting":          map[string]any{"large": strings.Repeat("x", 9000)},
		},
	}
	step := &runmodel.Step{
		ID:    "run-context-definitions/step-1",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageExecution,
			stepInputKeyRuntimeStageMode: "strict_convergence",
		},
	}

	pkg := orch.runExecutionContextPackage(context.Background(), run, step, nil)

	goal := mapFromValue(pkg["goal"])
	if goal["id"] != "goal-context-definitions" || goal["text"] != "Build the requested gateway behavior." {
		t.Fatalf("goal package = %+v, want compact goal definition", pkg["goal"])
	}
	dod := mapFromValue(pkg["dod"])
	items := stringSliceFromValue(dod["items"])
	if dod["id"] != "dod-context-definitions" || len(items) != 2 || items[0] != "proxy request" || items[1] != "record usage" {
		t.Fatalf("dod package = %+v, want compact DoD definition", pkg["dod"])
	}
	if pkg["goalId"] != "goal-context-definitions" || pkg["doDId"] != "dod-context-definitions" {
		t.Fatalf("definition IDs = %v/%v, want goal/dod IDs", pkg["goalId"], pkg["doDId"])
	}
	if _, ok := pkg["soulRouting"]; ok {
		t.Fatalf("executionContextPackage retained raw control context: %+v", pkg)
	}
}

func TestRunExecutionContextPackagePreservesExecutionContractDetails(t *testing.T) {
	orch := &RuntimeOrchestrator{}
	run := &runmodel.Run{
		ID:          "run-context-contract-details",
		WorkspaceID: "ws-context-contract-details",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "source_change", "required": true},
						},
						"evidenceRequirements": []map[string]any{
							{
								"id":                "side_effect",
								"scope":             "acceptanceDimension",
								"acceptedStepKinds": []string{"shell/command", "patch/apply"},
								"rejectedStepKinds": []string{"file/read"},
							},
						},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-context-contract-details/step-1",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageExecution,
		},
	}

	pkg := orch.runExecutionContextPackage(context.Background(), run, step, nil)

	contract := mapFromValue(pkg["executionContract"])
	profiles := mapFromValue(contract["profileContracts"])
	profile := mapFromValue(profiles["neutral-profile"])
	deliverables := mapSliceFromValue(profile["deliverableExpectations"])
	if len(deliverables) != 1 || deliverables[0]["id"] != "source_change" || deliverables[0]["required"] != true {
		t.Fatalf("deliverableExpectations = %+v, want full required contract", profile["deliverableExpectations"])
	}
	requirements := mapSliceFromValue(profile["evidenceRequirements"])
	if len(requirements) != 1 || requirements[0]["id"] != "side_effect" {
		t.Fatalf("evidenceRequirements = %+v, want full evidence requirement", profile["evidenceRequirements"])
	}
	if accepted := stringSliceFromValue(requirements[0]["acceptedStepKinds"]); len(accepted) != 2 || accepted[0] != "shell/command" || accepted[1] != "patch/apply" {
		t.Fatalf("acceptedStepKinds = %+v, want preserved accepted kinds", requirements[0]["acceptedStepKinds"])
	}
}

func TestCompactMemoryContextsMarksLegacyRunSummaryMemory(t *testing.T) {
	items := []map[string]any{
		{
			"id":         "memory-legacy",
			"memoryType": "run_summary",
			"summary":    "legacy summary",
			"tags":       []string{"run_summary", "dod_assessment"},
		},
		{
			"id":         "memory-v2",
			"memoryType": "run_summary",
			"summary":    "v2 summary",
			"tags":       []string{"run_summary", runSummaryDoDGateVersionTag},
		},
	}

	compact := compactMemoryContexts(items)
	if len(compact) != 2 {
		t.Fatalf("compactMemoryContexts len = %d, want 2", len(compact))
	}
	if compact[0]["evidenceTrust"] != "legacy_pre_dod_gate" {
		t.Fatalf("legacy evidenceTrust = %#v, want legacy_pre_dod_gate", compact[0]["evidenceTrust"])
	}
	if _, ok := compact[1]["evidenceTrust"]; ok {
		t.Fatalf("v2 memory evidenceTrust = %#v, want unset", compact[1]["evidenceTrust"])
	}
}

func TestMemoryItemContextMapsOmitProcessLineageNoise(t *testing.T) {
	runID := common.RunID("run-memory-context")
	items := []*memorymodel.MemoryItem{
		{
			ID:                  "run-memory-context/provider-retry-001-of-step-1/memory/1",
			WorkspaceID:         "ws-memory-context",
			RunID:               runID,
			SourceType:          "run",
			SourceRef:           "run-memory-context/provider-retry-001-of-step-1",
			MemoryType:          "dod_feedback",
			Summary:             "DoD gap requires concrete verification",
			Content:             "Add a targeted test and cite its validation evidence.",
			EmbeddingProviderID: "embedder",
			EmbeddingModel:      "model",
			Vector:              []float64{0.1, 0.2},
			VectorScore:         0.87,
		},
	}

	compact := memoryItemContextMaps(items, runID)

	if len(compact) != 1 {
		t.Fatalf("memoryItemContextMaps len = %d, want 1", len(compact))
	}
	for _, noisyKey := range []string{"workspaceId", "runId", "sourceType", "sourceRef", "embeddingProviderId", "embeddingModel", "vectorReady", "vectorDimension"} {
		if _, ok := compact[0][noisyKey]; ok {
			t.Fatalf("compact memory contains %s: %+v", noisyKey, compact[0])
		}
	}
	if compact[0]["provenance"] != "current_run" || compact[0]["summary"] == "" || compact[0]["content"] == "" {
		t.Fatalf("compact memory = %+v, want decision content with current_run provenance", compact[0])
	}
}

func TestStepInputMemorySelectionExcludesProviderRetryPlumbing(t *testing.T) {
	currentRunID := common.RunID("run-memory-filter-current")
	selected, candidates, excluded := selectStepInputMemoryItems([]*memorymodel.MemoryItem{
		{
			ID:         "memory-provider-retry-plumbing",
			RunID:      currentRunID,
			MemoryType: memorycommon.MemoryTypeToolFeedback,
			Summary:    "Provider retry outcome captured",
			Content:    "Provider retry attempt: 1\nfailureClass: provider_timeout",
			Tags:       []string{"provider_retry", "tool_context", "necessary_output"},
			Importance: 100,
			Confidence: 100,
			Visibility: "workspace",
		},
		{
			ID:         "memory-accepted-mcp",
			RunID:      currentRunID,
			MemoryType: memorycommon.MemoryTypeToolFeedback,
			Summary:    "Accepted tool and MCP context",
			Content:    "mcpServers: mcp-code\nmcpCapabilities: cap-read",
			Tags:       []string{"accepted_input", "tool_context", "mcp_context"},
			Importance: 50,
			Confidence: 50,
			Visibility: "workspace",
		},
	}, currentRunID, nil, stepInputMemoryLimit)

	if memoryItemsContainID(selected, "memory-provider-retry-plumbing") {
		t.Fatalf("selected = %+v, want provider retry plumbing excluded from LLM memory context", selected)
	}
	if !memoryItemsContainID(selected, "memory-accepted-mcp") {
		t.Fatalf("selected = %+v, want accepted tool/MCP context selected", selected)
	}
	if !memoryItemsContainID(excluded, "memory-provider-retry-plumbing") {
		t.Fatalf("excluded = %+v, want provider retry plumbing visible in exclusion trace", excluded)
	}
	var providerRetryCandidate *memorymodel.MemoryItem
	for _, item := range candidates {
		if item != nil && item.ID == "memory-provider-retry-plumbing" {
			providerRetryCandidate = item
			break
		}
	}
	if providerRetryCandidate == nil {
		t.Fatalf("candidates = %+v, want provider retry plumbing candidate for trace scoring", candidates)
	}
	reason := stepInputMemoryScoreReason(providerRetryCandidate, currentRunID, nil)
	if !strings.Contains(reason, "excluded_low_value_process_memory") {
		t.Fatalf("score reason = %q, want low-value process exclusion reason", reason)
	}
}

func memoryItemsContainID(items []*memorymodel.MemoryItem, id common.MemoryID) bool {
	for _, item := range items {
		if item != nil && item.ID == id {
			return true
		}
	}
	return false
}
