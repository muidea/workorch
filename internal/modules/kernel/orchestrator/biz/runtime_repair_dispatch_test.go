package orchestrator

import (
	"context"
	"errors"
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type repairDispatchSpecBuilderStub struct{}

func (repairDispatchSpecBuilderStub) runtimeRepairFollowUpStepInput(_ context.Context, _ *runmodel.Run, _ *runmodel.Step, attempt int, execErr error) map[string]any {
	return map[string]any{
		stepInputKeyRuntimeRepairFollowUp: true,
		stepInputKeyRuntimeRepairAttempt:  attempt,
		"failureReason":                   errorString(execErr),
	}
}

type repairDispatchStaleForcedNarrowBuilder struct{}

func (repairDispatchStaleForcedNarrowBuilder) runtimeRepairFollowUpStepInput(_ context.Context, _ *runmodel.Run, _ *runmodel.Step, attempt int, execErr error) map[string]any {
	return map[string]any{
		stepInputKeyRuntimeRepairFollowUp: true,
		stepInputKeyRuntimeRepairAttempt:  attempt,
		"failureReason":                   errorString(execErr),
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
}

type repairDispatchValidationAuthorityBuilder struct{}

func (repairDispatchValidationAuthorityBuilder) runtimeRepairFollowUpStepInput(_ context.Context, run *runmodel.Run, _ *runmodel.Step, attempt int, execErr error) map[string]any {
	workspaceID := "ws-validation-repair-authority"
	runID := "run-validation-repair-authority"
	if run != nil {
		if run.WorkspaceID != "" {
			workspaceID = string(run.WorkspaceID)
		}
		if run.ID != "" {
			runID = string(run.ID)
		}
	}
	required := map[string]any{
		"mustProduceConcreteValidationEvidence": true,
		"mustDeclareValidationEvidenceMode":     true,
		"acceptedKinds":                         []any{"shell/command", managedValidationSessionNodeKind},
		"forbiddenKinds":                        []any{"patch/apply", "git/write", "file/write"},
	}
	return map[string]any{
		stepInputKeyRuntimeRepairFollowUp: true,
		stepInputKeyRuntimeRepairAttempt:  attempt,
		"failureReason":                   errorString(execErr),
		"repairObjective":                 repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired":       true,
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"workspaceId":   workspaceID,
			"runId":         runID,
			"currentFocus": map[string]any{
				"id":                   "cwi-current",
				"missingEvidenceKinds": []string{"validation"},
				"status":               "in_progress",
			},
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		"convergenceContract": map[string]any{
			"state":           convergenceStateNeedValidation,
			"runtimeStage":    runtimeStageVerification,
			"repairObjective": repairObjectiveConcreteValidationEvidence,
		},
		"repairActionPackage": map[string]any{
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"requiredOutput":            cloneStepPayload(required),
		},
		"requiredOutput": cloneStepPayload(required),
	}
}

type repairDispatchValidationAuthorityWithStateChangeGapBuilder struct{}

func (repairDispatchValidationAuthorityWithStateChangeGapBuilder) runtimeRepairFollowUpStepInput(ctx context.Context, run *runmodel.Run, step *runmodel.Step, attempt int, execErr error) map[string]any {
	input := repairDispatchValidationAuthorityBuilder{}.runtimeRepairFollowUpStepInput(ctx, run, step, attempt, execErr)
	focus := map[string]any{
		"id":                    "cwi-current",
		"missingEvidenceKinds":  []string{runtimemodel.EvidenceKindStateChange},
		"requiredEvidenceKinds": []string{runtimemodel.EvidenceKindStateChange, runtimemodel.EvidenceKindValidation},
		"status":                "failed",
		"sourceRequirementId":   "source_change",
		"sourceScope":           "deliverable",
		"targetRefs":            []string{"cwi-current", "source_change"},
		"structuredTargetRefs":  []string{"cwi-current", "source_change"},
	}
	if authority := mapFromValue(input[convergenceAuthorityKey]); len(authority) > 0 {
		authority["currentFocus"] = cloneStepPayload(focus)
		input[convergenceAuthorityKey] = authority
	}
	input[coverageWorkItemContextKey] = map[string]any{
		"currentFocus": cloneStepPayload(focus),
	}
	input[currentActionContractKey] = map[string]any{
		"currentFocus": cloneStepPayload(focus),
		"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeValidate},
	}
	return input
}

type repairDispatchStaleValidationMirrorBuilder struct{}

func (repairDispatchStaleValidationMirrorBuilder) runtimeRepairFollowUpStepInput(_ context.Context, _ *runmodel.Run, _ *runmodel.Step, attempt int, execErr error) map[string]any {
	required := concreteValidationEvidenceRequiredOutputWithScratch(map[string]any{
		"coverageWorkItemIds":  []string{"cwi-current"},
		"structuredTargetRefs": []string{"cwi-current", "source_change"},
	}, nil, nil)
	return map[string]any{
		stepInputKeyRuntimeRepairFollowUp:                true,
		stepInputKeyRuntimeRepairAttempt:                 attempt,
		"failureReason":                                  errorString(execErr),
		"currentAuthorityRequiredOutputMirrorsCanonical": true,
		"stateChangingOutputRequired":                    true,
		"repairObjective":                                repairObjectiveStateChangingOutput,
		"requiredOutput":                                 cloneStepPayload(required),
		"repairActionPackage":                            map[string]any{"requiredOutput": cloneStepPayload(required)},
		"convergenceContract":                            map[string]any{"state": convergenceStateNeedStateChange, "stateChangingOutputRequired": true},
		stepInputKeyStagePacket: map[string]any{
			"requiredOutput": cloneStepPayload(required),
			"nextAction": map[string]any{
				"type":           runtimemodel.NextActionTypeStateChange,
				"requiredOutput": cloneStepPayload(required),
			},
		},
	}
}

type repairBlockedRecoveryExecutorStub struct {
	called runtimeRecoveryAction
	err    error
}

func (s *repairBlockedRecoveryExecutorStub) blockOnProviderRetryExhaustion(context.Context, *runmodel.Run, *runmodel.Step, error) error {
	s.called = runtimeRecoveryActionProviderRetryExhausted
	return s.err
}

func (s *repairBlockedRecoveryExecutorStub) blockOnRuntimeRepairExhaustion(context.Context, *runmodel.Run, *runmodel.Step, error) error {
	s.called = runtimeRecoveryActionRuntimeRepairExhausted
	return s.err
}

func (s *repairBlockedRecoveryExecutorStub) blockOnContractViolationRecoveryExhaustion(context.Context, *runmodel.Run, *runmodel.Step, error) error {
	s.called = runtimeRecoveryActionContractViolationExhausted
	return s.err
}

func (s *repairBlockedRecoveryExecutorStub) blockOnDuplicateRuntimeFailure(context.Context, *runmodel.Run, *runmodel.Step, error) error {
	s.called = runtimeRecoveryActionDuplicateFailure
	return s.err
}

func (s *repairBlockedRecoveryExecutorStub) blockOnContextIntegrityGate(context.Context, *runmodel.Run, *runmodel.Step, error) error {
	s.called = runtimeRecoveryActionContextIntegrityBlocked
	return s.err
}

func TestRepairSubsystemRecoveryDispatchSpecPlansProviderRetry(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-repair-dispatch-provider")
	rootStepID := common.StepID(runID + "/planning-001")
	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    providerRetryStepID(runID, rootStepID, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryRootStep: string(rootStepID),
		},
		Output: map[string]any{"stage": "llm_provider_timeout"},
	}); err != nil {
		t.Fatalf("seed provider retry: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    rootStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}

	spec, ok, err := newRepairSubsystem(stepRepo, nil).recoveryDispatchSpec(ctx, &runmodel.Run{ID: runID}, failedStep, errors.New("timeout"), runtimeRecoveryActionProviderRetry, maxProviderRetryAttempts, repairDispatchSpecBuilderStub{}, false)

	if err != nil || !ok {
		t.Fatalf("recoveryDispatchSpec ok/err = %v/%v, want provider retry spec", ok, err)
	}
	if spec.ID != providerRetryStepID(runID, rootStepID, 2) || spec.Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("spec = %+v, want provider retry attempt 2", spec)
	}
	if spec.Input[stepInputKeyProviderRetryAttempt] != 2 {
		t.Fatalf("provider retry attempt = %#v, want 2", spec.Input[stepInputKeyProviderRetryAttempt])
	}
}

func TestRepairSubsystemRecoveryDispatchSpecFinalizesProviderRetryFromCurrentAuthority(t *testing.T) {
	ctx := context.Background()
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	run.InitialStepKind = llmcommon.StepKindWorkspaceExecution
	stateChangeFocusForRepairDispatchTest(run)
	stepRepo := memory.NewStepRepository()
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(run.ID) + "/planning-001"),
		RunID: run.ID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: repairDispatchValidationAuthorityBuilder{}.runtimeRepairFollowUpStepInput(ctx, run, nil, 1, nil),
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}

	spec, ok, err := newRepairSubsystem(stepRepo, nil).recoveryDispatchSpec(ctx, run, failedStep, errors.New("timeout"), runtimeRecoveryActionProviderRetry, maxProviderRetryAttempts, repairDispatchSpecBuilderStub{}, false)

	if err != nil || !ok {
		t.Fatalf("recoveryDispatchSpec ok/err = %v/%v, want provider retry spec", ok, err)
	}
	assertStateChangingAuthorityDispatchForTest(t, spec.Input)
	if spec.Input["authoritySnapshotId"] == "" {
		t.Fatalf("authoritySnapshotId missing in provider retry input: %+v", spec.Input)
	}
	if err := validateConvergenceDispatchInput(spec.Input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, spec.Input)
	}
}

func TestRepairSubsystemRecoveryDispatchSpecPlansRuntimeRepairWithExactScope(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-repair-dispatch-runtime")
	stepRepo := memory.NewStepRepository()
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/verify"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime|target=exact",
		},
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    runtimeRepairFollowUpStepID(runID, failedStep, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfFailureClass:  failureClassRuntime,
			stepInputKeyRepairOfFailureSig:    "class=runtime|target=exact",
		},
		Output: map[string]any{"status": "failed"},
	}); err != nil {
		t.Fatalf("seed runtime repair: %v", err)
	}

	spec, ok, err := newRepairSubsystem(stepRepo, nil).recoveryDispatchSpec(ctx, &runmodel.Run{ID: runID, InitialStepKind: llmcommon.StepKindWorkspaceExecution}, failedStep, errors.New("failed"), runtimeRecoveryActionRuntimeRepair, maxProviderRetryAttempts, repairDispatchSpecBuilderStub{}, false)

	if err != nil || !ok {
		t.Fatalf("recoveryDispatchSpec ok/err = %v/%v, want runtime repair spec", ok, err)
	}
	if spec.ID != runtimeRepairFollowUpStepID(runID, failedStep, 2) || spec.Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("spec = %+v, want runtime repair attempt 2", spec)
	}
	if spec.Input[stepInputKeyRuntimeRepairAttempt] != 2 {
		t.Fatalf("runtime repair attempt = %#v, want 2", spec.Input[stepInputKeyRuntimeRepairAttempt])
	}
}

func TestResolvedDecisionRuntimeRepairSpecFinalizesCurrentAuthority(t *testing.T) {
	ctx := context.Background()
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	run.InitialStepKind = llmcommon.StepKindWorkspaceExecution
	stateChangeFocusForRepairDispatchTest(run)
	stepRepo := memory.NewStepRepository()
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(run.ID) + "/failed"),
		RunID: run.ID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass: failureClassRuntime,
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	specs, ok, err := runtimeRepairSpecsAfterResolvedDecision(ctx, stepRepo, newRepairSubsystem(stepRepo, nil), repairDispatchValidationAuthorityBuilder{}, run, failedStep.ID)

	if err != nil || !ok || len(specs) != 1 {
		t.Fatalf("runtimeRepairSpecsAfterResolvedDecision specs/ok/err = %d/%v/%v, want one spec", len(specs), ok, err)
	}
	assertStateChangingAuthorityDispatchForTest(t, specs[0].Input)
	if specs[0].Input["decisionAuthorizedRetry"] != true {
		t.Fatalf("decisionAuthorizedRetry = %#v, want true", specs[0].Input["decisionAuthorizedRetry"])
	}
	if err := validateConvergenceDispatchInput(specs[0].Input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, specs[0].Input)
	}
}

func TestResolvedDecisionProviderRetrySpecFinalizesCurrentAuthority(t *testing.T) {
	ctx := context.Background()
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	run.InitialStepKind = llmcommon.StepKindWorkspaceExecution
	stateChangeFocusForRepairDispatchTest(run)
	stepRepo := memory.NewStepRepository()
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(run.ID) + "/provider-failed"),
		RunID: run.ID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: repairDispatchValidationAuthorityBuilder{}.runtimeRepairFollowUpStepInput(ctx, run, nil, 1, nil),
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed provider failed step: %v", err)
	}

	specs, ok, err := providerRetrySpecsAfterResolvedDecision(ctx, stepRepo, newRepairSubsystem(stepRepo, nil), RuntimePolicy{}, run, failedStep.ID)

	if err != nil || !ok || len(specs) != 1 {
		t.Fatalf("providerRetrySpecsAfterResolvedDecision specs/ok/err = %d/%v/%v, want one spec", len(specs), ok, err)
	}
	assertStateChangingAuthorityDispatchForTest(t, specs[0].Input)
	if specs[0].Input["decisionAuthorizedRetry"] != true {
		t.Fatalf("decisionAuthorizedRetry = %#v, want true", specs[0].Input["decisionAuthorizedRetry"])
	}
	if err := validateConvergenceDispatchInput(specs[0].Input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, specs[0].Input)
	}
}

func TestResolvedDecisionKnownReadableRepairSpecFinalizesCurrentAuthority(t *testing.T) {
	ctx := context.Background()
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	run.InitialStepKind = llmcommon.StepKindWorkspaceExecution
	stateChangeFocusForRepairDispatchTest(run)
	stepRepo := memory.NewStepRepository()
	step := &runmodel.Step{
		ID:    common.StepID(string(run.ID) + "/decision"),
		RunID: run.ID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateSucceeded,
		Input: repairDispatchValidationAuthorityBuilder{}.runtimeRepairFollowUpStepInput(ctx, run, nil, 1, nil),
		Output: map[string]any{
			stepOutputKeyBlockDecisionReason:   "Need to inspect known-resource.txt",
			stepOutputKeyBlockDecisionQuestion: "Please provide known-resource.txt",
			stepOutputKeyBlockDecisionKind:     decisioncommon.ResolutionTypeProvideMissingInfo,
			stepOutputKeyBlockDecisionContext: map[string]any{
				"workspaceFiles":               []any{"known-resource.txt"},
				decisionContextKeyQuestion:     "Please provide known-resource.txt",
				decisionContextKeyReason:       "Need to inspect known-resource.txt",
				decisionContextKeyResultPolicy: decisionContextResultPolicyNone,
			},
		},
	}
	if err := stepRepo.Create(ctx, step); err != nil {
		t.Fatalf("seed decision step: %v", err)
	}

	specs, ok, err := knownReadableDecisionRepairSpecs(ctx, stepRepo, nil, newRepairSubsystem(stepRepo, nil), repairDispatchValidationAuthorityBuilder{}, run, step.ID)

	if err != nil || !ok || len(specs) != 1 {
		t.Fatalf("knownReadableDecisionRepairSpecs specs/ok/err = %d/%v/%v, want one spec", len(specs), ok, err)
	}
	assertStateChangingAuthorityDispatchForTest(t, specs[0].Input)
	if err := validateConvergenceDispatchInput(specs[0].Input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, specs[0].Input)
	}
}

func TestResolvedDecisionContextIntegritySpecFinalizesCurrentAuthority(t *testing.T) {
	ctx := context.Background()
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	run.InitialStepKind = llmcommon.StepKindWorkspaceExecution
	stateChangeFocusForRepairDispatchTest(run)
	stepRepo := memory.NewStepRepository()
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(run.ID) + "/context-gate"),
		RunID: run.ID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: repairDispatchValidationAuthorityBuilder{}.runtimeRepairFollowUpStepInput(ctx, run, nil, 1, nil),
		Output: map[string]any{
			"stage": "llm_context_integrity_failed",
			"error": "invalid argument: llm context gate read_required: runtime_repair_current_failure_missing",
			"contextBudget": map[string]any{
				"contextGateAction": "read_required",
				"contextGateReason": "runtime_repair_current_failure_missing",
			},
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed context gate step: %v", err)
	}
	resumeInput := map[string]any{
		"ticketContext": map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyContext,
		},
	}

	specs, ok, err := contextIntegritySpecsAfterResolvedDecision(ctx, stepRepo, run, failedStep.ID, resumeInput)

	if err != nil || !ok || len(specs) != 1 {
		t.Fatalf("contextIntegritySpecsAfterResolvedDecision specs/ok/err = %d/%v/%v, want one spec", len(specs), ok, err)
	}
	assertStateChangingAuthorityDispatchForTest(t, specs[0].Input)
	if err := validateConvergenceDispatchInput(specs[0].Input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, specs[0].Input)
	}
}

func TestRepairSubsystemRecoveryDispatchSpecRebuildsStaleForcedNarrowFromStateChangeAuthority(t *testing.T) {
	ctx := context.Background()
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	run.InitialStepKind = llmcommon.StepKindWorkspaceExecution
	stepRepo := memory.NewStepRepository()
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(run.ID) + "/graph-002/001-tidy-deps"),
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime|target=graph-node",
		},
	}

	spec, ok, err := newRepairSubsystem(stepRepo, nil).recoveryDispatchSpec(ctx, run, failedStep, errors.New("exit status 1"), runtimeRecoveryActionRuntimeRepair, maxProviderRetryAttempts, repairDispatchStaleForcedNarrowBuilder{}, false)

	if err != nil || !ok {
		t.Fatalf("recoveryDispatchSpec ok/err = %v/%v, want state-changing repair spec", ok, err)
	}
	contract := mapFromValue(spec.Input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
	required := mapFromValue(mapFromValue(spec.Input["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		boolValue(required["mustProduceConcreteValidationEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want canonical state-changing output", required)
	}
	if err := validateConvergenceDispatchInput(spec.Input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, spec.Input)
	}
}

func TestRepairSubsystemRecoveryDispatchSpecRebuildsStaleValidationMirrorFromStateChangeAuthority(t *testing.T) {
	ctx := context.Background()
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	run.InitialStepKind = llmcommon.StepKindWorkspaceExecution
	stepRepo := memory.NewStepRepository()
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(run.ID) + "/graph-003/001-validate"),
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|stage=runtime_step_failed|failureTargets=main.go|exitCode=1",
			"validationFailureDiagnostic": map[string]any{
				"class":                         runtimemodel.ValidationDiagnosticBuildFailed,
				"recommendedEvidenceTargetType": runtimemodel.ValidationEvidenceTargetFailureSurface,
				"targetRefs":                    []string{"cwi-current", "source_change", "main.go"},
			},
		},
	}

	spec, ok, err := newRepairSubsystem(stepRepo, nil).recoveryDispatchSpec(ctx, run, failedStep, errors.New("exit status 1"), runtimeRecoveryActionRuntimeRepair, maxProviderRetryAttempts, repairDispatchStaleValidationMirrorBuilder{}, false)

	if err != nil || !ok {
		t.Fatalf("recoveryDispatchSpec ok/err = %v/%v, want state-changing repair spec", ok, err)
	}
	contract := mapFromValue(spec.Input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
	for label, required := range map[string]map[string]any{
		"requiredOutput":                        mapFromValue(spec.Input["requiredOutput"]),
		"repairActionPackage.requiredOutput":    mapFromValue(mapFromValue(spec.Input["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":            mapFromValue(mapFromValue(spec.Input[stepInputKeyStagePacket])["requiredOutput"]),
		"stagePacket.nextAction.requiredOutput": mapFromValue(mapFromValue(mapFromValue(spec.Input[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
		"currentActionContract.requiredOutput":  mapFromValue(mapFromValue(spec.Input[currentActionContractKey])["requiredOutput"]),
	} {
		if !boolValue(required["mustIncludeStateChangingNode"]) ||
			!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
			stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), "patch/apply") ||
			boolValue(required["mustProduceConcreteValidationEvidence"]) {
			t.Fatalf("%s = %+v, want state-changing requiredOutput", label, required)
		}
	}
	if err := validateConvergenceDispatchInput(spec.Input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, spec.Input)
	}
}

func TestRepairSubsystemRecoveryDispatchSpecPreservesTerminalValidationAuthority(t *testing.T) {
	ctx := context.Background()
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	run.ID = "run-validation-repair-authority"
	run.WorkspaceID = "ws-validation-repair-authority"
	run.InitialStepKind = llmcommon.StepKindWorkspaceExecution
	updateAuthorityScopeForTest(run, run.WorkspaceID, run.ID)
	stepRepo := memory.NewStepRepository()
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(run.ID) + "/graph-004/001-validate"),
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Input: map[string]any{
			"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
			convergenceAuthorityKey: map[string]any{
				"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeValidate},
			},
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|stage=runtime_step_failed",
			"stage":                   "runtime_step_failed",
			"error":                   "exit status 1",
		},
	}

	spec, ok, err := newRepairSubsystem(stepRepo, nil).recoveryDispatchSpec(ctx, run, failedStep, errors.New("exit status 1"), runtimeRecoveryActionRuntimeRepair, maxProviderRetryAttempts, repairDispatchValidationAuthorityBuilder{}, false)

	if err != nil || !ok {
		t.Fatalf("recoveryDispatchSpec ok/err = %v/%v, want validation repair spec", ok, err)
	}
	contract := mapFromValue(spec.Input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want need_validation", contract)
	}
	required := mapFromValue(mapFromValue(spec.Input["repairActionPackage"])["requiredOutput"])
	if boolValue(required["mustIncludeStateChangingNode"]) ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		!boolValue(required["mustProduceConcreteValidationEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want validation-only output", required)
	}
	if err := validateConvergenceDispatchInput(spec.Input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, spec.Input)
	}
}

func TestRepairSubsystemRecoveryDispatchSpecRejectsLocalValidationAuthorityWhenFocusStillNeedsStateChange(t *testing.T) {
	ctx := context.Background()
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	run.ID = "run-state-change-gap-local-validation"
	run.WorkspaceID = "ws-state-change-gap-local-validation"
	run.InitialStepKind = llmcommon.StepKindWorkspaceExecution
	stateChangeFocus := map[string]any{
		"id":                    "cwi-current",
		"missingEvidenceKinds":  []string{runtimemodel.EvidenceKindStateChange},
		"requiredEvidenceKinds": []string{runtimemodel.EvidenceKindStateChange, runtimemodel.EvidenceKindValidation},
		"status":                "failed",
		"sourceRequirementId":   "source_change",
		"sourceScope":           "deliverable",
		"targetRefs":            []string{"cwi-current", "source_change"},
		"structuredTargetRefs":  []string{"cwi-current", "source_change"},
	}
	for _, key := range []string{convergenceAuthorityKey, gapBaselineKey} {
		authority := mapFromValue(run.ExecutionContext[key])
		authority["workspaceId"] = string(run.WorkspaceID)
		authority["runId"] = string(run.ID)
		authority["currentFocus"] = cloneStepPayload(stateChangeFocus)
		authority["nextAction"] = map[string]any{"kind": runtimemodel.NextActionTypeStateChange}
		run.ExecutionContext[key] = authority
	}
	run.ExecutionContext[coverageWorkItemContextKey] = map[string]any{"currentFocus": cloneStepPayload(stateChangeFocus)}
	stepRepo := memory.NewStepRepository()
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(run.ID) + "/graph-004/001-validate"),
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|stage=runtime_step_failed",
			"stage":                   "runtime_step_failed",
			"error":                   "exit status 1",
		},
	}

	spec, ok, err := newRepairSubsystem(stepRepo, nil).recoveryDispatchSpec(ctx, run, failedStep, errors.New("exit status 1"), runtimeRecoveryActionRuntimeRepair, maxProviderRetryAttempts, repairDispatchValidationAuthorityWithStateChangeGapBuilder{}, false)

	if err != nil || !ok {
		t.Fatalf("recoveryDispatchSpec ok/err = %v/%v, want state_change repair spec", ok, err)
	}
	contract := mapFromValue(spec.Input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
	required := mapFromValue(mapFromValue(spec.Input["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), "patch/apply") ||
		boolValue(required["mustProduceConcreteValidationEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want state-changing output", required)
	}
	current := mapFromValue(spec.Input[currentActionContractKey])
	if stringFromMap(mapFromValue(current["nextAction"]), "kind") != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("currentActionContract = %+v, want state_change nextAction", current)
	}
	if err := validateConvergenceDispatchInput(spec.Input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, spec.Input)
	}
}

func TestDecisionEligibilityDeniedAnnotationMarksRetryInput(t *testing.T) {
	input := annotateDecisionEligibilityDeniedRetry(map[string]any{"existing": true}, "continuable_next_action_available")

	if input["existing"] != true {
		t.Fatalf("input existing marker lost: %+v", input)
	}
	if input[stepInputKeyDecisionEligibilityDenied] != true ||
		input[stepInputKeyDecisionEligibilityDeniedReason] != "continuable_next_action_available" ||
		input["decisionNotEligibleReason"] != "continuable_next_action_available" ||
		input["decisionEligibilityGate"] != "automatic_next_action" {
		t.Fatalf("decision eligibility annotation = %+v, want structured denied retry markers", input)
	}
}

func TestRuntimeRepairExhaustionDecisionGateAllowsOneContinuableRetry(t *testing.T) {
	run := &runmodel.Run{ID: "run-decision-gate", WorkspaceID: "ws-decision-gate"}
	failed := &runmodel.Step{
		ID:    "run-decision-gate/failed",
		RunID: run.ID,
		State: common.StepStateFailed,
		Input: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"nextAction": map[string]any{"kind": "state_change"},
			},
		},
		Output: map[string]any{
			"error": "invalid argument: decision requests a continuable candidate choice",
		},
	}

	if !runtimeRepairExhaustionDecisionNotEligible(run, failed) {
		t.Fatal("runtimeRepairExhaustionDecisionNotEligible = false, want automatic retry for continuable next action")
	}
	failed.Input = annotateDecisionEligibilityDeniedRetry(failed.Input, "continuable_runtime_repair_available")
	if runtimeRepairExhaustionDecisionNotEligible(run, failed) {
		t.Fatal("runtimeRepairExhaustionDecisionNotEligible = true after denied retry marker, want escalation allowed")
	}
}

func TestRuntimeRepairExhaustionDecisionGateAllowsExternalDependencyDecision(t *testing.T) {
	run := &runmodel.Run{ID: "run-decision-gate-external", WorkspaceID: "ws-decision-gate-external"}
	failed := &runmodel.Step{
		ID:    "run-decision-gate-external/failed",
		RunID: run.ID,
		State: common.StepStateFailed,
		Input: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"nextAction": map[string]any{"kind": "state_change"},
			},
		},
		Output: map[string]any{
			"error":                      "invalid argument: decision requests a continuable candidate choice",
			"requiresExternalInput":      true,
			decisionContextKeyDependency: "external_information",
		},
	}

	if runtimeRepairExhaustionDecisionNotEligible(run, failed) {
		t.Fatal("runtimeRepairExhaustionDecisionNotEligible = true, want decision eligibility for external dependency")
	}
}

func TestRuntimeRepairExhaustionDecisionGateRejectsContinuableRuntimeContractDecision(t *testing.T) {
	run := &runmodel.Run{ID: "run-decision-gate-runtime-contract", WorkspaceID: "ws-decision-gate-runtime-contract"}
	failed := &runmodel.Step{
		ID:    "run-decision-gate-runtime-contract/failed",
		RunID: run.ID,
		State: common.StepStateFailed,
		Input: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"nextAction": map[string]any{"kind": "state_change"},
			},
		},
		Output: map[string]any{
			"error": "invalid argument: decision requests a continuable candidate choice",
			stepOutputKeyBlockDecisionContext: map[string]any{
				decisionContextKeyDependency: decisionContextDependencyRuntimeContract,
			},
		},
	}

	if !runtimeRepairExhaustionDecisionNotEligible(run, failed) {
		t.Fatal("runtimeRepairExhaustionDecisionNotEligible = false, want runtime contract decision denied while next action is continuable")
	}
}

func TestRuntimeRepairExhaustionDecisionGateRejectsRuntimeAcquirableTargetSnapshot(t *testing.T) {
	run := &runmodel.Run{ID: "run-decision-gate-target-snapshot", WorkspaceID: "ws-decision-gate-target-snapshot"}
	failed := &runmodel.Step{
		ID:    "run-decision-gate-target-snapshot/failed",
		RunID: run.ID,
		State: common.StepStateFailed,
		Input: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
			},
			"convergenceContract": map[string]any{
				"state":            convergenceStateNeedEvidence,
				"runtimeStageMode": "target_snapshot_acquisition",
			},
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"acceptedKinds":                      []string{"file/read"},
			},
		},
		Output: map[string]any{
			"error": "invalid argument: decision requests a continuable candidate choice",
			stepOutputKeyBlockDecisionContext: map[string]any{
				decisionContextKeyDependency: decisionContextDependencyRuntimeContract,
			},
		},
	}

	if !runtimeRepairExhaustionDecisionNotEligible(run, failed) {
		t.Fatal("runtimeRepairExhaustionDecisionNotEligible = false, want runtime-acquirable target snapshot to deny decision")
	}
}

func TestRuntimeRepairExhaustionDecisionGateAllowsProviderDependencyDecision(t *testing.T) {
	run := &runmodel.Run{ID: "run-decision-gate-provider", WorkspaceID: "ws-decision-gate-provider"}
	failed := &runmodel.Step{
		ID:    "run-decision-gate-provider/failed",
		RunID: run.ID,
		State: common.StepStateFailed,
		Input: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"nextAction": map[string]any{"kind": "state_change"},
			},
		},
		Output: map[string]any{
			"error": "invalid argument: decision requests a continuable candidate choice",
			stepOutputKeyBlockDecisionContext: map[string]any{
				decisionContextKeyDependency: decisionContextDependencyLLMProvider,
			},
		},
	}

	if runtimeRepairExhaustionDecisionNotEligible(run, failed) {
		t.Fatal("runtimeRepairExhaustionDecisionNotEligible = true, want provider dependency decision allowed")
	}
}

func TestRuntimeRepairExhaustionDecisionGateAllowsCommandContractBlocker(t *testing.T) {
	run := &runmodel.Run{ID: "run-command-contract-gate", WorkspaceID: "ws-command-contract-gate"}
	failed := &runmodel.Step{
		ID:    "run-command-contract-gate/failed",
		RunID: run.ID,
		State: common.StepStateFailed,
		Input: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"nextAction": map[string]any{"kind": "validate"},
			},
			"validationFailureDiagnostic": map[string]any{
				"class":                         runtimemodel.ValidationDiagnosticCommandInvalid,
				"stage":                         "shell_validation_contract_rejected",
				"recommendedEvidenceTargetType": runtimemodel.ValidationEvidenceTargetCommandContract,
				"commandPolicyViolation":        true,
			},
		},
		Output: map[string]any{
			"error": "validation command contract retry exhausted",
		},
	}

	if runtimeRepairExhaustionDecisionNotEligible(run, failed) {
		t.Fatal("runtimeRepairExhaustionDecisionNotEligible = true, want blocker for exhausted command contract validation retry")
	}
}

func TestRepairSubsystemBlockRecoveryActionPersistsBlockerBeforeErrBlocked(t *testing.T) {
	executor := &repairBlockedRecoveryExecutorStub{}
	err := newRepairSubsystem(nil, nil).blockRecoveryActionFromFailedStep(
		context.Background(),
		&runmodel.Run{ID: "run-repair-block"},
		&runmodel.Step{ID: "run-repair-block/failed", State: common.StepStateFailed},
		errors.New("failed"),
		runtimeRecoveryActionDuplicateFailure,
		executor,
	)

	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("blockRecoveryActionFromFailedStep error = %v, want ErrBlocked", err)
	}
	if executor.called != runtimeRecoveryActionDuplicateFailure {
		t.Fatalf("executor side effect = %s, want duplicate failure persisted before ErrBlocked", executor.called)
	}
}

func TestCloseRepairDispatchFailureForNonGraphStepLeavesUniqueExit(t *testing.T) {
	for _, stage := range []string{"prepare_authority_snapshot", "event_publish"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			runID := common.RunID("run-repair-dispatch-failure-" + stage)
			workspaceID := common.WorkspaceID("ws-repair-dispatch-failure-" + stage)
			runRepo := memory.NewRunRepository()
			stepRepo := memory.NewStepRepository()
			workspaceRepo := memory.NewWorkspaceRepository()
			ticketRepo := memory.NewDecisionTicketRepository()
			run := &runmodel.Run{
				ID:          runID,
				WorkspaceID: workspaceID,
				State:       common.RunStateActive,
			}
			failedStep := &runmodel.Step{
				ID:    common.StepID(runID + "/failed"),
				RunID: runID,
				Kind:  llmcommon.StepKindWorkspaceExecution,
				State: common.StepStateFailed,
				Input: map[string]any{
					stepInputKeyRuntimeRepairFollowUp: true,
				},
				Output: map[string]any{
					failureOutputKeyClass:     failureClassRuntime,
					failureOutputKeySignature: "class=runtime|target=dispatch",
				},
			}
			if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
				t.Fatalf("seed workspace: %v", err)
			}
			if err := runRepo.Create(ctx, run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := stepRepo.Create(ctx, failedStep); err != nil {
				t.Fatalf("seed step: %v", err)
			}
			orch := NewRuntimeOrchestrator(
				nil,
				runRepo,
				stepRepo,
				workspaceRepo,
				ticketRepo,
				memory.NewApprovalRepository())

			err := orch.closeRepairDispatchFailure(ctx, run, failedStep, errors.New("original failure"), runtimeDispatchRequestError{
				Stage: stage,
				Err:   common.ErrConflict,
			})
			if err != nil {
				t.Fatalf("closeRepairDispatchFailure: %v", err)
			}
			updatedRun, err := runRepo.Get(ctx, runID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if !runHasFailedStepUniqueExit(updatedRun) {
				t.Fatalf("run state = %s ticket=%s approval=%s, want unique blocker or terminal exit", updatedRun.State, updatedRun.BlockingTicketID, updatedRun.BlockingApprovalID)
			}
			updatedStep, err := stepRepo.Get(ctx, failedStep.ID)
			if err != nil {
				t.Fatalf("get step: %v", err)
			}
			if stringFromMap(updatedStep.Output, "repairDispatchErrorStage") != stage {
				t.Fatalf("repair dispatch diagnostic = %+v, want %s stage", updatedStep.Output, stage)
			}
		})
	}
}

func TestRepairSubsystemFailRunReasonFromFailedStep(t *testing.T) {
	manager := newRepairSubsystem(nil, nil)
	tests := []struct {
		name       string
		failedStep *runmodel.Step
		execErr    error
		want       string
	}{
		{
			name:       "exec error takes precedence",
			failedStep: &runmodel.Step{Output: map[string]any{"error": "output error"}},
			execErr:    errors.New("exec failed"),
			want:       "exec failed",
		},
		{
			name:       "output error fallback",
			failedStep: &runmodel.Step{Output: map[string]any{"error": " output error "}},
			want:       "output error",
		},
		{
			name:       "output reason fallback",
			failedStep: &runmodel.Step{Output: map[string]any{"reason": " output reason "}},
			want:       "output reason",
		},
		{
			name: "default reason",
			want: "runtime step failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := manager.failRunReasonFromFailedStep(tt.failedStep, tt.execErr); got != tt.want {
				t.Fatalf("failRunReasonFromFailedStep() = %q, want %q", got, tt.want)
			}
		})
	}
}

func stateChangeFocusForRepairDispatchTest(run *runmodel.Run) {
	if run == nil || run.ExecutionContext == nil {
		return
	}
	focus := map[string]any{
		"id":                    "cwi-current",
		"missingEvidenceKinds":  []string{runtimemodel.EvidenceKindStateChange},
		"requiredEvidenceKinds": []string{runtimemodel.EvidenceKindStateChange, runtimemodel.EvidenceKindValidation},
		"status":                "failed",
		"sourceRequirementId":   "source_change",
		"sourceScope":           "deliverable",
		"targetRefs":            []string{"cwi-current", "source_change"},
		"structuredTargetRefs":  []string{"cwi-current", "source_change"},
	}
	for _, key := range []string{convergenceAuthorityKey, gapBaselineKey} {
		authority := mapFromValue(run.ExecutionContext[key])
		authority["currentFocus"] = cloneStepPayload(focus)
		authority["nextAction"] = map[string]any{"kind": runtimemodel.NextActionTypeStateChange}
		run.ExecutionContext[key] = authority
	}
	run.ExecutionContext[coverageWorkItemContextKey] = map[string]any{"currentFocus": cloneStepPayload(focus)}
}

func assertStateChangingAuthorityDispatchForTest(t *testing.T, input map[string]any) {
	t.Helper()
	contract := mapFromValue(input["convergenceContract"])
	if got := stringFromMap(contract, "state"); got != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract.state = %q, want %q; input=%+v", got, convergenceStateNeedStateChange, input)
	}
	current := mapFromValue(input[currentActionContractKey])
	if got := stringFromMap(mapFromValue(current["nextAction"]), "kind"); got != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("currentActionContract.nextAction.kind = %q, want state_change; current=%+v input=%+v", got, current, input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if len(required) == 0 {
		required = activeRequiredOutputForCurrentAction(input)
	}
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), "patch/apply") ||
		boolValue(required["mustProduceConcreteValidationEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want canonical state-changing output", required)
	}
	snapshotID := stringFromMap(input, "authoritySnapshotId")
	if snapshotID == "" {
		t.Fatalf("authoritySnapshotId missing: %+v", input)
	}
	for label, value := range map[string]map[string]any{
		"convergenceContract":    contract,
		"repairActionPackage":    mapFromValue(input["repairActionPackage"]),
		"currentActionContract":  current,
		"stagePacket":            mapFromValue(input[stepInputKeyStagePacket]),
		"stagePacket.nextAction": mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["nextAction"]),
	} {
		if len(value) == 0 {
			continue
		}
		if got := stringFromMap(value, "authoritySnapshotId"); got != snapshotID {
			t.Fatalf("%s.authoritySnapshotId = %q, want %q", label, got, snapshotID)
		}
	}
}
