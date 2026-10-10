package workorchruntime

import (
	"context"
	"errors"
	"fmt"
	plugincommon "github.com/muidea/magicCommon/framework/plugin/common"
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

func TestRuntimeInitiatorSetupAfterTeardownPublishesFreshProvider(t *testing.T) {
	closeRepositoryInitiatorConfig(t)
	configDir := writeRepositoryInitiatorConfig(t, "memory", "http://127.0.0.1:8080")
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() {
		if err := workorchconfig.Close(); err != nil {
			t.Errorf("close config manager: %v", err)
		}
	})

	runtime := New()
	if setupErr := runtime.Setup(context.Background(), nil, nil); setupErr != nil {
		t.Fatalf("first Setup: %v", setupErr)
	}
	first := runtime.repositories
	if first == nil {
		t.Fatal("first Setup did not publish repositories")
	}

	runtime.Teardown(context.Background())
	if runtime.RepositoryProvider() != nil || runtime.repositories != nil {
		t.Fatal("Teardown did not revoke the first provider")
	}

	if setupErr := runtime.Setup(context.Background(), nil, nil); setupErr != nil {
		t.Fatalf("second Setup: %v", setupErr)
	}
	second := runtime.repositories
	if second == nil {
		t.Fatal("second Setup did not publish repositories")
	}
	if second == first {
		t.Fatal("Setup after Teardown reused the revoked repository set")
	}
	assertRepositoryGroupsComplete(t, *second)
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

			originalProvider := runtime.RepositoryProvider()
			if err := runtime.Setup(context.Background(), nil, nil); err == nil || runtime.RepositoryProvider() != originalProvider {
				t.Fatal("repeated Setup revoked a live provider")
			}
			runtime.Teardown(context.Background())
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

func TestRuntimeInitiatorStagesRetainProviderUntilCheckedRelease(t *testing.T) {
	repositories, err := buildRepositories("memory", "")
	if err != nil {
		t.Fatal(err)
	}
	runtime := New()
	runtime.repositories = &repositories
	provider := runtime.RepositoryProvider()
	manager := plugincommon.NewPluginMgr("repository-stages-test")
	if err := manager.Register(runtime); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.BeginShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runtime.RepositoryProvider() != provider {
		t.Fatal("BeginShutdown released provider")
	}
	if err := manager.TeardownChecked(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled teardown=%v", err)
	}
	if runtime.RepositoryProvider() != provider {
		t.Fatal("failed shutdown released provider")
	}
	if err := runtime.Setup(context.Background(), nil, nil); err == nil || runtime.RepositoryProvider() != provider {
		t.Fatal("closing Setup replaced provider")
	}
	if err := manager.TeardownChecked(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runtime.RepositoryProvider() != nil {
		t.Fatal("checked release retained provider")
	}
	if err := manager.TeardownChecked(context.Background()); err != nil {
		t.Fatal(err)
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

	workspaceOwner := repositories.Workspace().Owner
	if err := workspaceOwner.Validate(); err != nil {
		t.Fatalf("workspace owner repository group: %v", err)
	}
	requireNotNil(t, "workspace workspaces", workspaceOwner.Workspaces())
	requireNotNil(t, "workspace deletion material cleaner", workspaceOwner.DeletionMaterialCleaner())
	requireNotNil(t, "workspace goals", workspaceOwner.Goals())
	requireNotNil(t, "workspace dods", workspaceOwner.DoDs())
	requireNotNil(t, "workspace templates", workspaceOwner.Templates())

	decisionOwner := repositories.Decision().Owner
	if err := decisionOwner.Validate(); err != nil {
		t.Fatalf("decision owner repository group: %v", err)
	}
	requireNotNil(t, "decision tickets", decisionOwner.Tickets())
	requireNotNil(t, "decision resolutions", decisionOwner.Resolutions())
	requireNotNil(t, "decision workspace deletions", decisionOwner.WorkspaceDeletions())

	approvalOwner := repositories.Approval().Owner
	if err := approvalOwner.Validate(); err != nil {
		t.Fatalf("approval owner repository group: %v", err)
	}
	requireNotNil(t, "approval approvals", approvalOwner.Approvals())
	requireNotNil(t, "approval workspace deletions", approvalOwner.WorkspaceDeletions())

	eventStreamOwner := repositories.EventStream().Owner
	if err := eventStreamOwner.Validate(); err != nil {
		t.Fatalf("eventstream owner repository group: %v", err)
	}
	requireNotNil(t, "eventstream records", eventStreamOwner.EventRecords())
	requireNotNil(t, "eventstream workspace deletions", eventStreamOwner.WorkspaceDeletions())
	requireNotNil(t, "eventstream workspace purger", eventStreamOwner.WorkspacePurger())
	requireNotNil(t, "governance traces", repositories.Governance().AutoGovernanceTraces)
	requireNotNil(t, "mcp registry", repositories.MCP().Registry)

	memoryOwner := repositories.Memory().Owner
	if err := memoryOwner.Validate(); err != nil {
		t.Fatalf("memory owner repository group: %v", err)
	}
	requireNotNil(t, "memory items", memoryOwner.Items())
	requireNotNil(t, "memory recall traces", memoryOwner.RecallTraces())
	requireNotNil(t, "memory workspace deletions", memoryOwner.WorkspaceDeletions())

	orchestratorOwner := repositories.Orchestrator()
	if err := orchestratorOwner.Validate(); err != nil {
		t.Fatalf("orchestrator owner repository group: %v", err)
	}
	requireNotNil(t, "orchestrator tool calls", orchestratorOwner.ToolCalls())
	requireNotNil(t, "orchestrator patches", orchestratorOwner.Patches())
	requireNotNil(t, "orchestrator validation results", orchestratorOwner.ValidationResults())
	requireNotNil(t, "orchestrator evidence", orchestratorOwner.EvidenceRecords())
	requireNotNil(t, "orchestrator coverage", orchestratorOwner.CoverageWorkItems())
	requireNotNil(t, "orchestrator artifact continuations", orchestratorOwner.ArtifactContinuations())
	requireNotNil(t, "orchestrator workspace deletions", orchestratorOwner.WorkspaceDeletions())
	requireNotNil(t, "orchestrator workspace purger", orchestratorOwner.WorkspacePurger())

	resultOwner := repositories.Result().Owner
	if err := resultOwner.Validate(); err != nil {
		t.Fatalf("result owner repository group: %v", err)
	}
	requireNotNil(t, "result results", resultOwner.Results())
	requireNotNil(t, "result artifacts", resultOwner.Artifacts())
	requireNotNil(t, "result workspace deletions", resultOwner.WorkspaceDeletions())
	requireNotNil(t, "run runs", repositories.Run().Runs)
	requireNotNil(t, "run steps", repositories.Run().Steps)
	requireNotNil(t, "run checkpoints", repositories.Run().Checkpoints)
	if err := repositories.Run().Owner.Validate(); err != nil {
		t.Fatalf("run owner repository group: %v", err)
	}
	requireNotNil(t, "run owner runs", repositories.Run().Owner.Runs())
	requireNotNil(t, "run owner steps", repositories.Run().Owner.Steps())
	requireNotNil(t, "run owner checkpoints", repositories.Run().Owner.Checkpoints())
	requireNotNil(t, "run workspace purger", repositories.Run().Owner.WorkspacePurger())
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
