package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestRunSnapshotFromModelsTerminalRunPrefersFailedStepOverStaleCheckpoint(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-terminal-snapshot-step")
	runID := common.RunID("run-terminal-snapshot-step")
	failedStepID := common.StepID("run-terminal-snapshot-step/contract-retry-final")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive},
		Run: &runmodel.Run{
			ID:          runID,
			WorkspaceID: workspaceID,
			State:       common.RunStateFailed,
		},
		Steps: []*runmodel.Step{
			{ID: "run-terminal-snapshot-step/graph-018-completed", RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateSucceeded},
			{ID: failedStepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateFailed},
		},
		Checkpoints: []*runmodel.RuntimeCheckpoint{
			{
				ID:            "checkpoint-stale-completed",
				WorkspaceID:   workspaceID,
				RunID:         runID,
				CurrentStepID: "run-terminal-snapshot-step/graph-018-completed",
				OccurredAt:    "2026-07-04T00:29:35Z",
			},
			{
				ID:            "checkpoint-final-failed",
				WorkspaceID:   workspaceID,
				RunID:         runID,
				CurrentStepID: failedStepID,
				OccurredAt:    "2026-07-04T00:29:30Z",
			},
		},
	})

	if got.Step == nil || got.Step.ID != string(failedStepID) {
		t.Fatalf("Step = %+v, want terminal failed step %s", got.Step, failedStepID)
	}
	if got.Run.CurrentStepID != string(failedStepID) || got.TaskProgress.CurrentStepID != string(failedStepID) {
		t.Fatalf("current step run=%q progress=%q, want %s", got.Run.CurrentStepID, got.TaskProgress.CurrentStepID, failedStepID)
	}
}

func TestRunSnapshotFromModelsRunningRunDoesNotProjectCompletedStepAsActive(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-running-completed-step")
	runID := common.RunID("run-running-completed-step")
	stepID := common.StepID("run-running-completed-step/graph-001/002-build")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive},
		Run:       &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive},
		Steps: []*runmodel.Step{
			{ID: stepID, RunID: runID, Kind: "shell/command", State: common.StepStateSucceeded},
		},
		Checkpoints: []*runmodel.RuntimeCheckpoint{
			{
				ID:            "checkpoint-completed",
				WorkspaceID:   workspaceID,
				RunID:         runID,
				CurrentStepID: stepID,
				OccurredAt:    "2026-07-04T00:29:30Z",
			},
		},
	})

	if got.Step != nil || got.Run.CurrentStepID != "" || got.TaskProgress.CurrentStepID != "" {
		t.Fatalf("current step projection = step:%+v run:%q progress:%q, want no active current step for running completed terminal gap", got.Step, got.Run.CurrentStepID, got.TaskProgress.CurrentStepID)
	}
	status := MapRunStatus(got)
	if status.CurrentStepID != "" || status.CurrentStepState != "" {
		t.Fatalf("status current step = %q/%q, want no active current step", status.CurrentStepID, status.CurrentStepState)
	}
}

func TestCoverageSummaryFromRunSnapshotDerivesStatusCounts(t *testing.T) {
	t.Parallel()

	got := CoverageSummaryFromRunSnapshot(readmodelcore.RunSnapshot{
		CoverageWorkItems: []map[string]any{
			{"id": "open-item", "status": "open"},
			{"id": "empty-status-item"},
			{"id": "evidence-ready-item", "status": "evidence_ready"},
			{"id": "satisfied-item", "status": "satisfied"},
			{"id": "blocked-item", "status": "blocked"},
			{"id": "skipped-item", "status": "skipped"},
			{"id": "superseded-item", "status": "superseded"},
		},
	})
	if got["total"] != 7 || got["open"] != 3 || got["satisfied"] != 1 || got["blocked"] != 1 || got["skipped"] != 1 || got["superseded"] != 1 {
		t.Fatalf("CoverageSummary = %+v, want status counts", got)
	}
	if _, ok := got["coverageWorkItems"]; ok {
		t.Fatalf("CoverageSummary = %+v, must not include full ledger", got)
	}
}

func TestRunOutcomeIgnoresAcceptedResultWhenCompletedRunHasOpenCoverage(t *testing.T) {
	t.Parallel()

	workspaceID := "ws-run-outcome-open-coverage"
	runID := "run-outcome-open-coverage"
	snapshot := readmodelcore.RunSnapshot{
		Workspace: appmodelcore.Workspace{ID: workspaceID},
		Run: appmodelcore.Run{
			ID:          runID,
			WorkspaceID: workspaceID,
			State:       string(common.RunStateCompleted),
		},
		Results: []projectioncontract.Result{{
			ID:           runID + "/result/dod-assessment",
			WorkspaceID:  workspaceID,
			RunID:        runID,
			State:        string(common.ResultStateAccepted),
			SourceType:   resultcommon.SourceTypeDoDAssessment,
			Summary:      "accepted current focus only",
			EvidenceRefs: []string{runID + "/step-1"},
		}},
		CoverageWorkItems: []map[string]any{{
			"id":     "cwi-open",
			"status": "open",
		}},
	}

	if HasAcceptedRunResult(snapshot) {
		t.Fatalf("HasAcceptedRunResult = true, want false while coverage is open")
	}
	if got := ResultProgressState(snapshot); got == "completed" {
		t.Fatalf("ResultProgressState = %q, want non-completed while coverage is open", got)
	}
	outcome := MapRunOutcome(snapshot)
	if outcome.Accepted || outcome.State == "accepted" || outcome.ResultID != "" {
		t.Fatalf("MapRunOutcome = %+v, want accepted result ineligible while coverage is open", outcome)
	}
	if outcome.State != "failed" || outcome.Reason != "run completed without accepted result" {
		t.Fatalf("MapRunOutcome = %+v, want completed drift fallback failure", outcome)
	}
	diagnostics := AuthorityDiagnosticsFromRunSnapshot(snapshot)
	if len(diagnostics) != 1 || diagnostics[0].Code != "completion_gate_failed_but_run_completed" {
		t.Fatalf("AuthorityDiagnostics = %+v, want completion gate drift diagnostic", diagnostics)
	}
}

func TestTaskProgressAssessmentOutcomeShowsPartialProgressAndRejectedGlobalAssessment(t *testing.T) {
	t.Parallel()

	workspaceID := "ws-partial-assessment-outcome"
	runID := "run-partial-assessment-outcome"
	progress := MapLocalizedTaskProgress(readmodelcore.RunSnapshot{
		Workspace: appmodelcore.Workspace{ID: workspaceID},
		Run: appmodelcore.Run{
			ID:          runID,
			WorkspaceID: workspaceID,
			State:       string(common.RunStateActive),
		},
		Results: []projectioncontract.Result{{
			ID:          runID + "/result/acceptance-slice/slice-001",
			WorkspaceID: workspaceID,
			RunID:       runID,
			State:       string(common.ResultStateAccepted),
			SourceType:  resultcommon.SourceTypeAcceptanceSlice,
			Summary:     "current focus accepted",
			SummaryDetail: &projectioncontract.ResultSummary{
				Diagnostics: map[string]any{
					"resultScope":              resultcommon.SourceTypeAcceptanceSlice,
					"finalCompletionGateState": "not_final_result",
				},
			},
		}, {
			ID:          runID + "/result/dod-assessment",
			WorkspaceID: workspaceID,
			RunID:       runID,
			State:       string(common.ResultStateRejected),
			SourceType:  resultcommon.SourceTypeDoDAssessment,
			Summary:     "dod_coverage_missing",
			DoDAssessment: &projectioncontract.DoDAssessment{
				Satisfied: false,
			},
		}},
		CoverageWorkItems: []map[string]any{
			{"id": "cwi-satisfied", "status": "satisfied"},
			{"id": "cwi-open", "status": "open"},
			{"id": "cwi-evidence-ready", "status": "evidence_ready"},
		},
	})

	if progress.AssessmentOutcome.State != "partial_progress_accepted_global_assessment_rejected" {
		t.Fatalf("AssessmentOutcome = %+v, want partial progress plus rejected global assessment", progress.AssessmentOutcome)
	}
	if progress.AssessmentOutcome.AcceptedSlices != 1 ||
		progress.AssessmentOutcome.RejectedGlobalAssessments != 1 ||
		progress.AssessmentOutcome.SatisfiedCoverageCount != 1 ||
		progress.AssessmentOutcome.RemainingCoverageCount != 2 {
		t.Fatalf("AssessmentOutcome = %+v, want accepted/rejected/coverage counts", progress.AssessmentOutcome)
	}
}

func TestGapBaselineNextActionDerivesFromMissingEvidenceKind(t *testing.T) {
	t.Parallel()

	got := GapBaselineFromRunSnapshot(readmodelcore.RunSnapshot{
		Run: appmodelcore.Run{
			State: "running",
		},
		CoverageWorkItemContext: map[string]any{
			"definitionRevision":   "rev-next-action",
			"coverageBaselineHash": "sha256:next-action",
			"currentFocus": map[string]any{
				"id":                   "cwi-state-change",
				"status":               "open",
				"missingEvidenceKinds": []any{"state_change", "validation"},
			},
			"unresolvedItems": []any{map[string]any{
				"id":                   "cwi-state-change",
				"status":               "open",
				"missingEvidenceKinds": []any{"state_change", "validation"},
			}},
		},
	})

	if got == nil {
		t.Fatal("GapBaseline = nil, want derived baseline")
	}
	if got.NextAction.Kind != "produce_state_change" {
		t.Fatalf("NextAction = %+v, want produce_state_change for missing state_change evidence", got.NextAction)
	}
}

func TestGapBaselineNextActionDerivesValidationAfterStateChangeEvidence(t *testing.T) {
	t.Parallel()

	got := GapBaselineFromRunSnapshot(readmodelcore.RunSnapshot{
		Run: appmodelcore.Run{
			State: "running",
		},
		CoverageWorkItemContext: map[string]any{
			"definitionRevision":   "rev-next-validation",
			"coverageBaselineHash": "sha256:next-validation",
			"currentFocus": map[string]any{
				"id":                   "cwi-validation",
				"status":               "open",
				"missingEvidenceKinds": []any{"validation"},
			},
			"unresolvedItems": []any{map[string]any{
				"id":                   "cwi-validation",
				"status":               "open",
				"missingEvidenceKinds": []any{"validation"},
			}},
		},
	})

	if got == nil {
		t.Fatal("GapBaseline = nil, want derived baseline")
	}
	if got.NextAction.Kind != "produce_validation_evidence" {
		t.Fatalf("NextAction = %+v, want produce_validation_evidence for missing validation evidence", got.NextAction)
	}
}

func TestProjectStepViewIncludesCompactContractAuditWithoutFullInput(t *testing.T) {
	t.Parallel()

	projected := ProjectStepView(readmodelcore.StepView{
		ID:    "run-a/step-current-contract",
		Kind:  "llm/workspace_execution",
		State: "completed",
		Input: map[string]any{
			"runtimeStage": "repair",
			"currentActionContract": map[string]any{
				"nextAction": map[string]any{"kind": "state_change"},
				"currentFocus": map[string]any{
					"id":                    "cwi-101",
					"coverageWorkItemIds":   []any{"cwi-101"},
					"missingEvidenceKinds":  []any{"validation", "state_change"},
					"requiredEvidenceKinds": []any{"validation", "state_change"},
				},
				"requiredOutput": map[string]any{
					"coverageFocusId":                       "cwi-101",
					"coverageWorkItemIds":                   []any{"cwi-101"},
					"missingEvidenceKinds":                  []any{"validation", "state_change"},
					"mustReturnExecutionGraph":              true,
					"mustProduceConcreteValidationEvidence": true,
					"largePayloadNotForProjection":          "must not be projected",
				},
			},
		},
		Output: map[string]any{"stage": "llm_decision_required", "status": "failed"},
	})

	if projected.Input != nil {
		t.Fatalf("projected Input = %+v, want omitted full input", projected.Input)
	}
	audit := projected.ContractAudit
	if audit["schemaVersion"] != "workorch.step_contract_audit.v2" ||
		audit["runtimeStage"] != "repair" ||
		audit["currentActionKind"] != "state_change" ||
		audit["currentFocusId"] != "cwi-101" ||
		audit["requiredOutputSource"] != "currentActionContract.requiredOutput" {
		t.Fatalf("ContractAudit = %+v, want compact current contract fields", audit)
	}
	if got := readModelStringSliceValue(audit["missingEvidenceKinds"]); len(got) != 2 || got[0] != "validation" || got[1] != "state_change" {
		t.Fatalf("missingEvidenceKinds = %+v, want validation/state_change", got)
	}
	required := readModelMapValue(audit["requiredOutput"])
	if required["largePayloadNotForProjection"] != nil {
		t.Fatalf("requiredOutput = %+v, want compact output without unrelated payload", required)
	}
	if required["mustReturnExecutionGraph"] != true || required["mustProduceConcreteValidationEvidence"] != true {
		t.Fatalf("requiredOutput = %+v, want compact boolean contract flags", required)
	}
}

func TestProjectStepViewDoesNotProjectLegacyStagePacketContractAudit(t *testing.T) {
	t.Parallel()

	projected := ProjectStepView(readmodelcore.StepView{
		ID:    "run-a/legacy-step",
		Kind:  "llm/workspace_execution",
		State: "completed",
		Input: map[string]any{
			"stagePacket": map[string]any{
				"runtimeStage": "repair",
				"nextAction":   map[string]any{"kind": "state_change"},
			},
		},
	})

	if projected.ContractAudit != nil {
		t.Fatalf("ContractAudit = %+v, want no projection from legacy stagePacket", projected.ContractAudit)
	}
}

func TestProjectStepViewPrefersValidationCurrentActionOverStaleRepairPackage(t *testing.T) {
	t.Parallel()

	projected := ProjectStepView(readmodelcore.StepView{
		ID:    "run-a/step-command-contract",
		Kind:  "llm/workspace_execution",
		State: "failed",
		Input: map[string]any{
			"runtimeStage": "verification",
			"currentActionContract": map[string]any{
				"nextAction": map[string]any{"kind": "validate"},
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence": true,
					"mustIncludeStateChangingNode":          false,
					"acceptedKinds":                         []any{"shell/command", "mcp/tool_call", "validation/managed_session"},
					"forbiddenKinds":                        []any{"patch/apply", "git/write"},
				},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []any{"patch/apply"},
				},
			},
			"validationFailureDiagnostic": map[string]any{
				"class":                         "validation_command_invalid",
				"recommendedEvidenceTargetType": "command_contract",
				"stage":                         "shell_validation_contract_rejected",
				"commandPolicyViolation":        true,
			},
		},
	})

	audit := projected.ContractAudit
	if audit["currentActionKind"] != "validate" ||
		audit["requiredOutputSource"] != "currentActionContract.requiredOutput" {
		t.Fatalf("ContractAudit = %+v, want current validation action as authority", audit)
	}
	required := readModelMapValue(audit["requiredOutput"])
	if required["mustIncludeStateChangingNode"] == true ||
		readModelStringSliceContains(readModelStringSliceValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want stale state-changing package ignored", required)
	}
	diagnostic := readModelMapValue(audit["validationFailureDiagnostic"])
	if diagnostic["class"] != "validation_command_invalid" ||
		diagnostic["recommendedEvidenceTargetType"] != "command_contract" ||
		diagnostic["commandPolicyViolation"] != true {
		t.Fatalf("validationFailureDiagnostic = %+v, want command contract diagnostic visible", diagnostic)
	}
}

func readModelStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestAuthorityDiagnosticsFromRunSnapshotReportsCompletedOpenCoverageDrift(t *testing.T) {
	t.Parallel()

	got := AuthorityDiagnosticsFromRunSnapshot(readmodelcore.RunSnapshot{
		Run: appmodelcore.Run{
			ID:    "run-completed-open-coverage",
			State: string(common.RunStateCompleted),
		},
		CoverageWorkItems: []map[string]any{{
			"id":     "cwi-open",
			"status": "open",
		}, {
			"id":     "cwi-satisfied",
			"status": "satisfied",
		}},
	})

	if len(got) != 1 {
		t.Fatalf("AuthorityDiagnostics = %+v, want one completion drift diagnostic", got)
	}
	if got[0].Code != "completion_gate_failed_but_run_completed" ||
		got[0].Severity != "error" ||
		got[0].Kind != "owner_projection_drift" ||
		got[0].Source != "run.state+coverageWorkItems" {
		t.Fatalf("diagnostic = %+v, want completed/open coverage drift", got[0])
	}
}

func TestAuthorityDiagnosticsFromRunSnapshotReportsStaleValidationResources(t *testing.T) {
	t.Parallel()

	got := AuthorityDiagnosticsFromRunSnapshot(readmodelcore.RunSnapshot{
		Run: appmodelcore.Run{
			ID:    "run-stale-validation-resource",
			State: string(common.RunStateActive),
		},
		Steps: []readmodelcore.StepView{{
			ID:    "run-stale-validation-resource/verify",
			State: string(common.StepStateSucceeded),
			Output: map[string]any{
				"validationResources": []any{map[string]any{
					"kind":   "process",
					"status": "released",
				}, map[string]any{
					"kind":         "scratch_dir",
					"status":       "cleanup_failed",
					"cleanupError": "permission denied",
				}},
			},
		}},
	})

	if len(got) != 1 {
		t.Fatalf("AuthorityDiagnostics = %+v, want one stale resource diagnostic", got)
	}
	if got[0].Code != "stale_validation_resource_lifecycle" ||
		got[0].Severity != "warning" ||
		got[0].Kind != "resource_lifecycle" ||
		got[0].Source != "step[run-stale-validation-resource/verify].output.validationResources" ||
		got[0].Candidate != "scratch_dir:cleanup_failed" {
		t.Fatalf("diagnostic = %+v, want stale validation resource lifecycle diagnostic", got[0])
	}
}

func TestAuthorityDiagnosticsFromRunSnapshotReportsTerminalPlannedValidationResourcesWithoutOutput(t *testing.T) {
	t.Parallel()

	got := AuthorityDiagnosticsFromRunSnapshot(readmodelcore.RunSnapshot{
		Run: appmodelcore.Run{
			ID:    "run-terminal-pending-validation-resource",
			State: string(common.RunStateActive),
		},
		Steps: []readmodelcore.StepView{{
			ID:    "run-terminal-pending-validation-resource/verify",
			State: string(common.StepStateFailed),
			Input: map[string]any{
				"validationResources": []any{map[string]any{
					"kind":   "process_group",
					"status": "pending",
				}},
			},
		}},
	})

	if len(got) != 1 {
		t.Fatalf("AuthorityDiagnostics = %+v, want one stale planned resource diagnostic", got)
	}
	if got[0].Code != "stale_validation_resource_lifecycle" ||
		got[0].Source != "step[run-terminal-pending-validation-resource/verify].input.validationResources" ||
		got[0].Candidate != "process_group:pending" {
		t.Fatalf("diagnostic = %+v, want pending planned validation resource diagnostic", got[0])
	}
}

func TestAuthorityDiagnosticsFromRunSnapshotIgnoresClosedValidationResources(t *testing.T) {
	t.Parallel()

	got := AuthorityDiagnosticsFromRunSnapshot(readmodelcore.RunSnapshot{
		Run: appmodelcore.Run{
			ID:    "run-closed-validation-resource",
			State: string(common.RunStateActive),
		},
		Steps: []readmodelcore.StepView{{
			ID:    "run-closed-validation-resource/verify",
			State: string(common.StepStateSucceeded),
			Output: map[string]any{
				"validationResources": []any{map[string]any{
					"kind":   "process_group",
					"status": "skipped",
				}, map[string]any{
					"kind":   "scratch_dir",
					"status": "removed",
				}, map[string]any{
					"kind":   "scratch_dir",
					"status": "cleanup_skipped",
				}},
			},
		}},
	})

	if len(got) != 0 {
		t.Fatalf("AuthorityDiagnostics = %+v, want no diagnostics for closed validation resources", got)
	}
}

func TestRunRecoverySnapshotFromModelsUsesCheckpointStep(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-run-recovery-projection")
	runID := common.RunID("run-recovery-projection")
	currentStepID := common.StepID("step-current")
	got := RunRecoverySnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "recovery workspace", State: common.WorkspaceStateActive},
		Run:       &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive},
		Steps: []*runmodel.Step{
			{ID: "step-old", RunID: runID, Kind: "llm/planning", State: common.StepStateSucceeded},
			{ID: currentStepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateExecuting},
		},
		Checkpoints: []*runmodel.RuntimeCheckpoint{
			{ID: "checkpoint-current", WorkspaceID: workspaceID, RunID: runID, CurrentStepID: currentStepID},
		},
	})

	if got.Step == nil || got.Step.ID != string(currentStepID) {
		t.Fatalf("Step = %+v, want checkpoint current step %s", got.Step, currentStepID)
	}
	if got.TaskProgress.CurrentStepID != string(currentStepID) {
		t.Fatalf("TaskProgress.CurrentStepID = %q, want %s", got.TaskProgress.CurrentStepID, currentStepID)
	}
	if len(got.TaskDecompositions) != 0 {
		t.Fatalf("TaskDecompositions = %+v, want recovery snapshot without full task decomposition projection", got.TaskDecompositions)
	}
}

func TestRunSnapshotProjectsRejectedExecutionGraphAsTaskPlan(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-rejected-graph-task-plan")
	runID := common.RunID("run-rejected-graph-task-plan")
	rootStepID := common.StepID("step-rejected-graph")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "task plan projection", State: common.WorkspaceStateActive},
		Run:       &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive},
		Steps: []*runmodel.Step{{
			ID:    rootStepID,
			RunID: runID,
			Kind:  "llm/workspace_execution",
			State: common.StepStateFailed,
			Output: mustWorkspaceRunEventPayload(t, map[string]any{
				"stage":            "candidate_rejected",
				"failureClass":     "candidate_rejected",
				"failureSignature": "class=candidate_rejected|reason=validation_evidence_missing",
				"targetScope":      "targetRef=artifact",
				"executionGraph": map[string]any{
					"nodes": []map[string]any{{
						"id":   "validation-node",
						"kind": "shell/command",
						"input": map[string]any{
							"coverageTargets": []any{"coverage-001"},
						},
					}},
				},
			}),
		}},
	})

	if len(got.TaskPlans) != 1 {
		t.Fatalf("TaskPlans = %+v, want rejected plan", got.TaskPlans)
	}
	plan := got.TaskPlans[0]
	if plan.State != "rejected" || plan.SourceOutputState != "rejected" || plan.FailureClass != "candidate_rejected" {
		t.Fatalf("TaskPlan = %+v, want rejected candidate plan", plan)
	}
	if len(plan.Nodes) != 1 || plan.Nodes[0].State != "rejected" || plan.Nodes[0].FailureClass != "candidate_rejected" {
		t.Fatalf("TaskPlan.Nodes = %+v, want rejected node with failure metadata", plan.Nodes)
	}
	if len(plan.Nodes[0].CoverageTargets) != 1 || plan.Nodes[0].CoverageTargets[0] != "coverage-001" {
		t.Fatalf("TaskPlan node coverage = %+v, want projected coverage target", plan.Nodes[0].CoverageTargets)
	}
	if len(got.TaskProgress.Items) == 0 {
		t.Fatalf("TaskProgress.Items empty, want raw activity retained separately")
	}
}

func TestRunSnapshotProjectsRetryWithoutNewGraphAsPlanRevision(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-retry-plan-revision")
	runID := common.RunID("run-retry-plan-revision")
	rootStepID := common.StepID("step-original-plan")
	retryStepID := common.StepID("step-retry")
	ticketID := common.TicketID("ticket-retry-exhausted")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "task retry projection", State: common.WorkspaceStateActive},
		Run: &runmodel.Run{
			ID:               runID,
			WorkspaceID:      workspaceID,
			State:            common.RunStateBlockedOnDecision,
			BlockingTicketID: ticketID,
		},
		Steps: []*runmodel.Step{
			{
				ID:    rootStepID,
				RunID: runID,
				Kind:  "llm/workspace_execution",
				State: common.StepStateSucceeded,
				Output: mustWorkspaceRunEventPayload(t, map[string]any{
					"executionGraph": map[string]any{
						"nodes": []map[string]any{{
							"id":   "patch-node",
							"kind": "patch/apply",
							"input": map[string]any{
								"coverageWorkItemIds": []any{"cwi-001"},
							},
						}},
					},
				}),
			},
			{
				ID:    retryStepID,
				RunID: runID,
				Kind:  "llm/workspace_execution",
				State: common.StepStateFailed,
				Input: mustWorkspaceRunEventPayload(t, map[string]any{
					"retryOf":      string(rootStepID),
					"retryAttempt": float64(2),
				}),
				Output: mustWorkspaceRunEventPayload(t, map[string]any{
					"stage":            "candidate_rejected",
					"failureClass":     "candidate_rejected",
					"failureSignature": "class=candidate_rejected|reason=available_snapshot_ignored",
					"targetScope":      "targetRef=artifact",
					"blockingTicketId": string(ticketID),
				}),
			},
		},
		Decisions: []projectioncontract.Decision{{
			ID:          string(ticketID),
			WorkspaceID: string(workspaceID),
			RunID:       string(runID),
			StepID:      string(retryStepID),
			State:       string(common.DecisionTicketStateOpen),
		}},
	})

	if len(got.TaskPlans) != 2 {
		t.Fatalf("TaskPlans = %+v, want original plan and retry revision", got.TaskPlans)
	}
	var retry projectioncontract.TaskPlan
	for _, item := range got.TaskPlans {
		if item.SourceProjection == "retry" {
			retry = item
			break
		}
	}
	if retry.SourceProjection != "retry" ||
		retry.State != "blocked_on_decision" ||
		retry.RetryOf != string(rootStepID) ||
		retry.RetryAttempt != 2 ||
		retry.FailureClass != "candidate_rejected" ||
		retry.BlockingTicketID != string(ticketID) {
		t.Fatalf("retry TaskPlan = %+v, want blocked retry revision with lineage", retry)
	}
}
