package models

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestEvidenceClonesAreDetachedAndPreserveCollectionShape(t *testing.T) {
	tool := &ToolCall{ID: "tool-1", Arguments: workorcheventpayload.New(workorcheventpayload.StringField("arg", "value"))}
	patch := &Patch{
		ID:              "patch-1",
		CoverageTargets: []PatchCoverageTarget{{GoalRequirementID: "goal-1"}},
		AffectedFiles:   []string{"main.go"},
		RiskReasons:     []string{"risk"},
		Diagnostics:     []workorcheventpayload.Payload{workorcheventpayload.New(workorcheventpayload.StringField("diagnostic", "value"))},
	}
	validation := &ValidationResult{ID: "validation-1", EvidenceTags: []string{"tag"}}

	tools := CloneToolCalls([]*ToolCall{tool})
	patches := ClonePatches([]*Patch{patch})
	validations := CloneValidationResults([]*ValidationResult{validation})
	tools[0].Arguments.Fields[0].Value.String = "changed"
	patches[0].AffectedFiles[0] = "changed.go"
	patches[0].Diagnostics[0].Fields[0].Value.String = "changed"
	validations[0].EvidenceTags[0] = "changed"
	if value, _ := tool.Arguments.String("arg"); value != "value" || patch.AffectedFiles[0] != "main.go" || validation.EvidenceTags[0] != "tag" {
		t.Fatal("clone retained mutable owner model storage")
	}
	if value, _ := patch.Diagnostics[0].String("diagnostic"); value != "value" {
		t.Fatal("clone retained mutable diagnostic payload storage")
	}

	empty := ClonePatches([]*Patch{{CoverageTargets: []PatchCoverageTarget{}, AffectedFiles: []string{}, RiskReasons: []string{}, Diagnostics: []workorcheventpayload.Payload{}}})[0]
	if empty.CoverageTargets == nil || empty.AffectedFiles == nil || empty.RiskReasons == nil || empty.Diagnostics == nil {
		t.Fatal("clone collapsed non-nil empty collections")
	}
	if CloneToolCalls(nil) != nil || ClonePatches(nil) != nil || CloneValidationResults(nil) != nil {
		t.Fatal("nil slices must remain nil")
	}
}
