package orchestrator

import (
	"context"
	"errors"
	"testing"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOrchestrator_RunningResumeReplansFromResolvedDecisionOnFailedNonTerminalStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	runtime := &requestCapturingRuntimeAdapter{fn: func(request runtimemodel.StepExecutionRequest) map[string]any {
		if boolValue(request.Input["graphFollowUp"]) ||
			boolValue(request.Input[stepInputKeyAssessmentOnly]) ||
			boolValue(request.Input[stepInputKeyGraphLimitFollowUp]) ||
			boolValue(request.Input["assessmentOnlyContractCorrection"]) {
			evidenceRefs := []string{string(request.StepID)}
			if evidence := mapSliceFromValue(request.Input[stepInputKeyGraphEvidence]); len(evidence) > 0 {
				for _, item := range evidence {
					if ref := stringFromMap(item, "stepId"); ref != "" {
						evidenceRefs = append(evidenceRefs, ref)
					}
				}
				evidenceRefs = compactUniqueStrings(evidenceRefs)
			}
			return map[string]any{
				"status": "completed",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied":   true,
					"summary":     "runtime replan resume assessment completed",
					"goalOutcome": "runtime replan resume test fixture completed",
					"items": []map[string]any{{
						"coverageWorkItemId":    "cwi-001",
						"item":                  "current requested outcome remains governed by the current definition",
						"state":                 "satisfied",
						"requiredEvidenceKinds": []string{"validation"},
						"evidenceRefs":          evidenceRefs,
					}},
				},
			}
		}
		if request.Input["repairObjective"] == repairObjectiveConcreteValidationEvidence || boolValue(request.Input["boundedValidationRequired"]) {
			return map[string]any{
				"status":             "completed",
				"stage":              "llm_completion_assessment",
				"coverageWorkItemId": "cwi-001",
				"coverageTargets": []map[string]any{{
					"coverageWorkItemId":  "cwi-001",
					"sourceRequirementId": "dod-running-resolved-replan-item",
					"targetText":          "current requested outcome remains governed by the current definition",
				}},
				"validationEvidence": map[string]any{
					"mode":            "controlled_substitute",
					"observationType": "controlled_substitute_observation",
					"substituteFor":   "runtime replan resume test fixture",
					"targetRefs":      []string{"current requested outcome remains governed by the current definition"},
					"observedClaims":  []string{"runtime replan resume test fixture completed"},
				},
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied":   true,
					"summary":     "runtime replan resume assessment completed",
					"goalOutcome": "runtime replan resume test fixture completed",
					"items": []map[string]any{{
						"coverageWorkItemId":    "cwi-001",
						"item":                  "current requested outcome remains governed by the current definition",
						"state":                 "satisfied",
						"requiredEvidenceKinds": []string{"validation"},
						"evidenceRefs":          []string{string(request.StepID), string(request.StepID) + "/validation/runtime-step"},
					}},
				},
			}
		}
		return map[string]any{"status": "completed"}
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-running-resolved-replan")
	runID := common.RunID("run-running-resolved-replan")
	ticketStepID := common.StepID(runID + "/provider-retry-001")
	terminalStepID := common.StepID(runID + "/repair-branch-001")
	ticketID := common.TicketID(string(ticketStepID) + "-duplicate-failure-decision")
	resolutionID := common.ResolutionID("resolution-" + string(ticketID))
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		GoalID:             "goal-running-resolved-replan",
		DoDID:              "dod-running-resolved-replan",
		DefinitionRevision: "rev-running-resolved-replan",
		State:              common.RunStateRunning,
		InitialStepKind:    llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-running-resolved-replan",
			"goal": map[string]any{
				"id":   "goal-running-resolved-replan",
				"text": "- complete the current requested outcome after replan",
			},
			"dod": map[string]any{
				"id": "dod-running-resolved-replan",
				"items": []map[string]any{{
					"id":                    "dod-running-resolved-replan-item",
					"text":                  "current requested outcome remains governed by the current definition",
					"requiredEvidenceKinds": []string{"validation"},
				}},
			},
			convergenceBudgetKey: map[string]any{
				"state":                     "escalated",
				"decision":                  "escalate_decision",
				"focusId":                   "focus-cwi-001",
				"sliceId":                   "slice-primary-state-change",
				"repairAttempts":            int64(4),
				"llmRounds":                 int64(4),
				"validationOnlyRetries":     int64(3),
				"noCoverageDeltaRounds":     int64(3),
				"repeatedFailureSignatures": int64(2),
				"lastDecisionReason":        "max_repair_attempts_per_focus_exceeded",
				"lastFailureSignature":      "class=shell_command_failed|kind=shell/command",
				"maxRepairAttemptsPerFocus": int64(7),
			},
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    ticketStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":               "llm_schema_invalid",
			failureOutputKeyClass: failureClassLLMDecisionInvalid,
		},
	}); err != nil {
		t.Fatalf("seed ticket step: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    terminalStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			"previousStepId": string(ticketStepID),
		},
		Output: map[string]any{
			"stage":               "llm_schema_invalid",
			failureOutputKeyClass: failureClassLLMDecisionInvalid,
		},
	}); err != nil {
		t.Fatalf("seed terminal step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      ticketStepID,
		State:       common.DecisionTicketStateResolved,
		CreatedAt:   "2026-05-20T11:31:32Z",
		UpdatedAt:   "2026-05-20T11:31:33Z",
		Context: map[string]any{
			"allowedRecoveryActions": []any{"replan"},
			"recoveryPolicy":         "runtime_repair_exhausted_requires_strategy",
		},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       resolutionID,
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeReplan,
		Payload: map[string]any{
			"source":             "auto_governance",
			"recoveryAction":     "replan",
			"goalId":             "goal-from-resolution-should-not-override",
			"doDId":              "dod-from-resolution-should-not-override",
			"definitionRevision": "rev-from-resolution-should-not-override",
		},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume running resolved replan: %v", err)
	}

	predecessor, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get predecessor: %v", err)
	}
	if predecessor.State != common.RunStateSuperseded {
		t.Fatalf("predecessor state = %s, want superseded", predecessor.State)
	}
	successorID := common.RunID("run-running-resolved-replan-replan-001")
	successor, err := runRepo.Get(ctx, successorID)
	if err != nil {
		t.Fatalf("get successor: %v", err)
	}
	if successor.ExecutionContext == nil || successor.ExecutionContext["replanContinuation"] == nil {
		t.Fatalf("successor execution context = %#v, want replan continuation", successor.ExecutionContext)
	}
	if successor.GoalID != "goal-running-resolved-replan" ||
		successor.DoDID != "dod-running-resolved-replan" ||
		successor.DefinitionRevision != "rev-running-resolved-replan" {
		t.Fatalf("successor definitions = goal:%q dod:%q revision:%q, want predecessor definitions", successor.GoalID, successor.DoDID, successor.DefinitionRevision)
	}
	if stringFromMap(successor.ExecutionContext, "definitionRevision") != "rev-running-resolved-replan" {
		t.Fatalf("successor executionContext.definitionRevision = %q, want inherited current revision", successor.ExecutionContext["definitionRevision"])
	}
	successorBudget := mapFromValue(successor.ExecutionContext[convergenceBudgetKey])
	if stringFromMap(successorBudget, "decision") != "continue" || stringFromMap(successorBudget, "state") != "tracking" {
		t.Fatalf("successor convergenceBudget = %+v, want reset tracking budget", successorBudget)
	}
	if reason := stringFromMap(successorBudget, "lastDecisionReason"); reason == "max_repair_attempts_per_focus_exceeded" {
		t.Fatalf("successor convergenceBudget retained predecessor decision reason: %+v", successorBudget)
	}
	if got := int64FromValue(successorBudget["maxRepairAttemptsPerFocus"]); got != 7 {
		t.Fatalf("successor maxRepairAttemptsPerFocus = %d, want inherited limit 7", got)
	}
	if baseline := mapFromValue(successor.ExecutionContext[gapBaselineKey]); len(baseline) == 0 {
		t.Fatalf("successor execution context = %#v, want gapBaseline after replan start", successor.ExecutionContext)
	}
	if len(runtime.requests) == 0 {
		t.Fatalf("runtime requests = 0, want successor dispatch")
	}
	if baseline := mapFromValue(runtime.requests[len(runtime.requests)-1].Input[gapBaselineKey]); len(baseline) == 0 {
		t.Fatalf("successor runtime input = %#v, want gapBaseline", runtime.requests[len(runtime.requests)-1].Input)
	}
	requestBudget := mapFromValue(mapFromValue(runtime.requests[len(runtime.requests)-1].Input["acceptance"])["budget"])
	if stringFromMap(requestBudget, "decision") != "continue" || stringFromMap(requestBudget, "state") != "tracking" {
		t.Fatalf("successor runtime input acceptance budget = %+v, want reset tracking budget", requestBudget)
	}
}

func TestRuntimeOrchestrator_BlockerAutoResumePolicyMatrix(t *testing.T) {
	type expected struct {
		err              error
		state            common.RunState
		blockingTicket   bool
		blockingApproval bool
		runtimeCalls     int
	}
	tests := []struct {
		name string
		seed func(ctx context.Context, t *testing.T, orch *RuntimeOrchestrator, repos blockerPolicyRepos) common.RunID
		want expected
	}{
		{
			name: "duplicate failure below scoped repair budget can resume",
			seed: func(ctx context.Context, t *testing.T, _ *RuntimeOrchestrator, repos blockerPolicyRepos) common.RunID {
				t.Helper()
				workspaceID := common.WorkspaceID("ws-policy-duplicate")
				runID := common.RunID("run-policy-duplicate")
				failedStepID := common.StepID(runID + "/block/verify")
				ticketID := common.TicketID(string(failedStepID) + "-duplicate-failure-decision")
				signature := "class=shell_command_failed|kind=shell/command|node=verify|failureTargets=config/settings.yaml|exitCode=1"
				seedBlockerPolicyWorkspaceAndRun(ctx, t, repos, workspaceID, runID, common.RunStateBlockedOnDecision, ticketID, "")
				if err := repos.steps.Create(ctx, &runmodel.Step{
					ID:    common.StepID(runID + "/repair-1"),
					RunID: runID,
					Kind:  llmcommon.StepKindWorkspaceExecution,
					State: common.StepStateCompleted,
					Input: map[string]any{
						stepInputKeyRuntimeRepairFollowUp: true,
						stepInputKeyRuntimeRepairAttempt:  1,
						stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
						stepInputKeyRepairOfFailureSig:    signature,
					},
					Output: map[string]any{"status": "completed"},
				}); err != nil {
					t.Fatalf("seed repair step: %v", err)
				}
				if err := repos.steps.Create(ctx, &runmodel.Step{
					ID:    failedStepID,
					RunID: runID,
					Kind:  shellcommon.StepKindCommand,
					State: common.StepStateFailed,
					Output: map[string]any{
						failureOutputKeyClass:     failureClassShellCommand,
						failureOutputKeySignature: signature,
					},
				}); err != nil {
					t.Fatalf("seed failed step: %v", err)
				}
				if err := repos.tickets.Create(ctx, &decisionmodel.Ticket{
					ID:          ticketID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					StepID:      failedStepID,
					State:       common.DecisionTicketStateOpen,
					Context: map[string]any{
						"failureClass":     failureClassShellCommand,
						"failureSignature": signature,
						"failedStepId":     string(failedStepID),
					},
				}); err != nil {
					t.Fatalf("seed duplicate ticket: %v", err)
				}
				return runID
			},
			want: expected{state: common.RunStateCompleted, runtimeCalls: 1},
		},
		{
			name: "runtime repair exhaustion without success evidence stays blocked",
			seed: func(ctx context.Context, t *testing.T, _ *RuntimeOrchestrator, repos blockerPolicyRepos) common.RunID {
				t.Helper()
				workspaceID := common.WorkspaceID("ws-policy-repair-exhausted")
				runID := common.RunID("run-policy-repair-exhausted")
				failedStepID := common.StepID(runID + "/block/verify")
				ticketID := common.TicketID(string(failedStepID) + "-repair-exhausted-decision")
				signature := "class=runtime_failed|target=verification"
				seedBlockerPolicyWorkspaceAndRun(ctx, t, repos, workspaceID, runID, common.RunStateBlockedOnDecision, ticketID, "")
				if err := repos.steps.Create(ctx, &runmodel.Step{
					ID:    failedStepID,
					RunID: runID,
					Kind:  shellcommon.StepKindCommand,
					State: common.StepStateFailed,
					Output: map[string]any{
						failureOutputKeyClass:     failureClassRuntime,
						failureOutputKeySignature: signature,
					},
				}); err != nil {
					t.Fatalf("seed failed step: %v", err)
				}
				if err := repos.tickets.Create(ctx, &decisionmodel.Ticket{
					ID:          ticketID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					StepID:      failedStepID,
					State:       common.DecisionTicketStateOpen,
					Context: map[string]any{
						decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
						"failureClass":                   failureClassRuntime,
						"failureSignature":               signature,
						"failedStepId":                   string(failedStepID),
					},
				}); err != nil {
					t.Fatalf("seed repair exhaustion ticket: %v", err)
				}
				return runID
			},
			want: expected{err: common.ErrBlocked, state: common.RunStateBlockedOnDecision, blockingTicket: true},
		},
		{
			name: "runtime repair exhaustion with completed coverage action evidence can resume without target scope",
			seed: func(ctx context.Context, t *testing.T, _ *RuntimeOrchestrator, repos blockerPolicyRepos) common.RunID {
				t.Helper()
				workspaceID := common.WorkspaceID("ws-policy-repair-exhausted-coverage-evidence")
				runID := common.RunID("run-policy-repair-exhausted-coverage-evidence")
				failedStepID := common.StepID(runID + "/block/assessment")
				ticketID := common.TicketID(string(failedStepID) + "-repair-exhausted-decision")
				signature := "class=runtime_failed|stage=completion_gate|reason=coverage_missing"
				seedBlockerPolicyWorkspaceAndRun(ctx, t, repos, workspaceID, runID, common.RunStateBlockedOnDecision, ticketID, "")
				if err := repos.steps.Create(ctx, &runmodel.Step{
					ID:    failedStepID,
					RunID: runID,
					Kind:  llmcommon.StepKindWorkspaceExecution,
					State: common.StepStateFailed,
					Output: map[string]any{
						failureOutputKeyClass:     failureClassRuntime,
						failureOutputKeySignature: signature,
					},
				}); err != nil {
					t.Fatalf("seed failed step: %v", err)
				}
				if err := repos.tickets.Create(ctx, &decisionmodel.Ticket{
					ID:          ticketID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					StepID:      failedStepID,
					State:       common.DecisionTicketStateOpen,
					Context: map[string]any{
						decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
						"failureClass":                   failureClassRuntime,
						"failureSignature":               signature,
						"failedStepId":                   string(failedStepID),
						"attempts":                       defaultMaxRuntimeRepairAttempts,
						"coverageTargetSet": []map[string]any{{
							"targetText": "required coverage target",
							"evidenceCandidates": []map[string]any{{
								"kind":  shellcommon.StepKindCommand,
								"state": common.StepStateCompleted,
								"ref":   string(runID) + "/graph-009/verify",
							}},
						}},
					},
				}); err != nil {
					t.Fatalf("seed repair exhaustion ticket: %v", err)
				}
				return runID
			},
			want: expected{state: common.RunStateCompleted, runtimeCalls: 1},
		},
		{
			name: "runtime repair exhaustion with only read or failed coverage evidence stays blocked",
			seed: func(ctx context.Context, t *testing.T, _ *RuntimeOrchestrator, repos blockerPolicyRepos) common.RunID {
				t.Helper()
				workspaceID := common.WorkspaceID("ws-policy-repair-exhausted-read-evidence")
				runID := common.RunID("run-policy-repair-exhausted-read-evidence")
				failedStepID := common.StepID(runID + "/block/assessment")
				ticketID := common.TicketID(string(failedStepID) + "-repair-exhausted-decision")
				signature := "class=runtime_failed|stage=completion_gate|reason=coverage_missing"
				seedBlockerPolicyWorkspaceAndRun(ctx, t, repos, workspaceID, runID, common.RunStateBlockedOnDecision, ticketID, "")
				if err := repos.steps.Create(ctx, &runmodel.Step{
					ID:    failedStepID,
					RunID: runID,
					Kind:  llmcommon.StepKindWorkspaceExecution,
					State: common.StepStateFailed,
					Output: map[string]any{
						failureOutputKeyClass:     failureClassRuntime,
						failureOutputKeySignature: signature,
					},
				}); err != nil {
					t.Fatalf("seed failed step: %v", err)
				}
				if err := repos.tickets.Create(ctx, &decisionmodel.Ticket{
					ID:          ticketID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					StepID:      failedStepID,
					State:       common.DecisionTicketStateOpen,
					Context: map[string]any{
						decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
						"failureClass":                   failureClassRuntime,
						"failureSignature":               signature,
						"failedStepId":                   string(failedStepID),
						"attempts":                       defaultMaxRuntimeRepairAttempts,
						"coverageTargetSet": []map[string]any{{
							"targetText": "required coverage target",
							"evidenceCandidates": []map[string]any{
								{"kind": "file/read", "state": common.StepStateCompleted, "ref": string(runID) + "/graph-009/read"},
								{"kind": shellcommon.StepKindCommand, "state": common.StepStateFailed, "ref": string(runID) + "/graph-009/verify"},
							},
						}},
					},
				}); err != nil {
					t.Fatalf("seed repair exhaustion ticket: %v", err)
				}
				return runID
			},
			want: expected{err: common.ErrBlocked, state: common.RunStateBlockedOnDecision, blockingTicket: true},
		},
		{
			name: "provider retry exhaustion stays blocked for explicit runtime provider recovery",
			seed: func(ctx context.Context, t *testing.T, _ *RuntimeOrchestrator, repos blockerPolicyRepos) common.RunID {
				t.Helper()
				workspaceID := common.WorkspaceID("ws-policy-provider-exhausted")
				runID := common.RunID("run-policy-provider-exhausted")
				failedStepID := common.StepID(runID + "/block/provider")
				ticketID := common.TicketID(string(failedStepID) + "-provider-retry-exhausted-decision")
				seedBlockerPolicyWorkspaceAndRun(ctx, t, repos, workspaceID, runID, common.RunStateBlockedOnDecision, ticketID, "")
				if err := repos.steps.Create(ctx, &runmodel.Step{
					ID:    failedStepID,
					RunID: runID,
					Kind:  llmcommon.StepKindWorkspaceExecution,
					State: common.StepStateFailed,
					Output: map[string]any{
						"stage":                   "llm_provider_timeout",
						failureOutputKeyClass:     failureClassProviderTimeout,
						failureOutputKeySignature: "class=provider_timeout|stage=llm_provider_timeout",
					},
				}); err != nil {
					t.Fatalf("seed provider failed step: %v", err)
				}
				if err := repos.tickets.Create(ctx, &decisionmodel.Ticket{
					ID:          ticketID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					StepID:      failedStepID,
					State:       common.DecisionTicketStateOpen,
					Context: map[string]any{
						decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
						decisionContextKeyDependency:     decisionContextDependencyLLMProvider,
						decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
					},
				}); err != nil {
					t.Fatalf("seed provider exhaustion ticket: %v", err)
				}
				return runID
			},
			want: expected{err: common.ErrBlocked, state: common.RunStateBlockedOnDecision, blockingTicket: true},
		},
		{
			name: "pending approval stays blocked",
			seed: func(ctx context.Context, t *testing.T, _ *RuntimeOrchestrator, repos blockerPolicyRepos) common.RunID {
				t.Helper()
				workspaceID := common.WorkspaceID("ws-policy-approval")
				runID := common.RunID("run-policy-approval")
				approvalID := common.ApprovalID("approval-policy-pending")
				seedBlockerPolicyWorkspaceAndRun(ctx, t, repos, workspaceID, runID, common.RunStateBlockedOnApproval, "", approvalID)
				if err := repos.approvals.Create(ctx, &approvalmodel.Approval{
					ID:          approvalID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					State:       common.ApprovalStatePending,
				}); err != nil {
					t.Fatalf("seed approval: %v", err)
				}
				return runID
			},
			want: expected{err: common.ErrBlocked, state: common.RunStateBlockedOnApproval, blockingApproval: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repos := newBlockerPolicyRepos()
			runtime := &requestCapturingRuntimeAdapter{fn: func(request runtimemodel.StepExecutionRequest) map[string]any {
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied":   true,
						"summary":     "blocker policy resumed and completed",
						"goalOutcome": "blocker policy resumed and completed",
						"items": []map[string]any{{
							"item":         "blocker policy verification",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID) + "/validation/runtime-step"},
						}},
					},
				}
			}}
			orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
				runtime,
				repos.runs,
				repos.steps,
				repos.workspaces,
				repos.tickets,
				repos.resolutions,
				repos.approvals,
				RuntimeEvidenceRepositories{
					ToolCalls:          memory.NewToolCallRepository(),
					ValidationResults:  memory.NewValidationResultRepository(),
					RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
				},
				events.NewMemoryHub(),
			)
			runID := tt.seed(ctx, t, orch, repos)

			err := orch.ResumeRun(ctx, runID)
			if tt.want.err != nil {
				if !errors.Is(err, tt.want.err) {
					t.Fatalf("ResumeRun err = %v, want %v", err, tt.want.err)
				}
			} else if err != nil {
				t.Fatalf("ResumeRun err = %v", err)
			}
			runItem, err := repos.runs.Get(ctx, runID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if runItem.State != tt.want.state {
				t.Fatalf("run state = %q, want %q", runItem.State, tt.want.state)
			}
			if (runItem.BlockingTicketID != "") != tt.want.blockingTicket {
				t.Fatalf("blocking ticket = %q, want present=%v", runItem.BlockingTicketID, tt.want.blockingTicket)
			}
			if (runItem.BlockingApprovalID != "") != tt.want.blockingApproval {
				t.Fatalf("blocking approval = %q, want present=%v", runItem.BlockingApprovalID, tt.want.blockingApproval)
			}
			if len(runtime.requests) != tt.want.runtimeCalls {
				t.Fatalf("runtime calls = %d, want %d", len(runtime.requests), tt.want.runtimeCalls)
			}
		})
	}
}

type blockerPolicyRepos struct {
	workspaces  *memory.WorkspaceRepository
	runs        *memory.RunRepository
	steps       *memory.StepRepository
	tickets     *memory.DecisionTicketRepository
	resolutions *memory.DecisionResolutionRepository
	approvals   *memory.ApprovalRepository
}

func newBlockerPolicyRepos() blockerPolicyRepos {
	return blockerPolicyRepos{
		workspaces:  memory.NewWorkspaceRepository(),
		runs:        memory.NewRunRepository(),
		steps:       memory.NewStepRepository(),
		tickets:     memory.NewDecisionTicketRepository(),
		resolutions: memory.NewDecisionResolutionRepository(),
		approvals:   memory.NewApprovalRepository(),
	}
}

func seedBlockerPolicyWorkspaceAndRun(
	ctx context.Context,
	t *testing.T,
	repos blockerPolicyRepos,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	state common.RunState,
	ticketID common.TicketID,
	approvalID common.ApprovalID,
) {
	t.Helper()
	if err := repos.workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateWaitingDecision,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		DoDID:              common.DoDID("dod-" + string(runID)),
		State:              state,
		InitialStepKind:    llmcommon.StepKindWorkspaceExecution,
		BlockingTicketID:   ticketID,
		BlockingApprovalID: approvalID,
	}
	if err := repos.runs.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
}
