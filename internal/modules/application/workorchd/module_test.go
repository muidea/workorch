package workorchd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestModuleIdentityAndDoneCommandContract(t *testing.T) {
	mod := New()
	if mod.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", mod.ID(), ModuleID)
	}
	if mod.Weight() != 100 {
		t.Fatalf("Weight = %d, want 100", mod.Weight())
	}

	err := mod.handleDoneCommand(context.Background(), events.Envelope{
		Topic:       workorchdevents.TopicWorkorchDCommandDone,
		SubjectType: "workorchd",
		SubjectID:   "done",
		WorkspaceID: "ws-001",
		RunID:       "run-001",
		StepID:      "step-001",
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("handleDoneCommand error = %v, want response envelope", err)
	}
	if response.Topic != workorchdevents.TopicWorkorchDCommandDone+"/response" {
		t.Fatalf("response topic = %q", response.Topic)
	}
	if response.SourceModule != ModuleID {
		t.Fatalf("response source module = %q, want %q", response.SourceModule, ModuleID)
	}
	if response.SubjectType != "workorchd" ||
		response.SubjectID != "done" ||
		response.WorkspaceID != "ws-001" ||
		response.RunID != "run-001" ||
		response.StepID != "step-001" {
		t.Fatalf("response scope = %+v", response)
	}
	done, ok := response.Payload[events.PayloadKeyDoneChannel].(<-chan error)
	if !ok {
		t.Fatalf("done channel payload = %#v, want receive-only error channel", response.Payload[events.PayloadKeyDoneChannel])
	}
	select {
	case _, ok := <-done:
		if ok {
			t.Fatal("done channel is open before setup")
		}
	default:
		t.Fatal("done channel is not closed before setup")
	}
}

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
