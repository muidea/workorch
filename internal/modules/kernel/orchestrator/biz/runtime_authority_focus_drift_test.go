package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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

func TestRebuildDispatchInputStateChangeAuthorityRebindsEvidenceContract(t *testing.T) {
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	input := map[string]any{
		convergenceAuthorityKey: cloneStepPayload(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])),
		gapBaselineKey:          cloneStepPayload(mapFromValue(run.ExecutionContext[gapBaselineKey])),
		"convergenceContract": map[string]any{
			"state":           convergenceStateNeedEvidence,
			"runtimeStage":    runtimeStageRepair,
			"repairObjective": repairObjectiveAcquireFailureEvidence,
		},
		"repairObjective": repairObjectiveAcquireFailureEvidence,
		"repairActionPackage": map[string]any{
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"evidenceAcquisitionRequired":     true,
			"concreteFailureEvidenceRequired": true,
			"requiredOutput": map[string]any{
				"acceptedKinds":                []string{"file/read", "shell/command"},
				"stateChangingForbidden":       true,
				"mustIncludeStateChangingNode": false,
				"forbiddenKinds":               []string{"patch/apply"},
			},
		},
		"requiredOutput": map[string]any{
			"acceptedKinds":                []string{"file/read", "shell/command"},
			"stateChangingForbidden":       true,
			"mustIncludeStateChangingNode": false,
			"forbiddenKinds":               []string{"patch/apply"},
		},
	}
	got, err := rebuildDispatchInputFromConvergenceAuthority(run, "step-state-change-rebind", input)
	if err != nil {
		t.Fatalf("rebuildDispatchInputFromConvergenceAuthority: %v", err)
	}
	if state := stringFromMap(mapFromValue(got["convergenceContract"]), "state"); state != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract.state = %q, want %q", state, convergenceStateNeedStateChange)
	}
	if kind := stringFromMap(currentActionNextActionFromInput(got), "kind"); kind != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("current nextAction = %+v, want state_change", currentActionNextActionFromInput(got))
	}
	for label, required := range map[string]map[string]any{
		"requiredOutput":                       mapFromValue(got["requiredOutput"]),
		"repairActionPackage.requiredOutput":   mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":           mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["requiredOutput"]),
		"currentActionContract.requiredOutput": mapFromValue(mapFromValue(got[currentActionContractKey])["requiredOutput"]),
	} {
		if !requiredOutputHasAllowedStateChangingKind(required) {
			t.Fatalf("%s = %+v, want state-changing required output", label, required)
		}
	}
}

func TestStripDerivedActionContractForReplanSuccessorKeepsOnlyLineageFacts(t *testing.T) {
	context := map[string]any{
		convergenceAuthorityKey:  map[string]any{"schemaVersion": convergenceAuthoritySchema},
		gapBaselineKey:           map[string]any{"schemaVersion": gapBaselineSchema},
		stepInputKeyStagePacket:  map[string]any{"nextAction": map[string]any{"kind": "request_governance"}},
		"convergenceContract":    map[string]any{"state": convergenceStateNeedGovernance},
		currentActionContractKey: map[string]any{"nextAction": map[string]any{"kind": "request_governance"}},
		"repairActionPackage":    map[string]any{"requiredOutput": map[string]any{"mustRequestGovernance": true}},
		"requiredOutput":         map[string]any{"mustRequestGovernance": true},
		stepInputKeyRuntimeStage: "governance",
		"repairObjective":        "request_governance",
		"targetSnapshots":        []map[string]any{{"id": "snapshot-001"}},
		"executionContract":      map[string]any{"version": "v1"},
		"replanContinuation":     map[string]any{"predecessorRunId": "run-001"},
	}

	if !stripDerivedActionContractForReplanSuccessor(context) {
		t.Fatal("stripDerivedActionContractForReplanSuccessor returned false, want derived fields removed")
	}
	for _, key := range []string{
		stepInputKeyStagePacket,
		"convergenceContract",
		currentActionContractKey,
		"repairActionPackage",
		"requiredOutput",
		stepInputKeyRuntimeStage,
		"repairObjective",
	} {
		if _, ok := context[key]; ok {
			t.Fatalf("context[%q] = %+v, want stripped", key, context[key])
		}
	}
	if len(mapFromValue(context[convergenceAuthorityKey])) == 0 || len(mapFromValue(context[gapBaselineKey])) == 0 {
		t.Fatalf("authority facts were stripped: %+v", context)
	}
	if len(mapSliceFromValue(context["targetSnapshots"])) != 1 ||
		len(mapFromValue(context["executionContract"])) == 0 ||
		len(mapFromValue(context["replanContinuation"])) == 0 {
		t.Fatalf("lineage/fact fields were not preserved: %+v", context)
	}
}

func TestMergeRunProgressUpdateDeletesOmittedDerivedActionContracts(t *testing.T) {
	current := &runModel.Run{
		ID:          "run-progress-current",
		WorkspaceID: "ws-progress",
		ExecutionContext: map[string]any{
			stepInputKeyStagePacket:  map[string]any{"nextAction": map[string]any{"kind": "request_governance"}},
			"convergenceContract":    map[string]any{"state": convergenceStateNeedGovernance},
			currentActionContractKey: map[string]any{"nextAction": map[string]any{"kind": "request_governance"}},
			"repairActionPackage":    map[string]any{"requiredOutput": map[string]any{"mustRequestGovernance": true}},
			"requiredOutput":         map[string]any{"mustRequestGovernance": true},
			stepInputKeyRuntimeStage: "governance",
			"targetSnapshots":        []map[string]any{{"id": "snapshot-001"}},
			"unmanagedFact":          "preserve",
		},
	}
	progress := &runModel.Run{
		ID:          current.ID,
		WorkspaceID: current.WorkspaceID,
		ExecutionContext: map[string]any{
			"targetSnapshots": []map[string]any{{"id": "snapshot-002"}},
		},
	}

	got := mergeRunProgressUpdate(current, progress)
	for _, key := range []string{
		stepInputKeyStagePacket,
		"convergenceContract",
		currentActionContractKey,
		"repairActionPackage",
		"requiredOutput",
		stepInputKeyRuntimeStage,
	} {
		if _, ok := got.ExecutionContext[key]; ok {
			t.Fatalf("ExecutionContext[%q] = %+v, want deleted when progress omits derived key", key, got.ExecutionContext[key])
		}
	}
	if got.ExecutionContext["unmanagedFact"] != "preserve" {
		t.Fatalf("unmanagedFact = %+v, want preserved", got.ExecutionContext["unmanagedFact"])
	}
	if snapshots := mapSliceFromValue(got.ExecutionContext["targetSnapshots"]); len(snapshots) != 1 || stringFromMap(snapshots[0], "id") != "snapshot-002" {
		t.Fatalf("targetSnapshots = %+v, want progress value", got.ExecutionContext["targetSnapshots"])
	}
}

func TestRefreshCurrentAuthorityDerivedSnapshotBuildsSingleStateChangeContract(t *testing.T) {
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	input := map[string]any{
		"convergenceContract":   map[string]any{"state": convergenceStateNeedEvidence},
		stepInputKeyStagePacket: map[string]any{"nextAction": map[string]any{"kind": "acquire_evidence"}},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteFailureEvidence": true,
				"acceptedKinds":                      []string{"file/read"},
				"forbiddenKinds":                     []string{"patch/apply"},
			},
		},
	}

	got, err := refreshCurrentAuthorityDerivedSnapshot(run, "step-state-change", input)
	if err != nil {
		t.Fatalf("refreshCurrentAuthorityDerivedSnapshot: %v", err)
	}
	if state := stringFromMap(mapFromValue(got["convergenceContract"]), "state"); state != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract.state = %q, want %q", state, convergenceStateNeedStateChange)
	}
	if action := mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["nextAction"]); firstNonEmptyString(stringFromMap(action, "type"), stringFromMap(action, "kind")) != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("stagePacket.nextAction = %+v, want state_change", action)
	}
	for label, required := range map[string]map[string]any{
		"requiredOutput":                       mapFromValue(got["requiredOutput"]),
		"repairActionPackage.requiredOutput":   mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":           mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["requiredOutput"]),
		"currentActionContract.requiredOutput": mapFromValue(mapFromValue(got[currentActionContractKey])["requiredOutput"]),
	} {
		if !requiredOutputHasAllowedStateChangingKind(required) {
			t.Fatalf("%s = %+v, want state-changing required output", label, required)
		}
	}
}

func TestRequiredOutputFocusNormalizationRewritesTargetRefsAndExamples(t *testing.T) {
	coverageFocus := map[string]any{
		"id":                  "focus-cwi-current",
		"coverageWorkItemIds": []string{"cwi-current"},
		"targetRefs":          []string{"goal.leaves[4]", "goal-req-current"},
		"structuredTargetRefs": []string{
			"cwi-current",
			"goal:goal_leaf:goal-req-current",
		},
	}
	required := map[string]any{
		"coverageFocusId":     "focus-cwi-stale",
		"coverageWorkItemIds": []string{"cwi-stale"},
		"targetRefs":          []string{"dod.items[19]"},
		"structuredTargetRefs": []string{
			"cwi-stale",
			"dod:dod_item:dod-stale",
		},
		"validationEvidenceNodeExample": map[string]any{
			"id":   "verify_behavior",
			"kind": "shell/command",
			"input": map[string]any{
				"validationEvidence": map[string]any{
					"mode":                 "runtime_behavior",
					"observationType":      "runtime_observation",
					"coverageWorkItemIds":  []string{"cwi-stale"},
					"targetRefs":           []string{"dod.items[19]"},
					"structuredTargetRefs": []string{"cwi-stale", "dod:dod_item:dod-stale"},
				},
			},
		},
		"managedValidationNodeExample": map[string]any{
			"id":   "verify_managed_behavior",
			"kind": managedValidationSessionNodeKind,
			"input": map[string]any{
				"observation": map[string]any{
					"validationEvidence": map[string]any{
						"mode":                 "runtime_behavior",
						"observationType":      "runtime_observation",
						"coverageWorkItemIds":  []string{"cwi-stale"},
						"targetRefs":           []string{"dod.items[19]"},
						"structuredTargetRefs": []string{"cwi-stale", "dod:dod_item:dod-stale"},
					},
				},
			},
		},
	}

	got := normalizeRequiredOutputFocusBinding(required, coverageFocus)
	if gotID := stringFromMap(got, "coverageFocusId"); gotID != "focus-cwi-current" {
		t.Fatalf("coverageFocusId = %q, want focus-cwi-current", gotID)
	}
	if ids := stringSliceFromValue(got["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != "cwi-current" {
		t.Fatalf("coverageWorkItemIds = %+v, want [cwi-current]", ids)
	}
	if refs := stringSliceFromValue(got["structuredTargetRefs"]); !stringSliceContains(refs, "cwi-current") || stringSliceContains(refs, "cwi-stale") {
		t.Fatalf("structuredTargetRefs = %+v, want current refs only", refs)
	}
	validationEvidence := mapFromValue(mapFromValue(mapFromValue(got["validationEvidenceNodeExample"])["input"])["validationEvidence"])
	assertValidationEvidenceFocusForTest(t, validationEvidence, "cwi-current", "goal:goal_leaf:goal-req-current")
	managedEvidence := mapFromValue(mapFromValue(mapFromValue(mapFromValue(got["managedValidationNodeExample"])["input"])["observation"])["validationEvidence"])
	assertValidationEvidenceFocusForTest(t, managedEvidence, "cwi-current", "goal:goal_leaf:goal-req-current")
}

func TestValidateConvergenceDispatchInputRejectsNestedRequiredOutputFocusDrift(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"currentFocus": map[string]any{
				"id":                  "focus-cwi-current",
				"coverageWorkItemIds": []string{"cwi-current"},
				"structuredTargetRefs": []string{
					"cwi-current",
					"goal:goal_leaf:goal-req-current",
				},
			},
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		"requiredOutput": map[string]any{
			"coverageFocusId":     "focus-cwi-current",
			"coverageWorkItemIds": []string{"cwi-current"},
			"structuredTargetRefs": []string{
				"cwi-current",
			},
			"validationEvidenceNodeExample": map[string]any{
				"id":   "verify_behavior",
				"kind": "shell/command",
				"input": map[string]any{
					"validationEvidence": map[string]any{
						"mode":                 "runtime_behavior",
						"observationType":      "runtime_observation",
						"coverageWorkItemIds":  []string{"cwi-stale"},
						"targetRefs":           []string{"dod.items[19]"},
						"structuredTargetRefs": []string{"cwi-stale", "dod:dod_item:dod-stale"},
					},
				},
			},
		},
		stepInputKeyStagePacket: map[string]any{
			"activeFocus": map[string]any{"id": "focus-cwi-current", "coverageWorkItemIds": []string{"cwi-current"}},
		},
	}
	run := &runModel.Run{ID: "run-stage-packet-mirror-mismatch", WorkspaceID: "ws-stage-packet-mirror-mismatch"}
	input = synchronizeCurrentRequiredOutputMirrors(input)
	input = attachCurrentActionContractToInput(run, "run-stage-packet-mirror-mismatch/step-001", input)
	stagePacket := mapFromValue(input[stepInputKeyStagePacket])
	stagePacket["requiredOutput"] = concreteValidationEvidenceRequiredOutputWithScratch(
		map[string]any{
			"coverageWorkItemIds":  []string{"cwi-stale"},
			"structuredTargetRefs": []string{"cwi-stale"},
		},
		nil,
		nil,
	)
	input[stepInputKeyStagePacket] = stagePacket

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want nested validation evidence focus mismatch")
	}
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), failureClassRuntimeContractMisalign) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want %s marker", err, failureClassRuntimeContractMisalign)
	}
}

func TestValidateConvergenceDispatchInputRejectsNextActionContractStateDrift(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus": map[string]any{
				"id":                   "cwi-current",
				"coverageWorkItemIds":  []string{"cwi-current"},
				"missingEvidenceKinds": []string{"validation", "state_change"},
			},
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
		},
		"convergenceContract": map[string]any{
			"state":           convergenceStateNeedValidation,
			"repairObjective": repairObjectiveConcreteValidationEvidence,
		},
		currentActionContractKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
		},
		"requiredOutput": map[string]any{
			"mustProduceConcreteValidationEvidence": true,
			"acceptedKinds":                         []string{"validation/managed_session"},
			"coverageWorkItemIds":                   []string{"cwi-current"},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want nextAction/contract state drift")
	}
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "nextAction=state_change contractState=need_validation") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want nextAction/contractState drift", err)
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

func TestRebuildDispatchInputRebindsStaleTopRequiredOutputToCurrentAuthority(t *testing.T) {
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeAcquireTargetSnapshot)
	input := map[string]any{
		convergenceAuthorityKey: cloneStepPayload(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])),
		gapBaselineKey:          cloneStepPayload(mapFromValue(run.ExecutionContext[gapBaselineKey])),
		"repairActionPackage": map[string]any{
			"requiresCurrentTargetSnapshotForStateChange": true,
			"repairTargetSet": []string{"target.txt"},
			"requiredOutput": map[string]any{
				"mustAcquireCurrentTargetSnapshot":   true,
				"mustUseReadOnlySnapshotAcquisition": true,
				"mustIncludeStateChangingNode":       false,
				"acceptedKinds":                      []string{"file/list", "file/read", "git/read"},
				"forbiddenKinds":                     []string{"patch/apply", "file/write", "shell/write"},
				"coverageWorkItemIds":                []string{"cwi-current"},
				"coverageFocusId":                    "focus-cwi-current",
			},
		},
		"requiredOutput": map[string]any{
			"mustIncludeStateChangingNode": true,
			"acceptedKinds":                []string{"patch/apply", "shell/command"},
			"coverageWorkItemIds":          []string{"cwi-current"},
			"coverageFocusId":              "focus-cwi-current",
		},
		currentActionContractKey: map[string]any{
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []string{"patch/apply"},
			},
		},
	}

	got, err := rebuildDispatchInputFromConvergenceAuthority(run, "step-acquire-target-snapshot", input)
	if err != nil {
		t.Fatalf("rebuildDispatchInputFromConvergenceAuthority: %v", err)
	}

	for label, required := range map[string]map[string]any{
		"requiredOutput":                       mapFromValue(got["requiredOutput"]),
		"repairActionPackage.requiredOutput":   mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":           mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["requiredOutput"]),
		"currentActionContract.requiredOutput": mapFromValue(mapFromValue(got[currentActionContractKey])["requiredOutput"]),
	} {
		if len(required) == 0 {
			t.Fatalf("%s = nil, want current read-only acquisition requiredOutput; input=%+v", label, got)
		}
		if boolValue(required["mustIncludeStateChangingNode"]) {
			t.Fatalf("%s = %+v, want read-only acquisition contract", label, required)
		}
		if !boolValue(required["mustAcquireCurrentTargetSnapshot"]) ||
			!boolValue(required["mustUseReadOnlySnapshotAcquisition"]) {
			t.Fatalf("%s = %+v, want target snapshot acquisition contract", label, required)
		}
		if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
			t.Fatalf("%s = %+v, want patch/apply removed from read-only acquisition", label, required)
		}
	}
	if err := validateConvergenceDispatchInput(got); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v", err)
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
	if err := workspaceRepo.Create(ctx, &workspaceModel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
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
		memory.NewApprovalRepository())

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

func TestStateChangeDispatchHydratesAcceptedTargetSnapshotsBeforeRebuild(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dispatch-state-change-snapshot")
	workspaceID := common.WorkspaceID("ws-dispatch-state-change-snapshot")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	patchRepo := memory.NewPatchRepository()
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	run.ID = runID
	run.WorkspaceID = workspaceID
	run.InitialStepKind = llmWorkspaceExecutionStepKind
	run.ExecutionContext["stateChangingOutputRequired"] = true
	run.ExecutionContext["repairActionPackage"] = map[string]any{
		"requiresCurrentTargetSnapshotForStateChange": true,
		"repairTargetSet": []string{"target.txt"},
	}
	updateAuthorityScopeForTest(run, workspaceID, runID)
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspaceModel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	sourceStep := &runModel.Step{
		ID:    "run-dispatch-state-change-snapshot/read-target",
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: "run-dispatch-state-change-snapshot/root",
			stepInputKeyGraphNodeID:     "read-target",
		},
		Output: map[string]any{
			"stage":            "file_read",
			"path":             "target.txt",
			"exists":           true,
			"content":          "current target content\n",
			"contentHash":      "hash-dispatch-snapshot",
			"contentComplete":  true,
			"safeForFileWrite": true,
			"truncated":        false,
			"isBinary":         false,
		},
	}
	scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	if err := stepRepo.Create(scopedCtx, sourceStep); err != nil {
		t.Fatalf("seed source step: %v", err)
	}
	record := acceptedTargetSnapshotEvidenceRecordForStep(run, sourceStep)
	if record == nil {
		t.Fatalf("accepted target snapshot evidence record = nil")
	}
	if err := evidenceRepo.Create(scopedCtx, record); err != nil {
		t.Fatalf("seed evidence record: %v", err)
	}
	records, err := evidenceRepo.ListByRun(scopedCtx, runID)
	if err != nil {
		t.Fatalf("list evidence records: %v", err)
	}
	if snapshots := acceptedTargetSnapshotsFromEvidenceRecords(scopedCtx, run, records, stepRepo, nil); len(snapshots) == 0 {
		t.Fatalf("acceptedTargetSnapshotsFromEvidenceRecords empty; record=%+v step=%+v", record, sourceStep)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		nil,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{EvidenceRecords: evidenceRepo, Patches: patchRepo})

	if !dispatchRequiresStateChangeTargetSnapshotHydration(run, nil) {
		t.Fatalf("dispatchRequiresStateChangeTargetSnapshotHydration = false; authority=%+v", run.ExecutionContext)
	}
	if err := orch.hydrateStateChangeTargetSnapshotsBeforeDispatch(ctx, run, nil); err != nil {
		t.Fatalf("hydrate target snapshots: %v", err)
	}
	got, err := rebuildDispatchInputFromConvergenceAuthority(run, "run-dispatch-state-change-snapshot/patch-target", map[string]any{})
	if err != nil {
		t.Fatalf("rebuildDispatchInputFromConvergenceAuthority: %v", err)
	}
	snapshots := mapSliceFromValue(mapFromValue(got["repairActionPackage"])["targetSnapshots"])
	if len(snapshots) != 1 || stringFromMap(snapshots[0], "targetRef") != "target.txt" {
		t.Fatalf("repairActionPackage.targetSnapshots = %+v, want accepted target snapshot for target.txt", snapshots)
	}
	reloadedRun, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if len(mapSliceFromValue(mapFromValue(reloadedRun.ExecutionContext["repairActionPackage"])["targetSnapshots"])) == 0 &&
		len(mapSliceFromValue(reloadedRun.ExecutionContext["targetSnapshots"])) == 0 {
		t.Fatalf("persisted run context = %+v, want dispatch hydration persisted before rebuild", reloadedRun.ExecutionContext)
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

func TestRepairFinalizationFailsFastOnAuthorityMisalignment(t *testing.T) {
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	input := map[string]any{
		convergenceAuthorityKey:       cloneStepPayload(mapFromValue(run.ExecutionContext[convergenceAuthorityKey])),
		gapBaselineKey:                cloneStepPayload(mapFromValue(run.ExecutionContext[gapBaselineKey])),
		"graphLocalDispatchAuthority": true,
		"graphLocalNextAction":        map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []string{"patch/apply"},
			},
		},
	}
	failed := &runModel.Step{ID: "run-authority-focus/failed-graph-local", RunID: run.ID}

	got := finalizeRuntimeRepairFollowUpStepInput(run, failed, input)
	if errText := strings.TrimSpace(stringFromMap(mapFromValue(got["runtimeInternalContractError"]), "error")); errText == "" {
		t.Fatalf("runtimeInternalContractError = %+v, want fail-fast authority misalignment", got["runtimeInternalContractError"])
	}
	if err := validateConvergenceDispatchInput(got); err == nil || !strings.Contains(err.Error(), "runtime_internal_contract_misalignment") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want internal contract misalignment rejection", err)
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

func TestRebuildDispatchInputPreemptsLocalAssessmentWhenOwnerNeedsStateChange(t *testing.T) {
	run := runWithAuthorityFocusForTest(runtimemodel.NextActionTypeStateChange)
	for _, key := range []string{convergenceAuthorityKey, gapBaselineKey} {
		authority := mapFromValue(run.ExecutionContext[key])
		focus := mapFromValue(authority["currentFocus"])
		focus["missingEvidenceKinds"] = []string{"validation", "state_change"}
		authority["currentFocus"] = focus
		run.ExecutionContext[key] = authority
	}
	input := map[string]any{
		stepInputKeyAssessmentOnly:     true,
		stepInputKeyGraphLimitFollowUp: true,
		"convergenceContract": map[string]any{
			"state":           convergenceStateNeedAssessment,
			"repairObjective": "complete_dod_goal_coverage_assessment",
		},
		currentActionContractKey: map[string]any{
			"nextAction": map[string]any{
				"kind": "assessment",
			},
		},
	}

	got, err := rebuildDispatchInputFromConvergenceAuthority(run, "step-stale-assessment", input)
	if err != nil {
		t.Fatalf("rebuildDispatchInputFromConvergenceAuthority: %v", err)
	}

	contract := mapFromValue(got["convergenceContract"])
	if state := stringFromMap(contract, "state"); state != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract.state = %q, want %s; input=%+v", state, convergenceStateNeedStateChange, got)
	}
	if got[stepInputKeyAssessmentOnly] != nil {
		t.Fatalf("%s = %v, want removed after latest owner authority preempts local assessment", stepInputKeyAssessmentOnly, got[stepInputKeyAssessmentOnly])
	}
	packet := mapFromValue(got[stepInputKeyStagePacket])
	nextAction := mapFromValue(packet["nextAction"])
	if kind := stringFromMap(nextAction, "type"); kind != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("stagePacket.nextAction = %+v, want state_change", nextAction)
	}
	required := mapFromValue(packet["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) {
		t.Fatalf("stagePacket.requiredOutput = %+v, want state-changing contract", required)
	}
	if err := validateConvergenceDispatchInput(got); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v", err)
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
	run.State = common.RunStateActive
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
	if err := workspaceRepo.Create(ctx, &workspaceModel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := NewRuntimeOrchestrator(nil, runRepo, memory.NewStepRepository(), workspaceRepo, memory.NewDecisionTicketRepository(), memory.NewApprovalRepository())

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

func TestCloseLoadedRunPersistsAuthorityNormalizedOwnerState(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-terminal-normalized-authority")
	workspaceID := common.WorkspaceID("ws-terminal-normalized-authority")
	run := runWithAuthorityFocusForTest("validate")
	run.ID = runID
	run.WorkspaceID = workspaceID
	run.State = common.RunStateActive
	run.ExecutionContext[coverageFocusKey] = map[string]any{
		"id":                  "focus-cwi-stale",
		"coverageWorkItemIds": []string{"cwi-stale"},
	}
	updateAuthorityScopeForTest(run, workspaceID, runID)

	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspaceModel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := NewRuntimeOrchestrator(nil, runRepo, memory.NewStepRepository(), workspaceRepo, memory.NewDecisionTicketRepository(), memory.NewApprovalRepository())

	if err := orch.closeLoadedRun(ctx, run, common.RunStateFailed, "terminal failure"); err != nil {
		t.Fatalf("closeLoadedRun: %v", err)
	}
	persisted, err := runRepo.GetByWorkspaceID(ctx, workspaceID, runID)
	if err != nil {
		t.Fatalf("load persisted run: %v", err)
	}
	if persisted.State != common.RunStateFailed {
		t.Fatalf("persisted state = %s, want failed", persisted.State)
	}
	assertFocusIDForTest(t, mapFromValue(persisted.ExecutionContext[coverageFocusKey]), "focus-cwi-current")
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

func TestValidateConvergenceDispatchInputRejectsGraphLocalMixedAuthority(t *testing.T) {
	input := map[string]any{
		"graphLocalDispatchAuthority": true,
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus":  map[string]any{"id": "cwi-current"},
			"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		gapBaselineKey: map[string]any{
			"schemaVersion": gapBaselineSchema,
			"currentFocus":  map[string]any{"id": "cwi-current"},
			"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"stateChangingOutputRequired": true,
		currentActionContractKey: map[string]any{
			"nextAction":     map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			"requiredOutput": stateChangingRepairRequiredOutput(),
		},
		"requiredOutput": stateChangingRepairRequiredOutput(),
		stepInputKeyStagePacket: map[string]any{
			"activeFocus": map[string]any{"id": "cwi-current"},
			"nextAction":  map[string]any{"type": runtimemodel.NextActionTypeStateChange},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want graph-local mixed authority conflict")
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

func TestValidateConvergenceDispatchInputRejectsRequiredOutputMirrorMismatch(t *testing.T) {
	input := map[string]any{
		"currentAuthorityRequiredOutputMirrorsCanonical": true,
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus": map[string]any{
				"id":                  "cwi-current",
				"coverageWorkItemIds": []string{"cwi-current"},
			},
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		"requiredOutput": map[string]any{
			"mustProduceConcreteValidationEvidence": true,
			"mustIncludeStateChangingNode":          false,
			"acceptedKinds":                         []string{"shell/command"},
			"forbiddenKinds":                        []string{"patch/apply"},
			"coverageWorkItemIds":                   []string{"cwi-current"},
			"structuredTargetRefs":                  []string{"cwi-current"},
			"targetRefs":                            []string{"cwi-current"},
			"validationEvidence": map[string]any{
				"targetRefs": []string{"cwi-current"},
			},
		},
		stepInputKeyStagePacket: map[string]any{
			"activeFocus": map[string]any{"id": "cwi-current"},
			"nextAction": map[string]any{
				"type": runtimemodel.NextActionTypeValidate,
			},
			"requiredOutput": map[string]any{
				"mustProduceConcreteValidationEvidence": true,
				"mustIncludeStateChangingNode":          false,
				"acceptedKinds":                         []string{"mcp/tool_call"},
				"forbiddenKinds":                        []string{"patch/apply"},
				"coverageWorkItemIds":                   []string{"cwi-current"},
				"structuredTargetRefs":                  []string{"cwi-current"},
				"targetRefs":                            []string{"cwi-current"},
				"validationEvidence": map[string]any{
					"targetRefs": []string{"cwi-current"},
				},
			},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want requiredOutput mirror mismatch")
	}
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "does not match current authority requiredOutput") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want mirror mismatch", err)
	}
	diagnostics := mapSliceFromValue(input["authorityDiagnostics"])
	if len(diagnostics) != 1 || stringFromMap(diagnostics[0], "code") != "required_output_mirror_mismatch" {
		t.Fatalf("authorityDiagnostics = %+v, want required_output_mirror_mismatch", diagnostics)
	}
}

func TestSynchronizeCurrentRequiredOutputMirrorsRebuildsDerivedMirrorsWithoutStaleDiagnostics(t *testing.T) {
	required := map[string]any{
		"mustProduceConcreteValidationEvidence": true,
		"mustIncludeStateChangingNode":          false,
		"acceptedKinds":                         []string{"shell/command"},
		"forbiddenKinds":                        []string{"patch/apply"},
	}
	stale := map[string]any{
		"mustIncludeStateChangingNode": true,
		"acceptedKinds":                []string{"patch/apply"},
	}
	input := map[string]any{
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		"repairActionPackage": map[string]any{"requiredOutput": cloneStepPayload(required)},
		"requiredOutput":      cloneStepPayload(stale),
		stepInputKeyStagePacket: map[string]any{
			"requiredOutput": cloneStepPayload(stale),
			"nextAction": map[string]any{
				"kind":           runtimemodel.NextActionTypeValidate,
				"requiredOutput": cloneStepPayload(stale),
			},
		},
		"executionContext": map[string]any{
			"requiredOutput": cloneStepPayload(stale),
			stepInputKeyStagePacket: map[string]any{
				"requiredOutput": cloneStepPayload(stale),
				"nextAction": map[string]any{
					"requiredOutput": cloneStepPayload(stale),
				},
			},
		},
	}

	got := synchronizeCurrentRequiredOutputMirrors(input)
	canonical := mapFromValue(got["requiredOutput"])
	if len(canonical) == 0 {
		t.Fatal("requiredOutput is empty after mirror synchronization")
	}
	wantJSON := stableCoverageJSON(canonical)
	executionContext := mapFromValue(got["executionContext"])
	for label, mirror := range map[string]map[string]any{
		"repairActionPackage.requiredOutput":                     mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":                             mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["requiredOutput"]),
		"stagePacket.nextAction.requiredOutput":                  mapFromValue(mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
		"executionContext.requiredOutput":                        mapFromValue(executionContext["requiredOutput"]),
		"executionContext.stagePacket.requiredOutput":            mapFromValue(mapFromValue(executionContext[stepInputKeyStagePacket])["requiredOutput"]),
		"executionContext.stagePacket.nextAction.requiredOutput": mapFromValue(mapFromValue(mapFromValue(executionContext[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
	} {
		if stableCoverageJSON(mirror) != wantJSON {
			t.Fatalf("%s = %+v, want canonical requiredOutput %+v", label, mirror, canonical)
		}
	}
	if diagnostics := mapSliceFromValue(got["authorityDiagnostics"]); len(diagnostics) != 0 {
		t.Fatalf("authorityDiagnostics = %+v, want derived mirror rebuild without stale diagnostic noise", diagnostics)
	}
}

func TestAppendAuthorityDiagnosticDeduplicatesExactDiagnostic(t *testing.T) {
	input := map[string]any{"seed": true}
	appendAuthorityDiagnostic(input, "contract_mismatch", "error", "internal_authority_mismatch", "requiredOutput", "contract mismatch")
	appendAuthorityDiagnostic(input, "contract_mismatch", "error", "internal_authority_mismatch", "requiredOutput", "contract mismatch")
	appendAuthorityDiagnostic(input, "contract_mismatch", "error", "internal_authority_mismatch", "stagePacket", "contract mismatch")

	diagnostics := mapSliceFromValue(input["authorityDiagnostics"])
	if len(diagnostics) != 2 {
		t.Fatalf("authorityDiagnostics = %+v, want exact duplicates collapsed while distinct sources remain", diagnostics)
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

func TestValidateConvergenceDispatchInputRejectsValidationWithoutRequiredOutputRoute(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"currentFocus": map[string]any{
				"id":                  "cwi-current",
				"coverageWorkItemIds": []string{"cwi-current"},
				"missingEvidenceKinds": []string{
					runtimemodel.EvidenceKindValidation,
				},
			},
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		stepInputKeyStagePacket: map[string]any{
			"activeFocus": map[string]any{"id": "cwi-current"},
			"nextAction":  map[string]any{"type": runtimemodel.NextActionTypeValidate},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want validation requiredOutput route rejection")
	}
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "validation missing requiredOutput") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want validation missing requiredOutput marker", err)
	}
	diagnostics := mapSliceFromValue(input["authorityDiagnostics"])
	if len(diagnostics) != 1 || stringFromMap(diagnostics[0], "code") != "required_output_mismatch_next_action" {
		t.Fatalf("authorityDiagnostics = %+v, want required_output_mismatch_next_action", diagnostics)
	}
}

func TestValidateConvergenceDispatchInputRejectsStagePacketNextActionDrift(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"currentFocus": map[string]any{"id": "cwi-current"},
			"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		"requiredOutput": map[string]any{
			"mustProduceConcreteValidationEvidence": true,
			"mustNotMutateWorkspace":                true,
			"acceptedKinds":                         []string{"shell/command"},
		},
		stepInputKeyStagePacket: map[string]any{
			"activeFocus": map[string]any{"id": "cwi-current"},
			"nextAction":  map[string]any{"type": runtimemodel.NextActionTypeStateChange},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want stagePacket nextAction drift")
	}
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "stage_packet_mismatch_next_action") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want stage_packet_mismatch_next_action", err)
	}
	if diagnostics := mapSliceFromValue(input["authorityDiagnostics"]); len(diagnostics) != 1 || stringFromMap(diagnostics[0], "code") != "stage_packet_mismatch_next_action" {
		t.Fatalf("authorityDiagnostics = %+v, want stage_packet_mismatch_next_action", input["authorityDiagnostics"])
	}
}

func TestValidateConvergenceDispatchInputRejectsAcquireTargetSnapshotWithoutReadOnlyRequiredOutput(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"currentFocus": map[string]any{"id": "cwi-current"},
			"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeAcquireTargetSnapshot},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedEvidence},
		"requiredOutput": map[string]any{
			"acceptedKinds": []string{"file/read"},
		},
		stepInputKeyStagePacket: map[string]any{
			"activeFocus": map[string]any{"id": "cwi-current"},
			"nextAction":  map[string]any{"type": runtimemodel.NextActionTypeAcquireTargetSnapshot},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want acquire_target_snapshot read-only requiredOutput drift")
	}
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "acquire_target_snapshot requires read-only requiredOutput") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want acquire_target_snapshot read-only marker", err)
	}
	if diagnostics := mapSliceFromValue(input["authorityDiagnostics"]); len(diagnostics) != 1 || stringFromMap(diagnostics[0], "code") != "required_output_mismatch_next_action" {
		t.Fatalf("authorityDiagnostics = %+v, want required_output_mismatch_next_action", input["authorityDiagnostics"])
	}
}

func TestValidateConvergenceDispatchInputRejectsExecutionContextRequiredOutputDrift(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"currentFocus": map[string]any{"id": "cwi-current"},
			"nextAction":   map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		"requiredOutput": map[string]any{
			"mustProduceConcreteValidationEvidence": true,
			"mustNotMutateWorkspace":                true,
			"acceptedKinds":                         []string{"shell/command"},
		},
		stepInputKeyStagePacket: map[string]any{
			"activeFocus": map[string]any{"id": "cwi-current"},
			"nextAction":  map[string]any{"type": runtimemodel.NextActionTypeValidate},
		},
		"executionContext": map[string]any{
			stepInputKeyStagePacket: map[string]any{
				"activeFocus": map[string]any{"id": "cwi-current"},
				"nextAction": map[string]any{
					"type": runtimemodel.NextActionTypeValidate,
					"requiredOutput": map[string]any{
						"mustIncludeStateChangingNode": true,
						"acceptedKinds":                []string{"patch/apply"},
					},
				},
			},
		},
	}

	err := validateConvergenceDispatchInput(input)
	if err == nil {
		t.Fatal("validateConvergenceDispatchInput returned nil, want executionContext requiredOutput drift")
	}
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "requiredOutput state-changing mismatch") {
		t.Fatalf("validateConvergenceDispatchInput error = %v, want requiredOutput state-changing mismatch", err)
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

func TestFinalizeLLMDispatchAuthorityInputRebuildsStaleStagePacket(t *testing.T) {
	run := &runModel.Run{ID: "run-final-stage-packet", WorkspaceID: "ws-final-stage-packet"}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus": map[string]any{
				"id":                  "focus-cwi-current",
				"coverageWorkItemIds": []string{"cwi-current"},
			},
			"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeValidate},
		},
		"convergenceContract": map[string]any{"state": convergenceStateNeedValidation},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"coverageFocusId":                       "focus-cwi-stale",
				"coverageWorkItemIds":                   []string{"cwi-stale"},
				"mustProduceConcreteValidationEvidence": true,
				"acceptedKinds":                         []string{"validation/managed_session"},
			},
		},
		stepInputKeyStagePacket: map[string]any{
			"schemaVersion": "workorch.stage_packet.v1",
			"activeFocus": map[string]any{
				"id":                  "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
			"requiredOutput": map[string]any{
				"coverageFocusId":     "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
		},
	}

	got, err := finalizeLLMDispatchAuthorityInput(run, "run-final-stage-packet/step-001", input)
	if err != nil {
		t.Fatalf("finalizeLLMDispatchAuthorityInput returned error = %v", err)
	}
	packet := mapFromValue(got[stepInputKeyStagePacket])
	assertFocusIDForTest(t, mapFromValue(packet["activeFocus"]), "focus-cwi-current")
	for label, required := range map[string]map[string]any{
		"requiredOutput":                        mapFromValue(got["requiredOutput"]),
		"repairActionPackage.requiredOutput":    mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":            mapFromValue(packet["requiredOutput"]),
		"stagePacket.nextAction.requiredOutput": mapFromValue(mapFromValue(packet["nextAction"])["requiredOutput"]),
		"currentActionContract.requiredOutput":  mapFromValue(mapFromValue(got[currentActionContractKey])["requiredOutput"]),
	} {
		if stringFromMap(required, "coverageFocusId") != "focus-cwi-current" {
			t.Fatalf("%s = %+v, want current focus", label, required)
		}
		if ids := stringSliceFromValue(required["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != "cwi-current" {
			t.Fatalf("%s.coverageWorkItemIds = %+v, want [cwi-current]", label, ids)
		}
	}
}

func TestFinalizeLLMDispatchAuthorityInputRebuildsExecutionContextStateChangeAuthority(t *testing.T) {
	focus := map[string]any{
		"id":                    "cwi-current",
		"activeCoverageFocusId": "focus-cwi-current",
		"coverageWorkItemIds":   []string{"cwi-current"},
		"missingEvidenceKinds":  []string{"validation", "state_change"},
		"requiredEvidenceKinds": []string{"validation", "state_change"},
		"status":                "open",
	}
	run := &runModel.Run{
		ID:          "run-final-state-change",
		WorkspaceID: "ws-final-state-change",
		State:       common.RunStateActive,
		ExecutionContext: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"currentFocus":  cloneStepPayload(focus),
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			},
			gapBaselineKey: map[string]any{
				"schemaVersion": gapBaselineSchema,
				"currentFocus":  cloneStepPayload(focus),
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			},
			coverageWorkItemContextKey: map[string]any{
				"currentFocus":    cloneStepPayload(focus),
				"unresolvedItems": []map[string]any{cloneStepPayload(focus)},
			},
		},
	}
	input := map[string]any{
		"executionContext": cloneStepPayload(run.ExecutionContext),
		"convergenceContract": map[string]any{
			"state":                           convergenceStateNeedEvidence,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"concreteFailureEvidenceRequired": true,
			"runtimeStage":                    runtimeStageRepair,
			"runtimeStageMode":                "strict_convergence",
		},
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		"repairActionPackage": map[string]any{
			"evidenceAcquisitionRequired": true,
			"requiredOutput": map[string]any{
				"mustProduceConcreteFailureEvidence": true,
				"acceptedKinds":                      []string{"file/read", "file/list", "shell/command"},
				"forbiddenKinds":                     []string{"patch/apply", "patch/preview"},
			},
		},
		stepInputKeyStagePacket: map[string]any{
			"schemaVersion": "workorch.stage_packet.v1",
			"nextAction": map[string]any{
				"kind": "acquire_evidence",
			},
			"requiredOutput": map[string]any{
				"mustProduceConcreteFailureEvidence": true,
				"acceptedKinds":                      []string{"file/read", "file/list", "shell/command"},
				"forbiddenKinds":                     []string{"patch/apply", "patch/preview"},
			},
		},
	}

	got, err := finalizeLLMDispatchAuthorityInput(run, "run-final-state-change/step-001", input)
	if err != nil {
		t.Fatalf("finalizeLLMDispatchAuthorityInput returned error = %v; input=%+v", err, got)
	}
	if state := stringFromMap(mapFromValue(got["convergenceContract"]), "state"); state != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract.state = %q, want %s; input=%+v", state, convergenceStateNeedStateChange, got)
	}
	packetNext := mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["nextAction"])
	if kind := firstNonEmptyString(stringFromMap(packetNext, "type"), stringFromMap(packetNext, "kind")); kind != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("stagePacket.nextAction = %+v, want state_change", packetNext)
	}
	currentNext := mapFromValue(mapFromValue(got[currentActionContractKey])["nextAction"])
	if kind := stringFromMap(currentNext, "kind"); kind != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("currentActionContract.nextAction = %+v, want state_change", currentNext)
	}
	for label, required := range map[string]map[string]any{
		"requiredOutput":                       mapFromValue(got["requiredOutput"]),
		"repairActionPackage.requiredOutput":   mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":           mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["requiredOutput"]),
		"currentActionContract.requiredOutput": mapFromValue(mapFromValue(got[currentActionContractKey])["requiredOutput"]),
	} {
		if !requiredOutputHasAllowedStateChangingKind(required) {
			t.Fatalf("%s = %+v, want state-changing accepted kind", label, required)
		}
		if stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), "patch/apply") {
			t.Fatalf("%s.forbiddenKinds = %+v, want patch/apply not forbidden", label, required["forbiddenKinds"])
		}
	}
}

func TestPrepareGapBaselineDispatchSpecsRebuildsStaleStateChangeRequiredOutputMirrors(t *testing.T) {
	focus := map[string]any{
		"id":                    "cwi-current",
		"activeCoverageFocusId": "focus-cwi-current",
		"coverageWorkItemIds":   []string{"cwi-current"},
		"missingEvidenceKinds":  []string{"validation", "state_change"},
		"requiredEvidenceKinds": []string{"validation", "state_change"},
		"status":                "open",
	}
	run := &runModel.Run{
		ID:          "run-prepare-state-change",
		WorkspaceID: "ws-prepare-state-change",
		State:       common.RunStateActive,
		ExecutionContext: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"currentFocus":  cloneStepPayload(focus),
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			},
			gapBaselineKey: map[string]any{
				"schemaVersion": gapBaselineSchema,
				"currentFocus":  cloneStepPayload(focus),
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			},
			coverageWorkItemContextKey: map[string]any{
				"currentFocus":    cloneStepPayload(focus),
				"unresolvedItems": []map[string]any{cloneStepPayload(focus)},
			},
			"convergenceContract": map[string]any{
				"state":                       convergenceStateNeedStateChange,
				"runtimeStage":                runtimeStageRepair,
				"runtimeStageMode":            "strict_convergence",
				"repairObjective":             repairObjectiveStateChangingOutput,
				"stateChangingOutputRequired": true,
			},
			"requiredOutput": stateChangingRepairRequiredOutput(),
		},
	}
	staleValidationRequired := map[string]any{
		"mustProduceConcreteValidationEvidence": true,
		"acceptedKinds":                         []string{"shell/command", "validation/managed_session"},
		"forbiddenKinds":                        []string{"patch/apply", "git/write"},
	}
	specs, err := (&RuntimeOrchestrator{}).prepareGapBaselineDispatchSpecs(context.Background(), run, []stepDispatchSpec{{
		ID:   "run-prepare-state-change/step-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"convergenceContract": map[string]any{
				"state":        convergenceStateNeedValidation,
				"runtimeStage": runtimeStageVerification,
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": cloneStepPayload(staleValidationRequired),
			},
			stepInputKeyStagePacket: map[string]any{
				"schemaVersion": "workorch.stage_packet.v1",
				"nextAction": map[string]any{
					"kind":           runtimemodel.NextActionTypeValidate,
					"requiredOutput": cloneStepPayload(staleValidationRequired),
				},
				"requiredOutput": cloneStepPayload(staleValidationRequired),
			},
			"executionContext": map[string]any{
				"requiredOutput": cloneStepPayload(staleValidationRequired),
				"repairActionPackage": map[string]any{
					"requiredOutput": cloneStepPayload(staleValidationRequired),
				},
				stepInputKeyStagePacket: map[string]any{
					"nextAction": map[string]any{
						"kind":           runtimemodel.NextActionTypeValidate,
						"requiredOutput": cloneStepPayload(staleValidationRequired),
					},
					"requiredOutput": cloneStepPayload(staleValidationRequired),
				},
			},
			"requiredOutput": cloneStepPayload(staleValidationRequired),
		},
	}})
	if err != nil {
		t.Fatalf("prepareGapBaselineDispatchSpecs returned error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("prepared specs = %d, want 1", len(specs))
	}
	input := specs[0].Input
	if err := validateConvergenceDispatchInput(input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput returned error = %v; input=%+v", err, input)
	}
	for label, required := range map[string]map[string]any{
		"requiredOutput":                                         mapFromValue(input["requiredOutput"]),
		"repairActionPackage.requiredOutput":                     mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":                             mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		"stagePacket.nextAction.requiredOutput":                  mapFromValue(mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
		"currentActionContract.requiredOutput":                   mapFromValue(mapFromValue(input[currentActionContractKey])["requiredOutput"]),
		"executionContext.requiredOutput":                        mapFromValue(mapFromValue(input["executionContext"])["requiredOutput"]),
		"executionContext.repairActionPackage.requiredOutput":    mapFromValue(mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"])["requiredOutput"]),
		"executionContext.stagePacket.requiredOutput":            mapFromValue(mapFromValue(mapFromValue(input["executionContext"])[stepInputKeyStagePacket])["requiredOutput"]),
		"executionContext.stagePacket.nextAction.requiredOutput": mapFromValue(mapFromValue(mapFromValue(mapFromValue(input["executionContext"])[stepInputKeyStagePacket])["nextAction"])["requiredOutput"]),
	} {
		if !requiredOutputRequiresStateChanging(required) || !requiredOutputHasAllowedStateChangingKind(required) {
			t.Fatalf("%s = %+v, want canonical state-changing requiredOutput", label, required)
		}
	}
}

func TestInitialStepInputFinalizesReplanSuccessorStateChangeRequiredOutput(t *testing.T) {
	focus := map[string]any{
		"id":                    "cwi-current",
		"activeCoverageFocusId": "focus-cwi-current",
		"coverageWorkItemIds":   []string{"cwi-current"},
		"missingEvidenceKinds":  []string{"state_change"},
		"requiredEvidenceKinds": []string{"state_change"},
		"status":                "open",
	}
	run := &runModel.Run{
		ID:              "run-replan-successor-state-change",
		WorkspaceID:     "ws-replan-successor-state-change",
		State:           common.RunStateActive,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"currentFocus":  cloneStepPayload(focus),
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			},
			gapBaselineKey: map[string]any{
				"schemaVersion": gapBaselineSchema,
				"currentFocus":  cloneStepPayload(focus),
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			},
			coverageWorkItemContextKey: map[string]any{
				"currentFocus":    cloneStepPayload(focus),
				"unresolvedItems": []map[string]any{cloneStepPayload(focus)},
			},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-predecessor",
				"reason":           "decision_replan",
				stepInputKeyProcessEvidenceTimeline: []map[string]any{{
					"stepId":              "run-predecessor/step-1",
					"kind":                "shell/command",
					failureOutputKeyClass: failureClassShellCommand,
					"failureReason":       "previous state-changing attempt failed",
				}},
			},
		},
	}

	input := initialStepInput(run)

	if err := validateConvergenceDispatchInput(input); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, input)
	}
	if state := stringFromMap(mapFromValue(input["convergenceContract"]), "state"); state != convergenceStateNeedEvidence {
		t.Fatalf("convergenceContract.state = %q, want %s; input=%+v", state, convergenceStateNeedEvidence, input)
	}
	nextAction := mapFromValue(mapFromValue(input[currentActionContractKey])["nextAction"])
	if kind := stringFromMap(nextAction, "kind"); kind != runtimemodel.NextActionTypeAcquireTargetSnapshot {
		t.Fatalf("currentAction.nextAction = %+v, want acquire_target_snapshot; input=%+v", nextAction, input)
	}
	for label, required := range map[string]map[string]any{
		"requiredOutput":                       mapFromValue(input["requiredOutput"]),
		"repairActionPackage.requiredOutput":   mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":           mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
		"currentActionContract.requiredOutput": mapFromValue(mapFromValue(input[currentActionContractKey])["requiredOutput"]),
	} {
		if !requiredOutputIsReadOnlyTargetSnapshotPrerequisite(required) {
			t.Fatalf("%s = %+v, want read-only target snapshot acquisition requiredOutput", label, required)
		}
	}
}

func TestFinalizeRuntimeRepairFollowUpInputRebuildsStateChangeAuthority(t *testing.T) {
	focus := map[string]any{
		"id":                    "cwi-current",
		"activeCoverageFocusId": "focus-cwi-current",
		"coverageWorkItemIds":   []string{"cwi-current"},
		"missingEvidenceKinds":  []string{"validation", "state_change"},
		"requiredEvidenceKinds": []string{"validation", "state_change"},
		"status":                "open",
	}
	run := &runModel.Run{
		ID:          "run-repair-state-change",
		WorkspaceID: "ws-repair-state-change",
		State:       common.RunStateActive,
		ExecutionContext: map[string]any{
			convergenceAuthorityKey: map[string]any{
				"schemaVersion": convergenceAuthoritySchema,
				"currentFocus":  cloneStepPayload(focus),
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			},
			gapBaselineKey: map[string]any{
				"schemaVersion": gapBaselineSchema,
				"currentFocus":  cloneStepPayload(focus),
				"nextAction":    map[string]any{"kind": runtimemodel.NextActionTypeStateChange},
			},
			coverageWorkItemContextKey: map[string]any{
				"currentFocus":    cloneStepPayload(focus),
				"unresolvedItems": []map[string]any{cloneStepPayload(focus)},
			},
		},
	}
	failed := &runModel.Step{
		ID:    "run-repair-state-change/failed-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
	}
	input := map[string]any{
		"executionContext": cloneStepPayload(run.ExecutionContext),
		"convergenceContract": map[string]any{
			"state":                           convergenceStateNeedEvidence,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"concreteFailureEvidenceRequired": true,
			"runtimeStage":                    runtimeStageRepair,
			"runtimeStageMode":                "strict_convergence",
		},
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		"repairActionPackage": map[string]any{
			"evidenceAcquisitionRequired": true,
			"requiredOutput": map[string]any{
				"mustProduceConcreteFailureEvidence": true,
				"acceptedKinds":                      []string{"file/read", "file/list", "shell/command"},
				"forbiddenKinds":                     []string{"patch/apply", "patch/preview"},
			},
		},
		stepInputKeyStagePacket: map[string]any{
			"schemaVersion": "workorch.stage_packet.v1",
			"nextAction": map[string]any{
				"kind": "acquire_evidence",
			},
			"requiredOutput": map[string]any{
				"mustProduceConcreteFailureEvidence": true,
				"acceptedKinds":                      []string{"file/read", "file/list", "shell/command"},
				"forbiddenKinds":                     []string{"patch/apply", "patch/preview"},
			},
		},
	}

	got := finalizeRuntimeRepairFollowUpStepInput(run, failed, input)
	if state := stringFromMap(mapFromValue(got["convergenceContract"]), "state"); state != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract.state = %q, want %s; input=%+v", state, convergenceStateNeedStateChange, got)
	}
	packetNext := mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["nextAction"])
	if kind := firstNonEmptyString(stringFromMap(packetNext, "type"), stringFromMap(packetNext, "kind")); kind != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("stagePacket.nextAction = %+v, want state_change", packetNext)
	}
	for label, required := range map[string]map[string]any{
		"requiredOutput":                       mapFromValue(got["requiredOutput"]),
		"repairActionPackage.requiredOutput":   mapFromValue(mapFromValue(got["repairActionPackage"])["requiredOutput"]),
		"stagePacket.requiredOutput":           mapFromValue(mapFromValue(got[stepInputKeyStagePacket])["requiredOutput"]),
		"currentActionContract.requiredOutput": mapFromValue(mapFromValue(got[currentActionContractKey])["requiredOutput"]),
	} {
		if !requiredOutputHasAllowedStateChangingKind(required) {
			t.Fatalf("%s = %+v, want state-changing accepted kind", label, required)
		}
		if stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), "patch/apply") {
			t.Fatalf("%s.forbiddenKinds = %+v, want patch/apply not forbidden", label, required["forbiddenKinds"])
		}
	}
	if err := validateConvergenceDispatchInput(got); err != nil {
		t.Fatalf("validateConvergenceDispatchInput: %v; input=%+v", err, got)
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
		State:       common.RunStateActive,
	}
	runtime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "completed"}}
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspaceModel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, memory.NewDecisionTicketRepository(), memory.NewApprovalRepository())

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
		State:       common.RunStateActive,
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

func assertValidationEvidenceFocusForTest(t *testing.T, evidence map[string]any, wantWorkItemID string, wantStructuredRef string) {
	t.Helper()
	if ids := stringSliceFromValue(evidence["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != wantWorkItemID {
		t.Fatalf("validationEvidence.coverageWorkItemIds = %+v, want [%s]", ids, wantWorkItemID)
	}
	if refs := stringSliceFromValue(evidence["structuredTargetRefs"]); !stringSliceContains(refs, wantStructuredRef) || stringSliceContains(refs, "cwi-stale") {
		t.Fatalf("validationEvidence.structuredTargetRefs = %+v, want current structured refs", refs)
	}
	if refs := stringSliceFromValue(evidence["targetRefs"]); !stringSliceContains(refs, "goal.leaves[4]") || stringSliceContains(refs, "dod.items[19]") {
		t.Fatalf("validationEvidence.targetRefs = %+v, want current target refs", refs)
	}
}
