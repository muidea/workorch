package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 真实 Workspace slot 已释放、Run 尚 active 的持久前像不能成为第二次首次激活。
// provider/capability 不参与本测试；终态前像来自现有完整 Runtime 夹具。
func TestWorkspaceExecutionRestartRetainsBarrierAfterTerminalHandoff(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-v2")
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
	results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
	goals := memory.NewGoalRepository()
	dods := memory.NewDoDRepository()
	goal := &workspacemodel.Goal{ID: "goal-v2", WorkspaceID: "ws-v2", Text: "完成目标", DefinitionRevision: "goal:00000000000000000001:hash", LineageState: "active"}
	dod := &workspacemodel.DoD{ID: "dod-v2", WorkspaceID: "ws-v2", Items: []string{"产生可验证交付物"}, State: common.DoDStateReady, DefinitionRevision: "dod:00000000000000000001:hash", LineageState: "active"}
	if err := goals.Create(ctx, goal); err != nil {
		t.Fatal(err)
	}
	if err := dods.Create(ctx, dod); err != nil {
		t.Fatal(err)
	}
	bootstrap := runtimeBootstrap()
	bootstrap.Definition.Revision = workspacecontracts.ExecutionDefinitionRevision(string(goal.ID), goal.DefinitionRevision, string(dod.ID), dod.DefinitionRevision)
	profileHash := runtimecontract.StableRuntimeProfileContractsHash(bootstrap.Activation.ExecutionContract.ProfileContracts)
	candidate := (workspacemodel.DefinitionAdmissionCandidate{
		SchemaVersion: workspacemodel.DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: "ws-v2", CandidateID: "barrier-candidate", CandidateRevision: 1,
		GoalInput:                 workspacemodel.DefinitionCandidateItem{CandidateItemRef: "goal", Text: goal.Text, SourceIdentity: "goal:" + string(goal.ID) + "@" + goal.DefinitionRevision},
		DoDInputs:                 []workspacemodel.DefinitionCandidateItem{{CandidateItemRef: "dod", Text: dod.Items[0], SourceIdentity: "dod:" + string(dod.ID) + "@" + dod.DefinitionRevision}},
		WorkspaceTemplateSnapshot: workspacemodel.DefinitionTemplateSnapshot{TemplateRef: "template-barrier", Revision: "template-r1", SnapshotHash: "template-hash"},
		RuntimeProfileContracts:   bootstrap.Activation.ExecutionContract.ProfileContracts, RuntimeProfileContractsHash: profileHash,
		AdmissionCompilerVersion: workspacemodel.DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: root,
		ProposedRelationInputs: []workspacemodel.DefinitionProposedRelationInput{{RelationRef: "terminal-all-of", RelationKind: "all_of", SourceItemRefs: []string{"dod"}, TargetItemRefs: []string{"goal"}}},
	}).WithHash()
	startup := (workspacemodel.WorkspaceStartupInputContract{
		WorkspaceID: "ws-v2", DefinitionRevision: bootstrap.Definition.Revision, GoalDefinitionRef: string(goal.ID), GoalTextHash: workspacemodel.StableDefinitionTextHash(goal.Text),
		DoDDefinitionRefs: []string{string(dod.ID)}, DoDItemSetHash: workspacemodel.StableDefinitionStringSetHash(dod.Items),
		WorkspaceTemplateRef: "template-barrier", TemplateRevision: "template-r1", TemplateSnapshotHash: "template-hash", RuntimeProfileContractsHash: profileHash,
		ResolvedProjectWorkspaceDirectory: root, CandidateItemToItemRefMappings: []workspacemodel.CandidateItemToItemRefMapping{{CandidateItemRef: "goal", ItemRef: string(goal.ID)}, {CandidateItemRef: "dod", ItemRef: string(dod.ID)}},
	}).WithHash()
	admission := workspacemodel.WorkspaceActivationAdmission{
		WorkspaceID: "ws-v2", CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash,
		DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: workspacemodel.WorkspaceActivationEligible,
	}
	bootstrap.Definition.StartupInputHash = startup.StartupInputHash
	bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
	command := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "ws-v2", RequestID: "restart-during-handoff", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &bootstrap}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	workspace := &workspacemodel.Workspace{
		ID: "ws-v2", State: common.WorkspaceStateActive, DeliverableDirectory: root, StartupInput: &startup, ActivationAdmission: &admission,
		DefinitionAdmissionCandidates: []workspacemodel.DefinitionAdmissionCandidate{candidate},
	}
	if _, eligible, err := workspace.ValidateActivationAdmissionPersistence(); err != nil || !eligible {
		t.Fatalf("屏障请求缺少独立 eligible startup 前像: %v", err)
	}

	startOwners := func(hub magicevent.Hub, workspaceRepository *filerepository.WorkspaceRepository, interrupted *failAfterWorkspaceUpdateRepository) func() {
		runOwner := runmodule.New()
		if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
			t.Fatal(err)
		}
		if err := runOwner.Start(); err != nil {
			t.Fatal(err)
		}
		workspaceOwner := workspacemodule.New()
		dependencies := workspaceOwnerTestDependencies(hub, workspaceRepository, workspaceRepository, goals, dods, memory.NewWorkspaceTemplateRepository())
		if interrupted != nil {
			dependencies = workspaceOwnerTestDependencies(hub, interrupted, workspaceRepository, goals, dods, memory.NewWorkspaceTemplateRepository())
		}
		if err := workspaceOwner.Configure(dependencies); err != nil {
			t.Fatal(err)
		}
		if err := workspaceOwner.Start(); err != nil {
			t.Fatal(err)
		}
		resultOwner := resultmodule.New()
		if err := resultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(results, memory.NewArtifactRepository(), memory.NewResultWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
			t.Fatal(err)
		}
		if err := resultOwner.Start(); err != nil {
			t.Fatal(err)
		}
		stopped := false
		stop := func() {
			if stopped {
				return
			}
			for _, err := range []error{resultOwner.Stop(ctx), workspaceOwner.Stop(ctx), runOwner.Stop(ctx)} {
				if err != nil {
					t.Fatal(err)
				}
			}
			stopped = true
			hub.Terminate(ctx)
		}
		t.Cleanup(stop)
		return stop
	}
	execute := func(hub magicevent.Hub, request workspaceevents.WorkspaceExecutionIntentCommand) workspaceevents.WorkspaceExecutionIntentResult {
		response := hub.Send(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandExecutionIntent, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, request))
		if response == nil || response.Error() != nil {
			t.Fatalf("合法执行请求在 owner admission 前失败: %+v", response)
		}
		value, err := response.Get()
		result, ok := value.(workspaceevents.WorkspaceExecutionIntentResult)
		if err != nil || !ok || result.Validate() != nil {
			t.Fatalf("执行请求没有合法 typed owner 回执: %+v %v", value, err)
		}
		return result
	}

	// 同一完整请求在无 predecessor 的真实 owner 装配上能够首次激活，排除无效 bootstrap 早拒绝假阳性。
	controlWorkspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "control-workspaces.json"))
	if err := controlWorkspaces.Create(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	controlHub := magicevent.NewHub(32)
	stopControl := startOwners(controlHub, controlWorkspaces, nil)
	control := execute(controlHub, command)
	if control.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested || control.Reason != runtimecontract.RuntimeExecutionIntentReasonNewRunActivated {
		t.Fatalf("合法首激活正例未命中: %+v", control)
	}
	stopControl()

	// 屏障场景使用另一份 Run 文件，不删除或重写正例的持久材料。
	runs = filerepository.NewRunRepository(filepath.Join(root, "barrier-runs.json"))
	aggregate := runtimeFocusGroundedTerminalAggregate(t)
	record := runtimeStateRecordFromAggregate(aggregate)
	run := runtimeRunForTest(common.RunStateActive, aggregate.Definition)
	run.RuntimeState = &record
	if err := runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	projection := runtimeWorkspaceForPersistedRunTest(t, ctx, runs, "ws-v2", "run-v2", root, "")
	workspace.ActiveRunID = projection.ActiveRunID
	workspace.RunProjectionCursor = projection.RunProjectionCursor
	if err := workspaces.Create(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	interrupted := &failAfterWorkspaceUpdateRepository{WorkspaceRepository: workspaces}
	firstHub := magicevent.NewHub(32)
	stopFirst := startOwners(firstHub, workspaces, interrupted)
	first := newRuntimeProjectionTestModule(t, firstHub)
	if err := first.deliverRuntimeTerminal(ctx, aggregate); err == nil || !interrupted.fired {
		t.Fatalf("未命中真实 Workspace handoff 响应丢失: %v", err)
	}
	stopFirst()

	for attempt := range 2 {
		hub := magicevent.NewHub(32)
		stop := startOwners(hub, workspaces, nil)
		storedWorkspace, err := workspaces.Get(ctx, "ws-v2")
		if err != nil || storedWorkspace.ActiveRunID != "" || storedWorkspace.TerminalDelivery == nil {
			t.Fatalf("缺少真实空 slot handoff 前像: %+v %v", storedWorkspace, err)
		}
		before, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
		if err != nil || before.State != common.RunStateActive || before.RuntimeState == nil {
			t.Fatalf("Run 尚未终态的窗口丢失: %+v %v", before, err)
		}
		sealed := runtimeAggregateFromRecord(*before.RuntimeState)
		if err := sealed.Validate(); err != nil || sealed.TerminalMaterialSeal == nil || len(sealed.DeliveryAcknowledgements) != 1 {
			t.Fatalf("缺少 exact seal 与未完成的 ACK 前像: %v", err)
		}
		resume := execute(hub, workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "ws-v2", RequestID: "resume-during-handoff", Kind: runtimecontract.RuntimeExecutionIntentResume})
		if resume.Outcome != runtimecontract.RuntimeExecutionIntentRestartRequired || resume.Reason != runtimecontract.RuntimeExecutionIntentReasonWorkspaceActiveRunAbsent {
			t.Fatalf("空 slot Resume 冒充首次激活: %+v", resume)
		}
		restart := execute(hub, command)
		if restart.Outcome != runtimecontract.RuntimeExecutionIntentReplacementPending || restart.RunID != "run-v2" {
			t.Fatalf("未终态 handoff 放行第二 Run: %+v", restart)
		}
		inventory, err := runs.ListByWorkspaceID(ctx, "ws-v2")
		if err != nil || len(inventory) != 1 || inventory[0].ID != "run-v2" || inventory[0].State != common.RunStateActive || inventory[0].ReplacementIntent == nil || inventory[0].ReplacementIntent.SuccessorRunID != "" || !reflect.DeepEqual(inventory[0].RuntimeState, before.RuntimeState) {
			t.Fatalf("handoff 屏障期间创建后继或改写 Runtime 权威: %+v %v", inventory, err)
		}
		competing := command
		competing.RequestID = "another-restart-during-handoff"
		response := hub.Send(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandExecutionIntent, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, competing))
		if response == nil || response.Error() == nil || !errors.Is(response.Error(), common.ErrConflict) {
			t.Fatalf("另一合法执行身份绕过未完成交接: %+v", response)
		}
		unchanged, err := runs.ListByWorkspaceID(ctx, "ws-v2")
		if err != nil || !reflect.DeepEqual(inventory, unchanged) {
			t.Fatal("被拒绝的竞争请求改写 Run 或持久 replacement 身份")
		}
		if attempt == 1 {
			recovered := newRuntimeProjectionTestModule(t, hub)
			for range 2 {
				if err := recovered.deliverRuntimeTerminal(ctx, sealed); err != nil {
					t.Fatal(err)
				}
			}
			after, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
			if err != nil || after.State != common.RunStateFailed || after.TerminalResultID == "" {
				t.Fatalf("屏障后的原终态无法恢复: %+v %v", after, err)
			}
			final := runtimeAggregateFromRecord(*after.RuntimeState)
			if final.Validate() != nil || !runtimeTerminalDeliveryFullyAcknowledged(final) || !reflect.DeepEqual(final.TerminalMaterialSeal, sealed.TerminalMaterialSeal) {
				t.Fatal("恢复更换原 seal 或未完成有序交接")
			}
			items, err := results.ListByRun(ctx, "run-v2")
			if err != nil || len(items) != 1 || items[0].ID != after.TerminalResultID || !reflect.DeepEqual(items[0].RuntimeCloseout, sealed.TerminalMaterialSeal.Material.RuntimeCloseout) {
				t.Fatal("恢复复制 Result 或改变 owner 材料")
			}
			inventory, err = runs.ListByWorkspaceID(ctx, "ws-v2")
			if err != nil || len(inventory) != 1 || inventory[0].Lineage.SuccessorRunID != "" {
				t.Fatal("终态交付冒充 replacement 完成并创建后继")
			}
		}
		stop()
	}
}
