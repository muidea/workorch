package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	memorycommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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

	pkg := runExecutionContextPackage(context.Background(), runtimeExecutionContextPackageDeps(orch), run, step, nil)

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

	pkg := runExecutionContextPackage(context.Background(), runtimeExecutionContextPackageDeps(orch), run, step, nil)

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

func TestProcessEvidenceTimelineCompactsMCPToolEvidence(t *testing.T) {
	run := &runmodel.Run{ID: "run-mcp-evidence", WorkspaceID: "ws-mcp-evidence"}
	step := &runmodel.Step{
		ID:    "run-mcp-evidence/tool-001",
		RunID: run.ID,
		Kind:  "mcp/tool_call",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"serverId": "mcp-code",
			"tool":     "semantic.lookup",
			"input": map[string]any{
				"path": "/repo/main.go",
				"line": 12,
				"col":  4,
			},
		},
		Output: map[string]any{
			"operation": "mcp/tool_call",
			"serverId":  "mcp-code",
			"tool":      "semantic.lookup",
			"status":    "completed",
			"stage":     "mcp_tool_call_completed",
			"result": map[string]any{
				"diagnostics": []map[string]any{
					{
						"path":     "/repo/main.go",
						"line":     12,
						"col":      4,
						"severity": "error",
						"message":  "undefined symbol",
					},
				},
				"blob": strings.Repeat("x", 16*1024),
			},
		},
	}

	timeline := processEvidenceTimelineFromRunSteps(run, []*runmodel.Step{step}, "")
	if len(timeline) != 1 {
		t.Fatalf("timeline = %+v, want one MCP evidence item", timeline)
	}
	item := timeline[0]
	output := mapFromValue(item["output"])
	if _, ok := output["result"]; ok {
		t.Fatalf("output = %+v, want raw MCP result omitted from process evidence", output)
	}
	toolEvidence := mapFromValue(item["toolEvidence"])
	resultSummary := mapFromValue(toolEvidence["resultSummary"])
	diagnostics := mapSliceFromValue(resultSummary["diagnostics"])
	if toolEvidence["serverId"] != "mcp-code" ||
		toolEvidence["tool"] != "semantic.lookup" ||
		len(diagnostics) != 1 ||
		diagnostics[0]["message"] != "undefined symbol" {
		t.Fatalf("toolEvidence = %+v, want compact diagnostic summary", toolEvidence)
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal timeline item: %v", err)
	}
	if strings.Contains(string(encoded), strings.Repeat("x", 256)) {
		t.Fatalf("timeline item leaked large MCP result payload")
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
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustReturnValidJSON": true,
				},
			},
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

func TestCompactProcessEvidenceTimelineForStateChangingRepairPrioritizesCurrentFailure(t *testing.T) {
	step := &runmodel.Step{
		ID:    "run-process-context/repair-001",
		RunID: "run-process-context",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"stateChangingOutputRequired":     true,
			"failedStepId":                    "run-process-context/graph-001/002-verify",
			"repairActionPackage": map[string]any{
				"failedStepId": "run-process-context/graph-001/002-verify",
			},
		},
	}
	timeline := []map[string]any{
		{
			"order":         1,
			"stepId":        "run-process-context/provider-retry-001",
			"kind":          "llm/workspace_execution",
			"state":         string(common.StepStateFailed),
			"failureReason": "invalid argument: openai llm provider error: context canceled",
		},
		{
			"order":      2,
			"stepId":     "run-process-context/graph-001/001-write",
			"kind":       "patch/apply",
			"actionKind": "artifact_write",
			"state":      string(common.StepStateCompleted),
			"summary":    "state-changing output completed",
		},
		{
			"order":         3,
			"stepId":        "run-process-context/graph-001/002-verify",
			"kind":          "shell/command",
			"state":         string(common.StepStateFailed),
			"failureReason": "current verification failure",
		},
		{
			"order":         4,
			"stepId":        "run-process-context/provider-retry-002",
			"kind":          "llm/workspace_execution",
			"state":         string(common.StepStateFailed),
			"failureReason": "invalid argument: openai llm provider error: context canceled",
		},
	}

	compact := compactProcessEvidenceTimelineForStateChangingRepair(timeline, step)
	digest := currentProgressDigestFromProcessEvidenceTimeline(compact)

	failure := mapFromValue(digest["lastObservedFailure"])
	if failure["stepId"] != "run-process-context/graph-001/002-verify" {
		t.Fatalf("lastObservedFailure = %+v, want current repair target failure", failure)
	}
	if digest["currentGap"] != "current verification failure" {
		t.Fatalf("currentGap = %v, want current failure reason", digest["currentGap"])
	}
}

func TestCompactProcessEvidenceTimelineForStateChangingRepairKeepsActionableFailureOverSchemaInvalid(t *testing.T) {
	step := &runmodel.Step{
		ID:    "run-process-context/repair-002",
		RunID: "run-process-context",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"stateChangingOutputRequired":     true,
			"failedStepId":                    "run-process-context/repair-001",
			"repairActionPackage": map[string]any{
				"failedStepId": "run-process-context/repair-001",
			},
		},
	}
	timeline := []map[string]any{
		{
			"order":            1,
			"stepId":           "run-process-context/graph-001/002-verify",
			"kind":             "shell/command",
			"state":            string(common.StepStateFailed),
			"failureClass":     failureClassShellCommand,
			"failureReason":    "current verification failure",
			"evidenceValidity": "actionable_failure",
		},
		{
			"order":            2,
			"stepId":           "run-process-context/repair-001",
			"kind":             llmWorkspaceExecutionStepKind,
			"state":            string(common.StepStateFailed),
			"failureClass":     failureClassLLMDecisionInvalid,
			"failureReason":    "invalid llm decision json",
			"evidenceValidity": "stale_or_rejected",
		},
	}

	compact := compactProcessEvidenceTimelineForStateChangingRepair(timeline, step)
	digest := currentProgressDigestFromProcessEvidenceTimeline(compact)

	failure := mapFromValue(digest["lastObservedFailure"])
	if failure["stepId"] != "run-process-context/graph-001/002-verify" {
		t.Fatalf("lastObservedFailure = %+v, want original actionable failure", failure)
	}
	if digest["currentGap"] != "current verification failure" {
		t.Fatalf("currentGap = %v, want actionable failure reason", digest["currentGap"])
	}
}

func TestCurrentProgressDigestPrefersLatestActionableFailureOverOlderActionableFailure(t *testing.T) {
	timeline := []map[string]any{
		{
			"order":            1,
			"stepId":           "run-process-context/graph-001/002-test",
			"kind":             "shell/command",
			"state":            string(common.StepStateFailed),
			"failureClass":     failureClassShellCommand,
			"failureReason":    "old compile failure",
			"evidenceValidity": "actionable_failure",
		},
		{
			"order":      2,
			"stepId":     "run-process-context/graph-002/001-fix",
			"kind":       "patch/apply",
			"state":      string(common.StepStateCompleted),
			"summary":    "old compile failure fixed",
			"actionKind": "artifact_write",
		},
		{
			"order":            3,
			"stepId":           "run-process-context/graph-003/002-verify",
			"kind":             "shell/command",
			"state":            string(common.StepStateFailed),
			"failureClass":     failureClassShellCommand,
			"failureReason":    "latest verification failure",
			"evidenceValidity": "actionable_failure",
		},
		{
			"order":            4,
			"stepId":           "run-process-context/follow-up-graph-verify",
			"kind":             llmWorkspaceExecutionStepKind,
			"state":            string(common.StepStateFailed),
			"failureClass":     failureClassRuntime,
			"failureReason":    "validation follow-up llm step returned read-only executionGraph without patch output",
			"evidenceValidity": "stale_or_rejected",
		},
	}

	digest := currentProgressDigestFromProcessEvidenceTimeline(timeline)

	failure := mapFromValue(digest["lastObservedFailure"])
	if failure["stepId"] != "run-process-context/graph-003/002-verify" {
		t.Fatalf("lastObservedFailure = %+v, want latest actionable shell failure", failure)
	}
	if digest["currentGap"] != "latest verification failure" {
		t.Fatalf("currentGap = %v, want latest actionable failure", digest["currentGap"])
	}

	overlaid := currentProgressDigestFromCurrentExecutionProgress(map[string]any{
		"currentExecutionProgress": map[string]any{
			"latestActionEvidence": []map[string]any{{
				"stepId": "run-process-context/graph-004/002-verify",
				"kind":   "shell/command",
				"state":  string(common.StepStateCompleted),
				"outputSummary": map[string]any{
					"stdoutPreview": "./main.go:5:2: \"bytes\" imported and not used\nFAIL",
				},
			}},
		},
	}, digest)
	overlaidFailure := mapFromValue(overlaid["lastObservedFailure"])
	if overlaidFailure["stepId"] != "run-process-context/graph-004/002-verify" {
		t.Fatalf("overlaid lastObservedFailure = %+v, want latest concrete action evidence", overlaidFailure)
	}
	if !strings.Contains(stringFromMap(overlaid, "currentGap"), "main.go:5:2") {
		t.Fatalf("overlaid currentGap = %v, want concrete latest failure text", overlaid["currentGap"])
	}
}

func TestPromoteRepairActionPackageContextPrefersLatestActionableProgressDigest(t *testing.T) {
	input := map[string]any{
		stepInputKeyCurrentProgressDigest: map[string]any{
			"currentGap": "main_test.go:28 expected /v1/chat/completions actual /chat/completions",
			"lastObservedFailure": map[string]any{
				"stepId":           "run-progress/graph-001/002-run-tests",
				"kind":             "shell/command",
				"state":            string(common.StepStateFailed),
				"failureClass":     failureClassShellCommand,
				"failureSignature": "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=0",
				"failureReason":    "main_test.go:28 expected /v1/chat/completions actual /chat/completions",
				"evidenceValidity": "actionable_failure",
			},
		},
		"repairActionPackage": map[string]any{
			"currentFailure": map[string]any{
				"failedStepId": "run-progress/old-repair",
				"failureClass": failureClassLLMDecisionInvalid,
				"reason":       "old invalid llm decision",
			},
			"currentExecutionProgress": map[string]any{
				"currentGap": "old patch baseHash mismatch",
			},
		},
	}

	promoteRepairActionPackageContext(input)

	currentFailure := mapFromValue(input["currentFailure"])
	if currentFailure["failedStepId"] != "run-progress/graph-001/002-run-tests" ||
		currentFailure["failureClass"] != failureClassShellCommand {
		t.Fatalf("currentFailure = %+v, want latest actionable shell failure", currentFailure)
	}
	progress := mapFromValue(input["currentExecutionProgress"])
	if !strings.Contains(stringFromMap(progress, "currentGap"), "main_test.go:28") {
		t.Fatalf("currentExecutionProgress = %+v, want latest actionable gap", progress)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	pkgFailure := mapFromValue(pkg["currentFailure"])
	if pkgFailure["failedStepId"] != "run-progress/graph-001/002-run-tests" {
		t.Fatalf("repairActionPackage.currentFailure = %+v, want latest failure", pkgFailure)
	}
}

func TestPromoteRepairActionPackageContextCopiesInformativeProgressDigest(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeRepairFollowUp: true,
		"stateChangingOutputRequired":     true,
		stepInputKeyCurrentProgressDigest: map[string]any{
			"currentGap":         "none_observed_in_recent_process_evidence",
			"nextRequiredAction": "continue_current_stage_contract",
			"lastAttempt": map[string]any{
				"stepId": "run-progress/graph-001/002-collect-evidence",
				"kind":   "shell/command",
				"state":  string(common.StepStateCompleted),
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
			},
		},
	}

	promoteRepairActionPackageContext(input)

	progress := mapFromValue(input["currentExecutionProgress"])
	if progress["nextRequiredAction"] != "continue_current_stage_contract" {
		t.Fatalf("currentExecutionProgress = %+v, want informative progress digest", progress)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	pkgProgress := mapFromValue(pkg["currentExecutionProgress"])
	if pkgProgress["nextRequiredAction"] != "continue_current_stage_contract" {
		t.Fatalf("repairActionPackage.currentExecutionProgress = %+v, want informative progress digest", pkgProgress)
	}
	if len(mapFromValue(input["currentFailure"])) > 0 || len(mapFromValue(pkg["currentFailure"])) > 0 {
		t.Fatalf("input = %+v, did not expect non-failure digest to synthesize currentFailure", input)
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
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustReturnValidJSON": true,
				},
			},
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

func TestRunExecutionContextPackageRejectedResultContinuationSupersedesReplanProgress(t *testing.T) {
	ctx := context.Background()
	run := &runmodel.Run{
		ID:          "run-process-context-rejected-result",
		WorkspaceID: "ws-process-context-rejected-result",
		ExecutionContext: map[string]any{
			"rejectedResultContinuation": map[string]any{
				"predecessorRunId": "run-latest",
				"resultId":         "run-latest/result/dod-assessment",
				"sourceRef":        "run-latest/repair-001",
				"summary":          "latest rejected result requires current resource evidence",
				"requiredEvidence": []string{
					"read current implementation resource",
					"patch implementation resource",
					"run bounded verification",
				},
			},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-older",
				"failureContext": map[string]any{
					"failedStepId":   "run-older/graph-001/001-patch",
					"failedStepKind": "patch/apply",
					"failureClass":   failureClassPatchApply,
					"failedStepOutput": map[string]any{
						"reason": "older patch hunk mismatch",
					},
				},
			},
		},
	}
	current := &runmodel.Step{
		ID:    "run-process-context-rejected-result/step-1",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: "repair",
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustReturnValidJSON": true,
				},
			},
		},
	}

	orch := &RuntimeOrchestrator{steps: memrepo.NewStepRepository()}
	orch.enrichLLMStepInputContext(ctx, run, current, nil)

	digest := mapFromValue(current.Input[stepInputKeyCurrentProgressDigest])
	if !strings.Contains(stringFromMap(digest, "currentGap"), "read current implementation resource") {
		t.Fatalf("currentProgressDigest = %+v, want rejected result required evidence as current gap", digest)
	}
	if strings.Contains(stringFromMap(digest, "currentGap"), "older patch hunk mismatch") {
		t.Fatalf("currentProgressDigest = %+v, want older replan failure suppressed", digest)
	}
	pkg := mapFromValue(current.Input[stepInputKeyExecutionContextPackage])
	replan := mapFromValue(pkg["replanContinuation"])
	if replan["historicalOnly"] != true || replan["useAsCurrentProgress"] != false {
		t.Fatalf("replanContinuation = %+v, want historical-only marker when rejected result exists", replan)
	}
	timeline := mapSliceFromValue(pkg[stepInputKeyProcessEvidenceTimeline])
	if len(timeline) != 1 || timeline[0]["kind"] != "result/rejected" {
		t.Fatalf("processEvidenceTimeline = %+v, want rejected result synthetic evidence", timeline)
	}
	repairActionPackage := mapFromValue(current.Input["repairActionPackage"])
	currentFailure := mapFromValue(repairActionPackage["currentFailure"])
	if currentFailure["evidenceValidity"] != "actionable_gap" || !strings.Contains(stringFromMap(currentFailure, "reason"), "patch implementation resource") {
		t.Fatalf("repairActionPackage.currentFailure = %+v, want promoted rejected-result actionable gap", currentFailure)
	}
}

func TestRunExecutionContextPackageActionableReplanFailureSupersedesRejectedResultGap(t *testing.T) {
	ctx := context.Background()
	run := &runmodel.Run{
		ID:          "run-process-context-actionable-replan",
		WorkspaceID: "ws-process-context-actionable-replan",
		ExecutionContext: map[string]any{
			"rejectedResultContinuation": map[string]any{
				"predecessorRunId": "run-latest",
				"resultId":         "run-latest/result/dod-assessment",
				"summary":          "latest rejected result requires version control evidence",
				"requiredEvidence": []string{"version_control_snapshot", "coverage_map"},
			},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-before-replan",
				stepInputKeyCurrentProgressDigest: map[string]any{
					"currentGap": "invalid argument: state-changing llm repair step returned no state-changing output",
					"lastObservedFailure": map[string]any{
						"stepId":           "run-before-replan/repair-009",
						"kind":             llmWorkspaceExecutionStepKind,
						"failureClass":     failureClassRuntime,
						"failureReason":    "invalid argument: state-changing llm repair step returned no state-changing output",
						"evidenceValidity": "actionable_failure",
					},
				},
				stepInputKeyProcessEvidenceTimeline: []map[string]any{
					{
						"stepId":                    "run-before-replan/graph-001/009-verify-build",
						"kind":                      "shell/command",
						"state":                     string(common.StepStateFailed),
						"failureClass":              failureClassShellCommand,
						"failureSignature":          "class=shell_command_failed|kind=shell/command|failureTargets=pkg/lsp/server.go,main.go|exitCode=1",
						"failureReason":             "pkg/lsp/server.go:10:24: undefined: LSPInstance\nmain.go:28:28: undefined: NewServer",
						"concreteFailureDiagnostic": "pkg/lsp/server.go:10:24: undefined: LSPInstance\nmain.go:28:28: undefined: NewServer",
						"evidenceValidity":          "actionable_failure",
					},
					{
						"stepId":           "run-before-replan/repair-009",
						"kind":             llmWorkspaceExecutionStepKind,
						"state":            string(common.StepStateFailed),
						"failureClass":     failureClassRuntime,
						"failureReason":    "invalid argument: state-changing llm repair step returned no state-changing output",
						"failureSignature": "class=runtime_failed|kind=llm/workspace_execution|stage=llm_plan|error=invalid argument: state-changing llm repair step returned no state-changing output",
					},
				},
			},
		},
	}
	current := &runmodel.Step{
		ID:    "run-process-context-actionable-replan/step-1",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: "repair",
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustReturnValidJSON": true,
				},
			},
		},
	}

	orch := &RuntimeOrchestrator{steps: memrepo.NewStepRepository()}
	orch.enrichLLMStepInputContext(ctx, run, current, nil)

	digest := mapFromValue(current.Input[stepInputKeyCurrentProgressDigest])
	if stringFromMap(mapFromValue(digest["lastObservedFailure"]), "failureClass") != failureClassShellCommand ||
		!strings.Contains(stringFromMap(digest, "currentGap"), "LSPInstance") {
		t.Fatalf("currentProgressDigest = %+v, want actionable replan build failure", digest)
	}
	if strings.Contains(stringFromMap(digest, "currentGap"), "version_control_snapshot") {
		t.Fatalf("currentProgressDigest = %+v, want rejected result gap suppressed while concrete failure is open", digest)
	}
	pkg := mapFromValue(current.Input[stepInputKeyExecutionContextPackage])
	replan := mapFromValue(pkg["replanContinuation"])
	if replan["historicalOnly"] == true || replan["useAsCurrentProgress"] == false {
		t.Fatalf("replanContinuation = %+v, want active current-progress replan", replan)
	}
	timeline := mapSliceFromValue(pkg[stepInputKeyProcessEvidenceTimeline])
	if len(timeline) == 0 || timeline[0]["kind"] != "shell/command" {
		t.Fatalf("processEvidenceTimeline = %+v, want inherited concrete replan evidence", timeline)
	}
}

func TestRejectedResultContinuationCurrentGapPrefersDiagnosticsOverRequiredEvidence(t *testing.T) {
	ctx := context.Background()
	run := &runmodel.Run{
		ID:          "run-rejected-diagnostics-gap",
		WorkspaceID: "ws-rejected-diagnostics-gap",
		ExecutionContext: map[string]any{
			"rejectedResultContinuation": map[string]any{
				"summary": "DoD assessment rejected by runtime completion gate.",
				"requiredEvidence": []string{
					"可在后续迭代中添加更多断言。",
					"未来可以补充额外验证。",
				},
				"diagnostics": map[string]any{
					"gateFailureReason": "requires evidence text or coverageTargets aligned with the satisfied claim",
					"runtimeDiagnostics": map[string]any{
						"kind":                       "dod_evidence_quality_gap",
						"claimAlignedEvidenceNeeded": true,
					},
				},
			},
		},
	}
	current := &runmodel.Step{
		ID:    "run-rejected-diagnostics-gap/step-1",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{stepInputKeyRuntimeStage: "repair"},
	}

	orch := &RuntimeOrchestrator{steps: memrepo.NewStepRepository()}
	orch.enrichLLMStepInputContext(ctx, run, current, nil)

	digest := mapFromValue(current.Input[stepInputKeyCurrentProgressDigest])
	gap := stringFromMap(digest, "currentGap")
	if !strings.Contains(gap, "coverageTargets aligned") {
		t.Fatalf("currentProgressDigest = %+v, want gate diagnostics as current gap", digest)
	}
	if strings.Contains(gap, "后续迭代") || strings.Contains(gap, "未来可以") {
		t.Fatalf("currentProgressDigest = %+v, should not prefer generic requiredEvidence", digest)
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

func TestStepInputMemorySelectionForStrictRepairRequiresHistoricalBoundary(t *testing.T) {
	currentRunID := common.RunID("run-memory-strict-current")
	oldFailedRunID := common.RunID("run-memory-strict-failed")
	step := &runmodel.Step{
		ID:    "run-memory-strict-current/repair-001",
		RunID: currentRunID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageRepair,
			stepInputKeyRuntimeStageMode: "strict_convergence",
		},
	}
	selected, _, excluded := selectStepInputMemoryItemsForStep([]*memorymodel.MemoryItem{
		{
			ID:         "memory-failed-run-process",
			RunID:      oldFailedRunID,
			MemoryType: memorycommon.MemoryTypeExecutionLesson,
			Summary:    "Runtime repair lesson captured",
			Content:    "Repair attempt: 2\nfailureClass: runtime_failed",
			Importance: 100,
			Confidence: 100,
		},
		{
			ID:         "memory-failed-run-reference",
			RunID:      oldFailedRunID,
			MemoryType: memorycommon.MemoryTypeExecutionLesson,
			Summary:    "Missing file reads require optional probes or listing before repair",
			Content:    "scope: runtime repair reusable lesson\nprecondition: file/read target missing\nappliesTo: missing resource recovery",
			Tags:       []string{"reference_value"},
			Importance: 90,
			Confidence: 90,
		},
		{
			ID:         "memory-failed-run-generic-dod-feedback",
			RunID:      common.RunID("run-memory-strict-failed-generic-dod"),
			MemoryType: memorycommon.MemoryTypeDoDFeedback,
			Summary:    "DoD validation feedback",
			Content:    "Validation failed in a previous run.",
			Tags:       []string{"reference_value"},
			Importance: 95,
			Confidence: 95,
		},
		{
			ID:         "memory-failed-run-scoped-dod-feedback",
			RunID:      common.RunID("run-memory-strict-failed-scoped-dod"),
			MemoryType: memorycommon.MemoryTypeDoDFeedback,
			Summary:    "DoD validation feedback for scoped target",
			Content:    "scope: current acceptance boundary\nappliesTo: reusable verification gap",
			Tags:       []string{"reference_value"},
			Importance: 94,
			Confidence: 94,
		},
	}, currentRunID, map[common.RunID]common.RunState{
		oldFailedRunID:                         common.RunStateFailed,
		"run-memory-strict-failed-generic-dod": common.RunStateFailed,
		"run-memory-strict-failed-scoped-dod":  common.RunStateFailed,
	}, step, stepInputMemoryLimit)

	if memoryItemsContainID(selected, "memory-failed-run-process") {
		t.Fatalf("selected = %+v, want unbounded failed-run process memory excluded", selected)
	}
	if !memoryItemsContainID(selected, "memory-failed-run-reference") {
		t.Fatalf("selected = %+v, want bounded reference memory retained", selected)
	}
	if memoryItemsContainID(selected, "memory-failed-run-generic-dod-feedback") {
		t.Fatalf("selected = %+v, want generic historical DoD feedback excluded in strict repair", selected)
	}
	if !memoryItemsContainID(selected, "memory-failed-run-scoped-dod-feedback") {
		t.Fatalf("selected = %+v, want scoped historical DoD feedback retained", selected)
	}
	if !memoryItemsContainID(excluded, "memory-failed-run-process") {
		t.Fatalf("excluded = %+v, want unbounded failed-run memory in exclusion trace", excluded)
	}
	if !memoryItemsContainID(excluded, "memory-failed-run-generic-dod-feedback") {
		t.Fatalf("excluded = %+v, want generic historical DoD feedback in exclusion trace", excluded)
	}
}

func TestRemoveExcludedSelectedContextMemoriesDropsStaleStableMemories(t *testing.T) {
	selected := map[string]any{
		"memories": []map[string]any{
			{"id": "memory-excluded", "summary": "old runtime loop"},
			{"id": "memory-unselected", "summary": "not selected"},
		},
	}
	retrieval := map[string]any{
		"selectedMemoryIds": []string{},
		"excludedMemoryIds": []string{"memory-excluded"},
	}
	got := removeExcludedSelectedContextMemories(selected, retrieval)
	if _, ok := got["memories"]; ok {
		t.Fatalf("selectedContext.memories = %+v, want dropped when selectedMemoryIds is empty", got["memories"])
	}

	selected = map[string]any{
		"memories": []map[string]any{
			{"id": "memory-selected", "summary": "current reference"},
			{"id": "memory-excluded", "summary": "old runtime loop"},
		},
	}
	retrieval = map[string]any{
		"selectedMemoryIds": []string{"memory-selected"},
		"excludedMemoryIds": []string{"memory-excluded"},
	}
	got = removeExcludedSelectedContextMemories(selected, retrieval)
	memories := mapSliceFromValue(got["memories"])
	if len(memories) != 1 || memories[0]["id"] != "memory-selected" {
		t.Fatalf("selectedContext.memories = %+v, want only selected non-excluded memory", memories)
	}
}

func TestCompactCoverageWorkItemContextForRepairFocusKeepsOnlyFocusRejectedEvidenceDetails(t *testing.T) {
	focus := map[string]any{
		"id":                  "focus-cwi-primary",
		"coverageWorkItemIds": []string{"cwi-primary"},
		"targetRefs":          []string{"target-primary"},
	}
	context := map[string]any{
		"schemaVersion":      coverageWorkItemContextSchema,
		"definitionRevision": "rev-current",
		"currentFocus": map[string]any{
			"id":                   "cwi-primary",
			"targetText":           "primary closure",
			"rejectedEvidenceRefs": []string{"step://reject-primary"},
		},
		"unresolvedItems": []map[string]any{
			{
				"id":                   "cwi-primary",
				"targetText":           "primary closure",
				"targetRef":            "target-primary",
				"rejectedEvidenceRefs": []string{"step://reject-primary"},
				"rejectedEvidenceByKind": map[string][]string{
					"validation": {"step://reject-primary"},
				},
			},
			{
				"id":                   "cwi-secondary",
				"targetText":           "secondary closure",
				"rejectedEvidenceRefs": []string{"step://reject-secondary-a", "step://reject-secondary-b"},
				"rejectedEvidenceByKind": map[string][]string{
					"validation": {"step://reject-secondary-a", "step://reject-secondary-b"},
				},
			},
		},
	}

	got := compactCoverageWorkItemContextForRepairFocus(context, focus)
	unresolved := mapSliceFromValue(got["unresolvedItems"])
	if len(unresolved) != 2 {
		t.Fatalf("unresolvedItems = %+v, want focus and one summarized non-focus item", unresolved)
	}
	if refs := stringSliceFromValue(unresolved[0]["rejectedEvidenceRefs"]); len(refs) != 1 || refs[0] != "step://reject-primary" {
		t.Fatalf("focus rejected refs = %+v, want full focus refs", unresolved[0]["rejectedEvidenceRefs"])
	}
	if _, ok := unresolved[1]["rejectedEvidenceRefs"]; ok {
		t.Fatalf("non-focus item leaked rejectedEvidenceRefs: %+v", unresolved[1])
	}
	if unresolved[1]["rejectedEvidenceCount"] != 2 || unresolved[1]["evidenceDetailOmitted"] != true {
		t.Fatalf("non-focus summary = %+v, want rejected count and omission marker", unresolved[1])
	}
}

func TestCompactSelectedContextForCurrentDefinitionDropsStaleAndOldRevisionMemories(t *testing.T) {
	selected := map[string]any{
		"memories": []map[string]any{
			{"id": "memory-current", "summary": "current usable reference", "definitionRevision": "rev-current", "importance": 0.8, "confidence": 0.8},
			{"id": "memory-old", "summary": "old definition reference", "definitionRevision": "rev-old", "importance": 0.9, "confidence": 0.9},
			{"id": "memory-stale", "summary": "stale reference", "definitionRevision": "rev-current", "status": "stale", "importance": 0.9, "confidence": 0.9},
			{"id": "memory-low", "summary": "low value reference", "definitionRevision": "rev-current", "importance": 0.1, "confidence": 0.9},
		},
		"memoryRetrieval": map[string]any{"selectedMemoryIds": []string{"memory-current", "memory-old", "memory-stale", "memory-low"}},
	}

	got := compactSelectedContextForCurrentDefinition(selected, "rev-current")
	memories := mapSliceFromValue(got["memories"])
	if len(memories) != 1 || memories[0]["id"] != "memory-current" {
		t.Fatalf("selected memories = %+v, want only current high-value memory", memories)
	}
	compaction := mapFromValue(got["memoryCompaction"])
	if compaction["policy"] != "current_definition_high_value_only" || compaction["omittedCount"] != 3 {
		t.Fatalf("memoryCompaction = %+v, want current-definition omission summary", compaction)
	}
}

func TestCompactStepOutputForLLMContextSummarizesExecutionGraph(t *testing.T) {
	output := map[string]any{
		"summary": "historical graph output",
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{{
			ID:    "old-node",
			Kind:  "patch/apply",
			Input: map[string]any{"content": strings.Repeat("graph-noise ", 300)},
		}}},
	}

	got := compactStepOutputForLLMContext(output)
	if _, ok := got[stepOutputKeyExecutionGraph]; ok {
		t.Fatalf("compact output leaked executionGraph: %+v", got)
	}
	summary := mapFromValue(got["executionGraphSummary"])
	if summary["nodeCount"] != 1 {
		t.Fatalf("executionGraphSummary = %+v, want one-node summary", summary)
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "graph-noise") {
		t.Fatalf("compact output leaked graph node input: %s", encoded)
	}
}

func TestStepInputMemorySelectionExcludesTransientExecutionLesson(t *testing.T) {
	currentRunID := common.RunID("run-memory-current")
	selected, candidates, excluded := selectStepInputMemoryItemsForStep([]*memorymodel.MemoryItem{
		{
			ID:         "memory-truncated-snapshot-lesson",
			RunID:      currentRunID,
			MemoryType: memorycommon.MemoryTypeExecutionLesson,
			Summary:    "Missing file reads require optional probes or listing before repair",
			Content:    "scope: runtime repair reusable lesson\noutput.summary: contentTruncatedForPrompt=true; snapshot truncated before patch",
			Tags:       []string{"repair_loop", "necessary_output", "reference_value"},
			Importance: 100,
			Confidence: 100,
		},
		{
			ID:         "memory-reference-lesson",
			RunID:      currentRunID,
			MemoryType: memorycommon.MemoryTypeExecutionLesson,
			Summary:    "Missing file reads require optional probes or listing before repair",
			Content:    "scope: runtime repair reusable lesson\nprecondition: file/read target missing\nappliesTo: missing resource recovery\nevidenceRef: run/block/read",
			Tags:       []string{"reference_value"},
			Importance: 90,
			Confidence: 90,
		},
	}, currentRunID, nil, nil, stepInputMemoryLimit)

	if memoryItemsContainID(selected, "memory-truncated-snapshot-lesson") {
		t.Fatalf("selected = %+v, want transient truncated-snapshot execution lesson excluded", selected)
	}
	if !memoryItemsContainID(selected, "memory-reference-lesson") {
		t.Fatalf("selected = %+v, want stable reference lesson retained", selected)
	}
	if memoryItemsContainID(candidates, "memory-truncated-snapshot-lesson") ||
		memoryItemsContainID(excluded, "memory-truncated-snapshot-lesson") {
		t.Fatalf("candidates=%+v excluded=%+v, want duplicate transient lesson replaced before scoring trace", candidates, excluded)
	}
	reason := stepInputMemoryScoreReason(&memorymodel.MemoryItem{
		ID:         "memory-truncated-snapshot-lesson",
		RunID:      currentRunID,
		MemoryType: memorycommon.MemoryTypeExecutionLesson,
		Summary:    "contentTruncatedForPrompt=true",
		Content:    "snapshot truncated",
	}, currentRunID, nil)
	if !strings.Contains(reason, "excluded_low_value_process_memory") {
		t.Fatalf("score reason = %q, want low-value process exclusion", reason)
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
