package orchestrator

import (
	"context"
	"strings"
	"testing"

	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
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
	orch.applyLLMOutputModeSelection(context.Background(), run, input, nil, 0)

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
	if input["artifactOutputContract"] != nil {
		t.Fatalf("artifactOutputContract = %+v, want nil for small graph", input["artifactOutputContract"])
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

	orch.applyLLMOutputModeSelection(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-output-mode-large-schema"}, input, failed, 2)

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

	orch.applyLLMOutputModeSelection(context.Background(), &runmodel.Run{ID: "run-output-mode-required-output", WorkspaceID: "ws-output-mode-required-output"}, input, nil, 0)

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

	orch.applyLLMOutputModeSelection(context.Background(), &runmodel.Run{ID: "run-output-mode-requested", WorkspaceID: "ws-output-mode-requested"}, input, nil, 0)

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

	orch.applyLLMOutputModeSelection(context.Background(), run, input, nil, 0)

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

	orch.applyLLMOutputModeSelection(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-output-mode-open-continuation"}, input, nil, 0)

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

func TestOutputModeSelectorReturnsTerminalArtifactContinuationToExecutionGraph(t *testing.T) {
	ctx := context.Background()
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"artifactContinuationContext": map[string]any{
			"id":         "artifact-continuation-applied-001",
			"state":      string(common.ArtifactContinuationStateApplied),
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/target.txt",
		},
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}

	orch.applyLLMOutputModeSelection(ctx, &runmodel.Run{ID: "run-output-mode-applied", WorkspaceID: "ws-output-mode-applied"}, input, nil, 0)

	if outputModeFromInput(input) != outputModeExecutionGraph {
		t.Fatalf("outputMode = %v, want executionGraph", input["outputMode"])
	}
	if _, ok := input["artifactContinuationContext"]; ok {
		t.Fatalf("artifactContinuationContext still present for terminal continuation: %+v", input["artifactContinuationContext"])
	}
	selector := mapFromValue(input["outputModeSelector"])
	if selector["reason"] != "terminal_artifact_continuation_returns_to_graph" {
		t.Fatalf("outputModeSelector = %+v, want terminal continuation reason", selector)
	}
}

func TestOutputModeSelectorDoesNotInjectIncrementalArtifactWithoutStateChange(t *testing.T) {
	input := map[string]any{
		"requestedOutputMode": outputModeIncrementalArtifact,
		"repairObjective":     repairObjectiveConcreteValidationEvidence,
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}

	orch.applyLLMOutputModeSelection(context.Background(), &runmodel.Run{ID: "run-output-mode-p0", WorkspaceID: "ws-output-mode-p0"}, input, nil, 0)

	if input["outputMode"] != nil || input["outputModeSelector"] != nil || input["artifactOutputContract"] != nil {
		t.Fatalf("input = %+v, want no output mode injection without state-changing contract", input)
	}
}
