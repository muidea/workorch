package workorchd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/magicCommon/event"
	plugininitiator "github.com/muidea/magicCommon/framework/plugin/initiator"
	"github.com/muidea/magicCommon/task"
	_ "github.com/muidea/workorch/internal/initiators/workorchruntime"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestModuleSetupReturnsConfigError(t *testing.T) {
	useWorkorchModuleTestConfig(t, "127.0.0.1:0", "memory", "", "not-a-duration")
	mod := New()

	if err := mod.Setup(context.Background(), event.NewHub(8), task.NewBackgroundRoutine(8)); err == nil {
		t.Fatal("expected setup error")
	}
}

func TestModuleRunRequiresSetup(t *testing.T) {
	mod := New()

	if err := mod.Run(context.Background()); err == nil {
		t.Fatal("expected run error")
	}
}

func TestModuleTeardownIsIdempotent(t *testing.T) {
	mod := New()
	mod.Teardown(context.Background())
	mod.Teardown(context.Background())
}

func TestDoneReturnsClosedChannelBeforeSetup(t *testing.T) {
	mod := New()

	select {
	case _, ok := <-mod.Done():
		if ok {
			t.Fatal("expected closed done channel")
		}
	default:
		t.Fatal("expected done channel to be closed before setup")
	}
}

func TestModuleSetupAndTeardownWithEphemeralAddress(t *testing.T) {
	useWorkorchModuleTestConfig(t, "127.0.0.1:0", "memory", "", "15s")
	ctx := context.Background()
	rawHub := event.NewHub(8)
	bg := task.NewBackgroundRoutine(8)
	setupWorkorchRuntimeInitiator(t, ctx, rawHub, bg)
	mod := New()

	if err := mod.Setup(ctx, rawHub, bg); err != nil {
		t.Fatalf("setup module: %v", err)
	}
	mod.Teardown(ctx)
}

func TestModuleDoneReportsServerCloseAsCleanShutdown(t *testing.T) {
	useWorkorchModuleTestConfig(t, "127.0.0.1:0", "memory", "", "15s")
	ctx := context.Background()
	rawHub := event.NewHub(8)
	bg := task.NewBackgroundRoutine(8)
	setupWorkorchRuntimeInitiator(t, ctx, rawHub, bg)
	mod := New()

	if err := mod.Setup(ctx, rawHub, bg); err != nil {
		t.Fatalf("setup module: %v", err)
	}
	if err := mod.Run(ctx); err != nil {
		t.Fatalf("run module: %v", err)
	}
	mod.Teardown(ctx)

	select {
	case err, ok := <-mod.Done():
		if ok && err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("done error = %v, want nil or context canceled", err)
		}
	default:
		t.Fatal("expected done channel to close after teardown")
	}
}

func TestModuleDoneCommandReturnsServerDoneChannel(t *testing.T) {
	useWorkorchModuleTestConfig(t, "127.0.0.1:0", "memory", "", "15s")
	ctx := context.Background()
	rawHub := event.NewHub(8)
	bg := task.NewBackgroundRoutine(8)
	setupWorkorchRuntimeInitiator(t, ctx, rawHub, bg)
	mod := New()

	if err := mod.Setup(ctx, rawHub, bg); err != nil {
		t.Fatalf("setup module: %v", err)
	}
	if err := mod.Run(ctx); err != nil {
		t.Fatalf("run module: %v", err)
	}
	defer mod.Teardown(ctx)

	hub := workorchevents.NewMagicHubWithHubOwner(rawHub, "workorchd/module-test")
	err := hub.Publish(context.Background(), workorchevents.Envelope{
		Topic:        workorchevents.TopicWorkorchDCommandDone,
		SourceModule: "test",
		SubjectType:  "workorchd",
		SubjectID:    "server",
	})
	response, ok := workorchevents.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("done command err = %v, want response", err)
	}
	done, ok := response.Payload[workorchevents.PayloadKeyDoneChannel].(<-chan error)
	if !ok || done == nil {
		t.Fatalf("done payload = %#v, want done channel", response.Payload)
	}
}

func setupWorkorchRuntimeInitiator(t *testing.T, ctx context.Context, rawHub event.Hub, bg task.BackgroundRoutine) {
	t.Helper()
	if err := plugininitiator.Setup(ctx, rawHub, bg); err != nil {
		t.Fatalf("setup runtime initiator: %v", err)
	}
	if err := plugininitiator.Run(ctx); err != nil {
		t.Fatalf("run runtime initiator: %v", err)
	}
	t.Cleanup(func() { plugininitiator.Teardown(ctx) })
}

func useWorkorchModuleTestConfig(t *testing.T, addr string, backend string, dataRoot string, readTimeout string) {
	t.Helper()

	_ = workorchconfig.Close()
	configDir := t.TempDir()
	content := fmt.Sprintf(`endpointName = "workorch"

[server]
addr = %q
readHeaderTimeout = "5s"
readTimeout = %q
writeTimeout = "180s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 1

[repository]
backend = %q
dataDir = %q

[llm]
timeout = "60s"
disabled = false
`, addr, readTimeout, backend, dataRoot)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CONFIG_PATH", configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
}
