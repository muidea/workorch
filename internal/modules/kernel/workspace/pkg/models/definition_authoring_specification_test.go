package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func authoringSpecificationFixture(t testing.TB) (DefinitionAuthoringInput, WorkspaceTemplate, DefinitionDerivedAcceptance, GoalSpecification) {
	t.Helper()
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "交付结果", TemplateID: "t", AcceptanceRequirements: []string{"用户要求"}}
	template := WorkspaceTemplate{ID: "t", DoDItems: []string{"模板要求"}}
	derived := DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template), AdditionalItems: []string{"补充要求"}}
	items, err := derived.CandidateItems(input, &template)
	if err != nil {
		t.Fatal(err)
	}
	spec := GoalSpecification{Version: GoalSpecificationVersionCurrent, GoalItemRef: DefinitionAuthoringGoalItemRef, GoalTextHash: StableDefinitionTextHash(input.GoalText), Dimensions: []GoalSpecificationDimension{{ID: "dimension", Required: true, Activated: true}}, Requirements: []GoalSpecificationRequirement{{ID: "goal", DimensionID: "dimension", Required: true, SourceKind: GoalRequirementUserExplicit, SourceItemRefs: []string{DefinitionAuthoringGoalItemRef}, Text: input.GoalText}}, Obligations: []GoalSpecificationObligation{{ID: "goal-obligation", RequirementID: "goal", DoDItemRefs: []string{items[0].CandidateItemRef}, Text: "验证目标"}}}
	for index, item := range items {
		kind := GoalRequirementDerived
		switch {
		case strings.HasPrefix(item.CandidateItemRef, "derived-dod:user:"):
			kind = GoalRequirementUserExplicit
		case strings.HasPrefix(item.CandidateItemRef, "derived-dod:template:"):
			kind = GoalRequirementTemplateRequired
		}
		reqID := "req-" + string(rune('a'+index))
		obligationID := "obl-" + string(rune('a'+index))
		spec.Requirements = append(spec.Requirements, GoalSpecificationRequirement{ID: reqID, DimensionID: "dimension", Required: true, SourceKind: kind, SourceItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
		spec.Obligations = append(spec.Obligations, GoalSpecificationObligation{ID: obligationID, RequirementID: reqID, DoDItemRefs: []string{item.CandidateItemRef}, Text: item.Text})
		spec.Verifications = append(spec.Verifications, GoalSpecificationVerification{ID: "verify-" + reqID, ObligationID: obligationID, Declaration: "尚未声明", Status: GoalVerificationUnverified})
	}
	spec.Verifications = append(spec.Verifications, GoalSpecificationVerification{ID: "verify-goal", ObligationID: "goal-obligation", Declaration: "尚未声明", Status: GoalVerificationUnverified})
	return input, template, derived, spec
}

func TestAuthoringSpecificationRequiresExactPersistedResources(t *testing.T) {
	input, template, _, spec := authoringSpecificationFixture(t)
	input.ResourceRefs = []string{"requirements.txt", "more.txt"}
	derived := DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template), AdditionalItems: []string{"补充要求"}}
	content := []string{"第一份材料", "第二份材料"}
	snapshots := make([]DefinitionAuthoringResourceSnapshot, 0, len(content))
	for index, text := range content {
		sum := sha256.Sum256([]byte(text))
		snapshots = append(snapshots, DefinitionAuthoringResourceSnapshot{Ref: input.ResourceRefs[index], Content: text, ContentHash: "sha256:" + hex.EncodeToString(sum[:])})
	}
	expected, err := AuthoringSpecificationResources(input, snapshots)
	if err != nil {
		t.Fatal(err)
	}
	spec.Resources = expected
	if err := ValidateAuthoringSpecification(input, &template, derived, snapshots, spec); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*GoalSpecification){
		func(value *GoalSpecification) { value.Resources = value.Resources[:1] },
		func(value *GoalSpecification) { value.Resources[0].Available = false },
		func(value *GoalSpecification) { value.Resources[0].SnapshotHash = "sha256:forged" },
		func(value *GoalSpecification) { value.Resources[0].Ref = "other.txt" },
		func(value *GoalSpecification) { value.Resources[0].Required = false },
		func(value *GoalSpecification) { value.Resources[0].ID = "another-identity" },
		func(value *GoalSpecification) { value.Resources[1] = value.Resources[0] },
	} {
		changed := spec.Clone()
		change(&changed)
		if err := ValidateAuthoringSpecification(input, &template, derived, snapshots, changed); err == nil {
			t.Fatal("provider changed a File-origin resource fact")
		}
	}
	if err := ValidateAuthoringSpecification(input, &template, derived, nil, spec); err == nil {
		t.Fatal("resource refs were accepted without a snapshot")
	}
}

func TestAuthoringSpecificationResourcesRejectsSelfInconsistentSnapshot(t *testing.T) {
	input := DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws", Revision: 1, GoalText: "目标", TemplateID: "t", ResourceRefs: []string{"requirements.txt"}}
	if _, err := AuthoringSpecificationResources(input, []DefinitionAuthoringResourceSnapshot{{Ref: "requirements.txt", Content: "实际内容", ContentHash: "sha256:forged"}}); err == nil {
		t.Fatal("snapshot with wrong content hash was accepted")
	}
}

func TestAuthoringSpecificationPreservesSealedGoalAndAllItemSources(t *testing.T) {
	input, template, derived, spec := authoringSpecificationFixture(t)
	if err := ValidateAuthoringSpecification(input, &template, derived, nil, spec); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*GoalSpecification)
	}{
		{"dropped user requirement", func(s *GoalSpecification) { s.Obligations[1].DoDItemRefs = nil }},
		{"downgraded template requirement", func(s *GoalSpecification) { s.Requirements[2].Required = false }},
		{"relabelled template", func(s *GoalSpecification) { s.Requirements[2].SourceKind = GoalRequirementDerived }},
		{"rewritten template text", func(s *GoalSpecification) { s.Obligations[2].Text = "弱化" }},
		{"dropped generated item", func(s *GoalSpecification) { s.Obligations[3].DoDItemRefs = nil }},
		{"lost original goal", func(s *GoalSpecification) { s.Requirements[0].Text = "另一目标" }},
		{"optional requirement", func(s *GoalSpecification) { s.Requirements[3].Required = false }},
		{"unverified mapping", func(s *GoalSpecification) { s.Verifications[2].Declaration = "" }},
		{"inactive required dimension", func(s *GoalSpecification) { s.Dimensions[0].Activated = false }},
		{"provider invented resource", func(s *GoalSpecification) {
			s.Resources = []GoalSpecificationResource{{ID: "r", Ref: "file", Available: true, SnapshotHash: "untrusted"}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			altered := spec.Clone()
			test.change(&altered)
			if err := ValidateAuthoringSpecification(input, &template, derived, nil, altered); err == nil {
				t.Fatal("changed or missing sealed authoring requirement was accepted")
			}
		})
	}
	input.ResourceRefs = []string{"requirements.txt"}
	if err := ValidateAuthoringSpecification(input, &template, derived, nil, spec); err == nil {
		t.Fatal("unverified resource references passed authoring specification validation")
	}
}
