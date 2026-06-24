package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestStagePacketNormalizedCarriesDefinitionFocusAndRequiredOutput(t *testing.T) {
	packet := StagePacket{
		WorkspaceID: common.WorkspaceID("ws-001"),
		RunID:       common.RunID("run-001"),
		Definition: StagePacketDefinition{
			Revision: " rev-001 ",
			GoalID:   common.GoalID("goal-001"),
			DoDID:    common.DoDID("dod-001"),
		},
		ActiveFocus: map[string]any{"id": "cwi-001"},
		RequiredOutput: map[string]any{
			"mustBindCoverageFocus": true,
		},
		RequiredEvidenceKinds: []string{" validation ", "validation"},
	}

	normalized := packet.Normalized()
	rendered := normalized.ToMap()

	if normalized.SchemaVersion != StagePacketSchema {
		t.Fatalf("schemaVersion = %q, want %q", normalized.SchemaVersion, StagePacketSchema)
	}
	definition := rendered["definition"].(map[string]any)
	if definition["revision"] != "rev-001" || definition["goalId"] != "goal-001" || definition["doDId"] != "dod-001" {
		t.Fatalf("definition = %+v, want revision and ids", definition)
	}
	if rendered["activeFocus"].(map[string]any)["id"] != "cwi-001" {
		t.Fatalf("activeFocus = %+v, want cwi-001", rendered["activeFocus"])
	}
	if rendered["requiredOutput"].(map[string]any)["mustBindCoverageFocus"] != true {
		t.Fatalf("requiredOutput = %+v, want focus binding", rendered["requiredOutput"])
	}
	if len(normalized.RequiredEvidenceKinds) != 1 || normalized.RequiredEvidenceKinds[0] != "validation" {
		t.Fatalf("requiredEvidenceKinds = %+v, want compact validation", normalized.RequiredEvidenceKinds)
	}
	if normalized.IdempotencyKey == "" {
		t.Fatal("idempotencyKey is empty")
	}
}

func TestStagePacketCanInheritNextActionContract(t *testing.T) {
	action := NextAction{
		WorkspaceID:           common.WorkspaceID("ws-001"),
		RunID:                 common.RunID("run-001"),
		Type:                  NextActionTypeValidate,
		Stage:                 "validate",
		RequiredOutput:        map[string]any{"mustReturnEvidence": true},
		AllowedActions:        []string{NextActionTypeValidate},
		ForbiddenActions:      []string{NextActionTypeStateChange},
		RequiredEvidenceKinds: []string{EvidenceKindValidation},
	}

	packet := StagePacket{
		WorkspaceID: common.WorkspaceID("ws-001"),
		RunID:       common.RunID("run-001"),
		NextAction:  &action,
	}.Normalized()

	if packet.ActiveStage != "validate" {
		t.Fatalf("activeStage = %q, want validate", packet.ActiveStage)
	}
	if packet.RequiredOutput["mustReturnEvidence"] != true {
		t.Fatalf("requiredOutput = %+v, want inherited next action output", packet.RequiredOutput)
	}
	if len(packet.AllowedActions) != 1 || packet.AllowedActions[0] != NextActionTypeValidate {
		t.Fatalf("allowedActions = %+v, want validate", packet.AllowedActions)
	}
	rendered := packet.ToMap()
	nextAction, ok := rendered["nextAction"].(map[string]any)
	if !ok || nextAction["type"] != NextActionTypeValidate || nextAction["stage"] != "validate" {
		t.Fatalf("rendered nextAction = %+v, want map with validate action", rendered["nextAction"])
	}
	if packet.IdempotencyKey == "" {
		t.Fatal("idempotencyKey is empty")
	}
}
