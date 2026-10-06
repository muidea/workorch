package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 私有 seal 产生的原 physical claim；该回归不代替公开 activation 或 fresh-process 验收。
func implementationWorkUnknownLateKnown(t *testing.T, sealed RuntimeAggregate) {
	t.Helper()
	data, err := json.Marshal(sealed)
	if err != nil {
		t.Fatal(err)
	}
	var initial RuntimeAggregate
	if err := json.Unmarshal(data, &initial); err != nil {
		t.Fatal(err)
	}
	repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &initial}}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	scope := RuntimeInternalEventScope{WorkspaceID: initial.Facts.WorkspaceID, RunID: initial.Facts.RunID}
	unstartedOperation := initial.ActiveDispatch.OperationExecutions[0].OperationID
	if _, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventOperationExecutionOutcomeUnknownObserved, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{OperationOutcomeUnknownID: unstartedOperation}}); err == nil || repository.saves != 0 {
		t.Fatalf("unstarted operation fabricated unknown physical claim: %v", err)
	}
	unchanged, err := controller.Load(ctx, scope.WorkspaceID, scope.RunID)
	if err != nil || !reflect.DeepEqual(unchanged, initial) {
		t.Fatalf("unstarted unknown event changed aggregate: %v", err)
	}
	call, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventDispatchExecutionRequested, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{}})
	if err != nil || call.OperationCall == nil {
		t.Fatalf("original physical claim: %v", err)
	}
	execution := call.OperationCall.Request.Execution
	dispatchID := call.Aggregate.ActiveDispatch.ID
	unknown, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventOperationExecutionOutcomeUnknownObserved, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{OperationOutcomeUnknownID: execution.OperationID}})
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Aggregate.ActiveDispatch != nil || unknown.Aggregate.ImplementationWork.Rounds[0].OutcomeUnknownExecutionKey != execution.ExecutionKey {
		t.Fatal("unknown claim did not archive original work authority")
	}
	budget := unknown.Aggregate.ExecutionBudget.Clone()
	if _, found, err := controller.QueryOperationReceipt(ctx, scope.WorkspaceID, scope.RunID, dispatchID, execution.OperationID, execution.ExecutionKey, execution.Fence, runtimecontract.RuntimeModuleFile, nil); err == nil || found {
		t.Fatal("archived unknown claim granted a new owner execution before durable receipt")
	}
	for _, marker := range []string{"", runtimecontract.RuntimeOperationClaimMarkerGovernanceGranted} {
		data, err := json.Marshal(unknown.Aggregate)
		if err != nil {
			t.Fatal(err)
		}
		var invalid RuntimeAggregate
		if err := json.Unmarshal(data, &invalid); err != nil {
			t.Fatal(err)
		}
		for i := range invalid.CompletedDispatches[0].OperationExecutions {
			record := &invalid.CompletedDispatches[0].OperationExecutions[i]
			if record.ExecutionKey == execution.ExecutionKey {
				record.ClaimedAt = marker
			}
		}
		if err := invalid.Validate(); err == nil {
			t.Fatal("archived unknown execution without started claim survived original-source gate")
		}
	}
	dependency := unknown.Aggregate.ImplementationWork.Rounds[0].WorkingBinding.CatalogObservation.Dependencies[0]
	receipt := runtimecontract.RuntimeOperationReceipt{OperationID: execution.OperationID, ExecutionKey: execution.ExecutionKey, Fence: execution.Fence,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true,
		TargetRefs: call.OperationCall.Request.Operation.TargetRefs, BeforeHash: "before", AfterHash: "late-after", Evidence: runtimecontract.RuntimeOperationEvidence{ContentHash: "late-after", Quality: common.EvidenceQualityValid},
		ResourceChanges: []runtimecontract.RuntimeResourceChange{{Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, BeforeRevision: dependency.Revision, AfterRevision: "late-tree"}},
	}
	event := RuntimeInternalEvent{Kind: RuntimeInternalEventOperationReceiptObserved, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{DispatchID: dispatchID, Fence: receipt.Fence, OperationReceiptOwnerSource: runtimecontract.RuntimeModuleFile, OperationReceipt: &receipt}}
	for _, test := range []struct {
		name   string
		mutate func(*RuntimeInternalEvent)
	}{
		{"wrong-owner", func(value *RuntimeInternalEvent) {
			value.Transaction.OperationReceiptOwnerSource = runtimecontract.RuntimeModuleShell
		}},
		{"wrong-dispatch", func(value *RuntimeInternalEvent) { value.Transaction.DispatchID += "-other" }},
		{"wrong-key", func(value *RuntimeInternalEvent) { value.Transaction.OperationReceipt.ExecutionKey += "-other" }},
		{"wrong-fence", func(value *RuntimeInternalEvent) {
			value.Transaction.Fence++
			value.Transaction.OperationReceipt.Fence++
		}},
		{"wrong-operation", func(value *RuntimeInternalEvent) { value.Transaction.OperationReceipt.OperationID += "-other" }},
		{"wrong-target", func(value *RuntimeInternalEvent) {
			value.Transaction.OperationReceipt.TargetRefs = []string{"__unsealed_receipt_target__.txt"}
		}},
	} {
		t.Run("unknown-late/"+test.name, func(t *testing.T) {
			invalid := event
			payload := *event.Transaction
			original := receipt.Clone()
			payload.OperationReceipt = &original
			invalid.Transaction = &payload
			test.mutate(&invalid)
			before := repository.saves
			if _, err := controller.ApplyInternalEvent(ctx, invalid); err == nil || repository.saves != before {
				t.Fatalf("invalid original callback admitted: %v", err)
			}
			persisted, err := controller.Load(ctx, scope.WorkspaceID, scope.RunID)
			if err != nil || !reflect.DeepEqual(persisted, unknown.Aggregate) {
				t.Fatalf("invalid callback changed original aggregate: %v", err)
			}
		})
	}
	for _, loss := range []string{"before", "after"} {
		t.Run("unknown-late/CAS-"+loss, func(t *testing.T) {
			data, err := json.Marshal(unknown.Aggregate)
			if err != nil {
				t.Fatal(err)
			}
			var initial RuntimeAggregate
			if err := json.Unmarshal(data, &initial); err != nil {
				t.Fatal(err)
			}
			repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &initial}}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			repository.failBefore, repository.failAfter = loss == "before", loss == "after"
			if _, err := controller.ApplyInternalEvent(ctx, event); err == nil {
				t.Fatal("CAS loss was not observed")
			}
			persisted, err := controller.Load(ctx, scope.WorkspaceID, scope.RunID)
			if err != nil || !reflect.DeepEqual(persisted.ExecutionBudget, budget) {
				t.Fatalf("CAS loss altered unknown consumption: %v", err)
			}
			if loss == "before" && !reflect.DeepEqual(persisted, unknown.Aggregate) {
				t.Fatal("failed CAS leaked partial receipt/invalidation")
			}
			if loss == "after" && (repository.saves != 1 || len(persisted.OperationReceipts) != len(unknown.Aggregate.OperationReceipts)+1) {
				t.Fatal("CAS response loss lost original owner receipt")
			}
			reopened, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			before := repository.saves
			result, err := reopened.ApplyInternalEvent(ctx, event)
			wantSaves := before
			if loss == "before" {
				wantSaves++
			}
			if err != nil || repository.saves != wantSaves || !reflect.DeepEqual(result.Aggregate.ExecutionBudget, budget) {
				t.Fatalf("same-process reopen did not preserve exact callback: %v", err)
			}
		})
	}
	implementationWorkUnknownLateReceiptAcrossProcesses(t, unknown.Aggregate, event)
	before := repository.saves
	settled, err := controller.ApplyInternalEvent(ctx, event)
	if err != nil || repository.saves != before+1 {
		t.Fatalf("original unknown claim late-known callback single CAS: %v", err)
	}
	if !reflect.DeepEqual(settled.Aggregate.ExecutionBudget, budget) || settled.Aggregate.ActiveDispatch != nil || settled.Aggregate.ImplementationWork.Rounds[0].CompletedReceiptSetHash != "" || settled.Aggregate.ImplementationWork.Rounds[0].OutcomeUnknownExecutionKey != execution.ExecutionKey {
		t.Fatal("late-known receipt reopened work or released unknown physical consumption")
	}
	if !reflect.DeepEqual(settled.Aggregate.Facts.Evidence, unknown.Aggregate.Facts.Evidence) || !reflect.DeepEqual(settled.Aggregate.Facts.Coverage, unknown.Aggregate.Facts.Coverage) || !reflect.DeepEqual(settled.Aggregate.ImplementationWork, unknown.Aggregate.ImplementationWork) || !reflect.DeepEqual(settled.Aggregate.Decision, unknown.Aggregate.Decision) {
		t.Fatal("late-known owner fact granted effect or terminal credit")
	}
	invalidationMatches := 0
	for _, invalidation := range settled.Aggregate.PrerequisiteInvalidations {
		if invalidation.DispatchID == dispatchID && invalidation.OperationID == execution.OperationID && invalidation.ReceiptHash == runtimecontract.StableRuntimeOperationReceiptHash(receipt) && reflect.DeepEqual(invalidation.ResourceChanges, receipt.ResourceChanges) {
			invalidationMatches++
		}
	}
	if invalidationMatches != 1 {
		t.Fatal("late-known resource changes were not retired in original receipt CAS")
	}
	original, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(settled.Aggregate.OperationReceipts, receipt.ExecutionKey, receipt.Fence)
	if !found || !reflect.DeepEqual(original, receipt.Normalized()) {
		t.Fatal("late-known owner receipt was lost or rewritten")
	}
	fresh, err := controller.Load(ctx, scope.WorkspaceID, scope.RunID)
	if err != nil || !reflect.DeepEqual(fresh, settled.Aggregate) {
		t.Fatalf("late-known full aggregate reload: %v", err)
	}
	if err := runtimeStateRecordForTerminalNonterminalRecoveryTest(t, fresh).Validate(); err != nil {
		t.Fatalf("late-known opaque state full validation: %v", err)
	}
	before = repository.saves
	if _, err := controller.ApplyInternalEvent(ctx, event); err != nil || repository.saves != before {
		t.Fatalf("late-known duplicate mutated aggregate: %v", err)
	}
	conflicting := receipt.Clone()
	conflicting.AfterHash = "conflicting-after"
	payload := *event.Transaction
	payload.OperationReceipt = &conflicting
	conflict := event
	conflict.Transaction = &payload
	if _, err := controller.ApplyInternalEvent(ctx, conflict); err == nil || repository.saves != before {
		t.Fatalf("conflicting late-known receipt replaced original: %v", err)
	}
	replayed, found, err := controller.QueryOperationReceipt(ctx, scope.WorkspaceID, scope.RunID, dispatchID, execution.OperationID, execution.ExecutionKey, execution.Fence, runtimecontract.RuntimeModuleFile, nil)
	if err != nil || !found || !reflect.DeepEqual(replayed, receipt.Normalized()) {
		t.Fatalf("archived query lost original owner fact: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*RuntimeAggregate)
	}{
		{"missing-original-receipt", func(value *RuntimeAggregate) { value.OperationReceipts = nil }},
		{"fabricated-credit", func(value *RuntimeAggregate) {
			for i := range value.CompletedDispatches[0].OperationExecutions {
				record := &value.CompletedDispatches[0].OperationExecutions[i]
				if record.ExecutionKey == execution.ExecutionKey {
					record.State = runtimecontract.OperationExecutionEvidenceCommitted
					record.EvidenceFactID, record.EvidenceFactHash = "fake-credit", "fake-credit-hash"
				}
			}
		}},
		{"missing-unknown-allocation", func(value *RuntimeAggregate) {
			for i, allocation := range value.ExecutionBudget.Allocations {
				if allocation.LogicalConsumptionID == execution.ExecutionKey {
					value.ExecutionBudget.Allocations = append(value.ExecutionBudget.Allocations[:i], value.ExecutionBudget.Allocations[i+1:]...)
					return
				}
			}
		}},
		{"late-receipt-confirms-consumption", func(value *RuntimeAggregate) {
			for i := range value.ExecutionBudget.Consumptions {
				consumption := &value.ExecutionBudget.Consumptions[i]
				if consumption.LogicalConsumptionID == execution.ExecutionKey {
					consumption.ConfirmedAmount, consumption.UnknownAmount = 1, 0
					consumption.OwnerResultHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
					consumption.ConsumptionHash = runtimecontract.StableRuntimeBudgetConsumptionHash(*consumption)
				}
			}
			for i := range value.ExecutionBudget.Allocations {
				allocation := &value.ExecutionBudget.Allocations[i]
				if allocation.LogicalConsumptionID == execution.ExecutionKey {
					allocation.State = runtimecontract.RuntimeBudgetAllocationConfirmed
					allocation.AllocationHash = runtimecontract.StableRuntimeBudgetAllocationHash(*allocation)
				}
			}
			if err := value.ExecutionBudget.Validate(); err != nil {
				t.Fatalf("coordinated budget tamper must pass budget shape before source rejection: %v", err)
			}
		}},
	} {
		t.Run("unknown-late/reload-tamper/"+test.name, func(t *testing.T) {
			data, err := json.Marshal(fresh)
			if err != nil {
				t.Fatal(err)
			}
			var invalid RuntimeAggregate
			if err := json.Unmarshal(data, &invalid); err != nil {
				t.Fatal(err)
			}
			test.mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatal("tampered late-known authority survived full aggregate gate")
			}
			if err := runtimeStateRecordForTerminalNonterminalRecoveryTest(t, invalid).Validate(); err == nil {
				t.Fatal("tampered late-known authority survived opaque state gate")
			}
		})
	}
}
