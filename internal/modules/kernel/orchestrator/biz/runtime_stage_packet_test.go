package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
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
