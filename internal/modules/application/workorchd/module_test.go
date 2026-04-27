package workorchd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestModuleSetupReturnsConfigError(t *testing.T) {
	useWorkorchModuleTestConfig(t, "127.0.0.1:0", "memory", "", "not-a-duration")
	mod := New()

	if err := mod.Setup(event.NewHub(8), task.NewBackgroundRoutine(8)); err == nil {
		t.Fatal("expected setup error")
	}
}

func TestModuleRunRequiresSetup(t *testing.T) {
	mod := New()

	if err := mod.Run(); err == nil {
		t.Fatal("expected run error")
	}
}

func TestModuleTeardownIsIdempotent(t *testing.T) {
	mod := New()
	mod.Teardown()
	mod.Teardown()
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
	mod := New()

	if err := mod.Setup(event.NewHub(8), task.NewBackgroundRoutine(8)); err != nil {
		t.Fatalf("setup module: %v", err)
	}
	mod.Teardown()
}

func TestModuleDoneReportsServerCloseAsCleanShutdown(t *testing.T) {
	useWorkorchModuleTestConfig(t, "127.0.0.1:0", "memory", "", "15s")
	mod := New()

	if err := mod.Setup(event.NewHub(8), task.NewBackgroundRoutine(8)); err != nil {
		t.Fatalf("setup module: %v", err)
	}
	if err := mod.Run(); err != nil {
		t.Fatalf("run module: %v", err)
	}
	mod.Teardown()

	select {
	case err, ok := <-mod.Done():
		if ok && err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("done error = %v, want nil or context canceled", err)
		}
	default:
		t.Fatal("expected done channel to close after teardown")
	}
}

func useWorkorchModuleTestConfig(t *testing.T, addr string, backend string, dataRoot string, readTimeout string) {
	t.Helper()

	_ = workorchconfig.Close()
	configDir := t.TempDir()
	content := fmt.Sprintf(`endpointName = "workorchd"

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
