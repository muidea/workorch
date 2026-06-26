package orchestrator

import (
	"context"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestNextActionForRunPendingApprovalRequestsApproval(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		memory.NewStepRepository(),
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		approvalRepo,
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-approval")
	runID := common.RunID("run-next-action-approval")
	approvalID := common.ApprovalID("approval-next-action")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		State:              common.RunStateBlockedOnApproval,
		BlockingApprovalID: approvalID,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		Action:      "review pending runtime action",
		State:       common.ApprovalStatePending,
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeRequestApproval || action.Stage != "approval" || action.StageMode != "governance" {
		t.Fatalf("next action = %+v, want request approval", action)
	}
	if action.SubjectType != "approval" || action.SubjectID != string(approvalID) {
		t.Fatalf("next action subject = %s/%s, want approval %s", action.SubjectType, action.SubjectID, approvalID)
	}
	if action.RequiredOutput["approvalId"] != string(approvalID) {
		t.Fatalf("required output = %+v, want approval id", action.RequiredOutput)
	}
	if !stringSliceContains(action.ForbiddenActions, runtimemodel.NextActionTypeStateChange) {
		t.Fatalf("forbidden actions = %+v, want state_change forbidden", action.ForbiddenActions)
	}
}

func TestNextActionForRunOpenDecisionRequestsDecision(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		memory.NewStepRepository(),
		workspaceRepo,
		ticketRepo,
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-decision")
	runID := common.RunID("run-next-action-decision")
	ticketID := common.TicketID("ticket-next-action")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		State:       common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeRequestDecision || action.Stage != "decision" || action.StageMode != "governance" {
		t.Fatalf("next action = %+v, want request decision", action)
	}
	if action.SubjectType != "decision" || action.SubjectID != string(ticketID) {
		t.Fatalf("next action subject = %s/%s, want decision %s", action.SubjectType, action.SubjectID, ticketID)
	}
	if action.RequiredOutput["ticketId"] != string(ticketID) {
		t.Fatalf("required output = %+v, want ticket id", action.RequiredOutput)
	}
	if !stringSliceContains(action.ForbiddenActions, runtimemodel.NextActionTypeStateChange) {
		t.Fatalf("forbidden actions = %+v, want state_change forbidden", action.ForbiddenActions)
	}
}

func TestNextActionForRunMissingGapBaselineRequestsAssessment(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		memory.NewStepRepository(),
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-gap")
	runID := common.RunID("run-next-action-gap")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeAssessGap || action.Stage != "gap_analysis" || action.StageMode != "baseline" {
		t.Fatalf("next action = %+v, want assess gap", action)
	}
	if action.SubjectType != "run" || action.SubjectID != string(runID) {
		t.Fatalf("next action subject = %s/%s, want run %s", action.SubjectType, action.SubjectID, runID)
	}
	if action.RequiredOutput["gapBaselineRequired"] != true {
		t.Fatalf("required output = %+v, want gap baseline required", action.RequiredOutput)
	}
	if !stringSliceContains(action.ForbiddenActions, runtimemodel.NextActionTypeStateChange) {
		t.Fatalf("forbidden actions = %+v, want state_change forbidden", action.ForbiddenActions)
	}
}

func TestDeriveMissingGapBaselineNextActionSkipsExistingBaselineAndTerminalRuns(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-gap-rule",
		WorkspaceID: "ws-gap-rule",
		State:       common.RunStateRunning,
	}
	if action, ok := deriveMissingGapBaselineNextAction(run); !ok || action.Type != runtimemodel.NextActionTypeAssessGap {
		t.Fatalf("derive missing gap baseline = %+v/%v, want assess_gap", action, ok)
	}

	run.ExecutionContext = map[string]any{
		gapBaselineKey: map[string]any{"schemaVersion": gapBaselineSchema, "baselineRevision": "gap-present"},
	}
	if action, ok := deriveMissingGapBaselineNextAction(run); ok || action.Type != "" {
		t.Fatalf("derive with existing baseline = %+v/%v, want no assess_gap", action, ok)
	}

	run.State = common.RunStateCompleted
	run.ExecutionContext = nil
	if action, ok := deriveMissingGapBaselineNextAction(run); ok || action.Type != "" {
		t.Fatalf("derive terminal run = %+v/%v, want no assess_gap", action, ok)
	}
}

func TestNextActionForRunMissingStateChangeRequestsStateChange(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	coverageRepo := memory.NewCoverageWorkItemRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		memory.NewStepRepository(),
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{CoverageWorkItems: coverageRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-state-change")
	runID := common.RunID("run-next-action-state-change")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			gapBaselineKey: map[string]any{"schemaVersion": gapBaselineSchema, "baselineRevision": "gap-present"},
		},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := coverageRepo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runtimemodel.CoverageWorkItem{
		ID:                    "cwi-repo-state-change",
		WorkspaceID:           workspaceID,
		RunID:                 runID,
		DefinitionRevision:    "rev-state-change",
		NormalizedSourceKey:   "source-state-change",
		RequiredEvidenceKinds: []string{runtimemodel.EvidenceKindStateChange, runtimemodel.EvidenceKindValidation},
		Status:                "open",
		Metadata: map[string]any{
			"id": "cwi-state-change",
		},
	}); err != nil {
		t.Fatalf("create coverage item: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeStateChange || action.Stage != "state_change" || action.StageMode != "evidence" {
		t.Fatalf("next action = %+v, want state_change", action)
	}
	if action.SubjectType != "coverage" || action.SubjectID != "cwi-state-change" {
		t.Fatalf("next action subject = %s/%s, want coverage cwi-state-change", action.SubjectType, action.SubjectID)
	}
	if len(action.FocusCoverageItemIDs) != 1 || action.FocusCoverageItemIDs[0] != "cwi-state-change" {
		t.Fatalf("focus coverage ids = %+v, want cwi-state-change", action.FocusCoverageItemIDs)
	}
	if !stringSliceContains(action.RequiredEvidenceKinds, runtimemodel.EvidenceKindStateChange) {
		t.Fatalf("required evidence kinds = %+v, want state_change", action.RequiredEvidenceKinds)
	}
	if !stringSliceContains(action.ForbiddenActions, runtimemodel.NextActionTypeComplete) {
		t.Fatalf("forbidden actions = %+v, want complete forbidden", action.ForbiddenActions)
	}
}

func TestNextActionForRunMissingStateChangeAcquiresTargetSnapshotFirst(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	coverageRepo := memory.NewCoverageWorkItemRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		memory.NewStepRepository(),
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{CoverageWorkItems: coverageRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-acquire-snapshot")
	runID := common.RunID("run-next-action-acquire-snapshot")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			gapBaselineKey: map[string]any{"schemaVersion": gapBaselineSchema, "baselineRevision": "gap-present"},
			"repairActionPackage": map[string]any{
				"requiresCurrentTargetSnapshotForStateChange": true,
				"repairTargetSet": []string{
					"target.txt",
				},
				"requiredOutput": map[string]any{
					"mustUseCurrentTargetSnapshot": true,
				},
			},
		},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := coverageRepo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runtimemodel.CoverageWorkItem{
		ID:                    "cwi-repo-acquire-snapshot",
		WorkspaceID:           workspaceID,
		RunID:                 runID,
		DefinitionRevision:    "rev-acquire-snapshot",
		NormalizedSourceKey:   "source-acquire-snapshot",
		RequiredEvidenceKinds: []string{runtimemodel.EvidenceKindStateChange},
		Status:                "open",
		Metadata: map[string]any{
			"id": "cwi-acquire-snapshot",
		},
	}); err != nil {
		t.Fatalf("create coverage item: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeAcquireTargetSnapshot || action.Stage != runtimemodel.NextActionTypeAcquireTargetSnapshot || action.StageMode != "target_snapshot_acquisition" {
		t.Fatalf("next action = %+v, want acquire_target_snapshot", action)
	}
	if action.RequiredOutput["mustAcquireCurrentTargetSnapshot"] != true || action.RequiredOutput["mustUseReadOnlySnapshotAcquisition"] != true {
		t.Fatalf("required output = %+v, want read-only target snapshot acquisition", action.RequiredOutput)
	}
	if !stringSliceContains(action.ForbiddenActions, runtimemodel.NextActionTypeStateChange) {
		t.Fatalf("forbidden actions = %+v, want state_change forbidden before snapshot", action.ForbiddenActions)
	}
}

func TestNextActionForRunHydratesAcceptedTargetSnapshotEvidenceBeforeStateChange(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	coverageRepo := memory.NewCoverageWorkItemRepository()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	patchRepo := memory.NewPatchRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{CoverageWorkItems: coverageRepo, EvidenceRecords: evidenceRepo, Patches: patchRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-accepted-snapshot")
	runID := common.RunID("run-next-action-accepted-snapshot")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	run := &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		State:              common.RunStateRunning,
		DefinitionRevision: "rev-accepted-snapshot",
		ExecutionContext: map[string]any{
			gapBaselineKey: map[string]any{"schemaVersion": gapBaselineSchema, "baselineRevision": "gap-present"},
			"repairActionPackage": map[string]any{
				"requiresCurrentTargetSnapshotForStateChange": true,
				"repairTargetSet": []string{"target.txt"},
				"requiredOutput": map[string]any{
					"mustUseCurrentTargetSnapshot": true,
				},
			},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := coverageRepo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runtimemodel.CoverageWorkItem{
		ID:                    "cwi-repo-accepted-snapshot",
		WorkspaceID:           workspaceID,
		RunID:                 runID,
		DefinitionRevision:    run.DefinitionRevision,
		NormalizedSourceKey:   "source-accepted-snapshot",
		RequiredEvidenceKinds: []string{runtimemodel.EvidenceKindStateChange},
		Status:                "open",
		Metadata: map[string]any{
			"id": "cwi-accepted-snapshot",
		},
	}); err != nil {
		t.Fatalf("create coverage item: %v", err)
	}
	step := &runmodel.Step{
		ID:    "run-next-action-accepted-snapshot/read-target",
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: "run-next-action-accepted-snapshot/root",
			stepInputKeyGraphNodeID:     "read-target",
		},
		Output: map[string]any{
			"stage":            "file_read",
			"path":             "target.txt",
			"exists":           true,
			"content":          "current target content\n",
			"contentHash":      "hash-accepted-snapshot",
			"contentComplete":  true,
			"safeForFileWrite": true,
			"truncated":        false,
			"isBinary":         false,
		},
	}
	if err := stepRepo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), step); err != nil {
		t.Fatalf("create source step: %v", err)
	}
	record := acceptedTargetSnapshotEvidenceRecordForStep(run, step)
	if record == nil {
		t.Fatalf("accepted target snapshot evidence record = nil")
	}
	if err := evidenceRepo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), record); err != nil {
		t.Fatalf("create target snapshot evidence: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("next action = %+v, want state_change using accepted target snapshot evidence", action)
	}
	if stringSliceContains(action.ForbiddenActions, runtimemodel.NextActionTypeStateChange) ||
		stringSliceContains(action.AllowedActions, runtimemodel.NextActionTypeAcquireTargetSnapshot) {
		t.Fatalf("next action actions = allowed:%+v forbidden:%+v, want state_change contract without snapshot acquisition", action.AllowedActions, action.ForbiddenActions)
	}
	reloadedRun, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if len(mapSliceFromValue(mapFromValue(reloadedRun.ExecutionContext["repairActionPackage"])["targetSnapshots"])) != 0 {
		t.Fatalf("persisted run context = %+v, want hydration scoped to current authority calculation", reloadedRun.ExecutionContext)
	}
}

func TestNextActionForRunIgnoresAcceptedTargetSnapshotAfterLaterAppliedPatch(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	coverageRepo := memory.NewCoverageWorkItemRepository()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	patchRepo := memory.NewPatchRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{CoverageWorkItems: coverageRepo, EvidenceRecords: evidenceRepo, Patches: patchRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-stale-snapshot")
	runID := common.RunID("run-next-action-stale-snapshot")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	run := &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		State:              common.RunStateRunning,
		DefinitionRevision: "rev-stale-snapshot",
		ExecutionContext: map[string]any{
			gapBaselineKey: map[string]any{"schemaVersion": gapBaselineSchema, "baselineRevision": "gap-present"},
			"repairActionPackage": map[string]any{
				"requiresCurrentTargetSnapshotForStateChange": true,
				"repairTargetSet": []string{"target.txt"},
				"requiredOutput": map[string]any{
					"mustUseCurrentTargetSnapshot": true,
				},
			},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := coverageRepo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runtimemodel.CoverageWorkItem{
		ID:                    "cwi-repo-stale-snapshot",
		WorkspaceID:           workspaceID,
		RunID:                 runID,
		DefinitionRevision:    run.DefinitionRevision,
		NormalizedSourceKey:   "source-stale-snapshot",
		RequiredEvidenceKinds: []string{runtimemodel.EvidenceKindStateChange},
		Status:                "open",
		Metadata: map[string]any{
			"id": "cwi-stale-snapshot",
		},
	}); err != nil {
		t.Fatalf("create coverage item: %v", err)
	}
	step := &runmodel.Step{
		ID:    "run-next-action-stale-snapshot/read-target",
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"stage":            "file_read",
			"path":             "target.txt",
			"exists":           true,
			"content":          "old target content\n",
			"contentHash":      "hash-old-snapshot",
			"contentComplete":  true,
			"safeForFileWrite": true,
			"truncated":        false,
			"isBinary":         false,
		},
	}
	if err := stepRepo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), step); err != nil {
		t.Fatalf("create source step: %v", err)
	}
	record := acceptedTargetSnapshotEvidenceRecordForStep(run, step)
	if record == nil {
		t.Fatalf("accepted target snapshot evidence record = nil")
	}
	record.CreatedAt = "2026-06-26T01:00:00Z"
	record.UpdatedAt = record.CreatedAt
	if record.Metadata == nil {
		record.Metadata = map[string]any{}
	}
	record.Metadata["capturedAt"] = record.CreatedAt
	if err := evidenceRepo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), record); err != nil {
		t.Fatalf("create target snapshot evidence: %v", err)
	}
	if err := patchRepo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runmodel.Patch{
		ID:          "run-next-action-stale-snapshot/patch/after-snapshot",
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      "run-next-action-stale-snapshot/patch-step",
		PatchType:   "file_write",
		TargetRef:   "target.txt",
		State:       common.PatchStateApplied,
		BeforeHash:  "hash-old-snapshot",
		AfterHash:   "hash-new-snapshot",
		AppliedAt:   "2026-06-26T01:01:00Z",
	}); err != nil {
		t.Fatalf("create later applied patch: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeAcquireTargetSnapshot {
		t.Fatalf("next action = %+v, want acquire_target_snapshot because accepted snapshot is stale", action)
	}
}

func TestDeriveAcquireTargetSnapshotNextActionSkipsWhenSnapshotExists(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-acquire-snapshot-skip",
		WorkspaceID: "ws-acquire-snapshot-skip",
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			"repairActionPackage": map[string]any{
				"requiresCurrentTargetSnapshotForStateChange": true,
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target.txt",
					"exists":           true,
					"contentHash":      "hash-target",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
				"requiredOutput": map[string]any{
					"mustUseCurrentTargetSnapshot": true,
				},
			},
		},
	}
	stateChange := runtimemodel.NextAction{
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		Type:        runtimemodel.NextActionTypeStateChange,
	}.Normalized()
	if action, ok := deriveAcquireTargetSnapshotNextAction(run, stateChange); ok || action.Type != "" {
		t.Fatalf("derive acquire snapshot with existing target snapshot = %+v/%v, want no action", action, ok)
	}
}

func TestDeriveMissingStateChangeNextActionSkipsInactiveOrSatisfiedItems(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-state-change-rule",
		WorkspaceID: "ws-state-change-rule",
		State:       common.RunStateRunning,
	}
	items := []map[string]any{
		{
			"id":                    "cwi-satisfied",
			"status":                "satisfied",
			"requiredEvidenceKinds": []string{runtimemodel.EvidenceKindStateChange},
		},
		{
			"id":                    "cwi-validation-only",
			"status":                "open",
			"requiredEvidenceKinds": []string{runtimemodel.EvidenceKindValidation},
			"missingEvidenceKinds":  []string{runtimemodel.EvidenceKindValidation},
		},
	}
	if action, ok := deriveMissingStateChangeNextAction(run, items); ok || action.Type != "" {
		t.Fatalf("derive without state_change gap = %+v/%v, want no action", action, ok)
	}

	items = append(items, map[string]any{
		"id":                    "cwi-state-change",
		"status":                "open",
		"requiredEvidenceKinds": []string{runtimemodel.EvidenceKindStateChange},
		"missingEvidenceKinds":  []string{runtimemodel.EvidenceKindStateChange},
	})
	action, ok := deriveMissingStateChangeNextAction(run, items)
	if !ok || action.Type != runtimemodel.NextActionTypeStateChange || action.SubjectID != "cwi-state-change" {
		t.Fatalf("derive state_change gap = %+v/%v, want cwi-state-change action", action, ok)
	}

	run.State = common.RunStateCompleted
	if action, ok := deriveMissingStateChangeNextAction(run, items); ok || action.Type != "" {
		t.Fatalf("derive terminal run = %+v/%v, want no action", action, ok)
	}
}

func TestNextActionForRunMissingValidationRequestsValidation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		memory.NewStepRepository(),
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-validation")
	runID := common.RunID("run-next-action-validation")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			gapBaselineKey: map[string]any{"schemaVersion": gapBaselineSchema, "baselineRevision": "gap-present"},
			coverageWorkItemsKey: []map[string]any{{
				"id":                     "cwi-validation",
				"status":                 "open",
				"requiredEvidenceKinds":  []string{runtimemodel.EvidenceKindStateChange, runtimemodel.EvidenceKindValidation},
				"acceptedEvidenceByKind": map[string][]string{runtimemodel.EvidenceKindStateChange: {"step-state-change"}},
				"missingEvidenceKinds":   []string{runtimemodel.EvidenceKindValidation},
			}},
		},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeValidate || action.Stage != "validate" || action.StageMode != "evidence" {
		t.Fatalf("next action = %+v, want validate", action)
	}
	if action.SubjectType != "coverage" || action.SubjectID != "cwi-validation" {
		t.Fatalf("next action subject = %s/%s, want coverage cwi-validation", action.SubjectType, action.SubjectID)
	}
	if !stringSliceContains(action.RequiredEvidenceKinds, runtimemodel.EvidenceKindValidation) {
		t.Fatalf("required evidence kinds = %+v, want validation", action.RequiredEvidenceKinds)
	}
	if !stringSliceContains(action.ForbiddenActions, runtimemodel.NextActionTypeStateChange) {
		t.Fatalf("forbidden actions = %+v, want state_change forbidden", action.ForbiddenActions)
	}
}

func TestDeriveMissingValidationNextActionSkipsWhenStateChangeHasPriority(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-validation-rule",
		WorkspaceID: "ws-validation-rule",
		State:       common.RunStateRunning,
	}
	items := []map[string]any{{
		"id":                    "cwi-both",
		"status":                "open",
		"requiredEvidenceKinds": []string{runtimemodel.EvidenceKindStateChange, runtimemodel.EvidenceKindValidation},
		"missingEvidenceKinds":  []string{runtimemodel.EvidenceKindStateChange, runtimemodel.EvidenceKindValidation},
	}}
	if action, ok := deriveMissingStateChangeNextAction(run, items); !ok || action.Type != runtimemodel.NextActionTypeStateChange {
		t.Fatalf("derive state_change priority = %+v/%v, want state_change", action, ok)
	}

	items[0]["acceptedEvidenceByKind"] = map[string][]string{runtimemodel.EvidenceKindStateChange: {"step-state-change"}}
	items[0]["missingEvidenceKinds"] = []string{runtimemodel.EvidenceKindValidation}
	action, ok := deriveMissingValidationNextAction(run, items)
	if !ok || action.Type != runtimemodel.NextActionTypeValidate || action.SubjectID != "cwi-both" {
		t.Fatalf("derive validation gap = %+v/%v, want validate", action, ok)
	}

	items[0]["status"] = "satisfied"
	if action, ok := deriveMissingValidationNextAction(run, items); ok || action.Type != "" {
		t.Fatalf("derive inactive item = %+v/%v, want no action", action, ok)
	}

	run.State = common.RunStateCompleted
	items[0]["status"] = "open"
	if action, ok := deriveMissingValidationNextAction(run, items); ok || action.Type != "" {
		t.Fatalf("derive terminal run = %+v/%v, want no action", action, ok)
	}
}

func TestNextActionForRunRecoverableDiagnosticRequestsRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		memory.NewStepRepository(),
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{EvidenceRecords: evidenceRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-diagnostic")
	runID := common.RunID("run-next-action-diagnostic")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			gapBaselineKey: map[string]any{"schemaVersion": gapBaselineSchema, "baselineRevision": "gap-present"},
		},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := evidenceRepo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runtimemodel.EvidenceRecord{
		ID:          "diagnostic-retry",
		WorkspaceID: workspaceID,
		RunID:       runID,
		Kind:        runtimemodel.EvidenceKindDiagnostic,
		Status:      runtimemodel.EvidenceStatusRejected,
		SourceType:  evidenceRecordSourceTypeContractViolation,
		SourceRef:   "run-next-action-diagnostic/failed/contract-violation",
		Metadata: map[string]any{
			"recoverability":   runtimemodel.ContractViolationRecoverabilityNarrowContract,
			"failureClass":     failureClassRuntime,
			"failureSignature": "class=runtime_failed",
		},
		CreatedAt: "2026-06-24T01:00:00Z",
	}); err != nil {
		t.Fatalf("create diagnostic evidence: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeRepair || action.Stage != "repair" || action.StageMode != "diagnostic_retry" {
		t.Fatalf("next action = %+v, want diagnostic repair", action)
	}
	if action.SubjectType != "diagnostic" || action.SubjectID != "diagnostic-retry" {
		t.Fatalf("next action subject = %s/%s, want diagnostic-retry", action.SubjectType, action.SubjectID)
	}
	if action.RequiredOutput["diagnosticEvidenceId"] != "diagnostic-retry" ||
		action.RequiredOutput["recoverability"] != runtimemodel.ContractViolationRecoverabilityNarrowContract {
		t.Fatalf("required output = %+v, want diagnostic retry contract", action.RequiredOutput)
	}
	if !stringSliceContains(action.ForbiddenActions, runtimemodel.NextActionTypeRequestDecision) {
		t.Fatalf("forbidden actions = %+v, want request_decision forbidden", action.ForbiddenActions)
	}
}

func TestNextActionForRunRecoverableDiagnosticHasPriorityOverEvidenceGaps(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		memory.NewStepRepository(),
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{EvidenceRecords: evidenceRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-diagnostic-priority")
	runID := common.RunID("run-next-action-diagnostic-priority")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			coverageWorkItemsKey: []map[string]any{{
				"id":                    "cwi-state-change-priority",
				"status":                "open",
				"requiredEvidenceKinds": []string{runtimemodel.EvidenceKindStateChange},
				"missingEvidenceKinds":  []string{runtimemodel.EvidenceKindStateChange},
			}},
		},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := evidenceRepo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runtimemodel.EvidenceRecord{
		ID:          "diagnostic-priority",
		WorkspaceID: workspaceID,
		RunID:       runID,
		Kind:        runtimemodel.EvidenceKindDiagnostic,
		Status:      runtimemodel.EvidenceStatusRejected,
		SourceRef:   "diagnostic-priority-source",
		Metadata:    map[string]any{"recoverability": runtimemodel.ContractViolationRecoverabilityRetrySameContract},
	}); err != nil {
		t.Fatalf("create diagnostic evidence: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeRepair || action.SubjectID != "diagnostic-priority" {
		t.Fatalf("next action = %+v, want recoverable diagnostic priority", action)
	}
}

func TestDeriveRecoverableDiagnosticNextActionSkipsDecisionRequiredAndSuperseded(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-diagnostic-rule",
		WorkspaceID: "ws-diagnostic-rule",
		State:       common.RunStateRunning,
	}
	records := []*runtimemodel.EvidenceRecord{
		{
			ID:          "diagnostic-superseded",
			WorkspaceID: run.WorkspaceID,
			RunID:       run.ID,
			Kind:        runtimemodel.EvidenceKindDiagnostic,
			Status:      runtimemodel.EvidenceStatusRejected,
			SourceRef:   "diagnostic-superseded-source",
			Metadata:    map[string]any{"recoverability": runtimemodel.ContractViolationRecoverabilityRetrySameContract},
		},
		{
			ID:          "diagnostic-superseding",
			WorkspaceID: run.WorkspaceID,
			RunID:       run.ID,
			Kind:        runtimemodel.EvidenceKindDiagnostic,
			Status:      runtimemodel.EvidenceStatusAccepted,
			SourceRef:   "diagnostic-superseding-source",
			Supersedes:  []common.EvidenceRecordID{"diagnostic-superseded"},
		},
		{
			ID:          "diagnostic-decision-required",
			WorkspaceID: run.WorkspaceID,
			RunID:       run.ID,
			Kind:        runtimemodel.EvidenceKindDiagnostic,
			Status:      runtimemodel.EvidenceStatusRejected,
			SourceRef:   "diagnostic-decision-required-source",
			Metadata:    map[string]any{"recoverability": runtimemodel.ContractViolationRecoverabilityDecisionRequired},
		},
	}
	if action, ok := deriveRecoverableDiagnosticNextAction(run, records); ok || action.Type != "" {
		t.Fatalf("derive unrecoverable diagnostics = %+v/%v, want no repair", action, ok)
	}

	records = append(records, &runtimemodel.EvidenceRecord{
		ID:          "diagnostic-nested-retry",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		Kind:        runtimemodel.EvidenceKindDiagnostic,
		Status:      runtimemodel.EvidenceStatusRejected,
		SourceRef:   "diagnostic-nested-retry-source",
		Metadata: map[string]any{
			"contractViolation": map[string]any{
				"recoverability": runtimemodel.ContractViolationRecoverabilityNormalizeThenRetry,
			},
		},
	})
	action, ok := deriveRecoverableDiagnosticNextAction(run, records)
	if !ok || action.Type != runtimemodel.NextActionTypeRepair || action.SubjectID != "diagnostic-nested-retry" {
		t.Fatalf("derive nested recoverable diagnostic = %+v/%v, want repair", action, ok)
	}

	run.State = common.RunStateCompleted
	if action, ok := deriveRecoverableDiagnosticNextAction(run, records); ok || action.Type != "" {
		t.Fatalf("derive terminal run = %+v/%v, want no action", action, ok)
	}
}

func TestNextActionForRunAllSatisfiedRequestsComplete(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		memory.NewStepRepository(),
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-complete")
	runID := common.RunID("run-next-action-complete")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			gapBaselineKey: map[string]any{"schemaVersion": gapBaselineSchema, "baselineRevision": "gap-present"},
			coverageWorkItemsKey: []map[string]any{{
				"id":                     "cwi-complete",
				"status":                 "satisfied",
				"requiredEvidenceKinds":  []string{runtimemodel.EvidenceKindStateChange, runtimemodel.EvidenceKindValidation},
				"acceptedEvidenceByKind": map[string][]string{runtimemodel.EvidenceKindStateChange: {"patch-001"}, runtimemodel.EvidenceKindValidation: {"validation-001"}},
			}},
		},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeComplete || action.Stage != "complete" || action.StageMode != "accepted_evidence" {
		t.Fatalf("next action = %+v, want complete", action)
	}
	if action.SubjectType != "run" || action.SubjectID != string(runID) {
		t.Fatalf("next action subject = %s/%s, want run %s", action.SubjectType, action.SubjectID, runID)
	}
	if action.RequiredOutput["completionReady"] != true || action.RequiredOutput["mustFreezeEvidence"] != true {
		t.Fatalf("required output = %+v, want completion freeze contract", action.RequiredOutput)
	}
	if len(action.FocusCoverageItemIDs) != 1 || action.FocusCoverageItemIDs[0] != "cwi-complete" {
		t.Fatalf("focus coverage ids = %+v, want cwi-complete", action.FocusCoverageItemIDs)
	}
	if !stringSliceContains(action.ForbiddenActions, runtimemodel.NextActionTypeRepair) {
		t.Fatalf("forbidden actions = %+v, want repair forbidden", action.ForbiddenActions)
	}
}

func TestNextActionForRunAllSatisfiedWaitsForRecoverableDiagnostic(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		memory.NewStepRepository(),
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{EvidenceRecords: evidenceRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-next-action-complete-diagnostic")
	runID := common.RunID("run-next-action-complete-diagnostic")
	mustCreateWorkspaceForNextAction(t, ctx, workspaceRepo, workspaceID, runID)
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			gapBaselineKey: map[string]any{"schemaVersion": gapBaselineSchema, "baselineRevision": "gap-present"},
			coverageWorkItemsKey: []map[string]any{{
				"id":                     "cwi-complete-diagnostic",
				"status":                 "satisfied",
				"requiredEvidenceKinds":  []string{runtimemodel.EvidenceKindValidation},
				"acceptedEvidenceByKind": map[string][]string{runtimemodel.EvidenceKindValidation: {"validation-001"}},
			}},
		},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := evidenceRepo.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runtimemodel.EvidenceRecord{
		ID:          "diagnostic-before-complete",
		WorkspaceID: workspaceID,
		RunID:       runID,
		Kind:        runtimemodel.EvidenceKindDiagnostic,
		Status:      runtimemodel.EvidenceStatusRejected,
		SourceRef:   "diagnostic-before-complete-source",
		Metadata:    map[string]any{"recoverability": runtimemodel.ContractViolationRecoverabilityRetrySameContract},
	}); err != nil {
		t.Fatalf("create diagnostic evidence: %v", err)
	}

	action, err := orch.NextActionForRun(ctx, runID)
	if err != nil {
		t.Fatalf("NextActionForRun: %v", err)
	}
	if action.Type != runtimemodel.NextActionTypeRepair || action.SubjectID != "diagnostic-before-complete" {
		t.Fatalf("next action = %+v, want recoverable diagnostic before complete", action)
	}
}

func TestDeriveAllSatisfiedCompleteNextActionRequiresSatisfiedAcceptedEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-complete-rule",
		WorkspaceID: "ws-complete-rule",
		State:       common.RunStateRunning,
	}
	if action, ok := deriveAllSatisfiedCompleteNextAction(run, nil); ok || action.Type != "" {
		t.Fatalf("derive empty ledger = %+v/%v, want no complete", action, ok)
	}

	items := []map[string]any{{
		"id":                     "cwi-evidence-ready",
		"status":                 "evidence_ready",
		"requiredEvidenceKinds":  []string{runtimemodel.EvidenceKindValidation},
		"acceptedEvidenceByKind": map[string][]string{runtimemodel.EvidenceKindValidation: {"validation-001"}},
	}}
	if action, ok := deriveAllSatisfiedCompleteNextAction(run, items); ok || action.Type != "" {
		t.Fatalf("derive evidence_ready item = %+v/%v, want no complete", action, ok)
	}

	items[0]["status"] = "satisfied"
	delete(items[0], "acceptedEvidenceByKind")
	if action, ok := deriveAllSatisfiedCompleteNextAction(run, items); ok || action.Type != "" {
		t.Fatalf("derive satisfied without evidence = %+v/%v, want no complete", action, ok)
	}

	items[0]["acceptedEvidenceByKind"] = map[string][]string{runtimemodel.EvidenceKindValidation: {"validation-001"}}
	action, ok := deriveAllSatisfiedCompleteNextAction(run, items)
	if !ok || action.Type != runtimemodel.NextActionTypeComplete || action.SubjectID != string(run.ID) {
		t.Fatalf("derive complete = %+v/%v, want complete", action, ok)
	}

	run.State = common.RunStateCompleted
	if action, ok := deriveAllSatisfiedCompleteNextAction(run, items); ok || action.Type != "" {
		t.Fatalf("derive terminal run = %+v/%v, want no complete", action, ok)
	}
}

func TestDeriveOpenDecisionNextActionIgnoresResolvedOrUnreferencedTicket(t *testing.T) {
	run := &runmodel.Run{
		ID:               "run-open-decision-rule",
		WorkspaceID:      "ws-open-decision-rule",
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: "ticket-blocking",
	}
	resolved := &decisionmodel.Ticket{
		ID:          "ticket-blocking",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		State:       common.DecisionTicketStateResolved,
	}
	if action, ok := deriveOpenDecisionNextAction(run, resolved); ok || action.Type != "" {
		t.Fatalf("derive with resolved ticket = %+v/%v, want no request_decision", action, ok)
	}

	unreferencedOpen := &decisionmodel.Ticket{
		ID:          "ticket-other",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		State:       common.DecisionTicketStateOpen,
	}
	if action, ok := deriveOpenDecisionNextAction(run, unreferencedOpen); ok || action.Type != "" {
		t.Fatalf("derive with unreferenced ticket = %+v/%v, want no request_decision", action, ok)
	}
}

func TestDerivePendingApprovalNextActionIgnoresResolvedOrUnreferencedApproval(t *testing.T) {
	run := &runmodel.Run{
		ID:                 "run-pending-approval-rule",
		WorkspaceID:        "ws-pending-approval-rule",
		State:              common.RunStateBlockedOnApproval,
		BlockingApprovalID: "approval-blocking",
	}
	approved := &approvalmodel.Approval{
		ID:          "approval-blocking",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		State:       common.ApprovalStateApproved,
	}
	if action, ok := derivePendingApprovalNextAction(run, approved); ok || action.Type != "" {
		t.Fatalf("derive with approved approval = %+v/%v, want no request_approval", action, ok)
	}

	unreferencedPending := &approvalmodel.Approval{
		ID:          "approval-other",
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		State:       common.ApprovalStatePending,
	}
	if action, ok := derivePendingApprovalNextAction(run, unreferencedPending); ok || action.Type != "" {
		t.Fatalf("derive with unreferenced approval = %+v/%v, want no request_approval", action, ok)
	}
}

func mustCreateWorkspaceForNextAction(t *testing.T, ctx context.Context, repo *memory.WorkspaceRepository, workspaceID common.WorkspaceID, runID common.RunID) {
	t.Helper()
	if err := repo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		State:       common.WorkspaceStateRunning,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
}
