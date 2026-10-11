package biz

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestDefinitionChangeActualApplicationPreparesSeparateInputWithoutRun(t *testing.T) {
	for _, scenario := range []string{"prepare", "proposal_drift", "existing_run", "deletion", "concurrent"} {
		t.Run(scenario, func(t *testing.T) {
			testDefinitionChangeActualApplication(t, scenario)
		})
	}
}

func testDefinitionChangeActualApplication(t *testing.T, scenario string) {
	t.Helper()
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
	created, err := app.CreateDefinitionAuthoringView(ctx, "file", root, management.DefinitionAuthoringCreate{RequestID: "change-create", Title: "候选隔离", TemplateID: "software-dev", DeliverableDirectory: t.TempDir(), Input: management.DefinitionAuthoringInput{GoalText: "原目标"}})
	if err != nil {
		t.Fatal(err)
	}
	id := common.WorkspaceID(created.WorkspaceID)
	before, err := app.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil || before.Record == nil {
		t.Fatal(err)
	}
	proposal := decisionmodel.ContractChangeProposal{
		SchemaVersion: 1, WorkspaceID: id, BaseInputRevision: before.Record.Input.Revision, BaseInputHash: before.Record.InputHash,
		ProposedBy: "test-user", TriggerFacts: "观测", Discovery: "发现", RootCauseOrUnknown: "未知", WhyChange: "完整原因\r\n原文", EvidenceGaps: "尚缺证据",
		Differences:  []decisionmodel.ContractChangeDifference{{ItemID: workspace.DefinitionAuthoringGoalItemRef, Kind: "goal", Action: "modify", OriginalText: "原目标", ProposedText: "拟议目标\r\n保留行尾", SemanticChange: "拟议调整尚未批准"}},
		Alternatives: "保持原合同", NoChangeConsequences: "局部阻塞", UnaffectedContinuationConditions: "原授权不变",
		Impacts:            []decisionmodel.ContractChangeImpact{{SubjectID: "goal", Kind: "requirement", ProposedHandling: "unknown", Reason: "待审查"}},
		EffectsAndReceipts: "未结效果不清零", OutstandingCosts: "原账保留", ReservedBudget: "原预算保留", AdditionalCostDurationRisk: "未知", RequiredAuthorization: "未授权", DecisionScope: "原合同持有人", ApplicationConditions: "尚未approve", RecoveryRestrictions: "不能抹去unknown",
	}
	registration, err := app.RegisterContractChangeProposal(ctx, decisionevents.RegisterContractChangeProposalCommand{TicketID: "change-ticket", RequestID: "register", Proposal: proposal})
	if err != nil {
		t.Fatal(err)
	}
	cmd := workspaceevents.PrepareDefinitionChangeCommand{WorkspaceID: id, TicketID: registration.TicketID, RequestID: "prepare", ProposalRevision: registration.Revision, ProposalHash: registration.Hash, ExpectedWorkspaceRevision: before.WorkspaceRevision}
	bad := cmd
	bad.ProposalHash = "self-declared"
	for _, source := range []string{"untrusted/module", "kernel/governance"} {
		ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionChangePrepare, source, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
		response := hub.Send(ev)
		if response == nil || response.Error() == nil {
			t.Fatalf("伪造来源 %s 被接受", source)
		}
	}
	if _, err := app.PrepareDefinitionChange(ctx, bad); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("自证来源=%v", err)
	}
	bad = cmd
	bad.ExpectedWorkspaceRevision++
	if _, err := app.PrepareDefinitionChange(ctx, bad); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("旧基线CAS=%v", err)
	}
	absent, err := app.QueryDefinitionChange(ctx, workspaceevents.QueryDefinitionChangeCommand{WorkspaceID: id})
	if err != nil || absent.Candidate != nil || absent.WorkspaceRevision != before.WorkspaceRevision {
		t.Fatalf("拒绝路径仍写候选=%v", err)
	}
	repos := testkit.RuntimeExports(runtime).Repositories
	bound := common.ContextWithRuntimeWorkspaceID(ctx, id)
	switch scenario {
	case "proposal_drift":
		revision := decisionevents.RegisterContractChangeProposalCommand{TicketID: registration.TicketID, RequestID: "revise", ExpectedRevision: registration.Revision, ExpectedHash: registration.Hash, Proposal: proposal.Clone()}
		revision.Proposal.WhyChange = "追加版本未批准"
		if _, err := app.RegisterContractChangeProposal(ctx, revision); err != nil {
			t.Fatal(err)
		}
	case "existing_run":
		// 持久历史 Run 的 fixture 不执行 operation；ActiveRunID 仍为空。
		if err := repos.Runs.Create(bound, &runmodel.Run{ID: "historical-run", WorkspaceID: id, State: common.RunStateFailed}); err != nil {
			t.Fatal(err)
		}
	case "deletion":
		stored, err := repos.Workspaces.Get(bound, id)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		stored.Deletion = &workspace.WorkspaceDeletionRecord{DeletionID: "deletion-change", DeletionGeneration: 1, RequestID: "delete-change", Mode: deletioncontract.ModeRetain, Phase: workspace.WorkspaceDeletionPhaseRequested, Revision: 1, RequestedAt: now, UpdatedAt: now, SealedDeliverableDirectory: stored.DeliverableDirectory, SealedInternalDirectory: stored.InternalDirectory}
		stored.Deletion.IntentHash = workspace.StableWorkspaceDeletionIntentHash(id, *stored.Deletion)
		if err := stored.Deletion.Validate(id); err != nil {
			t.Fatal(err)
		}
		if err := repos.Workspaces.CompareAndSwap(bound, stored, stored.Revision); err != nil {
			t.Fatal(err)
		}
	}
	if scenario == "proposal_drift" || scenario == "existing_run" || scenario == "deletion" {
		preRejected, err := repos.Workspaces.Get(bound, id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := app.PrepareDefinitionChange(ctx, cmd); err == nil {
			t.Fatal("拒绝反例仍准备候选")
		}
		postRejected, err := repos.Workspaces.Get(bound, id)
		if err != nil || !reflect.DeepEqual(preRejected, postRejected) {
			t.Fatalf("拒绝路径修改 Workspace=%v", err)
		}
		return
	}
	var first workspaceevents.PrepareDefinitionChangeResult
	if scenario == "concurrent" {
		commands := []workspaceevents.PrepareDefinitionChangeCommand{cmd, cmd}
		commands[1].RequestID = "competing-prepare"
		results := make([]workspaceevents.PrepareDefinitionChangeResult, len(commands))
		errorsReturned := make([]error, len(commands))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for n := range commands {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				<-start
				results[n], errorsReturned[n] = app.PrepareDefinitionChange(ctx, commands[n])
			}(n)
		}
		close(start)
		wg.Wait()
		accepted := 0
		for n, err := range errorsReturned {
			if err == nil {
				accepted++
				first, cmd = results[n], commands[n]
			} else if !errors.Is(err, common.ErrConflict) {
				t.Fatalf("竞争准备错误=%v", err)
			}
		}
		if accepted != 1 {
			t.Fatalf("竞争准备接受次数=%d", accepted)
		}
	} else {
		first, err = app.PrepareDefinitionChange(ctx, cmd)
	}
	if err != nil || first.ValidateFor(cmd) != nil || first.Candidate.Record.Input.GoalText != proposal.Differences[0].ProposedText {
		t.Fatalf("实际候选准备=%+v %v", first, err)
	}
	duplicate, err := app.PrepareDefinitionChange(ctx, cmd)
	if err != nil || !reflect.DeepEqual(first, duplicate) {
		t.Fatalf("原回执重放=%v", err)
	}
	view, err := app.QueryDefinitionChange(ctx, workspaceevents.QueryDefinitionChangeCommand{WorkspaceID: id})
	if err != nil || view.Candidate == nil || !reflect.DeepEqual(*view.Candidate, first.Candidate) {
		t.Fatalf("实际候选查询=%v", err)
	}
	view.Candidate.Record.Input.GoalText = "污染副本"
	original, err := app.QueryDefinitionAuthoring(ctx, workspaceevents.QueryDefinitionAuthoringCommand{WorkspaceID: id})
	if err != nil || !reflect.DeepEqual(original.Record, before.Record) || original.ActivationAdmission != nil || original.Confirmation != nil {
		t.Fatalf("候选替代原权威=%v", err)
	}
	runs, err := repos.Runs.List(bound)
	if err != nil || len(runs) != 0 {
		t.Fatalf("候选准备启动Run=%v", err)
	}
	stored, err := repos.Workspaces.Get(bound, id)
	if err != nil || stored.DefinitionChangeCandidate == nil || stored.DefinitionChangeCandidate.Validate(stored) != nil || stored.DefinitionChangeCandidate.Record.Input.GoalText != proposal.Differences[0].ProposedText {
		t.Fatalf("原候选被副本改写=%v", err)
	}
	if _, _, err := stored.DefinitionChangeCandidate.Record.ConfirmableSpecification("old-permit", "old-hash"); err == nil {
		t.Fatal("未审候选获准确认")
	}
}
