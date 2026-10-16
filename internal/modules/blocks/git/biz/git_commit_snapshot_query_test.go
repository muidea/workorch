package git

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The Git producer and repository/archive are real. Original-source validation
// and Workspace reads here are explicit typed fixtures; the Runtime mainline
// test independently exercises the actual receipt handler and aggregate gate.
func TestGitCommitSnapshotQueryReadsOriginalArchiveAndFreshObjects(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repo, internal := newTestRepo(t), t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "validation"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "delivered.txt"), "original\x00\xff\n")
	writeFile(t, filepath.Join(repo, "validation", "contract_test.go"), "original reviewed input\x00\n")
	if err := os.Symlink("validation/contract_test.go", filepath.Join(repo, "validation-link")); err != nil {
		t.Fatal(err)
	}
	runGitForTest(t, repo, "add", "delivered.txt")
	runGitForTest(t, repo, "add", "validation", "validation-link")
	name, message := gitcommon.OperationCommit, "original delivered snapshot"
	operation := runtimecontract.RuntimeOperation{ID: "commit", Kind: runtimecontract.StepKindGitWrite, TargetRefs: []string{gitWorkspaceRepositorySelector},
		Parameters: []runtimecontract.OperationParameter{{Name: "operation", String: &name}, {Name: "message", String: &message}}}
	sealed, err := gitevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	scope := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: operation.ID, TargetRefs: operation.TargetRefs,
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority", RequiredOutputHash: "output", DefinitionRevision: "definition"},
		Execution: gitTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws/run/dispatch/commit", Fence: 1,
			ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable}),
		EvidenceKind: runtimecontract.EvidenceKindStateChange, WorkspaceDeliverableDirectory: repo, WorkspaceInternalDirectory: internal,
	}
	readSource := runtimecontract.CloneRuntimeOperationExecutionContext(scope)
	readSource.WorkspaceDeliverableDirectory, readSource.WorkspaceInternalDirectory = "", ""
	var persisted *runtimecontract.RuntimeOperationReceipt
	commits, sources, workspaces := 0, 0, 0
	queryMode := "valid"
	entered, release := make(chan struct{}), make(chan struct{})
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	if err := observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(ev magicevent.Event, result magicevent.Result) {
		command := ev.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		if command.ReadSource != nil {
			sources++
			if queryMode == "blocked" {
				close(entered)
				<-release
			}
			if ev.Source() != ModuleID || !reflect.DeepEqual(*command.ReadSource, readSource) {
				setCapabilityError(result, fmt.Errorf("original source mismatch"))
				return
			}
			if queryMode == "missing" {
				result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{}, nil)
				return
			}
			if queryMode == "wrong-type" {
				result.Set(struct{}{}, nil)
				return
			}
		}
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: persisted != nil, Receipt: persisted}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	if err := observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(ev magicevent.Event, result magicevent.Result) {
		commits++
		value := ev.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand).Receipt.Clone()
		persisted = &value
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: value}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	if err := workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(ev magicevent.Event, result magicevent.Result) {
		workspaces++
		command := ev.Data().(workspaceevents.GetWorkspaceCommand)
		if ev.Source() != ModuleID || command.WorkspaceID != "ws" {
			t.Error("workspace query source/scope mismatch")
		}
		if queryMode == "workspace-failure" {
			setCapabilityError(result, fmt.Errorf("Workspace is retired or unavailable"))
			return
		}
		if queryMode == "workspace-wrong-type" {
			result.Set(struct{}{}, nil)
			return
		}
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws", DeliverableDirectory: repo, InternalDirectory: internal}}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		_ = observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		_ = workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)
	})
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Teardown(ctx) })
	first := sendGitStateChangingReceiptCommand(t, ctx, hub, gitevents.ExecuteOperationCommand{RuntimeOperationExecutionContext: scope, SealedOperation: sealed})
	originalBytes, err := json.Marshal(first.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	command := gitevents.QueryRuntimeCommitSnapshotCommand{
		RuntimeOperationExecutionContext: readSource, ProtocolRef: gitevents.RuntimeCommitSnapshotQueryProtocol, RequestID: "snapshot-query", IssuedAtStateRevision: 17,
		SealedOperation: sealed, SourceReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(first.Receipt),
		SnapshotAddress: *first.Receipt.Evidence.ContentAddress, OriginalDependencies: *runtimecontract.CloneRuntimeOwnerRevisionSet(first.Receipt.Evidence.OwnerRevisionSet),
	}
	send := func(source string, value gitevents.QueryRuntimeCommitSnapshotCommand) (gitevents.QueryRuntimeCommitSnapshotResult, error) {
		response := hub.Send(magicevent.NewEventWithContext(gitevents.TopicQueryRuntimeCommitSnapshot, source, ModuleID, magicevent.NewHeader(), ctx, value))
		data, err := response.Get()
		if err != nil {
			return gitevents.QueryRuntimeCommitSnapshotResult{}, err
		}
		result, ok := data.(gitevents.QueryRuntimeCommitSnapshotResult)
		if !ok {
			t.Fatalf("unexpected snapshot response %T", data)
		}
		return result, nil
	}
	response, err := send(runtimecontract.RuntimeModuleOrchestrator, command)
	if err != nil || response.ValidateFor(command) != nil || response.CurrentDependencies == nil || response.SnapshotHead != gitOutputForTest(t, repo, "rev-parse", "HEAD") ||
		response.SnapshotRootTree != gitOutputForTest(t, repo, "rev-parse", "HEAD^{tree}") || sources != 1 || workspaces != 1 {
		t.Fatalf("original snapshot/current readback not connected: %+v %v", response, err)
	}
	// Raw binary bodies and counts are independently read by the archive oracle
	// tests; this query must reproduce their complete receipt-derived set.
	if !reflect.DeepEqual(response.CurrentDependencies, first.Receipt.Evidence.OwnerRevisionSet) {
		t.Fatal("query did not reproduce actual complete object dependencies")
	}
	// This fixture owns only Git membership. It deliberately does not claim
	// its caller-supplied binding has a Runtime review; that source is tested
	// separately using actual File/Shell/Run and the complete aggregate gate.
	inputCommand := command
	inputCommand.ProtocolRef = gitevents.RuntimeCommitSnapshotInputQueryProtocol
	inputCommand.InputBinding = gitSnapshotTestInputBinding("validation/contract_test.go", "original reviewed input\x00\n")
	inputs, err := send(runtimecontract.RuntimeModuleOrchestrator, inputCommand)
	if err != nil || inputs.ValidateFor(inputCommand) != nil || len(inputs.InputMemberships) != 1 ||
		inputs.InputMemberships[0].BlobID != gitOutputForTest(t, repo, "rev-parse", "HEAD:validation/contract_test.go") {
		t.Fatalf("actual original blob membership not produced: %+v %v", inputs, err)
	}
	for _, scenario := range []string{"v1-extension", "v2-missing", "untracked", "different-bytes", "symlink", "directory", "traversal", "duplicate", "partial-bytes", "invalid-utf8"} {
		t.Run("input-"+scenario, func(t *testing.T) {
			bad := inputCommand
			binding := bad.InputBinding.Clone()
			bad.InputBinding = &binding
			switch scenario {
			case "v1-extension":
				bad.ProtocolRef = gitevents.RuntimeCommitSnapshotQueryProtocol
			case "v2-missing":
				bad.InputBinding = nil
			case "untracked":
				binding.InputReview.Files[0].TargetRef = "not-tracked.go"
			case "different-bytes":
				binding.InputReview.Files[0].Content = "substituted\n"
			case "symlink":
				binding.InputReview.Files[0].TargetRef = "validation-link"
				binding.InputReview.Files[0].Content = "validation/contract_test.go"
			case "directory":
				binding.InputReview.Files[0].TargetRef = "validation"
			case "traversal":
				binding.InputReview.Files[0].TargetRef = "validation/../validation/contract_test.go"
			case "duplicate":
				binding.InputReview.Files = append(binding.InputReview.Files, binding.InputReview.Files[0])
			case "partial-bytes":
				binding.InputReview.Files[0].Content = "original reviewed input"
			case "invalid-utf8":
				binding.InputReview.Files[0].TargetRef = "delivered.txt"
				binding.InputReview.Files[0].Content = "original\x00\xff\n"
			}
			binding.InputReview.Files[0].ContentHash = runtimecontract.RuntimeValidationInputContentHash(binding.InputReview.Files[0].Content)
			binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
			if _, err := send(runtimecontract.RuntimeModuleOrchestrator, bad); err == nil {
				t.Fatal("missing/altered/nonregular input accepted as complete blob membership")
			}
		})
	}
	for _, scenario := range []string{"subject", "path", "hash", "blob", "missing", "duplicate"} {
		t.Run("input-response-"+scenario, func(t *testing.T) {
			bad := inputs.Clone()
			switch scenario {
			case "subject":
				bad.InputSubjectHash = "another-subject"
			case "path":
				bad.InputMemberships[0].TargetRef = "another-path"
			case "hash":
				bad.InputMemberships[0].ContentHash = strings.Repeat("0", 64)
			case "blob":
				bad.InputMemberships[0].BlobID = inputs.SnapshotHead
			case "missing":
				bad.InputMemberships = nil
			case "duplicate":
				bad.InputMemberships = append(bad.InputMemberships, bad.InputMemberships[0])
			}
			if bad.ValidateFor(inputCommand) == nil {
				t.Fatal("tampered input membership response accepted")
			}
		})
	}
	inputClone := inputs.Clone()
	inputClone.InputMemberships[0].TargetRef = "caller"
	if inputs.InputMemberships[0].TargetRef != "validation/contract_test.go" {
		t.Fatal("input membership result shares caller storage")
	}
	inputJSON, _ := json.Marshal(inputs)
	var inputReload gitevents.QueryRuntimeCommitSnapshotResult
	if json.Unmarshal(inputJSON, &inputReload) != nil || !reflect.DeepEqual(inputs, inputReload) || inputReload.ValidateFor(inputCommand) != nil {
		t.Fatal("input membership reload lost original identity")
	}
	terminalCommand := inputCommand
	terminalCommand.ProtocolRef = gitevents.RuntimeCommitSnapshotTerminalQueryProtocol
	if first.Receipt.Evidence.OwnerSnapshot == nil {
		t.Fatal("original commit receipt did not seal snapshot identity")
	}
	identity := *first.Receipt.Evidence.OwnerSnapshot
	terminalCommand.OriginalSnapshot = &identity
	reads := runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: command.Transaction.DefinitionRevision, Entries: []runtimecontract.RuntimeOwnerRevisionRef{
		{OwnerKind: "capability", OwnerInstanceID: runtimecontract.RuntimeModuleShell + "/ws", ResourceKind: string(runtimecontract.RuntimeResourceWorkspaceTarget), ResourceID: "delivered.txt", Revision: runtimecontract.RuntimeValidationInputContentHash("original\x00\xff\n"), DependencyHash: strings.Repeat("a", 64)},
		{OwnerKind: "capability", OwnerInstanceID: runtimecontract.RuntimeModuleShell + "/ws", ResourceKind: string(runtimecontract.RuntimeResourceWorkspaceTarget), ResourceID: "validation/contract_test.go", Revision: inputCommand.InputBinding.InputReview.Files[0].ContentHash, DependencyHash: strings.Repeat("b", 64)},
	}}.WithHash()
	terminalCommand.ValidationDependencies = &reads
	terminal, err := send(runtimecontract.RuntimeModuleOrchestrator, terminalCommand)
	if err != nil || terminal.ValidateFor(terminalCommand) != nil || terminal.WorkspaceClean == nil || !*terminal.WorkspaceClean || len(terminal.ValidationMemberships) != 2 {
		t.Fatalf("V3 complete original validation graph/clean observation: %+v / %v", terminal, err)
	}
	for _, scenario := range []string{"missing", "protocol", "absent", "swapped", "blob-as-head", "legacy-v1", "legacy-v2"} {
		t.Run("terminal-original-identity-"+scenario, func(t *testing.T) {
			bad := terminalCommand
			value := *terminalCommand.OriginalSnapshot
			bad.OriginalSnapshot = &value
			switch scenario {
			case "missing":
				bad.OriginalSnapshot = nil
			case "protocol":
				value.ProtocolRef = "unsupported"
			case "absent":
				value.SnapshotID = strings.Repeat("0", len(value.SnapshotID))
			case "swapped":
				value.SnapshotID, value.RootSnapshotID = value.RootSnapshotID, value.SnapshotID
			case "blob-as-head":
				value.SnapshotID = terminal.InputMemberships[0].BlobID
			case "legacy-v1":
				bad.ProtocolRef, bad.InputBinding, bad.ValidationDependencies = gitevents.RuntimeCommitSnapshotQueryProtocol, nil, nil
			case "legacy-v2":
				bad.ProtocolRef, bad.ValidationDependencies = gitevents.RuntimeCommitSnapshotInputQueryProtocol, nil
			}
			before := workspaces
			if _, err := send(runtimecontract.RuntimeModuleOrchestrator, bad); err == nil || workspaces != before {
				t.Fatal("substituted original identity reached Workspace/filesystem or was accepted")
			}
		})
	}
	for _, scenario := range []string{"head", "root", "swapped"} {
		t.Run("terminal-response-identity-"+scenario, func(t *testing.T) {
			bad := terminal.Clone()
			switch scenario {
			case "head":
				bad.SnapshotHead, bad.ObservedHead = terminal.InputMemberships[0].BlobID, terminal.InputMemberships[0].BlobID
			case "root":
				bad.SnapshotRootTree = terminal.InputMemberships[0].BlobID
			case "swapped":
				bad.SnapshotHead, bad.ObservedHead, bad.SnapshotRootTree = terminal.SnapshotRootTree, terminal.SnapshotRootTree, terminal.SnapshotHead
			}
			if bad.ValidateFor(terminalCommand) == nil {
				t.Fatal("another object from original graph substituted for commit/root")
			}
		})
	}
	for _, scenario := range []string{"nil-reads", "legacy-reads", "wrong-definition", "unread-bytes", "missing-member", "substitute-member", "nil-clean", "legacy-clean"} {
		t.Run("terminal-"+scenario, func(t *testing.T) {
			bad := terminalCommand
			bad.ValidationDependencies = runtimecontract.CloneRuntimeOwnerRevisionSet(terminalCommand.ValidationDependencies)
			result := terminal.Clone()
			switch scenario {
			case "nil-reads":
				bad.ValidationDependencies = nil
			case "legacy-reads":
				bad.ProtocolRef = gitevents.RuntimeCommitSnapshotInputQueryProtocol
			case "wrong-definition":
				bad.ValidationDependencies.DefinitionRevision = "another"
			case "unread-bytes":
				bad.ValidationDependencies.Entries[0].Revision = strings.Repeat("0", 64)
				*bad.ValidationDependencies = bad.ValidationDependencies.WithHash()
			case "missing-member":
				result.ValidationMemberships = result.ValidationMemberships[:1]
			case "substitute-member":
				result.ValidationMemberships[0].BlobID = result.SnapshotHead
			case "nil-clean":
				result.WorkspaceClean = nil
			case "legacy-clean":
				bad.ProtocolRef, bad.ValidationDependencies = gitevents.RuntimeCommitSnapshotInputQueryProtocol, nil
				bad.OriginalSnapshot = nil
				result.ProtocolRef, result.ValidationMemberships = bad.ProtocolRef, nil
			}
			if scenario == "nil-reads" || scenario == "legacy-reads" || scenario == "wrong-definition" || scenario == "unread-bytes" {
				if _, err := send(runtimecontract.RuntimeModuleOrchestrator, bad); err == nil {
					t.Fatal("unsupported/altered original readset admitted")
				}
			} else if result.ValidateFor(bad) == nil {
				t.Fatal("partial/altered terminal observation admitted")
			}
		})
	}
	for _, scenario := range []string{"untracked", "worktree", "index"} {
		t.Run("terminal-dirty-"+scenario, func(t *testing.T) {
			path := filepath.Join(repo, "delivered.txt")
			if scenario == "untracked" {
				path = filepath.Join(repo, "untracked.txt")
			}
			writeFile(t, path, "dirty\n")
			if scenario == "index" {
				runGitForTest(t, repo, "add", "delivered.txt")
			}
			result, err := send(runtimecontract.RuntimeModuleOrchestrator, terminalCommand)
			if err != nil || result.ValidateFor(terminalCommand) != nil || result.WorkspaceClean == nil || *result.WorkspaceClean || !reflect.DeepEqual(result.ValidationMemberships, terminal.ValidationMemberships) {
				t.Fatalf("dirty workspace concealed/original membership rewritten: %+v / %v", result, err)
			}
			if scenario == "untracked" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				writeFile(t, path, "original\x00\xff\n")
				if scenario == "index" {
					runGitForTest(t, repo, "add", "delivered.txt")
				}
			}
		})
	}
	terminalCopy := terminal.Clone()
	terminalCopy.ValidationMemberships[0].TargetRef, *terminalCopy.WorkspaceClean = "caller", false
	if terminal.ValidationMemberships[0].TargetRef == "caller" || !*terminal.WorkspaceClean {
		t.Fatal("terminal result aliases original owner observation")
	}
	terminalJSON, _ := json.Marshal(terminal)
	var terminalReload gitevents.QueryRuntimeCommitSnapshotResult
	if json.Unmarshal(terminalJSON, &terminalReload) != nil || !reflect.DeepEqual(terminal, terminalReload) || terminalReload.ValidateFor(terminalCommand) != nil {
		t.Fatal("terminal JSON reload changed exact owner fact")
	}
	// Neither a worktree edit nor a later HEAD may rewrite the original input
	// membership. A later HEAD continues to return no current original graph.
	writeFile(t, filepath.Join(repo, "validation", "contract_test.go"), "uncommitted replacement\n")
	inputRepeat, err := send(runtimecontract.RuntimeModuleOrchestrator, inputCommand)
	if err != nil || !reflect.DeepEqual(inputRepeat, inputs) {
		t.Fatalf("working tree replaced original blob bytes: %v", err)
	}
	cloned := response.Clone()
	cloned.CurrentDependencies.Entries[0].Revision = "caller"
	if reflect.DeepEqual(cloned.CurrentDependencies, response.CurrentDependencies) {
		t.Fatal("query result shared mutable dependencies")
	}
	encoded, _ := json.Marshal(response)
	var reloaded gitevents.QueryRuntimeCommitSnapshotResult
	if json.Unmarshal(encoded, &reloaded) != nil || !reflect.DeepEqual(response, reloaded) || reloaded.ValidateFor(command) != nil {
		t.Fatal("query observation reload changed original value")
	}
	for _, scenario := range []string{"protocol", "request", "scope", "fence", "directory", "receipt-hash", "address", "dependencies", "source", "missing", "wrong-type", "workspace-failure", "workspace-wrong-type"} {
		t.Run(scenario, func(t *testing.T) {
			value := command
			value.RuntimeOperationExecutionContext = runtimecontract.CloneRuntimeOperationExecutionContext(command.RuntimeOperationExecutionContext)
			value.OriginalDependencies = *runtimecontract.CloneRuntimeOwnerRevisionSet(&command.OriginalDependencies)
			source := runtimecontract.RuntimeModuleOrchestrator
			beforeSource, beforeWorkspace := sources, workspaces
			queryMode = "valid"
			switch scenario {
			case "protocol":
				value.ProtocolRef = "git/commit-object-snapshot-query/v0"
			case "request":
				value.RequestID = strings.Repeat("x", 2049)
			case "scope":
				value.Transaction.DefinitionRevision = "forged-definition"
			case "fence":
				value.Execution.Fence++
			case "directory":
				value.WorkspaceInternalDirectory = t.TempDir()
			case "receipt-hash":
				value.SourceReceiptHash = strings.Repeat("0", 64)
			case "address":
				value.SnapshotAddress.Digest = strings.Repeat("0", 64)
				value.SnapshotAddress.Ref = gitevents.RuntimeCommitSnapshotProtocol + "/" + value.SnapshotAddress.Digest
			case "dependencies":
				value.OriginalDependencies.Entries[0].Revision = strings.Repeat("0", 64)
				value.OriginalDependencies = value.OriginalDependencies.WithHash()
			case "source":
				source = "blocks/file"
			default:
				queryMode = scenario
			}
			if _, err := send(source, value); err == nil {
				t.Fatal("invalid original source/query accepted")
			}
			if scenario == "protocol" || scenario == "request" || scenario == "directory" || scenario == "source" || scenario == "scope" {
				if sources != beforeSource || workspaces != beforeWorkspace {
					t.Fatal("invalid ingress performed source/workspace I/O")
				}
			} else if !strings.HasPrefix(scenario, "workspace-") && workspaces != beforeWorkspace {
				t.Fatal("invalid source performed Workspace/filesystem I/O")
			}
		})
	}
	queryMode = "valid"
	// The archive being complete cannot hide an incomplete current repository.
	// Move only fixture-owned loose objects and restore them even on failure.
	for _, objectID := range []string{gitOutputForTest(t, repo, "rev-parse", "HEAD:delivered.txt"), response.SnapshotRootTree} {
		t.Run("missing-current-object-"+objectID, func(t *testing.T) {
			path := filepath.Join(repo, ".git", "objects", objectID[:2], objectID[2:])
			backup := filepath.Join(t.TempDir(), "original-object")
			if err := os.Rename(path, backup); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.Rename(backup, path); err != nil {
					t.Error(err)
				}
			}()
			if _, err := send(runtimecontract.RuntimeModuleOrchestrator, command); err == nil {
				t.Fatal("historical archive replaced missing actual current object read")
			}
		})
	}
	cancelled, cancelQuery := context.WithCancel(ctx)
	cancelQuery()
	if _, err := owner.runtimeAdapter.observeOriginalCommitSnapshot(cancelled, command, internal, repo); err == nil {
		t.Fatal("cancelled snapshot query claimed current object reads")
	}
	for _, scenario := range []string{"identity", "address", "head", "tree", "count", "bytes", "dependencies", "empty-current", "changed-with-current"} {
		t.Run("response-"+scenario, func(t *testing.T) {
			value := response.Clone()
			switch scenario {
			case "identity":
				value.IssuedAtStateRevision++
			case "address":
				value.SnapshotAddress.Digest = strings.Repeat("0", 64)
			case "head":
				value.SnapshotHead = strings.Repeat("0", len(value.SnapshotHead))
			case "tree":
				value.SnapshotRootTree = strings.Repeat("0", len(value.SnapshotRootTree))
			case "count":
				value.ObjectCount--
			case "bytes":
				value.ObjectBytes = 4*1024*1024 + 1
			case "dependencies":
				value.CurrentDependencies.Entries[0].Revision = strings.Repeat("0", 64)
				current := value.CurrentDependencies.WithHash()
				value.CurrentDependencies = &current
			case "empty-current":
				value.CurrentDependencies = nil
			case "changed-with-current":
				value.ObservedHead = strings.Repeat("0", len(value.ObservedHead))
			}
			if value.ValidateFor(command) == nil {
				t.Fatal("invalid query response accepted")
			}
		})
	}
	// Owner reconstruction reads the same original archive; queries never commit.
	if err := owner.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	owner = &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	second, err := send(runtimecontract.RuntimeModuleOrchestrator, command)
	if err != nil || !reflect.DeepEqual(response, second) {
		t.Fatalf("owner rebuild query changed: %v", err)
	}
	inputRebuilt, err := send(runtimecontract.RuntimeModuleOrchestrator, inputCommand)
	if err != nil || !reflect.DeepEqual(inputRebuilt, inputs) {
		t.Fatalf("owner rebuild changed original input membership: %v", err)
	}
	writeFile(t, filepath.Join(repo, "delivered.txt"), "uncommitted change\n")
	third, err := send(runtimecontract.RuntimeModuleOrchestrator, command)
	if err != nil || !reflect.DeepEqual(response, third) {
		t.Fatal("query falsely treated working-tree change as immutable object drift")
	}
	runGitForTest(t, repo, "add", "delivered.txt")
	runGitForTest(t, repo, "-c", "core.hooksPath=/dev/null", "commit", "-m", "later commit")
	later, err := send(runtimecontract.RuntimeModuleOrchestrator, command)
	if err != nil || later.ValidateFor(command) != nil || later.CurrentDependencies != nil || later.ObservedHead == response.SnapshotHead || later.SnapshotHead != response.SnapshotHead {
		t.Fatalf("later HEAD hidden by historical receipt: %+v %v", later, err)
	}
	inputLater, err := send(runtimecontract.RuntimeModuleOrchestrator, inputCommand)
	if err != nil || inputLater.ValidateFor(inputCommand) != nil || inputLater.CurrentDependencies != nil || !reflect.DeepEqual(inputLater.InputMemberships, inputs.InputMemberships) {
		t.Fatalf("later HEAD replaced original input membership/currentness distinction: %v", err)
	}
	// Archive corruption is a failed read, not a missing/current observation.
	archive := filepath.Join(internal, "git", "commit-object-snapshots", gitCommitSnapshotFilename(gitCommitSnapshotScopeForExecution(scope)))
	if err := os.WriteFile(archive, []byte("corrupted archive\x00\xff"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := send(runtimecontract.RuntimeModuleOrchestrator, command); err == nil {
		t.Fatal("corrupted archive accepted")
	}
	// A real accepted snapshot query holds its lifecycle ticket through the
	// nested original-receipt read. Closing admission is not its completion.
	queryMode = "blocked"
	done := make(chan error, 1)
	go func() { _, err := send(runtimecontract.RuntimeModuleOrchestrator, command); done <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("snapshot query did not enter nested source read")
	}
	if err := owner.BeginShutdown(ctx); err != nil {
		t.Fatal(err)
	}
	budget, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := owner.Quiesce(budget); err == nil || owner.runtimeAdapter == nil {
		t.Fatal("quiescence falsely completed/released live snapshot read")
	}
	close(release)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked read ignored archive corruption")
		}
	case <-time.After(time.Second):
		t.Fatal("snapshot query did not complete after source read")
	}
	if err := owner.Quiesce(ctx); err != nil {
		t.Fatal(err)
	}
	beforeSource, beforeWorkspace := sources, workspaces
	if _, err := send(runtimecontract.RuntimeModuleOrchestrator, command); err == nil || sources != beforeSource || workspaces != beforeWorkspace {
		t.Fatal("shutdown query performed fresh owner I/O")
	}
	after, _ := json.Marshal(*persisted)
	if commits != 1 || string(after) != string(originalBytes) {
		t.Fatal("query/restart/failure rewrote receipt or executed another commit")
	}
}

func gitSnapshotTestInputBinding(target, content string) *runtimecontract.RuntimeValidationProofRequirementBinding {
	binding := runtimecontract.RuntimeValidationProofRequirementBinding{
		RequirementHash: "original-requirement", DefinitionRevision: "definition", CoverageWorkItemID: "original-cwi", DeclarationRef: "original-declaration",
		ValidationRecipeID: "original-recipe", RequiredCheckRefs: []string{"original-check"},
		CheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "original-check", ObservationProtocolRef: "original-native-protocol", NativeCheckRef: "original-native-check"}},
	}
	binding.InputReview = &runtimecontract.RuntimeValidationInputReviewSubject{ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol,
		Files: []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: target, Content: content, ContentHash: runtimecontract.RuntimeValidationInputContentHash(content)}}}
	binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	return &binding
}
