package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRebuildDispatchInputAlignsStaleFocusFieldsToAuthority(t *testing.T) {
	run := runWithAuthorityFocusForTest("validate")
	input := map[string]any{
		convergenceAuthorityKey: cloneStepPayload(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])),
		gapBaselineKey:          cloneStepPayload(mapFromValue(run.ExecutionContext[gapBaselineKey])),
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": map[string]any{"id": "cwi-current", "missingEvidenceKinds": []string{"validation"}},
			"unresolvedItems": []map[string]any{{
				"id":                   "cwi-current",
				"missingEvidenceKinds": []string{"validation"},
			}},
		},
		coverageFocusKey: map[string]any{
			"id":                  "focus-cwi-stale",
			"coverageWorkItemIds": []string{"cwi-stale"},
			"sliceId":             "slice-a",
		},
		"acceptance": map[string]any{
			"activeSliceId": "slice-a",
			"activeSlice": map[string]any{
				"id":                  "slice-a",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
			"currentFocus": map[string]any{
				"id":                  "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"coverageFocusId":     "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
		},
		"convergenceBudget": map[string]any{
			"focusId":             "focus-cwi-stale",
			"coverageWorkItemIds": []string{"cwi-stale"},
		},
		"executionContext": map[string]any{
			coverageWorkItemContextKey: map[string]any{
				"currentFocus": map[string]any{
					"id":                  "focus-cwi-stale",
					"coverageWorkItemIds": []string{"cwi-stale"},
				},
			},
			"convergenceBudget": map[string]any{
				"focusId":             "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
			"requiredOutput": map[string]any{
				"coverageFocusId":     "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"coverageFocusId":     "focus-cwi-stale",
					"coverageWorkItemIds": []string{"cwi-stale"},
				},
			},
			stepInputKeyStagePacket: map[string]any{
				"activeFocus": map[string]any{
					"id":                  "focus-cwi-stale",
					"coverageWorkItemIds": []string{"cwi-stale"},
				},
				"requiredOutput": map[string]any{
					"coverageFocusId":     "focus-cwi-stale",
					"coverageWorkItemIds": []string{"cwi-stale"},
				},
			},
		},
	}

	got, err := rebuildDispatchInputFromConvergenceAuthority(run, "step-current", input)
	if err != nil {
		t.Fatalf("rebuildDispatchInputFromConvergenceAuthority: %v", err)
	}

	assertFocusIDForTest(t, mapFromValue(got[coverageFocusKey]), "focus-cwi-current")
	acceptance := mapFromValue(got["acceptance"])
	assertFocusIDForTest(t, mapFromValue(acceptance["currentFocus"]), "focus-cwi-current")
	if activeSlice := mapFromValue(acceptance["activeSlice"]); len(activeSlice) > 0 {
		t.Fatalf("acceptance.activeSlice = %+v, want stale slice removed from current action", activeSlice)
	}
	if stale := mapFromValue(mapFromValue(acceptance["diagnosticLineage"])["staleAcceptance"]); len(mapFromValue(stale["activeSlice"])) == 0 {
		t.Fatalf("acceptance.diagnosticLineage = %+v, want stale activeSlice lineage", acceptance["diagnosticLineage"])
	}
	required := mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"])
	if stringFromMap(required, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want current coverageFocusId", required)
	}
	if ids := stringSliceFromValue(required["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != "cwi-current" {
		t.Fatalf("repairActionPackage.requiredOutput.coverageWorkItemIds = %+v, want [cwi-current]", ids)
	}
	if budget := mapFromValue(got["convergenceBudget"]); stringFromMap(budget, "focusId") != "focus-cwi-current" {
		t.Fatalf("convergenceBudget = %+v, want current focus", budget)
	}
	executionContext := mapFromValue(got["executionContext"])
	if budget := mapFromValue(executionContext["convergenceBudget"]); stringFromMap(budget, "focusId") != "focus-cwi-current" {
		t.Fatalf("executionContext.convergenceBudget = %+v, want current focus", budget)
	}
	if contextFocus := mapFromValue(mapFromValue(executionContext[coverageWorkItemContextKey])["currentFocus"]); !dispatchFocusAligned(mapFromValue(got[coverageFocusKey]), contextFocus) {
		t.Fatalf("executionContext.%s.currentFocus = %+v, want current focus", coverageWorkItemContextKey, contextFocus)
	}
	if required := mapFromValue(executionContext["requiredOutput"]); stringFromMap(required, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("executionContext.requiredOutput = %+v, want current focus", required)
	}
	if required := mapFromValue(mapFromValue(executionContext["repairActionPackage"])["requiredOutput"]); stringFromMap(required, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("executionContext.repairActionPackage.requiredOutput = %+v, want current focus", required)
	}
	if required := mapFromValue(mapFromValue(executionContext[stepInputKeyStagePacket])["requiredOutput"]); stringFromMap(required, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("executionContext.stagePacket.requiredOutput = %+v, want current focus", required)
	}
	packet := mapFromValue(got[stepInputKeyStagePacket])
	assertFocusIDForTest(t, mapFromValue(packet["activeFocus"]), "cwi-current")
	stageRequired := mapFromValue(packet["requiredOutput"])
	if stringFromMap(stageRequired, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("stagePacket.requiredOutput = %+v, want current coverageFocusId", stageRequired)
	}
}

func TestNormalizeDispatchInputDerivesAuthoritySliceFromAcceptanceSlices(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"currentFocus": map[string]any{
				"id":                  "cwi-003",
				"coverageWorkItemIds": []string{"cwi-003"},
				"requiredEvidenceKinds": []string{
					"validation",
				},
			},
		},
		coverageFocusKey: map[string]any{
			"id":                  "focus-cwi-003",
			"sliceId":             "slice-primary-state-change",
			"coverageWorkItemIds": []string{"cwi-003"},
			"requiredEvidenceKinds": []string{
				"validation",
			},
		},
		"acceptance": map[string]any{
			"activeSliceId": "slice-primary-state-change",
			"activeSlice": map[string]any{
				"id":                  "slice-primary-state-change",
				"coverageWorkItemIds": []string{"cwi-085"},
			},
			"currentFocus": map[string]any{
				"id":                  "focus-cwi-003",
				"sliceId":             "slice-primary-state-change",
				"coverageWorkItemIds": []string{"cwi-003"},
			},
		},
		"executionContext": map[string]any{
			acceptanceSlicesKey: []map[string]any{
				{
					"id":                  "slice-primary-state-change",
					"coverageWorkItemIds": []string{"cwi-085"},
				},
				{
					"id":                  "slice-direct-validation-001",
					"coverageWorkItemIds": []string{"cwi-003", "cwi-004", "cwi-005", "cwi-006"},
				},
			},
			coverageFocusKey: map[string]any{
				"id":                  "focus-cwi-003",
				"sliceId":             "slice-primary-state-change",
				"coverageWorkItemIds": []string{"cwi-003"},
			},
		},
	}

	got := normalizeDispatchInputAuthorityFocus(input)
	focus := mapFromValue(got[coverageFocusKey])
	if stringFromMap(focus, "sliceId") != "slice-direct-validation-001" {
		t.Fatalf("coverageFocus = %+v, want slice-direct-validation-001", focus)
	}
	acceptance := mapFromValue(got["acceptance"])
	if stringFromMap(acceptance, "activeSliceId") != "slice-direct-validation-001" {
		t.Fatalf("acceptance = %+v, want activeSliceId slice-direct-validation-001", acceptance)
	}
	if activeSlice := mapFromValue(acceptance["activeSlice"]); len(activeSlice) > 0 {
		t.Fatalf("acceptance.activeSlice = %+v, want stale primary slice removed", activeSlice)
	}
}

func TestDispatchNextStepPreparesInputFromCurrentAuthority(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dispatch-next-authority")
	workspaceID := common.WorkspaceID("ws-dispatch-next-authority")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	run := runWithAuthorityFocusForTest("validate")
	run.ID = runID
	run.WorkspaceID = workspaceID
	run.InitialStepKind = llmWorkspaceExecutionStepKind
	updateAuthorityScopeForTest(run, workspaceID, runID)
	run.ExecutionContext["stateChangingOutputRequired"] = true
	run.ExecutionContext["repairObjective"] = repairObjectiveStateChangingOutput
	run.ExecutionContext["repairActionPackage"] = map[string]any{
		"requiredOutput": map[string]any{
			"mustIncludeStateChangingNode": true,
			"coverageFocusId":              "focus-cwi-stale",
			"coverageWorkItemIds":          []string{"cwi-stale"},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspaceModel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	var captured map[string]any
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			captured = cloneStepPayload(request.Input)
			return map[string]any{"completionState": "continue"}, nil
		},
	}
	orch := NewRuntimeOrchestrator(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("DispatchNextStep: %v", err)
	}
	if len(captured) == 0 {
		t.Fatal("runtime input was not captured")
	}
	if boolValue(captured["stateChangingOutputRequired"]) {
		t.Fatalf("runtime input retained stale stateChangingOutputRequired: %+v", captured)
	}
	contract := mapFromValue(captured["convergenceContract"])
	if got := stringFromMap(contract, "state"); got != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want need_validation", contract)
	}
	assertFocusIDForTest(t, mapFromValue(captured[coverageFocusKey]), "focus-cwi-current")
	packet := mapFromValue(captured[stepInputKeyStagePacket])
	assertFocusIDForTest(t, mapFromValue(packet["activeFocus"]), "cwi-current")
	required := mapFromValue(mapFromValue(captured["repairActionPackage"])["requiredOutput"])
	if boolValue(required["mustIncludeStateChangingNode"]) {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want validation contract without stale state-changing marker", required)
	}
	if got := stringFromMap(required, "coverageFocusId"); got != "focus-cwi-current" {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want current focus", required)
	}
}

func TestRebuildDispatchInputReplacesStaleRequiredOutputForAssessmentAuthority(t *testing.T) {
	run := runWithAuthorityFocusForTest("assessment")
	input := map[string]any{
		convergenceAuthorityKey: cloneStepPayload(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])),
		gapBaselineKey:          cloneStepPayload(mapFromValue(run.ExecutionContext[gapBaselineKey])),
		coverageFocusKey: map[string]any{
			"id":                  "focus-cwi-stale",
			"coverageWorkItemIds": []string{"cwi-stale"},
		},
		"requiredOutput": map[string]any{
			"coverageFocusId":     "focus-cwi-stale",
			"coverageWorkItemIds": []string{"cwi-stale"},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"coverageFocusId":     "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
		},
	}

	got, err := rebuildDispatchInputFromConvergenceAuthority(run, "step-assessment", input)
	if err != nil {
		t.Fatalf("rebuildDispatchInputFromConvergenceAuthority: %v", err)
	}

	packet := mapFromValue(got[stepInputKeyStagePacket])
	assertFocusIDForTest(t, mapFromValue(packet["activeFocus"]), "cwi-current")
	if required := mapFromValue(packet["requiredOutput"]); len(required) > 0 {
		if stringFromMap(required, "assessmentScope") != "current_focus" {
			t.Fatalf("stagePacket.requiredOutput = %+v, want focus-scoped assessment contract", required)
		}
		if ids := stringSliceFromValue(required["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != "cwi-current" {
			t.Fatalf("stagePacket.requiredOutput.coverageWorkItemIds = %+v, want [cwi-current]", ids)
		}
	}
	if required := mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"]); stringFromMap(required, "assessmentScope") != "current_focus" {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want focus-scoped assessment contract", required)
	}
}

func TestStagePacketRequiredOutputRebindsToAuthorityWithoutDispatchRebuild(t *testing.T) {
	run := runWithAuthorityFocusForTest("validate")
	input := map[string]any{
		convergenceAuthorityKey: cloneStepPayload(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])),
		gapBaselineKey:          cloneStepPayload(mapFromValue(run.ExecutionContext[gapBaselineKey])),
		"convergenceContract":   map[string]any{"state": convergenceStateNeedValidation},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"coverageFocusId":     "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
		},
	}

	packet := buildStagePacketFromStepInput(run, "step-current", input).ToMap()
	required := mapFromValue(packet["requiredOutput"])
	if stringFromMap(required, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("stagePacket.requiredOutput = %+v, want current coverageFocusId", required)
	}
	if ids := stringSliceFromValue(required["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != "cwi-current" {
		t.Fatalf("stagePacket.requiredOutput.coverageWorkItemIds = %+v, want [cwi-current]", ids)
	}
}

func TestStagePacketRequiredOutputDropsExecutableOutputForAssessmentAuthority(t *testing.T) {
	run := runWithAuthorityFocusForTest("assessment")
	input := map[string]any{
		convergenceAuthorityKey: cloneStepPayload(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])),
		gapBaselineKey:          cloneStepPayload(mapFromValue(run.ExecutionContext[gapBaselineKey])),
		"convergenceContract":   map[string]any{"state": convergenceStateNeedAssessment},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"coverageFocusId":     "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
		},
	}

	packet := buildStagePacketFromStepInput(run, "step-assessment", input).ToMap()
	required := mapFromValue(packet["requiredOutput"])
	if stringFromMap(required, "assessmentScope") != "current_focus" {
		t.Fatalf("stagePacket.requiredOutput = %+v, want focus-scoped assessment contract", required)
	}
	if ids := stringSliceFromValue(required["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != "cwi-current" {
		t.Fatalf("stagePacket.requiredOutput.coverageWorkItemIds = %+v, want [cwi-current]", ids)
	}
}

func TestFinalizeRuntimeRepairFollowUpStepInputRebindsStaleRetryFocus(t *testing.T) {
	run := runWithAuthorityFocusForTest("validate")
	input := map[string]any{
		convergenceAuthorityKey: cloneStepPayload(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])),
		gapBaselineKey:          cloneStepPayload(mapFromValue(run.ExecutionContext[gapBaselineKey])),
		"convergenceContract":   map[string]any{"state": convergenceStateNeedValidation},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"coverageFocusId":     "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
		},
		"requiredOutput": map[string]any{
			"coverageFocusId":     "focus-cwi-stale",
			"coverageWorkItemIds": []string{"cwi-stale"},
		},
	}
	failed := &runModel.Step{ID: "run-authority-focus/failed-001", RunID: run.ID}

	got := finalizeRuntimeRepairFollowUpStepInput(run, failed, input)

	required := mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"])
	if stringFromMap(required, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want current focus", required)
	}
	topLevelRequired := mapFromValue(got["requiredOutput"])
	if stringFromMap(topLevelRequired, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("requiredOutput = %+v, want current focus", topLevelRequired)
	}
	packet := mapFromValue(got[stepInputKeyStagePacket])
	stageRequired := mapFromValue(packet["requiredOutput"])
	if stringFromMap(stageRequired, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("stagePacket.requiredOutput = %+v, want current focus", stageRequired)
	}
	if err := validateConvergenceDispatchInput(got); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v", err)
	}
}

func TestNormalizeRunConvergenceOwnerStateRebindsPersistedDerivedFocus(t *testing.T) {
	run := runWithAuthorityFocusForTest("validate")
	run.ExecutionContext[coverageFocusKey] = map[string]any{
		"id":                  "focus-cwi-stale",
		"coverageWorkItemIds": []string{"cwi-stale"},
	}
	run.ExecutionContext["repairActionPackage"] = map[string]any{
		"requiredOutput": map[string]any{
			"coverageFocusId":     "focus-cwi-stale",
			"coverageWorkItemIds": []string{"cwi-stale"},
		},
	}

	if !normalizeRunConvergenceOwnerState(run) {
		t.Fatal("normalizeRunConvergenceOwnerState returned false, want stale derived fields updated")
	}

	assertFocusIDForTest(t, mapFromValue(run.ExecutionContext[coverageFocusKey]), "focus-cwi-current")
	required := mapFromValue(mapFromValue(run.ExecutionContext["repairActionPackage"])["requiredOutput"])
	if stringFromMap(required, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want current focus", required)
	}
	if ids := stringSliceFromValue(required["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != "cwi-current" {
		t.Fatalf("repairActionPackage.requiredOutput.coverageWorkItemIds = %+v, want [cwi-current]", ids)
	}
	packet := mapFromValue(run.ExecutionContext[stepInputKeyStagePacket])
	stageRequired := mapFromValue(packet["requiredOutput"])
	if stringFromMap(stageRequired, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("stagePacket.requiredOutput = %+v, want current focus", stageRequired)
	}
}

func TestRebuildDispatchInputUsesAuthorityAfterCoverageAssessmentIncomplete(t *testing.T) {
	run := runWithAuthorityFocusForTest("validate")
	input := map[string]any{
		convergenceAuthorityKey: cloneStepPayload(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])),
		gapBaselineKey:          cloneStepPayload(mapFromValue(run.ExecutionContext[gapBaselineKey])),
		"convergenceContract": map[string]any{
			"state":           convergenceStateNeedAssessment,
			"repairObjective": "complete_dod_goal_coverage_assessment",
		},
		"repairObjective":          "complete_dod_goal_coverage_assessment",
		stepInputKeyAssessmentOnly: true,
		stepInputKeyContractViolationRecovery: map[string]any{
			"violatedRule":   contractViolationRuleCoverageAssessmentIncomplete,
			"recoverability": runtimemodel.ContractViolationRecoverabilityRecomputeAuthority,
			"expectedContract": map[string]any{
				"convergenceContract": map[string]any{
					"state": convergenceStateNeedAssessment,
				},
			},
		},
		"contractViolationRetry": true,
	}

	got, err := rebuildDispatchInputFromConvergenceAuthority(run, "step-recomputed-authority", input)
	if err != nil {
		t.Fatalf("rebuildDispatchInputFromConvergenceAuthority: %v", err)
	}

	contract := mapFromValue(got["convergenceContract"])
	if state := stringFromMap(contract, "state"); state != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract.state = %q, want %s; input=%+v", state, convergenceStateNeedValidation, got)
	}
	if got["repairObjective"] != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("repairObjective = %v, want %s", got["repairObjective"], repairObjectiveConcreteValidationEvidence)
	}
	if got[stepInputKeyAssessmentOnly] != nil {
		t.Fatalf("%s = %v, want removed after authority recompute", stepInputKeyAssessmentOnly, got[stepInputKeyAssessmentOnly])
	}
	required := mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustProduceConcreteValidationEvidence"]) || boolValue(required["mustIncludeStateChangingNode"]) {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want validation-only evidence contract", required)
	}
	packet := mapFromValue(got[stepInputKeyStagePacket])
	assertFocusIDForTest(t, mapFromValue(packet["activeFocus"]), "cwi-current")
	stageRequired := mapFromValue(packet["requiredOutput"])
	if !boolValue(stageRequired["mustProduceConcreteValidationEvidence"]) || boolValue(stageRequired["mustIncludeStateChangingNode"]) {
		t.Fatalf("stagePacket.requiredOutput = %+v, want validation-only evidence contract", stageRequired)
	}
}

func TestBlockOnDecisionPersistsAuthorityNormalizedOwnerState(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-block-normalized-authority")
	workspaceID := common.WorkspaceID("ws-block-normalized-authority")
	ticketID := common.TicketID("ticket-block-normalized-authority")
	run := runWithAuthorityFocusForTest("validate")
	run.ID = runID
	run.WorkspaceID = workspaceID
	run.State = common.RunStateRunning
	run.ExecutionContext[coverageFocusKey] = map[string]any{
		"id":                  "focus-cwi-stale",
		"coverageWorkItemIds": []string{"cwi-stale"},
	}
	run.ExecutionContext["repairActionPackage"] = map[string]any{
		"requiredOutput": map[string]any{
			"coverageFocusId":     "focus-cwi-stale",
			"coverageWorkItemIds": []string{"cwi-stale"},
		},
	}
	updateAuthorityScopeForTest(run, workspaceID, runID)

	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspaceModel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := NewRuntimeOrchestrator(nil, runRepo, memory.NewStepRepository(), workspaceRepo, memory.NewDecisionTicketRepository(), memory.NewApprovalRepository(), events.NewMemoryHub())

	if err := orch.blockOnDecisionAtStep(ctx, run, nil, ticketID, ""); err != nil {
		t.Fatalf("blockOnDecisionAtStep: %v", err)
	}

	persisted, err := runRepo.GetByWorkspaceID(ctx, workspaceID, runID)
	if err != nil {
		t.Fatalf("load persisted run: %v", err)
	}
	if persisted.State != common.RunStateBlockedOnDecision || persisted.BlockingTicketID != ticketID {
		t.Fatalf("persisted lifecycle = %s/%s, want blocked on %s", persisted.State, persisted.BlockingTicketID, ticketID)
	}
	assertFocusIDForTest(t, mapFromValue(persisted.ExecutionContext[coverageFocusKey]), "focus-cwi-current")
	required := mapFromValue(mapFromValue(persisted.ExecutionContext["repairActionPackage"])["requiredOutput"])
	if stringFromMap(required, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("persisted repairActionPackage.requiredOutput = %+v, want current focus", required)
	}
}

func TestValidateConvergenceDispatchInputRejectsFocusDrift(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"currentFocus": map[string]any{"id": "cwi-current"},
			"nextAction":   map[string]any{"kind": "validate"},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		stepInputKeyStagePacket: map[string]any{
			"activeFocus":    map[string]any{"id": "cwi-current"},
			"requiredOutput": map[string]any{"coverageFocusId": "focus-cwi-stale"},
		},
	}

	if err := validateConvergenceDispatchInput(input); err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want focus mismatch conflict")
	} else if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want ErrConflict", err)
	} else if !strings.Contains(err.Error(), failureClassRuntimeContractMisalign) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want %s marker", err, failureClassRuntimeContractMisalign)
	}
}

func TestValidateConvergenceDispatchInputRejectsTopLevelRequiredOutputFocusDrift(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"currentFocus": map[string]any{"id": "cwi-current"},
			"nextAction":   map[string]any{"kind": "validate"},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		"requiredOutput": map[string]any{
			"coverageFocusId":     "focus-cwi-stale",
			"coverageWorkItemIds": []string{"cwi-stale"},
		},
		stepInputKeyStagePacket: map[string]any{
			"activeFocus": map[string]any{"id": "cwi-current"},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want requiredOutput focus mismatch")
	}
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), failureClassRuntimeContractMisalign) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want %s marker", err, failureClassRuntimeContractMisalign)
	}
}

func TestValidateConvergenceDispatchInputRejectsStateChangeRequiredOutputForbidsAcceptedKind(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"currentFocus": map[string]any{"id": "cwi-current"},
			"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
		},
		"convergenceContract":         map[string]any{"state": convergenceStateNeedStateChange},
		"stateChangingOutputRequired": true,
		"requiredOutput": map[string]any{
			"mustIncludeStateChangingNode": true,
			"acceptedKinds":                []string{"patch/apply"},
			"forbiddenKinds":               []string{"patch/apply"},
		},
		stepInputKeyStagePacket: map[string]any{
			"activeFocus": map[string]any{"id": "cwi-current"},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want requiredOutput/NextAction mismatch")
	}
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "required_output_mismatch_next_action") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want required_output_mismatch_next_action", err)
	}
	if diagnostics := mapSliceFromValue(input["authorityDiagnostics"]); len(diagnostics) != 1 || stringFromMap(diagnostics[0], "code") != "required_output_mismatch_next_action" {
		t.Fatalf("authorityDiagnostics = %+v, want required_output_mismatch_next_action", input["authorityDiagnostics"])
	}
}

func TestValidateConvergenceDispatchInputRejectsValidationRequiredOutputAcceptsPatchApply(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"currentFocus": map[string]any{"id": "cwi-current"},
			"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		"requiredOutput": map[string]any{
			"mustProduceConcreteValidationEvidence": true,
			"mustNotMutateWorkspace":                true,
			"acceptedKinds":                         []string{"patch/apply"},
		},
		stepInputKeyStagePacket: map[string]any{
			"activeFocus": map[string]any{"id": "cwi-current"},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want validation requiredOutput mismatch")
	}
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "required_output_mismatch_next_action") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want required_output_mismatch_next_action", err)
	}
	if diagnostics := mapSliceFromValue(input["authorityDiagnostics"]); len(diagnostics) != 1 || stringFromMap(diagnostics[0], "code") != "required_output_mismatch_next_action" {
		t.Fatalf("authorityDiagnostics = %+v, want required_output_mismatch_next_action", input["authorityDiagnostics"])
	}
}

func TestValidateConvergenceDispatchInputRejectsAcceptanceActiveSliceFocusDrift(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"currentFocus": map[string]any{"id": "cwi-current"},
			"nextAction":   map[string]any{"kind": "validate"},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		"acceptance": map[string]any{
			"activeSlice": map[string]any{
				"id":                  "slice-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
		},
		stepInputKeyStagePacket: map[string]any{
			"activeFocus": map[string]any{"id": "cwi-current"},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want activeSlice focus mismatch")
	}
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), failureClassRuntimeContractMisalign) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want %s marker", err, failureClassRuntimeContractMisalign)
	}
}

func TestDispatchPreCreateHardGateRejectsUnrebuildableFocusDriftBeforeRuntime(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-authority-hard-gate")
	workspaceID := common.WorkspaceID("ws-authority-hard-gate")
	stepID := common.StepID(runID + "/step-1")
	run := &runModel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}
	runtime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "completed"}}
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspaceModel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, memory.NewDecisionTicketRepository(), memory.NewApprovalRepository(), events.NewMemoryHub())

	err := orch.dispatchStepWithWorkspace(ctx, run, stepID, llmWorkspaceExecutionStepKind, map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"workspaceId":   string(workspaceID),
			"runId":         string(runID),
			"currentFocus": map[string]any{
				"id":                  "focus-cwi-current",
				"coverageWorkItemIds": []string{"cwi-current"},
			},
			"nextAction": map[string]any{"kind": "validate"},
		},
		gapBaselineKey: map[string]any{
			"schemaVersion": gapBaselineSchema,
			"workspaceId":   string(workspaceID),
			"runId":         string(runID),
			"currentFocus": map[string]any{
				"id":                  "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
			"nextAction": map[string]any{"kind": "validate"},
		},
	}, nil)
	if err == nil {
		t.Fatal("dispatchStepWithWorkspace returned nil, want runtime contract misalignment conflict")
	}
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("dispatchStepWithWorkspace error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), failureClassRuntimeContractMisalign) {
		t.Fatalf("dispatchStepWithWorkspace error = %v, want %s marker", err, failureClassRuntimeContractMisalign)
	}
	if len(runtime.requests) != 0 {
		t.Fatalf("runtime requests = %+v, want provider/runtime not called", runtime.requests)
	}
	if _, err := stepRepo.Get(ctx, stepID); err == nil {
		t.Fatal("gated step was created, want pre-create hard gate to reject before persisted dispatch")
	} else if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("load gated step error = %v, want ErrNotFound", err)
	}
}

func TestRuntimeInternalContractMisalignmentClassification(t *testing.T) {
	step := &runModel.Step{
		ID:    "run-1/step-1",
		RunID: "run-1",
		Kind:  llmWorkspaceExecutionStepKind,
	}
	output := map[string]any{
		"stage": "runtime_internal_contract_misalignment",
		"error": "conflict: runtime_internal_contract_misalignment: focus mismatch",
	}

	classification := classifyRuntimeFailure(step, output, "runtime convergence authority dispatch gate failed", nil)
	if classification.Class != failureClassRuntimeContractMisalign {
		t.Fatalf("classification.Class = %q, want %q", classification.Class, failureClassRuntimeContractMisalign)
	}
}

func runWithAuthorityFocusForTest(nextAction string) *runModel.Run {
	focus := map[string]any{
		"id":                   "cwi-current",
		"missingEvidenceKinds": []string{"validation"},
		"status":               "open",
	}
	return &runModel.Run{
		ID:          common.RunID("run-authority-focus"),
		WorkspaceID: common.WorkspaceID("ws-authority-focus"),
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"workspaceId":   "ws-authority-focus",
				"runId":         "run-authority-focus",
				"currentFocus":  focus,
				"nextAction":    map[string]any{"kind": nextAction},
			},
			gapBaselineKey: map[string]any{
				"schemaVersion": gapBaselineSchema,
				"workspaceId":   "ws-authority-focus",
				"runId":         "run-authority-focus",
				"currentFocus":  focus,
				"nextAction":    map[string]any{"kind": nextAction},
			},
		},
	}
}

func updateAuthorityScopeForTest(run *runModel.Run, workspaceID common.WorkspaceID, runID common.RunID) {
	if run == nil || run.ExecutionContext == nil {
		return
	}
	for _, key := range []string{convergenceAuthorityKey, gapBaselineKey} {
		value := mapFromValue(run.ExecutionContext[key])
		if len(value) == 0 {
			continue
		}
		value["workspaceId"] = string(workspaceID)
		value["runId"] = string(runID)
		run.ExecutionContext[key] = value
	}
}

func assertFocusIDForTest(t *testing.T, focus map[string]any, want string) {
	t.Helper()
	if got := stringFromMap(focus, "id"); got != want {
		t.Fatalf("focus = %+v, want id %q", focus, want)
	}
}
