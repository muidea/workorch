package specifications

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This fixture checks source preservation after explicit extraction. It does not
// claim that a model can extract every natural-language requirement correctly.
func TestAuthoringChecklistPreserves42ExtractedItemsAndReviewsEntireGoal(t *testing.T) {
	checks := make([]string, 42)
	for n := range checks {
		checks[n] = fmt.Sprintf("验收 %02d：成功返回原值；拒绝无效输入；阈值为 %d，例外须显式记录。", n+1, n+1)
	}
	input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-checklist", Revision: 1, GoalText: strings.Join(checks, "\n"), TemplateID: "template"}
	template := workspace.WorkspaceTemplate{ID: input.TemplateID, DoDItems: []string{"通用目标实现与验证"}}
	record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
	record, pending, _, err := record.ReserveDerivation("derive-checklist", "fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "fixture"}
	command, err := DerivationCommand(pending, target)
	if err != nil {
		t.Fatal(err)
	}
	var derivationInput struct {
		Goal string `json:"goal"`
	}
	if err := json.Unmarshal([]byte(command.Request.PromptContext.Sections[1].Content), &derivationInput); err != nil || derivationInput.Goal != input.GoalText {
		t.Fatalf("original checklist changed before extraction: %v", err)
	}
	derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: checks}
	record, err = record.CompleteDerivation(pending.PermitID, &derived, "")
	if err != nil {
		t.Fatal(err)
	}
	d := record.DerivationRequests[0]
	items, err := derived.CandidateItems(input, &template)
	if err != nil {
		t.Fatal(err)
	}
	mapping := workspace.DefinitionAuthoringMapping{Protocol: workspace.DefinitionAuthoringMappingProtocol, InputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, ResourceSnapshotHash: d.ResourceSnapshotHash, GoalRequirement: workspace.DefinitionAuthoringGoalMapping{DimensionRef: "all", ResourceIDs: []string{}}, Dimensions: []workspace.DefinitionAuthoringDimensionMapping{{Ref: "all", ResourceIDs: []string{}}}}
	for n, item := range items {
		ref := fmt.Sprintf("source-%03d", n)
		mapping.Requirements = append(mapping.Requirements, workspace.DefinitionAuthoringRequirementMapping{Ref: ref, DimensionRef: "all", SourceItemRefs: []string{item.CandidateItemRef}, ResourceIDs: []string{}, Text: "来源分组"})
		mapping.Obligations = append(mapping.Obligations, workspace.DefinitionAuthoringObligationMapping{CandidateItemRef: item.CandidateItemRef, RequirementRef: ref, Verification: workspace.DefinitionAuthoringVerificationMapping{Status: workspace.GoalVerificationUnverified, Declaration: "缺少可信验证声明"}})
	}
	payload, err := json.Marshal(struct {
		Mapping workspace.DefinitionAuthoringMapping `json:"mapping"`
	}{mapping})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := ParseAuthoringProposal(payload, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 43 || len(spec.Obligations) != 43 || len(spec.Requirements) != 44 {
		t.Fatal("checklist collapsed into generic template obligations")
	}
	for _, item := range items {
		count := 0
		for _, obligation := range spec.Obligations {
			if len(obligation.DoDItemRefs) == 1 && obligation.DoDItemRefs[0] == item.CandidateItemRef && obligation.Text == item.Text {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("source text not preserved exactly once: %s", item.CandidateItemRef)
		}
	}
	for _, kind := range ReviewKinds() {
		command, err := AuthoringReviewCommand(d, spec, kind, target)
		if err != nil {
			t.Fatal(err)
		}
		var reviewInput struct {
			RequiredRequirementIDs []string                          `json:"requiredRequirementIDs"`
			Goal                   workspace.DefinitionCandidateItem `json:"goal"`
		}
		if err := json.Unmarshal(authoringTestPayload(t, command), &reviewInput); err != nil || len(reviewInput.RequiredRequirementIDs) != 44 || reviewInput.Goal.Text != input.GoalText {
			t.Fatalf("%s lost original Goal or required scope: %v", kind, err)
		}
	}
	missing := mapping.Clone()
	missing.Obligations = missing.Obligations[:len(missing.Obligations)-1]
	payload, err = json.Marshal(struct {
		Mapping workspace.DefinitionAuthoringMapping `json:"mapping"`
	}{missing})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAuthoringProposal(payload, d); err == nil {
		t.Fatal("missing extracted item accepted")
	}
}
