package runtime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

const implementationWorkReceiptProcessDirectory = "WORKORCH_TEST_IMPLEMENTATION_RECEIPT_DIRECTORY"

// 只验证同 owner 原 aggregate 的独立进程重载；不替代公开 activation 或 Run owner 持久化验收。
type implementationWorkProcessRepository struct {
	path string
}

func (r *implementationWorkProcessRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	var aggregate RuntimeAggregate
	if err := json.Unmarshal(data, &aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Facts.WorkspaceID != workspaceID || aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, common.ErrConflict
	}
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (r *implementationWorkProcessRepository) Save(ctx context.Context, aggregate RuntimeAggregate, revision uint64) error {
	current, err := r.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		return err
	}
	if current.StateRevision != revision {
		return common.ErrConflict
	}
	if err := aggregate.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(aggregate)
	if err != nil {
		return err
	}
	if err := os.WriteFile(r.path+".pending", data, 0o600); err != nil {
		return err
	}
	return os.Rename(r.path+".pending", r.path)
}

func TestImplementationWorkUnknownLateReceiptProcess(t *testing.T) {
	directory := os.Getenv(implementationWorkReceiptProcessDirectory)
	if directory == "" {
		return // 父回归在真实 private claim 封存后显式启动本 helper。
	}
	data, err := os.ReadFile(filepath.Join(directory, "event.json"))
	if err != nil {
		t.Fatal(err)
	}
	var event RuntimeInternalEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	repository := &implementationWorkProcessRepository{path: filepath.Join(directory, "aggregate.json")}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	before, err := controller.Load(context.Background(), event.Scope.WorkspaceID, event.Scope.RunID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := controller.ApplyInternalEvent(context.Background(), event)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Aggregate.ExecutionBudget, before.ExecutionBudget) || !reflect.DeepEqual(result.Aggregate.ImplementationWork, before.ImplementationWork) || !reflect.DeepEqual(result.Aggregate.Facts.Evidence, before.Facts.Evidence) || !reflect.DeepEqual(result.Aggregate.Decision, before.Decision) {
		t.Fatal("独立进程迟到 receipt 改变了关闭权威、消费或 credit")
	}
}

func implementationWorkUnknownLateReceiptAcrossProcesses(t *testing.T, aggregate RuntimeAggregate, event RuntimeInternalEvent, alreadyCommitted ...bool) {
	t.Helper()
	directory := t.TempDir()
	aggregateData, err := json.Marshal(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	eventData, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []struct {
		name string
		data []byte
	}{
		{"aggregate.json", aggregateData},
		{"event.json", eventData},
	} {
		if err := os.WriteFile(filepath.Join(directory, file.name), file.data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository := &implementationWorkProcessRepository{path: filepath.Join(directory, "aggregate.json")}
	delta := 1
	if len(alreadyCommitted) > 0 && alreadyCommitted[0] {
		delta = 0
	}
	for invocation := 0; invocation < 2; invocation++ {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestImplementationWorkUnknownLateReceiptProcess$", "-test.count=1", "-test.timeout=15s")
		command.Env = []string{implementationWorkReceiptProcessDirectory + "=" + directory}
		output, err := command.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("独立进程 callback %d: %v\n%s", invocation, err, output)
		}
		persisted, err := repository.Load(context.Background(), event.Scope.WorkspaceID, event.Scope.RunID)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.StateRevision != aggregate.StateRevision+uint64(delta) || len(persisted.OperationReceipts) != len(aggregate.OperationReceipts)+delta {
			t.Fatalf("独立进程 callback %d 丢失或重复 CAS", invocation)
		}
		if !reflect.DeepEqual(persisted.ExecutionBudget, aggregate.ExecutionBudget) || !reflect.DeepEqual(persisted.ImplementationWork, aggregate.ImplementationWork) {
			t.Fatal("独立进程重载改变原 unknown 消费或关闭 round")
		}
	}
}
