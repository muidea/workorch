package run

import (
	"context"
	"errors"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPersistReplacementIntentKeepsSuccessorAbsent(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-restart")
	runs := memory.NewRunRepository()
	predecessor := &runmodel.Run{ID: "run-001", WorkspaceID: "ws-restart", State: common.RunStateActive}
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	biz := New()
	if err := biz.Configure(runBizTestDependencies(runs, nil, nil, magicevent.NewHub(8), nil)); err != nil {
		t.Fatal(err)
	}
	bootstrap := restartTestBootstrap(t, predecessor.WorkspaceID)
	intent := runmodel.ReplacementIntent{
		RequestID: "restart-1", WorkspaceID: "ws-restart", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1",
		StartupInputHash: "startup-input-test",
		Bootstrap:        &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		Kind: runtimecontract.RuntimeTerminationSuperseded, Reason: "restart",
	}
	if err := biz.persistReplacementIntent(ctx, predecessor, intent); err != nil {
		t.Fatalf("persist replacement intent: %v", err)
	}
	items, err := runs.ListByWorkspaceID(ctx, predecessor.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != predecessor.ID || items[0].ReplacementIntent == nil || items[0].ReplacementIntent.RequestID != intent.RequestID || items[0].ReplacementIntent.State != runmodel.ReplacementIntentPending {
		t.Fatalf("runs after intent = %+v", items)
	}
	callerSelectedState := intent
	callerSelectedState.State = runmodel.ReplacementIntentPending
	if err := biz.persistReplacementIntent(ctx, predecessor, callerSelectedState); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("caller-selected replacement state err = %v, want invalid argument", err)
	}
	if err := biz.persistReplacementIntent(ctx, predecessor, intent); err != nil {
		t.Fatalf("duplicate replacement intent: %v", err)
	}
	reusedIdentity := intent
	reusedIdentity.Reason = "different restart reason"
	if err := biz.persistReplacementIntent(ctx, predecessor, reusedIdentity); err != common.ErrConflict {
		t.Fatalf("same request with different sealed payload err = %v, want conflict", err)
	}
	conflicting := intent
	conflicting.RequestID = "restart-2"
	if err := biz.persistReplacementIntent(ctx, predecessor, conflicting); err != common.ErrConflict {
		t.Fatalf("conflicting replacement intent err = %v, want conflict", err)
	}
}

func TestPersistReplacementIntentRetiresOnlyRejectedClarificationOnExplicitNewRequest(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-restart")
	runs := memory.NewRunRepository()
	biz := New()
	if err := biz.Configure(runBizTestDependencies(runs, nil, nil, magicevent.NewHub(16), nil)); err != nil {
		t.Fatal(err)
	}
	bootstrap := restartTestBootstrap(t, "ws-restart")
	predecessor := &runmodel.Run{
		ID: "run-001", WorkspaceID: "ws-restart", DefinitionRevision: "def-old", StartupInputHash: "startup-old",
		State: common.RunStateBlockedOnDecision, BlockingTicketID: "runtime-definition-clarification:ticket-1",
	}
	first := replacementIntentFromCommand(runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: predecessor.WorkspaceID, RequestID: "restart-1", GoalID: "goal-1", DoDID: "dod-1",
		DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	})
	first = replacementIntentForPredecessor(first, predecessor)
	first.State = runmodel.ReplacementIntentClarificationResolutionRejected
	first.ClarificationResolutionBindingHash = "scope-mismatch-binding"
	first.ClarificationConflictKind = "scope_mismatch"
	first.Recovery = newReplacementRecoveryLease(first, runmodel.ReplacementRecoveryClarificationResolution, 1, time.Now().UTC().Format(time.RFC3339Nano))
	first.Recovery.State = runmodel.ReplacementRecoveryCompleted
	first.Recovery.DueAt = ""
	predecessor.ReplacementIntent = &first
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}

	second := replacementIntentFromCommand(runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: predecessor.WorkspaceID, RequestID: "restart-2", GoalID: "goal-1", DoDID: "dod-1",
		DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	})
	second = replacementIntentForPredecessor(second, predecessor)
	if err := biz.persistReplacementIntent(ctx, predecessor, second); err != nil {
		t.Fatalf("retire rejected clarification intent: %v", err)
	}
	stored, err := runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ReplacementIntent == nil || stored.ReplacementIntent.RequestID != second.RequestID || len(stored.RetiredReplacementIntents) != 1 || stored.RetiredReplacementIntents[0].RequestID != first.RequestID || stored.RetiredReplacementIntents[0].State != runmodel.ReplacementIntentClarificationResolutionRejected {
		t.Fatalf("replacement retirement lineage = %+v active=%+v", stored.RetiredReplacementIntents, stored.ReplacementIntent)
	}
	reusedRetired := first
	reusedRetired.State = ""
	reusedRetired.Recovery = runmodel.ReplacementRecoveryLease{}
	reusedRetired.ClarificationResolutionBindingHash = ""
	reusedRetired.ClarificationConflictKind = ""
	if err := biz.persistReplacementIntent(ctx, stored, reusedRetired); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("retired RequestID was reused: %v", err)
	}
	third := second
	third.RequestID = "restart-3"
	third.ReplacementIntentID = "runtime-definition-replacement/restart-3"
	third.Reason = "workspace execution restart/restart-3"
	third.ReplacementIntentHash = runmodel.StableReplacementIntentHash(third)
	if err := biz.persistReplacementIntent(ctx, stored, third); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("pending clarification intent was superseded: %v", err)
	}

	boundCtx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-bound-conflict")
	boundPredecessor := &runmodel.Run{
		ID: "run-bound", WorkspaceID: "ws-bound-conflict", DefinitionRevision: "def-old", StartupInputHash: "startup-old",
		State: common.RunStateBlockedOnDecision, BlockingTicketID: "runtime-definition-clarification:ticket-bound",
	}
	boundConflict := replacementIntentFromCommand(runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: boundPredecessor.WorkspaceID, RequestID: "restart-bound-1", GoalID: "goal-1", DoDID: "dod-1",
		DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	})
	boundConflict = replacementIntentForPredecessor(boundConflict, boundPredecessor)
	boundConflict.State = runmodel.ReplacementIntentClarificationResolutionRejected
	boundConflict.ClarificationResolutionBindingHash = "ticket-bound-binding"
	boundConflict.ClarificationConflictKind = "ticket_already_bound"
	boundConflict.ConflictingReplacementIntentID = "runtime-definition-replacement/restart-original"
	boundConflict.ConflictingReplacementIntentHash = "replacement-hash-original"
	boundConflict.Recovery = newReplacementRecoveryLease(boundConflict, runmodel.ReplacementRecoveryClarificationResolution, 1, time.Now().UTC().Format(time.RFC3339Nano))
	boundConflict.Recovery.State = runmodel.ReplacementRecoveryCompleted
	boundConflict.Recovery.DueAt = ""
	boundPredecessor.ReplacementIntent = &boundConflict
	if err := runs.Create(boundCtx, boundPredecessor); err != nil {
		t.Fatal(err)
	}
	retry := replacementIntentFromCommand(runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: boundPredecessor.WorkspaceID, RequestID: "restart-bound-2", GoalID: "goal-1", DoDID: "dod-1",
		DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
	})
	retry = replacementIntentForPredecessor(retry, boundPredecessor)
	if err := biz.persistReplacementIntent(boundCtx, boundPredecessor, retry); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("ticket-already-bound invariant breach was overwritten: %v", err)
	}
}

func TestMaterializeReplacementRequiresTerminalPredecessorAndIsIdempotent(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-restart")
	runs := memory.NewRunRepository()
	bootstrap := restartTestBootstrap(t, "ws-restart")
	intent := runmodel.ReplacementIntent{
		RequestID: "restart-1", WorkspaceID: "ws-restart", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1",
		StartupInputHash: "startup-input-test",
		Bootstrap:        &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		Kind: runtimecontract.RuntimeTerminationSuperseded, Reason: "restart", State: runmodel.ReplacementIntentPending,
	}
	intent.Recovery = newReplacementRecoveryLease(intent, runmodel.ReplacementRecoveryAdministrativeTermination, 1, time.Now().UTC().Format(time.RFC3339Nano))
	predecessor := &runmodel.Run{
		ID: "run-001", WorkspaceID: "ws-restart", GoalID: "goal-old", DoDID: "dod-old", DefinitionRevision: "def-old",
		State: common.RunStateActive, ReplacementIntent: &intent,
		ReconciliationRequestID: "resume-predecessor",
		RuntimeState: &runtimecontract.RuntimeStateRecord{
			StateRevision: 17,
			// Run stores this opaque aggregate marker but must never transfer it
			// into a fresh restart successor.
			ProcessedEventIDs: []string{"cwi-ledger/settled/cwi-1"},
		},
	}
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	biz := New()
	if err := biz.Configure(runBizTestDependencies(runs, nil, nil, magicevent.NewHub(16), nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := biz.materializeReplacementAfterTerminalDelivery(ctx, predecessor, "terminal-1"); err != common.ErrInvalidArgument {
		t.Fatalf("non-terminal predecessor materialized successor: %v", err)
	}
	items, _ := runs.ListByWorkspaceID(ctx, predecessor.WorkspaceID)
	if len(items) != 1 {
		t.Fatalf("successor exists before terminal delivery: %+v", items)
	}
	predecessor.State = common.RunStateSuperseded
	terminal := runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: predecessor.WorkspaceID, RunID: predecessor.ID, TerminalDeliveryID: "terminal-1"}
	predecessor.TerminalDelivery = &terminal
	predecessor.TerminalResultID = "ws-restart/run-001/result/dod-assessment"
	predecessor.Lineage.LineageState = "superseded"
	predecessor.ReplacementIntent.State = runmodel.ReplacementIntentTerminationAcknowledged
	predecessor.ReplacementIntent.TerminalDeliveryID = terminal.TerminalDeliveryID
	predecessor.ReplacementIntent.RuntimeStateRevision = 18
	predecessor.ReplacementIntent.Recovery = nextReplacementRecoveryStage(*predecessor.ReplacementIntent, runmodel.ReplacementRecoverySuccessorMaterialization, time.Now().UTC().Format(time.RFC3339Nano))
	predecessor.ReplacementIntent.Recovery.State = runmodel.ReplacementRecoveryRunning
	predecessor.ReplacementIntent.Recovery.DueAt = ""
	if err := runs.Update(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	created, err := biz.materializeReplacementAfterTerminalDelivery(ctx, predecessor, terminal.TerminalDeliveryID)
	if err != nil {
		t.Fatalf("materialize successor: %v", err)
	}
	created, err = runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, created.ID)
	if err != nil || created.State != common.RunStateActive || created.Lineage.PredecessorRunID != predecessor.ID || created.Lineage.PredecessorResultID != predecessor.TerminalResultID || created.ActivationRequestID != intent.RequestID {
		t.Fatalf("created successor = %+v", created)
	}
	if created.RuntimeState != nil || created.ReconciliationRequestID != "" ||
		created.ReconciliationScope != runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull {
		t.Fatalf("restart successor inherited predecessor Runtime authority: %+v", created)
	}
	again, err := biz.materializeReplacementAfterTerminalDelivery(ctx, predecessor, terminal.TerminalDeliveryID)
	if err != nil || again.ID != created.ID {
		t.Fatalf("duplicate materialization = %+v err=%v", again, err)
	}
	items, _ = runs.ListByWorkspaceID(ctx, predecessor.WorkspaceID)
	if len(items) != 2 {
		t.Fatalf("duplicate successor created: %+v", items)
	}
	reusedIdentity := intent
	changedBootstrap := intent.Bootstrap.Clone()
	changedBootstrap.Activation.ProviderSelection.Target.ModelID = "different-model"
	reusedIdentity.Bootstrap = &changedBootstrap
	if _, err := biz.createWorkspaceExecutionReplacement(ctx, reusedIdentity, predecessor.ID); err != common.ErrConflict {
		t.Fatalf("same activation request with different sealed payload err = %v, want conflict", err)
	}
}

func restartTestBootstrap(t *testing.T, workspaceID common.WorkspaceID) runtimecontract.RuntimeBootstrap {
	t.Helper()
	return restartTestBootstrapWithDefinition(t, workspaceID, "def-1", "startup-input-test")
}

func restartTestBootstrapWithDefinition(t *testing.T, workspaceID common.WorkspaceID, definitionRevision, startupInputHash string) runtimecontract.RuntimeBootstrap {
	t.Helper()
	definition := runtimecontract.RuntimeDefinitionContext{StartupInputHash: startupInputHash,
		Revision: definitionRevision, GoalID: "goal-1", GoalDefinition: "finish goal",
		DoDID: "dod-1", DoDDefinition: "produce evidence", CapabilityContract: []string{runtimecontract.StepKindFileEdit},
	}
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model"}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
			StateChangePolicy:         runtimecontract.RuntimeStateChangeAllowedButNotRequired,
		},
	}
	item := runtimecontract.RuntimeCoverageItem{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dod", SourceID: "dod-1", TargetText: "produce evidence", Order: 1}
	slice := runtimecontract.RuntimeAcceptanceSlice{
		ID: "slice-1", Title: "proof", Scope: "dod", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed,
	}
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(definition, activation, []runtimecontract.RuntimeCoverageItem{item}, []runtimecontract.RuntimeAcceptanceSlice{slice})
	if err != nil {
		t.Fatalf("compile definition seed: %v", err)
	}
	bootstrap := runtimecontract.RuntimeBootstrap{
		Definition: definition, Activation: activation, DefinitionSeed: seed,
		DefinitionFocuses: []runtimecontract.RuntimeDefinitionFocus{{
			ID: "focus-1", SliceID: item.SliceID, AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID),
		}},
		Slices: []runtimecontract.RuntimeAcceptanceSlice{slice}, CoverageItems: []runtimecontract.RuntimeCoverageItem{item},
		ConvergencePolicy: runtimecontract.DefaultRuntimeConvergencePolicy(), RecoveryPolicy: runtimecontract.DefaultRuntimeRecoveryPolicy(),
	}
	bootstrap.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(bootstrap.ConvergencePolicy, bootstrap.RecoveryPolicy)
	_ = workspaceID
	if err := bootstrap.Validate(); err != nil {
		t.Fatalf("restart bootstrap: %v", err)
	}
	return bootstrap
}
