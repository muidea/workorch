package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func prepareProofBudgetBatch(t *testing.T, controller *RuntimeController, aggregate RuntimeAggregate, logical string) (RuntimeAggregate, runtimecontract.RuntimeSemanticPlanBatch) {
	t.Helper()
	manifest, order, _ := semanticPlanningFixture(t, aggregate)
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID, order.BatchSequence, order.BatchCoverageWorkItemIDs, order.Proposals = "", 0, nil, nil
	order.CoverageOrder = []common.CoverageWorkItemID{manifest.CoverageItems[0].ID}
	apply := func(event RuntimeInternalEvent) {
		event.Scope = RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}
		result, err := controller.ApplyInternalEvent(context.Background(), event)
		if err != nil {
			t.Fatal(err)
		}
		aggregate = result.Aggregate
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: "proof-budget-prefix", LogicalRequestID: logical + "-order"}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: logical + "-order"}})
	batch, err := semanticplan.CompileFocusBatch(manifest, order, 0, logical, aggregate.Facts.Focus.CoverageWorkItemIDs, aggregate.SemanticPlanning.PendingCoverageIDs, 1)
	if err != nil {
		t.Fatal(err)
	}
	if RuntimeSemanticPrerequisitesPendingForAggregate(aggregate) {
		before := aggregate.CloneRaw()
		if _, err := controller.compileSemanticPlanBatch(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch); err == nil {
			t.Fatal("proof batch crossed pending owner facts")
		}
		loaded, err := controller.Load(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
		if err != nil || !reflect.DeepEqual(before, loaded) {
			t.Fatalf("blocked batch changed authority/budget/session: %v", err)
		}
		// This is an isolated owner-fact fixture, not a real File producer.
		// Commit the exact target debt before testing the independent budget.
		for _, prerequisite := range aggregate.Facts.Prerequisites.Pending {
			if prerequisite.Kind != runtimecontract.PrerequisiteKindTargetSnapshot {
				t.Fatalf("unsupported budget fixture prerequisite: %+v", prerequisite)
			}
			snapshot := runtimecontract.TargetSnapshot{TargetRef: prerequisite.SubjectRef, Content: "semantic budget fixture", ContentHash: "budget-target/" + prerequisite.SubjectRef, ContentState: runtimecontract.SnapshotContentStateComplete}
			observation := runtimecontract.RuntimePrerequisiteObservation{
				PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: snapshot.ContentHash,
				Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: prerequisite.OwnerModuleID, SubjectRef: prerequisite.SubjectRef, Revision: snapshot.ContentHash}},
			}
			observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
			aggregate, err = controller.commitPrerequisite(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSnapshot: &snapshot, Observation: &observation})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	return aggregate, batch
}

func TestSemanticProofBudgetCompileCASRestartAndDuplicate(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []providerTerminalCASMode{providerTerminalCASNormal, providerTerminalCASFailBeforeWrite, providerTerminalCASLoseResponse} {
		t.Run(string(mode), func(t *testing.T) {
			controller, _, initial := newSemanticPlanningController(t, &runtimecontract.RuntimeExecutionLimits{ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: 32, MaxCapabilityOperations: 16, MaxProofBatchesPerCWI: 1})
			before, batch := prepareProofBudgetBatch(t, controller, initial, "proof-budget")
			repository := &executionLimitsCASRepository{providerTerminalCASRepository: &providerTerminalCASRepository{aggregate: before, mode: mode}, t: t}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			event := RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchCompiled, Scope: RuntimeInternalEventScope{WorkspaceID: before.Facts.WorkspaceID, RunID: before.Facts.RunID}, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanBatch: &batch}}
			_, err = controller.ApplyInternalEvent(ctx, event)
			if (mode == providerTerminalCASNormal) != (err == nil) {
				t.Fatalf("CAS result: %v", err)
			}
			if mode == providerTerminalCASFailBeforeWrite && !reflect.DeepEqual(before, repository.aggregate) {
				t.Fatal("failed CAS leaked a reservation")
			}
			repository.aggregate = reloadExecutionLimitedAggregate(t, repository.aggregate)
			repository.mode = providerTerminalCASNormal
			fresh, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			result, err := fresh.ApplyInternalEvent(ctx, event)
			if err != nil {
				t.Fatal(err)
			}
			if runtimecontract.RuntimeSemanticProofBatchUsage(result.Aggregate.SemanticPlanning)[batch.CoverageWorkItemIDs[0]] != 1 {
				t.Fatal("restart duplicated reservation")
			}
			budget, _ := semanticExecutionBudgetSnapshot(t, result.Aggregate)
			if budget.VisibleProofBatches[0].Used != 1 || *budget.VisibleProofBatches[0].Remaining != 0 {
				t.Fatal("CAS/restart refunded proof budget in planning context")
			}
			writes := repository.saves
			if _, err := fresh.ApplyInternalEvent(ctx, event); err != nil || repository.saves != writes {
				t.Fatalf("duplicate wrote: %v", err)
			}
			stale := batch.Clone()
			stale.RequestLogicalID = "other-request"
			stale.Hash = runtimecontract.StableRuntimeSemanticPlanBatchHash(stale)
			event.Transaction = &RuntimeInternalTransactionPayload{SemanticPlanBatch: &stale}
			if _, err := fresh.ApplyInternalEvent(ctx, event); err == nil || repository.saves != writes {
				t.Fatal("stale batch replay accepted")
			}
			if len(result.Aggregate.CandidateRequestSessions) != 0 || !reflect.DeepEqual(before.ExecutionBudget, result.Aggregate.ExecutionBudget) {
				t.Fatal("compile created a session or physical allocation")
			}
		})
	}
}

func TestSemanticProofBudgetRetiredGenerationExhaustsBeforeNewSession(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate, catalog, prerequisite, observation := sealedSemanticPlanningAggregate(t, &runtimecontract.RuntimeExecutionLimits{ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: 32, MaxCapabilityOperations: 16, MaxProofBatchesPerCWI: 1})
	changed, err := applyReceiptResourceChangeInvalidation(&aggregate, "dispatch-budget", "operation-budget", runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "execution/budget", Fence: 1, OperationID: "operation-budget",
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", BeforeRevision: catalog.ObservedRevision, AfterRevision: "file-revision-2"}},
	})
	if err != nil || !changed {
		t.Fatalf("invalidation: %v", err)
	}
	aggregate.Facts, aggregate.Decision, err = ReduceRuntimeStateWithDirective(aggregate.Facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	repository.aggregate = &aggregate
	catalog.ObservedRevision = "file-revision-2"
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation.ContentHash = catalog.SnapshotID
	observation.Dependencies[0].Revision = catalog.ObservedRevision
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	aggregate, err = controller.commitPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, PrerequisiteResult{PrerequisiteID: prerequisite.ID, OperationAdmissibility: &catalog, Observation: &observation})
	if err != nil {
		t.Fatal(err)
	}
	if len(aggregate.SemanticPlanning.History) != 1 {
		t.Fatal("prior generation not retired")
	}
	retirement, found := NextRuntimeCWIBindingRetirementFact(aggregate)
	if !found {
		t.Fatal("missing owner binding retirement")
	}
	retired, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventCWIBindingRetired, Scope: RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{CWIBindingRetirement: &retirement},
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate = retired.Aggregate
	before, batch := prepareProofBudgetBatch(t, controller, aggregate, "new-manifest-new-request")
	budget, _ := semanticExecutionBudgetSnapshot(t, before)
	if budget.VisibleProofBatches[0].Used != 1 || *budget.VisibleProofBatches[0].Remaining != 0 {
		t.Fatal("retired generation disappeared from planning budget")
	}
	store := &executionLimitsCASRepository{providerTerminalCASRepository: &providerTerminalCASRepository{aggregate: reloadExecutionLimitedAggregate(t, before)}, t: t}
	fresh, err := NewRuntimeController(store)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		_, err := fresh.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchCompiled, Scope: RuntimeInternalEventScope{WorkspaceID: before.Facts.WorkspaceID, RunID: before.Facts.RunID}, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanBatch: &batch}})
		var exhausted *runtimecontract.RuntimeBudgetExhaustedError
		if !errors.As(err, &exhausted) || exhausted.Dimension != runtimecontract.RuntimeBudgetDimensionSemanticGeneration {
			t.Fatalf("expected proof exhaustion: %v", err)
		}
		if store.saves != 0 || !reflect.DeepEqual(before, store.aggregate) {
			t.Fatal("exhaustion wrote batch/session/allocation or authority")
		}
	}
}
