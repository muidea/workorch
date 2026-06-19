package orchestrator

import (
	"context"
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

func TestMCPRuntimeFailureTargetScopePrefersCapabilityNodeOverInternalDiagnostics(t *testing.T) {
	shellScope := runtimeFailureTargetScope("class=mcp_runtime_failed|kind=mcp/tool_call|stage=mcp_tool_call_failed|node=init-sh-lsp|serverId=mcp-lsp-bridge|tool=lsp_initialize|failureTargets=client.go|error=missing shell server")
	if shellScope != "serverId=mcp-lsp-bridge|tool=lsp_initialize|node=init-sh-lsp" {
		t.Fatalf("shell scope = %q, want capability call node scope", shellScope)
	}
	tsScope := runtimeFailureTargetScope("class=mcp_runtime_failed|kind=mcp/tool_call|stage=mcp_tool_call_failed|node=init-ts-lsp|serverId=mcp-lsp-bridge|tool=lsp_initialize|failureTargets=client.go|error=missing workspace dependency")
	if tsScope != "serverId=mcp-lsp-bridge|tool=lsp_initialize|node=init-ts-lsp" {
		t.Fatalf("ts scope = %q, want capability call node scope", tsScope)
	}
	if shellScope == tsScope {
		t.Fatalf("scopes should differ by graph node, got %q", shellScope)
	}
}

func TestNoActionableFollowUpSkipsFailedAssessmentWithActionableGap(t *testing.T) {
	run := &runModel.Run{DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runModel.Step{
		ID:    "run-actionable-assessment/follow-up",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{},
	}
	output := map[string]any{
		"completionState": "failed",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "coverage gap remains",
			"items": []map[string]any{{
				"item":  "required behavior",
				"state": "failed",
			}},
		},
	}

	if shouldDispatchNoActionableContinuationFollowUp(run, step, output) {
		t.Fatalf("shouldDispatchNoActionableContinuationFollowUp = true, want direct runtime repair for actionable failed assessment")
	}
}

func TestCurrentProgressDigestClearsFailureSupersededByRepairSuccess(t *testing.T) {
	runID := common.RunID("run-progress-superseded-failure")
	failedStepID := string(runID) + "/graph-001/002-check-target"
	timeline := []map[string]any{
		{
			"order":            1,
			"stepId":           failedStepID,
			"kind":             "shell/command",
			"state":            string(common.StepStateFailed),
			"failureClass":     failureClassShellCommand,
			"failureReason":    "target check failed",
			"evidenceValidity": "actionable_failure",
		},
		{
			"order":                    2,
			"stepId":                   string(runID) + "/graph-002/001-verify-target",
			"kind":                     "shell/command",
			"state":                    string(common.StepStateCompleted),
			stepInputKeyRepairOfStepID: failedStepID,
			"summary":                  "target check passed",
		},
	}

	digest := currentProgressDigestFromProcessEvidenceTimeline(timeline)
	if failure := mapFromValue(digest["lastObservedFailure"]); len(failure) > 0 {
		t.Fatalf("lastObservedFailure = %+v, want superseded failure cleared", failure)
	}
	if digest["nextRequiredAction"] != "continue_current_stage_contract" {
		t.Fatalf("digest = %+v, want continuation after superseding success", digest)
	}
}

func TestCurrentProgressDigestPreservesStructuredFailureTargetScope(t *testing.T) {
	runID := common.RunID("run-progress-target-scope")
	step := &runModel.Step{
		ID:    common.StepID(runID + "/graph-001/002-check-target"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|stage=validation",
			"failureTargetScope":      "failureTargets=target.txt",
			"error":                   "target check failed",
		},
	}
	run := &runModel.Run{ID: runID, WorkspaceID: "ws-progress-target-scope"}

	item := processEvidenceTimelineItem(run, step, 1)
	if item["failureTargetScope"] != "failureTargets=target.txt" || item["targetScope"] != "failureTargets=target.txt" {
		t.Fatalf("timeline item = %+v, want structured target scope", item)
	}
	digest := currentProgressDigestFromProcessEvidenceTimeline([]map[string]any{item})
	last := mapFromValue(digest["lastObservedFailure"])
	if last["failureTargetScope"] != "failureTargets=target.txt" || last["targetScope"] != "failureTargets=target.txt" {
		t.Fatalf("lastObservedFailure = %+v, want target scope preserved", last)
	}
	failure := currentFailureFromProgressDigest(digest)
	if failure["failureTargetScope"] != "failureTargets=target.txt" || failure["targetScope"] != "failureTargets=target.txt" {
		t.Fatalf("currentFailure = %+v, want target scope preserved", failure)
	}
	compact := compactProcessEvidenceTimelineForRuntimeContext([]map[string]any{item})
	if len(compact) != 1 || compact[0]["failureTargetScope"] != "failureTargets=target.txt" {
		t.Fatalf("compact timeline = %+v, want target scope preserved", compact)
	}
}

func TestCurrentProgressDigestPrefersUnresolvedConcreteFailureOverLaterCoverageGap(t *testing.T) {
	timeline := []map[string]any{
		{
			"order":              1,
			"stepId":             "run-001/graph-001/001-verify",
			"kind":               "shell/command",
			"state":              string(common.StepStateFailed),
			"failureClass":       failureClassShellCommand,
			"failureSignature":   "class=shell_command_failed|kind=shell/command|failureTargets=target.txt|exitCode=1",
			"failureReason":      "exit status 1",
			"evidenceValidity":   "actionable_failure",
			"failureTargetScope": "failureTargets=target.txt",
			"targetScope":        "failureTargets=target.txt",
			"outputSummary":      map[string]any{"stderrPreview": "target.txt:10: invalid shape"},
		},
		{
			"order":              2,
			"stepId":             "run-001/repair-001",
			"kind":               llmWorkspaceExecutionStepKind,
			"state":              string(common.StepStateFailed),
			"failureClass":       failureClassDoDCoverageMissing,
			"failureSignature":   "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
			"failureReason":      "state-changing repair returned assessment without executable output",
			"evidenceValidity":   "actionable_gap",
			"failureTargetScope": "failureTargets=target.txt",
			"targetScope":        "failureTargets=target.txt",
			"summary":            "latest assessment still reports target.txt verification failure",
		},
	}

	digest := currentProgressDigestFromProcessEvidenceTimeline(timeline)
	failure := mapFromValue(digest["lastObservedFailure"])
	if failure["stepId"] != "run-001/graph-001/001-verify" ||
		failure["failureClass"] != failureClassShellCommand ||
		failure["failureTargetScope"] != "failureTargets=target.txt" {
		t.Fatalf("lastObservedFailure = %+v, want unresolved concrete shell failure", failure)
	}
	currentFailure := currentFailureFromProgressDigest(digest)
	if currentFailure["failedStepId"] != "run-001/graph-001/001-verify" ||
		currentFailure["failureClass"] != failureClassShellCommand {
		t.Fatalf("currentFailure = %+v, want concrete shell failure", currentFailure)
	}
}

func TestProcessEvidenceTimelineKeepsCreationOrderAndSupersedesOldRepairGap(t *testing.T) {
	runID := common.RunID("run-progress-creation-order")
	run := &runModel.Run{ID: runID, WorkspaceID: "ws-progress-creation-order"}
	failureScope := "failureTargets=target.txt"
	steps := []*runModel.Step{
		{
			ID:    common.StepID(runID + "/graph-003/002-verify"),
			RunID: runID,
			Kind:  "shell/command",
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyGraphNodeID: "verify",
				"failureTargetScope":    failureScope,
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassShellCommand,
				failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=target.txt|exitCode=1",
				"error":                   "exit status 1",
			},
		},
		{
			ID:    common.StepID(runID + "/repair-007-of-graph-002-002-verify"),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRepairOfStepID: string(runID + "/graph-002/002-verify"),
				"failureTargetScope":       failureScope,
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassDoDCoverageMissing,
				failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
				"reason":                  "old repair gap",
			},
		},
		{
			ID:    common.StepID(runID + "/graph-004/002-verify"),
			RunID: runID,
			Kind:  "shell/command",
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyGraphNodeID:    "verify",
				stepInputKeyRepairOfStepID: string(runID + "/graph-003/002-verify"),
				"failureTargetScope":       failureScope,
				"validationEvidence": map[string]any{
					"mode":            "runtime_behavior",
					"observationType": "runtime_observation",
				},
			},
			Output: map[string]any{
				"status":   "completed",
				"command":  "verify",
				"exitCode": 0,
			},
		},
	}

	timeline := processEvidenceTimelineFromRunSteps(run, steps, "")
	if len(timeline) != 3 {
		t.Fatalf("timeline = %+v, want three items", timeline)
	}
	if timeline[0]["stepId"] != string(steps[0].ID) || timeline[2]["stepId"] != string(steps[2].ID) {
		t.Fatalf("timeline order = %+v, want repository creation order", timeline)
	}
	if validation := mapFromValue(timeline[2]["validationEvidence"]); validation["mode"] != "runtime_behavior" {
		t.Fatalf("validationEvidence = %+v, want retained validation metadata", timeline[2]["validationEvidence"])
	}
	digest := currentProgressDigestFromProcessEvidenceTimeline(timeline)
	if failure := mapFromValue(digest["lastObservedFailure"]); len(failure) > 0 {
		t.Fatalf("lastObservedFailure = %+v, want later scoped success to supersede old failure and repair gap", failure)
	}
	if digest["nextRequiredAction"] != "continue_current_stage_contract" {
		t.Fatalf("digest = %+v, want continuation after latest scoped success", digest)
	}
}

func TestCompletionEvidenceCatalogKeepsRepositoryOrderForLatestUsableScope(t *testing.T) {
	runID := common.RunID("run-completion-catalog-order")
	scope := "failureTargets=target.txt"
	oldFailure := &runModel.Step{
		ID:    common.StepID(runID + "/graph-003/002-verify"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Input: map[string]any{
			"failureTargetScope": scope,
		},
		Output: map[string]any{
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=target.txt|exitCode=1",
		},
	}
	oldRepairGap := &runModel.Step{
		ID:    common.StepID(runID + "/repair-007-of-graph-002-002-verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			"failureTargetScope": scope,
		},
		Output: map[string]any{
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
		},
	}
	laterSuccess := &runModel.Step{
		ID:    common.StepID(runID + "/graph-004/002-verify"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"failureTargetScope": scope,
		},
	}
	repo := orderedStepRepository{steps: []*runModel.Step{oldFailure, oldRepairGap, laterSuccess}}

	catalog, err := completionEvidenceRefCatalogForRun(context.Background(), repo, RuntimeEvidenceRepositories{}, &runModel.Run{ID: runID})
	if err != nil {
		t.Fatalf("completionEvidenceRefCatalogForRun: %v", err)
	}
	if got := catalog.latestUsableStepByScope[scope]; got != string(laterSuccess.ID) {
		t.Fatalf("latest usable step for scope = %q, want %q", got, laterSuccess.ID)
	}
	if got := catalog.supersededFailureStepRefs[string(oldFailure.ID)]; got != string(laterSuccess.ID) {
		t.Fatalf("old failure supersededBy = %q, want %q", got, laterSuccess.ID)
	}
	if got := catalog.supersededFailureStepRefs[string(oldRepairGap.ID)]; got != string(laterSuccess.ID) {
		t.Fatalf("old repair gap supersededBy = %q, want %q", got, laterSuccess.ID)
	}
}

func TestLatestActionEvidenceCandidatesKeepRepositoryOrder(t *testing.T) {
	runID := common.RunID("run-latest-action-order")
	earlySemanticRepair := &runModel.Step{
		ID:    common.StepID(runID + "/repair-007-of-graph-002-002-verify"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"summary": "older repair action",
		},
	}
	laterSemanticGraph := &runModel.Step{
		ID:    common.StepID(runID + "/graph-004/002-verify"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"summary": "later validation action",
		},
	}

	candidates := latestActionEvidenceCandidatesInSteps([]*runModel.Step{earlySemanticRepair, laterSemanticGraph}, nil, 2)
	if len(candidates) != 2 {
		t.Fatalf("candidates = %+v, want two", candidates)
	}
	if candidates[0]["stepId"] != string(laterSemanticGraph.ID) {
		t.Fatalf("first latest candidate = %+v, want later repository step", candidates[0])
	}
}

func TestRejectedResultContinuationPreservesStructuredFailureTargetScope(t *testing.T) {
	continuation := map[string]any{
		"summary":            "result still lacks required evidence",
		"failureTargetScope": "failureTargets=target.txt",
	}

	timeline := processEvidenceTimelineFromRejectedResultContinuation(continuation)
	if len(timeline) != 1 || timeline[0]["failureTargetScope"] != "failureTargets=target.txt" {
		t.Fatalf("timeline = %+v, want rejected result target scope", timeline)
	}
	digest := currentProgressDigestFromRejectedResultContinuation(continuation, timeline)
	failure := mapFromValue(digest["lastObservedFailure"])
	if failure["failureTargetScope"] != "failureTargets=target.txt" || failure["targetScope"] != "failureTargets=target.txt" {
		t.Fatalf("lastObservedFailure = %+v, want rejected result target scope", failure)
	}
}

type orderedStepRepository struct {
	steps []*runModel.Step
}

func (r orderedStepRepository) Create(context.Context, *runModel.Step) error { return nil }

func (r orderedStepRepository) Get(_ context.Context, id common.StepID) (*runModel.Step, error) {
	for _, step := range r.steps {
		if step != nil && step.ID == id {
			return step, nil
		}
	}
	return nil, common.ErrNotFound
}

func (r orderedStepRepository) ListByRun(context.Context, common.RunID) ([]*runModel.Step, error) {
	return append([]*runModel.Step(nil), r.steps...), nil
}

func (r orderedStepRepository) ListByRunSummary(context.Context, common.RunID) ([]*runModel.Step, error) {
	return append([]*runModel.Step(nil), r.steps...), nil
}

func (r orderedStepRepository) Update(context.Context, *runModel.Step) error { return nil }

func (r orderedStepRepository) Delete(context.Context, common.StepID) error { return nil }

func TestCurrentExecutionProgressPromotionPreservesStructuredFailureTargetScope(t *testing.T) {
	input := map[string]any{
		"currentExecutionProgress": map[string]any{
			"latestActionEvidence": []map[string]any{{
				"stepId":             "run-001/graph-001/002-check-target",
				"kind":               "shell/command",
				"state":              string(common.StepStateFailed),
				"failureTargetScope": "failureTargets=target.txt",
				"outputSummary": map[string]any{
					"stderrPreview": "target check failed",
				},
			}},
		},
	}

	digest := currentProgressDigestFromCurrentExecutionProgress(input, nil)
	failure := mapFromValue(digest["lastObservedFailure"])
	if failure["failureTargetScope"] != "failureTargets=target.txt" || failure["targetScope"] != "failureTargets=target.txt" {
		t.Fatalf("lastObservedFailure = %+v, want promoted target scope", failure)
	}
	currentFailure := currentFailureFromProgressDigest(digest)
	if currentFailure["failureTargetScope"] != "failureTargets=target.txt" {
		t.Fatalf("currentFailure = %+v, want target scope", currentFailure)
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

func TestEvidenceAcquisitionFollowUpUsesCoverageFocusAsActionableGap(t *testing.T) {
	input := map[string]any{
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-source-change",
				"status":                "open",
				"sourceRequirementId":   "source_change",
				"sourceScope":           "deliverable",
				"sourcePath":            "executionContract.profileContracts.generic.deliverableExpectations[0]",
				"targetText":            "source_change",
				"currentGap":            "missing accepted evidence binding for required evidence kind: validation, state_change",
				"missingEvidenceKinds":  []string{"validation", "state_change"},
				"requiredEvidenceKinds": []string{"validation", "state_change"},
			},
		},
	}

	failure := currentFailureForEvidenceAcquisitionFollowUp(input, nil)

	if failure["failureClass"] != failureClassCoverageActionableGap ||
		failure["evidenceValidity"] != "actionable_gap" ||
		failure["coverageWorkItemId"] != "cwi-source-change" ||
		failure["sourceRequirementId"] != "source_change" {
		t.Fatalf("currentFailure = %+v, want coverage actionable gap", failure)
	}
	if !strings.Contains(stringFromMap(failure, "reason"), "validation") ||
		!strings.Contains(stringFromMap(failure, "reason"), "state_change") {
		t.Fatalf("currentFailure.reason = %q, want missing evidence kinds", stringFromMap(failure, "reason"))
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

func TestProcessEvidenceTimelineIncludesArtifactContinuationMetadataOnly(t *testing.T) {
	runID := common.RunID("run-artifact-timeline")
	step := &runModel.Step{
		ID:    common.StepID(runID + "/artifact-apply-001"),
		RunID: runID,
		Kind:  artifactApplyStepKind,
		State: common.StepStateCompleted,
		Output: map[string]any{
			"artifactContinuationApply": map[string]any{
				"id":                "artifact-continuation-001",
				"artifactId":        "artifact-001",
				"targetRef":         "deliverable/target.txt",
				"state":             string(common.ArtifactContinuationStateApplied),
				"bytes":             128,
				"rollingHash":       "hash-001",
				"applyEvidenceRefs": []string{string(runID) + "/artifact-apply-001/patch/1"},
				"content":           "large content must not be projected",
			},
		},
	}
	run := &runModel.Run{ID: runID, WorkspaceID: "ws-artifact-timeline"}

	item := processEvidenceTimelineItem(run, step, 1)
	artifact := mapFromValue(item["artifactContinuation"])
	if artifact["targetRef"] != "deliverable/target.txt" || artifact["rollingHash"] != "hash-001" {
		t.Fatalf("artifactContinuation = %+v, want metadata", artifact)
	}
	if artifact["content"] != nil {
		t.Fatalf("artifactContinuation = %+v, want no content", artifact)
	}
	compact := compactProcessEvidenceTimelineForRuntimeContext([]map[string]any{item})
	compactArtifact := mapFromValue(compact[0]["artifactContinuation"])
	if compactArtifact["targetRef"] != "deliverable/target.txt" || compactArtifact["content"] != nil {
		t.Fatalf("compact artifactContinuation = %+v, want metadata only", compactArtifact)
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

func TestEvidenceAcquisitionFollowUpCopiesFollowUpProgressIntoRepairActionPackage(t *testing.T) {
	runID := common.RunID("run-evidence-follow-up-progress")
	source := &runModel.Step{
		ID:    common.StepID(runID + "/step-1"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateCompleted,
		Input: map[string]any{
			"concreteFailureEvidenceRequired": true,
			"repairActionPackage": map[string]any{
				"coverageRepairRequired": true,
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
				},
			},
		},
	}
	input := map[string]any{
		"workspaceId": string(common.WorkspaceID("ws-evidence-follow-up-progress")),
		"currentProgressDigest": map[string]any{
			"currentGap":         "none_observed_in_recent_process_evidence",
			"nextRequiredAction": "continue_current_stage_contract",
			"lastAttempt": map[string]any{
				"stepId": "run-evidence-follow-up-progress/graph-001/002-collect-evidence",
				"kind":   "shell/command",
				"state":  string(common.StepStateCompleted),
			},
		},
	}
	evidence := []map[string]any{{
		"kind":   "file/read",
		"stepId": string(runID) + "/graph-001/001-read-target",
		"output": map[string]any{
			"stage":       "file_read",
			"status":      "completed",
			"path":        "README.md",
			"exists":      true,
			"content":     "current content",
			"contentHash": "hash-current",
			"size":        15,
			"truncated":   false,
		},
	}}

	out := restoreRepairContractAfterConcreteEvidenceAcquisition(input, source, evidence)
	pkg := mapFromValue(out["repairActionPackage"])
	progress := mapFromValue(pkg["currentExecutionProgress"])
	if len(progress) == 0 {
		t.Fatalf("repairActionPackage.currentExecutionProgress = %+v, want follow-up progress digest", pkg["currentExecutionProgress"])
	}
	if progress["nextRequiredAction"] != "continue_current_stage_contract" {
		t.Fatalf("currentExecutionProgress = %+v, want follow-up digest", progress)
	}
	if len(mapFromValue(out["currentExecutionProgress"])) == 0 {
		t.Fatalf("currentExecutionProgress = %+v, want top-level copy for provider context gate", out["currentExecutionProgress"])
	}
}

func TestEvidenceAcquisitionFollowUpPreservesLastFailureDiagnostics(t *testing.T) {
	runID := common.RunID("run-evidence-follow-up-diagnostics")
	source := &runModel.Step{
		ID:    common.StepID(runID + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateCompleted,
		Input: map[string]any{
			"concreteFailureEvidenceRequired": true,
			"failedStepOutput": map[string]any{
				"error":        "exit status 1",
				"failureClass": failureClassShellCommand,
				"lastFailureDiagnostics": map[string]any{
					"failureClass":  failureClassShellCommand,
					"error":         "exit status 1",
					"stderrPreview": "target.txt:12:3: expected value",
					"stdoutPreview": "failed",
					"stepId":        string(runID) + "/graph-001/002-verify",
					"stepKind":      "shell/command",
				},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteFailureEvidence": true,
					"acceptedKinds":                      []string{"file/read"},
				},
			},
		},
	}
	input := map[string]any{
		"workspaceId": string(common.WorkspaceID("ws-evidence-follow-up-diagnostics")),
	}
	evidence := []map[string]any{{
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
	}}

	out := restoreRepairContractAfterConcreteEvidenceAcquisition(input, source, evidence)
	topLevel := mapFromValue(out["lastFailureDiagnostics"])
	if !strings.Contains(stringFromMap(topLevel, "stderrPreview"), "target.txt:12:3") {
		t.Fatalf("lastFailureDiagnostics = %+v, want stderr preview preserved", topLevel)
	}
	pkg := mapFromValue(out["repairActionPackage"])
	pkgDiagnostics := mapFromValue(pkg["lastFailureDiagnostics"])
	if !strings.Contains(stringFromMap(pkgDiagnostics, "stderrPreview"), "target.txt:12:3") {
		t.Fatalf("repairActionPackage.lastFailureDiagnostics = %+v, want stderr preview preserved", pkgDiagnostics)
	}
	if out["repairObjective"] != repairObjectiveStateChangingOutput || out["stateChangingOutputRequired"] != true {
		t.Fatalf("follow-up contract = %+v, want state-changing repair", out)
	}
}

func TestEvidenceAcquisitionFollowUpRestoresCurrentFailureFromRepairOfScope(t *testing.T) {
	runID := common.RunID("run-evidence-follow-up-repair-of-scope")
	source := &runModel.Step{
		ID:    common.StepID(runID + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateCompleted,
		Input: map[string]any{
			"concreteFailureEvidenceRequired": true,
			stepInputKeyRepairOfFailureClass:  failureClassMCPRuntime,
			stepInputKeyRepairOfFailureSig:    "class=mcp_runtime_failed|kind=mcp/tool_call|stage=mcp_tool_call_failed|node=lookup-symbol|serverId=semantic-capability|tool=references|targetRef=target.txt|operation=mcp/tool_call|failureTargets=target.txt|error=no identifier found",
			"currentProgressDigest": map[string]any{
				"currentGap":         "none_observed_in_recent_process_evidence",
				"nextRequiredAction": "continue_current_stage_contract",
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteFailureEvidence": true,
					"acceptedKinds":                      []string{"file/read", "mcp/tool_call"},
				},
			},
		},
	}
	input := map[string]any{
		"workspaceId": string(common.WorkspaceID("ws-evidence-follow-up-repair-of-scope")),
	}
	evidence := []map[string]any{{
		"kind":   "file/read",
		"stepId": string(runID) + "/graph-001/001-read-target",
		"output": map[string]any{
			"stage":       "file_read",
			"status":      "completed",
			"path":        "target.txt",
			"exists":      true,
			"content":     "package demo\n",
			"contentHash": "hash-current",
			"size":        13,
			"truncated":   false,
		},
	}}

	out := restoreRepairContractAfterConcreteEvidenceAcquisition(input, source, evidence)
	pkg := mapFromValue(out["repairActionPackage"])
	currentFailure := mapFromValue(pkg["currentFailure"])

	if currentFailure["failureClass"] != failureClassMCPRuntime ||
		currentFailure["failureSignature"] != source.Input[stepInputKeyRepairOfFailureSig] {
		t.Fatalf("currentFailure = %+v, want repairOf failure scope restored", currentFailure)
	}
	if currentFailure["targetScope"] != "serverId=semantic-capability|tool=references|node=lookup-symbol" {
		t.Fatalf("currentFailure = %+v, want capability failure scope from repairOf signature", currentFailure)
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
