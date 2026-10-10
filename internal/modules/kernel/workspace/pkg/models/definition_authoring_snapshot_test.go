package workspace

import "testing"

func TestAuthoringRecordSealsDetachedEffectiveTemplate(t *testing.T) {
	template := WorkspaceTemplate{ID: "t", ParentTemplateIDs: []string{"base"}, EffectiveSources: []TemplateSource{{TemplateID: "base", ContentHash: "base-hash"}, {TemplateID: "t", ContentHash: "child-hash"}}, DoDItems: []string{"必需条件"}, SoulBindings: map[string]string{"review": "reviewer"}}
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
	record := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template.Clone(), TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template)}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := record.Clone()
	clone.TemplateSnapshot.ParentTemplateIDs[0] = "other"
	clone.TemplateSnapshot.EffectiveSources[0].ContentHash = "changed"
	clone.TemplateSnapshot.DoDItems[0] = "删除条件"
	clone.TemplateSnapshot.SoulBindings["review"] = "other"
	if err := record.Validate(); err != nil {
		t.Fatalf("clone changed source: %v", err)
	}
	if clone.Validate() == nil {
		t.Fatal("tampered snapshot accepted")
	}
	record.TemplateSnapshot = WorkspaceTemplate{}
	if record.Validate() == nil {
		t.Fatal("hash-only record accepted")
	}
}
