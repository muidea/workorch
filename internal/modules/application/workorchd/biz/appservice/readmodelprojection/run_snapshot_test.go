package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRunSnapshotFromModelsProjectsBlockingDecisionStepAndCoverage(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-run-snapshot-projection")
	runID := common.RunID("run-snapshot-projection")
	blockingStepID := common.StepID("step-blocking")
	ticket := &decisionmodel.Ticket{
		ID:          "ticket-blocking",
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      blockingStepID,
		State:       common.DecisionTicketStateOpen,
	}
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "projection workspace", State: common.WorkspaceStateRunning},
		Run: &runmodel.Run{
			ID:               runID,
			WorkspaceID:      workspaceID,
			State:            common.RunStateBlockedOnDecision,
			BlockingTicketID: ticket.ID,
			ExecutionContext: map[string]any{
				"coverageWorkItems": []any{map[string]any{
					"id":                   "cwi-001",
					"acceptedEvidenceRefs": []any{"evidence-derived-accepted"},
					"rejectedEvidenceByKind": map[string]any{
						"validation": []any{"evidence-derived-rejected"},
					},
				}},
				"coverageWorkItemContext": map[string]any{
					"definitionRevision":   "rev-coverage",
					"coverageBaselineHash": "sha256:baseline",
					"summary":              map[string]any{"open": float64(1)},
					"currentFocus":         map[string]any{"id": "cwi-001", "status": "open"},
					"unresolvedItems": []any{map[string]any{
						"id":                   "cwi-001",
						"sourceScope":          "goal",
						"sourcePath":           "goal.leaves[0]",
						"missingEvidenceKinds": []any{"state_change"},
						"status":               "open",
						"reason":               "missing state change evidence",
					}},
				},
				"acceptanceSlices": []any{map[string]any{
					"id":                    "slice-001",
					"title":                 "required slice",
					"state":                 "active",
					"coverageWorkItemIds":   []any{"cwi-001"},
					"requiredEvidenceKinds": []any{"validation"},
				}},
				"activeSliceId": "slice-001",
				"coverageFocus": map[string]any{
					"id":                  "focus-001",
					"sliceId":             "slice-001",
					"coverageWorkItemIds": []any{"cwi-001"},
					"failureClass":        "validation_failed",
					"state":               "active",
					"attempt":             float64(2),
				},
				"convergenceBudget": map[string]any{
					"decision":              "resume",
					"repairAttempts":        float64(2),
					"noCoverageDeltaRounds": float64(1),
				},
			},
		},
		Steps: []*runmodel.Step{
			{ID: "step-stale", RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateCompleted},
			{ID: blockingStepID, RunID: runID, Kind: "llm/approval", State: common.StepStatePending},
		},
		Checkpoints: []*runmodel.RuntimeCheckpoint{
			{ID: "checkpoint-stale", WorkspaceID: workspaceID, RunID: runID, CurrentStepID: "step-stale"},
		},
		SelectedTicket: ticket,
		Decisions: []readmodelcore.Decision{{
			ID:          string(ticket.ID),
			WorkspaceID: string(workspaceID),
			RunID:       string(runID),
			StepID:      string(blockingStepID),
			State:       string(common.DecisionTicketStateOpen),
		}},
	})

	if got.Step == nil || got.Step.ID != string(blockingStepID) {
		t.Fatalf("Step = %+v, want blocking decision step %s", got.Step, blockingStepID)
	}
	if got.TaskProgress.BlockingTicketID != string(ticket.ID) {
		t.Fatalf("TaskProgress.BlockingTicketID = %q, want %s", got.TaskProgress.BlockingTicketID, ticket.ID)
	}
	if len(got.CoverageWorkItems) != 1 || got.CoverageWorkItems[0]["id"] != "cwi-001" {
		t.Fatalf("CoverageWorkItems = %+v, want copied coverage item", got.CoverageWorkItems)
	}
	if got.CoverageWorkItemContext["summary"] == nil {
		t.Fatalf("CoverageWorkItemContext = %+v, want copied coverage context", got.CoverageWorkItemContext)
	}
	if got.GapBaseline == nil || got.GapBaseline.DefinitionRevision != "rev-coverage" || got.GapBaseline.BaselineRevision != "sha256:baseline" {
		t.Fatalf("GapBaseline = %+v, want projected revision", got.GapBaseline)
	}
	if got.GapBaseline.NextAction.Kind != "wait_decision" {
		t.Fatalf("GapBaseline.NextAction = %+v, want wait_decision", got.GapBaseline.NextAction)
	}
	if len(got.GapBaseline.TodoItems) != 1 || got.GapBaseline.TodoItems[0].SourceRef != "goal.leaves[0]" || got.GapBaseline.TodoItems[0].RequiredEvidenceKinds[0] != "state_change" {
		t.Fatalf("GapBaseline.TodoItems = %+v, want coverage todo", got.GapBaseline.TodoItems)
	}
	if len(got.GapBaseline.AcceptedEvidenceRefs) != 1 || got.GapBaseline.AcceptedEvidenceRefs[0] != "evidence-derived-accepted" {
		t.Fatalf("GapBaseline.AcceptedEvidenceRefs = %+v, want derived accepted evidence", got.GapBaseline.AcceptedEvidenceRefs)
	}
	if len(got.GapBaseline.RejectedEvidenceRefs) != 1 || got.GapBaseline.RejectedEvidenceRefs[0] != "evidence-derived-rejected" {
		t.Fatalf("GapBaseline.RejectedEvidenceRefs = %+v, want derived rejected evidence", got.GapBaseline.RejectedEvidenceRefs)
	}
	if got.Acceptance.ActiveSliceID != "slice-001" || got.Acceptance.Summary.ActiveSlices != 1 {
		t.Fatalf("Acceptance = %+v, want active slice projection", got.Acceptance)
	}
	if got.Acceptance.CurrentFocus == nil || got.Acceptance.CurrentFocus.ID != "focus-001" || got.Acceptance.CurrentFocus.Attempt != 2 {
		t.Fatalf("Acceptance.CurrentFocus = %+v, want projected focus", got.Acceptance.CurrentFocus)
	}
	if got.Acceptance.Budget == nil || got.Acceptance.Budget.RepairAttempts != 2 || got.Acceptance.Budget.NoCoverageDeltaRounds != 1 {
		t.Fatalf("Acceptance.Budget = %+v, want projected budget", got.Acceptance.Budget)
	}
	observability := MapRunObservability(got)
	if observability.Acceptance.ActiveSliceID != "slice-001" {
		t.Fatalf("RunObservability.Acceptance = %+v, want run acceptance", observability.Acceptance)
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

func TestGapBaselineNextActionDerivesFromMissingEvidenceKind(t *testing.T) {
	t.Parallel()

	got := GapBaselineFromRunSnapshot(readmodelcore.RunSnapshot{
		Run: readmodelcore.Run{
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
		Run: readmodelcore.Run{
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

func TestRunSnapshotFromModelsNormalizesStalePersistedGapBaselineNextAction(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-run-snapshot-persisted-gap")
	runID := common.RunID("run-snapshot-persisted-gap")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "projection workspace", State: common.WorkspaceStateRunning},
		Run: &runmodel.Run{
			ID:          runID,
			WorkspaceID: workspaceID,
			State:       common.RunStateRunning,
			ExecutionContext: map[string]any{
				"coverageWorkItemContext": map[string]any{
					"definitionRevision":   "rev-derived",
					"coverageBaselineHash": "sha256:derived",
					"currentFocus": map[string]any{
						"id":     "cwi-derived",
						"status": "evidence_ready",
					},
					"unresolvedItems": []any{map[string]any{
						"id":         "cwi-derived",
						"sourcePath": "goal.derived",
						"status":     "evidence_ready",
					}},
				},
				"gapBaseline": map[string]any{
					"definitionRevision":   "rev-persisted",
					"baselineRevision":     "gap-persisted",
					"acceptedEvidenceRefs": []any{"evidence-accepted"},
					"rejectedEvidenceRefs": []any{"evidence-rejected"},
					"todoItems": []any{map[string]any{
						"id":                    "todo-persisted",
						"source":                "goal",
						"sourceRef":             "goal.persisted",
						"coverageWorkItemIds":   []any{"cwi-persisted"},
						"requiredEvidenceKinds": []any{"state_change"},
						"status":                "open",
						"priority":              float64(7),
						"reason":                "persisted baseline checkpoint",
					}},
					"nextAction": map[string]any{
						"kind":   "state_change",
						"reason": "persisted next action",
					},
				},
			},
		},
	})

	if got.GapBaseline == nil || got.GapBaseline.DefinitionRevision != "rev-persisted" || got.GapBaseline.BaselineRevision != "gap-persisted" {
		t.Fatalf("GapBaseline = %+v, want persisted baseline", got.GapBaseline)
	}
	if len(got.GapBaseline.TodoItems) != 1 || got.GapBaseline.TodoItems[0].ID != "todo-persisted" || got.GapBaseline.TodoItems[0].Priority != 7 {
		t.Fatalf("GapBaseline.TodoItems = %+v, want persisted todo", got.GapBaseline.TodoItems)
	}
	if len(got.GapBaseline.AcceptedEvidenceRefs) != 1 || got.GapBaseline.AcceptedEvidenceRefs[0] != "evidence-accepted" {
		t.Fatalf("GapBaseline.AcceptedEvidenceRefs = %+v, want persisted accepted evidence", got.GapBaseline.AcceptedEvidenceRefs)
	}
	if len(got.GapBaseline.RejectedEvidenceRefs) != 1 || got.GapBaseline.RejectedEvidenceRefs[0] != "evidence-rejected" {
		t.Fatalf("GapBaseline.RejectedEvidenceRefs = %+v, want persisted rejected evidence", got.GapBaseline.RejectedEvidenceRefs)
	}
	if got.GapBaseline.NextAction.Kind != "complete_coverage_assessment" {
		t.Fatalf("GapBaseline.NextAction = %+v, want normalized assessment action", got.GapBaseline.NextAction)
	}
	if len(got.GapBaseline.Diagnostics) != 1 || got.GapBaseline.Diagnostics[0].Code != "stale_persisted_next_action" {
		t.Fatalf("GapBaseline.Diagnostics = %+v, want stale next action diagnostic", got.GapBaseline.Diagnostics)
	}
}

func TestRunSnapshotFromModelsUsesConvergenceAuthorityNextAction(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-authority-next-action-projection")
	runID := common.RunID("run-authority-next-action-projection")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "authority projection", State: common.WorkspaceStateRunning},
		Run: &runmodel.Run{
			ID:          runID,
			WorkspaceID: workspaceID,
			State:       common.RunStateRunning,
			ExecutionContext: map[string]any{
				"coverageWorkItemContext": map[string]any{
					"definitionRevision": "rev-stale-context",
					"currentFocus": map[string]any{
						"id":                   "cwi-stale",
						"status":               "open",
						"missingEvidenceKinds": []any{"state_change"},
					},
					"unresolvedItems": []any{map[string]any{
						"id":                   "cwi-stale",
						"status":               "open",
						"missingEvidenceKinds": []any{"state_change"},
					}},
				},
				"convergenceAuthority": map[string]any{
					"schemaVersion":      "workorch.convergence_authority.v1",
					"workspaceId":        string(workspaceID),
					"runId":              string(runID),
					"definitionRevision": "rev-authority",
					"currentFocus": map[string]any{
						"id":     "cwi-ready",
						"status": "evidence_ready",
					},
					"nextAction": map[string]any{
						"kind":   "assessment",
						"reason": "authority-selected assessment",
					},
					"coverageWorkItemContext": map[string]any{
						"definitionRevision": "rev-authority",
						"currentFocus": map[string]any{
							"id":     "cwi-ready",
							"status": "evidence_ready",
						},
					},
				},
				"gapBaseline": map[string]any{
					"definitionRevision": "rev-persisted",
					"nextAction": map[string]any{
						"kind":   "state_change",
						"reason": "stale persisted action",
					},
				},
			},
		},
	})

	if got.GapBaseline == nil {
		t.Fatal("GapBaseline = nil, want authority-backed baseline")
	}
	if got.GapBaseline.NextAction.Kind != "assessment" {
		t.Fatalf("GapBaseline.NextAction = %+v, want authority assessment", got.GapBaseline.NextAction)
	}
	if len(got.GapBaseline.Diagnostics) != 1 || got.GapBaseline.Diagnostics[0].Code != "stale_persisted_next_action" {
		t.Fatalf("GapBaseline.Diagnostics = %+v, want stale persisted next action diagnostic", got.GapBaseline.Diagnostics)
	}
}

func TestRunSnapshotFromModelsProjectsAuthorityFocusDriftDiagnostics(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-authority-focus-drift-projection")
	runID := common.RunID("run-authority-focus-drift-projection")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "authority focus drift", State: common.WorkspaceStateRunning},
		Run: &runmodel.Run{
			ID:          runID,
			WorkspaceID: workspaceID,
			State:       common.RunStateRunning,
			ExecutionContext: map[string]any{
				"convergenceAuthority": map[string]any{
					"currentFocus": map[string]any{"id": "cwi-current"},
					"nextAction":   map[string]any{"kind": "validate"},
				},
				"gapBaseline": map[string]any{
					"currentFocus": map[string]any{"id": "cwi-current"},
					"nextAction":   map[string]any{"kind": "validate"},
				},
				"coverageFocus": map[string]any{
					"id":                  "focus-cwi-stale",
					"coverageWorkItemIds": []any{"cwi-stale"},
				},
				"acceptance": map[string]any{
					"currentFocus": map[string]any{
						"id":                  "focus-cwi-stale",
						"coverageWorkItemIds": []any{"cwi-stale"},
					},
				},
				"repairActionPackage": map[string]any{
					"requiredOutput": map[string]any{
						"coverageFocusId":     "focus-cwi-stale",
						"coverageWorkItemIds": []any{"cwi-stale"},
					},
				},
				"stagePacket": map[string]any{
					"activeFocus": map[string]any{"id": "cwi-current"},
					"requiredOutput": map[string]any{
						"coverageFocusId": "focus-cwi-stale",
					},
				},
			},
		},
	})

	if len(got.AuthorityDiagnostics) < 3 {
		t.Fatalf("AuthorityDiagnostics = %+v, want focus drift diagnostics", got.AuthorityDiagnostics)
	}
	for _, diagnostic := range got.AuthorityDiagnostics {
		if diagnostic.Code != "authority_focus_drift" || diagnostic.Severity != "warning" {
			t.Fatalf("AuthorityDiagnostics item = %+v, want warning authority_focus_drift", diagnostic)
		}
		if diagnostic.Kind != "stale_persisted_derived_field" || diagnostic.Source == "" ||
			diagnostic.Candidate == "" || diagnostic.CurrentAuthority != "cwi-current" {
			t.Fatalf("AuthorityDiagnostics item = %+v, want stale persisted derived field source/candidate/authority", diagnostic)
		}
	}
}

func TestRunRecoverySnapshotFromModelsUsesCheckpointStep(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-run-recovery-projection")
	runID := common.RunID("run-recovery-projection")
	currentStepID := common.StepID("step-current")
	got := RunRecoverySnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "recovery workspace", State: common.WorkspaceStateRunning},
		Run:       &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning},
		Steps: []*runmodel.Step{
			{ID: "step-old", RunID: runID, Kind: "llm/planning", State: common.StepStateCompleted},
			{ID: currentStepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateDispatched},
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

func TestRunSnapshotFromModelsProjectsRuntimeExecutionFinalizeGap(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-runtime-exec-gap")
	runID := common.RunID("run-runtime-exec-gap")
	stepID := common.StepID("step-runtime-exec-gap")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "runtime gap workspace", State: common.WorkspaceStateRunning},
		Run:       &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning},
		Steps: []*runmodel.Step{
			{ID: stepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateDispatched},
		},
		Checkpoints: []*runmodel.RuntimeCheckpoint{
			{
				ID:            "checkpoint-accepted",
				WorkspaceID:   workspaceID,
				RunID:         runID,
				CurrentStepID: stepID,
				OccurredAt:    "2026-06-17T01:00:00Z",
				Data: map[string]any{
					"event":      "runtime_execute_accepted",
					"requestId":  "request-001",
					"queueKey":   "queue-001",
					"acceptedAt": "2026-06-17T01:00:00Z",
				},
			},
			{
				ID:            "checkpoint-done",
				WorkspaceID:   workspaceID,
				RunID:         runID,
				CurrentStepID: stepID,
				OccurredAt:    "2026-06-17T01:00:05Z",
				Data: map[string]any{
					"event":     "runtime_step_done",
					"requestId": "request-001",
					"stage":     "stream_done",
					"summary":   "runtime completed but finalize has not run",
				},
			},
		},
	})

	if got.RuntimeExecution == nil {
		t.Fatal("RuntimeExecution is nil, want projected runtime gap")
	}
	if got.RuntimeExecution.RequestID != "request-001" ||
		got.RuntimeExecution.QueueKey != "queue-001" ||
		got.RuntimeExecution.DoneAt != "2026-06-17T01:00:05Z" ||
		got.RuntimeExecution.Pending != true ||
		got.RuntimeExecution.Gap != "runtime_terminal_not_finalized" {
		t.Fatalf("RuntimeExecution = %+v, want done-not-finalized gap", got.RuntimeExecution)
	}
}

func TestRunSnapshotFromModelsProjectsRuntimeExecutionResultAvailableGap(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-runtime-result-gap")
	runID := common.RunID("run-runtime-result-gap")
	stepID := common.StepID("step-runtime-result-gap")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "runtime result gap workspace", State: common.WorkspaceStateRunning},
		Run:       &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning},
		Steps: []*runmodel.Step{
			{ID: stepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateDispatched},
		},
		Checkpoints: []*runmodel.RuntimeCheckpoint{
			{
				ID:            "checkpoint-result-available",
				WorkspaceID:   workspaceID,
				RunID:         runID,
				CurrentStepID: stepID,
				OccurredAt:    "2026-06-17T01:00:05Z",
				Data: map[string]any{
					"event":       "runtime_step_result_available",
					"requestId":   "request-result",
					"completedAt": "2026-06-17T01:00:04Z",
					"output":      map[string]any{"status": "ok"},
				},
			},
		},
	})

	if got.RuntimeExecution == nil {
		t.Fatal("RuntimeExecution is nil, want projected runtime result gap")
	}
	if got.RuntimeExecution.RequestID != "request-result" ||
		got.RuntimeExecution.DoneAt != "2026-06-17T01:00:04Z" ||
		got.RuntimeExecution.Pending != true ||
		got.RuntimeExecution.Gap != "runtime_terminal_not_finalized" {
		t.Fatalf("RuntimeExecution = %+v, want result-available finalize gap", got.RuntimeExecution)
	}
}

func TestRunSnapshotFromModelsProjectsContractViolationWithoutRuntimeProgress(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-run-contract-violation-projection")
	runID := common.RunID("run-contract-violation-projection")
	stepID := common.StepID("step-contract-violation")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "contract violation projection", State: common.WorkspaceStateRunning},
		Run:       &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning},
		Steps: []*runmodel.Step{{
			ID:    stepID,
			RunID: runID,
			Kind:  "llm/workspace_execution",
			State: common.StepStateFailed,
			Output: map[string]any{
				"contractViolation": map[string]any{
					"id":               "violation-001",
					"workspaceId":      string(workspaceID),
					"runId":            string(runID),
					"stepId":           string(stepID),
					"violatedRule":     "state_changing_required_missing",
					"recoverability":   "retry_same_contract",
					"failureSignature": "class=runtime_failed|rule=state_changing_required_missing",
				},
			},
		}},
	})

	if got.RuntimeExecution == nil {
		t.Fatal("RuntimeExecution is nil, want contract violation projection")
	}
	if got.RuntimeExecution.ContractViolation == nil ||
		got.RuntimeExecution.ContractViolation.ID != "violation-001" ||
		got.RuntimeExecution.ContractViolation.ViolatedRule != "state_changing_required_missing" ||
		got.RuntimeExecution.ContractViolation.Recoverability != "retry_same_contract" {
		t.Fatalf("RuntimeExecution.ContractViolation = %+v, want projected contract violation", got.RuntimeExecution.ContractViolation)
	}
}

func TestRunSnapshotProjectsRejectedExecutionGraphAsTaskPlan(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-rejected-graph-task-plan")
	runID := common.RunID("run-rejected-graph-task-plan")
	rootStepID := common.StepID("step-rejected-graph")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "task plan projection", State: common.WorkspaceStateRunning},
		Run:       &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning},
		Steps: []*runmodel.Step{{
			ID:    rootStepID,
			RunID: runID,
			Kind:  "llm/workspace_execution",
			State: common.StepStateFailed,
			Output: map[string]any{
				"stage":                 "runtime_contract_failed",
				"contractViolationType": "validation_evidence_required",
				"contractViolation": map[string]any{
					"id":               "violation-rejected-graph",
					"sourceStepId":     string(rootStepID),
					"violatedRule":     "concrete_validation_contract_missing",
					"failureClass":     "runtime_failed",
					"failureSignature": "class=runtime_failed|rule=concrete_validation_contract_missing",
					"targetScope":      "targetRef=artifact",
					"sourceRefs":       []any{string(rootStepID), "validation-node"},
				},
				"executionGraph": map[string]any{
					"nodes": []map[string]any{{
						"id":   "validation-node",
						"kind": "shell/command",
						"input": map[string]any{
							"coverageTargets": []any{"coverage-001"},
						},
					}},
				},
			},
		}},
	})

	if len(got.TaskPlans) != 1 {
		t.Fatalf("TaskPlans = %+v, want rejected plan", got.TaskPlans)
	}
	plan := got.TaskPlans[0]
	if plan.State != "rejected" || plan.SourceOutputState != "rejected" || plan.ViolatedRule != "concrete_validation_contract_missing" {
		t.Fatalf("TaskPlan = %+v, want rejected contract violation plan", plan)
	}
	if len(plan.Nodes) != 1 || plan.Nodes[0].State != "rejected" || plan.Nodes[0].ViolatedRule != "concrete_validation_contract_missing" {
		t.Fatalf("TaskPlan.Nodes = %+v, want rejected node with violation metadata", plan.Nodes)
	}
	if len(plan.Nodes[0].CoverageTargets) != 1 || plan.Nodes[0].CoverageTargets[0] != "coverage-001" {
		t.Fatalf("TaskPlan node coverage = %+v, want projected coverage target", plan.Nodes[0].CoverageTargets)
	}
	if len(got.TaskProgress.Items) == 0 {
		t.Fatalf("TaskProgress.Items empty, want raw activity retained separately")
	}
}

func TestRunSnapshotProjectsContractRetryWithoutNewGraphAsPlanRevision(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-retry-plan-revision")
	runID := common.RunID("run-retry-plan-revision")
	rootStepID := common.StepID("step-original-plan")
	retryStepID := common.StepID("step-contract-retry")
	ticketID := common.TicketID("ticket-retry-exhausted")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "task retry projection", State: common.WorkspaceStateRunning},
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
				State: common.StepStateCompleted,
				Output: map[string]any{
					"executionGraph": map[string]any{
						"nodes": []map[string]any{{
							"id":   "patch-node",
							"kind": "patch/apply",
							"input": map[string]any{
								"coverageWorkItemIds": []any{"cwi-001"},
							},
						}},
					},
				},
			},
			{
				ID:    retryStepID,
				RunID: runID,
				Kind:  "llm/workspace_execution",
				State: common.StepStateFailed,
				Input: map[string]any{
					"contractViolationRecovery": map[string]any{
						"sourceStepId":     string(rootStepID),
						"attempt":          float64(2),
						"violatedRule":     "available_snapshot_ignored",
						"failureClass":     "runtime_failed",
						"failureSignature": "class=runtime_failed|rule=available_snapshot_ignored",
						"targetScope":      "targetRef=artifact",
					},
				},
				Output: map[string]any{
					"stage":            "runtime_contract_failed",
					"blockingTicketId": string(ticketID),
				},
			},
		},
		Decisions: []readmodelcore.Decision{{
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
	var retry readmodelcore.TaskPlan
	for _, item := range got.TaskPlans {
		if item.SourceProjection == "contract_violation_retry" {
			retry = item
			break
		}
	}
	if retry.SourceProjection != "contract_violation_retry" ||
		retry.State != "blocked_on_decision" ||
		retry.RetryOf != string(rootStepID) ||
		retry.RetryAttempt != 2 ||
		retry.ViolatedRule != "available_snapshot_ignored" ||
		retry.BlockingTicketID != string(ticketID) {
		t.Fatalf("retry TaskPlan = %+v, want blocked retry revision with lineage", retry)
	}
}

func TestRunSnapshotFromModelsProjectsRuntimeExecutionWorkerLostGap(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-runtime-worker-lost")
	runID := common.RunID("run-runtime-worker-lost")
	stepID := common.StepID("step-runtime-worker-lost")
	got := RunSnapshotFromModels(RunSnapshotModelInput{
		Workspace: &workspacemodel.Workspace{ID: workspaceID, Title: "runtime worker lost workspace", State: common.WorkspaceStateRunning},
		Run:       &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning},
		Steps: []*runmodel.Step{
			{ID: stepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateDispatched},
		},
		Checkpoints: []*runmodel.RuntimeCheckpoint{
			{
				ID:            "checkpoint-accepted",
				WorkspaceID:   workspaceID,
				RunID:         runID,
				CurrentStepID: stepID,
				OccurredAt:    "2026-06-17T01:00:00Z",
				Data: map[string]any{
					"event":      "runtime_execute_accepted",
					"requestId":  "request-worker-lost",
					"queueKey":   "queue-worker-lost",
					"acceptedAt": "2026-06-17T01:00:00Z",
					"runtimeRecoveryPolicy": runtimemodel.RuntimeRecoveryPolicy{
						SideEffectPolicy:  runtimemodel.SideEffectPolicyExternalRead,
						IdempotencyPolicy: runtimemodel.IdempotencyPolicyIdempotent,
						WatchdogRecovery:  runtimemodel.WatchdogRecoveryRetryExternal,
					}.ToMap(),
					"recoverySafety": "recoverable_idempotent",
				},
			},
			{
				ID:            "checkpoint-worker-heartbeat",
				WorkspaceID:   workspaceID,
				RunID:         runID,
				CurrentStepID: stepID,
				OccurredAt:    "2026-06-17T01:04:00Z",
				Data: map[string]any{
					"event":       "runtime_worker_heartbeat",
					"workerId":    "runtimeevent|request-worker-lost",
					"heartbeatAt": "2026-06-17T01:04:00Z",
				},
			},
			{
				ID:            "checkpoint-worker-lost",
				WorkspaceID:   workspaceID,
				RunID:         runID,
				CurrentStepID: stepID,
				OccurredAt:    "2026-06-17T01:05:00Z",
				Data: map[string]any{
					"event":    "runtime_worker_lost",
					"workerId": "runtimeevent|request-worker-lost",
					"lostAt":   "2026-06-17T01:05:00Z",
				},
			},
		},
	})

	if got.RuntimeExecution == nil {
		t.Fatal("RuntimeExecution is nil, want projected worker lost gap")
	}
	if got.RuntimeExecution.RequestID != "request-worker-lost" ||
		got.RuntimeExecution.QueueKey != "queue-worker-lost" ||
		got.RuntimeExecution.WorkerID != "runtimeevent|request-worker-lost" ||
		got.RuntimeExecution.WorkerHeartbeatAt != "2026-06-17T01:04:00Z" ||
		got.RuntimeExecution.WorkerLostAt != "2026-06-17T01:05:00Z" ||
		got.RuntimeExecution.Pending != true ||
		got.RuntimeExecution.Gap != "worker_lost" {
		t.Fatalf("RuntimeExecution = %+v, want worker lost gap", got.RuntimeExecution)
	}
	if got.RuntimeExecution.RecoverySafety != "recoverable_idempotent" ||
		got.RuntimeExecution.RuntimeRecoveryPolicy["watchdogRecovery"] != runtimemodel.WatchdogRecoveryRetryExternal {
		t.Fatalf("RuntimeExecution policy/safety = %+v/%q, want recoverable retry policy", got.RuntimeExecution.RuntimeRecoveryPolicy, got.RuntimeExecution.RecoverySafety)
	}
}
