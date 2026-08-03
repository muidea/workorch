package ownerreadports

import (
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestOrchestratorEvidenceModelsAreDetached(t *testing.T) {
	tool := &orchestratorevents.ToolCall{ID: "tool-1", Arguments: workorcheventpayload.New(workorcheventpayload.StringField("arg", "value"))}
	patch := &orchestratorevents.Patch{ID: "patch-1", CoverageTargets: []orchestratorevents.PatchCoverageTarget{{GoalRequirementID: "goal-1"}}, AffectedFiles: []string{"main.go"}, Diagnostics: []workorcheventpayload.Payload{workorcheventpayload.New(workorcheventpayload.StringField("diagnostic", "value"))}}
	validation := &orchestratorevents.ValidationResult{ID: "validation-1", EvidenceTags: []string{"tag"}}

	tools := toolCallModelsFromOrchestrator([]*orchestratorevents.ToolCall{tool})
	patches := patchModelsFromOrchestrator([]*orchestratorevents.Patch{patch})
	validations := validationResultModelsFromOrchestrator([]*orchestratorevents.ValidationResult{validation})
	tools[0].Arguments.Fields[0].Value.String = "changed"
	patches[0].AffectedFiles[0] = "changed.go"
	patches[0].Diagnostics[0].Fields[0].Value.String = "changed"
	validations[0].EvidenceTags[0] = "changed"
	if value, _ := tool.Arguments.String("arg"); value != "value" || patch.AffectedFiles[0] != "main.go" || validation.EvidenceTags[0] != "tag" {
		t.Fatal("application mapping retained mutable EventHub DTO storage")
	}
	if value, _ := patch.Diagnostics[0].String("diagnostic"); value != "value" {
		t.Fatal("application mapping diagnostics were not detached")
	}
	empty := patchModelsFromOrchestrator([]*orchestratorevents.Patch{{CoverageTargets: []orchestratorevents.PatchCoverageTarget{}, AffectedFiles: []string{}, Diagnostics: []workorcheventpayload.Payload{}}})[0]
	if empty.CoverageTargets == nil || empty.AffectedFiles == nil || empty.Diagnostics == nil {
		t.Fatal("application mapping collapsed non-nil empty collections")
	}
}
