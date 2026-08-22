package workorchconfig

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// This file is the only repository fixture allowed to name the removed
// application scheduler/watchdog keys. It proves that stale deployment
// configuration fails closed instead of being accepted as a no-op control
// surface.
func TestCurrentRejectsRetiredServerSchedulerFields(t *testing.T) {
	for name, retiredField := range map[string]string{
		"application run limit":       "maxConcurrentRuns = 2",
		"stale dispatch watchdog age": `staleDispatchedStepMinAge = "2m"`,
	} {
		t.Run(name, func(t *testing.T) {
			configDir := writeApplicationConfig(t, fmt.Sprintf(`
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"
%s

[repository]
backend = "file"
dataDir = ".workorch-data"

[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "gpt-4o"
`, retiredField))
			_ = Close()
			t.Setenv(EnvConfigDir, configDir)
			t.Cleanup(func() { _ = Close() })

			_, err := Current()
			if err == nil {
				t.Fatal("Current error = nil, want retired server field rejection")
			}
			if !errors.Is(err, common.ErrInvalidArgument) && !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("Current error = %v, want invalid legacy/unknown server field", err)
			}
		})
	}
}

func TestPersistedRejectsRetiredServerSchedulerFields(t *testing.T) {
	for name, retiredField := range map[string]string{
		"application run limit":       "maxConcurrentRuns = 2",
		"stale dispatch watchdog age": `staleDispatchedStepMinAge = "2m"`,
	} {
		t.Run(name, func(t *testing.T) {
			configDir := writeApplicationConfig(t, fmt.Sprintf(`
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"
%s

[repository]
backend = "file"
dataDir = ".workorch-data"

[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "gpt-4o"
`, retiredField))
			_ = Close()
			t.Setenv(EnvConfigDir, configDir)
			t.Cleanup(func() { _ = Close() })

			_, err := Persisted()
			if err == nil {
				t.Fatal("Persisted error = nil, want retired server field rejection")
			}
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Persisted error = %v, want invalid argument", err)
			}
			fieldName := strings.Fields(retiredField)[0]
			if !strings.Contains(err.Error(), fieldName) {
				t.Fatalf("Persisted error = %v, want rejected field %q", err, fieldName)
			}
		})
	}
}
