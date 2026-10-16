package git

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGitCommitDependenciesUseActualImmutableObjectBytes(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "delivered.txt"), "原交付\x00正文\n")
	runGitForTest(t, repo, "add", "delivered.txt")
	adapter := NewRuntimeAdapter()
	output, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{Kind: gitcommon.StepKindWrite, WorkspaceDeliverableDirectory: repo, Operation: gitcommon.OperationCommit, Message: "原提交\n\n完整诊断无改写"})
	if err != nil {
		t.Fatal(err)
	}
	scope := runtimecontract.RuntimeOperationExecutionContext{WorkspaceID: "ws-objects", Transaction: runtimecontract.AuthorityTransaction{DefinitionRevision: "definition-objects"}}
	dependencies, err := adapter.commitOwnerDependencies(ctx, scope, output)
	if err != nil || dependencies == nil || len(dependencies.Entries) != 4 {
		t.Fatalf("缺完整 commit/tree/blob 实际读取: %+v %v", dependencies, err)
	}
	if err := dependencies.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, entry := range dependencies.Entries {
		kind := gitOutputForTest(t, repo, "cat-file", "-t", entry.ResourceID)
		body, err := adapter.gitOutput(ctx, repo, adapter.timeout, "--no-replace-objects", "cat-file", kind, entry.ResourceID)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte(body))
		if entry.Revision != hex.EncodeToString(digest[:]) || entry.SnapshotID != entry.ResourceID || entry.OwnerInstanceID != ModuleID+"/ws-objects" || entry.ResourceKind != "git_object" {
			t.Fatalf("依赖没有绑定完整原 bytes: %+v", entry)
		}
	}
	before, err := json.Marshal(dependencies)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "delivered.txt"), "更晚的未提交工作\n")
	current, err := adapter.commitOwnerDependencies(ctx, scope, output)
	if err != nil || !reflect.DeepEqual(dependencies, current) {
		t.Fatal("无关工作树变化改变 immutable 对象依赖")
	}
	copy := runtimecontract.CloneRuntimeOwnerRevisionSet(current)
	copy.Entries[0].Revision = "caller-change"
	after, _ := json.Marshal(dependencies)
	if string(before) != string(after) {
		t.Fatal("返回依赖共享原值")
	}
	var reloaded runtimecontract.RuntimeOwnerRevisionSet
	if err := json.Unmarshal(before, &reloaded); err != nil || !reflect.DeepEqual(*dependencies, reloaded) {
		t.Fatal("依赖原值重载漂移")
	}
}

func TestGitCommitDependenciesReadSHA256ObjectRepository(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	command := exec.Command("git", "init", "-q", "--object-format=sha256")
	command.Dir = repo
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("SHA256 repository fixture: %v %s", err, output)
	}
	runGitForTest(t, repo, "config", "user.email", "workorch@example.test")
	runGitForTest(t, repo, "config", "user.name", "WorkOrch Test")
	writeFile(t, filepath.Join(repo, "delivered.txt"), "完整 SHA256 工程\n")
	runGitForTest(t, repo, "add", "delivered.txt")
	adapter := NewRuntimeAdapter()
	output, err := adapter.ExecuteOperation(ctx, RuntimeOperationRequest{Kind: gitcommon.StepKindWrite, WorkspaceDeliverableDirectory: repo, Operation: gitcommon.OperationCommit, Message: "SHA256 initial commit"})
	if err != nil {
		t.Fatal(err)
	}
	scope := runtimecontract.RuntimeOperationExecutionContext{WorkspaceID: "ws-sha256", Transaction: runtimecontract.AuthorityTransaction{DefinitionRevision: "definition-sha256"}}
	dependencies, err := adapter.commitOwnerDependencies(ctx, scope, output)
	if err != nil || dependencies == nil || len(dependencies.Entries) != 3 {
		t.Fatalf("SHA256 实际对象未读取: %+v %v", dependencies, err)
	}
	for _, entry := range dependencies.Entries {
		if len(entry.ResourceID) != 64 {
			t.Fatal("SHA256 object identity 被缩短")
		}
	}
}

func TestGitCommitDependencyReadRejectsMissingOversizedAndInvalidObjects(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	adapter := NewRuntimeAdapter()
	scope := runtimecontract.RuntimeOperationExecutionContext{WorkspaceID: "ws-objects", Transaction: runtimecontract.AuthorityTransaction{DefinitionRevision: "definition-objects"}}
	output := RuntimeOperationResult{Operation: gitcommon.OperationCommit, Status: "completed", RepoRoot: repo, CommitHash: gitOutputForTest(t, repo, "rev-parse", "HEAD")}
	for _, id := range []string{"--help", " HEAD ", strings.Repeat("0", 40), strings.Repeat("a", 39)} {
		changed := output
		changed.CommitHash = id
		if _, err := adapter.commitOwnerDependencies(ctx, scope, changed); err == nil {
			t.Fatalf("接受非法或缺失对象 %q", id)
		}
	}
	for _, invalid := range []runtimecontract.RuntimeOperationExecutionContext{{}, {WorkspaceID: " ws ", Transaction: scope.Transaction}, {WorkspaceID: scope.WorkspaceID, Transaction: runtimecontract.AuthorityTransaction{DefinitionRevision: " definition "}}} {
		if _, err := adapter.commitOwnerDependencies(ctx, invalid, output); err == nil {
			t.Fatal("接受非 exact scope")
		}
	}
	message := strings.Repeat("原文", maxOutputBytes)
	messagePath := filepath.Join(t.TempDir(), "commit-message.txt")
	if err := os.WriteFile(messagePath, []byte(message), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitForTest(t, repo, "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-F", messagePath)
	output.CommitHash = gitOutputForTest(t, repo, "rev-parse", "HEAD")
	if _, err := adapter.commitOwnerDependencies(ctx, scope, output); err == nil || !strings.Contains(err.Error(), "not completely retained") {
		t.Fatalf("未显式拒绝不完整对象: %v", err)
	}
	output.ZeroWork = true
	if dependencies, err := adapter.commitOwnerDependencies(ctx, scope, output); err != nil || dependencies != nil {
		t.Fatal("no-op 补造对象读取依赖")
	}
}

func TestGitCommitDependencyReadRejectsReplacementCorruptionAndCancellation(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	adapter := NewRuntimeAdapter()
	head := gitOutputForTest(t, repo, "rev-parse", "HEAD")
	body, err := adapter.readDependencyObject(ctx, repo, "commit", head)
	if err != nil {
		t.Fatal(err)
	}
	runGitForTest(t, repo, "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-m", "replacement")
	replacement := gitOutputForTest(t, repo, "rev-parse", "HEAD")
	runGitForTest(t, repo, "replace", head, replacement)
	if got, err := adapter.readDependencyObject(ctx, repo, "commit", head); err != nil || got != body {
		t.Fatal("replace ref 改变原对象读取")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := adapter.readDependencyObject(cancelled, repo, "commit", head); err == nil {
		t.Fatal("取消后仍接受对象依赖")
	}
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write([]byte("commit 7\x00altered")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	objectPath := filepath.Join(repo, ".git", "objects", head[:2], head[2:])
	if err := os.Chmod(objectPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objectPath, compressed.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.readDependencyObject(ctx, repo, "commit", head); err == nil || !strings.Contains(err.Error(), "does not match its identity") {
		t.Fatalf("篡改的对象原 bytes 仍被接受: %v", err)
	}
}

func TestGitOwnerCommitReceiptPreservesObjectDependenciesAcrossRecreation(t *testing.T) {
	testGitOwnerCommitDependencies(t, "replay")
}

func TestGitOwnerCommitSnapshotLostReceiptReplyReplaysOriginalArchive(t *testing.T) {
	testGitOwnerCommitDependencies(t, "receipt-response-loss")
}

func TestGitOwnerCommitDependencyFailureDoesNotForgeReceipt(t *testing.T) {
	for _, mode := range []string{"invalid-scope", "oversized-commit", "oversized-blob"} {
		t.Run(mode, func(t *testing.T) { testGitOwnerCommitDependencies(t, mode) })
	}
}

func testGitOwnerCommitDependencies(t *testing.T, mode string) {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	receipts := map[string][]byte{}
	lostResponse := false
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		encoded, found := receipts[command.ExecutionKey]
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			var receipt runtimecontract.RuntimeOperationReceipt
			if err := json.Unmarshal(encoded, &receipt); err != nil {
				t.Error(err)
			}
			response.Receipt = &receipt
		}
		result.Set(response, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		encoded, err := json.Marshal(command.Receipt)
		if err != nil {
			t.Error(err)
		}
		receipts[command.Receipt.ExecutionKey] = encoded
		if mode == "receipt-response-loss" && !lostResponse {
			lostResponse = true
			setCapabilityError(result, fmt.Errorf("test: lost reply after original receipt persistence"))
			return
		}
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})
	repo := newTestRepo(t)
	writeFile(t, filepath.Join(repo, "delivered.txt"), "first\n")
	runGitForTest(t, repo, "add", "delivered.txt")
	operationName, message := gitcommon.OperationCommit, "交付本地提交"
	if mode == "oversized-commit" {
		message = strings.Repeat("文", maxOutputBytes/2)
	}
	if mode == "oversized-blob" {
		writeFile(t, filepath.Join(repo, "delivered.txt"), strings.Repeat("x", maxOutputBytes+1))
		runGitForTest(t, repo, "add", "delivered.txt")
	}
	operation := runtimecontract.RuntimeOperation{ID: "git-commit", Kind: runtimecontract.StepKindGitWrite, TargetRefs: []string{gitWorkspaceRepositorySelector}, Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &operationName}, {Name: "message", String: &message}}}
	sealed, err := gitevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	command := gitevents.ExecuteOperationCommand{RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws-git", RunID: "run-git", DispatchID: "dispatch-git", OperationID: operation.ID,
		Transaction:  runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-git", RequiredOutputHash: "output-git", DefinitionRevision: "definition-git"},
		Execution:    gitTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws-git/run-git/dispatch-git/git-commit", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable}),
		EvidenceKind: runtimecontract.EvidenceKindStateChange, WorkspaceDeliverableDirectory: repo, TargetRefs: operation.TargetRefs,
	}, SealedOperation: sealed}
	replayMode := mode == "replay" || mode == "receipt-response-loss"
	if replayMode {
		command.WorkspaceInternalDirectory = t.TempDir()
	}
	if mode == "invalid-scope" {
		command.Transaction.DefinitionRevision = " definition-git "
	}
	beforeHead := gitOutputForTest(t, repo, "rev-parse", "HEAD")
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Teardown(ctx) })
	if !replayMode {
		result := hub.Send(magicevent.NewEventWithContext(gitevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
		_, err := result.Get()
		if err == nil || len(receipts) != 0 {
			t.Fatal("依赖失败仍发布 fabricated completed/rejected receipt")
		}
		afterHead := gitOutputForTest(t, repo, "rev-parse", "HEAD")
		if mode == "invalid-scope" && afterHead != beforeHead {
			t.Fatal("非 exact scope 已发生 commit")
		}
		if (mode == "oversized-commit" || mode == "oversized-blob") && (afterHead == beforeHead || !strings.Contains(err.Error(), "not completely retained")) {
			t.Fatalf("已发生 effect 的不完整依赖未明确报告: %v", err)
		}
		return
	}
	if mode == "receipt-response-loss" {
		result := hub.Send(magicevent.NewEventWithContext(gitevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
		if _, err := result.Get(); err == nil || len(receipts) != 1 || !lostResponse {
			t.Fatal("receipt response-loss cut did not preserve one exact committed receipt")
		}
		// Rebuild the owner before retrying: no adapter cache or initial
		// return value may substitute for the persisted original receipt.
		if err := owner.Teardown(ctx); err != nil {
			t.Fatal(err)
		}
		owner = &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
		if err := owner.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = owner.Teardown(ctx) })
	}
	first := sendGitStateChangingReceiptCommand(t, ctx, hub, command)
	if first.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !first.Receipt.Changed || first.Receipt.Evidence.OwnerRevisionSet == nil || len(first.Receipt.Evidence.OwnerRevisionSet.Entries) != 4 {
		t.Fatalf("真实 Git owner receipt 缺依赖: %+v", first.Receipt)
	}
	head := gitOutputForTest(t, repo, "rev-parse", "HEAD")
	if first.Receipt.Evidence.ContentAddress == nil {
		t.Fatal("真实 owner 回执未引用保全的完整快照原 bytes")
	}
	original, err := readGitCommitSnapshot(ctx, command.WorkspaceInternalDirectory, gitCommitSnapshotScopeForExecution(command.RuntimeOperationExecutionContext), *first.Receipt.Evidence.ContentAddress)
	if err != nil || original.Head != head {
		t.Fatalf("不能按原 scope/content address 读回快照: %v", err)
	}
	if err := owner.BeginShutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if replay := sendGitStateChangingReceiptCommand(t, ctx, hub, command); !runtimecontract.SameRuntimeOperationReceipt(first.Receipt, replay.Receipt) {
		t.Fatal("drain 未重放 exact 对象依赖")
	}
	if err := owner.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "delivered.txt"), "second\n")
	runGitForTest(t, repo, "add", "delivered.txt")
	recreated := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := recreated.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recreated.Teardown(ctx) })
	if replay := sendGitStateChangingReceiptCommand(t, ctx, hub, command); !runtimecontract.SameRuntimeOperationReceipt(first.Receipt, replay.Receipt) {
		t.Fatal("重建 owner 后依赖/receipt 漂移")
	}
	if got := gitOutputForTest(t, repo, "rev-parse", "HEAD"); got != head {
		t.Fatal("receipt 重放再次创建 commit")
	}
	restored, err := readGitCommitSnapshot(ctx, command.WorkspaceInternalDirectory, original.Scope, *first.Receipt.Evidence.ContentAddress)
	if err != nil || !reflect.DeepEqual(original, restored) {
		t.Fatal("owner 重建改变原始快照 bytes 或 execution identity")
	}
	if _, err := os.Stat(filepath.Join(repo, "delivered.txt")); err != nil {
		t.Fatal(err)
	}
}
