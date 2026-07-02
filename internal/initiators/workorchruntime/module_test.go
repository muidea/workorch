package workorchruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

func TestRuntimeInitiatorIdentityAndZeroStateContract(t *testing.T) {
	runtime := New()
	if runtime.ID() != common.InitiatorID {
		t.Fatalf("ID = %q, want %q", runtime.ID(), common.InitiatorID)
	}
	if runtime.Weight() != 10 {
		t.Fatalf("Weight = %d, want 10", runtime.Weight())
	}
	if runtime.EventHub() != nil {
		t.Fatal("new runtime EventHub is non-nil before Setup")
	}
	if got := runtime.RuntimePolicy(); got != (workorchruntimepolicy.RuntimePolicy{}) {
		t.Fatalf("new runtime policy = %+v, want zero policy before Setup", got)
	}

	if ((*Runtime)(nil)).RepositoryProvider() != nil {
		t.Fatal("nil runtime RepositoryProvider is non-nil")
	}
	if ((*Runtime)(nil)).EventHub() != nil {
		t.Fatal("nil runtime EventHub is non-nil")
	}
	if got := ((*Runtime)(nil)).RuntimePolicy(); got != (workorchruntimepolicy.RuntimePolicy{}) {
		t.Fatalf("nil runtime policy = %+v, want zero policy", got)
	}
}

func TestRuntimeInitiatorTeardownClearsRuntimeResources(t *testing.T) {
	repositories, err := buildRepositories("memory", "")
	if err != nil {
		t.Fatalf("build memory repositories: %v", err)
	}
	runtime := New()
	runtime.repositories = repositories
	runtime.runtimePolicy = workorchruntimepolicy.NormalizeRuntimePolicy(workorchruntimepolicy.RuntimePolicy{})

	runtime.Teardown(context.Background())
	if runtime.EventHub() != nil {
		t.Fatal("EventHub is non-nil after Teardown")
	}
	if got := runtime.RuntimePolicy(); got != (workorchruntimepolicy.RuntimePolicy{}) {
		t.Fatalf("runtime policy after Teardown = %+v, want zero policy", got)
	}
	teardownRepos := runtime.RepositoryProvider().(repositorySet)
	assertRepositorySetEmpty(t, teardownRepos)

	runtime.Teardown(context.Background())
	if runtime.EventHub() != nil {
		t.Fatal("EventHub is non-nil after repeated Teardown")
	}
}

func TestRuntimeInitiatorMemoryRepositoriesCoverModuleContracts(t *testing.T) {
	repositories, err := buildRepositories("memory", "")
	if err != nil {
		t.Fatalf("build memory repositories: %v", err)
	}

	requireNotNil(t, "system souls", repositories.System().Souls)
	requireNotNil(t, "system templates", repositories.System().Templates)
	requireNotNil(t, "system mcp servers", repositories.System().MCPServers)
	requireNotNil(t, "system mcp capabilities", repositories.System().MCPCapabilities)

	requireNotNil(t, "workspace workspaces", repositories.Workspace().Workspaces)
	requireNotNil(t, "workspace goals", repositories.Workspace().Goals)
	requireNotNil(t, "workspace dods", repositories.Workspace().DoDs)
	requireNotNil(t, "workspace templates", repositories.Workspace().Templates)

	requireNotNil(t, "decision tickets", repositories.Decision().Tickets)
	requireNotNil(t, "decision resolutions", repositories.Decision().Resolutions)
	requireNotNil(t, "decision evidence", repositories.Decision().EvidenceRecords)
	requireNotNil(t, "approval approvals", repositories.Approval().Approvals)
	requireNotNil(t, "approval evidence", repositories.Approval().EvidenceRecords)
	requireNotNil(t, "eventstream records", repositories.EventStream().EventRecords)
	requireNotNil(t, "governance traces", repositories.Governance().AutoGovernanceTraces)
	requireNotNil(t, "mcp servers", repositories.MCP().Servers)
	requireNotNil(t, "mcp capabilities", repositories.MCP().Capabilities)
	requireNotNil(t, "memory items", repositories.Memory().MemoryItems)
	requireNotNil(t, "memory recall traces", repositories.Memory().MemoryRecallTraces)

	requireNotNil(t, "orchestrator steps", repositories.Orchestrator().Steps)
	requireNotNil(t, "orchestrator tool calls", repositories.Orchestrator().ToolCalls)
	requireNotNil(t, "orchestrator patches", repositories.Orchestrator().Patches)
	requireNotNil(t, "orchestrator validation results", repositories.Orchestrator().ValidationResults)
	requireNotNil(t, "orchestrator checkpoints", repositories.Orchestrator().RuntimeCheckpoints)
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
