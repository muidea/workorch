package events

import (
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestAuthoringInputQueryRejectsWrongRecordAndOriginalAuthority(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 2, GoalText: "待审输入", TemplateID: "t"}
	template := workspace.WorkspaceTemplate{ID: "t"}
	view := QueryDefinitionAuthoringResult{WorkspaceID: "ws", WorkspaceRevision: 3, Record: &DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: WorkspaceTemplate{ID: template.ID}, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}}
	command := QueryDefinitionAuthoringCommand{WorkspaceID: "ws", InputHash: input.InputHash()}
	if err := view.ValidateFor(command); err != nil {
		t.Fatal(err)
	}
	wrong := command
	wrong.InputHash = "another-input"
	if view.ValidateFor(wrong) == nil {
		t.Fatal("错误 input 身份获准")
	}
	for _, kind := range []string{"confirmation", "activation", "admission"} {
		copy := view
		switch kind {
		case "confirmation":
			copy.Confirmation = &workspace.DefinitionAuthoringConfirmation{}
		case "activation":
			copy.ActivationAdmission = &workspace.WorkspaceActivationAdmission{}
		case "admission":
			copy.AdmissionCandidate = &workspace.DefinitionAdmissionCandidate{}
		}
		if copy.ValidateFor(command) == nil {
			t.Fatalf("待审输入借用原 %s", kind)
		}
	}
}
