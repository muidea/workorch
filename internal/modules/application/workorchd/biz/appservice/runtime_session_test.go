package appservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimeports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestBackendSessionUsesEventHubFromContext(t *testing.T) {
	hub := &countingActivationEventHub{base: events.NewMagicHubWithOwner("appservice/runtime-session-context-test")}
	ctx := ContextWithEventHub(context.Background(), hub)

	_, err := createWorkspaceWithIDWithBackend(ctx, "memory", "", "ws-context-hub", "context hub", "", "workspaces/context hub")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	if got := hub.Count(workspaceevents.TopicWorkspaceCommandCreate); got != 1 {
		t.Fatalf("workspace create command count = %d, want 1", got)
	}
	if got := hub.Count(workspaceevents.TopicWorkspaceCommandGet); got != 1 {
		t.Fatalf("workspace get command count = %d, want 1", got)
	}
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
	session := &RuntimeSession{}

	err := errOperation
	session.CloseOnReturn(context.Background(), &err)

	if !errors.Is(err, errOperation) {
		t.Fatalf("err = %v, want operation error", err)
	}
}

func TestRuntimeSessionExportsReturnsInvalidArgumentWhenBootstrapMissing(t *testing.T) {
	exports, err := (*RuntimeSession)(nil).Exports()
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
	if exports.Repositories.Workspaces != nil || exports.Infrastructure.EventHub != nil {
		t.Fatalf("exports = %#v, want zero exports", exports)
	}

	session := &RuntimeSession{}
	exports, err = session.Exports()
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
	if exports.Repositories.Workspaces != nil || exports.Infrastructure.EventHub != nil {
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
	if exports.Repositories.Workspaces == nil || exports.Infrastructure.EventHub == nil {
		t.Fatalf("exports missing assembled capabilities: %#v", exports)
	}
}

func TestRuntimeSessionFromExportsBorrowsLongLivedRuntime(t *testing.T) {
	bootstrapRuntime := newBootstrapWithBackend("memory", "")
	t.Cleanup(func() {
		if err := bootstrapRuntime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown bootstrap runtime: %v", err)
		}
	})

	session, err := NewRuntimeSessionFromExports(bootstrapRuntime.Exports())
	if err != nil {
		t.Fatalf("NewRuntimeSessionFromExports: %v", err)
	}
	if session.Bootstrap != nil || session.Runtime == nil {
		t.Fatalf("session = %+v, want non-owning managed runtime", session)
	}

	borrowed := session.Borrow()
	if borrowed.Bootstrap != nil || borrowed.Runtime == nil {
		t.Fatalf("borrowed session = %+v, want non-owning managed runtime", borrowed)
	}
	if err := borrowed.Close(context.Background()); err != nil {
		t.Fatalf("close borrowed session: %v", err)
	}

	exports := bootstrapRuntime.Exports()
	if err := runtimeports.NewEventHubActionPort(exports.Infrastructure.EventHub).EnsureWorkspace(context.Background(), "ws-long-lived-export-session", "long lived export session"); err != nil {
		t.Fatalf("create workspace after borrowed close: %v", err)
	}
	if _, err := exports.Repositories.Workspaces.Get(context.Background(), "ws-long-lived-export-session"); err != nil {
		t.Fatalf("get workspace after borrowed close: %v", err)
	}

	if err := session.Close(context.Background()); err != nil {
		t.Fatalf("close non-owning session: %v", err)
	}
	if _, err := exports.Repositories.Workspaces.Get(context.Background(), "ws-long-lived-export-session"); err != nil {
		t.Fatalf("non-owning session close should not shutdown bootstrap runtime: %v", err)
	}
}

func TestRuntimeSessionFromExportsRejectsIncompleteExports(t *testing.T) {
	if _, err := NewRuntimeSessionFromExports(bootstrap.Exports{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
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

func TestRuntimeSessionWithBackendPublishesOrchestratorCommand(t *testing.T) {
	session, err := NewRuntimeSessionWithBackend("file", t.TempDir())
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	t.Cleanup(func() { _ = session.Close(context.Background()) })
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	provider := exports.Infrastructure.EventHub.(interface{ RawHub() magicevent.Hub })
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandCancelRun, "workorchd/appservice/runtime-session-test", orchestratorevents.CommandDestination, magicevent.NewHeader(), context.Background(), orchestratorevents.CancelRunCommand{RunID: "run-missing"})
	response := provider.RawHub().Send(event)
	if response == nil || response.Error() == nil {
		t.Fatalf("cancel response = %#v, want owner error", response)
	}
}

func TestRuntimeSessionRunServiceBlockOnApprovalUsesEventCommand(t *testing.T) {
	session, err := NewRuntimeSessionWithBackend("file", t.TempDir())
	if err != nil {
		t.Fatalf("NewRuntimeSessionWithBackend: %v", err)
	}
	t.Cleanup(func() { _ = session.Close(context.Background()) })
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("Exports: %v", err)
	}
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-session-command")
	ensureTestWorkspace(t, ctx, session, "ws-session-command")
	actions := runtimeports.NewEventHubActionPort(exports.Infrastructure.EventHub)
	if err := actions.EnsureRun(ctx, "run-session-command", "ws-session-command"); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := actions.EnsureRunBlockedOnApproval(ctx, "run-session-command", "approval-session-command"); err != nil {
		t.Fatalf("BlockOnApproval: %v", err)
	}
}

func TestNewRuntimeSessionFromEnvReturnsInvalidArgumentWhenBackendUnsupported(t *testing.T) {
	useWorkorchTestConfig(t, "unsupported", t.TempDir())

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
	defaultDataDir := filepath.Join(tempRoot, "workorch-data")
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

	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	provider := exports.Infrastructure.EventHub.(interface{ RawHub() magicevent.Hub })
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandCreate, "workorchd/appservice/runtime-session-test", workspaceevents.CommandDestination, magicevent.NewHeader(), context.Background(), workspaceevents.CreateWorkspaceCommand{
		WorkspaceID:          "ws-runtime-session-default-file",
		Title:                "runtime session default file backend",
		DeliverableDirectory: "workspaces/ws-runtime-session-default-file",
		InternalDirectory:    filepath.Join(defaultDataDir, "internal", "workspaces", "ws-runtime-session-default-file"),
	})
	response := provider.RawHub().Send(event)
	if response == nil || response.Error() != nil {
		t.Fatalf("create workspace response = %#v", response)
	}

	if _, err := os.Stat(filepath.Join(defaultDataDir, "internal", "workspaces", "ws-runtime-session-default-file", "workspace.json")); err != nil {
		t.Fatalf("expected workspace persistence file in default data dir: %v", err)
	}
	if session.RepositoryBackend() != "file" || session.RepositoryDataDir() != defaultDataDir {
		t.Fatalf("repository config = %q/%q, want file/%q", session.RepositoryBackend(), session.RepositoryDataDir(), defaultDataDir)
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
