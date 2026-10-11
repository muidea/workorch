package biz

import (
	"context"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestContractChangeApplicationUsesActualOwnerInputBaselineWithoutRun(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	hub := magicevent.NewHub(128)
	runtime := testkit.NewRuntime("file", root, hub)
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	app := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	created, err := app.CreateDefinitionAuthoringView(ctx, "file", root, management.DefinitionAuthoringCreate{RequestID: "change-authoring", Title: "变更登记", TemplateID: "software-dev", DeliverableDirectory: t.TempDir(), Input: management.DefinitionAuthoringInput{GoalText: "原目标"}})
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := common.WorkspaceID(created.WorkspaceID)
	before, err := app.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: workspaceID})
	if err != nil || before.Record == nil {
		t.Fatalf("原输入=%v", err)
	}
	cmd := decisionevents.RegisterContractChangeProposalCommand{TicketID: "change-ticket", RequestID: "change-register", Proposal: decisionmodel.ContractChangeProposal{
		SchemaVersion: 1, WorkspaceID: workspaceID, BaseInputRevision: before.Record.Input.Revision, BaseInputHash: before.Record.InputHash,
		ProposedBy: "test-user", TriggerFacts: "需要澄清", Discovery: "测试观测", RootCauseOrUnknown: "未知", WhyChange: "完整调整原因\r\n原文",
		EvidenceGaps: "未提供证据", Differences: []decisionmodel.ContractChangeDifference{{ItemID: "goal", Kind: "goal", Action: "modify", OriginalText: "原目标", ProposedText: "拟议目标", SemanticChange: "拟改变承诺"}},
		Alternatives: "保留原合同", NoChangeConsequences: "保持原约束", UnaffectedContinuationConditions: "仍按原合同",
		Impacts:            []decisionmodel.ContractChangeImpact{{SubjectID: "goal", Kind: "requirement", ProposedHandling: "unknown", Reason: "尚未审查"}},
		EffectsAndReceipts: "无已知效果，未知不清零", OutstandingCosts: "原账保留", ReservedBudget: "原预算保留", AdditionalCostDurationRisk: "未知", RequiredAuthorization: "未授权", DecisionScope: "用户合同", ApplicationConditions: "尚未approve", RecoveryRestrictions: "不回滚效果",
	}}
	bad := cmd
	bad.Proposal = cmd.Proposal.Clone()
	bad.Proposal.BaseInputHash = "self-declared"
	if _, err := app.RegisterContractChangeProposal(ctx, bad); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("自证基线=%v", err)
	}
	repos := testkit.RuntimeExports(runtime).Repositories
	bound := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	if _, err := repos.Tickets.Get(bound, cmd.TicketID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("错误基线登记=%v", err)
	}
	result, err := app.RegisterContractChangeProposal(ctx, cmd)
	if err != nil || result.ValidateFor(cmd) != nil || result.Disposition != "registered" {
		t.Fatalf("实际登记=%+v %v", result, err)
	}
	result, err = app.RegisterContractChangeProposal(ctx, cmd)
	if err != nil || result.Disposition != "duplicate" {
		t.Fatalf("重复登记=%+v %v", result, err)
	}
	after, err := app.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: workspaceID})
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("登记改变原定义=%v", err)
	}
	runs, err := repos.Runs.List(bound)
	if err != nil || len(runs) != 0 {
		t.Fatalf("登记产生Run=%d %v", len(runs), err)
	}
	stored, err := repos.Tickets.Get(bound, cmd.TicketID)
	if err != nil || stored.ValidateContractChange() != nil || !reflect.DeepEqual(stored.ContractChangeRevisions[0].Proposal, cmd.Proposal) {
		t.Fatalf("原持久材料=%v", err)
	}
}
