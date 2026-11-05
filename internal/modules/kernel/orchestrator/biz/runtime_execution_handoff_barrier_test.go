package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	workspacecontracts "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
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
	inventory, err := definitionadmission.CompileRuntimeSourceObligationInventory(candidate, startup, admission)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap.Definition.SourceObligations = inventory
	bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
	command := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: "ws-v2", RequestID: "restart-during-handoff", Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: &bootstrap}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	workspace := &workspacemodel.Workspace{
		ID: "ws-v2", TemplateID: "template-barrier", State: common.WorkspaceStateActive, DeliverableDirectory: root, StartupInput: &startup, ActivationAdmission: &admission,
		DefinitionAdmissionCandidates: []workspacemodel.DefinitionAdmissionCandidate{candidate},
		// This is a current creation fixture, not a legacy Workspace. Supply
		// its creation identity before Create; ordinary updates cannot add one.
		PreparationFence: &workspacemodel.WorkspacePreparationFence{Version: workspacemodel.WorkspacePreparationFenceVersion, WorkspaceID: "ws-v2", CreationEpoch: strings.Repeat("a", 32)},
	}
	if _, eligible, err := workspace.ValidateActivationAdmissionPersistence(); err != nil || !eligible {
		t.Fatalf("屏障请求缺少独立 eligible startup 前像: %v", err)
	}
	template := &workspacemodel.WorkspaceTemplate{ID: workspace.TemplateID, Name: "handoff-barrier"}
	if err := definitionconfiguration.ValidateExecutionBootstrap(workspace, template, bootstrap); err != nil {
		t.Fatalf("屏障请求缺少当前 owner 合同: %v", err)
	}
	templates := memory.NewWorkspaceTemplateRepository()
	if err := templates.Create(ctx, template); err != nil {
		t.Fatal(err)
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
		dependencies := workspaceOwnerTestDependencies(hub, workspaceRepository, workspaceRepository, goals, dods, templates)
		if interrupted != nil {
			dependencies = workspaceOwnerTestDependencies(hub, interrupted, workspaceRepository, goals, dods, templates)
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
		if response == nil {
			t.Fatal("合法执行请求没有 owner 回执")
		}
		if response.Error() != nil {
			t.Fatalf("合法执行请求在 owner admission 前失败: %v", response.Error())
		}
		value, err := response.Get()
		result, ok := value.(workspaceevents.WorkspaceExecutionIntentResult)
		if err != nil || !ok || result.Validate() != nil {
			t.Fatalf("执行请求没有合法 typed owner 回执: %+v %v", value, err)
		}
		return result
	}

	// Seed the historical accepted activation in both owner stores. This is
	// an already-running Workspace fixture, not a first execution without the
	// current preparation evidence. All source hashes come from the original
	// Run bootstrap; normal reserve/settle CAS boundaries retain that history.
	seedActive := func(repo *filerepository.WorkspaceRepository) {
		t.Helper()
		aggregate := runtimeFocusGroundedTerminalAggregate(t)
		record := runtimeStateRecordFromAggregate(aggregate)
		run := runtimeRunForTest(common.RunStateActive, aggregate.Definition)
		run.RuntimeState = &record
		run = runmodel.CloneRun(run)
		run.ActivationRequestID = "original-barrier-activation"
		run.StartupInputHash = run.RuntimeBootstrap.Definition.StartupInputHash
		prior := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: workspace.ID, RequestID: run.ActivationRequestID, Kind: runtimecontract.RuntimeExecutionIntentRestart, Bootstrap: run.RuntimeBootstrap}
		runHash, err := runmodel.ExecutionIntentPayloadHash(prior.WorkspaceID, prior.RequestID, *prior.Bootstrap)
		if err != nil {
			t.Fatal(err)
		}
		receipt := runmodel.WorkspaceExecutionIntentReceipt{Version: runmodel.WorkspaceExecutionIntentReceiptVersion, PayloadHash: runHash, RecordedAt: time.Now().UTC(), Admission: runtimecontract.RuntimeExecutionIntentAdmission{WorkspaceID: prior.WorkspaceID, RunID: run.ID, RequestID: prior.RequestID, Kind: prior.Kind, Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated}}
		run.ExecutionIntentReceipts = []runmodel.WorkspaceExecutionIntentReceipt{receipt}
		if err := runs.Create(ctx, run); err != nil {
			t.Fatal(err)
		}
		projection := runtimeWorkspaceForPersistedRunTest(t, ctx, runs, workspace.ID, run.ID, root, "")
		w := *workspace
		w.Revision = 0
		w.ActiveRunID, w.RunProjectionCursor = projection.ActiveRunID, projection.RunProjectionCursor
		w.PreparationFence = workspace.PreparationFence.Clone()
		if err := repo.Create(ctx, &w); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(struct {
			Domain string
			Intent workspaceevents.WorkspaceExecutionIntentCommand
		}{"workorch-fresh-intent-payload/v1", prior.Normalized()})
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(encoded)
		reservation := workspacemodel.WorkspaceFreshIntentReservation{RequestID: prior.RequestID, Kind: prior.Kind, StartupInputHash: prior.Bootstrap.Definition.StartupInputHash, PayloadHash: hex.EncodeToString(digest[:]), RunPayloadHash: runHash}
		w.PreparationFence.PreparationEpoch = 1
		w.PreparationFence.Pending = &reservation
		if err := repo.CompareAndSwap(ctx, &w, w.Revision); err != nil {
			t.Fatal(err)
		}
		w.PreparationFence.Pending = nil
		w.PreparationFence.Settled = []workspacemodel.WorkspaceFreshIntentSettlement{{Reservation: reservation, Receipt: receipt}}
		if err := repo.CompareAndSwap(ctx, &w, w.Revision); err != nil {
			t.Fatal(err)
		}
	}
	// With the same historical activation and all three ACKs complete, the
	// current full request must reach real Run-owner successor admission.
	controlWorkspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "control-workspaces.json"))
	seedActive(controlWorkspaces)
	controlHub := magicevent.NewHub(32)
	stopControl := startOwners(controlHub, controlWorkspaces, nil)
	controlDelivery := newRuntimeProjectionTestModule(t, controlHub)
	if err := controlDelivery.deliverRuntimeTerminal(ctx, runtimeFocusGroundedTerminalAggregate(t)); err != nil {
		t.Fatal(err)
	}
	control := execute(controlHub, command)
	if control.Outcome != runtimecontract.RuntimeExecutionIntentReplacementPending || control.Reason != runtimecontract.RuntimeExecutionIntentReasonTerminalPredecessorSuccessorRequested {
		t.Fatalf("完整 ACK 后的合法后继准入正例未命中: %+v", control)
	}
	stopControl()

	// 屏障场景使用另一份 Run 文件，不删除或重写正例的持久材料。
	runs = filerepository.NewRunRepository(filepath.Join(root, "barrier-runs.json"))
	aggregate := runtimeFocusGroundedTerminalAggregate(t)
	seedActive(workspaces)
	interrupted := &failAfterWorkspaceUpdateRepository{WorkspaceRepository: workspaces}
	firstHub := magicevent.NewHub(32)
	stopFirst := startOwners(firstHub, workspaces, interrupted)
	first := newRuntimeProjectionTestModule(t, firstHub)
	if err := first.deliverRuntimeTerminal(ctx, aggregate); err == nil || !interrupted.fired {
		t.Fatalf("未命中真实 Workspace handoff 响应丢失: %v", err)
	}
	stopFirst()

	var competingReservation *workspacemodel.WorkspaceFreshIntentReservation
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
		if attempt == 0 {
			response := hub.Send(magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandExecutionIntent, "application/workorchd", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, competing))
			if response == nil || response.Error() == nil || !errors.Is(response.Error(), common.ErrConflict) {
				t.Fatalf("另一合法执行身份绕过未完成交接: %+v", response)
			}
		} else {
			// The first rejected forward left a durable reservation without a
			// Run-owner receipt. After restart, report that exact unknown; do
			// not forward again or infer a durable rejection from the old error.
			replay := execute(hub, competing)
			if competingReservation == nil || replay.Outcome != runtimecontract.RuntimeExecutionIntentFreshUnresolved || replay.Reason != runtimecontract.RuntimeExecutionIntentReasonFreshResultUnknown || replay.RunID != "" || replay.PendingRequestID != competingReservation.RequestID || replay.PendingStartupInputHash != competingReservation.StartupInputHash || replay.PendingPayloadHash != competingReservation.PayloadHash {
				t.Fatalf("竞争身份重放未保留原 unknown: %+v", replay)
			}
		}
		reservedWorkspace, err := workspaces.Get(ctx, command.WorkspaceID)
		if err != nil || reservedWorkspace.PreparationFence == nil || reservedWorkspace.PreparationFence.PreparationEpoch != 3 || len(reservedWorkspace.PreparationFence.Settled) != 2 || reservedWorkspace.PreparationFence.Pending == nil || reservedWorkspace.PreparationFence.Pending.RequestID != competing.RequestID || reservedWorkspace.PreparationFence.Pending.Validate() != nil {
			t.Fatalf("竞争请求原登记丢失或凭错误结算: %+v %v", reservedWorkspace, err)
		}
		if attempt == 0 {
			reservation := *reservedWorkspace.PreparationFence.Pending
			competingReservation = &reservation
		} else if !reflect.DeepEqual(competingReservation, reservedWorkspace.PreparationFence.Pending) {
			t.Fatal("重启后竞争请求更换了原登记身份")
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
