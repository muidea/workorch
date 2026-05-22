package orchestrator

import (
	"context"
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestBuildConvergenceContractNeedStateChangeWithTargetEvidence(t *testing.T) {
	step := &runmodel.Step{
		ID: "run-convergence/repair-001",
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target.txt",
					"exists":           true,
					"contentHash":      "hash-target",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
		},
	}

	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": step.Input}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedStateChange)
	}
	if !contract.StateChangingOutputRequired {
		t.Fatalf("stateChangingOutputRequired = false, want true")
	}
	if len(contract.TargetEvidence) == 0 {
		t.Fatalf("target evidence missing")
	}
}

func TestApplyConvergenceContractNeedEvidenceRemovesStateChangingRequirement(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput":              evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
			"evidenceAcquisitionRequired": true,
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": input}, nil)
	output := ApplyConvergenceContract(input, contract)

	if contract.State != convergenceStateNeedEvidence {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedEvidence)
	}
	if boolValue(output["stateChangingOutputRequired"]) {
		t.Fatalf("stateChangingOutputRequired = true, want removed for evidence acquisition")
	}
	if !boolValue(output["concreteFailureEvidenceRequired"]) {
		t.Fatalf("concreteFailureEvidenceRequired = false, want true")
	}
	required := mapFromValue(mapFromValue(output["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustProduceConcreteFailureEvidence"]) {
		t.Fatalf("requiredOutput = %#v, want concrete evidence acquisition", required)
	}
}

func TestBuildConvergenceContractBlockedByDefault(t *testing.T) {
	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": map[string]any{}}, nil)
	if contract.State != convergenceStateBlocked {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateBlocked)
	}
}

func TestShouldCreateDecisionTicketBlocksMechanicalStateChangingDecision(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		stepInputKeyRuntimeStageMode:  "strict_convergence",
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"targetSnapshots": []map[string]any{{
				"targetRef":        "target.txt",
				"exists":           true,
				"contentHash":      "hash-target",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
		},
	}
	step := &runmodel.Step{ID: "run-convergence/repair-001", Input: input}
	spec := runtimeDecisionBlockSpec{
		Reason:       "need target repair choice",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Should the repair update this target?",
	}
	output := decisionOutputFromSpec(spec)
	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": input}, map[string]any{"llmOutput": output})

	if ShouldCreateDecisionTicket(input, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = true, want false for mechanically repairable state-changing contract")
	}
}

func TestShouldCreateDecisionTicketKeepsExternalGovernanceDecision(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"targetSnapshots": []map[string]any{{
				"targetRef":   "target.txt",
				"exists":      true,
				"contentHash": "hash-target",
			}},
		},
	}
	step := &runmodel.Step{ID: "run-convergence/repair-001", Input: input}
	spec := runtimeDecisionBlockSpec{
		Reason:       "provider config required",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Please configure the provider.",
		Context: map[string]any{
			decisionContextKeyScope:      decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency: decisionContextDependencyLLMProvider,
		},
	}
	output := decisionOutputFromSpec(spec)
	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": input}, map[string]any{"llmOutput": output})

	if !ShouldCreateDecisionTicket(input, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = false, want true for governance dependency")
	}
}

func TestBuildConvergenceContractNeedGovernanceForRuntimeDependencyDecision(t *testing.T) {
	step := &runmodel.Step{
		ID: "run-governance/repair-001",
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			stepOutputKeyBlockDecisionContext: map[string]any{
				decisionContextKeyScope:      decisionContextScopeWorkorchRuntime,
				decisionContextKeyDependency: decisionContextDependencyContextGate,
				decisionContextKeyReason:     "runtime context gate requires missing protected snapshot",
			},
		},
	}
	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": step.Input}, nil)
	if contract.State != convergenceStateNeedGovernance {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedGovernance)
	}
	if contract.RuntimeStage != runtimeStageDecision {
		t.Fatalf("runtimeStage = %q, want %q", contract.RuntimeStage, runtimeStageDecision)
	}
}

func TestBuildConvergenceContractUsesLatestActionEvidenceAsConcreteTarget(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"currentExecutionProgress": map[string]any{
			"failedStepId":     "run-convergence/follow-up-verify",
			"failureClass":     failureClassRuntime,
			"failureReason":    "validation follow-up llm step returned read-only executionGraph without patch output",
			"failureStage":     "llm_execution_graph_plan",
			"coverageGapCount": 0,
			"latestActionEvidence": []map[string]any{{
				"stepId": "run-convergence/graph-003/002-verify",
				"kind":   "shell/command",
				"state":  string(common.StepStateCompleted),
				"outputSummary": map[string]any{
					"status":        "completed",
					"command":       "sh",
					"exitCode":      0,
					"stdoutPreview": "# target [target.test]\n./main.go:5:2: \"bytes\" imported and not used\nFAIL",
				},
			}},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"currentExecutionProgress": map[string]any{
				"latestActionEvidence": []map[string]any{{
					"kind": "shell/command",
					"outputSummary": map[string]any{
						"stdoutPreview": "./main.go:5:2: \"bytes\" imported and not used",
					},
				}},
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-convergence/follow-up-verify",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: input,
		Output: map[string]any{
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan",
			"error":                   "validation follow-up llm step returned read-only executionGraph without patch output",
		},
	}

	contract := BuildConvergenceContract(nil, nil, failed, map[string]any{"input": input}, nil)
	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedStateChange)
	}
	if len(contract.TargetEvidence) == 0 {
		t.Fatalf("targetEvidence missing from contract: %+v", contract)
	}
	signals := mapFromValue(contract.TargetEvidence["structuredFailureSignals"])
	if !stringSliceContains(stringSliceFromValue(signals["targetRefs"]), "main.go") {
		t.Fatalf("targetEvidence.structuredFailureSignals = %+v, want latest action target main.go", signals)
	}

	applied := ApplyConvergenceContract(map[string]any{}, contract)
	required := mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want patch/apply state-changing contract", required)
	}
}

func TestDecisionResolutionInputAppliesConvergenceContract(t *testing.T) {
	ticket := &decisionmodel.Ticket{
		ID:           "ticket-resolved-continue",
		StepID:       "run-resolved-continue/repair-001",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Should we continue the target repair?",
		Reason:       "need target repair choice",
		Context: map[string]any{
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target.txt",
					"exists":           true,
					"contentHash":      "hash-target",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
			"stateChangingOutputRequired": true,
		},
	}
	resolution := &decisionmodel.Resolution{
		ID:   "resolution-resolved-continue",
		Type: decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload: map[string]any{
			"answer":         "continue",
			"recoveryAction": "continue_same_target_repair",
			"decisionReason": "continue",
		},
	}

	input := decisionResolutionInput(ticket, resolution)

	if got := stringFromMap(input, stepInputKeyRuntimeStage); got != runtimeStageRepair {
		t.Fatalf("runtimeStage = %q, want %q", got, runtimeStageRepair)
	}
	if !boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("stateChangingOutputRequired = false, want true")
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
}

func TestInitialStepInputCarriesReplanConvergenceContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-replan-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-predecessor",
				"repairActionPackage": map[string]any{
					"requiredOutput": stateChangingRepairRequiredOutput(),
					"targetSnapshots": []map[string]any{{
						"targetRef":        "target.txt",
						"exists":           true,
						"contentHash":      "hash-target",
						"contentComplete":  true,
						"safeForFileWrite": true,
					}},
				},
				"stateChangingOutputRequired": true,
				"repairObjective":             repairObjectiveStateChangingOutput,
			},
		},
	}

	input := initialStepInput(run)

	if got := stringFromMap(input, stepInputKeyRuntimeStage); got != runtimeStageRepair {
		t.Fatalf("runtimeStage = %q, want %q", got, runtimeStageRepair)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
}

func TestBuildConvergenceContractNeedValidationWhenPatchAlreadySatisfied(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"currentTargetAlreadySatisfiesFailedPatch": []map[string]any{{
				"targetRef": "target.txt",
				"state":     "satisfied",
			}},
			"targetSnapshots": []map[string]any{{
				"targetRef":   "target.txt",
				"exists":      true,
				"contentHash": "hash-target",
			}},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": input}, nil)

	if contract.State != convergenceStateNeedValidation {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedValidation)
	}
}

func TestRuntimeRepairFollowUpSchemaInvalidKeepsStateChangingContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-schema-invalid",
		WorkspaceID:     "ws-schema-invalid",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-schema-invalid/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			stepInputKeySoulStage:         soulStageExecution,
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
				"targetSnapshots": []map[string]any{{
					"targetRef":   "target.txt",
					"exists":      true,
					"contentHash": "hash-target",
				}},
			},
		},
		Output: map[string]any{
			"stage":                   "llm_schema_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|stage=llm_schema_invalid",
			"error":                   "invalid llm decision json",
		},
	}
	orch := &RuntimeOrchestrator{}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), run, failed, 1, nil)
	contract := mapFromValue(input["convergenceContract"])

	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change for schema invalid repair", contract)
	}
}

func TestRuntimeRepairFollowUpProviderNormalizedInvalidKeepsStateChangingContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-provider-normalized-invalid",
		WorkspaceID:     "ws-provider-normalized-invalid",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-provider-normalized-invalid/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
				"targetSnapshots": []map[string]any{{
					"targetRef":   "target.txt",
					"exists":      true,
					"contentHash": "hash-target",
				}},
			},
		},
		Output: map[string]any{
			"stage":                   "llm_decision_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|stage=llm_decision_invalid",
			"error":                   "provider returned normalized output that does not satisfy the decision contract",
		},
	}
	orch := &RuntimeOrchestrator{}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), run, failed, 1, nil)
	contract := mapFromValue(input["convergenceContract"])

	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change for provider-normalized invalid output", contract)
	}
	if input["stateChangingOutputRequired"] != true || input["repairObjective"] != repairObjectiveStateChangingOutput {
		t.Fatalf("repair input = %+v, want provider-normalized invalid to stay on state-changing repair contract", input)
	}
}

func TestRuntimeRepairFollowUpToolFailureWithConcreteTargetNeedsStateChange(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-tool-failure-contract",
		WorkspaceID:     "ws-tool-failure-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-tool-failure-contract/graph-001/002-call-tool",
		RunID: run.ID,
		Kind:  "mcp/tool_call",
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageExecution,
			stepInputKeyGraphNodeID:     "call_tool",
			stepInputKeyGraphRootStepID: string(run.ID) + "/step-1",
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "target.txt:7:1: target contract mismatch",
			failureOutputKeyClass:     failureClassMCPRuntime,
			failureOutputKeySignature: "class=mcp_runtime_failed|kind=mcp/tool_call|failureTargets=target.txt|node=call_tool",
		},
	}
	orch := &RuntimeOrchestrator{steps: memory.NewStepRepository()}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), run, failed, 1, nil)
	contract := mapFromValue(input["convergenceContract"])

	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change for concrete target tool failure", contract)
	}
	if input["stateChangingOutputRequired"] != true || input["repairObjective"] != repairObjectiveStateChangingOutput {
		t.Fatalf("repair input = %+v, want concrete target tool failure to require state-changing repair", input)
	}
}

func TestAssessmentOnlyContractCorrectionStepInputAppliesConvergenceContract(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-assessment-correction/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyAssessmentOnly: true,
			"repairObjective":          "complete_dod_goal_coverage_assessment",
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req",
				"raw":               "coverage gap",
				"state":             "failed",
			}},
		},
	}

	input := assessmentOnlyContractCorrectionStepInput(step, map[string]any{
		"stage": "llm_dod_assessment",
		"error": "missing goal coverage",
	}, nil)

	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedAssessment {
		t.Fatalf("convergenceContract = %+v, want need_assessment for assessment-only correction", contract)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageVerification || input[stepInputKeyAssessmentOnly] != true {
		t.Fatalf("input = %+v, want verification assessment-only correction input", input)
	}
}

func TestBuildConvergenceContractReadsRejectedResultContinuationFromExecutionContext(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-rejected-result-continuation",
		WorkspaceID:     "ws-rejected-result-continuation",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"rejectedResultContinuation": map[string]any{
				"predecessorRunId": "run-previous",
				"summary":          "coverage gaps remain",
				"requiredEvidence": []string{"main.go"},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-rejected-result-continuation/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
		},
	}

	contract := BuildConvergenceContract(run, step, step, map[string]any{"input": map[string]any{"executionContext": run.ExecutionContext}}, nil)
	if contract.State != convergenceStateNeedEvidence {
		t.Fatalf("convergenceContract = %+v, want need_evidence from rejected result continuation", contract)
	}
}
