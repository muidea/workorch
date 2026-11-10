package workspace

import (
	"context"
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestAuthoringConfirmationRejectsCompletedSourceRewriteBeforePrepareAndRecovery(t *testing.T) {
	for _, phase := range []string{"before-prepare", "prepared"} {
		for _, field := range []string{"profile", "declaration", "scope"} {
			t.Run(phase+"/"+field, func(t *testing.T) {
				ctx := context.Background()
				root := t.TempDir()
				owners := openFileDefinitionAdmissionOwners(t, root)
				defer func() { owners.close() }()
				cmd := seedReviewedAuthoringConfirmation(t, owners, "ws-profile-recheck")
				current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
				if err != nil {
					t.Fatal(err)
				}
				journal, err := owners.workspace.prepareDefinitionAuthoringConfirmation(ctx, current, cmd)
				if err != nil {
					t.Fatal(err)
				}
				record := current.DefinitionAuthoring.Clone()
				proposal := record.SpecificationRequests[0].Proposal.Clone()
				switch field {
				case "profile":
					proposal.Verifications[0].ProfileID = "invented"
				case "declaration":
					proposal.Verifications[0].EvidenceRequirementID = "invented"
				case "scope":
					proposal.Verifications[0].Scope = "deliverable"
				}
				record.SpecificationRequests[0].Proposal = &proposal
				for index := range record.SpecificationRequests {
					request := &record.SpecificationRequests[index]
					if request.Review != nil {
						request.ProposalHash = proposal.ContentHash()
						request.Review.SpecificationHash = proposal.ContentHash()
						request.PermitID = request.ExpectedPermitID()
					}
				}
				resealAuthoringProposalClaimForTest(t, &record, proposal)
				if err := record.Validate(); err != nil {
					t.Fatalf("协调伪造必须通过结构校验，才能验证独立 Profile 门禁：%v", err)
				}
				current.DefinitionAuthoring = &record
				cmd = authoringConfirmationCommandFromPersistedLedger(t, current)
				if phase == "prepared" {
					// 保持完整自洽的 journal/六审身份，不能只靠 hash 漂移拒绝恢复。
					journal.ProposalHash, journal.SpecificationHash = cmd.ProposalHash, cmd.SpecificationHash
					identity := workspace.StableDefinitionAuthoringConfirmationID(current.ID, cmd.InputHash, cmd.DerivationPermitID, cmd.SpecificationHash)
					journal.Goal.ID = common.GoalID("authoring-goal-" + identity)
					journal.DoD.ID = common.DoDID("authoring-dod-" + identity)
					if err := journal.Validate(&record, current.ID); err != nil {
						t.Fatalf("反例 journal 必须结构自洽：%v", err)
					}
					current.DefinitionAuthoringConfirmation = &journal
				}
				persisted, err := owners.repositories.Workspaces.Get(ctx, current.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err == nil {
					t.Fatal("rewrote committed source before confirmation recovery")
				}
				actual, err := owners.repositories.Workspaces.Get(ctx, current.ID)
				if err != nil || !reflect.DeepEqual(persisted, actual) {
					t.Fatal("rejected source rewrite changed original")
				}
				owners.close()
				owners = openFileDefinitionAdmissionOwners(t, root)
				original := authoringConfirmationCommandFromPersistedLedger(t, actual)
				if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: original}); err != nil {
					t.Fatal(err)
				}

			})
		}
	}
}
