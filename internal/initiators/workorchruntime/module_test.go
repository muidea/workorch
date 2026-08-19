package workorchruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestRuntimeInitiatorIdentityAndZeroStateContract(t *testing.T) {
	runtime := New()
	if runtime.ID() != common.InitiatorID {
		t.Fatalf("ID = %q, want %q", runtime.ID(), common.InitiatorID)
	}
	if runtime.Weight() != 10 {
		t.Fatalf("Weight = %d, want 10", runtime.Weight())
	}
	if runtime.RepositoryProvider() != nil {
		t.Fatal("repository provider is available before Setup")
	}
	if ((*RepositoryInitiator)(nil)).RepositoryProvider() != nil {
		t.Fatal("nil runtime RepositoryProvider is non-nil")
	}
}

func TestRuntimeInitiatorTeardownClearsRuntimeResources(t *testing.T) {
	repositories, err := buildRepositories("memory", "")
	if err != nil {
		t.Fatalf("build memory repositories: %v", err)
	}
	runtime := New()
	runtime.repositories = &repositories

	runtime.Teardown(context.Background())
	if runtime.RepositoryProvider() != nil {
		t.Fatal("repository provider remains available after Teardown")
	}
	if runtime.repositories != nil {
		t.Fatal("repository reference remains after Teardown")
	}

	runtime.Teardown(context.Background())
	if runtime.RepositoryProvider() != nil {
		t.Fatal("repository provider remains available after repeated Teardown")
	}
}

func TestRuntimeInitiatorMemoryRepositoriesCoverOwnerContracts(t *testing.T) {
	repositories, err := buildRepositories("memory", "")
	if err != nil {
		t.Fatalf("build memory repositories: %v", err)
	}
	assertRepositoryGroupsComplete(t, repositories)
}

func TestRuntimeInitiatorFileRepositoriesCoverOwnerContracts(t *testing.T) {
	repositories, err := buildRepositories("file", t.TempDir())
	if err != nil {
		t.Fatalf("build file repositories: %v", err)
	}
	assertRepositoryGroupsComplete(t, repositories)
}

func TestRuntimeInitiatorSetupPublishesCompleteProviderOnlyOnSuccess(t *testing.T) {
	closeRepositoryInitiatorConfig(t)
	configDir := writeRepositoryInitiatorConfig(t, "memory", "http://127.0.0.1:8080")
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() {
		if err := workorchconfig.Close(); err != nil {
			t.Errorf("close config manager: %v", err)
		}
	})

	runtime := New()
	if runtime.RepositoryProvider() != nil {
		t.Fatal("repository provider is available before Setup")
	}
	if setupErr := runtime.Setup(context.Background(), nil, nil); setupErr != nil {
		t.Fatalf("Setup: %v", setupErr)
	}
	provider := runtime.RepositoryProvider()
	if provider == nil {
		t.Fatal("repository provider is unavailable after successful Setup")
	}
	repositories, ok := provider.(*repositorySet)
	if !ok {
		t.Fatalf("repository provider type = %T, want *repositorySet", provider)
	}
	assertRepositoryGroupsComplete(t, *repositories)
}

func TestRuntimeInitiatorSetupFailureLeavesRepositoriesEmpty(t *testing.T) {
	tests := []struct {
		name    string
		backend string
		baseURL string
		wantErr string
	}{
		{
			name:    "invalid configuration",
			backend: "memory",
			baseURL: "ftp://127.0.0.1",
			wantErr: "must use http or https",
		},
		{
			name:    "unsupported backend",
			backend: "unsupported-backend",
			baseURL: "http://127.0.0.1:8080",
			wantErr: "unsupported repository backend",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			closeRepositoryInitiatorConfig(t)
			validConfigDir := writeRepositoryInitiatorConfig(t, "memory", "http://127.0.0.1:8080")
			t.Setenv(workorchconfig.EnvConfigDir, validConfigDir)
			runtime := New()
			if setupErr := runtime.Setup(context.Background(), nil, nil); setupErr != nil {
				t.Fatalf("initial Setup: %v", setupErr)
			}
			if runtime.RepositoryProvider() == nil {
				t.Fatal("initial Setup did not publish a repository provider")
			}

			closeRepositoryInitiatorConfig(t)
			configDir := writeRepositoryInitiatorConfig(t, tc.backend, tc.baseURL)
			t.Setenv(workorchconfig.EnvConfigDir, configDir)
			t.Cleanup(func() {
				if err := workorchconfig.Close(); err != nil {
					t.Errorf("close config manager: %v", err)
				}
			})

			setupErr := runtime.Setup(context.Background(), nil, nil)
			if setupErr == nil {
				t.Fatal("Setup returned nil error")
			}
			if !strings.Contains(setupErr.Error(), tc.wantErr) {
				t.Fatalf("Setup error = %q, want %q", setupErr.Error(), tc.wantErr)
			}
			if runtime.RepositoryProvider() != nil {
				t.Fatal("failed Setup left a repository provider available")
			}
			if runtime.repositories != nil {
				t.Fatal("failed Setup retained stale repository references")
			}
		})
	}
}

func TestRuntimeInitiatorRunIsIdempotent(t *testing.T) {
	repositories, err := buildRepositories("memory", "")
	if err != nil {
		t.Fatalf("build memory repositories: %v", err)
	}
	runtime := New()
	runtime.repositories = &repositories

	before := runtime.RepositoryProvider()
	for invocation := 1; invocation <= 2; invocation++ {
		if runErr := runtime.Run(context.Background()); runErr != nil {
			t.Fatalf("Run invocation %d: %v", invocation, runErr)
		}
	}
	if after := runtime.RepositoryProvider(); !reflect.DeepEqual(after, before) {
		t.Fatalf("repository provider changed across repeated Run calls\nbefore: %+v\nafter:  %+v", before, after)
	}
}

func assertRepositoryGroupsComplete(t *testing.T, repositories repositorySet) {
	t.Helper()

	requireNotNil(t, "workspace workspaces", repositories.Workspace().Workspaces)
	requireNotNil(t, "workspace goals", repositories.Workspace().Goals)
	requireNotNil(t, "workspace dods", repositories.Workspace().DoDs)
	requireNotNil(t, "workspace templates", repositories.Workspace().Templates)

	requireNotNil(t, "decision tickets", repositories.Decision().Tickets)
	requireNotNil(t, "decision resolutions", repositories.Decision().Resolutions)
	requireNotNil(t, "approval approvals", repositories.Approval().Approvals)
	requireNotNil(t, "eventstream records", repositories.EventStream().EventRecords)
	requireNotNil(t, "governance traces", repositories.Governance().AutoGovernanceTraces)
	requireNotNil(t, "mcp registry", repositories.MCP().Registry)
	requireNotNil(t, "memory items", repositories.Memory().MemoryItems)
	requireNotNil(t, "memory recall traces", repositories.Memory().MemoryRecallTraces)

	requireNotNil(t, "orchestrator tool calls", repositories.Orchestrator().ToolCalls)
	requireNotNil(t, "orchestrator patches", repositories.Orchestrator().Patches)
	requireNotNil(t, "orchestrator validation results", repositories.Orchestrator().ValidationResults)
	requireNotNil(t, "orchestrator evidence", repositories.Orchestrator().EvidenceRecords)
	requireNotNil(t, "orchestrator coverage", repositories.Orchestrator().CoverageWorkItems)
	requireNotNil(t, "orchestrator artifact continuations", repositories.Orchestrator().ArtifactContinuations)

	requireNotNil(t, "result results", repositories.Result().Results)
	requireNotNil(t, "result artifacts", repositories.Result().Artifacts)
	requireNotNil(t, "run runs", repositories.Run().Runs)
	requireNotNil(t, "run steps", repositories.Run().Steps)
	requireNotNil(t, "run checkpoints", repositories.Run().Checkpoints)
	requireNotNil(t, "skill skills", repositories.Skill().Skills)
	requireNotNil(t, "soul souls", repositories.Soul().Souls)
}

func TestRuntimeInitiatorRejectsUnsupportedRepositoryBackend(t *testing.T) {
	_, err := buildRepositories("unsupported-backend", "")
	if err == nil {
		t.Fatal("buildRepositories returned nil error for unsupported backend")
	}
	if !strings.Contains(err.Error(), "unsupported repository backend") {
		t.Fatalf("error = %q, want unsupported backend message", err.Error())
	}
}

func requireNotNil(t *testing.T, name string, value any) {
	t.Helper()
	if value == nil {
		t.Fatalf("%s repository is nil", name)
	}
}

func writeRepositoryInitiatorConfig(t *testing.T, backend string, baseURL string) string {
	t.Helper()
	configDir := t.TempDir()
	content := fmt.Sprintf(`endpointName = "workorch"

[repository]
backend = %q

[llm.aiProxy]
baseURL = %q
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "test-model"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
`, backend, baseURL)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write initiator test config: %v", err)
	}
	return configDir
}

func closeRepositoryInitiatorConfig(t *testing.T) {
	t.Helper()
	if err := workorchconfig.Close(); err != nil {
		t.Fatalf("close existing config manager: %v", err)
	}
}
