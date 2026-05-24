package orchestrator

import (
	"strings"
	"testing"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestLatestFailedRepairableGraphStepSkipsSupersededFailure(t *testing.T) {
	runID := common.RunID("run-graph-superseded-failure")
	oldFailure := &runModel.Step{
		ID:    common.StepID(runID + "/graph-004/002-rerun-lint"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "rerun-lint",
			stepInputKeyGraphRootStepID: string(runID) + "/step-1",
			"targetRef":                 "web/src",
		},
	}
	laterSuccess := &runModel.Step{
		ID:    common.StepID(runID + "/graph-005/002-rerun-lint"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "rerun-lint",
			stepInputKeyGraphRootStepID: string(runID) + "/step-1",
			"targetRef":                 "web/src",
		},
	}
	if got := latestFailedRepairableGraphStep([]*runModel.Step{oldFailure, laterSuccess}); got != nil {
		t.Fatalf("latestFailedRepairableGraphStep = %+v, want nil because later success superseded the failure", got)
	}
}

func TestProcessEvidenceTimelineOmitsRejectedLLMDecisionSummary(t *testing.T) {
	runID := common.RunID("run-rejected-llm-summary")
	step := &runModel.Step{
		ID:    common.StepID(runID + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan",
			"error":                   "invalid argument: rejected graph",
			"summary":                 "stale model hypothesis should not be reused",
		},
	}
	run := &runModel.Run{ID: runID, WorkspaceID: "ws-rejected-llm-summary"}

	item := processEvidenceTimelineItem(run, step, 1)
	if item["summary"] != nil || item["outputSummary"] != nil {
		t.Fatalf("timeline item = %+v, want rejected LLM summary omitted", item)
	}
	if item["evidenceValidity"] != "stale_or_rejected" || item["rejectedOutputSummaryOmitted"] != true {
		t.Fatalf("timeline item = %+v, want stale/rejected marker", item)
	}
	digest := currentProgressDigestFromProcessEvidenceTimeline([]map[string]any{item})
	last := mapFromValue(digest["lastObservedFailure"])
	if last["summary"] != nil {
		t.Fatalf("digest last failure = %+v, want no stale summary", last)
	}
	if digest["currentGap"] != "invalid argument: rejected graph" {
		t.Fatalf("digest currentGap = %v, want runtime failure reason", digest["currentGap"])
	}
}

func TestProcessEvidenceTimelineTreatsCompletedShellWithFailureMarkersAsActionableFailure(t *testing.T) {
	runID := common.RunID("run-completed-shell-failure-markers")
	step := &runModel.Step{
		ID:    common.StepID(runID + "/graph-001/002-run-tests"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyRuntimeStage:        runtimeStageExecution,
			stepInputKeyGraphNodeID:         "run-tests",
			stepInputKeyGraphRootStepID:     string(runID) + "/step-1",
			stepInputKeyGraphCompletedNodes: []string{"read-main"},
		},
		Output: map[string]any{
			"command":  "sh",
			"args":     []string{"-c", "go test ./... 2>&1 | head -100"},
			"exitCode": 0,
			"stdout": "=== RUN   TestProxyHandlerBasic\n" +
				"    main_test.go:28: Error Trace: /workspace/gateway/main_test.go:28\n" +
				"        Error: Not equal:\n" +
				"            expected: \"/v1/chat/completions\"\n" +
				"            actual  : \"/chat/completions\"\n" +
				"--- FAIL: TestProxyHandlerBasic (0.04s)\n" +
				"FAIL\n",
		},
	}
	run := &runModel.Run{ID: runID, WorkspaceID: "ws-completed-shell-failure-markers"}

	item := processEvidenceTimelineItem(run, step, 1)
	if item["state"] != string(common.StepStateFailed) ||
		item["failureClass"] != failureClassShellCommand ||
		item["evidenceValidity"] != "actionable_failure" {
		t.Fatalf("timeline item = %+v, want actionable shell failure", item)
	}
	if !strings.Contains(stringFromMap(item, "failureReason"), "main_test.go:28") {
		t.Fatalf("failureReason = %v, want test failure output", item["failureReason"])
	}
	digest := currentProgressDigestFromProcessEvidenceTimeline([]map[string]any{item})
	if !strings.Contains(stringFromMap(digest, "currentGap"), "main_test.go:28") {
		t.Fatalf("currentGap = %v, want shell failure output", digest["currentGap"])
	}
	last := mapFromValue(digest["lastObservedFailure"])
	if last["stepId"] != string(step.ID) || last["failureClass"] != failureClassShellCommand {
		t.Fatalf("lastObservedFailure = %+v, want shell failure step", last)
	}
}

func TestCurrentProgressDigestPrefersConcreteDiagnosticsOverGenericExitStatus(t *testing.T) {
	runID := common.RunID("run-concrete-diagnostic")
	step := &runModel.Step{
		ID:    common.StepID(runID + "/graph-003/002-verify"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass: failureClassShellCommand,
			"error":               "exit status 1",
			"stderr": "proxy/proxy_test.go:32:10: undefined: detectProvider\n" +
				"proxy/proxy_test.go:47:3: unknown field APIKeys in struct literal of type config.Config\n",
		},
	}
	run := &runModel.Run{ID: runID, WorkspaceID: "ws-concrete-diagnostic"}

	item := processEvidenceTimelineItem(run, step, 1)
	if !strings.Contains(stringFromMap(item, "failureReason"), "undefined: detectProvider") {
		t.Fatalf("failureReason = %v, want concrete stderr diagnostic", item["failureReason"])
	}

	digest := currentProgressDigestFromProcessEvidenceTimeline([]map[string]any{item})
	if !strings.Contains(stringFromMap(digest, "currentGap"), "proxy/proxy_test.go:32:10") {
		t.Fatalf("currentGap = %v, want concrete target diagnostic", digest["currentGap"])
	}
	failure := currentFailureFromProgressDigest(digest)
	if !strings.Contains(stringFromMap(failure, "reason"), "unknown field APIKeys") {
		t.Fatalf("currentFailure = %+v, want concrete diagnostic reason", failure)
	}
}

func TestCurrentProgressDigestDoesNotPromoteSuccessfulActionEvidenceToFailure(t *testing.T) {
	input := map[string]any{
		"currentExecutionProgress": map[string]any{
			"failureClass": failureClassShellCommand,
			"latestActionEvidence": []map[string]any{{
				"stepId": "run-001/graph-001/002-test",
				"kind":   "shell/command",
				"state":  string(common.StepStateCompleted),
				"source": "latest_successful_action_evidence",
				"output": map[string]any{
					"stdout": "=== RUN   TestProxy\n--- PASS: TestProxy (0.00s)\nPASS\nok  \tmodule/proxy\t0.028s",
				},
			}},
		},
	}

	if digest := currentProgressDigestFromCurrentExecutionProgress(input, nil); len(digest) > 0 {
		t.Fatalf("currentProgressDigest = %+v, want successful action evidence to remain non-failure context", digest)
	}
}

func TestEvidenceAcquisitionFollowUpKeepsCurrentFailure(t *testing.T) {
	runID := common.RunID("run-evidence-follow-up-current-failure")
	source := &runModel.Step{
		ID:    common.StepID(runID + "/step-1"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateCompleted,
		Input: map[string]any{
			"concreteFailureEvidenceRequired": true,
			"failedStepId":                    string(runID) + "/repair-001",
			"failedStepKind":                  llmWorkspaceExecutionStepKind,
			failureOutputKeyClass:             failureClassLLMDecisionInvalid,
			failureOutputKeySignature:         "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid",
			"currentProgressDigest": map[string]any{
				"currentGap":         "invalid llm decision json",
				"nextRequiredAction": "address_last_failure_under_current_stage_contract",
			},
			"repairActionPackage": map[string]any{
				"failedStepId":                                string(runID) + "/repair-001",
				"failedStepKind":                              llmWorkspaceExecutionStepKind,
				failureOutputKeyClass:                         failureClassLLMDecisionInvalid,
				failureOutputKeySignature:                     "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid",
				"requiresCurrentTargetSnapshotForStateChange": true,
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
				},
			},
		},
	}
	input := map[string]any{
		"workspaceId": string(common.WorkspaceID("ws-evidence-follow-up-current-failure")),
	}
	evidence := []map[string]any{
		{
			"kind":   "file/read",
			"stepId": string(runID) + "/graph-001/001-read-target",
			"output": map[string]any{
				"stage":       "file_read",
				"status":      "completed",
				"path":        "target.txt",
				"exists":      true,
				"content":     "current content",
				"contentHash": "hash-current",
				"size":        15,
				"truncated":   false,
			},
		},
	}

	out := restoreRepairContractAfterConcreteEvidenceAcquisition(input, source, evidence)
	pkg := mapFromValue(out["repairActionPackage"])
	currentFailure := mapFromValue(pkg["currentFailure"])
	if currentFailure["failedStepId"] != string(runID)+"/repair-001" ||
		currentFailure["failureClass"] != failureClassLLMDecisionInvalid {
		t.Fatalf("currentFailure = %+v, want inherited failed step and failure class", currentFailure)
	}
	progress := mapFromValue(pkg["currentExecutionProgress"])
	if progress["currentGap"] != "invalid llm decision json" {
		t.Fatalf("currentExecutionProgress = %+v, want source progress digest", progress)
	}
	if !boolValue(out["stateChangingOutputRequired"]) || out["repairObjective"] != repairObjectiveStateChangingOutput {
		t.Fatalf("follow-up contract = %+v, want state-changing repair", out)
	}
}

func TestEvidenceAcquisitionFollowUpPrefersRejectedResultGapOverHistoricalReplan(t *testing.T) {
	runID := common.RunID("run-evidence-follow-up-rejected-result")
	source := &runModel.Step{
		ID:    common.StepID(runID + "/step-1"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateCompleted,
		Input: map[string]any{
			"concreteFailureEvidenceRequired": true,
			"failedStepId":                    string(runID) + "/repair-001",
			"failedStepKind":                  llmWorkspaceExecutionStepKind,
			failureOutputKeyClass:             "rejected_result_continuation",
			"currentProgressDigest": map[string]any{
				"currentGap":         "read current resource\npatch current resource\nrun bounded verification",
				"nextRequiredAction": "address_rejected_result_required_evidence",
				"lastObservedFailure": map[string]any{
					"kind":             "result/rejected",
					"failureClass":     "rejected_result_continuation",
					"failureReason":    "read current resource\npatch current resource\nrun bounded verification",
					"source":           "rejectedResultContinuation",
					"evidenceValidity": "actionable_gap",
				},
			},
			"executionContext": map[string]any{
				"rejectedResultContinuation": map[string]any{
					"requiredEvidence": []string{"read current resource", "patch current resource", "run bounded verification"},
				},
				"replanContinuation": map[string]any{
					"currentProgressDigest": map[string]any{
						"currentGap": "older patch mismatch",
					},
				},
			},
			"repairActionPackage": map[string]any{
				"requiresCurrentTargetSnapshotForStateChange": true,
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
				},
			},
		},
	}
	input := map[string]any{
		"workspaceId": string(common.WorkspaceID("ws-evidence-follow-up-rejected-result")),
	}
	evidence := []map[string]any{
		{
			"kind":   "file/read",
			"stepId": string(runID) + "/graph-001/001-read-target",
			"output": map[string]any{
				"stage":       "file_read",
				"status":      "completed",
				"path":        "target.txt",
				"exists":      true,
				"content":     "current content",
				"contentHash": "hash-current",
				"size":        15,
				"truncated":   false,
			},
		},
	}

	out := restoreRepairContractAfterConcreteEvidenceAcquisition(input, source, evidence)
	pkg := mapFromValue(out["repairActionPackage"])
	progress := mapFromValue(pkg["currentExecutionProgress"])
	if strings.Contains(stringFromMap(progress, "currentGap"), "older patch mismatch") ||
		!strings.Contains(stringFromMap(progress, "currentGap"), "patch current resource") {
		t.Fatalf("currentExecutionProgress = %+v, want rejected-result gap instead of historical replan", progress)
	}
	currentFailure := mapFromValue(pkg["currentFailure"])
	if currentFailure["evidenceValidity"] != "actionable_gap" || currentFailure["source"] != "rejectedResultContinuation" {
		t.Fatalf("currentFailure = %+v, want rejected-result actionable gap", currentFailure)
	}
}
