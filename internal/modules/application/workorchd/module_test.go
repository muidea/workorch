package workorchd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
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

	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDCommandDone, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), workorchdevents.DoneCommand{})
	response := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	mod.handleDoneCommand(event, response)
	if err := response.Error(); err != nil {
		t.Fatalf("handleDoneCommand error = %v", err)
	}
	data, err := response.Get()
	if err != nil {
		t.Fatalf("handleDoneCommand result: %v", err)
	}
	result, ok := data.(workorchdevents.DoneCommandResult)
	if !ok {
		t.Fatalf("done result = %T, want DoneCommandResult", data)
	}
	done := result.Done
	if done == nil {
		t.Fatal("done channel is nil")
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

	if err := mod.Setup(context.Background(), magicevent.NewHub(8), task.NewBackgroundRoutine(8)); err == nil {
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


[llm.aiProxy]
baseURL = "http://127.0.0.1:9"
apiKey = "test-key"
timeout = "60s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "custom-test"
modelID = "model-1"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
defaultMaxOutputTokens = 4096
maxRequestBytes = 4194304

[llm.customProviders.custom-test]
provider = "compatible"
apiSurface = "openai_compatible_http"
baseURL = "http://127.0.0.1:9"
apiKey = "test-key"
timeout = "30s"

[llm.customProviders.custom-test.models.model-1]
contextWindowTokens = 128000
maxOutputTokens = 8192
operations = ["chat_completions"]

`, addr, readTimeout, backend, dataRoot)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CONFIG_PATH", configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
}
