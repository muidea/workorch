package appservice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func newTestRuntimeSession(t *testing.T, backend string, dataRoot string) (*RuntimeSession, context.Context) {
	t.Helper()

	ctx := context.Background()
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(ctx); err != nil {
			t.Errorf("close runtime session: %v", err)
		}
	})
	return session, ctx
}

func useWorkorchTestConfig(t *testing.T, backend string, dataRoot string) {
	t.Helper()

	_ = workorchconfig.Close()
	configDir := t.TempDir()
	content := fmt.Sprintf(`endpointName = "workorchd"

[server]
addr = ":8080"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "30s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 1

[repository]
backend = %q
dataDir = %q

[llm]
timeout = "60s"
disabled = false
`, backend, dataRoot)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CONFIG_PATH", configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
}
