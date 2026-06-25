package orchestrator

import (
	"context"
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOutputModeSelectorKeepsSmallStateChangingRepairOnExecutionGraph(t *testing.T) {
	run := &runmodel.Run{ID: "run-output-mode-small", WorkspaceID: "ws-output-mode-small"}
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
		},
	}

	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}
	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, run, input, nil, 0)

	if outputModeFromInput(input) != outputModeExecutionGraph {
		t.Fatalf("outputMode = %v, want executionGraph", input["outputMode"])
	}
	selector := mapFromValue(input["outputModeSelector"])
	if selector["priority"] != "P6" {
		t.Fatalf("outputModeSelector = %+v, want P6", selector)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["outputMode"] != outputModeExecutionGraph || required["mustIncludeStateChangingNode"] != true {
		t.Fatalf("requiredOutput = %+v, want executionGraph state-changing contract", required)
	}
	granularity := mapFromValue(required["outputGranularityPolicy"])
	if !boolValue(granularity["mustAvoidMonolithicGeneratedFiles"]) ||
		granularity["maxPatchContentBytesRef"] != "input.llmOutputPolicy.maxPatchContentBytes" {
		t.Fatalf("outputGranularityPolicy = %+v, want bounded cohesive output contract", granularity)
	}
	if input["artifactOutputContract"] != nil {
		t.Fatalf("artifactOutputContract = %+v, want nil for small graph", input["artifactOutputContract"])
	}
}

func TestOutputModeSelectorDoesNotReinheritStateChangeWhenCurrentContractRequiresEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-output-mode-evidence-lock",
		WorkspaceID: "ws-output-mode-evidence-lock",
		ExecutionContext: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
			},
		},
	}
	input := map[string]any{
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		"repairActionPackage": map[string]any{
			"evidenceAcquisitionRequired": true,
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode":       false,
				"mustProduceConcreteFailureEvidence": true,
				"stateChangingForbidden":             true,
			},
		},
	}

	applyLLMOutputModeSelection(context.Background(), NormalizeRuntimePolicy(RuntimePolicy{}).LLMOutput, nil, run, input, nil, 1)

	if input["stateChangingOutputRequired"] != nil {
		t.Fatalf("stateChangingOutputRequired = %v, want no inherited state-changing requirement", input["stateChangingOutputRequired"])
	}
	if input["repairObjective"] != repairObjectiveAcquireFailureEvidence {
		t.Fatalf("repairObjective = %v, want %s", input["repairObjective"], repairObjectiveAcquireFailureEvidence)
	}
	if input["outputMode"] != nil || input["outputModeSelector"] != nil {
		t.Fatalf("input = %+v, want no output mode selection for evidence acquisition", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustProduceConcreteFailureEvidence"] != true || required["mustIncludeStateChangingNode"] != false {
		t.Fatalf("requiredOutput = %+v, want evidence acquisition contract preserved", required)
	}
}

func TestOutputModeSelectorUsesIncrementalArtifactForLargeSchemaInvalidPreview(t *testing.T) {
	runID := common.RunID("run-output-mode-large-schema")
	failed := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
		},
		Output: map[string]any{
			"stage":             "llm_schema_invalid",
			"rawContentPreview": strings.Repeat(`{"content":"x"}`, 900),
		},
	}
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
		},
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{
		LLMOutput: LLMOutputRuntimePolicy{SchemaInvalidLargePreviewThreshold: 1024},
	})}

	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, &runmodel.Run{ID: runID, WorkspaceID: "ws-output-mode-large-schema"}, input, failed, 2)

	if outputModeFromInput(input) != outputModeIncrementalArtifact {
		t.Fatalf("outputMode = %v, want incrementalArtifact", input["outputMode"])
	}
	selector := mapFromValue(input["outputModeSelector"])
	if selector["priority"] != "P3" {
		t.Fatalf("outputModeSelector = %+v, want P3", selector)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustReturnArtifactPlan"] != true ||
		required["mustIncludeStateChangingNode"] != nil ||
		required["outputMode"] != outputModeIncrementalArtifact {
		t.Fatalf("requiredOutput = %+v, want artifact plan contract without graph node requirement", required)
	}
	contract := mapFromValue(input["artifactOutputContract"])
	if contract["phase"] != "artifact_plan" || contract["stateChangingEvidenceStage"] != "artifact_apply" {
		t.Fatalf("artifactOutputContract = %+v, want artifact plan apply contract", contract)
	}
}

func TestOutputModeSelectorUsesIncrementalArtifactForRequiredOutputBudgetSignal(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode":       true,
				"estimatedPatchContentBytes":         4096,
				"expectedStateChangingArtifactCount": 2,
			},
		},
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{
		LLMOutput: LLMOutputRuntimePolicy{
			MaxPatchContentBytes: 512,
			MaxGraphFileWrites:   1,
		},
	})}

	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, &runmodel.Run{ID: "run-output-mode-required-output", WorkspaceID: "ws-output-mode-required-output"}, input, nil, 0)

	if outputModeFromInput(input) != outputModeIncrementalArtifact {
		t.Fatalf("outputMode = %v, want incrementalArtifact", input["outputMode"])
	}
	selector := mapFromValue(input["outputModeSelector"])
	if selector["priority"] != "P4" || selector["reason"] != "estimated_graph_output_exceeds_budget" {
		t.Fatalf("outputModeSelector = %+v, want P4 budget signal", selector)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustReturnArtifactPlan"] != true || required["stateChangingEvidenceDeferredUntilApply"] != true {
		t.Fatalf("requiredOutput = %+v, want artifact plan requirements", required)
	}
}

func TestOutputModeSelectorRetainsIncrementalArtifactAfterContractFailure(t *testing.T) {
	runID := common.RunID("run-output-mode-artifact-contract-retry")
	failed := &runmodel.Step{
		ID:    common.StepID(runID + "/artifact-plan-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			"outputMode": outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": artifactOutputPhasePlan,
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"outputMode":             outputModeIncrementalArtifact,
					"mustReturnArtifactPlan": true,
				},
			},
		},
		Output: map[string]any{
			"stage":               "llm_schema_invalid",
			failureOutputKeyClass: failureClassLLMDecisionInvalid,
			"error":               "invalid argument: incrementalArtifact output must not include executionGraph",
		},
	}
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
		},
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}

	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, &runmodel.Run{ID: runID, WorkspaceID: "ws-output-mode-artifact-contract-retry"}, input, failed, 1)

	if outputModeFromInput(input) != outputModeIncrementalArtifact {
		t.Fatalf("outputMode = %v, want incrementalArtifact", input["outputMode"])
	}
	selector := mapFromValue(input["outputModeSelector"])
	if selector["priority"] != "P2" || selector["reason"] != "incremental_artifact_contract_retry" {
		t.Fatalf("outputModeSelector = %+v, want P2 incremental artifact contract retry", selector)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["outputMode"] != outputModeIncrementalArtifact ||
		required["mustReturnArtifactPlan"] != true ||
		required["mustIncludeStateChangingNode"] != nil {
		t.Fatalf("requiredOutput = %+v, want artifact plan contract without graph state-changing node", required)
	}
	contract := mapFromValue(input["artifactOutputContract"])
	if contract["phase"] != artifactOutputPhasePlan || contract["mustReturnArtifactPlan"] != true {
		t.Fatalf("artifactOutputContract = %+v, want artifact plan contract", contract)
	}
}

func TestOutputModeSelectorUsesIncrementalArtifactForRequestedModeWithInheritedBudgetSignal(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"requestedOutputMode":         outputModeIncrementalArtifact,
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode":       true,
				"estimatedPatchContentBytes":         defaultMaxPatchContentBytes + 1,
				"expectedStateChangingArtifactCount": 1,
			},
		},
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}

	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, &runmodel.Run{ID: "run-output-mode-requested", WorkspaceID: "ws-output-mode-requested"}, input, nil, 0)

	if outputModeFromInput(input) != outputModeIncrementalArtifact {
		t.Fatalf("outputMode = %v, want incrementalArtifact", input["outputMode"])
	}
	selector := mapFromValue(input["outputModeSelector"])
	if selector["priority"] != "P4" || selector["reason"] != "estimated_graph_output_exceeds_budget" {
		t.Fatalf("outputModeSelector = %+v, want P4 budget signal", selector)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if intFromAny(required["estimatedPatchContentBytes"]) <= defaultMaxPatchContentBytes ||
		required["mustReturnArtifactPlan"] != true ||
		required["outputMode"] != outputModeIncrementalArtifact {
		t.Fatalf("requiredOutput = %+v, want preserved budget signal and artifact plan contract", required)
	}
}

func TestOutputModeSelectorHonorsExplicitRequestedIncrementalArtifactWithinGraphBudget(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"requestedOutputMode":         outputModeIncrementalArtifact,
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode":       true,
				"estimatedPatchContentBytes":         20000,
				"expectedStateChangingArtifactCount": 1,
			},
		},
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}

	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, &runmodel.Run{ID: "run-output-mode-requested-small", WorkspaceID: "ws-output-mode-requested-small"}, input, nil, 0)

	if outputModeFromInput(input) != outputModeIncrementalArtifact {
		t.Fatalf("outputMode = %v, want incrementalArtifact", input["outputMode"])
	}
	selector := mapFromValue(input["outputModeSelector"])
	if selector["priority"] != "P5" || selector["reason"] != "requested_incremental_artifact_with_budget_signal" {
		t.Fatalf("outputModeSelector = %+v, want P5 explicit requested mode", selector)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["outputMode"] != outputModeIncrementalArtifact ||
		required["mustReturnArtifactPlan"] != true ||
		required["mustIncludeStateChangingNode"] != nil {
		t.Fatalf("requiredOutput = %+v, want explicit requested incremental artifact plan contract", required)
	}
}

func TestOutputModeSelectorInheritsStateChangingSignalFromRunExecutionContext(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-output-mode-state-changing-context",
		WorkspaceID: "ws-output-mode-state-changing-context",
		ExecutionContext: map[string]any{
			"requestedOutputMode":         outputModeIncrementalArtifact,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"estimatedPatchContentBytes":         defaultMaxPatchContentBytes + 1,
					"expectedStateChangingArtifactCount": 1,
				},
			},
		},
	}
	input := map[string]any{}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}

	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, run, input, nil, 0)

	if input["stateChangingOutputRequired"] != true ||
		input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["requestedOutputMode"] != outputModeIncrementalArtifact {
		t.Fatalf("input did not inherit run execution-context state-changing signals: %+v", input)
	}
	if outputModeFromInput(input) != outputModeIncrementalArtifact {
		t.Fatalf("outputMode = %v, want incrementalArtifact", input["outputMode"])
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustReturnArtifactPlan"] != true ||
		required["outputMode"] != outputModeIncrementalArtifact {
		t.Fatalf("requiredOutput = %+v, want artifact plan contract", required)
	}
	if inputRequiresConcreteValidationEvidence(input) {
		t.Fatalf("inputRequiresConcreteValidationEvidence = true, want incremental artifact state-changing contract to take precedence")
	}
	contract := mapFromValue(input["artifactOutputContract"])
	if contract["phase"] != artifactOutputPhasePlan {
		t.Fatalf("artifactOutputContract = %+v, want artifact_plan phase", contract)
	}
}

func TestOutputModeSelectorDoesNotInheritStateChangingSignalForCompletionFollowUp(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-output-mode-completion-follow-up",
		WorkspaceID: "ws-output-mode-completion-follow-up",
		ExecutionContext: map[string]any{
			"requestedOutputMode":         outputModeIncrementalArtifact,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"estimatedPatchContentBytes":         defaultMaxPatchContentBytes + 1,
					"expectedStateChangingArtifactCount": 1,
				},
			},
		},
	}
	input := map[string]any{
		"completionFollowUp":       true,
		stepInputKeyAssessmentOnly: true,
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}

	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, run, input, nil, 0)

	for _, key := range []string{"stateChangingOutputRequired", "repairObjective", "requestedOutputMode", "outputMode", "outputModeSelector", "artifactOutputContract"} {
		if input[key] != nil {
			t.Fatalf("%s = %v, want nil for assessment-only completion follow-up; input=%+v", key, input[key], input)
		}
	}
	if input["completionFollowUp"] != true || input[stepInputKeyAssessmentOnly] != true {
		t.Fatalf("completion follow-up markers were not preserved: %+v", input)
	}
}

func TestOutputModeSelectorRestoresBudgetSignalFromRunExecutionContext(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-output-mode-restored",
		WorkspaceID: "ws-output-mode-restored",
		ExecutionContext: map[string]any{
			"requestedOutputMode": outputModeIncrementalArtifact,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"estimatedPatchContentBytes": defaultMaxPatchContentBytes + 1,
				},
			},
		},
	}
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"requestedOutputMode":         outputModeIncrementalArtifact,
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
		},
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}

	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, run, input, nil, 0)

	if outputModeFromInput(input) != outputModeIncrementalArtifact {
		t.Fatalf("outputMode = %v, want incrementalArtifact", input["outputMode"])
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if intFromAny(required["estimatedPatchContentBytes"]) <= defaultMaxPatchContentBytes ||
		required["mustReturnArtifactPlan"] != true {
		t.Fatalf("requiredOutput = %+v, want restored budget signal and artifact plan contract", required)
	}
}

func TestOutputModeSelectorContinuesOpenArtifactContinuation(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-output-mode-open-continuation")
	continuations := memory.NewArtifactContinuationRepository()
	if err := continuations.Create(ctx, &orchestratormodel.ArtifactContinuation{
		ID:               common.ArtifactContinuationID("artifact-continuation-open-001"),
		WorkspaceID:      common.WorkspaceID("ws-output-mode-open-continuation"),
		RunID:            runID,
		RootStepID:       common.StepID(runID + "/step-1"),
		ArtifactID:       "artifact-001",
		TargetRef:        "deliverable/target.txt",
		State:            common.ArtifactContinuationStateChunking,
		ExpectedSequence: 2,
		CoverageTargets:  []string{"coverage-001"},
	}); err != nil {
		t.Fatalf("seed artifact continuation: %v", err)
	}
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{ArtifactContinuations: continuations},
		policy:   NormalizeRuntimePolicy(RuntimePolicy{}),
	}

	applyLLMOutputModeSelection(ctx, orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, &runmodel.Run{ID: runID, WorkspaceID: "ws-output-mode-open-continuation"}, input, nil, 0)

	if outputModeFromInput(input) != outputModeIncrementalArtifact {
		t.Fatalf("outputMode = %v, want incrementalArtifact", input["outputMode"])
	}
	selector := mapFromValue(input["outputModeSelector"])
	if selector["priority"] != "P1" {
		t.Fatalf("outputModeSelector = %+v, want P1", selector)
	}
	continuation := mapFromValue(input["artifactContinuationContext"])
	if continuation["id"] != "artifact-continuation-open-001" || continuation["expectedSequence"] != 2 {
		t.Fatalf("artifactContinuationContext = %+v, want open continuation summary", continuation)
	}
	contract := mapFromValue(input["artifactOutputContract"])
	if contract["phase"] != artifactOutputPhaseChunk || contract["expectedSequence"] != 2 {
		t.Fatalf("artifactOutputContract = %+v, want chunk phase continuation contract", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustReturnArtifactChunk"] != true || required["mustReturnArtifactPlan"] != nil {
		t.Fatalf("requiredOutput = %+v, want artifact chunk contract", required)
	}
}

func TestOutputModeSelectorUsesArtifactContextPhaseWhenContinuationRequestsContext(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-output-mode-context-continuation")
	continuations := memory.NewArtifactContinuationRepository()
	if err := continuations.Create(ctx, &orchestratormodel.ArtifactContinuation{
		ID:               common.ArtifactContinuationID("artifact-continuation-context-001"),
		WorkspaceID:      common.WorkspaceID("ws-output-mode-context-continuation"),
		RunID:            runID,
		RootStepID:       common.StepID(runID + "/step-1"),
		ArtifactID:       "artifact-001",
		TargetRef:        "deliverable/target.txt",
		State:            common.ArtifactContinuationStateChunking,
		ExpectedSequence: 2,
		CoverageTargets:  []string{"coverage-001"},
	}); err != nil {
		t.Fatalf("seed artifact continuation: %v", err)
	}
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"requestedArtifactContext":    true,
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{ArtifactContinuations: continuations},
		policy:   NormalizeRuntimePolicy(RuntimePolicy{}),
	}

	applyLLMOutputModeSelection(ctx, orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, &runmodel.Run{ID: runID, WorkspaceID: "ws-output-mode-context-continuation"}, input, nil, 0)

	if outputModeFromInput(input) != outputModeIncrementalArtifact {
		t.Fatalf("outputMode = %v, want incrementalArtifact", input["outputMode"])
	}
	selector := mapFromValue(input["outputModeSelector"])
	if selector["reason"] != "open_artifact_continuation_context_required" {
		t.Fatalf("outputModeSelector = %+v, want artifact context reason", selector)
	}
	contract := mapFromValue(input["artifactOutputContract"])
	if contract["phase"] != artifactOutputPhaseContext || contract["stateChangingForbidden"] != true {
		t.Fatalf("artifactOutputContract = %+v, want artifact_context read-only contract", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustReturnArtifactContextGraph"] != true || required["mustReturnArtifactChunk"] != nil {
		t.Fatalf("requiredOutput = %+v, want artifact_context contract only", required)
	}
	if !stringSliceContains(stringSliceFromValue(required["allowedArtifactContextKinds"]), "file/read") ||
		!stringSliceContains(stringSliceFromValue(required["allowedArtifactContextKinds"]), "mcp/tool_call") {
		t.Fatalf("allowedArtifactContextKinds = %+v, want neutral read/tool allowlist", required["allowedArtifactContextKinds"])
	}
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "file/read") ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "mcp/tool_call") ||
		!boolValue(required["acceptedKindsOnly"]) ||
		!boolValue(required["stateChangingForbidden"]) {
		t.Fatalf("requiredOutput = %+v, want artifact context acceptedKinds to match read-only allowlist", required)
	}
}

func TestOutputModeSelectorUsesArtifactContextPhaseAfterChunkReturnedToolGraph(t *testing.T) {
	runID := common.RunID("run-output-mode-context-after-invalid-chunk")
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"artifactContinuationContext": map[string]any{
			"id":               "artifact-continuation-context-002",
			"state":            string(common.ArtifactContinuationStateChunking),
			"artifactId":       "artifact-001",
			"targetRef":        "deliverable/target.txt",
			"expectedSequence": 2,
		},
	}
	failed := &runmodel.Step{
		ID:    common.StepID(runID + "/artifact-continue-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"outputMode": outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": artifactOutputPhaseChunk,
			},
		},
		Output: map[string]any{
			stepOutputKeyExecutionGraph: map[string]any{
				"nodes": []map[string]any{{"id": "read-current", "kind": "file/read"}},
			},
		},
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}

	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, &runmodel.Run{ID: runID, WorkspaceID: "ws-output-mode-context-after-invalid-chunk"}, input, failed, 1)

	contract := mapFromValue(input["artifactOutputContract"])
	if contract["phase"] != artifactOutputPhaseContext {
		t.Fatalf("artifactOutputContract = %+v, want artifact_context phase", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustReturnArtifactContextGraph"] != true {
		t.Fatalf("requiredOutput = %+v, want artifact_context graph requirement", required)
	}
}

func TestOutputModeSelectorReturnsTerminalArtifactContinuationToExecutionGraph(t *testing.T) {
	ctx := context.Background()
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"artifactContinuationId":      "artifact-continuation-applied-001",
		"artifactContinuationContext": map[string]any{
			"id":         "artifact-continuation-applied-001",
			"state":      string(common.ArtifactContinuationStateApplied),
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/target.txt",
		},
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}

	applyLLMOutputModeSelection(ctx, orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, &runmodel.Run{ID: "run-output-mode-applied", WorkspaceID: "ws-output-mode-applied"}, input, nil, 0)

	if outputModeFromInput(input) != outputModeExecutionGraph {
		t.Fatalf("outputMode = %v, want executionGraph", input["outputMode"])
	}
	if _, ok := input["artifactContinuationContext"]; ok {
		t.Fatalf("artifactContinuationContext still present for terminal continuation: %+v", input["artifactContinuationContext"])
	}
	if _, ok := input["artifactContinuationId"]; ok {
		t.Fatalf("artifactContinuationId still present for terminal continuation: %+v", input["artifactContinuationId"])
	}
	selector := mapFromValue(input["outputModeSelector"])
	if selector["reason"] != "terminal_artifact_continuation_returns_to_graph" {
		t.Fatalf("outputModeSelector = %+v, want terminal continuation reason", selector)
	}
}

func TestOutputModeSelectorClearsIncrementalArtifactContractWhenReturningToExecutionGraph(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"outputMode":                  outputModeIncrementalArtifact,
		"artifactOutputContract": map[string]any{
			"mode":  outputModeIncrementalArtifact,
			"phase": artifactOutputPhasePlan,
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"outputMode":                              outputModeIncrementalArtifact,
				"mustReturnArtifactPlan":                  true,
				"stateChangingEvidenceDeferredUntilApply": true,
				"forbiddenTopLevelOutputs": []string{
					"executionGraph",
					"dodAssessment",
				},
				"artifactPlanSchema": map[string]any{
					"requiredFields": []string{"artifactPlan.artifacts[].artifactId"},
				},
			},
		},
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}

	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, &runmodel.Run{ID: "run-output-mode-cleanup", WorkspaceID: "ws-output-mode-cleanup"}, input, nil, 0)

	if outputModeFromInput(input) != outputModeExecutionGraph {
		t.Fatalf("outputMode = %v, want executionGraph", input["outputMode"])
	}
	if input["artifactOutputContract"] != nil {
		t.Fatalf("artifactOutputContract = %+v, want nil after selector falls back to executionGraph", input["artifactOutputContract"])
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustReturnArtifactPlan"] != nil || required["artifactPlanSchema"] != nil || required["forbiddenTopLevelOutputs"] != nil {
		t.Fatalf("requiredOutput = %+v, want incrementalArtifact-only fields cleared", required)
	}
	if required["outputMode"] != outputModeExecutionGraph || required["mustIncludeStateChangingNode"] != true {
		t.Fatalf("requiredOutput = %+v, want executionGraph state-changing contract restored", required)
	}
}

func TestOutputModeSelectorDoesNotInjectIncrementalArtifactWithoutStateChange(t *testing.T) {
	input := map[string]any{
		"requestedOutputMode": outputModeIncrementalArtifact,
		"repairObjective":     repairObjectiveConcreteValidationEvidence,
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}

	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, &runmodel.Run{ID: "run-output-mode-p0", WorkspaceID: "ws-output-mode-p0"}, input, nil, 0)

	if input["outputMode"] != nil || input["outputModeSelector"] != nil || input["artifactOutputContract"] != nil {
		t.Fatalf("input = %+v, want no output mode injection without state-changing contract", input)
	}
}
