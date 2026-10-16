package shell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellValidationCopiedInputsPreserveActualBytesAndRejectPartialCopy(t *testing.T) {
	source, destination := t.TempDir(), t.TempDir()
	original := []byte("原始\r\n\x00\xffbytes\n")
	if err := os.WriteFile(filepath.Join(source, "input.bin"), original, 0700); err != nil {
		t.Fatal(err)
	}
	inputs, err := copyManagedValidationWorkspace(source, destination)
	if err != nil || len(inputs) != 1 {
		t.Fatalf("actual copy observation: %v %+v", err, inputs)
	}
	digest := sha256.Sum256(original)
	if inputs[0] != (shellValidationCopiedInput{TargetRef: "input.bin", ContentHash: hex.EncodeToString(digest[:]), Size: int64(len(original)), Mode: 0700}) {
		t.Fatal("复制事实未绑定原始 bytes／长度／mode")
	}
	copied, err := os.ReadFile(filepath.Join(destination, "input.bin"))
	if err != nil || !reflect.DeepEqual(copied, original) {
		t.Fatalf("复制字节与观察不同: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "input.bin"), []byte("newer"), 0600); err != nil {
		t.Fatal(err)
	}
	request := RuntimeCommandRequest{WorkspaceID: "ws", ValidationRecipeID: "recipe", Command: "go", Args: []string{"test", "./..."}, ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true}}
	dependencies, err := shellValidationOwnerDependencies("definition", request, inputs)
	if err != nil || dependencies.Entries[0].Revision != hex.EncodeToString(digest[:]) {
		t.Fatalf("依赖借用了 newer 源文件: %v", err)
	}
	preimage, err := json.Marshal(struct {
		WorkspaceID string
		Input       struct {
			TargetRef, ContentHash string
			Size                   int64
			Mode                   uint32
		}
		CopyProtocol        string
		ExcludedDirectories []string
		MaxFiles            int
		MaxBytes            int64
	}{WorkspaceID: "ws", Input: struct {
		TargetRef, ContentHash string
		Size                   int64
		Mode                   uint32
	}{"input.bin", hex.EncodeToString(digest[:]), int64(len(original)), 0700}, CopyProtocol: "shell-managed-workspace-copy/v1", ExcludedDirectories: []string{".git", "node_modules", "vendor", "dist", "build", ".workorch-data", ".workorch-workspaces"}, MaxFiles: 8192, MaxBytes: 256 * 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	expectedDependency := sha256.Sum256(append([]byte("shell-validation-copied-input/v2\x00"), preimage...))
	if dependencies.Entries[0].DependencyHash != hex.EncodeToString(expectedDependency[:]) {
		t.Fatal("实际复制依赖与独立 bytes／selector 前像不符")
	}
	for _, source := range []string{"definition", "recipe", "command", "args", "scope", "checks", "workspace", "mode", "size"} {
		t.Run(source, func(t *testing.T) {
			changedRequest, definition := request, "definition"
			changedInputs := append([]shellValidationCopiedInput(nil), inputs...)
			switch source {
			case "definition":
				definition = "other"
			case "recipe":
				changedRequest.ValidationRecipeID = "other"
			case "command":
				changedRequest.Command = "other"
			case "args":
				changedRequest.Args = []string{"test", "-race", "./..."}
			case "scope":
				changedRequest.ValidationScopeTargetRefs = []string{"input.bin"}
			case "checks":
				changedRequest.ValidationCheckAttestations = []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "required", NativeCheckRef: "native", ObservationProtocolRef: "protocol"}}
			case "workspace":
				changedRequest.WorkspaceID = "other"
			case "mode":
				changedInputs[0].Mode = 0600
			case "size":
				changedInputs[0].Size++
			}
			changed, err := shellValidationOwnerDependencies(definition, changedRequest, changedInputs)
			executionOnly := source == "recipe" || source == "command" || source == "args" || source == "scope" || source == "checks"
			if executionOnly {
				if err != nil || !reflect.DeepEqual(changed, dependencies) {
					t.Fatalf("相同 full-copy 因无关执行身份产生读取冲突: %v", err)
				}
				if union, err := runtimecontract.CompileRuntimeOwnerRevisionSetUnion(definition, dependencies, changed); err != nil || !reflect.DeepEqual(union, dependencies) {
					t.Fatalf("相同实际读取不能幂等合并: %v", err)
				}
			} else if err != nil || changed.SetHash == dependencies.SetHash {
				t.Fatalf("读取 scope／selector 未绑定: %v", err)
			}
		})
	}
	emptyInputs, err := copyManagedValidationWorkspace(t.TempDir(), t.TempDir())
	if err != nil || emptyInputs == nil || len(emptyInputs) != 0 {
		t.Fatal("完整空目录复制与缺少本次观察混淆")
	}
	if empty, err := shellValidationOwnerDependencies("definition", request, emptyInputs); err != nil || len(empty.Entries) != 0 || empty.Validate() != nil {
		t.Fatalf("完整空复制补造了 owner 读取: %v", err)
	}
	for _, scenario := range []string{"definition", "workspace", "target", "content", "nil"} {
		changedRequest, changedInputs, definition := request, append([]shellValidationCopiedInput(nil), inputs...), "definition"
		switch scenario {
		case "definition":
			definition = " definition"
		case "workspace":
			changedRequest.WorkspaceID = " ws"
		case "target":
			changedInputs[0].TargetRef = " input.bin"
		case "content":
			changedInputs[0].ContentHash = " " + changedInputs[0].ContentHash
		case "nil":
			changedInputs = nil
		}
		if invalid, err := shellValidationOwnerDependencies(definition, changedRequest, changedInputs); err == nil || invalid.SetHash != "" {
			t.Fatalf("raw identity 或缺实际观察未拒绝: %s", scenario)
		}
	}
	if err := os.Symlink("input.bin", filepath.Join(source, "z-link")); err != nil {
		t.Fatal(err)
	}
	if partial, err := copyManagedValidationWorkspace(source, t.TempDir()); err == nil || partial != nil {
		t.Fatal("部分复制失败却返回完整读取集合")
	}
	if partial, err := shellValidationOwnerDependencies("definition", request, append(inputs, inputs[0])); err == nil || partial.SetHash != "" {
		t.Fatal("重复资源依赖未拒绝")
	}
	oversized := t.TempDir()
	file, err := os.Create(filepath.Join(oversized, "too-large"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxObservedWorkspaceBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if partial, err := copyManagedValidationWorkspace(oversized, t.TempDir()); err == nil || partial != nil {
		t.Fatal("超容量复制未显式拒绝完整读取集合")
	}
}

func TestShellValidationOwnerPersistsCopiedDependenciesAndReplaysOriginalReceipt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Teardown(ctx); err != nil {
			t.Error(err)
		}
	})
	var persisted *runtimecontract.RuntimeOperationReceipt
	commits := 0
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		query := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		if query.ReadSource != nil && (query.ReadSource.WorkspaceID != query.WorkspaceID || query.ReadSource.Execution.ExecutionKey != query.ExecutionKey) {
			t.Fatal("copied-input query omitted original full read source")
		}
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: persisted != nil, Receipt: persisted}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		commits++
		encoded, err := json.Marshal(command.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		var loaded runtimecontract.RuntimeOperationReceipt
		if err := json.Unmarshal(encoded, &loaded); err != nil {
			t.Fatal(err)
		}
		persisted = &loaded
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: loaded}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
	workspace := newGoValidationWorkspace(t, "package fixture\nimport \"testing\"\nfunc TestWorkOrchCWI_SHELLSEALTEST(t *testing.T) { if 2+2 != 4 { t.Fatal(\"original assertion\") } }\n")
	raw := []byte("原文\r\ninput\n")
	if err := os.WriteFile(filepath.Join(workspace, "source.txt"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	// 真实 native 进程仅用于验证 owner observation；不代表完整公开验收。
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "copy-observation", Command: "go", Args: []string{"test", "-v", "./..."}, TimeoutMillis: 60000}
	command := sealedShellTestCommand(t, owner.runtimeAdapter, runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "validate",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority", DefinitionRevision: "definition"},
		Execution:   runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: "exec/ws/run/dispatch/validate", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectEphemeralSession},
		TargetRefs:  []string{"fixture_test.go"}, EvidenceKind: runtimecontract.EvidenceKindValidation, WorkspaceDeliverableDirectory: workspace,
	}, runtimecontract.RuntimeBlockKindManagedValidationSession, shellevents.OperationData{ValidationRecipeID: recipe.ID}, "implementation", []runtimecontract.RuntimeValidationRecipe{recipe}, []string{"fixture.go", "fixture_test.go", "go.mod", "source.txt"})
	send := func(command shellevents.ExecuteOperationCommand) (shellevents.ExecuteOperationResult, error) {
		data, err := hub.Send(magicevent.NewEventWithContext(shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command)).Get()
		if err != nil {
			return shellevents.ExecuteOperationResult{}, err
		}
		response, ok := data.(shellevents.ExecuteOperationResult)
		if !ok {
			t.Fatalf("错误 typed response %T", data)
		}
		return response, nil
	}
	for _, definition := range []string{"", " definition", "definition "} {
		invalid := command
		invalid.Transaction.DefinitionRevision = definition
		if _, err := send(invalid); err == nil || commits != 0 {
			t.Fatal("未封存 exact Definition 却执行/commit")
		}
	}
	response, err := send(command)
	if err != nil {
		t.Fatal(err)
	}
	dependencies := response.Receipt.Evidence.OwnerRevisionSet
	digest := sha256.Sum256(raw)
	if dependencies == nil || dependencies.Validate() != nil || dependencies.DefinitionRevision != "definition" || len(dependencies.Entries) != 4 || len(response.Receipt.ResourceChanges) != 0 {
		t.Fatalf("缺实际复制依赖或混入 mutation: %+v", dependencies)
	}
	if response.Receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || response.Receipt.Evidence.Validation == nil || response.Receipt.Evidence.ExitCode == nil || *response.Receipt.Evidence.ExitCode != 0 {
		t.Fatal("真实 native validation 未执行完成")
	}
	if response.Receipt.Evidence.ApplicabilityDependencies == nil || !reflect.DeepEqual(response.Receipt.Evidence.ApplicabilityDependencies.OwnerRevisionSet, *dependencies) {
		t.Fatal("actual copy snapshot was not preserved independently for applicability")
	}
	for _, input := range dependencies.Entries {
		if input.OwnerInstanceID != ModuleID+"/ws" {
			t.Fatal("实际复制来源 owner scope 漂移")
		}
		body, err := os.ReadFile(filepath.Join(workspace, input.ResourceID))
		if err != nil {
			t.Fatal(err)
		}
		expected := sha256.Sum256(body)
		if input.Revision != hex.EncodeToString(expected[:]) {
			t.Fatal("依赖不是本次原始复制 bytes")
		}
		if input.ResourceID == "source.txt" && input.Revision != hex.EncodeToString(digest[:]) {
			t.Fatal("未封存非 operation target 的实际复制输入")
		}
	}
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	if err := workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(event magicevent.Event, result magicevent.Result) {
		query := event.Data().(workspaceevents.GetWorkspaceCommand)
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: query.WorkspaceID, DeliverableDirectory: workspace}}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	query := shellevents.QueryRuntimeValidationReadDependenciesCommand{
		RuntimeOperationExecutionContext: command.RuntimeOperationExecutionContext, Operation: command.SealedOperation.Operation,
		RequestID: "copy-current", IssuedAtStateRevision: 7, SourceReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(response.Receipt),
		OriginalDependencies: *runtimecontract.CloneRuntimeOwnerRevisionSet(dependencies), ApplicabilitySource: runtimecontract.CloneRuntimeEvidenceApplicabilityDependencies(response.Receipt.Evidence.ApplicabilityDependencies),
	}
	// Deliberately wrong caller directory: owner must obtain Workspace's directory.
	query.WorkspaceDeliverableDirectory = t.TempDir()
	sendQuery := func(c shellevents.QueryRuntimeValidationReadDependenciesCommand) (shellevents.QueryRuntimeValidationReadDependenciesResult, error) {
		data, err := hub.Send(magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeValidationReadDependencies, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, c)).Get()
		if err != nil {
			return shellevents.QueryRuntimeValidationReadDependenciesResult{}, err
		}
		value, ok := data.(shellevents.QueryRuntimeValidationReadDependenciesResult)
		if !ok {
			t.Fatalf("invalid dependency query response %T", data)
		}
		return value, nil
	}
	current, err := sendQuery(query)
	if err != nil || !reflect.DeepEqual(current.CurrentDependencies, dependencies) || current.Missing || commits != 1 {
		t.Fatalf("actual unchanged observation: %v %+v", err, current)
	}
	for _, kind := range []string{"receipt", "definition", "resource", "pair"} {
		invalid := query
		invalid.OriginalDependencies = *runtimecontract.CloneRuntimeOwnerRevisionSet(&query.OriginalDependencies)
		invalid.ApplicabilitySource = runtimecontract.CloneRuntimeEvidenceApplicabilityDependencies(query.ApplicabilitySource)
		switch kind {
		case "receipt":
			invalid.SourceReceiptHash = "wrong"
		case "definition":
			invalid.Transaction.DefinitionRevision = "wrong"
		case "resource":
			invalid.OriginalDependencies.Entries[0].ResourceID = "../outside"
			invalid.OriginalDependencies = invalid.OriginalDependencies.WithHash()
			invalid.ApplicabilitySource.OwnerRevisionSet = invalid.OriginalDependencies
		case "pair":
			invalid.ApplicabilitySource = nil
		}
		if _, err := sendQuery(invalid); err == nil || commits != 1 {
			t.Fatalf("invalid %s source reached successful read/write", kind)
		}
	}
	if err := os.Chmod(filepath.Join(workspace, "source.txt"), 0700); err != nil {
		t.Fatal(err)
	}
	modeChanged, err := sendQuery(query)
	if err != nil || modeChanged.CurrentDependencies.SetHash == dependencies.SetHash {
		t.Fatalf("mode observation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "source.txt"), []byte{0, 0xff, '\n'}, 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := sendQuery(query)
	if err != nil || changed.CurrentDependencies.SetHash == dependencies.SetHash {
		t.Fatalf("actual changed bytes: %v", err)
	}
	changedHash := sha256.Sum256([]byte{0, 0xff, '\n'})
	for _, entry := range changed.CurrentDependencies.Entries {
		if entry.ResourceID == "source.txt" && entry.Revision != hex.EncodeToString(changedHash[:]) {
			t.Fatal("query changed original bytes")
		}
	}
	if err := os.Remove(filepath.Join(workspace, "source.txt")); err != nil {
		t.Fatal(err)
	}
	missing, err := sendQuery(query)
	if err != nil || !missing.Missing || missing.CurrentDependencies != nil {
		t.Fatalf("missing observation: %v", err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "source.txt"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := sendQuery(query); err == nil {
		t.Fatal("special entry read failure became missing")
	}
	owner.BeginShutdown(ctx)
	if _, err := sendQuery(query); err == nil {
		t.Fatal("shutdown allowed fresh dependency query")
	}
	replay, err := send(command)
	if err != nil || commits != 1 || !reflect.DeepEqual(replay.Receipt, response.Receipt) {
		t.Fatalf("重放读取了 newer workspace: %v commits=%d", err, commits)
	}
	originalRevision := persisted.Evidence.OwnerRevisionSet.Entries[0].Revision
	response.Receipt.Evidence.OwnerRevisionSet.Entries[0].Revision = "caller-change"
	if persisted.Evidence.OwnerRevisionSet.Entries[0].Revision != originalRevision {
		t.Fatal("response 与原保存集合共享可变 entries")
	}
}
