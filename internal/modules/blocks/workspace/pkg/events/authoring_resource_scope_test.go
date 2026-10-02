package events

import (
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringResourceScopeBindsCurrentDraftAndRef(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t", ResourceRefs: []string{"requirements.txt"}}
	template := workspace.WorkspaceTemplate{ID: "t"}
	record := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: WorkspaceTemplate{ID: template.ID}, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	view := QueryDefinitionAuthoringResult{WorkspaceID: input.WorkspaceID, WorkspaceRevision: 1, Record: &record, DeliverableDirectory: "/workspace/project", CurrentTemplateAvailable: true, CurrentTemplateSnapshotHash: record.TemplateSnapshotHash}
	command := QueryAuthoringResourceScopeCommand{WorkspaceID: input.WorkspaceID, InputHash: record.InputHash, ResourceRef: input.ResourceRefs[0]}
	got, err := AuthoringResourceScopeFor(view, command)
	if err != nil || got.ValidateFor(command) != nil || got.DeliverableDirectory != view.DeliverableDirectory || got.WorkspaceRevision != view.WorkspaceRevision {
		t.Fatalf("current scope: %+v %v", got, err)
	}
	got.WorkspaceRevision = 0
	if got.ValidateFor(command) == nil {
		t.Fatal("unversioned resource scope was accepted")
	}
	for _, test := range []struct {
		name   string
		mutate func(*QueryDefinitionAuthoringResult, *QueryAuthoringResourceScopeCommand)
	}{
		{"workspace", func(_ *QueryDefinitionAuthoringResult, c *QueryAuthoringResourceScopeCommand) {
			c.WorkspaceID = "other"
		}},
		{"input hash", func(_ *QueryDefinitionAuthoringResult, c *QueryAuthoringResourceScopeCommand) { c.InputHash = "old" }},
		{"ref", func(_ *QueryDefinitionAuthoringResult, c *QueryAuthoringResourceScopeCommand) {
			c.ResourceRef = "other.txt"
		}},
		{"template", func(v *QueryDefinitionAuthoringResult, _ *QueryAuthoringResourceScopeCommand) {
			v.CurrentTemplateSnapshotHash = "changed"
		}},
		{"removed template", func(v *QueryDefinitionAuthoringResult, _ *QueryAuthoringResourceScopeCommand) {
			v.CurrentTemplateAvailable = false
			v.CurrentTemplateSnapshotHash = ""
		}},
		{"no directory", func(v *QueryDefinitionAuthoringResult, _ *QueryAuthoringResourceScopeCommand) {
			v.DeliverableDirectory = ""
		}},
		{"no record", func(v *QueryDefinitionAuthoringResult, _ *QueryAuthoringResourceScopeCommand) { v.Record = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := view
			request := command
			test.mutate(&changed, &request)
			if result, err := AuthoringResourceScopeFor(changed, request); err == nil || result.DeliverableDirectory != "" {
				t.Fatalf("untrusted scope admitted: %+v %v", result, err)
			}
		})
	}
}
