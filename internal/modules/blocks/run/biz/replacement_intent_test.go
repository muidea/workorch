package run

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
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
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: magicevent.NewHub(8)}); err != nil {
		t.Fatal(err)
	}
	bootstrap := restartTestBootstrap(t, predecessor.WorkspaceID)
	intent := runmodel.ReplacementIntent{
		RequestID: "restart-1", WorkspaceID: "ws-restart", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1",
		Bootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		Kind: runtimecontract.RuntimeTerminationSuperseded, Reason: "restart", State: runmodel.ReplacementIntentPending,
	}
	if err := biz.persistReplacementIntent(ctx, predecessor, intent); err != nil {
		t.Fatalf("persist replacement intent: %v", err)
	}
	items, err := runs.ListByWorkspaceID(ctx, predecessor.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != predecessor.ID || items[0].ReplacementIntent == nil || items[0].ReplacementIntent.RequestID != intent.RequestID {
		t.Fatalf("runs after intent = %+v", items)
	}
	if err := biz.persistReplacementIntent(ctx, predecessor, intent); err != nil {
		t.Fatalf("duplicate replacement intent: %v", err)
	}
	reusedIdentity := intent
	reusedIdentity.Priority = intent.Priority + 1
	if err := biz.persistReplacementIntent(ctx, predecessor, reusedIdentity); err != common.ErrConflict {
		t.Fatalf("same request with different sealed payload err = %v, want conflict", err)
	}
	conflicting := intent
	conflicting.RequestID = "restart-2"
	if err := biz.persistReplacementIntent(ctx, predecessor, conflicting); err != common.ErrConflict {
		t.Fatalf("conflicting replacement intent err = %v, want conflict", err)
	}
}

func TestMaterializeReplacementRequiresTerminalPredecessorAndIsIdempotent(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-restart")
	runs := memory.NewRunRepository()
	bootstrap := restartTestBootstrap(t, "ws-restart")
	intent := runmodel.ReplacementIntent{
		RequestID: "restart-1", WorkspaceID: "ws-restart", GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1",
		Bootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		Kind: runtimecontract.RuntimeTerminationSuperseded, Reason: "restart", State: runmodel.ReplacementIntentPending,
	}
	predecessor := &runmodel.Run{
		ID: "run-001", WorkspaceID: "ws-restart", GoalID: "goal-old", DoDID: "dod-old", DefinitionRevision: "def-old",
		State: common.RunStateActive, ReplacementIntent: &intent,
		ReconciliationRequestID: "resume-predecessor",
		RuntimeState: &runtimecontract.RuntimeStateRecord{
			SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
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
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: magicevent.NewHub(16)}); err != nil {
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
	if err := runs.Update(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	created, err := biz.materializeReplacementAfterTerminalDelivery(ctx, predecessor, terminal.TerminalDeliveryID)
	if err != nil {
		t.Fatalf("materialize successor: %v", err)
	}
	created, err = runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, created.ID)
	if err != nil || created.State != common.RunStateActive || created.Lineage.PredecessorRunID != predecessor.ID || created.ActivationRequestID != intent.RequestID {
		t.Fatalf("created successor = %+v", created)
	}
	if created.RuntimeState != nil || created.RuntimeActivationStatus != nil || created.ReconciliationRequestID != "" ||
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
	reusedIdentity.Priority = intent.Priority + 1
	if _, err := biz.createWorkspaceExecutionReplacement(ctx, reusedIdentity, predecessor.ID); err != common.ErrConflict {
		t.Fatalf("same activation request with different sealed payload err = %v, want conflict", err)
	}
}

func restartTestBootstrap(t *testing.T, workspaceID common.WorkspaceID) runtimecontract.RuntimeBootstrap {
	t.Helper()
	definition := runtimecontract.RuntimeDefinitionContext{
		Revision: "def-1", GoalID: "goal-1", GoalDefinition: "finish goal",
		DoDID: "dod-1", DoDDefinition: "produce evidence", CapabilityContract: []string{runtimecontract.StepKindFileEdit},
	}
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model"}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{OperationContractRevision: runtimecontract.RuntimeOperationContractRevision()},
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
		SchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent, Definition: definition, Activation: activation, DefinitionSeed: seed,
		Focuses: []runtimecontract.CoverageFocus{{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}},
		Slices:  []runtimecontract.RuntimeAcceptanceSlice{slice}, CoverageItems: []runtimecontract.RuntimeCoverageItem{item},
		ConvergencePolicy: runtimecontract.DefaultRuntimeConvergencePolicy(), RecoveryPolicy: runtimecontract.DefaultRuntimeRecoveryPolicy(),
	}
	_ = workspaceID
	if err := bootstrap.Validate(); err != nil {
		t.Fatalf("restart bootstrap: %v", err)
	}
	return bootstrap
}
