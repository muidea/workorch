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
		AcceptedEvidenceRefs:  []string{" evidence-accepted ", "evidence-accepted"},
		RejectedEvidenceRefs:  []string{" evidence-rejected "},
		TargetSnapshots: []map[string]any{{
			"targetRef":        "target-full",
			"contentHash":      "sha256:full",
			"contentComplete":  true,
			"safeForFileWrite": true,
			"truncated":        false,
			"redacted":         false,
			"isBinary":         false,
			"content":          "content must not render",
		}, {
			"targetRef":        "target-truncated",
			"contentComplete":  false,
			"safeForFileWrite": true,
			"truncated":        true,
		}},
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
	if len(normalized.AcceptedEvidenceRefs) != 1 || normalized.AcceptedEvidenceRefs[0] != "evidence-accepted" {
		t.Fatalf("acceptedEvidenceRefs = %+v, want compact accepted ref", normalized.AcceptedEvidenceRefs)
	}
	if len(normalized.RejectedEvidenceRefs) != 1 || normalized.RejectedEvidenceRefs[0] != "evidence-rejected" {
		t.Fatalf("rejectedEvidenceRefs = %+v, want compact rejected ref", normalized.RejectedEvidenceRefs)
	}
	if rendered["acceptedEvidenceRefs"].([]string)[0] != "evidence-accepted" ||
		rendered["rejectedEvidenceRefs"].([]string)[0] != "evidence-rejected" {
		t.Fatalf("rendered evidence refs = accepted:%+v rejected:%+v", rendered["acceptedEvidenceRefs"], rendered["rejectedEvidenceRefs"])
	}
	snapshots := rendered["targetSnapshots"].([]map[string]any)
	if len(snapshots) != 2 {
		t.Fatalf("targetSnapshots len = %d, want 2: %+v", len(snapshots), snapshots)
	}
	if snapshots[0]["integrityState"] != "full" || snapshots[0]["safeForExactFileWrite"] != true {
		t.Fatalf("full target snapshot = %+v, want full safe exact write", snapshots[0])
	}
	if _, ok := snapshots[0]["content"]; ok {
		t.Fatalf("target snapshot carried content: %+v", snapshots[0])
	}
	if snapshots[1]["integrityState"] != "truncated" ||
		snapshots[1]["safeForExactFileWrite"] != false ||
		snapshots[1]["safeForFileWrite"] != false {
		t.Fatalf("truncated target snapshot = %+v, want unsafe truncated metadata", snapshots[1])
	}
	if normalized.IdempotencyKey == "" {
		t.Fatal("idempotencyKey is empty")
	}
}

func TestNormalizeStagePacketTargetSnapshotsMarksRedactedAsUnsafe(t *testing.T) {
	snapshots := NormalizeStagePacketTargetSnapshots([]map[string]any{{
		"targetRef":        "target-redacted",
		"contentHash":      "sha256:redacted",
		"contentComplete":  true,
		"safeForFileWrite": true,
		"redacted":         true,
	}})

	if len(snapshots) != 1 {
		t.Fatalf("snapshots len = %d, want 1", len(snapshots))
	}
	if snapshots[0]["integrityState"] != "redacted" ||
		snapshots[0]["safeForExactFileWrite"] != false ||
		snapshots[0]["safeForFileWrite"] != false {
		t.Fatalf("redacted snapshot = %+v, want unsafe redacted metadata", snapshots[0])
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
