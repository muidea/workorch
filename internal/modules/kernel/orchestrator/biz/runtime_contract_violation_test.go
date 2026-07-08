package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestContractViolationRecoveryActionRetriesWithinBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-retry")
	repo := memory.NewStepRepository()
	failed := contractViolationFailedStep(runID, "failed-001")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-retry", InitialStepKind: llmWorkspaceExecutionStepKind}
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{}), true, true, run, failed)

	if action != runtimeRecoveryActionContractViolationRetry {
		t.Fatalf("recovery action = %s, want contract violation retry", action)
	}
}

func TestContractViolationRecoveryActionSkipsRetryWhenCurrentActionIsGovernance(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-governance-action")
	repo := memory.NewStepRepository()
	failed := contractViolationFailedStep(runID, "failed-governance-action")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-contract-violation-governance-action",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"workspaceId":   "ws-contract-violation-governance-action",
				"runId":         string(runID),
				"nextAction":    map[string]any{"kind": "request_decision"},
			},
		},
	}
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{}), true, true, run, failed)

	if action != runtimeRecoveryActionContractViolationExhausted {
		t.Fatalf("recovery action = %s, want governance escalation without contract retry", action)
	}
}

func TestContractViolationRecoveryActionSkipsLLMRetryForDeterministicValidationRoute(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-runtime-action-route")
	repo := memory.NewStepRepository()
	failed := contractViolationFailedStep(runID, "failed-runtime-action-route")
	pkg := mapFromValue(failed.Input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	required["validationRoute"] = runtimemodel.ValidationRouteRuntimeAction
	required["mustUseRuntimeAction"] = true
	pkg["requiredOutput"] = required
	failed.Input["repairActionPackage"] = pkg
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-runtime-action-route", InitialStepKind: llmWorkspaceExecutionStepKind}
	attachContractViolationDiagnostics(run, failed, errors.New("concrete validation evidence repair returned invalid LLM graph for runtime action route"))

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{}), true, true, run, failed)

	if action == runtimeRecoveryActionContractViolationRetry {
		t.Fatalf("recovery action = %s, want no LLM retry for deterministic runtime_action validation route", action)
	}
}

func TestContractViolationRecoveryActionExhaustsIndependentBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-exhausted")
	repo := memory.NewStepRepository()
	failed := contractViolationFailedStep(runID, "failed-001")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-exhausted", InitialStepKind: llmWorkspaceExecutionStepKind}
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	for attempt := 1; attempt <= 2; attempt++ {
		step := &runmodel.Step{
			ID:    contractViolationRetryStepID(runID, failed, attempt),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyContractViolationRecovery: map[string]any{
					"sourceStepId": string(failed.ID),
				},
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassRuntime,
				failureOutputKeySignature: "class=runtime_failed|stage=runtime_contract_failed|rule=state_changing_required_missing",
			},
		}
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed retry step %d: %v", attempt, err)
		}
	}

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{MaxContractViolationRetriesPerStep: 2}), true, true, run, failed)

	if action != runtimeRecoveryActionContractViolationExhausted {
		t.Fatalf("recovery action = %s, want contract violation exhausted", action)
	}
}

func TestContractViolationExhaustedContinuableReadableResourceDecisionRepairs(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-readable-resource-repair")
	repo := memory.NewStepRepository()
	failed := contractViolationFailedStep(runID, "failed-readable-resource")
	failed.Output["error"] = "invalid argument: decision requests a continuable candidate choice"
	failed.Output["continuableDecisionConvertedToRepair"] = true
	failed.Output["convertedDecisionGate"] = "convergence_contract"
	failed.Output["convertedDecision"] = map[string]any{
		"question": "Please provide pkg/service/recorder.go current content.",
		"reason":   "pkg/service/recorder.go is needed before producing a precise state-changing repair.",
		"context": map[string]any{
			"missingFields": []string{"pkg/service/recorder.go content"},
			"readableResources": []string{
				"pkg/service/recorder.go",
			},
		},
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-readable-resource-repair", InitialStepKind: llmWorkspaceExecutionStepKind}
	attachContractViolationDiagnostics(run, failed, errors.New("invalid argument: decision requests a continuable candidate choice"))
	for attempt := 1; attempt <= 2; attempt++ {
		step := &runmodel.Step{
			ID:    contractViolationRetryStepID(runID, failed, attempt),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyContractViolationRecovery: map[string]any{
					"sourceStepId": string(failed.ID),
				},
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassDoDCoverageMissing,
				failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_decision_required",
			},
		}
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed contract retry step %d: %v", attempt, err)
		}
	}

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{MaxContractViolationRetriesPerStep: 2}), true, true, run, failed)

	if action != runtimeRecoveryActionRuntimeRepair {
		t.Fatalf("recovery action = %s, want runtime repair for continuable readable resource decision", action)
	}
}

func TestContractViolationExhaustedStaticValidationContinuesRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-static-validation-repair")
	repo := memory.NewStepRepository()
	failed := contractViolationFailedStep(runID, "failed-static-validation")
	failed.Input[stepInputKeyRuntimeStage] = runtimeStageVerification
	failed.Input["repairObjective"] = repairObjectiveConcreteValidationEvidence
	failed.Input["stateChangingOutputRequired"] = false
	failed.Output["stage"] = "llm_execution_graph_plan"
	failed.Output["error"] = "concrete validation evidence cannot use static inspection or source scanning alone to close runtime-behavior coverage"
	failed.Output[outputKeyContractViolationType] = contractViolationValidationEvidenceRequired
	failed.Output["contractViolationRule"] = contractViolationRuleValidationStaticOnly
	failed.Output[failureOutputKeyClass] = failureClassRuntime
	failed.Output[failureOutputKeySignature] = "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|rule=validation_static_only"
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-static-validation-repair", InitialStepKind: llmWorkspaceExecutionStepKind}
	attachContractViolationDiagnostics(run, failed, common.ErrInvalidArgument)
	for attempt := 1; attempt <= 2; attempt++ {
		step := &runmodel.Step{
			ID:    contractViolationRetryStepID(runID, failed, attempt),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyContractViolationRecovery: map[string]any{
					"sourceStepId": string(failed.ID),
				},
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassRuntime,
				failureOutputKeySignature: stringFromMap(failed.Output, failureOutputKeySignature),
			},
		}
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed contract retry step %d: %v", attempt, err)
		}
	}

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{MaxContractViolationRetriesPerStep: 2}), true, true, run, failed)

	if action != runtimeRecoveryActionRuntimeRepair {
		t.Fatalf("recovery action = %s, want runtime repair for exhausted static validation contract violation", action)
	}
}

func TestCoverageAssessmentIncompleteContractViolationExhaustionContinuesRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-coverage-assessment-contract-exhausted")
	repo := memory.NewStepRepository()
	failed := coverageAssessmentIncompleteFailedStep(runID, "assessment-failed")
	failed.Output["contractViolationRule"] = contractViolationRuleCoverageAssessmentIncomplete
	failed.Output[stepOutputKeyContractViolation] = map[string]any{
		"violatedRule":   contractViolationRuleCoverageAssessmentIncomplete,
		"recoverability": runtimemodel.ContractViolationRecoverabilityRecomputeAuthority,
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-coverage-assessment-contract-exhausted", InitialStepKind: llmWorkspaceExecutionStepKind}
	attachContractViolationDiagnostics(run, failed, errors.New("invalid argument: dodAssessment is not satisfied"))
	for attempt := 1; attempt <= 2; attempt++ {
		step := &runmodel.Step{
			ID:    common.StepID(string(runID) + "/contract-retry-coverage-" + string(rune('0'+attempt))),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyContractViolationRecovery: map[string]any{
					"sourceStepId": string(failed.ID),
				},
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassDoDCoverageMissing,
				failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
			},
		}
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed coverage assessment retry step %d: %v", attempt, err)
		}
	}

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{MaxContractViolationRetriesPerStep: 2}), true, true, run, failed)

	if action != runtimeRecoveryActionRuntimeRepair {
		t.Fatalf("recovery action = %s, want deterministic runtime repair for repeated coverage assessment violation", action)
	}
}

func TestCoverageAssessmentIncompleteContractViolationRetriesWithinBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-coverage-assessment-contract-retry")
	repo := memory.NewStepRepository()
	failed := coverageAssessmentIncompleteFailedStep(runID, "assessment-failed")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-coverage-assessment-contract-retry", InitialStepKind: llmWorkspaceExecutionStepKind}
	attachContractViolationDiagnostics(run, failed, errors.New("invalid argument: dodAssessment is not satisfied"))

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{MaxContractViolationRetriesPerStep: 2}), true, true, run, failed)

	if action != runtimeRecoveryActionContractViolationRetry {
		t.Fatalf("recovery action = %s, want contract violation retry for first coverage assessment violation", action)
	}
}

func TestContractViolationRecoveryActionExhaustsRunBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-run-budget")
	repo := memory.NewStepRepository()
	failed := contractViolationFailedStep(runID, "failed-run-budget")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-run-budget", InitialStepKind: llmWorkspaceExecutionStepKind}
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	for attempt := 1; attempt <= 2; attempt++ {
		step := &runmodel.Step{
			ID:    common.StepID(string(runID) + "/contract-retry-run-budget-" + string(rune('0'+attempt))),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyContractViolationRecovery: map[string]any{
					"sourceStepId": "other-source",
				},
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassRuntime,
				failureOutputKeySignature: "class=runtime_failed|stage=runtime_contract_failed|run-budget",
			},
		}
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed run-budget retry step %d: %v", attempt, err)
		}
	}

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{
		MaxContractViolationRetriesPerStep: 5,
		MaxContractViolationRetriesPerRun:  2,
	}), true, true, run, failed)

	if action != runtimeRecoveryActionContractViolationExhausted {
		t.Fatalf("recovery action = %s, want contract violation exhausted by run budget", action)
	}
}

func TestContractViolationRecoveryActionExhaustsRepeatedSignatureBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-signature-budget")
	repo := memory.NewStepRepository()
	failed := contractViolationFailedStep(runID, "failed-signature-budget")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-signature-budget", InitialStepKind: llmWorkspaceExecutionStepKind}
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	signature := stringFromMap(failed.Output, failureOutputKeySignature)
	for attempt := 1; attempt <= 2; attempt++ {
		step := &runmodel.Step{
			ID:    common.StepID(string(runID) + "/contract-retry-signature-budget-" + string(rune('0'+attempt))),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyContractViolationRecovery: map[string]any{
					"sourceStepId": "previous-source",
				},
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassRuntime,
				failureOutputKeySignature: signature,
				stepOutputKeyContractViolation: map[string]any{
					"violatedRule": contractViolationRuleStateChangingRequiredMissing,
				},
			},
		}
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed signature-budget retry step %d: %v", attempt, err)
		}
	}

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{
		MaxContractViolationRetriesPerStep:    5,
		MaxContractViolationRetriesPerRun:     5,
		MaxRepeatedContractViolationSignature: 1,
	}), true, true, run, failed)

	if action != runtimeRecoveryActionContractViolationExhausted {
		t.Fatalf("recovery action = %s, want contract violation exhausted by repeated signature", action)
	}
}

func TestContractViolationClassifiesStructuredCoverageGapsAsCoverageIncomplete(t *testing.T) {
	runID := common.RunID("run-contract-coverage-incomplete")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-coverage-incomplete", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-coverage-incomplete")
	failed.Input = map[string]any{
		stepInputKeyRuntimeStage: runtimeStageVerification,
		"repairObjective":        "complete_dod_goal_coverage_assessment",
	}
	failed.Output = map[string]any{
		"stage":                   "llm_dod_assessment",
		"error":                   "invalid argument: dodAssessment is not satisfied",
		failureOutputKeyClass:     failureClassDoDCoverageMissing,
		failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
		"runtimeDiagnostics": map[string]any{
			"kind": failureClassDoDCoverageMissing,
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-neutral",
				"item":              "neutral goal leaf",
				"state":             "evidence_ready",
				"evidenceRefs":      []string{"run/graph/verify"},
			}},
		},
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"items": []map[string]any{{
				"item":         "neutral DoD item",
				"state":        "satisfied",
				"evidenceRefs": []string{"run/graph/verify"},
			}},
			"goalCoverage": map[string]any{
				"satisfied": false,
				"items": []map[string]any{{
					"goalRequirementId": "goal-neutral",
					"item":              "neutral goal leaf",
					"state":             "evidence_ready",
					"evidenceRefs":      []string{"run/graph/verify"},
				}},
			},
		},
	}

	attachContractViolationDiagnostics(run, failed, errors.New("invalid argument: dodAssessment is not satisfied"))

	recovery := contractViolationFromFailedOutput(failed)
	if got := stringFromMap(recovery, "violatedRule"); got != contractViolationRuleCoverageAssessmentIncomplete {
		t.Fatalf("violatedRule = %q, want %s; recovery=%+v", got, contractViolationRuleCoverageAssessmentIncomplete, recovery)
	}
	if got := stringFromMap(recovery, "recoverability"); got != runtimemodel.ContractViolationRecoverabilityRecomputeAuthority {
		t.Fatalf("recoverability = %q, want recompute_authority", got)
	}
	contract := mapFromValue(recovery["correctionContract"])
	if got := stringFromMap(contract, "requiredOutputShape"); got != "traceable_coverage_assessment" {
		t.Fatalf("requiredOutputShape = %q, want traceable_coverage_assessment; contract=%+v", got, contract)
	}
	if runtimeAuthority := mapFromValue(contract["runtimeRecomputedAuthority"]); stringFromMap(runtimeAuthority, "owner") != "runtime" {
		t.Fatalf("runtimeRecomputedAuthority = %+v, want runtime-owned diagnostic", runtimeAuthority)
	}
	if !stringSliceContains(stringSliceFromValue(contract["forbiddenOutputs"]), "same_failed_global_dodAssessment") {
		t.Fatalf("forbiddenOutputs = %+v, want same_failed_global_dodAssessment", contract["forbiddenOutputs"])
	}
}

func TestCoverageAssessmentIncompleteUsesContractRetryBudgetBeforeRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-coverage-incomplete-runtime-repair")
	repo := memory.NewStepRepository()
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-coverage-incomplete-runtime-repair",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		DoDID:           "dod-coverage-incomplete",
	}
	failed := contractViolationFailedStep(runID, "failed-coverage-incomplete")
	failed.Input = map[string]any{
		stepInputKeyRuntimeStage: runtimeStageVerification,
		"repairObjective":        "complete_dod_goal_coverage_assessment",
	}
	failed.Output = map[string]any{
		"stage":                   "llm_dod_assessment",
		"error":                   "invalid argument: dodAssessment is not satisfied",
		failureOutputKeyClass:     failureClassDoDCoverageMissing,
		failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
		"runtimeDiagnostics": map[string]any{
			"kind": failureClassDoDCoverageMissing,
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-neutral",
				"item":              "neutral goal leaf",
				"state":             "evidence_ready",
				"evidenceRefs":      []string{"run/graph/verify"},
			}},
		},
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"items": []map[string]any{{
				"item":         "neutral DoD item",
				"state":        "satisfied",
				"evidenceRefs": []string{"run/graph/verify"},
			}},
			"goalCoverage": map[string]any{
				"satisfied": false,
				"items": []map[string]any{{
					"goalRequirementId": "goal-neutral",
					"item":              "neutral goal leaf",
					"state":             "evidence_ready",
					"evidenceRefs":      []string{"run/graph/verify"},
				}},
			},
		},
	}
	attachContractViolationDiagnostics(run, failed, errors.New("invalid argument: dodAssessment is not satisfied"))

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{}), true, true, run, failed)

	if action != runtimeRecoveryActionContractViolationRetry {
		t.Fatalf("recovery action = %s, want bounded contract violation retry before runtime repair", action)
	}
}

func TestRuntimeRepairInputIncludesContractViolationRecovery(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-input")
	coverageContext := map[string]any{
		"definitionRevision": "rev-contract-retry",
		"currentFocus": map[string]any{
			"id":                   "cwi-contract-retry",
			"targetText":           "neutral contract retry focus",
			"rejectedEvidenceRefs": []string{"rejected-contract-diagnostic"},
		},
		"unresolvedItems": []map[string]any{{
			"id":                   "cwi-contract-retry",
			"missingEvidenceKinds": []string{"state_change"},
		}},
	}
	run := &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        "ws-contract-violation-input",
		GoalID:             "goal-contract-retry",
		DoDID:              "dod-contract-retry",
		DefinitionRevision: "rev-contract-retry",
		InitialStepKind:    llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"definitionRevision":       "rev-contract-retry",
			coverageWorkItemContextKey: coverageContext,
			gapBaselineKey: map[string]any{
				"definitionRevision": "rev-contract-retry",
			},
		},
	}
	failed := contractViolationFailedStep(runID, "failed-001")
	failed.Input[coverageWorkItemContextKey] = coverageContext
	pkg := mapFromValue(failed.Input["repairActionPackage"])
	pkg[coverageWorkItemContextKey] = coverageContext
	pkg["targetSnapshots"] = []map[string]any{{
		"targetRef":        "neutral-target",
		"contentHash":      "sha256:neutral",
		"contentComplete":  true,
		"safeForFileWrite": true,
	}}
	failed.Input["repairActionPackage"] = pkg
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	orch := &RuntimeOrchestrator{
		steps:  memory.NewStepRepository(),
		policy: NormalizeRuntimePolicy(RuntimePolicy{}),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failed, 1, common.ErrInvalidArgument)

	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if stringFromMap(recovery, "violationType") != contractViolationRepairNoStateChangingOutput {
		t.Fatalf("contractViolationRecovery = %+v, want violationType", recovery)
	}
	if stringFromMap(recovery, "violatedRule") != contractViolationRuleStateChangingRequiredMissing {
		t.Fatalf("contractViolationRecovery = %+v, want state-changing rule", recovery)
	}
	if signature := stringFromMap(recovery, "signature"); !strings.Contains(signature, "type="+contractViolationRepairNoStateChangingOutput) ||
		!strings.Contains(signature, "rule="+contractViolationRuleStateChangingRequiredMissing) {
		t.Fatalf("contractViolationRecovery signature = %q, want type and rule", signature)
	}
	if recovery["attempt"] != 1 || recovery["maxAttempts"] != NormalizeRuntimePolicy(RuntimePolicy{}).MaxContractViolationRetriesPerStep {
		t.Fatalf("contractViolationRecovery budget = %+v, want attempt/max", recovery)
	}
	if stringFromMap(recovery, "correctionProfileId") != "state_changing_required_output" ||
		stringFromMap(recovery, "correctionMode") != contractViolationCorrectionModeTargeted {
		t.Fatalf("contractViolationRecovery correction = %+v, want targeted state-changing profile", recovery)
	}
	correction := mapFromValue(recovery["correctionContract"])
	if stringFromMap(correction, "requiredOutputShape") != "state_changing_output" ||
		!stringSliceContains(stringSliceFromValue(correction["forbiddenOutputs"]), "failed_dodAssessment_only") {
		t.Fatalf("correctionContract = %+v, want state-changing output contract", correction)
	}
	expected := mapFromValue(recovery["expectedContract"])
	if expected["stateChangingOutputRequired"] != true {
		t.Fatalf("expectedContract = %+v, want stateChangingOutputRequired", expected)
	}
	if input[stepInputKeyRecoveryLineageType] != "contract_violation_retry" {
		t.Fatalf("recovery lineage = %v, want contract_violation_retry", input[stepInputKeyRecoveryLineageType])
	}
	budgetScope := mapFromValue(input[stepInputKeyRecoveryBudgetScope])
	if stringFromMap(budgetScope, "lineageType") != "contract_violation_retry" ||
		stringFromMap(budgetScope, "focusId") != "cwi-contract-retry" ||
		!stringSliceContains(stringSliceFromValue(budgetScope["coverageWorkItemIds"]), "cwi-contract-retry") {
		t.Fatalf("recoveryBudgetScope = %+v, want contract violation retry scoped to current authority focus", budgetScope)
	}
	recoveryBudget := mapFromValue(recovery["budgetScope"])
	if stableCoverageJSON(recoveryBudget) != stableCoverageJSON(budgetScope) {
		t.Fatalf("contractViolationRecovery.budgetScope = %+v, want recoveryBudgetScope %+v", recoveryBudget, budgetScope)
	}
	if !strings.Contains(strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n"), "contractViolationRecovery.expectedContract") {
		t.Fatalf("repairGuidance = %+v, want contract violation guidance", input["repairGuidance"])
	}
	if !strings.Contains(strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n"), "contractViolationRecovery.correctionContract") {
		t.Fatalf("repairGuidance = %+v, want correction contract guidance", input["repairGuidance"])
	}
	packet := mapFromValue(input[stepInputKeyStagePacket])
	if len(packet) == 0 {
		t.Fatalf("stagePacket missing from contract retry input: %+v", input)
	}
	definition := mapFromValue(packet["definition"])
	if definition["revision"] != "rev-contract-retry" ||
		definition["goalId"] != "goal-contract-retry" ||
		definition["doDId"] != "dod-contract-retry" {
		t.Fatalf("stagePacket.definition = %+v, want current definition", definition)
	}
	if packet["activeStage"] != runtimeStageRepair {
		t.Fatalf("stagePacket.activeStage = %v, want %s", packet["activeStage"], runtimeStageRepair)
	}
	focus := mapFromValue(packet["activeFocus"])
	if focus["id"] != "cwi-contract-retry" {
		t.Fatalf("stagePacket.activeFocus = %+v, want inherited focus", focus)
	}
	required := mapFromValue(packet["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("stagePacket.requiredOutput = %+v, want inherited required output", required)
	}
	if refs := stringSliceFromValue(packet["rejectedEvidenceRefs"]); !stringSliceContains(refs, "rejected-contract-diagnostic") {
		t.Fatalf("stagePacket.rejectedEvidenceRefs = %+v, want rejected diagnostic ref", refs)
	}
}

func TestContractViolationClassifiesLLMSchemaInvalidAsRecoverableDiagnostic(t *testing.T) {
	runID := common.RunID("run-contract-llm-schema-invalid")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-llm-schema-invalid", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := &runmodel.Step{
		ID:    common.StepID(runID + "/failed-schema"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: "execution",
			"requiredOutput": map[string]any{
				"mustReturnSingleJSONDecision": true,
			},
		},
		Output: map[string]any{
			"status":      "failed",
			"stage":       "llm_schema_invalid",
			"error":       "invalid argument: invalid llm decision json: no json object found",
			"parseError":  "invalid argument: invalid llm decision json: no json object found",
			"repairError": "invalid argument: invalid llm decision json: unknown completion state",
		},
	}
	execErr := errors.New("invalid argument: invalid llm decision json: no json object found")

	attachContractViolationType(failed.Output, execErr)
	classification := classifyRuntimeFailure(failed, failed.Output, "runtime step failed", execErr)
	attachRuntimeFailureClassification(failed.Output, classification)
	attachContractViolationDiagnostics(run, failed, execErr)

	if got := stringFromMap(failed.Output, outputKeyContractViolationType); got != contractViolationLLMSchemaInvalid {
		t.Fatalf("contractViolationType = %q, want %s", got, contractViolationLLMSchemaInvalid)
	}
	recovery := contractViolationFromFailedOutput(failed)
	if got := stringFromMap(recovery, "violationType"); got != contractViolationLLMSchemaInvalid {
		t.Fatalf("violationType = %q, want %s; recovery=%+v", got, contractViolationLLMSchemaInvalid, recovery)
	}
	if got := stringFromMap(recovery, "violatedRule"); got != contractViolationRuleUnparseableOutput {
		t.Fatalf("violatedRule = %q, want %s; recovery=%+v", got, contractViolationRuleUnparseableOutput, recovery)
	}
	if got := stringFromMap(recovery, "recoverability"); got != runtimemodel.ContractViolationRecoverabilityRetrySameContract {
		t.Fatalf("recoverability = %q, want retry_same_contract; recovery=%+v", got, recovery)
	}
	if !strings.Contains(stringFromMap(recovery, "signature"), "type="+contractViolationLLMSchemaInvalid) ||
		!strings.Contains(stringFromMap(recovery, "signature"), "rule="+contractViolationRuleUnparseableOutput) {
		t.Fatalf("signature = %q, want type and rule", stringFromMap(recovery, "signature"))
	}
	actual := mapFromValue(recovery["actualOutputSummary"])
	if stringFromMap(actual, "parseError") == "" || stringFromMap(actual, "repairError") == "" {
		t.Fatalf("actualOutputSummary = %+v, want parse and repair diagnostics", actual)
	}
	if got := stringFromMap(recovery, "correctionProfileId"); got != "json_output_shape" {
		t.Fatalf("correctionProfileId = %q, want json_output_shape; recovery=%+v", got, recovery)
	}
	if !failedStepHasRecoverableContractViolation(failed) {
		t.Fatalf("failed step should have recoverable contract violation: %+v", recovery)
	}
}

func TestContractViolationValidationRequiredOutputCapabilityMismatchRebuildsAuthority(t *testing.T) {
	runID := common.RunID("run-contract-validation-required-output-mismatch")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-validation-required-output-mismatch", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-validation-required-output-mismatch")
	failed.Input = validationDispatchInputFixture()
	failed.Input[stepInputKeyRuntimeStage] = runtimeStageVerification
	failed.Input["repairObjective"] = repairObjectiveConcreteValidationEvidence
	required := mapFromValue(failed.Input["requiredOutput"])
	required["acceptedKinds"] = []string{runtimemodel.StepKindShellCommand, managedValidationSessionNodeKind}
	failed.Input["requiredOutput"] = required
	failed.Input["repairActionPackage"] = map[string]any{"requiredOutput": required}
	failed.Input[stepInputKeyStagePacket] = map[string]any{"requiredOutput": required}
	failed.Input[currentActionContractKey] = map[string]any{
		"currentFocus":   mapFromValue(failed.Input[coverageFocusKey]),
		"requiredOutput": required,
		"nextAction":     map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		"schemaVersion":  currentActionContractSchema,
	}
	failed.Output = map[string]any{
		"stage":                   "runtime_required_output_kind_rejected",
		"error":                   "validation_required_output_capability_mismatch: runtime step kind mcp/tool_call is not accepted by current requiredOutput.acceptedKinds",
		failureOutputKeyClass:     failureClassRuntimeContractMisalign,
		failureOutputKeySignature: "class=runtime_internal_contract_misalignment|kind=mcp/tool_call|stage=runtime_required_output_kind_rejected",
	}

	attachContractViolationType(failed.Output, common.ErrInvalidArgument)
	attachContractViolationDiagnostics(run, failed, common.ErrInvalidArgument)

	recovery := contractViolationFromFailedOutput(failed)
	if got := stringFromMap(recovery, "violationType"); got != contractViolationValidationRequiredOutputCapabilityMismatch {
		t.Fatalf("violationType = %q, want %s; recovery=%+v", got, contractViolationValidationRequiredOutputCapabilityMismatch, recovery)
	}
	if got := stringFromMap(recovery, "violatedRule"); got != contractViolationRuleValidationRequiredOutputCapabilityMismatch {
		t.Fatalf("violatedRule = %q, want %s; recovery=%+v", got, contractViolationRuleValidationRequiredOutputCapabilityMismatch, recovery)
	}
	if got := stringFromMap(recovery, "recoverability"); got != runtimemodel.ContractViolationRecoverabilityRebuildValidationRequiredOutput {
		t.Fatalf("recoverability = %q, want validation requiredOutput authority rebuild; recovery=%+v", got, recovery)
	}
	if got := stringFromMap(recovery, "correctionProfileId"); got != "validation_required_output_authority_rebuild" {
		t.Fatalf("correctionProfileId = %q, want validation authority rebuild; recovery=%+v", got, recovery)
	}
	contract := mapFromValue(recovery["correctionContract"])
	if got := stringFromMap(contract, "requiredOutputShape"); got != "authority_derived_validation_required_output" {
		t.Fatalf("requiredOutputShape = %q, want authority-derived validation requiredOutput; contract=%+v", got, contract)
	}
	if !failedStepHasRecoverableContractViolation(failed) || !failedStepRequiresAuthorityRecompute(failed) {
		t.Fatalf("failed step should be recoverable authority recompute: recovery=%+v", recovery)
	}
}

func TestContractViolationCorrectionProfilesCoverPrimaryRules(t *testing.T) {
	tests := []struct {
		name           string
		violationType  string
		errText        string
		wantRule       string
		wantProfile    string
		wantShape      string
		wantForbidden  string
		wantTargetRule bool
	}{
		{
			name:          "state changing",
			violationType: contractViolationRepairNoStateChangingOutput,
			errText:       "state-changing llm repair step returned no state-changing output",
			wantRule:      contractViolationRuleStateChangingRequiredMissing,
			wantProfile:   "state_changing_required_output",
			wantShape:     "state_changing_output",
			wantForbidden: "failed_dodAssessment_only",
		},
		{
			name:          "concrete validation",
			violationType: contractViolationValidationEvidenceRequired,
			errText:       "concrete validation evidence repair must return bounded shell/command",
			wantRule:      contractViolationRuleConcreteValidationMissing,
			wantProfile:   "concrete_validation_evidence",
			wantShape:     "bounded_validation_evidence",
			wantForbidden: "write_outside_validationScratchDir",
		},
		{
			name:           "patch apply",
			violationType:  contractViolationPatchContentRequired,
			errText:        "patch content is required",
			wantRule:       contractViolationRulePatchApplyContractFailed,
			wantProfile:    "patch_apply_contract",
			wantShape:      "valid_patch_apply",
			wantForbidden:  "missing_content_or_diff",
			wantTargetRule: true,
		},
		{
			name:          "same graph read before patch",
			violationType: contractViolationGraphDependsOnLLMBeforePatch,
			errText:       "executionGraph patch/apply must not depend on same-graph file/read",
			wantRule:      contractViolationRuleGraphDependsOnLLMBeforePatch,
			wantProfile:   "target_snapshot_before_patch",
			wantShape:     "target_snapshot_acquisition_or_direct_snapshot_patch",
			wantForbidden: "same_graph_file_read_before_patch",
		},
		{
			name:          "coverage evidence",
			violationType: contractViolationDoDAssessmentEvidenceMissing,
			errText:       "satisfied dodAssessment item requires evidenceRefs",
			wantRule:      contractViolationRuleDoDAssessmentEvidenceMissing,
			wantProfile:   "coverage_evidence_assessment",
			wantShape:     "traceable_coverage_assessment",
			wantForbidden: "satisfied_item_without_evidenceRefs",
		},
		{
			name:          "unparseable",
			violationType: "",
			errText:       "invalid llm decision json",
			wantRule:      contractViolationRuleUnparseableOutput,
			wantProfile:   "json_output_shape",
			wantShape:     "single_valid_json_decision",
			wantForbidden: "partial_json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runID := common.RunID("run-contract-correction-profile-" + strings.ReplaceAll(tt.name, " ", "-"))
			run := &runmodel.Run{ID: runID, WorkspaceID: common.WorkspaceID("ws-" + strings.ReplaceAll(tt.name, " ", "-")), InitialStepKind: llmWorkspaceExecutionStepKind}
			failed := contractViolationFailedStep(runID, "failed-profile")
			failed.Output["error"] = tt.errText
			failed.Output[failureOutputKeySignature] = "class=runtime_failed|stage=runtime_contract_failed|failureTargets=target.txt"
			if tt.violationType != "" {
				failed.Output[outputKeyContractViolationType] = tt.violationType
			} else {
				delete(failed.Output, outputKeyContractViolationType)
				failed.Output["stage"] = "llm_schema_invalid"
			}
			if tt.wantRule == contractViolationRuleConcreteValidationMissing {
				failed.Input["repairObjective"] = repairObjectiveConcreteValidationEvidence
				pkg := mapFromValue(failed.Input["repairActionPackage"])
				required := mapFromValue(pkg["requiredOutput"])
				required["mustProduceConcreteValidationEvidence"] = true
				pkg["requiredOutput"] = required
				ensureConcreteValidationScratchContract(failed.Input, pkg, required)
				failed.Input["repairActionPackage"] = pkg
			}
			if tt.wantRule == contractViolationRuleDoDAssessmentEvidenceMissing {
				failed.Input = map[string]any{
					stepInputKeyRuntimeStage: "assessment",
				}
			}
			if tt.wantTargetRule {
				pkg := mapFromValue(failed.Input["repairActionPackage"])
				pkg["targetSnapshots"] = []map[string]any{{
					"targetRef":        "target.txt",
					"contentHash":      "hash-target",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}}
				failed.Input["repairActionPackage"] = pkg
			}
			attachContractViolationDiagnostics(run, failed, common.ErrInvalidArgument)

			recovery := contractViolationFromFailedOutput(failed)
			if got := stringFromMap(recovery, "violatedRule"); got != tt.wantRule {
				t.Fatalf("violatedRule = %q, want %s; recovery=%+v", got, tt.wantRule, recovery)
			}
			if got := stringFromMap(recovery, "correctionProfileId"); got != tt.wantProfile {
				t.Fatalf("correctionProfileId = %q, want %s; recovery=%+v", got, tt.wantProfile, recovery)
			}
			contract := mapFromValue(recovery["correctionContract"])
			if got := stringFromMap(contract, "requiredOutputShape"); got != tt.wantShape {
				t.Fatalf("requiredOutputShape = %q, want %s; contract=%+v", got, tt.wantShape, contract)
			}
			if !stringSliceContains(stringSliceFromValue(contract["forbiddenOutputs"]), tt.wantForbidden) {
				t.Fatalf("forbiddenOutputs = %+v, want %s", contract["forbiddenOutputs"], tt.wantForbidden)
			}
			if tt.wantTargetRule {
				if stringFromMap(contract, "targetScope") != "failureTargets=target.txt" {
					t.Fatalf("correctionContract targetScope = %+v, want failureTargets=target.txt", contract)
				}
				if snapshots := mapSliceFromValue(contract["targetSnapshots"]); len(snapshots) != 1 || stringFromMap(snapshots[0], "targetRef") != "target.txt" {
					t.Fatalf("targetSnapshots = %+v, want compact target snapshot", contract["targetSnapshots"])
				}
			}
		})
	}
}

func TestContractViolationWrongShapePreservesStateChangingRequiredOutput(t *testing.T) {
	runID := common.RunID("run-contract-wrong-shape-state-changing")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-wrong-shape-state-changing", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-wrong-shape-state-changing")
	failed.Input["repairObjective"] = repairObjectiveStateChangingOutput
	failed.Input["stateChangingOutputRequired"] = true
	pkg := mapFromValue(failed.Input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	required["mustIncludeStateChangingNode"] = true
	required["acceptedKinds"] = []string{"patch/apply", "git/write"}
	required["mustProduceConcreteValidationEvidence"] = true
	pkg["requiredOutput"] = required
	failed.Input["repairActionPackage"] = pkg
	failed.Output[outputKeyContractViolationType] = contractViolationAssessmentOnlyExecutableOutput
	failed.Output["error"] = "assessment-only step returned executable or state-changing output fields: nextKind, nextStep"
	failed.Output[failureOutputKeySignature] = "class=runtime_failed|kind=llm/workspace_execution|stage=llm_decision_required|error=wrong output shape"

	attachContractViolationDiagnostics(run, failed, common.ErrInvalidArgument)

	recovery := contractViolationFromFailedOutput(failed)
	if got := stringFromMap(recovery, "violatedRule"); got != contractViolationRuleWrongOutputShape {
		t.Fatalf("violatedRule = %q, want %s; recovery=%+v", got, contractViolationRuleWrongOutputShape, recovery)
	}
	if got := stringFromMap(recovery, "correctionProfileId"); got != "state_changing_required_output" {
		t.Fatalf("correctionProfileId = %q, want state_changing_required_output; recovery=%+v", got, recovery)
	}
	contract := mapFromValue(recovery["correctionContract"])
	if got := stringFromMap(contract, "requiredOutputShape"); got != "state_changing_output" {
		t.Fatalf("requiredOutputShape = %q, want state_changing_output; contract=%+v", got, contract)
	}
	if !boolValue(contract["mustReturnValidJSON"]) {
		t.Fatalf("correctionContract = %+v, want JSON-shape recovery to preserve valid JSON requirement", contract)
	}
	if !stringSliceContains(stringSliceFromValue(contract["allowedOutputs"]), "patch/apply") {
		t.Fatalf("allowedOutputs = %+v, want state-changing outputs preserved", contract["allowedOutputs"])
	}
}

func TestContractViolationConcreteValidationForbidsContinuableDecisionBlocker(t *testing.T) {
	runID := common.RunID("run-contract-continuable-decision-blocker")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-continuable-decision-blocker", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-continuable-decision")
	failed.Input["repairObjective"] = repairObjectiveConcreteValidationEvidence
	pkg := mapFromValue(failed.Input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	required["mustProduceConcreteValidationEvidence"] = true
	required["acceptedKinds"] = []string{"shell/command", "mcp/tool_call"}
	pkg["requiredOutput"] = required
	failed.Input["repairActionPackage"] = pkg
	failed.Output[outputKeyContractViolationType] = contractViolationValidationEvidenceRequired
	failed.Output["error"] = "invalid argument: decision requests a continuable candidate choice"
	failed.Output[failureOutputKeySignature] = "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_decision_required"
	attachContractViolationDiagnostics(run, failed, common.ErrInvalidArgument)

	recovery := contractViolationFromFailedOutput(failed)
	contract := mapFromValue(recovery["correctionContract"])
	allowed := stringSliceFromValue(contract["allowedOutputs"])
	if stringSliceContains(allowed, "blocked_on_decision") {
		t.Fatalf("allowedOutputs = %+v, want no bare blocked_on_decision", allowed)
	}
	if !stringSliceContains(allowed, "blocked_on_decision_with_required_external_dependency") {
		t.Fatalf("allowedOutputs = %+v, want external-dependency decision blocker allowed", allowed)
	}
	forbidden := stringSliceFromValue(contract["forbiddenOutputs"])
	if !stringSliceContains(forbidden, "decision_request_when_runtime_can_continue") ||
		!stringSliceContains(forbidden, "blocked_on_decision_for_continuable_candidate_choice") {
		t.Fatalf("forbiddenOutputs = %+v, want continuable decision blockers forbidden", forbidden)
	}
}

func TestContractViolationCoverageAssessmentPreservesCurrentRequiredOutputShape(t *testing.T) {
	tests := []struct {
		name          string
		configure     func(map[string]any, map[string]any)
		wantProfile   string
		wantShape     string
		wantRequired  string
		wantForbidden string
	}{
		{
			name: "evidence acquisition",
			configure: func(input map[string]any, required map[string]any) {
				input[stepInputKeyRuntimeStage] = runtimeStageRepair
				input["repairObjective"] = repairObjectiveAcquireFailureEvidence
				delete(input, "stateChangingOutputRequired")
				required["mustIncludeStateChangingNode"] = false
				required["mustProduceConcreteFailureEvidence"] = true
				required["acceptedKinds"] = []string{"file/read", "shell/command", "mcp/tool_call", "git/read"}
				required["forbiddenKinds"] = []string{"patch/apply", "llm/workspace_execution"}
				required["deliverableWriteForbidden"] = true
			},
			wantProfile:   "concrete_failure_evidence",
			wantShape:     "bounded_failure_evidence",
			wantRequired:  "executionGraph.nodes",
			wantForbidden: "patch/apply",
		},
		{
			name: "state changing",
			configure: func(input map[string]any, required map[string]any) {
				input[stepInputKeyRuntimeStage] = runtimeStageRepair
				input["repairObjective"] = repairObjectiveStateChangingOutput
				input["stateChangingOutputRequired"] = true
				required["mustIncludeStateChangingNode"] = true
				required["acceptedKinds"] = []string{"patch/apply"}
			},
			wantProfile:   "state_changing_required_output",
			wantShape:     "state_changing_output",
			wantRequired:  "stateChangingOutputRequired",
			wantForbidden: "failed_dodAssessment_only",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runID := common.RunID("run-contract-coverage-required-output-" + strings.ReplaceAll(tt.name, " ", "-"))
			run := &runmodel.Run{ID: runID, WorkspaceID: common.WorkspaceID("ws-" + strings.ReplaceAll(tt.name, " ", "-")), InitialStepKind: llmWorkspaceExecutionStepKind}
			failed := contractViolationFailedStep(runID, "failed-coverage-required-output")
			failed.Output = map[string]any{
				"stage":                   "llm_dod_assessment",
				"error":                   "invalid argument: dodAssessment is not satisfied",
				failureOutputKeyClass:     failureClassDoDCoverageMissing,
				failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
				"runtimeDiagnostics": map[string]any{
					"missingGoalCoverage": []map[string]any{{
						"goalRequirementId": "goal-current",
						"item":              "neutral current gap",
						"state":             "open",
					}},
				},
				"dodAssessment": map[string]any{
					"satisfied": false,
					"summary":   "not enough evidence",
				},
			}
			pkg := mapFromValue(failed.Input["repairActionPackage"])
			required := mapFromValue(pkg["requiredOutput"])
			tt.configure(failed.Input, required)
			pkg["requiredOutput"] = required
			failed.Input["repairActionPackage"] = pkg
			failed.Input[convergenceAuthorityKey] = map[string]any{
				"nextAction": map[string]any{"kind": "validate"},
			}

			attachContractViolationDiagnostics(run, failed, errors.New("invalid argument: dodAssessment is not satisfied"))

			recovery := contractViolationFromFailedOutput(failed)
			if got := stringFromMap(recovery, "violatedRule"); got != contractViolationRuleCoverageAssessmentIncomplete {
				t.Fatalf("violatedRule = %q, want %s; recovery=%+v", got, contractViolationRuleCoverageAssessmentIncomplete, recovery)
			}
			if got := stringFromMap(recovery, "correctionProfileId"); got != tt.wantProfile {
				t.Fatalf("correctionProfileId = %q, want %s; recovery=%+v", got, tt.wantProfile, recovery)
			}
			contract := mapFromValue(recovery["correctionContract"])
			if got := stringFromMap(contract, "requiredOutputShape"); got != tt.wantShape {
				t.Fatalf("requiredOutputShape = %q, want %s; contract=%+v", got, tt.wantShape, contract)
			}
			if got := stringFromMap(mapFromValue(contract["runtimeRecomputedAuthority"]), "llmRequiredOutputShape"); got != tt.wantShape {
				t.Fatalf("runtimeRecomputedAuthority = %+v, want llmRequiredOutputShape %s", contract["runtimeRecomputedAuthority"], tt.wantShape)
			}
			if !stringSliceContains(stringSliceFromValue(contract["forbiddenOutputs"]), tt.wantForbidden) {
				t.Fatalf("forbiddenOutputs = %+v, want %s", contract["forbiddenOutputs"], tt.wantForbidden)
			}
			if !stringSliceContains(stringSliceFromValue(contract["requiredFields"]), tt.wantRequired) &&
				!stringSliceContains(stringSliceFromValue(contract["mustSatisfy"]), tt.wantRequired) {
				t.Fatalf("contract = %+v, want required marker %s", contract, tt.wantRequired)
			}
		})
	}
}

func TestContractViolationEvidenceAcquisitionPreservesFailureEvidenceShape(t *testing.T) {
	runID := common.RunID("run-contract-failure-evidence-shape")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-failure-evidence-shape", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-failure-evidence-shape")
	failed.Input[stepInputKeyRuntimeStage] = runtimeStageRepair
	failed.Input["repairObjective"] = repairObjectiveAcquireFailureEvidence
	pkg := mapFromValue(failed.Input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	required["mustProduceConcreteFailureEvidence"] = true
	required["mustProduceConcreteValidationEvidence"] = false
	required["acceptedKinds"] = []string{"file/read", "shell/command", "mcp/tool_call"}
	required["forbiddenKinds"] = []string{"patch/apply", "llm/workspace_execution"}
	required["deliverableWriteForbidden"] = true
	pkg["requiredOutput"] = required
	failed.Input["repairActionPackage"] = pkg
	failed.Output = map[string]any{
		"stage":                        "runtime_contract_failed",
		"error":                        "concrete failure evidence acquisition must return bounded read-only evidence without patch/apply or deliverable writes",
		outputKeyContractViolationType: contractViolationValidationEvidenceRequired,
		failureOutputKeyClass:          failureClassRuntime,
		failureOutputKeySignature:      "class=runtime_failed|stage=runtime_contract_failed|rule=validation_evidence_required",
	}
	attachContractViolationDiagnostics(run, failed, common.ErrInvalidArgument)

	recovery := contractViolationFromFailedOutput(failed)
	if got := stringFromMap(recovery, "correctionProfileId"); got != "concrete_failure_evidence" {
		t.Fatalf("correctionProfileId = %q, want concrete_failure_evidence; recovery=%+v", got, recovery)
	}
	contract := mapFromValue(recovery["correctionContract"])
	if got := stringFromMap(contract, "requiredOutputShape"); got != "bounded_failure_evidence" {
		t.Fatalf("requiredOutputShape = %q, want bounded_failure_evidence; contract=%+v", got, contract)
	}
	if !boolValue(contract["deliverableWriteForbidden"]) {
		t.Fatalf("contract = %+v, want deliverableWriteForbidden preserved", contract)
	}
	if !stringSliceContains(stringSliceFromValue(contract["forbiddenKinds"]), "patch/apply") ||
		!stringSliceContains(stringSliceFromValue(contract["forbiddenKinds"]), "llm/workspace_execution") {
		t.Fatalf("forbiddenKinds = %+v, want current failure-evidence forbidden kinds preserved", contract["forbiddenKinds"])
	}
	if !failedStepHasRecoverableContractViolation(failed) {
		t.Fatalf("failed step should remain recoverable under same current action: %+v", recovery)
	}
}

func TestContractViolationClassifiesAvailableSnapshotIgnored(t *testing.T) {
	runID := common.RunID("run-contract-available-snapshot-ignored")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-available-snapshot-ignored", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-available-snapshot")
	pkg := mapFromValue(failed.Input["repairActionPackage"])
	pkg["targetSnapshots"] = []map[string]any{{
		"targetRef":        "target.txt",
		"content":          "current target content\n",
		"contentHash":      "sha256-current-target",
		"contentComplete":  true,
		"safeForFileWrite": true,
	}}
	failed.Input["repairActionPackage"] = pkg
	failed.Output[outputKeyContractViolationType] = contractViolationAvailableSnapshotIgnored

	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))

	recovery := contractViolationFromFailedOutput(failed)
	if got := stringFromMap(recovery, "violatedRule"); got != contractViolationRuleAvailableSnapshotIgnored {
		t.Fatalf("violatedRule = %q, want %s; recovery=%+v", got, contractViolationRuleAvailableSnapshotIgnored, recovery)
	}
	if got := stringFromMap(recovery, "correctionProfileId"); got != "available_snapshot_direct_action" {
		t.Fatalf("correctionProfileId = %q, want available_snapshot_direct_action; recovery=%+v", got, recovery)
	}
	contract := mapFromValue(recovery["correctionContract"])
	if got := stringFromMap(contract, "requiredOutputShape"); got != "state_changing_output_from_available_snapshot" {
		t.Fatalf("requiredOutputShape = %q, want state_changing_output_from_available_snapshot; contract=%+v", got, contract)
	}
	if !stringSliceContains(stringSliceFromValue(contract["forbiddenOutputs"]), "missing_snapshot_claim") {
		t.Fatalf("forbiddenOutputs = %+v, want missing_snapshot_claim", contract["forbiddenOutputs"])
	}
	snapshots := mapSliceFromValue(contract["availableTargetSnapshots"])
	if len(snapshots) != 1 || stringFromMap(snapshots[0], "targetRef") != "target.txt" ||
		stringFromMap(snapshots[0], "contentHash") != "sha256-current-target" {
		t.Fatalf("availableTargetSnapshots = %+v, want current target snapshot summary", snapshots)
	}
}

func TestContractViolationClassifiesAvailableSnapshotIgnoredFromNestedStructuredContext(t *testing.T) {
	runID := common.RunID("run-contract-available-snapshot-ignored-nested")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-available-snapshot-ignored-nested", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-available-snapshot-nested")
	delete(mapFromValue(failed.Input["repairActionPackage"]), "targetSnapshots")
	failed.Input["failureTargetRepairContext"] = []map[string]any{{
		"targetRef":        "target.txt",
		"content":          "current target content\n",
		"contentHash":      "sha256-current-target",
		"contentComplete":  true,
		"safeForFileWrite": true,
		"exists":           true,
	}}
	failed.Output[outputKeyContractViolationType] = contractViolationAvailableSnapshotIgnored

	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))

	recovery := contractViolationFromFailedOutput(failed)
	if got := stringFromMap(recovery, "violatedRule"); got != contractViolationRuleAvailableSnapshotIgnored {
		t.Fatalf("violatedRule = %q, want %s; recovery=%+v", got, contractViolationRuleAvailableSnapshotIgnored, recovery)
	}
	contract := mapFromValue(recovery["correctionContract"])
	snapshots := mapSliceFromValue(contract["availableTargetSnapshots"])
	if len(snapshots) != 1 || stringFromMap(snapshots[0], "targetRef") != "target.txt" ||
		stringFromMap(snapshots[0], "contentHash") != "sha256-current-target" {
		t.Fatalf("availableTargetSnapshots = %+v, want nested current target snapshot summary", snapshots)
	}
}

func TestContractViolationDoesNotClassifyAvailableSnapshotIgnoredFromNaturalLanguage(t *testing.T) {
	runID := common.RunID("run-contract-missing-snapshot-prose")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-missing-snapshot-prose", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-prose-available-snapshot")
	pkg := mapFromValue(failed.Input["repairActionPackage"])
	pkg["targetSnapshots"] = []map[string]any{{
		"targetRef":        "target.txt",
		"content":          "current target content\n",
		"contentHash":      "sha256-current-target",
		"contentComplete":  true,
		"safeForFileWrite": true,
	}}
	failed.Input["repairActionPackage"] = pkg
	failed.Output["error"] = "failed assessment claimed current target snapshot is missing and must read the file before patching"

	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))

	recovery := contractViolationFromFailedOutput(failed)
	if got := stringFromMap(recovery, "violatedRule"); got == contractViolationRuleAvailableSnapshotIgnored {
		t.Fatalf("violatedRule = %q, want no available snapshot classification from prose; recovery=%+v", got, recovery)
	}
}

func TestContractViolationRetryUsesForcedNarrowAfterRepeatedRule(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-forced-narrow")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-forced-narrow", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-forced-narrow")
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	repo := memory.NewStepRepository()
	previous := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/previous-contract-violation"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: stringFromMap(failed.Output, failureOutputKeySignature),
			stepOutputKeyContractViolation: map[string]any{
				"violatedRule": contractViolationRuleStateChangingRequiredMissing,
			},
		},
	}
	if err := repo.Create(ctx, previous); err != nil {
		t.Fatalf("seed previous violation: %v", err)
	}
	orch := &RuntimeOrchestrator{
		steps:  repo,
		policy: NormalizeRuntimePolicy(RuntimePolicy{}),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failed, 2, common.ErrInvalidArgument)

	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if got := stringFromMap(recovery, "correctionMode"); got != contractViolationCorrectionModeForcedNarrow {
		t.Fatalf("correctionMode = %q, want forced_narrow; recovery=%+v", got, recovery)
	}
	contract := mapFromValue(recovery["correctionContract"])
	if contract["forcedNarrow"] != true ||
		!strings.Contains(stringFromMap(contract, "forcedNarrowRule"), "same-rule or same-target") {
		t.Fatalf("correctionContract = %+v, want forced narrow rule", contract)
	}
}

func TestNormalizeCurrentActionStateChangingContractClearsStaleEvidenceAcquisition(t *testing.T) {
	runID := common.RunID("run-contract-violation-forced-narrow-state-change")
	failed := contractViolationFailedStep(runID, "failed-state-change")
	input := cloneStepPayload(failed.Input)
	input[stepInputKeyContractViolationRecovery] = map[string]any{
		"correctionMode":      contractViolationCorrectionModeForcedNarrow,
		"correctionProfileId": "state_changing_required_output",
		"correctionContract": map[string]any{
			"requiredOutputShape": "state_changing_output",
			"allowedOutputs": []string{
				"executionGraph_with_state_changing_node",
				"patch/apply",
			},
			"forbiddenOutputs": []string{
				"read_only_executionGraph",
			},
		},
	}
	pkg := mapFromValue(input["repairActionPackage"])
	pkg["repairObjective"] = repairObjectiveAcquireFailureEvidence
	pkg["evidenceAcquisitionRequired"] = true
	pkg["evidenceAcquisitionReason"] = "stale previous repair action"
	pkg["outputDecisionContract"] = map[string]any{"requiredMode": repairObjectiveAcquireFailureEvidence}
	pkg["exitCriteria"] = []string{"concrete_failure_evidence_acquired_then_target_scoped_repair"}
	input["repairActionPackage"] = pkg

	normalized := normalizeCurrentActionContract(input, failed)
	step := &runmodel.Step{ID: common.StepID(string(runID) + "/retry"), RunID: runID, Kind: llmWorkspaceExecutionStepKind, Input: normalized}
	if stepRequiresConcreteFailureEvidenceAcquisition(step) {
		t.Fatalf("normalized input still requires concrete failure evidence: %+v", normalized["repairActionPackage"])
	}
	if got := stringFromMap(normalized, "repairObjective"); got != repairObjectiveStateChangingOutput {
		t.Fatalf("repairObjective = %q, want %s", got, repairObjectiveStateChangingOutput)
	}
	pkg = mapFromValue(normalized["repairActionPackage"])
	if got := stringFromMap(pkg, "repairObjective"); got != repairObjectiveStateChangingOutput {
		t.Fatalf("repairActionPackage.repairObjective = %q, want %s; pkg=%+v", got, repairObjectiveStateChangingOutput, pkg)
	}
	for _, key := range []string{"evidenceAcquisitionRequired", "concreteFailureEvidenceRequired", "evidenceAcquisitionReason", "outputDecisionContract", "exitCriteria"} {
		if _, ok := pkg[key]; ok {
			t.Fatalf("repairActionPackage retained stale %s: %+v", key, pkg)
		}
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) || !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want state-changing patch contract", required)
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{{
			ID:   "patch",
			Kind: "patch/apply",
			Input: map[string]any{
				"targetRef":       "target.txt",
				"operation":       "file_write",
				"content":         "updated",
				"coverageTargets": []string{"neutral target"},
			},
		}}},
	}
	if err := validateLLMFollowUpGraphStateChangingContract(&runmodel.Run{}, step, output); err != nil {
		t.Fatalf("state-changing graph was validated as stale evidence acquisition: %v", err)
	}
}

func TestAuthorityRecomputeContractViolationDoesNotInheritStaleStateChangingPackage(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-recompute-authority")
	focus := map[string]any{
		"id":                   "cwi-validation",
		"coverageWorkItemIds":  []string{"cwi-validation"},
		"missingEvidenceKinds": []string{"validation"},
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-contract-violation-recompute-authority",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"currentFocus":  cloneStepPayload(focus),
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			},
			gapBaselineKey: map[string]any{
				"schemaVersion": gapBaselineSchema,
				"currentFocus":  cloneStepPayload(focus),
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			},
			coverageWorkItemContextKey: map[string]any{
				"currentFocus":    cloneStepPayload(focus),
				"unresolvedItems": []map[string]any{cloneStepPayload(focus)},
			},
		},
	}
	failed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/failed-recompute"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"repairObjective":             repairObjectiveStateChangingOutput,
				"stateChangingOutputRequired": true,
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
				},
			},
		},
		Output: map[string]any{
			"stage":                   "runtime_contract_failed",
			"error":                   "dodAssessment coverage is incomplete",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|rule=coverage_assessment_incomplete",
			stepOutputKeyContractViolation: map[string]any{
				"violatedRule":   contractViolationRuleCoverageAssessmentIncomplete,
				"recoverability": runtimemodel.ContractViolationRecoverabilityRecomputeAuthority,
			},
		},
	}
	orch := &RuntimeOrchestrator{steps: memory.NewStepRepository()}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failed, 1, common.ErrInvalidArgument)

	contract := mapFromValue(input["convergenceContract"])
	if state := stringFromMap(contract, "state"); state != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract.state = %q, want %s; input=%+v", state, convergenceStateNeedValidation, input)
	}
	nextAction := mapFromValue(mapFromValue(input[currentActionContractKey])["nextAction"])
	if kind := stringFromMap(nextAction, "kind"); kind != runtimemodel.NextActionTypeValidate {
		t.Fatalf("currentAction.nextAction = %+v, want validate; input=%+v", nextAction, input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		boolValue(required["mustIncludeStateChangingNode"]) {
		t.Fatalf("requiredOutput = %+v, want validation authority without inherited patch/apply", required)
	}
	if !boolValue(required["mustProduceConcreteValidationEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want concrete validation evidence", required)
	}
	if err := validateConvergenceDispatchInput(input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, input)
	}
}

func TestContractViolationRetryForcedNarrowValidationRequiredOutput(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-forced-narrow-validation")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-forced-narrow-validation", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/failed-validation"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: "validation",
			"repairObjective":        repairObjectiveConcreteValidationEvidence,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode":          true,
					"mustProduceConcreteValidationEvidence": true,
					"acceptedKinds":                         []string{"patch/apply", "shell/command"},
				},
			},
		},
		Output: map[string]any{
			"stage":                        "runtime_contract_failed",
			"error":                        "concrete validation evidence graph must not start detached or background processes",
			outputKeyContractViolationType: contractViolationValidationEvidenceRequired,
			"contractViolationRule":        contractViolationRuleValidationDetachedProcessForbidden,
			failureOutputKeyClass:          failureClassRuntime,
			failureOutputKeySignature:      "class=runtime_failed|stage=runtime_contract_failed|rule=validation_detached_process_forbidden",
		},
	}
	attachContractViolationDiagnostics(run, failed, errors.New("concrete validation evidence graph must not start detached or background processes"))
	orch := &RuntimeOrchestrator{
		steps:  memory.NewStepRepository(),
		policy: NormalizeRuntimePolicy(RuntimePolicy{}),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failed, 2, common.ErrInvalidArgument)

	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if got := stringFromMap(recovery, "correctionMode"); got != contractViolationCorrectionModeForcedNarrow {
		t.Fatalf("correctionMode = %q, want forced_narrow; recovery=%+v", got, recovery)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["contractViolationRetryNarrowed"] != true ||
		stringFromMap(required, "contractViolationRequiredOutputShape") != "managed_validation_evidence" {
		t.Fatalf("requiredOutput = %+v, want forced-narrow managed validation marker", required)
	}
	if required["mustProduceConcreteValidationEvidence"] != true || required["mustIncludeStateChangingNode"] != false {
		t.Fatalf("requiredOutput = %+v, want validation-only output contract", required)
	}
	if stringFromMap(required, "validationRoute") != runtimemodel.ValidationRouteManagedHarness ||
		required["mustUseManagedHarness"] != true {
		t.Fatalf("requiredOutput = %+v, want managed validation route", required)
	}
	if got := stringSliceFromValue(required["acceptedKinds"]); len(got) != 1 || got[0] != managedValidationSessionNodeKind {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, want managed session only", required["acceptedKinds"])
	}
	if !stringSliceContains(stringSliceFromValue(required["forbiddenOutputShapes"]), "plain_shell_validation") {
		t.Fatalf("requiredOutput.forbiddenOutputShapes = %+v, want plain shell forbidden", required["forbiddenOutputShapes"])
	}
	topRequired := mapFromValue(input["requiredOutput"])
	if topRequired["contractViolationRetryNarrowed"] != true ||
		topRequired["mustProduceConcreteValidationEvidence"] != true ||
		topRequired["mustIncludeStateChangingNode"] != false ||
		stringFromMap(topRequired, "validationRoute") != runtimemodel.ValidationRouteManagedHarness ||
		!stringSliceContains(stringSliceFromValue(topRequired["acceptedKinds"]), managedValidationSessionNodeKind) ||
		stringSliceContains(stringSliceFromValue(topRequired["acceptedKinds"]), "shell/command") {
		t.Fatalf("top-level requiredOutput = %+v, want same forced-narrow validation contract", topRequired)
	}
	stageRequired := mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"])
	if stageRequired["contractViolationRetryNarrowed"] != true ||
		stageRequired["mustProduceConcreteValidationEvidence"] != true ||
		stageRequired["mustIncludeStateChangingNode"] != false ||
		stringFromMap(stageRequired, "validationRoute") != runtimemodel.ValidationRouteManagedHarness ||
		!stringSliceContains(stringSliceFromValue(stageRequired["acceptedKinds"]), managedValidationSessionNodeKind) ||
		stringSliceContains(stringSliceFromValue(stageRequired["acceptedKinds"]), "shell/command") {
		t.Fatalf("stagePacket.requiredOutput = %+v, want same forced-narrow validation contract", stageRequired)
	}
	if boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("stateChangingOutputRequired = %v, want cleared for forced-narrow validation", input["stateChangingOutputRequired"])
	}
	if got := stringFromMap(input, "repairObjective"); got != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("repairObjective = %q, want %s", got, repairObjectiveConcreteValidationEvidence)
	}
	convergence := mapFromValue(input["convergenceContract"])
	if got := stringFromMap(convergence, "state"); got != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want %s", convergence, convergenceStateNeedValidation)
	}
	if err := validateConvergenceDispatchInput(input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput() error = %v; input=%+v", err, input)
	}
}

func TestContractViolationProcessControlImmediatelyNarrowsToManagedValidation(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-process-control-managed")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-process-control-managed", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/failed-validation"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence": true,
					"acceptedKinds": []string{
						"shell/command",
						"validation/managed_session",
					},
				},
			},
		},
		Output: map[string]any{
			"stage":                        "llm_execution_graph_plan",
			"error":                        "concrete validation evidence graph must use non-destructive bounded validation; shell process termination is not allowed",
			outputKeyContractViolationType: contractViolationValidationEvidenceRequired,
			"contractViolationRule":        contractViolationRuleValidationProcessControlForbidden,
			failureOutputKeyClass:          failureClassRuntime,
			failureOutputKeySignature:      "class=runtime_failed|stage=llm_execution_graph_plan|rule=validation_process_control_forbidden",
		},
	}
	attachContractViolationDiagnostics(run, failed, errors.New("concrete validation evidence graph must use non-destructive bounded validation; shell process termination is not allowed"))
	orch := &RuntimeOrchestrator{
		steps:  memory.NewStepRepository(),
		policy: NormalizeRuntimePolicy(RuntimePolicy{}),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failed, 1, common.ErrInvalidArgument)

	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if got := stringFromMap(recovery, "correctionMode"); got != contractViolationCorrectionModeForcedNarrow {
		t.Fatalf("correctionMode = %q, want forced_narrow for process-control violation; recovery=%+v", got, recovery)
	}
	required := mapFromValue(input["requiredOutput"])
	if stringFromMap(required, "validationRoute") != runtimemodel.ValidationRouteManagedHarness ||
		required["mustUseManagedHarness"] != true ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "shell/command") ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), managedValidationSessionNodeKind) {
		t.Fatalf("requiredOutput = %+v, want managed validation only", required)
	}
	if !stringSliceContains(stringSliceFromValue(required["forbiddenOutputShapes"]), "plain_shell_validation") {
		t.Fatalf("forbiddenOutputShapes = %+v, want plain shell forbidden", required["forbiddenOutputShapes"])
	}
}

func TestForcedNarrowManagedValidationDoesNotOverrideStateChangeAuthority(t *testing.T) {
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	input := map[string]any{
		convergenceAuthorityKey: cloneStepPayload(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])),
		gapBaselineKey:          cloneStepPayload(mapFromValue(run.ExecutionContext[gapBaselineKey])),
		stepInputKeyContractViolationRecovery: map[string]any{
			"correctionMode":      contractViolationCorrectionModeForcedNarrow,
			"correctionProfileId": "managed_validation_evidence",
			"correctionContract": map[string]any{
				"requiredOutputShape":   "managed_validation_evidence",
				"allowedOutputs":        []any{"executionGraph_with_managed_validation_session"},
				"forbiddenOutputs":      []any{"plain_shell_validation"},
				"validationRoute":       runtimemodel.ValidationRouteManagedHarness,
				"mustUseManagedHarness": true,
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteValidationEvidence": true,
				"acceptedKinds":                         []any{managedValidationSessionNodeKind},
			},
		},
		"requiredOutput": map[string]any{
			"mustProduceConcreteValidationEvidence": true,
			"acceptedKinds":                         []any{managedValidationSessionNodeKind},
		},
	}

	got, err := refreshCurrentAuthorityDerivedSnapshot(run, common.StepID("run-authority-focus/contract-retry"), input)
	if err != nil {
		t.Fatalf("refreshCurrentAuthorityDerivedSnapshot: %v; input=%+v", err, got)
	}
	contract := mapFromValue(got["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want state_change owner authority", contract)
	}
	required := mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		boolValue(required["mustProduceConcreteValidationEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want state-changing owner authority to override stale forced-narrow validation", required)
	}
}

func TestForcedNarrowManagedValidationRecoveryRewritesAllVisibleContracts(t *testing.T) {
	runID := common.RunID("run-forced-narrow-managed-visible")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-forced-narrow-managed-visible"}
	input := map[string]any{
		stepInputKeyContractViolationRecovery: map[string]any{
			"correctionMode":      contractViolationCorrectionModeForcedNarrow,
			"correctionProfileId": "managed_validation_evidence",
			"correctionContract": map[string]any{
				"requiredOutputShape":   "managed_validation_evidence",
				"allowedOutputs":        []any{"executionGraph_with_managed_validation_session"},
				"forbiddenOutputs":      []any{"plain_shell_validation", "top_level_shellCommands"},
				"validationRoute":       runtimemodel.ValidationRouteManagedHarness,
				"mustUseManagedHarness": true,
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteValidationEvidence": true,
				"acceptedKinds":                         []any{"shell/command", "mcp/tool_call", managedValidationSessionNodeKind},
				"validationEvidenceNodeExample":         map[string]any{"kind": "shell/command"},
				"coverageWorkItemIds":                   []any{"cwi-005"},
			},
		},
		"requiredOutput": map[string]any{
			"mustProduceConcreteValidationEvidence": true,
			"acceptedKinds":                         []any{"shell/command", "mcp/tool_call", managedValidationSessionNodeKind},
		},
		stepInputKeyStagePacket: map[string]any{
			"requiredOutput": map[string]any{
				"acceptedKinds": []any{"shell/command", managedValidationSessionNodeKind},
			},
		},
		currentActionContractKey: map[string]any{
			"requiredOutput": map[string]any{
				"acceptedKinds": []any{"shell/command", managedValidationSessionNodeKind},
			},
		},
	}

	input = attachStagePacketToStepInput(run, common.StepID("run-forced-narrow-managed-visible/step-001"), input)
	input = attachCurrentActionContractToInput(run, common.StepID("run-forced-narrow-managed-visible/step-001"), input)

	for name, required := range map[string]map[string]any{
		"top":     mapFromValue(input["requiredOutput"]),
		"pkg":     mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		"stage":   mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		"current": mapFromValue(mapFromValue(input[currentActionContractKey])["requiredOutput"]),
	} {
		if required["contractViolationRetryNarrowed"] != true ||
			stringFromMap(required, "validationRoute") != runtimemodel.ValidationRouteManagedHarness ||
			required["mustUseManagedHarness"] != true {
			t.Fatalf("%s requiredOutput = %+v, want managed forced-narrow markers", name, required)
		}
		if got := stringSliceFromValue(required["acceptedKinds"]); len(got) != 1 || got[0] != managedValidationSessionNodeKind {
			t.Fatalf("%s acceptedKinds = %+v, want managed session only", name, required["acceptedKinds"])
		}
		if _, ok := required["validationEvidenceNodeExample"]; ok {
			t.Fatalf("%s requiredOutput kept shell validation example: %+v", name, required)
		}
	}
}

func TestConvergenceDispatchRejectsSplitCurrentActionRequiredOutput(t *testing.T) {
	input := map[string]any{
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		gapBaselineKey: map[string]any{
			"nextAction": map[string]any{"kind": "state_change"},
		},
		"requiredOutput": map[string]any{
			"contractViolationRequiredOutputShape":  "bounded_validation_evidence",
			"mustProduceConcreteValidationEvidence": true,
			"mustIncludeStateChangingNode":          false,
			"acceptedKinds":                         []string{"shell/command"},
			"forbiddenKinds":                        []string{"patch/apply"},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []string{"patch/apply"},
			},
		},
		stepInputKeyStagePacket: map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []string{"patch/apply"},
			},
		},
	}
	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want split current action contract conflict")
	}
	if !errors.Is(err, common.ErrConflict) || !strings.Contains(err.Error(), failureClassRuntimeContractMisalign) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want runtime contract misalignment conflict", err)
	}
}

func TestConvergenceDispatchRejectsUnsatisfiableRequiredOutput(t *testing.T) {
	input := map[string]any{
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		gapBaselineKey: map[string]any{
			"nextAction": map[string]any{"kind": "state_change"},
		},
		"requiredOutput": map[string]any{
			"mustIncludeStateChangingNode": true,
			"stateChangingForbidden":       true,
			"acceptedKinds":                []string{"patch/apply"},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"stateChangingForbidden":       true,
				"acceptedKinds":                []string{"patch/apply"},
			},
		},
		stepInputKeyStagePacket: map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"stateChangingForbidden":       true,
				"acceptedKinds":                []string{"patch/apply"},
			},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want unsatisfiable contract conflict")
	}
	if !errors.Is(err, common.ErrConflict) ||
		!strings.Contains(err.Error(), failureClassRuntimeContractMisalign) ||
		!strings.Contains(err.Error(), "runtime_internal_contract_unsatisfiable") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want unsatisfiable runtime contract conflict", err)
	}
	diagnostics := mapSliceFromValue(input["authorityDiagnostics"])
	if len(diagnostics) == 0 || stringFromMap(diagnostics[len(diagnostics)-1], "code") != "required_output_unsatisfiable" {
		t.Fatalf("authorityDiagnostics = %+v, want required_output_unsatisfiable", diagnostics)
	}
}

func TestContractViolationRetryPreservesStateChangingExpectedContract(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-state-contract")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-state-contract", InitialStepKind: llmWorkspaceExecutionStepKind}
	coverageContext := coverageWorkItemContextFromItems("rev-contract-retry", []map[string]any{{
		"id":                    "cwi-001",
		"definitionRevision":    "rev-contract-retry",
		"source":                "goal",
		"sourceScope":           "goal_leaf",
		"sourceRequirementId":   "goal-req-001",
		"sourcePath":            "goal.leaves[0]",
		"targetText":            "must keep repairing the rejected state-changing output",
		"requiredEvidenceKinds": []string{"validation"},
		"missingEvidenceKinds":  []string{"validation"},
		"rejectedEvidenceRefs":  []string{"run/graph/verify"},
		"status":                "failed",
	}}, nil, nil)
	failed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			coverageWorkItemContextKey:    coverageContext,
			"repairActionPackage": map[string]any{
				coverageWorkItemContextKey: coverageContext,
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target.txt",
					"content":          "old content",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
				},
			},
		},
		Output: map[string]any{
			"stage":                        "llm_execution_graph_plan",
			"error":                        "runtime repair patch/apply file_write cannot replace existing target \"target.txt\" when current repair context is available; use a minimal unified_diff or exact replacement against the current snapshot",
			outputKeyContractViolationType: contractViolationPatchDispatchInvalid,
			failureOutputKeyClass:          failureClassRuntime,
			failureOutputKeySignature:      "class=runtime_failed|stage=llm_execution_graph_plan|error=patch_dispatch_invalid",
		},
	}
	attachContractViolationDiagnostics(run, failed, common.ErrInvalidArgument)
	orch := &RuntimeOrchestrator{
		steps:  memory.NewStepRepository(),
		policy: NormalizeRuntimePolicy(RuntimePolicy{}),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failed, 1, common.ErrInvalidArgument)

	if got := stringFromMap(input, "repairObjective"); got != repairObjectiveStateChangingOutput {
		t.Fatalf("repairObjective = %q, want %s; input=%+v", got, repairObjectiveStateChangingOutput, input)
	}
	if input["stateChangingOutputRequired"] != true {
		t.Fatalf("stateChangingOutputRequired = %v, want true", input["stateChangingOutputRequired"])
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want state-changing retry contract", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true {
		t.Fatalf("requiredOutput = %+v, want state-changing output", required)
	}
	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	expected := mapFromValue(recovery["expectedContract"])
	if expected["stateChangingOutputRequired"] != true {
		t.Fatalf("contractViolationRecovery.expectedContract = %+v, want original state-changing contract", expected)
	}
}

func TestContractViolationRetryInputPreservesGapBaselineFromFailedStep(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-gap-baseline")
	baseline := map[string]any{
		"schemaVersion":    gapBaselineSchema,
		"workspaceId":      "ws-contract-violation-gap-baseline",
		"runId":            string(runID),
		"baselineRevision": "gap-from-failed-step",
		"todoItems": []map[string]any{{
			"id":     "todo-001",
			"status": "open",
		}},
		"nextAction": map[string]any{"kind": "state_change"},
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-contract-violation-gap-baseline",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	failed := contractViolationFailedStep(runID, "failed-gap-baseline")
	failed.Input[gapBaselineKey] = baseline
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	orch := &RuntimeOrchestrator{
		steps:  memory.NewStepRepository(),
		policy: NormalizeRuntimePolicy(RuntimePolicy{}),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failed, 1, common.ErrInvalidArgument)

	got := mapFromValue(input[gapBaselineKey])
	if stringFromMap(got, "baselineRevision") != "gap-from-failed-step" {
		t.Fatalf("gapBaseline = %+v, want failed-step baseline", got)
	}
	if todos := mapSliceFromValue(got["todoItems"]); len(todos) != 1 || stringFromMap(todos[0], "id") != "todo-001" {
		t.Fatalf("gapBaseline.todoItems = %+v, want failed-step todo", got["todoItems"])
	}
}

func TestContractViolationRetryInputKeepsLatestRunGapBaseline(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-run-gap-baseline")
	runBaseline := map[string]any{
		"schemaVersion":    gapBaselineSchema,
		"workspaceId":      "ws-contract-violation-run-gap-baseline",
		"runId":            string(runID),
		"baselineRevision": "gap-from-run",
		"nextAction":       map[string]any{"kind": "state_change"},
	}
	stepBaseline := map[string]any{
		"schemaVersion":    gapBaselineSchema,
		"workspaceId":      "ws-contract-violation-run-gap-baseline",
		"runId":            string(runID),
		"baselineRevision": "gap-from-failed-step",
		"nextAction":       map[string]any{"kind": "state_change"},
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-contract-violation-run-gap-baseline",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			gapBaselineKey: runBaseline,
		},
	}
	failed := contractViolationFailedStep(runID, "failed-run-gap-baseline")
	failed.Input[gapBaselineKey] = stepBaseline
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	orch := &RuntimeOrchestrator{
		steps:  memory.NewStepRepository(),
		policy: NormalizeRuntimePolicy(RuntimePolicy{}),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failed, 1, common.ErrInvalidArgument)

	got := mapFromValue(input[gapBaselineKey])
	if stringFromMap(got, "baselineRevision") != "gap-from-run" {
		t.Fatalf("gapBaseline = %+v, want latest run baseline", got)
	}
}

func TestContractViolationEventsAreRegisteredAsStateNotifications(t *testing.T) {
	contracts := orchestratorevents.Contracts()
	for _, topic := range []string{
		orchestratorevents.TopicOrchestratorContractViolationRecorded,
		orchestratorevents.TopicOrchestratorContractViolationRetryRequested,
		orchestratorevents.TopicOrchestratorContractViolationEscalated,
	} {
		found := false
		for _, contract := range contracts {
			if contract.Topic == topic {
				found = true
				if contract.CommandClass != eventcontract.CommandClassNotify || contract.LanePolicy != eventcontract.LanePolicyState {
					t.Fatalf("contract for %s = %+v, want notify state", topic, contract)
				}
			}
		}
		if !found {
			t.Fatalf("missing event contract for %s", topic)
		}
	}
}

func TestContractViolationRecoveryPublishesRecordedRetryAndDispatchEvents(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-events")
	workspaceID := common.WorkspaceID("ws-contract-violation-events")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		State:           common.RunStateActive,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	failed := contractViolationFailedStep(runID, "failed-events")
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := stepRepo.Create(ctx, failed); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{}, runRepo, stepRepo, workspaceRepo, memory.NewDecisionTicketRepository(), memory.NewApprovalRepository(), hub)
	orch.EnableEventDrivenDispatch()

	if err := orch.eventPublisher().publishContractViolationRecorded(ctx, run, failed); err != nil {
		t.Fatalf("publish recorded: %v", err)
	}
	if err := orch.dispatchRuntimeRepairFollowUp(ctx, run, failed, common.ErrInvalidArgument); err != nil {
		t.Fatalf("dispatchRuntimeRepairFollowUp: %v", err)
	}

	recorded := publishedTopic(hub.Published(), orchestratorevents.TopicOrchestratorContractViolationRecorded)
	if recorded == nil {
		t.Fatalf("published events = %+v, want contract violation recorded", hub.Published())
	}
	assertContractViolationEventScope(t, *recorded, workspaceID, runID, failed.ID)
	if got := stringFromMap(recorded.Payload, "violatedRule"); got != contractViolationRuleStateChangingRequiredMissing {
		t.Fatalf("recorded payload rule = %q, want %s", got, contractViolationRuleStateChangingRequiredMissing)
	}

	retry := publishedTopic(hub.Published(), orchestratorevents.TopicOrchestratorContractViolationRetryRequested)
	if retry == nil {
		t.Fatalf("published events = %+v, want contract violation retry requested", hub.Published())
	}
	assertContractViolationEventScope(t, *retry, workspaceID, runID, failed.ID)
	if retry.Payload["retryStepId"] == "" {
		t.Fatalf("retry payload = %+v, want retryStepId", retry.Payload)
	}
	if got := intFromAny(retry.Payload["attempt"]); got != 1 {
		t.Fatalf("retry attempt = %d, want 1", got)
	}
	if got := stringFromMap(retry.Payload, "correctionProfileId"); got != "state_changing_required_output" {
		t.Fatalf("retry correctionProfileId = %q, want state_changing_required_output; payload=%+v", got, retry.Payload)
	}
	if got := stringFromMap(retry.Payload, "correctionMode"); got != contractViolationCorrectionModeTargeted {
		t.Fatalf("retry correctionMode = %q, want targeted; payload=%+v", got, retry.Payload)
	}

	dispatch := publishedTopic(hub.Published(), orchestratorevents.TopicOrchestratorDispatchStepRequested)
	if dispatch == nil {
		t.Fatalf("published events = %+v, want dispatch requested", hub.Published())
	}
	if dispatch.WorkspaceID != string(workspaceID) || dispatch.RunID != string(runID) || dispatch.StepID == "" {
		t.Fatalf("dispatch event = %+v, want scoped workspace/run/retry step", *dispatch)
	}
}

func TestContractViolationEscalatedEventPayloadIsScoped(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-escalated-event")
	workspaceID := common.WorkspaceID("ws-contract-violation-escalated-event")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID}
	failed := contractViolationFailedStep(runID, "failed-escalated")
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	hub := events.NewMemoryHub()
	publisher := newEventSubsystem(hub, nil, nil)
	violation := contractViolationFromFailedOutput(failed)
	context := map[string]any{"attempts": 2, "runAttempts": 3}

	if err := publisher.publishContractViolationEscalated(ctx, run, failed, "ticket-contract-violation", violation, context); err != nil {
		t.Fatalf("publish escalated: %v", err)
	}

	escalated := publishedTopic(hub.Published(), orchestratorevents.TopicOrchestratorContractViolationEscalated)
	if escalated == nil {
		t.Fatalf("published events = %+v, want contract violation escalated", hub.Published())
	}
	assertContractViolationEventScope(t, *escalated, workspaceID, runID, failed.ID)
	if got := stringFromMap(escalated.Payload, "ticketId"); got != "ticket-contract-violation" {
		t.Fatalf("escalated payload = %+v, want ticket id", escalated.Payload)
	}
	if got := intFromAny(mapFromValue(escalated.Payload["context"])["attempts"]); got != 2 {
		t.Fatalf("escalated context = %+v, want attempts", escalated.Payload["context"])
	}
}

func TestEventSubsystemPublishesNotificationsThroughAsyncHub(t *testing.T) {
	ctx := context.Background()
	hub := &recordingOrchestratorAsyncHub{}
	publisher := newEventSubsystem(hub, nil, nil)
	run := &runmodel.Run{
		ID:          "run-event-subsystem-async",
		WorkspaceID: "ws-event-subsystem-async",
		State:       common.RunStateActive,
	}
	failed := contractViolationFailedStep(run.ID, "failed-event-subsystem-async")
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))

	if err := publisher.publishRunCreated(ctx, run); err != nil {
		t.Fatalf("publish run created: %v", err)
	}
	if err := publisher.publishContractViolationRecorded(ctx, run, failed); err != nil {
		t.Fatalf("publish contract violation recorded: %v", err)
	}

	if hub.syncPublishes != 0 || hub.asyncPublishes != 2 {
		t.Fatalf("publish counts = sync:%d async:%d, want async event subsystem notifications", hub.syncPublishes, hub.asyncPublishes)
	}
	if len(hub.published) != 2 ||
		hub.published[0].Topic == "" ||
		hub.published[1].Topic != orchestratorevents.TopicOrchestratorContractViolationRecorded {
		t.Fatalf("published = %+v, want run created and contract violation notifications", hub.published)
	}
}

func TestContractViolationRecoveryScrubsConflictingDiagnosticOutputShapeAdvice(t *testing.T) {
	runID := common.RunID("run-contract-violation-diagnostic-scrub")
	failed := contractViolationFailedStep(runID, "failed-diagnostic-scrub")
	failed.Input = map[string]any{
		stepInputKeyRuntimeStage: runtimeStageVerification,
		"repairObjective":        repairObjectiveConcreteValidationEvidence,
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteValidationEvidence": true,
			},
		},
		"requiredOutput": map[string]any{
			"mustProduceConcreteValidationEvidence": true,
		},
	}
	failed.Output = map[string]any{
		"stage":                        "runtime_contract_failed",
		"error":                        "return top-level `shellCommands` with validationEvidence",
		outputKeyContractViolationType: contractViolationValidationEvidenceRequired,
		failureOutputKeyClass:          failureClassRuntime,
		failureOutputKeySignature:      "class=runtime_failed|stage=runtime_contract_failed|rule=concrete_validation_contract_missing",
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-diagnostic-scrub", InitialStepKind: llmWorkspaceExecutionStepKind}
	attachContractViolationDiagnostics(run, failed, errors.New("return top-level `shellCommands` with validationEvidence"))
	input := cloneStepPayload(failed.Input)

	applyContractViolationRecoveryProjection(input, failed, 1, 2, 0)

	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if !boolValue(recovery["diagnosticOnly"]) || boolValue(recovery["actionAdviceAuthoritative"]) {
		t.Fatalf("contractViolationRecovery = %+v, want diagnostic-only non-authoritative recovery", recovery)
	}
	actual := mapFromValue(recovery["actualOutputSummary"])
	if strings.Contains(strings.ToLower(stableCoverageJSON(actual)), "top-level shellcommands") {
		t.Fatalf("actualOutputSummary = %+v, want conflicting output-shape advice scrubbed", actual)
	}
	hint := stringFromMap(recovery, "retryHint")
	if !strings.Contains(hint, "Do not return top-level shellCommands") ||
		!strings.Contains(hint, "input.validationEvidence.mode") {
		t.Fatalf("retryHint = %q, want requiredOutput-derived validation instruction", hint)
	}
}

func TestValidateConvergenceDispatchRejectsOutputShapeAdviceConflict(t *testing.T) {
	required := concreteValidationEvidenceRequiredOutputWithScratch(
		map[string]any{validationScratchDirKey: "/tmp/workorch-validation-test/output-shape-conflict"},
		nil,
		nil,
	)
	input := map[string]any{
		"convergenceContract": map[string]any{
			"state": convergenceStateNeedValidation,
		},
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": "validate"},
		},
		"requiredOutput": required,
		"repairActionPackage": map[string]any{
			"requiredOutput": required,
		},
		stepInputKeyStagePacket: map[string]any{
			"requiredOutput": required,
		},
		"repairGuidance": []string{"return top-level `shellCommands` for this validation"},
	}

	err := validateConvergenceDispatchInput(input)

	if err == nil || !strings.Contains(err.Error(), "output-shape advice conflicts") {
		t.Fatalf("validateConvergenceDispatchInput err = %v, want output-shape conflict", err)
	}
	diagnostics := mapSliceFromValue(input["authorityDiagnostics"])
	if len(diagnostics) == 0 || stringFromMap(diagnostics[0], "code") != "output_shape_conflict" {
		t.Fatalf("authorityDiagnostics = %+v, want output_shape_conflict", diagnostics)
	}
}

func TestScrubConvergenceDispatchDiagnosticAdviceRemovesConflictingGuidance(t *testing.T) {
	input := map[string]any{
		"requiredOutput": map[string]any{
			"mustProduceConcreteValidationEvidence": true,
		},
		"repairGuidance": []string{
			"return top-level `shellCommands` for validation",
			"Use the current requiredOutput as authority.",
		},
	}

	got := scrubConvergenceDispatchDiagnosticAdvice(input)

	guidance := strings.Join(stringSliceFromValue(got["repairGuidance"]), "\n")
	if strings.Contains(strings.ToLower(guidance), "top-level shellcommands") {
		t.Fatalf("repairGuidance = %q, want conflicting stale guidance removed", guidance)
	}
	if !strings.Contains(guidance, "current requiredOutput") {
		t.Fatalf("repairGuidance = %q, want non-conflicting guidance preserved", guidance)
	}
	diagnostics := mapSliceFromValue(got["authorityDiagnostics"])
	if len(diagnostics) == 0 || stringFromMap(diagnostics[0], "code") != "stale_diagnostic_action_advice_removed" {
		t.Fatalf("authorityDiagnostics = %+v, want stale advice diagnostic", diagnostics)
	}
}

func TestContractViolationEscalationCorrectionContextUsesLastRetryRecovery(t *testing.T) {
	runID := common.RunID("run-contract-violation-escalation-correction")
	failed := contractViolationFailedStep(runID, "failed-escalation-correction")
	failed.Input[stepInputKeyContractViolationRecovery] = map[string]any{
		"correctionProfileId": "state_changing_required_output",
		"correctionMode":      contractViolationCorrectionModeForcedNarrow,
		"targetScope":         "failureTargets=target.txt",
		"correctionContract": map[string]any{
			"requiredOutputShape": "state_changing_output",
			"forcedNarrow":        true,
		},
	}
	violation := map[string]any{
		"correctionProfileId": "older_profile",
		"correctionMode":      contractViolationCorrectionModeTargeted,
	}

	context := contractViolationEscalationCorrectionContext(failed, violation)

	if stringFromMap(context, "correctionProfileId") != "state_changing_required_output" ||
		stringFromMap(context, "correctionMode") != contractViolationCorrectionModeForcedNarrow ||
		stringFromMap(context, "targetScope") != "failureTargets=target.txt" {
		t.Fatalf("correction context = %+v, want last retry recovery correction fields", context)
	}
	contract := mapFromValue(context["correctionContract"])
	if stringFromMap(contract, "requiredOutputShape") != "state_changing_output" || contract["forcedNarrow"] != true {
		t.Fatalf("correctionContract = %+v, want forced-narrow state-changing contract", contract)
	}
}

func publishedTopic(items []events.Envelope, topic string) *events.Envelope {
	for idx := range items {
		if items[idx].Topic == topic {
			return &items[idx]
		}
	}
	return nil
}

func assertContractViolationEventScope(t *testing.T, envelope events.Envelope, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID) {
	t.Helper()
	if envelope.WorkspaceID != string(workspaceID) || envelope.RunID != string(runID) || envelope.StepID != string(stepID) {
		t.Fatalf("event scope = %+v, want workspace=%s run=%s step=%s", envelope, workspaceID, runID, stepID)
	}
	if got := stringFromMap(envelope.Payload, "workspaceId"); got != string(workspaceID) {
		t.Fatalf("payload workspaceId = %q, want %s", got, workspaceID)
	}
	if got := stringFromMap(envelope.Payload, "runId"); got != string(runID) {
		t.Fatalf("payload runId = %q, want %s", got, runID)
	}
	if got := stringFromMap(envelope.Payload, "sourceStepId"); got != string(stepID) {
		t.Fatalf("payload sourceStepId = %q, want %s", got, stepID)
	}
	if len(mapFromValue(envelope.Payload["contractViolation"])) == 0 {
		t.Fatalf("payload = %+v, want nested contractViolation", envelope.Payload)
	}
}

func contractViolationFailedStep(runID common.RunID, label string) *runmodel.Step {
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/" + label),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
				},
			},
		},
		Output: map[string]any{
			"stage":                        "runtime_contract_failed",
			"error":                        "state-changing llm repair step returned no state-changing output",
			outputKeyContractViolationType: contractViolationRepairNoStateChangingOutput,
			failureOutputKeyClass:          failureClassRuntime,
			failureOutputKeySignature:      "class=runtime_failed|stage=runtime_contract_failed|rule=state_changing_required_missing",
		},
	}
	return step
}

func coverageAssessmentIncompleteFailedStep(runID common.RunID, label string) *runmodel.Step {
	return &runmodel.Step{
		ID:    common.StepID(string(runID) + "/" + label),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyAssessmentOnly: true,
			stepInputKeyRuntimeStage:   runtimeStageVerification,
			"runtimeStageMode":         "strict_convergence",
			convergenceAuthorityKey: map[string]any{
				"nextAction": map[string]any{"kind": "assessment"},
			},
		},
		Output: map[string]any{
			"stage":                    "llm_dod_assessment",
			"error":                    "invalid argument: dodAssessment is not satisfied",
			failureOutputKeyClass:      failureClassDoDCoverageMissing,
			failureOutputKeySignature:  "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{"satisfied": false},
		},
	}
}

type recordingOrchestratorAsyncHub struct {
	syncPublishes  int
	asyncPublishes int
	published      []events.Envelope
}

func (h *recordingOrchestratorAsyncHub) Publish(_ context.Context, envelope events.Envelope) error {
	h.syncPublishes++
	h.published = append(h.published, envelope)
	return nil
}

func (h *recordingOrchestratorAsyncHub) PublishAsync(_ context.Context, envelope events.Envelope) error {
	h.asyncPublishes++
	h.published = append(h.published, envelope)
	return nil
}

func (h *recordingOrchestratorAsyncHub) Subscribe(string, events.Handler) func() {
	return func() {}
}
