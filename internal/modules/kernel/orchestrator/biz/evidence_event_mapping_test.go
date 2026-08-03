package orchestrator

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestEvidenceEventDTOsAreDetachedAndComplete(t *testing.T) {
	tool := &runmodel.ToolCall{ID: "tool-1", WorkspaceID: "ws-1", RunID: "run-1", Arguments: workorcheventpayload.New(workorcheventpayload.StringField("arg", "value")), Output: workorcheventpayload.New(workorcheventpayload.StringField("out", "value"))}
	patch := &runmodel.Patch{ID: "patch-1", WorkspaceID: "ws-1", RunID: "run-1", CoverageTargets: []runmodel.PatchCoverageTarget{{GoalRequirementID: "goal-1", TargetText: "target"}}, AffectedFiles: []string{"main.go"}, RiskReasons: []string{"risk"}, Diagnostics: []workorcheventpayload.Payload{workorcheventpayload.New(workorcheventpayload.StringField("diagnostic", "value"))}}
	validation := &runmodel.ValidationResult{ID: "validation-1", WorkspaceID: "ws-1", RunID: "run-1", EvidenceTags: []string{"tag"}}

	tools := toolCallEventDTOs([]*runmodel.ToolCall{tool})
	patches := patchEventDTOs([]*runmodel.Patch{patch})
	validations := validationResultEventDTOs([]*runmodel.ValidationResult{validation})
	if tools[0].ID != tool.ID || patches[0].CoverageTargets[0].GoalRequirementID != "goal-1" || validations[0].ID != validation.ID {
		t.Fatalf("event DTO mapping lost evidence fields tools=%+v patches=%+v validations=%+v", tools, patches, validations)
	}
	tools[0].Arguments.Fields[0].Value.String = "changed"
	patches[0].AffectedFiles[0] = "changed.go"
	patches[0].Diagnostics[0].Fields[0].Value.String = "changed"
	validations[0].EvidenceTags[0] = "changed"
	if value, _ := tool.Arguments.String("arg"); value != "value" || patch.AffectedFiles[0] != "main.go" || validation.EvidenceTags[0] != "tag" {
		t.Fatal("event DTO mapping retained mutable owner model storage")
	}
	if value, _ := patch.Diagnostics[0].String("diagnostic"); value != "value" {
		t.Fatal("event DTO diagnostics were not detached")
	}
	empty := patchEventDTOs([]*runmodel.Patch{{CoverageTargets: []runmodel.PatchCoverageTarget{}, AffectedFiles: []string{}, Diagnostics: []workorcheventpayload.Payload{}}})[0]
	if empty.CoverageTargets == nil || empty.AffectedFiles == nil || empty.Diagnostics == nil {
		t.Fatal("event DTO mapping collapsed non-nil empty collections")
	}
}
