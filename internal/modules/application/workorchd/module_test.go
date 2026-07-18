package workorchd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestModuleIdentityAndStoppedNotificationContract(t *testing.T) {
	mod := New()
	if mod.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", mod.ID(), ModuleID)
	}
	if mod.Weight() != 100 {
		t.Fatalf("Weight = %d, want 100", mod.Weight())
	}

	data := workorchdevents.ServiceStoppedData{ServiceName: "workorch", State: "failed", ErrorCode: "server_failed", ErrorMessage: "bounded", StoppedAt: time.Now()}
	if data.ServiceName == "" || data.State == "" || data.ErrorCode == "" || data.StoppedAt.IsZero() {
		t.Fatalf("invalid stopped notification: %+v", data)
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
