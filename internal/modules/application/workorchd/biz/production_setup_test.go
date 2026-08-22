package biz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestProductionSetupUsesTypedApplicationPorts(t *testing.T) {
	configDir := t.TempDir()
	config := fmt.Sprintf(`endpointName = "workorch"

[server]
addr = "127.0.0.1:0"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "30s"
idleTimeout = "30s"
shutdownTimeout = "5s"

[repository]
backend = "memory"
dataDir = %q

[llm.aiProxy]
baseURL = "http://127.0.0.1:9"
apiKey = "test-key"
timeout = "5s"
catalogRequestTimeout = "1s"
catalogTTL = "1m"
catalogMaxStaleAge = "1h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 100

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "model-1"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
`, filepath.Join(configDir, "data"))
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv("CONFIG_PATH", configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	hub := magicevent.NewHub(16)
	background := task.NewBackgroundRoutine(16)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t.Cleanup(func() {
		hub.Terminate(ctx)
		background.Shutdown(ctx)
	})

	owner := New()
	if err := owner.Setup(ctx, hub, background); err != nil {
		t.Fatalf("production setup: %v", err)
	}
	owner.Stop(ctx)
	owner.Stop(ctx)
	if owner.execution != nil || owner.service != nil || owner.projection != nil || owner.runtime != nil || owner.workspaceGate != nil || owner.cliDispatcher != nil {
		t.Fatalf("duplicate Stop retained application lifecycle state: %+v", owner)
	}
}
