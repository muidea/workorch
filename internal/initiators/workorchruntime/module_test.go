package workorchruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
)

func TestRuntimeInitiatorIdentityAndZeroStateContract(t *testing.T) {
	runtime := New()
	if runtime.ID() != common.InitiatorID {
		t.Fatalf("ID = %q, want %q", runtime.ID(), common.InitiatorID)
	}
	if runtime.Weight() != 10 {
		t.Fatalf("Weight = %d, want 10", runtime.Weight())
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
	runtime.repositories = repositories

	runtime.Teardown(context.Background())
	teardownRepos := runtime.RepositoryProvider().(repositorySet)
	assertRepositorySetEmpty(t, teardownRepos)

	runtime.Teardown(context.Background())
}

func TestRuntimeInitiatorMemoryRepositoriesCoverModuleContracts(t *testing.T) {
	repositories, err := buildRepositories("memory", "")
	if err != nil {
		t.Fatalf("build memory repositories: %v", err)
	}

	requireNotNil(t, "workspace workspaces", repositories.Workspace().Workspaces)
	requireNotNil(t, "workspace goals", repositories.Workspace().Goals)
	requireNotNil(t, "workspace dods", repositories.Workspace().DoDs)
	requireNotNil(t, "workspace templates", repositories.Workspace().Templates)

	requireNotNil(t, "decision tickets", repositories.Decision().Tickets)
	requireNotNil(t, "decision resolutions", repositories.Decision().Resolutions)
	requireNotNil(t, "approval approvals", repositories.Approval().Approvals)
	requireNotNil(t, "eventstream records", repositories.EventStream().EventRecords)
	requireNotNil(t, "governance traces", repositories.Governance().AutoGovernanceTraces)
	requireNotNil(t, "mcp servers", repositories.MCP().Servers)
	requireNotNil(t, "mcp capabilities", repositories.MCP().Capabilities)
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

func assertRepositorySetEmpty(t *testing.T, repositories repositorySet) {
	t.Helper()
	if repositories.Souls != nil ||
		repositories.WorkspaceTemplates != nil ||
		repositories.MemoryItems != nil ||
		repositories.MemoryRecallTraces != nil ||
		repositories.AutoGovernanceTraces != nil ||
		repositories.Skills != nil ||
		repositories.MCPServers != nil ||
		repositories.MCPCapabilities != nil ||
		repositories.Results != nil ||
		repositories.Artifacts != nil ||
		repositories.EventRecords != nil ||
		repositories.Workspaces != nil ||
		repositories.Goals != nil ||
		repositories.DoDs != nil ||
		repositories.Runs != nil ||
		repositories.Steps != nil ||
		repositories.ToolCalls != nil ||
		repositories.Patches != nil ||
		repositories.ValidationResults != nil ||
		repositories.RuntimeCheckpoints != nil ||
		repositories.EvidenceRecords != nil ||
		repositories.CoverageWorkItems != nil ||
		repositories.ArtifactContinuations != nil ||
		repositories.Tickets != nil ||
		repositories.Resolutions != nil ||
		repositories.Approvals != nil {
		t.Fatalf("repository set after Teardown = %+v, want empty", repositories)
	}
}

func requireNotNil(t *testing.T, name string, value any) {
	t.Helper()
	if value == nil {
		t.Fatalf("%s repository is nil", name)
	}
}
