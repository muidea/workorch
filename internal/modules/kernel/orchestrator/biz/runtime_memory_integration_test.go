package orchestrator

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	memorycommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRuntimeOrchestrator_StartRunPersistsRuntimeMemoryWrites(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status":  "completed",
		"stage":   "llm_dod_assessment",
		"summary": "Run completed with targeted tests",
		"memoryWrites": []map[string]any{
			{
				"memoryType": "user_preference",
				"summary":    "Default test command preference: go test ./...",
				"content":    "Prefer go test ./... for final verification.",
				"tags":       []string{"testing", "workflow"},
			},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memoryRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-memory-write")
	runID := common.RunID("run-memory-write")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	items, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memory items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("memory items = %+v, want explicit user preference only", items)
	}
	if !hasMemoryItem(items, runID, "user_preference", "Default test command preference: go test ./...") {
		t.Fatalf("memory items = %+v, want persisted user preference", items)
	}
	step, err := stepRepo.Get(ctx, common.StepID("run-memory-write/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	refs, ok := step.Output["memoryRefs"].([]string)
	if !ok || len(refs) != 1 {
		t.Fatalf("memoryRefs = %+v, want created memory ids", step.Output["memoryRefs"])
	}
}

func TestRuntimeOrchestrator_DoesNotCreateImplicitRunSummaryMemoryFromDoDAssessment(t *testing.T) {
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_dod_assessment",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied":             true,
			"summary":               "目标已完成，成功查询到服务配置信息",
			"goalOutcome":           "已成功查询远程服务器配置信息。",
			"implementationSummary": "通过 SSH 获取 CPU、内存、磁盘和 Docker 版本。",
			"verificationSummary":   "命令执行成功，exit code 0。",
		},
	}

	requests := runtimeRunSummaryMemoryWrite(output)
	if len(requests) != 0 {
		t.Fatalf("memory requests = %+v, want no implicit run_summary duplicate", requests)
	}
}

func TestRuntimeOrchestrator_DoDFeedbackMemoryCapturesAssessmentGaps(t *testing.T) {
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_dod_assessment",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "DoD still lacks verification evidence",
			"items": []map[string]any{
				{
					"item":   "streaming usage is verified",
					"state":  "failed",
					"reason": "no stream test evidence",
				},
				{
					"item":         "non-streaming usage is verified",
					"state":        "satisfied",
					"evidenceRefs": []string{"run/verify"},
				},
			},
		},
	}

	requests := runtimeMemoryWriteRequestsMust(t, output)
	if !memoryWriteRequestContains(requests, memorycommon.MemoryTypeDoDFeedback, "streaming usage is verified") {
		t.Fatalf("memory requests = %+v, want dod_feedback gap", requests)
	}
}

func TestRuntimeOrchestrator_RuntimeMemoryWriteRejectsTransientProviderRetry(t *testing.T) {
	requests, err := runtimeMemoryWriteRequests(map[string]any{
		"memoryWrites": []map[string]any{
			{
				"memoryType": memorycommon.MemoryTypeToolFeedback,
				"summary":    "Provider retry outcome captured",
				"content":    "Provider retry attempt: 1\nfailureClass: provider_timeout",
				"tags":       []string{"provider_retry", "tool_context", "necessary_output"},
			},
		},
	})
	if err != nil {
		t.Fatalf("runtimeMemoryWriteRequests: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("memory requests = %+v, want transient provider retry rejected", requests)
	}
}

func TestRuntimeOrchestrator_RuntimeMemoryWriteRejectsContractViolationDiagnostic(t *testing.T) {
	requests, err := runtimeMemoryWriteRequests(map[string]any{
		"memoryWrites": []map[string]any{
			{
				"memoryType": memorycommon.MemoryTypeProjectContext,
				"summary":    "contractViolationRecovery retry detail",
				"content":    "violatedRule: state_changing_required_missing\nrecoverability: retry_same_contract",
				"tags":       []string{"contract_violation", "diagnostic_only"},
			},
		},
	})
	if err != nil {
		t.Fatalf("runtimeMemoryWriteRequests: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("memory requests = %+v, want contract violation diagnostic rejected", requests)
	}
}

func TestRuntimeOrchestrator_ContractViolationRetryDoesNotCreateRepairLessonMemory(t *testing.T) {
	runID := common.RunID("run-contract-violation-memory")
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/contract-retry-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"failedStepKind":                  "file/read",
			"failureReason":                   "open target: no such file",
			"contractViolationRetry":          true,
			stepInputKeyContractViolationRecovery: map[string]any{
				"violatedRule":   "state_changing_required_missing",
				"recoverability": "retry_same_contract",
			},
		},
	}

	requests := appendNecessaryOutputMemoryWriteRequests(nil, &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-memory"}, step, map[string]any{"status": "completed"})
	if len(requests) != 0 {
		t.Fatalf("memory requests = %+v, want no implicit repair lesson for contract violation retry", requests)
	}
}

func TestRuntimeOrchestrator_MCPFailureWritesBoundedToolFeedback(t *testing.T) {
	runID := common.RunID("run-mcp-tool-feedback")
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/tool-001"),
		RunID: runID,
		Kind:  "mcp/tool_call",
	}
	output := map[string]any{
		"status":                  "failed",
		"stage":                   "mcp_tool_call_server_unavailable",
		"serverId":                "mcp-code",
		"tool":                    "semantic.lookup",
		"error":                   "executable not found",
		failureOutputKeyClass:     failureClassMCPRuntime,
		failureOutputKeySignature: "class=mcp_runtime_failed|kind=mcp/tool_call|serverId=mcp-code|tool=semantic.lookup",
	}

	requests := appendNecessaryOutputMemoryWriteRequests(nil, &runmodel.Run{ID: runID, WorkspaceID: "ws-mcp-tool-feedback"}, step, output)
	if !memoryWriteRequestContains(requests, memorycommon.MemoryTypeToolFeedback, "serverId: mcp-code") ||
		!memoryWriteRequestContains(requests, memorycommon.MemoryTypeToolFeedback, "tool: semantic.lookup") ||
		!memoryWriteRequestContains(requests, memorycommon.MemoryTypeToolFeedback, "evidenceRef: "+string(step.ID)) {
		t.Fatalf("memory requests = %+v, want bounded MCP tool feedback", requests)
	}
}

func TestRuntimeOrchestrator_RuntimeMemoryRejectsSensitiveMCPEnvFeedback(t *testing.T) {
	requests, err := runtimeMemoryWriteRequests(map[string]any{
		"memoryWrites": []map[string]any{
			{
				"memoryType": memorycommon.MemoryTypeToolFeedback,
				"summary":    "Accepted tool and MCP context",
				"content":    "mcpServers: mcp-code\nenv: API_KEY=secret-value",
				"tags":       []string{"accepted_input", "tool_context", "mcp_context"},
			},
		},
	})
	if err != nil {
		t.Fatalf("runtimeMemoryWriteRequests: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("memory requests = %+v, want sensitive env feedback rejected", requests)
	}
}

func TestRuntimeOrchestrator_ArtifactChunkStepSuppressesMemoryWrites(t *testing.T) {
	ctx := context.Background()
	run := &runmodel.Run{ID: "run-artifact-memory", WorkspaceID: "ws-artifact-memory"}
	step := &runmodel.Step{
		ID:    "run-artifact-memory/artifact-chunk-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"outputMode": outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": artifactOutputPhaseChunk,
			},
			"artifactContinuationContext": map[string]any{
				"state":            string(common.ArtifactContinuationStateChunking),
				"artifactId":       "artifact-001",
				"targetRef":        "deliverable.txt",
				"expectedSequence": 1,
			},
		},
	}
	output := map[string]any{
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable.txt",
			"sequence":   1,
			"content":    "artifact body must stay out of memory",
			"isFinal":    true,
			"chunkHash":  "sha256:chunk",
		},
		stepOutputKeyMemoryWrites: []map[string]any{{
			"memoryType": memorycommon.MemoryTypeProjectContext,
			"summary":    "Artifact content",
			"content":    "artifact body must stay out of memory",
			"tags":       []string{"reference_value"},
		}},
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			MemoryItems: memory.NewMemoryItemRepository(),
		},
	}

	refs, err := orch.recordMemoryWrites(ctx, run, step, output)
	if err != nil {
		t.Fatalf("recordMemoryWrites: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("memory refs = %+v, want artifact chunk memory suppressed", refs)
	}
	items, err := orch.evidence.MemoryItems.ListByWorkspaceID(ctx, run.WorkspaceID)
	if err != nil {
		t.Fatalf("ListByWorkspace: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("memory items = %+v, want none", items)
	}
}

func TestRuntimeOrchestrator_DoDFeedbackMemoryRejectsTransientProviderAssessment(t *testing.T) {
	requests := runtimeMemoryWriteRequestsMust(t, map[string]any{
		"status": "failed",
		"stage":  "llm_dod_assessment",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "因 LLM 提供者超时（context deadline exceeded），无法完成本次工作区执行规划。",
			"items": []map[string]any{
				{
					"item":   "功能准则",
					"state":  "skipped",
					"reason": "Provider retry attempt 1 failed",
				},
			},
		},
	})
	if len(requests) != 0 {
		t.Fatalf("memory requests = %+v, want transient provider assessment excluded", requests)
	}
}

func TestRuntimeOrchestrator_ExplicitDoDFeedbackMemoryRejectsTransientText(t *testing.T) {
	requests, err := runtimeMemoryWriteRequests(map[string]any{
		"memoryWrites": []map[string]any{
			{
				"memoryType": memorycommon.MemoryTypeDoDFeedback,
				"summary":    "Provider retry failed with context deadline exceeded",
				"content":    "DoD gaps were skipped because provider_timeout prevented assessment.\nreference_value",
				"tags":       []string{"dod_feedback", "reference_value"},
			},
		},
	})
	if err != nil {
		t.Fatalf("runtimeMemoryWriteRequests: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("memory requests = %+v, want explicit transient dod feedback rejected", requests)
	}
}

func TestRuntimeOrchestrator_AcceptedInputMemoryWritesCaptureSkillSoulToolContext(t *testing.T) {
	runID := common.RunID("run-accepted-input-memory")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-accepted-input-memory",
		ExecutionContext: map[string]any{
			"soul": map[string]any{
				"id":               "soul-software-dev-execution",
				"name":             "software execution",
				"description":      "execute implementation safely",
				"allowedToolKinds": []string{"shell/command", "patch/apply"},
			},
			"soulRouting": map[string]any{
				"policy": "workspace_stage_soul_routing",
			},
			"mcpSelection": map[string]any{
				"autoBoundMCPServerIds": []string{"mcp-code"},
			},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeySoulStage: soulStageExecution,
			stepInputKeyExecutionContextPackage: map[string]any{
				"selectedContext": map[string]any{
					"skills": []map[string]any{
						{
							"id":                "skill-go",
							"description":       "Go implementation workflow",
							"requiredToolKinds": []string{"shell/command"},
							"constraints":       []string{"run targeted tests"},
						},
					},
					"mcpServers": []map[string]any{
						{"id": "mcp-code", "transport": "stdio"},
					},
					"mcpCapabilities": []map[string]any{
						{"id": "cap-read", "kind": "resource"},
					},
				},
			},
		},
	}

	requests := acceptedInputMemoryWriteRequests(run, step)
	if !memoryWriteRequestContains(requests, memorycommon.MemoryTypeSkillFeedback, "skill-go") ||
		!memoryWriteRequestContains(requests, memorycommon.MemoryTypeToolFeedback, "mcp-code") ||
		!memoryWriteRequestContains(requests, memorycommon.MemoryTypeProjectContext, "soul-software-dev-execution") {
		t.Fatalf("accepted input memory requests = %+v, want skill/tool/soul context", requests)
	}
}

func TestRuntimeOrchestrator_RepairOutputMemorySkipsUnboundedFailureLesson(t *testing.T) {
	runID := common.RunID("run-repair-lesson-memory")
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/zz-llm-repair-failed-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  2,
			"failedStepId":                    string(runID) + "/block/test",
			"failureReason":                   "go test failed",
			failureOutputKeyClass:             failureClassShellCommand,
			failureOutputKeySignature:         "kind=shell/command|cmd=go test",
			"missingDoDItems": []map[string]any{
				{"item": "streaming usage verified"},
			},
		},
	}
	output := map[string]any{
		"status":  "completed",
		"stage":   "llm_execution_graph_repair",
		"summary": "Repair will add missing stream tests",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{"id": "patch-tests"}},
		},
	}

	requests := appendNecessaryOutputMemoryWriteRequests(nil, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-lesson-memory"}, step, output)
	if len(requests) != 0 {
		t.Fatalf("repair memory requests = %+v, want no unbounded process memory", requests)
	}
}

func TestRuntimeOrchestrator_RepairOutputMemoryWritesDistillMissingFileLesson(t *testing.T) {
	runID := common.RunID("run-repair-missing-file-lesson")
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-missing-file"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			"failedStepId":                    string(runID) + "/block/read_test_file",
			"failedStepKind":                  "file/read",
			"failureReason":                   "open main_test.go: no such file or directory",
			failureOutputKeySignature:         "kind=file/read|path=main_test.go",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "file_read_invalid_path",
	}

	requests := appendNecessaryOutputMemoryWriteRequests(nil, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-missing-file-lesson"}, step, output)
	if len(requests) != 1 ||
		requests[0].MemoryType != memorycommon.MemoryTypeExecutionLesson ||
		requests[0].Summary != "Missing file reads require optional probes or listing before repair" ||
		!strings.Contains(requests[0].Content, "lesson: Missing file reads require optional probes or listing before repair") ||
		!strings.Contains(requests[0].Content, "evidenceRef: "+string(step.ID)) {
		t.Fatalf("repair memory requests = %+v, want distilled missing-file lesson", requests)
	}
}

func TestRuntimeOrchestrator_RepairOutputMemoryDoesNotTreatPatchTextMismatchAsMissingFile(t *testing.T) {
	runID := common.RunID("run-repair-oldtext-not-found")
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-oldtext"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			"failedStepId":                    string(runID) + "/block/patch",
			"failedStepKind":                  "patch/apply",
			"failureReason":                   "conflict: patch text_replace oldText not found",
			failureOutputKeySignature:         "kind=patch/apply|error=oldText not found",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "patch_apply",
	}

	requests := appendNecessaryOutputMemoryWriteRequests(nil, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-oldtext-not-found"}, step, output)
	if len(requests) != 0 {
		t.Fatalf("repair memory requests = %+v, want patch text mismatch kept out of missing-file lesson memory", requests)
	}
}

func TestRuntimeOrchestrator_RepairOutputMemoryOmitsRejectedPlanSummary(t *testing.T) {
	runID := common.RunID("run-repair-rejected-summary-memory")
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-rejected-summary"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  4,
			"failedStepId":                    string(runID) + "/block/test",
			"failureReason":                   "contract validation failed",
			failureOutputKeyClass:             failureClassRuntime,
		},
	}
	output := map[string]any{
		"status":  "completed",
		"stage":   "llm_execution_graph_plan",
		"error":   "invalid argument: strategyReassessment.required=true requires state-changing patch output",
		"summary": "rootCauseTarget=config.yaml; repairTargetDecision=add a marker comment",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{"id": "patch-unrelated-marker"}},
		},
	}

	requests := appendNecessaryOutputMemoryWriteRequests(nil, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-rejected-summary-memory"}, step, output)
	if len(requests) != 0 {
		t.Fatalf("repair memory requests = %+v, want rejected repair process kept out of long-term memory", requests)
	}
}

func TestRuntimeOrchestrator_OptionalRunSummaryMemoryDoesNotRequireRepository(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{}
	refs, err := orch.recordMemoryWrites(ctx,
		&runmodel.Run{ID: "run-memory-optional", WorkspaceID: "ws-memory-optional"},
		&runmodel.Step{ID: "run-memory-optional/step-1", RunID: "run-memory-optional"},
		map[string]any{
			"status": "completed",
			"stage":  "llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "DoD satisfied",
			},
		},
	)
	if err != nil || len(refs) != 0 {
		t.Fatalf("recordMemoryWrites refs=%+v err=%v, want optional run summary skipped without repository", refs, err)
	}
}

func TestRuntimeOrchestrator_RuntimeMemoryWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	memoryRepo := &blockingMemoryItemRepository{
		inner:   memory.NewMemoryItemRepository(),
		started: started,
		release: release,
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			MemoryItems: memoryRepo,
		},
	}
	workspaceID := common.WorkspaceID("ws-memory-write-parallel")
	runID := common.RunID("run-memory-write-parallel")
	stepID := common.StepID("run-memory-write-parallel/step-1")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID}
	step := &runmodel.Step{ID: stepID, RunID: runID}
	output := map[string]any{
		"status":  "completed",
		"stage":   "llm_dod_assessment",
		"summary": "parallel memory write summary",
		stepOutputKeyMemoryWrites: []map[string]any{
			{
				"memoryType": "user_preference",
				"summary":    "first default preference",
				"content":    "first content",
			},
			{
				"memoryType": "project_context",
				"summary":    "second confirmed project convention",
				"content":    "second content",
				"tags":       []string{"confirmed"},
			},
		},
	}

	done := make(chan struct {
		refs []string
		err  error
	}, 1)
	go func() {
		refs, err := orch.recordMemoryWrites(ctx, run, step, output)
		done <- struct {
			refs []string
			err  error
		}{refs: refs, err: err}
	}()

	wantStarted := map[string]bool{
		"memory-create:run-memory-write-parallel/step-1/memory/1": false,
		"memory-create:run-memory-write-parallel/step-1/memory/2": false,
	}
	for !allSeen(wantStarted) {
		select {
		case name := <-started:
			if _, ok := wantStarted[name]; ok {
				wantStarted[name] = true
			}
		case result := <-done:
			t.Fatalf("recordMemoryWrites returned before all memory writes started: refs=%+v err=%v seen=%+v", result.refs, result.err, wantStarted)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for concurrent memory writes, saw %+v: %v", wantStarted, ctx.Err())
		}
	}
	close(release)
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("recordMemoryWrites: %v", result.err)
		}
		wantRefs := []string{
			"run-memory-write-parallel/step-1/memory/1",
			"run-memory-write-parallel/step-1/memory/2",
		}
		if !reflect.DeepEqual(result.refs, wantRefs) {
			t.Fatalf("refs = %+v, want %+v", result.refs, wantRefs)
		}
	case <-ctx.Done():
		t.Fatalf("recordMemoryWrites did not finish after release: %v", ctx.Err())
	}
	items, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memory items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("memory items = %+v, want 2", items)
	}
}

func TestRuntimeOrchestrator_FiltersDisallowedRuntimeMemoryWrites(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"stage":  "runtime_step",
		"memoryWrites": []map[string]any{
			{
				"memoryType": "decision",
				"summary":    "approval transcript",
				"content":    "single approval raw text",
			},
			{
				"memoryType": "project_context",
				"summary":    "confirmed workspace convention",
				"content":    "Prefer workspace-local commands.",
				"tags":       []string{"confirmed"},
			},
			{
				"memoryType": "user_preference",
				"summary":    "deployment secret",
				"content":    "api key: should not persist",
			},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memoryRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-memory-filter")
	runID := common.RunID("run-memory-filter")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	items, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memory items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("memory items = %+v, want project context write only", items)
	}
	if hasMemoryItem(items, runID, memorycommon.MemoryTypeDecision, "approval transcript") ||
		!hasMemoryItem(items, runID, memorycommon.MemoryTypeProjectContext, "confirmed workspace convention") {
		t.Fatalf("memory items = %+v, want decision filtered with project context only", items)
	}
}

func allSeen(items map[string]bool) bool {
	for _, seen := range items {
		if !seen {
			return false
		}
	}
	return true
}
