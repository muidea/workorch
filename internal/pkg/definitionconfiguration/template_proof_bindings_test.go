package definitionconfiguration

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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
			artifactRecipeBound := map[string]bool{}
			for _, r := range got.ProfileContracts[0].EvidenceRequirements {
				if len(r.LineageAssertions) > 0 {
					guardCount++
					if !reflect.DeepEqual(r.RequiredEvidenceKinds, []string{"validation"}) {
						t.Fatal("guard replaced ordinary validation")
					}
					if r.ID != fmt.Sprintf("dod-%03d", users+1) && r.ID != fmt.Sprintf("dod-%03d", users+3) && r.ID != fmt.Sprintf("dod-%03d", users+4) && r.ID != fmt.Sprintf("dod-%03d", users+13) {
						t.Fatal("guard attached to unrelated item", r.ID)
					}
					if r.LineageAssertions[0].ConsumptionPoint != runtimecontract.RuntimeProofPhasePostTerminalDelivery {
						t.Fatal("terminal proof introduced ordinary acceptance dependency")
					}
				}
				if r.ValidationRecipeID != "" {
					if r.ValidationRecipeID != "software-dev-artifact-validation-evidence" {
						t.Fatalf("unexpected exact validationRecipeId on %s: %q", r.ID, r.ValidationRecipeID)
					}
					artifactRecipeBound[r.ID] = true
				}
			}
			if guardCount != 4 {
				t.Fatalf("guards=%d", guardCount)
			}
			wantArtifactIDs := map[string]bool{
				fmt.Sprintf("dod-%03d", users+2):  true,
				fmt.Sprintf("dod-%03d", users+3):  true,
				fmt.Sprintf("dod-%03d", users+4):  true,
				fmt.Sprintf("dod-%03d", users+11): true,
				fmt.Sprintf("dod-%03d", users+12): true,
				fmt.Sprintf("dod-%03d", users+13): true,
			}
			if !reflect.DeepEqual(artifactRecipeBound, wantArtifactIDs) {
				t.Fatalf("artifact recipe bindings = %+v, want %+v", artifactRecipeBound, wantArtifactIDs)
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
		"unknown recipe": func(tpl *workspace.WorkspaceTemplate, i *[]workspace.DefinitionCandidateItem) {
			h := tpl.ContextHints.Map()
			h["templateProofBindings"].(map[string]any)["bindings"].([]any)[1].(map[string]any)["validationRecipeId"] = "undeclared-recipe"
			tpl.ContextHints = payload(t, h)
		},
		"empty validationRecipeId": func(tpl *workspace.WorkspaceTemplate, i *[]workspace.DefinitionCandidateItem) {
			h := tpl.ContextHints.Map()
			h["templateProofBindings"].(map[string]any)["bindings"].([]any)[1].(map[string]any)["validationRecipeId"] = "   "
			tpl.ContextHints = payload(t, h)
		},
		"missing all consumers": func(tpl *workspace.WorkspaceTemplate, i *[]workspace.DefinitionCandidateItem) {
			h := tpl.ContextHints.Map()
			b := h["templateProofBindings"].(map[string]any)["bindings"].([]any)[1].(map[string]any)
			delete(b, "validationRecipeId")
			delete(b, "lineageAssertions")
			tpl.ContextHints = payload(t, h)
		},
		"unknown field": func(tpl *workspace.WorkspaceTemplate, i *[]workspace.DefinitionCandidateItem) {
			h := tpl.ContextHints.Map()
			h["templateProofBindings"].(map[string]any)["bindings"].([]any)[1].(map[string]any)["extraField"] = true
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

func TestSealedWorkspaceTemplateUpdateRefreshesArtifactRecipesWithoutChangingStartupInput(t *testing.T) {
	currentTemplate, input, _ := templateProofFixture(t, 2)
	oldTemplate := currentTemplate.Clone()
	oldTemplate.EvidencePolicies = oldTemplate.EvidencePolicies[:1]
	oldHints := oldTemplate.ContextHints.Map()
	rawTPB := oldHints["templateProofBindings"].(map[string]any)
	rawBindings := rawTPB["bindings"].([]any)
	var historicalBindings []any
	for _, rawEntry := range rawBindings {
		entry := rawEntry.(map[string]any)
		if _, hasLineage := entry["lineageAssertions"]; !hasLineage {
			continue
		}
		delete(entry, "validationRecipeId")
		historicalBindings = append(historicalBindings, entry)
	}
	rawTPB["bindings"] = historicalBindings
	oldHints["executionLimits"] = limits(10, 5)
	oldTemplate.ContextHints = payload(t, oldHints)

	lim := executionLimits(oldTemplate.ContextHints.Map())
	limBinding := executionLimitsBinding(*lim)
	startup := &workspace.WorkspaceStartupInputContract{
		StartupInputHash:   "startup-hash-v1",
		DefinitionRevision: "def-rev-1",
	}
	admission := &workspace.WorkspaceActivationAdmission{
		CandidateHash:    "cand-hash-1",
		StartupInputHash: startup.StartupInputHash,
		State:            workspace.WorkspaceActivationEligible,
	}
	sealedWS := &workspace.Workspace{
		ID:                  input.WorkspaceID,
		TemplateID:          currentTemplate.ID,
		ProfileID:           currentTemplate.ID,
		DefaultSoulID:       common.SoulID(oldTemplate.DefaultSoulID),
		SoulBindings:        map[string]common.SoulID{"planning": common.SoulID(oldTemplate.SoulBindings["planning"])},
		ContextHints:        oldTemplate.ContextHints.Clone(),
		EvidencePolicies:    append([]workorcheventpayload.Payload(nil), oldTemplate.EvidencePolicies...),
		StartupInput:        startup,
		ActivationAdmission: admission,
		DefinitionAdmissionCandidates: []workspace.DefinitionAdmissionCandidate{{
			CandidateHash:           "cand-hash-1",
			AdmissionPolicyBindings: []workspace.DefinitionAdmissionPolicyBinding{limBinding},
		}},
		DefinitionAuthoring: &workspace.DefinitionAuthoringRecord{
			TemplateSnapshot: oldTemplate,
			Input:            input,
		},
	}

	resolvedWS, err := workspace.WorkspaceForCurrentTemplate(sealedWS, &currentTemplate)
	if err != nil {
		t.Fatalf("WorkspaceForCurrentTemplate() failed on sealed workspace upgrade: %v", err)
	}
	if len(resolvedWS.EvidencePolicies) != 2 {
		t.Fatalf("expected inherited template EvidencePolicies upgrade to 2 policies, got %d", len(resolvedWS.EvidencePolicies))
	}
	if !reflect.DeepEqual(resolvedWS.StartupInput, startup) || !reflect.DeepEqual(resolvedWS.ActivationAdmission, admission) {
		t.Fatal("template update modified StartupInput or ActivationAdmission")
	}

	compiled, err := Compile(sealedWS, &currentTemplate)
	if err != nil {
		t.Fatalf("Compile() failed on existing sealed workspace restart: %v", err)
	}
	if compiled.ExecutionLimits == nil || runtimecontract.StableRuntimeExecutionLimitsHash(*compiled.ExecutionLimits) != limBinding.SnapshotHash {
		t.Fatalf("confirmed execution limits drifted on template upgrade: %+v", compiled.ExecutionLimits)
	}
	artifactCount := 0
	for _, r := range compiled.ProfileContracts[0].EvidenceRequirements {
		switch r.ID {
		case "dod-004", "dod-005", "dod-006", "dod-013", "dod-014", "dod-015":
			if r.ValidationRecipeID != "software-dev-artifact-validation-evidence" {
				t.Fatalf("expected artifact recipe on %s after restart, got %q", r.ID, r.ValidationRecipeID)
			}
			artifactCount++
		default:
			if r.ValidationRecipeID != "" {
				t.Fatalf("ordinary/goal item %s unexpectedly got recipe %q", r.ID, r.ValidationRecipeID)
			}
		}
	}
	if artifactCount != 6 {
		t.Fatalf("artifactCount=%d, want 6", artifactCount)
	}
}
