package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/content"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Shell read、receipt handler、controller 和文件型 Run owner 均为实际实现。
// 原文 wire 与 Workspace 为显式 fixture；不冒充尚未接线的公开 project producer。
func TestRuntimeShellRawOutputQueryPairsPersistedReceiptAuthority(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runPath := filepath.Join(t.TempDir(), "runs.json")
	runs := filerepository.NewRunRepository(runPath)
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	startRun := func() *runmodule.Module {
		t.Helper()
		owner := runmodule.New()
		if err := owner.Configure(runOwnerTestDependencies(filerepository.NewRunRepository(runPath), nil, nil, hub)); err != nil {
			t.Fatal(err)
		}
		if err := owner.Start(); err != nil {
			t.Fatal(err)
		}
		return owner
	}
	runOwner := startRun()
	t.Cleanup(func() {
		if err := runOwner.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	repository := runtimeStateRepository{hub: hub}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := runtimeAggregate(t, 1).Facts
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Focus.ValidationRecipeID = runtimeFileBackedValidationRecipeID
	facts.Focus.TargetRefs = []string{"target.txt"}
	facts.Focus.RequiredCheckRefs = []string{runtimeFileBackedValidationCheck}
	facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts = runtimeFactsWithFileBackedManagedValidationContract(t, facts)
	aggregate, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate = runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, aggregate)
	if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite {
		aggregate = commitRuntimeAssessmentTargetSnapshotForTest(t, ctx, controller, aggregate, "target.txt")
	}
	if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "raw-read-receipt"); err != nil {
		t.Fatal(err)
	}
	recipeID := runtimeFileBackedValidationRecipeID
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "raw-read-origin", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
		Parameters: []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}}}}}
	proposal, err := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
	if err != nil || !proposal.Admission.Accepted() {
		t.Fatalf("proposal: %+v err=%v", proposal.Admission, err)
	}
	recordRuntimeFileBackedManagedValidationFeasibility(t, ctx, controller)
	sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
	if err != nil || !sealed.Admission.Accepted() {
		t.Fatalf("seal: %+v err=%v", sealed.Admission, err)
	}
	claim, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventDispatchExecutionRequested,
		Scope:       runtime.RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"},
		Transaction: &runtime.RuntimeInternalTransactionPayload{RecoveryClaimNow: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	if err != nil || claim.OperationCall == nil {
		t.Fatalf("claim: %v", err)
	}
	request := claim.OperationCall.Request
	internal := t.TempDir()
	raw := []byte("原始工程失败\r\n\x00\xff fixture 原样\n")
	digest := sha256.Sum256(raw)
	identity := runtimecontract.RuntimeValidationOutputIdentity{Bytes: int64(len(raw)), SavedBytes: int64(len(raw)), SHA256: hex.EncodeToString(digest[:]), Complete: true}
	stageID := "project-test"
	// 独立 wire oracle 保全 bytes/base64、scope 及两类 SHA256，不调用 Shell 的私有编码 helper。
	type rawOutputScopeFixture struct {
		WorkspaceID                   common.WorkspaceID
		RunID                         common.RunID
		ExecutionKey, StageID, Stream string
	}
	scope := rawOutputScopeFixture{request.WorkspaceID, request.RunID, request.Execution.ExecutionKey, stageID, "stdout"}
	scopeBody, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	scopeHash := sha256.Sum256(scopeBody)
	record := struct {
		ProtocolRef string
		Scope       rawOutputScopeFixture
		Output      struct {
			Bytes         int64
			SHA256        string
			SavedBytes    int64
			Complete      bool
			OmittedReason string
			Raw           []byte
		}
		SavedSHA256 string
	}{ProtocolRef: "shell/project-stage-raw/v1", Scope: scope, SavedSHA256: identity.SHA256}
	record.Output.Bytes, record.Output.SavedBytes, record.Output.SHA256, record.Output.Complete, record.Output.Raw = identity.Bytes, identity.SavedBytes, identity.SHA256, true, raw
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	refHash := sha256.Sum256(body)
	ref := hex.EncodeToString(refHash[:])
	directory := filepath.Join(internal, "shell", "project-stage-raw", hex.EncodeToString(scopeHash[:]))
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ref+".json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	identity.Storage = &runtimecontract.RuntimeValidationOutputStorage{Reference: ref}
	stageIdentity := runtimecontract.RuntimeValidationExecutionStageIdentity{ID: stageID, Scope: "root-module", RequireTests: true}
	at := time.Now().UTC().Format(time.RFC3339Nano)
	conjunction := runtimecontract.RuntimeValidationExecutionConjunction{
		ProtocolRef: "shell/go-test-json/project-conjunction-behavior/v1", RequiredStagesHash: runtimecontract.StableRuntimeValidationExecutionStagesHash([]runtimecontract.RuntimeValidationExecutionStageIdentity{stageIdentity}),
		Stages: []runtimecontract.RuntimeValidationExecutionStage{{Identity: stageIdentity, Argv: []string{"go", "test", "-json", "-count=1", "./..."}, Started: true, StartedAt: at, FinishedAt: at, TerminalComplete: true, KnownFailure: true, ExitCode: 7, FailureCode: "original_known_failure", SourceBefore: identity.SHA256, SourceAfter: identity.SHA256, Stdout: identity, Stderr: identity.Clone()}},
	}
	conjunction.Stages[0].Stderr.Storage = &runtimecontract.RuntimeValidationOutputStorage{OmittedReason: "fixture_stderr_not_saved"}
	if err := conjunction.Validate(); err != nil {
		t.Fatal(err)
	}
	feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, AuthoritySnapshotID: request.Transaction.AuthoritySnapshotID}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOperationOutcomeRejected), runtimecontract.RuntimeModuleShell, runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Code: "original_known_failure", Signature: "fixture-failure", Message: "原始工程失败；完整原文仅在受限 owner 存储中"})
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = request.Operation.ID
	exit := int64(7)
	receipt := runtimecontract.RuntimeOperationReceipt{ExecutionKey: request.Execution.ExecutionKey, Fence: request.Execution.Fence, OperationID: request.Operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeRejected, ObservedEffectScope: runtimecontract.RuntimeEffectEphemeralSession, TargetRefs: request.Operation.TargetRefs, Feedback: &feedback,
		Evidence: runtimecontract.RuntimeOperationEvidence{Quality: common.EvidenceQualityInvalid, ExitCode: &exit, Validation: &runtimecontract.RuntimeValidationObservation{RecipeID: recipeID, ObservationProtocolRef: conjunction.ProtocolRef, ExecutionConjunction: &conjunction}},
	}
	owner := newTestBiz(newRuntimeTestOwner(), hub, nil)
	owner.runtimeController = controller
	receipts := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	for _, topic := range []string{orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt} {
		if err := receipts.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			owner.handleRuntimeOperationReceipt(event, result)
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := receipts.Unsubscribe(topic); err != nil {
				t.Error(err)
			}
		})
	}
	commit := orchestratorevents.CommitRuntimeOperationReceiptCommand{WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Fence: request.Execution.Fence, Receipt: receipt}
	commitResult := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, runtimecontract.RuntimeModuleShell, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, commit))
	if commitResult == nil || commitResult.Error() != nil {
		t.Fatalf("receipt commit: %+v", commitResult)
	}
	workspace := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceCalls := 0
	if err := workspace.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(event magicevent.Event, result magicevent.Result) {
		workspaceCalls++
		command := event.Data().(workspaceevents.GetWorkspaceCommand)
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: command.WorkspaceID, InternalDirectory: internal}}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := workspace.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet); err != nil {
			t.Error(err)
		}
	})
	startShell := func() *shellbiz.Biz {
		t.Helper()
		value, err := shellbiz.New(hub, nil, shellbiz.Dependencies{RuntimeAdapter: shellbiz.NewRuntimeAdapter()})
		if err != nil {
			t.Fatal(err)
		}
		if err := value.Start(); err != nil {
			t.Fatal(err)
		}
		return value
	}
	shell := startShell()
	t.Cleanup(func() {
		if err := shell.Teardown(ctx); err != nil {
			t.Error(err)
		}
	})
	command := shellevents.QueryRuntimeValidationRawOutputCommand{WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, OperationID: request.Operation.ID, ExecutionKey: request.Execution.ExecutionKey, Fence: request.Execution.Fence, SourceReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt), StageID: stageID, Stream: "stdout"}
	if err := receipts.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeValidationOutput, owner.handleRuntimeValidationOutput); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := receipts.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeValidationOutput); err != nil {
			t.Error(err)
		}
	})
	read := func(c shellevents.QueryRuntimeValidationRawOutputCommand) (shellevents.QueryRuntimeValidationRawOutputResult, error) {
		selector := orchestratorevents.QueryRuntimeValidationOutputCommand{
			WorkspaceID: c.WorkspaceID, RunID: c.RunID, DispatchID: c.DispatchID, OperationID: c.OperationID,
			ExecutionKey: c.ExecutionKey, Fence: c.Fence, SourceReceiptHash: c.SourceReceiptHash, StageID: c.StageID, Stream: c.Stream,
		}
		ref, err := selector.ContentRef()
		if err != nil {
			return shellevents.QueryRuntimeValidationRawOutputResult{}, err
		}
		value, err := content.NewReader(hub).ReadRuntimeRaw(ctx, ref)
		if err != nil {
			return shellevents.QueryRuntimeValidationRawOutputResult{}, err
		}
		if value.OutputIdentity == nil {
			t.Fatal("application read lost original output identity")
		}
		return shellevents.QueryRuntimeValidationRawOutputResult{
			SourceReceiptHash: c.SourceReceiptHash, StageID: c.StageID, Stream: c.Stream,
			Identity: value.OutputIdentity.Clone(), SavedSHA256: value.ContentSHA256, Raw: value.Content,
		}, nil
	}
	before, err := controller.Load(ctx, request.WorkspaceID, request.RunID)
	if err != nil || before.Validate() != nil {
		t.Fatalf("load: %v", err)
	}
	assertRead := func() {
		t.Helper()
		value, err := read(command)
		if err != nil || value.ValidateFor(command) != nil || !reflect.DeepEqual(value.Raw, raw) {
			t.Fatalf("raw read: %+v err=%v", value, err)
		}
		value.Raw[0]++
		value.Identity.Storage.Reference = "caller"
	}
	assertRead()
	for _, field := range []string{"dispatch", "operation", "fence", "hash", "stream"} {
		t.Run(field, func(t *testing.T) {
			changed := command
			switch field {
			case "dispatch":
				changed.DispatchID = "other"
				changed.ExecutionKey = runtimecontract.StableRuntimeOperationExecutionKey(changed.WorkspaceID, changed.RunID, changed.DispatchID, changed.OperationID)
			case "operation":
				changed.OperationID = "other"
				changed.ExecutionKey = runtimecontract.StableRuntimeOperationExecutionKey(changed.WorkspaceID, changed.RunID, changed.DispatchID, changed.OperationID)
			case "fence":
				changed.Fence++
			case "hash":
				changed.SourceReceiptHash = "other"
			case "stream":
				changed.Stream = "stderr"
			}
			calls := workspaceCalls
			if _, err := read(changed); err == nil || workspaceCalls != calls {
				t.Fatal("invalid identity reached raw directory")
			}
		})
	}
	after, err := controller.Load(ctx, request.WorkspaceID, request.RunID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("read changed Runtime/receipt/budget")
	}
	// 重建每个 owner 和 repository 句柄，不称同进程重建为 fresh-process 验收。
	if err := shell.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	runOwner = startRun()
	controller, err = runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	owner = newTestBiz(newRuntimeTestOwner(), hub, nil)
	owner.runtimeController = controller
	shell = startShell()
	assertRead()
	duplicate := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, runtimecontract.RuntimeModuleShell, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, commit))
	if duplicate == nil || duplicate.Error() != nil {
		t.Fatal("reload duplicate receipt failed")
	}
	after, err = controller.Load(ctx, request.WorkspaceID, request.RunID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("reload/duplicate changed original receipt or CAS revision")
	}
	conflicting := commit
	conflicting.Receipt = receipt.Clone()
	conflicting.Receipt.Evidence.Validation.ExecutionConjunction.Stages[0].Stdout.Storage.Reference = "different-reference"
	response := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, runtimecontract.RuntimeModuleShell, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, conflicting))
	if response == nil || response.Error() == nil {
		t.Fatal("same execution accepted conflicting raw reference")
	}
	after, err = controller.Load(ctx, request.WorkspaceID, request.RunID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("conflicting receipt changed persisted before-image")
	}
	assertRead()
	intent := deletioncontract.WorkspaceRuntimeQuiescenceIntent{WorkspaceID: request.WorkspaceID, DeletionID: "raw-read-retirement", DeletionGeneration: 1, DataDispositionIntent: deletioncontract.ModeRetain, WorkspaceIntentHash: "workspace-intent/raw-read", RunDrainingRevision: 1}
	deletion, err := reduceWorkspaceDeletionInternalEvent(nil, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventQuiescingStarted, Intent: intent, GateID: orchestratorevents.StableWorkspaceRuntimeQuiescingGateID(intent)}, "2026-10-12T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.workspaceDeletions.Create(ctx, deletion); err != nil {
		t.Fatal(err)
	}
	calls := workspaceCalls
	if _, err := read(command); err == nil || workspaceCalls != calls {
		t.Fatal("retired receipt access reached raw directory")
	}
}
