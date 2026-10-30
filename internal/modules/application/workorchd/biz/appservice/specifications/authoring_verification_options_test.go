package specifications

import (
	"encoding/json"
	"reflect"
	"sync"
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
	got := mustAuthoringVerificationOptions(t, items, profiles)
	want := []authoringCandidateVerificationOptions{
		{CandidateItemRef: items[0].CandidateItemRef, SourceScope: "dodItem", SourceID: "dod-002", DeclarationBindingMode: authoringSingleDeclarationBinding, Declarations: []authoringVerificationDeclaration{{"all", "*", "dodItem"}}, MergedRequiredEvidenceKinds: []string{"validation"}, MergedRequiredConsumers: []runtimecontract.RuntimeLineageAssertionRule{}},
		{CandidateItemRef: items[1].CandidateItemRef, SourceScope: "dodItem", SourceID: "dod-001", DeclarationBindingMode: authoringSingleDeclarationBinding, Declarations: []authoringVerificationDeclaration{{"exact", "dod-001", "dodItem"}, {"all", "*", "dodItem"}}, MergedRequiredEvidenceKinds: []string{"validation"}, MergedRequiredConsumers: []runtimecontract.RuntimeLineageAssertionRule{}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("options = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(profiles, before) {
		t.Fatal("option compilation changed sealed profiles")
	}
	if got := mustAuthoringVerificationOptions(t, items, nil); len(got) != 2 || got[0].Declarations == nil || len(got[0].Declarations) != 0 {
		t.Fatalf("absent declaration must remain explicit empty: %+v", got)
	}
	for n := 0; n < 2; n++ {
		profiles[n].EvidenceRequirements[0].VerificationPolicy = &runtimecontract.RuntimeVerificationPolicy{Version: runtimecontract.RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: 2 + 2*n}
	}
	if _, err := authoringVerificationOptions(items, profiles); err == nil {
		t.Fatal("conflicting merged policy reached provider input")
	}
}

func TestAuthoringVerificationOptionsConcurrentProfilesKeepIndependentContracts(t *testing.T) {
	items := []workspace.DefinitionCandidateItem{{CandidateItemRef: "derived-dod:user:000001"}}
	profiles := [][]runtimecontract.RuntimeProfileContract{
		{{ID: "document-review", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
			ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		}}}},
		{{ID: "inventory-audit", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
			ID: "dod-001", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			LineageAssertions: []runtimecontract.RuntimeLineageAssertionRule{{Ref: "audit-delivery", Kind: runtimecontract.RuntimeLineageDeliveryAcknowledgements, ConsumptionPoint: runtimecontract.RuntimeProofPhasePostTerminalDelivery}},
		}}}},
	}
	expected := make([][]authoringCandidateVerificationOptions, len(profiles))
	for i := range profiles {
		expected[i] = mustAuthoringVerificationOptions(t, items, profiles[i])
		if len(expected[i]) != 1 || len(expected[i][0].Declarations) != 1 {
			t.Fatalf("profile %s is not applicable to the exact source", profiles[i][0].ID)
		}
	}
	var wg sync.WaitGroup
	for i := range profiles {
		for range 8 {
			wg.Go(func() {
				got, err := authoringVerificationOptions(items, profiles[i])
				if err != nil || !reflect.DeepEqual(got, expected[i]) {
					t.Errorf("profile %s borrowed another contract: %v", profiles[i][0].ID, err)
					return
				}
				got[0].Declarations[0].ProfileID = "local-only"
				got[0].MergedRequiredEvidenceKinds[0] = "local-only"
				if len(got[0].MergedRequiredConsumers) != 0 {
					got[0].MergedRequiredConsumers[0].Ref = "local-only"
				}
			})
		}
	}
	wg.Wait()
	for i := range profiles {
		if !reflect.DeepEqual(mustAuthoringVerificationOptions(t, items, profiles[i]), expected[i]) {
			t.Fatal("concurrent rendering mutated sealed Profile")
		}
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
		options := mustAuthoringVerificationOptions(t, items, bindings.ProfileContracts)
		if len(options) != len(template.DoDItems) || len(options) == 0 {
			t.Fatal("default template sources missing")
		}
		for _, item := range options {
			if len(item.Declarations) == 0 {
				t.Fatalf("default DoD source has no applicable declaration: %s", item.CandidateItemRef)
			}
			for _, declaration := range item.Declarations {
				wantConsumer := item.CandidateItemRef == "derived-dod:template:000003" || item.CandidateItemRef == "derived-dod:template:000004" || item.CandidateItemRef == "derived-dod:template:000013"
				if wantConsumer != (len(item.MergedRequiredConsumers) != 0) {
					t.Fatalf("consumer source mismatch: %+v", item)
				}
				for _, consumer := range item.MergedRequiredConsumers {
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

func mustAuthoringVerificationOptions(t *testing.T, items []workspace.DefinitionCandidateItem, profiles []runtimecontract.RuntimeProfileContract) []authoringCandidateVerificationOptions {
	t.Helper()
	value, err := authoringVerificationOptions(items, profiles)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestAuthoringVerificationOptionsDoNotInventOrAliasConstraints(t *testing.T) {
	items := []workspace.DefinitionCandidateItem{{CandidateItemRef: "derived-dod:user:000001"}}
	profiles := []runtimecontract.RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
		ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		LineageAssertions: []runtimecontract.RuntimeLineageAssertionRule{{Ref: "delivery", Kind: runtimecontract.RuntimeLineageDeliveryAcknowledgements, ConsumptionPoint: runtimecontract.RuntimeProofPhasePostTerminalDelivery}},
	}}}}
	before := runtimecontract.CloneRuntimeProfileContracts(profiles)
	first := mustAuthoringVerificationOptions(t, items, profiles)
	first[0].Declarations[0].ProfileID = "another-profile"
	first[0].MergedRequiredEvidenceKinds[0] = "fabricated"
	first[0].MergedRequiredConsumers[0].Ref = "fabricated"
	if !reflect.DeepEqual(profiles, before) {
		t.Fatal("provider projection aliases sealed profile")
	}
	second := mustAuthoringVerificationOptions(t, items, profiles)
	if second[0].Declarations[0].ProfileID != "profile" || second[0].MergedRequiredConsumers[0].Ref != "delivery" {
		t.Fatal("one request changed another request's constraints")
	}
	profiles[0].EvidenceRequirements[0].LineageAssertions = nil
	without := mustAuthoringVerificationOptions(t, items, profiles)
	if len(without[0].MergedRequiredConsumers) != 0 || without[0].MergedRequiredConsumers == nil {
		t.Fatal("missing consumer invented or hidden")
	}
	noDeclaration := mustAuthoringVerificationOptions(t, items, nil)
	if len(noDeclaration[0].Declarations) != 0 || len(noDeclaration[0].MergedRequiredConsumers) != 0 {
		t.Fatal("compiler fallback evidence kind invented an applicable declaration")
	}
	for _, invalid := range [][]workspace.DefinitionCandidateItem{
		{{CandidateItemRef: "unknown-source"}},
		{{CandidateItemRef: items[0].CandidateItemRef}, {CandidateItemRef: items[0].CandidateItemRef}},
	} {
		if _, err := authoringVerificationOptions(invalid, profiles); err == nil {
			t.Fatal("ambiguous source identity accepted")
		}
	}
}
