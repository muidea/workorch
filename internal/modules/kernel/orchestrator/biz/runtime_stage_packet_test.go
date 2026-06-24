package orchestrator

import (
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
						"id":         "cwi-stage-packet",
						"targetText": "neutral target",
					},
				},
				"requiredOutput": map[string]any{
					"mustBindCoverageFocus":        true,
					"mustIncludeStateChangingNode": true,
				},
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
					"currentFocus":       map[string]any{"id": "cwi-mutated"},
				},
				"repairActionPackage": map[string]any{
					"requiredOutput": map[string]any{"mustReturnValidation": true},
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
}
