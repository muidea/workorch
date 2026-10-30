package specifications

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAuthoringReviewsShareFullSnapshotWithIndependentTail(t *testing.T) {
	for _, wireRef := range []string{"", workspace.DefinitionAuthoringOwnerBoundV2, workspace.DefinitionAuthoringOwnerBoundV3} {
		t.Run(wireRef, func(t *testing.T) {
			input := workspace.DefinitionAuthoringInput{Version: 1, WorkspaceID: "ws-cache", Revision: 1, GoalText: "完整目标\n不能省略条件", TemplateID: "t", AcceptanceRequirements: []string{"用户验收"}}
			template := workspace.WorkspaceTemplate{ID: "t", DoDItems: []string{"模板验收"}}
			record := workspace.DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&template)}
			record, d, _, err := record.ReserveDerivation("derive", "provider", nil, wireRef)
			if err != nil {
				t.Fatal(err)
			}
			derived := workspace.DefinitionDerivedAcceptance{AuthoringInputHash: input.InputHash(), TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"派生验收"}}
			if wireRef == "" {
				record, err = record.CompleteDerivation(d.PermitID, &derived, "")
			} else {
				raw := []byte(`{"AdditionalItems":["派生验收"]}`)
				derived, err = workspace.DecodeOwnerBoundDerivation(d, raw)
				if err != nil {
					t.Fatal(err)
				}
				hash := sha256.Sum256(raw)
				observation := workspace.DefinitionProviderObservation{Observed: true, Complete: true, Saved: true, ObservedBytes: uint64(len(raw)), SHA256: hex.EncodeToString(hash[:]), Content: raw}
				audit := workspace.DefinitionAuthoringSuccessByteAudit{Document: observation, Wire: observation.Clone()}
				record, err = record.CompleteDerivationDocument(d.PermitID, &derived, "", &audit, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			d = record.DerivationRequests[0]

			spec := authoringCommandFixture(t, input, template, derived)
			target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model"}
			var prefix []llmmodels.PromptSection
			tails := map[string]bool{}
			for _, kind := range ReviewKinds() {
				command, err := AuthoringReviewCommand(d, spec, kind, target)
				if err != nil {
					t.Fatal(err)
				}
				ctx := command.Request.PromptContext
				if ctx.MessageLayout != llmmodels.PromptMessageLayoutSectionsV1 || len(ctx.Sections) != 4 {
					t.Fatal("missing explicit partitions")
				}
				if prefix == nil {
					prefix = ctx.Sections[:3]
				} else if !reflect.DeepEqual(prefix, ctx.Sections[:3]) {
					t.Fatal("kind changed shared snapshot or contract")
				}
				if ctx.Sections[1].Role != "user" || ctx.Sections[2].Role != "user" || !ctx.Sections[1].Protected || !ctx.Sections[2].Protected {
					t.Fatal("review data promoted or unprotected")
				}
				var joined authoringReviewInput
				if err := json.Unmarshal(authoringTestPayload(t, command), &joined); err != nil {
					t.Fatal(err)
				}
				expectedIDs, _ := spec.RequiredReviewRequirementIDs(kind)
				if joined.Goal.Text != input.GoalText || !reflect.DeepEqual(joined.Specification, spec) || !reflect.DeepEqual(joined.RequiredRequirementIDs, expectedIDs) || joined.Kind != kind || joined.ProposalHash != spec.ContentHash() {
					t.Fatal("partition changed complete input")
				}
				if !strings.Contains(ctx.Sections[3].Content, `"kind":"`+kind+`"`) || strings.Contains(ctx.Sections[1].Content, `"kind":`) {
					t.Fatal("review kind leaked into snapshot")
				}
				tails[ctx.Sections[3].Content] = true
			}
			if len(tails) != 6 {
				t.Fatal("independent reviewers collapsed")
			}
		})
	}
}
