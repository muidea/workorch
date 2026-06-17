package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDecisionWithQuestionReferencingReadableWorkspaceFileShouldRepair(t *testing.T) {
	spec := runtimeDecisionBlockSpec{
		DecisionKind: "provide_missing_info",
		Question:     "请提供 handler/handler.go 和 config/config.go 的完整内容。",
		Reason:       "测试文件引用未定义符号，需要读取实际源文件后修复。",
		Context: map[string]any{
			"missingFields":       []any{"actualExportedSymbols"},
			"requiresHumanFields": []any{"actualExportedSymbols"},
			"readableResources":   []any{"handler/handler.go", "config/config.go"},
		},
	}

	if !knownReadableWorkspaceResourceDecisionSpec(spec) {
		t.Fatalf("knownReadableWorkspaceResourceDecisionSpec = false, want true for question-referenced workspace files")
	}
	if !continuableDecisionShouldRepair(&runmodel.Step{ID: "run-1/step-1"}, spec) {
		t.Fatalf("continuableDecisionShouldRepair = false, want true for readable workspace resources")
	}
}

func TestApplyWorkspaceUnifiedDiffReportsContextMismatch(t *testing.T) {
	deliverableDirectory := t.TempDir()
	targetPath := filepath.Join(deliverableDirectory, "main_test.go")
	if err := os.WriteFile(targetPath, []byte("package main\n\nfunc TestAuth() {\n\tif auth != \"Bearer [REDACTED]\" {\n\t\tpanic(\"bad auth\")\n\t}\n}\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	err := applyWorkspaceUnifiedDiff(deliverableDirectory, "main_test.go", "--- a/main_test.go\n+++ b/main_test.go\n@@ -34,7 +34,7 @@\n \tauth := r.Header.Get(\"Authorization\")\n-\tif auth != \"Bearer [REDACTED]\" {\n+\tif auth != \"Bearer test-key\" {\n \t\thttp.Error(w, \"Unauthorized\", http.StatusUnauthorized)\n", "")
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "unified diff hunk") {
		t.Fatalf("applyWorkspaceUnifiedDiff err = %v, want contextual unified diff error", err)
	}
}

func TestNextGraphStepInputRendersDependencyOutputTemplates(t *testing.T) {
	rootStepID := common.StepID("run-render/step-1")
	completedStep := &runmodel.Step{
		ID:    "run-render/block/get-weather",
		RunID: "run-render",
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: string(rootStepID),
			stepInputKeyGraphNodeID:     "get_weather",
		},
		Output: map[string]any{
			"stdout": "Clear +21C",
			"nested": map[string]any{
				"value": "ok",
			},
		},
	}
	input := nextGraphStepInput(
		&runmodel.Run{ID: "run-render"},
		&runmodel.Step{ID: rootStepID},
		runtimemodel.BlockExecutionGraph{},
		runtimemodel.BlockExecutionNode{
			ID:   "write_report",
			Kind: llmcommon.RuntimeBlockKindPatchApply,
			Input: map[string]any{
				"patchType": "file_write",
				"targetRef": "docs/weather.md",
				"content":   "Weather: {{ get_weather.stdout }} / {{ get_weather.nested.value }}",
			},
		},
		map[string]bool{"get_weather": true},
		"",
		[]*runmodel.Step{completedStep},
		rootStepID,
	)
	if input["content"] != "Weather: Clear +21C / ok" {
		t.Fatalf("content = %q, want rendered dependency output", input["content"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesPatchConflictDiagnosticsAndWorkspaceManifest(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	patchRepo := memory.NewPatchRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	toolCallRepo := memory.NewToolCallRepository()
	var repairInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch {
		case request.StepID == "run-patch-conflict-repair/step-1":
			return map[string]any{
				"status": "completed",
				"patches": []map[string]any{
					{
						"patchType": "file_write",
						"targetRef": "llm-proxy/config.yaml",
						"content":   "config",
						"summary":   "write nested config",
					},
				},
			}, nil
		case strings.Contains(string(request.StepID), "/repair-"):
			repairInput = request.Input
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "repair received path diagnostics",
					"items": []map[string]any{
						{"item": "diagnostics available", "state": "satisfied", "evidenceRefs": []string{string(request.StepID)}},
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
			ToolCalls:          toolCallRepo,
			Patches:            patchRepo,
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
			MemoryItems:        memory.NewMemoryItemRepository(),
		},
		events.NewMemoryHub(),
	)

	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "llm-proxy"), []byte("binary"), 0o755); err != nil {
		t.Fatalf("seed executable file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-patch-conflict-repair")
	runID := common.RunID("run-patch-conflict-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-patch-conflict-repair"),
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if repairInput == nil {
		t.Fatalf("repair input was not captured")
	}
	failedOutput, ok := repairInput["failedStepOutput"].(map[string]any)
	if !ok {
		t.Fatalf("failedStepOutput = %#v, want map", repairInput["failedStepOutput"])
	}
	diagnostics, ok := failedOutput["runtimeDiagnostics"].(map[string]any)
	if !ok || diagnostics["kind"] != "workspace_path_conflict" || diagnostics["conflictRef"] != "llm-proxy" {
		t.Fatalf("runtimeDiagnostics = %#v, want workspace path conflict", failedOutput["runtimeDiagnostics"])
	}
	if failedOutput["failureClass"] != failureClassWorkspacePath || !strings.Contains(fmt.Sprint(failedOutput["failureSignature"]), "conflictRef=llm-proxy") {
		t.Fatalf("failure classification = class:%#v signature:%#v, want workspace path conflict signature", failedOutput["failureClass"], failedOutput["failureSignature"])
	}
	manifest, ok := repairInput[stepInputKeyWorkspaceFileManifest].([]map[string]any)
	if !ok || len(manifest) != 1 || manifest[0]["path"] != "llm-proxy" || manifest[0]["executable"] != true {
		t.Fatalf("workspaceFileManifest = %#v, want executable llm-proxy file", repairInput[stepInputKeyWorkspaceFileManifest])
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if !hasCheckpointFailureClass(checkpoints, failureClassWorkspacePath) {
		t.Fatalf("checkpoints = %+v, want workspace path failure class checkpoint", checkpoints)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputSkipsWorkspaceManifestForDoDFailure(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "README.md"), []byte("workspace"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-dod-repair-no-manifest")
	runID := common.RunID("run-dod-repair-no-manifest")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			"status": "completed",
			"dodAssessment": map[string]any{
				"satisfied": false,
				"summary":   "needs more evidence",
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingDoDItems": []map[string]any{
					{
						"raw":                  "配置文件必须被代码真实加载并有测试断言",
						"normalized":           "配置文件必须被代码真实加载并有测试断言",
						"kind":                 "leaf",
						"requiredEvidenceHint": "Provide concrete evidence refs for this DoD item according to the active runtime stage and profile contract.",
					},
				},
			},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, common.ErrInvalidArgument)

	if manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest]); len(manifest) != 0 {
		t.Fatalf("workspaceFileManifest = %+v, want no manifest for non-path DoD repair", input[stepInputKeyWorkspaceFileManifest])
	}
	if _, ok := mapFromValue(input["runtimeDiagnostics"])["missingDoDItems"]; !ok {
		t.Fatalf("runtimeDiagnostics = %#v, want missingDoDItems diagnostics", input["runtimeDiagnostics"])
	}
	if missing := mapSliceFromValue(input["missingDoDItems"]); len(missing) != 1 || missing[0]["kind"] != "leaf" {
		t.Fatalf("missingDoDItems = %#v, want one leaf item", input["missingDoDItems"])
	}
}

func TestOutputWithDoDAssessmentDiagnosticsNormalizesMissingItems(t *testing.T) {
	output := outputWithDoDAssessmentDiagnostics(map[string]any{
		"status": "completed",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "one item missing",
			"items": []map[string]any{
				{
					"item":         "first item",
					"state":        "satisfied",
					"evidenceRefs": []string{"run/step-1"},
				},
				{
					"item":         "second item",
					"state":        "failed",
					"reason":       "missing implementation evidence",
					"impact":       "cannot accept the run",
					"evidenceRefs": []string{"run/step-2"},
					"nextActions":  []string{"implement the missing behavior", "rerun verification"},
				},
			},
		},
	})

	diagnostics := mapFromValue(output["runtimeDiagnostics"])
	if diagnostics["kind"] != failureClassDoDCoverageMissing {
		t.Fatalf("runtimeDiagnostics = %+v, want DoD coverage missing kind", diagnostics)
	}
	missing := mapSliceFromValue(output["missingDoDItems"])
	if len(missing) != 1 || missing[0]["item"] != "second item" || missing[0]["state"] != "failed" || missing[0]["source"] != "dodAssessment" {
		t.Fatalf("missingDoDItems = %+v, want normalized failed DoD item", missing)
	}
	if refs := stringSliceFromValue(missing[0]["evidenceRefs"]); len(refs) != 1 || refs[0] != "run/step-2" {
		t.Fatalf("missingDoDItems evidenceRefs = %+v, want run/step-2", missing[0]["evidenceRefs"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesPatchTargetSnapshot(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	source := "func sample(arg *Type) {\n\t_ = handler(func(v *Value) {})\n}\n"
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "source.txt"), []byte(source), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-patch-repair-context")
	runID := common.RunID("run-patch-repair-context")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/block/apply"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "source.txt",
			"diff":      "--- a/source.txt\n+++ b/source.txt\n@@\n-func sample(arg Type)\n+func sample(arg *Type)",
		},
		Output: map[string]any{
			"status":                  "completed",
			"stage":                   "patch_apply",
			"error":                   "invalid argument",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: "class=patch_apply_failed|targetRef=source.txt",
			"patches": []map[string]any{
				{"targetRef": "source.txt"},
			},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, common.ErrInvalidArgument)

	patchContext := mapSliceFromValue(input["patchRepairContext"])
	if len(patchContext) != 1 {
		t.Fatalf("patchRepairContext = %#v, want one snapshot", input["patchRepairContext"])
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change for patch hunk mismatch", contract)
	}
	item := patchContext[0]
	if item["targetRef"] != "source.txt" || item["exists"] != true || item["source"] != "current_workspace_file" {
		t.Fatalf("patchRepairContext item = %#v, want current source.txt snapshot", item)
	}
	content, _ := item["content"].(string)
	if !strings.Contains(content, "*Type") || !strings.Contains(content, "*Value") {
		t.Fatalf("patchRepairContext content = %q, want exact current source operators", content)
	}
	if strings.TrimSpace(stringFromMap(item, "contentHash")) == "" {
		t.Fatalf("patchRepairContext contentHash missing: %#v", item)
	}
	if item["contentComplete"] != true || item["safeForFileWrite"] != true {
		t.Fatalf("patchRepairContext safety = %#v, want complete safe snapshot", item)
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	for _, want := range []string{"patchRepairContext", "failed patch text as stale", "safeForFileWrite", "minimal unified_diff", "runtime redaction marker tokens", "synchronizes modified file content", "input.content"} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("repairGuidance = %q, want %q", guidance, want)
		}
	}

	failedPreviewStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/block/preview"),
		RunID: runID,
		Kind:  "patch/preview",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "source.txt",
			"diff":      "--- a/source.txt\n+++ b/source.txt\n@@\n-func sample(arg Type)\n+func sample(arg *Type)",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "patch_preview",
			"error":                   "conflict",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=patch/preview|targetRef=source.txt",
		},
	}

	previewInput := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedPreviewStep, 1, common.ErrConflict)
	previewPatchContext := mapSliceFromValue(previewInput["patchRepairContext"])
	if len(previewPatchContext) != 1 {
		t.Fatalf("patchRepairContext for preview = %#v, want one snapshot", previewInput["patchRepairContext"])
	}
	previewContract := mapFromValue(previewInput["convergenceContract"])
	if stringFromMap(previewContract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract for preview = %+v, want need_state_change", previewContract)
	}
	if previewPatchContext[0]["targetRef"] != "source.txt" {
		t.Fatalf("patchRepairContext for preview = %#v, want source.txt target", previewPatchContext[0])
	}

	failedEditStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/block/edit"),
		RunID: runID,
		Kind:  "file/edit",
		State: common.StepStateFailed,
		Input: map[string]any{
			"path":    "source.txt",
			"oldText": "func sample(arg Type)",
			"newText": "func sample(arg *Type)",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "file_edit_conflict",
			"error":                   "conflict: file/edit oldText not found",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=file/edit|path=source.txt",
		},
	}
	editInput := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedEditStep, 1, common.ErrConflict)
	editPatchContext := mapSliceFromValue(editInput["patchRepairContext"])
	if len(editPatchContext) != 1 || editPatchContext[0]["targetRef"] != "source.txt" {
		t.Fatalf("patchRepairContext for file/edit conflict = %#v, want source.txt snapshot", editInput["patchRepairContext"])
	}
	editContract := mapFromValue(editInput["convergenceContract"])
	if stringFromMap(editContract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract for file/edit conflict = %+v, want need_state_change", editContract)
	}
	editGuidance := strings.Join(stringSliceFromValue(editInput["repairGuidance"]), "\n")
	if !strings.Contains(editGuidance, "file/edit oldText conflict") || !strings.Contains(editGuidance, "current target snapshot") {
		t.Fatalf("repairGuidance for file/edit conflict = %q, want stale oldText guidance", editGuidance)
	}
}

func TestRuntimeOrchestrator_PatchApplyDuplicateFailureContinuesMechanicalRepair(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	runID := common.RunID("run-patch-duplicate-repair")
	signature := "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|targetRef=source.txt|error=context mismatch"
	current := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-002/001-apply"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "source.txt",
			"patchType": "unified_diff",
		},
		Output: map[string]any{
			"stage":                   "patch_apply",
			"error":                   "invalid argument: unified diff hunk context mismatch",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: signature,
		},
	}
	prior := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/001-apply"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{"targetRef": "source.txt"},
		Output: map[string]any{
			"stage":                   "patch_apply",
			"error":                   "invalid argument: unified diff hunk context mismatch",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: signature,
		},
	}
	repairAttempt := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-graph-001-001-apply"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfFailureClass:  failureClassPatchApply,
			stepInputKeyRepairOfFailureSig:    signature,
			"failureTargetScope":              "targetRef=source.txt",
		},
		Output: map[string]any{"status": "completed"},
	}
	for _, step := range []*runmodel.Step{prior, repairAttempt, current} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	orch := &RuntimeOrchestrator{
		steps:   stepRepo,
		tickets: ticketRepo,
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: common.WorkspaceID("ws-patch-duplicate-repair"),
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	if shouldBlockOnDuplicateRuntimeFailureForTest(ctx, orch, run, current) {
		t.Fatalf("shouldBlockOnDuplicateRuntimeFailure = true, want false for budgeted patch/apply target repair")
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesPatchRepairSuggestions(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	source := "alpha\nold value\nomega\n"
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "source.txt"), []byte(source), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-patch-repair-suggestion")
	runID := common.RunID("run-patch-repair-suggestion")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/block/apply"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "source.txt",
			"patchType": "unified_diff",
			"diff":      "--- a/source.txt\n+++ b/source.txt\n@@ -10,3 +10,3 @@\n-old value\n+new value\n",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "patch_apply",
			"error":                   "invalid argument: unified diff hunk context mismatch",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: "class=patch_apply_failed|targetRef=source.txt",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, common.ErrInvalidArgument)

	suggestions := mapSliceFromValue(input["patchRepairSuggestions"])
	if len(suggestions) != 1 {
		t.Fatalf("patchRepairSuggestions = %#v, want one suggestion", input["patchRepairSuggestions"])
	}
	if suggestions[0]["targetRef"] != "source.txt" || suggestions[0]["oldText"] != "old value" || suggestions[0]["newText"] != "new value" || suggestions[0]["safeForMinimal"] != true {
		t.Fatalf("patchRepairSuggestions[0] = %#v, want exact old/new replacement", suggestions[0])
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "patchRepairSuggestions") || !strings.Contains(guidance, "smallest current unified_diff") {
		t.Fatalf("repairGuidance = %q, want patch repair suggestion guidance", guidance)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputDetectsAlreadySatisfiedFailedPatch(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	source := "package main\n\nimport (\n\t\"fmt\"\n\t\"strconv\"\n)\n"
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-patch-repair-already-satisfied")
	runID := common.RunID("run-patch-repair-already-satisfied")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-027/002-apply-import-fix"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageExecution,
			"targetRef":              "main.go",
			"patchType":              "unified_diff",
			"diff":                   "--- a/main.go\n+++ b/main.go\n@@ -2,5 +2,6 @@\n \n import (\n \t\"fmt\"\n+\t\"strconv\"\n )\n",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "patch_apply",
			"error":                   "invalid argument: unified diff hunk context mismatch",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|targetRef=main.go|error=context mismatch",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}, failedStep, 1, common.ErrInvalidArgument)

	signals := mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])
	if len(signals) != 1 {
		t.Fatalf("currentTargetAlreadySatisfiesFailedPatch = %#v, want one signal", input["currentTargetAlreadySatisfiesFailedPatch"])
	}
	if signals[0]["targetRef"] != "main.go" || signals[0]["safeForValidation"] != true || signals[0]["businessJudgment"] != false {
		t.Fatalf("already satisfied signal = %#v, want neutral validation signal", signals[0])
	}
	if input["stateChangeAlreadySatisfiedByCurrentTarget"] != true {
		t.Fatalf("stateChangeAlreadySatisfiedByCurrentTarget = %#v, want true", input["stateChangeAlreadySatisfiedByCurrentTarget"])
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want need_validation for already-satisfied failed patch", contract)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != false || required["mustProduceConcreteValidationEvidence"] != true {
		t.Fatalf("repairActionPackage.requiredOutput = %#v, want validation evidence package", required)
	}
	if required["mustPreferExecutableBehaviorValidation"] != true ||
		required["staticInspectionOnlyMayNotSatisfyRuntimeBehavior"] != true ||
		required["mustAttemptControlledSubstituteWhenPrerequisiteUnavailable"] != true {
		t.Fatalf("repairActionPackage.requiredOutput = %#v, want executable controlled validation contract", required)
	}
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "shell/command") {
		t.Fatalf("repairActionPackage.requiredOutput.acceptedKinds = %#v, want shell/command", required["acceptedKinds"])
	}
	if !stringSliceContains(stringSliceFromValue(actionPackage["forbiddenActions"]), "repeat_same_failed_patch") {
		t.Fatalf("repairActionPackage.forbiddenActions = %#v, want repeat_same_failed_patch", actionPackage["forbiddenActions"])
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "currentTargetAlreadySatisfiesFailedPatch") || !strings.Contains(guidance, "bounded concrete validation evidence graph") {
		t.Fatalf("repairGuidance = %q, want already-satisfied validation guidance", guidance)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputDetectsAlreadySatisfiedTextReplacePatch(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	source := "alpha\nnew value\nomega\n"
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "source.txt"), []byte(source), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-text-replace-already-satisfied")
	runID := common.RunID("run-text-replace-already-satisfied")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-003/001-apply-replacement"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "source.txt",
			"patchType": "text_replace",
			"oldText":   "old value",
			"newText":   "new value",
		},
		Output: map[string]any{
			"status":                  "completed",
			"stage":                   "patch_apply",
			"error":                   "conflict: patch text_replace oldText not found",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|targetRef=source.txt|error=oldText not found",
			"patches": []map[string]any{{
				"targetRef": "source.txt",
				"patchType": "text_replace",
				"oldText":   "old value",
				"newText":   "new value",
			}},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}, failedStep, 1, common.ErrConflict)

	signals := mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])
	if len(signals) != 1 {
		t.Fatalf("currentTargetAlreadySatisfiesFailedPatch = %#v, want one text_replace signal", input["currentTargetAlreadySatisfiesFailedPatch"])
	}
	if signals[0]["targetRef"] != "source.txt" || signals[0]["patchType"] != "text_replace" || signals[0]["safeForValidation"] != true {
		t.Fatalf("already satisfied text_replace signal = %#v, want neutral validation signal", signals[0])
	}
	if input["stateChangeAlreadySatisfiedByCurrentTarget"] != true {
		t.Fatalf("stateChangeAlreadySatisfiedByCurrentTarget = %#v, want true", input["stateChangeAlreadySatisfiedByCurrentTarget"])
	}
	if input["stateChangingOutputRequired"] == true {
		t.Fatalf("stateChangingOutputRequired = true, want validation path for already-satisfied patch")
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want need_validation for already-satisfied text_replace patch", contract)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != false || required["mustProduceConcreteValidationEvidence"] != true {
		t.Fatalf("repairActionPackage.requiredOutput = %#v, want validation evidence package", required)
	}
	if !stringSliceContains(stringSliceFromValue(actionPackage["forbiddenActions"]), "repeat_same_failed_patch") {
		t.Fatalf("repairActionPackage.forbiddenActions = %#v, want repeat_same_failed_patch", actionPackage["forbiddenActions"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesLineRemovalPatchSuggestion(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	source := "package main\n\nimport (\n\t\"encoding/json\"\n\t\"io\"\n\t\"net/http\"\n)\n"
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "gateway_test.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-patch-repair-line-removal")
	runID := common.RunID("run-patch-repair-line-removal")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-006/001-fix-import"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "gateway_test.go",
		},
		Output: map[string]any{
			"status":                  "completed",
			"stage":                   "patch_apply",
			"error":                   "invalid argument: unified diff hunk context mismatch",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|node=fix-import",
			"patches": []map[string]any{{
				"targetRef": "gateway_test.go",
				"patchType": "unified_diff",
				"diff":      "--- a/gateway_test.go\n+++ b/gateway_test.go\n@@ -2,8 +2,7 @@\n import (\n \t\"encoding/json\"\n-\t\"io\"\n \t\"net/http\"\n )",
			}},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, common.ErrInvalidArgument)

	suggestions := mapSliceFromValue(input["patchRepairSuggestions"])
	found := false
	for _, suggestion := range suggestions {
		if suggestion["targetRef"] == "gateway_test.go" &&
			suggestion["oldText"] == "\t\"io\"" &&
			suggestion["newText"] == "" &&
			suggestion["safeForMinimal"] == true {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("patchRepairSuggestions = %#v, want exact import line removal suggestion", input["patchRepairSuggestions"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesMalformedDiffAndMissingNameGuidance(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "main.go"), []byte("package main\n\ntype Gateway struct{}\n"), 0o644); err != nil {
		t.Fatalf("seed main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "main_test.go"), []byte("package main\n\nfunc makeApp() *App { return nil }\n"), 0o644); err != nil {
		t.Fatalf("seed main_test.go: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-repair-guidance-symbols")
	runID := common.RunID("run-repair-guidance-symbols")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-006/002-run-go-test-after-patch"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "patch_apply",
			"stderr":                  "./main_test.go:16:7: undefined: NewGateway\n./main_test.go:18:4: unknown field ListenPort in struct literal of type Config\n",
			"error":                   "invalid argument: patch diff contains redacted runtime markers",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|failureTargets=main_test.go|error=invalid argument: patch diff contains redacted runtime markers",
			"failedStepOutput": map[string]any{
				"stage": "patch_apply",
				"error": "invalid argument: patch diff contains redacted runtime markers",
				"patches": []map[string]any{{
					"patchType": "unified_diff",
					"targetRef": "main_test.go",
					"diff":      "--- a/main_test.go\n+++ b/main_test.go\n@@\n-func makeApp() *App { return nil }\n+func makeApp() *Gateway { return [REDACTED] }\n",
				}},
			},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, common.ErrInvalidArgument)
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	for _, want := range []string{
		"discard that diff entirely",
		"explicit line ranges such as @@ -12,3 +12,4 @@",
		"do not invent new names",
		"patch against names that already exist",
	} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("repairGuidance = %q, want %q", guidance, want)
		}
	}
}

func TestPatchRepairTargetSnapshotMarksRedactedOrTruncatedContentUnsafeForFileWrite(t *testing.T) {
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "secret.txt"), []byte("token [REDACTED]\n"), 0o644); err != nil {
		t.Fatalf("seed redacted file: %v", err)
	}
	largeSafe := strings.Repeat("safe-current-line\n", 900)
	if len(largeSafe) <= maxLLMContextStringBytes {
		t.Fatalf("test fixture too small")
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "large-safe.txt"), []byte(largeSafe), 0o644); err != nil {
		t.Fatalf("seed large safe file: %v", err)
	}
	large := strings.Repeat("x", maxPatchRepairSnapshotBytes+1)
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "large.txt"), []byte(large), 0o644); err != nil {
		t.Fatalf("seed large file: %v", err)
	}

	redacted := patchRepairTargetSnapshot(deliverableDirectory, "secret.txt")
	if redacted["redacted"] != true || redacted["safeForFileWrite"] != false || redacted["contentComplete"] != true {
		t.Fatalf("redacted snapshot = %#v, want unsafe redacted complete snapshot", redacted)
	}
	safe := patchRepairTargetSnapshot(deliverableDirectory, "large-safe.txt")
	if safe["truncated"] == true || safe["contentComplete"] != true || safe["safeForFileWrite"] != true {
		t.Fatalf("large safe snapshot = %#v, want complete safe snapshot", safe)
	}
	if got, want := stringFromMap(safe, "content"), strings.TrimSpace(largeSafe); got != want {
		t.Fatalf("large safe content length = %d, want full trimmed length %d", len(got), len(want))
	}
	truncated := patchRepairTargetSnapshot(deliverableDirectory, "large.txt")
	if truncated["truncated"] != true || truncated["contentComplete"] != false || truncated["safeForFileWrite"] != false {
		t.Fatalf("truncated snapshot = %#v, want unsafe truncated snapshot", truncated)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesFailureTargetSnapshot(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	source := "package main\n\nimport (\n\t\"fmt\"\n\t\"unused/pkg\"\n)\n"
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "main_test.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("seed failure target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "support.txt"), []byte("related resource"), 0o644); err != nil {
		t.Fatalf("seed related resource: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-failure-target-repair-context")
	runID := common.RunID("run-failure-target-repair-context")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/004-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "exit status 1",
			"stderr":                  "./main_test.go:5:2: \"unused/pkg\" imported and not used\n",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, errors.New("exit status 1"))

	targetContext := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(targetContext) != 1 {
		t.Fatalf("failureTargetRepairContext = %#v, want one target snapshot", input["failureTargetRepairContext"])
	}
	item := targetContext[0]
	if item["targetRef"] != "main_test.go" || item["exists"] != true || item["source"] != "failure_target_current_workspace_file" {
		t.Fatalf("failureTargetRepairContext item = %#v, want current main_test.go target", item)
	}
	if content := stringFromMap(item, "content"); !strings.Contains(content, "unused/pkg") {
		t.Fatalf("failureTargetRepairContext content = %q, want current failing target content", content)
	}
	manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) != 2 || manifest[0]["path"] != "main_test.go" || manifest[1]["path"] != "support.txt" {
		t.Fatalf("workspaceFileManifest = %#v, want known workspace resources for target repair", input[stepInputKeyWorkspaceFileManifest])
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "failureTargetRepairContext") || !strings.Contains(guidance, "patch/apply") || !strings.Contains(guidance, "workspaceFileManifest") {
		t.Fatalf("repairGuidance = %q, want target snapshot patch guidance", guidance)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	if len(actionPackage) == 0 {
		t.Fatalf("repairActionPackage = %#v, want actionable package", input["repairActionPackage"])
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true || !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("repairActionPackage.requiredOutput = %#v, want patch/apply state-changing contract", required)
	}
	if required["mustTargetFailureScope"] != true || required["targetRelation"] != "failure_target_repair_snapshot_or_declared_related_failure_target" {
		t.Fatalf("repairActionPackage.requiredOutput = %#v, want target-relevant repair contract", required)
	}
	targets := mapSliceFromValue(actionPackage["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "main_test.go" || strings.TrimSpace(stringFromMap(targets[0], "contentHash")) == "" {
		t.Fatalf("repairActionPackage.targetSnapshots = %#v, want current target snapshot", actionPackage["targetSnapshots"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesRootCauseCandidatesForResolvedFailureSurface(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(deliverableDirectory, "gateway"), 0o755); err != nil {
		t.Fatalf("seed gateway dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "gateway", "main.go"), []byte("package main\n\nfunc proxyPath() string { return \"/chat/completions\" }\n"), 0o644); err != nil {
		t.Fatalf("seed implementation file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "gateway", "main_test.go"), []byte("package main\n\nfunc TestProxyPath(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatalf("seed failure surface file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-root-cause-candidate")
	runID := common.RunID("run-root-cause-candidate")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/006-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"status": "failed",
			"error":  "exit status 1",
			"stdout": strings.Join([]string{
				"=== RUN   TestProxyHandlerBasic",
				"    main_test.go:28:",
				"        expected: \"/v1/chat/completions\"",
				"        actual  : \"/chat/completions\"",
			}, "\n"),
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1|error=exit status 1",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, errors.New("exit status 1"))

	targetContext := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(targetContext) != 1 || targetContext[0]["targetRef"] != "gateway/main_test.go" {
		t.Fatalf("failureTargetRepairContext = %#v, want resolved gateway/main_test.go surface", input["failureTargetRepairContext"])
	}
	candidates := mapSliceFromValue(input["rootCauseCandidateContext"])
	if len(candidates) == 0 {
		t.Fatalf("rootCauseCandidateContext = %#v, want adjacent implementation candidates", input["rootCauseCandidateContext"])
	}
	foundMain := false
	for _, candidate := range candidates {
		if candidate["targetRef"] == "gateway/main.go" && strings.Contains(stringFromMap(candidate, "content"), "proxyPath") {
			foundMain = true
			break
		}
	}
	if !foundMain {
		t.Fatalf("rootCauseCandidateContext = %#v, want gateway/main.go current content", input["rootCauseCandidateContext"])
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	targetSet := stringSliceFromValue(actionPackage["repairTargetSet"])
	if !stringSliceContains(targetSet, "gateway/main.go") || !stringSliceContains(targetSet, "gateway/main_test.go") {
		t.Fatalf("repairActionPackage = %#v, repairTargetSet = %#v, want failed surface and root-cause candidate", actionPackage, targetSet)
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if !strings.Contains(stringFromMap(required, "rootCauseCandidateRule"), "rootCauseCandidateContext") {
		t.Fatalf("requiredOutput = %#v, want root-cause candidate rule", required)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesFailureTargetSnapshotFromStructuredStdout(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "target-alpha.data"), []byte("alpha current content\n"), 0o644); err != nil {
		t.Fatalf("seed target alpha: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "target-beta.data"), []byte("beta current content\n"), 0o644); err != nil {
		t.Fatalf("seed target beta: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-failure-target-stdout")
	runID := common.RunID("run-failure-target-stdout")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/003-validate"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "exit status 1",
			"stdout":                  "validation failed for target resources:\ntarget-alpha.data\ntarget-beta.data\n",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|command=sh|exitCode=1",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, errors.New("exit status 1"))

	targetContext := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(targetContext) != 2 {
		t.Fatalf("failureTargetRepairContext = %#v, want two stdout target snapshots", input["failureTargetRepairContext"])
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	targets := mapSliceFromValue(actionPackage["targetSnapshots"])
	if len(targets) != 2 {
		t.Fatalf("repairActionPackage.targetSnapshots = %#v, want two target snapshots", actionPackage["targetSnapshots"])
	}
	targetSet := stringSliceFromValue(actionPackage["repairTargetSet"])
	if !stringSliceContains(targetSet, "target-alpha.data") || !stringSliceContains(targetSet, "target-beta.data") {
		t.Fatalf("repairTargetSet = %#v, want stdout target refs", actionPackage["repairTargetSet"])
	}
}

func TestValidateRepairPatchRequestGranularityRejectsNonSubstantiveUnifiedDiffWithContext(t *testing.T) {
	targetSnapshot := map[string]any{
		"targetRef":        "main_test.go",
		"exists":           true,
		"content":          "package main\n\nfunc TestX(t *testing.T) {}\n",
		"contentComplete":  true,
		"safeForFileWrite": true,
	}
	for _, tc := range []struct {
		name string
		diff string
		want string
	}{
		{
			name: "identical replacement",
			diff: "--- a/main_test.go\n+++ b/main_test.go\n@@ -1,1 +1,1 @@\n" +
				"-package main\n+package main\n",
			want: "identical_removed_and_added_lines",
		},
		{
			name: "blank only",
			diff: "--- a/main_test.go\n+++ b/main_test.go\n@@ -1,1 +1,2 @@\n" +
				" package main\n+\n",
			want: "blank_line_only_change",
		},
	} {
		for _, contextCase := range []struct {
			name  string
			input map[string]any
		}{
			{
				name: "failure target context",
				input: map[string]any{
					"failureTargetRepairContext": []map[string]any{targetSnapshot},
				},
			},
			{
				name: "repair action target snapshot",
				input: map[string]any{
					"repairActionPackage": map[string]any{
						"targetSnapshots": []map[string]any{targetSnapshot},
					},
				},
			},
		} {
			t.Run(tc.name+"/"+contextCase.name, func(t *testing.T) {
				input := map[string]any{
					stepInputKeyRuntimeStage:      runtimeStageRepair,
					"stateChangingOutputRequired": true,
					"repairObjective":             repairObjectiveStateChangingOutput,
				}
				for key, value := range contextCase.input {
					input[key] = value
				}
				step := &runmodel.Step{
					ID:    "run/repair-1",
					RunID: "run",
					Kind:  llmcommon.StepKindWorkspaceExecution,
					Input: input,
				}
				err := validateRepairPatchRequestGranularity(step, runtimePatchRequest{
					TargetRef: "main_test.go",
					PatchType: "unified_diff",
					Diff:      tc.diff,
				})
				if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("validateRepairPatchRequestGranularity err = %v, want %s", err, tc.want)
				}
			})
		}
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputPrioritizesResourceShapeDiagnostics(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "gateway.go"), []byte("package main\n\ntype Config struct { Address string }\nfunc newHandler(cfg Config) any { return cfg }\n"), 0o644); err != nil {
		t.Fatalf("seed gateway.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "streaming_fallback_estimator_test.go"), []byte("package main\n\nfunc TestFallback(t *testing.T) {\n\t_ = Config{Port: 8080}\n\t_ = newGatewayHandler()\n}\n"), 0o644); err != nil {
		t.Fatalf("seed test target: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-api-mismatch-repair")
	runID := common.RunID("run-api-mismatch-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-014/002-rerun-targeted-streaming-fallback-tests"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"status": "failed",
			"error":  "exit status 1",
			"stderr": strings.Join([]string{
				"./streaming_fallback_estimator_test.go:4:13: unknown field Port in struct literal of type Config",
				"./streaming_fallback_estimator_test.go:5:6: undefined: newGatewayHandler",
			}, "\n"),
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=streaming_fallback_estimator_test.go|exitCode=1",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, errors.New("exit status 1"))

	targetContext := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(targetContext) != 1 || targetContext[0]["targetRef"] != "streaming_fallback_estimator_test.go" {
		t.Fatalf("failureTargetRepairContext = %#v, want targeted test snapshot", input["failureTargetRepairContext"])
	}
	shapeContext := mapSliceFromValue(input["resourceShapeAlignmentContext"])
	if len(shapeContext) == 0 {
		t.Fatalf("resourceShapeAlignmentContext missing for resource shape diagnostics: %#v", input)
	}
	foundGateway := false
	for _, item := range shapeContext {
		if item["targetRef"] == "gateway.go" && strings.Contains(stringFromMap(item, "content"), "type Config struct { Address string }") {
			foundGateway = true
			break
		}
	}
	if !foundGateway {
		t.Fatalf("resourceShapeAlignmentContext = %#v, want current gateway.go resource snapshot", shapeContext)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	signals := mapFromValue(actionPackage["structuredFailureSignals"])
	if len(signals) == 0 {
		t.Fatalf("repairActionPackage.structuredFailureSignals missing for resource shape diagnostics: %#v", actionPackage)
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	for _, want := range []string{
		"Prioritize the primary diagnostics from structuredFailureSignals",
		"invalid attributes, shape mismatches",
		"repair those reported blockers before secondary validation",
		"resourceShapeAlignmentContext",
		"current workspace resources",
	} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("repairGuidance = %q, want %q", guidance, want)
		}
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputResolvesUniqueSuffixFailureTarget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(deliverableDirectory, "nested"), 0o755); err != nil {
		t.Fatalf("seed nested dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "nested", "target_test.go"), []byte("package nested\n"), 0o644); err != nil {
		t.Fatalf("seed nested target: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-failure-target-suffix-context")
	runID := common.RunID("run-failure-target-suffix-context")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/004-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageExecution,
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "exit status 1",
			"stdout":                  "target_test.go:7: expected value\n",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=target_test.go|exitCode=1",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, errors.New("exit status 1"))

	targetContext := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(targetContext) != 1 || targetContext[0]["targetRef"] != "nested/target_test.go" || targetContext[0]["exists"] != true {
		t.Fatalf("failureTargetRepairContext = %#v, want resolved nested target snapshot", input["failureTargetRepairContext"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputKeepsAmbiguousSuffixFailureTarget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	for _, dir := range []string{"first", "second"} {
		if err := os.MkdirAll(filepath.Join(deliverableDirectory, dir), 0o755); err != nil {
			t.Fatalf("seed %s dir: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(deliverableDirectory, dir, "target_test.go"), []byte("package sample\n"), 0o644); err != nil {
			t.Fatalf("seed %s target: %v", dir, err)
		}
	}
	workspaceID := common.WorkspaceID("ws-failure-target-ambiguous-suffix")
	runID := common.RunID("run-failure-target-ambiguous-suffix")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/004-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageExecution,
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "exit status 1",
			"stdout":                  "target_test.go:7: expected value\n",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=target_test.go|exitCode=1",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, errors.New("exit status 1"))

	targetContext := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(targetContext) != 1 || targetContext[0]["targetRef"] != "target_test.go" || targetContext[0]["exists"] != false {
		t.Fatalf("failureTargetRepairContext = %#v, want ambiguous short target preserved as missing snapshot", input["failureTargetRepairContext"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputAcquiresEvidenceForContractFailureWithoutTarget(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-state-contract")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/block/implement"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphNodeID:       "unit-001",
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"error": "state-changing execution-stage llm graph node returned no state-changing output",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-state-contract"}, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveAcquireFailureEvidence ||
		input["stateChangingOutputRequired"] != nil ||
		input["concreteFailureEvidenceRequired"] != true {
		t.Fatalf("repair input = %+v, want concrete evidence acquisition objective", input)
	}
	guidance := stringSliceFromValue(input["repairGuidance"])
	joined := strings.Join(guidance, "\n")
	if !strings.Contains(joined, "Acquire bounded concrete failure evidence") || strings.Contains(joined, "next output must include patch/apply") {
		t.Fatalf("repairGuidance = %#v, want evidence acquisition guidance", guidance)
	}
}

func TestRuntimeOrchestrator_InitialReplanStateChangingInputIncludesRepairActionPackage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-replan-action-package",
		WorkspaceID:     "ws-replan-action-package",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-before-replan-action-package",
				"reason":           "runtime_repair_exhausted_requires_strategy",
				"failureContext": map[string]any{
					failureOutputKeyClass:     failureClassRuntime,
					failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|failureTargets=main.go",
					"failureTargetScope":      "failureTargets=main.go",
					"failedStepId":            "run-before-replan-action-package/repair-003",
					"failedStepKind":          llmWorkspaceExecutionStepKind,
					"failureTargetRepairContext": []map[string]any{{
						"targetRef":        "main.go",
						"exists":           true,
						"contentHash":      "hash-main",
						"contentComplete":  true,
						"safeForFileWrite": true,
						"content":          "package main\nfunc main() {}\n",
					}},
					"failedStepOutput": map[string]any{
						"error": "invalid argument: state-changing llm repair step returned executionGraph without patch/apply",
						stepOutputKeyExecutionGraph: map[string]any{"nodes": []map[string]any{{
							"id":   "test-only",
							"kind": "shell/command",
						}}},
					},
				},
			},
		},
	}

	input := initialStepInput(run)

	if input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true ||
		input["replanStateChangingContinuation"] != true {
		t.Fatalf("input = %+v, want state-changing replan continuation", input)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	if len(actionPackage) == 0 {
		t.Fatalf("repairActionPackage = %#v, want package from replan continuation", input["repairActionPackage"])
	}
	targets := mapSliceFromValue(actionPackage["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "main.go" || targets[0]["contentHash"] != "hash-main" {
		t.Fatalf("repairActionPackage.targetSnapshots = %#v, want target snapshot", actionPackage["targetSnapshots"])
	}
	if summary := mapFromValue(actionPackage["previousRejectedOutputSummary"]); len(summary) == 0 || mapFromValue(summary["rejectedExecutionGraph"])["nodeCount"] == nil {
		t.Fatalf("repairActionPackage.previousRejectedOutputSummary = %#v, want rejected graph summary", actionPackage["previousRejectedOutputSummary"])
	}
	reassessment := mapFromValue(input["strategyReassessment"])
	if !boolValue(reassessment["required"]) || stringFromMap(reassessment, "failedSurface") != "failureTargets=main.go" {
		t.Fatalf("strategyReassessment = %#v, want repeated failed-surface reassessment", input["strategyReassessment"])
	}
	if int64FromValue(input["targetRepairAttempts"]) < 2 {
		t.Fatalf("targetRepairAttempts = %#v, want inherited repeated-target budget", input["targetRepairAttempts"])
	}
	pkgReassessment := mapFromValue(actionPackage["strategyReassessment"])
	if !boolValue(pkgReassessment["required"]) || stringFromMap(pkgReassessment, "failedSurface") != "failureTargets=main.go" {
		t.Fatalf("repairActionPackage.strategyReassessment = %#v, want inherited reassessment", actionPackage["strategyReassessment"])
	}
	repeatedMode := mapFromValue(actionPackage["repeatedTargetRepairMode"])
	if !boolValue(repeatedMode["enabled"]) || !boolValue(repeatedMode["mustReassessRepeatedFailedSurface"]) {
		t.Fatalf("repairActionPackage.repeatedTargetRepairMode = %#v, want repeated-target successor mode", actionPackage["repeatedTargetRepairMode"])
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "choose repairMode") {
		t.Fatalf("repairGuidance = %q, want strategy reassessment repair mode guidance", guidance)
	}
}

func TestRuntimeOrchestrator_InitialRejectedResultContinuationSupersedesReplanStateChangingInput(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-rejected-supersedes-replan-input",
		WorkspaceID:     "ws-rejected-supersedes-replan-input",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"rejectedResultContinuation": map[string]any{
				"predecessorRunId": "run-latest",
				"summary":          "latest rejected result requires current evidence",
				"requiredEvidence": []string{"replan missing work", "run concrete verification"},
			},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-older",
				"reason":           "runtime_repair_exhausted_requires_strategy",
				"failureContext": map[string]any{
					failureOutputKeyClass:     failureClassPatchApply,
					failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|targetRef=old_test.go|error=context mismatch",
					"failureTargetScope":      "targetRef=old_test.go",
					"failedStepId":            "run-older/graph-005/001-patch-target",
					"failedStepKind":          "patch/apply",
				},
			},
		},
	}

	input := initialStepInput(run)

	if input["replanStateChangingContinuation"] != nil ||
		input["stateChangingOutputRequired"] != nil ||
		input["repairObjective"] != repairObjectiveAcquireFailureEvidence ||
		input["concreteFailureEvidenceRequired"] != true {
		t.Fatalf("input = %+v, want rejected result evidence continuation instead of replan state-changing repair", input)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(pkg["evidenceAcquisitionRequired"]) || !boolValue(required["mustProduceConcreteFailureEvidence"]) {
		t.Fatalf("repairActionPackage = %+v, want evidence acquisition package from rejected result continuation", pkg)
	}
	replan := mapFromValue(mapFromValue(input["executionContext"])["replanContinuation"])
	if replan["historicalOnly"] != true || replan["useAsCurrentProgress"] != false {
		t.Fatalf("executionContext.replanContinuation = %+v, want historical-only replan", replan)
	}
	if strings.Contains(stringFromMap(pkg, "failureSignature"), "old_test.go") {
		t.Fatalf("repairActionPackage = %+v, want old replan failure suppressed", pkg)
	}
}

func TestRuntimeOrchestrator_InitialRejectedResultContinuationDoesNotSupersedeActionableReplanFailure(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-rejected-keeps-actionable-replan-input",
		WorkspaceID:     "ws-rejected-keeps-actionable-replan-input",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"rejectedResultContinuation": map[string]any{
				"predecessorRunId": "run-latest",
				"summary":          "deliverable evidence still missing",
				"requiredEvidence": []string{"coverage_map", "version_control_snapshot"},
			},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-before-replan",
				"reason":           "runtime_repair_exhausted_requires_strategy",
				"failureContext": map[string]any{
					failureOutputKeyClass:     failureClassLLMDecisionInvalid,
					failureOutputKeySignature: "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid",
					"failedStepId":            "run-before-replan/repair-010",
					"failedStepKind":          llmWorkspaceExecutionStepKind,
					"failedStepOutput": map[string]any{
						"stage":               "llm_schema_invalid",
						"error":               "invalid argument: invalid llm decision json: no json object found",
						failureOutputKeyClass: failureClassLLMDecisionInvalid,
					},
				},
				stepInputKeyCurrentProgressDigest: map[string]any{
					"currentGap": "# target/pkg\npkg/lsp/server.go:10:24: undefined: LSPInstance\nmain.go:28:28: undefined: NewServer",
					"lastObservedFailure": map[string]any{
						"stepId":                    "run-before-replan/graph-001/011-build",
						"kind":                      "shell/command",
						"failureClass":              failureClassShellCommand,
						"failureSignature":          "class=shell_command_failed|kind=shell/command|failureTargets=pkg/lsp/server.go,main.go|exitCode=1",
						"concreteFailureDiagnostic": "pkg/lsp/server.go:10:24: undefined: LSPInstance\nmain.go:28:28: undefined: NewServer",
						"evidenceValidity":          "actionable_failure",
					},
					"nextRequiredAction": "address_last_failure_under_current_stage_contract",
				},
			},
		},
	}

	input := initialStepInput(run)

	if input["repairObjective"] != repairObjectiveAcquireFailureEvidence ||
		input["stateChangingOutputRequired"] != nil ||
		input["concreteFailureEvidenceRequired"] != true {
		t.Fatalf("input = %+v, want actionable replan failure to drive evidence acquisition", input)
	}
	currentFailure := mapFromValue(input["currentFailure"])
	if currentFailure["failureClass"] != failureClassShellCommand ||
		currentFailure["failedStepId"] != "run-before-replan/graph-001/011-build" ||
		!strings.Contains(stringFromMap(currentFailure, "reason"), "LSPInstance") {
		t.Fatalf("currentFailure = %+v, want predecessor build failure", currentFailure)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	pkgFailure := mapFromValue(pkg["currentFailure"])
	if pkgFailure["failureClass"] != failureClassShellCommand ||
		!strings.Contains(stringFromMap(pkgFailure, "targetScope"), "pkg/lsp/server.go") {
		t.Fatalf("repairActionPackage.currentFailure = %+v, want actionable build failure", pkgFailure)
	}
	signals := mapFromValue(pkg["structuredFailureSignals"])
	if refs := stringSliceFromValue(signals["targetRefs"]); !stringSliceContains(refs, "pkg/lsp/server.go") || !stringSliceContains(refs, "main.go") {
		t.Fatalf("structuredFailureSignals = %+v, want build target refs", signals)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if required["mustProduceConcreteFailureEvidence"] != true || boolValue(required["mustIncludeStateChangingNode"]) {
		t.Fatalf("requiredOutput = %+v, want evidence acquisition until current target snapshots are loaded", required)
	}
}

func TestRuntimeOrchestrator_InitialRejectedResultContinuationDoesNotSupersedeReplanTimelineConcreteFailure(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-rejected-keeps-replan-timeline-failure",
		WorkspaceID:     "ws-rejected-keeps-replan-timeline-failure",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"rejectedResultContinuation": map[string]any{
				"predecessorRunId": "run-latest",
				"summary":          "deliverable evidence still missing",
				"requiredEvidence": []string{"coverage_map", "version_control_snapshot"},
			},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-before-replan",
				"reason":           "runtime_repair_exhausted_requires_strategy",
				stepInputKeyCurrentProgressDigest: map[string]any{
					"currentGap": "invalid argument: state-changing llm repair step returned no state-changing output",
					"lastObservedFailure": map[string]any{
						"stepId":           "run-before-replan/repair-009",
						"kind":             llmWorkspaceExecutionStepKind,
						"failureClass":     failureClassRuntime,
						"failureReason":    "invalid argument: state-changing llm repair step returned no state-changing output",
						"evidenceValidity": "actionable_failure",
					},
				},
				stepInputKeyProcessEvidenceTimeline: []map[string]any{
					{
						"stepId":                    "run-before-replan/graph-001/009-verify-build",
						"kind":                      shellcommon.StepKindCommand,
						"state":                     string(common.StepStateFailed),
						"failureClass":              failureClassShellCommand,
						"failureSignature":          "class=shell_command_failed|kind=shell/command|failureTargets=pkg/lsp/server.go,main.go|exitCode=1",
						"failureReason":             "# module\npkg/lsp/server.go:10:24: undefined: LSPInstance\nmain.go:28:28: undefined: NewServer",
						"concreteFailureDiagnostic": "pkg/lsp/server.go:10:24: undefined: LSPInstance\nmain.go:28:28: undefined: NewServer",
						"evidenceValidity":          "actionable_failure",
					},
					{
						"stepId":           "run-before-replan/repair-009",
						"kind":             llmWorkspaceExecutionStepKind,
						"state":            string(common.StepStateFailed),
						"failureClass":     failureClassRuntime,
						"failureReason":    "invalid argument: state-changing llm repair step returned no state-changing output",
						"failureSignature": "class=runtime_failed|kind=llm/workspace_execution|stage=llm_plan|error=invalid argument: state-changing llm repair step returned no state-changing output",
					},
				},
			},
		},
	}

	input := initialStepInput(run)

	currentFailure := mapFromValue(input["currentFailure"])
	if currentFailure["failureClass"] != failureClassShellCommand ||
		currentFailure["failedStepId"] != "run-before-replan/graph-001/009-verify-build" ||
		!strings.Contains(stringFromMap(currentFailure, "reason"), "LSPInstance") {
		t.Fatalf("currentFailure = %+v, want concrete predecessor build failure", currentFailure)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	signals := mapFromValue(pkg["structuredFailureSignals"])
	if refs := stringSliceFromValue(signals["targetRefs"]); !stringSliceContains(refs, "pkg/lsp/server.go") || !stringSliceContains(refs, "main.go") {
		t.Fatalf("structuredFailureSignals = %+v, want concrete build target refs", signals)
	}
	if strings.Contains(stringFromMap(mapFromValue(pkg["currentFailure"]), "reason"), "state-changing llm repair step returned no state-changing output") {
		t.Fatalf("repairActionPackage.currentFailure = %+v, want wrapper failure skipped", pkg["currentFailure"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairReturnsToRejectedContinuationAfterHistoricalReplanFailure(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-repair-rejected-supersedes-replan",
		WorkspaceID:     "ws-repair-rejected-supersedes-replan",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"rejectedResultContinuation": map[string]any{
				"predecessorRunId": "run-latest",
				"summary":          "latest rejected result requires current evidence",
				"requiredEvidence": []string{"replan missing work", "run concrete verification"},
			},
			"replanContinuation": map[string]any{
				"failureContext": map[string]any{
					"failureTargetScope": "targetRef=old_test.go",
				},
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-repair-rejected-supersedes-replan/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			"replanStateChangingContinuation": true,
			"executionContext": map[string]any{
				"rejectedResultContinuation": run.ExecutionContext["rejectedResultContinuation"],
				"replanContinuation": map[string]any{
					"historicalOnly": true,
					"failureContext": map[string]any{
						"failureTargetScope": "targetRef=old_test.go",
					},
				},
			},
			"repairActionPackage": map[string]any{
				"repairTargetSet": []string{"old_test.go"},
			},
		},
		Output: map[string]any{
			"error":                   "invalid argument: runtime repair patch/apply unified_diff for target \"old_test.go\" is non-substantive",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|targetRef=old_test.go",
		},
	}

	input := (&RuntimeOrchestrator{}).runtimeRepairFollowUpStepInput(context.Background(), run, failed, 2, common.ErrInvalidArgument)

	if input["stateChangingOutputRequired"] != nil || input["repairObjective"] != repairObjectiveAcquireFailureEvidence {
		t.Fatalf("repair input = %+v, want rejected-result evidence acquisition", input)
	}
	currentFailure := mapFromValue(input["currentFailure"])
	if currentFailure["source"] != "rejectedResultContinuation" || currentFailure["evidenceValidity"] != "actionable_gap" {
		t.Fatalf("currentFailure = %+v, want rejected-result actionable gap", currentFailure)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if targets := stringSliceFromValue(pkg["repairTargetSet"]); len(targets) > 0 {
		t.Fatalf("repairActionPackage = %+v, want historical replan target suppressed", pkg)
	}
	historical := mapFromValue(input["historicalRepairAttempt"])
	if historical["supersededBy"] != "rejectedResultContinuation" {
		t.Fatalf("historicalRepairAttempt = %+v, want superseded marker", historical)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairKeepsCurrentPatchFailureOverRejectedContinuation(t *testing.T) {
	ctx := context.Background()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("seed main.go: %v", err)
	}
	workspaceRepo := memory.NewWorkspaceRepository()
	workspaceID := common.WorkspaceID("ws-current-patch-over-rejected")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: deliverableDirectory,
		State:                common.WorkspaceStateRunning,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              "run-current-patch-over-rejected",
		WorkspaceID:     workspaceID,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"rejectedResultContinuation": map[string]any{
				"summary":          "older rejected result still needs evidence",
				"requiredEvidence": []string{"bounded validation"},
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-current-patch-over-rejected/graph-001/001-patch-main",
		RunID: run.ID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "main.go",
			"patchType": "text_replace",
			"oldText":   "package main\n",
			"newText":   "package main\n\n",
			"executionContext": map[string]any{
				"rejectedResultContinuation": run.ExecutionContext["rejectedResultContinuation"],
				"replanContinuation": map[string]any{
					"historicalOnly": true,
					"failureContext": map[string]any{
						"failureTargetScope": "targetRef=main.go",
					},
				},
			},
		},
		Output: map[string]any{
			"stage":                   "patch_apply",
			"error":                   "conflict: patch baseHash mismatch",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|failureTargets=main.go|error=conflict: patch basehash mismatch",
		},
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: workspaceRepo,
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failed, 1, common.ErrConflict)

	if input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true ||
		input["concreteFailureEvidenceRequired"] != nil {
		t.Fatalf("repair input = %+v, want current patch failure to stay state-changing", input)
	}
	if currentFailure := mapFromValue(input["currentFailure"]); currentFailure["source"] == "rejectedResultContinuation" {
		t.Fatalf("currentFailure = %+v, should not return to rejected result continuation", currentFailure)
	}
	if snapshots := mapSliceFromValue(input["patchRepairContext"]); len(snapshots) != 1 || snapshots[0]["targetRef"] != "main.go" {
		t.Fatalf("patchRepairContext = %+v, want current main.go snapshot", input["patchRepairContext"])
	}
	targets := mapSliceFromValue(mapFromValue(input["repairActionPackage"])["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "main.go" {
		t.Fatalf("repairActionPackage.targetSnapshots = %+v, want current main.go snapshot", targets)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairKeepsCurrentSnapshotsOverRejectedContinuation(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-current-snapshots-over-rejected",
		WorkspaceID:     "ws-current-snapshots-over-rejected",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"rejectedResultContinuation": map[string]any{
				"summary":          "older rejected result still needs evidence",
				"requiredEvidence": []string{"bounded validation"},
			},
		},
	}
	snapshot := map[string]any{
		"targetRef":        "main.go",
		"exists":           true,
		"contentHash":      "hash-main-current",
		"contentComplete":  true,
		"safeForFileWrite": true,
		"content":          "package main\n\nfunc main() {}\n",
	}
	failed := &runmodel.Step{
		ID:    "run-current-snapshots-over-rejected/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"stateChangingOutputRequired":     true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"failureTargetRepairContext":      []map[string]any{snapshot},
			"executionContext": map[string]any{
				"rejectedResultContinuation": run.ExecutionContext["rejectedResultContinuation"],
				"replanContinuation": map[string]any{
					"historicalOnly": true,
					"failureContext": map[string]any{
						"failureTargetScope": "targetRef=main.go",
					},
				},
			},
			"repairActionPackage": map[string]any{
				"targetSnapshots": []map[string]any{snapshot},
				"requiredOutput":  stateChangingRepairRequiredOutput(),
			},
		},
		Output: map[string]any{
			outputKeyContractViolationType: contractViolationPatchContentRequired,
			"stage":                        "llm_execution_graph_plan",
			"error":                        "invalid argument: executionGraph patch/apply input invalid: invalid argument: patch text_replace oldText/newText must describe a substantive change",
			failureOutputKeyClass:          failureClassRuntime,
			failureOutputKeySignature:      "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|failureTargets=main.go",
		},
	}

	input := (&RuntimeOrchestrator{steps: memory.NewStepRepository()}).runtimeRepairFollowUpStepInput(context.Background(), run, failed, 2, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true ||
		input["concreteFailureEvidenceRequired"] != nil {
		t.Fatalf("repair input = %+v, want current snapshot repair to stay state-changing", input)
	}
	if currentFailure := mapFromValue(input["currentFailure"]); currentFailure["source"] == "rejectedResultContinuation" {
		t.Fatalf("currentFailure = %+v, should not return to rejected result continuation", currentFailure)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	targets := mapSliceFromValue(pkg["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "main.go" || targets[0]["contentHash"] != "hash-main-current" {
		t.Fatalf("repairActionPackage.targetSnapshots = %+v, want inherited current main.go snapshot", targets)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustAvoidNoOpPatch"]) || !boolValue(required["mustProduceSubstantivePatch"]) {
		t.Fatalf("requiredOutput = %+v, want no-op patch repair contract", required)
	}
}

func TestRuntimeOrchestrator_InitialReplanStateChangingInputPromotesPatchFailureIntoStrategyReassessment(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-replan-patch-failure-reassessment",
		WorkspaceID:     "ws-replan-patch-failure-reassessment",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-before-replan-patch-failure",
				"reason":           "runtime_repair_exhausted_requires_strategy",
				"failureContext": map[string]any{
					failureOutputKeyClass:     "patch_apply_failed",
					failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|targetRef=e2e_gateway_contract_test.go|error=context mismatch",
					"failureTargetScope":      "targetRef=e2e_gateway_contract_test.go",
					"failedStepId":            "run-before-replan-patch-failure/graph-005/001-patch-target",
					"failedStepKind":          "patch/apply",
					"attempts":                3,
					"failedStepOutput": map[string]any{
						"error":  "invalid argument: unified diff hunk context mismatch",
						"reason": "invalid argument: unified diff hunk context mismatch",
						"stage":  "patch_apply",
						"status": "completed",
					},
				},
			},
		},
	}

	input := initialStepInput(run)

	reassessment := mapFromValue(input["strategyReassessment"])
	if !boolValue(reassessment["required"]) {
		t.Fatalf("strategyReassessment = %#v, want required reassessment", input["strategyReassessment"])
	}
	if stringFromMap(reassessment, "failedSurface") != "targetRef=e2e_gateway_contract_test.go" {
		t.Fatalf("strategyReassessment.failedSurface = %#v, want failed patch target scope", reassessment["failedSurface"])
	}
	if int64FromValue(reassessment["targetRepairAttempts"]) < 2 || int64FromValue(input["targetRepairAttempts"]) < 2 {
		t.Fatalf("targetRepairAttempts = input:%#v reassessment:%#v, want repeated target attempts", input["targetRepairAttempts"], reassessment["targetRepairAttempts"])
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	if len(actionPackage) == 0 {
		t.Fatalf("repairActionPackage = %#v, want package", input["repairActionPackage"])
	}
	pkgReassessment := mapFromValue(actionPackage["strategyReassessment"])
	if stringFromMap(pkgReassessment, "failedSurface") != "targetRef=e2e_gateway_contract_test.go" {
		t.Fatalf("repairActionPackage.strategyReassessment = %#v, want same failed surface contract", actionPackage["strategyReassessment"])
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if boolValue(required["mustIncludeStateChangingNode"]) || !boolValue(required["mustProduceConcreteFailureEvidence"]) {
		t.Fatalf("repairActionPackage.requiredOutput = %#v, want evidence acquisition until current target snapshot is loaded", actionPackage["requiredOutput"])
	}
}

func TestRuntimeOrchestrator_NextDispatchHydratesReplanRepairTargetSnapshot(t *testing.T) {
	ctx := context.Background()
	deliverableDirectory := t.TempDir()
	targetRef := "e2e_gateway_contract_test.go"
	targetContent := "package main\n\nfunc TestGateway(t *testing.T) {}\n"
	if err := os.WriteFile(filepath.Join(deliverableDirectory, targetRef), []byte(targetContent), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	workspaceRepo := memory.NewWorkspaceRepository()
	workspaceID := common.WorkspaceID("ws-replan-hydrate-target")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: deliverableDirectory,
		State:                common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              "run-replan-hydrate-target",
		WorkspaceID:     workspaceID,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-before-replan-hydrate",
				"reason":           "runtime_repair_exhausted_requires_strategy",
				"failureContext": map[string]any{
					failureOutputKeyClass:     failureClassPatchApply,
					failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|targetRef=" + targetRef + "|error=context mismatch",
					"failureTargetScope":      "targetRef=" + targetRef,
					"failedStepId":            "run-before-replan-hydrate/graph-005/001-patch-target",
					"failedStepKind":          "patch/apply",
					"attempts":                3,
					"failedStepOutput": map[string]any{
						"error":  "invalid argument: unified diff hunk context mismatch",
						"reason": "invalid argument: unified diff hunk context mismatch",
						"stage":  "patch_apply",
						"status": "completed",
					},
				},
			},
		},
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: workspaceRepo,
	}

	specs, err := orch.nextDispatchSpecs(ctx, run)
	if err != nil {
		t.Fatalf("nextDispatchSpecs: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %d, want 1", len(specs))
	}
	input := specs[0].Input
	if input["repairObjective"] != repairObjectiveStateChangingOutput || input["stateChangingOutputRequired"] != true {
		t.Fatalf("input = %+v, want state-changing repair after target snapshot hydration", input)
	}
	if snapshots := mapSliceFromValue(input["patchRepairContext"]); len(snapshots) != 1 || snapshots[0]["targetRef"] != targetRef || !strings.Contains(stringFromMap(snapshots[0], "content"), "TestGateway") {
		t.Fatalf("patchRepairContext = %#v, want hydrated current target snapshot", input["patchRepairContext"])
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	targets := mapSliceFromValue(actionPackage["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != targetRef || !strings.Contains(stringFromMap(targets[0], "content"), "TestGateway") {
		t.Fatalf("repairActionPackage.targetSnapshots = %#v, want hydrated target snapshot", actionPackage["targetSnapshots"])
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true || !boolValue(required["mustUseCurrentTargetSnapshot"]) {
		t.Fatalf("requiredOutput = %#v, want state-changing current-snapshot contract", required)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputInheritsTargetContextAfterContractFailure(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-context-inherit")
	targetContext := []map[string]any{
		{
			"targetRef":   "main.go",
			"exists":      true,
			"source":      "failure_target_current_workspace_file",
			"contentHash": "hash-main",
			"content":     "package main\n\nimport \"old/module/config\"\n",
		},
	}
	manifest := []map[string]any{
		{"path": "main.go", "kind": "file", "size": float64(40)},
		{"path": "config.go", "kind": "file", "size": float64(80)},
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-graph-001-002-test"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"stateChangingOutputRequired":     true,
			"failureTargetRepairContext":      targetContext,
			stepInputKeyWorkspaceFileManifest: manifest,
		},
		Output: map[string]any{
			"error":                   "invalid argument: state-changing llm repair step returned executionGraph without patch/apply",
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan",
			stepOutputKeyExecutionGraph: map[string]any{"nodes": []map[string]any{{
				"id":   "test-only",
				"kind": "shell/command",
			}}},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-context-inherit"}, failedStep, 2, common.ErrInvalidArgument)

	if input["stateChangingRecoveryMode"] != "direct_state_change" {
		t.Fatalf("stateChangingRecoveryMode = %#v, want direct_state_change", input["stateChangingRecoveryMode"])
	}
	inherited := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(inherited) != 1 || inherited[0]["targetRef"] != "main.go" || inherited[0]["contentHash"] != "hash-main" {
		t.Fatalf("failureTargetRepairContext = %#v, want inherited target context", input["failureTargetRepairContext"])
	}
	inheritedManifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(inheritedManifest) != 2 || inheritedManifest[0]["path"] != "main.go" || inheritedManifest[1]["path"] != "config.go" {
		t.Fatalf("workspaceFileManifest = %#v, want inherited manifest preserved for direct recovery", input[stepInputKeyWorkspaceFileManifest])
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "Preserve inherited repair context") || !strings.Contains(guidance, "accepted state-changing artifacts") || !strings.Contains(guidance, "workspace manifest") {
		t.Fatalf("repairGuidance = %q, want inherited repair context guidance", guidance)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	if len(actionPackage) == 0 {
		t.Fatalf("repairActionPackage = %#v, want inherited actionable package", input["repairActionPackage"])
	}
	targets := mapSliceFromValue(actionPackage["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "main.go" || targets[0]["contentHash"] != "hash-main" {
		t.Fatalf("repairActionPackage.targetSnapshots = %#v, want inherited target snapshot", actionPackage["targetSnapshots"])
	}
	if summary := mapFromValue(actionPackage["previousRejectedOutputSummary"]); len(summary) == 0 || mapFromValue(summary["rejectedExecutionGraph"])["nodeCount"] == nil {
		t.Fatalf("repairActionPackage.previousRejectedOutputSummary = %#v, want rejected graph summary", actionPackage["previousRejectedOutputSummary"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputInheritsStrategyReassessmentAfterContractFailure(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-strategy-reassessment-inherit")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-step-1"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"failureTargetScope":          "targetRef=surface.txt",
			"targetRepairAttempts":        int64(3),
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "same_target_scope_repeated_failure",
				"failedSurface":        "targetRef=surface.txt",
				"targetRepairAttempts": int64(3),
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
			"rootCauseCandidateContext": []map[string]any{
				{"targetRef": "candidate.txt", "source": "workspace_manifest"},
			},
		},
		Output: map[string]any{
			"error":                   "invalid argument: strategyReassessment.required=true requires state-changing patch output to declare failedSurface/rootCauseTarget/repairTargetDecision/forbiddenRepair/verificationPlan",
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-strategy-reassessment-inherit"}, failedStep, 2, common.ErrInvalidArgument)

	reassessment := mapFromValue(input["strategyReassessment"])
	if !boolValue(reassessment["required"]) || stringFromMap(reassessment, "failedSurface") != "targetRef=surface.txt" {
		t.Fatalf("strategyReassessment = %#v, want inherited repeated-surface contract", input["strategyReassessment"])
	}
	candidates := mapSliceFromValue(input["rootCauseCandidateContext"])
	if len(candidates) != 1 || candidates[0]["targetRef"] != "candidate.txt" {
		t.Fatalf("rootCauseCandidateContext = %#v, want inherited candidates", input["rootCauseCandidateContext"])
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	pkgReassessment := mapFromValue(actionPackage["strategyReassessment"])
	if !boolValue(pkgReassessment["required"]) || stringFromMap(pkgReassessment, "failedSurface") != "targetRef=surface.txt" {
		t.Fatalf("repairActionPackage.strategyReassessment = %#v, want inherited contract", actionPackage["strategyReassessment"])
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "runtime contract failure on the repair wrapper does not clear") ||
		!strings.Contains(guidance, "surfaceContractCorrection") {
		t.Fatalf("repairGuidance = %q, want inherited reassessment guidance", guidance)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputHydratesRootCauseCandidatesFromInheritedReassessment(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(deliverableDirectory, "scope"), 0o755); err != nil {
		t.Fatalf("mkdir scope: %v", err)
	}
	files := map[string][]byte{
		"scope/resource_check.txt": []byte("observes resource shape and required attribute\n"),
		"scope/resource.txt":       []byte("current resource shape\n"),
		"scope/config.txt":         []byte("configuration\n"),
		"scope/manifest.txt":       []byte("manifest\n"),
		"scope/checksum.txt":       []byte("checksum\n"),
		"scope/runtime.bin":        []byte{0, 1, 2, 3},
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(deliverableDirectory, filepath.FromSlash(name)), content, 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	workspaceID := common.WorkspaceID("ws-root-cause-candidate-hydration")
	runID := common.RunID("run-root-cause-candidate-hydration")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"failureTargetScope":          "targetRef=scope/resource_check.txt",
			"targetRepairAttempts":        int64(2),
			"strategyReassessment": map[string]any{
				"required":             true,
				"reason":               "same_target_scope_repeated_failure",
				"failedSurface":        "targetRef=scope/resource_check.txt",
				"failureTargetScope":   "targetRef=scope/resource_check.txt",
				"targetRepairAttempts": int64(2),
				"requiredDecisionFields": []string{
					"failedSurface",
					"rootCauseTarget",
					"repairTargetDecision",
					"repairMode",
					"forbiddenRepair",
					"verificationPlan",
				},
			},
		},
		Output: map[string]any{
			"error":                   "invalid argument: strategyReassessment.required=true requires patch/apply decision fields",
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 2, common.ErrInvalidArgument)

	candidates := mapSliceFromValue(input["rootCauseCandidateContext"])
	if len(candidates) == 0 {
		t.Fatalf("rootCauseCandidateContext = %#v, want hydrated candidates", input["rootCauseCandidateContext"])
	}
	var foundCandidate bool
	for _, candidate := range candidates {
		if boolValue(candidate["isBinary"]) {
			t.Fatalf("rootCauseCandidateContext = %#v, want binary candidates excluded", input["rootCauseCandidateContext"])
		}
		if stringFromMap(candidate, "targetRef") == "scope/resource.txt" {
			foundCandidate = true
		}
	}
	if !foundCandidate {
		t.Fatalf("rootCauseCandidateContext = %#v, want same-directory non-binary candidate scope/resource.txt", input["rootCauseCandidateContext"])
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	pkgCandidates := mapSliceFromValue(actionPackage["rootCauseCandidateContext"])
	if len(pkgCandidates) == 0 {
		t.Fatalf("repairActionPackage.rootCauseCandidateContext = %#v, want hydrated candidates", actionPackage["rootCauseCandidateContext"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputPromotesPrerequisiteManifestCandidate(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	files := map[string][]byte{
		"go.mod":          []byte("module example.test/app\n\ngo 1.21\n"),
		"mcp-config.json": []byte(`{"servers":{}}` + "\n"),
		"main_test.go":    []byte("package main\n"),
		"server/main.go":  []byte("package main\n"),
	}
	for name, content := range files {
		path := filepath.Join(deliverableDirectory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	workspaceID := common.WorkspaceID("ws-prereq-manifest-candidate")
	runID := common.RunID("run-prereq-manifest-candidate")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: deliverableDirectory,
		State:                common.WorkspaceStateActive,
		ContextHints: map[string]any{
			"runtimeRepair": map[string]any{
				"prerequisiteManifestTargetPatterns": []any{"go.mod"},
			},
		},
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/004-dependency-resolution"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"stderr": strings.Join([]string{
				"go: finding module for package example.test/dependency",
				"go: toolchain upgrade needed to resolve example.test/dependency",
				`Get "https://external.example/toolchain?go-get=1": dial tcp 142.251.45.145:443: i/o timeout`,
			}, "\n"),
			"exitCode":                1,
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=142.251.45.145|exitCode=1",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, errors.New("exit status 1"))

	if targets := mapSliceFromValue(input["failureTargetRepairContext"]); len(targets) != 0 {
		t.Fatalf("failureTargetRepairContext = %#v, want external endpoint excluded from repair targets", targets)
	}
	candidates := mapSliceFromValue(input["rootCauseCandidateContext"])
	if len(candidates) == 0 || stringFromMap(candidates[0], "targetRef") != "go.mod" {
		t.Fatalf("rootCauseCandidateContext = %#v, want go.mod prerequisite manifest candidate", candidates)
	}
	if len(candidates) != 1 {
		t.Fatalf("rootCauseCandidateContext = %#v, want only template-declared prerequisite manifest candidate", candidates)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	targets := mapSliceFromValue(actionPackage["targetSnapshots"])
	if len(targets) == 0 || stringFromMap(targets[0], "targetRef") != "go.mod" || !strings.Contains(stringFromMap(targets[0], "content"), "module example.test/app") {
		t.Fatalf("repairActionPackage = %#v, rootCauseCandidateContext = %#v, want promoted go.mod current snapshot", actionPackage, input["rootCauseCandidateContext"])
	}
	targetSet := stringSliceFromValue(actionPackage["repairTargetSet"])
	if !stringSliceContains(targetSet, "go.mod") || stringSliceContains(targetSet, "142.251.45.145") {
		t.Fatalf("repairTargetSet = %#v, want go.mod and no external endpoint", targetSet)
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustPatchDirectlyFromProvidedSnapshot"] != true {
		t.Fatalf("requiredOutput = %#v, want current snapshot patch contract", required)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputKeepsPrerequisiteManifestCandidateWithConcreteFailureTarget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	files := map[string][]byte{
		"dependency.manifest": []byte("dependency = stable\n"),
		"src/client.txt":      []byte("client implementation\n"),
	}
	for name, content := range files {
		path := filepath.Join(deliverableDirectory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	workspaceID := common.WorkspaceID("ws-prereq-manifest-with-target")
	runID := common.RunID("run-prereq-manifest-with-target")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: deliverableDirectory,
		State:                common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/002-verify"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"executionContract": map[string]any{
				"runtimeRepair": map[string]any{
					"prerequisiteManifestTargetPatterns": []any{"dependency.manifest"},
				},
			},
		},
		Output: map[string]any{
			"stderr": strings.Join([]string{
				"dependency resolve failed: lock/checksum metadata unavailable",
				"src/client.txt:10: prerequisite version mismatch",
			}, "\n"),
			"exitCode":                1,
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=src/client.txt|exitCode=1",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, errors.New("exit status 1"))

	candidates := mapSliceFromValue(input["rootCauseCandidateContext"])
	if len(candidates) == 0 || stringFromMap(candidates[0], "targetRef") != "dependency.manifest" {
		t.Fatalf("rootCauseCandidateContext = %#v, want prerequisite manifest candidate retained before adjacent target candidates", candidates)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	pkgCandidates := mapSliceFromValue(actionPackage["rootCauseCandidateContext"])
	if len(pkgCandidates) == 0 || stringFromMap(pkgCandidates[0], "targetRef") != "dependency.manifest" {
		t.Fatalf("repairActionPackage.rootCauseCandidateContext = %#v, want prerequisite manifest candidate retained", actionPackage["rootCauseCandidateContext"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesManifestForCoverageValidationEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	for name, content := range map[string]string{
		"resource-a.txt": "primary",
		"resource-b.txt": "supporting",
	} {
		if err := os.WriteFile(filepath.Join(deliverableDirectory, name), []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	workspaceID := common.WorkspaceID("ws-state-changing-resource-manifest")
	runID := common.RunID("run-state-changing-resource-manifest")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"stateChangingOutputRequired": true,
			"missingDoDItems": []map[string]any{
				{"item": "produce required artifact", "state": "failed"},
			},
			stepInputKeyGraphEvidence: []map[string]any{
				{
					"kind":   "file/list",
					"nodeId": "list_resources",
					"output": map[string]any{
						"stage": "file_list",
						"entries": []map[string]any{
							{"path": "resource-a.txt", "kind": "file"},
							{"path": "resource-b.txt", "kind": "file"},
						},
					},
				},
			},
		},
		Output: map[string]any{
			"error":                   "invalid argument: state-changing llm repair step returned executionGraph without patch/apply",
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 2, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveConcreteValidationEvidence || boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want validation evidence repair until concrete implementation target evidence exists", input)
	}
	manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) != 2 || manifest[0]["path"] != "resource-a.txt" || manifest[1]["path"] != "resource-b.txt" {
		t.Fatalf("workspaceFileManifest = %#v, want manifest retained as evidence context", input[stepInputKeyWorkspaceFileManifest])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesManifestForOutputMissingDoD(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	for name, content := range map[string]string{
		"resource-a.txt": "primary",
		"resource-b.txt": "supporting",
	} {
		if err := os.WriteFile(filepath.Join(deliverableDirectory, name), []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	workspaceID := common.WorkspaceID("ws-output-dod-manifest")
	runID := common.RunID("run-output-dod-manifest")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up-graph-planning-001-probe"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"stateChangingOutputRequired": true,
			stepInputKeyGraphEvidence: []map[string]any{
				{
					"kind":   "file/list",
					"nodeId": "probe",
					"output": map[string]any{
						"stage": "file_list",
						"entries": []map[string]any{
							{"path": "resource-a.txt", "kind": "file"},
							{"path": "resource-b.txt", "kind": "file"},
						},
					},
				},
			},
		},
		Output: map[string]any{
			"error":                   "invalid argument: completionState failed has no actionable continuation",
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
			"missingDoDItems": []map[string]any{
				{"item": "produce required artifact", "state": "failed", "source": "dodAssessment"},
			},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, common.ErrInvalidArgument)

	manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) != 2 || manifest[0]["path"] != "resource-a.txt" || manifest[1]["path"] != "resource-b.txt" {
		t.Fatalf("workspaceFileManifest = %#v, want manifest from output missing DoD repair", input[stepInputKeyWorkspaceFileManifest])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputDoesNotPromotePlanningReadOnlyFailureToStateChange(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-planning-state-contract")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeySoulStage: soulStagePlanning,
		},
		Output: map[string]any{
			"error": "invalid argument: state-changing planning llm step returned read-only executionGraph without patch output",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-planning-state-contract"}, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != nil || boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want planning read-only failure to stay open judgment without state-changing repair", input)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputRequiresValidationEvidenceForMissingDoDWithoutTarget(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-dod-gap-state-change")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/assessment"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
		},
		Output: map[string]any{
			"stage":               "llm_dod_assessment",
			failureOutputKeyClass: failureClassDoDCoverageMissing,
			"missingDoDItems": []map[string]any{
				{"item": "produce state-changing result", "state": "failed"},
			},
		},
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-repair-dod-gap-state-change",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveConcreteValidationEvidence || boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want missing DoD repair without target evidence to require validation evidence", input)
	}
	if len(mapSliceFromValue(input["missingDoDItems"])) != 1 {
		t.Fatalf("missingDoDItems = %+v, want missing DoD carried into repair input", input["missingDoDItems"])
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want validation evidence contract", required)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputDoesNotForceStateChangeForValidationFollowUpReadOnlyGraph(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-validation-read-only-graph")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up-graph-validation-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
		},
		Output: map[string]any{
			"error": "invalid argument: validation follow-up llm step returned read-only executionGraph without patch output",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-validation-read-only-graph"}, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] == "produce_state_changing_output" || boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want validation follow-up read-only graph repair not to force state-changing output", input)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputUsesEvidenceObjectiveForVerificationEvidenceGap(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-dod-evidence-gap")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
			"runtimeDiagnostics": map[string]any{
				"kind": "dod_evidence_quality_gap",
			},
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied":           false,
				"summary":             "Current resources exist, but concrete verification evidence is missing.",
				"verificationSummary": "No actual verification evidence was produced.",
				"items": []map[string]any{
					{"item": "deliver verified outcome", "state": "failed"},
				},
				"knownLimitations": []string{"not verified"},
				"nextActions":      []string{"run bounded verification"},
			},
			"missingDoDItems": []map[string]any{
				{"item": "deliver verified outcome", "state": "failed"},
			},
		},
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-repair-dod-evidence-gap",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveConcreteValidationEvidence || boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want concrete evidence objective without state-changing requirement", input)
	}
	if input["evidenceRepairRequired"] != true {
		t.Fatalf("evidenceRepairRequired = %v, want true", input["evidenceRepairRequired"])
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "concrete verification evidence") || !strings.Contains(guidance, "bounded executionGraph") {
		t.Fatalf("repairGuidance = %#v, want concrete evidence guidance", input["repairGuidance"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputUsesValidationEvidenceForCoverageGapWithoutTarget(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-dod-coverage-unclassified-gap")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "DoD coverage is missing.",
				"items": []map[string]any{
					{"item": "deliver accepted outcome", "state": "failed"},
				},
			},
			"missingDoDItems": []map[string]any{
				{"item": "deliver accepted outcome", "state": "failed"},
			},
		},
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-repair-dod-coverage-unclassified-gap",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveConcreteValidationEvidence || boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want validation evidence objective without state-changing output for coverage gap without implementation target", input)
	}
	if input["boundedValidationRequired"] != true || input["evidenceRepairRequired"] != true {
		t.Fatalf("repair input = %+v, want bounded validation evidence repair", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want need_validation", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != false ||
		required["mustProduceConcreteValidationEvidence"] != true ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want bounded validation evidence output only", required)
	}
	if !stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), "destructive_shell_approval") ||
		!strings.Contains(stringFromMap(required, "destructiveValidationApprovalRule"), "do not request approval") {
		t.Fatalf("requiredOutput = %+v, want destructive validation approval forbidden", required)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputEscalatesStateChangingEvidenceQualityGap(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-state-changing-evidence-quality")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/assessment"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
			"patchRepairContext": []map[string]any{{
				"targetRef":        "deliverable.txt",
				"exists":           true,
				"contentHash":      "hash-deliverable",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
			"runtimeDiagnostics": map[string]any{
				"kind":                        "dod_evidence_quality_gap",
				"stateChangingEvidenceNeeded": true,
				"missingDeliverableCoverage": []map[string]any{{
					"kind":                  "leaf",
					"item":                  "stateful_deliverable",
					"state":                 "failed",
					"source":                "evidenceKind",
					"sourceScope":           "deliverable",
					"sourceRequirementId":   "stateful_deliverable",
					"targetText":            "stateful_deliverable",
					"evidenceRefs":          []string{string(runID) + "/graph-001/read-only"},
					"acceptedStepKinds":     []string{"patch/apply"},
					"rejectedStepKinds":     []string{"file/read"},
					"rejectedEvidenceKind":  "file/read",
					"requiredEvidenceKinds": []string{"validation", "state_change"},
				}},
			},
		},
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-repair-state-changing-evidence-quality",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                    "stateful_deliverable",
							"scope":                 "deliverable",
							"requireDirectEvidence": true,
							"acceptedStepKinds":     []string{"patch/apply"},
							"rejectedStepKinds":     []string{"file/read"},
						}},
					},
				},
			},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true ||
		input["stateChangingEvidenceRepairRequired"] != true ||
		input[stepInputKeyAssessmentOnly] != nil ||
		input["boundedValidationRequired"] != nil {
		t.Fatalf("repair input = %+v, want state-changing evidence repair without assessment-only or validation-only mode", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		!boolValue(required["mustDeclareCoverageTargets"]) {
		t.Fatalf("requiredOutput = %+v, want state-changing coverage evidence contract", required)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIgnoresHistoricalTargetForCoverageAssessmentFailure(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "client.go"), []byte("package demo\n"), 0o644); err != nil {
		t.Fatalf("seed client.go: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-repair-dod-coverage-historical-target")
	runID := common.RunID("run-repair-dod-coverage-historical-target")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: deliverableDirectory,
		State:                common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: workspaceRepo,
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/provider-retry-assessment"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:       runtimeStageVerification,
			stepInputKeyRuntimeStageMode:   "strict_convergence",
			stepInputKeyRepairOfFailureSig: "class=tool_failed|kind=mcp/tool_call|failureTargets=client.go|error=historical tool failure",
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment|error=invalid argument: dodassessment is not satisfied",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "Coverage still lacks accepted validation evidence.",
				"items": []map[string]any{{
					"item":  "deliver verified behavior",
					"state": "failed",
				}},
			},
			"missingDoDItems": []map[string]any{{
				"item":  "deliver verified behavior",
				"state": "failed",
			}},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveConcreteValidationEvidence || boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want bounded validation evidence instead of historical target state-changing repair", input)
	}
	if targets := mapSliceFromValue(input["failureTargetRepairContext"]); len(targets) != 0 {
		t.Fatalf("failureTargetRepairContext = %+v, want no target from historical repairOfFailureSignature", targets)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if targets := mapSliceFromValue(pkg["targetSnapshots"]); len(targets) != 0 {
		t.Fatalf("repairActionPackage.targetSnapshots = %+v, want no historical target snapshots", targets)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if boolValue(required["mustIncludeStateChangingNode"]) ||
		boolValue(required["mustTargetFailureScope"]) ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want validation evidence contract without patch target scope", required)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputKeepsRepeatedValidationGapAsEvidenceWithoutTarget(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps:      stepRepo,
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-repeated-validation-only-gap")
	failedStepID := common.StepID(string(runID) + "/verify")
	signature := "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment"
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfStepID:        string(failedStepID),
			stepInputKeyRepairOfRootStepID:    string(failedStepID),
			stepInputKeyRepairOfFailureClass:  failureClassDoDCoverageMissing,
			stepInputKeyRepairOfFailureSig:    signature,
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: signature,
		},
	}); err != nil {
		t.Fatalf("seed prior repair step: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: signature,
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "Coverage is still missing after validation attempts.",
				"items": []map[string]any{
					{"item": "deliver implemented behavior", "state": "failed"},
				},
			},
			"missingDoDItems": []map[string]any{
				{"item": "deliver implemented behavior", "state": "failed"},
			},
		},
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-repair-repeated-validation-only-gap",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 2, common.ErrInvalidArgument)

	if input["validationRepairEscalatedToImplementation"] != nil || boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want no state-changing escalation without concrete implementation target evidence", input)
	}
	if input["repairObjective"] != repairObjectiveConcreteValidationEvidence || !boolValue(input["boundedValidationRequired"]) {
		t.Fatalf("repair input = %+v, want bounded validation evidence repair without target evidence", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true ||
		required["mustIncludeStateChangingNode"] == true ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want concrete validation evidence contract and no patch/apply", required)
	}
	if !stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), "destructive_shell_approval") ||
		!strings.Contains(stringFromMap(required, "destructiveValidationApprovalRule"), "do not request approval") {
		t.Fatalf("requiredOutput = %+v, want destructive validation approval forbidden", required)
	}
	if !strings.Contains(strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n"), "Do not return patch/apply unless a later validation failure provides concrete target refs") {
		t.Fatalf("repairGuidance = %+v, want no-target validation guidance", input["repairGuidance"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputEscalatesRepeatedValidationGapWithTargetEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps:      stepRepo,
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-repeated-validation-target-gap")
	failedStepID := common.StepID(string(runID) + "/verify")
	signature := "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment"
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfStepID:        string(failedStepID),
			stepInputKeyRepairOfRootStepID:    string(failedStepID),
			stepInputKeyRepairOfFailureClass:  failureClassDoDCoverageMissing,
			stepInputKeyRepairOfFailureSig:    signature,
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: signature,
		},
	}); err != nil {
		t.Fatalf("seed prior repair step: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
			"patchRepairContext": []map[string]any{{
				"targetRef":        "target.go",
				"exists":           true,
				"contentHash":      "hash-target",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: signature,
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "Coverage is still missing after validation attempts.",
				"items": []map[string]any{
					{"item": "deliver implemented behavior", "state": "failed"},
				},
			},
			"missingDoDItems": []map[string]any{
				{"item": "deliver implemented behavior", "state": "failed"},
			},
		},
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-repair-repeated-validation-target-gap",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 2, common.ErrInvalidArgument)

	if input["validationRepairEscalatedToImplementation"] != true ||
		input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true {
		t.Fatalf("repair input = %+v, want target-backed state-changing escalation", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want state-changing patch contract", required)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputEscalatesValidationGapWithCurrentTargetSnapshot(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "target.txt"), []byte("current target\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-repair-validation-current-target")
	runID := common.RunID("run-repair-validation-current-target")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: deliverableDirectory,
		State:                common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: workspaceRepo,
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			coverageWorkItemContextKey: map[string]any{
				"currentFocus": map[string]any{
					"id":                   "cwi-001",
					"status":               "open",
					"missingEvidenceKinds": []string{"validation"},
					"targetText":           "deliver verified behavior",
				},
			},
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment|failureTargets=target.txt",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "Verification is blocked by a target diagnostic.",
				"items": []map[string]any{{
					"item":  "deliver verified behavior",
					"state": "failed",
				}},
			},
			"missingDoDItems": []map[string]any{{
				"item":  "deliver verified behavior",
				"state": "failed",
			}},
			"stderr": "./target.txt:2:7: shape mismatch: MissingSymbol",
		},
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true {
		t.Fatalf("repair input = %+v, want target-backed validation gap escalated to state-changing repair", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
	if input["boundedValidationRequired"] != nil || input["evidenceRepairRequired"] != nil {
		t.Fatalf("repair input = %+v, want validation-only contract cleared after target evidence is available", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want state-changing patch contract", required)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputDemotesInheritedCoveragePatchContractToValidation(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-inherited-coverage-patch-contract")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-002-of-follow-up"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"stateChangingOutputRequired":     true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"coverageRepairRequired": true,
				"coverageTargetSet": []map[string]any{{
					"type":  "dod",
					"item":  "verify functional behavior",
					"state": "skipped",
				}},
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
				},
			},
			"missingDoDItems": []map[string]any{{
				"item":  "verify functional behavior",
				"state": "skipped",
			}},
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|error=invalid argument: state-changing llm repair step returned executiongraph without patch/apply",
			"error":                   "invalid argument: state-changing llm repair step returned executionGraph without patch/apply",
		},
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-repair-inherited-coverage-patch-contract",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 2, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveConcreteValidationEvidence || boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want inherited coverage-only patch contract demoted to validation evidence", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != false ||
		required["mustProduceConcreteValidationEvidence"] != true ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want no patch/apply for coverage-only validation gap", required)
	}
	if targets := mapSliceFromValue(input["coverageTargetSet"]); len(targets) != 1 {
		t.Fatalf("coverageTargetSet = %+v, want inherited coverage target", input["coverageTargetSet"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputSwitchesInheritedStateChangingRequirementToEvidenceAcquisitionWithoutTarget(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-state-contract-inherit")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/zz-llm-repair-1"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"stateChangingOutputRequired":     true,
			stepInputKeySoulStage:             soulStageExecution,
		},
		Output: map[string]any{
			"error": "state-changing llm repair step returned executionGraph instead of concrete patch output",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-state-contract-inherit"}, failedStep, 2, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveAcquireFailureEvidence ||
		input["stateChangingOutputRequired"] != nil ||
		input["concreteFailureEvidenceRequired"] != true {
		t.Fatalf("repair input = %+v, want inherited wrapper failure to acquire concrete target evidence", input)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputNarrowsRepeatedReadOnlyStateChangingLoopToPatchApply(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-repeated-read-only-patch-only")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"stateChangingOutputRequired":     true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"targetSnapshots": []map[string]any{{
					"targetRef":        "main.go",
					"exists":           true,
					"contentComplete":  true,
					"safeForFileWrite": true,
					"contentHash":      "hash-main",
					"content":          "package main\nfunc main() {}\n",
				}},
				"requiredOutput": stateChangingRepairRequiredOutput(),
			},
		},
		Output: map[string]any{
			outputKeyContractViolationType: contractViolationRepairGraphWithoutPatchApply,
			"stage":                        "llm_execution_graph_plan",
			failureOutputKeyClass:          failureClassRuntime,
			failureOutputKeySignature:      "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|failureTargets=main.go",
			"error":                        "invalid argument: state-changing llm repair step returned executionGraph without patch/apply",
		},
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-repair-repeated-read-only-patch-only",
		DoDID:           common.DoDID("dod"),
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 2, common.ErrInvalidArgument)

	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustReturnPatchApplyNode"]) {
		t.Fatalf("requiredOutput = %+v, want patch-apply-only narrowing marker", required)
	}
	if got := stringSliceFromValue(required["acceptedKinds"]); len(got) != 1 || got[0] != "patch/apply" {
		t.Fatalf("acceptedKinds = %+v, want [patch/apply]", got)
	}
	if !boolValue(required["mustUseCurrentTargetSnapshot"]) || !boolValue(required["mustPatchDirectlyFromProvidedSnapshot"]) {
		t.Fatalf("requiredOutput = %+v, want current target snapshot patch contract", required)
	}
	decisionContract := mapFromValue(mapFromValue(input["repairActionPackage"])["outputDecisionContract"])
	if decisionContract["requiredMode"] != "state_changing_patch_apply" {
		t.Fatalf("outputDecisionContract = %+v, want state_changing_patch_apply", decisionContract)
	}
	forbidden := strings.Join(stringSliceFromValue(decisionContract["forbiddenOutputs"]), "\n")
	if !strings.Contains(forbidden, "read_only_or_inspection_only_graph") ||
		!strings.Contains(forbidden, "shell_only_validation_for_state_change") {
		t.Fatalf("forbiddenOutputs = %q, want read-only and shell-only state-change forbiddance", forbidden)
	}
	joined := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(joined, "Return a concrete patch/apply node") {
		t.Fatalf("repairGuidance = %q, want explicit patch/apply narrowing", joined)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputUsesProgressFailureTargetSnapshotAfterReadOnlyWrapper(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-repair-progress-target-snapshot")
	runID := common.RunID("run-repair-progress-target-snapshot")
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "target.txt"), []byte("before\ncurrent\n"), 0644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	workspaceRepo := memory.NewWorkspaceRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "ws",
		DeliverableDirectory: deliverableDirectory,
		State:                common.WorkspaceStateRunning,
		ActiveRunID:          runID,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: workspaceRepo,
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"stateChangingOutputRequired":     true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"currentExecutionProgress": map[string]any{
				"lastObservedFailure": map[string]any{
					"kind":                    "patch/apply",
					"state":                   "failed",
					"failureClass":            failureClassPatchApply,
					"failureReason":           "conflict: replacement matched multiple locations",
					failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|node=patch-target|failureTargets=target.txt|error=conflict",
				},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
			},
		},
		Output: map[string]any{
			outputKeyContractViolationType: contractViolationRepairGraphWithoutPatchApply,
			"stage":                        "llm_execution_graph_plan",
			failureOutputKeyClass:          failureClassRuntime,
			failureOutputKeySignature:      "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|error=invalid argument: state-changing repair returned read-only graph",
			"error":                        "invalid argument: state-changing repair returned read-only graph",
		},
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 2, common.ErrInvalidArgument)

	targets := mapSliceFromValue(mapFromValue(input["repairActionPackage"])["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "target.txt" || !strings.Contains(stringFromMap(targets[0], "content"), "before\ncurrent") {
		t.Fatalf("repairActionPackage.targetSnapshots = %+v, want current target snapshot from progress failure", targets)
	}
	if len(mapSliceFromValue(input["failureTargetRepairContext"])) != 1 {
		t.Fatalf("failureTargetRepairContext = %+v, want current target snapshot", input["failureTargetRepairContext"])
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustUseCurrentTargetSnapshot"]) || !boolValue(required["mustReturnPatchApplyNode"]) {
		t.Fatalf("requiredOutput = %+v, want direct patch from current snapshot", required)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputForbidsRepeatedNoOpPatch(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-noop-patch")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"stateChangingOutputRequired":     true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"targetSnapshots": []map[string]any{{
					"targetRef":        "main.go",
					"exists":           true,
					"contentComplete":  true,
					"safeForFileWrite": true,
					"contentHash":      "hash-main",
					"content":          "package main\nfunc main() {}\n",
				}},
				"requiredOutput": stateChangingRepairRequiredOutput(),
			},
		},
		Output: map[string]any{
			outputKeyContractViolationType: contractViolationPatchContentRequired,
			"stage":                        "llm_execution_graph_plan",
			failureOutputKeyClass:          failureClassRuntime,
			failureOutputKeySignature:      "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|failureTargets=main.go",
			"error":                        "invalid argument: executionGraph patch/apply input invalid: patch text_replace oldText/newText must describe a substantive change",
		},
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-repair-noop-patch",
		DoDID:           common.DoDID("dod"),
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 2, common.ErrInvalidArgument)

	actionPackage := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(actionPackage["requiredOutput"])
	if !boolValue(required["mustAvoidNoOpPatch"]) || !boolValue(required["mustProduceSubstantivePatch"]) {
		t.Fatalf("requiredOutput = %+v, want no-op patch forbiddance", required)
	}
	forbiddenKinds := strings.Join(stringSliceFromValue(required["forbiddenKinds"]), "\n")
	if !strings.Contains(forbiddenKinds, "identical_text_replace") {
		t.Fatalf("forbiddenKinds = %q, want identical_text_replace", forbiddenKinds)
	}
	decisionContract := mapFromValue(actionPackage["outputDecisionContract"])
	forbiddenOutputs := strings.Join(stringSliceFromValue(decisionContract["forbiddenOutputs"]), "\n")
	if decisionContract["requiredMode"] != "state_changing_output" ||
		!strings.Contains(forbiddenOutputs, "no_op_text_replace") ||
		!strings.Contains(forbiddenOutputs, "identical_oldText_newText") {
		t.Fatalf("outputDecisionContract = %+v, want no-op text_replace forbiddance", decisionContract)
	}
	rejected := mapFromValue(actionPackage["lastRejectedOutputContract"])
	if rejected["forbiddenPattern"] != "no_op_text_replace" {
		t.Fatalf("lastRejectedOutputContract = %+v, want no_op_text_replace", rejected)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputCarriesResolvedGovernanceAndFailedGraph(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	runID := common.RunID("run-repair-governance-context")
	ticketID := common.TicketID("ticket-repair-governance-context")
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  "ws-repair-governance-context",
		RunID:        runID,
		StepID:       "run-repair-governance-context/block/failed",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Provide missing direction.",
		State:        common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-repair-governance-context",
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"answer": "confirmed direction"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}
	orch := &RuntimeOrchestrator{tickets: ticketRepo, resolutions: resolutionRepo, steps: memory.NewStepRepository()}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "failed", Kind: shellcommon.StepKindCommand, Input: map[string]any{"command": "tool"}},
	}}
	failedStep := &runmodel.Step{
		ID:    "run-repair-governance-context/block/failed",
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepOutputKeyExecutionGraph: graph,
			stepInputKeyGraphNodeID:     "failed",
		},
		Output: map[string]any{"error": "failed"},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-governance-context"}, failedStep, 1, errors.New("failed"))

	if _, ok := input[stepOutputKeyExecutionGraph]; ok {
		t.Fatalf("executionGraph leaked into repair input: %+v", input[stepOutputKeyExecutionGraph])
	}
	if _, ok := input["failedExecutionGraph"].(runtimemodel.BlockExecutionGraph); !ok {
		t.Fatalf("failedExecutionGraph = %#v, want compact failed graph", input["failedExecutionGraph"])
	}
	history := mapSliceFromValue(input["governanceHistory"])
	if len(history) != 1 {
		t.Fatalf("governanceHistory = %#v, want resolved decision", input["governanceHistory"])
	}
	payload := mapFromValue(history[0]["resolutionPayload"])
	if history[0]["resolutionType"] != decisioncommon.ResolutionTypeProvideMissingInfo || payload["answer"] != "confirmed direction" {
		t.Fatalf("governanceHistory[0] = %#v, want resolution payload", history[0])
	}
}
