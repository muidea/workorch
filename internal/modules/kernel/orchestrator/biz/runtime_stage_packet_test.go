package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestInitialStepInputAttachesMinimalStagePacket(t *testing.T) {
	run := &runModel.Run{
		ID:                 common.RunID("run-stage-packet"),
		WorkspaceID:        common.WorkspaceID("ws-stage-packet"),
		GoalID:             common.GoalID("goal-stage-packet"),
		DoDID:              common.DoDID("dod-stage-packet"),
		DefinitionRevision: "rev-stage-packet",
		InitialStepKind:    llmWorkspaceExecutionStepKind,
		State:              common.RunStateRunning,
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-stage-packet",
			"repairActionPackage": map[string]any{
				coverageWorkItemContextKey: map[string]any{
					"definitionRevision": "rev-stage-packet",
					"currentFocus": map[string]any{
						"id":                   "cwi-stage-packet",
						"targetText":           "neutral target",
						"acceptedEvidenceRefs": []string{"accepted-stage-packet"},
						"rejectedEvidenceRefs": []string{"rejected-stage-packet"},
					},
				},
				"requiredOutput": map[string]any{
					"mustBindCoverageFocus":        true,
					"mustIncludeStateChangingNode": true,
				},
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target-full",
					"contentHash":      "sha256:full",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}, {
					"targetRef":        "target-truncated",
					"contentComplete":  false,
					"safeForFileWrite": true,
					"truncated":        true,
				}},
			},
		},
	}

	input := initialStepInput(run)
	packet := mapFromValue(input[stepInputKeyStagePacket])
	if len(packet) == 0 {
		t.Fatalf("stagePacket missing from input: %+v", input)
	}
	definition := mapFromValue(packet["definition"])
	if definition["revision"] != "rev-stage-packet" || definition["goalId"] != "goal-stage-packet" || definition["doDId"] != "dod-stage-packet" {
		t.Fatalf("stagePacket.definition = %+v, want revision and ids", definition)
	}
	focus := mapFromValue(packet["activeFocus"])
	if focus["id"] != "cwi-stage-packet" {
		t.Fatalf("stagePacket.activeFocus = %+v, want current focus", focus)
	}
	required := mapFromValue(packet["requiredOutput"])
	if required["mustBindCoverageFocus"] != true || required["mustIncludeStateChangingNode"] != true {
		t.Fatalf("stagePacket.requiredOutput = %+v, want state-changing focus contract", required)
	}
	if refs := stringSliceFromValue(packet["acceptedEvidenceRefs"]); len(refs) != 1 || refs[0] != "accepted-stage-packet" {
		t.Fatalf("stagePacket.acceptedEvidenceRefs = %+v, want accepted ref", refs)
	}
	if refs := stringSliceFromValue(packet["rejectedEvidenceRefs"]); len(refs) != 1 || refs[0] != "rejected-stage-packet" {
		t.Fatalf("stagePacket.rejectedEvidenceRefs = %+v, want rejected ref", refs)
	}
	snapshots := mapSliceFromValue(packet["targetSnapshots"])
	if len(snapshots) != 2 {
		t.Fatalf("stagePacket.targetSnapshots = %+v, want two snapshots", snapshots)
	}
	if snapshots[0]["integrityState"] != "full" || snapshots[0]["safeForExactFileWrite"] != true {
		t.Fatalf("stagePacket full snapshot = %+v, want exact-write safe", snapshots[0])
	}
	if snapshots[1]["integrityState"] != "truncated" ||
		snapshots[1]["safeForExactFileWrite"] != false ||
		snapshots[1]["safeForFileWrite"] != false {
		t.Fatalf("stagePacket truncated snapshot = %+v, want unsafe truncated metadata", snapshots[1])
	}
}

func TestStagePacketActiveFocusExposesStructuredCoverageRefs(t *testing.T) {
	run := &runModel.Run{
		ID:              common.RunID("run-stage-packet-structured-focus"),
		WorkspaceID:     common.WorkspaceID("ws-stage-packet-structured-focus"),
		InitialStepKind: llmWorkspaceExecutionStepKind,
		State:           common.RunStateRunning,
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-stage-packet-structured-focus",
			"dod": map[string]any{
				"id":    "dod-stage-packet-structured-focus",
				"items": []string{"natural language validation target must stay out of binding refs"},
			},
		},
	}

	input := initialStepInput(run)
	packet := mapFromValue(input[stepInputKeyStagePacket])
	focus := mapFromValue(packet["activeFocus"])
	if len(focus) == 0 {
		t.Fatalf("stagePacket.activeFocus missing: %+v", packet)
	}
	if refs := stringSliceFromValue(focus["coverageWorkItemIds"]); len(refs) != 1 || refs[0] != "cwi-001" {
		t.Fatalf("activeFocus.coverageWorkItemIds = %+v, want cwi-001", focus["coverageWorkItemIds"])
	}
	structured := stringSliceFromValue(focus["structuredTargetRefs"])
	if !stringSliceContains(structured, "cwi-001") ||
		!stringSliceContains(structured, "dod-stage-packet-structured-focus-dod-item-001") ||
		!stringSliceContains(structured, "dod:dod_item:dod-stage-packet-structured-focus-dod-item-001") {
		t.Fatalf("activeFocus.structuredTargetRefs = %+v, want cwi/source/normalized refs", structured)
	}
	targetRefs := stringSliceFromValue(focus["targetRefs"])
	if !stringSliceContains(targetRefs, "cwi-001") {
		t.Fatalf("activeFocus.targetRefs = %+v, want structured cwi ref", targetRefs)
	}
	if stringSliceContains(targetRefs, "natural language validation target must stay out of binding refs") {
		t.Fatalf("activeFocus.targetRefs = %+v, want natural language targetText excluded", targetRefs)
	}
}

func TestStagePacketUsesOnlyAuthorityCurrentFocusWhenStaleFocusExists(t *testing.T) {
	run := &runModel.Run{
		ID:                 common.RunID("run-stage-packet-authority"),
		WorkspaceID:        common.WorkspaceID("ws-stage-packet-authority"),
		GoalID:             common.GoalID("goal-stage-packet-authority"),
		DoDID:              common.DoDID("dod-stage-packet-authority"),
		DefinitionRevision: "rev-stage-packet-authority",
		State:              common.RunStateRunning,
	}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus": map[string]any{
				"id":                   "cwi-current",
				"targetText":           "current neutral target",
				"missingEvidenceKinds": []string{"validation"},
			},
		},
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": map[string]any{
				"id":         "cwi-stale",
				"targetText": "stale target must not become active",
			},
			"currentFocusItems": []map[string]any{{
				"id":         "cwi-stale-list",
				"targetText": "stale list target must not become active",
			}},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"coverageFocusId":     "focus-cwi-stale",
				"coverageWorkItemIds": []string{"cwi-stale"},
			},
		},
	}

	packet := buildStagePacketFromStepInput(run, "step-stage-packet-authority", input).ToMap()

	focus := mapFromValue(packet["activeFocus"])
	if focus["id"] != "cwi-current" || focus["targetText"] != "current neutral target" {
		t.Fatalf("stagePacket.activeFocus = %+v, want authority current focus", focus)
	}
	required := mapFromValue(packet["requiredOutput"])
	if required["coverageFocusId"] != "focus-cwi-current" {
		t.Fatalf("stagePacket.requiredOutput = %+v, want rebound authority focus", required)
	}
	if ids := stringSliceFromValue(required["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != "cwi-current" {
		t.Fatalf("stagePacket.requiredOutput.coverageWorkItemIds = %+v, want [cwi-current]", ids)
	}
	encoded, err := json.Marshal(packet)
	if err != nil {
		t.Fatalf("marshal stage packet: %v", err)
	}
	if strings.Contains(string(encoded), "cwi-stale") {
		t.Fatalf("stagePacket leaked stale focus: %s", encoded)
	}
}

func TestStagePacketCarriesCurrentNextActionAuthority(t *testing.T) {
	run := &runModel.Run{
		ID:          common.RunID("run-stage-packet-next-action"),
		WorkspaceID: common.WorkspaceID("ws-stage-packet-next-action"),
		State:       common.RunStateRunning,
	}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus": map[string]any{
				"id":                  "focus-cwi-current",
				"coverageWorkItemIds": []string{"cwi-current"},
			},
			"nextAction": map[string]any{
				"kind":                  runtimemodel.NextActionTypeValidate,
				"stage":                 runtimemodel.NextActionTypeValidate,
				"stageMode":             "evidence",
				"allowedActions":        []string{runtimemodel.NextActionTypeValidate},
				"forbiddenActions":      []string{runtimemodel.NextActionTypeStateChange},
				"requiredEvidenceKinds": []string{"validation"},
				"reason":                "validation evidence is missing",
			},
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
	}

	packet := buildStagePacketFromStepInput(run, "step-stage-packet-next-action", input).ToMap()

	nextAction := mapFromValue(packet["nextAction"])
	if nextAction["type"] != runtimemodel.NextActionTypeValidate ||
		nextAction["workspaceId"] != string(run.WorkspaceID) ||
		nextAction["runId"] != string(run.ID) {
		t.Fatalf("stagePacket.nextAction identity = %+v, want scoped validate action", nextAction)
	}
	if ids := stringSliceFromValue(nextAction["focusCoverageItemIds"]); len(ids) != 1 || ids[0] != "cwi-current" {
		t.Fatalf("stagePacket.nextAction.focusCoverageItemIds = %+v, want [cwi-current]", nextAction["focusCoverageItemIds"])
	}
	required := mapFromValue(nextAction["requiredOutput"])
	if stringFromMap(required, "coverageFocusId") != "focus-cwi-current" {
		t.Fatalf("stagePacket.nextAction.requiredOutput = %+v, want current focus", required)
	}
	if allowed := stringSliceFromValue(nextAction["allowedActions"]); len(allowed) != 1 || allowed[0] != runtimemodel.NextActionTypeValidate {
		t.Fatalf("stagePacket.nextAction.allowedActions = %+v, want validate", allowed)
	}
	if forbidden := stringSliceFromValue(nextAction["forbiddenActions"]); len(forbidden) != 1 || forbidden[0] != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("stagePacket.nextAction.forbiddenActions = %+v, want state_change", forbidden)
	}
	if evidence := stringSliceFromValue(nextAction["requiredEvidenceKinds"]); len(evidence) != 1 || evidence[0] != "validation" {
		t.Fatalf("stagePacket.nextAction.requiredEvidenceKinds = %+v, want validation", evidence)
	}
}

func TestStagePacketCarriesCurrentFailureDiagnostic(t *testing.T) {
	run := &runModel.Run{
		ID:          common.RunID("run-stage-packet-current-failure"),
		WorkspaceID: common.WorkspaceID("ws-stage-packet-current-failure"),
		State:       common.RunStateRunning,
	}
	input := map[string]any{
		"currentFailure": map[string]any{
			"failedStepId":     "run-stage-packet-current-failure/step-1",
			"failureClass":     failureClassRuntime,
			"failureSignature": "class=runtime_failed|stage=contract",
			"reason":           "structured runtime diagnostic",
		},
		stepInputKeyContractViolationRecovery: map[string]any{
			"violatedRule":   "wrong_shape",
			"recoverability": "retry_same_contract",
		},
	}

	packet := buildStagePacketFromStepInput(run, "step-stage-packet-current-failure", input).ToMap()

	failure := mapFromValue(packet["currentFailure"])
	if failure["failedStepId"] != "run-stage-packet-current-failure/step-1" ||
		failure["failureClass"] != failureClassRuntime ||
		failure["failureSignature"] != "class=runtime_failed|stage=contract" ||
		failure["reason"] != "structured runtime diagnostic" {
		t.Fatalf("stagePacket.currentFailure = %+v, want explicit current failure", failure)
	}
	if _, ok := failure["violatedRule"]; ok {
		t.Fatalf("stagePacket.currentFailure = %+v, want explicit failure to preempt contract violation recovery", failure)
	}
}

func TestStagePacketDerivesCurrentFailureFromCoverageGap(t *testing.T) {
	run := &runModel.Run{
		ID:          common.RunID("run-stage-packet-coverage-gap"),
		WorkspaceID: common.WorkspaceID("ws-stage-packet-coverage-gap"),
		State:       common.RunStateRunning,
	}
	input := map[string]any{
		stepInputKeyRuntimeRepairFollowUp: true,
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus": map[string]any{
				"id":                    "cwi-gap",
				"currentGap":            "missing accepted evidence binding for required evidence kind: validation, state_change",
				"missingEvidenceKinds":  []string{"validation", "state_change"},
				"requiredEvidenceKinds": []string{"validation", "state_change"},
				"coverageWorkItemIds":   []string{"cwi-gap"},
				"sourceRequirementId":   "source_change",
				"sourceScope":           "deliverable",
				"sourcePath":            "executionContract.profileContracts.neutral.deliverableExpectations[0]",
				"targetText":            "source_change",
			},
			"nextAction": map[string]any{
				"kind": "acquire_evidence",
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteFailureEvidence": true,
				"acceptedKinds":                      []string{"file/list"},
			},
		},
	}

	input = attachStagePacketToStepInput(run, "step-stage-packet-coverage-gap", input)
	packet := mapFromValue(input[stepInputKeyStagePacket])
	failure := mapFromValue(packet["currentFailure"])
	if failure["failureClass"] != failureClassCoverageActionableGap ||
		failure["coverageWorkItemId"] != "cwi-gap" ||
		failure["evidenceValidity"] != "actionable_gap" {
		t.Fatalf("stagePacket.currentFailure = %+v, want coverage actionable gap", failure)
	}
	pkgFailure := mapFromValue(mapFromValue(input["repairActionPackage"])["currentFailure"])
	if pkgFailure["coverageWorkItemId"] != "cwi-gap" {
		t.Fatalf("repairActionPackage.currentFailure = %+v, want mirrored coverage gap", pkgFailure)
	}
}

func TestStagePacketDerivesCurrentFailureFromContractViolationRecovery(t *testing.T) {
	run := &runModel.Run{
		ID:          common.RunID("run-stage-packet-contract-violation"),
		WorkspaceID: common.WorkspaceID("ws-stage-packet-contract-violation"),
		State:       common.RunStateRunning,
	}
	input := map[string]any{
		stepInputKeyContractViolationRecovery: map[string]any{
			"violatedRule":        "output_shape_mismatch",
			"violationType":       "runtime_contract_violation",
			"recoverability":      "retry_same_contract",
			"retryHint":           "return validation evidence only",
			"failureClass":        failureClassRuntime,
			"failureSignature":    "class=runtime_failed|rule=output_shape_mismatch",
			"rejectedOutputRef":   "evidence/rejected/001",
			"expectedContract":    map[string]any{"nextAction": "validate"},
			"actualOutputSummary": map[string]any{"shape": "decision"},
		},
	}

	packet := buildStagePacketFromStepInput(run, "step-stage-packet-contract-violation", input).ToMap()

	failure := mapFromValue(packet["currentFailure"])
	if failure["source"] != "contractViolationRecovery" ||
		failure["violatedRule"] != "output_shape_mismatch" ||
		failure["recoverability"] != "retry_same_contract" ||
		failure["rejectedOutputRef"] != "evidence/rejected/001" {
		t.Fatalf("stagePacket.currentFailure = %+v, want diagnostic-only contract violation", failure)
	}
	if expected := mapFromValue(failure["expectedContract"]); expected["nextAction"] != "validate" {
		t.Fatalf("stagePacket.currentFailure.expectedContract = %+v, want compact expected contract", expected)
	}
	if summary := mapFromValue(failure["actualOutputSummary"]); summary["shape"] != "decision" {
		t.Fatalf("stagePacket.currentFailure.actualOutputSummary = %+v, want compact output summary", summary)
	}
	withoutFailure := buildStagePacketFromStepInput(run, "step-stage-packet-contract-violation", map[string]any{}).ToMap()
	if packet["idempotencyKey"] == withoutFailure["idempotencyKey"] {
		t.Fatalf("idempotencyKey = %v, want currentFailure to affect stage packet identity", packet["idempotencyKey"])
	}
}

func TestStagePacketCarriesCurrentGapFromAuthorityFocus(t *testing.T) {
	run := &runModel.Run{
		ID:          common.RunID("run-stage-packet-current-gap"),
		WorkspaceID: common.WorkspaceID("ws-stage-packet-current-gap"),
		State:       common.RunStateRunning,
	}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus": map[string]any{
				"id":                   "cwi-current-gap",
				"currentGap":           "validation evidence remains missing",
				"missingEvidenceKinds": []string{"validation"},
				"coverageWorkItemIds":  []string{"cwi-current-gap"},
				"structuredTargetRefs": []string{"dod:item:current-gap"},
				"targetRefs":           []string{"target:current-gap"},
			},
		},
		"currentProgressDigest": map[string]any{
			"currentGap": "stale diagnostic must not preempt authority gap",
		},
	}

	packet := buildStagePacketFromStepInput(run, "step-stage-packet-current-gap", input).ToMap()

	gap := mapFromValue(packet["currentGap"])
	if gap["source"] != "authority.currentFocus" ||
		gap["focusId"] != "cwi-current-gap" ||
		gap["summary"] != "validation evidence remains missing" {
		t.Fatalf("stagePacket.currentGap = %+v, want authority focus gap", gap)
	}
	if missing := stringSliceFromValue(gap["missingEvidenceKinds"]); len(missing) != 1 || missing[0] != "validation" {
		t.Fatalf("stagePacket.currentGap.missingEvidenceKinds = %+v, want validation", gap["missingEvidenceKinds"])
	}
	if ids := stringSliceFromValue(gap["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != "cwi-current-gap" {
		t.Fatalf("stagePacket.currentGap.coverageWorkItemIds = %+v, want current coverage id", gap["coverageWorkItemIds"])
	}
	if refs := stringSliceFromValue(gap["structuredTargetRefs"]); len(refs) != 1 || refs[0] != "dod:item:current-gap" {
		t.Fatalf("stagePacket.currentGap.structuredTargetRefs = %+v, want structured target ref", gap["structuredTargetRefs"])
	}
	withoutGap := buildStagePacketFromStepInput(run, "step-stage-packet-current-gap", map[string]any{}).ToMap()
	if packet["idempotencyKey"] == withoutGap["idempotencyKey"] {
		t.Fatalf("idempotencyKey = %v, want currentGap to affect stage packet identity", packet["idempotencyKey"])
	}
}

func TestStagePacketExplicitCurrentGapPreemptsProgressDigest(t *testing.T) {
	run := &runModel.Run{
		ID:          common.RunID("run-stage-packet-explicit-gap"),
		WorkspaceID: common.WorkspaceID("ws-stage-packet-explicit-gap"),
		State:       common.RunStateRunning,
	}
	input := map[string]any{
		"currentGap": map[string]any{
			"summary":              "explicit structured gap",
			"missingEvidenceKinds": []string{"state_change"},
		},
		stepInputKeyCurrentProgressDigest: map[string]any{
			"currentGap":           "progress digest gap must not preempt explicit gap",
			"missingEvidenceKinds": []string{"validation"},
		},
	}

	packet := buildStagePacketFromStepInput(run, "step-stage-packet-explicit-gap", input).ToMap()

	gap := mapFromValue(packet["currentGap"])
	if gap["source"] != "input.currentGap" || gap["summary"] != "explicit structured gap" {
		t.Fatalf("stagePacket.currentGap = %+v, want explicit currentGap", gap)
	}
	if missing := stringSliceFromValue(gap["missingEvidenceKinds"]); len(missing) != 1 || missing[0] != "state_change" {
		t.Fatalf("stagePacket.currentGap.missingEvidenceKinds = %+v, want state_change", gap["missingEvidenceKinds"])
	}
}

func TestStagePacketDerivesCurrentGapFromProgressDigest(t *testing.T) {
	run := &runModel.Run{
		ID:          common.RunID("run-stage-packet-progress-gap"),
		WorkspaceID: common.WorkspaceID("ws-stage-packet-progress-gap"),
		State:       common.RunStateRunning,
	}
	input := map[string]any{
		stepInputKeyCurrentProgressDigest: map[string]any{
			"currentGap":           "latest accepted evidence does not bind target",
			"missingEvidenceKinds": []string{"validation"},
			"coverageWorkItemIds":  []string{"cwi-progress-gap"},
			"targetRefs":           []string{"target:progress-gap"},
		},
	}

	packet := buildStagePacketFromStepInput(run, "step-stage-packet-progress-gap", input).ToMap()

	gap := mapFromValue(packet["currentGap"])
	if gap["source"] != "currentProgressDigest" ||
		gap["summary"] != "latest accepted evidence does not bind target" {
		t.Fatalf("stagePacket.currentGap = %+v, want progress digest gap", gap)
	}
	if ids := stringSliceFromValue(gap["coverageWorkItemIds"]); len(ids) != 1 || ids[0] != "cwi-progress-gap" {
		t.Fatalf("stagePacket.currentGap.coverageWorkItemIds = %+v, want progress coverage id", gap["coverageWorkItemIds"])
	}
}

func TestStagePacketCarriesProcessEvidenceTimeline(t *testing.T) {
	run := &runModel.Run{
		ID:          common.RunID("run-stage-packet-timeline"),
		WorkspaceID: common.WorkspaceID("ws-stage-packet-timeline"),
		State:       common.RunStateRunning,
	}
	input := map[string]any{
		stepInputKeyProcessEvidenceTimeline: []map[string]any{{
			"order":          1,
			"sourceRef":      "history://runs/run-stage-packet-timeline/process-evidence",
			"summaryKind":    "compact_history",
			"actionKind":     "history_summary",
			"historicalOnly": true,
			"itemCount":      3,
			"firstSourceRef": "step://older-001",
			"lastSourceRef":  "step://older-003",
			"sourceRefs":     []string{"step://older-001", "step://older-002", "step://older-003"},
		}, {
			"order":        2,
			"sourceRef":    "step://recent-001",
			"stepId":       "recent-001",
			"kind":         llmWorkspaceExecutionStepKind,
			"actionKind":   "llm_candidate",
			"state":        string(common.StepStateFailed),
			"stage":        runtimeStageRepair,
			"failureClass": failureClassRuntime,
			"evidenceRefs": []string{"evidence://rejected/recent-001"},
			"payloadRef":   "payload://recent-001",
			"ignored":      "non-contract timeline field",
		}},
		"executionContext": map[string]any{
			stepInputKeyProcessEvidenceTimeline: []map[string]any{{
				"sourceRef": "step://stale-context",
			}},
		},
	}

	packet := buildStagePacketFromStepInput(run, "step-stage-packet-timeline", input).ToMap()

	timeline := mapSliceFromValue(packet["processEvidenceTimeline"])
	if len(timeline) != 2 {
		t.Fatalf("stagePacket.processEvidenceTimeline = %+v, want two compact items", timeline)
	}
	if timeline[0]["summaryKind"] != "compact_history" ||
		timeline[0]["sourceRef"] != "history://runs/run-stage-packet-timeline/process-evidence" ||
		timeline[0]["firstSourceRef"] != "step://older-001" ||
		timeline[0]["lastSourceRef"] != "step://older-003" {
		t.Fatalf("stagePacket.processEvidenceTimeline[0] = %+v, want compact history lineage", timeline[0])
	}
	if refs := stringSliceFromValue(timeline[0]["sourceRefs"]); len(refs) != 3 || refs[0] != "step://older-001" || refs[2] != "step://older-003" {
		t.Fatalf("stagePacket.processEvidenceTimeline[0].sourceRefs = %+v, want ordered refs", timeline[0]["sourceRefs"])
	}
	if timeline[1]["sourceRef"] != "step://recent-001" ||
		timeline[1]["stepId"] != "recent-001" ||
		timeline[1]["state"] != string(common.StepStateFailed) ||
		timeline[1]["stage"] != runtimeStageRepair ||
		timeline[1]["payloadRef"] != "payload://recent-001" {
		t.Fatalf("stagePacket.processEvidenceTimeline[1] = %+v, want recent evidence item", timeline[1])
	}
	if refs := stringSliceFromValue(timeline[1]["evidenceRefs"]); len(refs) != 1 || refs[0] != "evidence://rejected/recent-001" {
		t.Fatalf("stagePacket.processEvidenceTimeline[1].evidenceRefs = %+v, want rejected evidence ref", timeline[1]["evidenceRefs"])
	}
	if _, ok := timeline[1]["ignored"]; ok {
		t.Fatalf("stagePacket.processEvidenceTimeline[1] = %+v, want compact contract fields only", timeline[1])
	}
	withoutTimeline := buildStagePacketFromStepInput(run, "step-stage-packet-timeline", map[string]any{}).ToMap()
	if packet["idempotencyKey"] == withoutTimeline["idempotencyKey"] {
		t.Fatalf("idempotencyKey = %v, want processEvidenceTimeline to affect stage packet identity", packet["idempotencyKey"])
	}
}

func TestStagePacketCarriesTemplatePolicyRefs(t *testing.T) {
	run := &runModel.Run{
		ID:          common.RunID("run-stage-packet-policy-refs"),
		WorkspaceID: common.WorkspaceID("ws-stage-packet-policy-refs"),
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			"templatePolicyRefs": []string{"policy://run-bound"},
		},
	}
	input := map[string]any{
		"policyRefs": map[string]any{
			"evidence": []any{"policy://evidence", "policy://quality"},
		},
		"executionContext": map[string]any{
			"templatePolicyRefs": []any{"policy://context-bound", "policy://evidence"},
		},
	}

	packet := buildStagePacketFromStepInput(run, "step-stage-packet-policy-refs", input).ToMap()

	refs := stringSliceFromValue(packet["templatePolicyRefs"])
	for _, expected := range []string{"policy://evidence", "policy://quality", "policy://context-bound", "policy://run-bound"} {
		if !stringSliceContains(refs, expected) {
			t.Fatalf("stagePacket.templatePolicyRefs = %+v, want %s", refs, expected)
		}
	}
	if len(refs) != 4 {
		t.Fatalf("stagePacket.templatePolicyRefs = %+v, want deduplicated refs", refs)
	}
	withoutRefs := buildStagePacketFromStepInput(&runModel.Run{
		ID:          run.ID,
		WorkspaceID: run.WorkspaceID,
		State:       common.RunStateRunning,
	}, "step-stage-packet-policy-refs", map[string]any{}).ToMap()
	if packet["idempotencyKey"] == withoutRefs["idempotencyKey"] {
		t.Fatalf("idempotencyKey = %v, want templatePolicyRefs to affect stage packet identity", packet["idempotencyKey"])
	}
}

func TestStagePacketCarriesCompactSummaryRef(t *testing.T) {
	run := &runModel.Run{
		ID:          common.RunID("run-stage-packet-summary-ref"),
		WorkspaceID: common.WorkspaceID("ws-stage-packet-summary-ref"),
		State:       common.RunStateRunning,
	}
	input := map[string]any{
		"executionContext": map[string]any{
			"summaryRef": "summary://stale-context",
		},
		"compactSummaryRef": "summary://current-compact",
		stepInputKeyProcessEvidenceTimeline: []map[string]any{{
			"summaryKind": "compact_history",
			"sourceRef":   "history://runs/run-stage-packet-summary-ref/process-evidence",
		}},
	}

	packet := buildStagePacketFromStepInput(run, "step-stage-packet-summary-ref", input).ToMap()

	if packet["compactSummaryRef"] != "summary://current-compact" {
		t.Fatalf("stagePacket.compactSummaryRef = %v, want explicit compact summary ref", packet["compactSummaryRef"])
	}
	fallback := buildStagePacketFromStepInput(run, "step-stage-packet-summary-ref", map[string]any{
		stepInputKeyProcessEvidenceTimeline: []map[string]any{{
			"summaryKind": "compact_history",
			"sourceRef":   "history://runs/run-stage-packet-summary-ref/process-evidence",
		}},
	}).ToMap()
	if fallback["compactSummaryRef"] != "history://runs/run-stage-packet-summary-ref/process-evidence" {
		t.Fatalf("stagePacket.compactSummaryRef fallback = %v, want compact history sourceRef", fallback["compactSummaryRef"])
	}
	withoutRef := buildStagePacketFromStepInput(run, "step-stage-packet-summary-ref", map[string]any{}).ToMap()
	if packet["idempotencyKey"] == withoutRef["idempotencyKey"] {
		t.Fatalf("idempotencyKey = %v, want compactSummaryRef to affect stage packet identity", packet["idempotencyKey"])
	}
}

func TestStagePacketCarriesMemoryRefs(t *testing.T) {
	run := &runModel.Run{
		ID:          common.RunID("run-stage-packet-memory-refs"),
		WorkspaceID: common.WorkspaceID("ws-stage-packet-memory-refs"),
		State:       common.RunStateRunning,
	}
	input := map[string]any{
		"memoryRefs": []string{"memory://explicit"},
		stepInputKeyExecutionContextPackage: map[string]any{
			"memoryRetrieval": map[string]any{
				"selectedMemoryIds": []any{"memory://selected", "memory://explicit"},
			},
			"selectedContext": map[string]any{
				"memories": []map[string]any{{
					"id":      "memory://context",
					"summary": "selected memory summary must not become a ref",
				}},
			},
		},
	}

	packet := buildStagePacketFromStepInput(run, "step-stage-packet-memory-refs", input).ToMap()

	refs := stringSliceFromValue(packet["memoryRefs"])
	for _, expected := range []string{"memory://explicit", "memory://selected", "memory://context"} {
		if !stringSliceContains(refs, expected) {
			t.Fatalf("stagePacket.memoryRefs = %+v, want %s", refs, expected)
		}
	}
	if len(refs) != 3 {
		t.Fatalf("stagePacket.memoryRefs = %+v, want deduplicated refs only", refs)
	}
	encoded, err := json.Marshal(packet)
	if err != nil {
		t.Fatalf("marshal stage packet: %v", err)
	}
	if strings.Contains(string(encoded), "selected memory summary") {
		t.Fatalf("stagePacket leaked memory body: %s", encoded)
	}
	withoutRefs := buildStagePacketFromStepInput(run, "step-stage-packet-memory-refs", map[string]any{}).ToMap()
	if packet["idempotencyKey"] == withoutRefs["idempotencyKey"] {
		t.Fatalf("idempotencyKey = %v, want memoryRefs to affect stage packet identity", packet["idempotencyKey"])
	}
}

func TestFollowUpStepInputRebuildsStagePacketAfterNextInputMutation(t *testing.T) {
	run := &runModel.Run{
		ID:                 common.RunID("run-follow-up-stage-packet"),
		WorkspaceID:        common.WorkspaceID("ws-follow-up-stage-packet"),
		GoalID:             common.GoalID("goal-follow-up"),
		DoDID:              common.DoDID("dod-follow-up"),
		DefinitionRevision: "rev-follow-up",
		State:              common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemContextKey: map[string]any{
				"definitionRevision": "rev-follow-up",
				"currentFocus":       map[string]any{"id": "cwi-original"},
			},
		},
	}
	previous := &runModel.Step{
		ID:    common.StepID("planning-001"),
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{},
		Output: map[string]any{
			stepOutputKeyNextInput: map[string]any{
				coverageWorkItemContextKey: map[string]any{
					"definitionRevision": "rev-follow-up",
					"currentFocus": map[string]any{
						"id":                   "cwi-mutated",
						"acceptedEvidenceRefs": []string{"accepted-mutated"},
						"rejectedEvidenceRefs": []string{"rejected-mutated"},
					},
				},
				"repairActionPackage": map[string]any{
					"requiredOutput": map[string]any{"mustReturnValidation": true},
					"targetSnapshots": []map[string]any{{
						"targetRef":        "target-redacted",
						"contentComplete":  true,
						"safeForFileWrite": true,
						"redacted":         true,
					}},
				},
			},
		},
	}

	input := nextStepInputFromOutput(run, previous)
	packet := mapFromValue(input[stepInputKeyStagePacket])
	focus := mapFromValue(packet["activeFocus"])
	if focus["id"] != "cwi-mutated" {
		t.Fatalf("stagePacket.activeFocus = %+v, want rebuilt mutated focus", focus)
	}
	required := mapFromValue(packet["requiredOutput"])
	if required["mustReturnValidation"] != true {
		t.Fatalf("stagePacket.requiredOutput = %+v, want rebuilt required output", required)
	}
	if packet["stepId"] != "planning-001" {
		t.Fatalf("stagePacket.stepId = %v, want previous step scope", packet["stepId"])
	}
	if refs := stringSliceFromValue(packet["acceptedEvidenceRefs"]); len(refs) != 1 || refs[0] != "accepted-mutated" {
		t.Fatalf("stagePacket.acceptedEvidenceRefs = %+v, want rebuilt accepted ref", refs)
	}
	if refs := stringSliceFromValue(packet["rejectedEvidenceRefs"]); len(refs) != 1 || refs[0] != "rejected-mutated" {
		t.Fatalf("stagePacket.rejectedEvidenceRefs = %+v, want rebuilt rejected ref", refs)
	}
	snapshots := mapSliceFromValue(packet["targetSnapshots"])
	if len(snapshots) != 1 || snapshots[0]["integrityState"] != "redacted" || snapshots[0]["safeForExactFileWrite"] != false {
		t.Fatalf("stagePacket.targetSnapshots = %+v, want rebuilt redacted unsafe snapshot", snapshots)
	}
}
