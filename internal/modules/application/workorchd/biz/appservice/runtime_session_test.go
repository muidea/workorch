package appservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type runtimeSessionTestLifecycle struct {
	id          string
	teardownErr error
	teardownSeq *[]string
}

func (l runtimeSessionTestLifecycle) ID() string {
	if l.id == "" {
		return "test-lifecycle"
	}
	return l.id
}

func (runtimeSessionTestLifecycle) Run() error {
	return nil
}

func (l runtimeSessionTestLifecycle) Teardown(context.Context) error {
	if l.teardownSeq != nil {
		*l.teardownSeq = append(*l.teardownSeq, l.ID())
	}
	return l.teardownErr
}

func TestBackendSessionUsesEventHubFromContext(t *testing.T) {
	hub := events.NewMemoryHub()
	ctx := ContextWithEventHub(context.Background(), hub)

	_, err := CreateWorkspaceWithBackend(ctx, "memory", "", "ws-context-hub", "context hub")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic != events.TopicWorkspaceCreated {
		t.Fatalf("topic = %s, want %s", published[0].Topic, events.TopicWorkspaceCreated)
	}
}

func bootstrapModulesField(t *testing.T, runtime *bootstrap.Runtime) reflect.Value {
	t.Helper()
	if runtime == nil {
		t.Fatal("expected bootstrap runtime")
	}
	field := reflect.ValueOf(runtime).Elem().FieldByName("modules")
	if !field.IsValid() {
		t.Fatal("bootstrap runtime modules field missing")
	}
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
}

func setBootstrapRuntimeModules(t *testing.T, runtime *bootstrap.Runtime, modules ...runtimeSessionTestLifecycle) {
	t.Helper()

	field := bootstrapModulesField(t, runtime)
	slice := reflect.MakeSlice(field.Type(), len(modules), len(modules))
	for idx, module := range modules {
		slice.Index(idx).Set(reflect.ValueOf(module))
	}
	field.Set(slice)
}

func prependBootstrapRuntimeModule(t *testing.T, runtime *bootstrap.Runtime, module runtimeSessionTestLifecycle) {
	t.Helper()

	field := bootstrapModulesField(t, runtime)
	current := field
	slice := reflect.MakeSlice(field.Type(), current.Len()+1, current.Len()+1)
	slice.Index(0).Set(reflect.ValueOf(module))
	for idx := 0; idx < current.Len(); idx++ {
		slice.Index(idx + 1).Set(current.Index(idx))
	}
	field.Set(slice)
}

func TestRuntimeSessionCloseReleasesManagedAndBootstrapRuntime(t *testing.T) {
	session, err := NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	if session.Bootstrap == nil {
		t.Fatal("expected bootstrap runtime")
	}
	if session.Runtime == nil {
		t.Fatal("expected managed runtime")
	}

	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("close runtime session: %v", err)
	}
	if session.Bootstrap != nil {
		t.Fatal("expected bootstrap runtime to be released")
	}
	if session.Runtime != nil {
		t.Fatal("expected managed runtime to be released")
	}

	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("second close runtime session: %v", err)
	}
}

func TestBorrowedRuntimeSessionCloseKeepsBootstrapRuntime(t *testing.T) {
	owner, err := NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("close owner runtime session: %v", err)
		}
	})

	borrowed := owner.Borrow()
	if borrowed.Bootstrap == nil {
		t.Fatal("expected borrowed bootstrap runtime")
	}
	if borrowed.Runtime == nil {
		t.Fatal("expected borrowed managed runtime")
	}

	if err := borrowed.Close(context.Background()); err != nil {
		t.Fatalf("close borrowed runtime session: %v", err)
	}
	if borrowed.Bootstrap != nil {
		t.Fatal("expected borrowed bootstrap reference to be released")
	}
	if owner.Bootstrap == nil {
		t.Fatal("expected owner bootstrap runtime to stay available")
	}
}

func TestRuntimeSessionCloseOnReturnJoinsOperationAndShutdownErrors(t *testing.T) {
	errOperation := errors.New("operation failed")
	errShutdown := errors.New("shutdown failed")
	session := &RuntimeSession{
		Bootstrap: &bootstrap.Runtime{},
		Runtime:   &ManagedRuntime{},
	}
	setBootstrapRuntimeModules(t, session.Bootstrap, runtimeSessionTestLifecycle{
		id:          "broken",
		teardownErr: errShutdown,
	})

	err := errOperation
	session.CloseOnReturn(context.Background(), &err)

	if !errors.Is(err, errOperation) {
		t.Fatalf("err = %v, want operation error", err)
	}
	if !errors.Is(err, errShutdown) {
		t.Fatalf("err = %v, want shutdown error", err)
	}
}

func TestRuntimeSessionExportsReturnsInvalidArgumentWhenBootstrapMissing(t *testing.T) {
	exports, err := (*RuntimeSession)(nil).Exports()
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
	if exports.Services.WorkspaceService != nil {
		t.Fatalf("exports = %#v, want zero exports", exports)
	}

	session := &RuntimeSession{}
	exports, err = session.Exports()
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
	if exports.Services.WorkspaceService != nil {
		t.Fatalf("exports = %#v, want zero exports", exports)
	}
}

func TestRuntimeSessionExportsReturnsAssembledCapabilities(t *testing.T) {
	session, err := NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(context.Background()); err != nil {
			t.Errorf("close runtime session: %v", err)
		}
	})

	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	if exports.Services.WorkspaceService == nil || exports.Repositories.Workspaces == nil || exports.Infrastructure.EventHub == nil {
		t.Fatalf("exports missing assembled capabilities: %#v", exports)
	}
}

func TestRuntimeSessionCloseReturnsShutdownErrorAndReleasesState(t *testing.T) {
	errShutdown := errors.New("shutdown failed")
	session := &RuntimeSession{
		Bootstrap: &bootstrap.Runtime{},
		Runtime:   &ManagedRuntime{},
	}
	setBootstrapRuntimeModules(t, session.Bootstrap, runtimeSessionTestLifecycle{
		id:          "broken",
		teardownErr: errShutdown,
	})

	err := session.Close(context.Background())
	if !errors.Is(err, errShutdown) {
		t.Fatalf("err = %v, want shutdown error", err)
	}
	if session.Bootstrap != nil {
		t.Fatal("expected bootstrap runtime to be released after shutdown error")
	}
	if session.Runtime != nil {
		t.Fatal("expected managed runtime to be released after shutdown error")
	}
}

func TestNewRuntimeFromBootstrapReturnsNilWhenBootstrapMissing(t *testing.T) {
	if runtime := NewRuntimeFromBootstrap(nil); runtime != nil {
		t.Fatalf("runtime = %#v, want nil", runtime)
	}
}

func TestNewRuntimeFromExportsReturnsNilWhenRequiredExportsMissing(t *testing.T) {
	if runtime := NewRuntimeFromExports(bootstrap.Exports{}); runtime != nil {
		t.Fatalf("runtime = %#v, want nil", runtime)
	}
}

func TestNewRuntimeSessionWithBackendReturnsInvalidArgumentWhenBackendUnsupported(t *testing.T) {
	session, err := NewRuntimeSessionWithBackend("unsupported", "")
	if session != nil {
		t.Fatalf("session = %#v, want nil", session)
	}
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestNewRuntimeSessionFromEnvReturnsInvalidArgumentWhenBackendUnsupported(t *testing.T) {
	t.Setenv("WORKORCH_REPOSITORY_BACKEND", "unsupported")
	t.Setenv("WORKORCH_DATA_DIR", t.TempDir())

	session, err := NewRuntimeSessionFromEnv()
	if session != nil {
		t.Fatalf("session = %#v, want nil", session)
	}
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestNewRuntimeSessionWithBackendFileUsesDefaultDataRootWhenUnset(t *testing.T) {
	tempRoot := t.TempDir()
	defaultDataDir := filepath.Join(tempRoot, "workorchd-data")
	t.Setenv("TMPDIR", tempRoot)

	session, err := NewRuntimeSessionWithBackend("file", "")
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(context.Background()); err != nil {
			t.Errorf("close runtime session: %v", err)
		}
	})

	if err := session.Bootstrap.Exports().Services.WorkspaceService.CreateWorkspace(context.Background(), workspacemodule.CreateWorkspaceCommand{
		WorkspaceID: common.WorkspaceID("ws-runtime-session-default-file"),
		Title:       "runtime session default file backend",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	if _, err := os.Stat(filepath.Join(defaultDataDir, "workspace.json")); err != nil {
		t.Fatalf("expected workspace persistence file in default data dir: %v", err)
	}
}

func TestBuildBootstrapRuntimeRepanicsForNonConfigPanic(t *testing.T) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
		if recovered != "boom" {
			t.Fatalf("recovered = %v, want boom", recovered)
		}
	}()

	_, _ = buildBootstrapRuntime(func() *bootstrap.Runtime {
		panic("boom")
	})
}
