package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	memorycommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRuntimeOrchestrator_LLMInputIncludesExecutionContextPackage(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	var capturedInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		capturedInput = request.Input
		return map[string]any{
			"status": "completed",
			"dodAssessment": map[string]any{
				"satisfied": true,
				"summary":   "context package received",
				"items": []map[string]any{
					{"item": "context package available", "state": "satisfied", "evidenceRefs": []string{string(request.StepID)}},
				},
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
			Results:            resultRepo,
			Artifacts:          artifactRepo,
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memory.NewMemoryItemRepository(),
		},
		events.NewMemoryHub(),
	)

	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "README.md"), []byte("workspace"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-context-package")
	runID := common.RunID("run-context-package")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := resultRepo.Create(ctx, &resultmodel.Result{
		ID:           "result-context-package-accepted",
		WorkspaceID:  workspaceID,
		RunID:        "run-context-package-old",
		GoalID:       "goal-context-package",
		DoDID:        "dod-context-package",
		State:        common.ResultStateAccepted,
		Title:        "Historical Accepted Result",
		Summary:      "previous accepted delivery",
		EvidenceRefs: []string{"evidence-old"},
		ArtifactRefs: []common.ArtifactID{"artifact-context-package-old"},
		UpdatedAt:    "2026-04-30T01:00:00Z",
	}); err != nil {
		t.Fatalf("seed result: %v", err)
	}
	if err := artifactRepo.Create(ctx, &resultmodel.Artifact{
		ID:          "artifact-context-package-old",
		WorkspaceID: workspaceID,
		RunID:       "run-context-package-old",
		ResultID:    "result-context-package-accepted",
		StepID:      "step-old",
		Name:        "Accepted summary",
		Summary:     "accepted artifact summary",
		URI:         "workorch://runs/run-context-package-old/results/summary",
		State:       common.ArtifactStateAvailable,
		UpdatedAt:   "2026-04-30T01:00:00Z",
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-context-package",
		DoDID:           "dod-context-package",
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"skills": []map[string]any{
				{"id": "skill-context", "name": "Context Skill", "instruction": "Use packaged context."},
			},
			"mcpCapabilities": []map[string]any{
				{"id": "mcp-context:tool:repo.summary", "serverId": "mcp-context", "kind": "tool", "name": "repo.summary", "usageHint": "Summarize repository."},
			},
			"memories": []map[string]any{
				{"id": "memory-context", "memoryType": "project_context", "summary": "Use context package", "content": "Package must include selected memory."},
			},
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	pkg := mapFromValue(capturedInput[stepInputKeyExecutionContextPackage])
	if pkg["version"] != executionContextPackageVersion {
		t.Fatalf("executionContextPackage = %+v, want version %s", pkg, executionContextPackageVersion)
	}
	if manifest := mapSliceFromValue(pkg["workspaceFileManifest"]); len(manifest) != 1 || manifest[0]["path"] != "README.md" {
		t.Fatalf("workspaceFileManifest = %+v, want bounded current workspace manifest in planning context package", pkg["workspaceFileManifest"])
	}
	accepted := mapFromValue(pkg["latestAcceptedResult"])
	if accepted["id"] != "result-context-package-accepted" {
		t.Fatalf("latestAcceptedResult = %+v, want historical accepted result", accepted)
	}
	if deliverables := mapSliceFromValue(pkg["currentDeliverables"]); len(deliverables) != 1 || deliverables[0]["id"] != "artifact-context-package-old" {
		t.Fatalf("currentDeliverables = %+v, want historical artifact", pkg["currentDeliverables"])
	}
	selected := mapFromValue(pkg["selectedContext"])
	if len(mapSliceFromValue(selected["skills"])) != 1 || len(mapSliceFromValue(selected["mcpCapabilities"])) != 1 {
		t.Fatalf("selectedContext = %+v, want selected skill and mcp capability", selected)
	}
	if len(mapSliceFromValue(selected["memories"])) != 0 {
		t.Fatalf("selectedContext.memories = %+v, want stale stable memories omitted when dynamic memory recall selected none", selected["memories"])
	}
}

func TestRuntimeOrchestrator_EnrichLLMStepInputContextDoesNotPersistIntermediateStep(t *testing.T) {
	ctx := context.Background()
	stepRepo := newCountingStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-context-package-efficiency")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-context-package-efficiency",
		ExecutionContext: map[string]any{
			"skills": []map[string]any{
				{"id": "skill-efficiency", "name": "Efficiency Skill", "instruction": "Keep context available."},
			},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{stepInputKeySoulStage: soulStageExecution},
	}

	orch.enrichLLMStepInputContext(ctx, run, step, nil)

	if got := stepRepo.updateCount.Load(); got != 0 {
		t.Fatalf("step update count = %d, want 0 redundant intermediate updates", got)
	}
	pkg := mapFromValue(step.Input[stepInputKeyExecutionContextPackage])
	if pkg["version"] != executionContextPackageVersion {
		t.Fatalf("executionContextPackage = %+v, want version %s", pkg, executionContextPackageVersion)
	}
	selected := mapFromValue(pkg["selectedContext"])
	if skills := mapSliceFromValue(selected["skills"]); len(skills) != 1 || skills[0]["id"] != "skill-efficiency" {
		t.Fatalf("selected skills = %+v, want injected skill context", selected["skills"])
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageReusesInputWorkspaceManifest(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-context-package-manifest-reuse")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-context-package-manifest-reuse"}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyWorkspaceFileManifest: []map[string]any{
				{"path": "README.md", "kind": "file", "size": float64(12)},
			},
		},
	}

	pkg := orch.runExecutionContextPackage(ctx, run, step, nil)

	manifest := mapSliceFromValue(pkg["workspaceFileManifest"])
	if len(manifest) != 1 || manifest[0]["path"] != "README.md" {
		t.Fatalf("workspaceFileManifest = %+v, want manifest from step input", pkg["workspaceFileManifest"])
	}
}

func TestWorkspaceFileManifestSkipsRepositoryAndRuntimeNoise(t *testing.T) {
	deliverableDirectory := t.TempDir()
	for _, path := range []string{
		".git/objects/aa/object",
		".workorch-data/system/event_record.json",
		"node_modules/pkg/index.js",
		"dist/bundle.js",
		"README.md",
		"cmd/workorch/main.go",
	} {
		target := filepath.Join(deliverableDirectory, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(target, []byte("content"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	manifest := workspaceFileManifestFromDirectory(deliverableDirectory)
	var paths []string
	for _, item := range manifest {
		paths = append(paths, fmt.Sprint(item["path"]))
	}
	if !reflect.DeepEqual(paths, []string{"README.md", "cmd/workorch/main.go"}) {
		t.Fatalf("manifest paths = %+v, want only source-facing files", paths)
	}
}

func TestResolveWorkspaceTargetRefStripsDiagnosticPrefix(t *testing.T) {
	files := []workspaceDeliverableFile{
		{Path: "ai-gateway"},
		{Path: "proxy/proxy.go"},
		{Path: "proxy/proxy_test.go"},
	}
	if got := resolveWorkspaceTargetRef(files, "ai-gateway/proxy/proxy.go"); got != "proxy/proxy.go" {
		t.Fatalf("resolve target = %q, want proxy/proxy.go", got)
	}
}

func TestRepairPatchRelevantTargetRefsUseManifestCanonicalTargets(t *testing.T) {
	input := map[string]any{
		stepInputKeyWorkspaceFileManifest: []map[string]any{
			{"path": "ai-gateway", "kind": "file", "executable": true},
			{"path": "proxy/proxy.go", "kind": "file"},
			{"path": "proxy/proxy_test.go", "kind": "file"},
		},
		"failureTargetScope": "failureTargets=ai-gateway/proxy/proxy.go,proxy/non_stream_test.go",
		"repairActionPackage": map[string]any{
			"targetSnapshots": []map[string]any{{
				"targetRef": "proxy/proxy.go",
				"exists":    true,
			}},
		},
	}
	targets := repairPatchRelevantTargetRefsFromInput(input)
	if !reflect.DeepEqual(targets, []string{"proxy/proxy.go"}) {
		t.Fatalf("repair targets = %+v, want only canonical existing target", targets)
	}
}

func TestValidateRepairPatchTargetRelevanceAllowsDeclaredRelatedCanonicalTarget(t *testing.T) {
	step := &runmodel.Step{
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"runtimeStage":                runtimeStageRepair,
			"repairObjective":             "produce_state_changing_output",
			"stateChangingOutputRequired": true,
			stepInputKeyWorkspaceFileManifest: []map[string]any{
				{"path": "ai-gateway", "kind": "file", "executable": true},
				{"path": "proxy/proxy.go", "kind": "file"},
				{"path": "proxy/proxy_test.go", "kind": "file"},
			},
			"failureTargetScope": "failureTargets=ai-gateway/proxy/proxy.go",
		},
	}
	request := runtimePatchRequest{
		TargetRef: "proxy/proxy_test.go",
		PatchType: "text_replace",
		OldText:   "old",
		NewText:   "new",
		Extra: map[string]any{
			"relatedFailureTargets": []string{"ai-gateway/proxy/proxy.go"},
		},
	}
	if err := validateRepairPatchTargetRelevance(step, request); err != nil {
		t.Fatalf("validateRepairPatchTargetRelevance: %v", err)
	}
}

func TestRuntimeEvidenceAndStepEventsUseRawRefsInsteadOfDuplicatingFullPayloads(t *testing.T) {
	run := &runmodel.Run{ID: "run-ref-evidence", WorkspaceID: "ws-ref-evidence"}
	step := &runmodel.Step{
		ID:    "run-ref-evidence/repair-001-of-planning-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"executionContext": map[string]any{
				"governanceHistory": []map[string]any{{
					"ticketContext": map[string]any{"failedStepInput": map[string]any{"large": strings.Repeat("x", 64*1024)}},
				}},
			},
			"repairActionPackage": map[string]any{
				"targetSnapshots": []map[string]any{{"targetRef": "main.go", "content": "package main\n"}},
			},
		},
		Output: map[string]any{
			"status": "failed",
			"stderr": strings.Repeat("compile error\n", 4096),
		},
	}

	llmContext := newLLMContextSubsystem()
	args := runtimeToolCallArguments(llmContext, run, step)
	eventInput := runtimeStepEventInput(llmContext, run, step)
	eventOutput := runtimeStepEventOutput(llmContext, run, step)

	encoded, err := json.Marshal(map[string]any{
		"args":        args,
		"eventInput":  eventInput,
		"eventOutput": eventOutput,
	})
	if err != nil {
		t.Fatalf("marshal compact evidence payloads: %v", err)
	}
	text := string(encoded)
	if strings.Contains(text, strings.Repeat("x", 1024)) || strings.Contains(text, "compile error\ncompile error\ncompile error\ncompile error\n") {
		t.Fatalf("compact evidence payloads retained raw heavy data: %s", text)
	}
	if args["inputRef"] == "" || eventInput["inputRef"] == "" || eventOutput["outputRef"] == "" {
		t.Fatalf("refs missing: args=%+v eventInput=%+v eventOutput=%+v", args, eventInput, eventOutput)
	}
}

func TestRuntimeRepairInputOmitsInvalidLLMRawContentFromRepairContext(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-invalid-llm-redacted")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"stage":               "llm_schema_invalid",
			"status":              "failed",
			"error":               "invalid character '}' after array element",
			failureOutputKeyClass: failureClassLLMDecisionInvalid,
			"rawContentPreview":   `{"executionGraph":{"nodes":[{"kind":"patch/apply","input":{"diff":"@@ -1 +1 @@\n+[REDACTED]"}}]}}`,
			"streamEvents": []map[string]any{{
				"delta": `{"patches":[{"content":"[REDACTED]"}]}`,
			}},
			"streamDiagnostics": map[string]any{
				"doneReceived":     true,
				"streamIncomplete": false,
			},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-invalid-llm-redacted"}, failedStep, 1, common.ErrInvalidArgument)

	for _, key := range []string{"failedStepOutput", "previousStepOutput"} {
		if _, ok := input[key]; ok {
			t.Fatalf("%s should be omitted when repairActionPackage is primary", key)
		}
	}
	compaction := mapFromValue(input["repairInputCompaction"])
	for _, key := range []string{"failedStepOutput", "previousStepOutput"} {
		if !stringSliceContains(stringSliceFromValue(compaction["omittedFields"]), key) {
			t.Fatalf("repairInputCompaction = %#v, want %s omitted", compaction, key)
		}
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	summary := mapFromValue(actionPackage["previousRejectedOutputSummary"])
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal previousRejectedOutputSummary: %v", err)
	}
	if strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("previousRejectedOutputSummary still exposes redacted marker: %s", encoded)
	}
	if summary["rawContentPreviewOmitted"] != true || summary["streamEventsOmitted"] != true {
		t.Fatalf("previousRejectedOutputSummary = %#v, want invalid content omitted", summary)
	}
	if _, ok := summary["partialOutputPreview"]; ok {
		t.Fatalf("previousRejectedOutputSummary.partialOutputPreview should be omitted for invalid LLM decisions")
	}
}

func TestRunExecutionInputKeepsOnlySlimExecutionContext(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-compact-execution-context",
		WorkspaceID: "ws-compact-execution-context",
		GoalID:      "goal-compact",
		DoDID:       "dod-compact",
		ExecutionContext: map[string]any{
			"dialogLanguage": "zh-CN",
			"llmProviderId":  "reasoning",
			"skills":         []map[string]any{{"id": "skill-compact", "instruction": strings.Repeat("x", 1024)}},
			"compressedContext": map[string]any{
				"version": "workorch.compressedContext.v1",
				"mode":    "deterministic_recovery_summary",
			},
		},
	}

	input := runExecutionInput(run, "run-compact-execution-context/step-1")

	if input["workspaceId"] != "ws-compact-execution-context" || input["goalId"] != "goal-compact" || input["doDId"] != "dod-compact" {
		t.Fatalf("input = %+v, want run identity", input)
	}
	executionContext := mapFromValue(input["executionContext"])
	if executionContext["dialogLanguage"] != "zh-CN" || executionContext["llmProviderId"] != nil {
		t.Fatalf("executionContext = %+v, want slim control fields without persisted LLM provider binding", executionContext)
	}
	if _, ok := executionContext["skills"]; ok {
		t.Fatalf("executionContext retained raw skills: %+v", executionContext["skills"])
	}
	if compressed := mapFromValue(executionContext["compressedContext"]); compressed["version"] != "workorch.compressedContext.v1" {
		t.Fatalf("compressedContext = %+v, want compact compressed context", compressed)
	}
}

func TestInitialStepInputUsesEvidenceAcquisitionForReplanContinuationWithoutTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-replan-state-changing-repair",
		WorkspaceID:     "ws-replan-state-changing-repair",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-replan-state-changing",
				"reason":           "previous repair returned read-only graph",
			},
		},
	}

	input := initialStepInput(run)

	if input[stepInputKeySoulStage] != soulStageExecution ||
		input[stepInputKeyRuntimeStage] != runtimeStageRepair ||
		input["repairObjective"] != repairObjectiveAcquireFailureEvidence ||
		input["stateChangingOutputRequired"] != nil ||
		input["concreteFailureEvidenceRequired"] != true ||
		input["replanStateChangingContinuation"] != true {
		t.Fatalf("initial replan input = %+v, want evidence acquisition repair constraints", input)
	}
	guidance := stringSliceFromValue(input["repairGuidance"])
	joined := strings.Join(guidance, "\n")
	if !strings.Contains(joined, "Acquire bounded concrete failure evidence") || strings.Contains(joined, "patch/apply until current failure evidence") == false {
		t.Fatalf("repairGuidance = %#v, want evidence acquisition guidance", guidance)
	}
	executionContext := mapFromValue(input["executionContext"])
	if len(mapFromValue(executionContext["replanContinuation"])) == 0 {
		t.Fatalf("executionContext = %+v, want compact replan continuation", executionContext)
	}
}

func TestRuntimeOrchestrator_FollowUpInputInheritsWorkspaceManifest(t *testing.T) {
	runID := common.RunID("run-follow-up-manifest-reuse")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-follow-up-manifest-reuse"}
	previousStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{
				stepInputKeyWorkspaceFileManifest: []map[string]any{
					{"path": "README.md", "kind": "file", "size": float64(12)},
				},
			},
		},
		Output: map[string]any{
			stepOutputKeyNextStep: "follow-up",
			stepOutputKeyNextKind: llmcommon.StepKindWorkspaceExecution,
		},
	}

	input := nextStepInputFromOutput(run, previousStep)
	manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) != 1 || manifest[0]["path"] != "README.md" {
		t.Fatalf("workspaceFileManifest = %+v, want inherited manifest", input[stepInputKeyWorkspaceFileManifest])
	}
	manifest[0]["path"] = "mutated.md"
	previousManifest := mapSliceFromValue(mapFromValue(previousStep.Input[stepInputKeyExecutionContextPackage])[stepInputKeyWorkspaceFileManifest])
	if previousManifest[0]["path"] != "README.md" {
		t.Fatalf("previous manifest mutated to %+v, want cloned follow-up manifest", previousManifest)
	}
}

func TestRuntimeOrchestrator_FollowUpInputInheritsStableExecutionContextPackage(t *testing.T) {
	runID := common.RunID("run-follow-up-context-package-reuse")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-follow-up-context-package-reuse"}
	previousStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{
				"version":     executionContextPackageVersion,
				"workspaceId": "ws-follow-up-context-package-reuse",
				"runId":       string(runID),
				"stepId":      string(runID) + "/step-1",
				"stage":       soulStageExecution,
				"latestAcceptedResult": map[string]any{
					"id":      "result-follow-up-reuse",
					"summary": "accepted historical result",
				},
				"currentDeliverables": []map[string]any{
					{"id": "artifact-follow-up-reuse", "summary": "available artifact"},
				},
				"selectedContext": map[string]any{
					"skills": []map[string]any{{"id": "skill-follow-up-reuse"}},
				},
				executionContextPackageHistoryKey: map[string]any{
					"latestAcceptedResult": true,
					"currentDeliverables":  true,
				},
			},
		},
		Output: map[string]any{
			stepOutputKeyNextStep: "follow-up",
			stepOutputKeyNextKind: llmcommon.StepKindWorkspaceExecution,
		},
	}

	input := nextStepInputFromOutput(run, previousStep)
	pkg := mapFromValue(input[stepInputKeyExecutionContextPackage])
	if pkg["stepId"] != nil || pkg["stage"] != nil || pkg["workspaceId"] != nil || pkg["runId"] != nil {
		t.Fatalf("inherited stable package = %+v, want no previous step identity fields", pkg)
	}
	if accepted := mapFromValue(pkg["latestAcceptedResult"]); accepted["id"] != "result-follow-up-reuse" {
		t.Fatalf("latestAcceptedResult = %+v, want inherited result", accepted)
	}
	if deliverables := mapSliceFromValue(pkg["currentDeliverables"]); len(deliverables) != 1 || deliverables[0]["id"] != "artifact-follow-up-reuse" {
		t.Fatalf("currentDeliverables = %+v, want inherited deliverable", pkg["currentDeliverables"])
	}
	if selected := mapFromValue(pkg["selectedContext"]); len(mapSliceFromValue(selected["skills"])) != 1 {
		t.Fatalf("selectedContext = %+v, want inherited selected context", selected)
	}
	historyChecked := mapFromValue(pkg[executionContextPackageHistoryKey])
	if !boolFromValue(historyChecked["latestAcceptedResult"]) || !boolFromValue(historyChecked["currentDeliverables"]) {
		t.Fatalf("workspaceHistoryChecked = %+v, want inherited history checks", historyChecked)
	}
	if _, ok := pkg["governanceHistory"]; ok {
		t.Fatalf("governanceHistory = %+v, want governance history rebuilt from current run context", pkg["governanceHistory"])
	}
}

func TestNextStepInputFromOutputInheritsExplicitStageForDirectLLMContinuation(t *testing.T) {
	runID := common.RunID("run-direct-llm-next-stage")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-direct-llm-next-stage"}
	previousStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageRepair,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			stepInputKeySoulStage:        soulStageExecution,
		},
		Output: map[string]any{
			stepOutputKeyNextStep: "follow-up",
			stepOutputKeyNextKind: llmcommon.StepKindWorkspaceExecution,
		},
	}

	input := nextStepInputFromOutput(run, previousStep)

	if input[stepInputKeyRuntimeStage] != runtimeStageRepair ||
		input[stepInputKeyRuntimeStageMode] != "strict_convergence" ||
		input[stepInputKeySoulStage] != soulStageExecution {
		t.Fatalf("nextStepInput = %+v, want inherited explicit repair stage", input)
	}
}

func TestNextStepInputFromOutputDoesNotInferGovernanceStageFromBlockedLLMStep(t *testing.T) {
	runID := common.RunID("run-direct-llm-next-governance")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-direct-llm-next-governance"}
	previousStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStagePlanning,
			stepInputKeySoulStage:    soulStagePlanning,
		},
		Output: map[string]any{
			stepOutputKeyBlockDecisionReason: "decision required",
			stepOutputKeyNextStep:            "after-decision",
			stepOutputKeyNextKind:            llmcommon.StepKindWorkspaceExecution,
		},
	}

	input := nextStepInputFromOutput(run, previousStep)

	if input[stepInputKeyRuntimeStage] != nil || input[stepInputKeySoulStage] != nil {
		t.Fatalf("nextStepInput = %+v, want governance continuation stage to remain unresolved until resume merge", input)
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageReusesEmptyHistoryChecks(t *testing.T) {
	ctx := context.Background()
	resultRepo := &countingResultRepository{inner: memory.NewResultRepository()}
	artifactRepo := &countingArtifactRepository{inner: memory.NewArtifactRepository()}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
	}
	runID := common.RunID("run-context-package-empty-history-reuse")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-context-package-empty-history-reuse"}
	firstStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{stepInputKeySoulStage: soulStageExecution},
	}

	firstPackage := orch.runExecutionContextPackage(ctx, run, firstStep, &workspacemodel.Workspace{ID: run.WorkspaceID, DeliverableDirectory: t.TempDir()})
	if got := resultRepo.listByWorkspaceCount.Load(); got != 1 {
		t.Fatalf("first result ListByWorkspace count = %d, want initial history check", got)
	}
	if got := artifactRepo.listByWorkspaceCount.Load(); got != 1 {
		t.Fatalf("first artifact ListByWorkspace count = %d, want initial history check", got)
	}
	historyChecked := mapFromValue(firstPackage[executionContextPackageHistoryKey])
	if !boolFromValue(historyChecked["latestAcceptedResult"]) || !boolFromValue(historyChecked["currentDeliverables"]) {
		t.Fatalf("first workspaceHistoryChecked = %+v, want explicit empty history check markers", historyChecked)
	}

	followUpStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeySoulStage:               soulStageValidation,
			stepInputKeyExecutionContextPackage: firstPackage,
		},
	}
	secondPackage := orch.runExecutionContextPackage(ctx, run, followUpStep, &workspacemodel.Workspace{ID: run.WorkspaceID, DeliverableDirectory: t.TempDir()})

	if got := resultRepo.listByWorkspaceCount.Load(); got != 1 {
		t.Fatalf("result ListByWorkspace count = %d, want follow-up to reuse empty history check", got)
	}
	if got := artifactRepo.listByWorkspaceCount.Load(); got != 1 {
		t.Fatalf("artifact ListByWorkspace count = %d, want follow-up to reuse empty history check", got)
	}
	secondHistoryChecked := mapFromValue(secondPackage[executionContextPackageHistoryKey])
	if !boolFromValue(secondHistoryChecked["latestAcceptedResult"]) || !boolFromValue(secondHistoryChecked["currentDeliverables"]) {
		t.Fatalf("second workspaceHistoryChecked = %+v, want retained empty history checks", secondHistoryChecked)
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageReusesStableFollowUpPackage(t *testing.T) {
	ctx := context.Background()
	resultRepo := &countingResultRepository{inner: memory.NewResultRepository()}
	artifactRepo := &countingArtifactRepository{inner: memory.NewArtifactRepository()}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
	}
	runID := common.RunID("run-context-package-stable-reuse")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-context-package-stable-reuse",
		ExecutionContext: map[string]any{
			"governanceHistory": []map[string]any{
				{"kind": "decision", "summary": "current run context decision"},
			},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			stepInputKeySoulStage:        soulStageValidation,
			stepInputKeyExecutionContextPackage: map[string]any{
				"stepId": "old-step-should-not-leak",
				"stage":  soulStageExecution,
				"latestAcceptedResult": map[string]any{
					"id":      "result-context-package-stable-reuse",
					"summary": "accepted reused result",
				},
				"currentDeliverables": []map[string]any{
					{"id": "artifact-context-package-stable-reuse", "summary": "reused artifact"},
				},
				"selectedContext": map[string]any{
					"skills": []map[string]any{{"id": "skill-context-package-stable-reuse"}},
				},
				"governanceHistory": []map[string]any{
					{"kind": "approval", "summary": "stale package approval"},
				},
			},
		},
	}

	pkg := orch.runExecutionContextPackage(ctx, run, step, &workspacemodel.Workspace{ID: run.WorkspaceID, DeliverableDirectory: t.TempDir()})

	if got := resultRepo.listByWorkspaceCount.Load(); got != 0 {
		t.Fatalf("result ListByWorkspace count = %d, want 0 when stable result context is inherited", got)
	}
	if got := artifactRepo.listByWorkspaceCount.Load(); got != 0 {
		t.Fatalf("artifact ListByWorkspace count = %d, want 0 when stable deliverable context is inherited", got)
	}
	if pkg["stepId"] != string(step.ID) || pkg["stage"] != runtimeStageVerification {
		t.Fatalf("executionContextPackage = %+v, want current step identity", pkg)
	}
	if pkg["stageMode"] != "strict_convergence" {
		t.Fatalf("executionContextPackage stageMode = %v, want strict_convergence", pkg["stageMode"])
	}
	if accepted := mapFromValue(pkg["latestAcceptedResult"]); accepted["id"] != "result-context-package-stable-reuse" {
		t.Fatalf("latestAcceptedResult = %+v, want inherited result", accepted)
	}
	if deliverables := mapSliceFromValue(pkg["currentDeliverables"]); len(deliverables) != 1 || deliverables[0]["id"] != "artifact-context-package-stable-reuse" {
		t.Fatalf("currentDeliverables = %+v, want inherited deliverable", pkg["currentDeliverables"])
	}
	if selected := mapFromValue(pkg["selectedContext"]); len(mapSliceFromValue(selected["skills"])) != 1 {
		t.Fatalf("selectedContext = %+v, want inherited selected context", selected)
	}
	if history := mapSliceFromValue(pkg["governanceHistory"]); len(history) != 1 || history[0]["summary"] != "current run context decision" {
		t.Fatalf("governanceHistory = %+v, want current run context governance history", pkg["governanceHistory"])
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageDoesNotScanProvidedWorkspaceByDefault(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := newCountingWorkspaceRepository()
	workspaceID := common.WorkspaceID("ws-context-package-workspace-reuse")
	workingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: workingDir, State: common.WorkspaceStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo}
	runID := common.RunID("run-context-package-workspace-reuse")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{stepInputKeySoulStage: soulStageExecution},
	}

	pkg := orch.runExecutionContextPackage(ctx, run, step, workspace)

	if got := workspaceRepo.getCount.Load(); got != 0 {
		t.Fatalf("workspace Get count = %d, want 0 when workspace is already provided", got)
	}
	if manifest := mapSliceFromValue(pkg["workspaceFileManifest"]); len(manifest) != 0 {
		t.Fatalf("workspaceFileManifest = %+v, want no default workspace scan in LLM context package", pkg["workspaceFileManifest"])
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageIncludesManifestForReplanStateChangingContinuation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := newCountingWorkspaceRepository()
	workspaceID := common.WorkspaceID("ws-context-package-replan-manifest")
	workingDir := t.TempDir()
	for _, path := range []string{"README.md", "go.mod"} {
		if err := os.WriteFile(filepath.Join(workingDir, path), []byte("content"), 0o644); err != nil {
			t.Fatalf("seed workspace file %s: %v", path, err)
		}
	}
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: workingDir, State: common.WorkspaceStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo}
	runID := common.RunID("run-context-package-replan-manifest")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		ExecutionContext: map[string]any{
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-context-package-replan-manifest-old",
				"ticketContext": map[string]any{
					"failureSignature": "class=runtime_failed|kind=llm/workspace_execution",
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
		},
	}

	pkg := orch.runExecutionContextPackage(ctx, run, step, workspace)

	if got := workspaceRepo.getCount.Load(); got != 0 {
		t.Fatalf("workspace Get count = %d, want 0 when workspace is already provided", got)
	}
	manifest := mapSliceFromValue(pkg["workspaceFileManifest"])
	if len(manifest) != 2 || manifest[0]["path"] != "README.md" || manifest[1]["path"] != "go.mod" {
		t.Fatalf("workspaceFileManifest = %+v, want current workspace manifest for replan state-changing continuation", pkg["workspaceFileManifest"])
	}
	replan := mapFromValue(pkg["replanContinuation"])
	if mapFromValue(replan["ticketContext"])["failureSignature"] == "" {
		t.Fatalf("replanContinuation = %+v, want failure context retained", replan)
	}
}

func TestRuntimeOrchestrator_ResultManifestReusesProvidedWorkspace(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := newCountingWorkspaceRepository()
	workspaceID := common.WorkspaceID("ws-result-manifest-workspace-reuse")
	workingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDir, "README.md"), []byte("accepted"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: workingDir, State: common.WorkspaceStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	runID := common.RunID("run-result-manifest-workspace-reuse")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-result-manifest-workspace-reuse",
		DoDID:           "dod-result-manifest-workspace-reuse",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	orch := &RuntimeOrchestrator{
		workspaces: workspaceRepo,
		evidence: RuntimeEvidenceRepositories{
			Results:   memory.NewResultRepository(),
			Artifacts: memory.NewArtifactRepository(),
		},
	}

	err := orch.resultManager().recordRunResultFromDoDAssessment(ctx, run, step, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "accepted with reusable workspace",
			"items": []map[string]any{
				{"item": "workspace reused", "state": "satisfied", "evidenceRefs": []string{string(step.ID)}},
			},
		},
	}, workspace)
	if err != nil {
		t.Fatalf("record result: %v", err)
	}

	if got := workspaceRepo.getCount.Load(); got != 0 {
		t.Fatalf("workspace Get count = %d, want 0 when result manifest receives workspace", got)
	}
	artifacts, err := orch.evidence.Artifacts.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %+v, want result summary and deliverable manifest", artifacts)
	}
	for _, artifact := range artifacts {
		if !reflect.DeepEqual(artifact.EvidenceRefs, []string{string(step.ID)}) {
			t.Fatalf("artifact %s evidence refs = %+v, want assessment refs", artifact.ID, artifact.EvidenceRefs)
		}
		if artifact.Category == resultcommon.ArtifactCategoryRuntimeEvidence && !strings.Contains(artifact.Summary, fmt.Sprintf("- `%s`", step.ID)) {
			t.Fatalf("manifest summary = %q, want acceptance evidence ref", artifact.Summary)
		}
	}
}

func TestRuntimeOrchestrator_ResultManifestDoesNotListWorkspaceFilesWhenNoPatch(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-result-manifest-step-context-reuse")
	workingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDir, "from-disk.md"), []byte("stale disk content"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	runID := common.RunID("run-result-manifest-step-context-reuse")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-result-manifest-step-context-reuse",
		DoDID:           "dod-result-manifest-step-context-reuse",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{
				stepInputKeyWorkspaceFileManifest: []map[string]any{
					{"path": "from-context.md", "kind": "file", "size": float64(12)},
				},
			},
		},
	}
	orch := &RuntimeOrchestrator{}

	manifest := orch.resultManager().workspaceDeliverableManifestContent(ctx, run, step, map[string]any{
		"summary": "accepted with context manifest",
	}, []string{string(step.ID)}, &workspacemodel.Workspace{ID: workspaceID, DeliverableDirectory: workingDir}, nil)

	if !strings.Contains(manifest, "No workspace file deliverables were produced in this run.") ||
		strings.Contains(manifest, "`from-context.md`") ||
		strings.Contains(manifest, "`from-disk.md`") {
		t.Fatalf("manifest = %q, want no workspace file deliverables without explicit patches", manifest)
	}
}

func TestRuntimeOrchestrator_ResultManifestListsOnlyPatchTargets(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-result-manifest-patch-rescan")
	workingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDir, "from-disk.md"), []byte("fresh disk content"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	runID := common.RunID("run-result-manifest-patch-rescan")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-result-manifest-patch-rescan",
		DoDID:           "dod-result-manifest-patch-rescan",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{
				stepInputKeyWorkspaceFileManifest: []map[string]any{
					{"path": "from-context.md", "kind": "file", "size": float64(12)},
				},
			},
		},
		Output: map[string]any{
			stepOutputKeyPatches: []map[string]any{
				{"targetRef": "from-disk.md", "content": "fresh disk content"},
			},
		},
	}
	orch := &RuntimeOrchestrator{}

	manifest := orch.resultManager().workspaceDeliverableManifestContent(ctx, run, step, map[string]any{
		"summary": "accepted after patch",
	}, []string{string(step.ID)}, &workspacemodel.Workspace{ID: workspaceID, DeliverableDirectory: workingDir}, []resultPatchDetail{
		{TargetRef: "from-disk.md", PatchType: "file_write", Content: "fresh disk content", State: common.PatchStateApplied},
		{TargetRef: "from-disk.md", PatchType: "file_write", Content: "fresh disk content", State: common.PatchStateApplied},
	})

	if !strings.Contains(manifest, "`from-disk.md`") || strings.Contains(manifest, "`from-context.md`") {
		t.Fatalf("manifest = %q, want only explicit patch target deliverables", manifest)
	}
	deliverableSection := strings.Split(manifest, "## Workspace Change Evidence")[0]
	if got := strings.Count(deliverableSection, "`from-disk.md`"); got != 1 {
		t.Fatalf("manifest = %q, deliverable file row count = %d, want deduplicated row", manifest, got)
	}
}

func TestRuntimeOrchestrator_RunStateTransitionsReuseProvidedWorkspace(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := newCountingWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	workspaceID := common.WorkspaceID("ws-state-transition-workspace-reuse")
	runID := common.RunID("run-state-transition-workspace-reuse")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:       runRepo,
		workspaces: workspaceRepo,
	}

	if err := orch.blockOnDecision(ctx, run, workspace, common.TicketID("ticket-state-transition-workspace-reuse")); err != nil {
		t.Fatalf("block on decision: %v", err)
	}
	if got := workspaceRepo.getCount.Load(); got != 0 {
		t.Fatalf("workspace Get count after decision block = %d, want 0 when workspace is already provided", got)
	}
	if workspace.State != common.WorkspaceStateWaitingDecision {
		t.Fatalf("workspace state = %s, want waiting decision", workspace.State)
	}

	run.State = common.RunStateRunning
	run.BlockingTicketID = ""
	workspace.State = common.WorkspaceStateRunning
	if err := runRepo.Update(ctx, run); err != nil {
		t.Fatalf("reset run: %v", err)
	}
	if err := workspaceRepo.Update(ctx, workspace); err != nil {
		t.Fatalf("reset workspace: %v", err)
	}
	if err := orch.blockOnApproval(ctx, run, workspace, common.ApprovalID("approval-state-transition-workspace-reuse")); err != nil {
		t.Fatalf("block on approval: %v", err)
	}
	if got := workspaceRepo.getCount.Load(); got != 0 {
		t.Fatalf("workspace Get count after approval block = %d, want 0 when workspace is already provided", got)
	}
	if workspace.State != common.WorkspaceStateBlocked {
		t.Fatalf("workspace state = %s, want blocked", workspace.State)
	}
}

func TestRuntimeOrchestrator_ValidateCompleteReusesProvidedWorkspace(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := newCountingWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	workspaceID := common.WorkspaceID("ws-complete-workspace-reuse")
	runID := common.RunID("run-complete-workspace-reuse")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:       runRepo,
		workspaces: workspaceRepo,
	}

	if err := orch.validateAndCompleteRunWithWorkspace(ctx, run, common.StepID("run-complete-workspace-reuse/step-1"), workspace); err != nil {
		t.Fatalf("validate and complete: %v", err)
	}
	if got := workspaceRepo.getCount.Load(); got != 0 {
		t.Fatalf("workspace Get count = %d, want 0 when workspace is already provided", got)
	}
	if run.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", run.State)
	}
	if workspace.State != common.WorkspaceStateCompleted || workspace.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want completed and released", workspace)
	}
}

func TestRuntimeOrchestrator_ValidationRunAndWorkspaceWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	runRepo := &blockingValidatingRunRepository{
		inner:   memory.NewRunRepository(),
		started: started,
		release: release,
	}
	workspaceRepo := &blockingValidatingWorkspaceRepository{
		inner:   memory.NewWorkspaceRepository(),
		started: started,
		release: release,
	}
	workspaceID := common.WorkspaceID("ws-validating-write-parallel")
	runID := common.RunID("run-validating-write-parallel")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:       runRepo,
		workspaces: workspaceRepo,
	}

	done := make(chan error, 1)
	go func() {
		_, err := orch.syncRunAndWorkspaceForValidation(ctx, run, workspace)
		done <- err
	}()

	seen := map[string]bool{}
	for !seen["run-validating-update"] || !seen["workspace-validating-update"] {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("validation updates before release = %+v, want run and workspace writes started: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("sync validation state: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("sync validation state did not finish after release: %v", ctx.Err())
	}
	if run.State != common.RunStateValidating {
		t.Fatalf("run state = %s, want validating", run.State)
	}
	if workspace.State != common.WorkspaceStateValidating || workspace.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want validating with active run", workspace)
	}
}

func TestRuntimeOrchestrator_ActiveRunAndWorkspaceWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	runRepo := &blockingRunningRunRepository{
		inner:   memory.NewRunRepository(),
		started: started,
		release: release,
	}
	workspaceRepo := &blockingRunningWorkspaceRepository{
		inner:   memory.NewWorkspaceRepository(),
		started: started,
		release: release,
	}
	workspaceID := common.WorkspaceID("ws-active-write-parallel")
	runID := common.RunID("run-active-write-parallel")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateWaitingDecision, ActiveRunID: runID}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:       runRepo,
		workspaces: workspaceRepo,
	}

	done := make(chan error, 1)
	go func() {
		_, err := orch.runStateManager().syncRunAndWorkspaceForActiveState(ctx, run, common.WorkspaceStateRunning, workspace)
		done <- err
	}()

	seen := map[string]bool{}
	for !seen["run-running-update"] || !seen["workspace-running-update"] {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("active updates before release = %+v, want run and workspace writes started: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("sync active state: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("sync active state did not finish after release: %v", ctx.Err())
	}
	if run.State != common.RunStateRunning {
		t.Fatalf("run state = %s, want running", run.State)
	}
	if workspace.State != common.WorkspaceStateRunning || workspace.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want running with active run", workspace)
	}
}

func TestRuntimeOrchestrator_StartRunActiveWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	runRepo := &blockingRunningRunRepository{
		inner:   memory.NewRunRepository(),
		started: started,
		release: release,
	}
	workspaceRepo := &blockingRunningWorkspaceRepository{
		inner:   memory.NewWorkspaceRepository(),
		started: started,
		release: release,
	}
	stepRepo := memory.NewStepRepository()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-start-active-write-parallel")
	runID := common.RunID("run-start-active-write-parallel")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runtime:    &stubRuntimeAdapter{output: map[string]any{"status": "completed"}},
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		eventHub:   hub,
	}

	done := make(chan error, 1)
	go func() {
		done <- orch.StartRun(ctx, runID)
	}()

	seen := map[string]bool{}
	for !seen["run-running-update"] || !seen["workspace-running-update"] {
		select {
		case name := <-started:
			seen[name] = true
		case err := <-done:
			t.Fatalf("StartRun returned before both active writes started: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for concurrent StartRun active writes, saw %+v: %v", seen, ctx.Err())
		}
	}
	for _, item := range hub.Published() {
		if item.Topic == events.TopicRunStateChanged && item.Payload[events.PayloadKeyState] == string(common.RunStateRunning) {
			t.Fatalf("published running run event before active writes were released: %+v", hub.Published())
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("StartRun did not complete after releasing active writes: %v", ctx.Err())
	}
	published := hub.Published()
	var runningRunIndex = -1
	var runningWorkspaceIndex = -1
	for idx, item := range published {
		if item.Topic == events.TopicRunStateChanged && item.Payload[events.PayloadKeyState] == string(common.RunStateRunning) {
			runningRunIndex = idx
		}
		if item.Topic == events.TopicWorkspaceStateChanged && item.Payload[events.PayloadKeyState] == string(common.WorkspaceStateRunning) {
			runningWorkspaceIndex = idx
		}
	}
	if runningRunIndex < 0 || runningWorkspaceIndex < 0 || runningRunIndex > runningWorkspaceIndex {
		t.Fatalf("published events = %+v, want running run event before running workspace event", published)
	}
	updatedRun, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updatedRun.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", updatedRun.State)
	}
	updatedWorkspace, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if updatedWorkspace.State != common.WorkspaceStateCompleted || updatedWorkspace.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want completed and released", updatedWorkspace)
	}
}

func TestRuntimeOrchestrator_BlockedRunAndWorkspaceWritesConcurrently(t *testing.T) {
	cases := []struct {
		name           string
		runState       common.RunState
		workspaceState common.WorkspaceState
		configureRun   func(*runmodel.Run)
	}{
		{
			name:           "decision",
			runState:       common.RunStateBlockedOnDecision,
			workspaceState: common.WorkspaceStateWaitingDecision,
			configureRun: func(run *runmodel.Run) {
				run.BlockingTicketID = common.TicketID("ticket-blocked-write-parallel")
			},
		},
		{
			name:           "approval",
			runState:       common.RunStateBlockedOnApproval,
			workspaceState: common.WorkspaceStateBlocked,
			configureRun: func(run *runmodel.Run) {
				run.BlockingApprovalID = common.ApprovalID("approval-blocked-write-parallel")
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			started := make(chan string, 4)
			release := make(chan struct{})
			runRepo := &blockingBlockedRunRepository{
				inner:   memory.NewRunRepository(),
				started: started,
				release: release,
			}
			workspaceRepo := &blockingBlockedWorkspaceRepository{
				inner:   memory.NewWorkspaceRepository(),
				started: started,
				release: release,
			}
			workspaceID := common.WorkspaceID("ws-blocked-write-parallel-" + tt.name)
			runID := common.RunID("run-blocked-write-parallel-" + tt.name)
			workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}
			run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: tt.runState}
			tt.configureRun(run)
			if err := workspaceRepo.Create(ctx, workspace); err != nil {
				t.Fatalf("seed workspace: %v", err)
			}
			if err := runRepo.Create(ctx, run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			orch := &RuntimeOrchestrator{
				runs:       runRepo,
				workspaces: workspaceRepo,
			}

			done := make(chan error, 1)
			go func() {
				_, err := orch.runStateManager().syncRunAndWorkspaceForActiveState(ctx, run, tt.workspaceState, workspace)
				done <- err
			}()

			seen := map[string]bool{}
			for !seen["run-blocked-update"] || !seen["workspace-blocked-update"] {
				select {
				case name := <-started:
					seen[name] = true
				case <-ctx.Done():
					t.Fatalf("blocked updates before release = %+v, want run and workspace writes started: %v", seen, ctx.Err())
				}
			}
			close(release)
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("sync blocked state: %v", err)
				}
			case <-ctx.Done():
				t.Fatalf("sync blocked state did not finish after release: %v", ctx.Err())
			}
			if run.State != tt.runState {
				t.Fatalf("run state = %s, want %s", run.State, tt.runState)
			}
			if workspace.State != tt.workspaceState || workspace.ActiveRunID != runID {
				t.Fatalf("workspace = %+v, want %s with active run", workspace, tt.workspaceState)
			}
		})
	}
}

func TestRuntimeOrchestrator_BlockCheckpointUsesProvidedStepID(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	stepRepo := newCountingStepRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	workspaceID := common.WorkspaceID("ws-block-checkpoint-step-id")
	runID := common.RunID("run-block-checkpoint-step-id")
	stepID := common.StepID("run-block-checkpoint-step-id/step-1")
	ticketID := common.TicketID("run-block-checkpoint-step-id/step-1-decision")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		evidence: RuntimeEvidenceRepositories{
			RuntimeCheckpoints: checkpointRepo,
		},
		eventHub: events.NewMemoryHub(),
	}

	if err := orch.blockOnDecisionAtStep(ctx, run, workspace, ticketID, stepID); err != nil {
		t.Fatalf("block on decision: %v", err)
	}
	if got := stepRepo.listCount.Load(); got != 0 {
		t.Fatalf("step ListByRun count = %d, want 0 when current step id is provided", got)
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(checkpoints) != 1 {
		t.Fatalf("checkpoints = %+v, want one blocked checkpoint", checkpoints)
	}
	if checkpoints[0].CurrentStepID != stepID || checkpoints[0].CheckpointType != common.CheckpointTypeBlocked {
		t.Fatalf("checkpoint = %+v, want blocked checkpoint at %s", checkpoints[0], stepID)
	}
}

func TestRuntimeOrchestrator_CompletedStepEvidenceWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	toolCallRepo := &blockingToolCallRepository{
		inner:   memory.NewToolCallRepository(),
		started: started,
		release: release,
	}
	validationRepo := &blockingValidationResultRepository{
		inner:   memory.NewValidationResultRepository(),
		started: started,
		release: release,
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			ToolCalls:         toolCallRepo,
			ValidationResults: validationRepo,
		},
	}
	workspaceID := common.WorkspaceID("ws-completed-step-evidence-parallel")
	runID := common.RunID("run-completed-step-evidence-parallel")
	stepID := common.StepID("run-completed-step-evidence-parallel/step-1")
	toolCallID := common.ToolCallID("run-completed-step-evidence-parallel/step-1/tool-call")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	step := &runmodel.Step{ID: stepID, RunID: runID, State: common.StepStateCompleted}
	toolCall := &runmodel.ToolCall{
		ID:          toolCallID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		State:       common.ToolCallStateRunning,
	}
	if err := toolCallRepo.Create(ctx, toolCall); err != nil {
		t.Fatalf("seed tool call: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- orch.recordCompletedRuntimeStepEvidence(ctx, run, step, toolCall, map[string]any{"status": "ok"})
	}()

	seen := map[string]bool{}
	for !seen["tool-call-completed-update"] || !seen["validation-passed-create"] {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("evidence writes before release = %+v, want tool call and validation writes started: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("record completed evidence: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("record completed evidence did not finish after release: %v", ctx.Err())
	}
	updatedToolCall, err := toolCallRepo.Get(ctx, toolCallID)
	if err != nil {
		t.Fatalf("get tool call: %v", err)
	}
	if updatedToolCall.State != common.ToolCallStateCompleted {
		t.Fatalf("tool call state = %s, want completed", updatedToolCall.State)
	}
	validations, err := validationRepo.ListByStep(ctx, stepID)
	if err != nil {
		t.Fatalf("list validations: %v", err)
	}
	if len(validations) != 1 || validations[0].State != common.ValidationResultStatePassed || validations[0].EvidenceRef != string(toolCallID) {
		t.Fatalf("validations = %+v, want passed validation with tool call evidence", validations)
	}
}

func TestRuntimeOrchestrator_CompletedStepStateAndCheckpointWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	stepRepo := &blockingCompletedStepRepository{
		inner:   memory.NewStepRepository(),
		started: started,
		release: release,
	}
	checkpointRepo := &blockingRuntimeCheckpointRepository{
		inner:   memory.NewRuntimeCheckpointRepository(),
		started: started,
		release: release,
	}
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			RuntimeCheckpoints: checkpointRepo,
		},
	}
	workspaceID := common.WorkspaceID("ws-completed-step-state-parallel")
	runID := common.RunID("run-completed-step-state-parallel")
	stepID := common.StepID("run-completed-step-state-parallel/step-1")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	step := &runmodel.Step{
		ID:     stepID,
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{"status": "completed"},
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateDispatched,
		Output: map[string]any{"status": "running"},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- orch.persistCompletedStepState(ctx, run, step)
	}()

	seen := map[string]bool{}
	for !seen["step-completed-update"] || !seen["step_completed-checkpoint-create"] {
		select {
		case name := <-started:
			seen[name] = true
		case err := <-done:
			t.Fatalf("persistCompletedStepState returned before both writes started: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for completed step state writes, saw %+v: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("persistCompletedStepState: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("persistCompletedStepState did not finish after release: %v", ctx.Err())
	}
	updatedStep, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if updatedStep.State != common.StepStateCompleted {
		t.Fatalf("step state = %s, want completed", updatedStep.State)
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(checkpoints) != 1 || checkpoints[0].CurrentStepID != stepID || checkpoints[0].Data["event"] != "step_completed" {
		t.Fatalf("checkpoints = %+v, want completed step checkpoint", checkpoints)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchReusesLoadedRun(t *testing.T) {
	ctx := context.Background()
	runRepo := newCountingRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	stepRepo := memory.NewStepRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-start-run-loaded-run-reuse")
	runID := common.RunID("run-start-run-loaded-run-reuse")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if got := runRepo.getCount.Load(); got != 1 {
		t.Fatalf("run Get count = %d, want only initial StartRun read", got)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchReusesLoadedWorkspace(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceRepo := newCountingWorkspaceRepository()
	stepRepo := memory.NewStepRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-start-run-loaded-workspace-reuse")
	runID := common.RunID("run-start-run-loaded-workspace-reuse")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if got := workspaceRepo.getCount.Load(); got != 2 {
		t.Fatalf("workspace Get count = %d, want initial StartRun read plus final active-run reconcile", got)
	}
}

func TestRuntimeOrchestrator_ResumeRunDispatchReusesSyncedWorkspace(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceRepo := newCountingWorkspaceRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	workspaceID := common.WorkspaceID("ws-resume-run-loaded-workspace-reuse")
	runID := common.RunID("run-resume-run-loaded-workspace-reuse")
	blockStepID := common.StepID(string(runID) + "/step-1")
	nextStepID := common.StepID(string(runID) + "/step-2")
	ticketID := common.TicketID(string(blockStepID) + "-decision")
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"completionState": "completed",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "accepted after resume",
			"items": []map[string]any{
				{"item": "resume workspace reused", "state": "satisfied", "evidenceRefs": []string{string(nextStepID)}},
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
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: t.TempDir(),
		State:                common.WorkspaceStateWaitingDecision,
		ActiveRunID:          runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            "dod-resume-run-loaded-workspace-reuse",
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    blockStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateCompleted,
		Output: map[string]any{
			stepOutputKeyNextStep: string(nextStepID),
			stepOutputKeyNextKind: llmcommon.StepKindWorkspaceExecution,
		},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      blockStepID,
		Reason:      "resume",
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-resume-run-loaded-workspace-reuse",
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"answer": "continue"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if got := workspaceRepo.getCount.Load(); got != 2 {
		t.Fatalf("workspace Get count = %d, want resume state sync read plus final active-run reconcile", got)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
}

func TestRuntimeOrchestrator_GraphBatchDispatchReusesLoadedWorkspace(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceRepo := newCountingWorkspaceRepository()
	stepRepo := memory.NewStepRepository()
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch {
		case request.StepID == "run-graph-batch-workspace-reuse/step-1":
			return map[string]any{
				"status": "completed",
				"executionGraph": map[string]any{
					"nodes": []map[string]any{
						{"id": "node-a", "kind": "test/node"},
						{"id": "node-b", "kind": "test/node"},
					},
				},
			}, nil
		case strings.HasSuffix(string(request.StepID), "-node-a"):
			return map[string]any{"status": "completed", "node": "a"}, nil
		case strings.HasSuffix(string(request.StepID), "-node-b"):
			return map[string]any{"status": "completed", "node": "b"}, nil
		default:
			return nil, fmt.Errorf("unexpected step %s", request.StepID)
		}
	}}
	orch := NewRuntimeOrchestrator(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-graph-batch-workspace-reuse")
	runID := common.RunID("run-graph-batch-workspace-reuse")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if got := workspaceRepo.getCount.Load(); got != 2 {
		t.Fatalf("workspace Get count = %d, want graph batch reuse plus final active-run reconcile", got)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("step count = %d, want initial graph step plus two batch nodes", len(steps))
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageIncludesRecentFailureSummary(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-context-package-failure")
	runID := common.RunID("run-context-package-failure")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-context-package-failure",
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateRunning}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	failed := duplicateShellFailureStep(runID, "failed-before-context")
	if err := stepRepo.Create(ctx, failed); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	pkg := orch.runExecutionContextPackage(ctx, run, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-context"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeySoulStage:             soulStageExecution,
			stepInputKeyRuntimeRepairFollowUp: true,
			"failedStepId":                    string(failed.ID),
		},
	}, nil)
	failures := mapSliceFromValue(pkg["recentFailureSummary"])
	if len(failures) != 1 || failures[0]["failureClass"] != failureClassShellCommand || failures[0]["failureSignature"] == "" {
		t.Fatalf("recentFailureSummary = %+v, want shell failure signature", failures)
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageRefreshesCurrentRunHighValueMemory(t *testing.T) {
	ctx := context.Background()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewMemoryRecallTraceRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			MemoryItems:        memoryRepo,
			MemoryRecallTraces: traceRepo,
		},
	}
	workspaceID := common.WorkspaceID("ws-context-package-memory-refresh")
	runID := common.RunID("run-context-package-memory-refresh")
	oldRunID := common.RunID("run-context-package-memory-old")
	for idx := 1; idx <= 4; idx++ {
		if err := memoryRepo.Create(ctx, &memorymodel.MemoryItem{
			ID:          common.MemoryID(fmt.Sprintf("%s/memory/old-summary-%d", oldRunID, idx)),
			WorkspaceID: workspaceID,
			RunID:       oldRunID,
			SourceType:  memorycommon.MemorySourceRun,
			SourceRef:   string(oldRunID),
			MemoryType:  memorycommon.MemoryTypeRunSummary,
			Summary:     fmt.Sprintf("old process summary %d", idx),
			Content:     "old runtime process summary",
			Importance:  90,
			Confidence:  90,
			Visibility:  "workspace",
		}); err != nil {
			t.Fatalf("seed old memory %d: %v", idx, err)
		}
	}
	currentMemories := []*memorymodel.MemoryItem{
		{
			ID:          common.MemoryID(string(runID) + "/memory/dod-feedback"),
			WorkspaceID: workspaceID,
			RunID:       runID,
			SourceType:  memorycommon.MemorySourceRun,
			SourceRef:   string(runID) + "/assessment",
			MemoryType:  memorycommon.MemoryTypeDoDFeedback,
			Summary:     "DoD gaps require tests",
			Content:     "DoD gaps: missing verification evidence.",
			Importance:  50,
			Confidence:  50,
			Visibility:  "workspace",
		},
		{
			ID:          common.MemoryID(string(runID) + "/memory/execution-lesson"),
			WorkspaceID: workspaceID,
			RunID:       runID,
			SourceType:  memorycommon.MemorySourceRun,
			SourceRef:   string(runID) + "/repair",
			MemoryType:  memorycommon.MemoryTypeExecutionLesson,
			Summary:     "Repair must add verification",
			Content:     "Next repair should produce tests and evidence.",
			Importance:  50,
			Confidence:  50,
			Visibility:  "workspace",
		},
	}
	for _, item := range currentMemories {
		if err := memoryRepo.Create(ctx, item); err != nil {
			t.Fatalf("seed current memory %s: %v", item.ID, err)
		}
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		ExecutionContext: map[string]any{
			"memories": []map[string]any{
				{
					"id":         string(oldRunID) + "/memory/activation-only",
					"runId":      string(oldRunID),
					"memoryType": memorycommon.MemoryTypeRunSummary,
					"summary":    "activation old summary",
				},
			},
			"memoryRetrieval": map[string]any{
				"policy": "activation_static",
				"status": "selected",
			},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeySoulStage:             soulStageExecution,
			stepInputKeyRuntimeRepairFollowUp: true,
		},
	}

	orch.enrichLLMStepInputContext(ctx, run, step, nil)

	pkg := mapFromValue(step.Input[stepInputKeyExecutionContextPackage])
	selected := mapFromValue(pkg["selectedContext"])
	memories := mapSliceFromValue(selected["memories"])
	if !memoryContextContains(memories, string(runID)+"/memory/dod-feedback") ||
		!memoryContextContains(memories, string(runID)+"/memory/execution-lesson") {
		t.Fatalf("selected memories = %+v, want current run high-value memories", memories)
	}
	oldRunSummaryCount := 0
	for _, item := range memories {
		if item["runId"] == string(oldRunID) && item["memoryType"] == memorycommon.MemoryTypeRunSummary {
			oldRunSummaryCount++
		}
	}
	if oldRunSummaryCount > 1 {
		t.Fatalf("selected memories = %+v, want at most one old run_summary", memories)
	}
	retrieval := mapFromValue(selected["memoryRetrieval"])
	if retrieval["policy"] != stepInputMemorySelectionPolicy || retrieval["refreshSource"] != "repair" {
		t.Fatalf("memoryRetrieval = %+v, want dynamic repair refresh", retrieval)
	}
	trace, err := traceRepo.Get(ctx, common.MemoryRecallTraceID(string(step.ID)+"/memory-recall/step-input"))
	if err != nil {
		t.Fatalf("get recall trace: %v", err)
	}
	if !memoryIDsContain(trace.RetrievedMemoryIDs, common.MemoryID(string(runID)+"/memory/dod-feedback")) {
		t.Fatalf("trace retrieved ids = %+v, want current dod feedback", trace.RetrievedMemoryIDs)
	}
	if trace.RefreshSource != "repair" ||
		trace.SelectionPolicyVersion != stepInputMemorySelectionVersion ||
		!memoryIDsContain(trace.CandidateMemoryIDs, common.MemoryID(string(runID)+"/memory/dod-feedback")) ||
		!memoryIDsContain(trace.SelectedMemoryIDs, common.MemoryID(string(runID)+"/memory/dod-feedback")) ||
		len(trace.ExcludedMemoryIDs) == 0 ||
		trace.TypeQuota[memorycommon.MemoryTypeRunSummary] != 1 ||
		len(trace.SelectionScores) == 0 {
		t.Fatalf("trace selection fields = %+v, want structured selection evidence", trace)
	}
}

func TestStepInputMemorySelectionDowngradesSupersededRunMemory(t *testing.T) {
	currentRunID := common.RunID("run-memory-selection-current")
	completedRunID := common.RunID("run-memory-selection-completed")
	supersededRunID := common.RunID("run-memory-selection-superseded")
	selected, candidates, _ := selectStepInputMemoryItems([]*memorymodel.MemoryItem{
		{
			ID:         "memory-superseded-summary",
			RunID:      supersededRunID,
			MemoryType: memorycommon.MemoryTypeRunSummary,
			Summary:    "superseded summary",
			Importance: 90,
			Confidence: 90,
			Visibility: "workspace",
		},
		{
			ID:         "memory-completed-summary",
			RunID:      completedRunID,
			MemoryType: memorycommon.MemoryTypeRunSummary,
			Summary:    "completed summary",
			Importance: 90,
			Confidence: 90,
			Visibility: "workspace",
		},
	}, currentRunID, map[common.RunID]common.RunState{
		completedRunID:  common.RunStateCompleted,
		supersededRunID: common.RunStateSuperseded,
	}, stepInputMemoryLimit)

	if len(selected) != 1 || selected[0].RunID != completedRunID {
		t.Fatalf("selected = %+v, want completed run summary over superseded run summary", selected)
	}
	if len(candidates) != 2 || candidates[0].RunID != completedRunID {
		t.Fatalf("candidates = %+v, want completed run ranked before superseded run", candidates)
	}
	reason := stepInputMemoryScoreReason(candidates[1], currentRunID, map[common.RunID]common.RunState{supersededRunID: common.RunStateSuperseded})
	if !strings.Contains(reason, "run_state=superseded") || !strings.Contains(reason, "run_state_downgraded") {
		t.Fatalf("reason = %q, want superseded run downgrade explanation", reason)
	}
}

func TestStepInputMemorySelectionLimitsSingleHistoricalRunDominance(t *testing.T) {
	currentRunID := common.RunID("run-memory-selection-current")
	firstHistoricalRunID := common.RunID("run-memory-selection-history-a")
	secondHistoricalRunID := common.RunID("run-memory-selection-history-b")
	items := []*memorymodel.MemoryItem{
		{
			ID:         "history-a-dod",
			RunID:      firstHistoricalRunID,
			MemoryType: memorycommon.MemoryTypeDoDFeedback,
			Summary:    "history a dod gap",
			Importance: 90,
			Confidence: 90,
			Visibility: "workspace",
		},
		{
			ID:         "history-a-lesson",
			RunID:      firstHistoricalRunID,
			MemoryType: memorycommon.MemoryTypeExecutionLesson,
			Summary:    "history a repair lesson",
			Importance: 90,
			Confidence: 90,
			Visibility: "workspace",
		},
		{
			ID:         "history-a-context",
			RunID:      firstHistoricalRunID,
			MemoryType: memorycommon.MemoryTypeProjectContext,
			Summary:    "history a project context",
			Importance: 90,
			Confidence: 90,
			Visibility: "workspace",
		},
		{
			ID:         "history-a-decision",
			RunID:      firstHistoricalRunID,
			MemoryType: memorycommon.MemoryTypeDecision,
			Summary:    "history a decision",
			Importance: 90,
			Confidence: 90,
			Visibility: "workspace",
		},
		{
			ID:         "history-b-skill",
			RunID:      secondHistoricalRunID,
			MemoryType: memorycommon.MemoryTypeSkillFeedback,
			Summary:    "history b skill context",
			Importance: 80,
			Confidence: 80,
			Visibility: "workspace",
		},
		{
			ID:         "history-b-tool",
			RunID:      secondHistoricalRunID,
			MemoryType: memorycommon.MemoryTypeToolFeedback,
			Summary:    "history b tool context",
			Importance: 80,
			Confidence: 80,
			Visibility: "workspace",
		},
	}

	selected, _, excluded := selectStepInputMemoryItems(items, currentRunID, map[common.RunID]common.RunState{
		firstHistoricalRunID:  common.RunStateCompleted,
		secondHistoricalRunID: common.RunStateCompleted,
	}, stepInputMemoryLimit)

	firstHistoricalSelected := 0
	for _, item := range selected {
		if item.RunID == firstHistoricalRunID {
			firstHistoricalSelected++
		}
	}
	if firstHistoricalSelected > stepInputHistoricalRunMemoryMax {
		t.Fatalf("selected = %+v, want at most %d memories from one historical run", selected, stepInputHistoricalRunMemoryMax)
	}
	if !memoryItemsContainRun(selected, secondHistoricalRunID) {
		t.Fatalf("selected = %+v, want other historical run to remain represented", selected)
	}
	if !memoryItemsContainRun(excluded, firstHistoricalRunID) {
		t.Fatalf("excluded = %+v, want excess first historical run memories excluded", excluded)
	}
}

func TestRuntimeOrchestrator_StepInputMemoryTraceRecordsExcludedProviderRetryMemory(t *testing.T) {
	ctx := context.Background()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewMemoryRecallTraceRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			MemoryItems:        memoryRepo,
			MemoryRecallTraces: traceRepo,
		},
	}
	workspaceID := common.WorkspaceID("ws-provider-retry-memory-trace")
	runID := common.RunID("run-provider-retry-memory-trace")
	memoryID := common.MemoryID(string(runID) + "/provider-retry-001/memory/1")
	if err := memoryRepo.Create(ctx, &memorymodel.MemoryItem{
		ID:          memoryID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		SourceType:  memorycommon.MemorySourceRun,
		SourceRef:   string(runID),
		MemoryType:  memorycommon.MemoryTypeToolFeedback,
		Summary:     "Provider retry outcome captured",
		Content:     "Provider retry attempt: 1\nfailureClass: provider_timeout",
		Tags:        []string{"provider_retry", "tool_context", "necessary_output"},
		Importance:  90,
		Confidence:  90,
		Visibility:  "workspace",
	}); err != nil {
		t.Fatalf("seed provider retry memory: %v", err)
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeySoulStage:             soulStageExecution,
			stepInputKeyRuntimeRepairFollowUp: true,
		},
	}

	orch.enrichLLMStepInputContext(ctx, run, step, nil)

	pkg := mapFromValue(step.Input[stepInputKeyExecutionContextPackage])
	selected := mapFromValue(pkg["selectedContext"])
	if memories := mapSliceFromValue(selected["memories"]); len(memories) != 0 {
		t.Fatalf("selected memories = %+v, want provider retry process memory excluded", memories)
	}
	retrieval := mapFromValue(selected["memoryRetrieval"])
	if retrieval["status"] != "none_selected" || retrieval["refreshSource"] != "repair" {
		t.Fatalf("memoryRetrieval = %+v, want none_selected repair trace", retrieval)
	}
	trace, err := traceRepo.Get(ctx, common.MemoryRecallTraceID(string(step.ID)+"/memory-recall/step-input"))
	if err != nil {
		t.Fatalf("get recall trace: %v", err)
	}
	if !memoryIDsContain(trace.CandidateMemoryIDs, memoryID) ||
		memoryIDsContain(trace.SelectedMemoryIDs, memoryID) ||
		!memoryIDsContain(trace.ExcludedMemoryIDs, memoryID) ||
		trace.SelectionPolicyVersion != stepInputMemorySelectionVersion ||
		len(trace.SelectionScores) == 0 {
		t.Fatalf("trace = %+v, want provider retry memory candidate excluded with scores", trace)
	}
	if !strings.Contains(trace.SelectionScores[0].Reason, "excluded_low_value_process_memory") {
		t.Fatalf("selection score reason = %q, want low-value process exclusion", trace.SelectionScores[0].Reason)
	}
}

func TestRuntimeOrchestrator_StepInputMemoryTraceExcludesTransientDoDFeedback(t *testing.T) {
	ctx := context.Background()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewMemoryRecallTraceRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			MemoryItems:        memoryRepo,
			MemoryRecallTraces: traceRepo,
		},
	}
	workspaceID := common.WorkspaceID("ws-transient-dod-memory-trace")
	runID := common.RunID("run-transient-dod-memory-trace")
	memoryID := common.MemoryID(string(runID) + "/provider-retry-001/memory/1")
	if err := memoryRepo.Create(ctx, &memorymodel.MemoryItem{
		ID:          memoryID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		SourceType:  memorycommon.MemorySourceRun,
		SourceRef:   string(runID),
		MemoryType:  memorycommon.MemoryTypeDoDFeedback,
		Summary:     "因 LLM 提供者超时（context deadline exceeded），无法完成本次工作区执行规划。",
		Content:     "Provider retry attempt 1 failed.\nDoD gaps:\n- [skipped] 功能准则",
		Tags:        []string{"dod_feedback", "repair_loop"},
		Importance:  90,
		Confidence:  90,
		Visibility:  "workspace",
	}); err != nil {
		t.Fatalf("seed transient dod memory: %v", err)
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeySoulStage:             soulStageExecution,
			stepInputKeyRuntimeRepairFollowUp: true,
		},
	}

	orch.enrichLLMStepInputContext(ctx, run, step, nil)

	pkg := mapFromValue(step.Input[stepInputKeyExecutionContextPackage])
	selected := mapFromValue(pkg["selectedContext"])
	if memories := mapSliceFromValue(selected["memories"]); len(memories) != 0 {
		t.Fatalf("selected memories = %+v, want transient dod feedback excluded", memories)
	}
	trace, err := traceRepo.Get(ctx, common.MemoryRecallTraceID(string(step.ID)+"/memory-recall/step-input"))
	if err != nil {
		t.Fatalf("get recall trace: %v", err)
	}
	if !memoryIDsContain(trace.CandidateMemoryIDs, memoryID) ||
		memoryIDsContain(trace.SelectedMemoryIDs, memoryID) ||
		!memoryIDsContain(trace.ExcludedMemoryIDs, memoryID) ||
		len(trace.SelectionScores) == 0 {
		t.Fatalf("trace = %+v, want transient dod feedback candidate excluded with scores", trace)
	}
	if !strings.Contains(trace.SelectionScores[0].Reason, "excluded_low_value_process_memory") {
		t.Fatalf("selection score reason = %q, want low-value process exclusion", trace.SelectionScores[0].Reason)
	}
}

func TestRuntimeOrchestrator_StepInputMemoryTraceExcludesRepairContractDoDFeedback(t *testing.T) {
	ctx := context.Background()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewMemoryRecallTraceRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			MemoryItems:        memoryRepo,
			MemoryRecallTraces: traceRepo,
		},
	}
	workspaceID := common.WorkspaceID("ws-repair-contract-dod-memory-trace")
	runID := common.RunID("run-repair-contract-dod-memory-trace")
	historicalRunID := common.RunID("run-repair-contract-dod-memory-old")
	memoryID := common.MemoryID(string(historicalRunID) + "/repair/memory/1")
	if err := memoryRepo.Create(ctx, &memorymodel.MemoryItem{
		ID:          memoryID,
		WorkspaceID: workspaceID,
		RunID:       historicalRunID,
		SourceType:  memorycommon.MemorySourceRun,
		SourceRef:   string(historicalRunID),
		MemoryType:  memorycommon.MemoryTypeDoDFeedback,
		Summary:     "构建修复失败：连续多次补丁尝试因 coverageTargets 缺失或 schema 错误被拒绝",
		Content:     "DoD gaps:\n- [failed] target alpha\ninvalid_unified_diff\n非实质性变更",
		Tags:        []string{"dod_feedback", "repair_loop", "reference_value"},
		Importance:  90,
		Confidence:  90,
		Visibility:  "workspace",
	}); err != nil {
		t.Fatalf("seed repair contract memory: %v", err)
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRuntimeRepairFollowUp: true,
			"stateChangingOutputRequired":     true,
		},
	}

	orch.enrichLLMStepInputContext(ctx, run, step, nil)

	pkg := mapFromValue(step.Input[stepInputKeyExecutionContextPackage])
	selected := mapFromValue(pkg["selectedContext"])
	if memories := mapSliceFromValue(selected["memories"]); len(memories) != 0 {
		t.Fatalf("selected memories = %+v, want repair contract DoD feedback excluded", memories)
	}
	trace, err := traceRepo.Get(ctx, common.MemoryRecallTraceID(string(step.ID)+"/memory-recall/step-input"))
	if err != nil {
		t.Fatalf("get recall trace: %v", err)
	}
	if !memoryIDsContain(trace.CandidateMemoryIDs, memoryID) ||
		memoryIDsContain(trace.SelectedMemoryIDs, memoryID) ||
		!memoryIDsContain(trace.ExcludedMemoryIDs, memoryID) ||
		len(trace.SelectionScores) == 0 {
		t.Fatalf("trace = %+v, want repair contract memory candidate excluded with scores", trace)
	}
	if !strings.Contains(trace.SelectionScores[0].Reason, "excluded_low_value_process_memory") {
		t.Fatalf("selection score reason = %q, want low-value process exclusion", trace.SelectionScores[0].Reason)
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageSkipsInitialStepFailureHistoryRead(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := newCountingStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-context-package-initial")
	runID := common.RunID("run-context-package-initial")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-context-package-initial",
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateRunning}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	pkg := orch.runExecutionContextPackage(ctx, run, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{stepInputKeySoulStage: soulStagePlanning},
	}, nil)
	if got := stepRepo.listCount.Load(); got != 0 {
		t.Fatalf("step ListByRun count = %d, want no failure-history read for initial step", got)
	}
	if failures := mapSliceFromValue(pkg["recentFailureSummary"]); len(failures) != 0 {
		t.Fatalf("recentFailureSummary = %+v, want empty for initial step", failures)
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageSkipsFailureHistoryForNormalFollowUp(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := newCountingStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-context-package-normal-follow-up")
	runID := common.RunID("run-context-package-normal-follow-up")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-context-package-normal-follow-up",
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateRunning}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, duplicateShellFailureStep(runID, "failed-before-normal-follow-up")); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	pkg := orch.runExecutionContextPackage(ctx, run, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/llm-after-decision"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{stepInputKeySoulStage: soulStageDecision},
	}, nil)
	if got := stepRepo.listCount.Load(); got != 0 {
		t.Fatalf("step ListByRun count = %d, want no failure-history read for normal follow-up", got)
	}
	if failures := mapSliceFromValue(pkg["recentFailureSummary"]); len(failures) != 0 {
		t.Fatalf("recentFailureSummary = %+v, want empty for normal follow-up", failures)
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageBuildsIndependentContextReadsConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	started := make(chan string, 2)
	release := make(chan struct{})
	resultRepo := &blockingResultRepository{inner: memory.NewResultRepository(), started: started, release: release}
	artifactRepo := &blockingArtifactRepository{inner: memory.NewArtifactRepository(), started: started, release: release}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
	}
	run := &runmodel.Run{
		ID:          "run-context-package-concurrent",
		WorkspaceID: "ws-context-package-concurrent",
		State:       common.RunStateRunning,
	}
	step := &runmodel.Step{
		ID:    "run-context-package-concurrent/step-1",
		RunID: run.ID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}

	done := make(chan map[string]any, 1)
	go func() {
		done <- orch.runExecutionContextPackage(ctx, run, step, &workspacemodel.Workspace{ID: run.WorkspaceID, DeliverableDirectory: t.TempDir()})
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("context package reads did not start concurrently before release: seen=%v err=%v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case pkg := <-done:
		if pkg["version"] != executionContextPackageVersion {
			t.Fatalf("executionContextPackage = %+v, want version %s", pkg, executionContextPackageVersion)
		}
	case <-ctx.Done():
		t.Fatalf("context package did not finish after release: %v", ctx.Err())
	}
}
