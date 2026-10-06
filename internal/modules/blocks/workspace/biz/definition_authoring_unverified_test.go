package workspace

import (
	"context"
	"reflect"
	"testing"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func TestSixPASSReviewsWithUnverifiedCannotMaterializeDefinitions(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	owners := openFileDefinitionAdmissionOwners(t, root)
	defer func() { owners.close() }()
	cmd := seedReviewedAuthoringConfirmationWithProof(t, owners, "ws-unverified-blocked", false)
	before, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if err := before.DefinitionAuthoring.Validate(); err != nil {
		t.Fatalf("真实无绑定草稿必须合法：%v", err)
	}
	if state, _, _, count := before.DefinitionAuthoring.CurrentAuthoringSummary(); state != "review_blocked" || count != 6 {
		t.Fatalf("六审不能代替声明：%s/%d", state, count)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := owners.workspace.submitDefinitionAuthoringConfirmationInternalEvent(ctx, definitionAuthoringConfirmationInternalEvent{Command: cmd}); err == nil {
			t.Fatal("UNVERIFIED 获得正式物化")
		}
		stored, err := owners.repositories.Workspaces.Get(ctx, cmd.WorkspaceID)
		if err != nil || !reflect.DeepEqual(before, stored) {
			t.Fatalf("拒绝确认改写了草稿：%+v %v", stored, err)
		}
		goals, goalErr := owners.repositories.Goals.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		dods, dodErr := owners.repositories.DoDs.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		runs, runErr := owners.repositories.Runs.List(ctx)
		if goalErr != nil || dodErr != nil || runErr != nil || len(goals) != 0 || len(dods) != 0 || len(runs) != 0 {
			t.Fatalf("拒绝确认产生正式数据：%d/%d/%d %v/%v/%v", len(goals), len(dods), len(runs), goalErr, dodErr, runErr)
		}
		view, err := owners.workspace.queryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: cmd.WorkspaceID})
		if err != nil || view.Record == nil || stored.DefinitionAuthoringConfirmation != nil || len(stored.DefinitionAdmissionCandidates) != 0 || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.ActiveRunID != "" {
			t.Fatalf("拒绝确认生成可执行身份：%+v %v", stored, err)
		}
		owners.close()
		owners = openFileDefinitionAdmissionOwners(t, root)
	}
}

// 确认门禁还必须拒绝空声明，不能仅由 DECLARED 枚举授予物化权。
func TestDeclaredEmptyVerificationCannotConfirm(t *testing.T) {
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	cmd := seedReviewedAuthoringConfirmation(t, owners, "ws-empty-declaration")
	current, err := owners.repositories.Workspaces.Get(context.Background(), cmd.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	proposal := current.DefinitionAuthoring.SpecificationRequests[0].Proposal.Clone()
	proposal.Verifications[0].Declaration = " \t\r\n"
	record := current.DefinitionAuthoring.Clone()
	for index := range record.SpecificationRequests {
		request := &record.SpecificationRequests[index]
		if request.Proposal != nil {
			request.Proposal = &proposal
		} else if request.Review != nil {
			request.ProposalHash = proposal.ContentHash()
			request.Review.SpecificationHash = proposal.ContentHash()
			request.PermitID = request.ExpectedPermitID()
		}
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("空声明草稿的结构校验应独立于确认资格：%v", err)
	}
	if _, _, err := record.ConfirmableSpecification(cmd.DerivationPermitID, proposal.ContentHash()); err == nil {
		t.Fatal("空声明获得确认资格")
	}
	for _, verification := range proposal.Verifications {
		if verification.Status != workspace.GoalVerificationDeclared {
			t.Fatal("反例不应依赖 UNVERIFIED 状态")
		}
	}
}
