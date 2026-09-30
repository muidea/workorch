package specifications

import (
	"encoding/json"
	"strings"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestDerivedAcceptancePreservesUserAndTemplateRequirements(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t", AcceptanceRequirements: []string{"不得放宽用户条件"}}
	template := &workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"必须满足模板约束"}}
	proposal := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(template), AdditionalItems: []string{"派生条件"}}
	payload, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseDerivedAcceptance(payload, input, template)
	if err != nil {
		t.Fatal(err)
	}
	items, err := parsed.CandidateItems(input, template)
	if err != nil || len(items) != 3 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if items[0].Text != input.AcceptanceRequirements[0] || items[1].Text != template.DoDItems[0] || items[2].Text != "派生条件" {
		t.Fatal("explicit requirements lost")
	}
	if items[0].SourceIdentity != input.InputHash() || items[1].SourceIdentity != proposal.TemplateSnapshotHash || items[2].SourceIdentity != proposal.ContentHash() {
		t.Fatal("source identity lost")
	}
	items[0].Text = "外部改写"
	if input.AcceptanceRequirements[0] != "不得放宽用户条件" {
		t.Fatal("aliased input")
	}
	for _, bad := range []string{
		strings.TrimSuffix(string(payload), "}") + `,"GoalText":"修改目标"}`,
		strings.TrimSuffix(string(payload), "}") + `,"Reviews":[]}`,
		strings.TrimSuffix(string(payload), "}") + `,"additionalitems":[]}`,
		"```json\n" + string(payload) + "\n```",
		string(payload) + `{}`,
	} {
		if _, err := ParseDerivedAcceptance([]byte(bad), input, template); err == nil {
			t.Fatal("invalid provider document accepted")
		}
	}
	input.Revision++
	if _, err := ParseDerivedAcceptance(payload, input, template); err == nil {
		t.Fatal("stale input accepted")
	}
	input.Revision--
	template.DoDItems[0] = "模板新条件"
	if _, err := ParseDerivedAcceptance(payload, input, template); err == nil {
		t.Fatal("template drift accepted")
	}
}

func TestDerivedAcceptanceRequiresRealItems(t *testing.T) {
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t"}
	template := &workspace.WorkspaceTemplate{ID: "t"}
	p := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(template)}
	if p.ValidateFor(input, template) == nil {
		t.Fatal("empty acceptance accepted")
	}
	for _, text := range []string{" ", "a\x00b", strings.Repeat("a", 4097)} {
		p.AdditionalItems = []string{text}
		if p.ValidateFor(input, template) == nil {
			t.Fatal("invalid item accepted")
		}
	}
}
