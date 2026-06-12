package repository_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
		return file.NewSoulRepository(filepath.Join(t.TempDir(), "souls"))
	})
}

func TestWorkspaceTemplateRepositoryContract_Memory(t *testing.T) {
	testWorkspaceTemplateRepositoryContract(t, func() repository.WorkspaceTemplateRepository {
		return memory.NewWorkspaceTemplateRepository()
	})
}

func TestWorkspaceTemplateRepositoryContract_File(t *testing.T) {
	testWorkspaceTemplateRepositoryContract(t, func() repository.WorkspaceTemplateRepository {
		return file.NewWorkspaceTemplateRepository(filepath.Join(t.TempDir(), "workspace_templates.json"))
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

func TestMemoryRecallTraceRepositoryContract_Memory(t *testing.T) {
	testMemoryRecallTraceRepositoryContract(t, func() repository.MemoryRecallTraceRepository {
		return memory.NewMemoryRecallTraceRepository()
	})
}

func TestMemoryRecallTraceRepositoryContract_File(t *testing.T) {
	testMemoryRecallTraceRepositoryContract(t, func() repository.MemoryRecallTraceRepository {
		return file.NewMemoryRecallTraceRepository(filepath.Join(t.TempDir(), "memory_recall_trace.json"))
	})
}

func TestAutoGovernanceTraceRepositoryContract_Memory(t *testing.T) {
	testAutoGovernanceTraceRepositoryContract(t, func() repository.AutoGovernanceTraceRepository {
		return memory.NewAutoGovernanceTraceRepository()
	})
}

func TestAutoGovernanceTraceRepositoryContract_File(t *testing.T) {
	testAutoGovernanceTraceRepositoryContract(t, func() repository.AutoGovernanceTraceRepository {
		return file.NewAutoGovernanceTraceRepository(filepath.Join(t.TempDir(), "auto_governance_trace.json"))
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

func TestMCPCapabilityRepositoryContract_Memory(t *testing.T) {
	testMCPCapabilityRepositoryContract(t, func() repository.MCPCapabilityRepository {
		return memory.NewMCPCapabilityRepository()
	})
}

func TestMCPServerRepositoryContract_File(t *testing.T) {
	testMCPServerRepositoryContract(t, func() repository.MCPServerRepository {
		return file.NewMCPServerRepository(filepath.Join(t.TempDir(), "mcp_server.json"))
	})
}

func TestMCPCapabilityRepositoryContract_File(t *testing.T) {
	testMCPCapabilityRepositoryContract(t, func() repository.MCPCapabilityRepository {
		return file.NewMCPCapabilityRepository(filepath.Join(t.TempDir(), "mcp_capability.json"))
	})
}

func TestResultRepositoryContract_Memory(t *testing.T) {
	testResultRepositoryContract(t, func() repository.ResultRepository {
		return memory.NewResultRepository()
	})
}

func TestResultRepositoryContract_File(t *testing.T) {
	testResultRepositoryContract(t, func() repository.ResultRepository {
		return file.NewResultRepository(filepath.Join(t.TempDir(), "result.json"))
	})
}

func TestArtifactRepositoryContract_Memory(t *testing.T) {
	testArtifactRepositoryContract(t, func() repository.ArtifactRepository {
		return memory.NewArtifactRepository()
	})
}

func TestArtifactRepositoryContract_File(t *testing.T) {
	testArtifactRepositoryContract(t, func() repository.ArtifactRepository {
		return file.NewArtifactRepository(filepath.Join(t.TempDir(), "artifact.json"))
	})
}

func TestEventRecordRepositoryContract_Memory(t *testing.T) {
	testEventRecordRepositoryContract(t, func() repository.EventRecordRepository {
		return memory.NewEventRecordRepository()
	})
}

func TestEventRecordRepositoryContract_File(t *testing.T) {
	testEventRecordRepositoryContract(t, func() repository.EventRecordRepository {
		return file.NewEventRecordRepository(filepath.Join(t.TempDir(), "event_record.json"))
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

func TestArtifactContinuationRepositoryContract_Memory(t *testing.T) {
	testArtifactContinuationRepositoryContract(t, func() repository.ArtifactContinuationRepository {
		return memory.NewArtifactContinuationRepository()
	})
}

func TestArtifactContinuationRepositoryContract_File(t *testing.T) {
	testArtifactContinuationRepositoryContract(t, func() repository.ArtifactContinuationRepository {
		return file.NewArtifactContinuationRepository(filepath.Join(t.TempDir(), "artifact-continuations"))
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
		souls:                 memory.NewSoulRepository(),
		memoryItems:           memory.NewMemoryItemRepository(),
		memoryRecallTraces:    memory.NewMemoryRecallTraceRepository(),
		autoGovernanceTraces:  memory.NewAutoGovernanceTraceRepository(),
		skills:                memory.NewSkillRepository(),
		mcpServers:            memory.NewMCPServerRepository(),
		mcpCapabilities:       memory.NewMCPCapabilityRepository(),
		results:               memory.NewResultRepository(),
		artifacts:             memory.NewArtifactRepository(),
		eventRecords:          memory.NewEventRecordRepository(),
		workspaces:            memory.NewWorkspaceRepository(),
		goals:                 memory.NewGoalRepository(),
		dods:                  memory.NewDoDRepository(),
		runs:                  memory.NewRunRepository(),
		steps:                 memory.NewStepRepository(),
		toolCalls:             memory.NewToolCallRepository(),
		patches:               memory.NewPatchRepository(),
		validationResults:     memory.NewValidationResultRepository(),
		runtimeCheckpoints:    memory.NewRuntimeCheckpointRepository(),
		artifactContinuations: memory.NewArtifactContinuationRepository(),
		tickets:               memory.NewDecisionTicketRepository(),
		resolutions:           memory.NewDecisionResolutionRepository(),
		approvals:             memory.NewApprovalRepository(),
	}
	testRepositoryBundleContract(t, bundle, func() repositoryBundle { return bundle })
}

func TestRepositoryBundleContract_File(t *testing.T) {
	dir := t.TempDir()
	build := func() repositoryBundle {
		return repositoryBundle{
			souls:                 file.NewSoulRepository(filepath.Join(dir, "souls")),
			memoryItems:           file.NewMemoryItemRepository(filepath.Join(dir, "memory_item.json")),
			memoryRecallTraces:    file.NewMemoryRecallTraceRepository(filepath.Join(dir, "memory_recall_trace.json")),
			autoGovernanceTraces:  file.NewAutoGovernanceTraceRepository(filepath.Join(dir, "auto_governance_trace.json")),
			skills:                file.NewSkillRepository(filepath.Join(dir, "skill.json")),
			mcpServers:            file.NewMCPServerRepository(filepath.Join(dir, "mcp_server.json")),
			mcpCapabilities:       file.NewMCPCapabilityRepository(filepath.Join(dir, "mcp_capability.json")),
			results:               file.NewResultRepository(filepath.Join(dir, "result.json")),
			artifacts:             file.NewArtifactRepository(filepath.Join(dir, "artifact.json")),
			eventRecords:          file.NewEventRecordRepository(filepath.Join(dir, "event_record.json")),
			workspaces:            file.NewWorkspaceRepository(filepath.Join(dir, "workspace.json")),
			goals:                 file.NewGoalRepository(filepath.Join(dir, "goal.json")),
			dods:                  file.NewDoDRepository(filepath.Join(dir, "dod.json")),
			runs:                  file.NewRunRepository(filepath.Join(dir, "run.json")),
			steps:                 file.NewStepRepository(filepath.Join(dir, "step.json")),
			toolCalls:             file.NewToolCallRepository(filepath.Join(dir, "tool_call.json")),
			patches:               file.NewPatchRepository(filepath.Join(dir, "patch.json")),
			validationResults:     file.NewValidationResultRepository(filepath.Join(dir, "validation_result.json")),
			runtimeCheckpoints:    file.NewRuntimeCheckpointRepository(filepath.Join(dir, "runtime_checkpoint.json")),
			artifactContinuations: file.NewArtifactContinuationRepository(filepath.Join(dir, "artifact-continuations")),
			tickets:               file.NewDecisionTicketRepository(filepath.Join(dir, "ticket.json")),
			resolutions:           file.NewDecisionResolutionRepository(filepath.Join(dir, "resolution.json")),
			approvals:             file.NewApprovalRepository(filepath.Join(dir, "approval.json")),
		}
	}
	testRepositoryBundleContract(t, build(), build)
}

type repositoryBundle struct {
	souls                 repository.SoulRepository
	memoryItems           repository.MemoryItemRepository
	memoryRecallTraces    repository.MemoryRecallTraceRepository
	autoGovernanceTraces  repository.AutoGovernanceTraceRepository
	skills                repository.SkillRepository
	mcpServers            repository.MCPServerRepository
	mcpCapabilities       repository.MCPCapabilityRepository
	results               repository.ResultRepository
	artifacts             repository.ArtifactRepository
	eventRecords          repository.EventRecordRepository
	workspaces            repository.WorkspaceRepository
	goals                 repository.GoalRepository
	dods                  repository.DoDRepository
	runs                  repository.RunRepository
	steps                 repository.StepRepository
	toolCalls             repository.ToolCallRepository
	patches               repository.PatchRepository
	validationResults     repository.ValidationResultRepository
	runtimeCheckpoints    repository.RuntimeCheckpointRepository
	artifactContinuations repository.ArtifactContinuationRepository
	tickets               repository.DecisionTicketRepository
	resolutions           repository.DecisionResolutionRepository
	approvals             repository.ApprovalRepository
}

func testSkillRepositoryContract(t *testing.T, factory func() repository.SkillRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &skillmodel.Skill{
		ID:                     common.SkillID("skill-contract"),
		Name:                   "go service delivery",
		Version:                "v1",
		Description:            "deliver Go service changes",
		TriggerRules:           []string{"go", "service"},
		Instruction:            "Keep changes small and verified.",
		ReferenceRefs:          []string{"docs/structure.md"},
		RequiredToolKinds:      []string{"shell/command"},
		RequiredMCPServerRefs:  []string{"mcp-contract"},
		RequiredCapabilityRefs: []string{"mcp-contract:tool:repo.summary"},
		Constraints:            []string{"cite evidence"},
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
	got.RequiredCapabilityRefs[0] = "mutated"
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get skill fresh: %v", err)
	}
	if fresh.TriggerRules[0] != "go" {
		t.Fatalf("skill clone isolation failed, triggerRules = %+v", fresh.TriggerRules)
	}
	if fresh.RequiredCapabilityRefs[0] != "mcp-contract:tool:repo.summary" {
		t.Fatalf("skill clone isolation failed, requiredCapabilityRefs = %+v", fresh.RequiredCapabilityRefs)
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

func testMCPCapabilityRepositoryContract(t *testing.T, factory func() repository.MCPCapabilityRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &mcpmodel.Capability{
		ID:             common.MCPCapabilityID("mcp-contract:tool:repo.summary"),
		ServerID:       common.MCPServerID("mcp-contract"),
		Kind:           "tool",
		Name:           "repo.summary",
		DisplayName:    "仓库摘要",
		Description:    "获取代码仓库结构、关键文件和当前变更概览。",
		Category:       "代码仓库",
		Tags:           []string{"repo", "summary"},
		UsageHint:      "规划前收集上下文。",
		InputSummary:   "可选仓库路径。",
		OutputSummary:  "返回仓库结构和变更摘要。",
		StageHints:     []string{"planning"},
		RiskLevel:      "read_only",
		Schema:         map[string]any{"type": "object"},
		Enabled:        true,
		ApprovalPolicy: "operator",
		Status:         "draft",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create mcp capability: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate mcp capability create err = %v, want conflict", err)
	}
	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get mcp capability: %v", err)
	}
	got.Tags[0] = "mutated"
	got.Schema["type"] = "mutated"
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get mcp capability fresh: %v", err)
	}
	if fresh.Tags[0] != "repo" || fresh.Schema["type"] != "object" {
		t.Fatalf("mcp capability clone isolation failed: %+v", fresh)
	}
	fresh.Enabled = false
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update mcp capability: %v", err)
	}
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list mcp capabilities: %v", err)
	}
	if len(list) != 1 || list[0].Enabled {
		t.Fatalf("mcp capabilities = %+v, want disabled capability", list)
	}
	if _, err := repo.Get(ctx, common.MCPCapabilityID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing mcp capability err = %v, want not found", err)
	}
}

func testResultRepositoryContract(t *testing.T, factory func() repository.ResultRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &resultmodel.Result{
		ID:           common.ResultID("result-contract"),
		WorkspaceID:  common.WorkspaceID("ws-contract"),
		RunID:        common.RunID("run-contract"),
		GoalID:       common.GoalID("goal-contract"),
		DoDID:        common.DoDID("dod-contract"),
		State:        common.ResultStateAccepted,
		Title:        "accepted outcome",
		Summary:      "Run completed with evidence-backed result.",
		SourceType:   "dod_assessment",
		SourceRef:    "step-contract",
		EvidenceRefs: []string{"step-contract", "validation-contract"},
		ArtifactRefs: []common.ArtifactID{common.ArtifactID("artifact-contract")},
		DoDAssessment: resultmodel.DoDAssessment{
			Satisfied: true,
			Summary:   "Run completed with evidence-backed result.",
			Items: []resultmodel.DoDAssessmentItem{
				{
					Item:         "validation passes",
					State:        "satisfied",
					EvidenceRefs: []string{"validation-contract"},
				},
			},
		},
		CreatedAt: "2026-04-28T00:00:00Z",
		UpdatedAt: "2026-04-28T00:00:00Z",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create result: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate result create err = %v, want conflict", err)
	}
	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get result: %v", err)
	}
	got.EvidenceRefs[0] = "mutated"
	got.ArtifactRefs[0] = common.ArtifactID("mutated")
	got.DoDAssessment.Items[0].EvidenceRefs[0] = "mutated"
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get result fresh: %v", err)
	}
	if fresh.EvidenceRefs[0] != "step-contract" ||
		fresh.ArtifactRefs[0] != "artifact-contract" ||
		fresh.DoDAssessment.Items[0].EvidenceRefs[0] != "validation-contract" {
		t.Fatalf("result clone isolation failed: %+v", fresh)
	}
	fresh.Summary = "updated accepted outcome"
	fresh.UpdatedAt = "2026-04-28T00:01:00Z"
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update result: %v", err)
	}
	byWorkspace, err := repo.ListByWorkspace(ctx, item.WorkspaceID)
	if err != nil {
		t.Fatalf("list results by workspace: %v", err)
	}
	if len(byWorkspace) != 1 || byWorkspace[0].Summary != "updated accepted outcome" {
		t.Fatalf("workspace results = %+v, want updated result", byWorkspace)
	}
	byRun, err := repo.ListByRun(ctx, item.RunID)
	if err != nil {
		t.Fatalf("list results by run: %v", err)
	}
	if len(byRun) != 1 || byRun[0].ID != item.ID {
		t.Fatalf("run results = %+v, want %s", byRun, item.ID)
	}
	empty, err := repo.ListByRun(ctx, common.RunID("missing"))
	if err != nil {
		t.Fatalf("list missing run results: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("missing run results = %+v, want empty", empty)
	}
	if _, err := repo.Get(ctx, common.ResultID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing result err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &resultmodel.Result{ID: common.ResultID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("update missing result err = %v, want not found", err)
	}
}

func testArtifactRepositoryContract(t *testing.T, factory func() repository.ArtifactRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &resultmodel.Artifact{
		ID:                 common.ArtifactID("artifact-contract"),
		WorkspaceID:        common.WorkspaceID("ws-contract"),
		RunID:              common.RunID("run-contract"),
		ResultID:           common.ResultID("result-contract"),
		StepID:             common.StepID("step-contract"),
		ToolCallID:         common.ToolCallID("tool-contract"),
		PatchID:            common.PatchID("patch-contract"),
		ValidationResultID: common.ValidationResultID("validation-contract"),
		ArtifactType:       "report",
		Name:               "acceptance report",
		Summary:            "Final acceptance report",
		URI:                "workspace://results/acceptance.md",
		ContentType:        "text/markdown",
		State:              common.ArtifactStateAvailable,
		EvidenceRefs:       []string{"tool-contract", "validation-contract"},
		Metadata:           map[string]any{"kind": "acceptance", "score": float64(1)},
		CreatedAt:          "2026-04-28T00:00:00Z",
		UpdatedAt:          "2026-04-28T00:00:00Z",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate artifact create err = %v, want conflict", err)
	}
	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	got.EvidenceRefs[0] = "mutated"
	got.Metadata["kind"] = "mutated"
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get artifact fresh: %v", err)
	}
	if fresh.EvidenceRefs[0] != "tool-contract" || fresh.Metadata["kind"] != "acceptance" {
		t.Fatalf("artifact clone isolation failed: %+v", fresh)
	}
	fresh.Summary = "Updated final report"
	fresh.UpdatedAt = "2026-04-28T00:01:00Z"
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update artifact: %v", err)
	}
	byWorkspace, err := repo.ListByWorkspace(ctx, item.WorkspaceID)
	if err != nil {
		t.Fatalf("list artifacts by workspace: %v", err)
	}
	if len(byWorkspace) != 1 || byWorkspace[0].Summary != "Updated final report" {
		t.Fatalf("workspace artifacts = %+v, want updated artifact", byWorkspace)
	}
	byRun, err := repo.ListByRun(ctx, item.RunID)
	if err != nil {
		t.Fatalf("list artifacts by run: %v", err)
	}
	if len(byRun) != 1 || byRun[0].ID != item.ID {
		t.Fatalf("run artifacts = %+v, want %s", byRun, item.ID)
	}
	byResult, err := repo.ListByResult(ctx, item.ResultID)
	if err != nil {
		t.Fatalf("list artifacts by result: %v", err)
	}
	if len(byResult) != 1 || byResult[0].ID != item.ID {
		t.Fatalf("result artifacts = %+v, want %s", byResult, item.ID)
	}
	empty, err := repo.ListByResult(ctx, common.ResultID("missing"))
	if err != nil {
		t.Fatalf("list missing result artifacts: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("missing result artifacts = %+v, want empty", empty)
	}
	if _, err := repo.Get(ctx, common.ArtifactID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing artifact err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &resultmodel.Artifact{ID: common.ArtifactID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("update missing artifact err = %v, want not found", err)
	}
}

func testEventRecordRepositoryContract(t *testing.T, factory func() repository.EventRecordRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	first := &eventstreammodel.EventRecord{
		ID:             common.EventRecordID("event-record-1"),
		Version:        "workorch.event.v1",
		Topic:          "/run/state/changed",
		OccurredAt:     "2026-04-28T00:00:00Z",
		SourceModule:   "kernel/run",
		SubjectType:    "run",
		SubjectID:      "run-contract",
		WorkspaceID:    common.WorkspaceID("ws-contract"),
		RunID:          common.RunID("run-contract"),
		Payload:        map[string]any{"state": "running"},
		PayloadSummary: map[string]any{"fieldCount": 1},
		PayloadRef:     "payloads/event-record-1.json",
	}
	second := &eventstreammodel.EventRecord{
		ID:           common.EventRecordID("event-record-2"),
		Version:      "workorch.event.v1",
		Topic:        "/step/state/changed",
		OccurredAt:   "2026-04-28T00:01:00Z",
		SourceModule: "kernel/orchestrator",
		SubjectType:  "step",
		SubjectID:    "step-contract",
		WorkspaceID:  common.WorkspaceID("ws-contract"),
		RunID:        common.RunID("run-contract"),
		StepID:       common.StepID("step-contract"),
		Payload:      map[string]any{"state": "completed"},
	}
	if err := repo.Append(ctx, first); err != nil {
		t.Fatalf("append first event record: %v", err)
	}
	if err := repo.Append(ctx, first); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate event record append err = %v, want conflict", err)
	}
	if err := repo.Append(ctx, second); err != nil {
		t.Fatalf("append second event record: %v", err)
	}
	got, err := repo.Get(ctx, first.ID)
	if err != nil {
		t.Fatalf("get event record: %v", err)
	}
	got.Payload["state"] = "mutated"
	fresh, err := repo.Get(ctx, first.ID)
	if err != nil {
		t.Fatalf("get event record fresh: %v", err)
	}
	if fresh.Payload["state"] != "running" {
		t.Fatalf("event record clone isolation failed: %+v", fresh)
	}
	if fresh.PayloadSummary["fieldCount"] != 1 && fresh.PayloadSummary["fieldCount"] != float64(1) || fresh.PayloadRef != "payloads/event-record-1.json" {
		t.Fatalf("event record summary/ref = %+v/%q, want persisted summary/ref", fresh.PayloadSummary, fresh.PayloadRef)
	}
	byRun, err := repo.List(ctx, repository.EventRecordQuery{RunID: first.RunID})
	if err != nil {
		t.Fatalf("list events by run: %v", err)
	}
	if len(byRun) != 2 || byRun[0].ID != first.ID || byRun[1].ID != second.ID {
		t.Fatalf("run events = %+v, want ordered first and second", byRun)
	}
	if len(byRun[0].Payload) != 0 {
		t.Fatalf("run events payload = %+v, want lightweight list result without payload", byRun[0].Payload)
	}
	byRunWithPayload, err := repo.List(ctx, repository.EventRecordQuery{RunID: first.RunID, IncludePayload: true})
	if err != nil {
		t.Fatalf("list events by run with payload: %v", err)
	}
	if len(byRunWithPayload) != 2 || byRunWithPayload[0].Payload["state"] != "running" {
		t.Fatalf("run events with payload = %+v, want hydrated payload", byRunWithPayload)
	}
	byTopic, err := repo.List(ctx, repository.EventRecordQuery{Topic: second.Topic})
	if err != nil {
		t.Fatalf("list events by topic: %v", err)
	}
	if len(byTopic) != 1 || byTopic[0].ID != second.ID {
		t.Fatalf("topic events = %+v, want second", byTopic)
	}
	afterCursor, err := repo.List(ctx, repository.EventRecordQuery{RunID: first.RunID, CursorAfterID: first.ID, Limit: 1})
	if err != nil {
		t.Fatalf("list events after cursor: %v", err)
	}
	if len(afterCursor) != 1 || afterCursor[0].ID != second.ID {
		t.Fatalf("cursor events = %+v, want second", afterCursor)
	}
	if _, err := repo.Get(ctx, common.EventRecordID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing event record err = %v, want not found", err)
	}
}

func testMemoryItemRepositoryContract(t *testing.T, factory func() repository.MemoryItemRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &memorymodel.MemoryItem{
		ID:                  common.MemoryID("memory-contract"),
		WorkspaceID:         common.WorkspaceID("ws-contract"),
		RunID:               common.RunID("run-contract"),
		SourceType:          "run",
		SourceRef:           "run-contract",
		MemoryType:          "run_summary",
		Content:             "Local verification completed through workorchd APIs.",
		Summary:             "workorchd local verification completed",
		Tags:                []string{"local", "verification"},
		Importance:          9,
		Confidence:          8,
		Visibility:          "workspace",
		EmbeddingProviderID: "embed-local",
		EmbeddingModel:      "embed-model",
		EmbeddingText:       "workorchd local verification completed",
		Vector:              []float64{0.9, 0.1, 0},
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
	got.Vector[0] = 0

	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get memory item fresh: %v", err)
	}
	if fresh.Tags[0] != "local" {
		t.Fatalf("memory tags clone isolation failed, tags = %+v", fresh.Tags)
	}
	if fresh.Vector[0] != 0.9 || fresh.EmbeddingProviderID != "embed-local" {
		t.Fatalf("memory vector clone isolation failed, item = %+v", fresh)
	}

	fresh.Summary = "updated local verification summary"
	fresh.Importance = 10
	fresh.Vector = []float64{0.1, 0.95, 0}
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
	vectorSearch, err := repo.Search(ctx, repository.MemorySearchQuery{
		WorkspaceID: item.WorkspaceID,
		Vector:      []float64{0, 1, 0},
		MinScore:    0.8,
		Limit:       1,
	})
	if err != nil {
		t.Fatalf("vector search memory: %v", err)
	}
	if len(vectorSearch) != 1 || vectorSearch[0].ID != item.ID || vectorSearch[0].VectorScore <= 0.8 {
		t.Fatalf("vector search memory = %+v, want %q with score", vectorSearch, item.ID)
	}

	if _, err := repo.Get(ctx, common.MemoryID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing memory err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &memorymodel.MemoryItem{ID: common.MemoryID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("update missing memory err = %v, want not found", err)
	}
}

func testMemoryRecallTraceRepositoryContract(t *testing.T, factory func() repository.MemoryRecallTraceRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &memorymodel.MemoryRecallTrace{
		ID:                     common.MemoryRecallTraceID("memory-recall-contract"),
		WorkspaceID:            common.WorkspaceID("ws-contract"),
		RunID:                  common.RunID("run-contract"),
		StepID:                 common.StepID("run-contract/step-1"),
		GraphNodeID:            "node-planning",
		Stage:                  "planning",
		Source:                 "activation",
		Policy:                 "semantic_vector_search",
		Status:                 "selected",
		ProviderID:             "embedding-contract",
		Operation:              "embeddings",
		QueryTextPreview:       "Goal: use memory",
		QueryTextSha256:        "sha-contract",
		MinScore:               0.1,
		Limit:                  8,
		EmbeddingModel:         "embedding-model",
		VectorDimension:        3,
		RefreshSource:          "repair",
		SelectionPolicyVersion: "workorch.memory.step_input.v1",
		CandidateMemoryIDs:     []common.MemoryID{common.MemoryID("memory-contract"), common.MemoryID("memory-other")},
		SelectedMemoryIDs:      []common.MemoryID{common.MemoryID("memory-contract")},
		ExcludedMemoryIDs:      []common.MemoryID{common.MemoryID("memory-other")},
		TypeQuota:              map[string]int{"run_summary": 1},
		SelectionScores: []memorymodel.MemoryRecallSelectionScore{
			{
				ID:     common.MemoryID("memory-contract"),
				Score:  100,
				Reason: "current_run",
			},
		},
		RetrievedMemoryIDs: []common.MemoryID{common.MemoryID("memory-contract")},
		RetrievedMemoryHits: []memorymodel.MemoryRecallHit{
			{
				ID:              common.MemoryID("memory-contract"),
				Summary:         "contract memory",
				VectorScore:     0.98,
				VectorDimension: 3,
			},
		},
		OccurredAt:  "2026-05-04T00:00:00Z",
		EvidenceRef: "memory-recall-contract",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create memory recall trace: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate memory recall create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get memory recall trace: %v", err)
	}
	got.RetrievedMemoryIDs[0] = common.MemoryID("mutated")
	got.RetrievedMemoryHits[0].Summary = "mutated"
	got.CandidateMemoryIDs[0] = common.MemoryID("mutated")
	got.SelectedMemoryIDs[0] = common.MemoryID("mutated")
	got.ExcludedMemoryIDs[0] = common.MemoryID("mutated")
	got.TypeQuota["run_summary"] = 9
	got.SelectionScores[0].Score = 9
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get memory recall trace fresh: %v", err)
	}
	if fresh.RetrievedMemoryIDs[0] != "memory-contract" ||
		fresh.RetrievedMemoryHits[0].Summary != "contract memory" ||
		fresh.CandidateMemoryIDs[0] != "memory-contract" ||
		fresh.SelectedMemoryIDs[0] != "memory-contract" ||
		fresh.ExcludedMemoryIDs[0] != "memory-other" ||
		fresh.TypeQuota["run_summary"] != 1 ||
		fresh.SelectionScores[0].Score != 100 {
		t.Fatalf("memory recall clone isolation failed: %+v", fresh)
	}

	fresh.Source = "step_input"
	fresh.Stage = "execution"
	fresh.Status = "degraded"
	fresh.Reason = "provider unavailable"
	if err := repo.Upsert(ctx, fresh); err != nil {
		t.Fatalf("upsert memory recall trace: %v", err)
	}
	byRun, err := repo.List(ctx, repository.MemoryRecallTraceQuery{RunID: item.RunID})
	if err != nil {
		t.Fatalf("list memory recall by run: %v", err)
	}
	if len(byRun) != 1 || byRun[0].Source != "step_input" {
		t.Fatalf("memory recall by run = %+v, want updated trace", byRun)
	}
	byStage, err := repo.List(ctx, repository.MemoryRecallTraceQuery{WorkspaceID: item.WorkspaceID, Stage: "execution", Source: "step_input", Status: "degraded"})
	if err != nil {
		t.Fatalf("list memory recall by stage/source/status: %v", err)
	}
	if len(byStage) != 1 || byStage[0].Reason != "provider unavailable" {
		t.Fatalf("memory recall by stage = %+v, want degraded trace", byStage)
	}

	if _, err := repo.Get(ctx, common.MemoryRecallTraceID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing memory recall err = %v, want not found", err)
	}
	if err := repo.Delete(ctx, item.ID); err != nil {
		t.Fatalf("delete memory recall trace: %v", err)
	}
	if _, err := repo.Get(ctx, item.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get deleted memory recall err = %v, want not found", err)
	}
}

func testAutoGovernanceTraceRepositoryContract(t *testing.T, factory func() repository.AutoGovernanceTraceRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &governancemodel.AutoGovernanceTrace{
		ID:                 common.AutoGovernanceTraceID("auto-governance-contract"),
		WorkspaceID:        common.WorkspaceID("ws-contract"),
		RunID:              common.RunID("run-contract"),
		StepID:             common.StepID("run-contract/step-1"),
		ItemType:           "approval",
		ItemID:             "approval-contract",
		Action:             "approve",
		RiskLevel:          "L1",
		Confidence:         0.96,
		PolicyMatched:      "read_only",
		PolicyBlocked:      "",
		BudgetBefore:       3,
		BudgetAfter:        2,
		Reason:             "auto-approved read-only operation",
		EvidenceRefs:       []string{"memory-recall-contract"},
		LLMProviderID:      "local",
		LLMModel:           "contract-model",
		LLMDecisionPreview: "approve read-only",
		Status:             "resolved",
		OccurredAt:         "2026-05-06T00:00:00Z",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create auto governance trace: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate auto governance trace create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get auto governance trace: %v", err)
	}
	got.EvidenceRefs[0] = "mutated"
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get auto governance trace fresh: %v", err)
	}
	if len(fresh.EvidenceRefs) != 1 || fresh.EvidenceRefs[0] != "memory-recall-contract" {
		t.Fatalf("auto governance clone isolation failed: %+v", fresh)
	}

	fresh.Status = "blocked"
	fresh.PolicyBlocked = "budget_exhausted"
	fresh.Confidence = 0.31
	if err := repo.Upsert(ctx, fresh); err != nil {
		t.Fatalf("upsert auto governance trace: %v", err)
	}
	byStatus, err := repo.List(ctx, repository.AutoGovernanceTraceQuery{
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		ItemType:    "approval",
		ItemID:      "approval-contract",
		Status:      "blocked",
	})
	if err != nil {
		t.Fatalf("list auto governance traces by status: %v", err)
	}
	if len(byStatus) != 1 || byStatus[0].PolicyBlocked != "budget_exhausted" {
		t.Fatalf("auto governance by status = %+v, want blocked trace", byStatus)
	}

	if _, err := repo.Get(ctx, common.AutoGovernanceTraceID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing auto governance trace err = %v, want not found", err)
	}
	if err := repo.Delete(ctx, item.ID); err != nil {
		t.Fatalf("delete auto governance trace: %v", err)
	}
	if _, err := repo.Get(ctx, item.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get deleted auto governance trace err = %v, want not found", err)
	}
}

func testSoulRepositoryContract(t *testing.T, factory func() repository.SoulRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &soulmodel.Soul{
		ID:                          common.SoulID("soul-contract"),
		Name:                        "engineering delivery",
		Description:                 "drives engineering delivery",
		BusinessDomains:             []string{"engineering"},
		SoulMD:                      "# Engineering Delivery\n\nPlan and verify before completion.\n\n## Planning\n\nUse small steps.\n\n## Decision\n\nAsk when information is missing.\n\n## Approval\n\nRequest approval for risky actions.\n\n## Validation\n\nRequire evidence.\n",
		AllowedSkillRefs:            []string{"skill-go"},
		AllowedMCPServerRefs:        []string{"mcp-repo"},
		AllowedCapabilityRefs:       []string{"mcp-repo:tool:repo.summary"},
		AllowedCapabilityCategories: []string{"代码仓库"},
		AllowedToolKinds:            []string{"shell/command"},
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
	got.AllowedCapabilityRefs[0] = "mutated"
	got.AllowedCapabilityCategories[0] = "mutated"

	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get soul fresh: %v", err)
	}
	if fresh.Name != "engineering delivery" || !strings.Contains(fresh.SoulMD, "Plan and verify") {
		t.Fatalf("clone isolation failed, soul = %+v", fresh)
	}
	if fresh.AllowedSkillRefs[0] != "skill-go" {
		t.Fatalf("skill refs clone isolation failed, refs = %+v", fresh.AllowedSkillRefs)
	}
	if fresh.AllowedCapabilityRefs[0] != "mcp-repo:tool:repo.summary" || fresh.AllowedCapabilityCategories[0] != "代码仓库" {
		t.Fatalf("capability refs clone isolation failed, refs/categories = %+v/%+v", fresh.AllowedCapabilityRefs, fresh.AllowedCapabilityCategories)
	}

	fresh.Description = "updated description"
	fresh.SoulMD = "# Engineering Delivery\n\nUpdated soul definition.\n"
	fresh.AllowedToolKinds = []string{"shell/command", "patch/apply"}
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update soul: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list soul: %v", err)
	}
	if len(list) != 1 || list[0].Description != "updated description" || !strings.Contains(list[0].SoulMD, "Updated soul definition") || len(list[0].AllowedToolKinds) != 2 {
		t.Fatalf("list soul = %+v, want updated soul", list)
	}

	if _, err := repo.Get(ctx, common.SoulID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing soul err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &soulmodel.Soul{ID: common.SoulID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("update missing soul err = %v, want not found", err)
	}
	if err := repo.Delete(ctx, item.ID); err != nil {
		t.Fatalf("delete soul: %v", err)
	}
	if _, err := repo.Get(ctx, item.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get deleted soul err = %v, want not found", err)
	}
	if err := repo.Delete(ctx, item.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("delete missing soul err = %v, want not found", err)
	}
}

func testWorkspaceTemplateRepositoryContract(t *testing.T, factory func() repository.WorkspaceTemplateRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &repository.WorkspaceTemplate{
		ID:            "workspace-template-contract",
		Name:          "contract template",
		Description:   "template contract description",
		GoalText:      "Deliver a verified workspace",
		DoDItems:      []string{"goal is defined", "result is accepted"},
		DefaultSoulID: "soul-contract",
		SoulBindings: map[string]string{
			"planning":  "soul-contract",
			"execution": "soul-contract",
		},
		EvidencePolicies: []map[string]any{
			{
				"id":              "policy-contract",
				"zeroWorkMarkers": []any{"no work"},
			},
		},
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create workspace template: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get workspace template: %v", err)
	}
	got.Name = "mutated"
	got.DoDItems[0] = "mutated"
	got.SoulBindings["planning"] = "mutated"
	got.EvidencePolicies[0]["id"] = "mutated"
	got.EvidencePolicies[0]["zeroWorkMarkers"].([]any)[0] = "mutated"

	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get workspace template fresh: %v", err)
	}
	if fresh.Name != "contract template" || fresh.DoDItems[0] != "goal is defined" || fresh.SoulBindings["planning"] != "soul-contract" || fresh.EvidencePolicies[0]["id"] != "policy-contract" || fresh.EvidencePolicies[0]["zeroWorkMarkers"].([]any)[0] != "no work" {
		t.Fatalf("clone isolation failed, workspace template = %+v", fresh)
	}

	fresh.Description = "updated description"
	fresh.DoDItems = append(fresh.DoDItems, "evidence is available")
	fresh.SoulBindings["validation"] = "soul-contract"
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update workspace template: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list workspace template: %v", err)
	}
	if len(list) != 1 || list[0].Description != "updated description" || len(list[0].DoDItems) != 3 || list[0].SoulBindings["validation"] != "soul-contract" {
		t.Fatalf("list workspace template = %+v, want updated template", list)
	}

	if _, err := repo.Get(ctx, "missing"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get missing workspace template err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &repository.WorkspaceTemplate{ID: "missing"}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("update missing workspace template err = %v, want not found", err)
	}
	if err := repo.Delete(ctx, item.ID); err != nil {
		t.Fatalf("delete workspace template: %v", err)
	}
	if _, err := repo.Get(ctx, item.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get deleted workspace template err = %v, want not found", err)
	}
	if err := repo.Delete(ctx, item.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("delete missing workspace template err = %v, want not found", err)
	}
}

func testWorkspaceRepositoryContract(t *testing.T, factory func() repository.WorkspaceRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &workspacemodel.Workspace{
		ID:                   common.WorkspaceID("ws-contract"),
		Title:                "workspace-contract",
		Description:          "workspace contract description",
		DeliverableDirectory: "/tmp/workorch/ws-contract",
		State:                common.WorkspaceStateActive,
		EvidencePolicies: []map[string]any{
			{
				"id":              "workspace-policy-contract",
				"zeroWorkMarkers": []any{"no work"},
			},
		},
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
	got.EvidencePolicies[0]["id"] = "mutated"
	got.EvidencePolicies[0]["zeroWorkMarkers"].([]any)[0] = "mutated"

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
	if fresh.DeliverableDirectory != "/tmp/workorch/ws-contract" {
		t.Fatalf("deliverableDirectory = %q, want /tmp/workorch/ws-contract", fresh.DeliverableDirectory)
	}
	if fresh.EvidencePolicies[0]["id"] != "workspace-policy-contract" || fresh.EvidencePolicies[0]["zeroWorkMarkers"].([]any)[0] != "no work" {
		t.Fatalf("evidencePolicies clone isolation failed: %+v", fresh.EvidencePolicies)
	}

	fresh.ActiveRunID = common.RunID("run-1")
	fresh.Description = "workspace contract updated description"
	fresh.DeliverableDirectory = "/tmp/workorch/ws-contract-updated"
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
	if updated.DeliverableDirectory != "/tmp/workorch/ws-contract-updated" {
		t.Fatalf("updated deliverableDirectory = %q, want /tmp/workorch/ws-contract-updated", updated.DeliverableDirectory)
	}

	second := &workspacemodel.Workspace{
		ID:                   common.WorkspaceID("ws-contract-2"),
		Title:                "workspace-contract-2",
		DeliverableDirectory: "/tmp/workorch/ws-contract-2",
		State:                common.WorkspaceStateActive,
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
	fresh.Text = "updated contract goal"
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("update goal: %v", err)
	}
	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated goal: %v", err)
	}
	if updated.Text != "updated contract goal" {
		t.Fatalf("goal text = %q, want updated contract goal", updated.Text)
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
	if err := repo.Update(ctx, &workspacemodel.Goal{ID: common.GoalID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing update err = %v, want not found", err)
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
	byWorkspace, err := repo.ListByWorkspaceID(ctx, item.WorkspaceID)
	if err != nil {
		t.Fatalf("list runs by workspace: %v", err)
	}
	if len(byWorkspace) != 2 || byWorkspace[0].ID != item.ID || byWorkspace[1].ID != second.ID {
		t.Fatalf("runs by workspace = %+v, want %q/%q", byWorkspace, item.ID, second.ID)
	}
	missingWorkspace, err := repo.ListByWorkspaceID(ctx, common.WorkspaceID("ws-missing"))
	if err != nil {
		t.Fatalf("list runs by missing workspace: %v", err)
	}
	if len(missingWorkspace) != 0 {
		t.Fatalf("runs by missing workspace = %+v, want empty", missingWorkspace)
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
		Kind:  "llm/workspace_execution",
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
	fresh.Output = map[string]any{
		"status":           "completed",
		"failureSignature": "class=runtime_failed|targetRef=main.go",
		"result":           "done",
	}
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
		Kind:  "llm/workspace_execution",
		State: common.StepStateCompleted,
		Input: map[string]any{"workspaceId": "ws-contract", "previousStepId": "run-contract/step-1"},
	}); err != nil {
		t.Fatalf("create second step: %v", err)
	}
	if err := repo.Create(ctx, &runmodel.Step{
		ID:    common.StepID("run-other/step-1"),
		RunID: common.RunID("run-other"),
		Kind:  "llm/workspace_execution",
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

	summaries, err := repo.ListByRunSummary(ctx, common.RunID("run-contract"))
	if err != nil {
		t.Fatalf("list step summaries by run: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("list step summaries len = %d, want 2", len(summaries))
	}
	if summaries[0].Input["workspaceId"] != "ws-contract" {
		t.Fatalf("summary input workspaceId = %v, want ws-contract", summaries[0].Input["workspaceId"])
	}
	if summaries[0].Output["status"] != "completed" {
		t.Fatalf("summary output status = %v, want completed", summaries[0].Output["status"])
	}
	if summaries[0].Output["failureSignature"] != "class=runtime_failed|targetRef=main.go" {
		t.Fatalf("summary output failureSignature = %v, want retained signature", summaries[0].Output["failureSignature"])
	}
	if _, ok := summaries[0].Output["result"]; ok {
		t.Fatalf("summary retained non-whitelisted output result, want omitted")
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
		Diff:        "--- a/internal/example.go\n+++ b/internal/example.go\n@@ -1 +1 @@\n-before\n+after\n",
		Summary:     "initial patch",
		CoverageTargets: []any{map[string]any{
			"goalRequirementId": "goal-contract",
			"targetText":        "contract target",
		}},
		State:     common.PatchStateCreated,
		CreatedAt: "2026-04-25T00:00:00Z",
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
	if updated.Diff != item.Diff {
		t.Fatalf("patch diff = %q, want persisted diff", updated.Diff)
	}
	targets, ok := updated.CoverageTargets.([]any)
	if !ok || len(targets) != 1 {
		t.Fatalf("coverage targets = %#v, want persisted target metadata", updated.CoverageTargets)
	}
	target, ok := targets[0].(map[string]any)
	if !ok || target["goalRequirementId"] != "goal-contract" || target["targetText"] != "contract target" {
		t.Fatalf("coverage target = %#v, want preserved structured target", updated.CoverageTargets)
	}

	second := &runmodel.Patch{
		ID:          common.PatchID("patch-contract-2"),
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		StepID:      common.StepID("run-contract/step-2"),
		PatchType:   "file_replace",
		Content:     "replacement content\n",
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

func testArtifactContinuationRepositoryContract(t *testing.T, factory func() repository.ArtifactContinuationRepository) {
	t.Helper()
	ctx := context.Background()
	repo := factory()

	item := &orchestratormodel.ArtifactContinuation{
		ID:                  common.ArtifactContinuationID("artifact-continuation-contract-001"),
		WorkspaceID:         common.WorkspaceID("ws-contract"),
		RunID:               common.RunID("run-contract"),
		RootStepID:          common.StepID("run-contract/step-1"),
		CurrentStepID:       common.StepID("run-contract/step-1"),
		ArtifactID:          "artifact-001",
		TargetRef:           "deliverable/example.txt",
		Operation:           "replace",
		OutputMode:          "incrementalArtifact",
		CoverageTargets:     []string{"coverage-001"},
		State:               common.ArtifactContinuationStatePlanning,
		ExpectedSequence:    1,
		ReceivedSequences:   []int{1},
		ReceivedChunkHashes: map[int]string{1: "hash-001"},
		RetryCounters:       map[string]int{"hash_mismatch": 1},
		EvidenceRefs:        []string{"run-contract/step-1"},
		Metadata:            map[string]any{"scope": "contract"},
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create artifact continuation: %v", err)
	}
	if err := repo.Create(ctx, item); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate artifact continuation create err = %v, want conflict", err)
	}
	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get artifact continuation: %v", err)
	}
	got.Metadata["scope"] = "mutated"
	got.CoverageTargets[0] = "mutated"
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get fresh artifact continuation: %v", err)
	}
	if fresh.Metadata["scope"] != "contract" || fresh.CoverageTargets[0] != "coverage-001" || fresh.ReceivedChunkHashes[1] != "hash-001" ||
		fresh.CurrentStepID != item.CurrentStepID || fresh.OutputMode != "incrementalArtifact" {
		t.Fatalf("artifact continuation clone isolation failed: %+v", fresh)
	}

	plan := map[string]any{"artifacts": []any{map[string]any{"artifactId": "artifact-001"}}}
	if err := repo.SavePlan(ctx, item.ID, plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}
	plan["mutated"] = true
	storedPlan, err := repo.GetPlan(ctx, item.ID)
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if storedPlan["mutated"] != nil {
		t.Fatalf("stored plan leaked caller mutation: %+v", storedPlan)
	}

	chunk := &orchestratormodel.ArtifactContinuationChunk{
		ContinuationID:   item.ID,
		Sequence:         1,
		ArtifactID:       item.ArtifactID,
		TargetRef:        item.TargetRef,
		ContentHash:      "hash-001",
		Bytes:            5,
		CurrentChunkTail: "hello",
		LLMCallRefs:      []string{"llm-call-001"},
		StepRefs:         []common.StepID{item.RootStepID},
		Validation:       map[string]any{"state": "accepted"},
		Metadata:         map[string]any{"scope": "chunk"},
	}
	if err := repo.SaveChunk(ctx, chunk, "hello"); err != nil {
		t.Fatalf("save chunk: %v", err)
	}
	chunk.Metadata["scope"] = "mutated"
	storedChunk, content, err := repo.GetChunk(ctx, item.ID, 1)
	if err != nil {
		t.Fatalf("get chunk: %v", err)
	}
	if content != "hello" || storedChunk.Metadata["scope"] != "chunk" {
		t.Fatalf("stored chunk/content = %+v/%q, want isolated chunk content", storedChunk, content)
	}
	chunks, err := repo.ListChunks(ctx, item.ID)
	if err != nil {
		t.Fatalf("list chunks: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Sequence != 1 {
		t.Fatalf("chunks = %+v, want sequence 1", chunks)
	}

	assembled := &orchestratormodel.ArtifactContinuationAssembledMetadata{
		ContinuationID:    item.ID,
		TargetRef:         item.TargetRef,
		Bytes:             5,
		RollingHash:       "rolling-hash",
		SourceChunkRefs:   []string{"001"},
		ApplyEvidenceRefs: []string{"apply-step"},
		Metadata:          map[string]any{"scope": "assembled"},
	}
	if err := repo.SaveAssembled(ctx, assembled, "hello"); err != nil {
		t.Fatalf("save assembled: %v", err)
	}
	storedAssembled, assembledContent, err := repo.GetAssembled(ctx, item.ID)
	if err != nil {
		t.Fatalf("get assembled: %v", err)
	}
	if assembledContent != "hello" || storedAssembled.RollingHash != "rolling-hash" {
		t.Fatalf("assembled = %+v/%q, want content and metadata", storedAssembled, assembledContent)
	}

	item.State = common.ArtifactContinuationStateChunking
	if err := repo.Update(ctx, item); err != nil {
		t.Fatalf("update artifact continuation: %v", err)
	}
	otherRun := &orchestratormodel.ArtifactContinuation{
		ID:          common.ArtifactContinuationID("artifact-continuation-contract-002"),
		WorkspaceID: item.WorkspaceID,
		RunID:       common.RunID("run-other"),
		ArtifactID:  "artifact-002",
		TargetRef:   "deliverable/other.txt",
		State:       common.ArtifactContinuationStatePlanning,
	}
	if err := repo.Create(ctx, otherRun); err != nil {
		t.Fatalf("create other run artifact continuation: %v", err)
	}
	byRun, err := repo.ListByRun(ctx, item.RunID)
	if err != nil {
		t.Fatalf("list by run: %v", err)
	}
	if len(byRun) != 1 || byRun[0].ID != item.ID {
		t.Fatalf("artifact continuations by run = %+v, want %q", byRun, item.ID)
	}
	open, err := repo.ListOpenByRun(ctx, item.RunID)
	if err != nil {
		t.Fatalf("list open by run: %v", err)
	}
	if len(open) != 1 || open[0].State != common.ArtifactContinuationStateChunking {
		t.Fatalf("open artifact continuations = %+v, want chunking item", open)
	}
	item.State = common.ArtifactContinuationStateApplied
	if err := repo.Update(ctx, item); err != nil {
		t.Fatalf("update applied artifact continuation: %v", err)
	}
	open, err = repo.ListOpenByRun(ctx, item.RunID)
	if err != nil {
		t.Fatalf("list open by run after applied: %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("open artifact continuations = %+v, want none after applied", open)
	}
	if _, err := repo.Get(ctx, common.ArtifactContinuationID("missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing get err = %v, want not found", err)
	}
	if err := repo.Update(ctx, &orchestratormodel.ArtifactContinuation{ID: common.ArtifactContinuationID("missing")}); !errors.Is(err, common.ErrNotFound) {
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
	byRun, err := repo.ListByRunID(ctx, item.RunID)
	if err != nil {
		t.Fatalf("list tickets by run: %v", err)
	}
	if len(byRun) != 1 || byRun[0].ID != item.ID {
		t.Fatalf("tickets by run = %+v, want %q", byRun, item.ID)
	}
	byWorkspace, err := repo.ListByWorkspaceID(ctx, item.WorkspaceID)
	if err != nil {
		t.Fatalf("list tickets by workspace: %v", err)
	}
	if len(byWorkspace) != 2 || byWorkspace[0].ID != item.ID || byWorkspace[1].ID != second.ID {
		t.Fatalf("tickets by workspace = %+v, want %q/%q", byWorkspace, item.ID, second.ID)
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
		Action:      "apply risky code patch",
		Context:     map[string]any{"risk": "public_api_change", "files": []any{"api.go"}},
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
	got.Context["risk"] = "mutated"
	fresh, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get fresh approval: %v", err)
	}
	if fresh.Action != "apply risky code patch" || fresh.Context["risk"] != "public_api_change" {
		t.Fatalf("approval action/context = %q/%+v, want original values", fresh.Action, fresh.Context)
	}
	got = fresh
	got.State = common.ApprovalStateApproved
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	updated, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get updated approval: %v", err)
	}
	if updated.State != common.ApprovalStateApproved || updated.Action != "apply risky code patch" || updated.Context["risk"] != "public_api_change" {
		t.Fatalf("approval state/action/context = %q/%q/%+v, want approved with original action/context", updated.State, updated.Action, updated.Context)
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
	byRun, err := repo.ListByRunID(ctx, item.RunID)
	if err != nil {
		t.Fatalf("list approvals by run: %v", err)
	}
	if len(byRun) != 1 || byRun[0].ID != item.ID {
		t.Fatalf("approvals by run = %+v, want %q", byRun, item.ID)
	}
	byWorkspace, err := repo.ListByWorkspaceID(ctx, item.WorkspaceID)
	if err != nil {
		t.Fatalf("list approvals by workspace: %v", err)
	}
	if len(byWorkspace) != 2 || byWorkspace[0].ID != item.ID || byWorkspace[1].ID != second.ID {
		t.Fatalf("approvals by workspace = %+v, want %q/%q", byWorkspace, item.ID, second.ID)
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
	artifactContinuationID := common.ArtifactContinuationID("artifact-continuation-bundle-contract-001")
	resultID := common.ResultID("result-bundle-contract")
	artifactID := common.ArtifactID("artifact-bundle-contract")
	eventRecordID := common.EventRecordID("event-bundle-contract")
	ticketID := common.TicketID("ticket-bundle-contract")
	resolutionID := common.ResolutionID("resolution-bundle-contract")
	approvalID := common.ApprovalID("approval-bundle-contract")
	soulID := common.SoulID("soul-bundle-contract")
	memoryID := common.MemoryID("memory-bundle-contract")
	memoryRecallTraceID := common.MemoryRecallTraceID("memory-recall-bundle-contract")
	autoGovernanceTraceID := common.AutoGovernanceTraceID("auto-governance-bundle-contract")
	skillID := common.SkillID("skill-bundle-contract")
	mcpServerID := common.MCPServerID("mcp-bundle-contract")

	soul := &soulmodel.Soul{
		ID:                   soulID,
		Name:                 "bundle soul",
		SoulMD:               "# Evidence Soul\n\nPlan and cite evidence.\n",
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
		ID:                  memoryID,
		WorkspaceID:         workspaceID,
		RunID:               runID,
		SourceType:          "run",
		SourceRef:           string(runID),
		MemoryType:          "run_summary",
		Summary:             "bundle run summary",
		Tags:                []string{"bundle"},
		Importance:          5,
		EmbeddingProviderID: "embedding-bundle",
		EmbeddingModel:      "embedding-model",
		EmbeddingText:       "bundle run summary",
		Vector:              []float64{1, 0, 0},
	}
	if err := bundle.memoryItems.Create(ctx, memoryItem); err != nil {
		t.Fatalf("create memory item: %v", err)
	}
	if err := bundle.memoryItems.Create(ctx, memoryItem); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate memory item create err = %v, want conflict", err)
	}

	memoryRecallTrace := &memorymodel.MemoryRecallTrace{
		ID:                 memoryRecallTraceID,
		WorkspaceID:        workspaceID,
		RunID:              runID,
		StepID:             stepID,
		Stage:              "planning",
		Source:             "activation",
		Policy:             "semantic_vector_search",
		Status:             "selected",
		RetrievedMemoryIDs: []common.MemoryID{memoryID},
		RetrievedMemoryHits: []memorymodel.MemoryRecallHit{
			{ID: memoryID, Summary: "bundle run summary", VectorScore: 0.99, VectorDimension: 3},
		},
		OccurredAt:  "2026-05-04T00:00:00Z",
		EvidenceRef: string(memoryRecallTraceID),
	}
	if err := bundle.memoryRecallTraces.Create(ctx, memoryRecallTrace); err != nil {
		t.Fatalf("create memory recall trace: %v", err)
	}
	if err := bundle.memoryRecallTraces.Create(ctx, memoryRecallTrace); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate memory recall trace create err = %v, want conflict", err)
	}

	autoGovernanceTrace := &governancemodel.AutoGovernanceTrace{
		ID:                 autoGovernanceTraceID,
		WorkspaceID:        workspaceID,
		RunID:              runID,
		StepID:             stepID,
		ItemType:           "approval",
		ItemID:             string(approvalID),
		Action:             "approve",
		RiskLevel:          "L1",
		Confidence:         0.95,
		PolicyMatched:      "read_only",
		BudgetBefore:       3,
		BudgetAfter:        2,
		Reason:             "auto-approved read-only action",
		EvidenceRefs:       []string{string(memoryRecallTraceID)},
		LLMProviderID:      "local",
		LLMModel:           "bundle-model",
		LLMDecisionPreview: "approve read-only action",
		Status:             "resolved",
		OccurredAt:         "2026-05-04T00:00:01Z",
	}
	if err := bundle.autoGovernanceTraces.Create(ctx, autoGovernanceTrace); err != nil {
		t.Fatalf("create auto governance trace: %v", err)
	}
	if err := bundle.autoGovernanceTraces.Create(ctx, autoGovernanceTrace); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate auto governance trace create err = %v, want conflict", err)
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
	mcpCapability := &mcpmodel.Capability{
		ID:          common.MCPCapabilityID("mcp-bundle-contract:tool:repo.summary"),
		ServerID:    mcpServerID,
		Kind:        "tool",
		Name:        "repo.summary",
		DisplayName: "仓库摘要",
		Enabled:     true,
	}
	if err := bundle.mcpCapabilities.Create(ctx, mcpCapability); err != nil {
		t.Fatalf("create mcp capability: %v", err)
	}
	if err := bundle.mcpCapabilities.Create(ctx, mcpCapability); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate mcp capability create err = %v, want conflict", err)
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
		Kind:  "manual/checkpoint",
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

	artifactContinuation := &orchestratormodel.ArtifactContinuation{
		ID:              artifactContinuationID,
		WorkspaceID:     workspaceID,
		RunID:           runID,
		RootStepID:      stepID,
		ArtifactID:      "artifact-001",
		TargetRef:       "deliverable/bundle.txt",
		Operation:       "replace",
		CoverageTargets: []string{"bundle-coverage"},
		State:           common.ArtifactContinuationStateChunking,
		Metadata:        map[string]any{"scope": "bundle"},
	}
	if err := bundle.artifactContinuations.Create(ctx, artifactContinuation); err != nil {
		t.Fatalf("create artifact continuation: %v", err)
	}
	if err := bundle.artifactContinuations.SavePlan(ctx, artifactContinuationID, map[string]any{"artifactId": "artifact-001"}); err != nil {
		t.Fatalf("save artifact continuation plan: %v", err)
	}
	if err := bundle.artifactContinuations.SaveChunk(ctx, &orchestratormodel.ArtifactContinuationChunk{
		ContinuationID: artifactContinuationID,
		Sequence:       1,
		ArtifactID:     "artifact-001",
		TargetRef:      "deliverable/bundle.txt",
		ContentHash:    "hash-001",
		Bytes:          6,
	}, "bundle"); err != nil {
		t.Fatalf("save artifact continuation chunk: %v", err)
	}
	if err := bundle.artifactContinuations.SaveAssembled(ctx, &orchestratormodel.ArtifactContinuationAssembledMetadata{
		ContinuationID:  artifactContinuationID,
		TargetRef:       "deliverable/bundle.txt",
		Bytes:           6,
		RollingHash:     "rolling-hash",
		SourceChunkRefs: []string{"001"},
	}, "bundle"); err != nil {
		t.Fatalf("save assembled artifact continuation: %v", err)
	}

	result := &resultmodel.Result{
		ID:           resultID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		GoalID:       goalID,
		DoDID:        dodID,
		State:        common.ResultStateAccepted,
		Title:        "bundle result",
		Summary:      "bundle accepted outcome",
		SourceType:   "dod_assessment",
		SourceRef:    string(stepID),
		EvidenceRefs: []string{string(stepID), string(validationResultID)},
		ArtifactRefs: []common.ArtifactID{artifactID},
	}
	if err := bundle.results.Create(ctx, result); err != nil {
		t.Fatalf("create result: %v", err)
	}
	if err := bundle.results.Create(ctx, result); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate result create err = %v, want conflict", err)
	}

	artifact := &resultmodel.Artifact{
		ID:                 artifactID,
		WorkspaceID:        workspaceID,
		RunID:              runID,
		ResultID:           resultID,
		StepID:             stepID,
		ToolCallID:         toolCallID,
		PatchID:            patchID,
		ValidationResultID: validationResultID,
		ArtifactType:       "report",
		Name:               "bundle report",
		Summary:            "bundle report summary",
		URI:                "workspace://results/bundle-report.md",
		ContentType:        "text/markdown",
		State:              common.ArtifactStateAvailable,
		EvidenceRefs:       []string{string(toolCallID), string(validationResultID)},
		Metadata:           map[string]any{"scope": "bundle"},
	}
	if err := bundle.artifacts.Create(ctx, artifact); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if err := bundle.artifacts.Create(ctx, artifact); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate artifact create err = %v, want conflict", err)
	}

	eventRecord := &eventstreammodel.EventRecord{
		ID:           eventRecordID,
		Version:      "workorch.event.v1",
		Topic:        "/run/state/changed",
		OccurredAt:   "2026-04-28T00:00:00Z",
		SourceModule: "kernel/run",
		SubjectType:  "run",
		SubjectID:    string(runID),
		WorkspaceID:  workspaceID,
		RunID:        runID,
		Payload:      map[string]any{"state": string(common.RunStateBlockedOnDecision)},
	}
	if err := bundle.eventRecords.Append(ctx, eventRecord); err != nil {
		t.Fatalf("append event record: %v", err)
	}
	if err := bundle.eventRecords.Append(ctx, eventRecord); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate event record append err = %v, want conflict", err)
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
	reloadedVectorMemory, err := reloaded.memoryItems.Search(ctx, repository.MemorySearchQuery{
		WorkspaceID: workspaceID,
		Vector:      []float64{1, 0, 0},
		MinScore:    0.99,
		Limit:       1,
	})
	if err != nil {
		t.Fatalf("reload memory vector search: %v", err)
	}
	if len(reloadedVectorMemory) != 1 ||
		reloadedVectorMemory[0].ID != memoryID ||
		reloadedVectorMemory[0].EmbeddingProviderID != "embedding-bundle" ||
		reloadedVectorMemory[0].VectorScore < 0.99 {
		t.Fatalf("reloaded vector memory = %+v, want preserved vector metadata", reloadedVectorMemory)
	}
	reloadedRecallTraces, err := reloaded.memoryRecallTraces.List(ctx, repository.MemoryRecallTraceQuery{
		WorkspaceID: workspaceID,
		RunID:       runID,
		Stage:       "planning",
		Source:      "activation",
		Status:      "selected",
	})
	if err != nil {
		t.Fatalf("reload memory recall traces: %v", err)
	}
	if len(reloadedRecallTraces) != 1 ||
		reloadedRecallTraces[0].ID != memoryRecallTraceID ||
		len(reloadedRecallTraces[0].RetrievedMemoryIDs) != 1 ||
		reloadedRecallTraces[0].RetrievedMemoryIDs[0] != memoryID {
		t.Fatalf("reloaded memory recall traces = %+v, want %q -> %q", reloadedRecallTraces, memoryRecallTraceID, memoryID)
	}

	reloadedAutoGovernanceTraces, err := reloaded.autoGovernanceTraces.List(ctx, repository.AutoGovernanceTraceQuery{
		WorkspaceID: workspaceID,
		RunID:       runID,
		ItemType:    "approval",
		ItemID:      string(approvalID),
		Status:      "resolved",
	})
	if err != nil {
		t.Fatalf("reload auto governance traces: %v", err)
	}
	if len(reloadedAutoGovernanceTraces) != 1 ||
		reloadedAutoGovernanceTraces[0].ID != autoGovernanceTraceID ||
		reloadedAutoGovernanceTraces[0].PolicyMatched != "read_only" {
		t.Fatalf("reloaded auto governance traces = %+v, want %q", reloadedAutoGovernanceTraces, autoGovernanceTraceID)
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
	reloadedMCPCapability, err := reloaded.mcpCapabilities.Get(ctx, mcpCapability.ID)
	if err != nil {
		t.Fatalf("reload mcp capability: %v", err)
	}
	if reloadedMCPCapability.DisplayName != "仓库摘要" || !reloadedMCPCapability.Enabled {
		t.Fatalf("reloaded mcp capability = %+v, want enabled 仓库摘要", reloadedMCPCapability)
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

	reloadedArtifactContinuations, err := reloaded.artifactContinuations.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("reload artifact continuations by run: %v", err)
	}
	if len(reloadedArtifactContinuations) != 1 ||
		reloadedArtifactContinuations[0].ID != artifactContinuationID ||
		reloadedArtifactContinuations[0].Metadata["scope"] != "bundle" {
		t.Fatalf("reloaded artifact continuations = %+v, want %q", reloadedArtifactContinuations, artifactContinuationID)
	}
	reloadedArtifactPlan, err := reloaded.artifactContinuations.GetPlan(ctx, artifactContinuationID)
	if err != nil {
		t.Fatalf("reload artifact continuation plan: %v", err)
	}
	if reloadedArtifactPlan["artifactId"] != "artifact-001" {
		t.Fatalf("reloaded artifact continuation plan = %+v, want artifact id", reloadedArtifactPlan)
	}
	reloadedArtifactChunk, reloadedArtifactChunkContent, err := reloaded.artifactContinuations.GetChunk(ctx, artifactContinuationID, 1)
	if err != nil {
		t.Fatalf("reload artifact continuation chunk: %v", err)
	}
	if reloadedArtifactChunk.ContentHash != "hash-001" || reloadedArtifactChunkContent != "bundle" {
		t.Fatalf("reloaded artifact continuation chunk = %+v/%q, want metadata and content", reloadedArtifactChunk, reloadedArtifactChunkContent)
	}
	reloadedAssembledArtifact, reloadedAssembledContent, err := reloaded.artifactContinuations.GetAssembled(ctx, artifactContinuationID)
	if err != nil {
		t.Fatalf("reload assembled artifact continuation: %v", err)
	}
	if reloadedAssembledArtifact.RollingHash != "rolling-hash" || reloadedAssembledContent != "bundle" {
		t.Fatalf("reloaded assembled artifact continuation = %+v/%q, want metadata and content", reloadedAssembledArtifact, reloadedAssembledContent)
	}

	reloadedResults, err := reloaded.results.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("reload results by run: %v", err)
	}
	if len(reloadedResults) != 1 || reloadedResults[0].ID != resultID || reloadedResults[0].ArtifactRefs[0] != artifactID {
		t.Fatalf("reloaded results = %+v, want %q with artifact ref", reloadedResults, resultID)
	}

	reloadedArtifacts, err := reloaded.artifacts.ListByResult(ctx, resultID)
	if err != nil {
		t.Fatalf("reload artifacts by result: %v", err)
	}
	if len(reloadedArtifacts) != 1 || reloadedArtifacts[0].ID != artifactID || reloadedArtifacts[0].Metadata["scope"] != "bundle" {
		t.Fatalf("reloaded artifacts = %+v, want %q with metadata", reloadedArtifacts, artifactID)
	}

	reloadedEvents, err := reloaded.eventRecords.List(ctx, repository.EventRecordQuery{RunID: runID})
	if err != nil {
		t.Fatalf("reload event records by run: %v", err)
	}
	if len(reloadedEvents) != 1 || reloadedEvents[0].ID != eventRecordID || len(reloadedEvents[0].Payload) != 0 {
		t.Fatalf("reloaded event records = %+v, want %q", reloadedEvents, eventRecordID)
	}
	reloadedEventsWithPayload, err := reloaded.eventRecords.List(ctx, repository.EventRecordQuery{RunID: runID, IncludePayload: true})
	if err != nil {
		t.Fatalf("reload event records by run with payload: %v", err)
	}
	if len(reloadedEventsWithPayload) != 1 || reloadedEventsWithPayload[0].ID != eventRecordID || reloadedEventsWithPayload[0].Payload["state"] != string(common.RunStateBlockedOnDecision) {
		t.Fatalf("reloaded event records with payload = %+v, want %q", reloadedEventsWithPayload, eventRecordID)
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
	if _, err := reloaded.mcpCapabilities.Get(ctx, common.MCPCapabilityID("mcp-capability-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing mcp capability get err = %v, want not found", err)
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
	if _, err := reloaded.artifactContinuations.Get(ctx, common.ArtifactContinuationID("artifact-continuation-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing artifact continuation get err = %v, want not found", err)
	}
	if _, err := reloaded.eventRecords.Get(ctx, common.EventRecordID("event-missing")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing event record get err = %v, want not found", err)
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
