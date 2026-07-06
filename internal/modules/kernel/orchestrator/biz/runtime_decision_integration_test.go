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
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOrchestrator_ResumeRunFailsResolvedDecisionWithoutContinuation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	workspaceID := common.WorkspaceID("ws-decision-no-continuation")
	runID := common.RunID("run-decision-no-continuation")
	stepID := common.StepID("run-decision-no-continuation/step-1")
	ticketID := common.TicketID("ticket-decision-no-continuation")
	hub := events.NewMemoryHub()
	var runStates []string
	unsubscribe := hub.Subscribe(runevents.TopicRunStateChanged, func(_ context.Context, envelope events.Envelope) error {
		if envelope.RunID == string(runID) {
			runStates = append(runStates, stringFromMap(envelope.Payload, events.PayloadKeyState))
		}
		return nil
	})
	defer unsubscribe()
	orch := NewRuntimeOrchestratorWithResolutions(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		hub,
	)

	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateSucceeded,
		Output: map[string]any{"stage": "blocked_on_decision"},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-decision-no-continuation"),
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed terminal state", runItem.State)
	}
	if runItem.BlockingTicketID != "" || !strings.Contains(runItem.FailureReason, common.ErrConflict.Error()) {
		t.Fatalf("run terminal details = %+v, want cleared blocker and conflict reason", runItem)
	}
	for _, state := range runStates {
		if state == string(common.RunStateActive) {
			t.Fatalf("run states = %v, want no intermediate running event for failed resolved-decision resume", runStates)
		}
	}
	if len(runStates) == 0 || runStates[len(runStates)-1] != string(common.RunStateFailed) {
		t.Fatalf("run states = %v, want terminal failed event", runStates)
	}
}

func TestRuntimeOrchestrator_CreatesDecisionTicketWithStructuredFields(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status":                "completed",
		"blockDecisionReason":   "need implementation direction",
		"blockDecisionKind":     "provide_missing_info",
		"blockDecisionQuestion": "Should the run call an external MCP tool?",
		"blockDecisionContext": map[string]any{
			"candidate": "mcp/tool_call",
		},
		"nextStep": "run-decision-structured/llm-after-decision",
		"nextKind": "llm/workspace_execution",
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
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memory.NewMemoryItemRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-decision-structured")
	runID := common.RunID("run-decision-structured")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStatePending}); err != nil {
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
		t.Fatalf("run = %+v, want blocked on decision", runItem)
	}
	ticket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.Reason != "need implementation direction" ||
		ticket.DecisionKind != "provide_missing_info" ||
		ticket.Question != "Should the run call an external MCP tool?" ||
		ticket.Context["candidate"] != "mcp/tool_call" {
		t.Fatalf("ticket = %+v, want structured decision fields", ticket)
	}
}

func TestRuntimeOrchestrator_ConvertsKnownReadableResourceDecisionToRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "main_test.go"), []byte("package main\n\nfunc TestKnownResource() {}\n"), 0o644); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}

	var repairInput map[string]any
	call := 0
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		call++
		if call == 1 {
			return map[string]any{
				"status":                "completed",
				"blockDecisionReason":   "need main_test.go content",
				"blockDecisionKind":     "provide_missing_info",
				"blockDecisionQuestion": "Please provide main_test.go.",
				"blockDecisionContext": map[string]any{
					"knownValues": map[string]any{
						"existingFiles": []string{"main.go", "main_test.go"},
					},
					"missingFields":       []string{"main_test.go content"},
					"requiresHumanFields": []string{"main_test.go content"},
				},
				"nextStep": "run-known-resource-decision/llm-after-decision",
				"nextKind": "llm/workspace_execution",
			}, nil
		}
		repairInput = cloneStepPayload(request.Input)
		return map[string]any{
			"status": "completed",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "completed after readable resource repair",
				"items": []map[string]any{
					{
						"item":         "runtime repair",
						"state":        "satisfied",
						"evidenceRefs": []string{string(request.StepID)},
					},
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
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-known-resource-decision")
	runID := common.RunID("run-known-resource-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-known-resource-decision"),
		State:           common.RunStatePending,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if call != 2 {
		t.Fatalf("runtime calls = %d, want initial decision plus repair", call)
	}
	tickets, err := ticketRepo.ListByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 0 {
		t.Fatalf("tickets = %+v, want no human decision for known readable resource", tickets)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if repairInput == nil || !boolValue(repairInput[stepInputKeyRuntimeRepairFollowUp]) {
		t.Fatalf("repair input = %+v, want runtime repair follow-up", repairInput)
	}
	resourceContext := mapSliceFromValue(repairInput["readableResourceRepairContext"])
	if len(resourceContext) != 1 ||
		resourceContext[0]["targetRef"] != "main_test.go" ||
		!strings.Contains(stringFromMap(resourceContext[0], "content"), "TestKnownResource") {
		t.Fatalf("readableResourceRepairContext = %+v, want main_test.go content", resourceContext)
	}
}

func TestRuntimeOrchestrator_ConvertsQuestionOnlyReadableResourceDecisionToRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	deliverableDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(deliverableDirectory, "config"), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(deliverableDirectory, "handler"), 0o755); err != nil {
		t.Fatalf("create handler dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "config", "config.go"), []byte("package config\n\nconst AppName = \"workorch-test\"\n"), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "handler", "handler.go"), []byte("package handler\n\nfunc Route() string { return \"ok\" }\n"), 0o644); err != nil {
		t.Fatalf("write handler file: %v", err)
	}

	var repairInput map[string]any
	call := 0
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		call++
		if call == 1 {
			return map[string]any{
				"status":                "completed",
				"blockDecisionReason":   "tests need actual file contents before patching",
				"blockDecisionKind":     "provide_missing_info",
				"blockDecisionQuestion": "请提供 config/config.go 和 handler/handler.go 的完整内容。",
				"nextStep":              "run-question-readable-decision/llm-after-decision",
				"nextKind":              "llm/workspace_execution",
			}, nil
		}
		repairInput = cloneStepPayload(request.Input)
		return map[string]any{
			"status": "completed",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "completed after question-only readable resource repair",
				"items": []map[string]any{{
					"item":         "runtime repair",
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
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-question-readable-decision")
	runID := common.RunID("run-question-readable-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-question-readable-decision"),
		State:           common.RunStatePending,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if call != 2 {
		t.Fatalf("runtime calls = %d, want initial decision plus repair", call)
	}
	tickets, err := ticketRepo.ListByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 0 {
		t.Fatalf("tickets = %+v, want no human decision for workspace-readable files", tickets)
	}
	resourceContext := mapSliceFromValue(repairInput["readableResourceRepairContext"])
	if len(resourceContext) != 2 {
		t.Fatalf("readableResourceRepairContext = %+v, want two file snapshots", resourceContext)
	}
	contentsByTarget := map[string]string{}
	for _, item := range resourceContext {
		contentsByTarget[stringFromMap(item, "targetRef")] = stringFromMap(item, "content")
	}
	if !strings.Contains(contentsByTarget["config/config.go"], "AppName") ||
		!strings.Contains(contentsByTarget["handler/handler.go"], "Route") {
		t.Fatalf("readableResourceRepairContext = %+v, want config and handler snapshots", resourceContext)
	}
}

func TestRuntimeRepairInputPromotesReadableResourceDecisionIntoStateChangingTargetSnapshot(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	deliverableDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(deliverableDirectory, "target"), 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "target", "resource.txt"), []byte("current target content\n"), 0o644); err != nil {
		t.Fatalf("write target resource: %v", err)
	}

	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-readable-state-changing")
	runID := common.RunID("run-readable-state-changing")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive, InitialStepKind: llmWorkspaceExecutionStepKind}
	failedStep := &runmodel.Step{
		ID:    "run-readable-state-changing/repair-001",
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"status":                               "failed",
			"error":                                "decision requested a readable resource",
			"continuableDecisionConvertedToRepair": true,
			"convertedDecisionGate":                "convergence_contract",
			"convertedDecision": map[string]any{
				"reason":   "target/resource.txt is needed before producing a precise state-changing artifact",
				"question": "Please provide target/resource.txt current content.",
				"context": map[string]any{
					"knownValues": map[string]any{
						"targetRef": "target/resource.txt",
					},
					"missingFields":       []string{"target/resource.txt current content"},
					"requiresHumanFields": []string{"target/resource.txt current content"},
				},
			},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 2, common.ErrInvalidArgument)

	resourceContext := mapSliceFromValue(input["readableResourceRepairContext"])
	if len(resourceContext) != 1 ||
		resourceContext[0]["targetRef"] != "target/resource.txt" ||
		!strings.Contains(stringFromMap(resourceContext[0], "content"), "current target content") {
		t.Fatalf("readableResourceRepairContext = %+v, want current target resource snapshot", resourceContext)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	targets := mapSliceFromValue(actionPackage["targetSnapshots"])
	if len(targets) != 1 ||
		targets[0]["targetRef"] != "target/resource.txt" ||
		!strings.Contains(stringFromMap(targets[0], "content"), "current target content") {
		t.Fatalf("repairActionPackage.targetSnapshots = %+v, want readable resource promoted to actionable target snapshot", actionPackage["targetSnapshots"])
	}
	if required := mapFromValue(actionPackage["requiredOutput"]); required["mustIncludeStateChangingNode"] != true {
		t.Fatalf("requiredOutput = %+v, want state-changing contract", required)
	}
}

func TestContinuableDecisionShouldRepairRejectsMechanicalTargetChoiceBlock(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-mechanical-decision/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			stepInputKeySoulStage:         soulStageExecution,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
				"targetSnapshots": []map[string]any{{
					"targetRef":        "main.go",
					"exists":           true,
					"contentHash":      "hash-main",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
		},
	}
	spec := runtimeDecisionBlockSpec{
		Reason:       "need implementation direction",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Should the repair patch this target or rewrite the adjacent target?",
	}

	if !continuableDecisionShouldRepair(step, spec) {
		t.Fatalf("expected mechanically repairable target-choice decision to be converted to repair")
	}
}

func TestContinuableDecisionShouldRepairConvertsConcreteValidationChoice(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-validation-dependency/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			"repairObjective":            repairObjectiveConcreteValidationEvidence,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds":                         []string{"shell/command", "mcp/tool_call"},
					"mustProduceConcreteValidationEvidence": true,
				},
			},
		},
	}
	spec := runtimeDecisionBlockSpec{
		Reason:       "validation decision did not return required bounded evidence",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "choose whether to continue with an equivalent bounded validation action",
		Context: map[string]any{
			"candidateValues": map[string]any{
				"provide_input":  "provide additional input",
				"use_equivalent": "continue with equivalent bounded validation",
			},
			"knownValues": map[string]any{
				"validation_constraint": "direct evidence is not yet available",
			},
			"missingFields":       []any{"user choice"},
			"requiresHumanFields": []any{"user choice"},
		},
	}

	if !continuableDecisionShouldRepair(step, spec) {
		t.Fatalf("expected concrete validation choice to be converted to repair")
	}
}

func TestContinuableDecisionShouldRepairKeepsRequiredExternalEvidenceBlocked(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-real-dependency/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			"repairObjective":            repairObjectiveConcreteValidationEvidence,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds":                         []string{"shell/command", "mcp/tool_call"},
					"mustProduceConcreteValidationEvidence": true,
				},
			},
		},
	}
	spec := runtimeDecisionBlockSpec{
		Reason:       "validation requires an external dependency declared by the acceptance contract",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "provide required external dependency input",
		Context: map[string]any{
			decisionContextKeyScope: "external_dependency",
			"knownValues": map[string]any{
				"requiresExternalEvidence": true,
			},
			"missingFields":       []any{"requiredExternalInput"},
			"requiresHumanFields": []any{"requiredExternalInput"},
		},
	}

	if continuableDecisionShouldRepair(step, spec) {
		t.Fatalf("expected required external evidence decision to remain blocked")
	}
}

func TestContinuableDecisionShouldRepairKeepsRuntimeCannotContinueHumanDependencyBlocked(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-human-runtime-dependency/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			"repairObjective":            repairObjectiveConcreteValidationEvidence,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds":                         []string{"shell/command", "mcp/tool_call"},
					"mustProduceConcreteValidationEvidence": true,
				},
			},
		},
	}
	spec := runtimeDecisionBlockSpec{
		Reason:       "runtime cannot continue without governed input",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "provide missing governed runtime input",
		Context: map[string]any{
			"runtimeCanContinue":  false,
			"missingFields":       []any{"dependency_config"},
			"requiresHumanFields": []any{"dependency_config"},
			"knownValues": map[string]any{
				"failureClass": "external_dependency_unavailable",
			},
		},
	}

	if continuableDecisionShouldRepair(step, spec) {
		t.Fatalf("expected runtimeCanContinue=false human dependency decision to remain blocked")
	}
}

func TestContinuableDecisionShouldRepairIgnoresFreeTextWordsWithoutStructuredBlocker(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-neutrality-text/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			"repairObjective":            repairObjectiveConcreteValidationEvidence,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds":                         []string{"shell/command", "mcp/tool_call"},
					"mustProduceConcreteValidationEvidence": true,
				},
			},
		},
	}
	spec := runtimeDecisionBlockSpec{
		Reason:       "validation mentions api key, network, mock server, fixture, stub, gateway, go build, and test failure",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "should runtime use a mock server or request credentials?",
		Context: map[string]any{
			"knownValues": map[string]any{
				"summary": "the free-text contains technology and business words but no structured blocker contract",
			},
			"missingFields":       []any{"user choice"},
			"requiresHumanFields": []any{"user choice"},
		},
	}

	if !continuableDecisionShouldRepair(step, spec) {
		t.Fatalf("expected free-text words without structured blocker to remain continuable")
	}
}

func TestContinuableDecisionShouldRepairRuntimeCannotContinueWithConcreteTargetStaysRepairable(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-runtime-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			stepInputKeySoulStage:         soulStageExecution,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
				"targetSnapshots": []map[string]any{{
					"targetRef":        "main.go",
					"exists":           true,
					"contentHash":      "hash-main",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
		},
	}
	spec := runtimeDecisionBlockSpec{
		Reason:       "runtime needs to continue with the concrete target snapshot",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "continue with the visible target",
		Context: map[string]any{
			"runtimeCanContinue":  false,
			"missingFields":       []any{"target confirmation"},
			"requiresHumanFields": []any{"target confirmation"},
			"candidateValues": map[string]any{
				"repairTargetSnapshot.path": "main.go",
			},
		},
	}

	if !continuableDecisionShouldRepair(step, spec) {
		t.Fatalf("expected runtimeCanContinue=false decision with concrete workspace target to remain repairable")
	}
}

func TestContinuableDecisionShouldRepairStructuredBlockerOverridesBenignFreeText(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-neutrality-structured/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			"repairObjective":            repairObjectiveConcreteValidationEvidence,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"acceptedKinds":                         []string{"shell/command", "mcp/tool_call"},
					"mustProduceConcreteValidationEvidence": true,
				},
			},
		},
	}
	spec := runtimeDecisionBlockSpec{
		Reason:       "plain text sounds easy to continue",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "continue with bounded validation",
		Context: map[string]any{
			decisionContextKeyScope: "external_dependency",
			"knownValues": map[string]any{
				"requiresExternalEvidence": true,
				"summary":                  "free-text says continue, but structured blocker requires governance",
			},
		},
	}

	if continuableDecisionShouldRepair(step, spec) {
		t.Fatalf("expected structured blocker to override benign free-text")
	}
}

func TestContinuableDecisionShouldRepairKeepsUngroundedDecisionBlocked(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-ungrounded-decision/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			stepInputKeySoulStage:         soulStageExecution,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
			},
		},
	}
	spec := runtimeDecisionBlockSpec{
		Reason:       "missing implementation target",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Please provide the implementation target.",
	}

	if continuableDecisionShouldRepair(step, spec) {
		t.Fatalf("expected ungrounded decision without concrete target evidence to remain blocked")
	}
}

func TestRuntimeOrchestrator_ResumeRunAutoRepairsOpenKnownReadableResourceDecision(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "main_test.go"), []byte("package main\n\nfunc TestReadableTicketResource(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}

	var repairInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		repairInput = cloneStepPayload(request.Input)
		return map[string]any{
			"status": "completed",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "completed after auto readable-resource repair",
				"items": []map[string]any{{
					"item":         "readable resource used",
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
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{Results: memory.NewResultRepository()},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-open-known-resource-decision")
	runID := common.RunID("run-open-known-resource-decision")
	stepID := common.StepID("run-open-known-resource-decision/follow-up-graph-001")
	ticketID := common.TicketID("ticket-open-known-resource-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            common.DoDID("dod-open-known-resource-decision"),
		State:            common.RunStateBlockedOnDecision,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			stepInputKeySoulStage:        soulStageValidation,
			"graphFollowUp":              true,
			"completionFollowUp":         true,
		},
		Output: map[string]any{
			"status":                "completed",
			"stage":                 "llm_decision_required",
			"blockDecisionReason":   "need main_test.go content",
			"blockDecisionKind":     decisioncommon.ResolutionTypeProvideMissingInfo,
			"blockDecisionQuestion": "Please provide main_test.go.",
			"blockDecisionContext": map[string]any{
				"knownValues": map[string]any{
					"existingFiles": []string{"go.mod", "config.yaml", "main.go"},
				},
				"missingFields":       []string{"main_test.go content"},
				"requiresHumanFields": []string{"main_test.go content"},
			},
		},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		StepID:       stepID,
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Reason:       "need main_test.go content",
		Question:     "Please provide main_test.go.",
		Context: map[string]any{
			"knownValues": map[string]any{
				"existingFiles": []string{"go.mod", "config.yaml", "main.go"},
			},
			"missingFields":       []string{"main_test.go content"},
			"requiresHumanFields": []string{"main_test.go content"},
		},
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted || runItem.BlockingTicketID != "" {
		t.Fatalf("run = %+v, want completed without blocker", runItem)
	}
	ticket, err := ticketRepo.Get(ctx, ticketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateSuperseded {
		t.Fatalf("ticket state = %s, want dismissed auto-repairable decision", ticket.State)
	}
	if repairInput == nil || !boolValue(repairInput[stepInputKeyRuntimeRepairFollowUp]) {
		t.Fatalf("repair input = %+v, want runtime repair", repairInput)
	}
	resourceContext := mapSliceFromValue(repairInput["readableResourceRepairContext"])
	if len(resourceContext) != 1 ||
		resourceContext[0]["targetRef"] != "main_test.go" ||
		!strings.Contains(stringFromMap(resourceContext[0], "content"), "TestReadableTicketResource") {
		t.Fatalf("readableResourceRepairContext = %+v, want current main_test.go content", resourceContext)
	}
}

func TestRuntimeOrchestrator_DecisionBlockResultPolicyRejectedWritesResult(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status":                "completed",
		"summary":               "cannot continue without a concrete target",
		"blockDecisionReason":   "missing implementation target",
		"blockDecisionKind":     "provide_missing_info",
		"blockDecisionQuestion": "Please provide the implementation target.",
		"blockDecisionContext": map[string]any{
			decisionContextKeyResultPolicy: decisionContextResultPolicyRejected,
			"missingFields":                []string{"target"},
		},
		"nextStep": "run-decision-result-policy/llm-after-decision",
		"nextKind": "llm/workspace_execution",
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

	workspaceID := common.WorkspaceID("ws-decision-result-policy")
	runID := common.RunID("run-decision-result-policy")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-decision-result-policy"),
		DoDID:           common.DoDID("dod-decision-result-policy"),
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
		t.Fatalf("run = %+v, want blocked on decision", runItem)
	}
	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected || result.DoDAssessment.Satisfied {
		t.Fatalf("result = %+v, want rejected result for decision result policy", result)
	}
}

func TestRuntimeOrchestrator_FailsLLMRunCompletionWithoutDoDAssessment(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "completed"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-dod-gate-missing")
	runID := common.RunID("run-dod-gate-missing")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-1"),
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close as failed without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed || !strings.Contains(runItem.FailureReason, "dodAssessment is required") {
		t.Fatalf("run = %+v, want failed DoD assessment gate", runItem)
	}
	step, err := stepRepo.Get(ctx, common.StepID("run-dod-gate-missing/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed || !strings.Contains(step.Output["error"].(string), "dodAssessment is required") {
		t.Fatalf("step = %+v, want failed DoD assessment gate", step)
	}
}

func TestRuntimeOrchestrator_FailsContinueWithoutActionableContinuationWithoutDoDGate(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		output: map[string]any{
			"status": "completed",
			"llmDecision": map[string]any{
				"completionState": "continue",
			},
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-continue-no-action")
	runID := common.RunID("run-continue-no-action")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close as failed without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed || !strings.Contains(runItem.FailureReason, "completionState continue has no actionable continuation") {
		t.Fatalf("run = %+v, want failed for continue without actionable continuation", runItem)
	}
	step, err := stepRepo.Get(ctx, common.StepID("run-continue-no-action/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed || !strings.Contains(step.Output["error"].(string), "completionState continue has no actionable continuation") {
		t.Fatalf("step = %+v, want failed completion continuation gate", step)
	}
}

func TestRuntimeOrchestrator_DispatchesValidationFollowUpForContinueWithoutActionableContinuation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var followUpInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if boolValue(request.Input[stepInputKeyNoActionableFollowUp]) {
			followUpInput = request.Input
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "current evidence satisfies the DoD",
					"items": []map[string]any{
						{
							"item":         "goal completed",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		}
		return map[string]any{
			"status": "completed",
			"llmDecision": map[string]any{
				"completionState": "continue",
			},
		}, nil
	}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-continue-follow-up")
	runID := common.RunID("run-continue-follow-up")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-1"),
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
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
		t.Fatalf("run = %+v, want completed after validation follow-up", runItem)
	}
	if !boolValue(followUpInput[stepInputKeyNoActionableFollowUp]) {
		t.Fatalf("followUpInput = %+v, want no-actionable continuation marker", followUpInput)
	}
	if followUpInput["previousCompletionState"] != "continue" {
		t.Fatalf("followUpInput previousCompletionState = %#v, want continue", followUpInput["previousCompletionState"])
	}
	if evidence := mapSliceFromValue(followUpInput[stepInputKeyRunEvidenceSummary]); len(evidence) == 0 {
		t.Fatalf("followUpInput runEvidenceSummary = %#v, want prior evidence", followUpInput[stepInputKeyRunEvidenceSummary])
	}
}

func TestNoActionableContinuationFailedAssessmentBuildsCoverageRepairInput(t *testing.T) {
	runID := common.RunID("run-failed-assessment-coverage-follow-up")
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-001/004-assess"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateSucceeded,
		Input: map[string]any{},
		Output: map[string]any{
			"status": "completed",
			stepOutputKeyLLMDecision: map[string]any{
				"completionState": "failed",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied": false,
					"summary":   "coverage gaps remain",
					"items": []map[string]any{{
						"item":  "primary acceptance dimension",
						"state": "satisfied",
					}},
					"goalCoverage": map[string]any{
						"satisfied": false,
						"items": []map[string]any{{
							"goalRequirementId": "goal-req-001-target",
							"item":              "target requirement is not covered",
							"state":             "failed",
							"reason":            "no direct evidence closes the requirement",
							"nextActions":       []string{"collect bounded evidence for target requirement"},
						}},
					},
					"deliverableCoverage": map[string]any{
						"satisfied": false,
						"items": []map[string]any{{
							"id":          "deliverable-target",
							"state":       "failed",
							"reason":      "deliverable evidence is missing",
							"nextActions": []string{"produce deliverable evidence"},
						}},
					},
				},
			},
		},
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-failed-assessment-coverage-follow-up", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}

	if shouldDispatchNoActionableContinuationFollowUp(run, step, step.Output) {
		t.Fatalf("failed assessment with actionable coverage gaps should enter runtime repair directly")
	}
	input := noActionableContinuationFollowUpStepInput(run, step)
	if !boolValue(input["actionableFailedAssessmentContinuation"]) || input["repairObjective"] != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("input = %+v, want concrete evidence repair for actionable failed assessment", input)
	}
	if missing := mapSliceFromValue(input["missingGoalCoverage"]); len(missing) != 1 || missing[0]["goalRequirementId"] != "goal-req-001-target" {
		t.Fatalf("missingGoalCoverage = %+v, want goal coverage from failed assessment", input["missingGoalCoverage"])
	}
	if missing := mapSliceFromValue(input["missingDeliverableCoverage"]); len(missing) != 1 || missing[0]["item"] != "deliverable-target" {
		t.Fatalf("missingDeliverableCoverage = %+v, want deliverable coverage from failed assessment", input["missingDeliverableCoverage"])
	}
	targets := mapSliceFromValue(input["coverageTargetSet"])
	if len(targets) != 2 {
		t.Fatalf("coverageTargetSet = %+v, want goal and deliverable targets", input["coverageTargetSet"])
	}
	if targets[0]["kind"] != "goal" || targets[0]["goalRequirementId"] != "goal-req-001-target" {
		t.Fatalf("coverageTargetSet[0] = %+v, want goal target", targets[0])
	}
	if targets[1]["kind"] != "deliverable" || targets[1]["targetText"] != "deliverable-target" {
		t.Fatalf("coverageTargetSet[1] = %+v, want deliverable target", targets[1])
	}
}

func TestNoActionableContinuationPreservesConcreteValidationRepairContract(t *testing.T) {
	runID := common.RunID("run-validation-contract-continuation")
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageVerification,
			"repairObjective":           repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired": true,
			"evidenceRepairRequired":    true,
			"coverageTargetSet": []map[string]any{{
				"kind":       "dod",
				"targetText": "verify accepted behavior",
			}},
			"convergenceContract": map[string]any{
				"state":           convergenceStateNeedValidation,
				"repairObjective": repairObjectiveConcreteValidationEvidence,
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustProduceConcreteValidationEvidence": true,
					"acceptedKinds":                         []string{"shell/command", "mcp/tool_call"},
					"forbiddenKinds":                        []string{"patch/apply"},
				},
			},
		},
		Output: map[string]any{
			"status": "completed",
			stepOutputKeyLLMDecision: map[string]any{
				"completionState": "failed",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied": false,
					"summary":   "verification evidence is still missing",
					"items": []map[string]any{{
						"item":  "verify accepted behavior",
						"state": "failed",
					}},
				},
			},
		},
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-validation-contract-continuation", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}

	input := noActionableContinuationFollowUpStepInput(run, step)

	if input["repairObjective"] != repairObjectiveConcreteValidationEvidence ||
		input["boundedValidationRequired"] != true ||
		input["evidenceRepairRequired"] != true {
		t.Fatalf("input = %+v, want concrete validation repair contract preserved", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want need_validation", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "shell/command") ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want validation evidence contract only", required)
	}
	if targets := mapSliceFromValue(input["coverageTargetSet"]); len(targets) != 1 || targets[0]["targetText"] != "verify accepted behavior" {
		t.Fatalf("coverageTargetSet = %+v, want inherited coverage target", input["coverageTargetSet"])
	}
}

func TestNoActionableContinuationPreservesStateChangingRepairContract(t *testing.T) {
	runID := common.RunID("run-state-changing-contract-continuation")
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"coverageTargetSet": []map[string]any{{
				"kind":       "dod",
				"targetText": "close target behavior",
			}},
			"convergenceContract": map[string]any{
				"state":                       convergenceStateNeedStateChange,
				"repairObjective":             repairObjectiveStateChangingOutput,
				"stateChangingOutputRequired": true,
			},
			"repairActionPackage": map[string]any{
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target.txt",
					"exists":           true,
					"contentHash":      "hash-target",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply", "mcp/tool_call"},
				},
			},
		},
		Output: map[string]any{
			"status": "completed",
			stepOutputKeyLLMDecision: map[string]any{
				"completionState":                  "blocked_on_decision",
				stepOutputKeyBlockDecisionReason:   "need stage choice",
				stepOutputKeyBlockDecisionQuestion: "Should the runtime switch stage?",
			},
		},
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-state-changing-contract-continuation", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}

	input := noActionableContinuationFollowUpStepInput(run, step)

	if input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true ||
		input[stepInputKeyRuntimeStage] != runtimeStageRepair ||
		input[stepInputKeySoulStage] != soulStageExecution {
		t.Fatalf("input = %+v, want state-changing repair contract preserved", input)
	}
	if boolValue(input["boundedValidationRequired"]) || boolValue(input["evidenceRepairRequired"]) {
		t.Fatalf("input = %+v, want no validation-only downgrade", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want state-changing output contract", required)
	}
}

func TestRuntimeOrchestrator_DispatchStepDoesNotOverwriteTerminalStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "completed"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, memory.NewDecisionTicketRepository(), memory.NewApprovalRepository(), events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-terminal-step-dispatch")
	runID := common.RunID("run-terminal-step-dispatch")
	stepID := common.StepID("run-terminal-step-dispatch/step-1")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive, InitialStepKind: llmWorkspaceExecutionStepKind}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateSucceeded,
		Input:  map[string]any{"original": true},
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	err := orch.dispatchStepWithWorkspace(ctx, run, stepID, llmWorkspaceExecutionStepKind, map[string]any{"replacement": true}, workspace)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("dispatch terminal step err = %v, want conflict", err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %+v, want no terminal step re-execution", runtime.calls)
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateSucceeded || step.Input["original"] != true || step.Input["replacement"] == true {
		t.Fatalf("step = %+v, want original completed step preserved", step)
	}
}

func TestRuntimeOrchestrator_DirectNextStepUsesUniqueIDWhenTerminalStepExists(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceID := common.WorkspaceID("ws-next-step-unique")
	runID := common.RunID("run-next-step-unique")
	var dispatchedShell bool
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch {
		case request.StepID == common.StepID(runID+"/step-1"):
			return map[string]any{
				"status":   "completed",
				"nextStep": string(runID) + "/shell-1",
				"nextKind": shellcommon.StepKindCommand,
				"nextInput": map[string]any{
					"command": "sh",
					"args":    []string{"-c", "echo ok"},
				},
			}, nil
		case request.StepID == common.StepID(runID+"/shell-1-2"):
			dispatchedShell = true
			return map[string]any{
				"status": "completed",
				"stdout": "ok\n",
			}, nil
		case boolValue(request.Input["completionFollowUp"]):
			return map[string]any{
				"status": "completed",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied": true,
					"items": []map[string]any{
						{"item": "verified", "state": "satisfied", "evidenceRefs": []string{string(request.StepID)}},
					},
				},
			}, nil
		default:
			return map[string]any{
				"status": "completed",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied": true,
					"items": []map[string]any{
						{"item": "verified", "state": "satisfied", "evidenceRefs": []string{string(request.StepID)}},
					},
				},
			}, nil
		}
	}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, memory.NewDecisionTicketRepository(), memory.NewApprovalRepository(), events.NewMemoryHub())
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, DoDID: "dod-next-step-unique", State: common.RunStateActive, InitialStepKind: llmWorkspaceExecutionStepKind}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     common.StepID(runID + "/shell-1"),
		RunID:  runID,
		Kind:   shellcommon.StepKindCommand,
		State:  common.StepStateFailed,
		Input:  map[string]any{"command": "go"},
		Output: map[string]any{"status": "failed"},
	}); err != nil {
		t.Fatalf("seed terminal shell step: %v", err)
	}

	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, DoDID: "dod-next-step-unique", State: common.RunStateActive, InitialStepKind: llmWorkspaceExecutionStepKind}
	if err := orch.dispatchStepWithWorkspace(ctx, run, common.StepID(runID+"/step-1"), llmWorkspaceExecutionStepKind, map[string]any{}, nil); err != nil {
		t.Fatalf("dispatch step: %v; calls=%+v", err, runtime.calls)
	}
	if !dispatchedShell {
		t.Fatalf("expected unique follow-up shell step to be dispatched; calls=%+v", runtime.calls)
	}
	if _, err := stepRepo.Get(ctx, common.StepID(runID+"/shell-1-2")); err != nil {
		t.Fatalf("get unique shell step: %v", err)
	}
}

func TestRuntimeOrchestrator_DirectNextLLMAliasCanonicalizesToWorkspaceExecution(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceID := common.WorkspaceID("ws-next-llm-alias")
	runID := common.RunID("run-next-llm-alias")
	runtime := &requestCapturingRuntimeAdapter{
		fn: func(request runtimemodel.StepExecutionRequest) map[string]any {
			switch request.StepID {
			case common.StepID(runID + "/step-1"):
				return map[string]any{
					"status":   "completed",
					"nextStep": string(runID) + "/llm-2",
					"nextKind": llmcommon.StepKindWorkspacePlan,
				}
			case common.StepID(runID + "/llm-2"):
				return map[string]any{
					"status": "completed",
				}
			default:
				return map[string]any{
					"status": "completed",
				}
			}
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, memory.NewDecisionTicketRepository(), memory.NewApprovalRepository(), events.NewMemoryHub())

	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		State:           common.RunStatePending,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if len(runtime.requests) < 2 {
		t.Fatalf("runtime requests = %+v, want direct llm alias continuation dispatched", runtime.requests)
	}
	second := runtime.requests[1]
	if second.StepID != common.StepID(runID+"/llm-2") || second.Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("second request = %+v, want canonical llm/workspace_execution continuation", second)
	}
	if mapFromValue(second.Input[stepInputKeyExecutionContextPackage]) == nil {
		t.Fatalf("second request input = %+v, want enriched LLM execution context package", second.Input)
	}
	step, err := stepRepo.Get(ctx, common.StepID(runID+"/llm-2"))
	if err != nil {
		t.Fatalf("get second step: %v", err)
	}
	if step.Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("stored step kind = %q, want canonical llm/workspace_execution", step.Kind)
	}
}

func TestRuntimeOrchestrator_RepairsRepeatedContinueWithoutActionableContinuation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var repairSeen bool
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			repairSeen = true
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "repair completed validation",
					"items": []map[string]any{
						{
							"item":         "goal completed",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		}
		return map[string]any{
			"status": "completed",
			"llmDecision": map[string]any{
				"completionState": "continue",
			},
		}, nil
	}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-repeated-continue-repair")
	runID := common.RunID("run-repeated-continue-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-1"),
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	if !repairSeen {
		t.Fatalf("runtime repair follow-up was not dispatched after repeated no-actionable continuation")
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want completed after repair", runItem)
	}
}

func TestRuntimeOrchestrator_GraphBatchFailedAssessmentRoutesToRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var repairInput map[string]any
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "patch-alpha", Kind: "graph/mock"},
		{ID: "verify-beta", Kind: "graph/mock"},
	}}
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			repairInput = request.Input
			return map[string]any{
				"status": "completed",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied": true,
					"summary":   "repair closed missing coverage",
					"items": []map[string]any{{
						"item":         "missing target is covered",
						"state":        "satisfied",
						"evidenceRefs": []string{string(request.StepID)},
					}},
				},
			}, nil
		}
		switch request.StepID {
		case "run-graph-batch-failed-assessment/step-1":
			return map[string]any{
				"status":                    "completed",
				stepOutputKeyExecutionGraph: graph,
			}, nil
		case graphScopedStepID("run-graph-batch-failed-assessment", 1, 1, "patch-alpha"):
			return map[string]any{"status": "completed"}, nil
		case graphScopedStepID("run-graph-batch-failed-assessment", 1, 2, "verify-beta"):
			return map[string]any{
				"status": "completed",
				stepOutputKeyLLMDecision: map[string]any{
					"completionState": "failed",
					stepOutputKeyDoDAssessment: map[string]any{
						"satisfied": false,
						"summary":   "missing target needs follow-up repair",
						"items": []map[string]any{{
							"item":  "missing target is covered",
							"state": "failed",
						}},
						"nextActions": []string{"repair missing target", "rerun bounded verification"},
					},
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected step %s", request.StepID)
		}
	}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-graph-batch-failed-assessment")
	runID := common.RunID("run-graph-batch-failed-assessment")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-graph-batch-failed-assessment"),
		State:           common.RunStatePending,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	if repairInput == nil {
		t.Fatalf("runtime repair follow-up was not dispatched for graph batch failed assessment")
	}
	if missing := mapSliceFromValue(repairInput["missingDoDItems"]); len(missing) != 1 || missing[0]["item"] != "missing target is covered" {
		t.Fatalf("repairInput missingDoDItems = %+v, want failed assessment item", repairInput["missingDoDItems"])
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want completed after graph batch repair", runItem)
	}
}

func TestRuntimeOrchestrator_RepairsInconclusiveSatisfiedDoDAssessmentBeforeEscalation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	validationRepo := memory.NewValidationResultRepository()
	var repairInputs []map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			repairInputs = append(repairInputs, request.Input)
		}
		return map[string]any{
			"status":          "completed",
			"evidenceQuality": string(common.EvidenceQualityInconclusive),
			"evidenceReason":  "runtime adapter reported inconclusive evidence",
			"zeroWork":        true,
			"evidenceTags":    []string{"validation", "zero_work"},
			"dodAssessment": map[string]any{
				"satisfied":           true,
				"summary":             "verification was inconclusive",
				"goalOutcome":         "done",
				"verificationSummary": "runtime adapter reported inconclusive evidence",
				"items": []map[string]any{
					{
						"item":         "verification passes",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-dod-weak-gate/step-1"},
					},
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
		RuntimeEvidenceRepositories{Results: resultRepo, ValidationResults: validationRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-dod-weak-gate")
	runID := common.RunID("run-dod-weak-gate")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-weak"),
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should repair and escalate without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed ||
		!strings.Contains(runItem.FailureReason, "satisfied dodAssessment item") {
		t.Fatalf("run = %+v, want failed after repair returned non-actionable weak assessment", runItem)
	}
	if len(repairInputs) != 1 {
		t.Fatalf("repair inputs = %d, want one non-actionable repair assessment", len(repairInputs))
	}
	if repairInputs[0]["failedStepId"] == "" || repairInputs[0]["failedStepOutput"] == nil {
		t.Fatalf("first repair input = %+v, want failed DoD context", repairInputs[0])
	}
	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected || result.DoDAssessment.Satisfied {
		t.Fatalf("result = %+v, want rejected unsatisfied DoD result", result)
	}
	if tickets, err := ticketRepo.ListByRunID(ctx, runID); err != nil || len(tickets) != 0 {
		t.Fatalf("tickets = %+v err=%v, want no repair exhaustion ticket for non-actionable repair assessment", tickets, err)
	}
}
