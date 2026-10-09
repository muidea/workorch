package workspace

import (
	"context"
	"reflect"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestAuthoringSummaryRechecksSealedProfileWithoutMutatingLedger(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	cmd := seedReviewedAuthoringConfirmation(t, owners, "ws-summary-profile")
	current, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	before := current.DefinitionAuthoring.Clone()
	valid := workspaceDTOWithAuthoringSummary(current)
	if valid == nil || valid.AuthoringSummary == nil || valid.AuthoringSummary.State != "reviewed" || valid.AuthoringSummary.ReviewCount != 6 {
		t.Fatalf("合法封存声明未显示已审查：%+v", valid)
	}
	if !reflect.DeepEqual(before, *current.DefinitionAuthoring) {
		t.Fatal("摘要投影改写原 ledger")
	}

	for _, field := range []string{"profile", "declaration", "scope"} {
		t.Run(field, func(t *testing.T) {
			record := before.Clone()
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
				t.Fatalf("协调伪造必须通过结构校验：%v", err)
			}
			if _, _, err := record.ConfirmableSpecification(cmd.DerivationPermitID, proposal.ContentHash()); err != nil {
				t.Fatalf("反例必须到达独立 Profile 检查：%v", err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				item := *current
				item.DefinitionAuthoring = &record
				snapshot := record.Clone()
				projected := workspaceDTOsWithAuthoringSummary([]*workspace.Workspace{&item})
				if len(projected) != 1 || projected[0].AuthoringSummary == nil || projected[0].AuthoringSummary.State != "review_blocked" || projected[0].AuthoringSummary.ReviewCount != 6 {
					t.Fatalf("非法自报声明被列表摘要认可：%+v", projected)
				}
				if !reflect.DeepEqual(snapshot, record) || item.StartupInput != nil || item.ActivationAdmission != nil || item.ActiveRunID != "" {
					t.Fatal("只读摘要产生或改写 authority")
				}
			}
		})
	}
}
