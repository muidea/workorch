package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOrchestrator_DispatchesLLMRepairAfterGraphNodeFailure(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var repairInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch {
		case request.StepID == "run-runtime-repair/step-1":
			return map[string]any{
				"status": "completed",
				"stage":  "llm_execution_graph_plan",
				"executionGraph": map[string]any{
					"nodes": []map[string]any{
						{
							"id":   "write-source",
							"kind": llmcommon.RuntimeBlockKindPatchApply,
							"input": map[string]any{
								"patchType": "file_write",
								"targetRef": "main.go",
								"content":   "package main\n",
							},
						},
						{
							"id":        "run-tests",
							"kind":      shellcommon.StepKindCommand,
							"dependsOn": []string{"write-source"},
							"input": map[string]any{
								"command": "go",
								"args":    []string{"test", "./..."},
							},
						},
					},
				},
			}, nil
		case strings.HasSuffix(string(request.StepID), "-write-source"):
			return map[string]any{
				"status": "completed",
				"patches": []map[string]any{
					{
						"patchType": "file_write",
						"targetRef": "main.go",
						"content":   "package main\n",
						"summary":   "write source",
					},
				},
			}, nil
		case strings.Contains(string(request.StepID), "/graph-") && strings.HasSuffix(string(request.StepID), "-run-tests"):
			return map[string]any{
				"status":    "failed",
				"stage":     "shell_command_completed",
				"exitCode":  1,
				"stderr":    "missing go.sum entry for module providing package github.com/spf13/viper",
				"graphHint": "dependency metadata missing",
			}, errors.New("exit status 1")
		case strings.Contains(string(request.StepID), "/repair-"):
			repairInput = request.Input
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "repair evidence handled and verification passed",
					"items": []map[string]any{
						{
							"item":         "tests pass",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected step %s", request.StepID)
		}
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memory.NewMemoryItemRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-runtime-repair")
	runID := common.RunID("run-runtime-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-runtime-repair"),
		State:           common.RunStatePending,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s failureReason=%q, want completed", runItem.State, runItem.FailureReason)
	}
	if repairInput == nil || repairInput[stepInputKeyRuntimeRepairFollowUp] != true || repairInput[stepInputKeyRuntimeRepairAttempt] != 1 {
		t.Fatalf("repair input = %+v, want runtime repair follow-up attempt 1", repairInput)
	}
	failedOutput, ok := repairInput["failedStepOutput"].(map[string]any)
	if !ok || !strings.Contains(fmt.Sprint(failedOutput["stderr"]), "missing go.sum") {
		t.Fatalf("failedStepOutput = %+v, want stderr evidence", repairInput["failedStepOutput"])
	}
	if repairInput["failureClass"] != failureClassShellCommand || !strings.Contains(fmt.Sprint(repairInput["failureSignature"]), "kind=shell/command") {
		t.Fatalf("repair failure classification = class:%#v signature:%#v, want shell command signature", repairInput["failureClass"], repairInput["failureSignature"])
	}
	failedStep, err := stepRepo.Get(ctx, graphScopedStepID(runID, 1, 2, "run-tests"))
	if err != nil {
		t.Fatalf("get failed step: %v", err)
	}
	if failedStep.State != common.StepStateFailed {
		t.Fatalf("failed step state = %s, want failed evidence retained", failedStep.State)
	}
}

func TestRuntimeOrchestrator_SoftwareDevCodeAgentRepairsTestFailureAndDeliversDiff(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	patchRepo := memory.NewPatchRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	deliverableDirectory := t.TempDir()
	targetPath := filepath.Join(deliverableDirectory, "calculator.go")
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		stepID := string(request.StepID)
		switch {
		case stepID == "run-software-dev-code-agent/step-1":
			return map[string]any{
				"status": "completed",
				"stage":  "llm_execution_graph_plan",
				"executionGraph": map[string]any{
					"nodes": []map[string]any{
						{
							"id":   "write-buggy-source",
							"kind": llmcommon.RuntimeBlockKindPatchApply,
							"input": map[string]any{
								"patchType": "file_write",
								"targetRef": "calculator.go",
								"content":   "package calculator\n\nfunc Add(a, b int) int { return a - b }\n",
							},
						},
						{
							"id":        "run-tests",
							"kind":      shellcommon.StepKindCommand,
							"dependsOn": []string{"write-buggy-source"},
							"input": map[string]any{
								"command": "go",
								"args":    []string{"test", "./..."},
							},
						},
					},
				},
			}, nil
		case strings.HasSuffix(stepID, "-write-buggy-source"):
			return map[string]any{
				"status": "completed",
				"patches": []map[string]any{{
					"patchType": "file_write",
					"targetRef": "calculator.go",
					"content":   "package calculator\n\nfunc Add(a, b int) int { return a - b }\n",
					"summary":   "write initial implementation",
				}},
			}, nil
		case strings.Contains(stepID, "/graph-") && strings.HasSuffix(stepID, "-run-tests-fixed"):
			return map[string]any{
				"status":   "completed",
				"stage":    "shell_command_completed",
				"command":  "go test ./...",
				"exitCode": 0,
				"stdout":   "ok calculator 0.003s",
			}, nil
		case strings.Contains(stepID, "/graph-") && strings.HasSuffix(stepID, "-run-tests"):
			return map[string]any{
				"status":   "failed",
				"stage":    "shell_command_completed",
				"command":  "go test ./...",
				"exitCode": 1,
				"stderr":   "calculator_test.go: Add(2,3)= -1, want 5",
			}, errors.New("exit status 1")
		case strings.Contains(stepID, "/repair-"):
			return map[string]any{
				"status": "completed",
				"stage":  "llm_execution_graph_repair",
				"executionGraph": map[string]any{
					"nodes": []map[string]any{
						{
							"id":   "fix-source",
							"kind": llmcommon.RuntimeBlockKindPatchApply,
							"input": map[string]any{
								"patchType": "unified_diff",
								"targetRef": "calculator.go",
								"diff":      "--- a/calculator.go\n+++ b/calculator.go\n@@ -1,3 +1,3 @@\n package calculator\n \n-func Add(a, b int) int { return a - b }\n+func Add(a, b int) int { return a + b }\n",
							},
						},
						{
							"id":        "run-tests-fixed",
							"kind":      shellcommon.StepKindCommand,
							"dependsOn": []string{"fix-source"},
							"input": map[string]any{
								"command": "go",
								"args":    []string{"test", "./..."},
							},
						},
					},
				},
			}, nil
		case strings.Contains(stepID, "/graph-") && strings.HasSuffix(stepID, "-fix-source"):
			return map[string]any{
				"status": "completed",
				"patches": []map[string]any{{
					"patchType": "unified_diff",
					"targetRef": "calculator.go",
					"diff":      "--- a/calculator.go\n+++ b/calculator.go\n@@ -1,3 +1,3 @@\n package calculator\n \n-func Add(a, b int) int { return a - b }\n+func Add(a, b int) int { return a + b }\n",
					"summary":   "fix Add implementation",
				}},
			}, nil
		case strings.Contains(stepID, "/follow-up-graph-"):
			return map[string]any{
				"status": "completed",
				"stage":  "llm_dod_assessment",
				"dodAssessment": map[string]any{
					"satisfied":             true,
					"summary":               "Software development DoD satisfied after automatic repair.",
					"goalOutcome":           "Add now returns the sum of two integers.",
					"implementationSummary": "Fixed calculator.go with a unified diff.",
					"verificationSummary":   "go test ./... passed after the repair.",
					"items": []map[string]any{
						{
							"item":         "implementation fixed",
							"state":        "satisfied",
							"evidenceRefs": []string{"run-software-dev-code-agent/graph-002/001-fix-source"},
						},
						{
							"item":         "tests pass",
							"state":        "satisfied",
							"evidenceRefs": []string{"run-software-dev-code-agent/graph-002/002-run-tests-fixed"},
						},
					},
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected software-dev step %s", request.StepID)
		}
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            resultRepo,
			Artifacts:          artifactRepo,
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            patchRepo,
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memoryRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-software-dev-code-agent")
	runID := common.RunID("run-software-dev-code-agent")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "software-dev", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-software-dev-code-agent"),
		State:           common.RunStatePending,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s failureReason=%q, want completed", runItem.State, runItem.FailureReason)
	}
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !strings.Contains(string(content), "return a + b") {
		t.Fatalf("target content = %q, want repaired implementation", string(content))
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	var repairPatch *runmodel.Patch
	for _, patch := range patches {
		if patch.PatchType == "unified_diff" {
			repairPatch = patch
			break
		}
	}
	if len(patches) != 2 || repairPatch == nil || !strings.Contains(repairPatch.Diff, "+func Add(a, b int) int { return a + b }") {
		t.Fatalf("patches = %+v, want initial write and repair unified diff", patches)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 || results[0].State != common.ResultStateAccepted {
		t.Fatalf("results = %+v, want accepted result", results)
	}
	artifacts, err := artifactRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	var diffArtifact *resultmodel.Artifact
	for _, artifact := range artifacts {
		if artifact.ID == common.ArtifactID("run-software-dev-code-agent/artifact/workspace-change-diff") {
			diffArtifact = artifact
			break
		}
	}
	if diffArtifact == nil || !strings.Contains(diffArtifact.Summary, "```diff") || !strings.Contains(diffArtifact.Summary, "+func Add(a, b int) int { return a + b }") {
		t.Fatalf("diff artifact = %+v, want full repair diff", diffArtifact)
	}
	memories, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if !hasRunSummaryContent(memories, runID, "State-change targets:", "go test ./...") {
		t.Fatalf("memories = %+v, want run summary memory with patch and verification context", memories)
	}
}

func TestRuntimeOrchestrator_RetriesHTTPStatusProviderErrorBeforeBlocking(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	attempts := 0
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if strings.Contains(string(request.StepID), "/repair-") {
			t.Fatalf("unexpected runtime repair step for provider failure: %s", request.StepID)
		}
		attempts++
		return map[string]any{
			"status": "failed",
			"stage":  "llm_provider_unavailable",
			"error":  "invalid argument: llm provider http status 404",
		}, fmt.Errorf("%w: llm provider http status 404", common.ErrInvalidArgument)
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-llm-provider-error")
	runID := common.RunID("run-llm-provider-error")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-llm-provider-error"),
		State:           common.RunStatePending,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on provider decision", runItem)
	}
	if attempts != maxProviderRetryAttempts+1 {
		t.Fatalf("attempts = %d, want initial attempt plus retry budget", attempts)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(steps) != maxProviderRetryAttempts+1 {
		t.Fatalf("steps = %+v, want initial failed provider step plus retry budget", steps)
	}
	for _, step := range steps {
		if step.Output["failureClass"] != failureClassProviderUnavailable {
			t.Fatalf("step %s failureClass = %#v, want provider unavailable", step.ID, step.Output["failureClass"])
		}
	}
	ticket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.Context[decisionContextKeyResultPolicy] != decisionContextResultPolicyNone ||
		ticket.Context[decisionContextKeyScope] != decisionContextScopeWorkorchRuntime {
		t.Fatalf("ticket context = %+v, want provider config decision without business result", ticket.Context)
	}
	if ticket.Context["attempts"] != maxProviderRetryAttempts || ticket.Context["maxAttempts"] != maxProviderRetryAttempts {
		t.Fatalf("ticket attempts = %+v/%+v, want actual attempts with max budget", ticket.Context["attempts"], ticket.Context["maxAttempts"])
	}
}

func TestRuntimeOrchestrator_RetriesLLMProviderStreamErrorBeforeConfigBlock(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-stream-error-retry")
	failedStepID := common.StepID(runID + "/repair-002-of-graph-001-004-build")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		runtime: &stubRuntimeAdapter{},
		steps:   stepRepo,
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxProviderRetryAttempts: 10}),
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-provider-stream-error-retry"),
		DoDID:           common.DoDID("dod-provider-stream-error-retry"),
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"stateChangingOutputRequired":     true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "llm_provider_stream_error",
			"error":                   "invalid argument: llm provider streamed response content is empty",
			failureOutputKeyClass:     failureClassProviderUnavailable,
			failureOutputKeySignature: "class=provider_unavailable|stage=llm_provider_stream_error",
			"streamDiagnostics": map[string]any{
				"doneReceived":       true,
				"receivedBytes":      0,
				"receivedDeltaCount": 0,
				"streamIncomplete":   false,
				"receivedEventCount": 1,
			},
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want provider retry spec", specs)
	}
	if specs[0].ID != providerRetryStepID(runID, failedStepID, 1) ||
		specs[0].Input[stepInputKeyProviderRetryFollowUp] != true ||
		specs[0].Input[stepInputKeyProviderRetryAttempt] != 1 {
		t.Fatalf("spec = %+v, want first provider retry for stream error", specs[0])
	}
	if specs[0].Input["stateChangingOutputRequired"] != true ||
		specs[0].Input["repairObjective"] != repairObjectiveStateChangingOutput {
		t.Fatalf("retry input = %+v, want state-changing repair contract preserved", specs[0].Input)
	}
}

func TestRuntimeOrchestrator_RetriesTransientProviderUnavailableBeforeConfigBlock(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-unavailable-retry")
	failedStepID := common.StepID(runID + "/step-1")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		runtime: &stubRuntimeAdapter{},
		steps:   stepRepo,
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxProviderRetryAttempts: 10}),
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-provider-unavailable-retry"),
		DoDID:           common.DoDID("dod-provider-unavailable-retry"),
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "llm_provider_unavailable",
			"error":                   "local error: tls: bad record MAC",
			failureOutputKeyClass:     failureClassProviderUnavailable,
			failureOutputKeySignature: "class=provider_unavailable|stage=llm_provider_unavailable|error=local error: tls: bad record mac",
			"resolutionHint":          "check LLM provider baseURL, model, API key, protocol compatibility, and network availability before rerunning the workspace",
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want provider retry spec", specs)
	}
	if specs[0].ID != providerRetryStepID(runID, failedStepID, 1) ||
		specs[0].Input[stepInputKeyProviderRetryFollowUp] != true ||
		specs[0].Input[stepInputKeyProviderRetryAttempt] != 1 {
		t.Fatalf("spec = %+v, want first provider retry for transient provider unavailable", specs[0])
	}
}

func TestRuntimeOrchestrator_RetriesTransientProviderHTTPStatusBeforeFailingRun(t *testing.T) {
	for _, statusCode := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		t.Run(fmt.Sprintf("http-%d", statusCode), func(t *testing.T) {
			ctx := context.Background()
			runID := common.RunID(fmt.Sprintf("run-provider-http-%d-retry", statusCode))
			failedStepID := common.StepID(string(runID) + "/step-1")
			stepRepo := memory.NewStepRepository()
			orch := &RuntimeOrchestrator{
				runtime: &stubRuntimeAdapter{},
				steps:   stepRepo,
				policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxProviderRetryAttempts: 3}),
			}
			run := &runmodel.Run{
				ID:              runID,
				WorkspaceID:     common.WorkspaceID(fmt.Sprintf("ws-provider-http-%d-retry", statusCode)),
				DoDID:           common.DoDID(fmt.Sprintf("dod-provider-http-%d-retry", statusCode)),
				State:           common.RunStateActive,
				InitialStepKind: llmcommon.StepKindWorkspaceExecution,
			}
			failedStep := &runmodel.Step{
				ID:    failedStepID,
				RunID: runID,
				Kind:  llmcommon.StepKindWorkspaceExecution,
				State: common.StepStateFailed,
				Output: map[string]any{
					"status":                  "failed",
					"stage":                   "llm_provider_unavailable",
					"error":                   fmt.Sprintf("invalid argument: llm provider http status %d: upstream temporarily unavailable", statusCode),
					failureOutputKeyClass:     failureClassProviderUnavailable,
					failureOutputKeySignature: fmt.Sprintf("class=provider_unavailable|stage=llm_provider_unavailable|error=http status %d", statusCode),
				},
			}
			if err := stepRepo.Create(ctx, failedStep); err != nil {
				t.Fatalf("seed failed step: %v", err)
			}

			specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
			if err != nil {
				t.Fatalf("nextDispatchSpecsFromStep: %v", err)
			}
			if len(specs) != 1 ||
				specs[0].ID != providerRetryStepID(runID, failedStepID, 1) ||
				specs[0].Input[stepInputKeyProviderRetryFollowUp] != true {
				t.Fatalf("specs = %+v, want provider retry for transient HTTP %d", specs, statusCode)
			}
		})
	}
}

func TestRuntimeOrchestrator_BlocksProviderHTTPStatusAfterRetryBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-http-404-retry-exhausted")
	rootStepID := common.StepID(runID + "/step-1")
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	orch := &RuntimeOrchestrator{
		runtime: &stubRuntimeAdapter{},
		steps:   stepRepo,
		tickets: ticketRepo,
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxProviderRetryAttempts: 2}),
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-provider-http-404-retry-exhausted"),
		DoDID:           common.DoDID("dod-provider-http-404-retry-exhausted"),
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedOutput := map[string]any{
		"status":                  "failed",
		"stage":                   "llm_provider_unavailable",
		"error":                   "invalid argument: llm provider http status 404: upstream temporarily unavailable",
		failureOutputKeyClass:     failureClassProviderUnavailable,
		failureOutputKeySignature: "class=provider_unavailable|stage=llm_provider_unavailable|error=http status 404",
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     rootStepID,
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateFailed,
		Output: failedOutput,
	}); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    providerRetryStepID(runID, rootStepID, attempt),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyProviderRetryFollowUp: true,
				stepInputKeyProviderRetryAttempt:  attempt,
				stepInputKeyProviderRetryRootStep: string(rootStepID),
			},
			Output: failedOutput,
		}); err != nil {
			t.Fatalf("seed retry %d: %v", attempt, err)
		}
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, &runmodel.Step{
		ID:     rootStepID,
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateFailed,
		Output: failedOutput,
	})
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("nextDispatchSpecsFromStep err = %v, want blocked after transient HTTP retry budget", err)
	}
	if len(specs) != 0 {
		t.Fatalf("specs = %+v, want no retry beyond budget", specs)
	}
}

func TestRuntimeOrchestrator_RetriesLLMProviderTimeoutBeforeFailingRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	attempts := 0
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		attempts++
		if attempts <= maxProviderRetryAttempts {
			return map[string]any{
				"status": "failed",
				"stage":  "llm_provider_timeout",
				"error":  "invalid argument: openai llm provider error: context deadline exceeded",
			}, fmt.Errorf("%w: openai llm provider error: context deadline exceeded", common.ErrInvalidArgument)
		}
		if request.Input[stepInputKeyProviderRetryFollowUp] != true || request.Input[stepInputKeyProviderRetryAttempt] != maxProviderRetryAttempts {
			t.Fatalf("provider retry input = %+v, want final retry metadata", request.Input)
		}
		return map[string]any{
			"status": "completed",
			"dodAssessment": map[string]any{
				"satisfied":   true,
				"summary":     "provider recovered and work completed",
				"goalOutcome": "provider recovered and work completed",
				"items": []map[string]any{{
					"item":         "work completes",
					"state":        "satisfied",
					"evidenceRefs": []string{string(request.StepID)},
				}},
			},
		}, nil
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-llm-provider-timeout-retry")
	runID := common.RunID("run-llm-provider-timeout-retry")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-llm-provider-timeout-retry"),
		State:           common.RunStatePending,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want completed after provider retry", runItem)
	}
	if attempts != maxProviderRetryAttempts+1 {
		t.Fatalf("attempts = %d, want initial + retries", attempts)
	}
}

func TestRuntimeOrchestrator_BlocksOnDecisionAfterProviderTimeoutRetriesExhausted(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		return map[string]any{
			"status": "failed",
			"stage":  "llm_provider_timeout",
			"error":  "invalid argument: openai llm provider error: context deadline exceeded",
		}, fmt.Errorf("%w: openai llm provider error: context deadline exceeded", common.ErrInvalidArgument)
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            resultRepo,
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-llm-provider-timeout-block")
	runID := common.RunID("run-llm-provider-timeout-block")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-llm-provider-timeout-block"),
		State:           common.RunStatePending,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"dialogLanguage": "zh-CN",
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on provider recovery decision", runItem)
	}
	ticket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateOpen || ticket.Context["attempts"] != maxProviderRetryAttempts {
		t.Fatalf("ticket = %+v, want provider retry exhaustion context", ticket)
	}
	if ticket.Context[decisionContextKeyScope] != decisionContextScopeWorkorchRuntime ||
		ticket.Context[decisionContextKeyDependency] != decisionContextDependencyLLMProvider ||
		ticket.Context[decisionContextKeyRecoveryPolicy] != decisionContextRecoveryPolicyConfigOnly {
		t.Fatalf("ticket context = %+v, want WorkOrch runtime provider recovery scope", ticket.Context)
	}
	if !strings.Contains(ticket.Question, "WorkOrch runtime LLM provider") || !strings.Contains(ticket.Question, "WorkOrch 系统配置") || !strings.Contains(ticket.Question, "API Key") {
		t.Fatalf("ticket question = %q, want provider configuration recovery prompt", ticket.Question)
	}
	if _, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("provider retry exhaustion result err = %v, want no business rejected result", err)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairScopeUsesCurrentFailureBeforeInheritedLineage(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-budget-current-signature")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/old-repair-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateSucceeded,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
				stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|command=verify|failureTargets=old-target",
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed old repair %d: %v", attempt, err)
		}
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/block/verify"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRepairOfFailureClass: failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:   "class=shell_command_failed|command=verify|failureTargets=old-target",
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|command=verify|failureTargets=new-target",
		},
	}

	if got := orch.repairManager().runtimeRepairAttemptCountForStep(ctx, runID, failedStep); got != 0 {
		t.Fatalf("attempt count = %d, want current output failure signature to ignore inherited lineage attempts", got)
	}
	if got := orch.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); got != 0 {
		t.Fatalf("effective attempt count = %d, want current output failure target to ignore inherited lineage attempts", got)
	}
	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: common.WorkspaceID("ws-current-signature")}, failedStep, 1, nil)
	if input[stepInputKeyRepairOfFailureSig] != "class=shell_command_failed|command=verify|failureTargets=new-target" {
		t.Fatalf("repair input signature = %#v, want current output failure signature", input[stepInputKeyRepairOfFailureSig])
	}
	if input[stepInputKeyRecoveryAction] != string(runtimeRecoveryActionRuntimeRepair) ||
		input[stepInputKeyRecoveryRootStepID] != string(failedStep.ID) ||
		input[stepInputKeyRecoveryLineageType] != "runtime_repair" {
		t.Fatalf("repair lineage = %+v, want explicit runtime repair lineage", input)
	}
	scope := mapFromValue(input[stepInputKeyRecoveryBudgetScope])
	if scope["lineageType"] != "runtime_repair" ||
		scope["rootStepId"] != string(failedStep.ID) ||
		scope["failureClass"] != failureClassShellCommand ||
		scope["failureSignature"] != "class=shell_command_failed|command=verify|failureTargets=new-target" {
		t.Fatalf("repair budget scope = %+v, want scoped repair lineage", scope)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairScopeUsesCurrentFailureWithoutTargetScope(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-budget-current-signature-no-target")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	oldSignature := "class=shell_command_failed|kind=shell/command|node=init|command=tool|exitCode=1|error=exit status 1"
	currentSignature := "class=shell_command_failed|kind=shell/command|node=verify|command=tool|exitCode=-1|error=context deadline exceeded"
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-init"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:    oldSignature,
		},
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("seed old repair: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-002/003-verify"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:         runtimeStageExecution,
			stepInputKeyGraphNodeID:          "verify",
			stepInputKeyGraphRootStepID:      string(runID) + "/repair-001-of-init",
			stepInputKeyRepairOfStepID:       string(runID) + "/graph-001/002-init",
			stepInputKeyRepairOfRootStepID:   string(runID) + "/step-1",
			stepInputKeyRepairOfFailureClass: failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:   oldSignature,
		},
		Output: map[string]any{
			"status":                  "timeout",
			"error":                   "context deadline exceeded",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: currentSignature,
		},
	}

	if got := orch.repairManager().runtimeRepairAttemptCountForStep(ctx, runID, failedStep); got != 0 {
		t.Fatalf("attempt count = %d, want current output failure signature to ignore inherited repair signature", got)
	}
	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: common.WorkspaceID("ws-current-signature-no-target"),
		DoDID:       common.DoDID("dod-current-signature-no-target"),
	}, failedStep, 1, errors.New("context deadline exceeded"))
	if input[stepInputKeyRepairOfFailureSig] != currentSignature || input[failureOutputKeySignature] != currentSignature {
		t.Fatalf("repair input signature = repair:%#v failure:%#v, want current output signature", input[stepInputKeyRepairOfFailureSig], input[failureOutputKeySignature])
	}
	if input["repairObjective"] != repairObjectiveTransientExecutionRetry || input["stateChangingOutputRequired"] != nil {
		t.Fatalf("repair input = %+v, want transient retry without state-changing requirement", input)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairEffectiveScopeCountsRepairWrapperRootClass(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-budget-wrapper-root-class")
	rootStepID := common.StepID(runID + "/step-1")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps:   stepRepo,
		tickets: memory.NewDecisionTicketRepository(),
	}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		stepID := common.StepID(fmt.Sprintf("%s/repair-001-of-planning-001-%d", runID, attempt))
		if attempt == 1 {
			stepID = common.StepID(string(runID) + "/repair-001-of-planning-001")
		}
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    stepID,
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  1,
				stepInputKeyRepairOfStepID:        string(rootStepID),
				stepInputKeyRepairOfRootStepID:    string(rootStepID),
				stepInputKeyRepairOfFailureClass:  failureClassDoDCoverageMissing,
				stepInputKeyRepairOfFailureSig:    fmt.Sprintf("class=dod_coverage_missing|stage=llm_dod_assessment|error=missing coverage attempt %d", attempt),
			},
			Output: map[string]any{
				"stage":                   "llm_dod_assessment",
				"error":                   fmt.Sprintf("dodAssessment does not cover all current DoD items attempt %d", attempt),
				failureOutputKeyClass:     failureClassDoDCoverageMissing,
				failureOutputKeySignature: fmt.Sprintf("class=dod_coverage_missing|stage=llm_dod_assessment|error=missing coverage output %d", attempt),
			},
		}); err != nil {
			t.Fatalf("seed repair wrapper %d: %v", attempt, err)
		}
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-planning-001-91"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfStepID:        string(rootStepID),
			stepInputKeyRepairOfRootStepID:    string(rootStepID),
			stepInputKeyRepairOfFailureClass:  failureClassDoDCoverageMissing,
			stepInputKeyRepairOfFailureSig:    "class=dod_coverage_missing|stage=llm_dod_assessment|error=missing coverage current input",
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			"error":                   "dodAssessment does not cover all current DoD items",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment|error=missing coverage current output",
		},
	}

	if got := orch.repairManager().runtimeRepairAttemptCountForStep(ctx, runID, failedStep); got != 0 {
		t.Fatalf("exact attempt count = %d, want signature-specific count to stay independent", got)
	}
	if got := orch.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); got != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("effective attempt count = %d, want root/class wrapper attempts counted", got)
	}
	if !shouldEscalateRuntimeRepairExhaustion(ctx, orch.repairManager(), orch.policy, orch.tickets != nil, &runmodel.Run{ID: runID, DoDID: common.DoDID("dod-wrapper-root-class")}, failedStep) {
		t.Fatalf("repeated repair wrapper failures should exhaust runtime repair budget")
	}
}

func TestRuntimeOrchestrator_RuntimeRepairScopeUsesCurrentRepairContractFailure(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-budget-current-repair-contract")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	oldSignature := "class=shell_command_failed|kind=shell/command|node=verify|failureTargets=main.go|exitCode=1"
	currentSignature := "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_contract|error=state-changing repair returned unrelated patch target"
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-002-of-graph-004-verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:    oldSignature,
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "llm_execution_contract",
			"error":                   "state-changing repair returned unrelated patch target",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: currentSignature,
		},
	}

	class, signature := runtimeRepairScopeClassAndSignature(failedStep)
	if class != failureClassRuntime || signature != currentSignature {
		t.Fatalf("runtimeRepairScopeClassAndSignature = class:%q signature:%q, want current repair contract failure", class, signature)
	}
	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: common.WorkspaceID("ws-current-repair-contract"),
		DoDID:       common.DoDID("dod-current-repair-contract"),
	}, failedStep, 1, errors.New("state-changing repair returned unrelated patch target"))
	if input[failureOutputKeyClass] != failureClassRuntime ||
		input[stepInputKeyRepairOfFailureClass] != failureClassRuntime ||
		input[failureOutputKeySignature] != currentSignature ||
		input[stepInputKeyRepairOfFailureSig] != currentSignature {
		t.Fatalf("repair input failure scope = class:%#v repairClass:%#v signature:%#v repairSignature:%#v, want current failure",
			input[failureOutputKeyClass],
			input[stepInputKeyRepairOfFailureClass],
			input[failureOutputKeySignature],
			input[stepInputKeyRepairOfFailureSig],
		)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairEffectiveScopeCountsFailureTargetAcrossGraphNodes(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-target-scope")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps:   stepRepo,
		tickets: memory.NewDecisionTicketRepository(),
	}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		node := fmt.Sprintf("target-scope-repair-%02d", attempt)
		signature := fmt.Sprintf("class=shell_command_failed|kind=shell/command|node=%s|failureTargets=stream_e2e_test.go|exitCode=1", node)
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateSucceeded,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed target repair %d: %v", attempt, err)
		}
	}

	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-013/002-run-stream-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=run-stream-test|failureTargets=stream_e2e_test.go|exitCode=1",
		},
	}
	if got := orch.repairManager().runtimeRepairAttemptCountForStep(ctx, runID, failedStep); got != 0 {
		t.Fatalf("exact attempt count = %d, want node-specific signature to remain independent", got)
	}
	if got := orch.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); got != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("effective attempt count = %d, want target scope exhausted", got)
	}
	if !shouldEscalateRuntimeRepairExhaustion(ctx, orch.repairManager(), orch.policy, orch.tickets != nil, &runmodel.Run{ID: runID, DoDID: common.DoDID("dod-target-scope")}, failedStep) {
		t.Fatalf("target-scoped repeated failure should escalate repair exhaustion")
	}

	otherTarget := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-014/002-run-other-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=run-other-test|failureTargets=other_target_test.go|exitCode=1",
		},
	}
	if got := orch.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, runID, otherTarget); got != 0 {
		t.Fatalf("other target effective attempt count = %d, want independent target scope", got)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairEffectiveScopeCountsTargetContractCorrectionsAcrossMarkers(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-target-contract-correction")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps:   stepRepo,
		tickets: memory.NewDecisionTicketRepository(),
	}
	markers := []string{
		contractViolationPatchDispatchInvalid,
		contractViolationGraphDependsOnLLMBeforePatch,
		contractViolationCoverageTargetsRequired,
		contractViolationPatchContentRequired,
	}
	for idx := 0; idx < defaultMaxRuntimeRepairAttempts; idx++ {
		attempt := idx + 1
		marker := markers[idx%len(markers)]
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-%03d", runID, attempt)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfRootStepID:    string(runID) + "/graph-001/verify",
				"repairActionPackage": map[string]any{
					"repairTargetSet": []string{"target.txt"},
				},
			},
			Output: map[string]any{
				"stage":                        "llm_execution_graph_plan",
				"error":                        "runtime repair contract correction failed",
				outputKeyContractViolationType: marker,
				failureOutputKeyClass:          failureClassRuntime,
				failureOutputKeySignature:      fmt.Sprintf("class=runtime_failed|kind=llm/workspace_execution|node=repair-%03d|targetRef=target.txt", attempt),
			},
		}); err != nil {
			t.Fatalf("seed contract correction %d: %v", attempt, err)
		}
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-005"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfRootStepID:    string(runID) + "/graph-001/verify",
			"repairActionPackage": map[string]any{
				"repairTargetSet": []string{"target.txt"},
			},
		},
		Output: map[string]any{
			"stage":                        "llm_execution_graph_plan",
			"error":                        "runtime repair patch/apply unified_diff for target \"target.txt\" is non-substantive",
			outputKeyContractViolationType: contractViolationPatchContentRequired,
			failureOutputKeyClass:          failureClassRuntime,
			failureOutputKeySignature:      "class=runtime_failed|kind=llm/workspace_execution|node=repair-current|targetRef=target.txt",
		},
	}

	if got := orch.repairManager().runtimeRepairAttemptCountForStep(ctx, runID, failedStep); got != 0 {
		t.Fatalf("exact attempt count = %d, want marker-specific signatures to remain independent", got)
	}
	if got := orch.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); got != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("effective attempt count = %d, want target contract corrections counted together", got)
	}
	if !shouldEscalateRuntimeRepairExhaustion(ctx, orch.repairManager(), orch.policy, orch.tickets != nil, &runmodel.Run{ID: runID, DoDID: common.DoDID("dod-target-contract-correction")}, failedStep) {
		t.Fatalf("target-scoped contract correction loop should escalate repair exhaustion")
	}
}

func TestRuntimeOrchestrator_RuntimeRepairTargetScopeUsesCurrentGraphWindow(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-target-window")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps:   stepRepo,
		tickets: memory.NewDecisionTicketRepository(),
	}
	signature := "class=shell_command_failed|kind=shell/command|node=build|failureTargets=main.go|exitCode=1|error=exit status 1"
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-old-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateSucceeded,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfRootStepID:    string(runID) + "/old-graph-root",
				stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed old target repair %d: %v", attempt, err)
		}
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-009/006-build"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: string(runID) + "/new-graph-root",
			stepInputKeyGraphNodeID:     "build",
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: signature,
			"error":                   "exit status 1",
		},
	}

	if got := orch.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); got != 0 {
		t.Fatalf("effective attempt count = %d, want old graph-root attempts excluded", got)
	}
	if shouldEscalateRuntimeRepairExhaustion(ctx, orch.repairManager(), orch.policy, orch.tickets != nil, &runmodel.Run{ID: runID, DoDID: common.DoDID("dod-target-window")}, failedStep) {
		t.Fatalf("new graph-root target failure should receive a fresh repair window")
	}

	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/repair-new-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfRootStepID:    string(runID) + "/new-graph-root",
			stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:    signature,
		},
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("seed current target repair: %v", err)
	}
	if got := orch.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); got != 1 {
		t.Fatalf("effective attempt count = %d, want current graph-root target attempt counted", got)
	}
}

func TestRuntimeOrchestrator_PatchApplyTargetBudgetCrossesGraphWindows(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-patch-target-budget")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps:   stepRepo,
		tickets: memory.NewDecisionTicketRepository(),
	}
	signature := "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|targetRef=target.txt|error=context mismatch"
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-old-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateSucceeded,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyGraphRootStepID:       fmt.Sprintf("%s/graph-%03d", runID, attempt),
				stepInputKeyRepairOfFailureClass:  failureClassPatchApply,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed patch target repair %d: %v", attempt, err)
		}
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-latest/patch"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: string(runID) + "/graph-latest",
		},
		Output: map[string]any{
			"stage":                   "patch_apply",
			"error":                   "invalid argument: unified diff hunk context mismatch",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: signature,
		},
	}

	if got := orch.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); got != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("effective attempt count = %d, want patch target attempts counted across graph roots", got)
	}
	if !shouldEscalateRuntimeRepairExhaustion(ctx, orch.repairManager(), orch.policy, orch.tickets != nil, &runmodel.Run{ID: runID, DoDID: common.DoDID("dod-patch-target-budget")}, failedStep) {
		t.Fatalf("repeated patch target failure should escalate repair exhaustion")
	}
}

func TestRuntimeOrchestrator_ResumeClearsStaleRuntimeRepairExhaustionBlocker(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "completed"}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-stale-repair-blocker")
	runID := common.RunID("run-stale-repair-blocker")
	ticketID := common.TicketID("ticket-stale-repair-blocker")
	signature := "class=runtime|target=stale"
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID(runID + "/block/original"),
		State:       common.DecisionTicketStateOpen,
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
			"failedStepId":                   string(runID + "/block/original"),
			"failureClass":                   failureClassRuntime,
			"failureSignature":               signature,
		},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/block/repair-success"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			"previousStepId":                 string(runID + "/block/original"),
			stepInputKeyGraphNodeID:          "repair-success",
			stepInputKeyRepairOfStepID:       string(runID + "/block/original"),
			stepInputKeyRepairOfFailureClass: failureClassRuntime,
			stepInputKeyRepairOfFailureSig:   signature,
		},
		Output: map[string]any{
			"status":               "completed",
			stepOutputKeyNextStep:  string(runID + "/step-after-stale-repair"),
			stepOutputKeyNextKind:  llmcommon.StepKindWorkspaceExecution,
			stepOutputKeyNextInput: map[string]any{"fromStaleRepairEvidence": true},
		},
	}); err != nil {
		t.Fatalf("seed repair success evidence: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume stale repair blocker: %v", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.BlockingTicketID != "" || runItem.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want stale blocker cleared and completed", runItem)
	}
}

func TestRuntimeOrchestrator_ResumeClearsRepairExhaustionWhenTargetWindowHasBudget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	runtime := &requestCapturingRuntimeAdapter{output: map[string]any{
		"status": "completed",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "repair completed in current graph window",
			"items": []map[string]any{{
				"item":         "current graph repair",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-repair-window-resume/repair-001-of-graph-009-006-build"},
			}},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Results: memory.NewResultRepository()},
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-repair-window-resume")
	runID := common.RunID("run-repair-window-resume")
	failedStepID := common.StepID(runID + "/graph-009/006-build")
	ticketID := common.TicketID(string(failedStepID) + "-repair-exhausted-decision")
	signature := "class=shell_command_failed|kind=shell/command|node=build|failureTargets=main.go|exitCode=1|error=exit status 1"
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            common.DoDID("dod-repair-window-resume"),
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-old-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateSucceeded,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfRootStepID:    string(runID) + "/old-graph-root",
				stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed old repair %d: %v", attempt, err)
		}
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: string(runID) + "/new-graph-root",
			stepInputKeyGraphNodeID:     "build",
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "exit status 1",
			"stderr":                  "./main.go:177:10: declared and not used: choice",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: signature,
		},
	}); err != nil {
		t.Fatalf("seed failed build step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		StepID:       failedStepID,
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Reason:       "runtime repair exhausted",
		Question:     "provide repair strategy",
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
			"failedStepId":                   string(failedStepID),
			"failureClass":                   failureClassShellCommand,
			"failureSignature":               signature,
			"failureTargetScope":             "failureTargets=main.go",
			"attempts":                       defaultMaxRuntimeRepairAttempts,
		},
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume repair exhaustion with fresh target window: %v", err)
	}
	ticket, err := ticketRepo.Get(ctx, ticketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateSuperseded {
		t.Fatalf("ticket state = %s, want dismissed stale repair exhaustion blocker", ticket.State)
	}
	if len(runtime.requests) != 1 || !boolValue(runtime.requests[0].Input[stepInputKeyRuntimeRepairFollowUp]) {
		t.Fatalf("runtime requests = %+v, want one runtime repair dispatch", runtime.requests)
	}
	if runtime.requests[0].Input["targetRepairAttempts"] != 0 {
		t.Fatalf("targetRepairAttempts = %v, want old graph attempts excluded", runtime.requests[0].Input["targetRepairAttempts"])
	}
}

func TestRuntimeRepairMCPFailureTargetUsesCompositeRunBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-mcp-target-budget")
	signature := "class=mcp_runtime_failed|kind=mcp/tool_call|stage=mcp_tool_call_server_unavailable|serverId=external-capability|tool=lookup.current|error=not found"
	if got := runtimeFailureTargetScope(signature); got != "serverId=external-capability|tool=lookup.current" {
		t.Fatalf("runtimeFailureTargetScope = %q, want composite server/tool target", got)
	}
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo, tickets: memory.NewDecisionTicketRepository()}
	for idx, root := range []string{"graph-001", "graph-002"} {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-%03d", runID, idx+1)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateSucceeded,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyGraphRootStepID:       string(runID) + "/" + root,
				stepInputKeyRepairOfFailureClass:  failureClassMCPRuntime,
				stepInputKeyRepairOfFailureSig:    signature,
			},
		}); err != nil {
			t.Fatalf("seed repair step: %v", err)
		}
	}
	priorFailed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-002/001-capability-call"),
		RunID: runID,
		Kind:  "mcp/tool_call",
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassMCPRuntime,
			failureOutputKeySignature: signature,
		},
	}
	if err := stepRepo.Create(ctx, priorFailed); err != nil {
		t.Fatalf("seed prior failed step: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-003/001-capability-call"),
		RunID: runID,
		Kind:  "mcp/tool_call",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: string(runID) + "/graph-003",
		},
		Output: map[string]any{
			"stage":                   "mcp_tool_call_server_unavailable",
			"serverId":                "external-capability",
			"tool":                    "lookup.current",
			"error":                   "not found",
			failureOutputKeyClass:     failureClassMCPRuntime,
			failureOutputKeySignature: signature,
		},
	}

	if attempts := orch.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); attempts != 2 {
		t.Fatalf("effective attempts = %d, want same external capability target counted across graph roots", attempts)
	}
	if !shouldBlockOnDuplicateRuntimeFailureForTest(ctx, orch, &runmodel.Run{ID: runID, DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, failedStep) {
		t.Fatalf("duplicate external capability target should block before repeating the same call again")
	}
}

func TestRuntimeOrchestrator_ResumeClearsDuplicateFailureBlockerBelowScopedBudget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	runtime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "completed"}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-stale-duplicate-blocker")
	runID := common.RunID("run-stale-duplicate-blocker")
	failedStepID := common.StepID(runID + "/block/verify")
	ticketID := common.TicketID(string(failedStepID) + "-duplicate-failure-decision")
	signature := "class=shell_command_failed|kind=shell/command|node=verify|failureTargets=config/settings.yaml|exitCode=1"
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            common.DoDID("dod-stale-duplicate-blocker"),
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/repair-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateSucceeded,
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
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: signature,
			"error":                   "exit status 1",
		},
	}); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		State:       common.DecisionTicketStateOpen,
		Context: map[string]any{
			"failedStepId":     string(failedStepID),
			"failureClass":     failureClassShellCommand,
			"failureSignature": signature,
		},
	}); err != nil {
		t.Fatalf("seed duplicate ticket: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume duplicate blocker below scoped budget: %v", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.BlockingTicketID != "" {
		t.Fatalf("run = %+v, want duplicate blocker cleared", runItem)
	}
	if len(runtime.requests) == 0 || !boolValue(runtime.requests[0].Input[stepInputKeyRuntimeRepairFollowUp]) {
		t.Fatalf("runtime requests = %+v, want runtime repair dispatch", runtime.requests)
	}
	if runtime.requests[0].Input[stepInputKeyRepairOfFailureSig] != signature {
		t.Fatalf("repair signature = %#v, want %q", runtime.requests[0].Input[stepInputKeyRepairOfFailureSig], signature)
	}
}
