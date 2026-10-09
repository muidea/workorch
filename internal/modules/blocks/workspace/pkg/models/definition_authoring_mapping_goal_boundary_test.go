package workspace

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestSealedMappingPreservesIndependentGoalByteBoundary(t *testing.T) {
	for _, size := range []int{4096, 4097, 32 * 1024, 32*1024 + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			input := DefinitionAuthoringInput{Version: DefinitionAuthoringInputVersionCurrent, WorkspaceID: "ws-goal-boundary", Revision: 1, GoalText: strings.Repeat("g", size), TemplateID: "t"}
			if size > 32*1024 {
				if input.Validate() == nil {
					t.Fatal("超出既有 Goal 边界的输入被接受")
				}
				return
			}
			template := WorkspaceTemplate{ID: "t", DoDItems: []string{"保留独立义务"}}
			record := DefinitionAuthoringRecord{Input: input, InputHash: input.InputHash(), TemplateSnapshot: template, TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&template)}
			record, derivation, _, err := record.ReserveDerivation("derive", "provider", nil)
			if err != nil {
				t.Fatal(err)
			}
			derived := DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash}
			record, err = record.CompleteDerivation(derivation.PermitID, &derived, "")
			if err != nil {
				t.Fatal(err)
			}
			derivation = record.DerivationRequests[0]
			mapping := mappingFixtureForDerivation(t, derivation)
			audit := mappingAuditFixture(t, mapping)
			decoded, err := DecodeDefinitionAuthoringMapping(audit.Document.Content)
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := CompileDefinitionAuthoringMapping(derivation, decoded)
			if err != nil {
				t.Fatalf("合法 Goal %d bytes 无法无损编译：%v", size, err)
			}
			goals := 0
			for _, requirement := range proposal.Requirements {
				if slices.Equal(requirement.SourceItemRefs, []string{DefinitionAuthoringGoalItemRef}) {
					goals++
					if requirement.Text != input.GoalText || len(requirement.Text) != size {
						t.Fatal("Goal 被截断或替换")
					}
				}
			}
			if goals != 1 || len(proposal.Obligations) != 1 {
				t.Fatal("Goal 被伪造成 DoD obligation")
			}
			if size > 4096 {
				items, err := derived.CandidateItems(input, &template)
				if err != nil {
					t.Fatal(err)
				}
				goal := DefinitionCandidateItem{CandidateItemRef: DefinitionAuthoringGoalItemRef, Text: input.GoalText}
				for _, field := range []string{"text", "source", "group", "required", "obligation"} {
					t.Run(field, func(t *testing.T) {
						changed := proposal.Clone()
						for i := range changed.Requirements {
							r := &changed.Requirements[i]
							if !slices.Equal(r.SourceItemRefs, []string{DefinitionAuthoringGoalItemRef}) {
								continue
							}
							switch field {
							case "text":
								r.Text = strings.Repeat("x", size)
							case "source":
								r.SourceKind = GoalRequirementDerived
							case "group":
								r.SourceItemRefs = append(r.SourceItemRefs, items[0].CandidateItemRef)
							case "required":
								r.Required = false
							case "obligation":
								changed.Obligations[0].Text = input.GoalText
							}
						}
						for _, kind := range goalReviewKinds {
							changed.Reviews = append(changed.Reviews, GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewUnknown, SpecificationHash: changed.ContentHash()})
						}
						if changed.Validate(goal, items) == nil {
							t.Fatal("非精确独立 Goal 或 obligation 借用了 Goal 字节边界")
						}
					})
				}
			}
			mapping.Requirements[0].Text = strings.Repeat("r", 4097)
			if _, err := CompileDefinitionAuthoringMapping(derivation, mapping); err == nil {
				t.Fatal("普通分组 requirement 的4096字节边界被扩张")
			}
		})
	}
}
