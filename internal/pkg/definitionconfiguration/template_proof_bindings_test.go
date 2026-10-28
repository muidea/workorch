package definitionconfiguration

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func templateProofFixture(t *testing.T, users int) (workspace.WorkspaceTemplate, workspace.DefinitionAuthoringInput, []workspace.DefinitionCandidateItem) {
	t.Helper()
	var template workspace.WorkspaceTemplate
	templates, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range templates {
		if v.ID == "software-dev" {
			raw, _ := json.Marshal(v)
			if err = json.Unmarshal(raw, &template); err != nil {
				t.Fatal(err)
			}
		}
	}
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-proof", Revision: 1, TemplateID: template.ID, GoalText: "完整目标"}
	for i := 0; i < users; i++ {
		input.AcceptanceRequirements = append(input.AcceptanceRequirements, fmt.Sprintf("用户条件 %d", i))
	}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template), AdditionalItems: []string{"额外真实验收"}}
	items, err := derived.CandidateItems(input, &template)
	if err != nil {
		t.Fatal(err)
	}
	return template, input, items
}
func TestTemplateProofSourcesResolveExactOriginalItemsAcrossUserOffsets(t *testing.T) {
	for _, users := range []int{0, 1, 4} {
		t.Run(fmt.Sprint(users), func(t *testing.T) {
			template, input, items := templateProofFixture(t, users)
			before, _ := json.Marshal(template)
			got, err := CompileForItems(&workspace.Workspace{}, &template, items)
			if err != nil {
				t.Fatal(err)
			}
			if err = runtimecontract.ValidateRuntimeProfileContracts(got.ProfileContracts); err != nil {
				t.Fatal(err)
			}
			guardCount := 0
			for _, r := range got.ProfileContracts[0].EvidenceRequirements {
				if len(r.LineageAssertions) == 0 {
					continue
				}
				guardCount++
				if !reflect.DeepEqual(r.RequiredEvidenceKinds, []string{"validation"}) {
					t.Fatal("guard replaced ordinary validation")
				}
				if r.ID != fmt.Sprintf("dod-%03d", users+3) && r.ID != fmt.Sprintf("dod-%03d", users+4) && r.ID != fmt.Sprintf("dod-%03d", users+13) {
					t.Fatal("guard attached to unrelated item", r.ID)
				}
				if r.LineageAssertions[0].ConsumptionPoint != runtimecontract.RuntimeProofPhasePostTerminalDelivery {
					t.Fatal("terminal proof introduced ordinary acceptance dependency")
				}
			}
			if guardCount != 3 {
				t.Fatalf("guards=%d", guardCount)
			}
			current := &workspace.Workspace{DefinitionAuthoring: &workspace.DefinitionAuthoringRecord{Input: input}}
			again, err := Compile(current, &template)
			if err != nil || again.ProfileContractsHash != got.ProfileContractsHash {
				t.Fatal("configuration and authoring compile diverged", err)
			}
			after, _ := json.Marshal(template)
			if string(before) != string(after) {
				t.Fatal("compiler mutated sealed template")
			}
		})
	}
}
func TestTemplateProofSourcesRejectMissingAmbiguousTextScopeAndRuleDrift(t *testing.T) {
	template, _, items := templateProofFixture(t, 1)
	for name, change := range map[string]func(*workspace.WorkspaceTemplate, *[]workspace.DefinitionCandidateItem){
		"missing": func(tpl *workspace.WorkspaceTemplate, i *[]workspace.DefinitionCandidateItem) {
			*i = append((*i)[:3:3], (*i)[4:]...)
		},
		"duplicate": func(tpl *workspace.WorkspaceTemplate, i *[]workspace.DefinitionCandidateItem) {
			*i = append(*i, (*i)[3])
		},
		"text": func(tpl *workspace.WorkspaceTemplate, i *[]workspace.DefinitionCandidateItem) {
			(*i)[3].Text += "漂移"
		},
		"reorder": func(tpl *workspace.WorkspaceTemplate, i *[]workspace.DefinitionCandidateItem) {
			tpl.DoDItems[2], tpl.DoDItems[3] = tpl.DoDItems[3], tpl.DoDItems[2]
		},
		"unknown consumer": func(tpl *workspace.WorkspaceTemplate, i *[]workspace.DefinitionCandidateItem) {
			h := tpl.ContextHints.Map()
			h["templateProofBindings"].(map[string]any)["bindings"].([]any)[0].(map[string]any)["lineageAssertions"].([]any)[0].(map[string]any)["kind"] = "unknown"
			tpl.ContextHints = payload(t, h)
		},
	} {
		t.Run(name, func(t *testing.T) {
			copy := template.Clone()
			sources := append([]workspace.DefinitionCandidateItem{}, items...)
			change(&copy, &sources)
			got, err := CompileForItems(&workspace.Workspace{}, &copy, sources)
			if err == nil && runtimecontract.ValidateRuntimeProfileContracts(got.ProfileContracts) == nil {
				t.Fatal("invalid proof source accepted")
			}
		})
	}
	changed := template.ContextHints.Map()
	changed["templateProofBindings"] = map[string]any{"protocolRef": "forged"}
	if _, err := CompileForItems(&workspace.Workspace{ContextHints: payload(t, changed)}, &template, items); err == nil {
		t.Fatal("Workspace replaced template contract")
	}
}

func TestTemplateProofRejectsFormalRowsWithoutOriginalSourceInventory(t *testing.T) {
	template, _, _ := templateProofFixture(t, 0)
	items := []workspace.DefinitionCandidateItem{}
	for i, text := range template.DoDItems {
		items = append(items, workspace.DefinitionCandidateItem{CandidateItemRef: fmt.Sprintf("dod:formal:item:%06d", i+1), Text: text, SourceIdentity: "dod:formal@r1"})
	}
	if _, err := CompileForItems(&workspace.Workspace{}, &template, items); err == nil {
		t.Fatal("guessed template sources from matching formal text")
	}
	if _, err := Compile(&workspace.Workspace{StartupInput: &workspace.WorkspaceStartupInputContract{}}, &template); err == nil {
		t.Fatal("used template preview as sealed startup provenance")
	}
}
