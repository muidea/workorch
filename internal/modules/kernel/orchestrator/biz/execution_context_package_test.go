package orchestrator

import (
	"context"
	"strings"
	"testing"

	memorycommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
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
			"definitionChangeImpact": map[string]any{
				"version": "workorch.definition_change_impact.v1",
				"changed": true,
				"policy":  "workspace_goal_dod_cumulative_latest_refines_previous",
				"goalChange": map[string]any{
					"effectiveGoalId": "goal-context-definitions",
					"previousGoalIds": []string{"goal-context-prior"},
				},
				"strategyRequirements": []string{"preserve still-valid prior Goal and DoD constraints"},
			},
			"soulRouting": map[string]any{"large": strings.Repeat("x", 9000)},
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
	impact := mapFromValue(pkg["definitionChangeImpact"])
	if impact["changed"] != true || impact["policy"] != "workspace_goal_dod_cumulative_latest_refines_previous" {
		t.Fatalf("definitionChangeImpact = %+v, want compact change impact", impact)
	}
	goalChange := mapFromValue(impact["goalChange"])
	if goalChange["effectiveGoalId"] != "goal-context-definitions" {
		t.Fatalf("goalChange = %+v, want effective goal id", goalChange)
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

func TestRunExecutionContextPackageIncludesProcessEvidenceTimeline(t *testing.T) {
	ctx := context.Background()
	stepRepo := memrepo.NewStepRepository()
	run := &runmodel.Run{
		ID:          "run-process-context",
		WorkspaceID: "ws-process-context",
	}
	completed := &runmodel.Step{
		ID:    "run-process-context/graph-001/001-write",
		RunID: run.ID,
		Kind:  "patch/apply",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyRuntimeStage: "execution",
			stepInputKeyGraphNodeID:  "write_target",
		},
		Output: map[string]any{
			"summary":      "state-changing artifact completed",
			"evidenceRefs": []string{"patch://run-process-context/001"},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-process-context/graph-001/002-verify",
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: "verification",
			stepInputKeyGraphNodeID:  "verify_target",
		},
		Output: map[string]any{
			"stderr":                  strings.Repeat("verification failed\n", 20) + "final failure detail",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=verify_target|exitCode=1",
		},
	}
	if err := stepRepo.Create(ctx, completed); err != nil {
		t.Fatalf("create completed step: %v", err)
	}
	if err := stepRepo.Create(ctx, failed); err != nil {
		t.Fatalf("create failed step: %v", err)
	}
	orch := &RuntimeOrchestrator{steps: stepRepo}
	current := &runmodel.Step{
		ID:    "run-process-context/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: "repair",
		},
	}

	orch.enrichLLMStepInputContext(ctx, run, current, nil)

	pkg := mapFromValue(current.Input[stepInputKeyExecutionContextPackage])
	timeline := mapSliceFromValue(pkg[stepInputKeyProcessEvidenceTimeline])
	if len(timeline) != 2 {
		t.Fatalf("processEvidenceTimeline = %+v, want two prior evidence items", timeline)
	}
	if timeline[0]["sourceRef"] != "step://run-process-context/graph-001/001-write" ||
		timeline[1]["sourceRef"] != "step://run-process-context/graph-001/002-verify" {
		t.Fatalf("processEvidenceTimeline order/sourceRef = %+v", timeline)
	}
	if timeline[1]["failureClass"] != failureClassShellCommand {
		t.Fatalf("failed timeline item = %+v, want failure class", timeline[1])
	}
	if timeline[1]["payloadRef"] != "workorch://workspaces/ws-process-context/runs/run-process-context/steps/run-process-context/graph-001/002-verify/output.json" {
		t.Fatalf("failed payloadRef = %#v", timeline[1]["payloadRef"])
	}
	digest := mapFromValue(pkg[stepInputKeyCurrentProgressDigest])
	if mapFromValue(digest["lastObservedFailure"])["stepId"] != string(failed.ID) ||
		digest["nextRequiredAction"] != "address_last_failure_under_current_stage_contract" {
		t.Fatalf("currentProgressDigest = %+v, want latest failure and next action", digest)
	}
	if len(mapSliceFromValue(current.Input[stepInputKeyProcessEvidenceTimeline])) != 2 {
		t.Fatalf("top-level processEvidenceTimeline missing from LLM step input: %+v", current.Input)
	}
	if len(mapFromValue(current.Input[stepInputKeyCurrentProgressDigest])) == 0 {
		t.Fatalf("top-level currentProgressDigest missing from LLM step input: %+v", current.Input)
	}
}

func TestRunExecutionContextPackageInheritsReplanProcessEvidence(t *testing.T) {
	ctx := context.Background()
	run := &runmodel.Run{
		ID:          "run-process-context-replan",
		WorkspaceID: "ws-process-context-replan",
		ExecutionContext: map[string]any{
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-before-replan",
				stepInputKeyProcessEvidenceTimeline: []map[string]any{
					{
						"order":         1,
						"sourceRef":     "step://run-before-replan/graph-001/001-patch",
						"stepId":        "run-before-replan/graph-001/001-patch",
						"kind":          "patch/apply",
						"actionKind":    "artifact_write",
						"state":         string(common.StepStateFailed),
						"stage":         runtimeStageExecution,
						"failureClass":  failureClassRuntime,
						"failureReason": "patch failed",
					},
				},
				stepInputKeyCurrentProgressDigest: map[string]any{
					"currentGap":         "patch failed",
					"nextRequiredAction": "address_last_failure_under_current_stage_contract",
				},
			},
		},
	}
	current := &runmodel.Step{
		ID:    "run-process-context-replan/step-1",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: "repair",
		},
	}

	orch := &RuntimeOrchestrator{steps: memrepo.NewStepRepository()}
	orch.enrichLLMStepInputContext(ctx, run, current, nil)

	pkg := mapFromValue(current.Input[stepInputKeyExecutionContextPackage])
	timeline := mapSliceFromValue(pkg[stepInputKeyProcessEvidenceTimeline])
	if len(timeline) != 1 || timeline[0]["stepId"] != "run-before-replan/graph-001/001-patch" {
		t.Fatalf("processEvidenceTimeline = %+v, want inherited predecessor evidence", timeline)
	}
	if digest := mapFromValue(current.Input[stepInputKeyCurrentProgressDigest]); digest["currentGap"] != "patch failed" {
		t.Fatalf("currentProgressDigest = %+v, want inherited predecessor digest", digest)
	}
}

func TestRunExecutionContextPackageSynthesizesReplanFailureProcessEvidence(t *testing.T) {
	ctx := context.Background()
	run := &runmodel.Run{
		ID:          "run-process-context-replan-synth",
		WorkspaceID: "ws-process-context-replan-synth",
		ExecutionContext: map[string]any{
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-before-replan",
				"failureContext": map[string]any{
					"failedStepId":     "run-before-replan/graph-001/001-patch",
					"failedStepKind":   "patch/apply",
					"failureClass":     failureClassPatchApply,
					"failureSignature": "class=patch_apply_failed|targetRef=main_test.go",
					"failedStepOutput": map[string]any{
						"stage":  "patch_apply",
						"reason": "unified diff hunk context mismatch",
					},
				},
			},
		},
	}
	current := &runmodel.Step{
		ID:    "run-process-context-replan-synth/step-1",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: "repair",
		},
	}

	orch := &RuntimeOrchestrator{steps: memrepo.NewStepRepository()}
	orch.enrichLLMStepInputContext(ctx, run, current, nil)

	pkg := mapFromValue(current.Input[stepInputKeyExecutionContextPackage])
	timeline := mapSliceFromValue(pkg[stepInputKeyProcessEvidenceTimeline])
	if len(timeline) != 1 || timeline[0]["stepId"] != "run-before-replan/graph-001/001-patch" {
		t.Fatalf("processEvidenceTimeline = %+v, want synthesized predecessor failure evidence", timeline)
	}
	digest := mapFromValue(current.Input[stepInputKeyCurrentProgressDigest])
	if strings.TrimSpace(stringFromMap(digest, "currentGap")) == "" {
		t.Fatalf("currentProgressDigest = %+v, want synthesized current gap", digest)
	}
}

func TestCompactMemoryContextsDoesNotInjectHistoricalCompatibilityTrust(t *testing.T) {
	items := []map[string]any{
		{
			"id":         "memory-without-gate-tag",
			"memoryType": "run_summary",
			"summary":    "summary without gate tag",
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
	for _, item := range compact {
		if _, ok := item["evidenceTrust"]; ok {
			t.Fatalf("memory context injected compatibility trust marker: %+v", item)
		}
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
