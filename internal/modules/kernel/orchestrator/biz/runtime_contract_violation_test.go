package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestContractViolationRecoveryActionRetriesWithinBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-retry")
	repo := memory.NewStepRepository()
	failed := contractViolationFailedStep(runID, "failed-001")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-retry", InitialStepKind: llmWorkspaceExecutionStepKind}
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{}), true, true, run, failed)

	if action != runtimeRecoveryActionContractViolationRetry {
		t.Fatalf("recovery action = %s, want contract violation retry", action)
	}
}

func TestContractViolationRecoveryActionExhaustsIndependentBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-exhausted")
	repo := memory.NewStepRepository()
	failed := contractViolationFailedStep(runID, "failed-001")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-exhausted", InitialStepKind: llmWorkspaceExecutionStepKind}
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	for attempt := 1; attempt <= 2; attempt++ {
		step := &runmodel.Step{
			ID:    contractViolationRetryStepID(runID, failed, attempt),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyContractViolationRecovery: map[string]any{
					"sourceStepId": string(failed.ID),
				},
			},
			Output: map[string]any{
				failureOutputKeyClass:     failureClassRuntime,
				failureOutputKeySignature: "class=runtime_failed|stage=runtime_contract_failed|rule=state_changing_required_missing",
			},
		}
		if err := repo.Create(ctx, step); err != nil {
			t.Fatalf("seed retry step %d: %v", attempt, err)
		}
	}

	action := runtimeRecoveryActionForFailedStep(ctx, newRepairSubsystem(repo, nil), NormalizeRuntimePolicy(RuntimePolicy{MaxContractViolationRetriesPerStep: 2}), true, true, run, failed)

	if action != runtimeRecoveryActionContractViolationExhausted {
		t.Fatalf("recovery action = %s, want contract violation exhausted", action)
	}
}

func TestRuntimeRepairInputIncludesContractViolationRecovery(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-input")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-contract-violation-input", InitialStepKind: llmWorkspaceExecutionStepKind}
	failed := contractViolationFailedStep(runID, "failed-001")
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	orch := &RuntimeOrchestrator{
		steps:  memory.NewStepRepository(),
		policy: NormalizeRuntimePolicy(RuntimePolicy{}),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failed, 1, common.ErrInvalidArgument)

	recovery := mapFromValue(input[stepInputKeyContractViolationRecovery])
	if stringFromMap(recovery, "violatedRule") != contractViolationRuleStateChangingRequiredMissing {
		t.Fatalf("contractViolationRecovery = %+v, want state-changing rule", recovery)
	}
	if recovery["attempt"] != 1 || recovery["maxAttempts"] != NormalizeRuntimePolicy(RuntimePolicy{}).MaxContractViolationRetriesPerStep {
		t.Fatalf("contractViolationRecovery budget = %+v, want attempt/max", recovery)
	}
	expected := mapFromValue(recovery["expectedContract"])
	if expected["stateChangingOutputRequired"] != true {
		t.Fatalf("expectedContract = %+v, want stateChangingOutputRequired", expected)
	}
	if input[stepInputKeyRecoveryLineageType] != "contract_violation_retry" {
		t.Fatalf("recovery lineage = %v, want contract_violation_retry", input[stepInputKeyRecoveryLineageType])
	}
	if !strings.Contains(strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n"), "contractViolationRecovery.expectedContract") {
		t.Fatalf("repairGuidance = %+v, want contract violation guidance", input["repairGuidance"])
	}
}

func TestContractViolationEventsAreRegisteredAsStateNotifications(t *testing.T) {
	contracts := orchestratorevents.Contracts()
	for _, topic := range []string{
		orchestratorevents.TopicOrchestratorContractViolationRecorded,
		orchestratorevents.TopicOrchestratorContractViolationRetryRequested,
		orchestratorevents.TopicOrchestratorContractViolationEscalated,
	} {
		found := false
		for _, contract := range contracts {
			if contract.Topic == topic {
				found = true
				if contract.CommandClass != eventcontract.CommandClassNotify || contract.LanePolicy != eventcontract.LanePolicyState {
					t.Fatalf("contract for %s = %+v, want notify state", topic, contract)
				}
			}
		}
		if !found {
			t.Fatalf("missing event contract for %s", topic)
		}
	}
}

func TestContractViolationRecoveryPublishesRecordedRetryAndDispatchEvents(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-events")
	workspaceID := common.WorkspaceID("ws-contract-violation-events")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	failed := contractViolationFailedStep(runID, "failed-events")
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := stepRepo.Create(ctx, failed); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{}, runRepo, stepRepo, workspaceRepo, memory.NewDecisionTicketRepository(), memory.NewApprovalRepository(), hub)
	orch.EnableEventDrivenDispatch()

	if err := orch.eventPublisher().publishContractViolationRecorded(ctx, run, failed); err != nil {
		t.Fatalf("publish recorded: %v", err)
	}
	if err := orch.dispatchRuntimeRepairFollowUp(ctx, run, failed, common.ErrInvalidArgument); err != nil {
		t.Fatalf("dispatchRuntimeRepairFollowUp: %v", err)
	}

	recorded := publishedTopic(hub.Published(), orchestratorevents.TopicOrchestratorContractViolationRecorded)
	if recorded == nil {
		t.Fatalf("published events = %+v, want contract violation recorded", hub.Published())
	}
	assertContractViolationEventScope(t, *recorded, workspaceID, runID, failed.ID)
	if got := stringFromMap(recorded.Payload, "violatedRule"); got != contractViolationRuleStateChangingRequiredMissing {
		t.Fatalf("recorded payload rule = %q, want %s", got, contractViolationRuleStateChangingRequiredMissing)
	}

	retry := publishedTopic(hub.Published(), orchestratorevents.TopicOrchestratorContractViolationRetryRequested)
	if retry == nil {
		t.Fatalf("published events = %+v, want contract violation retry requested", hub.Published())
	}
	assertContractViolationEventScope(t, *retry, workspaceID, runID, failed.ID)
	if retry.Payload["retryStepId"] == "" {
		t.Fatalf("retry payload = %+v, want retryStepId", retry.Payload)
	}
	if got := intFromAny(retry.Payload["attempt"]); got != 1 {
		t.Fatalf("retry attempt = %d, want 1", got)
	}

	dispatch := publishedTopic(hub.Published(), orchestratorevents.TopicOrchestratorDispatchStepRequested)
	if dispatch == nil {
		t.Fatalf("published events = %+v, want dispatch requested", hub.Published())
	}
	if dispatch.WorkspaceID != string(workspaceID) || dispatch.RunID != string(runID) || dispatch.StepID == "" {
		t.Fatalf("dispatch event = %+v, want scoped workspace/run/retry step", *dispatch)
	}
}

func TestContractViolationEscalatedEventPayloadIsScoped(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-contract-violation-escalated-event")
	workspaceID := common.WorkspaceID("ws-contract-violation-escalated-event")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID}
	failed := contractViolationFailedStep(runID, "failed-escalated")
	attachContractViolationDiagnostics(run, failed, errors.New("state-changing llm repair step returned no state-changing output"))
	hub := events.NewMemoryHub()
	publisher := newEventSubsystem(hub, nil, nil)
	violation := contractViolationFromFailedOutput(failed)
	context := map[string]any{"attempts": 2, "runAttempts": 3}

	if err := publisher.publishContractViolationEscalated(ctx, run, failed, "ticket-contract-violation", violation, context); err != nil {
		t.Fatalf("publish escalated: %v", err)
	}

	escalated := publishedTopic(hub.Published(), orchestratorevents.TopicOrchestratorContractViolationEscalated)
	if escalated == nil {
		t.Fatalf("published events = %+v, want contract violation escalated", hub.Published())
	}
	assertContractViolationEventScope(t, *escalated, workspaceID, runID, failed.ID)
	if got := stringFromMap(escalated.Payload, "ticketId"); got != "ticket-contract-violation" {
		t.Fatalf("escalated payload = %+v, want ticket id", escalated.Payload)
	}
	if got := intFromAny(mapFromValue(escalated.Payload["context"])["attempts"]); got != 2 {
		t.Fatalf("escalated context = %+v, want attempts", escalated.Payload["context"])
	}
}

func publishedTopic(items []events.Envelope, topic string) *events.Envelope {
	for idx := range items {
		if items[idx].Topic == topic {
			return &items[idx]
		}
	}
	return nil
}

func assertContractViolationEventScope(t *testing.T, envelope events.Envelope, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID) {
	t.Helper()
	if envelope.WorkspaceID != string(workspaceID) || envelope.RunID != string(runID) || envelope.StepID != string(stepID) {
		t.Fatalf("event scope = %+v, want workspace=%s run=%s step=%s", envelope, workspaceID, runID, stepID)
	}
	if got := stringFromMap(envelope.Payload, "workspaceId"); got != string(workspaceID) {
		t.Fatalf("payload workspaceId = %q, want %s", got, workspaceID)
	}
	if got := stringFromMap(envelope.Payload, "runId"); got != string(runID) {
		t.Fatalf("payload runId = %q, want %s", got, runID)
	}
	if got := stringFromMap(envelope.Payload, "sourceStepId"); got != string(stepID) {
		t.Fatalf("payload sourceStepId = %q, want %s", got, stepID)
	}
	if len(mapFromValue(envelope.Payload["contractViolation"])) == 0 {
		t.Fatalf("payload = %+v, want nested contractViolation", envelope.Payload)
	}
}

func contractViolationFailedStep(runID common.RunID, label string) *runmodel.Step {
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/" + label),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
				},
			},
		},
		Output: map[string]any{
			"stage":                        "runtime_contract_failed",
			"error":                        "state-changing llm repair step returned no state-changing output",
			outputKeyContractViolationType: contractViolationRepairNoStateChangingOutput,
			failureOutputKeyClass:          failureClassRuntime,
			failureOutputKeySignature:      "class=runtime_failed|stage=runtime_contract_failed|rule=state_changing_required_missing",
		},
	}
	return step
}
