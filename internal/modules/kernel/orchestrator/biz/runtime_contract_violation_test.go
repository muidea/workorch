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

func TestRuntimeRepairInputIncludesContractViolationRecovery(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-input")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-input", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-001")
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	orch := &RuntimeOrchestrator{
		steps:  memory.NewStepRepository(),
		policy: NormalizeRuntimePolicy(RuntimePolicy{}),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failed, 1, common.ErrInvalidArgument)

	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if stringFromMap(recovery, "violatedRule") != contractViolationRuleStateChangingRequiredMissing {
		t.Fatalf("contractViolationRecovery = %+v, want state-changing rule", recovery)
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
	if !strings.Contains(strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n"), "contractViolationRecovery.expectedContract") {
		t.Fatalf("repairGuidance = %+v, want contract violation guidance", input["repairGuidance"])
	}
	if !strings.Contains(strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n"), "contractViolationRecovery.correctionContract") {
		t.Fatalf("repairGuidance = %+v, want correction contract guidance", input["repairGuidance"])
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
	failed.Output["error"] = "failed assessment claimed current target snapshot is missing and must read the file before patching"
	failed.Output["dodAssessment"] = map[string]any{
		"satisfied": false,
		"summary":   "cannot proceed because the target snapshot is not provided",
	}

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

func TestContractViolationClassifiesAvailableSnapshotIgnoredFromNestedChineseOutput(t *testing.T) {
	runID := common.RunID("run-contract-available-snapshot-ignored-cn")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-available-snapshot-ignored-cn", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-available-snapshot-cn")
	delete(mapFromValue(failed.Input["repairActionPackage"]), "targetSnapshots")
	failed.Input["failureTargetRepairContext"] = []map[string]any{{
		"targetRef":        "target.txt",
		"content":          "current target content\n",
		"contentHash":      "sha256-current-target",
		"contentComplete":  true,
		"safeForFileWrite": true,
		"exists":           true,
	}}
	failed.Output["summary"] = "缺少可安全生成最小补丁所需的当前目标快照文本，无法继续生成 state-changing 输出"

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

func TestContractViolationDoesNotClassifyMissingSnapshotWithoutAvailableSnapshot(t *testing.T) {
	runID := common.RunID("run-contract-missing-snapshot-no-context")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-missing-snapshot-no-context", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-no-available-snapshot")
	failed.Output["error"] = "failed assessment claimed current target snapshot is missing and must read the file before patching"

	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))

	recovery := contractViolationFromFailedOutput(failed)
	if got := stringFromMap(recovery, "violatedRule"); got == contractViolationRuleAvailableSnapshotIgnored {
		t.Fatalf("violatedRule = %q, want no available snapshot classification without supplied snapshot; recovery=%+v", got, recovery)
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
		"nextAction": map[string]any{"kind": "schedule_step"},
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
		"nextAction":       map[string]any{"kind": "schedule_step"},
	}
	stepBaseline := map[string]any{
		"schemaVersion":    gapBaselineSchema,
		"workspaceId":      "ws-contract-violation-run-gap-baseline",
		"runId":            string(runID),
		"baselineRevision": "gap-from-failed-step",
		"nextAction":       map[string]any{"kind": "schedule_step"},
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
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	failed := contractViolationFailedStep(runID, "failed-events")
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
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
		State:       common.RunStateRunning,
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
