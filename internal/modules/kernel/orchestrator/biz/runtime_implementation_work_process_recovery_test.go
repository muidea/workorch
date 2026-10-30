package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const implementationWorkProcessCutExit = 73

// 首个子进程在真实 File receipt CAS 后、driver 收到 response 前退出；恢复进程不共享 Hub/owner/repository。
// 启动仍是 private fixture，本回归不开放公开 admission，也不代替全工程终验和正式交付。
func TestImplementationWorkRealFileReceiptFreshProcessRecovery(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "workspace")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	runImplementationWorkProcess(t, root, "cut", implementationWorkProcessCutExit)
	// 父进程首次前像按原 bytes 放到独立只读路径；避免 file repository 的同路径进程缓存遮蔽子进程后续写入。
	persisted, err := os.ReadFile(filepath.Join(root, "runs.json"))
	if err != nil {
		t.Fatal(err)
	}
	cutSnapshot := filepath.Join(root, "cut-runs.json")
	if err := os.WriteFile(cutSnapshot, persisted, 0o600); err != nil {
		t.Fatal(err)
	}
	repository := filerepository.NewRunRepository(cutSnapshot)
	first, err := repository.GetByWorkspaceID(context.Background(), "ws-v2", "run-v2")
	if err != nil || first.RuntimeState == nil {
		t.Fatalf("独立读取原 Run receipt 前像: %v", err)
	}
	before := first.RuntimeState.Clone()
	if err := before.Validate(); err != nil || before.ActiveDispatch != nil || len(before.CompletedDispatches) != 1 || len(before.OperationReceipts) != 1 || len(before.ImplementationWork.Rounds) != 1 {
		t.Fatalf("响应丢失前像未保存真实 work receipt: active=%t receipts=%d rounds=%d / %v", before.ActiveDispatch != nil, len(before.OperationReceipts), len(before.ImplementationWork.Rounds), err)
	}
	receipt := before.OperationReceipts[0]
	if len(before.CompletedDispatches[0].OperationExecutions) != 1 || before.CompletedDispatches[0].OperationExecutions[0].State != runtimecontract.OperationExecutionEvidenceCommitted || before.ImplementationWork.Rounds[0].CompletedReceiptSetHash == "" || before.Facts.Recovery.OperationRecoveryAttempts != 0 {
		t.Fatal("原 receipt 未与 work round/预算消费原子保存")
	}
	assertImplementationWorkProcessBudget(t, before, 1)
	assertImplementationWorkProcessNoTerminalCredit(t, before)
	content, err := os.ReadFile(filepath.Join(directory, "resource-a"))
	if err != nil || string(content) != "实际工作内容-1" {
		t.Fatalf("首进程未完成真实 File effect: %q / %v", content, err)
	}
	runImplementationWorkProcess(t, root, "resume", 0)
	second, err := filerepository.NewRunRepository(filepath.Join(root, "runs.json")).GetByWorkspaceID(context.Background(), "ws-v2", "run-v2")
	if err != nil || second.RuntimeState == nil {
		t.Fatalf("恢复后独立 Run readback: %v", err)
	}
	after := second.RuntimeState
	if err := after.Validate(); err != nil || len(after.OperationReceipts) != 2 || len(after.ImplementationWork.Rounds) != 2 || after.ImplementationWork.Rounds[1].CompletedReceiptSetHash == "" {
		t.Fatalf("新进程未完成同原 CWI 第二轮: receipts=%d rounds=%d / %v", len(after.OperationReceipts), len(after.ImplementationWork.Rounds), err)
	}
	original, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(after.OperationReceipts, receipt.ExecutionKey, receipt.Fence)
	if !found || !runtimecontract.SameRuntimeOperationReceipt(original, receipt) || !reflect.DeepEqual(before.Facts.Recovery, after.Facts.Recovery) || !reflect.DeepEqual(before.OperationRecoveryAuthorizations, after.OperationRecoveryAuthorizations) {
		t.Fatal("新进程改写原 receipt/fence 或消费额外 recovery")
	}
	for _, consumption := range before.ExecutionBudget.Consumptions {
		matches := 0
		for _, current := range after.ExecutionBudget.Consumptions {
			if current.ConsumptionID == consumption.ConsumptionID && reflect.DeepEqual(current, consumption) {
				matches++
			}
		}
		if matches != 1 {
			t.Fatal("恢复丢失或重复原 confirmed/unknown consumption")
		}
	}
	for _, allocation := range before.ExecutionBudget.Allocations {
		matches := 0
		for _, current := range after.ExecutionBudget.Allocations {
			if current.AllocationID == allocation.AllocationID && reflect.DeepEqual(current, allocation) {
				matches++
			}
		}
		if matches != 1 {
			t.Fatal("恢复丢失或重置原 allocation/reservation")
		}
	}
	if !reflect.DeepEqual(before.ImplementationWork.Rounds[0], after.ImplementationWork.Rounds[0]) || !reflect.DeepEqual(before.CompletedDispatches[0], after.CompletedDispatches[0]) {
		t.Fatal("第二轮改写原 working commitment/dispatch/receipt-set")
	}
	if !reflect.DeepEqual(first.RuntimeBootstrap, second.RuntimeBootstrap) || !reflect.DeepEqual(before.Definition, after.Definition) || !reflect.DeepEqual(before.Facts.DefinitionFocuses, after.Facts.DefinitionFocuses) {
		t.Fatal("新进程恢复改写原 startup/Definition")
	}
	left, right := after.ImplementationWork.Rounds[0], after.ImplementationWork.Rounds[1]
	if left.CoverageWorkItemID != right.CoverageWorkItemID || right.PrecedingReceiptSetHash != left.CompletedReceiptSetHash || left.ScopeObservation.Selection.ObservedRevision == right.ScopeObservation.Selection.ObservedRevision {
		t.Fatal("第二轮未反查原 receipt lineage 或 fresh scope")
	}
	assertImplementationWorkProcessBudget(t, *after, 2)
	assertImplementationWorkProcessNoTerminalCredit(t, *after)
	content, err = os.ReadFile(filepath.Join(directory, "resource-a"))
	if err != nil || string(content) != "实际工作内容-2" {
		t.Fatalf("新进程第二轮真实内容: %q / %v", content, err)
	}
}

func runImplementationWorkProcess(t *testing.T, root, mode string, expectedExit int) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-test.run=^TestImplementationWorkReceiptProcessChild$", "-test.timeout=90s", "-test.v")
	command.Env = append(os.Environ(), "WORKORCH_TEST_WORK_RECEIPT_ROOT="+root, "WORKORCH_TEST_WORK_RECEIPT_MODE="+mode)
	output, err := command.CombinedOutput()
	if writeErr := os.WriteFile(filepath.Join(root, mode+".log"), output, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("子进程未正常返回: %v\n%s", err, output)
		}
		exit = exitErr.ExitCode()
	}
	if ctx.Err() != nil || exit != expectedExit {
		t.Fatalf("子进程 %s exit=%d want=%d: %v\n%s", mode, exit, expectedExit, ctx.Err(), output)
	}
}

func TestImplementationWorkReceiptProcessChild(t *testing.T) {
	root, mode := os.Getenv("WORKORCH_TEST_WORK_RECEIPT_ROOT"), os.Getenv("WORKORCH_TEST_WORK_RECEIPT_MODE")
	if root == "" && mode == "" {
		return // 仅由父回归显式启用，普通 suite 不生成额外 fixture。
	}
	if !filepath.IsAbs(root) || mode != "cut" && mode != "resume" && mode != "chain-cut" && mode != "chain-resume" {
		t.Fatal("非法 process fixture 输入")
	}
	chain := strings.HasPrefix(mode, "chain-")
	cut, resume := mode == "cut" || mode == "chain-cut", mode == "resume" || mode == "chain-resume"
	targets := []string{"resource-a"}
	if chain {
		targets = []string{"resource-a", "resource-b", "resource-c", "resource-d"}
	}
	ctx := context.Background()
	directory := filepath.Join(root, "workspace")
	repository := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if cut {
		fixture, _, initial := implementationWorkPurposeDebtDriverFixtureWithDirectory(t, false, directory)
		configureImplementationWorkDriverTargets(t, &initial, fixture.EventHub(), false, targets)
		bootstrap := runtimeBootstrap()
		bootstrap.Definition, bootstrap.Activation, bootstrap.DefinitionSeed, bootstrap.DefinitionFocuses = initial.Definition, initial.Facts.Activation, initial.Facts.DefinitionSeed, initial.Facts.DefinitionFocuses
		item := runtimeRunForTestBootstrap(common.RunStateActive, bootstrap)
		record := runtimeStateRecordFromAggregate(initial)
		item.RuntimeState = &record
		if err := repository.Create(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(repository, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runOwner.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	fileOwner := filemodule.New()
	if err := fileOwner.Configure(filebiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fileOwner.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	startRuntimeManagedValidationTestOwner(t, hub)
	workspace := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	if err := workspace.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: directory, InternalDirectory: filepath.Join(directory, ".internal")}}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := workspace.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet); err != nil {
			t.Error(err)
		}
	})
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)
	receipts := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	if err := receipts.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, module.handleRuntimeOperationReceipt); err != nil {
		t.Fatal(err)
	}
	if err := receipts.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		committed := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
		module.handleRuntimeOperationReceipt(event, committed)
		data, err := committed.Get()
		if cut && err == nil {
			response, ok := data.(orchestratorevents.CommitRuntimeOperationReceiptResult)
			if !ok || response.StateRevision == 0 {
				t.Error("未取得真实 receipt CAS 回执")
			} else {
				os.Exit(implementationWorkProcessCutExit)
			}
		}
		result.Set(data, err)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := receipts.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt); err != nil {
			t.Error(err)
		}
		if err := receipts.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt); err != nil {
			t.Error(err)
		}
	})
	aggregate, err := module.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	startOperations := 0
	if resume {
		startOperations = 1
		assertImplementationWorkProcessDuplicate(t, module, hub, aggregate, directory)
	}
	operations := installImplementationWorkProcessProviderForTargets(t, hub, startOperations, targets)
	for transition := 0; transition < runtimeDriveTransitionLimit; transition++ {
		outcome, err := module.driveOneRuntimeTransition(ctx, nil, aggregate, orchestratorevents.DriveRuntimeCommand{WorkspaceID: "ws-v2", RunID: "run-v2"}, runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract))
		if err != nil {
			t.Fatalf("恢复 transition=%d phase=%s: %v", transition, aggregate.SemanticPlanning.Phase, err)
		}
		aggregate = outcome.aggregate
		if len(aggregate.ImplementationWork.Rounds) == 2 && aggregate.ImplementationWork.Rounds[1].CompletedReceiptSetHash != "" {
			if !resume || *operations != 2 {
				t.Fatal("新进程重新请求已完成工作候选")
			}
			return
		}
		if outcome.result != nil {
			t.Fatalf("同原 CWI 两轮前提前终结: %+v", outcome.result)
		}
	}
	t.Fatal("工作 process 回归未在原 transition 上界内完成")
}

func installImplementationWorkProcessProvider(t *testing.T, hub magicevent.Hub, initialOperations int, knownUsage ...bool) *int {
	return installImplementationWorkProcessProviderForTargets(t, hub, initialOperations, []string{"resource-a"}, knownUsage...)
}

func installImplementationWorkProcessProviderForTargets(t *testing.T, hub magicevent.Hub, initialOperations int, targets []string, knownUsage ...bool) *int {
	t.Helper()
	operations := initialOperations
	audited := func(command llmevents.RequestRuntimeCandidateCommand, result llmevents.RequestRuntimeCandidateResult) llmevents.RequestRuntimeCandidateResult {
		if len(knownUsage) > 0 && knownUsage[0] {
			result.Usage = &llmaudit.ProviderUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}
		}
		return runtimeProviderResultWithAuditForTest(command, result)
	}
	provider := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	if err := provider.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			manifest := command.SemanticManifest
			document := runtimecontract.RuntimeSemanticPlanDocument{SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: command.SemanticContext.PlanStep}
			if document.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
				document.CoverageOrder = []common.CoverageWorkItemID{manifest.CoverageItems[0].ID}
				if work := manifest.ProofObligations[0].ImplementationWork; work != nil && work.PurposeChoiceOnly {
					document.ImplementationPurpose = runtimecontract.RuntimeImplementationWorkPurposeEffect
				}
			} else {
				document.BatchID, document.BatchSequence = command.SemanticContext.BatchID, command.SemanticContext.BatchSequence
				document.BatchCoverageWorkItemIDs = command.SemanticContext.BatchCoverageWorkItemIDs
				document.Proposals = []runtimecontract.RuntimeSemanticProofProposal{{CoverageWorkItemID: manifest.CoverageItems[0].ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileEdit,
					Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: targets},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, DeclarationRef: runtimecontract.RuntimeSemanticCurrentDeclarationRef(manifest.ProofObligations[0])}}
			}
			payload, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
			if err != nil {
				t.Error(err)
				result.Set(nil, nil)
				return
			}
			result.Set(audited(command, llmevents.RequestRuntimeCandidateResult{DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload)}), nil)
			return
		}
		operations++
		oldText, newText := "original implementation", fmt.Sprintf("实际工作内容-%d", operations)
		if operations > 1 {
			oldText = fmt.Sprintf("实际工作内容-%d", operations-1)
		}
		manifest := command.CandidateManifest
		edits := []runtimecontract.StateChangingOperation{}
		for index, target := range targets {
			baseHash := ""
			for _, operation := range manifest.Operations {
				for _, binding := range operation.TargetParameterBindings {
					for _, value := range binding.Values {
						if binding.Name == "baseHash" && value.TargetRef == target {
							baseHash = value.Value
						}
					}
				}
			}
			requirement := ""
			for _, required := range manifest.RequiredOperationSequence {
				if required.TargetRef == target {
					requirement = required.RequirementID
				}
			}
			edits = append(edits, runtimecontract.StateChangingOperation{Operation: runtimecontract.RuntimeOperation{ID: fmt.Sprintf("work-operation-%d-%d", operations, index), Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{target}, RequiredOperationRef: requirement, Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}, {Name: "baseHash", String: &baseHash}}}})
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: edits}}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := provider.Unsubscribe(llmevents.TopicRequestRuntimeCandidate); err != nil {
			t.Error(err)
		}
	})
	return &operations
}

func assertImplementationWorkProcessDuplicate(t *testing.T, module *Biz, hub magicevent.Hub, aggregate runtime.RuntimeAggregate, directory string) {
	t.Helper()
	if (len(aggregate.CompletedDispatches) != 1 && aggregate.ActiveDispatch == nil) || len(aggregate.OperationReceipts) != 1 {
		t.Fatal("fresh owner 未读回原 completed dispatch/receipt")
	}
	var dispatch runtime.SealedDispatch
	if aggregate.ActiveDispatch != nil {
		dispatch = *aggregate.ActiveDispatch
	} else {
		dispatch = aggregate.CompletedDispatches[0]
	}
	if dispatch.Candidate.StateChange == nil || len(dispatch.Candidate.StateChange.Operations) == 0 || len(dispatch.OperationExecutions) == 0 {
		t.Fatal("原 sealed operation 前像缺失")
	}
	request := runtime.RuntimeOperationExecutionRequest{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, DispatchID: dispatch.ID, Transaction: dispatch.Transaction,
		Operation: dispatch.Candidate.StateChange.Operations[0].Operation, Execution: dispatch.OperationExecutions[0],
		EvidenceKind: runtimecontract.EvidenceKindStateChange, TargetRole: dispatch.FocusTargetRole,
	}
	result, err := (eventRuntimeOperationExecutor{hub: hub}).ExecuteRuntimeOperation(context.Background(), request)
	if err != nil || !runtimecontract.SameRuntimeOperationReceipt(result.Receipt, aggregate.OperationReceipts[0]) {
		t.Fatalf("fresh File owner 未返回 exact 原 receipt: %v", err)
	}
	fresh, err := module.loadRuntime(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil || !reflect.DeepEqual(fresh, aggregate) {
		t.Fatalf("duplicate 修改原 aggregate/预算/证据: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(directory, "resource-a"))
	if err != nil || string(content) != "实际工作内容-1" {
		t.Fatalf("duplicate 重复真实 effect: %q / %v", content, err)
	}
}

func assertImplementationWorkProcessBudget(t *testing.T, record runtimecontract.RuntimeStateRecord, count int) {
	t.Helper()
	allocations, consumptions := 0, 0
	for _, allocation := range record.ExecutionBudget.Allocations {
		if allocation.Dimension != runtimecontract.RuntimeBudgetDimensionCapabilityOperation {
			continue
		}
		allocations++
		if allocation.State != runtimecontract.RuntimeBudgetAllocationConfirmed || allocation.ReservedAmount != 1 {
			t.Fatal("真实工作 allocation 未按单次物理执行 confirmed")
		}
		for _, consumption := range record.ExecutionBudget.Consumptions {
			if consumption.AllocationID == allocation.AllocationID {
				consumptions++
				if consumption.Owner != runtimecontract.RuntimeModuleFile || consumption.ConfirmedAmount != 1 || consumption.UnknownAmount != 0 {
					t.Fatal("真实工作 consumption 未保持 exact File 单次消费")
				}
			}
		}
	}
	if allocations != count || consumptions != count {
		t.Fatalf("工作物理预算计数 allocation=%d consumption=%d want=%d", allocations, consumptions, count)
	}
}

func assertImplementationWorkProcessNoTerminalCredit(t *testing.T, record runtimecontract.RuntimeStateRecord) {
	t.Helper()
	for _, coverage := range record.Facts.Coverage {
		if coverage.State == runtimecontract.RuntimeCoverageItemStateSatisfied || coverage.CurrentBindingRef != nil || len(coverage.EvidenceRefs) != 0 {
			t.Fatal("工作 receipt 授予 terminal binding/credit")
		}
	}
	for _, fact := range record.Facts.Evidence.Accepted {
		if len(fact.BindingRefs) != 0 || len(fact.CoverageWorkItemIDs) != 0 || fact.Validation != nil || fact.Quality != "" {
			t.Fatal("工作 receipt 被当作终验证明")
		}
	}
}
