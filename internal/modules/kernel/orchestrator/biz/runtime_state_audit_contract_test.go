package orchestrator

import (
	"context"
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFailedAssessmentDecisionShouldRepairIgnoresRuntimeStage(t *testing.T) {
	step := &runmodel.Step{
		ID:    "run-state-audit/step-1",
		Input: map[string]any{stepInputKeyRuntimeStage: runtimeStageExecution},
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"goalCoverage": map[string]any{
				"items": []map[string]any{{
					"goal":  "produce accepted evidence",
					"state": "missing",
				}},
			},
		},
	}
	spec := runtimeDecisionBlockSpec{
		Reason:       "goal coverage evidence is missing",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
	}

	if !failedAssessmentDecisionShouldRepair(step, output, spec) {
		t.Fatal("failed assessment decision was not converted to repair outside verification stage")
	}
}

func TestValidateAndCompleteRunPrevalidatesBeforeValidating(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-validation-preflight")
	workspaceID := common.WorkspaceID("ws-validation-preflight")
	runRepo := memoryrepo.NewRunRepository()
	stepRepo := memoryrepo.NewStepRepository()
	workspaceRepo := memoryrepo.NewWorkspaceRepository()
	coverageRepo := memoryrepo.NewCoverageWorkItemRepository()
	run := &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		State:              common.RunStateRunning,
		DefinitionRevision: "rev-1",
		ExecutionContext:   map[string]any{},
	}
	workspace := &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := coverageRepo.Create(ctx, &runtimecontract.CoverageWorkItem{
		ID:                    "cwi-open",
		WorkspaceID:           workspaceID,
		RunID:                 runID,
		DefinitionRevision:    "rev-1",
		NormalizedSourceKey:   "source:open",
		SourceScope:           "dod",
		TargetText:            "open coverage item",
		RequiredEvidenceKinds: []string{"state_change"},
		Status:                "missing",
	}); err != nil {
		t.Fatalf("seed coverage: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		nil,
		runRepo,
		stepRepo,
		workspaceRepo,
		memoryrepo.NewDecisionTicketRepository(),
		nil,
		memoryrepo.NewApprovalRepository(),
		RuntimeEvidenceRepositories{CoverageWorkItems: coverageRepo},
		events.NewMemoryHub(),
	)

	err := orch.validateAndCompleteRunWithWorkspace(ctx, run, "run-validation-preflight/step-1", workspace)
	if err == nil {
		t.Fatal("validateAndCompleteRunWithWorkspace succeeded, want preflight coverage error")
	}
	persistedRun, getErr := runRepo.Get(ctx, runID)
	if getErr != nil {
		t.Fatalf("get run: %v", getErr)
	}
	persistedWorkspace, getErr := workspaceRepo.Get(ctx, workspaceID)
	if getErr != nil {
		t.Fatalf("get workspace: %v", getErr)
	}
	if persistedRun.State != common.RunStateRunning || persistedWorkspace.State != common.WorkspaceStateRunning {
		t.Fatalf("state after failed preflight run=%s workspace=%s, want running/running", persistedRun.State, persistedWorkspace.State)
	}
}

func TestConvergenceContractShouldNotPersistUnqualifiedBlocked(t *testing.T) {
	if convergenceContractShouldPersist(ConvergenceContract{State: convergenceStateBlocked}) {
		t.Fatal("unqualified blocked convergence contract should not be persisted")
	}
	if !convergenceContractShouldPersist(ConvergenceContract{
		State:            convergenceStateBlocked,
		GovernanceReason: "needs external decision",
	}) {
		t.Fatal("blocked convergence contract with governance reason should be persistable")
	}
}

func TestPreviousExecutionStepIDUsesBatchRoot(t *testing.T) {
	step := &runmodel.Step{
		ID: "run-batch-root/graph-001/002-b",
		Input: map[string]any{
			stepInputKeyGraphBatchRootStepID: "run-batch-root/root",
		},
	}

	if got := previousExecutionStepID(step); got != "run-batch-root/root" {
		t.Fatalf("previousExecutionStepID = %q, want batch root", got)
	}
}

func TestGraphFollowUpExpansionCandidateCountsEmptyGraphOutput(t *testing.T) {
	step := &runmodel.Step{
		ID:    "run-empty-graph/follow-up",
		Input: map[string]any{"graphFollowUp": true},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{"nodes": []any{}},
	}

	if !graphFollowUpExpansionCandidate(step, output) {
		t.Fatal("empty graph follow-up output was not counted as an expansion candidate")
	}
	if count := (&graphSubsystem{}).graphFollowUpExpansionCountIncludingCandidate(context.Background(), "", step, output); count != 1 {
		t.Fatalf("graph follow-up count = %d, want candidate counted", count)
	}
}
