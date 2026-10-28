package specifications

import (
	"encoding/json"
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAuthoringVerificationOptionsRespectAfterImageOrderScopeAndMergedPolicy(t *testing.T) {
	items := []workspace.DefinitionCandidateItem{{CandidateItemRef: "derived-dod:user:000002"}, {CandidateItemRef: "derived-dod:user:000001"}}
	requirement := func(id, scope string) runtimecontract.RuntimeProfileEvidenceRequirement {
		return runtimecontract.RuntimeProfileEvidenceRequirement{ID: id, Scope: scope, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}}
	}
	profiles := []runtimecontract.RuntimeProfileContract{
		{ID: "exact", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{requirement("dod-001", "dodItem")}},
		{ID: "all", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{requirement("*", "dodItem")}},
		{ID: "other-scope", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{requirement("*", "deliverable")}},
	}
	before := runtimecontract.CloneRuntimeProfileContracts(profiles)
	got := authoringVerificationOptions(items, profiles)
	want := []authoringCandidateVerificationOptions{
		{items[0].CandidateItemRef, []authoringVerificationDeclaration{{"all", "*", "dodItem", []runtimecontract.RuntimeLineageAssertionRule{}}}},
		{items[1].CandidateItemRef, []authoringVerificationDeclaration{{"exact", "dod-001", "dodItem", []runtimecontract.RuntimeLineageAssertionRule{}}, {"all", "*", "dodItem", []runtimecontract.RuntimeLineageAssertionRule{}}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(profiles, before) {
		t.Fatal("option compilation changed sealed profiles")
	}
	if got := authoringVerificationOptions(items, nil); len(got) != 2 || got[0].Declarations == nil || len(got[0].Declarations) != 0 {
		t.Fatalf("absent declaration must remain explicit empty: %+v", got)
	}
	for n := 0; n < 2; n++ {
		profiles[n].EvidenceRequirements[0].VerificationPolicy = &runtimecontract.RuntimeVerificationPolicy{Version: runtimecontract.RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: 2 + 2*n}
	}
	got = authoringVerificationOptions(items, profiles)
	if len(got[1].Declarations) != 0 || len(got[0].Declarations) != 1 || got[0].Declarations[0].ProfileID != "all" {
		t.Fatalf("conflicting merged policy was advertised as applicable: %+v", got)
	}
}

func TestDefaultSoftwareTemplateProvidesSourceOptionsWithoutBorrowingDeliverables(t *testing.T) {
	templates, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range templates {
		if value.ID != "software-dev" {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var template workspace.WorkspaceTemplate
		if err := json.Unmarshal(encoded, &template); err != nil {
			t.Fatal(err)
		}
		input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-default-options", Revision: 1, TemplateID: template.ID, GoalText: "验证默认模板来源"}
		derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
		items, err := derived.CandidateItems(input, &template)
		if err != nil {
			t.Fatal(err)
		}
		bindings, err := definitionconfiguration.Compile(&workspace.Workspace{}, &template)
		if err != nil {
			t.Fatal(err)
		}
		options := authoringVerificationOptions(items, bindings.ProfileContracts)
		if len(options) != len(template.DoDItems) || len(options) == 0 {
			t.Fatal("default template sources missing")
		}
		for _, item := range options {
			if len(item.Declarations) == 0 {
				t.Fatalf("default DoD source has no applicable declaration: %s", item.CandidateItemRef)
			}
			for _, declaration := range item.Declarations {
				wantConsumer := item.CandidateItemRef == "derived-dod:template:000003" || item.CandidateItemRef == "derived-dod:template:000004" || item.CandidateItemRef == "derived-dod:template:000013"
				if wantConsumer != (len(declaration.Consumers) != 0) {
					t.Fatalf("consumer source mismatch: %+v", item)
				}
				for _, consumer := range declaration.Consumers {
					if consumer.ConsumptionPoint != "post_terminal_delivery" || consumer.TerminalContractRef == "" {
						t.Fatal("missing actual consumer phase/version")
					}
				}
				if declaration.Scope != "dodItem" {
					t.Fatalf("borrowed another source domain: %+v", declaration)
				}
			}
		}
		return
	}
	t.Fatal("default software template missing")
}
