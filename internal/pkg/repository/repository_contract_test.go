package repository_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestWorkspaceRepositoryContract_Memory(t *testing.T) {
	testWorkspaceRepositoryContract(t, func() repository.WorkspaceRepository {
		return memory.NewWorkspaceRepository()
	})
}

func TestSoulRepositoryContract_Memory(t *testing.T) {
	testSoulRepositoryContract(t, func() repository.SoulRepository {
		return memory.NewSoulRepository()
	})
}

func TestSoulRepositoryContract_File(t *testing.T) {
	testSoulRepositoryContract(t, func() repository.SoulRepository {
		return file.NewSoulRepository(filepath.Join(t.TempDir(), "soul.json"))
	})
}

func TestMemoryItemRepositoryContract_Memory(t *testing.T) {
	testMemoryItemRepositoryContract(t, func() repository.MemoryItemRepository {
		return memory.NewMemoryItemRepository()
	})
}

func TestMemoryItemRepositoryContract_File(t *testing.T) {
	testMemoryItemRepositoryContract(t, func() repository.MemoryItemRepository {
		return file.NewMemoryItemRepository(filepath.Join(t.TempDir(), "memory_item.json"))
	})
}

func TestSkillRepositoryContract_Memory(t *testing.T) {
	testSkillRepositoryContract(t, func() repository.SkillRepository {
		return memory.NewSkillRepository()
	})
}

func TestSkillRepositoryContract_File(t *testing.T) {
	testSkillRepositoryContract(t, func() repository.SkillRepository {
		return file.NewSkillRepository(filepath.Join(t.TempDir(), "skill.json"))
	})
}

func TestMCPServerRepositoryContract_Memory(t *testing.T) {
	testMCPServerRepositoryContract(t, func() repository.MCPServerRepository {
		return memory.NewMCPServerRepository()
	})
}

func TestMCPServerRepositoryContract_File(t *testing.T) {
	testMCPServerRepositoryContract(t, func() repository.MCPServerRepository {
		return file.NewMCPServerRepository(filepath.Join(t.TempDir(), "mcp_server.json"))
	})
}

func TestWorkspaceRepositoryContract_File(t *testing.T) {
	testWorkspaceRepositoryContract(t, func() repository.WorkspaceRepository {
		return file.NewWorkspaceRepository(filepath.Join(t.TempDir(), "workspace.json"))
	})
}

func TestGoalRepositoryContract_Memory(t *testing.T) {
	testGoalRepositoryContract(t, func() repository.GoalRepository {
		return memory.NewGoalRepository()
	})
}

func TestGoalRepositoryContract_File(t *testing.T) {
	testGoalRepositoryContract(t, func() repository.GoalRepository {
		return file.NewGoalRepository(filepath.Join(t.TempDir(), "goal.json"))
	})
}

func TestDoDRepositoryContract_Memory(t *testing.T) {
	testDoDRepositoryContract(t, func() repository.DoDRepository {
		return memory.NewDoDRepository()
	})
}

func TestDoDRepositoryContract_File(t *testing.T) {
	testDoDRepositoryContract(t, func() repository.DoDRepository {
		return file.NewDoDRepository(filepath.Join(t.TempDir(), "dod.json"))
	})
}

func TestRunRepositoryContract_Memory(t *testing.T) {
	testRunRepositoryContract(t, func() repository.RunRepository {
		return memory.NewRunRepository()
	})
}

func TestRunRepositoryContract_File(t *testing.T) {
	testRunRepositoryContract(t, func() repository.RunRepository {
		return file.NewRunRepository(filepath.Join(t.TempDir(), "run.json"))
	})
}

func TestStepRepositoryContract_Memory(t *testing.T) {
	testStepRepositoryContract(t, func() repository.StepRepository {
		return memory.NewStepRepository()
	})
}

func TestStepRepositoryContract_File(t *testing.T) {
	testStepRepositoryContract(t, func() repository.StepRepository {
		return file.NewStepRepository(filepath.Join(t.TempDir(), "step.json"))
	})
}

func TestToolCallRepositoryContract_Memory(t *testing.T) {
	testToolCallRepositoryContract(t, func() repository.ToolCallRepository {
		return memory.NewToolCallRepository()
	})
}

func TestToolCallRepositoryContract_File(t *testing.T) {
	testToolCallRepositoryContract(t, func() repository.ToolCallRepository {
		return file.NewToolCallRepository(filepath.Join(t.TempDir(), "tool_call.json"))
	})
}

func TestPatchRepositoryContract_Memory(t *testing.T) {
	testPatchRepositoryContract(t, func() repository.PatchRepository {
		return memory.NewPatchRepository()
	})
}

func TestPatchRepositoryContract_File(t *testing.T) {
	testPatchRepositoryContract(t, func() repository.PatchRepository {
		return file.NewPatchRepository(filepath.Join(t.TempDir(), "patch.json"))
	})
}

func TestValidationResultRepositoryContract_Memory(t *testing.T) {
	testValidationResultRepositoryContract(t, func() repository.ValidationResultRepository {
		return memory.NewValidationResultRepository()
	})
}

func TestValidationResultRepositoryContract_File(t *testing.T) {
	testValidationResultRepositoryContract(t, func() repository.ValidationResultRepository {
		return file.NewValidationResultRepository(filepath.Join(t.TempDir(), "validation_result.json"))
	})
}

func TestRuntimeCheckpointRepositoryContract_Memory(t *testing.T) {
	testRuntimeCheckpointRepositoryContract(t, func() repository.RuntimeCheckpointRepository {
		return memory.NewRuntimeCheckpointRepository()
	})
}

func TestRuntimeCheckpointRepositoryContract_File(t *testing.T) {
	testRuntimeCheckpointRepositoryContract(t, func() repository.RuntimeCheckpointRepository {
		return file.NewRuntimeCheckpointRepository(filepath.Join(t.TempDir(), "runtime_checkpoint.json"))
	})
}

func TestDecisionTicketRepositoryContract_Memory(t *testing.T) {
	testDecisionTicketRepositoryContract(t, func() repository.DecisionTicketRepository {
		return memory.NewDecisionTicketRepository()
	})
}

func TestDecisionTicketRepositoryContract_File(t *testing.T) {
	testDecisionTicketRepositoryContract(t, func() repository.DecisionTicketRepository {
		return file.NewDecisionTicketRepository(filepath.Join(t.TempDir(), "ticket.json"))
	})
}

func TestDecisionResolutionRepositoryContract_Memory(t *testing.T) {
	testDecisionResolutionRepositoryContract(t, func() repository.DecisionResolutionRepository {
		return memory.NewDecisionResolutionRepository()
	})
}

func TestDecisionResolutionRepositoryContract_File(t *testing.T) {
	testDecisionResolutionRepositoryContract(t, func() repository.DecisionResolutionRepository {
		return file.NewDecisionResolutionRepository(filepath.Join(t.TempDir(), "resolution.json"))
	})
}

func TestApprovalRepositoryContract_Memory(t *testing.T) {
	testApprovalRepositoryContract(t, func() repository.ApprovalRepository {
		return memory.NewApprovalRepository()
	})
}

func TestApprovalRepositoryContract_File(t *testing.T) {
	testApprovalRepositoryContract(t, func() repository.ApprovalRepository {
		return file.NewApprovalRepository(filepath.Join(t.TempDir(), "approval.json"))
	})
}

func TestRepositoryBundleContract_Memory(t *testing.T) {
	bundle := repositoryBundle{
		souls:              memory.NewSoulRepository(),
		memoryItems:        memory.NewMemoryItemRepository(),
		skills:             memory.NewSkillRepository(),
		mcpServers:         memory.NewMCPServerRepository(),
		workspaces:         memory.NewWorkspaceRepository(),
		goals:              memory.NewGoalRepository(),
		dods:               memory.NewDoDRepository(),
		runs:               memory.NewRunRepository(),
		steps:              memory.NewStepRepository(),
		toolCalls:          memory.NewToolCallRepository(),
		patches:            memory.NewPatchRepository(),
		validationResults:  memory.NewValidationResultRepository(),
		runtimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		tickets:            memory.NewDecisionTicketRepository(),
		resolutions:        memory.NewDecisionResolutionRepository(),
		approvals:          memory.NewApprovalRepository(),
	}
	testRepositoryBundleContract(t, bundle, func() repositoryBundle { return bundle })
}

func TestRepositoryBundleContract_File(t *testing.T) {
	dir := t.TempDir()
	build := func() repositoryBundle {
		return repositoryBundle{
			souls:              file.NewSoulRepository(filepath.Join(dir, "soul.json")),
			memoryItems:        file.NewMemoryItemRepository(filepath.Join(dir, "memory_item.json")),
			skills:             file.NewSkillRepository(filepath.Join(dir, "skill.json")),
			mcpServers:         file.NewMCPServerRepository(filepath.Join(dir, "mcp_server.json")),
			workspaces:         file.NewWorkspaceRepository(filepath.Join(dir, "workspace.json")),
			goals:              file.NewGoalRepository(filepath.Join(dir, "goal.json")),
			dods:               file.NewDoDRepository(filepath.Join(dir, "dod.json")),
			runs:               file.NewRunRepository(filepath.Join(dir, "run.json")),
			steps:              file.NewStepRepository(filepath.Join(dir, "step.json")),
			toolCalls:          file.NewToolCallRepository(filepath.Join(dir, "tool_call.json")),
			patches:            file.NewPatchRepository(filepath.Join(dir, "patch.json")),
			validationResults:  file.NewValidationResultRepository(filepath.Join(dir, "validation_result.json")),
			runtimeCheckpoints: file.NewRuntimeCheckpointRepository(filepath.Join(dir, "runtime_checkpoint.json")),
			tickets:            file.NewDecisionTicketRepository(filepath.Join(dir, "ticket.json")),
			resolutions:        file.NewDecisionResolutionRepository(filepath.Join(dir, "resolution.json")),
			approvals:          file.NewApprovalRepository(filepath.Join(dir, "approval.json")),
		}
	}
	testRepositoryBundleContract(t, build(), build)
}

type repositoryBundle struct {
	souls              repository.SoulRepository
	memoryItems        repository.MemoryItemRepository
	skills             repository.SkillRepository
	mcpServers         repository.MCPServerRepository
	workspaces         repository.WorkspaceRepository
	goals              repository.GoalRepository
	dods               repository.DoDRepository
	runs               repository.RunRepository
	steps              repository.StepRepository
	toolCalls          repository.ToolCallRepository
	patches            repository.PatchRepository
	validationResults  repository.ValidationResultRepository
	runtimeCheckpoints repository.RuntimeCheckpointRepository
	tickets            repository.DecisionTicketRepository
	resolutions        repository.DecisionResolutionRepository
	approvals          repository.ApprovalRepository
}

func testSkillRepositoryContract(t *testing.T, factory func() repository.SkillRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &skillmodel.Skill{
		ID:                    common.SkillID("skill-contract"),
		Name:                  "go service delivery",
		Version:               "v1",
		Description:           "deliver Go service changes",
		TriggerRules:          []string{"go", "service"},
		Instruction:           "Keep changes small and verified.",
		ReferenceRefs:         []string{"docs/structure.md"},
		RequiredToolKinds:     []string{"shell/command"},
		RequiredMCPServerRefs: []string{"mcp-contract"},
		Constraints:           []string{"cite evidence"},
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate skill create err = %v, want conflict", err)
	}
	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get skill: %v", err)
	}
	got.TriggerRules[0] = "mutated"
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get skill fresh: %v", err)
	}
	if fresh.TriggerRules[0] != "go" {
		t.Fatalf("skill clone isolation failed, triggerRules = %+v", fresh.TriggerRules)
	}
	fresh.Description = "updated skill"
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update skill: %v", err)
	}
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if len(list) != 1 || list[0].Description != "updated skill" {
		t.Fatalf("skills = %+v, want updated skill", list)
	}
	if _, err := repo.Get(ctx, common.SkillID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing skill err = %v, want not found", err)
	}
}

func testMCPServerRepositoryContract(t *testing.T, factory func() repository.MCPServerRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &mcpmodel.Server{
		ID:                    common.MCPServerID("mcp-contract"),
		Name:                  "repo context",
		Transport:             "stdio",
		Endpoint:              "repo-context",
		Enabled:               true,
		AllowedTools:          []string{"repo.summary"},
		AllowedResources:      []string{"repo://summary"},
		AllowedPrompts:        []string{"delivery.plan"},
		DefaultApprovalPolicy: "manual_for_write",
		SessionPolicy:         "long_lived",
		TimeoutMillis:         30000,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate mcp server create err = %v, want conflict", err)
	}
	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get mcp server: %v", err)
	}
	got.AllowedTools[0] = "mutated"
	got.AllowedPrompts[0] = "mutated"
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get mcp server fresh: %v", err)
	}
	if fresh.AllowedTools[0] != "repo.summary" {
		t.Fatalf("mcp server clone isolation failed, tools = %+v", fresh.AllowedTools)
	}
	if fresh.AllowedPrompts[0] != "delivery.plan" || fresh.SessionPolicy != "long_lived" {
		t.Fatalf("mcp server clone isolation failed, prompts/session = %+v/%q", fresh.AllowedPrompts, fresh.SessionPolicy)
	}
	fresh.Enabled = false
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update mcp server: %v", err)
	}
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list mcp servers: %v", err)
	}
	if len(list) != 1 || list[0].Enabled {
		t.Fatalf("mcp servers = %+v, want disabled server", list)
	}
	if _, err := repo.Get(ctx, common.MCPServerID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing mcp server err = %v, want not found", err)
	}
}

func testMemoryItemRepositoryContract(t *testing.T, factory func() repository.MemoryItemRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &memorymodel.MemoryItem{
		ID:          common.MemoryID("memory-contract"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		RunID:       common.RunID("run-contract"),
		SourceType:  "run",
		SourceRef:   "run-contract",
		MemoryType:  "run_summary",
		Content:     "Local verification completed through workorchd APIs.",
		Summary:     "workorchd local verification completed",
		Tags:        []string{"local", "verification"},
		Importance:  9,
		Confidence:  8,
		Visibility:  "workspace",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create memory item: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate memory item create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get memory item: %v", err)
	}
	got.Tags[0] = "mutated"

	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get memory item fresh: %v", err)
	}
	if fresh.Tags[0] != "local" {
		t.Fatalf("memory tags clone isolation failed, tags = %+v", fresh.Tags)
	}

	fresh.Summary = "updated local verification summary"
	fresh.Importance = 10
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update memory item: %v", err)
	}

	list, err := repo.ListByWorkspaceID(ctx, item.WorkspaceID)
	if err != nil {
		t.Fatalf("list memory by workspace: %v", err)
	}
	if len(list) != 1 || list[0].Summary != "updated local verification summary" {
		t.Fatalf("workspace memory = %+v, want updated item", list)
	}

	search, err := repo.Search(ctx, repository.MemorySearchQuery{
		WorkspaceID: item.WorkspaceID,
		Query:       "verification",
		Tags:        []string{"local"},
		Types:       []string{"run_summary"},
		Limit:       1,
	})
	if err != nil {
		t.Fatalf("search memory: %v", err)
	}
	if len(search) != 1 || search[0].ID != item.ID {
		t.Fatalf("search memory = %+v, want %q", search, item.ID)
	}

	if _, err := repo.Get(ctx, common.MemoryID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing memory err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &memorymodel.MemoryItem{ID: common.MemoryID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("update missing memory err = %v, want not found", err)
	}
}

func testSoulRepositoryContract(t *testing.T, factory func() repository.SoulRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &soulmodel.Soul{
		ID:                   common.SoulID("soul-contract"),
		Name:                 "engineering delivery",
		Description:          "drives engineering delivery",
		BusinessDomains:      []string{"engineering"},
		SystemInstruction:    "Plan and verify before completion.",
		PlanningPolicy:       "small_steps",
		DecisionPolicy:       "ask_when_missing",
		ApprovalPolicy:       "approve_risky_actions",
		ValidationPolicy:     "evidence_required",
		AllowedSkillRefs:     []string{"skill-go"},
		AllowedMCPServerRefs: []string{"mcp-repo"},
		AllowedToolKinds:     []string{"shell/command"},
		OutputContract:       "LLMExecutionDecision",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create soul: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get soul: %v", err)
	}
	got.Name = "mutated"
	got.AllowedSkillRefs[0] = "mutated"

	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get soul fresh: %v", err)
	}
	if fresh.Name != "engineering delivery" {
		t.Fatalf("clone isolation failed, name = %q", fresh.Name)
	}
	if fresh.AllowedSkillRefs[0] != "skill-go" {
		t.Fatalf("skill refs clone isolation failed, refs = %+v", fresh.AllowedSkillRefs)
	}

	fresh.Description = "updated description"
	fresh.AllowedToolKinds = []string{"shell/command", "patch/apply"}
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update soul: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list soul: %v", err)
	}
	if len(list) != 1 || list[0].Description != "updated description" || len(list[0].AllowedToolKinds) != 2 {
		t.Fatalf("list soul = %+v, want updated soul", list)
	}

	if _, err := repo.Get(ctx, common.SoulID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing soul err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &soulmodel.Soul{ID: common.SoulID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("update missing soul err = %v, want not found", err)
	}
}

func testWorkspaceRepositoryContract(t *testing.T, factory func() repository.WorkspaceRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &workspacemodel.Workspace{
		ID:               common.WorkspaceID("ws-contract"),
		Title:            "workspace-contract",
		Description:      "workspace contract description",
		WorkingDirectory: "/tmp/workorch/ws-contract",
		State:            common.WorkspaceStateActive,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	got.Title = "mutated"

	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get workspace fresh: %v", err)
	}
	if fresh.Title != "workspace-contract" {
		t.Fatalf("clone isolation failed, title = %q", fresh.Title)
	}
	if fresh.Description != "workspace contract description" {
		t.Fatalf("description = %q, want workspace contract description", fresh.Description)
	}
	if fresh.WorkingDirectory != "/tmp/workorch/ws-contract" {
		t.Fatalf("workingDirectory = %q, want /tmp/workorch/ws-contract", fresh.WorkingDirectory)
	}

	fresh.ActiveRunID = common.RunID("run-1")
	fresh.Description = "workspace contract updated description"
	fresh.WorkingDirectory = "/tmp/workorch/ws-contract-updated"
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update workspace: %v", err)
	}

	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated workspace: %v", err)
	}
	if updated.ActiveRunID != common.RunID("run-1") {
		t.Fatalf("active run id = %q, want run-1", updated.ActiveRunID)
	}
	if updated.Description != "workspace contract updated description" {
		t.Fatalf("updated description = %q, want workspace contract updated description", updated.Description)
	}
	if updated.WorkingDirectory != "/tmp/workorch/ws-contract-updated" {
		t.Fatalf("updated workingDirectory = %q, want /tmp/workorch/ws-contract-updated", updated.WorkingDirectory)
	}

	second := &workspacemodel.Workspace{
		ID:               common.WorkspaceID("ws-contract-2"),
		Title:            "workspace-contract-2",
		WorkingDirectory: "/tmp/workorch/ws-contract-2",
		State:            common.WorkspaceStateActive,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second workspace: %v", err)
	}

	listed, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("len(listed) = %d, want 2", len(listed))
	}
	if listed[0].ID != item.ID || listed[1].ID != second.ID {
		t.Fatalf("listed ids = %q/%q, want %q/%q", listed[0].ID, listed[1].ID, item.ID, second.ID)
	}

	if _, err := repo.Get(ctx, common.WorkspaceID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &workspacemodel.Workspace{ID: common.WorkspaceID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testGoalRepositoryContract(t *testing.T, factory func() repository.GoalRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &workspacemodel.Goal{
		ID:          common.GoalID("goal-contract"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		Text:        "deliver contract goal",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}
	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get goal: %v", err)
	}
	got.Text = "mutated"
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get fresh goal: %v", err)
	}
	if fresh.Text != item.Text {
		t.Fatalf("goal text = %q, want %q", fresh.Text, item.Text)
	}
	second := &workspacemodel.Goal{
		ID:          common.GoalID("goal-contract-2"),
		WorkspaceID: item.WorkspaceID,
		Text:        "deliver second contract goal",
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second goal: %v", err)
	}
	listed, err := repo.ListByWorkspaceID(ctx, item.WorkspaceID)
	if err != nil {
		t.Fatalf("list goals: %v", err)
	}
	if len(listed) != 2 || listed[0].ID != item.ID || listed[1].ID != second.ID {
		t.Fatalf("listed goals = %+v, want %q/%q", listed, item.ID, second.ID)
	}
	if _, err := repo.Get(ctx, common.GoalID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
}

func testDoDRepositoryContract(t *testing.T, factory func() repository.DoDRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &workspacemodel.DoD{
		ID:          common.DoDID("dod-contract"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		Items:       []string{"workspace created", "run completed"},
		State:       common.DoDStateReady,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}
	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get dod: %v", err)
	}
	got.Items[0] = "mutated"
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get fresh dod: %v", err)
	}
	if fresh.Items[0] != "workspace created" {
		t.Fatalf("dod clone isolation failed, first item = %q", fresh.Items[0])
	}
	fresh.State = common.DoDStateSatisfied
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update dod: %v", err)
	}
	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated dod: %v", err)
	}
	if updated.State != common.DoDStateSatisfied {
		t.Fatalf("dod state = %q, want satisfied", updated.State)
	}
	second := &workspacemodel.DoD{
		ID:          common.DoDID("dod-contract-2"),
		WorkspaceID: item.WorkspaceID,
		Items:       []string{"second done"},
		State:       common.DoDStateReady,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second dod: %v", err)
	}
	listed, err := repo.ListByWorkspaceID(ctx, item.WorkspaceID)
	if err != nil {
		t.Fatalf("list dods: %v", err)
	}
	if len(listed) != 2 || listed[0].ID != item.ID || listed[1].ID != second.ID {
		t.Fatalf("listed dods = %+v, want %q/%q", listed, item.ID, second.ID)
	}
	if _, err := repo.Get(ctx, common.DoDID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &workspacemodel.DoD{ID: common.DoDID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testRunRepositoryContract(t *testing.T, factory func() repository.RunRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &runmodel.Run{
		ID:          common.RunID("run-contract"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		State:       common.RunStateCreated,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	got.State = common.RunStateCompleted
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update run: %v", err)
	}

	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated run: %v", err)
	}
	if updated.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", updated.State)
	}
	updated.FailureReason = "manual fail"
	if err := repo.Update(ctx, updated); err != nil {
		t.Fatalf("update run failure reason: %v", err)
	}

	refreshed, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get refreshed run: %v", err)
	}
	if refreshed.FailureReason != "manual fail" {
		t.Fatalf("failure reason = %q, want manual fail", refreshed.FailureReason)
	}

	second := &runmodel.Run{
		ID:          common.RunID("run-contract-2"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		State:       common.RunStateCreated,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second run: %v", err)
	}

	listed, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("len(listed) = %d, want 2", len(listed))
	}
	if listed[0].ID != item.ID || listed[1].ID != second.ID {
		t.Fatalf("listed ids = %q/%q, want %q/%q", listed[0].ID, listed[1].ID, item.ID, second.ID)
	}

	if _, err := repo.Get(ctx, common.RunID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &runmodel.Run{ID: common.RunID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testStepRepositoryContract(t *testing.T, factory func() repository.StepRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &runmodel.Step{
		ID:    common.StepID("run-contract/step-1"),
		RunID: common.RunID("run-contract"),
		Kind:  "legacy/minimal",
		State: common.StepStateDispatched,
		Input: map[string]any{"workspaceId": "ws-contract"},
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	got.Input["workspaceId"] = "mutated"
	got.Output = map[string]any{"result": "done"}

	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get fresh step: %v", err)
	}
	if fresh.Input["workspaceId"] != "ws-contract" {
		t.Fatalf("clone isolation failed, workspaceId = %v", fresh.Input["workspaceId"])
	}
	if fresh.Output != nil {
		t.Fatalf("fresh output = %#v, want nil", fresh.Output)
	}

	fresh.State = common.StepStateCompleted
	fresh.Output = map[string]any{"result": "done"}
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update step: %v", err)
	}

	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated step: %v", err)
	}
	if updated.State != common.StepStateCompleted {
		t.Fatalf("step state = %q, want completed", updated.State)
	}
	if updated.Output["result"] != "done" {
		t.Fatalf("output result = %v, want done", updated.Output["result"])
	}

	if err := repo.Create(ctx, &runmodel.Step{
		ID:    common.StepID("run-contract/step-2"),
		RunID: common.RunID("run-contract"),
		Kind:  "legacy/minimal",
		State: common.StepStateCompleted,
		Input: map[string]any{"workspaceId": "ws-contract", "previousStepId": "run-contract/step-1"},
	}); err != nil {
		t.Fatalf("create second step: %v", err)
	}
	if err := repo.Create(ctx, &runmodel.Step{
		ID:    common.StepID("run-other/step-1"),
		RunID: common.RunID("run-other"),
		Kind:  "legacy/minimal",
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("create other run step: %v", err)
	}

	steps, err := repo.ListByRun(ctx, common.RunID("run-contract"))
	if err != nil {
		t.Fatalf("list steps by run: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("list steps len = %d, want 2", len(steps))
	}
	if steps[0].ID != common.StepID("run-contract/step-1") || steps[1].ID != common.StepID("run-contract/step-2") {
		t.Fatalf("listed step ids = [%s %s], want [run-contract/step-1 run-contract/step-2]", steps[0].ID, steps[1].ID)
	}
	steps[0].Output["result"] = "mutated"
	refreshed, err := repo.Get(ctx, common.StepID("run-contract/step-1"))
	if err != nil {
		t.Fatalf("get refreshed step: %v", err)
	}
	if refreshed.Output["result"] != "done" {
		t.Fatalf("refreshed output result = %v, want done", refreshed.Output["result"])
	}

	steps, err = repo.ListByRun(ctx, common.RunID("run-missing"))
	if err != nil {
		t.Fatalf("list missing run steps: %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("missing run steps len = %d, want 0", len(steps))
	}

	if _, err := repo.Get(ctx, common.StepID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &runmodel.Step{ID: common.StepID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testToolCallRepositoryContract(t *testing.T, factory func() repository.ToolCallRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &runmodel.ToolCall{
		ID:          common.ToolCallID("tool-call-contract-1"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		RunID:       common.RunID("run-contract"),
		StepID:      common.StepID("run-contract/step-1"),
		ToolName:    "shell",
		Arguments:   map[string]any{"command": "go test ./..."},
		State:       common.ToolCallStateRunning,
		StartedAt:   "2026-04-25T00:00:00Z",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get tool call: %v", err)
	}
	got.Arguments["command"] = "mutated"
	got.State = common.ToolCallStateCompleted
	got.Output = map[string]any{"exitCode": 0}
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update tool call: %v", err)
	}

	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated tool call: %v", err)
	}
	if updated.Arguments["command"] != "mutated" || updated.Output["exitCode"] != float64(0) && updated.Output["exitCode"] != 0 {
		t.Fatalf("updated tool call = %+v, want updated argument and output", updated)
	}
	updated.Output["exitCode"] = 1
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get fresh tool call: %v", err)
	}
	if fresh.Output["exitCode"] != float64(0) && fresh.Output["exitCode"] != 0 {
		t.Fatalf("clone isolation failed, exitCode = %v", fresh.Output["exitCode"])
	}

	second := &runmodel.ToolCall{
		ID:          common.ToolCallID("tool-call-contract-2"),
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		StepID:      common.StepID("run-contract/step-2"),
		ToolName:    "editor",
		State:       common.ToolCallStateCompleted,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second tool call: %v", err)
	}
	other := &runmodel.ToolCall{
		ID:          common.ToolCallID("tool-call-other-run"),
		WorkspaceID: item.WorkspaceID,
		RunID:       common.RunID("run-other"),
		StepID:      common.StepID("run-other/step-1"),
		ToolName:    "shell",
		State:       common.ToolCallStateCompleted,
	}
	if err := repo.Create(ctx, other); err != nil {
		t.Fatalf("create other run tool call: %v", err)
	}

	byRun, err := repo.ListByRun(ctx, item.RunID)
	if err != nil {
		t.Fatalf("list tool calls by run: %v", err)
	}
	if len(byRun) != 2 || byRun[0].ID != item.ID || byRun[1].ID != second.ID {
		t.Fatalf("tool calls by run = %+v, want first two calls", byRun)
	}
	byStep, err := repo.ListByStep(ctx, item.StepID)
	if err != nil {
		t.Fatalf("list tool calls by step: %v", err)
	}
	if len(byStep) != 1 || byStep[0].ID != item.ID {
		t.Fatalf("tool calls by step = %+v, want first call", byStep)
	}
	if _, err := repo.Get(ctx, common.ToolCallID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &runmodel.ToolCall{ID: common.ToolCallID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testPatchRepositoryContract(t *testing.T, factory func() repository.PatchRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &runmodel.Patch{
		ID:          common.PatchID("patch-contract-1"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		RunID:       common.RunID("run-contract"),
		StepID:      common.StepID("run-contract/step-1"),
		ToolCallID:  common.ToolCallID("tool-call-contract-1"),
		PatchType:   "unified_diff",
		TargetRef:   "internal/example.go",
		Summary:     "initial patch",
		State:       common.PatchStateCreated,
		CreatedAt:   "2026-04-25T00:00:00Z",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create patch: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get patch: %v", err)
	}
	got.State = common.PatchStateApplied
	got.AppliedAt = "2026-04-25T00:01:00Z"
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update patch: %v", err)
	}
	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated patch: %v", err)
	}
	if updated.State != common.PatchStateApplied || updated.AppliedAt == "" {
		t.Fatalf("patch = %+v, want applied", updated)
	}

	second := &runmodel.Patch{
		ID:          common.PatchID("patch-contract-2"),
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		StepID:      common.StepID("run-contract/step-2"),
		PatchType:   "file_replace",
		State:       common.PatchStateCreated,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second patch: %v", err)
	}
	if err := repo.Create(ctx, &runmodel.Patch{
		ID:          common.PatchID("patch-other-run"),
		WorkspaceID: item.WorkspaceID,
		RunID:       common.RunID("run-other"),
		StepID:      common.StepID("run-other/step-1"),
		PatchType:   "unified_diff",
		State:       common.PatchStateCreated,
	}); err != nil {
		t.Fatalf("create other run patch: %v", err)
	}

	byRun, err := repo.ListByRun(ctx, item.RunID)
	if err != nil {
		t.Fatalf("list patches by run: %v", err)
	}
	if len(byRun) != 2 || byRun[0].ID != item.ID || byRun[1].ID != second.ID {
		t.Fatalf("patches by run = %+v, want first two patches", byRun)
	}
	byStep, err := repo.ListByStep(ctx, item.StepID)
	if err != nil {
		t.Fatalf("list patches by step: %v", err)
	}
	if len(byStep) != 1 || byStep[0].ID != item.ID {
		t.Fatalf("patches by step = %+v, want first patch", byStep)
	}
	if _, err := repo.Get(ctx, common.PatchID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &runmodel.Patch{ID: common.PatchID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testValidationResultRepositoryContract(t *testing.T, factory func() repository.ValidationResultRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &runmodel.ValidationResult{
		ID:             common.ValidationResultID("validation-contract-1"),
		WorkspaceID:    common.WorkspaceID("ws-contract"),
		RunID:          common.RunID("run-contract"),
		StepID:         common.StepID("run-contract/step-1"),
		ValidationKind: "go_test",
		TargetRef:      "./...",
		State:          common.ValidationResultStateFailed,
		Summary:        "initial failure",
		StartedAt:      "2026-04-25T00:00:00Z",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create validation result: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get validation result: %v", err)
	}
	got.State = common.ValidationResultStatePassed
	got.Summary = "passed"
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update validation result: %v", err)
	}
	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated validation result: %v", err)
	}
	if updated.State != common.ValidationResultStatePassed || updated.Summary != "passed" {
		t.Fatalf("validation result = %+v, want passed", updated)
	}

	second := &runmodel.ValidationResult{
		ID:             common.ValidationResultID("validation-contract-2"),
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         common.StepID("run-contract/step-2"),
		ValidationKind: "lint",
		State:          common.ValidationResultStateWarning,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second validation result: %v", err)
	}
	if err := repo.Create(ctx, &runmodel.ValidationResult{
		ID:          common.ValidationResultID("validation-other-run"),
		WorkspaceID: item.WorkspaceID,
		RunID:       common.RunID("run-other"),
		StepID:      common.StepID("run-other/step-1"),
		State:       common.ValidationResultStatePassed,
	}); err != nil {
		t.Fatalf("create other run validation result: %v", err)
	}

	byRun, err := repo.ListByRun(ctx, item.RunID)
	if err != nil {
		t.Fatalf("list validation results by run: %v", err)
	}
	if len(byRun) != 2 || byRun[0].ID != item.ID || byRun[1].ID != second.ID {
		t.Fatalf("validation results by run = %+v, want first two results", byRun)
	}
	byStep, err := repo.ListByStep(ctx, item.StepID)
	if err != nil {
		t.Fatalf("list validation results by step: %v", err)
	}
	if len(byStep) != 1 || byStep[0].ID != item.ID {
		t.Fatalf("validation results by step = %+v, want first result", byStep)
	}
	if _, err := repo.Get(ctx, common.ValidationResultID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &runmodel.ValidationResult{ID: common.ValidationResultID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testRuntimeCheckpointRepositoryContract(t *testing.T, factory func() repository.RuntimeCheckpointRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &runmodel.RuntimeCheckpoint{
		ID:               common.CheckpointID("checkpoint-contract-1"),
		WorkspaceID:      common.WorkspaceID("ws-contract"),
		RunID:            common.RunID("run-contract"),
		CheckpointType:   common.CheckpointTypeRunState,
		CurrentStepID:    common.StepID("run-contract/step-1"),
		RunStateSnapshot: common.RunStateRunning,
		OccurredAt:       "2026-04-25T00:00:00Z",
		Data:             map[string]any{"thread": "main"},
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create runtime checkpoint: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get runtime checkpoint: %v", err)
	}
	got.Data["thread"] = "mutated"
	got.RunStateSnapshot = common.RunStateBlockedOnDecision
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update runtime checkpoint: %v", err)
	}
	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated runtime checkpoint: %v", err)
	}
	if updated.Data["thread"] != "mutated" || updated.RunStateSnapshot != common.RunStateBlockedOnDecision {
		t.Fatalf("runtime checkpoint = %+v, want updated", updated)
	}
	updated.Data["thread"] = "leak"
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get fresh runtime checkpoint: %v", err)
	}
	if fresh.Data["thread"] != "mutated" {
		t.Fatalf("clone isolation failed, thread = %v", fresh.Data["thread"])
	}

	second := &runmodel.RuntimeCheckpoint{
		ID:               common.CheckpointID("checkpoint-contract-2"),
		WorkspaceID:      item.WorkspaceID,
		RunID:            item.RunID,
		CheckpointType:   common.CheckpointTypeBlocked,
		RunStateSnapshot: common.RunStateBlockedOnApproval,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second runtime checkpoint: %v", err)
	}
	if err := repo.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:               common.CheckpointID("checkpoint-other-run"),
		WorkspaceID:      item.WorkspaceID,
		RunID:            common.RunID("run-other"),
		CheckpointType:   common.CheckpointTypeRunState,
		RunStateSnapshot: common.RunStateRunning,
	}); err != nil {
		t.Fatalf("create other run runtime checkpoint: %v", err)
	}

	byRun, err := repo.ListByRun(ctx, item.RunID)
	if err != nil {
		t.Fatalf("list runtime checkpoints by run: %v", err)
	}
	if len(byRun) != 2 || byRun[0].ID != item.ID || byRun[1].ID != second.ID {
		t.Fatalf("runtime checkpoints by run = %+v, want first two checkpoints", byRun)
	}
	if _, err := repo.Get(ctx, common.CheckpointID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &runmodel.RuntimeCheckpoint{ID: common.CheckpointID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testDecisionTicketRepositoryContract(t *testing.T, factory func() repository.DecisionTicketRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &decisionmodel.Ticket{
		ID:          common.TicketID("ticket-contract"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		RunID:       common.RunID("run-contract"),
		StepID:      common.StepID("run-contract/manual-gate"),
		Reason:      "manual decision",
		State:       common.DecisionTicketStateOpen,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	got.State = common.DecisionTicketStateResolved
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update ticket: %v", err)
	}

	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated ticket: %v", err)
	}
	if updated.State != common.DecisionTicketStateResolved {
		t.Fatalf("ticket state = %q, want resolved", updated.State)
	}

	second := &decisionmodel.Ticket{
		ID:          common.TicketID("ticket-contract-2"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		RunID:       common.RunID("run-contract-2"),
		StepID:      common.StepID("run-contract-2/manual-gate"),
		Reason:      "manual decision 2",
		State:       common.DecisionTicketStateOpen,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second ticket: %v", err)
	}

	listed, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("len(listed) = %d, want 2", len(listed))
	}
	if listed[0].ID != item.ID || listed[1].ID != second.ID {
		t.Fatalf("listed ids = %q/%q, want %q/%q", listed[0].ID, listed[1].ID, item.ID, second.ID)
	}

	if _, err := repo.Get(ctx, common.TicketID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &decisionmodel.Ticket{ID: common.TicketID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testDecisionResolutionRepositoryContract(t *testing.T, factory func() repository.DecisionResolutionRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-contract"),
		TicketID: common.TicketID("ticket-contract"),
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "contract-test"},
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create resolution: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.GetByTicketID(ctx, item.TicketID)
	if err != nil {
		t.Fatalf("get resolution by ticket id: %v", err)
	}
	got.Type = "mutated"

	fresh, err := repo.GetByTicketID(ctx, item.TicketID)
	if err != nil {
		t.Fatalf("get resolution by ticket id fresh: %v", err)
	}
	if fresh.Type != "provide_missing_info" {
		t.Fatalf("clone isolation failed, type = %q", fresh.Type)
	}
	if fresh.Payload["operator"] != "contract-test" {
		t.Fatalf("payload operator = %v, want contract-test", fresh.Payload["operator"])
	}

	if _, err := repo.GetByTicketID(ctx, common.TicketID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
}

func testApprovalRepositoryContract(t *testing.T, factory func() repository.ApprovalRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &approvalmodel.Approval{
		ID:          common.ApprovalID("approval-contract"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		RunID:       common.RunID("run-contract"),
		StepID:      common.StepID("run-contract/manual-approval"),
		Reason:      "manual approval",
		State:       common.ApprovalStatePending,
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	got.State = common.ApprovalStateApproved
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated approval: %v", err)
	}
	if updated.State != common.ApprovalStateApproved {
		t.Fatalf("approval state = %q, want approved", updated.State)
	}

	second := &approvalmodel.Approval{
		ID:          common.ApprovalID("approval-contract-2"),
		WorkspaceID: common.WorkspaceID("ws-contract"),
		RunID:       common.RunID("run-contract-2"),
		StepID:      common.StepID("run-contract-2/manual-approval"),
		Reason:      "manual approval 2",
		State:       common.ApprovalStatePending,
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("create second approval: %v", err)
	}

	listed, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("len(listed) = %d, want 2", len(listed))
	}
	if listed[0].ID != item.ID || listed[1].ID != second.ID {
		t.Fatalf("listed ids = %q/%q, want %q/%q", listed[0].ID, listed[1].ID, item.ID, second.ID)
	}

	if _, err := repo.Get(ctx, common.ApprovalID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &approvalmodel.Approval{ID: common.ApprovalID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
	}
}

func testRepositoryBundleContract(t *testing.T, bundle repositoryBundle, reopen func() repositoryBundle) {
	t.Helper()

	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-bundle-contract")
	goalID := common.GoalID("goal-bundle-contract")
	dodID := common.DoDID("dod-bundle-contract")
	runID := common.RunID("run-bundle-contract")
	stepID := common.StepID("run-bundle-contract/step-1")
	toolCallID := common.ToolCallID("tool-call-bundle-contract")
	patchID := common.PatchID("patch-bundle-contract")
	validationResultID := common.ValidationResultID("validation-bundle-contract")
	checkpointID := common.CheckpointID("checkpoint-bundle-contract")
	ticketID := common.TicketID("ticket-bundle-contract")
	resolutionID := common.ResolutionID("resolution-bundle-contract")
	approvalID := common.ApprovalID("approval-bundle-contract")
	soulID := common.SoulID("soul-bundle-contract")
	memoryID := common.MemoryID("memory-bundle-contract")
	skillID := common.SkillID("skill-bundle-contract")
	mcpServerID := common.MCPServerID("mcp-bundle-contract")

	soul := &soulmodel.Soul{
		ID:                   soulID,
		Name:                 "bundle soul",
		SystemInstruction:    "Plan and cite evidence.",
		AllowedSkillRefs:     []string{"skill-bundle"},
		AllowedMCPServerRefs: []string{"mcp-bundle"},
		AllowedToolKinds:     []string{"shell/command"},
	}
	if err := bundle.souls.Create(ctx, soul); err != nil {
		t.Fatalf("create soul: %v", err)
	}
	if err := bundle.souls.Create(ctx, soul); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate soul create err = %v, want conflict", err)
	}

	memoryItem := &memorymodel.MemoryItem{
		ID:          memoryID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		SourceType:  "run",
		SourceRef:   string(runID),
		MemoryType:  "run_summary",
		Summary:     "bundle run summary",
		Tags:        []string{"bundle"},
		Importance:  5,
	}
	if err := bundle.memoryItems.Create(ctx, memoryItem); err != nil {
		t.Fatalf("create memory item: %v", err)
	}
	if err := bundle.memoryItems.Create(ctx, memoryItem); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate memory item create err = %v, want conflict", err)
	}

	skill := &skillmodel.Skill{
		ID:                    skillID,
		Name:                  "bundle skill",
		Version:               "v1",
		Instruction:           "Use bundle skill.",
		RequiredMCPServerRefs: []string{string(mcpServerID)},
	}
	if err := bundle.skills.Create(ctx, skill); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err := bundle.skills.Create(ctx, skill); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate skill create err = %v, want conflict", err)
	}

	mcpServer := &mcpmodel.Server{
		ID:             mcpServerID,
		Name:           "bundle mcp",
		Transport:      "stdio",
		Enabled:        true,
		AllowedTools:   []string{"repo.summary"},
		AllowedPrompts: []string{"delivery.plan"},
		SessionPolicy:  "long_lived",
		TimeoutMillis:  1000,
	}
	if err := bundle.mcpServers.Create(ctx, mcpServer); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}
	if err := bundle.mcpServers.Create(ctx, mcpServer); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate mcp server create err = %v, want conflict", err)
	}

	workspace := &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "bundle contract workspace",
		State: common.WorkspaceStateActive,
	}
	if err := bundle.workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := bundle.workspaces.Create(ctx, workspace); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate workspace create err = %v, want conflict", err)
	}

	goal := &workspacemodel.Goal{
		ID:          goalID,
		WorkspaceID: workspaceID,
		Text:        "bundle contract goal",
	}
	if err := bundle.goals.Create(ctx, goal); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := bundle.goals.Create(ctx, goal); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate goal create err = %v, want conflict", err)
	}

	dod := &workspacemodel.DoD{
		ID:          dodID,
		WorkspaceID: workspaceID,
		Items:       []string{"bundle done"},
		State:       common.DoDStateReady,
	}
	if err := bundle.dods.Create(ctx, dod); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	if err := bundle.dods.Create(ctx, dod); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate dod create err = %v, want conflict", err)
	}

	run := &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		GoalID:             goalID,
		DoDID:              dodID,
		State:              common.RunStateBlockedOnDecision,
		BlockingTicketID:   ticketID,
		BlockingApprovalID: approvalID,
	}
	if err := bundle.runs.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := bundle.runs.Create(ctx, run); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate run create err = %v, want conflict", err)
	}

	step := &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "legacy/manual",
		State: common.StepStateDispatched,
		Input: map[string]any{"workspaceId": string(workspaceID), "runId": string(runID)},
	}
	if err := bundle.steps.Create(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := bundle.steps.Create(ctx, step); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate step create err = %v, want conflict", err)
	}
	step.State = common.StepStateCompleted
	step.Output = map[string]any{"status": "completed", "ticketId": string(ticketID)}
	if err := bundle.steps.Update(ctx, step); err != nil {
		t.Fatalf("update step: %v", err)
	}

	toolCall := &runmodel.ToolCall{
		ID:          toolCallID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolName:    "shell",
		Arguments:   map[string]any{"command": "go test ./..."},
		State:       common.ToolCallStateCompleted,
		Output:      map[string]any{"exitCode": 0},
	}
	if err := bundle.toolCalls.Create(ctx, toolCall); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	if err := bundle.toolCalls.Create(ctx, toolCall); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate tool call create err = %v, want conflict", err)
	}

	patch := &runmodel.Patch{
		ID:          patchID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolCallID:  toolCallID,
		PatchType:   "unified_diff",
		TargetRef:   "internal/example.go",
		State:       common.PatchStateApplied,
	}
	if err := bundle.patches.Create(ctx, patch); err != nil {
		t.Fatalf("create patch: %v", err)
	}
	if err := bundle.patches.Create(ctx, patch); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate patch create err = %v, want conflict", err)
	}

	validationResult := &runmodel.ValidationResult{
		ID:             validationResultID,
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         stepID,
		ValidationKind: "go_test",
		State:          common.ValidationResultStatePassed,
		Summary:        "passed",
	}
	if err := bundle.validationResults.Create(ctx, validationResult); err != nil {
		t.Fatalf("create validation result: %v", err)
	}
	if err := bundle.validationResults.Create(ctx, validationResult); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate validation result create err = %v, want conflict", err)
	}

	checkpoint := &runmodel.RuntimeCheckpoint{
		ID:               checkpointID,
		WorkspaceID:      workspaceID,
		RunID:            runID,
		CheckpointType:   common.CheckpointTypeRunState,
		CurrentStepID:    stepID,
		RunStateSnapshot: common.RunStateBlockedOnDecision,
		Data:             map[string]any{"thread": "main"},
	}
	if err := bundle.runtimeCheckpoints.Create(ctx, checkpoint); err != nil {
		t.Fatalf("create runtime checkpoint: %v", err)
	}
	if err := bundle.runtimeCheckpoints.Create(ctx, checkpoint); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate runtime checkpoint create err = %v, want conflict", err)
	}

	ticket := &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Reason:      "bundle manual decision",
		State:       common.DecisionTicketStateOpen,
	}
	if err := bundle.tickets.Create(ctx, ticket); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := bundle.tickets.Create(ctx, ticket); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate ticket create err = %v, want conflict", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := bundle.tickets.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}

	resolution := &decisionmodel.Resolution{
		ID:       resolutionID,
		TicketID: ticketID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "bundle-test"},
	}
	if err := bundle.resolutions.Create(ctx, resolution); err != nil {
		t.Fatalf("create resolution: %v", err)
	}
	if err := bundle.resolutions.Create(ctx, resolution); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate resolution create err = %v, want conflict", err)
	}

	approval := &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Reason:      "bundle manual approval",
		State:       common.ApprovalStatePending,
	}
	if err := bundle.approvals.Create(ctx, approval); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	if err := bundle.approvals.Create(ctx, approval); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate approval create err = %v, want conflict", err)
	}
	approval.State = common.ApprovalStateRejected
	if err := bundle.approvals.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	workspace.ActiveRunID = runID
	if err := bundle.workspaces.Update(ctx, workspace); err != nil {
		t.Fatalf("update workspace: %v", err)
	}

	run.State = common.RunStateFailed
	run.FailureReason = "approval rejected"
	if err := bundle.runs.Update(ctx, run); err != nil {
		t.Fatalf("update run: %v", err)
	}

	reloaded := reopen()

	reloadedSoul, err := reloaded.souls.Get(ctx, soulID)
	if err != nil {
		t.Fatalf("reload soul: %v", err)
	}
	if reloadedSoul.Name != "bundle soul" || reloadedSoul.AllowedSkillRefs[0] != "skill-bundle" {
		t.Fatalf("reloaded soul = %+v, want bundle soul", reloadedSoul)
	}

	reloadedMemory, err := reloaded.memoryItems.Search(ctx, repository.MemorySearchQuery{
		WorkspaceID: workspaceID,
		Query:       "summary",
		Tags:        []string{"bundle"},
		Types:       []string{"run_summary"},
	})
	if err != nil {
		t.Fatalf("reload memory search: %v", err)
	}
	if len(reloadedMemory) != 1 || reloadedMemory[0].ID != memoryID {
		t.Fatalf("reloaded memory = %+v, want %q", reloadedMemory, memoryID)
	}

	reloadedSkill, err := reloaded.skills.Get(ctx, skillID)
	if err != nil {
		t.Fatalf("reload skill: %v", err)
	}
	if reloadedSkill.Name != "bundle skill" || reloadedSkill.RequiredMCPServerRefs[0] != string(mcpServerID) {
		t.Fatalf("reloaded skill = %+v, want bundle skill", reloadedSkill)
	}

	reloadedMCPServer, err := reloaded.mcpServers.Get(ctx, mcpServerID)
	if err != nil {
		t.Fatalf("reload mcp server: %v", err)
	}
	if reloadedMCPServer.Name != "bundle mcp" ||
		reloadedMCPServer.AllowedTools[0] != "repo.summary" ||
		reloadedMCPServer.AllowedPrompts[0] != "delivery.plan" ||
		reloadedMCPServer.SessionPolicy != "long_lived" {
		t.Fatalf("reloaded mcp server = %+v, want bundle mcp", reloadedMCPServer)
	}

	reloadedWorkspace, err := reloaded.workspaces.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if reloadedWorkspace.ActiveRunID != runID {
		t.Fatalf("reloaded workspace activeRunID = %q, want %q", reloadedWorkspace.ActiveRunID, runID)
	}

	reloadedGoals, err := reloaded.goals.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("reload goals: %v", err)
	}
	if len(reloadedGoals) != 1 || reloadedGoals[0].ID != goalID || reloadedGoals[0].Text != "bundle contract goal" {
		t.Fatalf("reloaded goals = %+v, want %q", reloadedGoals, goalID)
	}

	reloadedDoDs, err := reloaded.dods.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("reload dods: %v", err)
	}
	if len(reloadedDoDs) != 1 || reloadedDoDs[0].ID != dodID || reloadedDoDs[0].Items[0] != "bundle done" {
		t.Fatalf("reloaded dods = %+v, want %q", reloadedDoDs, dodID)
	}

	reloadedRun, err := reloaded.runs.Get(ctx, runID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if reloadedRun.State != common.RunStateFailed || reloadedRun.FailureReason != "approval rejected" {
		t.Fatalf("reloaded run = %+v, want failed with reason", reloadedRun)
	}
	if reloadedRun.BlockingTicketID != ticketID || reloadedRun.BlockingApprovalID != approvalID {
		t.Fatalf("reloaded run blocking refs = %+v, want %q/%q", reloadedRun, ticketID, approvalID)
	}

	reloadedSteps, err := reloaded.steps.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("reload steps by run: %v", err)
	}
	if len(reloadedSteps) != 1 || reloadedSteps[0].Output["status"] != "completed" {
		t.Fatalf("reloaded steps = %+v, want completed output", reloadedSteps)
	}
	reloadedSteps[0].Output["status"] = "mutated"
	stepFresh, err := reloaded.steps.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("reload step fresh: %v", err)
	}
	if stepFresh.Output["status"] != "completed" {
		t.Fatalf("step output status = %v, want completed", stepFresh.Output["status"])
	}

	reloadedToolCalls, err := reloaded.toolCalls.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("reload tool calls by run: %v", err)
	}
	if len(reloadedToolCalls) != 1 || reloadedToolCalls[0].ID != toolCallID || reloadedToolCalls[0].Output["exitCode"] == nil {
		t.Fatalf("reloaded tool calls = %+v, want %q with output", reloadedToolCalls, toolCallID)
	}

	reloadedPatches, err := reloaded.patches.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("reload patches by run: %v", err)
	}
	if len(reloadedPatches) != 1 || reloadedPatches[0].ID != patchID || reloadedPatches[0].State != common.PatchStateApplied {
		t.Fatalf("reloaded patches = %+v, want applied %q", reloadedPatches, patchID)
	}

	reloadedValidationResults, err := reloaded.validationResults.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("reload validation results by run: %v", err)
	}
	if len(reloadedValidationResults) != 1 || reloadedValidationResults[0].ID != validationResultID || reloadedValidationResults[0].State != common.ValidationResultStatePassed {
		t.Fatalf("reloaded validation results = %+v, want passed %q", reloadedValidationResults, validationResultID)
	}

	reloadedCheckpoints, err := reloaded.runtimeCheckpoints.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("reload runtime checkpoints by run: %v", err)
	}
	if len(reloadedCheckpoints) != 1 || reloadedCheckpoints[0].ID != checkpointID || reloadedCheckpoints[0].Data["thread"] != "main" {
		t.Fatalf("reloaded runtime checkpoints = %+v, want %q", reloadedCheckpoints, checkpointID)
	}

	reloadedTickets, err := reloaded.tickets.List(ctx)
	if err != nil {
		t.Fatalf("reload tickets: %v", err)
	}
	if len(reloadedTickets) != 1 || reloadedTickets[0].State != common.DecisionTicketStateResolved {
		t.Fatalf("reloaded tickets = %+v, want one resolved ticket", reloadedTickets)
	}

	reloadedResolution, err := reloaded.resolutions.GetByTicketID(ctx, ticketID)
	if err != nil {
		t.Fatalf("reload resolution: %v", err)
	}
	if reloadedResolution.ID != resolutionID || reloadedResolution.Payload["operator"] != "bundle-test" {
		t.Fatalf("reloaded resolution = %+v, want %q with payload", reloadedResolution, resolutionID)
	}

	reloadedApprovals, err := reloaded.approvals.List(ctx)
	if err != nil {
		t.Fatalf("reload approvals: %v", err)
	}
	if len(reloadedApprovals) != 1 || reloadedApprovals[0].State != common.ApprovalStateRejected {
		t.Fatalf("reloaded approvals = %+v, want one rejected approval", reloadedApprovals)
	}

	if _, err := reloaded.workspaces.Get(ctx, common.WorkspaceID("workspace-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing workspace get err = %v, want not found", err)
	}
	if _, err := reloaded.souls.Get(ctx, common.SoulID("soul-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing soul get err = %v, want not found", err)
	}
	if _, err := reloaded.memoryItems.Get(ctx, common.MemoryID("memory-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing memory get err = %v, want not found", err)
	}
	if _, err := reloaded.skills.Get(ctx, common.SkillID("skill-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing skill get err = %v, want not found", err)
	}
	if _, err := reloaded.mcpServers.Get(ctx, common.MCPServerID("mcp-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing mcp server get err = %v, want not found", err)
	}
	if _, err := reloaded.goals.Get(ctx, common.GoalID("goal-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing goal get err = %v, want not found", err)
	}
	if _, err := reloaded.dods.Get(ctx, common.DoDID("dod-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing dod get err = %v, want not found", err)
	}
	if _, err := reloaded.runs.Get(ctx, common.RunID("run-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing run get err = %v, want not found", err)
	}
	if _, err := reloaded.steps.Get(ctx, common.StepID("step-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing step get err = %v, want not found", err)
	}
	if _, err := reloaded.toolCalls.Get(ctx, common.ToolCallID("tool-call-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing tool call get err = %v, want not found", err)
	}
	if _, err := reloaded.patches.Get(ctx, common.PatchID("patch-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing patch get err = %v, want not found", err)
	}
	if _, err := reloaded.validationResults.Get(ctx, common.ValidationResultID("validation-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing validation result get err = %v, want not found", err)
	}
	if _, err := reloaded.runtimeCheckpoints.Get(ctx, common.CheckpointID("checkpoint-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing runtime checkpoint get err = %v, want not found", err)
	}
	if _, err := reloaded.tickets.Get(ctx, common.TicketID("ticket-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing ticket get err = %v, want not found", err)
	}
	if _, err := reloaded.resolutions.GetByTicketID(ctx, common.TicketID("ticket-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing resolution get err = %v, want not found", err)
	}
	if _, err := reloaded.approvals.Get(ctx, common.ApprovalID("approval-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing approval get err = %v, want not found", err)
	}
}
